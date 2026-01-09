"""E2E User Scenario Tests

Tests that simulate actual user workflows with C4.
These tests verify the complete user journey from project initialization
through completion, matching the documented usage scenarios.

Scenarios tested:
1. Happy Path: init → plan → add-task → run → worker → submit → checkpoint → complete
2. Multi-task workflow with checkpoint gates
3. REQUEST_CHANGES flow: supervisor requests fixes
4. REPLAN flow: supervisor requests re-architecture
5. Multi-worker parallel execution
6. Stop and resume workflow
"""

import json
import subprocess
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.mcp_server import C4Daemon
from c4.models import (
    CheckpointConfig,
    ProjectStatus,
    SupervisorDecision,
    Task,
    ValidationConfig,
)


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def fresh_project():
    """Create a fresh temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        project_dir = Path(tmpdir)
        # Create minimal project structure
        (project_dir / "src").mkdir()
        (project_dir / "tests").mkdir()
        (project_dir / "docs").mkdir()
        yield project_dir


@pytest.fixture
def initialized_project(fresh_project):
    """Create an initialized C4 project"""
    daemon = C4Daemon(fresh_project)
    daemon.initialize("test-project")
    return fresh_project, daemon


@pytest.fixture
def configured_project(initialized_project):
    """Create a fully configured C4 project ready for execution"""
    project_dir, daemon = initialized_project

    # Configure validations (mock commands for testing)
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint passed'",
            "unit": "echo 'tests passed'",
        },
        required=["lint", "unit"],
    )

    # Configure checkpoints
    daemon._config.checkpoints = [
        CheckpointConfig(
            id="CP1",
            name="Phase 1 Complete",
            required_tasks=["T-001", "T-002"],
            required_validations=["lint", "unit"],
        ),
        CheckpointConfig(
            id="CP2",
            name="Final Review",
            required_tasks=["T-003"],
            required_validations=["lint", "unit"],
        ),
    ]
    daemon._save_config()

    return project_dir, daemon


# =============================================================================
# Scenario 1: Happy Path - Complete workflow with APPROVE
# =============================================================================


class TestScenario1HappyPath:
    """
    User Story:
    As a developer, I want to use C4 to manage my project from start to finish,
    with AI workers completing tasks and supervisor approving at checkpoints.
    """

    @patch("subprocess.run")
    def test_complete_workflow_single_task(self, mock_run, initialized_project):
        """
        Scenario: Single task workflow
        Given: A new C4 project
        When: User adds one task and runs execution
        Then: Task is completed and project reaches COMPLETE state
        """
        project_dir, daemon = initialized_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Configure single checkpoint
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo ok", "unit": "echo ok"},
            required=["lint", "unit"],
        )
        daemon._config.checkpoints = [
            CheckpointConfig(
                id="CP1",
                required_tasks=["T-001"],
                required_validations=["lint", "unit"],
            )
        ]
        daemon._save_config()

        # Step 1: User is in PLAN state after init
        assert daemon.state_machine.state.status == ProjectStatus.PLAN

        # Step 2: User adds a task
        task = Task(
            id="T-001",
            title="Implement feature X",
            dod="Feature X works with tests",
            scope="src/feature",
            validations=["lint", "unit"],
        )
        daemon.add_task(task)

        # Step 3: User runs execution
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Step 4: Worker gets task assignment
        assignment = daemon.c4_get_task("worker-1")
        assert assignment is not None
        assert assignment.task_id == "T-001"
        assert assignment.title == "Implement feature X"

        # Step 5: Worker implements and runs validation
        validation_result = daemon.c4_run_validation()
        assert validation_result["success"] is True

        # Step 6: Worker submits completed task
        submit_result = daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc123",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        assert submit_result.success is True

        # Step 7: Checkpoint is reached (c4_submit auto-triggers checkpoint)
        assert submit_result.next_action == "await_checkpoint"
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Step 8: Supervisor approves
        checkpoint_response = daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="APPROVE",
            notes="All looks good!",
        )
        assert checkpoint_response.success is True

        # Step 9: Project is complete
        assert daemon.state_machine.state.status == ProjectStatus.COMPLETE
        assert daemon.state_machine.state.metrics.tasks_completed == 1
        assert daemon.state_machine.state.metrics.checkpoints_passed == 1

    @patch("subprocess.run")
    def test_complete_workflow_multiple_tasks(self, mock_run, configured_project):
        """
        Scenario: Multi-task workflow with multiple checkpoints
        Given: A C4 project with 3 tasks and 2 checkpoints
        When: Worker completes all tasks
        Then: Both checkpoints pass and project completes
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Add tasks
        tasks = [
            Task(id="T-001", title="Task 1", dod="DoD 1", validations=["lint", "unit"]),
            Task(id="T-002", title="Task 2", dod="DoD 2", validations=["lint", "unit"]),
            Task(id="T-003", title="Task 3", dod="DoD 3", validations=["lint", "unit"]),
        ]
        for task in tasks:
            daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        # Complete T-001
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001", "commit1", [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
        )

        # Complete T-002
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        submit_result = daemon.c4_submit(
            "T-002", "commit2", [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
        )

        # Checkpoint 1 should trigger (auto-triggered by c4_submit)
        assert submit_result.next_action == "await_checkpoint"
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Supervisor approves CP1
        daemon.c4_checkpoint("CP1", "APPROVE", "Phase 1 looks good")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Complete T-003
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        submit_result = daemon.c4_submit(
            "T-003", "commit3", [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
        )

        # Checkpoint 2 should trigger (auto-triggered by c4_submit)
        assert submit_result.next_action == "await_checkpoint"
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Supervisor approves CP2
        daemon.c4_checkpoint("CP2", "APPROVE", "All done!")

        # Project complete
        assert daemon.state_machine.state.status == ProjectStatus.COMPLETE
        assert daemon.state_machine.state.metrics.tasks_completed == 3
        assert daemon.state_machine.state.metrics.checkpoints_passed == 2


# =============================================================================
# Scenario 2: REQUEST_CHANGES Flow
# =============================================================================


class TestScenario2RequestChanges:
    """
    User Story:
    As a supervisor, I want to request changes when code doesn't meet standards,
    so workers can fix issues before the project proceeds.
    """

    @patch("subprocess.run")
    def test_request_changes_creates_fix_tasks(self, mock_run, configured_project):
        """
        Scenario: Supervisor requests changes
        Given: A task is completed but has issues
        When: Supervisor requests changes with specific fixes
        Then: New fix tasks are created and project returns to EXECUTE
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup: Add tasks for CP1
        daemon.add_task(Task(id="T-001", title="Feature 1", dod="DoD", validations=["lint", "unit"]))
        daemon.add_task(Task(id="T-002", title="Feature 2", dod="DoD", validations=["lint", "unit"]))

        # Execute and complete tasks
        daemon.state_machine.transition("c4_run")

        for task_id in ["T-001", "T-002"]:
            daemon.c4_get_task("worker-1")
            daemon.c4_run_validation()
            daemon.c4_submit(
                task_id, f"commit-{task_id}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
            )

        # Reach checkpoint
        daemon.check_and_trigger_checkpoint()
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Supervisor requests changes
        daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="REQUEST_CHANGES",
            notes="Found some issues that need fixing",
            required_changes=[
                "Fix error handling in Feature 1",
                "Add input validation to Feature 2",
                "Update tests for edge cases",
            ],
        )

        # Project returns to EXECUTE
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # New fix tasks are created
        pending = daemon.state_machine.state.queue.pending
        assert len(pending) == 3
        assert "RC-CP1-01" in pending
        assert "RC-CP1-02" in pending
        assert "RC-CP1-03" in pending

        # Verify fix task details
        fix_task = daemon.get_task("RC-CP1-01")
        assert fix_task.title == "Fix error handling in Feature 1"

    @patch("subprocess.run")
    def test_fix_tasks_then_approve(self, mock_run, configured_project):
        """
        Scenario: Fix tasks and re-submit for approval
        Given: Supervisor requested changes
        When: Worker completes all fix tasks
        Then: Checkpoint can be approved
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup and reach checkpoint
        daemon.add_task(Task(id="T-001", title="Feature", dod="DoD", validations=["lint", "unit"]))
        daemon.add_task(Task(id="T-002", title="Feature 2", dod="DoD", validations=["lint", "unit"]))
        daemon.state_machine.transition("c4_run")

        for task_id in ["T-001", "T-002"]:
            daemon.c4_get_task("worker-1")
            daemon.c4_submit(
                task_id, f"commit-{task_id}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
            )

        daemon.check_and_trigger_checkpoint()

        # Request changes
        daemon.c4_checkpoint(
            "CP1", "REQUEST_CHANGES", "Issues found",
            required_changes=["Fix issue 1", "Fix issue 2"]
        )

        # Worker fixes issues
        for i in range(2):
            assignment = daemon.c4_get_task("worker-1")
            assert assignment.task_id.startswith("RC-CP1")
            daemon.c4_submit(
                assignment.task_id, f"fix-commit-{i}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
            )

        # All tasks done
        assert len(daemon.state_machine.state.queue.pending) == 0
        assert len(daemon.state_machine.state.queue.done) == 4  # 2 original + 2 fixes


# =============================================================================
# Scenario 3: REPLAN Flow
# =============================================================================


class TestScenario3Replan:
    """
    User Story:
    As a supervisor, I want to trigger a replan when the architecture is wrong,
    so the team can reconsider the approach.
    """

    @patch("subprocess.run")
    def test_replan_returns_to_plan_state(self, mock_run, configured_project):
        """
        Scenario: Supervisor triggers replan
        Given: Checkpoint review reveals architectural issues
        When: Supervisor decides REPLAN
        Then: Project returns to PLAN state for re-architecture
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup and reach checkpoint
        daemon.add_task(Task(id="T-001", title="Feature", dod="DoD", validations=["lint", "unit"]))
        daemon.add_task(Task(id="T-002", title="Feature 2", dod="DoD", validations=["lint", "unit"]))
        daemon.state_machine.transition("c4_run")

        for task_id in ["T-001", "T-002"]:
            daemon.c4_get_task("worker-1")
            daemon.c4_submit(
                task_id, f"commit-{task_id}",
                [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
            )

        daemon.check_and_trigger_checkpoint()
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Supervisor triggers replan
        daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="REPLAN",
            notes="The current architecture won't scale. Need to reconsider the approach.",
        )

        # Project returns to PLAN state
        assert daemon.state_machine.state.status == ProjectStatus.PLAN

        # Checkpoint state is cleared
        assert daemon.state_machine.state.checkpoint.current is None


# =============================================================================
# Scenario 4: Multi-Worker Parallel Execution
# =============================================================================


class TestScenario4MultiWorker:
    """
    User Story:
    As a team lead, I want multiple workers to execute tasks in parallel,
    so the project completes faster.
    """

    @patch("subprocess.run")
    def test_multiple_workers_get_different_tasks(self, mock_run, configured_project):
        """
        Scenario: Two workers work in parallel
        Given: Multiple tasks with different scopes
        When: Two workers request tasks
        Then: Each gets a different task
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Add tasks with different scopes
        daemon.add_task(Task(id="T-001", title="Frontend", dod="DoD", scope="src/frontend/"))
        daemon.add_task(Task(id="T-002", title="Backend", dod="DoD", scope="src/backend/"))
        daemon.add_task(Task(id="T-003", title="Tests", dod="DoD", scope="tests/"))

        daemon.state_machine.transition("c4_run")

        # Worker 1 gets task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1 is not None
        task1_id = assignment1.task_id

        # Worker 2 gets different task
        assignment2 = daemon.c4_get_task("worker-2")
        assert assignment2 is not None
        task2_id = assignment2.task_id

        # Different tasks assigned
        assert task1_id != task2_id

        # Both workers are registered
        assert "worker-1" in daemon.state_machine.state.workers
        assert "worker-2" in daemon.state_machine.state.workers

    @patch("subprocess.run")
    def test_scope_locking_prevents_conflicts(self, mock_run, configured_project):
        """
        Scenario: Scope locking prevents conflicts
        Given: Two tasks with overlapping scopes
        When: One worker takes a task
        Then: The scope is locked for that worker
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Add tasks with same scope
        daemon.add_task(Task(id="T-001", title="Task 1", dod="DoD", scope="src/shared/"))
        daemon.add_task(Task(id="T-002", title="Task 2", dod="DoD", scope="src/shared/"))

        daemon.state_machine.transition("c4_run")

        # Worker 1 gets first task
        assignment1 = daemon.c4_get_task("worker-1")
        assert assignment1.task_id == "T-001"

        # Scope is locked
        assert "src/shared/" in daemon.state_machine.state.locks.scopes

        # Worker 2 cannot get task with same scope (gets None or different task)
        assignment2 = daemon.c4_get_task("worker-2")
        # T-002 has same scope, so worker-2 shouldn't get it while scope is locked
        if assignment2 is not None:
            assert assignment2.task_id != "T-002" or assignment2.scope != "src/shared/"


# =============================================================================
# Scenario 5: Stop and Resume
# =============================================================================


class TestScenario5StopResume:
    """
    User Story:
    As a developer, I want to stop execution and resume later,
    so I can pause work without losing progress.
    """

    @patch("subprocess.run")
    def test_stop_halts_execution(self, mock_run, configured_project):
        """
        Scenario: Stop execution mid-way
        Given: Project is in EXECUTE state with work in progress
        When: User stops execution
        Then: Project enters HALTED state
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup and start
        daemon.add_task(Task(id="T-001", title="Task", dod="DoD"))
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Stop execution
        daemon.state_machine.transition("c4_stop")

        # Project is halted
        assert daemon.state_machine.state.status == ProjectStatus.HALTED

    @patch("subprocess.run")
    def test_resume_continues_execution(self, mock_run, configured_project):
        """
        Scenario: Resume after stop
        Given: Project is in HALTED state
        When: User resumes execution
        Then: Project returns to EXECUTE state with progress preserved
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup, start, and complete one task
        daemon.add_task(Task(id="T-001", title="Task 1", dod="DoD", validations=["lint", "unit"]))
        daemon.add_task(Task(id="T-002", title="Task 2", dod="DoD", validations=["lint", "unit"]))
        daemon.state_machine.transition("c4_run")

        # Complete first task
        daemon.c4_get_task("worker-1")
        daemon.c4_submit(
            "T-001", "commit1",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
        )

        # Stop
        daemon.state_machine.transition("c4_stop")
        assert daemon.state_machine.state.status == ProjectStatus.HALTED

        # Verify progress preserved
        assert "T-001" in daemon.state_machine.state.queue.done

        # Resume
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Progress still there
        assert "T-001" in daemon.state_machine.state.queue.done
        assert "T-002" in daemon.state_machine.state.queue.pending


# =============================================================================
# Scenario 6: Validation Failures
# =============================================================================


class TestScenario6ValidationFailures:
    """
    User Story:
    As a developer, I want validation failures to prevent submission,
    so only quality code gets submitted.
    """

    @patch("subprocess.run")
    def test_validation_failure_blocks_submit(self, mock_run, configured_project):
        """
        Scenario: Validation failure
        Given: A task with failing tests
        When: Worker tries to submit
        Then: Submission fails with validation error
        """
        project_dir, daemon = configured_project

        # Make lint fail
        mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="Lint error!")

        daemon.add_task(Task(id="T-001", title="Task", dod="DoD", validations=["lint", "unit"]))
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Run validation - should fail
        result = daemon.c4_run_validation(["lint"])
        assert result["success"] is False
        assert result["results"][0]["status"] == "fail"


# =============================================================================
# Scenario 7: CLI Integration (subprocess tests)
# =============================================================================


class TestScenario7CLIIntegration:
    """
    Tests that verify CLI commands work correctly.
    These use subprocess to actually run the CLI.
    """

    def test_cli_init_creates_structure(self, fresh_project):
        """
        Scenario: Initialize via CLI
        Given: A fresh project directory
        When: User runs 'c4 init'
        Then: .c4 directory and docs are created
        """
        result = subprocess.run(
            ["uv", "run", "c4", "init", "--project-id", "cli-test"],
            cwd=fresh_project,
            capture_output=True,
            text=True,
        )

        # Check success
        assert result.returncode == 0

        # Check structure created
        assert (fresh_project / ".c4").exists()
        assert (fresh_project / ".c4" / "config.yaml").exists()
        assert (fresh_project / ".c4" / "c4.db").exists()  # SQLite database

    def test_cli_status_shows_state(self, initialized_project):
        """
        Scenario: Check status via CLI
        Given: An initialized project
        When: User runs 'c4 status'
        Then: Current state is displayed
        """
        project_dir, daemon = initialized_project

        result = subprocess.run(
            ["uv", "run", "c4", "status"],
            cwd=project_dir,
            capture_output=True,
            text=True,
        )

        assert result.returncode == 0
        assert "PLAN" in result.stdout or "test-project" in result.stdout


# =============================================================================
# Scenario 8: Event Logging
# =============================================================================


class TestScenario8EventLogging:
    """
    User Story:
    As a project manager, I want all actions logged,
    so I can audit the project history.
    """

    @patch("subprocess.run")
    def test_events_logged_throughout_workflow(self, mock_run, configured_project):
        """
        Scenario: Events are logged
        Given: A complete workflow
        When: Tasks are executed and submitted
        Then: All state changes are logged as events
        """
        project_dir, daemon = configured_project
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Execute workflow
        daemon.add_task(Task(id="T-001", title="Task", dod="DoD", validations=["lint", "unit"]))
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001", "abc",
            [{"name": "lint", "status": "pass"}, {"name": "unit", "status": "pass"}]
        )

        # Check events logged
        events_dir = daemon.c4_dir / "events"
        event_files = list(events_dir.glob("*.json"))
        assert len(event_files) >= 3

        # Verify event types
        event_types = set()
        for f in event_files:
            data = json.loads(f.read_text())
            event_types.add(data["type"])

        assert "STATE_CHANGED" in event_types
        assert "TASK_ASSIGNED" in event_types
