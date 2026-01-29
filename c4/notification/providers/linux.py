"""Linux notification provider using notify-send"""

import shutil
import subprocess
import sys

from ..base import Notification, NotificationProvider


class LinuxProvider(NotificationProvider):
    """Linux notification provider using notify-send (libnotify)"""

    def is_available(self) -> bool:
        """Check if running on Linux with notify-send available"""
        return sys.platform == "linux" and shutil.which("notify-send") is not None

    def send(self, notification: Notification) -> bool:
        """
        Send notification using notify-send.

        Args:
            notification: The notification to send

        Returns:
            True if sent successfully
        """
        urgency_map = {
            "low": "low",
            "normal": "normal",
            "critical": "critical",
        }

        cmd = [
            "notify-send",
            "--urgency",
            urgency_map[notification.urgency],
        ]

        # Add app name
        cmd.extend(["--app-name", "C4"])

        # Add title and message
        cmd.append(notification.title)

        # Build body with subtitle if present
        body = notification.message
        if notification.subtitle:
            body = f"{notification.subtitle}\n{body}"
        cmd.append(body)

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                timeout=5,
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, OSError):
            return False
