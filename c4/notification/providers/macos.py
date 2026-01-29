"""macOS notification provider using osascript"""

import shutil
import subprocess
import sys

from ..base import Notification, NotificationProvider


class MacOSProvider(NotificationProvider):
    """macOS notification provider using osascript (AppleScript)"""

    def is_available(self) -> bool:
        """Check if running on macOS with osascript available"""
        return sys.platform == "darwin" and shutil.which("osascript") is not None

    def send(self, notification: Notification) -> bool:
        """
        Send notification using osascript.

        Args:
            notification: The notification to send

        Returns:
            True if sent successfully
        """
        # Build AppleScript
        parts = [f'display notification "{self._escape(notification.message)}"']
        parts.append(f'with title "{self._escape(notification.title)}"')

        if notification.subtitle:
            parts.append(f'subtitle "{self._escape(notification.subtitle)}"')

        if notification.sound:
            parts.append('sound name "Glass"')

        script = " ".join(parts)

        try:
            result = subprocess.run(
                ["osascript", "-e", script],
                capture_output=True,
                timeout=5,
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, OSError):
            return False

    @staticmethod
    def _escape(text: str) -> str:
        """Escape special characters for AppleScript"""
        return text.replace("\\", "\\\\").replace('"', '\\"')
