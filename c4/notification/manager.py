"""Notification manager with automatic provider selection"""

import logging
from typing import Literal

from .base import Notification, NotificationProvider
from .providers import LinuxProvider, MacOSProvider, WindowsProvider

logger = logging.getLogger(__name__)


class NotificationManager:
    """
    Manager for sending notifications with automatic provider selection.

    Automatically detects the current platform and selects the appropriate
    notification provider. Falls back gracefully if no provider is available.
    """

    _providers: list[NotificationProvider] = [
        MacOSProvider(),
        LinuxProvider(),
        WindowsProvider(),
    ]
    _current: NotificationProvider | None = None
    _initialized: bool = False

    @classmethod
    def get_provider(cls) -> NotificationProvider | None:
        """
        Get the notification provider for the current platform.

        Returns:
            NotificationProvider if available, None otherwise
        """
        if not cls._initialized:
            cls._initialized = True
            for provider in cls._providers:
                if provider.is_available():
                    cls._current = provider
                    logger.debug(f"Selected notification provider: {provider.__class__.__name__}")
                    break
            else:
                logger.warning("No notification provider available")
        return cls._current

    @classmethod
    def notify(
        cls,
        title: str,
        message: str,
        subtitle: str | None = None,
        sound: bool = True,
        urgency: Literal["low", "normal", "critical"] = "normal",
    ) -> bool:
        """
        Send a notification.

        Args:
            title: Notification title
            message: Notification message body
            subtitle: Optional subtitle (may not be supported by all providers)
            sound: Whether to play a sound
            urgency: Urgency level (low, normal, critical)

        Returns:
            True if notification was sent successfully
        """
        provider = cls.get_provider()
        if provider is None:
            logger.debug("No notification provider available, skipping notification")
            return False

        notification = Notification(
            title=title,
            message=message,
            subtitle=subtitle,
            sound=sound,
            urgency=urgency,
        )

        try:
            success = provider.send(notification)
            if not success:
                logger.warning(f"Failed to send notification: {title}")
            return success
        except Exception as e:
            logger.error(f"Error sending notification: {e}")
            return False

    @classmethod
    def reset(cls) -> None:
        """Reset the manager state (mainly for testing)"""
        cls._current = None
        cls._initialized = False

    @classmethod
    def register_provider(cls, provider: NotificationProvider, priority: int = 0) -> None:
        """
        Register a custom notification provider.

        Args:
            provider: The provider to register
            priority: Position in the provider list (0 = first)
        """
        cls._providers.insert(priority, provider)
        cls._initialized = False  # Force re-detection
