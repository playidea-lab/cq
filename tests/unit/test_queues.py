"""Unit tests for C4 queue models and safety guards"""


from c4.constants import MAX_ITERATIONS_PER_TASK
from c4.daemon.safety import SafetyGuard
from c4.models import C4State, CheckpointQueueItem, RepairQueueItem, ValidationResult


class TestCheckpointQueueItem:
    """Tests for CheckpointQueueItem model"""

    def test_create_checkpoint_queue_item(self):
        """Test creating a checkpoint queue item"""
        item = CheckpointQueueItem(
            checkpoint_id="CP1",
            triggered_at="2024-01-01T00:00:00",
            tasks_completed=["T-001", "T-002"],
            validation_results=[
                ValidationResult(name="lint", status="pass"),
                ValidationResult(name="unit", status="pass"),
            ],
        )

        assert item.checkpoint_id == "CP1"
        assert item.triggered_at == "2024-01-01T00:00:00"
        assert len(item.tasks_completed) == 2
        assert len(item.validation_results) == 2

    def test_checkpoint_queue_item_defaults(self):
        """Test default values for checkpoint queue item"""
        item = CheckpointQueueItem(
            checkpoint_id="CP1",
            triggered_at="2024-01-01T00:00:00",
        )

        assert item.tasks_completed == []
        assert item.validation_results == []


class TestRepairQueueItem:
    """Tests for RepairQueueItem model"""

    def test_create_repair_queue_item(self):
        """Test creating a repair queue item"""
        item = RepairQueueItem(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="lint: undefined variable",
            attempts=5,
            blocked_at="2024-01-01T00:00:00",
            last_error="NameError: name 'foo' is not defined",
        )

        assert item.task_id == "T-001"
        assert item.worker_id == "worker-1"
        assert item.failure_signature == "lint: undefined variable"
        assert item.attempts == 5
        assert "foo" in item.last_error

    def test_repair_queue_item_default_last_error(self):
        """Test default last_error value"""
        item = RepairQueueItem(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=1,
            blocked_at="2024-01-01T00:00:00",
        )

        assert item.last_error == ""


class TestC4StateWithQueues:
    """Tests for C4State with queue fields"""

    def test_state_with_empty_queues(self):
        """Test state initialization with empty queues"""
        state = C4State(project_id="test-project")

        assert state.checkpoint_queue == []
        assert state.repair_queue == []

    def test_state_add_to_checkpoint_queue(self):
        """Test adding items to checkpoint queue"""
        state = C4State(project_id="test-project")

        item = CheckpointQueueItem(
            checkpoint_id="CP1",
            triggered_at="2024-01-01T00:00:00",
        )
        state.checkpoint_queue.append(item)

        assert len(state.checkpoint_queue) == 1
        assert state.checkpoint_queue[0].checkpoint_id == "CP1"

    def test_state_add_to_repair_queue(self):
        """Test adding items to repair queue"""
        state = C4State(project_id="test-project")

        item = RepairQueueItem(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="test failure",
            attempts=1,
            blocked_at="2024-01-01T00:00:00",
        )
        state.repair_queue.append(item)

        assert len(state.repair_queue) == 1
        assert state.repair_queue[0].task_id == "T-001"

    def test_state_serialization_with_queues(self):
        """Test state serialization includes queues"""
        state = C4State(project_id="test-project")
        state.checkpoint_queue.append(
            CheckpointQueueItem(
                checkpoint_id="CP1",
                triggered_at="2024-01-01T00:00:00",
            )
        )

        data = state.model_dump()
        assert "checkpoint_queue" in data
        assert "repair_queue" in data
        assert len(data["checkpoint_queue"]) == 1


class TestSafetyGuard:
    """Tests for SafetyGuard"""

    def test_safety_guard_initialization(self):
        """Test safety guard initialization with defaults"""
        guard = SafetyGuard()

        assert guard.max_iterations_per_task == MAX_ITERATIONS_PER_TASK
        assert guard.max_total_iterations == 100
        assert guard.max_consecutive_failures == 5

    def test_safety_guard_custom_limits(self):
        """Test safety guard with custom limits"""
        guard = SafetyGuard(
            max_iterations_per_task=5,
            max_total_iterations=50,
            max_consecutive_failures=3,
        )

        assert guard.max_iterations_per_task == 5
        assert guard.max_total_iterations == 50
        assert guard.max_consecutive_failures == 3

    def test_safety_guard_can_continue_initially(self):
        """Test that can_continue returns True initially"""
        guard = SafetyGuard()

        can_continue, reason = guard.check_can_continue()
        assert can_continue is True
        assert reason == ""

    def test_safety_guard_total_iterations_limit(self):
        """Test total iterations limit"""
        guard = SafetyGuard(max_total_iterations=3)

        for _ in range(3):
            guard.record_iteration()

        can_continue, reason = guard.check_can_continue()
        assert can_continue is False
        assert "total iterations" in reason.lower()

    def test_safety_guard_consecutive_failures_limit(self):
        """Test consecutive failures limit"""
        guard = SafetyGuard(max_consecutive_failures=2)

        guard.record_failure()
        guard.record_failure()

        can_continue, reason = guard.check_can_continue()
        assert can_continue is False
        assert "consecutive failures" in reason.lower()

    def test_safety_guard_success_resets_failures(self):
        """Test that success resets consecutive failures"""
        guard = SafetyGuard(max_consecutive_failures=3)

        guard.record_failure()
        guard.record_failure()
        guard.record_success()  # Should reset

        can_continue, reason = guard.check_can_continue()
        assert can_continue is True

    def test_safety_guard_per_task_iterations(self):
        """Test per-task iteration limit"""
        guard = SafetyGuard(max_iterations_per_task=2)

        guard.start_task("T-001")
        guard.record_iteration("T-001")
        guard.record_iteration("T-001")

        can_continue, reason = guard.check_can_continue("T-001")
        assert can_continue is False
        assert "T-001" in reason

    def test_safety_guard_reset(self):
        """Test safety guard reset"""
        guard = SafetyGuard(max_total_iterations=3)

        for _ in range(3):
            guard.record_iteration()

        guard.reset()

        can_continue, reason = guard.check_can_continue()
        assert can_continue is True

    def test_safety_guard_status(self):
        """Test getting safety status"""
        guard = SafetyGuard()
        guard.start_task("T-001")
        guard.record_iteration("T-001")
        guard.record_failure("T-001")

        status = guard.get_status()

        assert "total_iterations" in status
        assert status["total_iterations"] == 1
        assert "consecutive_failures" in status
        assert status["consecutive_failures"] == 1
        assert "limits" in status
        assert "can_continue" in status
