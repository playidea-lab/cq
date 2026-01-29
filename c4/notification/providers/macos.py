"""macOS notification provider using osascript."""

import subprocess
import sys

from ..base import Notification, NotificationProvider, Urgency


class MacOSProvider(NotificationProvider):
    """macOS notification provider using AppleScript (osascript).

    Uses the built-in `display notification` AppleScript command,
    which requires no additional dependencies.
    """

    @property
    def platform_name(self) -> str:
        return "macOS"

    def is_available(self) -> bool:
        """Check if running on macOS."""
        return sys.platform == "darwin"

    def send(self, notification: Notification) -> bool:
        """Send notification using osascript.

        Args:
            notification: The notification to send.

        Returns:
            True if osascript executed successfully.
        """
        if not self.is_available():
            return False

        # Build AppleScript command
        # Escape special characters for AppleScript string
        title = self._escape_applescript(notification.title)
        message = self._escape_applescript(notification.message)

        script_parts = [f'display notification "{message}" with title "{title}"']

        # Add subtitle if provided
        if notification.subtitle:
            subtitle = self._escape_applescript(notification.subtitle)
            script_parts[0] += f' subtitle "{subtitle}"'

        # Add sound based on urgency
        if notification.sound:
            sound_name = self._get_sound_for_urgency(notification.urgency)
            script_parts[0] += f' sound name "{sound_name}"'

        script = script_parts[0]

        try:
            result = subprocess.run(
                ["osascript", "-e", script],
                capture_output=True,
                text=True,
                timeout=5,
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
            return False

    @staticmethod
    def _escape_applescript(text: str) -> str:
        """Escape special characters for AppleScript strings."""
        # Escape backslashes first, then quotes
        text = text.replace("\\", "\\\\")
        text = text.replace('"', '\\"')
        return text

    @staticmethod
    def _get_sound_for_urgency(urgency: Urgency) -> str:
        """Map urgency level to macOS sound name."""
        sounds = {
            Urgency.LOW: "Pop",
            Urgency.NORMAL: "Glass",
            Urgency.CRITICAL: "Sosumi",
        }
        return sounds.get(urgency, "Glass")
