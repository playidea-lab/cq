"""Tests for Review-as-Task workflow functionality."""

import pytest

from c4.models import Task, TaskType
from c4.models.config import C4Config


class TestTaskType:
    """Test TaskType enum."""

    def test_implementation_value(self):
        """TaskType.IMPLEMENTATION has correct value."""
        assert TaskType.IMPLEMENTATION.value == "impl"

    def test_review_value(self):
        """TaskType.REVIEW has correct value."""
        assert TaskType.REVIEW.value == "review"

    def test_enum_members(self):
        """TaskType has exactly 3 members (impl, review, checkpoint)."""
        assert len(TaskType) == 3
        assert TaskType.IMPLEMENTATION in TaskType
        assert TaskType.REVIEW in TaskType
        assert TaskType.CHECKPOINT in TaskType


class TestTaskVersionFields:
    """Test Task model version-related fields."""

    def test_default_type_is_implementation(self):
        """Default task type is IMPLEMENTATION."""
        task = Task(id="T-001-0", title="Test", dod="Test DoD")
        assert task.type == TaskType.IMPLEMENTATION

    def test_default_version_is_zero(self):
        """Default version is 0."""
        task = Task(id="T-001-0", title="Test", dod="Test DoD")
        assert task.version == 0

    def test_base_id_can_be_set(self):
        """base_id can be set."""
        task = Task(id="T-001-0", title="Test", dod="Test DoD", base_id="001")
        assert task.base_id == "001"

    def test_parent_id_can_be_set(self):
        """parent_id can be set for review tasks."""
        task = Task(
            id="R-001-0",
            title="Review: Test",
            dod="Review DoD",
            type=TaskType.REVIEW,
            parent_id="T-001-0",
        )
        assert task.parent_id == "T-001-0"

    def test_completed_by_can_be_set(self):
        """completed_by can be set for review tasks."""
        task = Task(
            id="R-001-0",
            title="Review: Test",
            dod="Review DoD",
            type=TaskType.REVIEW,
            completed_by="worker-1",
        )
        assert task.completed_by == "worker-1"

    def test_review_comments_can_be_set(self):
        """review_comments can be set."""
        task = Task(
            id="T-001-1",
            title="Fix: Test",
            dod="Fix DoD",
            review_comments="Original issue comment",
        )
        assert task.review_comments == "Original issue comment"


class TestC4ConfigReviewOptions:
    """Test C4Config review-as-task options."""

    def test_review_as_task_default_true(self):
        """review_as_task defaults to True."""
        config = C4Config(project_id="test")
        assert config.review_as_task is True

    def test_max_revision_default(self):
        """max_revision defaults to 3."""
        config = C4Config(project_id="test")
        assert config.max_revision == 3

    def test_max_revision_bounds(self):
        """max_revision has valid bounds (1-10)."""
        config = C4Config(project_id="test", max_revision=1)
        assert config.max_revision == 1

        config = C4Config(project_id="test", max_revision=10)
        assert config.max_revision == 10

    def test_review_priority_offset_default(self):
        """review_priority_offset defaults to 10."""
        config = C4Config(project_id="test")
        assert config.review_priority_offset == 10

    def test_review_as_task_can_be_disabled(self):
        """review_as_task can be set to False."""
        config = C4Config(project_id="test", review_as_task=False)
        assert config.review_as_task is False


class TestTaskIdParsing:
    """Test _parse_task_id helper function."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create C4Daemon for testing."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon(project_root=tmp_path)
        return daemon

    def test_parse_implementation_with_version(self, daemon):
        """Parse T-001-0 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-001-0")
        assert normalized == "T-001-0"
        assert base_id == "001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_implementation_without_version(self, daemon):
        """Parse T-001 -> T-001-0 (auto-append)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-001")
        assert normalized == "T-001-0"
        assert base_id == "001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_review_task(self, daemon):
        """Parse R-001-0 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("R-001-0")
        assert normalized == "R-001-0"
        assert base_id == "001"
        assert version == 0
        assert task_type == TaskType.REVIEW

    def test_parse_higher_version(self, daemon):
        """Parse T-001-2 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-001-2")
        assert normalized == "T-001-2"
        assert base_id == "001"
        assert version == 2
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_larger_base_id(self, daemon):
        """Parse T-123-5 correctly."""
        normalized, base_id, version, task_type = daemon._parse_task_id("T-123-5")
        assert normalized == "T-123-5"
        assert base_id == "123"
        assert version == 5
        assert task_type == TaskType.IMPLEMENTATION

    def test_parse_unknown_prefix(self, daemon):
        """Parse X-001-0 as implementation (fallback)."""
        normalized, base_id, version, task_type = daemon._parse_task_id("X-001-0")
        assert normalized == "X-001-0"
        assert base_id == "001"
        assert version == 0
        assert task_type == TaskType.IMPLEMENTATION


class TestAddTodoNormalization:
    """Test c4_add_todo normalizes task IDs."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create initialized C4Daemon for testing."""
        from c4.mcp_server import C4Daemon
        from c4.models import ProjectStatus

        daemon = C4Daemon(project_root=tmp_path)
        daemon.initialize(project_id="test")
        # Skip to PLAN for adding tasks
        daemon.state_machine._state.status = ProjectStatus.PLAN
        daemon.state_machine.save_state()
        return daemon

    def test_add_todo_normalizes_id(self, daemon):
        """T-001 becomes T-001-0."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Test DoD",
        )
        assert result["success"] is True
        assert result["task_id"] == "T-001-0"

    def test_add_todo_keeps_versioned_id(self, daemon):
        """T-001-0 stays T-001-0."""
        result = daemon.c4_add_todo(
            task_id="T-001-0",
            title="Test task",
            scope=None,
            dod="Test DoD",
        )
        assert result["success"] is True
        assert result["task_id"] == "T-001-0"

    def test_add_todo_sets_base_id(self, daemon):
        """base_id is extracted from task ID."""
        result = daemon.c4_add_todo(
            task_id="T-042",
            title="Test task",
            scope=None,
            dod="Test DoD",
        )
        task = daemon.get_task(result["task_id"])
        assert task.base_id == "042"

    def test_add_todo_sets_version(self, daemon):
        """version is extracted from task ID."""
        result = daemon.c4_add_todo(
            task_id="T-001-3",
            title="Test task",
            scope=None,
            dod="Test DoD",
        )
        task = daemon.get_task(result["task_id"])
        assert task.version == 3

    def test_add_todo_sets_task_type(self, daemon):
        """task type is determined from prefix."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="Test DoD",
        )
        task = daemon.get_task(result["task_id"])
        assert task.type == TaskType.IMPLEMENTATION


class TestReviewTaskRouting:
    """Test review tasks are routed to code-reviewer agent."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create initialized C4Daemon for testing."""
        from c4.mcp_server import C4Daemon
        from c4.models import ProjectStatus

        daemon = C4Daemon(project_root=tmp_path)
        daemon.initialize(project_id="test")
        # Set to EXECUTE state for adding/generating tasks
        daemon.state_machine._state.status = ProjectStatus.EXECUTE
        daemon.state_machine.save_state()
        return daemon

    def test_review_task_has_task_type_review(self, daemon):
        """Review tasks should have task_type='review' for skill matching."""
        from c4.models import Task

        impl_task = Task(
            id="T-001-0",
            title="Test task",
            dod="Test DoD",
            base_id="001",
            version=0,
        )
        # Manually call _generate_review_task
        daemon._generate_review_task(impl_task, "worker-1")

        # Verify review task was created with task_type
        review_task = daemon.get_task("R-001-0")
        assert review_task is not None
        assert review_task.task_type == "review"
        assert review_task.type == TaskType.REVIEW

    def test_review_task_routed_to_code_reviewer(self, daemon):
        """Review tasks should be routed to code-reviewer agent."""
        from c4.models import Task

        # Create a review task with task_type
        review_task = Task(
            id="R-001-0",
            title="Review: Test implementation",
            dod="Review implementation of T-001-0.",
            domain="web-backend",
            task_type="review",
            type=TaskType.REVIEW,
        )

        # Get agent routing
        routing = daemon._get_agent_routing(review_task)

        # Verify code-reviewer is the primary agent
        assert routing["recommended_agent"] == "code-reviewer"
