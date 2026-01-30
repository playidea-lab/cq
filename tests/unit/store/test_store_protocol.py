"""Unit tests for Store Protocol (Abstract Base Classes)

Tests the StateStore and LockStore ABC contracts by creating
concrete implementations and verifying expected behaviors.
"""

from abc import ABC
from contextlib import contextmanager
from datetime import datetime, timedelta
from typing import Generator

import pytest

from c4.models import C4State
from c4.store.exceptions import StateNotFoundError
from c4.store.protocol import LockStore, StateStore

# ============================================================================
# Concrete implementations for testing
# ============================================================================


class MockStateStore(StateStore):
    """Concrete StateStore for testing protocol compliance."""

    def __init__(self):
        self._states: dict[str, C4State] = {}
        self._save_called = False
        self._delete_called = False

    def load(self, project_id: str) -> C4State:
        if project_id not in self._states:
            raise StateNotFoundError(f"State not found: {project_id}")
        return self._states[project_id]

    def save(self, state: C4State) -> None:
        state.updated_at = datetime.now()
        self._states[state.project_id] = state
        self._save_called = True

    def exists(self, project_id: str) -> bool:
        return project_id in self._states

    def delete(self, project_id: str) -> None:
        if project_id in self._states:
            del self._states[project_id]
        self._delete_called = True

    @contextmanager
    def atomic_modify(self, project_id: str) -> Generator[C4State, None, None]:
        if project_id not in self._states:
            raise StateNotFoundError(f"State not found: {project_id}")
        state = self._states[project_id]
        try:
            yield state
            # Commit on success
            state.updated_at = datetime.now()
            self._states[project_id] = state
        except Exception:
            # Rollback on exception - restore original state
            raise


class MockLockStore(LockStore):
    """Concrete LockStore for testing protocol compliance."""

    def __init__(self):
        self._locks: dict[tuple[str, str], tuple[str, datetime]] = {}
        self._expired_cleaned: list[str] = []

    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        key = (project_id, scope)
        if key in self._locks:
            existing_owner, expires_at = self._locks[key]
            if existing_owner != owner and expires_at > datetime.now():
                return False
        expires_at = datetime.now() + timedelta(seconds=ttl_seconds)
        self._locks[key] = (owner, expires_at)
        return True

    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        key = (project_id, scope)
        if key in self._locks:
            del self._locks[key]
            return True
        return False

    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        key = (project_id, scope)
        if key not in self._locks:
            return False
        existing_owner, _ = self._locks[key]
        if existing_owner != owner:
            return False
        expires_at = datetime.now() + timedelta(seconds=ttl_seconds)
        self._locks[key] = (owner, expires_at)
        return True

    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        key = (project_id, scope)
        return self._locks.get(key)

    def cleanup_expired(self, project_id: str) -> list[str]:
        expired = []
        for (pid, scope), (owner, expires_at) in list(self._locks.items()):
            if pid == project_id and expires_at <= datetime.now():
                del self._locks[(pid, scope)]
                expired.append(scope)
        self._expired_cleaned = expired
        return expired


# ============================================================================
# StateStore Protocol Tests
# ============================================================================


class TestStateStoreProtocol:
    """Tests for StateStore abstract base class contract."""

    @pytest.fixture
    def store(self):
        """Create a mock state store."""
        return MockStateStore()

    def test_load_returns_c4state(self, store):
        """load() should return C4State instance for existing project."""
        # Setup
        state = C4State(project_id="test-project")
        store._states["test-project"] = state

        # Execute
        result = store.load("test-project")

        # Verify
        assert isinstance(result, C4State)
        assert result.project_id == "test-project"

    def test_load_raises_state_not_found_error(self, store):
        """load() should raise StateNotFoundError for non-existent project."""
        with pytest.raises(StateNotFoundError):
            store.load("non-existent")

    def test_save_updates_timestamp(self, store):
        """save() should update updated_at timestamp."""
        state = C4State(project_id="test-project")
        original_time = state.updated_at

        # Small delay to ensure timestamp difference
        import time

        time.sleep(0.001)

        store.save(state)

        assert state.updated_at >= original_time
        assert store._save_called is True

    def test_exists_returns_true_for_existing_project(self, store):
        """exists() should return True when state exists."""
        state = C4State(project_id="test-project")
        store._states["test-project"] = state

        result = store.exists("test-project")

        assert result is True

    def test_exists_returns_false_for_non_existing_project(self, store):
        """exists() should return False when state doesn't exist."""
        result = store.exists("non-existent")

        assert result is False

    def test_delete_removes_state(self, store):
        """delete() should remove state from storage."""
        state = C4State(project_id="test-project")
        store._states["test-project"] = state

        store.delete("test-project")

        assert "test-project" not in store._states
        assert store._delete_called is True

    def test_atomic_modify_commits_on_success(self, store):
        """atomic_modify() should commit changes when block completes normally."""
        state = C4State(project_id="test-project")
        store._states["test-project"] = state
        original_time = state.updated_at

        import time

        time.sleep(0.001)

        with store.atomic_modify("test-project") as s:
            s.project_id = "test-project"  # No-op modification

        # Verify timestamp was updated (commit happened)
        assert store._states["test-project"].updated_at >= original_time

    def test_atomic_modify_raises_on_non_existent(self, store):
        """atomic_modify() should raise StateNotFoundError for non-existent project."""
        with pytest.raises(StateNotFoundError):
            with store.atomic_modify("non-existent"):
                pass

    def test_atomic_modify_preserves_exception(self, store):
        """atomic_modify() should preserve exceptions from the block."""
        state = C4State(project_id="test-project")
        store._states["test-project"] = state

        with pytest.raises(ValueError, match="test error"):
            with store.atomic_modify("test-project"):
                raise ValueError("test error")


# ============================================================================
# LockStore Protocol Tests
# ============================================================================


class TestLockStoreProtocol:
    """Tests for LockStore abstract base class contract."""

    @pytest.fixture
    def store(self):
        """Create a mock lock store."""
        return MockLockStore()

    def test_acquire_lock_returns_true_on_success(self, store):
        """acquire_scope_lock() should return True when lock is available."""
        result = store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        assert result is True
        assert ("test", "src/backend") in store._locks

    def test_acquire_lock_returns_false_when_held_by_other(self, store):
        """acquire_scope_lock() should return False when held by another owner."""
        # First acquire
        store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        # Second acquire by different owner
        result = store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-2",
            ttl_seconds=60,
        )

        assert result is False

    def test_release_scope_lock_returns_true_on_success(self, store):
        """release_scope_lock() should return True when lock exists."""
        store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        result = store.release_scope_lock("test", "src/backend")

        assert result is True
        assert ("test", "src/backend") not in store._locks

    def test_release_scope_lock_returns_false_when_not_found(self, store):
        """release_scope_lock() should return False when lock doesn't exist."""
        result = store.release_scope_lock("test", "non-existent")

        assert result is False

    def test_refresh_scope_lock_returns_true_when_owned(self, store):
        """refresh_scope_lock() should return True when owner matches."""
        store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        result = store.refresh_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=120,
        )

        assert result is True

    def test_refresh_scope_lock_returns_false_when_not_owned(self, store):
        """refresh_scope_lock() should return False when owner doesn't match."""
        store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        result = store.refresh_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-2",
            ttl_seconds=120,
        )

        assert result is False

    def test_get_scope_lock_returns_owner_and_expiry(self, store):
        """get_scope_lock() should return (owner, expires_at) when locked."""
        store.acquire_scope_lock(
            project_id="test",
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=60,
        )

        result = store.get_scope_lock("test", "src/backend")

        assert result is not None
        owner, expires_at = result
        assert owner == "worker-1"
        assert isinstance(expires_at, datetime)

    def test_get_scope_lock_returns_none_when_not_locked(self, store):
        """get_scope_lock() should return None when not locked."""
        result = store.get_scope_lock("test", "non-existent")

        assert result is None

    def test_leader_lock_not_implemented_raises(self, store):
        """acquire_leader_lock() should raise NotImplementedError by default."""
        with pytest.raises(NotImplementedError, match="Leader lock not supported"):
            store.acquire_leader_lock(project_id="test", owner="daemon", pid=12345)

    def test_release_leader_lock_not_implemented_raises(self, store):
        """release_leader_lock() should raise NotImplementedError by default."""
        with pytest.raises(NotImplementedError, match="Leader lock not supported"):
            store.release_leader_lock(project_id="test")


# ============================================================================
# ABC Contract Tests
# ============================================================================


class TestAbstractClassContracts:
    """Tests verifying ABC contracts are enforced."""

    def test_state_store_is_abstract(self):
        """StateStore should be an abstract base class."""
        assert issubclass(StateStore, ABC)

    def test_lock_store_is_abstract(self):
        """LockStore should be an abstract base class."""
        assert issubclass(LockStore, ABC)

    def test_cannot_instantiate_state_store_directly(self):
        """StateStore should not be instantiable directly."""
        with pytest.raises(TypeError, match="abstract"):
            StateStore()

    def test_cannot_instantiate_lock_store_directly(self):
        """LockStore should not be instantiable directly."""
        with pytest.raises(TypeError, match="abstract"):
            LockStore()
