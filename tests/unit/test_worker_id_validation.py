"""Tests for Worker ID validation and collision detection."""

from datetime import datetime, timedelta
from unittest.mock import MagicMock, Mock

import pytest

from c4.daemon.workers import WORKER_ID_PATTERN, WorkerManager
from c4.models import WorkerInfo


class TestWorkerIDPattern:
    """Test Worker ID format validation."""

    def test_valid_worker_ids(self) -> None:
        """Test valid worker IDs match the pattern."""
        valid_ids = [
            "worker-a1b2c3d4",
            "worker-00000000",
            "worker-ffffffff",
            "worker-12345678",
            "worker-abcdef01",
        ]
        for worker_id in valid_ids:
            assert WORKER_ID_PATTERN.match(worker_id), f"{worker_id} should be valid"

    def test_invalid_worker_ids(self) -> None:
        """Test invalid worker IDs are rejected."""
        invalid_ids = [
            "claude-worker",  # Old fixed value
            "worker-1",  # Too short
            "worker-123",  # Too short
            "worker-12345",  # Too short
            "worker-1234567",  # Too short (7 chars)
            "worker-123456789",  # Too long (9 chars)
            "worker-ABCDEF01",  # Uppercase not allowed
            "worker-g1234567",  # Invalid hex char 'g'
            "worker_a1b2c3d4",  # Underscore instead of hyphen
            "workera1b2c3d4",  # Missing hyphen
            "a1b2c3d4",  # Missing prefix
        ]
        for worker_id in invalid_ids:
            assert not WORKER_ID_PATTERN.match(worker_id), f"{worker_id} should be invalid"


class TestWorkerRegistration:
    """Test Worker registration with ID validation."""

    @pytest.fixture
    def mock_state_machine(self) -> Mock:
        """Create mock state machine."""
        state_machine = MagicMock()
        state_machine.state.workers = {}
        return state_machine

    @pytest.fixture
    def mock_config(self) -> Mock:
        """Create mock config."""
        return MagicMock()

    @pytest.fixture
    def worker_manager(self, mock_state_machine: Mock, mock_config: Mock) -> WorkerManager:
        """Create WorkerManager with mocks."""
        return WorkerManager(mock_state_machine, mock_config)

    def test_register_valid_worker_id(self, worker_manager: WorkerManager) -> None:
        """Test registering worker with valid UUID-based ID."""
        worker_id = "worker-a1b2c3d4"
        worker = worker_manager.register(worker_id)

        assert worker.worker_id == worker_id
        assert worker.state == "idle"
        assert worker_id in worker_manager._workers

    def test_register_invalid_worker_id_format(self, worker_manager: WorkerManager) -> None:
        """Test registering worker with invalid ID format raises ValueError."""
        invalid_ids = [
            "claude-worker",
            "worker-1",
            "worker-ABCD1234",
            "not-a-worker",
        ]

        for invalid_id in invalid_ids:
            with pytest.raises(ValueError, match="Invalid worker_id format"):
                worker_manager.register(invalid_id)

    def test_register_duplicate_worker_id(self, worker_manager: WorkerManager) -> None:
        """Test registering duplicate worker ID raises ValueError."""
        worker_id = "worker-a1b2c3d4"

        # First registration succeeds
        worker_manager.register(worker_id)

        # Second registration with same ID fails
        with pytest.raises(ValueError, match="already registered"):
            worker_manager.register(worker_id)

    def test_register_error_message_contains_uuid_hint(
        self, worker_manager: WorkerManager
    ) -> None:
        """Test error message guides users to use UUID generation."""
        with pytest.raises(ValueError) as exc_info:
            worker_manager.register("claude-worker")

        error_msg = str(exc_info.value)
        assert "uuid.uuid4().hex[:8]" in error_msg
        assert "worker-[a-f0-9]{8}" in error_msg


class TestInstanceCollisionDetection:
    """Test instance collision detection in task resume."""

    @pytest.fixture
    def mock_daemon(self) -> Mock:
        """Create mock daemon."""
        daemon = MagicMock()
        daemon.worker_manager = MagicMock()
        daemon.lock_store = MagicMock()
        daemon.config = MagicMock()
        daemon.get_task = MagicMock()
        return daemon

    def test_resume_detects_active_collision(self, mock_daemon: Mock) -> None:
        """Test resume rejects when another instance is actively using the worker ID."""
        from c4.daemon.task_ops import TaskOps

        task_ops = TaskOps(mock_daemon)

        worker_id = "worker-a1b2c3d4"
        task_id = "T-001-0"

        # Mock state with in_progress task
        state = MagicMock()
        state.queue.in_progress = {task_id: worker_id}
        state.project_id = "test-project"

        # Mock task
        mock_task = MagicMock()
        mock_task.scope = "src/"
        mock_daemon.get_task.return_value = mock_task

        # Mock worker with RECENT last_seen (indicates active instance)
        mock_worker = WorkerInfo(
            worker_id=worker_id,
            state="busy",
            joined_at=datetime.now(),
            last_seen=datetime.now() - timedelta(seconds=2),  # 2 seconds ago
        )
        mock_daemon.worker_manager.get_worker.return_value = mock_worker

        # Try to resume - should fail due to collision
        result = task_ops._try_resume_task(worker_id, state)

        assert result is None, "Resume should be rejected due to active instance collision"

    def test_resume_allows_stale_worker(self, mock_daemon: Mock) -> None:
        """Test resume allows when worker's last_seen is old (crashed instance)."""
        from c4.daemon.task_ops import TaskOps

        task_ops = TaskOps(mock_daemon)

        worker_id = "worker-a1b2c3d4"
        task_id = "T-001-0"

        # Mock state with in_progress task
        state = MagicMock()
        state.queue.in_progress = {task_id: worker_id}
        state.project_id = "test-project"

        # Mock task
        mock_task = MagicMock()
        mock_task.id = task_id
        mock_task.title = "Test task"
        mock_task.scope = "src/"
        mock_task.dod = "Complete the work"
        mock_task.validations = ["lint"]
        mock_task.branch = "c4/w-T-001-0"
        mock_task.model = "opus"
        mock_task.assigned_to = worker_id
        mock_task.status = "in_progress"
        mock_daemon.get_task.return_value = mock_task

        # Mock worker with OLD last_seen (indicates crashed/stale instance)
        mock_worker = WorkerInfo(
            worker_id=worker_id,
            state="busy",
            joined_at=datetime.now() - timedelta(minutes=10),
            last_seen=datetime.now() - timedelta(minutes=5),  # 5 minutes ago
        )
        mock_daemon.worker_manager.get_worker.return_value = mock_worker

        # Mock lock verification
        mock_daemon.lock_store.get_lock_owner.return_value = worker_id
        mock_daemon.lock_store.refresh_scope_lock.return_value = True

        # Mock agent routing
        mock_daemon._get_agent_routing.return_value = {
            "recommended_agent": "backend-architect",
            "agent_chain": [],
        }

        # Mock worktree
        task_ops._get_or_create_worktree = MagicMock(return_value=None)

        # Try to resume - should succeed (stale worker, not a collision)
        result = task_ops._try_resume_task(worker_id, state)

        assert result is not None, "Resume should succeed for stale worker"
        assert result.task_id == task_id


class TestWorkerIDGenerationGuidance:
    """Test that error messages guide users correctly."""

    def test_fixed_value_error_message(self) -> None:
        """Test error message for fixed values explains UUID requirement."""
        state_machine = MagicMock()
        state_machine.state.workers = {}
        config = MagicMock()
        manager = WorkerManager(state_machine, config)

        with pytest.raises(ValueError) as exc_info:
            manager.register("claude-worker")

        error_msg = str(exc_info.value).lower()
        assert "uuid" in error_msg
        assert "worker-[a-f0-9]{8}" in error_msg

    def test_short_id_error_message(self) -> None:
        """Test error message for short IDs."""
        state_machine = MagicMock()
        state_machine.state.workers = {}
        config = MagicMock()
        manager = WorkerManager(state_machine, config)

        with pytest.raises(ValueError) as exc_info:
            manager.register("worker-123")

        error_msg = str(exc_info.value)
        assert "Invalid worker_id format" in error_msg
