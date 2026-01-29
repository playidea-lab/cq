"""Tests for c4.notification module"""

import sys
from unittest.mock import MagicMock, patch

import pytest

from c4.notification import Notification, NotificationManager, NotificationProvider
from c4.notification.providers import LinuxProvider, MacOSProvider, WindowsProvider


class TestNotification:
    """Tests for Notification dataclass"""

    def test_notification_defaults(self):
        """Test default values"""
        n = Notification(title="Test", message="Hello")
        assert n.title == "Test"
        assert n.message == "Hello"
        assert n.subtitle is None
        assert n.sound is True
        assert n.urgency == "normal"

    def test_notification_with_all_fields(self):
        """Test with all fields specified"""
        n = Notification(
            title="Test",
            message="Hello",
            subtitle="Subtitle",
            sound=False,
            urgency="critical",
        )
        assert n.title == "Test"
        assert n.subtitle == "Subtitle"
        assert n.sound is False
        assert n.urgency == "critical"


class TestMacOSProvider:
    """Tests for MacOS notification provider"""

    def test_is_available_on_darwin(self):
        """Test availability check on macOS"""
        provider = MacOSProvider()
        with patch("sys.platform", "darwin"):
            with patch("shutil.which", return_value="/usr/bin/osascript"):
                assert provider.is_available() is True

    def test_not_available_on_linux(self):
        """Test not available on Linux"""
        provider = MacOSProvider()
        with patch("sys.platform", "linux"):
            assert provider.is_available() is False

    def test_send_success(self):
        """Test successful notification send"""
        provider = MacOSProvider()
        notification = Notification(title="Test", message="Hello")

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            result = provider.send(notification)

        assert result is True
        mock_run.assert_called_once()

    def test_send_failure(self):
        """Test failed notification send"""
        provider = MacOSProvider()
        notification = Notification(title="Test", message="Hello")

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=1)
            result = provider.send(notification)

        assert result is False

    def test_escape_special_characters(self):
        """Test escaping special characters"""
        assert MacOSProvider._escape('Test "quoted"') == 'Test \\"quoted\\"'
        assert MacOSProvider._escape("Test \\backslash") == "Test \\\\backslash"


class TestLinuxProvider:
    """Tests for Linux notification provider"""

    def test_is_available_on_linux(self):
        """Test availability check on Linux"""
        provider = LinuxProvider()
        with patch("sys.platform", "linux"):
            with patch("shutil.which", return_value="/usr/bin/notify-send"):
                assert provider.is_available() is True

    def test_not_available_on_darwin(self):
        """Test not available on macOS"""
        provider = LinuxProvider()
        with patch("sys.platform", "darwin"):
            assert provider.is_available() is False

    def test_send_success(self):
        """Test successful notification send"""
        provider = LinuxProvider()
        notification = Notification(title="Test", message="Hello", urgency="critical")

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            result = provider.send(notification)

        assert result is True
        mock_run.assert_called_once()
        # Verify urgency flag is passed
        call_args = mock_run.call_args[0][0]
        assert "--urgency" in call_args
        assert "critical" in call_args


class TestWindowsProvider:
    """Tests for Windows notification provider"""

    def test_is_available_on_windows(self):
        """Test availability check on Windows"""
        provider = WindowsProvider()
        with patch("sys.platform", "win32"):
            with patch("shutil.which", return_value="C:\\Windows\\System32\\powershell.exe"):
                assert provider.is_available() is True

    def test_not_available_on_linux(self):
        """Test not available on Linux"""
        provider = WindowsProvider()
        with patch("sys.platform", "linux"):
            assert provider.is_available() is False

    def test_escape_xml_characters(self):
        """Test escaping XML special characters"""
        assert WindowsProvider._escape("<test>") == "&lt;test&gt;"
        assert WindowsProvider._escape("a & b") == "a &amp; b"
        assert WindowsProvider._escape('"quoted"') == "&quot;quoted&quot;"


class TestNotificationManager:
    """Tests for NotificationManager"""

    def setup_method(self):
        """Reset manager state before each test"""
        NotificationManager.reset()

    def test_get_provider_returns_available(self):
        """Test that get_provider returns available provider"""

        class MockProvider(NotificationProvider):
            def is_available(self) -> bool:
                return True

            def send(self, notification: Notification) -> bool:
                return True

        NotificationManager.register_provider(MockProvider(), priority=0)
        provider = NotificationManager.get_provider()

        assert provider is not None
        assert isinstance(provider, MockProvider)

    def test_get_provider_skips_unavailable(self):
        """Test that unavailable providers are skipped"""

        class UnavailableProvider(NotificationProvider):
            def is_available(self) -> bool:
                return False

            def send(self, notification: Notification) -> bool:
                return True

        class AvailableProvider(NotificationProvider):
            def is_available(self) -> bool:
                return True

            def send(self, notification: Notification) -> bool:
                return True

        NotificationManager.reset()
        NotificationManager._providers = [UnavailableProvider(), AvailableProvider()]
        NotificationManager._initialized = False

        provider = NotificationManager.get_provider()
        assert isinstance(provider, AvailableProvider)

    def test_notify_success(self):
        """Test successful notification"""

        class MockProvider(NotificationProvider):
            sent = False

            def is_available(self) -> bool:
                return True

            def send(self, notification: Notification) -> bool:
                MockProvider.sent = True
                return True

        NotificationManager.reset()
        NotificationManager._providers = [MockProvider()]
        NotificationManager._initialized = False

        result = NotificationManager.notify("Test", "Hello")
        assert result is True
        assert MockProvider.sent is True

    def test_notify_no_provider(self):
        """Test notification when no provider available"""

        class UnavailableProvider(NotificationProvider):
            def is_available(self) -> bool:
                return False

            def send(self, notification: Notification) -> bool:
                return True

        NotificationManager.reset()
        NotificationManager._providers = [UnavailableProvider()]
        NotificationManager._initialized = False

        result = NotificationManager.notify("Test", "Hello")
        assert result is False

    def test_notify_with_all_params(self):
        """Test notification with all parameters"""
        received_notification = None

        class CaptureProvider(NotificationProvider):
            def is_available(self) -> bool:
                return True

            def send(self, notification: Notification) -> bool:
                nonlocal received_notification
                received_notification = notification
                return True

        NotificationManager.reset()
        NotificationManager._providers = [CaptureProvider()]
        NotificationManager._initialized = False

        NotificationManager.notify(
            title="Title",
            message="Message",
            subtitle="Subtitle",
            sound=False,
            urgency="critical",
        )

        assert received_notification is not None
        assert received_notification.title == "Title"
        assert received_notification.message == "Message"
        assert received_notification.subtitle == "Subtitle"
        assert received_notification.sound is False
        assert received_notification.urgency == "critical"
