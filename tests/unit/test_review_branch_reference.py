"""Test that review tasks use parent task's branch."""

import tempfile
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import TaskType


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon_in_execute(temp_project):
    """Create a daemon in EXECUTE state with tasks."""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    # Skip to PLAN, then transition to EXECUTE
    d.state_machine.transition("skip_discovery", "test")
    d.state_machine.transition("c4_run", "test")
    return d


class TestReviewTaskBranchReference:
    """Test that review tasks use parent implementation task's branch."""

    def test_review_task_uses_parent_branch(self, daemon_in_execute):
        """Review task should use parent implementation task's branch, not create new one."""
        daemon = daemon_in_execute

        # Add an implementation task
        daemon.c4_add_todo(
            task_id="T-001",
            title="Test implementation",
            scope=None,
            dod="Create a test file",
        )

        # Assign implementation task to worker
        assignment1 = daemon.c4_get_task(worker_id="impl-worker")
        assert assignment1 is not None
        assert assignment1.task_id == "T-001-0"
        impl_branch = assignment1.branch
        assert "T-001-0" in impl_branch

        # Complete the implementation task (this creates a review task)
        daemon.c4_submit(
            task_id="T-001-0",
            commit_sha="abc123",
            validation_results=[],
            worker_id="impl-worker",
        )

        # Get the review task
        assignment2 = daemon.c4_get_task(worker_id="review-worker")
        assert assignment2 is not None
        assert assignment2.task_id == "R-001-0"

        # Review task should use the SAME branch as implementation task
        assert assignment2.branch == impl_branch, (
            f"Review task should use parent branch '{impl_branch}', "
            f"but got '{assignment2.branch}'"
        )

    def test_implementation_task_creates_own_branch(self, daemon_in_execute):
        """Implementation task should create its own branch."""
        daemon = daemon_in_execute

        daemon.c4_add_todo(
            task_id="T-002",
            title="Another implementation",
            scope=None,
            dod="Create another file",
        )

        assignment = daemon.c4_get_task(worker_id="worker-1")
        assert assignment is not None
        assert "T-002-0" in assignment.branch
        # Should not use parent branch (it's an implementation task)
        assert "R-" not in assignment.branch

    def test_review_task_parent_id_is_set(self, daemon_in_execute):
        """Review task should have parent_id set to implementation task."""
        daemon = daemon_in_execute

        daemon.c4_add_todo(
            task_id="T-003",
            title="Test impl",
            scope=None,
            dod="Test",
        )

        # Complete implementation
        assignment = daemon.c4_get_task(worker_id="worker-1")
        daemon.c4_submit(
            task_id=assignment.task_id,
            commit_sha="def456",
            validation_results=[],
            worker_id="worker-1",
        )

        # Get review task
        review_task = daemon.get_task("R-003-0")
        assert review_task is not None
        assert review_task.parent_id == "T-003-0"
        assert review_task.type == TaskType.REVIEW

    def test_fallback_when_parent_branch_not_found(self, daemon_in_execute):
        """If parent task has no branch, review should compute branch from parent_id."""
        daemon = daemon_in_execute

        # Create implementation task
        daemon.c4_add_todo(
            task_id="T-004",
            title="Test impl",
            scope=None,
            dod="Test",
        )

        # Get and complete implementation
        assignment = daemon.c4_get_task(worker_id="worker-1")
        impl_task_id = assignment.task_id

        # Clear the parent task's branch to simulate edge case
        parent_task = daemon.get_task(impl_task_id)
        parent_task.branch = None
        daemon._save_task(parent_task)

        # Complete and create review
        daemon.c4_submit(
            task_id=impl_task_id,
            commit_sha="ghi789",
            validation_results=[],
            worker_id="worker-1",
        )

        # Review task should fall back to computing branch from parent_id
        review_assignment = daemon.c4_get_task(worker_id="review-worker")
        assert review_assignment is not None
        # Should compute from parent_id (T-004-0)
        assert "T-004-0" in review_assignment.branch
