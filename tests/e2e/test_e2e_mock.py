"""Mock E2E Tests - Full workflow testing without actual Claude CLI

These tests verify the complete C4 workflow using mock supervisor decisions,
allowing full integration testing without requiring the Claude CLI.

Scenarios:
1. Happy Path: Worker completes task → APPROVE → COMPLETE
2. REQUEST_CHANGES: Worker completes → REQUEST_CHANGES → Worker fixes → APPROVE
3. REPLAN: Worker completes → REPLAN → Back to PLAN state
"""

import json
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.bundle import BundleCreator
from c4.mcp_server import C4Daemon
from c4.models import (
    CheckpointConfig,
    ProjectStatus,
    SupervisorDecision,
    Task,
    ValidationConfig,
)
from c4.supervisor import Supervisor


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def configured_daemon(temp_project):
    """Create daemon with full configuration for E2E testing"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("e2e-test-project")
    # Skip discovery phase to go directly to PLAN for testing
    daemon.state_machine.transition("skip_discovery")

    # Configure validations
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'tests passed'",
        },
        required=["lint", "unit"],
    )

    # Configure checkpoint
    daemon._config.checkpoints = [
        CheckpointConfig(
            id="CP1",
            name="Phase 1 Review",
            required_tasks=["T-001"],
            required_validations=["lint", "unit"],
        ),
    ]
    daemon._save_config()

    return daemon


@pytest.fixture
def bundle_creator(temp_project, configured_daemon):
    """Create bundle creator for E2E testing"""
    return BundleCreator(temp_project, configured_daemon.c4_dir)


@pytest.fixture
def supervisor(temp_project):
    """Create supervisor for E2E testing"""
    return Supervisor(temp_project)


class TestE2EHappyPath:
    """Scenario 1: Complete workflow with APPROVE decision"""

    @patch("subprocess.run")
    def test_full_workflow_approve(
        self, mock_run, configured_daemon, bundle_creator, supervisor
    ):
        """Test complete workflow: Task → Checkpoint → APPROVE → COMPLETE"""
        daemon = configured_daemon

        # Mock git and validation commands
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # === Phase 1: Setup and Execute ===
        # Add task
        task = Task(
            id="T-001",
            title="Implement feature",
            dod="Feature works and tests pass",
            validations=["lint", "unit"],
        )
        daemon.add_task(task)

        # Verify initial state is PLAN
        assert daemon.state_machine.state.status == ProjectStatus.PLAN

        # Transition to EXECUTE
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # === Phase 2: Worker executes task ===
        # Worker gets task
        assignment = daemon.c4_get_task("worker-1")
        assert assignment is not None
        assert assignment.task_id == "T-001"

        # Worker runs validation
        validation_result = daemon.c4_run_validation()
        assert validation_result["success"] is True

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
        assert submit_result.next_action == "await_checkpoint"

        # === Phase 3: Checkpoint is now automatically triggered by c4_submit ===
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT
        assert daemon.state_machine.state.checkpoint.current == "CP1"

        # === Phase 4: Create bundle and run mock supervisor ===
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            validation_results=[
                {"name": "lint", "status": "pass", "duration_ms": 100},
                {"name": "unit", "status": "pass", "duration_ms": 200},
            ],
        )
        assert bundle_dir.exists()
        assert (bundle_dir / "summary.json").exists()

        # Run mock supervisor with APPROVE
        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.APPROVE,
            mock_notes="All tests pass, code looks good",
        )
        assert response.decision == SupervisorDecision.APPROVE

        # === Phase 5: Apply supervisor decision ===
        checkpoint_response = daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="APPROVE",
            notes="All tests pass, code looks good",
        )
        assert checkpoint_response.success is True

        # === Verify final state ===
        assert daemon.state_machine.state.status == ProjectStatus.COMPLETE
        assert daemon.state_machine.state.metrics.checkpoints_passed == 1
        assert daemon.state_machine.state.metrics.tasks_completed == 1


class TestE2ERequestChanges:
    """Scenario 2: REQUEST_CHANGES workflow"""

    @patch("subprocess.run")
    def test_full_workflow_request_changes(
        self, mock_run, configured_daemon, bundle_creator, supervisor
    ):
        """Test REQUEST_CHANGES: Task → Checkpoint → REQUEST_CHANGES → Fix → APPROVE"""
        daemon = configured_daemon

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # === Setup: Add task and execute ===
        task = Task(
            id="T-001",
            title="Implement feature",
            dod="Feature works",
            validations=["lint", "unit"],
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker completes task
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Checkpoint is now automatically triggered by c4_submit
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT
        assert daemon.state_machine.state.checkpoint.current == "CP1"

        # === Create bundle and get REQUEST_CHANGES decision ===
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.REQUEST_CHANGES,
            mock_notes="Found some issues",
            mock_changes=["Fix lint error in module.py", "Add edge case tests"],
        )
        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(response.required_changes) == 2

        # Apply REQUEST_CHANGES decision
        checkpoint_response = daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="REQUEST_CHANGES",
            notes="Found some issues",
            required_changes=["Fix lint error in module.py", "Add edge case tests"],
        )
        assert checkpoint_response.success is True

        # === Verify state returned to EXECUTE ===
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Verify new tasks were created
        pending = daemon.state_machine.state.queue.pending
        assert "RC-CP1-01" in pending
        assert "RC-CP1-02" in pending

        # Verify tasks exist
        rc_task_1 = daemon.get_task("RC-CP1-01")
        assert rc_task_1 is not None
        assert rc_task_1.title == "Fix lint error in module.py"

        rc_task_2 = daemon.get_task("RC-CP1-02")
        assert rc_task_2 is not None
        assert rc_task_2.title == "Add edge case tests"

        # === Worker fixes issues ===
        # Fix first issue
        assignment = daemon.c4_get_task("worker-1")
        assert assignment.task_id == "RC-CP1-01"
        daemon.c4_submit(
            "RC-CP1-01",
            "def456",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Fix second issue
        assignment = daemon.c4_get_task("worker-1")
        assert assignment.task_id == "RC-CP1-02"
        daemon.c4_submit(
            "RC-CP1-02",
            "ghi789",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Verify all tasks done
        assert len(daemon.state_machine.state.queue.done) == 3
        assert len(daemon.state_machine.state.queue.pending) == 0


class TestE2EReplan:
    """Scenario 3: REPLAN workflow"""

    @patch("subprocess.run")
    def test_full_workflow_replan(
        self, mock_run, configured_daemon, bundle_creator, supervisor
    ):
        """Test REPLAN: Task → Checkpoint → REPLAN → Back to PLAN"""
        daemon = configured_daemon

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # === Setup and execute task ===
        task = Task(
            id="T-001",
            title="Implement feature",
            dod="Feature works",
            validations=["lint", "unit"],
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker completes task
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc123",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Checkpoint is now automatically triggered by c4_submit
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT
        assert daemon.state_machine.state.checkpoint.current == "CP1"

        # === Get REPLAN decision ===
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.REPLAN,
            mock_notes="Architecture needs reconsideration",
        )
        assert response.decision == SupervisorDecision.REPLAN

        # Apply REPLAN decision
        checkpoint_response = daemon.c4_checkpoint(
            checkpoint_id="CP1",
            decision="REPLAN",
            notes="Architecture needs reconsideration",
        )
        assert checkpoint_response.success is True

        # === Verify state returned to PLAN ===
        assert daemon.state_machine.state.status == ProjectStatus.PLAN

        # Verify checkpoint state is cleared
        assert daemon.state_machine.state.checkpoint.current is None


class TestBundleCreator:
    """Test BundleCreator in isolation"""

    def test_create_bundle_structure(self, temp_project, configured_daemon):
        """Test bundle directory structure"""
        bundle_creator = BundleCreator(temp_project, configured_daemon.c4_dir)

        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001", "T-002"],
            validation_results=[
                {"name": "lint", "status": "pass", "duration_ms": 100},
                {"name": "unit", "status": "pass", "duration_ms": 500},
            ],
        )

        # Verify structure
        assert bundle_dir.exists()
        assert (bundle_dir / "summary.json").exists()
        assert (bundle_dir / "changes.diff").exists()
        assert (bundle_dir / "test_results.json").exists()

        # Verify summary content
        summary = json.loads((bundle_dir / "summary.json").read_text())
        assert summary["checkpoint_id"] == "CP1"
        assert summary["tasks_completed"] == ["T-001", "T-002"]

        # Verify test results
        results = json.loads((bundle_dir / "test_results.json").read_text())
        assert len(results) == 2
        assert results[0]["name"] == "lint"

    def test_get_latest_bundle(self, temp_project, configured_daemon):
        """Test retrieving latest bundle"""
        bundle_creator = BundleCreator(temp_project, configured_daemon.c4_dir)

        # Create multiple bundles
        _bundle1 = bundle_creator.create_bundle("CP1", ["T-001"], [])
        _bundle2 = bundle_creator.create_bundle("CP1", ["T-001", "T-002"], [])
        _bundle3 = bundle_creator.create_bundle("CP2", ["T-003"], [])

        # Get latest overall
        latest = bundle_creator.get_latest_bundle()
        assert latest is not None
        assert "cp-CP2" in latest.name

        # Get latest for specific checkpoint
        latest_cp1 = bundle_creator.get_latest_bundle("CP1")
        assert latest_cp1 is not None
        assert "cp-CP1" in latest_cp1.name

    def test_load_bundle_summary(self, temp_project, configured_daemon):
        """Test loading bundle summary"""
        bundle_creator = BundleCreator(temp_project, configured_daemon.c4_dir)

        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            validation_results=[],
        )

        summary = bundle_creator.load_bundle_summary(bundle_dir)
        assert summary is not None
        assert summary.checkpoint_id == "CP1"
        assert summary.tasks_completed == ["T-001"]


class TestSupervisor:
    """Test Supervisor in isolation"""

    def test_render_prompt(self, temp_project):
        """Test prompt rendering"""
        supervisor = Supervisor(temp_project)

        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=["T-001", "T-002"],
            test_results=[
                {"name": "lint", "status": "pass", "duration_ms": 100},
                {"name": "unit", "status": "pass", "duration_ms": 500},
            ],
            files_changed=5,
            lines_added=120,
            lines_removed=30,
            diff_content="+ new line\n- old line",
        )

        # Verify content
        assert "CP1" in prompt
        assert "T-001" in prompt
        assert "T-002" in prompt
        assert "lint" in prompt
        assert "unit" in prompt
        assert "120" in prompt  # lines added
        assert "APPROVE" in prompt
        assert "REQUEST_CHANGES" in prompt
        assert "REPLAN" in prompt

    def test_parse_decision_json_block(self, temp_project):
        """Test parsing JSON from code block"""
        supervisor = Supervisor(temp_project)

        output = '''
        I've reviewed the checkpoint and here is my decision:

        ```json
        {
          "decision": "APPROVE",
          "checkpoint": "CP1",
          "notes": "All tests pass",
          "required_changes": []
        }
        ```
        '''

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.APPROVE
        assert response.checkpoint_id == "CP1"
        assert response.notes == "All tests pass"

    def test_parse_decision_raw_json(self, temp_project):
        """Test parsing raw JSON without code block"""
        supervisor = Supervisor(temp_project)

        output = (
            'Here is my decision: {"decision": "REQUEST_CHANGES", '
            '"checkpoint": "CP1", "notes": "Found issues", '
            '"required_changes": ["Fix lint error"]}'
        )

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(response.required_changes) == 1

    def test_parse_decision_full_json(self, temp_project):
        """Test parsing when entire output is JSON"""
        supervisor = Supervisor(temp_project)

        output = (
            '{"decision": "REPLAN", "checkpoint": "CP1", '
            '"notes": "Need to reconsider", "required_changes": []}'
        )

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.REPLAN

    def test_parse_decision_invalid(self, temp_project):
        """Test parsing invalid output"""
        supervisor = Supervisor(temp_project)

        output = "I think everything looks good but I'm not sure."

        with pytest.raises(ValueError) as exc_info:
            supervisor.parse_decision(output)
        assert "No valid JSON found" in str(exc_info.value)

    def test_mock_supervisor_approve(self, temp_project, configured_daemon):
        """Test mock supervisor with APPROVE"""
        bundle_creator = BundleCreator(temp_project, configured_daemon.c4_dir)
        supervisor = Supervisor(temp_project)

        bundle_dir = bundle_creator.create_bundle("CP1", ["T-001"], [])

        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.APPROVE,
            mock_notes="Looks good!",
        )

        assert response.decision == SupervisorDecision.APPROVE
        assert response.notes == "Looks good!"
        assert (bundle_dir / "prompt.md").exists()
        assert (bundle_dir / "response.json").exists()

    def test_mock_supervisor_request_changes(self, temp_project, configured_daemon):
        """Test mock supervisor with REQUEST_CHANGES"""
        bundle_creator = BundleCreator(temp_project, configured_daemon.c4_dir)
        supervisor = Supervisor(temp_project)

        bundle_dir = bundle_creator.create_bundle("CP1", ["T-001"], [])

        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.REQUEST_CHANGES,
            mock_notes="Found issues",
            mock_changes=["Fix issue 1", "Fix issue 2"],
        )

        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(response.required_changes) == 2

        # Verify response was saved
        saved = json.loads((bundle_dir / "response.json").read_text())
        assert saved["decision"] == "REQUEST_CHANGES"
        assert len(saved["required_changes"]) == 2


class TestPromptTemplate:
    """Test Supervisor prompt template rendering"""

    def test_template_from_file(self, temp_project):
        """Test rendering from template file"""
        # Create prompts directory with template
        prompts_dir = temp_project / "prompts"
        prompts_dir.mkdir()

        supervisor = Supervisor(temp_project, prompts_dir=prompts_dir)

        # Use default template (prompts dir exists but no file)
        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            test_results=[],
            files_changed=1,
            lines_added=10,
            lines_removed=5,
        )

        assert "CP1" in prompt
        assert "T-001" in prompt

    def test_template_diff_truncation(self, temp_project):
        """Test that long diffs are truncated"""
        supervisor = Supervisor(temp_project)

        long_diff = "x" * 5000  # 5000 characters
        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=[],
            test_results=[],
            diff_content=long_diff,
        )

        # Should be truncated to ~2000 chars
        assert "more characters" in prompt


class TestEventLogging:
    """Test event logging during E2E workflow"""

    @patch("subprocess.run")
    def test_events_logged_during_workflow(self, mock_run, configured_daemon):
        """Verify events are logged throughout workflow"""
        daemon = configured_daemon
        events_dir = daemon.c4_dir / "events"

        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Add and execute task
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Worker flow
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Check events were logged
        event_files = list(events_dir.glob("*.json"))
        assert len(event_files) >= 3  # At minimum: state changes, task assigned, submitted

        # Verify event types exist
        event_types = set()
        for f in event_files:
            data = json.loads(f.read_text())
            event_types.add(data["type"])

        assert "STATE_CHANGED" in event_types
        assert "TASK_ASSIGNED" in event_types
        assert "WORKER_SUBMITTED" in event_types


class TestDaemonSupervisorIntegration:
    """Test Daemon-Supervisor integration methods"""

    @patch("subprocess.run")
    def test_create_checkpoint_bundle(self, mock_run, configured_daemon):
        """Test bundle creation from daemon"""
        daemon = configured_daemon
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup: complete a task
        task = Task(id="T-001", title="Test", dod="Test", validations=["lint", "unit"])
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Trigger checkpoint
        daemon.check_and_trigger_checkpoint()
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Create bundle
        bundle_dir = daemon.create_checkpoint_bundle()

        # Verify bundle contents
        assert bundle_dir.exists()
        assert (bundle_dir / "summary.json").exists()
        assert (bundle_dir / "test_results.json").exists()

        summary = json.loads((bundle_dir / "summary.json").read_text())
        assert summary["checkpoint_id"] == "CP1"
        assert "T-001" in summary["tasks_completed"]

    @patch("subprocess.run")
    def test_run_supervisor_review_mock_approve(self, mock_run, configured_daemon):
        """Test running mock supervisor review with APPROVE"""
        daemon = configured_daemon
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup and reach checkpoint
        task = Task(id="T-001", title="Test", dod="Test", validations=["lint", "unit"])
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        daemon.check_and_trigger_checkpoint()

        # Run mock supervisor
        result = daemon.run_supervisor_review(use_mock=True, mock_decision="APPROVE")

        assert result["success"] is True
        assert result["decision"] == "APPROVE"
        assert result["new_status"] == "COMPLETE"

    @patch("subprocess.run")
    def test_run_supervisor_review_mock_request_changes(self, mock_run, configured_daemon):
        """Test running mock supervisor review with REQUEST_CHANGES"""
        daemon = configured_daemon
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup and reach checkpoint
        task = Task(id="T-001", title="Test", dod="Test", validations=["lint", "unit"])
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        daemon.check_and_trigger_checkpoint()

        # Run mock supervisor with REQUEST_CHANGES
        result = daemon.run_supervisor_review(use_mock=True, mock_decision="REQUEST_CHANGES")

        assert result["success"] is True
        assert result["decision"] == "REQUEST_CHANGES"
        assert result["new_status"] == "EXECUTE"

        # Verify new tasks were created
        assert len(daemon.state_machine.state.queue.pending) > 0

    @patch("subprocess.run")
    def test_run_supervisor_review_not_in_checkpoint(self, mock_run, configured_daemon):
        """Test supervisor review fails when not in CHECKPOINT state"""
        daemon = configured_daemon
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Don't reach checkpoint state
        task = Task(id="T-001", title="Test", dod="Test")
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        # Try to run supervisor review
        result = daemon.run_supervisor_review(use_mock=True)

        assert result["success"] is False
        assert "Not in CHECKPOINT state" in result["error"]

    @patch("subprocess.run")
    def test_full_automated_workflow(self, mock_run, configured_daemon):
        """Test full automated workflow with supervisor integration"""
        daemon = configured_daemon
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup
        task = Task(id="T-001", title="Test", dod="Test", validations=["lint", "unit"])
        daemon.add_task(task)

        # Phase 1: PLAN -> EXECUTE
        assert daemon.state_machine.state.status == ProjectStatus.PLAN
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # Phase 2: Worker completes task
        daemon.c4_get_task("worker-1")
        daemon.c4_run_validation()
        daemon.c4_submit(
            "T-001",
            "abc",
            [
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Phase 3: Enter checkpoint
        daemon.check_and_trigger_checkpoint()
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT

        # Phase 4: Supervisor approves
        result = daemon.run_supervisor_review(use_mock=True, mock_decision="APPROVE")
        assert result["success"] is True

        # Phase 5: Project complete
        assert daemon.state_machine.state.status == ProjectStatus.COMPLETE
        assert daemon.state_machine.state.metrics.checkpoints_passed == 1
