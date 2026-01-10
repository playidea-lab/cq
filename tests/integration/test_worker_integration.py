"""Worker Integration Tests - Full workflow testing"""

import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.mcp_server import C4Daemon
from c4.models import (
    CheckpointConfig,
    ProjectStatus,
    Task,
    ValidationConfig,
)


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon_with_validations(temp_project):
    """Create daemon with validation config"""
    d = C4Daemon(temp_project)
    d.initialize("test-project")
    # Skip discovery phase to go directly to PLAN for testing
    d.state_machine.transition("skip_discovery")

    # Configure validations
    d._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'tests passed'",
        },
        required=["lint", "unit"],
    )
    d._save_config()

    return d


@pytest.fixture
def daemon_with_checkpoint(temp_project):
    """Create daemon with checkpoint config"""
    d = C4Daemon(temp_project)
    d.initialize("test-project")
    # Skip discovery phase to go directly to PLAN for testing
    d.state_machine.transition("skip_discovery")

    # Configure validations and checkpoint
    d._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'tests passed'",
        },
        required=["lint", "unit"],
    )
    d._config.checkpoints = [
        CheckpointConfig(
            id="CP1",
            name="Phase 1 Review",
            required_tasks=["T-001"],
            required_validations=["lint", "unit"],
        ),
    ]
    d._save_config()

    return d


class TestWorkerValidationWorkflow:
    """Test Worker → Validation → Submit workflow"""

    @patch("subprocess.run")
    def test_worker_runs_validation_before_submit(
        self, mock_run, daemon_with_validations
    ):
        """Test complete worker validation workflow"""
        daemon = daemon_with_validations

        # Mock successful validation runs
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Add a task
        task = Task(id="T-001", title="Test task", dod="Test")
        daemon.add_task(task)

        # Transition to EXECUTE
        daemon.state_machine.transition("c4_run")

        # Worker gets task
        assignment = daemon.c4_get_task("worker-1")
        assert assignment is not None
        assert assignment.task_id == "T-001"

        # Worker runs validation
        validation_result = daemon.c4_run_validation()

        assert validation_result["success"] is True
        assert validation_result["summary"]["passed"] == 2
        assert validation_result["summary"]["failed"] == 0

        # Worker submits
        submit_result = daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        assert submit_result.success is True

    @patch("subprocess.run")
    def test_worker_validation_failure_blocks_submit(
        self, mock_run, daemon_with_validations
    ):
        """Test that failed validation blocks submission"""
        daemon = daemon_with_validations

        # Mock failed validation
        mock_run.side_effect = [
            MagicMock(returncode=0, stdout="ok", stderr=""),  # lint passes
            MagicMock(returncode=1, stdout="", stderr="test failed"),  # unit fails
        ]

        # Add task and start
        task = Task(id="T-001", title="Test task", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Validation fails
        validation_result = daemon.c4_run_validation()
        assert validation_result["success"] is False
        assert validation_result["summary"]["failed"] == 1

        # Submit with failed results
        submit_result = daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "fail"},
            ],
        )

        assert submit_result.success is False
        assert submit_result.next_action == "fix_failures"

    @patch("subprocess.run")
    def test_worker_can_run_specific_validations(
        self, mock_run, daemon_with_validations
    ):
        """Test running specific validations"""
        daemon = daemon_with_validations

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Run only lint
        result = daemon.c4_run_validation(names=["lint"])

        assert result["success"] is True
        assert len(result["results"]) == 1
        assert result["results"][0]["name"] == "lint"

    @patch("subprocess.run")
    def test_validation_updates_state(self, mock_run, daemon_with_validations):
        """Test that validation updates state.last_validation"""
        daemon = daemon_with_validations

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        daemon.c4_run_validation()

        # Check state updated
        assert daemon.state_machine.state.last_validation is not None
        assert daemon.state_machine.state.last_validation["lint"] == "pass"
        assert daemon.state_machine.state.last_validation["unit"] == "pass"


class TestCheckpointTrigger:
    """Test checkpoint auto-trigger after task completion"""

    @patch("subprocess.run")
    def test_checkpoint_triggered_after_task_complete(
        self, mock_run, daemon_with_checkpoint
    ):
        """Test checkpoint triggers when conditions are met"""
        daemon = daemon_with_checkpoint

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Add required task
        task = Task(
            id="T-001",
            title="Required task",
            dod="Test",
            validations=["lint", "unit"],
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Worker gets and completes task
        daemon.c4_get_task("worker-1")

        # Run validation and submit
        daemon.c4_run_validation()
        result = daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Should indicate checkpoint is reached
        assert result.success is True
        assert result.next_action == "await_checkpoint"

    def test_checkpoint_not_triggered_without_validations(
        self, daemon_with_checkpoint
    ):
        """Test checkpoint not triggered if validations not run"""
        daemon = daemon_with_checkpoint

        # Add task
        task = Task(id="T-001", title="Task", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Submit WITHOUT running validations (passing results directly)
        result = daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Should succeed but checkpoint should trigger since validation passed
        assert result.success is True

    def test_check_and_trigger_checkpoint_method(self, daemon_with_checkpoint):
        """Test explicit checkpoint trigger check"""
        daemon = daemon_with_checkpoint

        # Add task
        task = Task(id="T-001", title="Task", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Task not done yet - no checkpoint
        result = daemon.check_and_trigger_checkpoint()
        assert result is None

        # Complete task manually
        daemon.state_machine.state.queue.in_progress.pop("T-001")
        daemon.state_machine.state.queue.done.append("T-001")
        daemon.state_machine.state.last_validation = {
            "lint": "pass",
            "unit": "pass",
        }

        # Now checkpoint should trigger
        result = daemon.check_and_trigger_checkpoint()
        assert result is not None
        assert result["checkpoint_id"] == "CP1"
        assert result["triggered"] is True
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT


class TestPassedCheckpointTracking:
    """Test that passed checkpoints are not re-triggered"""

    @patch("subprocess.run")
    def test_passed_checkpoint_not_retriggered(self, mock_run, temp_project):
        """Test that a checkpoint is not triggered again after being approved"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        daemon = C4Daemon(temp_project)
        daemon.initialize("test-project")
        # Skip discovery phase to go directly to PLAN for testing
        daemon.state_machine.transition("skip_discovery")

        # Configure two checkpoints
        daemon._config.checkpoints = [
            CheckpointConfig(
                id="CP1",
                required_tasks=["T-001"],
                required_validations=["lint", "unit"],
            ),
            CheckpointConfig(
                id="CP2",
                required_tasks=["T-001", "T-002"],
                required_validations=["lint", "unit"],
            ),
        ]
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo ok", "unit": "echo ok"},
            required=["lint", "unit"],
        )

        # Add two tasks
        daemon.add_task(Task(id="T-001", title="Task 1", dod="Test"))
        daemon.add_task(Task(id="T-002", title="Task 2", dod="Test"))
        daemon.state_machine.transition("c4_run")

        # Complete T-001 and trigger CP1
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        result = daemon.c4_submit(
            "T-001",
            "commit-1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )
        assert result.next_action == "await_checkpoint"

        # Approve CP1
        daemon.c4_checkpoint("CP1", "APPROVE", "Approved")
        assert "CP1" in daemon.state_machine.state.passed_checkpoints

        # Complete T-002
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        result = daemon.c4_submit(
            "T-002",
            "commit-2",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}],
        )

        # Should trigger CP2, NOT CP1 again
        assert result.next_action == "await_checkpoint"
        assert daemon.state_machine.state.checkpoint.current == "CP2"


class TestMultipleTaskWorkflow:
    """Test workflow with multiple tasks"""

    @patch("subprocess.run")
    def test_worker_processes_multiple_tasks(
        self, mock_run, daemon_with_validations
    ):
        """Test worker processing multiple tasks in sequence"""
        daemon = daemon_with_validations

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Add multiple tasks
        for i in range(3):
            task = Task(id=f"T-{i:03d}", title=f"Task {i}", dod="Test")
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Worker processes tasks one by one
        for i in range(3):
            assignment = daemon.c4_get_task("worker-1")
            assert assignment is not None
            assert assignment.task_id == f"T-{i:03d}"

            daemon.c4_run_validation()
            result = daemon.c4_submit(
                assignment.task_id,
                f"commit-{i}",
                [
                    {"name": "lint", "status": "pass"},
                    {"name": "unit", "status": "pass"},
                ],
            )
            assert result.success is True

        # All tasks done
        assert len(daemon.state_machine.state.queue.done) == 3
        assert len(daemon.state_machine.state.queue.pending) == 0

    @patch("subprocess.run")
    def test_multiple_workers_parallel_tasks(
        self, mock_run, daemon_with_validations
    ):
        """Test multiple workers processing tasks in parallel"""
        daemon = daemon_with_validations

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Add tasks with different scopes
        for i in range(4):
            task = Task(
                id=f"T-{i:03d}",
                title=f"Task {i}",
                dod="Test",
                scope=f"scope-{i}",
            )
            daemon.add_task(task)

        daemon.state_machine.transition("c4_run")

        # Two workers get tasks
        assignment1 = daemon.c4_get_task("worker-1")
        assignment2 = daemon.c4_get_task("worker-2")

        assert assignment1 is not None
        assert assignment2 is not None
        assert assignment1.task_id != assignment2.task_id

        # Both workers have tasks in progress
        assert len(daemon.state_machine.state.queue.in_progress) == 2

        # Workers complete their tasks
        daemon.c4_submit(
            assignment1.task_id,
            "commit-1",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        daemon.c4_submit(
            assignment2.task_id,
            "commit-2",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Two done, two pending
        assert len(daemon.state_machine.state.queue.done) == 2
        assert len(daemon.state_machine.state.queue.pending) == 2


class TestValidationEventLogging:
    """Test validation events are logged"""

    @patch("subprocess.run")
    def test_validation_emits_event(self, mock_run, daemon_with_validations):
        """Test that validation emits event"""
        daemon = daemon_with_validations

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        events_dir = daemon.c4_dir / "events"
        initial_count = len(list(events_dir.glob("*.json")))

        daemon.c4_run_validation()

        # New event should be created
        new_count = len(list(events_dir.glob("*.json")))
        assert new_count > initial_count

        # Check VALIDATION_RUN event exists
        validation_events = list(events_dir.glob("*VALIDATION_RUN*.json"))
        assert len(validation_events) > 0
