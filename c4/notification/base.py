"""Base classes for notification providers."""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from enum import Enum
from typing import Literal


class Urgency(str, Enum):
    """Notification urgency levels."""

    LOW = "low"
    NORMAL = "normal"
    CRITICAL = "critical"


@dataclass
class Notification:
    """Cross-platform notification data.

    Attributes:
        title: Notification title (required)
        message: Notification body text (required)
        subtitle: Optional subtitle (macOS only)
        sound: Whether to play a sound (default: True)
        urgency: Urgency level (low, normal, critical)
    """

    title: str
    message: str
    subtitle: str | None = None
    sound: bool = True
    urgency: Urgency | Literal["low", "normal", "critical"] = Urgency.NORMAL

    def __post_init__(self) -> None:
        """Convert string urgency to Urgency enum."""
        if isinstance(self.urgency, str):
            self.urgency = Urgency(self.urgency)


class NotificationProvider(ABC):
    """Abstract base class for platform-specific notification providers."""

    @property
    @abstractmethod
    def platform_name(self) -> str:
        """Return the platform name (e.g., 'macOS', 'Linux', 'Windows')."""
        pass

    @abstractmethod
    def is_available(self) -> bool:
        """Check if this provider is available on the current system.

        Returns:
            True if the provider can send notifications on this system.
        """
        pass

    @abstractmethod
    def send(self, notification: Notification) -> bool:
        """Send a notification.

        Args:
            notification: The notification to send.

        Returns:
            True if the notification was sent successfully.
        """
        pass
