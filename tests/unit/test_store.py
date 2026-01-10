"""Unit tests for C4 Store"""

from datetime import datetime, timedelta

import pytest

from c4.models import C4State
from c4.store import (
    LocalFileLockStore,
    LocalFileStateStore,
    StateNotFoundError,
)


@pytest.fixture
def c4_dir(tmp_path):
    """Create a temporary .c4 directory"""
    c4_dir = tmp_path / ".c4"
    c4_dir.mkdir()
    return c4_dir


@pytest.fixture
def state_store(c4_dir):
    """Create a LocalFileStateStore"""
    return LocalFileStateStore(c4_dir)


@pytest.fixture
def lock_store(state_store):
    """Create a LocalFileLockStore"""
    return LocalFileLockStore(state_store)


class TestLocalFileStateStore:
    """Tests for LocalFileStateStore"""

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
        """Test that save creates .c4 directory if needed"""
        c4_dir = tmp_path / "new_project" / ".c4"
        store = LocalFileStateStore(c4_dir)

        state = C4State(project_id="test")
        store.save(state)

        assert c4_dir.exists()
        assert store.exists("test")


class TestLocalFileLockStore:
    """Tests for LocalFileLockStore"""

    def test_acquire_scope_lock(self, lock_store, state_store):
        """Test acquiring a scope lock"""
        # First need to save state
        state = C4State(project_id="test")
        state_store.save(state)

        result = lock_store.acquire_scope_lock(
            project_id="test",
            scope="backend",
            owner="worker-1",
            ttl_seconds=60,
        )
        assert result is True

    def test_acquire_lock_conflict(self, lock_store, state_store):
        """Test lock conflict with different owner"""
        state = C4State(project_id="test")
        state_store.save(state)

        # First worker acquires
        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Second worker tries to acquire same scope
        result = lock_store.acquire_scope_lock("test", "backend", "worker-2", 60)
        assert result is False

    def test_acquire_same_owner_refreshes(self, lock_store, state_store):
        """Test same owner can refresh lock"""
        state = C4State(project_id="test")
        state_store.save(state)

        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Same owner can re-acquire (refreshes TTL)
        result = lock_store.acquire_scope_lock("test", "backend", "worker-1", 120)
        assert result is True

    def test_release_scope_lock(self, lock_store, state_store):
        """Test releasing a scope lock"""
        state = C4State(project_id="test")
        state_store.save(state)

        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        result = lock_store.release_scope_lock("test", "backend")
        assert result is True

        # Now another worker can acquire
        result = lock_store.acquire_scope_lock("test", "backend", "worker-2", 60)
        assert result is True

    def test_release_nonexistent_lock(self, lock_store, state_store):
        """Test releasing non-existent lock"""
        state = C4State(project_id="test")
        state_store.save(state)

        result = lock_store.release_scope_lock("test", "nonexistent")
        assert result is False

    def test_get_scope_lock(self, lock_store, state_store):
        """Test getting lock info"""
        state = C4State(project_id="test")
        state_store.save(state)

        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        result = lock_store.get_scope_lock("test", "backend")
        assert result is not None
        owner, expires_at = result
        assert owner == "worker-1"
        assert expires_at > datetime.now()

    def test_get_nonexistent_lock(self, lock_store, state_store):
        """Test getting non-existent lock"""
        state = C4State(project_id="test")
        state_store.save(state)

        result = lock_store.get_scope_lock("test", "nonexistent")
        assert result is None

    def test_refresh_scope_lock(self, lock_store, state_store):
        """Test refreshing lock TTL"""
        state = C4State(project_id="test")
        state_store.save(state)

        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Refresh with longer TTL
        result = lock_store.refresh_scope_lock("test", "backend", "worker-1", 120)
        assert result is True

    def test_refresh_wrong_owner(self, lock_store, state_store):
        """Test refresh fails for wrong owner"""
        state = C4State(project_id="test")
        state_store.save(state)

        lock_store.acquire_scope_lock("test", "backend", "worker-1", 60)

        # Different owner cannot refresh
        result = lock_store.refresh_scope_lock("test", "backend", "worker-2", 120)
        assert result is False

    def test_cleanup_expired(self, lock_store, state_store):
        """Test cleaning up expired locks"""
        from c4.models import ScopeLock

        state = C4State(project_id="test")
        # Add an already-expired lock
        state.locks.scopes["expired"] = ScopeLock(
            owner="worker-1",
            scope="expired",
            expires_at=datetime.now() - timedelta(seconds=10),
        )
        # Add a valid lock
        state.locks.scopes["valid"] = ScopeLock(
            owner="worker-2",
            scope="valid",
            expires_at=datetime.now() + timedelta(seconds=60),
        )
        state_store.save(state)

        expired = lock_store.cleanup_expired("test")
        assert "expired" in expired
        assert "valid" not in expired

        # Verify expired lock is gone
        loaded = state_store.load("test")
        assert "expired" not in loaded.locks.scopes
        assert "valid" in loaded.locks.scopes

    def test_leader_lock(self, lock_store, state_store):
        """Test leader lock acquire/release"""
        state = C4State(project_id="test")
        state_store.save(state)

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


class TestSQLiteTaskStore:
    """Tests for SQLiteTaskStore"""

    @pytest.fixture
    def db_path(self, tmp_path):
        """Create a temporary database path"""
        return tmp_path / "c4.db"

    @pytest.fixture
    def task_store(self, db_path):
        """Create a SQLiteTaskStore"""
        from c4.store import SQLiteTaskStore
        return SQLiteTaskStore(db_path)

    @pytest.fixture
    def sample_task(self):
        """Create a sample task"""
        from c4.models import Task, TaskStatus
        return Task(
            id="T-001",
            title="Test task",
            scope="backend",
            dod="Complete the test",
            status=TaskStatus.PENDING,
        )

    def test_save_and_load_task(self, task_store, sample_task):
        """Test saving and loading a single task"""
        task_store.save("test-project", sample_task)

        loaded = task_store.get("test-project", "T-001")
        assert loaded is not None
        assert loaded.id == "T-001"
        assert loaded.title == "Test task"
        assert loaded.scope == "backend"

    def test_load_all_tasks(self, task_store, sample_task):
        """Test loading all tasks for a project"""
        from c4.models import Task

        task_store.save("test-project", sample_task)
        task2 = Task(id="T-002", title="Task 2", dod="Do it")
        task_store.save("test-project", task2)

        tasks = task_store.load_all("test-project")
        assert len(tasks) == 2
        assert "T-001" in tasks
        assert "T-002" in tasks

    def test_update_status(self, task_store, sample_task):
        """Test updating task status"""
        from c4.models import TaskStatus

        task_store.save("test-project", sample_task)

        result = task_store.update_status(
            "test-project",
            "T-001",
            "in_progress",
            assigned_to="worker-1",
            branch="c4/T-001",
        )
        assert result is True

        loaded = task_store.get("test-project", "T-001")
        assert loaded.status == TaskStatus.IN_PROGRESS
        assert loaded.assigned_to == "worker-1"
        assert loaded.branch == "c4/T-001"

    def test_update_status_nonexistent(self, task_store):
        """Test updating non-existent task returns False"""
        result = task_store.update_status("test-project", "T-999", "done")
        assert result is False

    def test_delete_task(self, task_store, sample_task):
        """Test deleting a task"""
        task_store.save("test-project", sample_task)

        result = task_store.delete("test-project", "T-001")
        assert result is True

        loaded = task_store.get("test-project", "T-001")
        assert loaded is None

    def test_exists(self, task_store, sample_task):
        """Test exists check"""
        assert task_store.exists("test-project") is False

        task_store.save("test-project", sample_task)
        assert task_store.exists("test-project") is True

    def test_migrate_from_json(self, task_store, tmp_path):
        """Test migration from tasks.json"""
        import json

        # Create a tasks.json file
        tasks_json = tmp_path / "tasks.json"
        tasks_data = [
            {"id": "T-001", "title": "Task 1", "dod": "Do 1", "status": "pending"},
            {"id": "T-002", "title": "Task 2", "dod": "Do 2", "status": "done"},
        ]
        tasks_json.write_text(json.dumps(tasks_data))

        # Migrate
        count = task_store.migrate_from_json("test-project", tasks_json)
        assert count == 2

        # Verify tasks are in SQLite
        tasks = task_store.load_all("test-project")
        assert len(tasks) == 2
        assert tasks["T-001"].title == "Task 1"
        assert tasks["T-002"].title == "Task 2"

    def test_save_all_tasks(self, task_store):
        """Test bulk save of tasks"""
        from c4.models import Task

        tasks = {
            "T-001": Task(id="T-001", title="Task 1", dod="Do 1"),
            "T-002": Task(id="T-002", title="Task 2", dod="Do 2"),
            "T-003": Task(id="T-003", title="Task 3", dod="Do 3"),
        }

        task_store.save_all("test-project", tasks)

        loaded = task_store.load_all("test-project")
        assert len(loaded) == 3

    def test_project_isolation(self, task_store, sample_task):
        """Test tasks are isolated by project"""
        from c4.models import Task

        # Save task to project A
        task_store.save("project-a", sample_task)

        # Save different task to project B
        task_b = Task(id="T-002", title="Task B", dod="Project B task")
        task_store.save("project-b", task_b)

        # Verify isolation
        tasks_a = task_store.load_all("project-a")
        tasks_b = task_store.load_all("project-b")

        assert len(tasks_a) == 1
        assert len(tasks_b) == 1
        assert "T-001" in tasks_a
        assert "T-002" in tasks_b
