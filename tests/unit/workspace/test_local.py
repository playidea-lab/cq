"""Unit tests for LocalWorkspaceManager."""

import tempfile
from pathlib import Path
from unittest.mock import AsyncMock, patch

import pytest

from c4.workspace.local import LocalWorkspaceManager
from c4.workspace.manager import (
    WorkspaceCreationError,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from c4.workspace.models import WorkspaceStatus


@pytest.fixture
def temp_base_dir():
    """Create a temporary base directory for testing."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def manager(temp_base_dir):
    """Create a LocalWorkspaceManager with temp directory."""
    return LocalWorkspaceManager(base_dir=temp_base_dir)


class TestLocalWorkspaceManagerInit:
    """Tests for LocalWorkspaceManager initialization."""

    def test_init_with_default_path(self):
        """Test initialization with default path."""
        manager = LocalWorkspaceManager()
        assert manager.base_dir == Path("/tmp/c4-workspaces")
        assert manager.workspaces == {}

    def test_init_with_custom_path(self, temp_base_dir):
        """Test initialization with custom path."""
        manager = LocalWorkspaceManager(base_dir=temp_base_dir)
        assert manager.base_dir == temp_base_dir

    def test_init_with_string_path(self):
        """Test initialization with string path."""
        manager = LocalWorkspaceManager(base_dir="/custom/path")
        assert manager.base_dir == Path("/custom/path")


class TestLocalWorkspaceManagerCreate:
    """Tests for workspace creation."""

    @pytest.mark.asyncio
    async def test_create_workspace_success(self, manager, temp_base_dir):
        """Test successful workspace creation with mocked git."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            # Mock successful git clone
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create(
                user_id="user-1",
                git_url="https://github.com/test/repo",
                branch="main",
            )

            assert workspace.id.startswith("ws-")
            assert workspace.user_id == "user-1"
            assert workspace.git_url == "https://github.com/test/repo"
            assert workspace.branch == "main"
            assert workspace.status == WorkspaceStatus.READY
            assert workspace.id in manager.workspaces

    @pytest.mark.asyncio
    async def test_create_workspace_git_clone_fails(self, manager):
        """Test workspace creation when git clone fails."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            # Mock failed git clone
            mock_process = AsyncMock()
            mock_process.returncode = 128
            mock_process.communicate = AsyncMock(return_value=(b"", b"fatal: repository not found"))
            mock_subprocess.return_value = mock_process

            with pytest.raises(WorkspaceCreationError) as exc_info:
                await manager.create(
                    user_id="user-1",
                    git_url="https://github.com/test/nonexistent",
                )

            assert "Git clone failed" in str(exc_info.value)
            # Workspace should be in ERROR state
            assert len(manager.workspaces) == 1
            ws = list(manager.workspaces.values())[0]
            assert ws.status == WorkspaceStatus.ERROR

    @pytest.mark.asyncio
    async def test_create_workspace_custom_branch(self, manager):
        """Test workspace creation with custom branch."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create(
                user_id="user-1",
                git_url="https://github.com/test/repo",
                branch="develop",
            )

            assert workspace.branch == "develop"

            # Verify git clone command includes branch
            call_args = mock_subprocess.call_args
            assert "--branch develop" in call_args[0][0]

    @pytest.mark.asyncio
    async def test_create_generates_unique_ids(self, manager):
        """Test that each workspace gets a unique ID."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            ws1 = await manager.create("user-1", "https://github.com/test/repo1")
            ws2 = await manager.create("user-1", "https://github.com/test/repo2")

            assert ws1.id != ws2.id


class TestLocalWorkspaceManagerGet:
    """Tests for workspace retrieval."""

    @pytest.mark.asyncio
    async def test_get_existing_workspace(self, manager):
        """Test getting an existing workspace."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            created = await manager.create("user-1", "https://github.com/test/repo")
            retrieved = await manager.get(created.id)

            assert retrieved is not None
            assert retrieved.id == created.id
            assert retrieved.user_id == created.user_id

    @pytest.mark.asyncio
    async def test_get_nonexistent_workspace(self, manager):
        """Test getting a nonexistent workspace."""
        result = await manager.get("ws-nonexistent")
        assert result is None


class TestLocalWorkspaceManagerDestroy:
    """Tests for workspace destruction."""

    @pytest.mark.asyncio
    async def test_destroy_existing_workspace(self, manager, temp_base_dir):
        """Test destroying an existing workspace."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_id = workspace.id

            # Create the directory manually since git clone is mocked
            workspace_path = temp_base_dir / workspace_id
            workspace_path.mkdir(parents=True)

            result = await manager.destroy(workspace_id)

            assert result is True
            assert workspace_id not in manager.workspaces
            assert not workspace_path.exists()

    @pytest.mark.asyncio
    async def test_destroy_nonexistent_workspace(self, manager):
        """Test destroying a nonexistent workspace."""
        result = await manager.destroy("ws-nonexistent")
        assert result is False


class TestLocalWorkspaceManagerListByUser:
    """Tests for listing workspaces by user."""

    @pytest.mark.asyncio
    async def test_list_by_user_empty(self, manager):
        """Test listing when no workspaces exist."""
        result = await manager.list_by_user("user-1")
        assert result == []

    @pytest.mark.asyncio
    async def test_list_by_user_filters_correctly(self, manager):
        """Test that list_by_user filters by user ID."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            await manager.create("user-1", "https://github.com/test/repo1")
            await manager.create("user-1", "https://github.com/test/repo2")
            ws3 = await manager.create("user-2", "https://github.com/test/repo3")

            user1_workspaces = await manager.list_by_user("user-1")
            user2_workspaces = await manager.list_by_user("user-2")

            assert len(user1_workspaces) == 2
            assert len(user2_workspaces) == 1
            assert ws3.id == user2_workspaces[0].id


class TestLocalWorkspaceManagerExec:
    """Tests for command execution."""

    @pytest.mark.asyncio
    async def test_exec_success(self, manager, temp_base_dir):
        """Test successful command execution."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            # First call for git clone
            mock_git_process = AsyncMock()
            mock_git_process.returncode = 0
            mock_git_process.communicate = AsyncMock(return_value=(b"", b""))

            # Second call for exec
            mock_exec_process = AsyncMock()
            mock_exec_process.returncode = 0
            mock_exec_process.communicate = AsyncMock(return_value=(b"hello world", b""))

            mock_subprocess.side_effect = [mock_git_process, mock_exec_process]

            workspace = await manager.create("user-1", "https://github.com/test/repo")

            # Create workspace directory
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.exec(workspace.id, "echo hello world")

            assert result.exit_code == 0
            assert result.stdout == "hello world"
            assert result.stderr == ""
            assert not result.timed_out
            assert result.success

    @pytest.mark.asyncio
    async def test_exec_workspace_not_found(self, manager):
        """Test exec on nonexistent workspace."""
        with pytest.raises(WorkspaceNotFoundError):
            await manager.exec("ws-nonexistent", "echo hello")

    @pytest.mark.asyncio
    async def test_exec_workspace_not_ready(self, manager):
        """Test exec on workspace that isn't ready."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            # Mock git clone failure to leave workspace in ERROR state
            mock_process = AsyncMock()
            mock_process.returncode = 128
            mock_process.communicate = AsyncMock(return_value=(b"", b"fatal: error"))
            mock_subprocess.return_value = mock_process

            try:
                workspace = await manager.create("user-1", "https://github.com/test/repo")
            except WorkspaceCreationError:
                pass

            # Get the workspace that's in ERROR state
            workspace = list(manager.workspaces.values())[0]

            with pytest.raises(WorkspaceNotReadyError) as exc_info:
                await manager.exec(workspace.id, "echo hello")

            assert exc_info.value.status == "error"

    @pytest.mark.asyncio
    async def test_exec_command_failure(self, manager, temp_base_dir):
        """Test command that exits with non-zero status."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_git_process = AsyncMock()
            mock_git_process.returncode = 0
            mock_git_process.communicate = AsyncMock(return_value=(b"", b""))

            mock_exec_process = AsyncMock()
            mock_exec_process.returncode = 1
            mock_exec_process.communicate = AsyncMock(return_value=(b"", b"command not found"))

            mock_subprocess.side_effect = [mock_git_process, mock_exec_process]

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.exec(workspace.id, "nonexistent_command")

            assert result.exit_code == 1
            assert "command not found" in result.stderr
            assert not result.success


class TestLocalWorkspaceManagerFileOperations:
    """Tests for file read/write operations."""

    @pytest.mark.asyncio
    async def test_read_file_success(self, manager, temp_base_dir):
        """Test successful file read."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")

            # Create file in workspace
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)
            test_file = workspace_path / "test.txt"
            test_file.write_text("Hello, World!")

            content = await manager.read_file(workspace.id, "test.txt")
            assert content == "Hello, World!"

    @pytest.mark.asyncio
    async def test_read_file_not_found(self, manager, temp_base_dir):
        """Test reading nonexistent file."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            with pytest.raises(FileNotFoundError):
                await manager.read_file(workspace.id, "nonexistent.txt")

    @pytest.mark.asyncio
    async def test_read_file_workspace_not_found(self, manager):
        """Test reading file from nonexistent workspace."""
        with pytest.raises(WorkspaceNotFoundError):
            await manager.read_file("ws-nonexistent", "test.txt")

    @pytest.mark.asyncio
    async def test_read_file_path_traversal_blocked(self, manager, temp_base_dir):
        """Test that path traversal attacks are blocked."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            with pytest.raises(FileNotFoundError) as exc_info:
                await manager.read_file(workspace.id, "../../../etc/passwd")

            assert "escapes workspace" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_write_file_success(self, manager, temp_base_dir):
        """Test successful file write."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.write_file(workspace.id, "output.txt", "Test content")

            assert result is True
            written_file = workspace_path / "output.txt"
            assert written_file.exists()
            assert written_file.read_text() == "Test content"

    @pytest.mark.asyncio
    async def test_write_file_creates_directories(self, manager, temp_base_dir):
        """Test that write_file creates parent directories."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.write_file(workspace.id, "subdir/nested/file.txt", "Nested content")

            assert result is True
            nested_file = workspace_path / "subdir" / "nested" / "file.txt"
            assert nested_file.exists()
            assert nested_file.read_text() == "Nested content"

    @pytest.mark.asyncio
    async def test_write_file_workspace_not_found(self, manager):
        """Test writing to nonexistent workspace."""
        with pytest.raises(WorkspaceNotFoundError):
            await manager.write_file("ws-nonexistent", "test.txt", "content")

    @pytest.mark.asyncio
    async def test_write_file_path_traversal_blocked(self, manager, temp_base_dir):
        """Test that path traversal in write is blocked."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.write_file(workspace.id, "../../../tmp/evil.txt", "malicious")

            assert result is False


class TestLocalWorkspaceManagerHealthCheck:
    """Tests for health check functionality."""

    @pytest.mark.asyncio
    async def test_health_check_healthy(self, manager, temp_base_dir):
        """Test health check on healthy workspace."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            result = await manager.health_check(workspace.id)
            assert result is True

    @pytest.mark.asyncio
    async def test_health_check_nonexistent(self, manager):
        """Test health check on nonexistent workspace."""
        result = await manager.health_check("ws-nonexistent")
        assert result is False

    @pytest.mark.asyncio
    async def test_health_check_missing_directory(self, manager, temp_base_dir):
        """Test health check when directory is missing."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            # Don't create the directory

            result = await manager.health_check(workspace.id)
            assert result is False


class TestLocalWorkspaceManagerStats:
    """Tests for resource statistics."""

    @pytest.mark.asyncio
    async def test_get_stats_success(self, manager, temp_base_dir):
        """Test getting workspace stats."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_path = temp_base_dir / workspace.id
            workspace_path.mkdir(parents=True)

            # Create some files
            (workspace_path / "file1.txt").write_text("a" * 1000)
            (workspace_path / "file2.txt").write_text("b" * 2000)

            stats = await manager.get_stats(workspace.id)

            assert stats is not None
            assert stats.cpu_percent == 0.0  # Local doesn't track CPU
            assert stats.memory_mb == 0.0  # Local doesn't track memory
            assert stats.disk_mb > 0  # Should have some disk usage

    @pytest.mark.asyncio
    async def test_get_stats_nonexistent(self, manager):
        """Test getting stats for nonexistent workspace."""
        stats = await manager.get_stats("ws-nonexistent")
        assert stats is None


class TestLocalWorkspaceManagerCleanup:
    """Tests for cleanup functionality."""

    @pytest.mark.asyncio
    async def test_cleanup_all(self, manager, temp_base_dir):
        """Test cleaning up all workspaces."""
        with patch("asyncio.create_subprocess_shell") as mock_subprocess:
            mock_process = AsyncMock()
            mock_process.returncode = 0
            mock_process.communicate = AsyncMock(return_value=(b"", b""))
            mock_subprocess.return_value = mock_process

            # Create multiple workspaces
            ws1 = await manager.create("user-1", "https://github.com/test/repo1")
            ws2 = await manager.create("user-1", "https://github.com/test/repo2")

            # Create directories
            (temp_base_dir / ws1.id).mkdir(parents=True)
            (temp_base_dir / ws2.id).mkdir(parents=True)

            count = await manager.cleanup_all()

            assert count == 2
            assert len(manager.workspaces) == 0

    @pytest.mark.asyncio
    async def test_cleanup_all_empty(self, manager):
        """Test cleanup when no workspaces exist."""
        count = await manager.cleanup_all()
        assert count == 0
