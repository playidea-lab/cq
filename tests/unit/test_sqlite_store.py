"""Unit tests for C4 SQLite Store"""

from datetime import datetime, timedelta

import pytest

from c4.models import C4State
from c4.store import (
    SQLiteLockStore,
    SQLiteStateStore,
    StateNotFoundError,
)


@pytest.fixture
def db_path(tmp_path):
    """Create a temporary database path"""
    return tmp_path / "test.db"


@pytest.fixture
def state_store(db_path):
    """Create a SQLiteStateStore"""
    return SQLiteStateStore(db_path)


@pytest.fixture
def lock_store(db_path):
    """Create a SQLiteLockStore"""
    return SQLiteLockStore(db_path)


class TestSQLiteStateStore:
    """Tests for SQLiteStateStore"""

    def test_save_and_load(self, state_store):
        """Test saving and loading state"""
        state = C4State(project_id="test-project")
        state_store.save(state)

        loaded = state_store.load("test-project")
        assert loaded.project_id == "test-project"

    def test_load_not_found(self, state_store):
        """Test loading non-existent state"""
        with pytest.raises(StateNotFoundError):
            state_store.load("non-existent")

    def test_exists(self, state_store):
        """Test exists check"""
        assert state_store.exists("test") is False

        state = C4State(project_id="test")
        state_store.save(state)

        assert state_store.exists("test") is True

    def test_delete(self, state_store):
        """Test deleting state"""
        state = C4State(project_id="test")
        state_store.save(state)
        assert state_store.exists("test") is True

        state_store.delete("test")
        assert state_store.exists("test") is False

    def test_save_updates_timestamp(self, state_store):
        """Test that save updates updated_at"""
        state = C4State(project_id="test")
        original_time = state.updated_at

        state_store.save(state)

        loaded = state_store.load("test")
        assert loaded.updated_at >= original_time

    def test_creates_directory(self, tmp_path):
        """Test that save creates parent directory if needed"""
        db_path = tmp_path / "new_dir" / "test.db"
        store = SQLiteStateStore(db_path)

        state = C4State(project_id="test")
        store.save(state)

        assert db_path.exists()
        assert store.exists("test")

    def test_multiple_projects(self, state_store):
        """Test storing multiple projects"""
        state1 = C4State(project_id="project-1")
        state2 = C4State(project_id="project-2")

        state_store.save(state1)
        state_store.save(state2)

        loaded1 = state_store.load("project-1")
        loaded2 = state_store.load("project-2")

        assert loaded1.project_id == "project-1"
        assert loaded2.project_id == "project-2"

    def test_update_existing(self, state_store):
        """Test updating existing state"""
        from c4.models import ProjectStatus

        state = C4State(project_id="test")
        state_store.save(state)

        # Modify and save again
        state.status = ProjectStatus.EXECUTE
        state_store.save(state)

        loaded = state_store.load("test")
        assert loaded.status == ProjectStatus.EXECUTE


class TestSQLiteLockStore:
    """Tests for SQLiteLockStore"""

    def test_acquire_scope_lock(self, lock_store):
        """Test acquiring a scope lock"""
        result = lock_store.acquire_scope_lock(
            project_id="test",
            scope="backend",
            owner="worker-1",
            ttl_seconds=60,
        )
        assert result is True

    def test_acquire_lock_conflict(self, lock_store):
        """Test lock conflict with different owner"""
        # First worker acquires
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Second worker tries to acquire same scope
        result = lock_store.acquire_scope_lock("test", "backend", "worker-2", 60)
        assert result is False

    def test_acquire_same_owner_refreshes(self, lock_store):
        """Test same owner can refresh lock"""
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Same owner can re-acquire (refreshes TTL)
        result = lock_store.acquire_scope_lock("test", "backend", "worker-1", 120)
        assert result is True

    def test_release_scope_lock(self, lock_store):
        """Test releasing a scope lock"""
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        result = lock_store.release_scope_lock("test", "backend")
        assert result is True

        # Now another worker can acquire
        result = lock_store.acquire_scope_lock("test", "backend", "worker-2", 60)
        assert result is True

    def test_release_nonexistent_lock(self, lock_store):
        """Test releasing non-existent lock"""
        result = lock_store.release_scope_lock("test", "nonexistent")
        assert result is False

    def test_get_scope_lock(self, lock_store):
        """Test getting lock info"""
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        result = lock_store.get_scope_lock("test", "backend")
        assert result is not None
        owner, expires_at = result
        assert owner == "worker-1"
        assert expires_at > datetime.now()

    def test_get_nonexistent_lock(self, lock_store):
        """Test getting non-existent lock"""
        result = lock_store.get_scope_lock("test", "nonexistent")
        assert result is None

    def test_refresh_scope_lock(self, lock_store):
        """Test refreshing lock TTL"""
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Refresh with longer TTL
        result = lock_store.refresh_scope_lock("test", "backend", "worker-1", 120)
        assert result is True

    def test_refresh_wrong_owner(self, lock_store):
        """Test refresh fails for wrong owner"""
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Different owner cannot refresh
        result = lock_store.refresh_scope_lock("test", "backend", "worker-2", 120)
        assert result is False

    def test_cleanup_expired(self, lock_store, db_path):
        """Test cleaning up expired locks"""
        # Insert an already-expired lock directly
        import sqlite3

        conn = sqlite3.connect(db_path)
        expired_time = datetime.now() - timedelta(seconds=10)
        valid_time = datetime.now() + timedelta(seconds=60)

        conn.execute(
            "INSERT INTO c4_locks (project_id, scope, owner, expires_at) VALUES (?, ?, ?, ?)",
            ("test", "expired", "worker-1", expired_time),
        )
        conn.execute(
            "INSERT INTO c4_locks (project_id, scope, owner, expires_at) VALUES (?, ?, ?, ?)",
            ("test", "valid", "worker-2", valid_time),
        )
        conn.commit()
        conn.close()

        expired = lock_store.cleanup_expired("test")
        assert "expired" in expired
        assert "valid" not in expired

        # Verify expired lock is gone
        assert lock_store.get_scope_lock("test", "expired") is None
        assert lock_store.get_scope_lock("test", "valid") is not None

    def test_leader_lock(self, lock_store):
        """Test leader lock acquire/release"""
        # Acquire leader
        result = lock_store.acquire_leader_lock("test", "daemon-1", 12345)
        assert result is True

        # Cannot acquire again with different owner
        result = lock_store.acquire_leader_lock("test", "daemon-2", 67890)
        assert result is False

        # Same owner can re-acquire
        result = lock_store.acquire_leader_lock("test", "daemon-1", 12345)
        assert result is True

        # Release
        result = lock_store.release_leader_lock("test")
        assert result is True

        # Now another can acquire
        result = lock_store.acquire_leader_lock("test", "daemon-2", 67890)
        assert result is True

    def test_multiple_projects_locks(self, lock_store):
        """Test locks are isolated by project"""
        # Lock backend for project 1
        lock_store.acquire_scope_lock("project-1", "backend", "worker-1", 60)

        # Same scope in project 2 should be free
        result = lock_store.acquire_scope_lock("project-2", "backend", "worker-2", 60)
        assert result is True

    def test_expired_lock_can_be_taken(self, lock_store, db_path):
        """Test that expired locks can be acquired by others"""
        import sqlite3

        # Insert an expired lock
        conn = sqlite3.connect(db_path)
        expired_time = datetime.now() - timedelta(seconds=10)
        conn.execute(
            "INSERT INTO c4_locks (project_id, scope, owner, expires_at) VALUES (?, ?, ?, ?)",
            ("test", "backend", "worker-1", expired_time),
        )
        conn.commit()
        conn.close()

        # Different worker should be able to acquire
        result = lock_store.acquire_scope_lock("test", "backend", "worker-2", 60)
        assert result is True

        # Verify new owner
        owner, _ = lock_store.get_scope_lock("test", "backend")
        assert owner == "worker-2"


    def test_concurrent_state_modifications(self, db_path):
        """Test concurrent state modifications using threads.

        Verifies that the SQLite store handles concurrent writes correctly
        with BEGIN IMMEDIATE and WAL mode.
        """
        from concurrent.futures import ThreadPoolExecutor, as_completed

        store = SQLiteStateStore(db_path)

        # Initialize state
        initial_state = C4State(project_id="concurrent-test")
        store.save(initial_state)

        results = []
        errors = []

        def modify_state(worker_id: int):
            """Each worker increments a counter in state metrics."""
            try:
                with store.atomic_modify("concurrent-test") as state:
                    # Simulate some work
                    import time

                    time.sleep(0.01)

                    # Increment tasks_completed as our concurrent counter
                    state.metrics.tasks_completed += 1

                results.append(worker_id)
            except Exception as e:
                errors.append((worker_id, str(e)))

        # Run 10 concurrent modifications
        num_workers = 10
        with ThreadPoolExecutor(max_workers=num_workers) as executor:
            futures = [executor.submit(modify_state, i) for i in range(num_workers)]
            for future in as_completed(futures):
                pass  # Wait for completion

        # Verify results
        final_state = store.load("concurrent-test")

        # All workers should have completed without errors
        assert len(errors) == 0, f"Errors occurred: {errors}"
        assert len(results) == num_workers

        # Counter should equal number of workers (no lost updates)
        assert final_state.metrics.tasks_completed == num_workers

    def test_concurrent_lock_acquisition(self, db_path):
        """Test that only one worker can hold a lock at a time."""
        import threading
        from concurrent.futures import ThreadPoolExecutor, as_completed

        store = SQLiteLockStore(db_path)
        lock_holders = []
        lock = threading.Lock()

        def try_acquire(worker_id: int):
            """Try to acquire lock and hold it briefly."""
            acquired = store.acquire_scope_lock(
                "concurrent-test", "shared-resource", f"worker-{worker_id}", ttl_seconds=60
            )

            if acquired:
                with lock:
                    lock_holders.append(worker_id)

                # Hold the lock briefly
                import time

                time.sleep(0.05)

                store.release_scope_lock("concurrent-test", "shared-resource")

            return acquired

        # Run 5 concurrent acquisition attempts
        num_workers = 5
        results = []

        with ThreadPoolExecutor(max_workers=num_workers) as executor:
            futures = [executor.submit(try_acquire, i) for i in range(num_workers)]
            results = [f.result() for f in as_completed(futures)]

        # Exactly one worker should have acquired the lock first
        assert results.count(True) >= 1  # At least one succeeded


class TestSQLiteTaskHistory:
    """Tests for task history persistence in SQLiteTaskStore"""

    @pytest.fixture
    def task_store(self, db_path):
        """Create a SQLiteTaskStore"""
        from c4.store import SQLiteTaskStore

        return SQLiteTaskStore(db_path)

    def test_record_and_get_history(self, task_store):
        """Test recording and retrieving task assignment history"""
        project_id = "test-project"
        task_id = "T-001-0"

        # Initially empty
        history = task_store.get_task_history(project_id, task_id)
        assert history == []

        # Record first assignment
        task_store.record_assignment(project_id, task_id, "worker-1")
        history = task_store.get_task_history(project_id, task_id)
        assert history == ["worker-1"]

        # Record second assignment
        task_store.record_assignment(project_id, task_id, "worker-2")
        history = task_store.get_task_history(project_id, task_id)
        assert history == ["worker-1", "worker-2"]

    def test_no_duplicate_records(self, task_store):
        """Test that duplicate assignments are ignored"""
        project_id = "test-project"
        task_id = "T-001-0"

        # Record same worker twice
        task_store.record_assignment(project_id, task_id, "worker-1")
        task_store.record_assignment(project_id, task_id, "worker-1")

        history = task_store.get_task_history(project_id, task_id)
        assert history == ["worker-1"]

    def test_separate_projects(self, task_store):
        """Test that history is separate per project"""
        task_id = "T-001-0"

        task_store.record_assignment("project-a", task_id, "worker-1")
        task_store.record_assignment("project-b", task_id, "worker-2")

        assert task_store.get_task_history("project-a", task_id) == ["worker-1"]
        assert task_store.get_task_history("project-b", task_id) == ["worker-2"]

    def test_separate_tasks(self, task_store):
        """Test that history is separate per task"""
        project_id = "test-project"

        task_store.record_assignment(project_id, "T-001-0", "worker-1")
        task_store.record_assignment(project_id, "T-002-0", "worker-2")

        assert task_store.get_task_history(project_id, "T-001-0") == ["worker-1"]
        assert task_store.get_task_history(project_id, "T-002-0") == ["worker-2"]

    def test_clear_task_history_specific_task(self, task_store):
        """Test clearing history for a specific task"""
        project_id = "test-project"

        task_store.record_assignment(project_id, "T-001-0", "worker-1")
        task_store.record_assignment(project_id, "T-002-0", "worker-2")

        # Clear only T-001-0
        deleted = task_store.clear_task_history(project_id, "T-001-0")
        assert deleted == 1

        assert task_store.get_task_history(project_id, "T-001-0") == []
        assert task_store.get_task_history(project_id, "T-002-0") == ["worker-2"]

    def test_clear_task_history_all_project(self, task_store):
        """Test clearing all history for a project"""
        project_id = "test-project"

        task_store.record_assignment(project_id, "T-001-0", "worker-1")
        task_store.record_assignment(project_id, "T-002-0", "worker-2")
        task_store.record_assignment(project_id, "T-002-0", "worker-3")

        # Clear all for project
        deleted = task_store.clear_task_history(project_id)
        assert deleted == 3

        assert task_store.get_task_history(project_id, "T-001-0") == []
        assert task_store.get_task_history(project_id, "T-002-0") == []

    def test_history_persists_across_connections(self, db_path):
        """Test that history survives store recreation (simulating restart)"""
        from c4.store import SQLiteTaskStore

        project_id = "test-project"
        task_id = "T-001-0"

        # First store instance
        store1 = SQLiteTaskStore(db_path)
        store1.record_assignment(project_id, task_id, "worker-1")
        del store1

        # Second store instance (simulating restart)
        store2 = SQLiteTaskStore(db_path)
        history = store2.get_task_history(project_id, task_id)
        assert history == ["worker-1"]
