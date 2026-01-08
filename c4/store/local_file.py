"""Local File Store - File-based state and lock storage"""

import json
from datetime import datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING

from .exceptions import StateNotFoundError
from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.models import C4State


class LocalFileStateStore(StateStore):
    """
    File-based state storage.

    Matches current behavior: stores state in .c4/state.json
    """

    def __init__(self, c4_dir: Path):
        """
        Initialize local file store.

        Args:
            c4_dir: Path to .c4 directory
        """
        self.c4_dir = c4_dir
        self.state_file = c4_dir / "state.json"

    def load(self, project_id: str) -> "C4State":
        """Load state from state.json"""
        from c4.models import C4State

        if not self.state_file.exists():
            raise StateNotFoundError(f"State file not found: {self.state_file}")

        data = json.loads(self.state_file.read_text())
        return C4State.model_validate(data)

    def save(self, state: "C4State") -> None:
        """Save state to state.json"""
        state.updated_at = datetime.now()
        self.c4_dir.mkdir(parents=True, exist_ok=True)
        self.state_file.write_text(state.model_dump_json(indent=2))

    def exists(self, project_id: str) -> bool:
        """Check if state file exists"""
        return self.state_file.exists()

    def delete(self, project_id: str) -> None:
        """Delete state file"""
        if self.state_file.exists():
            self.state_file.unlink()


class LocalFileLockStore(LockStore):
    """
    File-based lock storage.

    Stores locks within state.json (matches current behavior).
    Requires a StateStore for persistence.
    """

    def __init__(self, state_store: LocalFileStateStore):
        """
        Initialize local file lock store.

        Args:
            state_store: State store for persistence
        """
        self.state_store = state_store

    def _load_state(self, project_id: str) -> "C4State":
        """Load state, return new if not exists"""
        from c4.models import C4State

        try:
            return self.state_store.load(project_id)
        except StateNotFoundError:
            return C4State(project_id=project_id)

    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Acquire scope lock with TTL"""
        from c4.models import ScopeLock

        state = self._load_state(project_id)
        now = datetime.now()

        # Check existing lock
        if scope in state.locks.scopes:
            lock = state.locks.scopes[scope]
            if lock.expires_at > now:
                # Still valid
                if lock.owner == owner:
                    # Same owner - refresh TTL
                    lock.expires_at = now + timedelta(seconds=ttl_seconds)
                    self.state_store.save(state)
                    return True
                else:
                    # Different owner - conflict
                    return False
            # Expired - can take over

        # Acquire lock
        state.locks.scopes[scope] = ScopeLock(
            owner=owner,
            scope=scope,
            expires_at=now + timedelta(seconds=ttl_seconds),
        )
        self.state_store.save(state)
        return True

    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """Release scope lock"""
        try:
            state = self.state_store.load(project_id)
        except StateNotFoundError:
            return False

        if scope in state.locks.scopes:
            del state.locks.scopes[scope]
            self.state_store.save(state)
            return True
        return False

    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Refresh lock TTL if owner matches"""
        try:
            state = self.state_store.load(project_id)
        except StateNotFoundError:
            return False

        if scope not in state.locks.scopes:
            return False

        lock = state.locks.scopes[scope]
        if lock.owner != owner:
            return False

        lock.expires_at = datetime.now() + timedelta(seconds=ttl_seconds)
        self.state_store.save(state)
        return True

    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """Get current lock holder and expiry"""
        try:
            state = self.state_store.load(project_id)
        except StateNotFoundError:
            return None

        if scope not in state.locks.scopes:
            return None

        lock = state.locks.scopes[scope]
        return (lock.owner, lock.expires_at)

    def cleanup_expired(self, project_id: str) -> list[str]:
        """Remove expired locks"""
        try:
            state = self.state_store.load(project_id)
        except StateNotFoundError:
            return []

        now = datetime.now()
        expired = []

        for scope, lock in list(state.locks.scopes.items()):
            if lock.expires_at < now:
                expired.append(scope)
                del state.locks.scopes[scope]

        if expired:
            self.state_store.save(state)

        return expired

    def acquire_leader_lock(
        self,
        project_id: str,
        owner: str,
        pid: int,
    ) -> bool:
        """Acquire leader lock"""
        from c4.models import LeaderLock

        state = self._load_state(project_id)

        # Check existing leader
        if state.locks.leader is not None:
            # Already has leader
            if state.locks.leader.owner == owner:
                return True
            return False

        state.locks.leader = LeaderLock(
            owner=owner,
            pid=pid,
            started_at=datetime.now(),
        )
        self.state_store.save(state)
        return True

    def release_leader_lock(self, project_id: str) -> bool:
        """Release leader lock"""
        try:
            state = self.state_store.load(project_id)
        except StateNotFoundError:
            return False

        if state.locks.leader is not None:
            state.locks.leader = None
            self.state_store.save(state)
            return True
        return False
