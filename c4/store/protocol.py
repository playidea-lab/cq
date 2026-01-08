"""Store Protocols - Abstract interfaces for state and lock storage"""

from abc import ABC, abstractmethod
from datetime import datetime
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from c4.models import C4State


class StateStore(ABC):
    """
    Abstract base class for state storage backends.

    Implementations:
    - LocalFileStateStore: File-based (.c4/state.json)
    - SQLiteStateStore: SQLite database
    - SupabaseStateStore: Supabase (future)
    """

    @abstractmethod
    def load(self, project_id: str) -> "C4State":
        """
        Load state for a project.

        Args:
            project_id: Project identifier

        Returns:
            C4State instance

        Raises:
            StateNotFoundError: If no state exists
        """
        pass

    @abstractmethod
    def save(self, state: "C4State") -> None:
        """
        Save state for a project.

        Args:
            state: State to persist (project_id is in state)

        Note:
            Updates state.updated_at automatically
        """
        pass

    @abstractmethod
    def exists(self, project_id: str) -> bool:
        """Check if state exists for project."""
        pass

    @abstractmethod
    def delete(self, project_id: str) -> None:
        """Delete state for project."""
        pass


class LockStore(ABC):
    """
    Abstract base class for distributed lock management.

    Supports:
    - Scope locks (task-level isolation)
    - Leader locks (daemon singleton)
    """

    @abstractmethod
    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """
        Acquire a scope lock.

        Args:
            project_id: Project identifier
            scope: Scope to lock (e.g., "src/backend")
            owner: Lock owner ID (e.g., worker_id)
            ttl_seconds: Time-to-live in seconds

        Returns:
            True if acquired, False if held by another owner
        """
        pass

    @abstractmethod
    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """
        Release a scope lock.

        Returns:
            True if released, False if not found
        """
        pass

    @abstractmethod
    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """
        Refresh lock TTL. Only succeeds if owner matches.

        Returns:
            True if refreshed, False if not owned
        """
        pass

    @abstractmethod
    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """
        Get current lock holder and expiry.

        Returns:
            (owner, expires_at) or None if not locked
        """
        pass

    @abstractmethod
    def cleanup_expired(self, project_id: str) -> list[str]:
        """
        Remove expired locks.

        Returns:
            List of cleaned scope names
        """
        pass

    # Leader lock (optional)
    def acquire_leader_lock(
        self,
        project_id: str,
        owner: str,
        pid: int,
    ) -> bool:
        """Acquire leader lock for daemon."""
        raise NotImplementedError("Leader lock not supported by this backend")

    def release_leader_lock(self, project_id: str) -> bool:
        """Release leader lock."""
        raise NotImplementedError("Leader lock not supported by this backend")
