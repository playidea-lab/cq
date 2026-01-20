"""Tests for Task Dispatcher."""

from datetime import datetime
from unittest.mock import MagicMock

import pytest

from c4.daemon.task_dispatcher import (
    TaskAssignment,
    TaskDispatcher,
    TaskPriority,
)
from c4.models import C4State, TaskQueue, WorkerInfo


@pytest.fixture
def mock_state() -> C4State:
    """Create mock C4 state."""
    state = C4State(project_id="test-project")
    state.queue = TaskQueue(
        pending=["T-001", "T-002", "T-003"],
        in_progress={},
        done=[],
    )
    state.workers = {
        "worker-1": WorkerInfo(
            worker_id="worker-1",
            state="idle",
            joined_at=datetime.now(),
            last_seen=datetime.now(),
        ),
        "worker-2": WorkerInfo(
            worker_id="worker-2",
            state="idle",
            joined_at=datetime.now(),
            last_seen=datetime.now(),
        ),
    }
    return state


@pytest.fixture
def mock_state_machine(mock_state: C4State) -> MagicMock:
    """Create mock state machine."""
    sm = MagicMock()
    sm.state = mock_state
    return sm


@pytest.fixture
def dispatcher(mock_state_machine: MagicMock) -> TaskDispatcher:
    """Create dispatcher with mock state machine."""
    return TaskDispatcher(mock_state_machine)


class TestTaskPriority:
    """Test TaskPriority enum."""

    def test_priority_values(self) -> None:
        """Test priority values are ordered correctly."""
        assert TaskPriority.CRITICAL.value > TaskPriority.HIGH.value
        assert TaskPriority.HIGH.value > TaskPriority.NORMAL.value
        assert TaskPriority.NORMAL.value > TaskPriority.LOW.value
        assert TaskPriority.LOW.value > TaskPriority.BACKGROUND.value

    def test_priority_as_int(self) -> None:
        """Test priority can be used as int."""
        assert TaskPriority.CRITICAL == 100
        assert TaskPriority.NORMAL == 50


class TestTaskAssignment:
    """Test TaskAssignment dataclass."""

    def test_basic_assignment(self) -> None:
        """Test basic assignment creation."""
        assignment = TaskAssignment(
            task_id="T-001",
            worker_id="worker-1",
            priority=50,
        )

        assert assignment.task_id == "T-001"
        assert assignment.worker_id == "worker-1"
        assert assignment.is_repair is False

    def test_repair_assignment(self) -> None:
        """Test repair assignment creation."""
        assignment = TaskAssignment(
            task_id="T-001",
            worker_id="worker-2",
            priority=80,
            is_repair=True,
            original_worker_id="worker-1",
        )

        assert assignment.is_repair is True
        assert assignment.original_worker_id == "worker-1"


class TestPriorityManagement:
    """Test priority management methods."""

    def test_default_priority(self, dispatcher: TaskDispatcher) -> None:
        """Test default priority is NORMAL."""
        priority = dispatcher.get_priority("T-001")
        assert priority == TaskPriority.NORMAL.value

    def test_set_priority_enum(self, dispatcher: TaskDispatcher) -> None:
        """Test setting priority with enum."""
        dispatcher.set_priority("T-001", TaskPriority.HIGH)
        assert dispatcher.get_priority("T-001") == TaskPriority.HIGH.value

    def test_set_priority_int(self, dispatcher: TaskDispatcher) -> None:
        """Test setting priority with int."""
        dispatcher.set_priority("T-001", 75)
        assert dispatcher.get_priority("T-001") == 75

    def test_set_repair_priority(self, dispatcher: TaskDispatcher) -> None:
        """Test setting repair priority."""
        dispatcher.set_repair_priority("T-001")
        assert dispatcher.get_priority("T-001") == TaskPriority.HIGH.value


class TestTaskHistory:
    """Test task history for peer review."""

    def test_record_assignment(self, dispatcher: TaskDispatcher) -> None:
        """Test recording task assignments."""
        dispatcher.record_assignment("T-001", "worker-1")

        workers = dispatcher.get_previous_workers("T-001")
        assert "worker-1" in workers

    def test_multiple_assignments(self, dispatcher: TaskDispatcher) -> None:
        """Test multiple assignments for same task."""
        dispatcher.record_assignment("T-001", "worker-1")
        dispatcher.record_assignment("T-001", "worker-2")

        workers = dispatcher.get_previous_workers("T-001")
        assert len(workers) == 2
        assert "worker-1" in workers
        assert "worker-2" in workers

    def test_no_duplicates(self, dispatcher: TaskDispatcher) -> None:
        """Test no duplicate workers recorded."""
        dispatcher.record_assignment("T-001", "worker-1")
        dispatcher.record_assignment("T-001", "worker-1")

        workers = dispatcher.get_previous_workers("T-001")
        assert len(workers) == 1

    def test_is_repair_task(self, dispatcher: TaskDispatcher) -> None:
        """Test repair task detection."""
        assert dispatcher.is_repair_task("T-001") is False

        dispatcher.record_assignment("T-001", "worker-1")
        assert dispatcher.is_repair_task("T-001") is True


class TestPrioritizedTasks:
    """Test task prioritization."""

    def test_default_order(self, dispatcher: TaskDispatcher) -> None:
        """Test default order (all same priority)."""
        tasks = dispatcher.get_prioritized_tasks()
        assert tasks == ["T-001", "T-002", "T-003"]

    def test_priority_order(self, dispatcher: TaskDispatcher) -> None:
        """Test tasks ordered by priority."""
        dispatcher.set_priority("T-003", TaskPriority.HIGH)
        dispatcher.set_priority("T-001", TaskPriority.LOW)

        tasks = dispatcher.get_prioritized_tasks()
        assert tasks[0] == "T-003"  # HIGH priority first
        assert tasks[-1] == "T-001"  # LOW priority last


class TestFindEligibleTask:
    """Test finding eligible tasks."""

    def test_find_first_task(self, dispatcher: TaskDispatcher) -> None:
        """Test finding first available task."""
        task = dispatcher.find_eligible_task("worker-1")
        assert task == "T-001"

    def test_skip_repair_for_original_worker(self, dispatcher: TaskDispatcher) -> None:
        """Test skipping repair task for original worker."""
        # Mark T-001 as previously attempted by worker-1
        dispatcher.record_assignment("T-001", "worker-1")

        # worker-1 should get T-002 instead
        task = dispatcher.find_eligible_task("worker-1")
        assert task == "T-002"

    def test_repair_task_for_different_worker(self, dispatcher: TaskDispatcher) -> None:
        """Test repair task eligible for different worker."""
        dispatcher.record_assignment("T-001", "worker-1")

        # worker-2 can get T-001
        task = dispatcher.find_eligible_task("worker-2")
        assert task == "T-001"


class TestAssignNextTask:
    """Test task assignment."""

    def test_successful_assignment(self, dispatcher: TaskDispatcher) -> None:
        """Test successful task assignment."""
        result = dispatcher.assign_next_task("worker-1")

        assert result.success is True
        assert result.assignment is not None
        assert result.assignment.task_id == "T-001"
        assert result.assignment.worker_id == "worker-1"

    def test_unregistered_worker(self, dispatcher: TaskDispatcher) -> None:
        """Test assignment fails for unregistered worker."""
        result = dispatcher.assign_next_task("unknown-worker")

        assert result.success is False
        assert "not registered" in result.reason

    def test_busy_worker(self, dispatcher: TaskDispatcher, mock_state: C4State) -> None:
        """Test assignment fails for busy worker."""
        mock_state.workers["worker-1"].state = "busy"

        result = dispatcher.assign_next_task("worker-1")

        assert result.success is False
        assert "not idle" in result.reason

    def test_no_pending_tasks(self, dispatcher: TaskDispatcher, mock_state: C4State) -> None:
        """Test assignment fails when no tasks."""
        mock_state.queue.pending = []

        result = dispatcher.assign_next_task("worker-1")

        assert result.success is False
        assert "No pending tasks" in result.reason


class TestPeerReview:
    """Test peer review functionality."""

    def test_peer_review_enabled(self, dispatcher: TaskDispatcher) -> None:
        """Test peer review is enabled by default."""
        assert dispatcher._peer_review_enabled is True

    def test_peer_review_disabled(self, mock_state_machine: MagicMock) -> None:
        """Test peer review can be disabled."""
        dispatcher = TaskDispatcher(mock_state_machine, enable_peer_review=False)
        assert dispatcher._peer_review_enabled is False

    def test_peer_review_assignment(self, dispatcher: TaskDispatcher) -> None:
        """Test repair task goes to different worker."""
        # First assignment
        result1 = dispatcher.assign_next_task("worker-1")
        assert result1.success is True
        assert result1.assignment.task_id == "T-001"

        # Simulate failure and return to queue
        dispatcher.state.queue.pending.insert(0, "T-001")

        # worker-1 should skip T-001 now
        result2 = dispatcher.assign_next_task("worker-1")
        assert result2.success is True
        assert result2.assignment.task_id == "T-002"  # Gets next task

    def test_assign_repair_task(self, dispatcher: TaskDispatcher) -> None:
        """Test explicit repair task assignment."""
        result = dispatcher.assign_repair_task("T-001", "worker-1")

        assert result.success is True
        assert result.assignment.task_id == "T-001"
        assert result.assignment.worker_id == "worker-2"  # Different worker
        assert result.assignment.is_repair is True
        assert result.assignment.original_worker_id == "worker-1"

    def test_assign_repair_no_other_workers(
        self, dispatcher: TaskDispatcher, mock_state: C4State
    ) -> None:
        """Test repair assignment fails when no other workers."""
        # Make worker-2 busy
        mock_state.workers["worker-2"].state = "busy"

        result = dispatcher.assign_repair_task("T-001", "worker-1")

        assert result.success is False
        assert "No other idle workers" in result.reason


class TestLoadBalancing:
    """Test load balancing features."""

    def test_get_worker_load(self, dispatcher: TaskDispatcher, mock_state: C4State) -> None:
        """Test getting worker load."""
        mock_state.workers["worker-1"].state = "busy"

        load = dispatcher.get_worker_load()

        assert load["worker-1"] == 1
        assert load["worker-2"] == 0

    def test_get_least_loaded_worker(self, dispatcher: TaskDispatcher) -> None:
        """Test finding least loaded worker."""
        worker = dispatcher.get_least_loaded_worker()

        assert worker in ["worker-1", "worker-2"]

    def test_no_idle_workers(self, dispatcher: TaskDispatcher, mock_state: C4State) -> None:
        """Test no least loaded when all busy."""
        mock_state.workers["worker-1"].state = "busy"
        mock_state.workers["worker-2"].state = "busy"

        worker = dispatcher.get_least_loaded_worker()

        assert worker is None


class TestStatistics:
    """Test dispatcher statistics."""

    def test_get_stats(self, dispatcher: TaskDispatcher) -> None:
        """Test getting dispatcher stats."""
        stats = dispatcher.get_stats()

        assert stats["pending_tasks"] == 3
        assert stats["repair_tasks"] == 0
        assert stats["peer_review_enabled"] is True
        assert "priorities" in stats
        assert "worker_load" in stats

    def test_stats_with_repairs(self, dispatcher: TaskDispatcher) -> None:
        """Test stats include repair count."""
        dispatcher.record_assignment("T-001", "worker-1")

        stats = dispatcher.get_stats()

        assert stats["repair_tasks"] == 1
