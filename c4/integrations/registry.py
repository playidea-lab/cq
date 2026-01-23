"""Integration Provider Registry.

Provides a central registry for all integration providers.
Supports dynamic registration via decorators and lookup by ID.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .base import IntegrationInfo, IntegrationProvider

logger = logging.getLogger(__name__)


class IntegrationRegistry:
    """Central registry for integration providers.

    This is a singleton class that manages all available integration providers.
    Providers register themselves using the @IntegrationRegistry.register decorator.

    Example:
        # Register a provider
        @IntegrationRegistry.register
        class GitHubProvider(IntegrationProvider):
            @property
            def id(self) -> str:
                return "github"
            # ...

        # Get a provider
        provider = IntegrationRegistry.get("github")
        if provider:
            url = provider.get_oauth_url(state)

        # List all providers
        providers = IntegrationRegistry.list_all()
    """

    _providers: dict[str, type[IntegrationProvider]] = {}
    _instances: dict[str, IntegrationProvider] = {}

    @classmethod
    def register(cls, provider_class: type[IntegrationProvider]) -> type[IntegrationProvider]:
        """Register a provider class.

        Use as a decorator:
            @IntegrationRegistry.register
            class MyProvider(IntegrationProvider):
                ...

        Args:
            provider_class: Provider class to register

        Returns:
            The same class (for decorator use)
        """
        # Create a temporary instance to get the ID
        instance = provider_class()
        provider_id = instance.id

        if provider_id in cls._providers:
            logger.warning(f"Provider '{provider_id}' is being re-registered")

        cls._providers[provider_id] = provider_class
        cls._instances[provider_id] = instance  # Cache the instance

        logger.debug(f"Registered integration provider: {provider_id}")

        return provider_class

    @classmethod
    def get(cls, provider_id: str) -> IntegrationProvider | None:
        """Get a provider instance by ID.

        Args:
            provider_id: Provider ID (e.g., 'github', 'discord')

        Returns:
            Provider instance or None if not found
        """
        # Return cached instance if available
        if provider_id in cls._instances:
            return cls._instances[provider_id]

        # Create new instance if class is registered
        provider_class = cls._providers.get(provider_id)
        if provider_class:
            instance = provider_class()
            cls._instances[provider_id] = instance
            return instance

        return None

    @classmethod
    def get_or_raise(cls, provider_id: str) -> IntegrationProvider:
        """Get a provider instance by ID, raising if not found.

        Args:
            provider_id: Provider ID

        Returns:
            Provider instance

        Raises:
            ValueError: If provider not found
        """
        provider = cls.get(provider_id)
        if provider is None:
            available = list(cls._providers.keys())
            raise ValueError(f"Unknown provider: {provider_id}. Available: {available}")
        return provider

    @classmethod
    def list_all(cls) -> list[IntegrationInfo]:
        """List all registered providers with their info.

        Returns:
            List of IntegrationInfo objects
        """
        result = []
        for provider_id in cls._providers:
            provider = cls.get(provider_id)
            if provider:
                result.append(provider.get_info())
        return result

    @classmethod
    def list_ids(cls) -> list[str]:
        """List all registered provider IDs.

        Returns:
            List of provider IDs
        """
        return list(cls._providers.keys())

    @classmethod
    def list_by_category(cls, category: str) -> list[IntegrationInfo]:
        """List providers by category.

        Args:
            category: Category to filter by

        Returns:
            List of IntegrationInfo objects in that category
        """
        return [
            info for info in cls.list_all() if info.category.value == category
        ]

    @classmethod
    def list_by_capability(cls, capability: str) -> list[IntegrationInfo]:
        """List providers by capability.

        Args:
            capability: Capability to filter by

        Returns:
            List of IntegrationInfo objects with that capability
        """
        return [
            info
            for info in cls.list_all()
            if any(cap.value == capability for cap in info.capabilities)
        ]

    @classmethod
    def has(cls, provider_id: str) -> bool:
        """Check if a provider is registered.

        Args:
            provider_id: Provider ID to check

        Returns:
            True if provider is registered
        """
        return provider_id in cls._providers

    @classmethod
    def unregister(cls, provider_id: str) -> bool:
        """Unregister a provider (mainly for testing).

        Args:
            provider_id: Provider ID to unregister

        Returns:
            True if provider was unregistered
        """
        if provider_id in cls._providers:
            del cls._providers[provider_id]
            cls._instances.pop(provider_id, None)
            return True
        return False

    @classmethod
    def clear(cls) -> None:
        """Clear all registered providers (mainly for testing)."""
        cls._providers.clear()
        cls._instances.clear()


def auto_discover_providers() -> None:
    """Auto-discover and register all providers in the integrations package.

    This imports all provider modules to trigger their @register decorators.
    Call this at application startup.
    """
    # Import provider modules to trigger registration
    # Each module uses @IntegrationRegistry.register on its provider class

    try:
        from . import github_provider  # noqa: F401

        logger.debug("Loaded github_provider")
    except ImportError as e:
        logger.debug(f"github_provider not available: {e}")

    try:
        from . import discord_provider  # noqa: F401

        logger.debug("Loaded discord_provider")
    except ImportError as e:
        logger.debug(f"discord_provider not available: {e}")

    try:
        from . import dooray_provider  # noqa: F401

        logger.debug("Loaded dooray_provider")
    except ImportError as e:
        logger.debug(f"dooray_provider not available: {e}")

    logger.info(f"Discovered {len(IntegrationRegistry.list_ids())} integration providers")
