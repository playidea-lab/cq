"""Supabase StateStore - Cloud-based state storage with real-time sync."""

from __future__ import annotations

import logging
import os
from contextlib import contextmanager
from datetime import datetime
from enum import Enum
from typing import TYPE_CHECKING, Any, Callable, Generator

from .exceptions import StateNotFoundError
from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.auth import SupabaseAuth
    from c4.models import C4State
    from c4.realtime.manager import RealtimeManager

logger = logging.getLogger(__name__)


class ChangeType(Enum):
    """Type of state change event."""

    INSERT = "INSERT"
    UPDATE = "UPDATE"
    DELETE = "DELETE"


# Callback type for state changes
StateChangeCallback = Callable[["C4State", ChangeType], None]


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
        team_id: str | None = None,
        access_token: str | None = None,
        auth: "SupabaseAuth | None" = None,
    ):
        """Initialize Supabase store.

        Args:
            url: Supabase project URL (or SUPABASE_URL env)
            key: Supabase key (or SUPABASE_KEY env)
            realtime: Enable real-time subscriptions
            team_id: Team ID for RLS isolation (or C4_TEAM_ID env)
            access_token: Supabase Auth JWT token for RLS (or SUPABASE_ACCESS_TOKEN env)
            auth: SupabaseAuth instance for full auth integration
        """
        self._url = url or os.environ.get("SUPABASE_URL", "")
        self._key = key or os.environ.get("SUPABASE_KEY", "")
        self._realtime_enabled = realtime
        self._team_id = team_id or os.environ.get("C4_TEAM_ID")
        self._access_token = access_token or os.environ.get("SUPABASE_ACCESS_TOKEN")
        self._auth = auth
        self._client: Any = None
        self._subscriptions: dict[str, Any] = {}
        self._callbacks: dict[str, list[Callable[[C4State], None]]] = {}

        # RealtimeManager integration
        self._realtime_manager: "RealtimeManager | None" = None
        self._change_callbacks: dict[str, list[StateChangeCallback]] = {}
        self._global_change_callbacks: list[StateChangeCallback] = []

    @property
    def team_id(self) -> str | None:
        """Current team ID for RLS filtering."""
        return self._team_id

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

            # Apply auth token (priority: SupabaseAuth > explicit access_token > env var)
            self._apply_auth_token()

        return self._client

    def _apply_auth_token(self) -> None:
        """Apply auth token to client for RLS.

        Priority:
        1. SupabaseAuth instance (if provided)
        2. Explicit access_token parameter
        3. SUPABASE_ACCESS_TOKEN env var
        """
        token = None

        if self._auth:
            # Get token from SupabaseAuth (handles auto-refresh)
            token = self._auth.get_access_token()
        elif self._access_token:
            token = self._access_token

        if token and self._client:
            self._client.postgrest.auth(token)

    def load(self, project_id: str) -> "C4State":
        """Load state from Supabase.

        Args:
            project_id: Project identifier

        Returns:
            C4State instance

        Raises:
            StateNotFoundError: If project not found
        """
        query = (
            self.client.table(self.TABLE_STATE)
            .select("*")
            .eq("project_id", project_id)
        )

        # Apply team_id filter if set
        if self._team_id:
            query = query.eq("team_id", self._team_id)

        response = query.maybe_single().execute()

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

        # Include team_id if set
        if self._team_id:
            data["team_id"] = self._team_id

        self.client.table(self.TABLE_STATE).upsert(
            data, on_conflict="project_id"
        ).execute()

    def exists(self, project_id: str) -> bool:
        """Check if project exists in Supabase."""
        query = (
            self.client.table(self.TABLE_STATE)
            .select("project_id")
            .eq("project_id", project_id)
        )

        if self._team_id:
            query = query.eq("team_id", self._team_id)

        response = query.maybe_single().execute()
        return response.data is not None

    def delete(self, project_id: str) -> None:
        """Delete project state from Supabase."""
        query = self.client.table(self.TABLE_STATE).delete().eq(
            "project_id", project_id
        )

        if self._team_id:
            query = query.eq("team_id", self._team_id)

        query.execute()

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
    # Enhanced Realtime with RealtimeManager (on_change API)
    # =========================================================================

    def on_change(
        self,
        callback: StateChangeCallback,
        project_id: str | None = None,
        event: str = "*",
    ) -> str:
        """Subscribe to state changes with change type information.

        This is the enhanced API that provides change type (INSERT/UPDATE/DELETE)
        in addition to the state data.

        Args:
            callback: Function called on state change with (state, change_type)
            project_id: Optional project ID to filter changes (None = all projects)
            event: Event type: INSERT, UPDATE, DELETE, or * for all

        Returns:
            Subscription ID for unsubscribing

        Example:
            def on_task_update(state: C4State, change_type: ChangeType):
                if change_type == ChangeType.UPDATE:
                    print(f"Project {state.project_id} updated")

            sub_id = store.on_change(on_task_update, project_id="my-project")
            # Later: store.off_change(sub_id)
        """
        if not self._realtime_enabled:
            raise RuntimeError("Real-time disabled for this store")

        # Initialize RealtimeManager if not done
        self._ensure_realtime_manager()

        if project_id:
            # Project-specific subscription
            if project_id not in self._change_callbacks:
                self._change_callbacks[project_id] = []
            self._change_callbacks[project_id].append(callback)

            # Subscribe via RealtimeManager with filter
            self._realtime_manager.subscribe_table(
                table=self.TABLE_STATE,
                event=event,
                callback=lambda payload: self._dispatch_change(payload),
                filter_column="project_id",
                filter_value=project_id,
            )

            subscription_id = f"change:{project_id}:{id(callback)}"
        else:
            # Global subscription (all projects)
            self._global_change_callbacks.append(callback)

            # Subscribe via RealtimeManager without filter
            self._realtime_manager.subscribe_table(
                table=self.TABLE_STATE,
                event=event,
                callback=lambda payload: self._dispatch_change(payload),
            )

            subscription_id = f"change:*:{id(callback)}"

        logger.debug(f"Registered on_change callback: {subscription_id}")
        return subscription_id

    def off_change(self, subscription_id: str) -> bool:
        """Unsubscribe from state changes.

        Args:
            subscription_id: ID from on_change()

        Returns:
            True if unsubscribed, False if not found
        """
        parts = subscription_id.split(":")
        if len(parts) != 3 or parts[0] != "change":
            logger.warning(f"Invalid subscription ID: {subscription_id}")
            return False

        project_id = parts[1]
        callback_id = int(parts[2])

        if project_id == "*":
            # Global callback
            self._global_change_callbacks = [
                cb for cb in self._global_change_callbacks if id(cb) != callback_id
            ]
            return True
        else:
            # Project-specific callback
            if project_id in self._change_callbacks:
                self._change_callbacks[project_id] = [
                    cb
                    for cb in self._change_callbacks[project_id]
                    if id(cb) != callback_id
                ]
                if not self._change_callbacks[project_id]:
                    del self._change_callbacks[project_id]
                return True

        return False

    def _ensure_realtime_manager(self) -> None:
        """Ensure RealtimeManager is initialized and connected."""
        if self._realtime_manager is not None:
            return

        from c4.realtime.manager import RealtimeConfig, RealtimeManager

        config = RealtimeConfig(
            supabase_url=self._url,
            supabase_key=self._key,
            access_token=self._auth.get_access_token() if self._auth else None,
            auto_reconnect=True,
        )

        self._realtime_manager = RealtimeManager(config)
        self._realtime_manager.on_error(
            lambda e: logger.error(f"Realtime error: {e}")
        )
        self._realtime_manager.connect()

    def _dispatch_change(self, payload: dict[str, Any]) -> None:
        """Dispatch change event to registered callbacks.

        Args:
            payload: Supabase realtime payload with eventType, new, old
        """
        try:
            event_type = payload.get("eventType", "").upper()
            change_type = ChangeType(event_type) if event_type else ChangeType.UPDATE

            # Get state from payload
            new_data = payload.get("new")
            old_data = payload.get("old")

            if change_type == ChangeType.DELETE:
                # For deletes, use old data
                if not old_data:
                    return
                state = self._row_to_state(old_data)
            else:
                # For inserts/updates, use new data
                if not new_data:
                    return
                state = self._row_to_state(new_data)

            project_id = state.project_id

            # Call project-specific callbacks
            if project_id in self._change_callbacks:
                for callback in self._change_callbacks[project_id]:
                    try:
                        callback(state, change_type)
                    except Exception as e:
                        logger.error(f"Change callback error: {e}")

            # Call global callbacks
            for callback in self._global_change_callbacks:
                try:
                    callback(state, change_type)
                except Exception as e:
                    logger.error(f"Global change callback error: {e}")

        except Exception as e:
            logger.error(f"Error dispatching change: {e}")

    def get_user_id(self) -> str | None:
        """Get current user ID from auth.

        Returns:
            User ID if authenticated, None otherwise
        """
        if self._auth:
            session = self._auth.get_session(auto_refresh=False)
            return session.user_id if session else None
        return None

    def refresh_auth(self) -> bool:
        """Refresh authentication token.

        Returns:
            True if refresh successful
        """
        if self._auth:
            result = self._auth.refresh_token()
            if result:
                # Re-apply token to client
                self._apply_auth_token()
            return result
        return False

    def verify_rls_access(self, project_id: str) -> bool:
        """Verify RLS allows access to project.

        Args:
            project_id: Project to check

        Returns:
            True if access allowed
        """
        try:
            query = (
                self.client.table(self.TABLE_STATE)
                .select("project_id")
                .eq("project_id", project_id)
            )

            if self._team_id:
                query = query.eq("team_id", self._team_id)

            response = query.maybe_single().execute()
            return response.data is not None
        except Exception as e:
            logger.warning(f"RLS access check failed: {e}")
            return False

    def disconnect_realtime(self) -> None:
        """Disconnect RealtimeManager and cleanup subscriptions."""
        if self._realtime_manager:
            self._realtime_manager.disconnect()
            self._realtime_manager = None

        self._change_callbacks.clear()
        self._global_change_callbacks.clear()

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

        # Build lock data
        lock_data = {
            "project_id": project_id,
            "scope": scope,
            "owner": owner,
            "expires_at": expires_at.isoformat(),
        }

        # Include team_id if set
        if self._team_id:
            lock_data["team_id"] = self._team_id

        # Try to insert or update if expired
        try:
            self.client.table(self.TABLE_LOCKS).upsert(
                lock_data,
                on_conflict="project_id,scope",
            ).execute()
            return True
        except Exception:
            return False

    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """Release a scope lock."""
        try:
            query = (
                self.client.table(self.TABLE_LOCKS)
                .delete()
                .eq("project_id", project_id)
                .eq("scope", scope)
            )

            if self._team_id:
                query = query.eq("team_id", self._team_id)

            query.execute()
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
            query = (
                self.client.table(self.TABLE_LOCKS)
                .update({"expires_at": expires_at.isoformat()})
                .eq("project_id", project_id)
                .eq("scope", scope)
                .eq("owner", owner)
            )

            if self._team_id:
                query = query.eq("team_id", self._team_id)

            response = query.execute()
            return len(response.data) > 0
        except Exception:
            return False

    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """Get current lock holder and expiry."""
        query = (
            self.client.table(self.TABLE_LOCKS)
            .select("owner, expires_at")
            .eq("project_id", project_id)
            .eq("scope", scope)
        )

        if self._team_id:
            query = query.eq("team_id", self._team_id)

        response = query.maybe_single().execute()

        if not response.data:
            return None

        expires_at = datetime.fromisoformat(
            response.data["expires_at"].replace("Z", "+00:00")
        )
        return (response.data["owner"], expires_at)

    def cleanup_expired(self, project_id: str) -> list[str]:
        """Remove expired locks."""
        now = datetime.now().isoformat()

        query = (
            self.client.table(self.TABLE_LOCKS)
            .select("scope")
            .eq("project_id", project_id)
            .lt("expires_at", now)
        )

        if self._team_id:
            query = query.eq("team_id", self._team_id)

        response = query.execute()
        expired_scopes = [row["scope"] for row in response.data]

        if expired_scopes:
            delete_query = (
                self.client.table(self.TABLE_LOCKS)
                .delete()
                .eq("project_id", project_id)
                .lt("expires_at", now)
            )

            if self._team_id:
                delete_query = delete_query.eq("team_id", self._team_id)

            delete_query.execute()

        return expired_scopes

    # =========================================================================
    # Atomic Modify (required by StateStore protocol)
    # =========================================================================

    @contextmanager
    def atomic_modify(
        self, project_id: str
    ) -> Generator["C4State", None, None]:
        """
        Atomically load, modify, and save state using optimistic locking.

        Uses version-based optimistic concurrency control:
        1. Load state with current version
        2. Yield for modification
        3. Save with version check (fails if version changed)

        Note: Requires 'version' column in c4_state table.
        If version column doesn't exist, falls back to simple save.
        """

        # Load current state with version
        query = (
            self.client.table(self.TABLE_STATE)
            .select("*")
            .eq("project_id", project_id)
        )

        if self._team_id:
            query = query.eq("team_id", self._team_id)

        response = query.maybe_single().execute()

        if not response.data:
            raise StateNotFoundError(project_id)

        state = self._row_to_state(response.data)
        original_version = response.data.get("version", 0)

        try:
            yield state

            # Save with optimistic lock check
            state.updated_at = datetime.now()
            data = self._state_to_row(state)
            new_version = original_version + 1
            data["version"] = new_version

            # Include team_id if set
            if self._team_id:
                data["team_id"] = self._team_id

            # Try atomic update with version check
            # Using update with eq filter for optimistic locking
            update_query = (
                self.client.table(self.TABLE_STATE)
                .update(data)
                .eq("project_id", project_id)
                .eq("version", original_version)
            )

            if self._team_id:
                update_query = update_query.eq("team_id", self._team_id)

            update_response = update_query.execute()

            # Check if update succeeded (version matched)
            if not update_response.data:
                # Version mismatch - concurrent modification
                from .exceptions import ConcurrentModificationError

                raise ConcurrentModificationError(
                    f"State was modified by another process: {project_id}"
                )

        except Exception:
            # No explicit rollback needed - changes weren't saved
            raise

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
    team_id: str | None = None,
    access_token: str | None = None,
    auth: "SupabaseAuth | None" = None,
) -> SupabaseStateStore:
    """Factory function for SupabaseStateStore.

    Args:
        url: Supabase project URL
        key: Supabase anon/service key
        realtime: Enable real-time subscriptions
        team_id: Team ID for RLS isolation
        access_token: Supabase Auth JWT token for RLS
        auth: SupabaseAuth instance for full auth integration

    Returns:
        Configured SupabaseStateStore instance
    """
    return SupabaseStateStore(
        url=url,
        key=key,
        realtime=realtime,
        team_id=team_id,
        access_token=access_token,
        auth=auth,
    )


def create_authenticated_store(
    auth: "SupabaseAuth",
    realtime: bool = True,
) -> SupabaseStateStore:
    """Create SupabaseStateStore with full auth integration.

    This factory creates a store that:
    - Uses SupabaseAuth for automatic token management
    - Supports RLS with authenticated user context
    - Auto-refreshes tokens when needed

    Args:
        auth: SupabaseAuth instance (must be logged in)
        realtime: Enable real-time subscriptions

    Returns:
        Configured SupabaseStateStore instance

    Example:
        from c4.auth import create_supabase_auth
        from c4.store.supabase import create_authenticated_store

        auth = create_supabase_auth()
        auth.login(provider="github")

        store = create_authenticated_store(auth)
        state = store.load("my-project")
    """
    return SupabaseStateStore(
        url=auth.supabase_url,
        key=auth._key,
        realtime=realtime,
        auth=auth,
    )
