"""Tests for Team Supervisor."""

import pytest

from c4.supervisor.team_supervisor import (
    CheckpointType,
    ReviewDecision,
    ReviewRequest,
    ReviewResult,
    TeamSupervisor,
)


class TestReviewDecision:
    """Test ReviewDecision enum."""

    def test_values(self) -> None:
        """Test enum values."""
        assert ReviewDecision.APPROVED.value == "approved"
        assert ReviewDecision.REJECTED.value == "rejected"
        assert ReviewDecision.NEEDS_CHANGES.value == "needs_changes"


class TestCheckpointType:
    """Test CheckpointType enum."""

    def test_values(self) -> None:
        """Test enum values."""
        assert CheckpointType.TASK_COMPLETE.value == "task_complete"
        assert CheckpointType.PHASE_COMPLETE.value == "phase_complete"


class TestReviewRequest:
    """Test ReviewRequest dataclass."""

    def test_basic_request(self) -> None:
        """Test creating a review request."""
        request = ReviewRequest(
            id="req-001",
            task_id="T-001",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="abc123",
        )

        assert request.id == "req-001"
        assert request.pr_number is None

    def test_request_with_pr(self) -> None:
        """Test request with PR number."""
        request = ReviewRequest(
            id="req-002",
            task_id="T-002",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="def456",
            pr_number=42,
        )

        assert request.pr_number == 42


class TestReviewResult:
    """Test ReviewResult dataclass."""

    def test_approved_result(self) -> None:
        """Test approved review result."""
        result = ReviewResult(
            request_id="req-001",
            decision=ReviewDecision.APPROVED,
            reviewer_id="team-lead",
            comments="Looks good!",
        )

        assert result.decision == ReviewDecision.APPROVED
        assert result.suggested_changes == []

    def test_result_with_suggestions(self) -> None:
        """Test result with suggestions."""
        result = ReviewResult(
            request_id="req-001",
            decision=ReviewDecision.NEEDS_CHANGES,
            reviewer_id="team-lead",
            comments="Some changes needed",
            suggested_changes=["Fix bug", "Add test"],
        )

        assert len(result.suggested_changes) == 2


class TestTeamSupervisorInit:
    """Test TeamSupervisor initialization."""

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("TEAM_LEAD_API_KEY", "sk-test")
        monkeypatch.setenv("GITHUB_TOKEN", "ghp-test")

        supervisor = TeamSupervisor()

        assert supervisor._api_key == "sk-test"
        assert supervisor._github_token == "ghp-test"

    def test_init_with_params(self) -> None:
        """Test initialization with params."""
        supervisor = TeamSupervisor(
            team_lead_api_key="sk-param",
            github_token="ghp-param",
            llm_model="gpt-4",
        )

        assert supervisor._api_key == "sk-param"
        assert supervisor._llm_model == "gpt-4"

    def test_context_manager(self) -> None:
        """Test context manager."""
        with TeamSupervisor() as supervisor:
            assert supervisor is not None


class TestReviewQueue:
    """Test review queue operations."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor instance."""
        return TeamSupervisor()

    @pytest.fixture
    def review_req(self) -> ReviewRequest:
        """Create review request."""
        return ReviewRequest(
            id="req-001",
            task_id="T-001",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="abc123",
        )

    def test_submit_for_review(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test submitting for review."""
        request_id = supervisor.submit_for_review(review_req)

        assert request_id == "req-001"
        assert len(supervisor.get_pending_reviews()) == 1

    def test_get_pending_reviews(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test getting pending reviews."""
        supervisor.submit_for_review(review_req)

        pending = supervisor.get_pending_reviews()

        assert len(pending) == 1
        assert pending[0].task_id == "T-001"

    def test_get_review_request(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test getting specific request."""
        supervisor.submit_for_review(review_req)

        found = supervisor.get_review_request("req-001")

        assert found is not None
        assert found.id == "req-001"

    def test_get_nonexistent_request(
        self,
        supervisor: TeamSupervisor,
    ) -> None:
        """Test getting non-existent request."""
        found = supervisor.get_review_request("nonexistent")
        assert found is None


class TestReviewProcessing:
    """Test review processing."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor (no API key = mock responses)."""
        return TeamSupervisor()

    @pytest.fixture
    def review_req(self) -> ReviewRequest:
        """Create review request."""
        return ReviewRequest(
            id="req-001",
            task_id="T-001",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="abc123",
        )

    def test_review_task(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test reviewing a task (mock LLM)."""
        supervisor.submit_for_review(review_req)

        result = supervisor.review_task(
            request=review_req,
            task_description="Add feature X",
            changes_summary="Added new module",
        )

        assert result.decision == ReviewDecision.APPROVED
        assert result.reviewer_id == "team-lead"

    def test_review_removes_from_pending(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test that review removes from pending."""
        supervisor.submit_for_review(review_req)
        assert len(supervisor.get_pending_reviews()) == 1

        supervisor.review_task(review_req, "desc", "summary")

        assert len(supervisor.get_pending_reviews()) == 0


class TestResponseParsing:
    """Test LLM response parsing."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor."""
        return TeamSupervisor()

    def test_parse_approved(self, supervisor: TeamSupervisor) -> None:
        """Test parsing approved response."""
        response = """DECISION: APPROVED
COMMENTS: Code looks great!
SUGGESTIONS:
"""
        decision, comments, suggestions = supervisor._parse_review_response(response)

        assert decision == ReviewDecision.APPROVED
        assert "great" in comments
        assert len(suggestions) == 0

    def test_parse_needs_changes(self, supervisor: TeamSupervisor) -> None:
        """Test parsing needs_changes response."""
        response = """DECISION: NEEDS_CHANGES
COMMENTS: Some improvements needed
SUGGESTIONS:
- Add tests
- Fix formatting
"""
        decision, comments, suggestions = supervisor._parse_review_response(response)

        assert decision == ReviewDecision.NEEDS_CHANGES
        assert len(suggestions) == 2

    def test_parse_rejected(self, supervisor: TeamSupervisor) -> None:
        """Test parsing rejected response."""
        response = """DECISION: REJECTED
COMMENTS: Does not meet requirements
SUGGESTIONS:
- Start over
"""
        decision, comments, suggestions = supervisor._parse_review_response(response)

        assert decision == ReviewDecision.REJECTED


class TestCheckpointManagement:
    """Test checkpoint approval/rejection."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor."""
        return TeamSupervisor()

    @pytest.fixture
    def review_req(self) -> ReviewRequest:
        """Create review request."""
        return ReviewRequest(
            id="req-001",
            task_id="T-001",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="abc123",
        )

    def test_approve_checkpoint(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test approving a checkpoint."""
        supervisor.submit_for_review(review_req)

        result = supervisor.approve_checkpoint(
            review_req,
            comments="Good work!",
        )

        assert result.decision == ReviewDecision.APPROVED
        assert len(supervisor.get_pending_reviews()) == 0

    def test_reject_checkpoint(
        self,
        supervisor: TeamSupervisor,
        review_req: ReviewRequest,
    ) -> None:
        """Test rejecting a checkpoint."""
        supervisor.submit_for_review(review_req)

        result = supervisor.reject_checkpoint(
            review_req,
            reason="Missing tests",
            suggestions=["Add unit tests"],
        )

        assert result.decision == ReviewDecision.REJECTED
        assert len(result.suggested_changes) == 1


class TestGitHubIntegration:
    """Test GitHub integration."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor with mock GitHub token."""
        return TeamSupervisor(github_token="ghp-test")

    def test_format_pr_comment_approved(
        self,
        supervisor: TeamSupervisor,
    ) -> None:
        """Test formatting approved PR comment."""
        result = ReviewResult(
            request_id="req-001",
            decision=ReviewDecision.APPROVED,
            reviewer_id="team-lead",
            comments="Excellent work!",
        )

        comment = supervisor._format_pr_comment(result)

        assert "[APPROVED]" in comment
        assert "Excellent work!" in comment

    def test_format_pr_comment_with_suggestions(
        self,
        supervisor: TeamSupervisor,
    ) -> None:
        """Test formatting comment with suggestions."""
        result = ReviewResult(
            request_id="req-001",
            decision=ReviewDecision.NEEDS_CHANGES,
            reviewer_id="team-lead",
            comments="Some fixes needed",
            suggested_changes=["Fix A", "Fix B"],
        )

        comment = supervisor._format_pr_comment(result)

        assert "[NEEDS CHANGES]" in comment
        assert "Fix A" in comment
        assert "Fix B" in comment

    def test_post_pr_comment_no_token(self) -> None:
        """Test posting without token returns False."""
        supervisor = TeamSupervisor()  # No token

        result = ReviewResult(
            request_id="req-001",
            decision=ReviewDecision.APPROVED,
            reviewer_id="team-lead",
            comments="Good",
        )

        success = supervisor.post_pr_comment(result, "owner/repo", 1)
        assert success is False


class TestHistory:
    """Test review history."""

    @pytest.fixture
    def supervisor(self) -> TeamSupervisor:
        """Create supervisor."""
        return TeamSupervisor()

    def test_get_review_history(self, supervisor: TeamSupervisor) -> None:
        """Test getting review history."""
        request = ReviewRequest(
            id="req-001",
            task_id="T-001",
            worker_id="worker-1",
            checkpoint_type=CheckpointType.TASK_COMPLETE,
            commit_sha="abc123",
        )
        supervisor.submit_for_review(request)
        supervisor.approve_checkpoint(request)

        history = supervisor.get_review_history()

        assert len(history) == 1

    def test_get_stats(self, supervisor: TeamSupervisor) -> None:
        """Test getting review stats."""
        for i in range(3):
            request = ReviewRequest(
                id=f"req-{i}",
                task_id=f"T-{i}",
                worker_id="worker-1",
                checkpoint_type=CheckpointType.TASK_COMPLETE,
                commit_sha=f"sha{i}",
            )
            supervisor.submit_for_review(request)
            supervisor.approve_checkpoint(request)

        stats = supervisor.get_stats()

        assert stats["total_reviews"] == 3
        assert stats["by_decision"]["approved"] == 3
        assert stats["approval_rate"] == 1.0
