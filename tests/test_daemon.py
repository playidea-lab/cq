"""Tests for C4D Daemon (MCP Server functionality)"""

import tempfile
from pathlib import Path

import pytest

from c4d.mcp_server import C4Daemon
from c4d.models import ProjectStatus, Task


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
        worker = daemon.register_worker("worker-1")

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
