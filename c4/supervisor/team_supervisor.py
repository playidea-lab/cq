"""Team Supervisor - Centralized review with team lead authority."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from httpx import Client


class ReviewDecision(str, Enum):
    """Review decision types."""

    APPROVED = "approved"
    REJECTED = "rejected"
    NEEDS_CHANGES = "needs_changes"
    PENDING = "pending"


class CheckpointType(str, Enum):
    """Checkpoint types."""

    TASK_COMPLETE = "task_complete"
    PHASE_COMPLETE = "phase_complete"
    MILESTONE = "milestone"


@dataclass
class ReviewRequest:
    """Request for team lead review.

    Attributes:
        id: Unique request ID
        task_id: Task being reviewed
        worker_id: Worker who completed task
        checkpoint_type: Type of checkpoint
        commit_sha: Git commit SHA
        pr_number: GitHub PR number if applicable
        created_at: When request was created
        metadata: Additional context
    """

    id: str
    task_id: str
    worker_id: str
    checkpoint_type: CheckpointType
    commit_sha: str
    pr_number: int | None = None
    created_at: datetime = field(default_factory=datetime.now)
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class ReviewResult:
    """Result of team lead review.

    Attributes:
        request_id: Original request ID
        decision: Review decision
        reviewer_id: Who performed review
        comments: Review comments
        reviewed_at: When review completed
        suggested_changes: List of suggested changes
    """

    request_id: str
    decision: ReviewDecision
    reviewer_id: str
    comments: str
    reviewed_at: datetime = field(default_factory=datetime.now)
    suggested_changes: list[str] = field(default_factory=list)


class TeamSupervisor:
    """
    Centralized supervisor with team lead authority.

    Features:
    - Review submissions using team lead's API key
    - Process checkpoints and approve/reject
    - Post GitHub PR comments on reviews
    - Track review history

    The team lead's API key is used for all LLM reviews,
    ensuring consistent billing and access control.

    Environment Variables:
        TEAM_LEAD_API_KEY: OpenAI/Anthropic API key for reviews
        GITHUB_TOKEN: GitHub token for PR comments

    Example:
        supervisor = TeamSupervisor(
            team_lead_api_key="sk-...",
            github_token="ghp_...",
        )

        result = supervisor.review_task(request)
        if result.decision == ReviewDecision.APPROVED:
            supervisor.post_pr_comment(result)
    """

    def __init__(
        self,
        team_lead_api_key: str | None = None,
        github_token: str | None = None,
        llm_model: str = "claude-sonnet-4-20250514",
    ):
        """Initialize team supervisor.

        Args:
            team_lead_api_key: API key for LLM reviews
            github_token: GitHub token for PR operations
            llm_model: Model to use for reviews
        """
        self._api_key = team_lead_api_key or os.environ.get("TEAM_LEAD_API_KEY", "")
        self._github_token = github_token or os.environ.get("GITHUB_TOKEN", "")
        self._llm_model = llm_model
        self._http_client: Client | None = None
        self._pending_reviews: dict[str, ReviewRequest] = {}
        self._completed_reviews: list[ReviewResult] = []

    @property
    def http_client(self) -> "Client":
        """Get HTTP client (lazy init)."""
        if self._http_client is None:
            import httpx

            self._http_client = httpx.Client(timeout=60.0)
        return self._http_client

    def close(self) -> None:
        """Close resources."""
        if self._http_client:
            self._http_client.close()
            self._http_client = None

    def __enter__(self) -> "TeamSupervisor":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    # =========================================================================
    # Review Queue
    # =========================================================================

    def submit_for_review(self, request: ReviewRequest) -> str:
        """Submit a task for team lead review.

        Args:
            request: Review request

        Returns:
            Request ID
        """
        self._pending_reviews[request.id] = request
        return request.id

    def get_pending_reviews(self) -> list[ReviewRequest]:
        """Get all pending review requests."""
        return list(self._pending_reviews.values())

    def get_review_request(self, request_id: str) -> ReviewRequest | None:
        """Get a specific review request."""
        return self._pending_reviews.get(request_id)

    # =========================================================================
    # Review Processing
    # =========================================================================

    def review_task(
        self,
        request: ReviewRequest,
        task_description: str,
        changes_summary: str,
    ) -> ReviewResult:
        """Review a task using team lead's LLM key.

        Args:
            request: Review request
            task_description: Description of the task
            changes_summary: Summary of changes made

        Returns:
            Review result
        """
        # Build review prompt
        prompt = self._build_review_prompt(
            task_description=task_description,
            changes_summary=changes_summary,
            checkpoint_type=request.checkpoint_type,
        )

        # Call LLM for review (using team lead's key)
        response = self._call_llm(prompt)

        # Parse response into decision
        decision, comments, suggestions = self._parse_review_response(response)

        result = ReviewResult(
            request_id=request.id,
            decision=decision,
            reviewer_id="team-lead",
            comments=comments,
            suggested_changes=suggestions,
        )

        # Move from pending to completed
        if request.id in self._pending_reviews:
            del self._pending_reviews[request.id]
        self._completed_reviews.append(result)

        return result

    def _build_review_prompt(
        self,
        task_description: str,
        changes_summary: str,
        checkpoint_type: CheckpointType,
    ) -> str:
        """Build the review prompt."""
        return f"""You are a team lead reviewing code changes.

## Task Description
{task_description}

## Changes Summary
{changes_summary}

## Checkpoint Type
{checkpoint_type.value}

## Instructions
Review the changes and provide:
1. Decision: APPROVED, REJECTED, or NEEDS_CHANGES
2. Comments: Explanation of your decision
3. Suggestions: If NEEDS_CHANGES, list specific changes needed

Format your response as:
DECISION: [your decision]
COMMENTS: [your comments]
SUGGESTIONS:
- [suggestion 1]
- [suggestion 2]
"""

    def _call_llm(self, prompt: str) -> str:
        """Call LLM for review using team lead's key.

        This uses the team lead's API key for billing
        and access control.
        """
        if not self._api_key:
            # Mock response for testing
            return """DECISION: APPROVED
COMMENTS: Changes look good. Code follows best practices.
SUGGESTIONS:
"""

        # Determine provider from model name
        if "claude" in self._llm_model:
            return self._call_anthropic(prompt)
        else:
            return self._call_openai(prompt)

    def _call_anthropic(self, prompt: str) -> str:
        """Call Anthropic API."""
        response = self.http_client.post(
            "https://api.anthropic.com/v1/messages",
            headers={
                "x-api-key": self._api_key,
                "anthropic-version": "2023-06-01",
                "content-type": "application/json",
            },
            json={
                "model": self._llm_model,
                "max_tokens": 1024,
                "messages": [{"role": "user", "content": prompt}],
            },
        )

        if response.status_code == 200:
            data = response.json()
            return data["content"][0]["text"]
        return f"Error: {response.status_code}"

    def _call_openai(self, prompt: str) -> str:
        """Call OpenAI API."""
        response = self.http_client.post(
            "https://api.openai.com/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {self._api_key}",
                "Content-Type": "application/json",
            },
            json={
                "model": self._llm_model,
                "messages": [{"role": "user", "content": prompt}],
                "max_tokens": 1024,
            },
        )

        if response.status_code == 200:
            data = response.json()
            return data["choices"][0]["message"]["content"]
        return f"Error: {response.status_code}"

    def _parse_review_response(
        self,
        response: str,
    ) -> tuple[ReviewDecision, str, list[str]]:
        """Parse LLM response into review result."""
        decision = ReviewDecision.PENDING
        comments = ""
        suggestions: list[str] = []

        lines = response.strip().split("\n")
        current_section = ""

        for line in lines:
            line = line.strip()

            if line.startswith("DECISION:"):
                decision_str = line.replace("DECISION:", "").strip().upper()
                if "APPROVED" in decision_str:
                    decision = ReviewDecision.APPROVED
                elif "REJECTED" in decision_str:
                    decision = ReviewDecision.REJECTED
                elif "NEEDS" in decision_str or "CHANGES" in decision_str:
                    decision = ReviewDecision.NEEDS_CHANGES

            elif line.startswith("COMMENTS:"):
                comments = line.replace("COMMENTS:", "").strip()
                current_section = "comments"

            elif line.startswith("SUGGESTIONS:"):
                current_section = "suggestions"

            elif line.startswith("- ") and current_section == "suggestions":
                suggestions.append(line[2:])

            elif current_section == "comments" and line:
                comments += " " + line

        return decision, comments.strip(), suggestions

    # =========================================================================
    # GitHub Integration
    # =========================================================================

    def post_pr_comment(
        self,
        result: ReviewResult,
        repo: str,
        pr_number: int,
    ) -> bool:
        """Post review result as PR comment.

        Args:
            result: Review result
            repo: Repository (owner/name)
            pr_number: PR number

        Returns:
            True if posted successfully
        """
        if not self._github_token:
            return False

        comment_body = self._format_pr_comment(result)

        response = self.http_client.post(
            f"https://api.github.com/repos/{repo}/issues/{pr_number}/comments",
            headers={
                "Authorization": f"token {self._github_token}",
                "Accept": "application/vnd.github.v3+json",
            },
            json={"body": comment_body},
        )

        return response.status_code == 201

    def _format_pr_comment(self, result: ReviewResult) -> str:
        """Format review result as PR comment."""
        emoji = {
            ReviewDecision.APPROVED: "[APPROVED]",
            ReviewDecision.REJECTED: "[REJECTED]",
            ReviewDecision.NEEDS_CHANGES: "[NEEDS CHANGES]",
            ReviewDecision.PENDING: "[PENDING]",
        }

        comment = f"""## C4 Team Lead Review {emoji.get(result.decision, "")}

**Decision:** {result.decision.value.upper()}

**Comments:**
{result.comments}
"""

        if result.suggested_changes:
            comment += "\n**Suggested Changes:**\n"
            for change in result.suggested_changes:
                comment += f"- {change}\n"

        comment += f"\n---\n*Reviewed at {result.reviewed_at.isoformat()}*"

        return comment

    # =========================================================================
    # Checkpoint Management
    # =========================================================================

    def approve_checkpoint(
        self,
        request: ReviewRequest,
        comments: str = "",
    ) -> ReviewResult:
        """Approve a checkpoint directly.

        Args:
            request: Review request
            comments: Optional comments

        Returns:
            Approved review result
        """
        result = ReviewResult(
            request_id=request.id,
            decision=ReviewDecision.APPROVED,
            reviewer_id="team-lead",
            comments=comments or "Checkpoint approved.",
        )

        if request.id in self._pending_reviews:
            del self._pending_reviews[request.id]
        self._completed_reviews.append(result)

        return result

    def reject_checkpoint(
        self,
        request: ReviewRequest,
        reason: str,
        suggestions: list[str] | None = None,
    ) -> ReviewResult:
        """Reject a checkpoint.

        Args:
            request: Review request
            reason: Rejection reason
            suggestions: Suggested fixes

        Returns:
            Rejected review result
        """
        result = ReviewResult(
            request_id=request.id,
            decision=ReviewDecision.REJECTED,
            reviewer_id="team-lead",
            comments=reason,
            suggested_changes=suggestions or [],
        )

        if request.id in self._pending_reviews:
            del self._pending_reviews[request.id]
        self._completed_reviews.append(result)

        return result

    # =========================================================================
    # History
    # =========================================================================

    def get_review_history(
        self,
        task_id: str | None = None,
    ) -> list[ReviewResult]:
        """Get review history.

        Args:
            task_id: Optional filter by task

        Returns:
            List of review results
        """
        if task_id:
            return [
                r
                for r in self._completed_reviews
                if any(
                    req.task_id == task_id for req in [self.get_review_request(r.request_id)] if req
                )
            ]
        return list(self._completed_reviews)

    def get_stats(self) -> dict[str, Any]:
        """Get review statistics."""
        total = len(self._completed_reviews)
        by_decision = {d.value: 0 for d in ReviewDecision}

        for result in self._completed_reviews:
            by_decision[result.decision.value] += 1

        return {
            "total_reviews": total,
            "pending": len(self._pending_reviews),
            "by_decision": by_decision,
            "approval_rate": (by_decision["approved"] / total if total > 0 else 0),
        }
