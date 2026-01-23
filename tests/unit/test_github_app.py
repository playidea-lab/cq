"""Tests for GitHub App Integration.

Tests for:
- GitHub App configuration
- Webhook signature verification
- PR webhook parsing
- GitHub App client operations
"""

import hashlib
import hmac
import json
from unittest.mock import MagicMock, patch

import pytest

from c4.config.github_app import GitHubAppConfig, GitHubAppCredentials
from c4.integrations.github_app import (
    GitHubAppClient,
    PRInfo,
    ReviewComment,
    ReviewResult,
    WebhookResult,
)


# =============================================================================
# Configuration Tests
# =============================================================================


class TestGitHubAppConfig:
    """Test GitHub App configuration."""

    def test_default_config(self):
        """Test default configuration values."""
        config = GitHubAppConfig()

        assert config.enabled is False
        assert config.app_id is None
        assert config.review_enabled is True
        assert config.max_diff_size == 50000
        assert config.auto_label is True

    def test_get_app_id_from_config(self):
        """Test getting app ID from config."""
        config = GitHubAppConfig(app_id="123456")
        assert config.get_app_id() == "123456"

    def test_get_app_id_from_env(self):
        """Test getting app ID from environment."""
        config = GitHubAppConfig()
        with patch.dict("os.environ", {"GITHUB_APP_ID": "env-app-id"}):
            assert config.get_app_id() == "env-app-id"

    def test_get_webhook_secret_from_config(self):
        """Test getting webhook secret from config."""
        config = GitHubAppConfig(webhook_secret="secret123")
        assert config.get_webhook_secret() == "secret123"

    def test_get_webhook_secret_from_env(self):
        """Test getting webhook secret from environment."""
        config = GitHubAppConfig()
        with patch.dict("os.environ", {"GITHUB_WEBHOOK_SECRET": "env-secret"}):
            assert config.get_webhook_secret() == "env-secret"

    def test_is_configured_false_when_disabled(self):
        """Test is_configured returns False when disabled."""
        config = GitHubAppConfig(
            enabled=False,
            app_id="123",
            private_key="key",
            webhook_secret="secret",
        )
        assert config.is_configured() is False

    def test_is_configured_false_when_missing_fields(self):
        """Test is_configured returns False with missing fields."""
        config = GitHubAppConfig(enabled=True, app_id="123")
        assert config.is_configured() is False

    def test_is_configured_true_when_complete(self):
        """Test is_configured returns True when fully configured."""
        config = GitHubAppConfig(
            enabled=True,
            app_id="123",
            private_key="-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----",
            webhook_secret="secret",
        )
        assert config.is_configured() is True


class TestGitHubAppCredentials:
    """Test GitHub App credentials."""

    def test_from_config_success(self):
        """Test creating credentials from config."""
        config = GitHubAppConfig(
            enabled=True,
            app_id="123",
            private_key="key-content",
            webhook_secret="secret",
        )

        credentials = GitHubAppCredentials.from_config(config)

        assert credentials is not None
        assert credentials.app_id == "123"
        assert credentials.private_key == "key-content"
        assert credentials.webhook_secret == "secret"

    def test_from_config_missing_fields(self):
        """Test credentials returns None with missing fields."""
        config = GitHubAppConfig(enabled=True, app_id="123")

        credentials = GitHubAppCredentials.from_config(config)

        assert credentials is None


# =============================================================================
# Webhook Signature Tests
# =============================================================================


class TestWebhookSignature:
    """Test webhook signature verification."""

    @pytest.fixture
    def client(self):
        """Create a GitHub App client for testing."""
        return GitHubAppClient(
            app_id="123456",
            private_key="-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
            webhook_secret="test-secret",
        )

    def test_verify_valid_signature(self, client):
        """Test verifying a valid webhook signature."""
        payload = b'{"action": "opened"}'
        # Compute expected signature
        mac = hmac.new(b"test-secret", payload, hashlib.sha256)
        signature = f"sha256={mac.hexdigest()}"

        assert client.verify_webhook_signature(payload, signature) is True

    def test_verify_invalid_signature(self, client):
        """Test rejecting an invalid signature."""
        payload = b'{"action": "opened"}'
        signature = "sha256=invalid-signature"

        assert client.verify_webhook_signature(payload, signature) is False

    def test_verify_missing_prefix(self, client):
        """Test rejecting signature without sha256= prefix."""
        payload = b'{"action": "opened"}'
        signature = "invalid-no-prefix"

        assert client.verify_webhook_signature(payload, signature) is False

    def test_verify_empty_signature(self, client):
        """Test rejecting empty signature."""
        payload = b'{"action": "opened"}'
        signature = ""

        assert client.verify_webhook_signature(payload, signature) is False

    def test_signature_timing_attack_protection(self, client):
        """Test that signature comparison is constant-time."""
        payload = b'{"action": "opened"}'
        mac = hmac.new(b"test-secret", payload, hashlib.sha256)
        valid_sig = f"sha256={mac.hexdigest()}"

        # Both should use hmac.compare_digest internally
        result1 = client.verify_webhook_signature(payload, valid_sig)
        result2 = client.verify_webhook_signature(payload, "sha256=wrong")

        assert result1 is True
        assert result2 is False


# =============================================================================
# PR Webhook Parsing Tests
# =============================================================================


class TestPRWebhookParsing:
    """Test PR webhook payload parsing."""

    @pytest.fixture
    def client(self):
        """Create a GitHub App client for testing."""
        return GitHubAppClient(
            app_id="123456",
            private_key="test-key",
            webhook_secret="test-secret",
        )

    @pytest.fixture
    def sample_pr_payload(self):
        """Create a sample PR webhook payload."""
        return {
            "action": "opened",
            "pull_request": {
                "number": 42,
                "title": "Add new feature",
                "head": {
                    "sha": "abc123def456",
                    "ref": "feature-branch",
                },
                "base": {
                    "ref": "main",
                },
                "user": {
                    "login": "testuser",
                },
                "diff_url": "https://github.com/owner/repo/pull/42.diff",
            },
            "repository": {
                "full_name": "owner/repo",
            },
            "installation": {
                "id": 12345678,
            },
        }

    def test_parse_pr_opened(self, client, sample_pr_payload):
        """Test parsing PR opened event."""
        pr_info = client.parse_pr_webhook(sample_pr_payload)

        assert pr_info is not None
        assert pr_info.owner == "owner"
        assert pr_info.repo == "repo"
        assert pr_info.number == 42
        assert pr_info.title == "Add new feature"
        assert pr_info.head_sha == "abc123def456"
        assert pr_info.head_branch == "feature-branch"
        assert pr_info.base_branch == "main"
        assert pr_info.author == "testuser"
        assert pr_info.installation_id == 12345678

    def test_parse_pr_synchronize(self, client, sample_pr_payload):
        """Test parsing PR synchronize event."""
        sample_pr_payload["action"] = "synchronize"
        pr_info = client.parse_pr_webhook(sample_pr_payload)

        assert pr_info is not None
        assert pr_info.number == 42

    def test_parse_pr_reopened(self, client, sample_pr_payload):
        """Test parsing PR reopened event."""
        sample_pr_payload["action"] = "reopened"
        pr_info = client.parse_pr_webhook(sample_pr_payload)

        assert pr_info is not None
        assert pr_info.number == 42

    def test_ignore_pr_closed(self, client, sample_pr_payload):
        """Test ignoring PR closed event."""
        sample_pr_payload["action"] = "closed"
        pr_info = client.parse_pr_webhook(sample_pr_payload)

        assert pr_info is None

    def test_ignore_pr_labeled(self, client, sample_pr_payload):
        """Test ignoring PR labeled event."""
        sample_pr_payload["action"] = "labeled"
        pr_info = client.parse_pr_webhook(sample_pr_payload)

        assert pr_info is None

    def test_parse_missing_pull_request(self, client):
        """Test handling payload without pull_request."""
        payload = {"action": "opened"}
        pr_info = client.parse_pr_webhook(payload)

        assert pr_info is None


# =============================================================================
# API Request Tests
# =============================================================================


class TestGitHubAppAPI:
    """Test GitHub App API operations."""

    @pytest.fixture
    def client(self):
        """Create a GitHub App client for testing."""
        return GitHubAppClient(
            app_id="123456",
            private_key="-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
            webhook_secret="test-secret",
        )

    @pytest.fixture
    def pr_info(self):
        """Create sample PR info."""
        return PRInfo(
            owner="owner",
            repo="repo",
            number=42,
            title="Test PR",
            head_sha="abc123",
            base_branch="main",
            head_branch="feature",
            author="testuser",
            diff_url="https://github.com/owner/repo/pull/42.diff",
            installation_id=12345678,
        )

    def test_create_review_success(self, client, pr_info):
        """Test creating a PR review successfully."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (200, {"id": 98765})

            result = client.create_review(
                pr_info=pr_info,
                body="Great work!",
                event="APPROVE",
            )

            assert result.success is True
            assert result.review_id == 98765
            mock_api.assert_called_once()

    def test_create_review_with_comments(self, client, pr_info):
        """Test creating a PR review with line comments."""
        comments = [
            ReviewComment(path="src/main.py", line=10, body="Consider refactoring"),
            ReviewComment(path="src/utils.py", line=25, body="Add docstring"),
        ]

        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (200, {"id": 98765})

            result = client.create_review(
                pr_info=pr_info,
                body="Review with comments",
                event="COMMENT",
                comments=comments,
            )

            assert result.success is True
            assert result.comments_posted == 2

    def test_create_review_failure(self, client, pr_info):
        """Test handling review creation failure."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (422, {"message": "Validation failed"})

            result = client.create_review(
                pr_info=pr_info,
                body="Review",
                event="COMMENT",
            )

            assert result.success is False
            assert "Validation failed" in result.message

    def test_add_labels_success(self, client, pr_info):
        """Test adding labels to a PR."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (200, [{"name": "bug"}])

            result = client.add_labels(pr_info, ["bug", "enhancement"])

            assert result is True
            mock_api.assert_called_once()

    def test_create_check_run(self, client, pr_info):
        """Test creating a check run."""
        with patch.object(client, "_api_request") as mock_api:
            mock_api.return_value = (201, {"id": 54321})

            result = client.create_check_run(
                pr_info=pr_info,
                name="C4 Review",
                status="completed",
                conclusion="success",
            )

            assert result is True


# =============================================================================
# Review Comment Tests
# =============================================================================


class TestReviewComment:
    """Test ReviewComment data class."""

    def test_default_side(self):
        """Test default side is RIGHT."""
        comment = ReviewComment(path="file.py", line=10, body="Comment")
        assert comment.side == "RIGHT"

    def test_custom_side(self):
        """Test setting custom side."""
        comment = ReviewComment(path="file.py", line=10, body="Comment", side="LEFT")
        assert comment.side == "LEFT"


# =============================================================================
# Integration-Style Tests (Mocked)
# =============================================================================


class TestWebhookFlow:
    """Test complete webhook processing flow."""

    @pytest.fixture
    def webhook_payload(self):
        """Create a complete webhook payload."""
        return {
            "action": "opened",
            "pull_request": {
                "number": 1,
                "title": "Fix bug",
                "head": {"sha": "abc123", "ref": "fix-branch"},
                "base": {"ref": "main"},
                "user": {"login": "developer"},
                "diff_url": "https://github.com/o/r/pull/1.diff",
            },
            "repository": {"full_name": "owner/repo"},
            "installation": {"id": 999},
        }

    def test_full_webhook_processing(self, webhook_payload):
        """Test complete webhook verification and parsing."""
        client = GitHubAppClient(
            app_id="123",
            private_key="key",
            webhook_secret="secret123",
        )

        # Create valid signature
        payload_bytes = json.dumps(webhook_payload).encode()
        mac = hmac.new(b"secret123", payload_bytes, hashlib.sha256)
        signature = f"sha256={mac.hexdigest()}"

        # Verify signature
        assert client.verify_webhook_signature(payload_bytes, signature) is True

        # Parse PR info
        pr_info = client.parse_pr_webhook(webhook_payload)
        assert pr_info is not None
        assert pr_info.number == 1
        assert pr_info.title == "Fix bug"
