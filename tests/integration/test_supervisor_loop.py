"""Integration tests for C4 Supervisor Loop and checkpoint/repair queue processing"""

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.daemon.supervisor_loop import SupervisorLoop, SupervisorLoopManager
from c4.mcp_server import C4Daemon
from c4.models import (
    CheckpointQueueItem,
    ProjectStatus,
    RepairQueueItem,
    SupervisorDecision,
)
from c4.supervisor import SupervisorResponse


@pytest.fixture
def daemon_with_project(tmp_path):
    """Create a daemon with initialized project"""
    daemon = C4Daemon(tmp_path)
    daemon.initialize("test-project", with_default_checkpoints=False)
    # Skip discovery phase to go directly to PLAN for testing
    daemon.state_machine.transition("skip_discovery")
    return daemon


@pytest.fixture
def daemon_in_checkpoint(daemon_with_project):
    """Create a daemon in CHECKPOINT state with queued checkpoint"""
    daemon = daemon_with_project

    # Add a task
    daemon.c4_add_todo(
        task_id="T-001",
        title="Test task",
        scope=None,
        dod="Complete test task",
    )

    # Transition to EXECUTE
    daemon.state_machine.transition("c4_run")

    # Assign task and complete it
    daemon.c4_get_task("worker-1")

    # Complete the task (move from in_progress to done)
    state = daemon.state_machine.state
    del state.queue.in_progress["T-001"]
    state.queue.done.append("T-001")

    # Manually create checkpoint state for testing
    state.status = ProjectStatus.CHECKPOINT
    state.checkpoint.current = "CP1"
    state.checkpoint.state = "reviewing"

    # Set worker to idle
    if "worker-1" in state.workers:
        state.workers["worker-1"].state = "idle"
        state.workers["worker-1"].task_id = None

    # Add to checkpoint queue
    state.checkpoint_queue.append(
        CheckpointQueueItem(
            checkpoint_id="CP1",
            triggered_at="2024-01-01T00:00:00",
            tasks_completed=["T-001"],
            validation_results=[],
        )
    )
    daemon.state_machine.save_state()

    return daemon


class TestSupervisorLoopBasics:
    """Basic tests for SupervisorLoop"""

    def test_supervisor_loop_initialization(self, daemon_with_project):
        """Test supervisor loop initialization"""
        loop = SupervisorLoop(daemon_with_project)

        assert loop.daemon is daemon_with_project
        assert loop.running is False
        assert loop.poll_interval == 1.0

    def test_supervisor_loop_custom_settings(self, daemon_with_project):
        """Test supervisor loop with custom settings"""
        loop = SupervisorLoop(
            daemon_with_project,
            poll_interval=0.5,
            max_retries=5,
            supervisor_timeout=600,
        )

        assert loop.poll_interval == 0.5
        assert loop.max_retries == 5
        assert loop.supervisor_timeout == 600

    def test_supervisor_loop_stop(self, daemon_with_project):
        """Test stopping supervisor loop"""
        loop = SupervisorLoop(daemon_with_project)
        loop.running = True

        loop.stop()

        assert loop.running is False


class TestSupervisorLoopManager:
    """Tests for SupervisorLoopManager"""

    def test_manager_initialization(self, daemon_with_project):
        """Test manager initialization"""
        manager = SupervisorLoopManager(daemon_with_project)

        assert manager.daemon is daemon_with_project
        assert manager._loop is None
        assert manager._task is None

    def test_manager_is_running_initially_false(self, daemon_with_project):
        """Test is_running property initially False"""
        manager = SupervisorLoopManager(daemon_with_project)

        assert manager.is_running is False


class TestDaemonSupervisorIntegration:
    """Tests for C4Daemon supervisor integration"""

    def test_daemon_has_supervisor_loop_manager(self, daemon_with_project):
        """Test daemon has supervisor_loop_manager property"""
        daemon = daemon_with_project

        manager = daemon.supervisor_loop_manager
        assert isinstance(manager, SupervisorLoopManager)

    def test_daemon_is_supervisor_loop_running(self, daemon_with_project):
        """Test is_supervisor_loop_running property"""
        daemon = daemon_with_project

        assert daemon.is_supervisor_loop_running is False

    def test_status_includes_supervisor_loop_running(self, daemon_with_project):
        """Test c4_status includes supervisor_loop_running"""
        daemon = daemon_with_project

        status = daemon.c4_status()
        assert "supervisor_loop_running" in status
        assert status["supervisor_loop_running"] is False


class TestCheckpointQueueProcessing:
    """Tests for checkpoint queue processing"""

    def test_checkpoint_queue_in_status(self, daemon_in_checkpoint):
        """Test checkpoint queue appears in status"""
        status = daemon_in_checkpoint.c4_status()

        assert "checkpoint_queue" in status
        assert len(status["checkpoint_queue"]) == 1
        assert status["checkpoint_queue"][0]["checkpoint_id"] == "CP1"

    @pytest.mark.asyncio
    async def test_process_checkpoint_queue_empty(self, daemon_with_project):
        """Test processing empty checkpoint queue"""
        loop = SupervisorLoop(daemon_with_project)

        result = await loop._process_checkpoint_queue()
        assert result is False

    @pytest.mark.asyncio
    async def test_process_checkpoint_queue_with_item(self, daemon_in_checkpoint):
        """Test processing checkpoint queue with item (mocked supervisor)"""
        loop = SupervisorLoop(daemon_in_checkpoint)

        # Mock the supervisor
        mock_response = SupervisorResponse(
            decision=SupervisorDecision.APPROVE,
            checkpoint_id="CP1",
            notes="Looks good",
            required_changes=[],
        )

        with patch.object(
            loop.daemon, "create_checkpoint_bundle"
        ) as mock_bundle:
            mock_bundle.return_value = Path("/tmp/bundle")

            with patch(
                "c4.daemon.supervisor_loop.Supervisor"
            ) as mock_supervisor_class:
                mock_supervisor = MagicMock()
                mock_supervisor.run_supervisor.return_value = mock_response
                mock_supervisor_class.return_value = mock_supervisor

                result = await loop._process_checkpoint_queue()

        assert result is True
        # Checkpoint should be removed from queue after processing
        assert len(daemon_in_checkpoint.state_machine.state.checkpoint_queue) == 0


class TestRepairQueueProcessing:
    """Tests for repair queue processing"""

    def test_repair_queue_in_status(self, daemon_with_project):
        """Test repair queue appears in status"""
        daemon = daemon_with_project

        # Add to repair queue
        daemon.state_machine.state.repair_queue.append(
            RepairQueueItem(
                task_id="T-001",
                worker_id="worker-1",
                failure_signature="test failure",
                attempts=5,
                blocked_at="2024-01-01T00:00:00",
            )
        )
        daemon.state_machine.save_state()

        status = daemon.c4_status()
        assert "repair_queue" in status
        assert len(status["repair_queue"]) == 1
        assert status["repair_queue"][0]["task_id"] == "T-001"

    @pytest.mark.asyncio
    async def test_process_repair_queue_empty(self, daemon_with_project):
        """Test processing empty repair queue"""
        loop = SupervisorLoop(daemon_with_project)

        result = await loop._process_repair_queue()
        assert result is False


class TestMarkBlockedIntegration:
    """Tests for mark_blocked integration with repair queue"""

    def test_mark_blocked_adds_to_repair_queue(self, daemon_with_project):
        """Test c4_mark_blocked adds task to repair queue"""
        daemon = daemon_with_project

        # Add and assign a task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Complete test",
        )
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Mark as blocked
        result = daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="lint: multiple errors",
            attempts=10,
            last_error="NameError: name 'foo' is not defined",
        )

        assert result["success"] is True
        assert result["repair_queue_size"] == 1

        # Verify in state
        state = daemon.state_machine.state
        assert len(state.repair_queue) == 1
        assert state.repair_queue[0].task_id == "T-001"
        assert state.repair_queue[0].attempts == 10

    def test_mark_blocked_releases_task(self, daemon_with_project):
        """Test c4_mark_blocked releases task from in_progress"""
        daemon = daemon_with_project

        # Add and assign a task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Complete test",
        )
        daemon.state_machine.transition("c4_run")
        daemon.c4_get_task("worker-1")

        # Verify task is in progress
        assert "T-001" in daemon.state_machine.state.queue.in_progress

        # Mark as blocked
        daemon.c4_mark_blocked(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=10,
        )

        # Verify task is no longer in progress
        assert "T-001" not in daemon.state_machine.state.queue.in_progress
