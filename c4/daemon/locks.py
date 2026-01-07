"""C4 Lock Manager - Scope lock management for multi-worker support"""

from datetime import datetime, timedelta
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.models import C4Config, ScopeLock, Task
    from c4.state_machine import StateMachine


class LockManager:
    """Manages scope locks for multi-worker task assignment"""

    def __init__(self, state_machine: "StateMachine", config: "C4Config"):
        self.state_machine = state_machine
        self.config = config

    @property
    def _locks(self) -> dict[str, "ScopeLock"]:
        """Get the scopes dict from state"""
        return self.state_machine.state.locks.scopes

    def can_assign_task(self, task: "Task", worker_id: str) -> bool:
        """
        Check if a task can be assigned to a worker.

        Handles:
        - Tasks without scope (always assignable)
        - Scope lock conflicts (same worker can reuse own lock)
        - Lock expiration (expired locks don't block)
        - Task dependencies
        """
        # Check dependencies first
        for dep_id in task.dependencies:
            if dep_id not in self.state_machine.state.queue.done:
                return False

        scope = task.scope
        if not scope:
            return True

        # Clean up any expired locks first
        self.cleanup_expired()

        # Check if scope is locked
        if scope in self._locks:
            lock = self._locks[scope]
            # Same worker can take task with their own lock
            if lock.owner == worker_id:
                return True
            # Otherwise scope is locked
            return False

        return True

    def acquire(self, scope: str, worker_id: str) -> "ScopeLock":
        """Acquire a scope lock for a worker"""
        from c4.models import ScopeLock

        ttl = self.config.scope_lock_ttl_sec
        lock = ScopeLock(
            owner=worker_id,
            scope=scope,
            expires_at=datetime.now() + timedelta(seconds=ttl),
        )
        self._locks[scope] = lock
        return lock

    def refresh(self, scope: str, worker_id: str) -> bool:
        """
        Refresh a scope lock's TTL.

        Returns:
            True if refresh successful, False if lock not owned
        """
        if scope not in self._locks:
            return False

        lock = self._locks[scope]
        if lock.owner != worker_id:
            return False

        # Extend TTL
        ttl = self.config.scope_lock_ttl_sec
        lock.expires_at = datetime.now() + timedelta(seconds=ttl)
        self.state_machine.save_state()
        return True

    def release(self, scope: str) -> bool:
        """
        Release a scope lock.

        Returns:
            True if lock was released, False if not found
        """
        if scope in self._locks:
            del self._locks[scope]
            self.state_machine.save_state()
            return True
        return False

    def cleanup_expired(self) -> list[str]:
        """
        Remove all expired scope locks.

        Returns:
            List of scopes that were cleaned up
        """
        now = datetime.now()
        expired = []

        for scope, lock in list(self._locks.items()):
            if lock.expires_at < now:
                expired.append(scope)
                del self._locks[scope]

        if expired:
            self.state_machine.save_state()

        return expired

    def get_status(self) -> dict[str, Any]:
        """Get current lock status for debugging/monitoring"""
        now = datetime.now()
        lock_info = {}

        for scope, lock in self._locks.items():
            remaining = (lock.expires_at - now).total_seconds()
            lock_info[scope] = {
                "owner": lock.owner,
                "expires_at": lock.expires_at.isoformat(),
                "remaining_seconds": max(0, remaining),
                "expired": remaining <= 0,
            }

        return {
            "total_locks": len(self._locks),
            "locks": lock_info,
        }
