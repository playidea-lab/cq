"""GitLab Integration Provider.

Implements the IntegrationProvider interface for GitLab integration.
Supports MR reviews, webhooks, and OAuth flow.
"""

from __future__ import annotations

import logging
import os
from typing import Any
from urllib.parse import urlencode

from .base import (
    ConnectionResult,
    IntegrationCapability,
    MergeRequestInfo,
    NotificationResult,
    SourceControlProvider,
    WebhookEvent,
)
from .gitlab_client import GitLabClient, MRInfo
from .registry import IntegrationRegistry

logger = logging.getLogger(__name__)


@IntegrationRegistry.register
class GitLabProvider(SourceControlProvider):
    """GitLab integration provider.

    Supports:
    - MR review via webhooks
    - OAuth2 application flow
    - Notifications via MR notes
    - Commit status for pipeline integration

    Environment Variables:
        GITLAB_URL: GitLab instance URL (default: https://gitlab.com)
        GITLAB_APP_ID: OAuth Application ID
        GITLAB_APP_SECRET: OAuth Application Secret
        GITLAB_WEBHOOK_SECRET: Webhook secret token
        GITLAB_PRIVATE_TOKEN: Personal access token (for testing)
    """

    def __init__(self) -> None:
        """Initialize GitLab provider."""
        self._client: GitLabClient | None = None
        self._gitlab_url = os.environ.get("GITLAB_URL", "https://gitlab.com")

    # =========================================================================
    # Provider Identity
    # =========================================================================

    @property
    def id(self) -> str:
        return "gitlab"

    @property
    def name(self) -> str:
        return "GitLab"

    @property
    def capabilities(self) -> list[IntegrationCapability]:
        return [
            IntegrationCapability.PR_REVIEW,
            IntegrationCapability.NOTIFICATIONS,
            IntegrationCapability.WEBHOOKS,
            IntegrationCapability.OAUTH,
        ]

    @property
    def description(self) -> str:
        return "GitLab integration for MR reviews, code analysis, and notifications"

    @property
    def icon_url(self) -> str | None:
        return "https://about.gitlab.com/images/press/press-kit-icon.svg"

    @property
    def docs_url(self) -> str | None:
        return "https://docs.gitlab.com/ee/api/"

    # =========================================================================
    # Client Management
    # =========================================================================

    def _get_client(self, credentials: dict[str, Any] | None = None) -> GitLabClient | None:
        """Get or create GitLab client.

        Args:
            credentials: Optional credentials override

        Returns:
            GitLabClient or None if not configured
        """
        if credentials:
            return GitLabClient(
                url=credentials.get("gitlab_url", self._gitlab_url),
                private_token=credentials.get("private_token"),
                oauth_token=credentials.get("access_token"),
                webhook_secret=credentials.get("webhook_secret"),
            )

        # Use environment variables
        if self._client is None:
            self._client = GitLabClient.from_env()

        return self._client

    # =========================================================================
    # OAuth Flow
    # =========================================================================

    def get_oauth_url(self, state: str) -> str:
        """Get GitLab OAuth authorization URL.

        Args:
            state: State parameter (contains workspace_id, user_id)

        Returns:
            OAuth authorization URL
        """
        app_id = os.environ.get("GITLAB_APP_ID", "")
        redirect_uri = os.environ.get("GITLAB_REDIRECT_URI", "")

        params = {
            "client_id": app_id,
            "redirect_uri": redirect_uri,
            "response_type": "code",
            "state": state,
            "scope": "api read_user read_repository",
        }

        return f"{self._gitlab_url}/oauth/authorize?{urlencode(params)}"

    async def exchange_code(self, code: str, state: str) -> ConnectionResult:
        """Exchange OAuth authorization code for tokens.

        Args:
            code: Authorization code from OAuth callback
            state: State parameter for verification

        Returns:
            ConnectionResult with credentials if successful
        """
        import json
        from urllib.request import Request, urlopen

        app_id = os.environ.get("GITLAB_APP_ID", "")
        app_secret = os.environ.get("GITLAB_APP_SECRET", "")
        redirect_uri = os.environ.get("GITLAB_REDIRECT_URI", "")

        if not all([app_id, app_secret, redirect_uri]):
            return ConnectionResult(
                success=False,
                message="GitLab OAuth not configured",
                error_code="not_configured",
            )

        token_url = f"{self._gitlab_url}/oauth/token"
        data = {
            "client_id": app_id,
            "client_secret": app_secret,
            "code": code,
            "grant_type": "authorization_code",
            "redirect_uri": redirect_uri,
        }

        try:
            request = Request(
                token_url,
                data=json.dumps(data).encode("utf-8"),
                headers={"Content-Type": "application/json"},
                method="POST",
            )

            with urlopen(request, timeout=30) as response:
                token_data = json.loads(response.read())

                # Get user info
                access_token = token_data.get("access_token", "")
                user_info = await self._get_user_info(access_token)

                return ConnectionResult(
                    success=True,
                    message="GitLab connected successfully",
                    external_id=str(user_info.get("id", "")),
                    external_name=user_info.get("username", ""),
                    credentials={
                        "access_token": access_token,
                        "refresh_token": token_data.get("refresh_token"),
                        "token_type": token_data.get("token_type", "bearer"),
                        "expires_in": token_data.get("expires_in"),
                        "user_id": user_info.get("id"),
                        "username": user_info.get("username"),
                        "gitlab_url": self._gitlab_url,
                    },
                )

        except Exception as e:
            logger.error(f"Failed to exchange GitLab OAuth code: {e}")
            return ConnectionResult(
                success=False,
                message=f"Failed to exchange code: {e}",
                error_code="exchange_failed",
            )

    async def _get_user_info(self, access_token: str) -> dict[str, Any]:
        """Get GitLab user info using access token."""
        import json
        from urllib.request import Request, urlopen

        url = f"{self._gitlab_url}/api/v4/user"
        request = Request(
            url,
            headers={"Authorization": f"Bearer {access_token}"},
        )

        try:
            with urlopen(request, timeout=30) as response:
                return json.loads(response.read())
        except Exception as e:
            logger.error(f"Failed to get GitLab user info: {e}")
            return {}

    # =========================================================================
    # Connection Management
    # =========================================================================

    async def connect(
        self,
        workspace_id: str,
        credentials: dict[str, Any],
    ) -> ConnectionResult:
        """Connect GitLab to workspace.

        Args:
            workspace_id: C4 workspace ID
            credentials: OAuth credentials

        Returns:
            ConnectionResult
        """
        access_token = credentials.get("access_token")
        if not access_token:
            return ConnectionResult(
                success=False,
                message="Missing access_token in credentials",
                error_code="missing_access_token",
            )

        return ConnectionResult(
            success=True,
            message="GitLab connected",
            external_id=str(credentials.get("user_id", "")),
            external_name=credentials.get("username", ""),
            credentials=credentials,
        )

    async def disconnect(
        self,
        workspace_id: str,
        external_id: str,
    ) -> bool:
        """Disconnect GitLab from workspace.

        Args:
            workspace_id: C4 workspace ID
            external_id: GitLab user ID

        Returns:
            True (always succeeds on our side)
        """
        logger.info(f"Disconnecting GitLab user {external_id} from workspace {workspace_id}")
        return True

    async def validate_connection(
        self,
        credentials: dict[str, Any],
    ) -> bool:
        """Validate GitLab credentials are still valid.

        Args:
            credentials: Stored credentials

        Returns:
            True if credentials are valid
        """
        access_token = credentials.get("access_token")
        if not access_token:
            return False

        try:
            user_info = await self._get_user_info(access_token)
            return bool(user_info.get("id"))
        except Exception as e:
            logger.warning(f"GitLab connection validation failed: {e}")
            return False

    async def refresh_token(
        self,
        credentials: dict[str, Any],
    ) -> dict[str, Any] | None:
        """Refresh GitLab OAuth tokens.

        Args:
            credentials: Current credentials with refresh_token

        Returns:
            Updated credentials or None if refresh failed
        """
        import json
        from urllib.request import Request, urlopen

        refresh_token = credentials.get("refresh_token")
        if not refresh_token:
            return None

        app_id = os.environ.get("GITLAB_APP_ID", "")
        app_secret = os.environ.get("GITLAB_APP_SECRET", "")

        if not all([app_id, app_secret]):
            return None

        token_url = f"{self._gitlab_url}/oauth/token"
        data = {
            "client_id": app_id,
            "client_secret": app_secret,
            "refresh_token": refresh_token,
            "grant_type": "refresh_token",
        }

        try:
            request = Request(
                token_url,
                data=json.dumps(data).encode("utf-8"),
                headers={"Content-Type": "application/json"},
                method="POST",
            )

            with urlopen(request, timeout=30) as response:
                token_data = json.loads(response.read())

                # Update credentials
                new_credentials = credentials.copy()
                new_credentials.update({
                    "access_token": token_data.get("access_token"),
                    "refresh_token": token_data.get("refresh_token"),
                    "expires_in": token_data.get("expires_in"),
                })

                return new_credentials

        except Exception as e:
            logger.error(f"Failed to refresh GitLab token: {e}")
            return None

    # =========================================================================
    # Notifications
    # =========================================================================

    async def send_notification(
        self,
        credentials: dict[str, Any],
        message: str,
        *,
        channel_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> NotificationResult:
        """Send notification via MR note.

        For GitLab, notifications are sent as MR notes.
        The channel_id should be in format: project_id#mr_iid

        Args:
            credentials: GitLab credentials
            message: Note body (markdown supported)
            channel_id: "project_id#mr_iid" format
            metadata: Additional options

        Returns:
            NotificationResult
        """
        if not channel_id:
            return NotificationResult(
                success=False,
                message="channel_id required in format: project_id#mr_iid",
            )

        # Parse channel_id
        try:
            project_id_str, mr_iid_str = channel_id.split("#")
            project_id = int(project_id_str)
            mr_iid = int(mr_iid_str)
        except ValueError:
            return NotificationResult(
                success=False,
                message=f"Invalid channel_id format: {channel_id}",
            )

        client = self._get_client(credentials)
        if not client:
            return NotificationResult(
                success=False,
                message="GitLab client not configured",
            )

        mr_info = MRInfo(
            project_id=project_id,
            mr_iid=mr_iid,
            title="",
            head_sha="",
            source_branch="",
            target_branch="",
            author="",
            namespace="",
            project_path="",
            diff_url="",
        )

        result = client.create_mr_note(mr_info, message)

        return NotificationResult(
            success=result.success,
            message=result.message,
            message_id=str(result.note_id) if result.note_id else None,
        )

    # =========================================================================
    # Webhooks
    # =========================================================================

    async def verify_webhook(
        self,
        payload: bytes,
        headers: dict[str, str],
        secret: str,
    ) -> bool:
        """Verify GitLab webhook token.

        GitLab uses X-Gitlab-Token header with a secret token.

        Args:
            payload: Raw request body (not used for GitLab)
            headers: Request headers
            secret: Webhook secret

        Returns:
            True if token is valid
        """
        token = headers.get("x-gitlab-token", headers.get("X-Gitlab-Token", ""))

        if not secret:
            logger.warning("No webhook secret configured, skipping verification")
            return True

        import hmac
        return hmac.compare_digest(token, secret)

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
    ) -> WebhookEvent | None:
        """Parse GitLab webhook into WebhookEvent.

        Args:
            payload: Parsed JSON payload
            headers: Request headers

        Returns:
            WebhookEvent or None
        """
        object_kind = payload.get("object_kind", "")
        project = payload.get("project", {})
        project_id = project.get("id")

        if not project_id:
            return None

        # Determine action from object_attributes if present
        action = payload.get("object_attributes", {}).get("action")

        return WebhookEvent(
            event_type=object_kind,  # merge_request, push, etc.
            external_id=str(project_id),
            action=action,
            data=payload,
            raw_payload=payload,
        )

    # =========================================================================
    # Source Control Specific
    # =========================================================================

    async def get_pr_diff(
        self,
        credentials: dict[str, Any],
        owner: str,
        repo: str,
        pr_number: int,
    ) -> str | None:
        """Get MR diff content.

        For GitLab, we need to look up the project by path.

        Args:
            credentials: GitLab credentials
            owner: Namespace (group or user)
            repo: Project name
            pr_number: MR IID

        Returns:
            Diff content
        """
        client = self._get_client(credentials)
        if not client:
            return None

        # Get project ID from path
        project_path = f"{owner}/{repo}"
        project_id = await self._get_project_id(credentials, project_path)

        if not project_id:
            logger.error(f"Could not find project: {project_path}")
            return None

        mr_info = MRInfo(
            project_id=project_id,
            mr_iid=pr_number,
            title="",
            head_sha="",
            source_branch="",
            target_branch="",
            author="",
            namespace=owner,
            project_path=project_path,
            diff_url="",
        )

        return client.get_mr_diff(mr_info)

    async def _get_project_id(
        self,
        credentials: dict[str, Any],
        project_path: str,
    ) -> int | None:
        """Get GitLab project ID from path.

        Args:
            credentials: GitLab credentials
            project_path: Full project path (namespace/project)

        Returns:
            Project ID or None
        """
        import json
        from urllib.parse import quote
        from urllib.request import Request, urlopen

        gitlab_url = credentials.get("gitlab_url", self._gitlab_url)
        access_token = credentials.get("access_token") or credentials.get("private_token")

        url = f"{gitlab_url}/api/v4/projects/{quote(project_path, safe='')}"
        headers = {}

        if access_token:
            headers["Authorization"] = f"Bearer {access_token}"

        request = Request(url, headers=headers)

        try:
            with urlopen(request, timeout=30) as response:
                data = json.loads(response.read())
                return data.get("id")
        except Exception as e:
            logger.error(f"Failed to get project ID: {e}")
            return None

    async def create_review(
        self,
        credentials: dict[str, Any],
        owner: str,
        repo: str,
        pr_number: int,
        body: str,
        event: str = "COMMENT",
        comments: list[dict[str, Any]] | None = None,
    ) -> bool:
        """Create an MR review (as note or discussion).

        GitLab doesn't have a concept of "review" like GitHub.
        We create a note for the overall review body.

        Args:
            credentials: GitLab credentials
            owner: Namespace
            repo: Project name
            pr_number: MR IID
            body: Review body
            event: Review event (mapped to GitLab concepts)
            comments: Line comments (created as discussions)

        Returns:
            True if successful
        """
        client = self._get_client(credentials)
        if not client:
            return False

        # Get project ID
        project_path = f"{owner}/{repo}"
        project_id = await self._get_project_id(credentials, project_path)

        if not project_id:
            return False

        mr_info = MRInfo(
            project_id=project_id,
            mr_iid=pr_number,
            title="",
            head_sha="",
            source_branch="",
            target_branch="",
            author="",
            namespace=owner,
            project_path=project_path,
            diff_url="",
        )

        # Post main review body
        result = client.create_mr_note(mr_info, body)
        if not result.success:
            return False

        # Post line comments as discussions if provided
        if comments:
            for comment in comments:
                # GitLab needs position info for line comments
                position = {
                    "base_sha": "",  # Will be filled by API
                    "start_sha": "",
                    "head_sha": mr_info.head_sha,
                    "position_type": "text",
                    "new_path": comment.get("path", ""),
                    "new_line": comment.get("line", 1),
                }

                client.create_discussion(
                    mr_info,
                    comment.get("body", ""),
                    position=position,
                )

        return True

    # =========================================================================
    # Helper Methods
    # =========================================================================

    def to_merge_request_info(self, mr_info: MRInfo) -> MergeRequestInfo:
        """Convert GitLab MRInfo to provider-agnostic MergeRequestInfo.

        Args:
            mr_info: GitLab-specific MR info

        Returns:
            Provider-agnostic MergeRequestInfo
        """
        parts = mr_info.project_path.rsplit("/", 1)
        owner = parts[0] if len(parts) > 1 else mr_info.namespace
        repo = parts[1] if len(parts) > 1 else mr_info.project_path

        return MergeRequestInfo(
            provider="gitlab",
            owner=owner,
            repo=repo,
            number=mr_info.mr_iid,
            title=mr_info.title,
            head_sha=mr_info.head_sha,
            base_branch=mr_info.target_branch,
            head_branch=mr_info.source_branch,
            author=str(mr_info.author),
            diff_url=mr_info.diff_url,
            project_id=mr_info.project_id,
        )
