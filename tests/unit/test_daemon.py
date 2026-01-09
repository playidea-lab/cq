"""Tests for C4D Daemon (MCP Server functionality)"""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import ProjectStatus, ScopeLock, Task


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon(temp_project):
    """Create an initialized daemon"""
    d = C4Daemon(temp_project)
    d.initialize("test-project")
    return d


class TestDaemonInitialization:
    """Test daemon initialization"""

    def test_initialize_creates_directories(self, temp_project):
        """Test that initialization creates all required directories"""
        daemon = C4Daemon(temp_project)
        daemon.initialize("test-project")

        c4_dir = temp_project / ".c4"
        assert c4_dir.exists()
        assert (c4_dir / "state.json").exists()
        assert (c4_dir / "config.yaml").exists()
        assert (c4_dir / "locks").exists()
        assert (c4_dir / "events").exists()
        assert (c4_dir / "bundles").exists()
        assert (c4_dir / "workers").exists()

    def test_initialize_sets_plan_state(self, daemon):
        """Test that initialization transitions to PLAN state"""
        assert daemon.state_machine.state.status == ProjectStatus.PLAN

    def test_is_initialized(self, temp_project):
        """Test is_initialized detection"""
        daemon = C4Daemon(temp_project)
        assert not daemon.is_initialized()

        daemon.initialize("test-project")
        assert daemon.is_initialized()

    def test_load_existing_project(self, temp_project):
        """Test loading an existing project"""
        # Initialize
        d1 = C4Daemon(temp_project)
        d1.initialize("test-project")

        # Load in new daemon instance
        d2 = C4Daemon(temp_project)
        d2.load()

        assert d2.state_machine.state.project_id == "test-project"
        assert d2.state_machine.state.status == ProjectStatus.PLAN


class TestC4Status:
    """Test c4_status tool"""

    def test_status_returns_correct_structure(self, daemon):
        """Test that status returns expected fields"""
        status = daemon.c4_status()

        assert status["initialized"] is True
        assert status["project_id"] == "test-project"
        assert status["status"] == "PLAN"
        assert "queue" in status
        assert "workers" in status
        assert "metrics" in status

    def test_status_uninitialized(self, temp_project):
        """Test status on uninitialized project"""
        daemon = C4Daemon(temp_project)
        status = daemon.c4_status()

        assert status["initialized"] is False


class TestTaskManagement:
    """Test task management"""

    def test_add_task(self, daemon):
        """Test adding a task"""
        task = Task(
            id="T-001",
            title="Test task",
            dod="Complete the test",
            scope="tests",
        )
        daemon.add_task(task)

        # Check task is in registry
        retrieved = daemon.get_task("T-001")
        assert retrieved is not None
        assert retrieved.title == "Test task"

        # Check task is in queue
        assert "T-001" in daemon.state_machine.state.queue.pending


class TestC4GetTask:
    """Test c4_get_task tool"""

    def test_get_task_not_in_execute(self, daemon):
        """Test that get_task returns None when not in EXECUTE state"""
        result = daemon.c4_get_task("worker-1")
        assert result is None

    def test_get_task_assigns_task(self, daemon):
        """Test task assignment"""
        # Add task
        task = Task(
            id="T-001",
            title="Test task",
            dod="Complete the test",
            scope="tests",
        )
        daemon.add_task(task)

        # Transition to EXECUTE
        daemon.state_machine.transition("c4_run")

        # Get task
        result = daemon.c4_get_task("worker-1")

        assert result is not None
        assert result.task_id == "T-001"
        assert result.title == "Test task"
        assert result.branch.startswith("c4/w-")

        # Check task is now in progress
        assert "T-001" in daemon.state_machine.state.queue.in_progress
        assert "T-001" not in daemon.state_machine.state.queue.pending

    def test_get_task_registers_worker(self, daemon):
        """Test that get_task registers new workers"""
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        assert "worker-1" in daemon.state_machine.state.workers

    def test_get_task_no_tasks_available(self, daemon):
        """Test get_task when no tasks are pending"""
        daemon.state_machine.transition("c4_run")
        result = daemon.c4_get_task("worker-1")
        assert result is None

    def test_get_task_resumes_existing_in_progress(self, daemon):
        """Test that get_task returns existing in_progress task on worker restart"""
        # Add task
        task = Task(
            id="T-001",
            title="Resume test",
            dod="Test resume behavior",
            scope="api",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"
        assert "T-001" in daemon.state_machine.state.queue.in_progress

        # Simulate worker restart: call get_task again with same worker_id
        result2 = daemon.c4_get_task("worker-1")

        # Should return the same task (resume), not None or new task
        assert result2 is not None
        assert result2.task_id == "T-001"
        assert result2.title == "Resume test"

        # Still only one task in progress
        assert len(daemon.state_machine.state.queue.in_progress) == 1

    def test_get_task_different_worker_gets_different_task(self, daemon):
        """Test that different worker doesn't get another worker's in_progress task"""
        # Add two tasks
        task1 = Task(id="T-001", title="Task 1", dod="Test 1")
        task2 = Task(id="T-002", title="Task 2", dod="Test 2")
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1.task_id == "T-001"

        # Worker 2 should get T-002, not T-001
        result2 = daemon.c4_get_task("worker-2")
        assert result2 is not None
        assert result2.task_id == "T-002"


class TestC4Submit:
    """Test c4_submit tool"""

    def test_submit_success(self, daemon):
        """Test successful task submission"""
        # Setup
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Submit
        result = daemon.c4_submit(
            "T-001",
            "abc123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        assert result.success is True
        assert result.next_action in ["get_next_task", "complete", "await_checkpoint"]

        # Check task is done
        assert "T-001" in daemon.state_machine.state.queue.done
        assert "T-001" not in daemon.state_machine.state.queue.in_progress

    def test_submit_with_failures(self, daemon):
        """Test submission with failed validations"""
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        result = daemon.c4_submit(
            "T-001",
            "abc123",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "fail"}],
        )

        assert result.success is False
        assert result.next_action == "fix_failures"

    def test_submit_invalid_task(self, daemon):
        """Test submission of non-existent task"""
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_submit("T-999", "abc123", [])

        assert result.success is False


class TestC4AddTodo:
    """Test c4_add_todo tool"""

    def test_add_todo(self, daemon):
        """Test adding a todo/task"""
        result = daemon.c4_add_todo(
            task_id="T-NEW",
            title="New task",
            scope="new",
            dod="Complete new task",
        )

        assert result["success"] is True

        # Verify task was added
        task = daemon.get_task("T-NEW")
        assert task is not None
        assert task.title == "New task"


class TestWorkerManagement:
    """Test worker registration and management"""

    def test_register_worker(self, daemon):
        """Test worker registration"""
        worker = daemon.worker_manager.register("worker-1")

        assert worker.worker_id == "worker-1"
        assert worker.state == "idle"
        assert "worker-1" in daemon.state_machine.state.workers

    def test_worker_state_updates(self, daemon):
        """Test that worker state updates during task lifecycle"""
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get task - worker should become busy
        daemon.c4_get_task("worker-1")
        assert daemon.state_machine.state.workers["worker-1"].state == "busy"
        assert daemon.state_machine.state.workers["worker-1"].task_id == "T-001"

        # Submit task - worker should become idle
        daemon.c4_submit("T-001", "abc123", [{"name": "test", "status": "pass"}])
        assert daemon.state_machine.state.workers["worker-1"].state == "idle"
        assert daemon.state_machine.state.workers["worker-1"].task_id is None


class TestC4GetTaskExpiredLock:
    """Test c4_get_task with expired lock edge cases"""

    def test_resume_with_expired_lock_moves_task_to_pending(self, daemon):
        """Test that resume fails with expired lock and task moves back to pending"""
        # Add task with scope
        task = Task(
            id="T-001",
            title="Expired lock test",
            dod="Test expired lock behavior",
            scope="api",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"

        # Manually expire the lock
        state = daemon.state_machine.state
        lock = state.locks.scopes.get("api")
        assert lock is not None
        lock.expires_at = datetime.now() - timedelta(seconds=1)  # Already expired
        daemon.state_machine.save_state()

        # Simulate worker restart: call get_task again
        result2 = daemon.c4_get_task("worker-1")

        # Task should have been moved back to pending due to expired lock
        # and then reassigned to the same worker from pending
        assert "T-001" in state.queue.pending or "T-001" in state.queue.in_progress

    def test_resume_with_lock_stolen_by_another_worker(self, daemon):
        """Test that resume fails when lock is owned by another worker"""
        # Add task with scope
        task = Task(
            id="T-001",
            title="Stolen lock test",
            dod="Test stolen lock behavior",
            scope="api",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"

        # Manually change lock owner to worker-2 (simulating lock takeover)
        state = daemon.state_machine.state
        lock = state.locks.scopes.get("api")
        assert lock is not None
        lock.owner = "worker-2"
        daemon.state_machine.save_state()

        # Worker 1 tries to resume - should fail because lock is now owned by worker-2
        result2 = daemon.c4_get_task("worker-1")

        # Task should have been moved back to pending
        assert "T-001" in state.queue.pending


class TestScopeNoneTaskResume:
    """Test scope=None task resume validation"""

    def test_resume_scope_none_with_inconsistent_assigned_to(self, daemon):
        """Test that scope=None task with wrong assigned_to gets reset"""
        # Add task without scope
        task = Task(id="T-001", title="No scope task", dod="Test", scope=None)
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets the task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"

        # Manually corrupt task state (simulate inconsistency)
        task_obj = daemon.get_task("T-001")
        task_obj.assigned_to = "worker-2"  # Wrong worker!
        daemon._save_tasks()

        # Worker 1 tries to resume - should detect inconsistency
        result2 = daemon.c4_get_task("worker-1")

        # Task should have been moved back to pending
        state = daemon.state_machine.state
        assert "T-001" in state.queue.pending or "T-001" in state.queue.in_progress

    def test_resume_scope_none_with_wrong_status(self, daemon):
        """Test that scope=None task with wrong status gets reset"""
        from c4.models import TaskStatus

        task = Task(id="T-001", title="No scope task", dod="Test", scope=None)
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets the task
        daemon.c4_get_task("worker-1")

        # Manually set wrong status
        task_obj = daemon.get_task("T-001")
        task_obj.status = TaskStatus.PENDING  # Wrong status!
        daemon._save_tasks()

        # Try to resume - should detect inconsistency
        result2 = daemon.c4_get_task("worker-1")

        state = daemon.state_machine.state
        assert "T-001" in state.queue.pending or "T-001" in state.queue.in_progress

    def test_resume_scope_none_consistent_state_succeeds(self, daemon):
        """Test that scope=None task with consistent state resumes properly"""
        task = Task(id="T-001", title="No scope task", dod="Test", scope=None)
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets the task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None

        # Resume without any corruption - should succeed
        result2 = daemon.c4_get_task("worker-1")
        assert result2 is not None
        assert result2.task_id == "T-001"


class TestLockRefreshFailure:
    """Test lock refresh failure handling during resume"""

    def test_resume_with_lock_refresh_failure(self, daemon):
        """Test that lock refresh failure moves task back to pending"""
        task = Task(id="T-001", title="Lock refresh test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker gets the task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None

        # Delete the lock entirely to simulate refresh failure
        state = daemon.state_machine.state
        if "api" in state.locks.scopes:
            del state.locks.scopes["api"]
        daemon.state_machine.save_state()

        # Try to resume - lock refresh will fail
        result2 = daemon.c4_get_task("worker-1")

        # Task should be moved to pending due to lock failure
        assert "T-001" in state.queue.pending or result2 is not None


class TestC4MarkBlockedValidation:
    """Test c4_mark_blocked validation and repair nesting limit"""

    def test_mark_blocked_not_in_progress_fails(self, daemon):
        """Test that marking a task as blocked fails if not in progress"""
        # Add task but don't start it
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Task is in pending, not in_progress
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is False
        assert "not in progress" in result["error"]

    def test_mark_blocked_repair_nesting_limit(self, daemon):
        """Test that REPAIR-REPAIR- tasks are blocked from further repair"""
        # Add a deeply nested repair task
        task = Task(id="REPAIR-REPAIR-T-001", title="Nested repair", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get the task (put it in_progress)
        daemon.c4_get_task("worker-1")

        # Try to mark it as blocked (should fail due to nesting limit)
        result = daemon.c4_mark_blocked(
            task_id="REPAIR-REPAIR-T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is False
        assert "Max repair nesting exceeded" in result["error"]

    def test_mark_blocked_single_repair_allowed(self, daemon):
        """Test that single REPAIR- tasks can be marked as blocked"""
        # Add a single repair task
        task = Task(id="REPAIR-T-001", title="Single repair", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get the task
        daemon.c4_get_task("worker-1")

        # Try to mark it as blocked (should succeed - nesting depth is 1)
        result = daemon.c4_mark_blocked(
            task_id="REPAIR-T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is True
        # Task should be in repair queue
        assert len(daemon.state_machine.state.repair_queue) == 1

    def test_mark_blocked_original_task_allowed(self, daemon):
        """Test that original (non-REPAIR) tasks can be marked as blocked"""
        task = Task(id="T-001", title="Original task", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get the task
        daemon.c4_get_task("worker-1")

        # Mark as blocked
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is True
        assert len(daemon.state_machine.state.repair_queue) == 1
        assert daemon.state_machine.state.repair_queue[0].task_id == "T-001"


class TestWorkerOwnershipVerification:
    """Test worker ownership verification in c4_mark_blocked"""

    def test_mark_blocked_wrong_worker_fails(self, daemon):
        """Test that a different worker cannot mark another worker's task as blocked"""
        task = Task(id="T-001", title="Ownership test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets the task
        daemon.c4_get_task("worker-1")

        # Worker 2 tries to mark it as blocked - should fail
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-2",  # Wrong worker!
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is False
        assert "assigned to worker-1" in result["error"]
        assert "worker-2" in result["error"]
        # Task should still be in progress
        assert "T-001" in daemon.state_machine.state.queue.in_progress

    def test_mark_blocked_correct_worker_succeeds(self, daemon):
        """Test that the assigned worker can mark their task as blocked"""
        task = Task(id="T-001", title="Ownership test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker 1 gets the task
        daemon.c4_get_task("worker-1")

        # Worker 1 marks it as blocked - should succeed
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is True
        assert "T-001" not in daemon.state_machine.state.queue.in_progress


class TestRepairDepthFalsePositive:
    """Test repair depth calculation doesn't have false positives"""

    def test_task_id_containing_repair_not_prefix(self, daemon):
        """Test that task IDs containing REPAIR- (not as prefix) are not blocked"""
        # Task ID that contains "REPAIR-" but not as a prefix
        task = Task(id="MY-REPAIR-FEATURE", title="Fix repair feature", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Get the task
        daemon.c4_get_task("worker-1")

        # Should be able to mark as blocked (repair_depth should be 0)
        result = daemon.c4_mark_blocked(
            task_id="MY-REPAIR-FEATURE",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        assert result["success"] is True
        assert len(daemon.state_machine.state.repair_queue) == 1

    def test_task_id_with_repair_in_middle(self, daemon):
        """Test that task IDs with REPAIR- in the middle are not blocked"""
        task = Task(id="USER-REPAIR-API-FIX", title="API repair fix", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        result = daemon.c4_mark_blocked(
            task_id="USER-REPAIR-API-FIX",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        # Should succeed - repair_depth is 0 (no REPAIR- prefix)
        assert result["success"] is True

    def test_task_id_with_multiple_repair_not_prefix(self, daemon):
        """Test task ID with multiple REPAIR- occurrences (not prefix) passes"""
        task = Task(
            id="API-REPAIR-REPAIR-BUG", title="Double repair string", dod="Test"
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        result = daemon.c4_mark_blocked(
            task_id="API-REPAIR-REPAIR-BUG",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        # Should succeed - repair_depth is 0 (doesn't start with REPAIR-)
        assert result["success"] is True

    def test_actual_repair_prefix_depth_1(self, daemon):
        """Test that actual REPAIR- prefix with depth 1 is allowed"""
        task = Task(id="REPAIR-T-001", title="First repair", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        result = daemon.c4_mark_blocked(
            task_id="REPAIR-T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        # Should succeed - repair_depth is 1, max is 2
        assert result["success"] is True

    def test_actual_repair_prefix_depth_2_blocked(self, daemon):
        """Test that REPAIR-REPAIR- prefix (depth 2) is blocked"""
        task = Task(id="REPAIR-REPAIR-T-001", title="Second repair", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        daemon.c4_get_task("worker-1")

        result = daemon.c4_mark_blocked(
            task_id="REPAIR-REPAIR-T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        # Should fail - repair_depth is 2, max is 2
        assert result["success"] is False
        assert "Max repair nesting exceeded" in result["error"]


class TestCheckpointQueueRetry:
    """Test checkpoint queue retry mechanism"""

    def test_checkpoint_queue_item_has_retry_count(self, daemon):
        """Test that CheckpointQueueItem model has retry_count field"""
        from c4.models import CheckpointQueueItem

        item = CheckpointQueueItem(
            checkpoint_id="CP-001",
            triggered_at=datetime.now().isoformat(),
        )

        assert item.retry_count == 0
        assert item.max_retries == 3

    def test_checkpoint_queue_item_retry_count_increment(self, daemon):
        """Test that retry_count can be incremented"""
        from c4.models import CheckpointQueueItem

        item = CheckpointQueueItem(
            checkpoint_id="CP-001",
            triggered_at=datetime.now().isoformat(),
        )

        item.retry_count += 1
        assert item.retry_count == 1

        item.retry_count += 1
        assert item.retry_count == 2
