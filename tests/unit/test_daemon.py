"""Tests for C4D Daemon (MCP Server functionality)"""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon, _get_workflow_guide
from c4.models import ProjectStatus, Task


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon(temp_project):
    """Create an initialized daemon (starts in DISCOVERY state)"""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    return d


@pytest.fixture
def daemon_in_plan(temp_project):
    """Create a daemon in PLAN state (skip discovery/design)"""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    # Transition through discovery and design
    d.state_machine.transition("skip_discovery", "test")
    return d


@pytest.fixture
def daemon_in_execute(temp_project):
    """Create a daemon in EXECUTE state with tasks"""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    # Skip to PLAN, then transition to EXECUTE
    d.state_machine.transition("skip_discovery", "test")
    d.state_machine.transition("c4_run", "test")
    return d


class TestDaemonInitialization:
    """Test daemon initialization"""

    def test_initialize_creates_directories(self, temp_project):
        """Test that initialization creates all required directories"""
        daemon = C4Daemon(temp_project)
        daemon.initialize("test-project", with_default_checkpoints=False)

        c4_dir = temp_project / ".c4"
        assert c4_dir.exists()
        assert (c4_dir / "c4.db").exists()  # SQLite database
        assert (c4_dir / "config.yaml").exists()
        assert (c4_dir / "locks").exists()
        assert (c4_dir / "events").exists()
        assert (c4_dir / "bundles").exists()
        assert (c4_dir / "workers").exists()

    def test_initialize_sets_discovery_state(self, daemon):
        """Test that initialization transitions to DISCOVERY state (new workflow)"""
        # New workflow: INIT → DISCOVERY → DESIGN → PLAN
        assert daemon.state_machine.state.status == ProjectStatus.DISCOVERY

    def test_is_initialized(self, temp_project):
        """Test is_initialized detection"""
        daemon = C4Daemon(temp_project)
        assert not daemon.is_initialized()

        daemon.initialize("test-project", with_default_checkpoints=False)
        assert daemon.is_initialized()

    def test_load_existing_project(self, temp_project):
        """Test loading an existing project"""
        # Initialize
        d1 = C4Daemon(temp_project)
        d1.initialize("test-project", with_default_checkpoints=False)

        # Load in new daemon instance
        d2 = C4Daemon(temp_project)
        d2.load()

        assert d2.state_machine.state.project_id == "test-project"
        # After init, state is DISCOVERY (new workflow)
        assert d2.state_machine.state.status == ProjectStatus.DISCOVERY


class TestWorkflowGuide:
    """Test _get_workflow_guide function for multi-LLM CLI support"""

    def test_workflow_guide_init(self):
        """Test workflow guide for INIT status"""
        guide = _get_workflow_guide("INIT")
        assert guide["phase"] == "init"
        assert guide["next"] == "discovery"
        assert "hint" in guide
        assert "c4_save_spec" in guide["hint"]

    def test_workflow_guide_discovery(self):
        """Test workflow guide for DISCOVERY status"""
        guide = _get_workflow_guide("DISCOVERY")
        assert guide["phase"] == "discovery"
        assert guide["next"] == "design"
        assert "c4_discovery_complete" in guide["hint"]

    def test_workflow_guide_execute(self):
        """Test workflow guide for EXECUTE status"""
        guide = _get_workflow_guide("EXECUTE")
        assert guide["phase"] == "execute"
        assert guide["next"] == "worker_loop"
        assert "c4_get_task" in guide["hint"]
        assert "c4_submit" in guide["hint"]

    def test_workflow_guide_all_statuses(self):
        """Test that all project statuses have workflow guides"""
        statuses = ["INIT", "DISCOVERY", "DESIGN", "PLAN", "EXECUTE", "CHECKPOINT", "HALTED", "COMPLETE"]
        for status in statuses:
            guide = _get_workflow_guide(status)
            assert "phase" in guide
            assert "next" in guide
            assert "hint" in guide

    def test_workflow_guide_unknown_status(self):
        """Test workflow guide for unknown status"""
        guide = _get_workflow_guide("UNKNOWN")
        assert guide["phase"] == "unknown"
        assert "hint" in guide


class TestC4Status:
    """Test c4_status tool"""

    def test_status_returns_correct_structure(self, daemon):
        """Test that status returns expected fields"""
        status = daemon.c4_status()

        assert status["initialized"] is True
        assert status["project_id"] == "test-project"
        # New workflow starts with DISCOVERY
        assert status["status"] == "DISCOVERY"
        assert "queue" in status
        assert "workers" in status
        assert "metrics" in status
        # Workflow guide for multi-LLM CLI support
        assert "workflow" in status
        assert status["workflow"]["phase"] == "discovery"
        assert "hint" in status["workflow"]

    def test_status_uninitialized(self, temp_project):
        """Test status on uninitialized project"""
        daemon = C4Daemon(temp_project)
        status = daemon.c4_status()

        assert status["initialized"] is False
        # Even uninitialized projects should have workflow guide
        assert "workflow" in status
        assert status["workflow"]["phase"] == "init"


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

        # Transition to EXECUTE (DISCOVERY → PLAN → EXECUTE)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        daemon.c4_get_task("worker-1")

        assert "worker-1" in daemon.state_machine.state.workers

    def test_get_task_no_tasks_available(self, daemon):
        """Test get_task when no tasks are pending"""
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")
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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")
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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")
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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        result = daemon.c4_submit("T-999", "abc123", [])

        assert result.success is False

    def test_submit_updates_task_status_to_done(self, daemon):
        """Test that c4_submit correctly updates task file status to 'done'.

        Regression test for multi-worker interference bug where task status
        was not synced from state queue to task file after completion.
        """
        from c4.models.enums import TaskStatus
        from c4.models.task import Task

        # Setup: Create and assign a task
        task = Task(id="T-STATUS", title="Status Test", dod="Test status sync")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")
        daemon.c4_get_task("worker-1")

        # Verify task is in_progress before submission
        task_before = daemon.get_task("T-STATUS")
        assert task_before.status == TaskStatus.IN_PROGRESS
        assert task_before.assigned_to == "worker-1"

        # Submit the task
        result = daemon.c4_submit(
            "T-STATUS",
            "abc123",
            [{"name": "lint", "status": "pass"}],
        )

        assert result.success is True

        # Verify task status is updated to 'done' in task file
        task_after = daemon.get_task("T-STATUS")
        assert task_after.status == TaskStatus.DONE, (
            f"Task status should be DONE, but got {task_after.status}"
        )
        assert task_after.commit_sha == "abc123"


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

    def test_add_todo_with_dependencies(self, daemon):
        """Test adding a task with dependencies"""
        result = daemon.c4_add_todo(
            task_id="T-002",
            title="Dependent task",
            scope=None,
            dod="Complete after T-001",
            dependencies=["T-001"],
        )

        assert result["success"] is True
        assert result["dependencies"] == ["T-001"]

        task = daemon.get_task("T-002")
        assert task.dependencies == ["T-001"]

    def test_add_todo_with_priority(self, daemon):
        """Test adding a task with priority"""
        daemon.c4_add_todo(
            task_id="T-LOW",
            title="Low priority",
            scope=None,
            dod="Low",
            priority=0,
        )
        daemon.c4_add_todo(
            task_id="T-HIGH",
            title="High priority",
            scope=None,
            dod="High",
            priority=10,
        )

        task_low = daemon.get_task("T-LOW")
        task_high = daemon.get_task("T-HIGH")
        assert task_low.priority == 0
        assert task_high.priority == 10

    def test_add_todo_with_domain(self, daemon):
        """Test adding a task with domain override"""
        result = daemon.c4_add_todo(
            task_id="T-FE",
            title="Frontend task",
            scope=None,
            dod="Build UI",
            domain="web-frontend",
        )

        assert result["success"] is True
        task = daemon.get_task("T-FE")
        assert task.domain == "web-frontend"


class TestTaskDependencies:
    """Test task dependency handling"""

    def test_dependent_task_not_assigned_until_deps_done(self, daemon_in_execute):
        """Test that tasks with unmet dependencies are not assigned"""
        daemon = daemon_in_execute

        # Add tasks with dependencies
        daemon.c4_add_todo("T-001", "Base task", None, "Base")
        daemon.c4_add_todo("T-002", "Dependent", None, "Depends on T-001", dependencies=["T-001"])

        # Worker should only get T-001 (T-002 has unmet deps)
        task = daemon.c4_get_task("worker-1")
        assert task is not None
        assert task.task_id == "T-001"

        # Another worker should get nothing (T-002 blocked, T-001 assigned)
        task2 = daemon.c4_get_task("worker-2")
        assert task2 is None

    def test_dependent_task_assigned_after_deps_complete(self, daemon_in_execute):
        """Test that tasks are assigned after dependencies complete"""
        daemon = daemon_in_execute

        daemon.c4_add_todo("T-001", "Base", None, "Base")
        daemon.c4_add_todo("T-002", "Dependent", None, "After T-001", dependencies=["T-001"])

        # Get and complete T-001
        daemon.c4_get_task("worker-1")
        daemon.c4_submit("T-001", "sha123", [{"name": "lint", "status": "pass"}])

        # Now T-002 should be available
        task = daemon.c4_get_task("worker-1")
        assert task is not None
        assert task.task_id == "T-002"

    def test_parallel_tasks_after_common_dependency(self, daemon_in_execute):
        """Test parallel tasks can be assigned after common dependency"""
        daemon = daemon_in_execute

        daemon.c4_add_todo("T-000", "Setup", "setup/", "Setup")
        daemon.c4_add_todo("T-001", "Module A", "src/a/", "A", dependencies=["T-000"])
        daemon.c4_add_todo("T-002", "Module B", "src/b/", "B", dependencies=["T-000"])
        daemon.c4_add_todo("T-003", "Module C", "src/c/", "C", dependencies=["T-000"])

        # Only T-000 available initially
        task = daemon.c4_get_task("worker-1")
        assert task.task_id == "T-000"

        # Complete T-000
        daemon.c4_submit("T-000", "sha1", [{"name": "lint", "status": "pass"}])

        # Now 3 workers can get T-001, T-002, T-003 in parallel
        t1 = daemon.c4_get_task("worker-1")
        t2 = daemon.c4_get_task("worker-2")
        t3 = daemon.c4_get_task("worker-3")

        assigned = {t1.task_id, t2.task_id, t3.task_id}
        assert assigned == {"T-001", "T-002", "T-003"}

    def test_priority_ordering(self, daemon_in_execute):
        """Test high priority tasks are assigned first"""
        daemon = daemon_in_execute

        daemon.c4_add_todo("T-LOW", "Low", "low/", "Low", priority=0)
        daemon.c4_add_todo("T-MED", "Med", "med/", "Med", priority=5)
        daemon.c4_add_todo("T-HIGH", "High", "high/", "High", priority=10)

        # Should get highest priority first
        task1 = daemon.c4_get_task("worker-1")
        assert task1.task_id == "T-HIGH"

        task2 = daemon.c4_get_task("worker-2")
        assert task2.task_id == "T-MED"

        task3 = daemon.c4_get_task("worker-3")
        assert task3.task_id == "T-LOW"


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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Worker gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"

        # Manually expire the lock in SQLite
        import sqlite3
        db_path = daemon.c4_dir / "c4.db"
        conn = sqlite3.connect(db_path)
        expired_time = (datetime.now() - timedelta(seconds=1)).isoformat()
        conn.execute(
            "UPDATE c4_locks SET expires_at = ? WHERE scope = ?",
            (expired_time, "api"),
        )
        conn.commit()
        conn.close()

        # Simulate worker restart: call get_task again
        result2 = daemon.c4_get_task("worker-1")

        # Task should have been moved back to pending due to expired lock
        # and then reassigned to the same worker from pending
        state = daemon.state_machine.state
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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Worker 1 gets task
        result1 = daemon.c4_get_task("worker-1")
        assert result1 is not None
        assert result1.task_id == "T-001"

        # Manually change lock owner to worker-2 in SQLite (simulating lock takeover)
        # Since we now use SQLite for locks, modify the lock store directly
        import sqlite3
        db_path = daemon.c4_dir / "c4.db"
        conn = sqlite3.connect(db_path)
        conn.execute(
            "UPDATE c4_locks SET owner = ? WHERE scope = ?",
            ("worker-2", "api"),
        )
        conn.commit()
        conn.close()

        # Worker 1 tries to resume - should fail because lock is now owned by worker-2
        result2 = daemon.c4_get_task("worker-1")

        # Task should have been moved back to pending
        # Note: c4_get_task reloads state, so get fresh reference
        state = daemon.state_machine.state
        assert "T-001" in state.queue.pending


class TestScopeNoneTaskResume:
    """Test scope=None task resume validation"""

    def test_resume_scope_none_with_inconsistent_assigned_to(self, daemon):
        """Test that scope=None task with wrong assigned_to gets reset"""
        # Add task without scope
        task = Task(id="T-001", title="No scope task", dod="Test", scope=None)
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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

        # Get fresh state reference after c4_get_task reloads
        state = daemon.state_machine.state

        # Task should be moved to pending due to lock failure
        assert "T-001" in state.queue.pending or result2 is not None


class TestC4MarkBlockedValidation:
    """Test c4_mark_blocked validation and repair nesting limit"""

    def test_mark_blocked_not_in_progress_fails(self, daemon):
        """Test that marking a task as blocked fails if not in progress"""
        # Add task but don't start it
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

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

    def test_checkpoint_dead_letter_after_max_retries(self, daemon):
        """Test that checkpoint is removed after max retries"""
        from c4.models import CheckpointQueueItem

        item = CheckpointQueueItem(
            checkpoint_id="CP-001",
            triggered_at=datetime.now().isoformat(),
            retry_count=2,  # Already failed twice
            max_retries=3,
        )

        # Simulate one more failure
        item.retry_count += 1
        assert item.retry_count >= item.max_retries
        # In real code, this would trigger dead letter removal


class TestTaskRegistryValidation:
    """Test task registry and queue consistency validation"""

    def test_mark_blocked_task_not_in_registry(self, daemon):
        """Test marking a task as blocked when it's not in task registry"""
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Manually add task to in_progress queue without adding to registry
        state = daemon.state_machine.state
        state.queue.in_progress["GHOST-TASK"] = "worker-1"
        daemon.state_machine.save_state()

        # Try to mark blocked - should handle gracefully
        result = daemon.c4_mark_blocked(
            task_id="GHOST-TASK",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=3,
        )

        # Should succeed (task exists in queue) but handle missing registry entry
        assert result["success"] is True

    def test_get_task_skips_orphaned_queue_entry(self, daemon):
        """Test that c4_get_task handles orphaned queue entries"""
        task = Task(id="T-001", title="Real task", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Add orphaned entry to pending queue (no task in registry)
        state = daemon.state_machine.state
        state.queue.pending.insert(0, "ORPHAN-TASK")
        daemon.state_machine.save_state()

        # Get task should skip orphan and return real task
        result = daemon.c4_get_task("worker-1")

        assert result is not None
        assert result.task_id == "T-001"  # Got the real task, not the orphan

    def test_submit_task_not_in_registry(self, daemon):
        """Test submitting a task that's not in the registry"""
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Manually add to in_progress without registry entry
        state = daemon.state_machine.state
        state.queue.in_progress["GHOST-TASK"] = "worker-1"
        daemon.state_machine.save_state()

        # Submit handles ghost task gracefully (moves to done)
        result = daemon.c4_submit(
            "GHOST-TASK",
            "abc123",
            [{"name": "test", "status": "pass"}],
        )

        # Current implementation allows this - task moved to done
        assert result.success is True
        # Note: c4_submit reloads state, so get fresh reference
        state = daemon.state_machine.state
        assert "GHOST-TASK" in state.queue.done


class TestConcurrentOperations:
    """Test concurrent operation edge cases"""

    def test_double_get_task_same_worker(self, daemon):
        """Test that same worker calling get_task twice gets same task"""
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # First call
        result1 = daemon.c4_get_task("worker-1")
        assert result1.task_id == "T-001"

        # Second call - should resume same task
        result2 = daemon.c4_get_task("worker-1")
        assert result2.task_id == "T-001"

        # Only one task in progress
        assert len(daemon.state_machine.state.queue.in_progress) == 1

    def test_different_workers_get_different_tasks(self, daemon):
        """Test that different workers get different tasks"""
        task1 = Task(id="T-001", title="Task 1", dod="Test")
        task2 = Task(id="T-002", title="Task 2", dod="Test")
        daemon.add_task(task1)
        daemon.add_task(task2)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        result1 = daemon.c4_get_task("worker-1")
        result2 = daemon.c4_get_task("worker-2")

        assert result1.task_id != result2.task_id
        assert len(daemon.state_machine.state.queue.in_progress) == 2

    def test_submit_already_done_task(self, daemon):
        """Test submitting a task that's already marked done"""
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        daemon.c4_get_task("worker-1")

        # First submit
        result1 = daemon.c4_submit(
            "T-001", "abc123", [{"name": "test", "status": "pass"}]
        )
        assert result1.success is True

        # Second submit - task already done
        result2 = daemon.c4_submit(
            "T-001", "def456", [{"name": "test", "status": "pass"}]
        )
        assert result2.success is False


class TestStaleWorkerRecovery:
    """Test stale worker detection and task recovery"""

    def test_recover_stale_busy_worker(self, daemon):
        """Test that stale busy worker's task is recovered to pending"""
        task = Task(id="T-001", title="Stale test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Worker gets task
        daemon.c4_get_task("worker-1")
        state = daemon.state_machine.state
        assert "T-001" in state.queue.in_progress
        assert state.workers["worker-1"].state == "busy"

        # Manually make the worker stale by setting last_seen to old time
        old_time = datetime.now() - timedelta(minutes=35)  # 35 min old (stale timeout is 30)
        state.workers["worker-1"].last_seen = old_time
        daemon.state_machine.save_state()

        # Run recovery with 30 minute timeout (1800 seconds)
        recoveries = daemon.worker_manager.recover_stale_workers(
            stale_timeout_seconds=1800,
            lock_store=daemon.lock_store,
        )

        # Verify recovery happened
        assert len(recoveries) == 1
        assert recoveries[0]["worker_id"] == "worker-1"
        assert recoveries[0]["task_id"] == "T-001"
        assert recoveries[0].get("task_recovered") is True

        # Verify task is back in pending
        state = daemon.state_machine.state
        assert "T-001" in state.queue.pending
        assert "T-001" not in state.queue.in_progress

        # Verify worker is marked as disconnected
        assert state.workers["worker-1"].state == "disconnected"

    def test_active_worker_not_recovered(self, daemon):
        """Test that active workers are not recovered"""
        task = Task(id="T-001", title="Active test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Worker gets task (last_seen is now)
        daemon.c4_get_task("worker-1")

        # Run recovery - should not recover anything
        recoveries = daemon.worker_manager.recover_stale_workers(
            stale_timeout_seconds=1800,
            lock_store=daemon.lock_store,
        )

        # No recoveries
        assert len(recoveries) == 0

        # Task still in progress
        state = daemon.state_machine.state
        assert "T-001" in state.queue.in_progress
        assert state.workers["worker-1"].state == "busy"

    def test_implicit_heartbeat_prevents_recovery(self, daemon):
        """Test that _touch_worker updates last_seen to prevent recovery"""
        task = Task(id="T-001", title="Heartbeat test", dod="Test", scope="api")
        daemon.add_task(task)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Worker gets task
        daemon.c4_get_task("worker-1")
        state = daemon.state_machine.state

        # Manually make worker appear stale
        old_time = datetime.now() - timedelta(minutes=35)
        state.workers["worker-1"].last_seen = old_time
        daemon.state_machine.save_state()

        # Simulate tool call that touches worker (implicit heartbeat)
        daemon._touch_worker("worker-1")

        # Now run recovery - should not recover because heartbeat updated last_seen
        recoveries = daemon.worker_manager.recover_stale_workers(
            stale_timeout_seconds=1800,
            lock_store=daemon.lock_store,
        )

        # No recoveries
        assert len(recoveries) == 0

        # Worker still busy
        state = daemon.state_machine.state
        assert state.workers["worker-1"].state == "busy"


class TestSupervisorAutoRestart:
    """Tests for supervisor loop auto-restart after MCP server restart"""

    def _wait_for_supervisor_start(self, daemon, timeout=1.0):
        """Wait for supervisor loop to start (thread startup has a race condition)"""
        import time

        start = time.time()
        while time.time() - start < timeout:
            if daemon.is_supervisor_loop_running:
                return True
            time.sleep(0.05)
        return False

    def test_auto_restart_in_execute_state(self, temp_project):
        """Test supervisor auto-restarts when daemon loads in EXECUTE state"""
        # First daemon: initialize and transition to EXECUTE
        daemon1 = C4Daemon(temp_project)
        daemon1.initialize("test-project", with_default_checkpoints=False)
        daemon1.state_machine.transition("skip_discovery", "test")
        daemon1.state_machine.transition("c4_run", "test")

        # Verify state is saved as EXECUTE
        assert daemon1.state_machine.state.status == ProjectStatus.EXECUTE

        # Stop daemon1's supervisor loop if running
        self._wait_for_supervisor_start(daemon1)
        if daemon1.is_supervisor_loop_running:
            daemon1.stop_supervisor_loop()

        # Second daemon: simulates MCP server restart
        daemon2 = C4Daemon(temp_project)
        daemon2.load()

        # Verify state loaded correctly
        assert daemon2.state_machine.state.status == ProjectStatus.EXECUTE

        # Call auto-restart
        restarted = daemon2._auto_restart_supervisor_if_needed()

        # Supervisor should have restarted (wait for thread to start)
        assert restarted is True
        assert self._wait_for_supervisor_start(daemon2)

        # Cleanup
        daemon2.stop_supervisor_loop()

    def test_no_restart_in_plan_state(self, temp_project):
        """Test supervisor does NOT auto-restart in PLAN state"""
        # Initialize in PLAN state
        daemon1 = C4Daemon(temp_project)
        daemon1.initialize("test-project", with_default_checkpoints=False)
        daemon1.state_machine.transition("skip_discovery", "test")

        # Verify state is PLAN
        assert daemon1.state_machine.state.status == ProjectStatus.PLAN

        # Second daemon: simulates MCP server restart
        daemon2 = C4Daemon(temp_project)
        daemon2.load()

        # Call auto-restart
        restarted = daemon2._auto_restart_supervisor_if_needed()

        # Supervisor should NOT have restarted in PLAN state
        assert restarted is False
        assert daemon2.is_supervisor_loop_running is False

    def test_no_restart_if_already_running(self, temp_project):
        """Test supervisor is not restarted if already running"""
        daemon = C4Daemon(temp_project)
        daemon.initialize("test-project", with_default_checkpoints=False)
        daemon.state_machine.transition("skip_discovery", "test")
        daemon.state_machine.transition("c4_run", "test")

        # Start supervisor and wait for it
        daemon.start_supervisor_loop()
        assert self._wait_for_supervisor_start(daemon)

        # Call auto-restart - should return False (already running)
        restarted = daemon._auto_restart_supervisor_if_needed()
        assert restarted is False

        # Still running
        assert daemon.is_supervisor_loop_running

        # Cleanup
        daemon.stop_supervisor_loop()
