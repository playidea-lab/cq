"""Tests for C4 Daemon Lifecycle management."""

import os
import signal
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.daemon.lifecycle import DaemonInfo, DaemonLifecycle, DaemonStatus


@pytest.fixture
def temp_c4_dir():
    """Create a temporary .c4 directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        c4_dir = Path(tmpdir) / ".c4"
        c4_dir.mkdir(parents=True)
        yield c4_dir


@pytest.fixture
def lifecycle(temp_c4_dir):
    """Create a DaemonLifecycle instance."""
    return DaemonLifecycle(temp_c4_dir)


class TestDaemonStatus:
    """Test DaemonStatus enum."""

    def test_status_values(self):
        """Test that all status values exist."""
        assert DaemonStatus.RUNNING.value == "running"
        assert DaemonStatus.STOPPED.value == "stopped"
        assert DaemonStatus.CRASHED.value == "crashed"
        assert DaemonStatus.UNKNOWN.value == "unknown"


class TestDaemonInfo:
    """Test DaemonInfo dataclass."""

    def test_basic_info(self):
        """Test creating basic DaemonInfo."""
        info = DaemonInfo(status=DaemonStatus.RUNNING, pid=12345)
        assert info.status == DaemonStatus.RUNNING
        assert info.pid == 12345
        assert info.uptime_seconds is None
        assert info.restart_count == 0

    def test_full_info(self):
        """Test creating DaemonInfo with all fields."""
        info = DaemonInfo(
            status=DaemonStatus.RUNNING,
            pid=12345,
            uptime_seconds=100.5,
            restart_count=2,
        )
        assert info.status == DaemonStatus.RUNNING
        assert info.pid == 12345
        assert info.uptime_seconds == 100.5
        assert info.restart_count == 2


class TestDaemonLifecycleInit:
    """Test DaemonLifecycle initialization."""

    def test_init_default_values(self, temp_c4_dir):
        """Test initialization with default values."""
        lifecycle = DaemonLifecycle(temp_c4_dir)

        assert lifecycle._c4_dir == temp_c4_dir
        assert lifecycle._graceful_timeout_sec == 10
        assert lifecycle._crash_check_interval_sec == 5
        assert lifecycle._max_restart_attempts == 3
        assert lifecycle._restart_count == 0

    def test_init_custom_values(self, temp_c4_dir):
        """Test initialization with custom values."""
        lifecycle = DaemonLifecycle(
            temp_c4_dir,
            graceful_timeout_sec=30,
            crash_check_interval_sec=10,
            max_restart_attempts=5,
            daemon_command=["echo", "test"],
        )

        assert lifecycle._graceful_timeout_sec == 30
        assert lifecycle._crash_check_interval_sec == 10
        assert lifecycle._max_restart_attempts == 5
        assert lifecycle._daemon_command == ["echo", "test"]

    def test_pid_file_path(self, lifecycle, temp_c4_dir):
        """Test PID file path is correct."""
        assert lifecycle.pid_file == temp_c4_dir / "daemon.pid"

    def test_log_file_path(self, lifecycle, temp_c4_dir):
        """Test log file path is correct."""
        assert lifecycle.log_file == temp_c4_dir / "daemon.log"


class TestPidFileManagement:
    """Test PID file read/write operations."""

    def test_read_pid_no_file(self, lifecycle):
        """Test reading PID when file doesn't exist."""
        assert lifecycle._read_pid() is None

    def test_read_pid_valid(self, lifecycle):
        """Test reading valid PID from file."""
        lifecycle.pid_file.write_text("12345")
        assert lifecycle._read_pid() == 12345

    def test_read_pid_empty_file(self, lifecycle):
        """Test reading PID from empty file."""
        lifecycle.pid_file.write_text("")
        assert lifecycle._read_pid() is None

    def test_read_pid_invalid_content(self, lifecycle):
        """Test reading PID from file with invalid content."""
        lifecycle.pid_file.write_text("not-a-number")
        assert lifecycle._read_pid() is None

    def test_write_pid(self, lifecycle):
        """Test writing PID to file."""
        lifecycle._write_pid(54321)
        assert lifecycle.pid_file.read_text() == "54321"

    def test_write_pid_creates_parent_dirs(self, temp_c4_dir):
        """Test write_pid creates parent directories if needed."""
        nested_dir = temp_c4_dir / "nested" / "daemon"
        lifecycle = DaemonLifecycle(nested_dir)

        lifecycle._write_pid(12345)

        assert lifecycle.pid_file.exists()
        assert lifecycle.pid_file.read_text() == "12345"

    def test_remove_pid(self, lifecycle):
        """Test removing PID file."""
        lifecycle.pid_file.write_text("12345")
        assert lifecycle.pid_file.exists()

        lifecycle._remove_pid()

        assert not lifecycle.pid_file.exists()

    def test_remove_pid_no_file(self, lifecycle):
        """Test removing PID file when it doesn't exist."""
        # Should not raise
        lifecycle._remove_pid()


class TestProcessRunningCheck:
    """Test process running detection."""

    def test_is_process_running_current_process(self, lifecycle):
        """Test checking if current process is running."""
        assert lifecycle._is_process_running(os.getpid()) is True

    def test_is_process_running_invalid_pid(self, lifecycle):
        """Test checking if invalid PID is running."""
        # PID 0 should not exist as a normal process
        assert lifecycle._is_process_running(99999999) is False


class TestDaemonStatusMethod:
    """Test status() method."""

    def test_status_no_pid_file(self, lifecycle):
        """Test status when no PID file exists."""
        info = lifecycle.status()
        assert info.status == DaemonStatus.STOPPED
        assert info.pid is None

    def test_status_process_running(self, lifecycle):
        """Test status when process is running."""
        # Write current process PID
        lifecycle._write_pid(os.getpid())

        info = lifecycle.status()

        assert info.status == DaemonStatus.RUNNING
        assert info.pid == os.getpid()

    def test_status_process_crashed(self, lifecycle):
        """Test status when process crashed (PID file exists but process not running)."""
        # Write non-existent PID
        lifecycle._write_pid(99999999)

        info = lifecycle.status()

        assert info.status == DaemonStatus.CRASHED
        assert info.pid == 99999999


class TestDaemonStart:
    """Test start() method."""

    def test_start_already_running(self, lifecycle):
        """Test start when daemon already running."""
        lifecycle._write_pid(os.getpid())

        info = lifecycle.start()

        assert info.status == DaemonStatus.RUNNING
        assert info.pid == os.getpid()

    @patch("subprocess.Popen")
    def test_start_new_daemon(self, mock_popen, lifecycle):
        """Test starting new daemon process."""
        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        # Mock _is_process_running to return True for the new process
        with patch.object(lifecycle, "_is_process_running", return_value=True):
            info = lifecycle.start()

        assert info.status == DaemonStatus.RUNNING
        assert info.pid == 54321
        assert info.restart_count == 0
        assert lifecycle.pid_file.read_text() == "54321"

    @patch("subprocess.Popen")
    def test_start_process_exits_immediately(self, mock_popen, lifecycle):
        """Test start when process exits immediately."""
        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        # Mock _is_process_running to return False (process exited)
        with patch.object(lifecycle, "_is_process_running", return_value=False):
            with pytest.raises(RuntimeError, match="exited immediately"):
                lifecycle.start()

        # PID file should be cleaned up
        assert not lifecycle.pid_file.exists()

    @patch("subprocess.Popen")
    def test_start_cleans_up_crashed_state(self, mock_popen, lifecycle):
        """Test start cleans up crashed state before starting."""
        # Simulate crashed state
        lifecycle._write_pid(99999999)

        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        with patch.object(lifecycle, "_is_process_running") as mock_running:
            # First call checks crashed PID, second checks new process
            mock_running.side_effect = [False, True]
            info = lifecycle.start()

        assert info.status == DaemonStatus.RUNNING
        assert info.pid == 54321


class TestDaemonStop:
    """Test stop() method."""

    def test_stop_no_pid_file(self, lifecycle):
        """Test stop when no PID file exists."""
        info = lifecycle.stop()
        assert info.status == DaemonStatus.STOPPED

    def test_stop_process_already_stopped(self, lifecycle):
        """Test stop when process already stopped."""
        lifecycle._write_pid(99999999)

        info = lifecycle.stop()

        assert info.status == DaemonStatus.STOPPED
        assert not lifecycle.pid_file.exists()

    @patch("os.kill")
    def test_stop_graceful_shutdown(self, mock_kill, lifecycle):
        """Test graceful shutdown with SIGTERM."""
        lifecycle._write_pid(12345)

        # Make _is_process_running return True once (for initial check),
        # then False (after SIGTERM)
        with patch.object(lifecycle, "_is_process_running") as mock_running:
            mock_running.side_effect = [True, False]
            info = lifecycle.stop()

        mock_kill.assert_called_once_with(12345, signal.SIGTERM)
        assert info.status == DaemonStatus.STOPPED
        assert not lifecycle.pid_file.exists()

    @patch("os.kill")
    def test_stop_force_kill(self, mock_kill, lifecycle):
        """Test force kill with SIGKILL."""
        lifecycle._write_pid(12345)

        with patch.object(lifecycle, "_is_process_running", return_value=True):
            lifecycle.stop(force=True)

        mock_kill.assert_called_once_with(12345, signal.SIGKILL)

    @patch("os.kill")
    @patch("time.sleep")
    def test_stop_falls_back_to_sigkill(self, mock_sleep, mock_kill, lifecycle):
        """Test fallback to SIGKILL after graceful timeout."""
        lifecycle._write_pid(12345)
        lifecycle._graceful_timeout_sec = 0.1  # Short timeout for test

        # Process doesn't stop after SIGTERM
        with patch.object(lifecycle, "_is_process_running", return_value=True):
            with patch("time.time") as mock_time:
                # Simulate time passing beyond timeout
                mock_time.side_effect = [0, 0.2, 0.3]
                lifecycle.stop()

        # Should call SIGTERM then SIGKILL
        calls = mock_kill.call_args_list
        assert len(calls) >= 2
        assert calls[0] == ((12345, signal.SIGTERM),)
        assert calls[1] == ((12345, signal.SIGKILL),)


class TestDaemonRestart:
    """Test restart() method."""

    @patch("subprocess.Popen")
    def test_restart_increments_count(self, mock_popen, lifecycle):
        """Test restart increments restart count."""
        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        with patch.object(lifecycle, "_is_process_running", return_value=True):
            info = lifecycle.restart()

        assert info.restart_count == 1

    @patch("subprocess.Popen")
    def test_restart_multiple_times(self, mock_popen, lifecycle):
        """Test multiple restarts increment count."""
        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        with patch.object(lifecycle, "_is_process_running", return_value=True):
            lifecycle.restart()
            info = lifecycle.restart()

        assert info.restart_count == 2


class TestCrashRecovery:
    """Test crash recovery functionality."""

    def test_recover_if_not_crashed(self, lifecycle):
        """Test recover_if_crashed when daemon not crashed."""
        info = lifecycle.recover_if_crashed()
        assert info.status == DaemonStatus.STOPPED

    @patch("subprocess.Popen")
    def test_recover_if_crashed_restarts(self, mock_popen, lifecycle):
        """Test crash recovery restarts daemon."""
        # Simulate crashed state
        lifecycle._write_pid(99999999)

        mock_process = MagicMock()
        mock_process.pid = 54321
        mock_popen.return_value = mock_process

        with patch.object(lifecycle, "_is_process_running") as mock_running:
            mock_running.side_effect = [False, True]
            info = lifecycle.recover_if_crashed()

        assert info.status == DaemonStatus.RUNNING
        assert info.restart_count == 1

    def test_recover_max_attempts_reached(self, lifecycle):
        """Test crash recovery gives up after max attempts."""
        lifecycle._write_pid(99999999)
        lifecycle._restart_count = 3  # Max attempts

        info = lifecycle.recover_if_crashed()

        assert info.status == DaemonStatus.CRASHED
        assert info.restart_count == 3


class TestHelperMethods:
    """Test helper methods."""

    def test_is_running_true(self, lifecycle):
        """Test is_running returns True when running."""
        lifecycle._write_pid(os.getpid())
        assert lifecycle.is_running() is True

    def test_is_running_false(self, lifecycle):
        """Test is_running returns False when not running."""
        assert lifecycle.is_running() is False

    def test_reset_restart_count(self, lifecycle):
        """Test reset_restart_count."""
        lifecycle._restart_count = 5
        lifecycle.reset_restart_count()
        assert lifecycle._restart_count == 0
