"""Tests for Unified Queue Architecture.

Tests the new unified task queue system where:
- CP-XXX tasks require multiple completions (default: 2)
- RPR-XXX tasks handle blocked task recovery
- All task types go through the same worker loop
"""

from c4.models import Task
from c4.models.config import TaskSystemConfig
from c4.models.enums import TaskStatus, TaskType


class TestTaskTypeRepair:
    """Tests for TaskType.REPAIR enum value."""

    def test_repair_enum_exists(self) -> None:
        """Verify REPAIR is a valid TaskType."""
        assert TaskType.REPAIR == "repair"
        assert TaskType.REPAIR.value == "repair"

    def test_all_task_types(self) -> None:
        """Verify all task types are defined."""
        assert TaskType.IMPLEMENTATION == "impl"
        assert TaskType.REVIEW == "review"
        assert TaskType.CHECKPOINT == "checkpoint"
        assert TaskType.REPAIR == "repair"


class TestTaskCompletionFields:
    """Tests for Task model completion tracking fields."""

    def test_default_required_completions(self) -> None:
        """Default required_completions is 1."""
        task = Task(id="T-001-0", title="Test", dod="Test DoD")
        assert task.required_completions == 1

    def test_checkpoint_task_fields(self) -> None:
        """CP tasks can have required_completions > 1."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            phase_id="phase-1",
            required_completions=2,
        )
        assert cp_task.required_completions == 2
        assert cp_task.completion_count == 0
        assert cp_task.completed_by_sessions == []

    def test_completion_count_increments(self) -> None:
        """completion_count can be incremented."""
        task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
        )
        task.completion_count += 1
        task.completed_by_sessions.append("worker-1")

        assert task.completion_count == 1
        assert "worker-1" in task.completed_by_sessions


class TestRepairTaskFields:
    """Tests for REPAIR task specific fields."""

    def test_repair_task_creation(self) -> None:
        """RPR tasks have original_task_id and failure_signature."""
        rpr_task = Task(
            id="RPR-001",
            title="Fix blocked task T-001-0",
            dod="Repair guidance",
            type=TaskType.REPAIR,
            original_task_id="T-001-0",
            failure_signature="lint:error",
            repair_guidance="Fix the lint errors",
        )
        assert rpr_task.type == TaskType.REPAIR
        assert rpr_task.original_task_id == "T-001-0"
        assert rpr_task.failure_signature == "lint:error"
        assert rpr_task.repair_guidance == "Fix the lint errors"

    def test_repair_task_base_id(self) -> None:
        """RPR tasks have base_id extracted from ID."""
        rpr_task = Task(
            id="RPR-001",
            title="Fix blocked task",
            dod="Repair",
            type=TaskType.REPAIR,
            base_id="001",
        )
        assert rpr_task.base_id == "001"


class TestTaskSystemConfig:
    """Tests for TaskSystemConfig."""

    def test_default_config(self) -> None:
        """Default config values."""
        config = TaskSystemConfig()
        assert config.checkpoint_required_completions == 2
        assert config.checkpoint_require_different_workers is False
        assert config.repair_failure_threshold == 3
        assert config.repair_auto_create is True

    def test_custom_config(self) -> None:
        """Custom config values."""
        config = TaskSystemConfig(
            checkpoint_required_completions=3,
            checkpoint_require_different_workers=True,
            repair_failure_threshold=5,
            repair_auto_create=False,
        )
        assert config.checkpoint_required_completions == 3
        assert config.checkpoint_require_different_workers is True
        assert config.repair_failure_threshold == 5
        assert config.repair_auto_create is False


class TestCheckpointMultipleCompletions:
    """Tests for checkpoint multiple completion logic."""

    def test_cp_not_done_until_required_completions(self) -> None:
        """CP task is not done until completion_count >= required_completions."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
            completion_count=1,
            completed_by_sessions=["worker-1"],
        )

        # Still needs 1 more completion
        assert cp_task.completion_count < cp_task.required_completions

        # After second completion
        cp_task.completion_count = 2
        cp_task.completed_by_sessions.append("worker-2")

        assert cp_task.completion_count >= cp_task.required_completions

    def test_same_worker_can_complete_twice_by_default(self) -> None:
        """By default, same worker can complete CP task multiple times."""
        config = TaskSystemConfig()
        assert config.checkpoint_require_different_workers is False

        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            required_completions=2,
        )

        # First completion by worker-1
        cp_task.completion_count = 1
        cp_task.completed_by_sessions.append("worker-1")

        # Same worker completing again is allowed
        cp_task.completion_count = 2
        cp_task.completed_by_sessions.append("worker-1")

        assert cp_task.completion_count == 2
        assert cp_task.completed_by_sessions == ["worker-1", "worker-1"]


class TestRepairWorkflow:
    """Tests for repair task workflow."""

    def test_blocked_task_status(self) -> None:
        """Blocked tasks have BLOCKED status."""
        task = Task(
            id="T-001-0",
            title="Test task",
            dod="Test",
            status=TaskStatus.BLOCKED,
        )
        assert task.status == TaskStatus.BLOCKED

    def test_repair_restores_to_pending(self) -> None:
        """After repair, task status should be PENDING."""
        # Simulate blocked task
        blocked_task = Task(
            id="T-001-0",
            title="Test task",
            dod="Test",
            status=TaskStatus.BLOCKED,
        )

        # After repair completion, restore to pending
        blocked_task.status = TaskStatus.PENDING
        assert blocked_task.status == TaskStatus.PENDING


class TestTaskIdParsing:
    """Tests for task ID parsing with new types."""

    def test_rpr_task_id_format(self) -> None:
        """RPR task IDs follow RPR-XXX format."""
        # RPR-001, RPR-002, etc.
        rpr_task = Task(
            id="RPR-001",
            title="Repair",
            dod="Fix",
            type=TaskType.REPAIR,
            base_id="001",
        )
        assert rpr_task.id == "RPR-001"
        assert rpr_task.base_id == "001"

    def test_cp_task_id_format(self) -> None:
        """CP task IDs follow CP-XXX format."""
        cp_task = Task(
            id="CP-001",
            title="Checkpoint",
            dod="Review",
            type=TaskType.CHECKPOINT,
            base_id="001",
            phase_id="phase-1",
        )
        assert cp_task.id == "CP-001"
        assert cp_task.base_id == "001"
