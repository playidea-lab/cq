"""Unit tests for workspace manager abstract class and exceptions."""

import pytest

from c4.workspace.manager import (
    WorkspaceCreationError,
    WorkspaceError,
    WorkspaceManager,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from c4.workspace.models import ExecResult, Workspace, WorkspaceStats


class TestWorkspaceExceptions:
    """Tests for workspace exception classes."""

    def test_workspace_error_base(self):
        """Test base WorkspaceError."""
        error = WorkspaceError("Something went wrong")
        assert str(error) == "Something went wrong"
        assert isinstance(error, Exception)

    def test_workspace_not_found_error(self):
        """Test WorkspaceNotFoundError."""
        error = WorkspaceNotFoundError("ws-123")
        assert error.workspace_id == "ws-123"
        assert "ws-123" in str(error)
        assert "not found" in str(error).lower()
        assert isinstance(error, WorkspaceError)

    def test_workspace_not_ready_error(self):
        """Test WorkspaceNotReadyError."""
        error = WorkspaceNotReadyError("ws-456", "creating")
        assert error.workspace_id == "ws-456"
        assert error.status == "creating"
        assert "ws-456" in str(error)
        assert "creating" in str(error)
        assert isinstance(error, WorkspaceError)

    def test_workspace_creation_error(self):
        """Test WorkspaceCreationError."""
        error = WorkspaceCreationError("Git clone failed")
        assert error.reason == "Git clone failed"
        assert "Git clone failed" in str(error)
        assert isinstance(error, WorkspaceError)


class TestWorkspaceManagerInterface:
    """Tests for WorkspaceManager abstract interface."""

    def test_cannot_instantiate_abstract_class(self):
        """Test that WorkspaceManager cannot be instantiated directly."""
        with pytest.raises(TypeError):
            WorkspaceManager()

    def test_concrete_implementation_required_methods(self):
        """Test that concrete implementation must implement all methods."""

        # Minimal incomplete implementation
        class IncompleteManager(WorkspaceManager):
            async def create(self, user_id, git_url, branch="main"):
                pass

            # Missing other methods

        with pytest.raises(TypeError) as exc_info:
            IncompleteManager()

        # Should mention missing abstract methods
        error_msg = str(exc_info.value)
        assert "abstract" in error_msg.lower()

    def test_complete_implementation(self):
        """Test that complete implementation can be instantiated."""

        class CompleteManager(WorkspaceManager):
            async def create(self, user_id, git_url, branch="main"):
                return Workspace(id="ws-1", user_id=user_id, git_url=git_url, branch=branch)

            async def destroy(self, workspace_id):
                return True

            async def get(self, workspace_id):
                return None

            async def list_by_user(self, user_id):
                return []

            async def exec(self, workspace_id, command, timeout=60):
                return ExecResult(exit_code=0, stdout="", stderr="")

            async def read_file(self, workspace_id, path):
                return ""

            async def write_file(self, workspace_id, path, content):
                return True

            async def health_check(self, workspace_id):
                return True

            async def get_stats(self, workspace_id):
                return WorkspaceStats(cpu_percent=0, memory_mb=0, disk_mb=0)

        # Should not raise
        manager = CompleteManager()
        assert manager is not None
