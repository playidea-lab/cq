"""GitHub App Integration - Webhook handling and App authentication.

This module provides:
- GitHub App JWT generation
- Installation token management
- Webhook signature verification (HMAC-SHA256)
- PR diff retrieval and review comment creation
"""

from __future__ import annotations

import hashlib
import hmac
import json
import logging
import time
from dataclasses import dataclass
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

logger = logging.getLogger(__name__)


# =============================================================================
# Result Types
# =============================================================================


@dataclass
class WebhookResult:
    """Result of webhook processing."""

    success: bool
    message: str
    action_taken: str | None = None
    data: dict[str, Any] | None = None


@dataclass
class PRInfo:
    """Pull request information extracted from webhook payload."""

    owner: str
    repo: str
    number: int
    title: str
    head_sha: str
    base_branch: str
    head_branch: str
    author: str
    diff_url: str
    installation_id: int


@dataclass
class ReviewComment:
    """A review comment to be posted."""

    path: str
    line: int
    body: str
    side: str = "RIGHT"  # LEFT or RIGHT


@dataclass
class ReviewResult:
    """Result of creating a PR review."""

    success: bool
    message: str
    review_id: int | None = None
    comments_posted: int = 0


# =============================================================================
# GitHub App Client
# =============================================================================


class GitHubAppClient:
    """GitHub App client for webhook handling and API operations.

    This client handles:
    1. Webhook signature verification
    2. JWT generation for App authentication
    3. Installation token acquisition
    4. PR operations (get diff, create review)
    """

    API_BASE = "https://api.github.com"
    JWT_EXPIRY_SECONDS = 600  # 10 minutes
    TOKEN_CACHE_MARGIN = 300  # Refresh token 5 minutes before expiry

    def __init__(
        self,
        app_id: str,
        private_key: str,
        webhook_secret: str,
    ) -> None:
        """Initialize GitHub App client.

        Args:
            app_id: GitHub App ID
            private_key: Private key in PEM format
            webhook_secret: Webhook secret for signature verification
        """
        self.app_id = app_id
        self.private_key = private_key
        self.webhook_secret = webhook_secret

        # Cache for installation tokens: {installation_id: (token, expiry_time)}
        self._token_cache: dict[int, tuple[str, float]] = {}

    # =========================================================================
    # Webhook Verification
    # =========================================================================

    def verify_webhook_signature(self, payload: bytes, signature: str) -> bool:
        """Verify webhook signature using HMAC-SHA256.

        Args:
            payload: Raw request body as bytes
            signature: X-Hub-Signature-256 header value

        Returns:
            True if signature is valid, False otherwise
        """
        if not signature.startswith("sha256="):
            logger.warning("Invalid signature format: missing sha256= prefix")
            return False

        expected_signature = signature[7:]  # Remove "sha256=" prefix

        # Compute HMAC-SHA256
        mac = hmac.new(
            self.webhook_secret.encode("utf-8"),
            payload,
            hashlib.sha256,
        )
        computed_signature = mac.hexdigest()

        # Constant-time comparison to prevent timing attacks
        return hmac.compare_digest(computed_signature, expected_signature)

    # =========================================================================
    # JWT Generation
    # =========================================================================

    def _generate_jwt(self) -> str:
        """Generate a JWT for GitHub App authentication.

        Uses PyJWT if available, otherwise falls back to manual construction.

        Returns:
            JWT string for Authorization header
        """
        try:
            import jwt

            now = int(time.time())
            payload = {
                "iat": now - 60,  # Issued 60 seconds ago (clock skew tolerance)
                "exp": now + self.JWT_EXPIRY_SECONDS,
                "iss": self.app_id,
            }

            return jwt.encode(payload, self.private_key, algorithm="RS256")

        except ImportError:
            # Fallback to manual JWT construction
            return self._generate_jwt_manual()

    def _generate_jwt_manual(self) -> str:
        """Generate JWT without PyJWT library.

        This is a fallback implementation using only standard library.
        """
        import base64
        import json

        def b64url_encode(data: bytes) -> str:
            return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

        now = int(time.time())

        # Header
        header = {"alg": "RS256", "typ": "JWT"}
        header_b64 = b64url_encode(json.dumps(header).encode())

        # Payload
        payload = {
            "iat": now - 60,
            "exp": now + self.JWT_EXPIRY_SECONDS,
            "iss": self.app_id,
        }
        payload_b64 = b64url_encode(json.dumps(payload).encode())

        # Message to sign
        message = f"{header_b64}.{payload_b64}"

        # Sign with RSA
        try:
            from cryptography.hazmat.primitives import hashes, serialization
            from cryptography.hazmat.primitives.asymmetric import padding

            private_key = serialization.load_pem_private_key(
                self.private_key.encode(),
                password=None,
            )
            signature = private_key.sign(  # type: ignore
                message.encode(),
                padding.PKCS1v15(),
                hashes.SHA256(),
            )
            signature_b64 = b64url_encode(signature)

            return f"{message}.{signature_b64}"

        except ImportError:
            raise ImportError(
                "Either 'PyJWT' or 'cryptography' package is required for GitHub App authentication. Install with: uv add PyJWT"
            )

    # =========================================================================
    # Installation Token Management
    # =========================================================================

    def _get_installation_token(self, installation_id: int) -> str:
        """Get or refresh installation access token.

        Tokens are cached and automatically refreshed before expiry.

        Args:
            installation_id: GitHub App installation ID

        Returns:
            Installation access token
        """
        # Check cache
        if installation_id in self._token_cache:
            token, expiry = self._token_cache[installation_id]
            if time.time() < expiry - self.TOKEN_CACHE_MARGIN:
                return token

        # Request new token
        jwt_token = self._generate_jwt()
        url = f"{self.API_BASE}/app/installations/{installation_id}/access_tokens"

        request = Request(
            url,
            method="POST",
            headers={
                "Authorization": f"Bearer {jwt_token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
            },
        )

        try:
            with urlopen(request, timeout=30) as response:
                data = json.loads(response.read())
                token = data["token"]
                # Parse expiry (ISO format) and convert to timestamp
                # GitHub returns expires_at like "2024-01-21T12:00:00Z"
                expiry_str = data.get("expires_at", "")
                if expiry_str:
                    from datetime import datetime

                    expiry = datetime.fromisoformat(expiry_str.replace("Z", "+00:00")).timestamp()
                else:
                    expiry = time.time() + 3600  # Default 1 hour

                self._token_cache[installation_id] = (token, expiry)
                logger.debug(f"Acquired installation token for {installation_id}")
                return token

        except (HTTPError, URLError) as e:
            logger.error(f"Failed to get installation token: {e}")
            raise

    # =========================================================================
    # API Request Helpers
    # =========================================================================

    def _api_request(
        self,
        method: str,
        url: str,
        installation_id: int,
        data: dict[str, Any] | None = None,
        accept: str = "application/vnd.github+json",
    ) -> tuple[int, dict[str, Any] | str]:
        """Make an authenticated API request.

        Args:
            method: HTTP method
            url: Full URL or path (will be prefixed with API_BASE)
            installation_id: Installation ID for authentication
            data: Request body (will be JSON encoded)
            accept: Accept header value

        Returns:
            Tuple of (status_code, response_data)
        """
        if not url.startswith("http"):
            url = f"{self.API_BASE}{url}"

        token = self._get_installation_token(installation_id)

        headers = {
            "Authorization": f"Bearer {token}",
            "Accept": accept,
            "X-GitHub-Api-Version": "2022-11-28",
        }

        body = None
        if data:
            body = json.dumps(data).encode("utf-8")
            headers["Content-Type"] = "application/json"

        request = Request(url, data=body, headers=headers, method=method)

        try:
            with urlopen(request, timeout=60) as response:
                content = response.read()
                if accept == "application/vnd.github.diff":
                    return response.status, content.decode("utf-8")
                return response.status, json.loads(content)

        except HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            logger.error(f"API request failed: {e.code} {error_body}")
            try:
                return e.code, json.loads(error_body)
            except json.JSONDecodeError:
                return e.code, {"error": error_body}

        except URLError as e:
            logger.error(f"API request error: {e}")
            return 0, {"error": str(e)}

    # =========================================================================
    # Webhook Payload Parsing
    # =========================================================================

    def parse_pr_webhook(self, payload: dict[str, Any]) -> PRInfo | None:
        """Parse PR information from webhook payload.

        Args:
            payload: Webhook payload dictionary

        Returns:
            PRInfo if valid PR event, None otherwise
        """
        action = payload.get("action")
        pr_data = payload.get("pull_request")
        installation = payload.get("installation", {})

        if not pr_data:
            return None

        # Only handle specific actions
        if action not in ("opened", "synchronize", "reopened"):
            logger.debug(f"Ignoring PR action: {action}")
            return None

        repo = payload.get("repository", {})
        full_name = repo.get("full_name", "")
        owner, repo_name = full_name.split("/") if "/" in full_name else ("", "")

        return PRInfo(
            owner=owner,
            repo=repo_name,
            number=pr_data.get("number", 0),
            title=pr_data.get("title", ""),
            head_sha=pr_data.get("head", {}).get("sha", ""),
            base_branch=pr_data.get("base", {}).get("ref", ""),
            head_branch=pr_data.get("head", {}).get("ref", ""),
            author=pr_data.get("user", {}).get("login", ""),
            diff_url=pr_data.get("diff_url", ""),
            installation_id=installation.get("id", 0),
        )

    # =========================================================================
    # PR Operations
    # =========================================================================

    def get_pr_diff(self, pr_info: PRInfo) -> str | None:
        """Get the diff content of a PR.

        Args:
            pr_info: PR information

        Returns:
            Diff content as string, or None on error
        """
        url = f"/repos/{pr_info.owner}/{pr_info.repo}/pulls/{pr_info.number}"

        status, data = self._api_request(
            "GET",
            url,
            pr_info.installation_id,
            accept="application/vnd.github.diff",
        )

        if status == 200 and isinstance(data, str):
            return data

        logger.error(f"Failed to get PR diff: {status}")
        return None

    def get_pr_files(self, pr_info: PRInfo) -> list[dict[str, Any]]:
        """Get list of changed files in a PR.

        Args:
            pr_info: PR information

        Returns:
            List of file change objects
        """
        url = f"/repos/{pr_info.owner}/{pr_info.repo}/pulls/{pr_info.number}/files"

        status, data = self._api_request(
            "GET",
            url,
            pr_info.installation_id,
        )

        if status == 200 and isinstance(data, list):
            return data

        logger.error(f"Failed to get PR files: {status}")
        return []

    def create_review(
        self,
        pr_info: PRInfo,
        body: str,
        event: str = "COMMENT",
        comments: list[ReviewComment] | None = None,
    ) -> ReviewResult:
        """Create a PR review with optional line comments.

        Args:
            pr_info: PR information
            body: Main review body
            event: Review event type (COMMENT, APPROVE, REQUEST_CHANGES)
            comments: Optional list of line comments

        Returns:
            ReviewResult with status
        """
        url = f"/repos/{pr_info.owner}/{pr_info.repo}/pulls/{pr_info.number}/reviews"

        review_data: dict[str, Any] = {
            "commit_id": pr_info.head_sha,
            "body": body,
            "event": event,
        }

        if comments:
            review_data["comments"] = [
                {
                    "path": c.path,
                    "line": c.line,
                    "body": c.body,
                    "side": c.side,
                }
                for c in comments
            ]

        status, data = self._api_request(
            "POST",
            url,
            pr_info.installation_id,
            data=review_data,
        )

        if status == 200 and isinstance(data, dict):
            return ReviewResult(
                success=True,
                message="Review created successfully",
                review_id=data.get("id"),
                comments_posted=len(comments) if comments else 0,
            )

        error_msg = data.get("message", "Unknown error") if isinstance(data, dict) else str(data)
        return ReviewResult(
            success=False,
            message=f"Failed to create review: {error_msg}",
        )

    def add_labels(
        self,
        pr_info: PRInfo,
        labels: list[str],
    ) -> bool:
        """Add labels to a PR.

        Args:
            pr_info: PR information
            labels: Labels to add

        Returns:
            True if successful
        """
        url = f"/repos/{pr_info.owner}/{pr_info.repo}/issues/{pr_info.number}/labels"

        status, _ = self._api_request(
            "POST",
            url,
            pr_info.installation_id,
            data={"labels": labels},
        )

        return status == 200

    def create_check_run(
        self,
        pr_info: PRInfo,
        name: str,
        status: str,
        conclusion: str | None = None,
        output: dict[str, str] | None = None,
    ) -> bool:
        """Create a check run for the PR.

        Args:
            pr_info: PR information
            name: Check run name
            status: Status (queued, in_progress, completed)
            conclusion: Conclusion (success, failure, neutral, etc.)
            output: Optional output with title and summary

        Returns:
            True if successful
        """
        url = f"/repos/{pr_info.owner}/{pr_info.repo}/check-runs"

        data: dict[str, Any] = {
            "name": name,
            "head_sha": pr_info.head_sha,
            "status": status,
        }

        if conclusion:
            data["conclusion"] = conclusion

        if output:
            data["output"] = output

        resp_status, _ = self._api_request(
            "POST",
            url,
            pr_info.installation_id,
            data=data,
        )

        return resp_status == 201
