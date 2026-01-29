"""Windows notification provider using PowerShell"""

import shutil
import subprocess
import sys

from ..base import Notification, NotificationProvider


class WindowsProvider(NotificationProvider):
    """Windows notification provider using PowerShell toast notifications"""

    def is_available(self) -> bool:
        """Check if running on Windows with PowerShell available"""
        return sys.platform == "win32" and shutil.which("powershell") is not None

    def send(self, notification: Notification) -> bool:
        """
        Send notification using PowerShell toast notification.

        Args:
            notification: The notification to send

        Returns:
            True if sent successfully
        """
        # Build PowerShell script for toast notification
        # Uses Windows 10+ toast notification API
        title = self._escape(notification.title)
        message = self._escape(notification.message)

        if notification.subtitle:
            message = f"{self._escape(notification.subtitle)}\\n{message}"

        # PowerShell script - line length is required for the API calls
        script = f"""
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$template = @"
<toast>
    <visual>
        <binding template="ToastText02">
            <text id="1">{title}</text>
            <text id="2">{message}</text>
        </binding>
    </visual>
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("C4").Show($toast)
"""

        try:
            result = subprocess.run(
                ["powershell", "-NoProfile", "-Command", script],
                capture_output=True,
                timeout=10,
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, OSError):
            return False

    @staticmethod
    def _escape(text: str) -> str:
        """Escape special characters for XML"""
        return (
            text.replace("&", "&amp;")
            .replace("<", "&lt;")
            .replace(">", "&gt;")
            .replace('"', "&quot;")
            .replace("'", "&apos;")
        )
