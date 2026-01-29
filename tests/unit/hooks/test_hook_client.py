"""Tests for C4 Hook Client - Unix Domain Socket communication."""

import json
import os
import socket
import tempfile
import threading
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.hooks import (
    DEFAULT_SOCKET_PATH,
    DEFAULT_SOCKET_TIMEOUT,
    HookClient,
    ValidationResult,
    get_pre_commit_status,
    get_pre_commit_template,
    install_pre_commit_hook,
    uninstall_pre_commit_hook,
)


# Use short temp dir for Unix socket tests (macOS has 104 char path limit)
@pytest.fixture
def short_tmp_path():
    """Create a short temporary path for Unix socket tests."""
    with tempfile.TemporaryDirectory(dir="/tmp", prefix="c4t_") as tmpdir:
        yield Path(tmpdir)


class TestValidationResult:
    """Tests for ValidationResult dataclass."""

    def test_passed_property(self):
        """Should correctly report pass status."""
        result_pass = ValidationResult(status="pass")
        result_fail = ValidationResult(status="fail")

        assert result_pass.passed is True
        assert result_fail.passed is False

    def test_to_dict(self):
        """Should convert to dictionary correctly."""
        result = ValidationResult(
            status="pass",
            message="All good",
            duration_ms=100,
            source="daemon",
        )

        d = result.to_dict()
        assert d["status"] == "pass"
        assert d["message"] == "All good"
        assert d["duration_ms"] == 100
        assert d["source"] == "daemon"

    def test_default_values(self):
        """Should have correct default values."""
        result = ValidationResult(status="pass")

        assert result.message is None
        assert result.duration_ms is None
        assert result.source == "unknown"


class TestHookClientBasic:
    """Tests for HookClient basic functionality."""

    def test_default_socket_path(self, tmp_path):
        """Should use default socket path."""
        client = HookClient(project_root=tmp_path)
        assert client.socket_path == tmp_path / DEFAULT_SOCKET_PATH

    def test_custom_socket_path(self, tmp_path):
        """Should allow custom socket path."""
        custom_path = tmp_path / "custom.sock"
        client = HookClient(socket_path=custom_path, project_root=tmp_path)
        assert client.socket_path == custom_path

    def test_default_timeout(self, tmp_path):
        """Should use default timeout."""
        client = HookClient(project_root=tmp_path)
        assert client.timeout == DEFAULT_SOCKET_TIMEOUT

    def test_custom_timeout(self, tmp_path):
        """Should allow custom timeout."""
        client = HookClient(timeout=10.0, project_root=tmp_path)
        assert client.timeout == 10.0


class TestHookClientDaemonDetection:
    """Tests for daemon detection functionality."""

    def test_is_daemon_running_no_socket(self, tmp_path):
        """Should return False when socket doesn't exist."""
        client = HookClient(project_root=tmp_path)
        assert client.is_daemon_running() is False

    def test_is_daemon_running_file_not_socket(self, tmp_path):
        """Should return False when path exists but is not a socket."""
        socket_path = tmp_path / ".c4" / "c4.sock"
        socket_path.parent.mkdir(parents=True)
        socket_path.write_text("not a socket")

        client = HookClient(project_root=tmp_path)
        assert client.is_daemon_running() is False

    def test_is_daemon_running_socket_exists(self, short_tmp_path):
        """Should return True when socket exists and is responsive."""
        socket_path = short_tmp_path / "c4.sock"

        # Create a real socket server
        server_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server_socket.bind(str(socket_path))
        server_socket.listen(1)

        try:
            client = HookClient(socket_path=socket_path, project_root=short_tmp_path)
            assert client.is_daemon_running() is True
        finally:
            server_socket.close()

    def test_is_daemon_running_socket_connection_refused(self, short_tmp_path):
        """Should return False when socket exists but no server."""
        socket_path = short_tmp_path / "c4.sock"

        # Create a socket file but no listener
        server_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server_socket.bind(str(socket_path))
        server_socket.close()  # Close immediately, no listen

        client = HookClient(socket_path=socket_path, project_root=short_tmp_path)
        # Socket file exists but no server listening
        assert client.is_daemon_running() is False


class TestHookClientFallbackValidation:
    """Tests for fallback validation functionality."""

    def test_run_fallback_validation_success(self, tmp_path):
        """Should return pass on successful validation."""
        # Create a mock project with no lint issues
        (tmp_path / ".c4").mkdir()
        (tmp_path / "test.py").write_text("# Valid Python file\n")

        # Mock the subprocess to simulate successful lint
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")

            client = HookClient(project_root=tmp_path)
            result = client.run_fallback_validation("lint")

            assert result.passed is True
            assert result.source == "fallback"

    def test_run_fallback_validation_failure(self, tmp_path):
        """Should return fail on validation failure."""
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(
                returncode=1,
                stdout="",
                stderr="Lint error: undefined variable",
            )

            client = HookClient(project_root=tmp_path)
            result = client.run_fallback_validation("lint")

            assert result.passed is False
            assert result.source == "fallback"
            assert "error" in result.message.lower() or "Lint" in result.message

    def test_run_fallback_validation_timeout(self, tmp_path):
        """Should handle timeout gracefully."""
        import subprocess

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = subprocess.TimeoutExpired(cmd="uv", timeout=60)

            client = HookClient(project_root=tmp_path)
            result = client.run_fallback_validation("lint")

            assert result.passed is False
            assert "timed out" in result.message.lower()

    def test_run_fallback_validation_uses_correct_command(self, tmp_path):
        """Should use correct command for validation type."""
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")

            client = HookClient(project_root=tmp_path)
            client.run_fallback_validation("lint")

            # Check that the command contains ruff
            args, kwargs = mock_run.call_args
            assert "ruff" in kwargs.get("command", args[0]) or "ruff" in str(args)


class TestHookClientDaemonValidation:
    """Tests for daemon validation via Unix Domain Socket."""

    def test_request_validation_no_socket(self, tmp_path):
        """Should raise ConnectionError when socket doesn't exist."""
        client = HookClient(project_root=tmp_path)

        with pytest.raises(ConnectionError):
            client.request_validation("lint")

    def test_request_validation_success(self, short_tmp_path):
        """Should return pass on successful daemon validation."""
        socket_path = short_tmp_path / "c4.sock"

        # Create a mock server
        server_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server_socket.bind(str(socket_path))
        server_socket.listen(1)
        server_socket.settimeout(5.0)

        def handle_client():
            try:
                conn, _ = server_socket.accept()
                conn.settimeout(2.0)
                # Read request
                conn.recv(4096)
                # Send success response and close
                response = json.dumps({"status": "pass", "duration_ms": 50})
                conn.sendall(response.encode("utf-8"))
                conn.close()
            except Exception:
                pass

        server_thread = threading.Thread(target=handle_client, daemon=True)
        server_thread.start()

        try:
            client = HookClient(socket_path=socket_path, project_root=short_tmp_path, timeout=2.0)
            result = client.request_validation("lint")

            assert result.passed is True
            assert result.source == "daemon"
            assert result.duration_ms == 50
        finally:
            server_thread.join(timeout=2)
            server_socket.close()

    def test_request_validation_failure(self, short_tmp_path):
        """Should return fail on daemon validation failure."""
        socket_path = short_tmp_path / "c4.sock"

        server_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server_socket.bind(str(socket_path))
        server_socket.listen(1)
        server_socket.settimeout(5.0)

        def handle_client():
            try:
                conn, _ = server_socket.accept()
                conn.settimeout(2.0)
                conn.recv(4096)
                response = json.dumps({
                    "status": "fail",
                    "message": "Lint errors found",
                })
                conn.sendall(response.encode("utf-8"))
                conn.close()
            except Exception:
                pass

        server_thread = threading.Thread(target=handle_client, daemon=True)
        server_thread.start()

        try:
            client = HookClient(socket_path=socket_path, project_root=short_tmp_path, timeout=2.0)
            result = client.request_validation("lint")

            assert result.passed is False
            assert result.source == "daemon"
            assert result.message == "Lint errors found"
        finally:
            server_thread.join(timeout=2)
            server_socket.close()


class TestHookClientValidate:
    """Tests for the main validate() method."""

    def test_validate_uses_daemon_when_available(self, short_tmp_path):
        """Should prefer daemon when available."""
        socket_path = short_tmp_path / "c4.sock"

        server_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server_socket.bind(str(socket_path))
        server_socket.listen(5)  # Allow multiple connections
        server_socket.settimeout(5.0)

        stop_event = threading.Event()

        def handle_clients():
            """Handle multiple client connections."""
            while not stop_event.is_set():
                try:
                    conn, _ = server_socket.accept()
                    conn.settimeout(2.0)
                    try:
                        data = conn.recv(4096)
                        if data:
                            # Only send response for validation requests (not just connect checks)
                            response = json.dumps({"status": "pass"})
                            conn.sendall(response.encode("utf-8"))
                    except Exception:
                        pass
                    finally:
                        conn.close()
                except socket.timeout:
                    continue
                except OSError:
                    break

        server_thread = threading.Thread(target=handle_clients, daemon=True)
        server_thread.start()

        try:
            client = HookClient(socket_path=socket_path, project_root=short_tmp_path, timeout=2.0)
            result = client.validate("lint")

            assert result.source == "daemon"
        finally:
            stop_event.set()
            server_socket.close()
            server_thread.join(timeout=2)

    def test_validate_falls_back_when_daemon_unavailable(self, tmp_path):
        """Should fall back to direct execution when daemon unavailable."""
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")

            client = HookClient(project_root=tmp_path)
            result = client.validate("lint")

            assert result.source == "fallback"
            mock_run.assert_called_once()


class TestPreCommitHookInstallation:
    """Tests for pre-commit hook installation functions."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a mock git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()
        return tmp_path

    def test_install_pre_commit_hook(self, git_repo):
        """Should install pre-commit hook."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            success, message = install_pre_commit_hook()

            assert success is True
            assert "Installed" in message

            hook_path = git_repo / ".git" / "hooks" / "pre-commit"
            assert hook_path.exists()
            assert os.access(hook_path, os.X_OK)

    def test_install_pre_commit_hook_idempotent(self, git_repo):
        """Should report success for already installed hooks."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            install_pre_commit_hook()
            success, message = install_pre_commit_hook()

            assert success is True
            assert "Already installed" in message

    def test_install_pre_commit_hook_force(self, git_repo):
        """Should overwrite existing hooks with force."""
        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom'")

        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            success, message = install_pre_commit_hook(force=True)

            assert success is True
            content = hook_path.read_text()
            assert "C4" in content

    def test_uninstall_pre_commit_hook(self, git_repo):
        """Should uninstall pre-commit hook."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            install_pre_commit_hook()
            success, message = uninstall_pre_commit_hook()

            assert success is True
            assert "Uninstalled" in message
            assert not (git_repo / ".git" / "hooks" / "pre-commit").exists()

    def test_get_pre_commit_status_installed(self, git_repo):
        """Should report correct status for installed hook."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            install_pre_commit_hook()
            status = get_pre_commit_status()

            assert status["installed"] is True
            assert status["is_c4"] is True
            assert status["executable"] is True

    def test_get_pre_commit_status_not_installed(self, git_repo):
        """Should report correct status for missing hook."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            status = get_pre_commit_status()

            assert status["installed"] is False
            assert status["is_c4"] is False


class TestPreCommitTemplate:
    """Tests for pre-commit template content."""

    def test_template_has_socket_support(self):
        """Template should include socket communication."""
        template = get_pre_commit_template()
        assert "C4_SOCKET" in template or "sock" in template.lower()

    def test_template_has_fallback(self):
        """Template should have fallback to ruff."""
        template = get_pre_commit_template()
        assert "ruff" in template.lower()

    def test_template_is_bash_script(self):
        """Template should be a bash script."""
        template = get_pre_commit_template()
        assert template.startswith("#!/bin/bash")

    def test_template_exits_correctly(self):
        """Template should have correct exit codes."""
        template = get_pre_commit_template()
        assert "exit 0" in template
        assert "exit 1" in template

    def test_template_skips_non_c4_projects(self):
        """Template should skip non-C4 projects."""
        template = get_pre_commit_template()
        assert ".c4/" in template
