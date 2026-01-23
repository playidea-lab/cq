"""GitHub Integration Provider.

Implements the IntegrationProvider interface for GitHub App integration.
This wraps the existing github_app.py functionality with the standardized interface.
"""

from __future__ import annotations

import hashlib
import hmac
import logging
import os
from typing import Any
from urllib.parse import urlencode

from .base import (
    ConnectionResult,
    IntegrationCapability,
    NotificationResult,
    SourceControlProvider,
    WebhookEvent,
)
from .github_app import GitHubAppClient
from .registry import IntegrationRegistry

logger = logging.getLogger(__name__)


@IntegrationRegistry.register
class GitHubProvider(SourceControlProvider):
    """GitHub App integration provider.

    Supports:
    - PR review via webhooks
    - OAuth installation flow
    - Notifications via PR comments
    - Check runs for status reporting

    Environment Variables:
        GITHUB_APP_ID: GitHub App ID
        GITHUB_APP_PRIVATE_KEY: Private key content (PEM format)
        GITHUB_APP_CLIENT_ID: OAuth Client ID
        GITHUB_APP_CLIENT_SECRET: OAuth Client Secret
        GITHUB_WEBHOOK_SECRET: Webhook secret
    """

    OAUTH_AUTHORIZE_URL = "https://github.com/apps/{app_name}/installations/new"
    OAUTH_TOKEN_URL = "https://github.com/login/oauth/access_token"
    API_BASE = "https://api.github.com"

    def __init__(self) -> None:
        """Initialize GitHub provider."""
        self._client: GitHubAppClient | None = None

    # =========================================================================
    # Provider Identity
    # =========================================================================

    @property
    def id(self) -> str:
        return "github"

    @property
    def name(self) -> str:
        return "GitHub"

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
        return "GitHub integration for PR reviews, code analysis, and notifications"

    @property
    def icon_url(self) -> str | None:
        return "https://github.githubassets.com/images/modules/logos_page/GitHub-Mark.png"

    @property
    def docs_url(self) -> str | None:
        return "https://docs.github.com/en/apps"

    # =========================================================================
    # Client Management
    # =========================================================================

    def _get_client(self, credentials: dict[str, Any] | None = None) -> GitHubAppClient | None:
        """Get or create GitHub App client.

        Args:
            credentials: Optional credentials override

        Returns:
            GitHubAppClient or None if not configured
        """
        if credentials:
            return GitHubAppClient(
                app_id=credentials.get("app_id", ""),
                private_key=credentials.get("private_key", ""),
                webhook_secret=credentials.get("webhook_secret", ""),
            )

        # Use environment variables
        app_id = os.environ.get("GITHUB_APP_ID")
        private_key = os.environ.get("GITHUB_APP_PRIVATE_KEY")
        webhook_secret = os.environ.get("GITHUB_WEBHOOK_SECRET")

        if not all([app_id, private_key, webhook_secret]):
            return None

        if self._client is None:
            self._client = GitHubAppClient(
                app_id=app_id,  # type: ignore
                private_key=private_key,  # type: ignore
                webhook_secret=webhook_secret,  # type: ignore
            )

        return self._client

    # =========================================================================
    # OAuth Flow
    # =========================================================================

    def get_oauth_url(self, state: str) -> str:
        """Get GitHub App installation URL.

        For GitHub Apps, this directs to the app installation page.

        Args:
            state: State parameter (contains workspace_id, user_id)

        Returns:
            Installation URL
        """
        app_name = os.environ.get("GITHUB_APP_NAME", "c4-code-review")
        params = {"state": state}
        return f"{self.OAUTH_AUTHORIZE_URL.format(app_name=app_name)}?{urlencode(params)}"

    async def exchange_code(self, code: str, state: str) -> ConnectionResult:
        """Exchange installation callback for connection.

        For GitHub Apps, the "code" is actually the installation_id
        passed via the callback after app installation.

        Args:
            code: Installation ID from callback
            state: State parameter for verification

        Returns:
            ConnectionResult with installation details
        """
        try:
            installation_id = int(code)
        except ValueError:
            return ConnectionResult(
                success=False,
                message="Invalid installation ID",
                error_code="invalid_installation_id",
            )

        # Get installation details
        client = self._get_client()
        if not client:
            return ConnectionResult(
                success=False,
                message="GitHub App not configured",
                error_code="not_configured",
            )

        # Fetch installation info to get repository/organization name
        try:
            import json
            from urllib.request import Request, urlopen

            jwt_token = client._generate_jwt()
            url = f"{self.API_BASE}/app/installations/{installation_id}"

            request = Request(
                url,
                headers={
                    "Authorization": f"Bearer {jwt_token}",
                    "Accept": "application/vnd.github+json",
                    "X-GitHub-Api-Version": "2022-11-28",
                },
            )

            with urlopen(request, timeout=30) as response:
                data = json.loads(response.read())
                account = data.get("account", {})

                return ConnectionResult(
                    success=True,
                    message="GitHub App installed successfully",
                    external_id=str(installation_id),
                    external_name=account.get("login", f"installation-{installation_id}"),
                    credentials={
                        "installation_id": installation_id,
                        "account_type": data.get("target_type", "User"),
                        "account_login": account.get("login"),
                        "account_id": account.get("id"),
                    },
                )

        except Exception as e:
            logger.error(f"Failed to get installation details: {e}")
            return ConnectionResult(
                success=False,
                message=f"Failed to verify installation: {e}",
                error_code="verification_failed",
            )

    # =========================================================================
    # Connection Management
    # =========================================================================

    async def connect(
        self,
        workspace_id: str,
        credentials: dict[str, Any],
    ) -> ConnectionResult:
        """Connect GitHub installation to workspace.

        Args:
            workspace_id: C4 workspace ID
            credentials: Installation credentials

        Returns:
            ConnectionResult
        """
        installation_id = credentials.get("installation_id")
        if not installation_id:
            return ConnectionResult(
                success=False,
                message="Missing installation_id in credentials",
                error_code="missing_installation_id",
            )

        return ConnectionResult(
            success=True,
            message="GitHub connected",
            external_id=str(installation_id),
            external_name=credentials.get("account_login", str(installation_id)),
            credentials=credentials,
        )

    async def disconnect(
        self,
        workspace_id: str,
        external_id: str,
    ) -> bool:
        """Disconnect GitHub installation.

        Note: This doesn't uninstall the GitHub App from the user's account,
        just removes the association in C4.

        Args:
            workspace_id: C4 workspace ID
            external_id: Installation ID

        Returns:
            True (always succeeds on our side)
        """
        logger.info(f"Disconnecting GitHub installation {external_id} from workspace {workspace_id}")
        return True

    async def validate_connection(
        self,
        credentials: dict[str, Any],
    ) -> bool:
        """Validate GitHub installation is still active.

        Args:
            credentials: Stored credentials

        Returns:
            True if installation is valid
        """
        installation_id = credentials.get("installation_id")
        if not installation_id:
            return False

        client = self._get_client()
        if not client:
            return False

        try:
            # Try to get an installation token
            token = client._get_installation_token(int(installation_id))
            return bool(token)
        except Exception as e:
            logger.warning(f"GitHub installation validation failed: {e}")
            return False

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
        """Send notification via PR comment.

        For GitHub, notifications are sent as PR comments.
        The channel_id should be in format: owner/repo#pr_number

        Args:
            credentials: Installation credentials
            message: Comment body (markdown supported)
            channel_id: "owner/repo#pr_number" format
            metadata: Additional options (e.g., review_event)

        Returns:
            NotificationResult
        """
        if not channel_id:
            return NotificationResult(
                success=False,
                message="channel_id required in format: owner/repo#pr_number",
            )

        # Parse channel_id
        try:
            repo_part, pr_part = channel_id.split("#")
            owner, repo = repo_part.split("/")
            pr_number = int(pr_part)
        except ValueError:
            return NotificationResult(
                success=False,
                message=f"Invalid channel_id format: {channel_id}",
            )

        installation_id = credentials.get("installation_id")
        if not installation_id:
            return NotificationResult(
                success=False,
                message="Missing installation_id",
            )

        client = self._get_client()
        if not client:
            return NotificationResult(
                success=False,
                message="GitHub App not configured",
            )

        # Create a comment on the PR
        try:
            from .github_app import PRInfo

            pr_info = PRInfo(
                owner=owner,
                repo=repo,
                number=pr_number,
                title="",
                head_sha="",
                base_branch="",
                head_branch="",
                author="",
                diff_url="",
                installation_id=int(installation_id),
            )

            # If metadata indicates this should be a review, create review
            if metadata and metadata.get("as_review"):
                result = client.create_review(
                    pr_info=pr_info,
                    body=message,
                    event=metadata.get("review_event", "COMMENT"),
                )
                return NotificationResult(
                    success=result.success,
                    message=result.message,
                    message_id=str(result.review_id) if result.review_id else None,
                )

            # Otherwise, create an issue comment
            status, data = client._api_request(
                "POST",
                f"/repos/{owner}/{repo}/issues/{pr_number}/comments",
                installation_id,
                data={"body": message},
            )

            if status == 201 and isinstance(data, dict):
                return NotificationResult(
                    success=True,
                    message="Comment posted",
                    message_id=str(data.get("id")),
                )

            return NotificationResult(
                success=False,
                message=f"Failed to post comment: {data}",
            )

        except Exception as e:
            logger.error(f"Failed to send GitHub notification: {e}")
            return NotificationResult(
                success=False,
                message=str(e),
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
        """Verify GitHub webhook signature.

        Args:
            payload: Raw request body
            headers: Request headers
            secret: Webhook secret

        Returns:
            True if signature is valid
        """
        signature = headers.get("x-hub-signature-256", headers.get("X-Hub-Signature-256", ""))

        if not signature.startswith("sha256="):
            return False

        expected_signature = signature[7:]
        mac = hmac.new(secret.encode("utf-8"), payload, hashlib.sha256)
        computed_signature = mac.hexdigest()

        return hmac.compare_digest(computed_signature, expected_signature)

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
    ) -> WebhookEvent | None:
        """Parse GitHub webhook into WebhookEvent.

        Args:
            payload: Parsed JSON payload
            headers: Request headers

        Returns:
            WebhookEvent or None
        """
        event_type = headers.get("x-github-event", headers.get("X-GitHub-Event", ""))
        action = payload.get("action")
        installation = payload.get("installation", {})
        installation_id = installation.get("id")

        if not installation_id:
            return None

        return WebhookEvent(
            event_type=event_type,
            external_id=str(installation_id),
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
        """Get PR diff content.

        Args:
            credentials: Installation credentials
            owner: Repository owner
            repo: Repository name
            pr_number: PR number

        Returns:
            Diff content
        """
        installation_id = credentials.get("installation_id")
        if not installation_id:
            return None

        client = self._get_client()
        if not client:
            return None

        from .github_app import PRInfo

        pr_info = PRInfo(
            owner=owner,
            repo=repo,
            number=pr_number,
            title="",
            head_sha="",
            base_branch="",
            head_branch="",
            author="",
            diff_url="",
            installation_id=int(installation_id),
        )

        return client.get_pr_diff(pr_info)

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
        """Create a PR review.

        Args:
            credentials: Installation credentials
            owner: Repository owner
            repo: Repository name
            pr_number: PR number
            body: Review body
            event: Review event
            comments: Line comments

        Returns:
            True if successful
        """
        installation_id = credentials.get("installation_id")
        if not installation_id:
            return False

        client = self._get_client()
        if not client:
            return False

        from .github_app import PRInfo, ReviewComment

        # Need to get the PR head SHA
        status, data = client._api_request(
            "GET",
            f"/repos/{owner}/{repo}/pulls/{pr_number}",
            int(installation_id),
        )

        if status != 200 or not isinstance(data, dict):
            return False

        head_sha = data.get("head", {}).get("sha", "")

        pr_info = PRInfo(
            owner=owner,
            repo=repo,
            number=pr_number,
            title=data.get("title", ""),
            head_sha=head_sha,
            base_branch=data.get("base", {}).get("ref", ""),
            head_branch=data.get("head", {}).get("ref", ""),
            author=data.get("user", {}).get("login", ""),
            diff_url=data.get("diff_url", ""),
            installation_id=int(installation_id),
        )

        review_comments = None
        if comments:
            review_comments = [
                ReviewComment(
                    path=c["path"],
                    line=c["line"],
                    body=c["body"],
                    side=c.get("side", "RIGHT"),
                )
                for c in comments
            ]

        result = client.create_review(
            pr_info=pr_info,
            body=body,
            event=event,
            comments=review_comments,
        )

        return result.success
