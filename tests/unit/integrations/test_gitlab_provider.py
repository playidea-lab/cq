"""Tests for GitLab integration provider.

Tests the GitLabProvider and GitLabClient implementations.
"""

from __future__ import annotations

import pytest

from c4.integrations.base import (
    IntegrationCapability,
    IntegrationCategory,
)


class TestGitLabProvider:
    """Tests for GitLabProvider."""

    @pytest.fixture
    def gitlab_provider(self):
        """Create GitLab provider instance."""
        from c4.integrations.gitlab_provider import GitLabProvider

        return GitLabProvider()

    def test_provider_id(self, gitlab_provider) -> None:
        """Test provider ID."""
        assert gitlab_provider.id == "gitlab"

    def test_provider_name(self, gitlab_provider) -> None:
        """Test provider name."""
        assert gitlab_provider.name == "GitLab"

    def test_provider_category(self, gitlab_provider) -> None:
        """Test provider category."""
        assert gitlab_provider.category == IntegrationCategory.SOURCE_CONTROL

    def test_provider_capabilities(self, gitlab_provider) -> None:
        """Test provider capabilities."""
        caps = gitlab_provider.capabilities
        assert IntegrationCapability.PR_REVIEW in caps
        assert IntegrationCapability.WEBHOOKS in caps
        assert IntegrationCapability.OAUTH in caps

    def test_get_oauth_url(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test OAuth URL generation."""
        # Set env vars before creating provider (provider reads at init time)
        monkeypatch.setenv("GITLAB_APP_ID", "test_app_id")
        monkeypatch.setenv("GITLAB_URL", "https://gitlab.example.com")

        from c4.integrations.gitlab_provider import GitLabProvider

        provider = GitLabProvider()
        url = provider.get_oauth_url("test_state")

        assert "https://gitlab.example.com/oauth/authorize" in url
        assert "test_app_id" in url
        assert "test_state" in url
        assert "api" in url  # scope

    def test_get_oauth_url_default_gitlab(
        self, gitlab_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test OAuth URL uses default gitlab.com when GITLAB_URL not set."""
        monkeypatch.setenv("GITLAB_APP_ID", "test_app_id")
        monkeypatch.delenv("GITLAB_URL", raising=False)

        url = gitlab_provider.get_oauth_url("test_state")

        assert "https://gitlab.com/oauth/authorize" in url

    @pytest.mark.asyncio
    async def test_verify_webhook_valid(self, gitlab_provider) -> None:
        """Test webhook token verification with valid token."""
        secret = "webhook_secret_123"
        payload = b'{"object_kind": "merge_request"}'

        # GitLab uses simple token comparison via X-Gitlab-Token header
        headers = {"x-gitlab-token": secret}

        result = await gitlab_provider.verify_webhook(payload, headers, secret)
        assert result is True

    @pytest.mark.asyncio
    async def test_verify_webhook_invalid(self, gitlab_provider) -> None:
        """Test webhook token verification with invalid token."""
        secret = "webhook_secret_123"
        payload = b'{"object_kind": "merge_request"}'
        headers = {"x-gitlab-token": "wrong_token"}

        result = await gitlab_provider.verify_webhook(payload, headers, secret)
        assert result is False

    @pytest.mark.asyncio
    async def test_verify_webhook_missing_header(self, gitlab_provider) -> None:
        """Test webhook verification with missing token header."""
        payload = b'{"object_kind": "merge_request"}'
        headers = {}
        secret = "webhook_secret"

        result = await gitlab_provider.verify_webhook(payload, headers, secret)
        assert result is False

    @pytest.mark.asyncio
    async def test_verify_webhook_no_secret_configured(self, gitlab_provider) -> None:
        """Test webhook verification when no secret is configured (accepts all)."""
        payload = b'{"object_kind": "merge_request"}'
        headers = {}
        secret = ""  # Empty secret means no verification required

        result = await gitlab_provider.verify_webhook(payload, headers, secret)
        assert result is True

    @pytest.mark.asyncio
    async def test_parse_webhook_mr_opened(self, gitlab_provider) -> None:
        """Test parsing MR opened webhook."""
        payload = {
            "object_kind": "merge_request",
            "event_type": "merge_request",
            "object_attributes": {
                "iid": 42,
                "title": "Test MR",
                "action": "open",
                "source_branch": "feature-branch",
                "target_branch": "main",
                "last_commit": {"id": "abc123def456"},
                "url": "https://gitlab.com/test/repo/-/merge_requests/42",
            },
            "project": {
                "id": 123,
                "path_with_namespace": "test/repo",
            },
            "user": {
                "username": "testuser",
            },
        }
        headers = {"x-gitlab-event": "Merge Request Hook"}

        event = await gitlab_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "merge_request"
        assert event.external_id == "123"  # project_id
        assert event.action == "open"

    @pytest.mark.asyncio
    async def test_parse_webhook_mr_update(self, gitlab_provider) -> None:
        """Test parsing MR update webhook."""
        payload = {
            "object_kind": "merge_request",
            "event_type": "merge_request",
            "object_attributes": {
                "iid": 42,
                "title": "Test MR",
                "action": "update",
                "oldrev": "old_sha",  # Has oldrev, meaning new commits
                "source_branch": "feature-branch",
                "target_branch": "main",
                "last_commit": {"id": "new_sha"},
                "url": "https://gitlab.com/test/repo/-/merge_requests/42",
            },
            "project": {
                "id": 123,
                "path_with_namespace": "test/repo",
            },
            "user": {
                "username": "testuser",
            },
        }
        headers = {"x-gitlab-event": "Merge Request Hook"}

        event = await gitlab_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "merge_request"
        assert event.action == "update"

    @pytest.mark.asyncio
    async def test_parse_webhook_push(self, gitlab_provider) -> None:
        """Test parsing push webhook."""
        payload = {
            "object_kind": "push",
            "event_name": "push",
            "project": {
                "id": 123,
                "path_with_namespace": "test/repo",
            },
            "ref": "refs/heads/main",
        }
        headers = {"x-gitlab-event": "Push Hook"}

        event = await gitlab_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "push"
        assert event.external_id == "123"

    @pytest.mark.asyncio
    async def test_parse_webhook_no_project(self, gitlab_provider) -> None:
        """Test parsing webhook without project ID."""
        payload = {
            "object_kind": "merge_request",
            "object_attributes": {"iid": 42, "action": "open"},
        }
        headers = {"x-gitlab-event": "Merge Request Hook"}

        event = await gitlab_provider.parse_webhook(payload, headers)

        assert event is None

    def test_get_info(self, gitlab_provider, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test getting provider info."""
        monkeypatch.setenv("GITLAB_APP_ID", "test_app_id")

        info = gitlab_provider.get_info()

        assert info.id == "gitlab"
        assert info.name == "GitLab"
        assert info.category == IntegrationCategory.SOURCE_CONTROL
        assert info.webhook_path == "/webhooks/gitlab"
        assert IntegrationCapability.PR_REVIEW in info.capabilities


class TestGitLabClient:
    """Tests for GitLabClient."""

    @pytest.fixture
    def gitlab_client(self, monkeypatch: pytest.MonkeyPatch):
        """Create GitLab client instance."""
        monkeypatch.setenv("GITLAB_PRIVATE_TOKEN", "test_token")
        from c4.integrations.gitlab_client import GitLabClient

        return GitLabClient(
            url="https://gitlab.example.com",
            private_token="test_token",
        )

    def test_verify_webhook_signature_valid(self, gitlab_client) -> None:
        """Test webhook signature verification with valid token."""
        payload = b'{"object_kind": "merge_request"}'
        token = "secret_token"

        result = gitlab_client.verify_webhook_signature(payload, token)

        assert result is True

    def test_verify_webhook_signature_invalid(self, gitlab_client) -> None:
        """Test webhook signature verification with invalid token."""
        # GitLabClient compares against stored webhook_secret
        from c4.integrations.gitlab_client import GitLabClient

        client = GitLabClient(
            url="https://gitlab.example.com",
            private_token="test_token",
            webhook_secret="expected_secret",
        )
        payload = b'{"object_kind": "merge_request"}'

        result = client.verify_webhook_signature(payload, "wrong_secret")

        assert result is False

    def test_verify_webhook_signature_no_secret(self, gitlab_client) -> None:
        """Test webhook verification when no secret is configured."""
        # Client without webhook_secret accepts any token
        payload = b'{"object_kind": "merge_request"}'

        result = gitlab_client.verify_webhook_signature(payload, "any_token")

        assert result is True

    def test_parse_mr_webhook(self, gitlab_client) -> None:
        """Test parsing MR webhook payload."""
        payload = {
            "object_kind": "merge_request",
            "object_attributes": {
                "iid": 42,
                "title": "Add feature X",
                "action": "open",
                "source_branch": "feature-x",
                "target_branch": "main",
                "last_commit": {"id": "abc123def456789"},
                "url": "https://gitlab.example.com/test/repo/-/merge_requests/42",
            },
            "project": {
                "id": 123,
                "path_with_namespace": "test/repo",
            },
            "user": {
                "username": "developer",
            },
        }

        mr_info = gitlab_client.parse_mr_webhook(payload)

        assert mr_info is not None
        assert mr_info.project_id == 123
        assert mr_info.mr_iid == 42
        assert mr_info.title == "Add feature X"
        assert mr_info.source_branch == "feature-x"
        assert mr_info.target_branch == "main"
        assert mr_info.author == "developer"
        assert mr_info.head_sha == "abc123def456789"

    def test_parse_mr_webhook_missing_attributes(self, gitlab_client) -> None:
        """Test parsing MR webhook with missing required attributes."""
        payload = {
            "object_kind": "merge_request",
            # Missing object_attributes and project
        }

        mr_info = gitlab_client.parse_mr_webhook(payload)

        assert mr_info is None

    def test_parse_mr_webhook_not_mr_event(self, gitlab_client) -> None:
        """Test parsing non-MR webhook returns None."""
        payload = {
            "object_kind": "push",
            "ref": "refs/heads/main",
        }

        mr_info = gitlab_client.parse_mr_webhook(payload)

        assert mr_info is None

    def test_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test creating client from environment variables."""
        monkeypatch.setenv("GITLAB_PRIVATE_TOKEN", "env_token")
        monkeypatch.setenv("GITLAB_URL", "https://gitlab.custom.com")
        monkeypatch.setenv("GITLAB_WEBHOOK_SECRET", "env_secret")

        from c4.integrations.gitlab_client import GitLabClient

        client = GitLabClient.from_env()

        assert client is not None
        assert client.url == "https://gitlab.custom.com"
        assert client.webhook_secret == "env_secret"

    def test_from_env_default_url(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test creating client with default GitLab URL."""
        monkeypatch.setenv("GITLAB_PRIVATE_TOKEN", "env_token")
        monkeypatch.delenv("GITLAB_URL", raising=False)

        from c4.integrations.gitlab_client import GitLabClient

        client = GitLabClient.from_env()

        assert client is not None
        assert client.url == "https://gitlab.com"

    def test_from_env_oauth_token(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test creating client with OAuth token instead of private token."""
        monkeypatch.delenv("GITLAB_PRIVATE_TOKEN", raising=False)
        monkeypatch.setenv("GITLAB_OAUTH_TOKEN", "oauth_token")

        from c4.integrations.gitlab_client import GitLabClient

        client = GitLabClient.from_env()

        assert client is not None

    def test_from_env_no_token(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test creating client without any token returns None."""
        monkeypatch.delenv("GITLAB_PRIVATE_TOKEN", raising=False)
        monkeypatch.delenv("GITLAB_OAUTH_TOKEN", raising=False)

        from c4.integrations.gitlab_client import GitLabClient

        client = GitLabClient.from_env()

        assert client is None


class TestMRInfo:
    """Tests for MRInfo dataclass."""

    def test_mr_info_creation(self) -> None:
        """Test creating MRInfo instance."""
        from c4.integrations.gitlab_client import MRInfo

        mr_info = MRInfo(
            project_id=123,
            mr_iid=42,
            title="Test MR",
            source_branch="feature",
            target_branch="main",
            author="developer",
            head_sha="abc123",
            namespace="test-group",
            project_path="test-group/repo",
            diff_url="https://gitlab.com/test-group/repo/-/merge_requests/42/diffs",
        )

        assert mr_info.project_id == 123
        assert mr_info.mr_iid == 42
        assert mr_info.title == "Test MR"
        assert mr_info.source_branch == "feature"
        assert mr_info.target_branch == "main"
        assert mr_info.author == "developer"
        assert mr_info.head_sha == "abc123"
        assert mr_info.namespace == "test-group"
        assert mr_info.project_path == "test-group/repo"
        assert mr_info.diff_url == "https://gitlab.com/test-group/repo/-/merge_requests/42/diffs"
