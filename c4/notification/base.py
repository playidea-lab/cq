"""Base classes for notification providers"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Literal


@dataclass
class Notification:
    """Notification data model"""

    title: str
    message: str
    subtitle: str | None = None
    sound: bool = True
    urgency: Literal["low", "normal", "critical"] = "normal"


class NotificationProvider(ABC):
    """Abstract base class for notification providers"""

    @abstractmethod
    def send(self, notification: Notification) -> bool:
        """
        Send a notification.

        Args:
            notification: The notification to send

        Returns:
            True if the notification was sent successfully
        """
        pass

    @abstractmethod
    def is_available(self) -> bool:
        """
        Check if this provider is available on the current system.

        Returns:
            True if the provider can be used
        """
        pass
