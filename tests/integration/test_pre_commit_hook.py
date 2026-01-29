"""Integration tests for C4 pre-commit hook with daemon communication."""

import json
import os
import socket
import subprocess
import threading
import time
from unittest.mock import patch

import pytest

from c4.hooks import HookClient, install_pre_commit_hook


class TestPreCommitHookIntegration:
    """Integration tests for pre-commit hook behavior."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a real git repository for testing."""
        # Initialize git repo
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=tmp_path,
            capture_output=True,
        )

        # Create .c4 directory to make it a C4 project
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        (c4_dir / "config.yaml").write_text("project_id: test\n")

        return tmp_path

    @pytest.fixture
    def mock_daemon_server(self, tmp_path):
        """Create a mock daemon server that responds to validation requests."""
        socket_path = tmp_path / ".c4" / "c4.sock"
        socket_path.parent.mkdir(parents=True, exist_ok=True)

        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        server.bind(str(socket_path))
        server.listen(5)
        server.settimeout(5.0)

        stop_event = threading.Event()
        responses = []

        def serve():
            while not stop_event.is_set():
                try:
                    conn, _ = server.accept()
                    conn.settimeout(2.0)
                    try:
                        data = conn.recv(4096)
                        if data:
                            # Parse request (not used currently, but validates format)
                            _ = json.loads(data.decode("utf-8"))
                            # Default response is pass
                            response = {"status": "pass", "duration_ms": 10}
                            if responses:
                                response = responses.pop(0)
                            conn.sendall(json.dumps(response).encode("utf-8"))
                    except socket.timeout:
                        pass
                    finally:
                        conn.close()
                except socket.timeout:
                    continue
                except OSError:
                    break

        server_thread = threading.Thread(target=serve)
        server_thread.start()

        class MockServer:
            def __init__(self):
                self.socket_path = socket_path
                self.responses = responses

            def set_next_response(self, response):
                responses.append(response)

            def stop(self):
                stop_event.set()
                server.close()
                server_thread.join(timeout=2)

        mock = MockServer()
        yield mock
        mock.stop()

    def test_hook_client_validates_via_daemon(self, git_repo, mock_daemon_server):
        """Hook client should validate via daemon when available."""
        client = HookClient(project_root=git_repo)

        # Verify daemon is detected
        assert client.is_daemon_running() is True

        # Request validation
        result = client.validate("lint")

        assert result.passed is True
        assert result.source == "daemon"

    def test_hook_client_handles_daemon_failure(self, git_repo, mock_daemon_server):
        """Hook client should report failure from daemon."""
        mock_daemon_server.set_next_response({
            "status": "fail",
            "message": "Lint errors found in 3 files",
        })

        client = HookClient(project_root=git_repo)
        result = client.validate("lint")

        assert result.passed is False
        assert "Lint errors" in result.message

    def test_hook_client_falls_back_without_daemon(self, git_repo):
        """Hook client should fall back to direct execution without daemon."""
        # Create a valid Python file so lint passes
        (git_repo / "test.py").write_text("# Valid Python\n")

        client = HookClient(project_root=git_repo)

        # No daemon running
        assert client.is_daemon_running() is False

        # Should use fallback
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = type(
                "Result", (), {"returncode": 0, "stdout": "", "stderr": ""}
            )()

            result = client.validate("lint")

            assert result.source == "fallback"
            mock_run.assert_called_once()

    def test_pre_commit_hook_installation(self, git_repo):
        """Pre-commit hook should be installable in git repo."""
        with patch("c4.hooks.Path.cwd", return_value=git_repo):
            success, message = install_pre_commit_hook()

        assert success is True

        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        assert hook_path.exists()
        assert os.access(hook_path, os.X_OK)

        # Verify content
        content = hook_path.read_text()
        assert "C4" in content
        assert "ruff" in content.lower()

    def test_pre_commit_hook_skips_non_c4_project(self, tmp_path):
        """Pre-commit hook should skip non-C4 projects."""
        # Create git repo without .c4 directory
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=tmp_path,
            capture_output=True,
        )

        # Install hook
        with patch("c4.hooks.Path.cwd", return_value=tmp_path):
            install_pre_commit_hook()

        # Create and stage a file
        test_file = tmp_path / "test.txt"
        test_file.write_text("test content")
        subprocess.run(["git", "add", "test.txt"], cwd=tmp_path)

        # Commit should succeed (hook skips non-C4 projects)
        result = subprocess.run(
            ["git", "commit", "-m", "test commit"],
            cwd=tmp_path,
            capture_output=True,
            text=True,
        )

        # The hook should exit 0 for non-C4 projects
        assert result.returncode == 0


class TestHookClientEdgeCases:
    """Test edge cases for HookClient."""

    def test_stale_socket_file(self, tmp_path):
        """Should handle stale socket files gracefully."""
        socket_path = tmp_path / ".c4" / "c4.sock"
        socket_path.parent.mkdir(parents=True)

        # Create socket, bind, then close (simulating crashed daemon)
        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(str(socket_path))
        server.close()

        client = HookClient(project_root=tmp_path)

        # Should return False for stale socket
        assert client.is_daemon_running() is False

    def test_socket_timeout(self, tmp_path):
        """Should handle slow daemon responses."""
        socket_path = tmp_path / ".c4" / "c4.sock"
        socket_path.parent.mkdir(parents=True)

        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(str(socket_path))
        server.listen(1)

        def slow_handler():
            conn, _ = server.accept()
            time.sleep(3)  # Slow response
            conn.close()

        thread = threading.Thread(target=slow_handler)
        thread.start()

        try:
            client = HookClient(project_root=tmp_path, timeout=0.5)

            # Should fall back due to timeout
            with patch("subprocess.run") as mock_run:
                mock_run.return_value = type(
                    "Result", (), {"returncode": 0, "stdout": "", "stderr": ""}
                )()

                result = client.validate("lint")
                assert result.source == "fallback"
        finally:
            server.close()
            thread.join(timeout=1)

    def test_invalid_json_response(self, tmp_path):
        """Should handle invalid JSON from daemon."""
        socket_path = tmp_path / ".c4" / "c4.sock"
        socket_path.parent.mkdir(parents=True)

        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(str(socket_path))
        server.listen(1)

        def bad_handler():
            conn, _ = server.accept()
            conn.recv(4096)
            conn.sendall(b"not json")
            conn.close()

        thread = threading.Thread(target=bad_handler)
        thread.start()

        try:
            client = HookClient(project_root=tmp_path, timeout=2.0)

            # Should fall back due to invalid JSON
            with patch("subprocess.run") as mock_run:
                mock_run.return_value = type(
                    "Result", (), {"returncode": 0, "stdout": "", "stderr": ""}
                )()

                result = client.validate("lint")
                assert result.source == "fallback"
        finally:
            server.close()
            thread.join(timeout=1)
