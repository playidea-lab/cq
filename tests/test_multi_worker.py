"""Tests for Multi-Worker Support

Tests concurrent worker scenarios including:
- Multiple workers getting different tasks
- Scope locking and conflict prevention
- Lock expiration and refresh
- Worker state management
"""

import tempfile
import time
from pathlib import Path
from unittest.mock import patch

import pytest

from c4.mcp_server import C4Daemon
from c4.models import (
    Task,
    ValidationConfig,
)


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def multi_worker_daemon(temp_project):
    """Create daemon configured for multi-worker testing"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("multi-worker-test")

    # Configure with short TTL for testing
    daemon._config.scope_lock_ttl_sec = 5  # 5 seconds for testing
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'ok'",
            "unit": "echo 'ok'",
        },
        required=["lint", "unit"],
    )
    daemon._save_config()

    return daemon


class TestMultiWorkerTaskAssignment:
    """Test task assignment with multiple workers"""

    def test_two_workers_get_different_tasks(self, multi_worker_daemon):
        """Two workers should get different tasks"""
        daemon = multi_worker_daemon

        # Add multiple tasks with different scopes
        for i in range(4):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod=f"Complete task {i}",
                scope=f"scope-{i}",
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Worker 1 gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1 is not None

        # Worker 2 gets different task
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is not None
        assert assignment1.task_id != assignment2.task_id

        # Both workers are busy
        assert daemon.state_machine.state.workers["worker-1"].state == "busy"
        assert daemon.state_machine.state.workers["worker-2"].state == "busy"

    def test_same_scope_blocked(self, multi_worker_daemon):
        """Tasks with same scope should not be assigned to different workers"""
        daemon = multi_worker_daemon

        # Add tasks with SAME scope
        for i in range(2):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod=f"Complete task {i}",
                scope="shared-scope",  # Same scope
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Worker 1 gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1 is not None
        assert assignment1.task_id == "T-000"

        # Worker 2 cannot get second task (same scope locked)
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is None

        # Verify lock exists
        locks = daemon.state_machine.state.locks.scopes
        assert "shared-scope" in locks
        assert locks["shared-scope"].owner == "worker-1"

    def test_tasks_without_scope_can_run_parallel(self, multi_worker_daemon):
        """Tasks without scope should be assignable in parallel"""
        daemon = multi_worker_daemon

        # Add tasks WITHOUT scope
        for i in range(3):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod=f"Complete task {i}",
                scope=None,  # No scope
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # All workers get tasks
        assignments = []
        for i in range(3):
            assignment = daemon.c4_get_task(f"worker-{i}")
            assert assignment is not None
            assignments.append(assignment)

        # All tasks assigned
        task_ids = [a.task_id for a in assignments]
        assert len(set(task_ids)) == 3

    def test_worker_gets_task_after_other_completes(self, multi_worker_daemon):
        """Worker gets blocked task after other worker completes"""
        daemon = multi_worker_daemon

        # Add tasks with same scope
        task1 = Task(id="T-001", title="First", dod="First", scope="api")
        task2 = Task(id="T-002", title="Second", dod="Second", scope="api")
        daemon.add_task(task1)
        daemon.add_task(task2)

        daemon.state_machine.transition("c4_run")

        # Worker 1 gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1.task_id == "T-001"

        # Worker 2 is blocked
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is None

        # Worker 1 completes task
        daemon.c4_submit(
            "T-001",
            "commit1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Now Worker 2 can get the second task
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is not None
        assert assignment2.task_id == "T-002"


class TestScopeLockManagement:
    """Test scope lock lifecycle"""

    def test_lock_acquired_on_task_assignment(self, multi_worker_daemon):
        """Lock should be acquired when task is assigned"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task", scope="ui")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Before assignment - no lock
        assert "ui" not in daemon.state_machine.state.locks.scopes

        # Assign task
        daemon.c4_get_task("worker-1")

        # After assignment - lock exists
        assert "ui" in daemon.state_machine.state.locks.scopes
        lock = daemon.state_machine.state.locks.scopes["ui"]
        assert lock.owner == "worker-1"
        assert lock.scope == "ui"

    def test_lock_released_on_task_completion(self, multi_worker_daemon):
        """Lock should be released when task is completed"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")
        assert "api" in daemon.state_machine.state.locks.scopes

        # Complete task
        daemon.c4_submit(
            "T-001",
            "commit1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Lock should be released
        assert "api" not in daemon.state_machine.state.locks.scopes

    def test_lock_expiration(self, multi_worker_daemon):
        """Expired locks should allow other workers to take over"""
        daemon = multi_worker_daemon

        # Set very short TTL
        daemon._config.scope_lock_ttl_sec = 1

        task1 = Task(id="T-001", title="First", dod="First", scope="db")
        task2 = Task(id="T-002", title="Second", dod="Second", scope="db")
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets task
        daemon.c4_get_task("worker-1")
        assert "db" in daemon.state_machine.state.locks.scopes

        # Wait for lock to expire
        time.sleep(1.1)

        # Now lock is expired, Worker 2 can get task
        assignment = daemon.c4_get_task("worker-2")
        assert assignment is not None
        assert assignment.task_id == "T-002"

    def test_same_worker_can_take_task_with_own_lock(self, multi_worker_daemon):
        """Worker can take task in scope they already own"""
        daemon = multi_worker_daemon

        task1 = Task(id="T-001", title="First", dod="First", scope="ui")
        task2 = Task(id="T-002", title="Second", dod="Second", scope="ui")
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1.task_id == "T-001"

        # Worker 1 completes first task
        daemon.c4_submit(
            "T-001",
            "commit1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Worker 1 can get second task (same scope, lock was released)
        assignment2 = daemon.c4_get_task("worker-1")
        assert assignment2 is not None
        assert assignment2.task_id == "T-002"


class TestWorkerStateManagement:
    """Test worker state tracking"""

    def test_worker_registered_on_first_task(self, multi_worker_daemon):
        """Worker should be registered when first requesting task"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # No workers initially
        assert "worker-1" not in daemon.state_machine.state.workers

        # Get task
        daemon.c4_get_task("worker-1")

        # Worker registered
        assert "worker-1" in daemon.state_machine.state.workers
        worker = daemon.state_machine.state.workers["worker-1"]
        assert worker.state == "busy"
        assert worker.task_id == "T-001"

    def test_worker_state_idle_after_completion(self, multi_worker_daemon):
        """Worker should be idle after completing task"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")
        assert daemon.state_machine.state.workers["worker-1"].state == "busy"

        daemon.c4_submit(
            "T-001",
            "commit",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        assert daemon.state_machine.state.workers["worker-1"].state == "idle"
        assert daemon.state_machine.state.workers["worker-1"].task_id is None

    def test_multiple_workers_state_tracking(self, multi_worker_daemon):
        """Track state of multiple workers correctly"""
        daemon = multi_worker_daemon

        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"s-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # All workers get tasks
        for i in range(3):
            daemon.c4_get_task(f"worker-{i}")

        # Check all busy
        for i in range(3):
            assert daemon.state_machine.state.workers[f"worker-{i}"].state == "busy"

        # Worker 1 completes
        daemon.c4_submit(
            "T-001",
            "commit",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Worker 1 idle, others still busy
        assert daemon.state_machine.state.workers["worker-0"].state == "busy"
        assert daemon.state_machine.state.workers["worker-1"].state == "idle"
        assert daemon.state_machine.state.workers["worker-2"].state == "busy"


class TestDependencyResolution:
    """Test task dependency handling with multiple workers"""

    def test_dependency_blocks_task(self, multi_worker_daemon):
        """Task with unmet dependency should not be assignable"""
        daemon = multi_worker_daemon

        task1 = Task(id="T-001", title="First", dod="First", scope="a")
        task2 = Task(
            id="T-002",
            title="Second",
            dod="Second",
            scope="b",
            dependencies=["T-001"],  # Depends on T-001
        )
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets T-001
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1.task_id == "T-001"

        # Worker 2 cannot get T-002 (dependency not met)
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is None

    def test_dependency_satisfied_allows_task(self, multi_worker_daemon):
        """Task with satisfied dependency should be assignable"""
        daemon = multi_worker_daemon

        task1 = Task(id="T-001", title="First", dod="First")
        task2 = Task(
            id="T-002",
            title="Second",
            dod="Second",
            dependencies=["T-001"],
        )
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets and completes T-001
        daemon.c4_get_task("worker-1")
        daemon.c4_submit(
            "T-001",
            "commit",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Now Worker 2 can get T-002
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is not None
        assert assignment2.task_id == "T-002"


class TestConcurrentCompletion:
    """Test concurrent task completion scenarios"""

    def test_multiple_workers_complete_simultaneously(self, multi_worker_daemon):
        """Multiple workers can complete tasks at the same time"""
        daemon = multi_worker_daemon

        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"s-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # All workers get tasks
        for i in range(3):
            daemon.c4_get_task(f"worker-{i}")

        # All complete
        for i in range(3):
            result = daemon.c4_submit(
                f"T-{i:03d}",
                f"commit-{i}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
            )
            assert result.success is True

        # All done
        assert len(daemon.state_machine.state.queue.done) == 3
        assert len(daemon.state_machine.state.queue.in_progress) == 0

    @patch("subprocess.run")
    def test_validation_failure_doesnt_affect_other_workers(
        self, mock_run, multi_worker_daemon
    ):
        """One worker's validation failure shouldn't affect others"""
        daemon = multi_worker_daemon

        for i in range(2):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"s-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Both workers get tasks
        daemon.c4_get_task("worker-0")
        daemon.c4_get_task("worker-1")

        # Worker 0 fails validation
        result0 = daemon.c4_submit(
            "T-000",
            "commit-0",
            [{"name": "lint", "status": "fail"}, {"name": "unit", "status": "pass"}],
        )
        assert result0.success is False

        # Worker 1 succeeds
        result1 = daemon.c4_submit(
            "T-001",
            "commit-1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )
        assert result1.success is True

        # T-000 still in progress, T-001 done
        assert "T-000" in daemon.state_machine.state.queue.in_progress
        assert "T-001" in daemon.state_machine.state.queue.done


class TestStatusWithMultipleWorkers:
    """Test c4_status with multiple workers"""

    def test_status_shows_all_workers(self, multi_worker_daemon):
        """Status should show all registered workers"""
        daemon = multi_worker_daemon

        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"s-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        for i in range(3):
            daemon.c4_get_task(f"worker-{i}")

        status = daemon.c4_status()

        assert len(status["workers"]) == 3
        for i in range(3):
            assert f"worker-{i}" in status["workers"]
            assert status["workers"][f"worker-{i}"]["state"] == "busy"

    def test_status_shows_in_progress_map(self, multi_worker_daemon):
        """Status should show which worker has which task"""
        daemon = multi_worker_daemon

        for i in range(2):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"s-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-a")
        daemon.c4_get_task("worker-b")

        status = daemon.c4_status()

        in_progress_map = status["queue"]["in_progress_map"]
        assert "T-000" in in_progress_map
        assert "T-001" in in_progress_map
        assert in_progress_map["T-000"] == "worker-a"
        assert in_progress_map["T-001"] == "worker-b"


class TestLockManagement:
    """Test lock management utilities"""

    def test_refresh_scope_lock(self, multi_worker_daemon):
        """Worker can refresh their own lock"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        # Get initial expiry
        initial_expiry = daemon.state_machine.state.locks.scopes["api"].expires_at

        # Wait a bit
        time.sleep(0.1)

        # Refresh lock
        result = daemon._refresh_scope_lock("api", "worker-1")
        assert result is True

        # Expiry should be extended
        new_expiry = daemon.state_machine.state.locks.scopes["api"].expires_at
        assert new_expiry > initial_expiry

    def test_refresh_lock_wrong_owner(self, multi_worker_daemon):
        """Cannot refresh lock owned by another worker"""
        daemon = multi_worker_daemon

        task = Task(id="T-001", title="Task", dod="Task", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        # Worker 2 tries to refresh Worker 1's lock
        result = daemon._refresh_scope_lock("api", "worker-2")
        assert result is False

    def test_cleanup_expired_locks(self, multi_worker_daemon):
        """Expired locks should be cleaned up"""
        daemon = multi_worker_daemon
        daemon._config.scope_lock_ttl_sec = 1

        task = Task(id="T-001", title="Task", dod="Task", scope="db")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")
        assert "db" in daemon.state_machine.state.locks.scopes

        # Wait for expiration
        time.sleep(1.1)

        # Cleanup
        expired = daemon._cleanup_expired_locks()
        assert "db" in expired
        assert "db" not in daemon.state_machine.state.locks.scopes

    def test_get_lock_status(self, multi_worker_daemon):
        """Get lock status shows detailed info"""
        daemon = multi_worker_daemon

        for i in range(2):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Task", scope=f"scope-{i}")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-a")
        daemon.c4_get_task("worker-b")

        status = daemon.get_lock_status()

        assert status["total_locks"] == 2
        assert "scope-0" in status["locks"]
        assert "scope-1" in status["locks"]
        assert status["locks"]["scope-0"]["owner"] == "worker-a"
        assert status["locks"]["scope-1"]["owner"] == "worker-b"
        assert status["locks"]["scope-0"]["remaining_seconds"] > 0

    def test_lock_status_shows_expired(self, multi_worker_daemon):
        """Lock status shows when lock is expired"""
        daemon = multi_worker_daemon
        daemon._config.scope_lock_ttl_sec = 1

        task = Task(id="T-001", title="Task", dod="Task", scope="cache")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        # Wait for expiration
        time.sleep(1.1)

        status = daemon.get_lock_status()
        assert status["locks"]["cache"]["expired"] is True
        assert status["locks"]["cache"]["remaining_seconds"] == 0
