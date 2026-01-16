"""Supabase StateStore - Cloud-based state storage with real-time sync."""

from __future__ import annotations

import os
from datetime import datetime
from typing import TYPE_CHECKING, Any, Callable

from .exceptions import StateNotFoundError
from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.models import C4State


class SupabaseStateStore(StateStore, LockStore):
    """
    Supabase-backed state storage with real-time subscriptions.

    Features:
    - PostgreSQL-backed persistent storage
    - Real-time state synchronization
    - Row Level Security for team isolation
    - Automatic timestamps

    Environment Variables:
        SUPABASE_URL: Supabase project URL
        SUPABASE_KEY: Supabase anon/service key

    Example:
        store = SupabaseStateStore()
        state = store.load("my-project")
        state.status = ProjectStatus.EXECUTE
        store.save(state)
    """

    TABLE_STATE = "c4_state"
    TABLE_LOCKS = "c4_locks"

    def __init__(
        self,
        url: str | None = None,
        key: str | None = None,
        realtime: bool = True,
    ):
        """Initialize Supabase store.

        Args:
            url: Supabase project URL (or SUPABASE_URL env)
            key: Supabase key (or SUPABASE_KEY env)
            realtime: Enable real-time subscriptions
        """
        self._url = url or os.environ.get("SUPABASE_URL", "")
        self._key = key or os.environ.get("SUPABASE_KEY", "")
        self._realtime_enabled = realtime
        self._client: Any = None
        self._subscriptions: dict[str, Any] = {}
        self._callbacks: dict[str, list[Callable[[C4State], None]]] = {}

    @property
    def client(self) -> Any:
        """Lazy-initialize Supabase client."""
        if self._client is None:
            if not self._url or not self._key:
                raise ValueError(
                    "Supabase URL and key required. "
                    "Set SUPABASE_URL and SUPABASE_KEY environment variables."
                )
            from supabase import create_client

            self._client = create_client(self._url, self._key)
        return self._client

    def load(self, project_id: str) -> "C4State":
        """Load state from Supabase.

        Args:
            project_id: Project identifier

        Returns:
            C4State instance

        Raises:
            StateNotFoundError: If project not found
        """
        response = (
            self.client.table(self.TABLE_STATE)
            .select("*")
            .eq("project_id", project_id)
            .maybe_single()
            .execute()
        )

        if not response.data:
            raise StateNotFoundError(project_id)

        return self._row_to_state(response.data)

    def save(self, state: "C4State") -> None:
        """Save state to Supabase.

        Args:
            state: State to persist
        """
        state.updated_at = datetime.now()
        data = self._state_to_row(state)

        self.client.table(self.TABLE_STATE).upsert(
            data, on_conflict="project_id"
        ).execute()

    def exists(self, project_id: str) -> bool:
        """Check if project exists in Supabase."""
        response = (
            self.client.table(self.TABLE_STATE)
            .select("project_id")
            .eq("project_id", project_id)
            .maybe_single()
            .execute()
        )
        return response.data is not None

    def delete(self, project_id: str) -> None:
        """Delete project state from Supabase."""
        self.client.table(self.TABLE_STATE).delete().eq(
            "project_id", project_id
        ).execute()

    # =========================================================================
    # Real-time Subscriptions
    # =========================================================================

    def subscribe(
        self,
        project_id: str,
        callback: Callable[["C4State"], None],
    ) -> str:
        """Subscribe to state changes for a project.

        Args:
            project_id: Project to watch
            callback: Function called on state change

        Returns:
            Subscription ID for unsubscribe
        """
        if not self._realtime_enabled:
            raise RuntimeError("Real-time disabled for this store")

        # Track callback
        if project_id not in self._callbacks:
            self._callbacks[project_id] = []
        self._callbacks[project_id].append(callback)

        # Create subscription if not exists
        if project_id not in self._subscriptions:
            channel = (
                self.client.channel(f"state:{project_id}")
                .on_postgres_changes(
                    event="*",
                    schema="public",
                    table=self.TABLE_STATE,
                    filter=f"project_id=eq.{project_id}",
                    callback=lambda payload: self._handle_change(
                        project_id, payload
                    ),
                )
                .subscribe()
            )
            self._subscriptions[project_id] = channel

        return f"{project_id}:{id(callback)}"

    def unsubscribe(self, subscription_id: str) -> None:
        """Unsubscribe from state changes.

        Args:
            subscription_id: ID from subscribe()
        """
        project_id, callback_id = subscription_id.split(":", 1)
        callback_id_int = int(callback_id)

        if project_id in self._callbacks:
            self._callbacks[project_id] = [
                cb
                for cb in self._callbacks[project_id]
                if id(cb) != callback_id_int
            ]

            # Remove channel if no more callbacks
            if not self._callbacks[project_id]:
                if project_id in self._subscriptions:
                    self._subscriptions[project_id].unsubscribe()
                    del self._subscriptions[project_id]
                del self._callbacks[project_id]

    def _handle_change(self, project_id: str, payload: dict[str, Any]) -> None:
        """Handle real-time change event."""
        if project_id not in self._callbacks:
            return

        try:
            new_data = payload.get("new") or payload.get("record")
            if new_data:
                state = self._row_to_state(new_data)
                for callback in self._callbacks[project_id]:
                    callback(state)
        except Exception:
            pass  # Silently ignore parse errors

    # =========================================================================
    # Lock Store Implementation
    # =========================================================================

    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Acquire a scope lock using Supabase."""
        from datetime import timedelta

        expires_at = datetime.now() + timedelta(seconds=ttl_seconds)

        # Try to insert or update if expired
        try:
            self.client.table(self.TABLE_LOCKS).upsert(
                {
                    "project_id": project_id,
                    "scope": scope,
                    "owner": owner,
                    "expires_at": expires_at.isoformat(),
                },
                on_conflict="project_id,scope",
            ).execute()
            return True
        except Exception:
            return False

    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """Release a scope lock."""
        try:
            self.client.table(self.TABLE_LOCKS).delete().eq(
                "project_id", project_id
            ).eq("scope", scope).execute()
            return True
        except Exception:
            return False

    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Refresh lock TTL if owned."""
        from datetime import timedelta

        expires_at = datetime.now() + timedelta(seconds=ttl_seconds)

        try:
            response = (
                self.client.table(self.TABLE_LOCKS)
                .update({"expires_at": expires_at.isoformat()})
                .eq("project_id", project_id)
                .eq("scope", scope)
                .eq("owner", owner)
                .execute()
            )
            return len(response.data) > 0
        except Exception:
            return False

    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """Get current lock holder and expiry."""
        response = (
            self.client.table(self.TABLE_LOCKS)
            .select("owner, expires_at")
            .eq("project_id", project_id)
            .eq("scope", scope)
            .maybe_single()
            .execute()
        )

        if not response.data:
            return None

        expires_at = datetime.fromisoformat(
            response.data["expires_at"].replace("Z", "+00:00")
        )
        return (response.data["owner"], expires_at)

    def cleanup_expired(self, project_id: str) -> list[str]:
        """Remove expired locks."""
        now = datetime.now().isoformat()

        response = (
            self.client.table(self.TABLE_LOCKS)
            .select("scope")
            .eq("project_id", project_id)
            .lt("expires_at", now)
            .execute()
        )

        expired_scopes = [row["scope"] for row in response.data]

        if expired_scopes:
            self.client.table(self.TABLE_LOCKS).delete().eq(
                "project_id", project_id
            ).lt("expires_at", now).execute()

        return expired_scopes

    # =========================================================================
    # Conversion Helpers
    # =========================================================================

    def _state_to_row(self, state: "C4State") -> dict[str, Any]:
        """Convert C4State to database row."""
        data = state.model_dump(mode="json")
        # Flatten for single row storage
        return {
            "project_id": data["project_id"],
            "status": data["status"],
            "execution_mode": data.get("execution_mode"),
            "state_data": data,  # Full state as JSONB
            "created_at": data["created_at"],
            "updated_at": data["updated_at"],
        }

    def _row_to_state(self, row: dict[str, Any]) -> "C4State":
        """Convert database row to C4State."""
        from c4.models import C4State

        # If state_data column exists, use it
        if "state_data" in row and row["state_data"]:
            return C4State.model_validate(row["state_data"])

        # Otherwise, try to reconstruct from columns
        return C4State.model_validate(row)


def create_supabase_store(
    url: str | None = None,
    key: str | None = None,
    realtime: bool = True,
) -> SupabaseStateStore:
    """Factory function for SupabaseStateStore.

    Args:
        url: Supabase project URL
        key: Supabase anon/service key
        realtime: Enable real-time subscriptions

    Returns:
        Configured SupabaseStateStore instance
    """
    return SupabaseStateStore(url=url, key=key, realtime=realtime)
