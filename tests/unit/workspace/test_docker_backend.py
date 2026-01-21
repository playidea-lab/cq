"""Unit tests for DockerWorkspaceManager."""

from __future__ import annotations

import tempfile
from datetime import UTC, datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.workspace.docker_backend import (
    DockerNotAvailableError,
    DockerWorkspaceManager,
)
from c4.workspace.manager import (
    WorkspaceCreationError,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from c4.workspace.models import WorkspaceStatus


# Custom exception classes for mocking
class MockNotFound(Exception):
    """Mock NotFound exception."""

    pass


class MockImageNotFound(Exception):
    """Mock ImageNotFound exception."""

    pass


class MockContainerError(Exception):
    """Mock ContainerError exception."""

    pass


class MockAPIError(Exception):
    """Mock APIError exception."""

    pass


@pytest.fixture
def temp_data_dir():
    """Create a temporary data directory for testing."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def mock_docker_client():
    """Create a mock Docker client."""
    client = MagicMock()
    client.containers = MagicMock()
    return client


@pytest.fixture
def mock_container():
    """Create a mock Docker container."""
    container = MagicMock()
    container.id = "container-123abc"
    container.name = "c4-ws-test123"
    container.status = "running"
    container.labels = {
        "c4.workspace_id": "ws-test123",
        "c4.user_id": "user-1",
    }

    # Mock exec_run to return (exit_code, (stdout, stderr))
    container.exec_run.return_value = (0, (b"output", b""))

    # Mock stats
    container.stats.return_value = {
        "cpu_stats": {
            "cpu_usage": {"total_usage": 1000000},
            "system_cpu_usage": 10000000,
        },
        "precpu_stats": {
            "cpu_usage": {"total_usage": 900000},
            "system_cpu_usage": 9000000,
        },
        "memory_stats": {"usage": 104857600},  # 100 MB
    }

    return container


@pytest.fixture
def manager(temp_data_dir, mock_docker_client):
    """Create a DockerWorkspaceManager with mocked Docker client."""
    return DockerWorkspaceManager(
        base_image="test-image:latest",
        data_dir=temp_data_dir,
        mem_limit="1g",
        cpu_quota=50000,
        cleanup_after_seconds=3600,
        docker_client=mock_docker_client,
    )


class TestDockerWorkspaceManagerInit:
    """Tests for DockerWorkspaceManager initialization."""

    def test_init_with_defaults(self, mock_docker_client):
        """Test initialization with default parameters."""
        manager = DockerWorkspaceManager(docker_client=mock_docker_client)

        assert manager.base_image == "c4-workspace:latest"
        assert manager.data_dir == Path("/data/c4-workspaces")
        assert manager.network_mode == "bridge"
        assert manager.mem_limit == "2g"
        assert manager.cpu_quota == 100000
        assert manager.cleanup_after_seconds == 3600
        assert manager.workspaces == {}

    def test_init_with_custom_params(self, temp_data_dir, mock_docker_client):
        """Test initialization with custom parameters."""
        manager = DockerWorkspaceManager(
            base_image="custom-image:v1",
            data_dir=temp_data_dir,
            network_mode="host",
            mem_limit="4g",
            cpu_quota=200000,
            cleanup_after_seconds=7200,
            docker_client=mock_docker_client,
        )

        assert manager.base_image == "custom-image:v1"
        assert manager.data_dir == temp_data_dir
        assert manager.network_mode == "host"
        assert manager.mem_limit == "4g"
        assert manager.cpu_quota == 200000
        assert manager.cleanup_after_seconds == 7200

    def test_init_with_string_data_dir(self, mock_docker_client):
        """Test initialization with string data directory."""
        manager = DockerWorkspaceManager(
            data_dir="/custom/path",
            docker_client=mock_docker_client,
        )

        assert manager.data_dir == Path("/custom/path")


class TestDockerWorkspaceManagerCreate:
    """Tests for workspace creation."""

    @pytest.mark.asyncio
    async def test_create_workspace_success(
        self, manager, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test successful workspace creation."""
        mock_docker_client.containers.run.return_value = mock_container
        # Git clone success
        mock_container.exec_run.return_value = (0, (b"Cloning...", b""))

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
        assert workspace.container_id == mock_container.id
        assert workspace.id in manager.workspaces

        # Verify container was created with correct params
        mock_docker_client.containers.run.assert_called_once()
        call_kwargs = mock_docker_client.containers.run.call_args[1]
        assert call_kwargs["image"] == "test-image:latest"
        assert call_kwargs["mem_limit"] == "1g"
        assert call_kwargs["cpu_quota"] == 50000
        assert "c4.workspace_id" in call_kwargs["labels"]

    @pytest.mark.asyncio
    async def test_create_workspace_git_clone_fails(
        self, manager, mock_docker_client, mock_container
    ):
        """Test workspace creation when git clone fails."""
        mock_docker_client.containers.run.return_value = mock_container
        # Git clone failure
        mock_container.exec_run.return_value = (
            128,
            (b"", b"fatal: repository not found"),
        )

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
    async def test_create_workspace_image_not_found(
        self, manager, mock_docker_client
    ):
        """Test workspace creation when Docker image is not found."""
        mock_docker_client.containers.run.side_effect = MockImageNotFound(
            "Image not found"
        )

        # Patch the exception check in the implementation
        import c4.workspace.docker_backend as docker_backend

        original_image_not_found = docker_backend.ImageNotFound
        docker_backend.ImageNotFound = MockImageNotFound

        try:
            with pytest.raises(WorkspaceCreationError) as exc_info:
                await manager.create(
                    user_id="user-1",
                    git_url="https://github.com/test/repo",
                )

            assert "Image not found" in str(exc_info.value)
        finally:
            docker_backend.ImageNotFound = original_image_not_found

    @pytest.mark.asyncio
    async def test_create_workspace_custom_branch(
        self, manager, mock_docker_client, mock_container
    ):
        """Test workspace creation with custom branch."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"Cloning...", b""))

        workspace = await manager.create(
            user_id="user-1",
            git_url="https://github.com/test/repo",
            branch="develop",
        )

        assert workspace.branch == "develop"

        # Verify environment variable was set
        call_kwargs = mock_docker_client.containers.run.call_args[1]
        assert call_kwargs["environment"]["GIT_BRANCH"] == "develop"

    @pytest.mark.asyncio
    async def test_create_generates_unique_ids(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that each workspace gets a unique ID."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        ws1 = await manager.create("user-1", "https://github.com/test/repo1")
        ws2 = await manager.create("user-1", "https://github.com/test/repo2")

        assert ws1.id != ws2.id

    @pytest.mark.asyncio
    async def test_create_workspace_creates_data_directory(
        self, manager, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test that workspace data directory is created."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create(
            user_id="user-1",
            git_url="https://github.com/test/repo",
        )

        workspace_path = temp_data_dir / workspace.id
        assert workspace_path.exists()


class TestDockerWorkspaceManagerGet:
    """Tests for workspace retrieval."""

    @pytest.mark.asyncio
    async def test_get_existing_workspace(
        self, manager, mock_docker_client, mock_container
    ):
        """Test getting an existing workspace."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

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


class TestDockerWorkspaceManagerDestroy:
    """Tests for workspace destruction."""

    @pytest.mark.asyncio
    async def test_destroy_existing_workspace(
        self, manager, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test destroying an existing workspace."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        workspace_id = workspace.id

        result = await manager.destroy(workspace_id)

        assert result is True
        assert workspace_id not in manager.workspaces
        # Container should be stopped and removed
        mock_container.stop.assert_called_with(timeout=10)
        mock_container.remove.assert_called_with(force=True)

    @pytest.mark.asyncio
    async def test_destroy_nonexistent_workspace(self, manager):
        """Test destroying a nonexistent workspace."""
        result = await manager.destroy("ws-nonexistent")
        assert result is False

    @pytest.mark.asyncio
    async def test_destroy_handles_container_not_found(
        self, manager, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test destroy handles container already removed."""
        import c4.workspace.docker_backend as docker_backend

        original_not_found = docker_backend.NotFound
        docker_backend.NotFound = MockNotFound

        try:
            mock_docker_client.containers.run.return_value = mock_container
            mock_docker_client.containers.get.side_effect = MockNotFound("Not found")
            mock_container.exec_run.return_value = (0, (b"", b""))

            workspace = await manager.create("user-1", "https://github.com/test/repo")
            workspace_id = workspace.id

            # Should not raise even if container not found
            result = await manager.destroy(workspace_id)
            assert result is True
            assert workspace_id not in manager.workspaces
        finally:
            docker_backend.NotFound = original_not_found


class TestDockerWorkspaceManagerListByUser:
    """Tests for listing workspaces by user."""

    @pytest.mark.asyncio
    async def test_list_by_user_empty(self, manager):
        """Test listing when no workspaces exist."""
        result = await manager.list_by_user("user-1")
        assert result == []

    @pytest.mark.asyncio
    async def test_list_by_user_filters_correctly(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that list_by_user filters by user ID."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        await manager.create("user-1", "https://github.com/test/repo1")
        await manager.create("user-1", "https://github.com/test/repo2")
        ws3 = await manager.create("user-2", "https://github.com/test/repo3")

        user1_workspaces = await manager.list_by_user("user-1")
        user2_workspaces = await manager.list_by_user("user-2")

        assert len(user1_workspaces) == 2
        assert len(user2_workspaces) == 1
        assert ws3.id == user2_workspaces[0].id


class TestDockerWorkspaceManagerExec:
    """Tests for command execution."""

    @pytest.mark.asyncio
    async def test_exec_success(self, manager, mock_docker_client, mock_container):
        """Test successful command execution."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container

        # First call for git clone, second for exec
        mock_container.exec_run.side_effect = [
            (0, (b"Cloning...", b"")),  # git clone
            (0, (b"hello world", b"")),  # exec
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
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
    async def test_exec_workspace_not_ready(
        self, manager, mock_docker_client, mock_container
    ):
        """Test exec on workspace that isn't ready."""
        mock_docker_client.containers.run.return_value = mock_container
        # Git clone failure
        mock_container.exec_run.return_value = (128, (b"", b"fatal: error"))

        try:
            await manager.create("user-1", "https://github.com/test/repo")
        except WorkspaceCreationError:
            pass

        # Get the workspace in ERROR state
        workspace = list(manager.workspaces.values())[0]

        with pytest.raises(WorkspaceNotReadyError) as exc_info:
            await manager.exec(workspace.id, "echo hello")

        assert exc_info.value.status == "error"

    @pytest.mark.asyncio
    async def test_exec_command_failure(
        self, manager, mock_docker_client, mock_container
    ):
        """Test command that exits with non-zero status."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container

        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (1, (b"", b"command not found")),  # exec
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        result = await manager.exec(workspace.id, "nonexistent_command")

        assert result.exit_code == 1
        assert "command not found" in result.stderr
        assert not result.success

    @pytest.mark.asyncio
    async def test_exec_updates_last_activity(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that exec updates last_activity metadata."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (0, (b"ok", b"")),  # exec
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        # Ensure no last_activity initially
        assert "last_activity" not in workspace.metadata

        await manager.exec(workspace.id, "echo ok")

        # Should have last_activity now
        assert "last_activity" in workspace.metadata


class TestDockerWorkspaceManagerFileOperations:
    """Tests for file read/write operations."""

    @pytest.mark.asyncio
    async def test_read_file_success(
        self, manager, mock_docker_client, mock_container
    ):
        """Test successful file read."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (0, (b"Hello, World!", b"")),  # cat
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        content = await manager.read_file(workspace.id, "test.txt")

        assert content == "Hello, World!"

    @pytest.mark.asyncio
    async def test_read_file_not_found(
        self, manager, mock_docker_client, mock_container
    ):
        """Test reading nonexistent file."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (1, (b"", b"cat: nonexistent.txt: No such file or directory")),  # cat
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        with pytest.raises(FileNotFoundError):
            await manager.read_file(workspace.id, "nonexistent.txt")

    @pytest.mark.asyncio
    async def test_read_file_workspace_not_found(self, manager):
        """Test reading file from nonexistent workspace."""
        with pytest.raises(WorkspaceNotFoundError):
            await manager.read_file("ws-nonexistent", "test.txt")

    @pytest.mark.asyncio
    async def test_read_file_path_traversal_blocked(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that path traversal attacks are blocked."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        with pytest.raises(FileNotFoundError) as exc_info:
            await manager.read_file(workspace.id, "../../../etc/passwd")

        assert "Invalid path" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_read_file_absolute_path_blocked(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that absolute paths are blocked."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        with pytest.raises(FileNotFoundError) as exc_info:
            await manager.read_file(workspace.id, "/etc/passwd")

        assert "Invalid path" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_write_file_success(
        self, manager, mock_docker_client, mock_container
    ):
        """Test successful file write."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (0, (b"", b"")),  # printf
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        result = await manager.write_file(workspace.id, "output.txt", "Test content")

        assert result is True

    @pytest.mark.asyncio
    async def test_write_file_creates_directories(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that write_file creates parent directories."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (0, (b"", b"")),  # mkdir
            (0, (b"", b"")),  # printf
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        result = await manager.write_file(
            workspace.id, "subdir/nested/file.txt", "Nested content"
        )

        assert result is True
        # Verify mkdir was called
        exec_calls = mock_container.exec_run.call_args_list
        assert any("mkdir" in str(call) for call in exec_calls)

    @pytest.mark.asyncio
    async def test_write_file_workspace_not_found(self, manager):
        """Test writing to nonexistent workspace."""
        with pytest.raises(WorkspaceNotFoundError):
            await manager.write_file("ws-nonexistent", "test.txt", "content")

    @pytest.mark.asyncio
    async def test_write_file_path_traversal_blocked(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that path traversal in write is blocked."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        result = await manager.write_file(
            workspace.id, "../../../tmp/evil.txt", "malicious"
        )

        assert result is False


class TestDockerWorkspaceManagerHealthCheck:
    """Tests for health check functionality."""

    @pytest.mark.asyncio
    async def test_health_check_healthy(
        self, manager, mock_docker_client, mock_container
    ):
        """Test health check on healthy workspace."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.status = "running"
        mock_container.exec_run.side_effect = [
            (0, (b"", b"")),  # git clone
            (0, (b"ok", b"")),  # echo ok
        ]

        workspace = await manager.create("user-1", "https://github.com/test/repo")
        result = await manager.health_check(workspace.id)

        assert result is True

    @pytest.mark.asyncio
    async def test_health_check_nonexistent(self, manager):
        """Test health check on nonexistent workspace."""
        result = await manager.health_check("ws-nonexistent")
        assert result is False

    @pytest.mark.asyncio
    async def test_health_check_container_not_running(
        self, manager, mock_docker_client, mock_container
    ):
        """Test health check when container is not running."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        # Container stopped
        mock_container.status = "exited"
        mock_docker_client.containers.get.return_value = mock_container

        result = await manager.health_check(workspace.id)
        assert result is False

    @pytest.mark.asyncio
    async def test_health_check_container_not_found(
        self, manager, mock_docker_client, mock_container
    ):
        """Test health check when container is gone."""
        import c4.workspace.docker_backend as docker_backend

        original_not_found = docker_backend.NotFound
        docker_backend.NotFound = MockNotFound

        try:
            mock_docker_client.containers.run.return_value = mock_container
            mock_container.exec_run.return_value = (0, (b"", b""))

            workspace = await manager.create("user-1", "https://github.com/test/repo")

            mock_docker_client.containers.get.side_effect = MockNotFound("Not found")

            result = await manager.health_check(workspace.id)
            assert result is False
        finally:
            docker_backend.NotFound = original_not_found


class TestDockerWorkspaceManagerStats:
    """Tests for resource statistics."""

    @pytest.mark.asyncio
    async def test_get_stats_success(
        self, manager, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test getting workspace stats."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        # Create some files in workspace directory
        workspace_path = temp_data_dir / workspace.id
        workspace_path.mkdir(parents=True, exist_ok=True)
        (workspace_path / "file1.txt").write_text("a" * 1000)
        (workspace_path / "file2.txt").write_text("b" * 2000)

        stats = await manager.get_stats(workspace.id)

        assert stats is not None
        assert stats.cpu_percent >= 0
        assert stats.memory_mb == 100.0  # 100 MB from mock
        assert stats.disk_mb > 0

    @pytest.mark.asyncio
    async def test_get_stats_nonexistent(self, manager):
        """Test getting stats for nonexistent workspace."""
        stats = await manager.get_stats("ws-nonexistent")
        assert stats is None

    @pytest.mark.asyncio
    async def test_get_stats_container_not_found(
        self, manager, mock_docker_client, mock_container
    ):
        """Test getting stats when container is gone."""
        import c4.workspace.docker_backend as docker_backend

        original_not_found = docker_backend.NotFound
        docker_backend.NotFound = MockNotFound

        try:
            mock_docker_client.containers.run.return_value = mock_container
            mock_container.exec_run.return_value = (0, (b"", b""))

            workspace = await manager.create("user-1", "https://github.com/test/repo")

            mock_docker_client.containers.get.side_effect = MockNotFound("Not found")

            stats = await manager.get_stats(workspace.id)
            assert stats is None
        finally:
            docker_backend.NotFound = original_not_found


class TestDockerWorkspaceManagerCleanup:
    """Tests for cleanup functionality."""

    @pytest.mark.asyncio
    async def test_cleanup_stale_workspaces(
        self, manager, mock_docker_client, mock_container
    ):
        """Test cleaning up stale workspaces."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        # Create workspace
        workspace = await manager.create("user-1", "https://github.com/test/repo")

        # Manually set created_at to 2 hours ago
        workspace.created_at = datetime.now(UTC) - timedelta(hours=2)

        count = await manager.cleanup_stale_workspaces()

        assert count == 1
        assert workspace.id not in manager.workspaces

    @pytest.mark.asyncio
    async def test_cleanup_stale_respects_activity(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that cleanup respects last_activity."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create("user-1", "https://github.com/test/repo")

        # Set created_at to 2 hours ago but last_activity to now
        workspace.created_at = datetime.now(UTC) - timedelta(hours=2)
        workspace.metadata["last_activity"] = datetime.now(UTC).isoformat()

        count = await manager.cleanup_stale_workspaces()

        assert count == 0
        assert workspace.id in manager.workspaces

    @pytest.mark.asyncio
    async def test_cleanup_all(self, manager, mock_docker_client, mock_container):
        """Test cleaning up all workspaces."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_docker_client.containers.get.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        # Create multiple workspaces
        await manager.create("user-1", "https://github.com/test/repo1")
        await manager.create("user-1", "https://github.com/test/repo2")

        count = await manager.cleanup_all()

        assert count == 2
        assert len(manager.workspaces) == 0

    @pytest.mark.asyncio
    async def test_cleanup_all_empty(self, manager):
        """Test cleanup when no workspaces exist."""
        count = await manager.cleanup_all()
        assert count == 0


class TestDockerWorkspaceManagerListContainers:
    """Tests for list_containers functionality."""

    def test_list_containers(self, manager, mock_docker_client, mock_container):
        """Test listing C4 workspace containers."""
        mock_docker_client.containers.list.return_value = [mock_container]

        containers = manager.list_containers()

        assert len(containers) == 1
        assert containers[0]["id"] == mock_container.id
        assert containers[0]["name"] == mock_container.name
        assert containers[0]["status"] == "running"
        assert "c4.workspace_id" in containers[0]["labels"]

        # Verify filter was applied
        mock_docker_client.containers.list.assert_called_with(
            all=True, filters={"label": "c4.workspace_id"}
        )

    def test_list_containers_empty(self, manager, mock_docker_client):
        """Test listing when no containers exist."""
        mock_docker_client.containers.list.return_value = []

        containers = manager.list_containers()

        assert containers == []


class TestDockerNotAvailable:
    """Tests for handling missing docker library."""

    def test_docker_not_available_error(self):
        """Test DockerNotAvailableError message."""
        error = DockerNotAvailableError()
        assert "pip install c4[docker]" in str(error)


class TestResourceLimits:
    """Tests for resource limit enforcement."""

    @pytest.mark.asyncio
    async def test_memory_limit_passed_to_container(
        self, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test that memory limit is passed to container."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        manager = DockerWorkspaceManager(
            data_dir=temp_data_dir,
            mem_limit="512m",
            docker_client=mock_docker_client,
        )

        await manager.create("user-1", "https://github.com/test/repo")

        call_kwargs = mock_docker_client.containers.run.call_args[1]
        assert call_kwargs["mem_limit"] == "512m"

    @pytest.mark.asyncio
    async def test_cpu_quota_passed_to_container(
        self, mock_docker_client, mock_container, temp_data_dir
    ):
        """Test that CPU quota is passed to container."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        manager = DockerWorkspaceManager(
            data_dir=temp_data_dir,
            cpu_quota=50000,  # 0.5 CPU
            docker_client=mock_docker_client,
        )

        await manager.create("user-1", "https://github.com/test/repo")

        call_kwargs = mock_docker_client.containers.run.call_args[1]
        assert call_kwargs["cpu_quota"] == 50000


class TestContainerLabels:
    """Tests for container label management."""

    @pytest.mark.asyncio
    async def test_labels_include_workspace_info(
        self, manager, mock_docker_client, mock_container
    ):
        """Test that container labels include workspace info."""
        mock_docker_client.containers.run.return_value = mock_container
        mock_container.exec_run.return_value = (0, (b"", b""))

        workspace = await manager.create(
            user_id="test-user-123",
            git_url="https://github.com/org/project",
            branch="feature-branch",
        )

        call_kwargs = mock_docker_client.containers.run.call_args[1]
        labels = call_kwargs["labels"]

        assert labels["c4.workspace_id"] == workspace.id
        assert labels["c4.user_id"] == "test-user-123"
        assert labels["c4.git_url"] == "https://github.com/org/project"
        assert labels["c4.branch"] == "feature-branch"
