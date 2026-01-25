"""GitLab Integration - API client and webhook handling.

This module provides:
- GitLab API operations via python-gitlab or REST API
- Webhook signature verification
- MR diff retrieval and note/discussion creation
"""

from __future__ import annotations

import hmac
import json
import logging
import os
from dataclasses import dataclass
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

logger = logging.getLogger(__name__)


# =============================================================================
# Result Types
# =============================================================================


@dataclass
class MRInfo:
    """Merge request information extracted from webhook payload."""

    project_id: int
    mr_iid: int  # Internal ID within project
    title: str
    head_sha: str
    source_branch: str
    target_branch: str
    author: str
    namespace: str  # Group or user namespace
    project_path: str  # Full project path (namespace/project)
    diff_url: str


@dataclass
class NoteResult:
    """Result of creating an MR note/comment."""

    success: bool
    message: str
    note_id: int | None = None


@dataclass
class ReviewResult:
    """Result of creating an MR review."""

    success: bool
    message: str
    review_id: int | None = None
    comments_posted: int = 0


# =============================================================================
# GitLab Client
# =============================================================================


class GitLabClient:
    """GitLab API client for webhook handling and MR operations.

    This client handles:
    1. Webhook token verification
    2. OAuth token management
    3. MR operations (get diff, create notes/discussions)
    4. Commit status updates

    Authentication modes:
    - Personal Access Token (for testing/self-hosted)
    - OAuth2 tokens (for application integration)
    """

    def __init__(
        self,
        url: str = "https://gitlab.com",
        private_token: str | None = None,
        oauth_token: str | None = None,
        webhook_secret: str | None = None,
    ) -> None:
        """Initialize GitLab client.

        Args:
            url: GitLab instance URL (default: gitlab.com)
            private_token: Personal access token for authentication
            oauth_token: OAuth2 access token for authentication
            webhook_secret: Webhook secret token for verification
        """
        self.url = url.rstrip("/")
        self.api_url = f"{self.url}/api/v4"
        self.private_token = private_token
        self.oauth_token = oauth_token
        self.webhook_secret = webhook_secret

        # Try to use python-gitlab if available
        self._gitlab: Any = None
        self._init_gitlab_client()

    def _init_gitlab_client(self) -> None:
        """Initialize python-gitlab client if available."""
        try:
            import gitlab

            token = self.private_token or self.oauth_token
            if token:
                if self.oauth_token:
                    self._gitlab = gitlab.Gitlab(self.url, oauth_token=self.oauth_token)
                else:
                    self._gitlab = gitlab.Gitlab(self.url, private_token=self.private_token)
                logger.debug("Initialized python-gitlab client")
        except ImportError:
            logger.debug("python-gitlab not available, using REST API directly")

    # =========================================================================
    # Webhook Verification
    # =========================================================================

    def verify_webhook_signature(self, payload: bytes, token: str) -> bool:
        """Verify webhook token.

        GitLab uses X-Gitlab-Token header with a secret token.
        This is a simple string comparison, not HMAC.

        Args:
            payload: Raw request body (not used for GitLab, but kept for interface consistency)
            token: X-Gitlab-Token header value

        Returns:
            True if token matches configured secret
        """
        if not self.webhook_secret:
            logger.warning("No webhook secret configured, skipping verification")
            return True

        return hmac.compare_digest(token, self.webhook_secret)

    # =========================================================================
    # REST API Helpers
    # =========================================================================

    def _get_auth_headers(self) -> dict[str, str]:
        """Get authentication headers."""
        headers = {
            "Content-Type": "application/json",
        }

        if self.oauth_token:
            headers["Authorization"] = f"Bearer {self.oauth_token}"
        elif self.private_token:
            headers["PRIVATE-TOKEN"] = self.private_token

        return headers

    def _api_request(
        self,
        method: str,
        endpoint: str,
        data: dict[str, Any] | None = None,
        accept: str = "application/json",
    ) -> tuple[int, dict[str, Any] | str | list[Any]]:
        """Make an authenticated API request.

        Args:
            method: HTTP method
            endpoint: API endpoint path
            data: Request body (will be JSON encoded)
            accept: Accept header value

        Returns:
            Tuple of (status_code, response_data)
        """
        url = f"{self.api_url}{endpoint}"
        headers = self._get_auth_headers()
        headers["Accept"] = accept

        body = None
        if data:
            body = json.dumps(data).encode("utf-8")

        request = Request(url, data=body, headers=headers, method=method)

        try:
            with urlopen(request, timeout=60) as response:
                content = response.read()
                if not content:
                    return response.status, {}
                if accept == "text/plain":
                    return response.status, content.decode("utf-8")
                return response.status, json.loads(content)

        except HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            logger.error(f"GitLab API request failed: {e.code} {error_body}")
            try:
                return e.code, json.loads(error_body)
            except json.JSONDecodeError:
                return e.code, {"error": error_body}

        except URLError as e:
            logger.error(f"GitLab API request error: {e}")
            return 0, {"error": str(e)}

    # =========================================================================
    # Webhook Payload Parsing
    # =========================================================================

    def parse_mr_webhook(self, payload: dict[str, Any]) -> MRInfo | None:
        """Parse MR information from webhook payload.

        Args:
            payload: Webhook payload dictionary

        Returns:
            MRInfo if valid MR event, None otherwise
        """
        object_kind = payload.get("object_kind")
        if object_kind != "merge_request":
            return None

        mr_data = payload.get("object_attributes", {})
        project = payload.get("project", {})

        action = mr_data.get("action")
        # Only handle specific actions
        if action not in ("open", "reopen", "update"):
            logger.debug(f"Ignoring MR action: {action}")
            return None

        # For update events, only process if there are new commits
        if action == "update":
            # Check if this is a push update (new commits)
            oldrev = payload.get("object_attributes", {}).get("oldrev")
            if not oldrev:
                logger.debug("Ignoring MR update without new commits")
                return None

        path_with_namespace = project.get("path_with_namespace", "")
        parts = path_with_namespace.rsplit("/", 1)
        namespace = parts[0] if len(parts) > 1 else ""

        return MRInfo(
            project_id=project.get("id", 0),
            mr_iid=mr_data.get("iid", 0),
            title=mr_data.get("title", ""),
            head_sha=mr_data.get("last_commit", {}).get("id", ""),
            source_branch=mr_data.get("source_branch", ""),
            target_branch=mr_data.get("target_branch", ""),
            author=mr_data.get("author_id", payload.get("user", {}).get("username", "")),
            namespace=namespace,
            project_path=path_with_namespace,
            diff_url=mr_data.get("url", ""),
        )

    # =========================================================================
    # MR Operations
    # =========================================================================

    def get_mr_diff(self, mr_info: MRInfo) -> str | None:
        """Get the diff content of an MR.

        Args:
            mr_info: MR information

        Returns:
            Diff content as string, or None on error
        """
        if self._gitlab:
            return self._get_mr_diff_gitlab(mr_info)
        return self._get_mr_diff_rest(mr_info)

    def _get_mr_diff_gitlab(self, mr_info: MRInfo) -> str | None:
        """Get MR diff using python-gitlab."""
        try:
            project = self._gitlab.projects.get(mr_info.project_id)
            mr = project.mergerequests.get(mr_info.mr_iid)

            # Get diff for each changed file
            changes = mr.changes()
            diff_parts = []

            for change in changes.get("changes", []):
                old_path = change.get("old_path", "")
                new_path = change.get("new_path", "")
                diff = change.get("diff", "")

                # Format as unified diff
                if old_path == new_path:
                    header = f"--- a/{old_path}\n+++ b/{new_path}\n"
                else:
                    header = f"--- a/{old_path}\n+++ b/{new_path}\n"

                diff_parts.append(header + diff)

            return "\n".join(diff_parts)

        except Exception as e:
            logger.error(f"Failed to get MR diff via python-gitlab: {e}")
            return None

    def _get_mr_diff_rest(self, mr_info: MRInfo) -> str | None:
        """Get MR diff using REST API."""
        endpoint = f"/projects/{mr_info.project_id}/merge_requests/{mr_info.mr_iid}/changes"

        status, data = self._api_request("GET", endpoint)

        if status == 200 and isinstance(data, dict):
            changes = data.get("changes", [])
            diff_parts = []

            for change in changes:
                old_path = change.get("old_path", "")
                new_path = change.get("new_path", "")
                diff = change.get("diff", "")

                if old_path == new_path:
                    header = f"--- a/{old_path}\n+++ b/{new_path}\n"
                else:
                    header = f"--- a/{old_path}\n+++ b/{new_path}\n"

                diff_parts.append(header + diff)

            return "\n".join(diff_parts)

        logger.error(f"Failed to get MR diff: {status}")
        return None

    def create_mr_note(
        self,
        mr_info: MRInfo,
        body: str,
    ) -> NoteResult:
        """Create a note (comment) on an MR.

        Args:
            mr_info: MR information
            body: Note body (markdown supported)

        Returns:
            NoteResult with status
        """
        if self._gitlab:
            return self._create_mr_note_gitlab(mr_info, body)
        return self._create_mr_note_rest(mr_info, body)

    def _create_mr_note_gitlab(self, mr_info: MRInfo, body: str) -> NoteResult:
        """Create MR note using python-gitlab."""
        try:
            project = self._gitlab.projects.get(mr_info.project_id)
            mr = project.mergerequests.get(mr_info.mr_iid)
            note = mr.notes.create({"body": body})

            return NoteResult(
                success=True,
                message="Note created successfully",
                note_id=note.id,
            )

        except Exception as e:
            logger.error(f"Failed to create MR note via python-gitlab: {e}")
            return NoteResult(
                success=False,
                message=str(e),
            )

    def _create_mr_note_rest(self, mr_info: MRInfo, body: str) -> NoteResult:
        """Create MR note using REST API."""
        endpoint = f"/projects/{mr_info.project_id}/merge_requests/{mr_info.mr_iid}/notes"

        status, data = self._api_request("POST", endpoint, data={"body": body})

        if status == 201 and isinstance(data, dict):
            return NoteResult(
                success=True,
                message="Note created successfully",
                note_id=data.get("id"),
            )

        error_msg = data.get("message", "Unknown error") if isinstance(data, dict) else str(data)
        return NoteResult(
            success=False,
            message=f"Failed to create note: {error_msg}",
        )

    def create_discussion(
        self,
        mr_info: MRInfo,
        body: str,
        position: dict[str, Any] | None = None,
    ) -> NoteResult:
        """Create a discussion on an MR.

        Discussions are threaded conversations, optionally on specific lines.

        Args:
            mr_info: MR information
            body: Discussion body
            position: Optional position info for line comments

        Returns:
            NoteResult with status
        """
        endpoint = f"/projects/{mr_info.project_id}/merge_requests/{mr_info.mr_iid}/discussions"

        data: dict[str, Any] = {"body": body}
        if position:
            data["position"] = position

        status, response = self._api_request("POST", endpoint, data=data)

        if status == 201 and isinstance(response, dict):
            return NoteResult(
                success=True,
                message="Discussion created successfully",
                note_id=response.get("id"),
            )

        error_msg = response.get("message", "Unknown error") if isinstance(response, dict) else str(response)
        return NoteResult(
            success=False,
            message=f"Failed to create discussion: {error_msg}",
        )

    def create_commit_status(
        self,
        project_id: int,
        sha: str,
        state: str,
        name: str = "C4 Code Review",
        description: str | None = None,
        target_url: str | None = None,
    ) -> bool:
        """Create a commit status (pipeline status).

        Args:
            project_id: GitLab project ID
            sha: Commit SHA
            state: Status state (pending, running, success, failed, canceled)
            name: Context name
            description: Status description
            target_url: URL to link to

        Returns:
            True if successful
        """
        endpoint = f"/projects/{project_id}/statuses/{sha}"

        data: dict[str, Any] = {
            "state": state,
            "name": name,
        }

        if description:
            data["description"] = description
        if target_url:
            data["target_url"] = target_url

        status, _ = self._api_request("POST", endpoint, data=data)

        return status == 201

    def add_mr_labels(
        self,
        mr_info: MRInfo,
        labels: list[str],
    ) -> bool:
        """Add labels to an MR.

        Args:
            mr_info: MR information
            labels: Labels to add

        Returns:
            True if successful
        """
        if self._gitlab:
            try:
                project = self._gitlab.projects.get(mr_info.project_id)
                mr = project.mergerequests.get(mr_info.mr_iid)

                # Get existing labels and add new ones
                existing = mr.labels or []
                mr.labels = list(set(existing + labels))
                mr.save()
                return True

            except Exception as e:
                logger.error(f"Failed to add labels via python-gitlab: {e}")
                return False

        # REST API fallback
        endpoint = f"/projects/{mr_info.project_id}/merge_requests/{mr_info.mr_iid}"

        # First get existing labels
        status, data = self._api_request("GET", endpoint)
        if status != 200 or not isinstance(data, dict):
            return False

        existing = data.get("labels", [])
        new_labels = list(set(existing + labels))

        status, _ = self._api_request("PUT", endpoint, data={"labels": ",".join(new_labels)})

        return status == 200

    # =========================================================================
    # Factory Methods
    # =========================================================================

    @classmethod
    def from_env(cls) -> GitLabClient | None:
        """Create client from environment variables.

        Environment Variables:
            GITLAB_URL: GitLab instance URL (default: https://gitlab.com)
            GITLAB_PRIVATE_TOKEN: Personal access token
            GITLAB_OAUTH_TOKEN: OAuth access token
            GITLAB_WEBHOOK_SECRET: Webhook secret token

        Returns:
            GitLabClient if configured, None otherwise
        """
        url = os.environ.get("GITLAB_URL", "https://gitlab.com")
        private_token = os.environ.get("GITLAB_PRIVATE_TOKEN")
        oauth_token = os.environ.get("GITLAB_OAUTH_TOKEN")
        webhook_secret = os.environ.get("GITLAB_WEBHOOK_SECRET")

        if not (private_token or oauth_token):
            return None

        return cls(
            url=url,
            private_token=private_token,
            oauth_token=oauth_token,
            webhook_secret=webhook_secret,
        )
