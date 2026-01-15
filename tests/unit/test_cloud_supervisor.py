"""Tests for Cloud Supervisor."""

from datetime import datetime
from unittest.mock import MagicMock, patch

import pytest

from c4.supervisor.cloud_supervisor import (
    CloudSupervisor,
    ReviewRequest,
    ReviewResult,
    ReviewStatus,
    ReviewType,
)


class TestReviewModels:
    """Test review data models."""

    def test_review_type_values(self) -> None:
        """Test ReviewType enum values."""
        assert ReviewType.CHECKPOINT.value == "checkpoint"
        assert ReviewType.REPAIR.value == "repair"
        assert ReviewType.PR.value == "pull_request"

    def test_review_status_values(self) -> None:
        """Test ReviewStatus enum values."""
        assert ReviewStatus.PENDING.value == "pending"
        assert ReviewStatus.APPROVED.value == "approved"
        assert ReviewStatus.REJECTED.value == "rejected"

    def test_review_request_defaults(self) -> None:
        """Test ReviewRequest default values."""
        request = ReviewRequest(
            id="REV-001",
            project_id="test",
            team_id="team-1",
            review_type=ReviewType.CHECKPOINT,
        )

        assert request.checkpoint_id is None
        assert request.task_id is None
        assert request.pr_number is None
        assert request.metadata == {}
        assert isinstance(request.created_at, datetime)

    def test_review_result(self) -> None:
        """Test ReviewResult structure."""
        result = ReviewResult(
            request_id="REV-001",
            status=ReviewStatus.APPROVED,
            decision="All tests pass",
            notes="Good work!",
            reviewer_id="user-123",
            reviewed_at=datetime.now(),
        )

        assert result.request_id == "REV-001"
        assert result.status == ReviewStatus.APPROVED
        assert result.required_changes is None


class TestCloudSupervisorInit:
    """Test CloudSupervisor initialization."""

    def test_init_with_store(self) -> None:
        """Test initialization with store."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(store=mock_store)

        assert supervisor._store == mock_store
        assert supervisor._github_token is None

    def test_init_with_github_token(self) -> None:
        """Test initialization with GitHub token."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(
            store=mock_store,
            github_token="ghp_test123",
        )

        assert supervisor._github_token == "ghp_test123"

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test GitHub token from environment."""
        monkeypatch.setenv("GITHUB_TOKEN", "ghp_env_token")
        mock_store = MagicMock()

        supervisor = CloudSupervisor(store=mock_store)

        assert supervisor._github_token == "ghp_env_token"


class TestReviewSubmission:
    """Test review submission methods."""

    @pytest.fixture
    def supervisor(self) -> CloudSupervisor:
        """Create supervisor with mock store."""
        mock_store = MagicMock()
        mock_store.client.table.return_value.insert.return_value.execute.return_value = None
        return CloudSupervisor(store=mock_store)

    def test_submit_checkpoint_review(self, supervisor: CloudSupervisor) -> None:
        """Test submitting checkpoint for review."""
        request = supervisor.submit_checkpoint_review(
            project_id="test-project",
            team_id="team-123",
            checkpoint_id="CP-001",
            bundle_path="/path/to/bundle",
        )

        assert request.id.startswith("REV-")
        assert request.project_id == "test-project"
        assert request.team_id == "team-123"
        assert request.review_type == ReviewType.CHECKPOINT
        assert request.checkpoint_id == "CP-001"

    def test_submit_repair_review(self, supervisor: CloudSupervisor) -> None:
        """Test submitting repair task for review."""
        request = supervisor.submit_repair_review(
            project_id="test-project",
            team_id="team-123",
            task_id="T-001",
            error_message="Build failed",
        )

        assert request.id.startswith("REP-")
        assert request.review_type == ReviewType.REPAIR
        assert request.task_id == "T-001"
        assert request.metadata["error_message"] == "Build failed"

    def test_submit_pr_review(self, supervisor: CloudSupervisor) -> None:
        """Test submitting PR for review."""
        request = supervisor.submit_pr_review(
            project_id="test-project",
            team_id="team-123",
            pr_number=42,
            pr_url="https://github.com/owner/repo/pull/42",
            repo_owner="owner",
            repo_name="repo",
        )

        assert request.id.startswith("PR-")
        assert request.review_type == ReviewType.PR
        assert request.pr_number == 42
        assert request.metadata["repo_owner"] == "owner"


class TestReviewProcessing:
    """Test review processing methods."""

    @pytest.fixture
    def supervisor(self) -> CloudSupervisor:
        """Create supervisor with mock store."""
        mock_store = MagicMock()
        return CloudSupervisor(store=mock_store)

    def test_get_pending_reviews(self, supervisor: CloudSupervisor) -> None:
        """Test getting pending reviews."""
        mock_response = MagicMock()
        mock_response.data = [
            {
                "id": "REV-001",
                "project_id": "test",
                "team_id": "team-1",
                "review_type": "checkpoint",
                "checkpoint_id": "CP-001",
                "created_at": "2024-01-01T00:00:00",
                "metadata": {},
            }
        ]
        supervisor._store.client.table.return_value.select.return_value.eq.return_value.eq.return_value.order.return_value.execute.return_value = (
            mock_response
        )

        reviews = supervisor.get_pending_reviews("team-1")

        assert len(reviews) == 1
        assert reviews[0].id == "REV-001"

    def test_complete_review(self, supervisor: CloudSupervisor) -> None:
        """Test completing a review."""
        result = supervisor.complete_review(
            request_id="REV-001",
            status=ReviewStatus.APPROVED,
            decision="All good",
            notes="Tests pass",
            reviewer_id="user-123",
        )

        assert result.request_id == "REV-001"
        assert result.status == ReviewStatus.APPROVED
        assert result.reviewed_at is not None

        supervisor._store.client.table.return_value.update.assert_called()


class TestGitHubIntegration:
    """Test GitHub integration methods."""

    @pytest.fixture
    def supervisor(self) -> CloudSupervisor:
        """Create supervisor with mock store and GitHub."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(
            store=mock_store,
            github_token="ghp_test",
        )
        supervisor._github_client = MagicMock()
        return supervisor

    def test_post_pr_review_comment(self, supervisor: CloudSupervisor) -> None:
        """Test posting PR review comment."""
        mock_response = MagicMock()
        mock_response.json.return_value = {"id": 12345}
        supervisor._github_client.post.return_value = mock_response

        comment_id = supervisor.post_pr_review_comment(
            repo_owner="owner",
            repo_name="repo",
            pr_number=42,
            body="LGTM!",
            event="APPROVE",
        )

        assert comment_id == 12345
        supervisor._github_client.post.assert_called_once()

    def test_post_pr_comment(self, supervisor: CloudSupervisor) -> None:
        """Test posting general PR comment."""
        mock_response = MagicMock()
        mock_response.json.return_value = {"id": 67890}
        supervisor._github_client.post.return_value = mock_response

        comment_id = supervisor.post_pr_comment(
            repo_owner="owner",
            repo_name="repo",
            pr_number=42,
            body="Nice work!",
        )

        assert comment_id == 67890

    def test_post_comment_no_github_client(self) -> None:
        """Test posting fails gracefully without GitHub client."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(store=mock_store)

        result = supervisor.post_pr_comment("owner", "repo", 42, "test")

        assert result is None

    def test_format_review_comment_approved(self) -> None:
        """Test formatting approved review comment."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(store=mock_store)

        result = ReviewResult(
            request_id="REV-001",
            status=ReviewStatus.APPROVED,
            decision="All tests pass",
            notes="Great implementation!",
            reviewed_at=datetime.now(),
        )

        comment = supervisor.format_review_comment(result, checkpoint_id="CP-001")

        assert "[PASS]" in comment
        assert "CP-001" in comment
        assert "approved" in comment
        assert "All tests pass" in comment

    def test_format_review_comment_rejected(self) -> None:
        """Test formatting rejected review comment."""
        mock_store = MagicMock()
        supervisor = CloudSupervisor(store=mock_store)

        result = ReviewResult(
            request_id="REV-001",
            status=ReviewStatus.NEEDS_CHANGES,
            notes="Needs work",
            required_changes=["Fix tests", "Add docs"],
            reviewed_at=datetime.now(),
        )

        comment = supervisor.format_review_comment(result)

        assert "[NEEDS_WORK]" in comment
        assert "Required Changes" in comment
        assert "Fix tests" in comment
        assert "Add docs" in comment
