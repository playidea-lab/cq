"""Tests for Webhook API Routes.

Tests for:
- GitHub webhook signature verification
- PR event handling
- Ping event handling
- Installation event handling
- Error handling
"""

import hashlib
import hmac
import json
from unittest.mock import MagicMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from c4.api.routes.webhooks import router
from c4.integrations.github_app import PRInfo

# =============================================================================
# Test App Setup
# =============================================================================


@pytest.fixture
def app():
    """Create FastAPI test app with webhook router."""
    app = FastAPI()
    app.include_router(router)
    return app


@pytest.fixture
def client(app):
    """Create test client."""
    return TestClient(app)


@pytest.fixture
def webhook_secret():
    """Webhook secret for testing."""
    return "test-webhook-secret"


@pytest.fixture
def sample_pr_payload():
    """Sample PR webhook payload."""
    return {
        "action": "opened",
        "pull_request": {
            "number": 42,
            "title": "Add new feature",
            "head": {"sha": "abc123", "ref": "feature-branch"},
            "base": {"ref": "main"},
            "user": {"login": "developer"},
            "diff_url": "https://github.com/owner/repo/pull/42.diff",
        },
        "repository": {"full_name": "owner/repo"},
        "installation": {"id": 12345},
    }


def create_signature(payload: dict, secret: str) -> str:
    """Create valid webhook signature."""
    body = json.dumps(payload).encode()
    mac = hmac.new(secret.encode(), body, hashlib.sha256)
    return f"sha256={mac.hexdigest()}"


# =============================================================================
# Signature Verification Tests
# =============================================================================


class TestWebhookSignature:
    """Test webhook signature verification."""

    def test_valid_signature_accepted(self, client, webhook_secret, sample_pr_payload):
        """Test that valid signature is accepted."""
        signature = create_signature(sample_pr_payload, webhook_secret)

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_gh.parse_pr_webhook.return_value = None  # Skip review
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": signature,
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200

    def test_invalid_signature_rejected(self, client, sample_pr_payload):
        """Test that invalid signature is rejected with 401."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = False
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=invalid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 401
        assert "Invalid webhook signature" in response.json()["detail"]

    def test_missing_signature_header_fails(self, client, sample_pr_payload):
        """Test that missing signature header fails."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_client.return_value = MagicMock()

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-GitHub-Delivery": "test-delivery-id",
                    # Missing X-Hub-Signature-256
                },
            )

        assert response.status_code == 422  # Validation error


# =============================================================================
# PR Event Tests
# =============================================================================


class TestPREvents:
    """Test PR webhook event handling."""

    @pytest.mark.parametrize("action", ["opened", "synchronize", "reopened"])
    def test_pr_actions_queue_review(self, client, sample_pr_payload, action):
        """Test that PR actions queue a review."""
        sample_pr_payload["action"] = action

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_gh.parse_pr_webhook.return_value = MagicMock(spec=PRInfo)
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] == "review_queued"
        assert "queued" in data["message"].lower()

    @pytest.mark.parametrize("action", ["closed", "labeled", "assigned", "edited"])
    def test_ignored_pr_actions(self, client, sample_pr_payload, action):
        """Test that non-review PR actions are ignored."""
        sample_pr_payload["action"] = action

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] is None
        assert f"Ignored PR action: {action}" in data["message"]


# =============================================================================
# Ping Event Tests
# =============================================================================


class TestPingEvent:
    """Test ping webhook event."""

    def test_ping_returns_pong(self, client):
        """Test that ping event returns pong."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json={"zen": "Test zen", "hook_id": 12345},
                headers={
                    "X-GitHub-Event": "ping",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] == "ping"
        assert "Pong" in data["message"]


# =============================================================================
# Installation Event Tests
# =============================================================================


class TestInstallationEvent:
    """Test installation webhook events."""

    @pytest.mark.parametrize("action", ["created", "deleted", "suspend", "unsuspend"])
    def test_installation_events(self, client, action):
        """Test installation event handling."""
        payload = {
            "action": action,
            "installation": {"id": 12345, "account": {"login": "org"}},
        }

        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json=payload,
                headers={
                    "X-GitHub-Event": "installation",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] == f"installation_{action}"


# =============================================================================
# Error Handling Tests
# =============================================================================


class TestErrorHandling:
    """Test error handling in webhook routes."""

    def test_unconfigured_github_app(self, client, sample_pr_payload):
        """Test error when GitHub App is not configured."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_client.return_value = None  # Not configured

            response = client.post(
                "/webhooks/github",
                json=sample_pr_payload,
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 503
        assert "not configured" in response.json()["detail"].lower()

    def test_invalid_json_payload(self, client):
        """Test error on invalid JSON payload."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                content=b"not valid json",
                headers={
                    "X-GitHub-Event": "pull_request",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                    "Content-Type": "application/json",
                },
            )

        assert response.status_code == 400
        assert "Invalid JSON" in response.json()["detail"]

    def test_unknown_event_type_handled(self, client):
        """Test that unknown event types are acknowledged."""
        with patch("c4.api.routes.webhooks.get_github_app_client") as mock_client:
            mock_gh = MagicMock()
            mock_gh.verify_webhook_signature.return_value = True
            mock_client.return_value = mock_gh

            response = client.post(
                "/webhooks/github",
                json={"action": "test"},
                headers={
                    "X-GitHub-Event": "unknown_event_type",
                    "X-Hub-Signature-256": "sha256=valid",
                    "X-GitHub-Delivery": "test-delivery-id",
                },
            )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["action"] is None
        assert "not handled" in data["message"].lower()


# =============================================================================
# Status Endpoint Tests
# =============================================================================


class TestStatusEndpoint:
    """Test webhook status endpoint."""

    def test_status_when_configured(self, client):
        """Test status endpoint when fully configured."""
        with patch("c4.api.routes.webhooks.GitHubAppConfig") as mock_config_class:
            mock_config = MagicMock()
            mock_config.is_configured.return_value = True
            mock_config.get_app_id.return_value = "123456"
            mock_config.get_private_key.return_value = "key"
            mock_config.get_webhook_secret.return_value = "secret"
            mock_config.review_enabled = True
            mock_config.review_model = "claude-sonnet-4-20250514"
            mock_config.max_diff_size = 50000
            mock_config_class.return_value = mock_config

            response = client.get("/webhooks/github/status")

        assert response.status_code == 200
        data = response.json()
        assert data["configured"] is True
        assert data["app_id_set"] is True
        assert data["private_key_set"] is True
        assert data["webhook_secret_set"] is True

    def test_status_when_not_configured(self, client):
        """Test status endpoint when not configured."""
        with patch("c4.api.routes.webhooks.GitHubAppConfig") as mock_config_class:
            mock_config = MagicMock()
            mock_config.is_configured.return_value = False
            mock_config.get_app_id.return_value = None
            mock_config.get_private_key.return_value = None
            mock_config.get_webhook_secret.return_value = None
            mock_config.review_enabled = True
            mock_config.review_model = "claude-sonnet-4-20250514"
            mock_config.max_diff_size = 50000
            mock_config_class.return_value = mock_config

            response = client.get("/webhooks/github/status")

        assert response.status_code == 200
        data = response.json()
        assert data["configured"] is False
        assert data["app_id_set"] is False
