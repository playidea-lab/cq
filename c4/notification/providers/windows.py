"""Windows notification provider using PowerShell."""

import subprocess
import sys

from ..base import Notification, NotificationProvider


class WindowsProvider(NotificationProvider):
    """Windows notification provider using PowerShell toast notifications.

    Uses the Windows.UI.Notifications API via PowerShell.
    Works on Windows 10 and later.
    """

    @property
    def platform_name(self) -> str:
        return "Windows"

    def is_available(self) -> bool:
        """Check if running on Windows."""
        return sys.platform == "win32"

    def send(self, notification: Notification) -> bool:
        """Send notification using PowerShell toast notification.

        Args:
            notification: The notification to send.

        Returns:
            True if PowerShell executed successfully.
        """
        if not self.is_available():
            return False

        # Escape special characters for PowerShell
        title = self._escape_powershell(notification.title)
        message = self._escape_powershell(notification.message)

        # PowerShell script for toast notification
        # Uses BurntToast module if available, falls back to basic toast
        script = f'''
$ErrorActionPreference = 'SilentlyContinue'

# Try BurntToast first (if installed)
if (Get-Module -ListAvailable -Name BurntToast) {{
    Import-Module BurntToast
    New-BurntToastNotification -Text "{title}", "{message}"
    exit 0
}}

# Fallback to Windows.UI.Notifications
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
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("C4").Show($toast)
'''

        try:
            result = subprocess.run(
                ["powershell", "-NoProfile", "-Command", script],
                capture_output=True,
                text=True,
                timeout=10,
            )
            # PowerShell may return non-zero but still show notification
            # Check stderr for actual errors
            return result.returncode == 0 or not result.stderr.strip()
        except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
            return False

    @staticmethod
    def _escape_powershell(text: str) -> str:
        """Escape special characters for PowerShell strings."""
        # Escape backticks, quotes, and dollar signs
        text = text.replace("`", "``")
        text = text.replace('"', '`"')
        text = text.replace("$", "`$")
        return text
