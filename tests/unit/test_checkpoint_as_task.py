"""Tests for Checkpoint-as-Task functionality.

Checkpoint-as-Task:
- Checkpoint tasks (CP-XXX) are created when all phase reviews are approved
- CP tasks go through the same queue as other tasks
- Any worker can handle CP tasks
- Decisions: APPROVE, REQUEST_CHANGES, REPLAN
"""

from unittest.mock import MagicMock, patch

import pytest

from c4.models import (
    C4Config,
    CheckpointConfig,
    Task,
    TaskType,
    ValidationConfig,
)


class TestParsePromblemTaskIds:
    """Test _parse_problem_task_ids helper method."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create a minimal daemon for testing."""
        from c4.mcp_server import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            # Import the method
            from c4.mcp_server import C4Daemon as RealDaemon

            d._parse_problem_task_ids = RealDaemon._parse_problem_task_ids.__get__(d)
        return d

    def test_parse_single_task(self, daemon):
        """Parse single task ID from comments."""
        comments = "T-001 needs to be fixed"
        result = daemon._parse_problem_task_ids(comments)
        assert "T-001" in result

    def test_parse_multiple_tasks(self, daemon):
        """Parse multiple task IDs."""
        comments = "T-001, T-003: both have issues"
        result = daemon._parse_problem_task_ids(comments)
        assert "T-001" in result
        assert "T-003" in result

    def test_parse_versioned_tasks(self, daemon):
        """Parse versioned task IDs (T-XXX-N format)."""
        comments = "Fix T-001-0 and T-003-2"
        result = daemon._parse_problem_task_ids(comments)
        assert "T-001-0" in result
        assert "T-003-2" in result

    def test_parse_mixed_format(self, daemon):
        """Parse mixed task ID formats."""
        comments = "Issues with T-001 and T-002-1"
        result = daemon._parse_problem_task_ids(comments)
        assert "T-001" in result
        assert "T-002-1" in result

    def test_parse_no_tasks(self, daemon):
        """Return empty list if no task IDs found."""
        comments = "General issues with the implementation"
        result = daemon._parse_problem_task_ids(comments)
        assert result == []

    def test_parse_dedupe(self, daemon):
        """Deduplicate repeated task IDs."""
        comments = "T-001 is broken. Also T-001 needs more work"
        result = daemon._parse_problem_task_ids(comments)
        assert result.count("T-001") == 1


class TestBuildCheckpointDod:
    """Test _build_checkpoint_dod helper method."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create a minimal daemon for testing."""
        from c4.mcp_server import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d._config = C4Config(
                project_id="test",
                validation=ValidationConfig(
                    commands={"lint": "npm run lint", "unit": "npm test"},
                    required=["lint", "unit"],
                ),
            )
            from c4.mcp_server import C4Daemon as RealDaemon

            d._build_checkpoint_dod = RealDaemon._build_checkpoint_dod.__get__(d)
        return d

    def test_basic_dod(self, daemon):
        """Build basic DoD from checkpoint config."""
        cp_config = CheckpointConfig(
            id="CP-001",
            description="Phase 1 Complete",
            required_tasks=["T-001", "T-002"],
            required_validations=["lint", "unit"],
        )

        dod = daemon._build_checkpoint_dod(cp_config)

        assert "Checkpoint: Phase 1 Complete" in dod
        assert "lint: pass" in dod
        assert "unit: pass" in dod
        assert "APPROVE" in dod
        assert "REQUEST_CHANGES" in dod
        assert "REPLAN" in dod

    def test_dod_with_no_validations(self, daemon):
        """Build DoD with no required validations."""
        cp_config = CheckpointConfig(
            id="CP-001",
            required_tasks=["T-001"],
            required_validations=[],
        )

        dod = daemon._build_checkpoint_dod(cp_config)

        assert "Required Validations" in dod


class TestFindTasksByPattern:
    """Test _find_tasks_by_pattern helper method."""

    @pytest.fixture
    def daemon_with_tasks(self, tmp_path):
        """Create daemon with some tasks."""
        from c4.mcp_server import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                ),
                "T-001-1": Task(
                    id="T-001-1",
                    title="Task 1 v1",
                    dod="...",
                    base_id="001",
                    version=1,
                    type=TaskType.IMPLEMENTATION,
                ),
                "T-002-0": Task(
                    id="T-002-0",
                    title="Task 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                ),
                "R-001-0": Task(
                    id="R-001-0",
                    title="Review 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.REVIEW,
                ),
            }
            d.get_all_tasks = MagicMock(return_value=d._tasks)

            from c4.mcp_server import C4Daemon as RealDaemon

            d._find_tasks_by_pattern = RealDaemon._find_tasks_by_pattern.__get__(d)
        return d

    def test_exact_match(self, daemon_with_tasks):
        """Find task by exact ID."""
        result = daemon_with_tasks._find_tasks_by_pattern("T-001-0")
        assert len(result) == 1
        assert result[0].id == "T-001-0"

    def test_base_id_pattern(self, daemon_with_tasks):
        """Find all versions by base ID pattern."""
        result = daemon_with_tasks._find_tasks_by_pattern("T-001", TaskType.IMPLEMENTATION)
        assert len(result) == 2
        ids = [t.id for t in result]
        assert "T-001-0" in ids
        assert "T-001-1" in ids

    def test_filter_by_type(self, daemon_with_tasks):
        """Filter by task type."""
        # Only reviews
        result = daemon_with_tasks._find_tasks_by_pattern("R-001-0", TaskType.REVIEW)
        assert len(result) == 1
        assert result[0].type == TaskType.REVIEW

        # Only impl, won't match R-001-0
        result = daemon_with_tasks._find_tasks_by_pattern("R-001-0", TaskType.IMPLEMENTATION)
        assert len(result) == 0

    def test_no_match(self, daemon_with_tasks):
        """Return empty list if no match."""
        result = daemon_with_tasks._find_tasks_by_pattern("T-999")
        assert result == []


class TestGetLatestReviewForImpl:
    """Test _get_latest_review_for_impl helper method."""

    @pytest.fixture
    def daemon_with_reviews(self, tmp_path):
        """Create daemon with review tasks."""
        from c4.mcp_server import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                ),
                "R-001-0": Task(
                    id="R-001-0",
                    title="Review 1 v0",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.REVIEW,
                ),
                "R-001-1": Task(
                    id="R-001-1",
                    title="Review 1 v1",
                    dod="...",
                    base_id="001",
                    version=1,
                    type=TaskType.REVIEW,
                ),
                "R-002-0": Task(
                    id="R-002-0",
                    title="Review 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.REVIEW,
                ),
            }
            d.get_all_tasks = MagicMock(return_value=d._tasks)

            from c4.mcp_server import C4Daemon as RealDaemon

            d._get_latest_review_for_impl = RealDaemon._get_latest_review_for_impl.__get__(d)
        return d

    def test_get_latest_review(self, daemon_with_reviews):
        """Get latest version review for impl task."""
        result = daemon_with_reviews._get_latest_review_for_impl("001")
        assert result is not None
        assert result.id == "R-001-1"
        assert result.version == 1

    def test_get_single_review(self, daemon_with_reviews):
        """Get review when only one version exists."""
        result = daemon_with_reviews._get_latest_review_for_impl("002")
        assert result is not None
        assert result.id == "R-002-0"

    def test_no_review_found(self, daemon_with_reviews):
        """Return None if no review exists."""
        result = daemon_with_reviews._get_latest_review_for_impl("999")
        assert result is None


class TestTaskExists:
    """Test _task_exists helper method."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create daemon with some tasks."""
        from c4.mcp_server import C4Daemon

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path
            d._tasks = {
                "T-001-0": Task(id="T-001-0", title="Task 1", dod="..."),
            }
            d.get_all_tasks = MagicMock(return_value=d._tasks)

            from c4.mcp_server import C4Daemon as RealDaemon

            d._task_exists = RealDaemon._task_exists.__get__(d)
        return d

    def test_task_exists_in_cache(self, daemon):
        """Check if task exists in cache."""
        assert daemon._task_exists("T-001-0") is True

    def test_task_not_exists(self, daemon):
        """Check if task doesn't exist."""
        assert daemon._task_exists("T-999-0") is False


class TestCheckpointTaskCreation:
    """Integration tests for checkpoint task creation flow."""

    @pytest.fixture
    def daemon_for_checkpoint(self, tmp_path):
        """Create daemon with full setup for checkpoint testing."""
        from c4.mcp_server import C4Daemon
        from c4.models import C4State, TaskQueue

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            # Config with checkpoint
            d._config = C4Config(
                project_id="test",
                checkpoints=[
                    CheckpointConfig(
                        id="001",
                        description="Phase 1",
                        required_tasks=["T-001", "T-002"],
                        required_validations=["lint", "unit"],
                    )
                ],
                checkpoint_as_task=True,
                checkpoint_priority_offset=20,
            )

            # Tasks
            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                    priority=100,
                ),
                "T-002-0": Task(
                    id="T-002-0",
                    title="Task 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                    priority=100,
                ),
                "R-001-0": Task(
                    id="R-001-0",
                    title="Review 1",
                    dod="...",
                    base_id="001",
                    version=0,
                    type=TaskType.REVIEW,
                    parent_id="T-001-0",
                    review_decision="APPROVE",
                ),
                "R-002-0": Task(
                    id="R-002-0",
                    title="Review 2",
                    dod="...",
                    base_id="002",
                    version=0,
                    type=TaskType.REVIEW,
                    parent_id="T-002-0",
                    review_decision="APPROVE",
                ),
            }

            # State machine mock
            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(
                    pending=[],
                    in_progress={},
                    done=["T-001-0", "T-002-0", "R-001-0", "R-002-0"],
                ),
                passed_checkpoints=[],
            )

            d.get_all_tasks = MagicMock(return_value=d._tasks)
            d.get_task = MagicMock(side_effect=lambda tid: d._tasks.get(tid))

            # Bind methods
            from c4.mcp_server import C4Daemon as RealDaemon

            d._check_and_create_checkpoint_task = (
                RealDaemon._check_and_create_checkpoint_task.__get__(d)
            )
            d._find_tasks_by_pattern = RealDaemon._find_tasks_by_pattern.__get__(d)
            d._get_latest_review_for_impl = RealDaemon._get_latest_review_for_impl.__get__(d)
            d._build_checkpoint_dod = RealDaemon._build_checkpoint_dod.__get__(d)
            d._task_exists = RealDaemon._task_exists.__get__(d)
            d.add_task = MagicMock()

        return d

    def test_create_checkpoint_when_all_reviews_approved(self, daemon_for_checkpoint):
        """Create CP task when all reviews in phase are approved."""
        completed_review = daemon_for_checkpoint._tasks["R-002-0"]

        result = daemon_for_checkpoint._check_and_create_checkpoint_task(completed_review)

        assert result is not None
        assert result.id == "CP-001"
        assert result.type == TaskType.CHECKPOINT
        assert result.phase_id == "001"
        assert "T-001-0" in result.required_tasks
        assert "T-002-0" in result.required_tasks
        daemon_for_checkpoint.add_task.assert_called_once()

    def test_no_checkpoint_if_review_not_approved(self, daemon_for_checkpoint):
        """Don't create CP if not all reviews are approved."""
        # Change one review to not approved
        daemon_for_checkpoint._tasks["R-002-0"].review_decision = "REQUEST_CHANGES"

        completed_review = daemon_for_checkpoint._tasks["R-001-0"]
        result = daemon_for_checkpoint._check_and_create_checkpoint_task(completed_review)

        assert result is None
        daemon_for_checkpoint.add_task.assert_not_called()

    def test_no_checkpoint_if_review_not_done(self, daemon_for_checkpoint):
        """Don't create CP if review is not in done queue."""
        # Remove R-002-0 from done
        daemon_for_checkpoint.state_machine.state.queue.done.remove("R-002-0")

        completed_review = daemon_for_checkpoint._tasks["R-001-0"]
        result = daemon_for_checkpoint._check_and_create_checkpoint_task(completed_review)

        assert result is None

    def test_no_duplicate_checkpoint(self, daemon_for_checkpoint):
        """Don't create duplicate CP task."""
        # Add existing CP task
        daemon_for_checkpoint._tasks["CP-001"] = Task(
            id="CP-001", title="Checkpoint", dod="...", type=TaskType.CHECKPOINT
        )

        completed_review = daemon_for_checkpoint._tasks["R-002-0"]
        result = daemon_for_checkpoint._check_and_create_checkpoint_task(completed_review)

        assert result is None

    def test_no_checkpoint_if_already_passed(self, daemon_for_checkpoint):
        """Don't create CP if checkpoint already passed."""
        daemon_for_checkpoint.state_machine.state.passed_checkpoints.append("001")

        completed_review = daemon_for_checkpoint._tasks["R-002-0"]
        result = daemon_for_checkpoint._check_and_create_checkpoint_task(completed_review)

        assert result is None


class TestCheckpointCompletion:
    """Test checkpoint completion handling (APPROVE, REQUEST_CHANGES, REPLAN)."""

    @pytest.fixture
    def daemon_for_completion(self, tmp_path):
        """Create daemon for checkpoint completion testing."""
        from c4.mcp_server import C4Daemon
        from c4.models import C4State, TaskQueue

        with patch.object(C4Daemon, "__init__", lambda self, root: None):
            d = C4Daemon.__new__(C4Daemon)
            d.root = tmp_path

            d._config = C4Config(
                project_id="test",
                checkpoints=[
                    CheckpointConfig(
                        id="001",
                        description="Phase 1",
                        required_tasks=["T-001"],
                        required_validations=["lint"],
                    )
                ],
                max_revision=3,
            )

            d._tasks = {
                "T-001-0": Task(
                    id="T-001-0",
                    title="Task 1",
                    dod="original",
                    base_id="001",
                    version=0,
                    type=TaskType.IMPLEMENTATION,
                    priority=100,
                ),
                "CP-001": Task(
                    id="CP-001",
                    title="Checkpoint 1",
                    dod="...",
                    type=TaskType.CHECKPOINT,
                    phase_id="001",
                    required_tasks=["T-001-0"],
                ),
            }

            d.state_machine = MagicMock()
            d.state_machine.state = C4State(
                project_id="test",
                queue=TaskQueue(pending=[], in_progress={}, done=[]),
                passed_checkpoints=[],
                repair_queue=[],
            )

            d._task_store = MagicMock()
            d.get_task = MagicMock(side_effect=lambda tid: d._tasks.get(tid))
            d.add_task = MagicMock(side_effect=lambda t: d._tasks.update({t.id: t}))

            from c4.mcp_server import C4Daemon as RealDaemon

            d._handle_checkpoint_completion = RealDaemon._handle_checkpoint_completion.__get__(d)
            d._parse_problem_task_ids = RealDaemon._parse_problem_task_ids.__get__(d)
            d._perform_completion_action = MagicMock()

        return d

    def test_approve_marks_checkpoint_passed(self, daemon_for_completion):
        """APPROVE marks checkpoint as passed."""
        cp_task = daemon_for_completion._tasks["CP-001"]

        result = daemon_for_completion._handle_checkpoint_completion(
            cp_task, "APPROVE", None, "worker-1"
        )

        assert result is None  # Continue normal flow
        assert "001" in daemon_for_completion.state_machine.state.passed_checkpoints
        daemon_for_completion._task_store.update_review_decision.assert_called_once_with(
            "test", "CP-001", "APPROVE"
        )

    def test_request_changes_creates_fix_task(self, daemon_for_completion):
        """REQUEST_CHANGES creates new version of problem task."""
        cp_task = daemon_for_completion._tasks["CP-001"]

        result = daemon_for_completion._handle_checkpoint_completion(
            cp_task, "REQUEST_CHANGES", "T-001-0: Need to fix validation", "worker-1"
        )

        assert result is not None
        assert result.success is True
        assert "T-001-1" in result.message

        # Check new task was created
        new_task = daemon_for_completion._tasks.get("T-001-1")
        assert new_task is not None
        assert new_task.type == TaskType.IMPLEMENTATION
        assert new_task.version == 1
        assert new_task.base_id == "001"
        assert "T-001-0: Need to fix validation" in new_task.dod

    def test_request_changes_requires_comments(self, daemon_for_completion):
        """REQUEST_CHANGES requires comments."""
        cp_task = daemon_for_completion._tasks["CP-001"]

        result = daemon_for_completion._handle_checkpoint_completion(
            cp_task, "REQUEST_CHANGES", None, "worker-1"
        )

        assert result is not None
        assert result.success is False
        assert "requires review_comments" in result.message

    def test_request_changes_respects_max_revision(self, daemon_for_completion):
        """REQUEST_CHANGES respects max_revision limit."""
        # Create task at max version
        daemon_for_completion._tasks["T-001-3"] = Task(
            id="T-001-3",
            title="Task 1 v3",
            dod="...",
            base_id="001",
            version=3,
            type=TaskType.IMPLEMENTATION,
        )
        daemon_for_completion._tasks["CP-001"].required_tasks = ["T-001-3"]

        cp_task = daemon_for_completion._tasks["CP-001"]

        daemon_for_completion._handle_checkpoint_completion(
            cp_task, "REQUEST_CHANGES", "T-001-3: Still broken", "worker-1"
        )

        # Should add to repair queue instead of creating new task
        assert len(daemon_for_completion.state_machine.state.repair_queue) == 1
        repair_item = daemon_for_completion.state_machine.state.repair_queue[0]
        assert repair_item.task_id == "T-001-3"

    def test_replan_adds_to_repair_queue(self, daemon_for_completion):
        """REPLAN adds checkpoint to repair queue."""
        cp_task = daemon_for_completion._tasks["CP-001"]

        result = daemon_for_completion._handle_checkpoint_completion(
            cp_task, "REPLAN", "Architecture needs rethinking", "worker-1"
        )

        assert result is not None
        assert result.success is True
        assert "escalate" in result.next_action

        assert len(daemon_for_completion.state_machine.state.repair_queue) == 1
        repair_item = daemon_for_completion.state_machine.state.repair_queue[0]
        assert repair_item.task_id == "CP-001"
        assert "checkpoint_replan" in repair_item.failure_signature

    def test_invalid_review_result(self, daemon_for_completion):
        """Invalid review result returns error."""
        cp_task = daemon_for_completion._tasks["CP-001"]

        result = daemon_for_completion._handle_checkpoint_completion(
            cp_task, "INVALID", None, "worker-1"
        )

        assert result is not None
        assert result.success is False
        assert "Invalid review_result" in result.message
