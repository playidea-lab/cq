"""Notification manager with automatic provider selection."""

import logging
from typing import Literal

from .base import Notification, NotificationProvider, Urgency
from .providers import LinuxProvider, MacOSProvider, WindowsProvider

logger = logging.getLogger(__name__)


class NotificationManager:
    """Manager for sending cross-platform notifications.

    Automatically selects the appropriate notification provider
    based on the current platform.

    Usage:
        # Simple usage
        NotificationManager.notify("Task Complete", "T-001-0 finished successfully")

        # With urgency
        NotificationManager.notify(
            "Checkpoint Required",
            "CP-001 requires review",
            urgency="critical"
        )
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
        """Get the appropriate notification provider for this platform.

        Returns:
            NotificationProvider if one is available, None otherwise.
        """
        if not cls._initialized:
            cls._initialized = True
            for provider in cls._providers:
                if provider.is_available():
                    cls._current = provider
                    logger.debug(
                        f"Notification provider selected: {provider.platform_name}"
                    )
                    break
            if cls._current is None:
                logger.debug("No notification provider available")

        return cls._current

    @classmethod
    def notify(
        cls,
        title: str,
        message: str,
        subtitle: str | None = None,
        sound: bool = True,
        urgency: Urgency | Literal["low", "normal", "critical"] = Urgency.NORMAL,
    ) -> bool:
        """Send a notification.

        Args:
            title: Notification title.
            message: Notification body text.
            subtitle: Optional subtitle (macOS only).
            sound: Whether to play a sound.
            urgency: Urgency level (low, normal, critical).

        Returns:
            True if notification was sent successfully, False otherwise.
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
            result = provider.send(notification)
            if result:
                logger.debug(f"Notification sent: {title}")
            else:
                logger.warning(f"Failed to send notification: {title}")
            return result
        except Exception as e:
            logger.warning(f"Error sending notification: {e}")
            return False

    @classmethod
    def reset(cls) -> None:
        """Reset the manager state (primarily for testing)."""
        cls._current = None
        cls._initialized = False


# Convenience functions for common C4 notifications
def notify_task_complete(task_id: str, success: bool, summary: str | None = None) -> bool:
    """Send notification when a task completes.

    Args:
        task_id: The completed task ID.
        success: Whether the task succeeded.
        summary: Optional summary message.

    Returns:
        True if notification was sent.
    """
    if success:
        title = "C4 Task Complete"
        message = summary or f"{task_id} completed successfully"
        urgency = Urgency.NORMAL
    else:
        title = "C4 Task Failed"
        message = summary or f"{task_id} failed"
        urgency = Urgency.CRITICAL

    return NotificationManager.notify(
        title=title,
        message=message,
        urgency=urgency,
    )


def notify_checkpoint(checkpoint_id: str, message: str | None = None) -> bool:
    """Send notification when a checkpoint requires review.

    Args:
        checkpoint_id: The checkpoint ID.
        message: Optional message.

    Returns:
        True if notification was sent.
    """
    return NotificationManager.notify(
        title="C4 Checkpoint",
        message=message or f"{checkpoint_id} requires review",
        urgency=Urgency.CRITICAL,
    )


def notify_worker_event(
    event: Literal["joined", "left", "stale"],
    worker_id: str,
    task_id: str | None = None,
) -> bool:
    """Send notification for worker events.

    Args:
        event: The event type.
        worker_id: The worker ID.
        task_id: Optional task ID (for stale workers).

    Returns:
        True if notification was sent.
    """
    messages = {
        "joined": f"Worker {worker_id} joined",
        "left": f"Worker {worker_id} left",
        "stale": f"Worker {worker_id} is stale (task: {task_id})",
    }

    urgency = Urgency.CRITICAL if event == "stale" else Urgency.LOW

    return NotificationManager.notify(
        title="C4 Worker Event",
        message=messages.get(event, f"Worker {worker_id}: {event}"),
        urgency=urgency,
    )
