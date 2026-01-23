"""Tests for integration providers (GitHub, Discord).

Tests the concrete provider implementations.
"""

from __future__ import annotations

import hashlib
import hmac

import pytest

from c4.integrations.base import (
    IntegrationCapability,
    IntegrationCategory,
)


class TestGitHubProvider:
    """Tests for GitHubProvider."""

    @pytest.fixture
    def github_provider(self):
        """Create GitHub provider instance."""
        from c4.integrations.github_provider import GitHubProvider

        return GitHubProvider()

    def test_provider_id(self, github_provider) -> None:
        """Test provider ID."""
        assert github_provider.id == "github"

    def test_provider_name(self, github_provider) -> None:
        """Test provider name."""
        assert github_provider.name == "GitHub"

    def test_provider_category(self, github_provider) -> None:
        """Test provider category."""
        assert github_provider.category == IntegrationCategory.SOURCE_CONTROL

    def test_provider_capabilities(self, github_provider) -> None:
        """Test provider capabilities."""
        caps = github_provider.capabilities
        assert IntegrationCapability.PR_REVIEW in caps
        assert IntegrationCapability.WEBHOOKS in caps
        assert IntegrationCapability.OAUTH in caps

    def test_get_oauth_url(
        self, github_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test OAuth URL generation."""
        monkeypatch.setenv("GITHUB_APP_NAME", "test-app")

        url = github_provider.get_oauth_url("test_state")

        assert "https://github.com" in url
        assert "test-app" in url
        assert "test_state" in url

    @pytest.mark.asyncio
    async def test_verify_webhook_valid(
        self, github_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test webhook signature verification with valid signature."""
        secret = "webhook_secret_123"
        payload = b'{"action": "opened"}'

        # Calculate expected signature
        signature = (
            "sha256="
            + hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
        )

        headers = {"x-hub-signature-256": signature}

        result = await github_provider.verify_webhook(payload, headers, secret)
        assert result is True

    @pytest.mark.asyncio
    async def test_verify_webhook_invalid(
        self, github_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test webhook signature verification with invalid signature."""
        secret = "webhook_secret_123"
        payload = b'{"action": "opened"}'
        headers = {"x-hub-signature-256": "sha256=invalid_signature"}

        result = await github_provider.verify_webhook(payload, headers, secret)
        assert result is False

    @pytest.mark.asyncio
    async def test_verify_webhook_missing_header(self, github_provider) -> None:
        """Test webhook verification with missing signature header."""
        payload = b'{"action": "opened"}'
        headers = {}
        secret = "webhook_secret"

        result = await github_provider.verify_webhook(payload, headers, secret)
        assert result is False

    @pytest.mark.asyncio
    async def test_parse_webhook_pr_opened(self, github_provider) -> None:
        """Test parsing PR opened webhook."""
        payload = {
            "action": "opened",
            "pull_request": {
                "number": 123,
                "title": "Test PR",
                "html_url": "https://github.com/test/repo/pull/123",
                "user": {"login": "testuser"},
            },
            "repository": {
                "full_name": "test/repo",
            },
            "installation": {"id": 12345},
        }
        headers = {"x-github-event": "pull_request"}

        event = await github_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "pull_request"
        assert event.external_id == "12345"
        assert event.action == "opened"

    @pytest.mark.asyncio
    async def test_parse_webhook_no_installation(self, github_provider) -> None:
        """Test parsing webhook without installation ID."""
        payload = {
            "action": "opened",
            "pull_request": {"number": 123},
        }
        headers = {"x-github-event": "pull_request"}

        event = await github_provider.parse_webhook(payload, headers)

        assert event is None

    def test_get_info(self, github_provider, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test getting provider info."""
        monkeypatch.setenv("GITHUB_APP_CLIENT_ID", "test_client_id")

        info = github_provider.get_info()

        assert info.id == "github"
        assert info.name == "GitHub"
        assert info.category == IntegrationCategory.SOURCE_CONTROL
        assert info.webhook_path == "/webhooks/github"


class TestDiscordProvider:
    """Tests for DiscordProvider."""

    @pytest.fixture
    def discord_provider(self):
        """Create Discord provider instance."""
        from c4.integrations.discord_provider import DiscordProvider

        return DiscordProvider()

    def test_provider_id(self, discord_provider) -> None:
        """Test provider ID."""
        assert discord_provider.id == "discord"

    def test_provider_name(self, discord_provider) -> None:
        """Test provider name."""
        assert discord_provider.name == "Discord"

    def test_provider_category(self, discord_provider) -> None:
        """Test provider category."""
        assert discord_provider.category == IntegrationCategory.MESSAGING

    def test_provider_capabilities(self, discord_provider) -> None:
        """Test provider capabilities."""
        caps = discord_provider.capabilities
        assert IntegrationCapability.NOTIFICATIONS in caps
        assert IntegrationCapability.COMMANDS in caps
        assert IntegrationCapability.WEBHOOKS in caps
        assert IntegrationCapability.OAUTH in caps
        assert IntegrationCapability.BOT in caps

    def test_get_oauth_url(
        self, discord_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test OAuth URL generation."""
        monkeypatch.setenv("DISCORD_CLIENT_ID", "discord_client_id")

        url = discord_provider.get_oauth_url("test_state")

        assert "https://discord.com/api/oauth2/authorize" in url
        assert "discord_client_id" in url
        assert "test_state" in url
        assert "bot" in url
        assert "applications.commands" in url

    def test_bot_permissions(self, discord_provider) -> None:
        """Test bot permissions are correctly configured."""
        # Expected permissions:
        # 2048 = Send Messages
        # 16384 = Embed Links
        # 32768 = Attach Files
        # 262144 = Add Reactions
        # 2147483648 = Use Slash Commands
        expected = 2048 | 16384 | 32768 | 262144 | 2147483648
        assert discord_provider.BOT_PERMISSIONS == expected

    @pytest.mark.asyncio
    async def test_parse_webhook_application_command(self, discord_provider) -> None:
        """Test parsing application command interaction."""
        payload = {
            "type": 2,  # APPLICATION_COMMAND
            "guild_id": "guild_123",
            "data": {"name": "status"},
        }
        headers = {}

        event = await discord_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "application_command"
        assert event.external_id == "guild_123"
        assert event.action == "status"

    @pytest.mark.asyncio
    async def test_parse_webhook_message_component(self, discord_provider) -> None:
        """Test parsing message component interaction."""
        payload = {
            "type": 3,  # MESSAGE_COMPONENT
            "guild_id": "guild_123",
            "data": {"custom_id": "approve_button"},
        }
        headers = {}

        event = await discord_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "message_component"
        assert event.external_id == "guild_123"
        assert event.action == "approve_button"

    @pytest.mark.asyncio
    async def test_parse_webhook_ping(self, discord_provider) -> None:
        """Test parsing ping interaction."""
        payload = {
            "type": 1,  # PING
            "guild_id": "guild_123",
        }
        headers = {}

        event = await discord_provider.parse_webhook(payload, headers)

        assert event is not None
        assert event.event_type == "ping"

    @pytest.mark.asyncio
    async def test_parse_webhook_no_guild(self, discord_provider) -> None:
        """Test parsing webhook without guild ID."""
        payload = {"type": 2, "data": {"name": "test"}}
        headers = {}

        event = await discord_provider.parse_webhook(payload, headers)

        assert event is None

    @pytest.mark.asyncio
    async def test_connect(self, discord_provider) -> None:
        """Test connecting Discord guild."""
        credentials = {
            "guild_id": "guild_123",
            "guild_name": "Test Server",
        }

        result = await discord_provider.connect("workspace_1", credentials)

        assert result.success is True
        assert result.external_id == "guild_123"
        assert result.external_name == "Test Server"

    @pytest.mark.asyncio
    async def test_connect_missing_guild_id(self, discord_provider) -> None:
        """Test connecting without guild ID fails."""
        credentials = {"guild_name": "Test Server"}

        result = await discord_provider.connect("workspace_1", credentials)

        assert result.success is False
        assert result.error_code == "missing_guild_id"

    @pytest.mark.asyncio
    async def test_disconnect(self, discord_provider) -> None:
        """Test disconnecting returns True."""
        result = await discord_provider.disconnect("workspace_1", "guild_123")
        assert result is True

    def test_get_info(self, discord_provider, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test getting provider info."""
        monkeypatch.setenv("DISCORD_CLIENT_ID", "discord_client_id")

        info = discord_provider.get_info()

        assert info.id == "discord"
        assert info.name == "Discord"
        assert info.category == IntegrationCategory.MESSAGING
        assert info.webhook_path == "/webhooks/discord"
        assert IntegrationCapability.NOTIFICATIONS in info.capabilities


class TestProviderNotificationMocking:
    """Test notification methods with mocking."""

    @pytest.fixture
    def discord_provider(self):
        """Create Discord provider instance."""
        from c4.integrations.discord_provider import DiscordProvider

        return DiscordProvider()

    @pytest.mark.asyncio
    async def test_send_notification_no_channel(self, discord_provider) -> None:
        """Test sending notification without channel fails."""
        credentials = {"guild_id": "guild_123"}

        result = await discord_provider.send_notification(credentials, "Hello")

        assert result.success is False
        assert "No channel_id" in result.message

    @pytest.mark.asyncio
    async def test_send_notification_no_bot_token(
        self, discord_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test sending notification without bot token fails."""
        monkeypatch.delenv("DISCORD_BOT_TOKEN", raising=False)
        credentials = {"guild_id": "guild_123"}

        result = await discord_provider.send_notification(
            credentials, "Hello", channel_id="channel_123"
        )

        assert result.success is False
        assert "not configured" in result.message

    @pytest.mark.asyncio
    async def test_validate_connection_no_token(
        self, discord_provider, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test validation without token fails."""
        monkeypatch.delenv("DISCORD_BOT_TOKEN", raising=False)
        credentials = {"guild_id": "guild_123"}

        result = await discord_provider.validate_connection(credentials)

        assert result is False
