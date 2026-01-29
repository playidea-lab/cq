"""Linux notification provider using notify-send."""

import shutil
import subprocess
import sys

from ..base import Notification, NotificationProvider, Urgency


class LinuxProvider(NotificationProvider):
    """Linux notification provider using notify-send.

    Requires libnotify (notify-send) to be installed.
    On most distributions: `apt install libnotify-bin` or `dnf install libnotify`
    """

    @property
    def platform_name(self) -> str:
        return "Linux"

    def is_available(self) -> bool:
        """Check if running on Linux and notify-send is available."""
        if sys.platform != "linux":
            return False
        return shutil.which("notify-send") is not None

    def send(self, notification: Notification) -> bool:
        """Send notification using notify-send.

        Args:
            notification: The notification to send.

        Returns:
            True if notify-send executed successfully.
        """
        if not self.is_available():
            return False

        # Map urgency to notify-send urgency levels
        urgency_map = {
            Urgency.LOW: "low",
            Urgency.NORMAL: "normal",
            Urgency.CRITICAL: "critical",
        }
        urgency_str = urgency_map.get(notification.urgency, "normal")

        # Build command arguments
        cmd = [
            "notify-send",
            "--urgency",
            urgency_str,
        ]

        # Add app name for identification
        cmd.extend(["--app-name", "C4"])

        # Add expiration time based on urgency (in milliseconds)
        expire_times = {
            Urgency.LOW: "5000",
            Urgency.NORMAL: "10000",
            Urgency.CRITICAL: "0",  # 0 = never expire
        }
        cmd.extend(["--expire-time", expire_times.get(notification.urgency, "10000")])

        # Title and message
        cmd.append(notification.title)

        # Combine subtitle and message if subtitle is provided
        message = notification.message
        if notification.subtitle:
            message = f"{notification.subtitle}\n{message}"
        cmd.append(message)

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=5,
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
            return False
