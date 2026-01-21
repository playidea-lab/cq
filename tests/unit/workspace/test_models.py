"""Unit tests for workspace models."""

from datetime import datetime

from c4.workspace.models import ExecResult, Workspace, WorkspaceStats, WorkspaceStatus


class TestWorkspaceStatus:
    """Tests for WorkspaceStatus enum."""

    def test_status_values(self):
        """Test all status values exist."""
        assert WorkspaceStatus.CREATING == "creating"
        assert WorkspaceStatus.READY == "ready"
        assert WorkspaceStatus.RUNNING == "running"
        assert WorkspaceStatus.STOPPED == "stopped"
        assert WorkspaceStatus.ERROR == "error"

    def test_status_is_string_enum(self):
        """Test status can be used as string."""
        status = WorkspaceStatus.READY
        assert status == "ready"
        assert str(status) == "WorkspaceStatus.READY"
        assert status.value == "ready"


class TestWorkspace:
    """Tests for Workspace dataclass."""

    def test_workspace_creation_with_required_fields(self):
        """Test workspace creation with only required fields."""
        ws = Workspace(
            id="ws-123",
            user_id="user-1",
            git_url="https://github.com/user/repo",
        )

        assert ws.id == "ws-123"
        assert ws.user_id == "user-1"
        assert ws.git_url == "https://github.com/user/repo"
        assert ws.branch == "main"
        assert ws.status == WorkspaceStatus.CREATING
        assert ws.container_id is None
        assert ws.error_message is None
        assert ws.metadata == {}

    def test_workspace_creation_with_all_fields(self):
        """Test workspace creation with all fields."""
        created = datetime(2025, 1, 1, 12, 0, 0)
        ws = Workspace(
            id="ws-456",
            user_id="user-2",
            git_url="https://github.com/user/repo2",
            branch="develop",
            status=WorkspaceStatus.READY,
            created_at=created,
            container_id="container-abc",
            error_message=None,
            metadata={"env": "test"},
        )

        assert ws.id == "ws-456"
        assert ws.branch == "develop"
        assert ws.status == WorkspaceStatus.READY
        assert ws.created_at == created
        assert ws.container_id == "container-abc"
        assert ws.metadata == {"env": "test"}

    def test_workspace_is_active(self):
        """Test is_active method."""
        ws = Workspace(id="ws-1", user_id="user", git_url="https://github.com/test")

        # CREATING is not active
        ws.status = WorkspaceStatus.CREATING
        assert not ws.is_active()

        # READY is active
        ws.status = WorkspaceStatus.READY
        assert ws.is_active()

        # RUNNING is active
        ws.status = WorkspaceStatus.RUNNING
        assert ws.is_active()

        # STOPPED is not active
        ws.status = WorkspaceStatus.STOPPED
        assert not ws.is_active()

        # ERROR is not active
        ws.status = WorkspaceStatus.ERROR
        assert not ws.is_active()

    def test_workspace_is_usable(self):
        """Test is_usable method."""
        ws = Workspace(id="ws-1", user_id="user", git_url="https://github.com/test")

        # Only READY is usable
        ws.status = WorkspaceStatus.CREATING
        assert not ws.is_usable()

        ws.status = WorkspaceStatus.READY
        assert ws.is_usable()

        ws.status = WorkspaceStatus.RUNNING
        assert not ws.is_usable()

        ws.status = WorkspaceStatus.STOPPED
        assert not ws.is_usable()

        ws.status = WorkspaceStatus.ERROR
        assert not ws.is_usable()


class TestExecResult:
    """Tests for ExecResult dataclass."""

    def test_exec_result_success(self):
        """Test successful execution result."""
        result = ExecResult(
            exit_code=0,
            stdout="Hello, World!",
            stderr="",
            timed_out=False,
            duration_seconds=0.5,
        )

        assert result.exit_code == 0
        assert result.stdout == "Hello, World!"
        assert result.stderr == ""
        assert not result.timed_out
        assert result.duration_seconds == 0.5
        assert result.success

    def test_exec_result_failure(self):
        """Test failed execution result."""
        result = ExecResult(
            exit_code=1,
            stdout="",
            stderr="Error: file not found",
            timed_out=False,
            duration_seconds=0.1,
        )

        assert result.exit_code == 1
        assert result.stderr == "Error: file not found"
        assert not result.success

    def test_exec_result_timeout(self):
        """Test timed out execution result."""
        result = ExecResult(
            exit_code=-1,
            stdout="",
            stderr="Command timed out",
            timed_out=True,
            duration_seconds=60.0,
        )

        assert result.timed_out
        assert not result.success

    def test_exec_result_default_values(self):
        """Test default values."""
        result = ExecResult(exit_code=0, stdout="", stderr="")

        assert not result.timed_out
        assert result.duration_seconds == 0.0


class TestWorkspaceStats:
    """Tests for WorkspaceStats dataclass."""

    def test_workspace_stats_creation(self):
        """Test stats creation."""
        stats = WorkspaceStats(
            cpu_percent=25.5,
            memory_mb=128.0,
            disk_mb=512.0,
        )

        assert stats.cpu_percent == 25.5
        assert stats.memory_mb == 128.0
        assert stats.disk_mb == 512.0

    def test_workspace_stats_zero_values(self):
        """Test stats with zero values."""
        stats = WorkspaceStats(
            cpu_percent=0.0,
            memory_mb=0.0,
            disk_mb=0.0,
        )

        assert stats.cpu_percent == 0.0
        assert stats.memory_mb == 0.0
        assert stats.disk_mb == 0.0
