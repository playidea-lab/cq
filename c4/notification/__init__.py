"""C4 Notification System - Cross-platform desktop notifications."""

from .base import Notification, NotificationProvider
from .manager import NotificationManager

__all__ = [
    "Notification",
    "NotificationProvider",
    "NotificationManager",
]
