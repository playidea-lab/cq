"""Tests for notification system."""

import sys
from unittest.mock import MagicMock, patch

import pytest

from c4.notification.base import Notification, Urgency
from c4.notification.manager import (
    NotificationManager,
    notify_checkpoint,
    notify_task_complete,
    notify_worker_event,
)
from c4.notification.providers.linux import LinuxProvider
from c4.notification.providers.macos import MacOSProvider
from c4.notification.providers.windows import WindowsProvider


class TestNotification:
    """Tests for Notification dataclass."""

    def test_notification_basic(self):
        """Test basic notification creation."""
        n = Notification(title="Test", message="Hello")
        assert n.title == "Test"
        assert n.message == "Hello"
        assert n.subtitle is None
        assert n.sound is True
        assert n.urgency == Urgency.NORMAL

    def test_notification_with_urgency_string(self):
        """Test notification with string urgency."""
        n = Notification(title="Test", message="Hello", urgency="critical")
        assert n.urgency == Urgency.CRITICAL

    def test_notification_with_urgency_enum(self):
        """Test notification with enum urgency."""
        n = Notification(title="Test", message="Hello", urgency=Urgency.LOW)
        assert n.urgency == Urgency.LOW

    def test_notification_all_fields(self):
        """Test notification with all fields."""
        n = Notification(
            title="Title",
            message="Message",
            subtitle="Subtitle",
            sound=False,
            urgency=Urgency.CRITICAL,
        )
        assert n.title == "Title"
        assert n.message == "Message"
        assert n.subtitle == "Subtitle"
        assert n.sound is False
        assert n.urgency == Urgency.CRITICAL


class TestMacOSProvider:
    """Tests for macOS notification provider."""

    def test_platform_name(self):
        """Test platform name."""
        provider = MacOSProvider()
        assert provider.platform_name == "macOS"

    def test_is_available_on_darwin(self):
        """Test availability on macOS."""
        provider = MacOSProvider()
        with patch.object(sys, "platform", "darwin"):
            assert provider.is_available() is True

    def test_is_available_on_linux(self):
        """Test not available on Linux."""
        provider = MacOSProvider()
        with patch.object(sys, "platform", "linux"):
            assert provider.is_available() is False

    def test_escape_applescript(self):
        """Test AppleScript escaping."""
        assert MacOSProvider._escape_applescript('Test "quote"') == 'Test \\"quote\\"'
        assert MacOSProvider._escape_applescript("Test\\back") == "Test\\\\back"

    def test_get_sound_for_urgency(self):
        """Test sound mapping."""
        assert MacOSProvider._get_sound_for_urgency(Urgency.LOW) == "Pop"
        assert MacOSProvider._get_sound_for_urgency(Urgency.NORMAL) == "Glass"
        assert MacOSProvider._get_sound_for_urgency(Urgency.CRITICAL) == "Sosumi"

    @patch("subprocess.run")
    def test_send_success(self, mock_run):
        """Test successful notification send."""
        mock_run.return_value = MagicMock(returncode=0)
        provider = MacOSProvider()

        with patch.object(sys, "platform", "darwin"):
            result = provider.send(Notification(title="Test", message="Hello"))

        assert result is True
        mock_run.assert_called_once()

    @patch("subprocess.run")
    def test_send_failure(self, mock_run):
        """Test failed notification send."""
        mock_run.return_value = MagicMock(returncode=1)
        provider = MacOSProvider()

        with patch.object(sys, "platform", "darwin"):
            result = provider.send(Notification(title="Test", message="Hello"))

        assert result is False


class TestLinuxProvider:
    """Tests for Linux notification provider."""

    def test_platform_name(self):
        """Test platform name."""
        provider = LinuxProvider()
        assert provider.platform_name == "Linux"

    def test_is_available_on_linux_with_notify_send(self):
        """Test availability on Linux with notify-send."""
        provider = LinuxProvider()
        with patch.object(sys, "platform", "linux"), patch(
            "shutil.which", return_value="/usr/bin/notify-send"
        ):
            assert provider.is_available() is True

    def test_is_available_on_linux_without_notify_send(self):
        """Test not available on Linux without notify-send."""
        provider = LinuxProvider()
        with patch.object(sys, "platform", "linux"), patch(
            "shutil.which", return_value=None
        ):
            assert provider.is_available() is False

    def test_is_available_on_darwin(self):
        """Test not available on macOS."""
        provider = LinuxProvider()
        with patch.object(sys, "platform", "darwin"):
            assert provider.is_available() is False

    @patch("subprocess.run")
    @patch("shutil.which", return_value="/usr/bin/notify-send")
    def test_send_success(self, mock_which, mock_run):
        """Test successful notification send."""
        mock_run.return_value = MagicMock(returncode=0)
        provider = LinuxProvider()

        with patch.object(sys, "platform", "linux"):
            result = provider.send(Notification(title="Test", message="Hello"))

        assert result is True
        mock_run.assert_called_once()


class TestWindowsProvider:
    """Tests for Windows notification provider."""

    def test_platform_name(self):
        """Test platform name."""
        provider = WindowsProvider()
        assert provider.platform_name == "Windows"

    def test_is_available_on_windows(self):
        """Test availability on Windows."""
        provider = WindowsProvider()
        with patch.object(sys, "platform", "win32"):
            assert provider.is_available() is True

    def test_is_available_on_darwin(self):
        """Test not available on macOS."""
        provider = WindowsProvider()
        with patch.object(sys, "platform", "darwin"):
            assert provider.is_available() is False

    def test_escape_powershell(self):
        """Test PowerShell escaping."""
        assert WindowsProvider._escape_powershell('Test "quote"') == 'Test `"quote`"'
        assert WindowsProvider._escape_powershell("Test$var") == "Test`$var"
        assert WindowsProvider._escape_powershell("Test`back") == "Test``back"


class TestNotificationManager:
    """Tests for NotificationManager."""

    def setup_method(self):
        """Reset manager before each test."""
        NotificationManager.reset()

    def test_get_provider_caches_result(self):
        """Test that provider selection is cached."""
        with patch.object(sys, "platform", "darwin"):
            provider1 = NotificationManager.get_provider()
            provider2 = NotificationManager.get_provider()
            assert provider1 is provider2

    @patch.object(MacOSProvider, "send", return_value=True)
    def test_notify_success(self, mock_send):
        """Test successful notification."""
        with patch.object(sys, "platform", "darwin"):
            result = NotificationManager.notify("Test", "Message")

        assert result is True
        mock_send.assert_called_once()

    @patch.object(MacOSProvider, "is_available", return_value=False)
    @patch.object(LinuxProvider, "is_available", return_value=False)
    @patch.object(WindowsProvider, "is_available", return_value=False)
    def test_notify_no_provider(self, mock_win, mock_linux, mock_mac):
        """Test notification when no provider is available."""
        result = NotificationManager.notify("Test", "Message")
        assert result is False


class TestConvenienceFunctions:
    """Tests for convenience notification functions."""

    def setup_method(self):
        """Reset manager before each test."""
        NotificationManager.reset()

    @patch.object(NotificationManager, "notify", return_value=True)
    def test_notify_task_complete_success(self, mock_notify):
        """Test task complete notification (success)."""
        result = notify_task_complete("T-001-0", success=True)
        assert result is True
        mock_notify.assert_called_once()
        args = mock_notify.call_args
        assert args.kwargs["title"] == "C4 Task Complete"
        assert args.kwargs["urgency"] == Urgency.NORMAL

    @patch.object(NotificationManager, "notify", return_value=True)
    def test_notify_task_complete_failure(self, mock_notify):
        """Test task complete notification (failure)."""
        result = notify_task_complete("T-001-0", success=False)
        assert result is True
        args = mock_notify.call_args
        assert args.kwargs["title"] == "C4 Task Failed"
        assert args.kwargs["urgency"] == Urgency.CRITICAL

    @patch.object(NotificationManager, "notify", return_value=True)
    def test_notify_checkpoint(self, mock_notify):
        """Test checkpoint notification."""
        result = notify_checkpoint("CP-001")
        assert result is True
        args = mock_notify.call_args
        assert args.kwargs["title"] == "C4 Checkpoint"
        assert args.kwargs["urgency"] == Urgency.CRITICAL

    @patch.object(NotificationManager, "notify", return_value=True)
    def test_notify_worker_event(self, mock_notify):
        """Test worker event notification."""
        result = notify_worker_event("stale", "worker-1", "T-001-0")
        assert result is True
        args = mock_notify.call_args
        assert args.kwargs["title"] == "C4 Worker Event"
        assert args.kwargs["urgency"] == Urgency.CRITICAL
