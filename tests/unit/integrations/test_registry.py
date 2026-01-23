"""Tests for IntegrationRegistry - Provider registration and lookup.

Tests the singleton registry pattern for managing integration providers.
"""

from __future__ import annotations

from typing import Any

import pytest

from c4.integrations.base import (
    ConnectionResult,
    IntegrationCapability,
    IntegrationCategory,
    IntegrationProvider,
    NotificationResult,
    WebhookEvent,
)
from c4.integrations.registry import IntegrationRegistry


class MockProvider(IntegrationProvider):
    """Mock provider for testing registry."""

    @property
    def id(self) -> str:
        return "mock_provider"

    @property
    def name(self) -> str:
        return "Mock Provider"

    @property
    def category(self) -> IntegrationCategory:
        return IntegrationCategory.MESSAGING

    @property
    def capabilities(self) -> list[IntegrationCapability]:
        return [IntegrationCapability.NOTIFICATIONS, IntegrationCapability.WEBHOOKS]

    @property
    def description(self) -> str:
        return "A mock provider for testing"

    def get_oauth_url(self, state: str) -> str:
        return f"https://mock.example.com/oauth?state={state}"

    async def exchange_code(self, code: str, state: str) -> ConnectionResult:
        return ConnectionResult(
            success=True,
            message="Connected",
            external_id="mock_123",
            external_name="Mock Instance",
            credentials={"token": "mock_token"},
        )

    async def connect(
        self, workspace_id: str, credentials: dict[str, Any]
    ) -> ConnectionResult:
        return ConnectionResult(
            success=True,
            message="Connected",
            external_id=credentials.get("external_id", ""),
            external_name=credentials.get("name"),
            credentials=credentials,
        )

    async def disconnect(self, workspace_id: str, external_id: str) -> bool:
        return True

    async def validate_connection(self, credentials: dict[str, Any]) -> bool:
        return credentials.get("token") is not None

    async def send_notification(
        self,
        credentials: dict[str, Any],
        message: str,
        *,
        channel_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> NotificationResult:
        return NotificationResult(success=True, message="Sent", message_id="msg_123")

    async def verify_webhook(
        self, payload: bytes, headers: dict[str, str], secret: str
    ) -> bool:
        return headers.get("X-Mock-Signature") == secret

    async def parse_webhook(
        self, payload: dict[str, Any], headers: dict[str, str]
    ) -> WebhookEvent | None:
        return WebhookEvent(
            event_type="message",
            external_id="mock_123",
            action="send",
            data=payload,
            raw_payload=payload,
        )


class AnotherMockProvider(MockProvider):
    """Another mock provider with different ID."""

    @property
    def id(self) -> str:
        return "another_mock"

    @property
    def name(self) -> str:
        return "Another Mock"

    @property
    def category(self) -> IntegrationCategory:
        return IntegrationCategory.SOURCE_CONTROL

    @property
    def capabilities(self) -> list[IntegrationCapability]:
        return [IntegrationCapability.PR_REVIEW, IntegrationCapability.OAUTH]


class TestIntegrationRegistryRegistration:
    """Test provider registration."""

    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Clear registry before and after each test."""
        IntegrationRegistry.clear()
        yield
        IntegrationRegistry.clear()

    def test_register_provider(self) -> None:
        """Test registering a provider."""
        IntegrationRegistry.register(MockProvider)

        assert IntegrationRegistry.has("mock_provider")
        assert "mock_provider" in IntegrationRegistry.list_ids()

    def test_register_multiple_providers(self) -> None:
        """Test registering multiple providers."""
        IntegrationRegistry.register(MockProvider)
        IntegrationRegistry.register(AnotherMockProvider)

        assert IntegrationRegistry.has("mock_provider")
        assert IntegrationRegistry.has("another_mock")
        assert len(IntegrationRegistry.list_ids()) == 2

    def test_register_returns_class(self) -> None:
        """Test that register returns the class for decorator use."""
        result = IntegrationRegistry.register(MockProvider)
        assert result is MockProvider

    def test_reregister_overwrites(self) -> None:
        """Test that re-registering overwrites existing provider."""
        IntegrationRegistry.register(MockProvider)
        # Register again - should not raise
        IntegrationRegistry.register(MockProvider)

        assert IntegrationRegistry.has("mock_provider")
        assert len(IntegrationRegistry.list_ids()) == 1


class TestIntegrationRegistryLookup:
    """Test provider lookup."""

    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Clear registry and register providers before each test."""
        IntegrationRegistry.clear()
        IntegrationRegistry.register(MockProvider)
        IntegrationRegistry.register(AnotherMockProvider)
        yield
        IntegrationRegistry.clear()

    def test_get_existing_provider(self) -> None:
        """Test getting an existing provider."""
        provider = IntegrationRegistry.get("mock_provider")

        assert provider is not None
        assert provider.id == "mock_provider"
        assert provider.name == "Mock Provider"

    def test_get_nonexistent_provider(self) -> None:
        """Test getting a nonexistent provider returns None."""
        provider = IntegrationRegistry.get("nonexistent")
        assert provider is None

    def test_get_or_raise_existing(self) -> None:
        """Test get_or_raise with existing provider."""
        provider = IntegrationRegistry.get_or_raise("mock_provider")
        assert provider.id == "mock_provider"

    def test_get_or_raise_nonexistent(self) -> None:
        """Test get_or_raise with nonexistent provider raises ValueError."""
        with pytest.raises(ValueError) as exc_info:
            IntegrationRegistry.get_or_raise("nonexistent")

        assert "Unknown provider: nonexistent" in str(exc_info.value)

    def test_get_returns_cached_instance(self) -> None:
        """Test that get returns the same cached instance."""
        provider1 = IntegrationRegistry.get("mock_provider")
        provider2 = IntegrationRegistry.get("mock_provider")

        assert provider1 is provider2

    def test_has_existing_provider(self) -> None:
        """Test has returns True for existing provider."""
        assert IntegrationRegistry.has("mock_provider") is True

    def test_has_nonexistent_provider(self) -> None:
        """Test has returns False for nonexistent provider."""
        assert IntegrationRegistry.has("nonexistent") is False


class TestIntegrationRegistryListing:
    """Test provider listing and filtering."""

    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Clear registry and register providers before each test."""
        IntegrationRegistry.clear()
        IntegrationRegistry.register(MockProvider)
        IntegrationRegistry.register(AnotherMockProvider)
        yield
        IntegrationRegistry.clear()

    def test_list_ids(self) -> None:
        """Test listing provider IDs."""
        ids = IntegrationRegistry.list_ids()

        assert "mock_provider" in ids
        assert "another_mock" in ids
        assert len(ids) == 2

    def test_list_all(self) -> None:
        """Test listing all providers with info."""
        providers = IntegrationRegistry.list_all()

        assert len(providers) == 2
        ids = [p.id for p in providers]
        assert "mock_provider" in ids
        assert "another_mock" in ids

    def test_list_all_returns_info_objects(self) -> None:
        """Test that list_all returns IntegrationInfo objects."""
        providers = IntegrationRegistry.list_all()

        mock_info = next(p for p in providers if p.id == "mock_provider")
        assert mock_info.name == "Mock Provider"
        assert mock_info.category == IntegrationCategory.MESSAGING
        assert IntegrationCapability.NOTIFICATIONS in mock_info.capabilities

    def test_list_by_category(self) -> None:
        """Test listing providers by category."""
        messaging = IntegrationRegistry.list_by_category("messaging")
        source_control = IntegrationRegistry.list_by_category("source_control")

        assert len(messaging) == 1
        assert messaging[0].id == "mock_provider"

        assert len(source_control) == 1
        assert source_control[0].id == "another_mock"

    def test_list_by_capability(self) -> None:
        """Test listing providers by capability."""
        notification_providers = IntegrationRegistry.list_by_capability("notifications")
        oauth_providers = IntegrationRegistry.list_by_capability("oauth")

        assert len(notification_providers) == 1
        assert notification_providers[0].id == "mock_provider"

        assert len(oauth_providers) == 1
        assert oauth_providers[0].id == "another_mock"


class TestIntegrationRegistryManagement:
    """Test registry management operations."""

    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Clear registry before and after each test."""
        IntegrationRegistry.clear()
        yield
        IntegrationRegistry.clear()

    def test_unregister_existing(self) -> None:
        """Test unregistering an existing provider."""
        IntegrationRegistry.register(MockProvider)
        assert IntegrationRegistry.has("mock_provider")

        result = IntegrationRegistry.unregister("mock_provider")

        assert result is True
        assert not IntegrationRegistry.has("mock_provider")

    def test_unregister_nonexistent(self) -> None:
        """Test unregistering a nonexistent provider."""
        result = IntegrationRegistry.unregister("nonexistent")
        assert result is False

    def test_clear_removes_all(self) -> None:
        """Test clear removes all providers."""
        IntegrationRegistry.register(MockProvider)
        IntegrationRegistry.register(AnotherMockProvider)
        assert len(IntegrationRegistry.list_ids()) == 2

        IntegrationRegistry.clear()

        assert len(IntegrationRegistry.list_ids()) == 0


class TestIntegrationProviderInfo:
    """Test provider info generation."""

    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Clear registry before and after each test."""
        IntegrationRegistry.clear()
        yield
        IntegrationRegistry.clear()

    def test_get_info(self) -> None:
        """Test getting provider info."""
        IntegrationRegistry.register(MockProvider)
        provider = IntegrationRegistry.get("mock_provider")

        info = provider.get_info()

        assert info.id == "mock_provider"
        assert info.name == "Mock Provider"
        assert info.category == IntegrationCategory.MESSAGING
        assert IntegrationCapability.NOTIFICATIONS in info.capabilities
        assert info.description == "A mock provider for testing"
        assert info.webhook_path == "/webhooks/mock_provider"

    def test_get_info_oauth_url(self) -> None:
        """Test that info includes OAuth URL."""
        IntegrationRegistry.register(MockProvider)
        provider = IntegrationRegistry.get("mock_provider")

        info = provider.get_info()

        assert info.oauth_url == "https://mock.example.com/oauth?state="
