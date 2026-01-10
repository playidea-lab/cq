"""E2E tests for C4 full automation scenario"""

import pytest
import asyncio
from pathlib import Path
from unittest.mock import patch, MagicMock

from c4.mcp_server import C4Daemon
from c4.daemon.supervisor_loop import SupervisorLoop
from c4.models import (
    CheckpointConfig,
    CheckpointQueueItem,
    ProjectStatus,
    SupervisorDecision,
    ValidationResult,
)
from c4.supervisor import SupervisorResponse


@pytest.fixture
def project_dir(tmp_path):
    """Create a temporary project directory"""
    return tmp_path


@pytest.fixture
def daemon(project_dir):
    """Create a daemon with initialized project"""
    d = C4Daemon(project_dir)
    d.initialize("auto-test")
    # Skip discovery phase to go directly to PLAN for testing
    d.state_machine.transition("skip_discovery")
    return d


class TestFullAutomationScenario:
    """E2E tests for full automation workflow"""

    def test_automation_workflow_happy_path(self, daemon):
        """
        Test complete automation workflow:
        1. Add tasks
        2. Start execution
        3. Get task → implement → validate → submit
        4. Checkpoint queue populated
        5. Supervisor processes checkpoint
        6. Continue to completion
        """
        # 1. Add tasks
        daemon.c4_add_todo(
            task_id="T-001",
            title="Implement feature A",
            scope="src/",
            dod="Create feature A with tests",
        )
        daemon.c4_add_todo(
            task_id="T-002",
            title="Implement feature B",
            scope="src/",
            dod="Create feature B with tests",
        )

        # 2. Start execution
        daemon.state_machine.transition("c4_run")
        assert daemon.state_machine.state.status == ProjectStatus.EXECUTE

        # 3. Worker gets task
        task = daemon.c4_get_task("worker-1")
        assert task is not None
        assert task.task_id == "T-001"

        # 4. Worker submits (no checkpoint triggered without config)
        result = daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc123",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        assert result.success is True
        assert result.next_action == "get_next_task"

        # 5. Get next task
        task2 = daemon.c4_get_task("worker-1")
        assert task2 is not None
        assert task2.task_id == "T-002"

        # 6. Submit final task
        result2 = daemon.c4_submit(
            task_id="T-002",
            commit_sha="def456",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )
        assert result2.success is True
        # Should be complete since no more tasks
        assert result2.next_action == "complete"

    def test_checkpoint_queue_flow(self, daemon):
        """Test checkpoint is added to queue on trigger"""
        # Setup with checkpoint config
        daemon._config.checkpoints = [
            CheckpointConfig(
                id="CP1",
                name="Phase 1 Review",
                required_tasks=["T-001"],
            )
        ]

        # Add task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Complete phase 1",
            scope=None,
            dod="Phase 1 work",
        )

        # Start and complete task
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        result = daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc123",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
        )

        # Checkpoint should be triggered
        assert result.next_action == "await_checkpoint"

        # Checkpoint should be in queue
        state = daemon.state_machine.state
        assert len(state.checkpoint_queue) == 1
        assert state.checkpoint_queue[0].checkpoint_id == "CP1"
        assert state.status == ProjectStatus.CHECKPOINT

    def test_mark_blocked_flow(self, daemon):
        """Test marking a task as blocked adds to repair queue"""
        # Add task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Difficult task",
            scope=None,
            dod="Complete difficult work",
        )

        # Start and get task
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Mark as blocked after failures
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="lint: syntax error",
            attempts=10,
            last_error="SyntaxError: unexpected token",
        )

        assert result["success"] is True

        # Should be in repair queue
        state = daemon.state_machine.state
        assert len(state.repair_queue) == 1
        assert state.repair_queue[0].task_id == "T-001"
        assert state.repair_queue[0].attempts == 10

    def test_status_shows_queues(self, daemon):
        """Test status includes queue information"""
        # Add to checkpoint queue manually
        daemon.state_machine.state.checkpoint_queue.append(
            CheckpointQueueItem(
                checkpoint_id="CP1",
                triggered_at="2024-01-01T00:00:00",
            )
        )
        daemon.state_machine.save_state()

        status = daemon.c4_status()

        assert "checkpoint_queue" in status
        assert "repair_queue" in status
        assert "supervisor_loop_running" in status
        assert len(status["checkpoint_queue"]) == 1


class TestMultiWorkerAutomation:
    """E2E tests for multi-worker automation"""

    def test_multiple_workers_parallel(self, daemon):
        """Test multiple workers working in parallel"""
        # Add multiple tasks
        for i in range(3):
            daemon.c4_add_todo(
                task_id=f"T-00{i+1}",
                title=f"Task {i+1}",
                scope=f"src/module{i+1}/",
                dod=f"Complete task {i+1}",
            )

        # Start execution
        daemon.state_machine.transition("c4_run")

        # Multiple workers get tasks
        task1 = daemon.c4_get_task("worker-1")
        task2 = daemon.c4_get_task("worker-2")
        task3 = daemon.c4_get_task("worker-3")

        assert task1 is not None
        assert task2 is not None
        assert task3 is not None

        # All should have different tasks
        task_ids = {task1.task_id, task2.task_id, task3.task_id}
        assert len(task_ids) == 3

    def test_workers_submit_independent(self, daemon):
        """Test workers can submit independently"""
        # Add tasks
        daemon.c4_add_todo(task_id="T-001", title="Task 1", scope=None, dod="DoD 1")
        daemon.c4_add_todo(task_id="T-002", title="Task 2", scope=None, dod="DoD 2")

        daemon.state_machine.transition("c4_run")

        # Workers get tasks
        daemon.c4_get_task("worker-1")
        daemon.c4_get_task("worker-2")

        # Worker 2 submits first
        result2 = daemon.c4_submit(
            task_id="T-002",
            commit_sha="222",
            validation_results=[{"name": "lint", "status": "pass"}],
        )
        assert result2.success is True

        # Worker 1 submits after
        result1 = daemon.c4_submit(
            task_id="T-001",
            commit_sha="111",
            validation_results=[{"name": "lint", "status": "pass"}],
        )
        assert result1.success is True

        # Both should be done
        state = daemon.state_machine.state
        assert "T-001" in state.queue.done
        assert "T-002" in state.queue.done


class TestSupervisorLoopAutomation:
    """E2E tests for supervisor loop automation"""

    @pytest.mark.asyncio
    async def test_supervisor_loop_processes_checkpoint(self, daemon):
        """Test supervisor loop processes queued checkpoints"""
        # Setup checkpoint config
        daemon._config.checkpoints = [
            CheckpointConfig(
                id="CP1",
                name="Review",
                required_tasks=["T-001"],
                required_validations=["lint"],  # Only require lint for this test
            )
        ]

        # Add and complete task
        daemon.c4_add_todo(task_id="T-001", title="Task 1", scope=None, dod="DoD")
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")
        daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
        )

        # Verify in checkpoint state
        assert daemon.state_machine.state.status == ProjectStatus.CHECKPOINT
        assert len(daemon.state_machine.state.checkpoint_queue) == 1

        # Create supervisor loop
        loop = SupervisorLoop(daemon)

        # Mock supervisor response
        mock_response = SupervisorResponse(
            decision=SupervisorDecision.APPROVE,
            checkpoint_id="CP1",
            notes="Looks good",
            required_changes=[],
        )

        with patch.object(daemon, "create_checkpoint_bundle") as mock_bundle:
            mock_bundle.return_value = Path("/tmp/bundle")

            with patch(
                "c4.daemon.supervisor_loop.Supervisor"
            ) as mock_supervisor_class:
                mock_supervisor = MagicMock()
                mock_supervisor.run_supervisor.return_value = mock_response
                mock_supervisor_class.return_value = mock_supervisor

                # Process checkpoint
                result = await loop._process_checkpoint_queue()

        assert result is True
        # Checkpoint should be processed
        assert len(daemon.state_machine.state.checkpoint_queue) == 0
        # State should transition
        assert daemon.state_machine.state.status == ProjectStatus.COMPLETE


class TestValidationFailureRecovery:
    """E2E tests for validation failure recovery"""

    def test_submit_fails_on_validation_failure(self, daemon):
        """Test submit fails when validations fail"""
        daemon.c4_add_todo(task_id="T-001", title="Task 1", scope=None, dod="DoD")
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        result = daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc",
            validation_results=[
                {"name": "lint", "status": "fail", "message": "lint errors"},
            ],
        )

        assert result.success is False
        assert result.next_action == "fix_failures"

        # Task should still be in progress
        assert "T-001" in daemon.state_machine.state.queue.in_progress

    def test_worker_can_resubmit_after_fix(self, daemon):
        """Test worker can resubmit after fixing issues"""
        daemon.c4_add_todo(task_id="T-001", title="Task 1", scope=None, dod="DoD")
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # First submit fails
        result1 = daemon.c4_submit(
            task_id="T-001",
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "fail"}],
        )
        assert result1.success is False

        # Second submit succeeds
        result2 = daemon.c4_submit(
            task_id="T-001",
            commit_sha="def",
            validation_results=[{"name": "lint", "status": "pass"}],
        )
        assert result2.success is True
