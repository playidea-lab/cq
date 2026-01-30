"""Unit tests for SafetyGuard in c4/daemon/safety.py

Tests safety monitoring including iteration limits, timeouts,
failure tracking, and can_continue checks.
"""

from datetime import datetime, timedelta
from unittest.mock import patch

from c4.daemon.safety import (
    DEFAULT_MAX_ITERATIONS_PER_TASK,
    DEFAULT_TASK_TIMEOUT_SEC,
    MAX_CONSECUTIVE_FAILURES,
    MAX_TOTAL_ITERATIONS,
    SESSION_TIMEOUT_SECONDS,
    SafetyGuard,
    SafetyState,
    TaskStats,
)


class TestTaskStats:
    """Tests for TaskStats dataclass."""

    def test_default_values(self):
        """Should initialize with default values."""
        stats = TaskStats(task_id="T-001-0")

        assert stats.task_id == "T-001-0"
        assert stats.iterations == 0
        assert stats.failures == 0
        assert isinstance(stats.started_at, datetime)
        assert isinstance(stats.last_activity, datetime)

    def test_custom_values(self):
        """Should accept custom values."""
        custom_time = datetime(2025, 1, 1, 12, 0, 0)
        stats = TaskStats(
            task_id="T-002-0",
            started_at=custom_time,
            iterations=5,
            failures=2,
            last_activity=custom_time,
        )

        assert stats.task_id == "T-002-0"
        assert stats.iterations == 5
        assert stats.failures == 2
        assert stats.started_at == custom_time


class TestSafetyState:
    """Tests for SafetyState dataclass."""

    def test_default_values(self):
        """Should initialize with default values."""
        state = SafetyState()

        assert state.total_iterations == 0
        assert state.consecutive_failures == 0
        assert state.task_stats == {}
        assert state.last_task_id is None
        assert isinstance(state.session_started, datetime)

    def test_custom_values(self):
        """Should accept custom values."""
        custom_time = datetime(2025, 1, 1, 12, 0, 0)
        task_stats = {"T-001-0": TaskStats(task_id="T-001-0")}

        state = SafetyState(
            session_started=custom_time,
            total_iterations=10,
            consecutive_failures=3,
            task_stats=task_stats,
            last_task_id="T-001-0",
        )

        assert state.total_iterations == 10
        assert state.consecutive_failures == 3
        assert len(state.task_stats) == 1


class TestSafetyGuardInit:
    """Tests for SafetyGuard initialization."""

    def test_default_limits(self):
        """Should use default limits when none specified."""
        guard = SafetyGuard()

        assert guard.max_iterations_per_task == DEFAULT_MAX_ITERATIONS_PER_TASK
        assert guard.max_total_iterations == MAX_TOTAL_ITERATIONS
        assert guard.max_consecutive_failures == MAX_CONSECUTIVE_FAILURES
        assert guard.task_timeout_seconds == DEFAULT_TASK_TIMEOUT_SEC
        assert guard.session_timeout_seconds == SESSION_TIMEOUT_SECONDS

    def test_custom_limits(self):
        """Should accept custom limits."""
        guard = SafetyGuard(
            max_iterations_per_task=50,
            max_total_iterations=500,
            max_consecutive_failures=10,
            task_timeout_seconds=1800,
            session_timeout_seconds=7200,
        )

        assert guard.max_iterations_per_task == 50
        assert guard.max_total_iterations == 500
        assert guard.max_consecutive_failures == 10
        assert guard.task_timeout_seconds == 1800
        assert guard.session_timeout_seconds == 7200


class TestSafetyGuardReset:
    """Tests for SafetyGuard.reset method."""

    def test_reset_clears_all_state(self):
        """reset() should clear all safety state."""
        guard = SafetyGuard()

        # Add some state
        guard.start_task("T-001-0")
        guard.record_iteration("T-001-0")
        guard.record_failure("T-001-0")

        guard.reset()

        assert guard._state.total_iterations == 0
        assert guard._state.consecutive_failures == 0
        assert guard._state.task_stats == {}
        assert guard._state.last_task_id is None


class TestSafetyGuardStartTask:
    """Tests for SafetyGuard.start_task method."""

    def test_start_task_creates_task_stats(self):
        """start_task() should create TaskStats for new task."""
        guard = SafetyGuard()

        guard.start_task("T-001-0")

        assert "T-001-0" in guard._state.task_stats
        stats = guard._state.task_stats["T-001-0"]
        assert stats.task_id == "T-001-0"
        assert stats.iterations == 0

    def test_start_task_updates_last_task_id(self):
        """start_task() should update last_task_id."""
        guard = SafetyGuard()

        guard.start_task("T-001-0")

        assert guard._state.last_task_id == "T-001-0"

    def test_start_task_does_not_overwrite_existing(self):
        """start_task() should not overwrite existing task stats."""
        guard = SafetyGuard()

        guard.start_task("T-001-0")
        guard.record_iteration("T-001-0")

        # Start same task again
        guard.start_task("T-001-0")

        # Iterations should be preserved
        assert guard._state.task_stats["T-001-0"].iterations == 1


class TestSafetyGuardRecordIteration:
    """Tests for SafetyGuard.record_iteration method."""

    def test_record_iteration_increments_total(self):
        """record_iteration() should increment total iterations."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        guard.record_iteration()

        assert guard._state.total_iterations == 1

    def test_record_iteration_increments_task_iterations(self):
        """record_iteration() should increment task-specific iterations."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        guard.record_iteration("T-001-0")
        guard.record_iteration("T-001-0")

        assert guard._state.task_stats["T-001-0"].iterations == 2

    def test_record_iteration_uses_last_task_id(self):
        """record_iteration() should use last_task_id if not specified."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        guard.record_iteration()  # No task_id

        assert guard._state.task_stats["T-001-0"].iterations == 1


class TestSafetyGuardRecordFailure:
    """Tests for SafetyGuard.record_failure method."""

    def test_record_failure_increments_consecutive(self):
        """record_failure() should increment consecutive failures."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        guard.record_failure()
        guard.record_failure()

        assert guard._state.consecutive_failures == 2

    def test_record_failure_increments_task_failures(self):
        """record_failure() should increment task-specific failures."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        guard.record_failure("T-001-0")

        assert guard._state.task_stats["T-001-0"].failures == 1


class TestSafetyGuardRecordSuccess:
    """Tests for SafetyGuard.record_success method."""

    def test_record_success_resets_consecutive_failures(self):
        """record_success() should reset consecutive failures to 0."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        # Record some failures
        guard.record_failure()
        guard.record_failure()
        assert guard._state.consecutive_failures == 2

        # Record success
        guard.record_success()

        assert guard._state.consecutive_failures == 0


class TestSafetyGuardCheckCanContinue:
    """Tests for SafetyGuard.check_can_continue method."""

    def test_returns_true_when_all_ok(self):
        """check_can_continue() should return (True, '') when no limits exceeded."""
        guard = SafetyGuard()
        guard.start_task("T-001-0")

        can_continue, reason = guard.check_can_continue()

        assert can_continue is True
        assert reason == ""

    def test_returns_false_on_max_iterations(self):
        """check_can_continue() should return False when max task iterations reached."""
        guard = SafetyGuard(max_iterations_per_task=3)
        guard.start_task("T-001-0")

        # Record max iterations
        for _ in range(3):
            guard.record_iteration("T-001-0")

        can_continue, reason = guard.check_can_continue("T-001-0")

        assert can_continue is False
        assert "Max iterations per task" in reason

    def test_returns_false_on_max_total_iterations(self):
        """check_can_continue() should return False when max total iterations reached."""
        guard = SafetyGuard(max_total_iterations=5)
        guard.start_task("T-001-0")

        for _ in range(5):
            guard.record_iteration()

        can_continue, reason = guard.check_can_continue()

        assert can_continue is False
        assert "Max total iterations" in reason

    def test_returns_false_on_timeout(self):
        """check_can_continue() should return False when task timeout exceeded."""
        guard = SafetyGuard(task_timeout_seconds=60)
        guard.start_task("T-001-0")

        # Simulate elapsed time by patching datetime
        future = datetime.now() + timedelta(seconds=61)
        with patch("c4.daemon.safety.datetime") as mock_datetime:
            mock_datetime.now.return_value = future
            # Need to ensure started_at is in the past
            guard._state.task_stats["T-001-0"].started_at = datetime.now() - timedelta(
                seconds=120
            )

            can_continue, reason = guard.check_can_continue("T-001-0")

        assert can_continue is False
        assert "Task timeout" in reason

    def test_returns_false_on_session_timeout(self):
        """check_can_continue() should return False when session timeout exceeded."""
        guard = SafetyGuard(session_timeout_seconds=3600)

        # Simulate old session start
        guard._state.session_started = datetime.now() - timedelta(seconds=3601)

        can_continue, reason = guard.check_can_continue()

        assert can_continue is False
        assert "Session timeout" in reason

    def test_returns_false_on_max_failures(self):
        """check_can_continue() should return False when max consecutive failures reached."""
        guard = SafetyGuard(max_consecutive_failures=3)
        guard.start_task("T-001-0")

        for _ in range(3):
            guard.record_failure()

        can_continue, reason = guard.check_can_continue()

        assert can_continue is False
        assert "Max consecutive failures" in reason


class TestSafetyGuardGetStatus:
    """Tests for SafetyGuard.get_status method."""

    def test_get_status_returns_complete_info(self):
        """get_status() should return complete status information."""
        guard = SafetyGuard(
            max_iterations_per_task=100,
            max_total_iterations=1000,
            max_consecutive_failures=5,
            task_timeout_seconds=900,
            session_timeout_seconds=3600,
        )
        guard.start_task("T-001-0")
        guard.record_iteration("T-001-0")
        guard.record_failure("T-001-0")

        status = guard.get_status()

        assert "session_started" in status
        assert "session_elapsed_seconds" in status
        assert status["total_iterations"] == 1
        assert status["consecutive_failures"] == 1
        assert status["task_count"] == 1
        assert status["can_continue"] is True

        # Check limits
        limits = status["limits"]
        assert limits["max_iterations_per_task"] == 100
        assert limits["max_total_iterations"] == 1000
        assert limits["max_consecutive_failures"] == 5
        assert limits["task_timeout_seconds"] == 900
        assert limits["session_timeout_seconds"] == 3600

    def test_get_status_can_continue_false_when_exceeded(self):
        """get_status() should show can_continue=False when limits exceeded."""
        guard = SafetyGuard(max_consecutive_failures=2)

        guard.record_failure()
        guard.record_failure()

        status = guard.get_status()

        assert status["can_continue"] is False
