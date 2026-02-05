"""Integration tests for Review-as-Task workflow.

These tests verify the complete workflow:
1. Implementation task -> Review task generation
2. APPROVE flow: both tasks complete
3. REQUEST_CHANGES flow: creates new version
4. max_revision exceeded: BLOCKED and escalation
"""

import pytest

from c4.mcp_server import C4Daemon
from c4.models import ProjectStatus, TaskType
from tests.conftest import WORKER_1, WORKER_2


@pytest.fixture
def daemon(tmp_path):
    """Create initialized C4Daemon for testing."""
    daemon = C4Daemon(project_root=tmp_path)
    daemon.initialize(project_id="test-review")

    # Skip to PLAN for adding tasks
    daemon.state_machine._state.status = ProjectStatus.PLAN
    daemon.state_machine.save_state()

    # Clear default checkpoints for cleaner tests
    daemon.config.checkpoints = []

    return daemon


class TestApproveFlow:
    """Test APPROVE flow: T-001-0 -> R-001-0 -> APPROVE -> complete."""

    def test_implementation_generates_review_task(self, daemon):
        """Submitting implementation task generates review task."""
        # Add and start task
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Worker implements
        assignment = daemon.c4_get_task(worker_id=WORKER_1)
        assert assignment.task_id == "T-001-0"

        # Submit implementation
        result = daemon.c4_submit(
            task_id="T-001-0",
            commit_sha="abc123",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )
        assert result.success is True

        # Review task should be created
        review = daemon.get_task("R-001-0")
        assert review is not None
        assert review.type == TaskType.REVIEW
        assert review.parent_id == "T-001-0"
        assert review.completed_by == WORKER_1

    def test_approve_completes_workflow(self, daemon):
        """APPROVE completes both tasks."""
        # Setup
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Implement
        impl_task = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl_task.task_id,
            commit_sha="abc123",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Review (different worker for peer review)
        review_task = daemon.c4_get_task(worker_id=WORKER_2)
        assert review_task.task_id == "R-001-0"

        result = daemon.c4_submit(
            task_id="R-001-0",
            commit_sha="def456",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="APPROVE",
        )

        # Should complete
        assert result.success is True
        assert result.next_action == "complete"

        # Both tasks in done queue
        state = daemon.state_machine.state
        assert "T-001-0" in state.queue.done
        assert "R-001-0" in state.queue.done

    def test_approve_without_explicit_result(self, daemon):
        """APPROVE is inferred when no comments provided."""
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Implement
        impl = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Review without review_result or comments -> APPROVE
        review = daemon.c4_get_task(worker_id=WORKER_2)
        result = daemon.c4_submit(
            task_id=review.task_id,
            commit_sha="def",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            # No review_result, no review_comments
        )

        assert result.success is True
        assert result.next_action == "complete"


class TestRequestChangesFlow:
    """Test REQUEST_CHANGES flow: creates new version tasks."""

    def test_request_changes_creates_new_version(self, daemon):
        """REQUEST_CHANGES creates T-001-1."""
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Implement T-001-0
        impl = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Review R-001-0 with REQUEST_CHANGES
        review = daemon.c4_get_task(worker_id=WORKER_2)
        result = daemon.c4_submit(
            task_id=review.task_id,
            commit_sha="def",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="REQUEST_CHANGES",
            review_comments="Fix the typo in variable name",
        )

        assert result.success is True
        assert result.next_action == "get_next_task"

        # T-001-1 should be created
        fix_task = daemon.get_task("T-001-1")
        assert fix_task is not None
        assert fix_task.type == TaskType.IMPLEMENTATION
        assert fix_task.version == 1
        assert fix_task.dod == "Fix the typo in variable name"
        assert fix_task.parent_id == "R-001-0"

    def test_request_changes_requires_comments(self, daemon):
        """REQUEST_CHANGES without comments fails."""
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Implement
        impl = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Review with REQUEST_CHANGES but no comments
        review = daemon.c4_get_task(worker_id=WORKER_2)
        result = daemon.c4_submit(
            task_id=review.task_id,
            commit_sha="def",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="REQUEST_CHANGES",
            # No review_comments
        )

        assert result.success is False
        assert "requires review_comments" in result.message

    def test_full_revision_cycle(self, daemon):
        """Full cycle: T-001-0 -> R-001-0 -> T-001-1 -> R-001-1 -> APPROVE."""
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Round 1: Implement -> Review -> REQUEST_CHANGES
        impl0 = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl0.task_id,
            commit_sha="v0",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        review0 = daemon.c4_get_task(worker_id=WORKER_2)
        daemon.c4_submit(
            task_id=review0.task_id,
            commit_sha="r0",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="REQUEST_CHANGES",
            review_comments="Fix bug",
        )

        # Round 2: Fix -> Review -> APPROVE
        impl1 = daemon.c4_get_task(worker_id=WORKER_1)
        assert impl1.task_id == "T-001-1"
        daemon.c4_submit(
            task_id=impl1.task_id,
            commit_sha="v1",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        review1 = daemon.c4_get_task(worker_id=WORKER_2)
        assert review1.task_id == "R-001-1"
        result = daemon.c4_submit(
            task_id=review1.task_id,
            commit_sha="r1",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="APPROVE",
        )

        assert result.success is True
        assert result.next_action == "complete"


class TestMaxRevisionBlocked:
    """Test max_revision exceeded scenario."""

    def test_exceeds_max_revision_blocked(self, daemon):
        """Exceeding max_revision returns escalate."""
        daemon.config.max_revision = 2
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Cycle through versions until max_revision exceeded
        for version in range(3):  # 0, 1, 2 -> 3 would exceed
            impl = daemon.c4_get_task(worker_id=WORKER_1)
            if impl is None:
                break

            daemon.c4_submit(
                task_id=impl.task_id,
                commit_sha=f"v{version}",
                validation_results=[{"name": "lint", "status": "pass"}],
                worker_id=WORKER_1,
            )

            review = daemon.c4_get_task(worker_id=WORKER_2)
            if review is None:
                break

            result = daemon.c4_submit(
                task_id=review.task_id,
                commit_sha=f"r{version}",
                validation_results=[{"name": "lint", "status": "pass"}],
                worker_id=WORKER_2,
                review_result="REQUEST_CHANGES",
                review_comments=f"Issue {version + 1}",
            )

            if result.next_action == "escalate":
                # Verify escalation
                assert result.success is True
                assert "max_revision" in result.message

                # Check repair queue
                state = daemon.state_machine.state
                assert len(state.repair_queue) == 1
                assert "max_revision_exceeded" in state.repair_queue[0].failure_signature
                return

        pytest.fail("Should have escalated before completing all iterations")

    def test_blocked_adds_to_repair_queue(self, daemon):
        """BLOCKED task is added to repair queue with correct info."""
        daemon.config.max_revision = 1  # Very low for quick test
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        # Version 0
        impl0 = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl0.task_id,
            commit_sha="v0",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        review0 = daemon.c4_get_task(worker_id=WORKER_2)
        daemon.c4_submit(
            task_id=review0.task_id,
            commit_sha="r0",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="REQUEST_CHANGES",
            review_comments="Fix v0",
        )

        # Version 1
        impl1 = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl1.task_id,
            commit_sha="v1",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        review1 = daemon.c4_get_task(worker_id=WORKER_2)
        result = daemon.c4_submit(
            task_id=review1.task_id,
            commit_sha="r1",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_2,
            review_result="REQUEST_CHANGES",
            review_comments="Fix v1",
        )

        # Should escalate (version 2 would exceed max_revision=1)
        assert result.next_action == "escalate"

        # Check repair queue details
        state = daemon.state_machine.state
        repair = state.repair_queue[0]
        assert repair.task_id == "T-001-1"
        assert "max_revision_exceeded:1" in repair.failure_signature
        assert repair.blocked_at is not None


class TestReviewPriority:
    """Test review task priority is lower than implementation."""

    def test_review_has_lower_priority(self, daemon):
        """Review task has priority reduced by review_priority_offset."""
        daemon.config.review_priority_offset = 10
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl", priority=50)
        daemon.c4_start()

        # Implement
        impl = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Check review priority
        review = daemon.get_task("R-001-0")
        assert review.priority == 40  # 50 - 10

    def test_review_priority_not_negative(self, daemon):
        """Review priority doesn't go below 0."""
        daemon.config.review_priority_offset = 100
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl", priority=5)
        daemon.c4_start()

        impl = daemon.c4_get_task(worker_id=WORKER_1)
        daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        review = daemon.get_task("R-001-0")
        assert review.priority == 0  # max(0, 5 - 100)


class TestReviewDisabled:
    """Test when review_as_task is disabled."""

    def test_no_review_generated_when_disabled(self, daemon):
        """No review task when review_as_task=False."""
        daemon.config.review_as_task = False
        daemon.c4_add_todo(task_id="T-001", title="Feature", scope=None, dod="Impl")
        daemon.c4_start()

        impl = daemon.c4_get_task(worker_id=WORKER_1)
        result = daemon.c4_submit(
            task_id=impl.task_id,
            commit_sha="abc",
            validation_results=[{"name": "lint", "status": "pass"}],
            worker_id=WORKER_1,
        )

        # Should complete directly
        assert result.success is True
        assert result.next_action == "complete"

        # No review task
        review = daemon.get_task("R-001-0")
        assert review is None
