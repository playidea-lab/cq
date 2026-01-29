"""C4 Daemon Lifecycle - Process lifecycle management for C4 daemon.

This module provides the DaemonLifecycle class for managing the C4 daemon
process, including starting, stopping, and monitoring the daemon.
"""

from __future__ import annotations

import logging
import os
import signal
import subprocess
import sys
import time
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


class DaemonStatus(Enum):
    """Status of the daemon process."""

    RUNNING = "running"
    STOPPED = "stopped"
    CRASHED = "crashed"
    UNKNOWN = "unknown"


@dataclass
class DaemonInfo:
    """Information about the daemon process.

    Attributes:
        status: Current status of the daemon
        pid: Process ID if running, None otherwise
        uptime_seconds: Time since daemon started, None if not running
        restart_count: Number of times daemon has been restarted
    """

    status: DaemonStatus
    pid: int | None = None
    uptime_seconds: float | None = None
    restart_count: int = 0


class DaemonLifecycle:
    """Manages the lifecycle of the C4 daemon process.

    This class provides methods to start, stop, and monitor the C4 daemon
    process. It uses PID files to track process state and supports graceful
    shutdown with automatic crash recovery.

    Example:
        >>> lifecycle = DaemonLifecycle(Path("/path/to/project/.c4"))
        >>> lifecycle.start()
        >>> info = lifecycle.status()
        >>> print(f"Daemon running with PID {info.pid}")
        >>> lifecycle.stop()
    """

    # Default timeout for graceful shutdown before SIGKILL
    DEFAULT_GRACEFUL_TIMEOUT_SEC = 10

    # Default interval for crash detection
    DEFAULT_CRASH_CHECK_INTERVAL_SEC = 5

    # Maximum restart attempts before giving up
    DEFAULT_MAX_RESTART_ATTEMPTS = 3

    def __init__(
        self,
        c4_dir: Path,
        *,
        graceful_timeout_sec: float = DEFAULT_GRACEFUL_TIMEOUT_SEC,
        crash_check_interval_sec: float = DEFAULT_CRASH_CHECK_INTERVAL_SEC,
        max_restart_attempts: int = DEFAULT_MAX_RESTART_ATTEMPTS,
        daemon_command: list[str] | None = None,
    ) -> None:
        """Initialize the DaemonLifecycle.

        Args:
            c4_dir: Path to the .c4 directory
            graceful_timeout_sec: Seconds to wait for graceful shutdown
            crash_check_interval_sec: Interval for crash detection checks
            max_restart_attempts: Maximum restart attempts on crash
            daemon_command: Command to start the daemon (default: python -m c4.mcp_server)
        """
        self._c4_dir = c4_dir
        self._graceful_timeout_sec = graceful_timeout_sec
        self._crash_check_interval_sec = crash_check_interval_sec
        self._max_restart_attempts = max_restart_attempts
        self._daemon_command = daemon_command or self._default_daemon_command()

        self._restart_count = 0
        self._start_time: float | None = None

    @property
    def pid_file(self) -> Path:
        """Path to the PID file."""
        return self._c4_dir / "daemon.pid"

    @property
    def log_file(self) -> Path:
        """Path to the daemon log file."""
        return self._c4_dir / "daemon.log"

    def _default_daemon_command(self) -> list[str]:
        """Get the default command to start the daemon."""
        return [sys.executable, "-m", "c4.mcp_server"]

    def _read_pid(self) -> int | None:
        """Read the PID from the PID file.

        Returns:
            The PID if file exists and is valid, None otherwise.
        """
        if not self.pid_file.exists():
            return None

        try:
            pid_str = self.pid_file.read_text().strip()
            if not pid_str:
                return None
            return int(pid_str)
        except (ValueError, OSError) as e:
            logger.warning(f"Failed to read PID file: {e}")
            return None

    def _write_pid(self, pid: int) -> None:
        """Write the PID to the PID file.

        Args:
            pid: Process ID to write
        """
        self.pid_file.parent.mkdir(parents=True, exist_ok=True)
        self.pid_file.write_text(str(pid))
        logger.debug(f"Wrote PID {pid} to {self.pid_file}")

    def _remove_pid(self) -> None:
        """Remove the PID file."""
        try:
            if self.pid_file.exists():
                self.pid_file.unlink()
                logger.debug(f"Removed PID file {self.pid_file}")
        except OSError as e:
            logger.warning(f"Failed to remove PID file: {e}")

    def _is_process_running(self, pid: int) -> bool:
        """Check if a process with the given PID is running.

        Args:
            pid: Process ID to check

        Returns:
            True if process is running, False otherwise
        """
        try:
            # Signal 0 doesn't actually send a signal but checks if process exists
            os.kill(pid, 0)
            return True
        except ProcessLookupError:
            return False
        except PermissionError:
            # Process exists but we don't have permission to signal it
            return True
        except OSError:
            return False

    def start(self, *, auto_restart: bool = True) -> DaemonInfo:
        """Start the daemon process.

        If the daemon is already running, returns its current status.
        Otherwise, starts a new daemon process in the background.

        Args:
            auto_restart: Whether to enable automatic restart on crash

        Returns:
            DaemonInfo with the current status

        Raises:
            RuntimeError: If daemon fails to start
        """
        # Check if already running
        current_status = self.status()
        if current_status.status == DaemonStatus.RUNNING:
            logger.info(f"Daemon already running with PID {current_status.pid}")
            return current_status

        # Clean up stale PID file if crashed
        if current_status.status == DaemonStatus.CRASHED:
            logger.info("Cleaning up stale PID file from crashed daemon")
            self._remove_pid()

        # Start the daemon process
        logger.info(f"Starting daemon with command: {self._daemon_command}")

        try:
            # Open log file for daemon output
            self.log_file.parent.mkdir(parents=True, exist_ok=True)
            log_handle = self.log_file.open("a")

            # Start process in background
            # Note: We use start_new_session=True to detach from terminal
            process = subprocess.Popen(
                self._daemon_command,
                stdout=log_handle,
                stderr=subprocess.STDOUT,
                cwd=self._c4_dir.parent,  # Project root
                start_new_session=True,
            )

            # Write PID file
            self._write_pid(process.pid)
            self._start_time = time.time()
            # Note: Don't reset restart_count here - it's managed by restart()/recover_if_crashed()

            logger.info(f"Daemon started with PID {process.pid}")

            # Verify process started successfully
            time.sleep(0.1)  # Brief wait to detect immediate failures
            if not self._is_process_running(process.pid):
                self._remove_pid()
                raise RuntimeError("Daemon process exited immediately after start")

            return DaemonInfo(
                status=DaemonStatus.RUNNING,
                pid=process.pid,
                uptime_seconds=0,
                restart_count=self._restart_count,
            )

        except Exception as e:
            logger.error(f"Failed to start daemon: {e}")
            self._remove_pid()
            raise RuntimeError(f"Failed to start daemon: {e}") from e

    def stop(self, *, force: bool = False) -> DaemonInfo:
        """Stop the daemon process.

        Attempts graceful shutdown with SIGTERM first, then SIGKILL
        if the process doesn't stop within the timeout.

        Args:
            force: If True, immediately send SIGKILL without graceful shutdown

        Returns:
            DaemonInfo with the final status
        """
        pid = self._read_pid()
        if pid is None:
            logger.info("No PID file found, daemon not running")
            return DaemonInfo(status=DaemonStatus.STOPPED)

        if not self._is_process_running(pid):
            logger.info(f"Daemon process {pid} not running, cleaning up PID file")
            self._remove_pid()
            return DaemonInfo(status=DaemonStatus.STOPPED)

        logger.info(f"Stopping daemon with PID {pid}")

        try:
            if force:
                # Immediate kill
                os.kill(pid, signal.SIGKILL)
                logger.info(f"Sent SIGKILL to daemon process {pid}")
            else:
                # Graceful shutdown
                os.kill(pid, signal.SIGTERM)
                logger.info(f"Sent SIGTERM to daemon process {pid}")

                # Wait for graceful shutdown
                deadline = time.time() + self._graceful_timeout_sec
                while time.time() < deadline:
                    if not self._is_process_running(pid):
                        logger.info(f"Daemon process {pid} stopped gracefully")
                        break
                    time.sleep(0.1)
                else:
                    # Graceful shutdown failed, force kill
                    logger.warning(
                        f"Daemon did not stop gracefully after {self._graceful_timeout_sec}s, "
                        f"sending SIGKILL"
                    )
                    os.kill(pid, signal.SIGKILL)
                    time.sleep(0.5)  # Brief wait for SIGKILL to take effect

        except ProcessLookupError:
            logger.info(f"Daemon process {pid} already stopped")
        except PermissionError as e:
            logger.error(f"Permission denied stopping daemon {pid}: {e}")
            return DaemonInfo(status=DaemonStatus.UNKNOWN, pid=pid)

        # Clean up PID file
        self._remove_pid()
        self._start_time = None

        return DaemonInfo(status=DaemonStatus.STOPPED)

    def status(self) -> DaemonInfo:
        """Get the current status of the daemon.

        Returns:
            DaemonInfo with the current status
        """
        pid = self._read_pid()

        if pid is None:
            return DaemonInfo(status=DaemonStatus.STOPPED)

        if self._is_process_running(pid):
            uptime = None
            if self._start_time is not None:
                uptime = time.time() - self._start_time

            return DaemonInfo(
                status=DaemonStatus.RUNNING,
                pid=pid,
                uptime_seconds=uptime,
                restart_count=self._restart_count,
            )

        # PID file exists but process not running = crashed
        return DaemonInfo(
            status=DaemonStatus.CRASHED,
            pid=pid,
            restart_count=self._restart_count,
        )

    def restart(self) -> DaemonInfo:
        """Restart the daemon process.

        Stops the daemon if running, then starts it again.

        Returns:
            DaemonInfo with the new status
        """
        logger.info("Restarting daemon")
        self.stop()
        self._restart_count += 1
        return self.start()

    def recover_if_crashed(self) -> DaemonInfo:
        """Check for crashes and restart if needed.

        This method implements the crash recovery logic. It checks if the
        daemon has crashed (PID file exists but process not running) and
        restarts it if under the maximum restart attempts.

        Returns:
            DaemonInfo with the current/new status
        """
        current_status = self.status()

        if current_status.status != DaemonStatus.CRASHED:
            return current_status

        # Daemon has crashed
        if self._restart_count >= self._max_restart_attempts:
            logger.error(
                f"Daemon crashed but max restart attempts ({self._max_restart_attempts}) "
                f"reached. Manual intervention required."
            )
            return DaemonInfo(
                status=DaemonStatus.CRASHED,
                restart_count=self._restart_count,
            )

        logger.warning(
            f"Daemon crashed (attempt {self._restart_count + 1}/{self._max_restart_attempts}), "
            f"attempting restart"
        )

        # Clean up and restart
        self._remove_pid()
        self._restart_count += 1

        try:
            return self.start()
        except RuntimeError as e:
            logger.error(f"Failed to restart daemon: {e}")
            return DaemonInfo(
                status=DaemonStatus.CRASHED,
                restart_count=self._restart_count,
            )

    def is_running(self) -> bool:
        """Check if the daemon is currently running.

        Returns:
            True if daemon is running, False otherwise
        """
        return self.status().status == DaemonStatus.RUNNING

    def reset_restart_count(self) -> None:
        """Reset the restart count.

        Call this after the daemon has been running stably for a while
        to allow new restart attempts if it crashes later.
        """
        self._restart_count = 0
        logger.debug("Reset daemon restart count")
