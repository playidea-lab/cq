"""Tests for Checkpoint Multiple Completion Logic.

Tests the unified queue checkpoint behavior:
- CP tasks require required_completions (default: 2) to be fully complete
- completion_count tracks current completion count
- completed_by_sessions tracks which workers have completed
- Same worker can complete twice (with context clear) unless configured otherwise
"""

from c4.models import Task
from c4.models.config import TaskSystemConfig
from c4.models.enums import TaskType


class TestCheckpointRequiredCompletions:
    """Tests for required_completions field behavior."""

    def test_default_is_one_for_non_cp(self) -> None:
        """Non-CP tasks default to 1 required completion."""
        impl_task = Task(
            id="T-001-0",
            title="Implement",
            dod="Test",
            type=TaskType.IMPLEMENTATION,
        )
        assert impl_task.required_completions == 1

        review_task = Task(
            id="R-001-0",
            title="Review",
            dod="Test",
            type=TaskType.REVIEW,
        )
        assert review_task.required_completions == 1

        repair_task = Task(
            id="RPR-001",
            title="Repair",
            dod="Test",
            type=TaskType.REPAIR,
        )
        assert repair_task.required_completions == 1

    def test_cp_task_with_required_completions(self) -> None:
        """CP tasks can specify required_completions."""
        cp_task = Task(
            id="CP-001",
            title="Phase 1 Checkpoint",
            dod="Review all changes",
            type=TaskType.CHECKPOINT,
            phase_id="phase-1",
            required_completions=2,
        )
        assert cp_task.required_completions == 2
        assert cp_task.completion_count == 0

    def test_cp_task_can_have_three_completions(self) -> None:
        """CP tasks can require 3 completions."""
        cp_task = Task(
            id="CP-001",
            title="Critical Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=3,
        )
        assert cp_task.required_completions == 3


class TestCompletionTracking:
    """Tests for completion_count and completed_by_sessions tracking."""

    def test_increment_completion_count(self) -> None:
        """completion_count increments on each completion."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
        )

        # First completion
        cp_task.completion_count += 1
        assert cp_task.completion_count == 1

        # Second completion
        cp_task.completion_count += 1
        assert cp_task.completion_count == 2

    def test_track_completing_sessions(self) -> None:
        """completed_by_sessions tracks who completed."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
        )

        # First completion by worker-1
        cp_task.completed_by_sessions.append("worker-1")
        assert cp_task.completed_by_sessions == ["worker-1"]

        # Second completion by worker-2
        cp_task.completed_by_sessions.append("worker-2")
        assert cp_task.completed_by_sessions == ["worker-1", "worker-2"]

    def test_same_worker_appears_twice(self) -> None:
        """Same worker can appear twice in completed_by_sessions."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
        )

        cp_task.completed_by_sessions.append("worker-1")
        cp_task.completed_by_sessions.append("worker-1")

        assert len(cp_task.completed_by_sessions) == 2
        assert cp_task.completed_by_sessions.count("worker-1") == 2


class TestCheckpointCompletionLogic:
    """Tests for checkpoint completion decision logic."""

    def test_not_complete_until_threshold(self) -> None:
        """CP is not fully complete until completion_count >= required."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=1,
        )

        is_fully_complete = cp_task.completion_count >= cp_task.required_completions
        assert not is_fully_complete

    def test_complete_at_threshold(self) -> None:
        """CP is complete when completion_count == required."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=2,
        )

        is_fully_complete = cp_task.completion_count >= cp_task.required_completions
        assert is_fully_complete

    def test_complete_above_threshold(self) -> None:
        """CP is complete when completion_count > required."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=3,  # More than required
        )

        is_fully_complete = cp_task.completion_count >= cp_task.required_completions
        assert is_fully_complete


class TestDifferentWorkerRequirement:
    """Tests for checkpoint_require_different_workers config."""

    def test_default_allows_same_worker(self) -> None:
        """Default config allows same worker to complete multiple times."""
        config = TaskSystemConfig()
        assert config.checkpoint_require_different_workers is False

    def test_require_different_workers(self) -> None:
        """Config can require different workers."""
        config = TaskSystemConfig(checkpoint_require_different_workers=True)
        assert config.checkpoint_require_different_workers is True

    def test_worker_already_completed_check(self) -> None:
        """Can check if worker already completed."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completed_by_sessions=["worker-1"],
        )

        # worker-1 already completed
        worker_already_completed = "worker-1" in cp_task.completed_by_sessions
        assert worker_already_completed

        # worker-2 has not completed
        worker_not_completed = "worker-2" not in cp_task.completed_by_sessions
        assert worker_not_completed


class TestCheckpointDeferral:
    """Tests for checkpoint deferral logic (defer if other tasks available)."""

    def test_cp_at_partial_completion_defers(self) -> None:
        """CP with completion_count < required-1 should defer to other tasks."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=0,  # No completions yet
        )

        # Should defer if other tasks available
        should_defer = (
            cp_task.type == TaskType.CHECKPOINT
            and cp_task.completion_count < cp_task.required_completions - 1
        )
        assert should_defer

    def test_cp_at_final_round_does_not_defer(self) -> None:
        """CP with completion_count == required-1 should not defer."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=1,  # One completion, needs one more
        )

        # Should not defer (final completion round)
        should_defer = (
            cp_task.type == TaskType.CHECKPOINT
            and cp_task.completion_count < cp_task.required_completions - 1
        )
        assert not should_defer


class TestTaskSystemConfigIntegration:
    """Tests for TaskSystemConfig integration with CP tasks."""

    def test_config_required_completions(self) -> None:
        """Config specifies checkpoint required completions."""
        config = TaskSystemConfig(checkpoint_required_completions=3)
        assert config.checkpoint_required_completions == 3

    def test_cp_uses_config_value(self) -> None:
        """CP task creation should use config value."""
        config = TaskSystemConfig(checkpoint_required_completions=3)

        # Simulate CP creation with config value
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=config.checkpoint_required_completions,
        )

        assert cp_task.required_completions == 3
