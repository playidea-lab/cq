"""Integration tests for GitHub App.

Tests the complete flow from webhook receipt to PR review creation.
Uses mocked GitHub API and LLM to test integration without external dependencies.
"""

import hashlib
import hmac
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from c4.api.routes.webhooks import router
from c4.config.github_app import GitHubAppConfig, GitHubAppCredentials
from c4.integrations.github_app import GitHubAppClient, PRInfo, ReviewResult
from c4.services.pr_review import PRReviewService

# =============================================================================
# Test Configuration
# =============================================================================


@pytest.fixture
def webhook_secret():
    """Webhook secret for testing."""
    return "integration-test-secret"


@pytest.fixture
def github_config(webhook_secret):
    """GitHub App configuration for testing."""
    return GitHubAppConfig(
        enabled=True,
        app_id="12345",
        private_key="-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
        webhook_secret=webhook_secret,
        review_enabled=True,
        max_diff_size=50000,
        review_model="claude-sonnet-4-20250514",
    )


@pytest.fixture
def github_credentials(github_config):
    """GitHub App credentials for testing."""
    return GitHubAppCredentials.from_config(github_config)


@pytest.fixture
def github_client(github_credentials):
    """GitHub App client for testing."""
    return GitHubAppClient(
        app_id=github_credentials.app_id,
        private_key=github_credentials.private_key,
        webhook_secret=github_credentials.webhook_secret,
    )


@pytest.fixture
def app():
    """Create FastAPI test app."""
    app = FastAPI()
    app.include_router(router)
    return app


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


# =============================================================================
# Helper Functions
# =============================================================================


def create_valid_signature(payload: dict, secret: str) -> str:
    """Create a valid webhook signature."""
    body = json.dumps(payload).encode()
    mac = hmac.new(secret.encode(), body, hashlib.sha256)
    return f"sha256={mac.hexdigest()}"


def create_pr_webhook_payload(
    action: str = "opened",
    pr_number: int = 42,
    title: str = "Test PR",
    author: str = "test-user",
    installation_id: int = 12345,
) -> dict:
    """Create a PR webhook payload."""
    return {
        "action": action,
        "pull_request": {
            "number": pr_number,
            "title": title,
            "head": {"sha": "abc123def456", "ref": "feature-branch"},
            "base": {"ref": "main"},
            "user": {"login": author},
            "diff_url": f"https://github.com/owner/repo/pull/{pr_number}.diff",
        },
        "repository": {"full_name": "owner/repo"},
        "installation": {"id": installation_id},
    }


# =============================================================================
# Configuration Integration Tests
# =============================================================================


class TestConfigurationIntegration:
    """Test GitHub App configuration flow."""

    def test_credentials_from_config(self, github_config):
        """Test creating credentials from valid config."""
        credentials = GitHubAppCredentials.from_config(github_config)

        assert credentials is not None
        assert credentials.app_id == "12345"
        assert "BEGIN RSA PRIVATE KEY" in credentials.private_key
        assert credentials.webhook_secret == "integration-test-secret"

    def test_credentials_from_incomplete_config(self):
        """Test that incomplete config returns None credentials."""
        config = GitHubAppConfig(enabled=True, app_id="123")
        # Missing private_key and webhook_secret

        credentials = GitHubAppCredentials.from_config(config)

        assert credentials is None

    def test_client_from_credentials(self, github_credentials):
        """Test creating client from credentials."""
        client = GitHubAppClient(
            app_id=github_credentials.app_id,
            private_key=github_credentials.private_key,
            webhook_secret=github_credentials.webhook_secret,
        )

        assert client.app_id == "12345"
        assert client.webhook_secret == "integration-test-secret"


# =============================================================================
# Webhook Processing Integration Tests
# =============================================================================


class TestWebhookProcessingIntegration:
    """Test complete webhook processing flow."""

    def test_signature_verification_flow(self, github_client, webhook_secret):
        """Test webhook signature verification."""
        payload = create_pr_webhook_payload()
        body = json.dumps(payload).encode()
        signature = create_valid_signature(payload, webhook_secret)

        # Valid signature should pass
        assert github_client.verify_webhook_signature(body, signature) is True

        # Invalid signature should fail
        assert github_client.verify_webhook_signature(body, "sha256=invalid") is False

    def test_pr_webhook_parsing_flow(self, github_client):
        """Test PR webhook payload parsing."""
        payload = create_pr_webhook_payload(
            action="opened",
            pr_number=123,
            title="Add awesome feature",
            author="developer",
        )

        pr_info = github_client.parse_pr_webhook(payload)

        assert pr_info is not None
        assert pr_info.owner == "owner"
        assert pr_info.repo == "repo"
        assert pr_info.number == 123
        assert pr_info.title == "Add awesome feature"
        assert pr_info.author == "developer"
        assert pr_info.head_sha == "abc123def456"

    def test_ignored_actions_not_parsed(self, github_client):
        """Test that closed/labeled actions return None."""
        for action in ["closed", "labeled", "assigned"]:
            payload = create_pr_webhook_payload(action=action)
            pr_info = github_client.parse_pr_webhook(payload)
            assert pr_info is None


# =============================================================================
# PR Review Service Integration Tests
# =============================================================================


class TestPRReviewServiceIntegration:
    """Test PR review service integration."""

    @pytest.fixture
    def review_service(self, github_client):
        """Create PR review service."""
        return PRReviewService(
            github_client=github_client,
            model="claude-sonnet-4-20250514",
            max_diff_size=50000,
        )

    @pytest.fixture
    def sample_pr_info(self):
        """Sample PR info for testing."""
        return PRInfo(
            owner="owner",
            repo="repo",
            number=42,
            title="Test PR",
            head_sha="abc123",
            base_branch="main",
            head_branch="feature",
            author="developer",
            diff_url="https://github.com/owner/repo/pull/42.diff",
            installation_id=12345,
        )

    @pytest.fixture
    def sample_diff(self):
        """Sample diff content."""
        return """diff --git a/src/main.py b/src/main.py
--- a/src/main.py
+++ b/src/main.py
@@ -1,3 +1,7 @@
+def new_function():
+    \"\"\"New function with docstring.\"\"\"
+    return 42
+
 def main():
     print("Hello")
"""

    @pytest.fixture
    def sample_llm_response(self):
        """Sample LLM response in JSON format."""
        return json.dumps(
            {
                "summary": "Adds a new function that returns 42",
                "overall_quality": "good",
                "issues": [
                    {
                        "severity": "praise",
                        "file_path": "src/main.py",
                        "line": 2,
                        "title": "Good documentation",
                        "description": "Nice docstring!",
                    }
                ],
                "suggestions": [],
                "security_concerns": [],
                "test_coverage": "No tests added",
                "labels": ["enhancement"],
            }
        )

    @pytest.mark.asyncio
    async def test_full_review_flow(self, review_service, sample_pr_info, sample_diff, sample_llm_response):
        """Test complete PR review flow from diff to review creation."""
        # Mock GitHub API calls
        review_service.github_client.get_pr_diff = MagicMock(return_value=sample_diff)
        review_service.github_client.create_review = MagicMock(
            return_value=ReviewResult(
                success=True,
                message="Review created",
                review_id=98765,
                comments_posted=1,
            )
        )
        review_service.github_client.add_labels = MagicMock(return_value=True)

        # Mock LLM call
        with patch.object(review_service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = sample_llm_response

            result = await review_service.review_pr(sample_pr_info)

        # Verify result
        assert result.success is True
        assert result.review_id == 98765

        # Verify GitHub API was called correctly
        review_service.github_client.get_pr_diff.assert_called_once_with(sample_pr_info)
        review_service.github_client.create_review.assert_called_once()

        # Verify labels were added
        review_service.github_client.add_labels.assert_called_once_with(sample_pr_info, ["enhancement"])

    @pytest.mark.asyncio
    async def test_review_with_security_concerns(self, review_service, sample_pr_info, sample_diff):
        """Test that security concerns trigger REQUEST_CHANGES."""
        llm_response = json.dumps(
            {
                "summary": "PR with security issue",
                "overall_quality": "needs_work",
                "issues": [],
                "suggestions": [],
                "security_concerns": ["Potential SQL injection in query builder"],
                "labels": ["security"],
            }
        )

        review_service.github_client.get_pr_diff = MagicMock(return_value=sample_diff)
        review_service.github_client.create_review = MagicMock(return_value=ReviewResult(success=True, message="Review created"))
        review_service.github_client.add_labels = MagicMock(return_value=True)

        with patch.object(review_service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = llm_response

            await review_service.review_pr(sample_pr_info)

        # Verify REQUEST_CHANGES event was used
        call_args = review_service.github_client.create_review.call_args
        assert call_args.kwargs["event"] == "REQUEST_CHANGES"


# =============================================================================
# End-to-End Webhook Flow Tests
# =============================================================================


class TestEndToEndWebhookFlow:
    """Test complete end-to-end webhook processing."""

    def test_webhook_to_review_queue(self, client, webhook_secret):
        """Test that valid webhook queues a review."""
        payload = create_pr_webhook_payload()
        signature = create_valid_signature(payload, webhook_secret)

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_gh.parse_pr_webhook.return_value = MagicMock(spec=PRInfo)
            mock_gh.create_check_run.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": signature,
                    "X-GitHub-Delivery": "test-delivery",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] == "review_queued"

    def test_invalid_signature_blocks_webhook(self, client):
        """Test that invalid signature blocks webhook processing."""
        payload = create_pr_webhook_payload()

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = False
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=invalid",
                    "X-GitHub-Delivery": "test-delivery",
                },
            )

        assert response.status_code == 401

    def test_ping_webhook_succeeds(self, client, webhook_secret):
        """Test ping webhook flow."""
        payload = {"zen": "Keep it simple", "hook_id": 12345}

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=payload,
                headers={
                    "X-GitHub-Event": "ping",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery",
                },
            )

        assert response.status_code == 200
        assert response.json()["action"] == "ping"


# =============================================================================
# Error Recovery Tests
# =============================================================================


class TestErrorRecovery:
    """Test error handling and recovery scenarios."""

    @pytest.mark.asyncio
    async def test_review_continues_after_label_failure(self):
        """Test that review succeeds even if labeling fails."""
        client = MagicMock(spec=GitHubAppClient)
        client.get_pr_diff.return_value = "diff content"
        client.create_review.return_value = ReviewResult(success=True, message="OK")
        client.add_labels.return_value = False  # Label addition fails

        service = PRReviewService(client)

        with patch.object(service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = json.dumps(
                {
                    "summary": "Good PR",
                    "overall_quality": "good",
                    "issues": [],
                    "labels": ["enhancement"],
                }
            )

            pr_info = PRInfo(
                owner="o",
                repo="r",
                number=1,
                title="T",
                head_sha="abc",
                base_branch="main",
                head_branch="feature",
                author="u",
                diff_url="url",
                installation_id=1,
            )

            result = await service.review_pr(pr_info)

        # Review should still succeed
        assert result.success is True

    @pytest.mark.asyncio
    async def test_handles_malformed_llm_response(self):
        """Test handling of malformed LLM response."""
        client = MagicMock(spec=GitHubAppClient)
        client.get_pr_diff.return_value = "diff content"
        client.create_review.return_value = ReviewResult(success=True, message="OK")

        service = PRReviewService(client)

        with patch.object(service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            # Return non-JSON response
            mock_llm.return_value = "This is not valid JSON at all"

            pr_info = PRInfo(
                owner="o",
                repo="r",
                number=1,
                title="T",
                head_sha="abc",
                base_branch="main",
                head_branch="feature",
                author="u",
                diff_url="url",
                installation_id=1,
            )

            result = await service.review_pr(pr_info)

        # Should still create a review with fallback analysis
        assert result.success is True
        client.create_review.assert_called_once()
