"""Cloud Supervisor - Centralized review for team collaboration."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..store import SupabaseStateStore


class ReviewType(str, Enum):
    """Type of review."""

    CHECKPOINT = "checkpoint"
    REPAIR = "repair"
    PR = "pull_request"


class ReviewStatus(str, Enum):
    """Status of a review."""

    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    APPROVED = "approved"
    REJECTED = "rejected"
    NEEDS_CHANGES = "needs_changes"


@dataclass
class ReviewRequest:
    """Request for supervisor review."""

    id: str
    project_id: str
    team_id: str
    review_type: ReviewType
    checkpoint_id: str | None = None
    task_id: str | None = None
    pr_number: int | None = None
    pr_url: str | None = None
    bundle_path: str | None = None
    requester_id: str | None = None
    created_at: datetime = field(default_factory=datetime.now)
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class ReviewResult:
    """Result of a supervisor review."""

    request_id: str
    status: ReviewStatus
    decision: str | None = None
    notes: str | None = None
    required_changes: list[str] | None = None
    reviewer_id: str | None = None
    reviewed_at: datetime | None = None
    github_comment_id: int | None = None


class CloudSupervisor:
    """
    Centralized supervisor for team collaboration.

    Features:
    - Team-wide checkpoint reviews
    - GitHub PR review integration
    - Repair task management
    - Real-time status updates

    Example:
        supervisor = CloudSupervisor(store)
        request = supervisor.submit_checkpoint_review(
            project_id="my-project",
            team_id="team-123",
            checkpoint_id="CP-001",
        )
        result = await supervisor.wait_for_review(request.id)
    """

    TABLE_REVIEWS = "c4_reviews"

    def __init__(
        self,
        store: "SupabaseStateStore",
        github_token: str | None = None,
    ):
        """Initialize cloud supervisor.

        Args:
            store: Supabase state store for persistence
            github_token: GitHub token for PR comments (or GITHUB_TOKEN env)
        """
        self._store = store
        self._github_token = github_token or os.environ.get("GITHUB_TOKEN")
        self._github_client: Any = None

    @property
    def github(self) -> Any:
        """Lazy-initialize GitHub client."""
        if self._github_client is None and self._github_token:
            try:
                import httpx

                self._github_client = httpx.Client(
                    base_url="https://api.github.com",
                    headers={
                        "Authorization": f"Bearer {self._github_token}",
                        "Accept": "application/vnd.github.v3+json",
                        "X-GitHub-Api-Version": "2022-11-28",
                    },
                )
            except ImportError:
                pass
        return self._github_client

    # =========================================================================
    # Review Submission
    # =========================================================================

    def submit_checkpoint_review(
        self,
        project_id: str,
        team_id: str,
        checkpoint_id: str,
        bundle_path: str | None = None,
        requester_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> ReviewRequest:
        """Submit a checkpoint for team review.

        Args:
            project_id: Project identifier
            team_id: Team identifier
            checkpoint_id: Checkpoint to review
            bundle_path: Path to review bundle
            requester_id: ID of user requesting review
            metadata: Additional metadata

        Returns:
            ReviewRequest with tracking ID
        """
        import uuid

        request = ReviewRequest(
            id=f"REV-{uuid.uuid4().hex[:8].upper()}",
            project_id=project_id,
            team_id=team_id,
            review_type=ReviewType.CHECKPOINT,
            checkpoint_id=checkpoint_id,
            bundle_path=bundle_path,
            requester_id=requester_id,
            metadata=metadata or {},
        )

        self._save_review_request(request)
        return request

    def submit_repair_review(
        self,
        project_id: str,
        team_id: str,
        task_id: str,
        error_message: str,
        requester_id: str | None = None,
    ) -> ReviewRequest:
        """Submit a failed task for repair review.

        Args:
            project_id: Project identifier
            team_id: Team identifier
            task_id: Failed task ID
            error_message: Error that caused failure
            requester_id: Worker that encountered error

        Returns:
            ReviewRequest for repair guidance
        """
        import uuid

        request = ReviewRequest(
            id=f"REP-{uuid.uuid4().hex[:8].upper()}",
            project_id=project_id,
            team_id=team_id,
            review_type=ReviewType.REPAIR,
            task_id=task_id,
            requester_id=requester_id,
            metadata={"error_message": error_message},
        )

        self._save_review_request(request)
        return request

    def submit_pr_review(
        self,
        project_id: str,
        team_id: str,
        pr_number: int,
        pr_url: str,
        repo_owner: str,
        repo_name: str,
        requester_id: str | None = None,
    ) -> ReviewRequest:
        """Submit a GitHub PR for review.

        Args:
            project_id: Project identifier
            team_id: Team identifier
            pr_number: Pull request number
            pr_url: Full PR URL
            repo_owner: Repository owner
            repo_name: Repository name
            requester_id: User requesting review

        Returns:
            ReviewRequest for PR review
        """
        import uuid

        request = ReviewRequest(
            id=f"PR-{uuid.uuid4().hex[:8].upper()}",
            project_id=project_id,
            team_id=team_id,
            review_type=ReviewType.PR,
            pr_number=pr_number,
            pr_url=pr_url,
            requester_id=requester_id,
            metadata={
                "repo_owner": repo_owner,
                "repo_name": repo_name,
            },
        )

        self._save_review_request(request)
        return request

    # =========================================================================
    # Review Processing
    # =========================================================================

    def get_pending_reviews(
        self,
        team_id: str,
        review_type: ReviewType | None = None,
    ) -> list[ReviewRequest]:
        """Get all pending reviews for a team.

        Args:
            team_id: Team identifier
            review_type: Filter by review type

        Returns:
            List of pending review requests
        """
        query = (
            self._store.client.table(self.TABLE_REVIEWS)
            .select("*")
            .eq("team_id", team_id)
            .eq("status", ReviewStatus.PENDING.value)
        )

        if review_type:
            query = query.eq("review_type", review_type.value)

        response = query.order("created_at", desc=False).execute()

        return [self._row_to_request(row) for row in response.data]

    def complete_review(
        self,
        request_id: str,
        status: ReviewStatus,
        decision: str | None = None,
        notes: str | None = None,
        required_changes: list[str] | None = None,
        reviewer_id: str | None = None,
    ) -> ReviewResult:
        """Complete a review with decision.

        Args:
            request_id: Review request ID
            status: Final status
            decision: Decision summary
            notes: Reviewer notes
            required_changes: Changes needed (if rejected)
            reviewer_id: Reviewer's user ID

        Returns:
            ReviewResult with completion details
        """
        result = ReviewResult(
            request_id=request_id,
            status=status,
            decision=decision,
            notes=notes,
            required_changes=required_changes,
            reviewer_id=reviewer_id,
            reviewed_at=datetime.now(),
        )

        # Update in database
        self._store.client.table(self.TABLE_REVIEWS).update({
            "status": status.value,
            "decision": decision,
            "notes": notes,
            "required_changes": required_changes,
            "reviewer_id": reviewer_id,
            "reviewed_at": result.reviewed_at.isoformat(),
        }).eq("id", request_id).execute()

        return result

    # =========================================================================
    # GitHub Integration
    # =========================================================================

    def post_pr_review_comment(
        self,
        repo_owner: str,
        repo_name: str,
        pr_number: int,
        body: str,
        event: str = "COMMENT",
    ) -> int | None:
        """Post a review comment on a GitHub PR.

        Args:
            repo_owner: Repository owner
            repo_name: Repository name
            pr_number: Pull request number
            body: Comment body (markdown)
            event: Review event (COMMENT, APPROVE, REQUEST_CHANGES)

        Returns:
            Comment ID if successful, None otherwise
        """
        if not self.github:
            return None

        try:
            response = self.github.post(
                f"/repos/{repo_owner}/{repo_name}/pulls/{pr_number}/reviews",
                json={
                    "body": body,
                    "event": event,
                },
            )
            response.raise_for_status()
            return response.json().get("id")
        except Exception:
            return None

    def post_pr_comment(
        self,
        repo_owner: str,
        repo_name: str,
        pr_number: int,
        body: str,
    ) -> int | None:
        """Post a general comment on a GitHub PR.

        Args:
            repo_owner: Repository owner
            repo_name: Repository name
            pr_number: Pull request number
            body: Comment body (markdown)

        Returns:
            Comment ID if successful, None otherwise
        """
        if not self.github:
            return None

        try:
            response = self.github.post(
                f"/repos/{repo_owner}/{repo_name}/issues/{pr_number}/comments",
                json={"body": body},
            )
            response.raise_for_status()
            return response.json().get("id")
        except Exception:
            return None

    def format_review_comment(
        self,
        result: ReviewResult,
        checkpoint_id: str | None = None,
    ) -> str:
        """Format a review result as a GitHub comment.

        Args:
            result: Review result
            checkpoint_id: Optional checkpoint ID

        Returns:
            Formatted markdown comment
        """
        status_emoji = {
            ReviewStatus.APPROVED: "[PASS]",
            ReviewStatus.REJECTED: "[FAIL]",
            ReviewStatus.NEEDS_CHANGES: "[NEEDS_WORK]",
        }.get(result.status, "[INFO]")

        lines = [
            f"## C4 Review {status_emoji}",
            "",
        ]

        if checkpoint_id:
            lines.append(f"**Checkpoint:** {checkpoint_id}")

        lines.append(f"**Status:** {result.status.value}")

        if result.decision:
            lines.extend(["", f"**Decision:** {result.decision}"])

        if result.notes:
            lines.extend(["", "### Notes", result.notes])

        if result.required_changes:
            lines.extend(["", "### Required Changes"])
            for change in result.required_changes:
                lines.append(f"- {change}")

        lines.extend([
            "",
            "---",
            f"*Reviewed at {result.reviewed_at.isoformat() if result.reviewed_at else 'N/A'}*",
        ])

        return "\n".join(lines)

    # =========================================================================
    # Helpers
    # =========================================================================

    def _save_review_request(self, request: ReviewRequest) -> None:
        """Save review request to database."""
        self._store.client.table(self.TABLE_REVIEWS).insert({
            "id": request.id,
            "project_id": request.project_id,
            "team_id": request.team_id,
            "review_type": request.review_type.value,
            "checkpoint_id": request.checkpoint_id,
            "task_id": request.task_id,
            "pr_number": request.pr_number,
            "pr_url": request.pr_url,
            "bundle_path": request.bundle_path,
            "requester_id": request.requester_id,
            "status": ReviewStatus.PENDING.value,
            "created_at": request.created_at.isoformat(),
            "metadata": request.metadata,
        }).execute()

    def _row_to_request(self, row: dict[str, Any]) -> ReviewRequest:
        """Convert database row to ReviewRequest."""
        return ReviewRequest(
            id=row["id"],
            project_id=row["project_id"],
            team_id=row["team_id"],
            review_type=ReviewType(row["review_type"]),
            checkpoint_id=row.get("checkpoint_id"),
            task_id=row.get("task_id"),
            pr_number=row.get("pr_number"),
            pr_url=row.get("pr_url"),
            bundle_path=row.get("bundle_path"),
            requester_id=row.get("requester_id"),
            created_at=datetime.fromisoformat(row["created_at"]),
            metadata=row.get("metadata", {}),
        )
