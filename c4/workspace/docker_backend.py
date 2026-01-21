"""Docker Workspace Manager - Container-based workspace management for production."""

from __future__ import annotations

import asyncio
import shutil
import time
import uuid
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING

try:
    from docker.errors import APIError, ContainerError, ImageNotFound, NotFound
    from docker.models.containers import Container

    import docker

    DOCKER_AVAILABLE = True
except ImportError:
    DOCKER_AVAILABLE = False
    docker = None  # type: ignore[assignment]
    APIError = Exception  # type: ignore[assignment, misc]
    ContainerError = Exception  # type: ignore[assignment, misc]
    ImageNotFound = Exception  # type: ignore[assignment, misc]
    NotFound = Exception  # type: ignore[assignment, misc]
    Container = None  # type: ignore[assignment, misc]

from .limits import ResourceLimits
from .manager import (
    WorkspaceCreationError,
    WorkspaceManager,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from .models import ExecResult, Workspace, WorkspaceStats, WorkspaceStatus

if TYPE_CHECKING:
    from docker import DockerClient


class DockerNotAvailableError(Exception):
    """Raised when docker-py library is not installed."""

    def __init__(self) -> None:
        super().__init__("Docker library not available. Install with: pip install c4[docker]")


class DockerWorkspaceManager(WorkspaceManager):
    """Docker-based workspace manager for production use.

    This implementation uses Docker containers to create isolated
    workspace environments. Each workspace runs in its own container
    with resource limits enforced.

    Features:
        - Container isolation for each workspace
        - Resource limits (memory, CPU)
        - Automatic cleanup of stale workspaces
        - Volume mounting for persistent workspace data
        - Container labels for tracking

    Example:
        manager = DockerWorkspaceManager(
            base_image="c4-workspace:latest",
            data_dir="/data/c4-workspaces",
            mem_limit="2g",
            cpu_quota=100000,  # 1 CPU
        )

        # Create workspace
        ws = await manager.create("user-1", "https://github.com/user/repo")

        # Execute command
        result = await manager.exec(ws.id, "pytest tests/")

        # Clean up
        await manager.destroy(ws.id)

    Note:
        Requires docker-py library: pip install c4[docker]
    """

    def __init__(
        self,
        base_image: str = "c4-workspace:latest",
        data_dir: Path | str = "/data/c4-workspaces",
        network_mode: str = "bridge",
        mem_limit: str = "2g",
        cpu_quota: int = 100000,  # 1 CPU (100000 microseconds per 100000 period)
        cleanup_after_seconds: int = 3600,  # 1 hour
        docker_client: "DockerClient | None" = None,
        resource_limits: ResourceLimits | None = None,
    ) -> None:
        """Initialize Docker workspace manager.

        Args:
            base_image: Docker image to use for workspaces
            data_dir: Base directory for workspace data volumes
            network_mode: Docker network mode (bridge, host, none)
            mem_limit: Memory limit (e.g., "2g", "512m") - deprecated, use resource_limits
            cpu_quota: CPU quota in microseconds (100000 = 1 CPU) - deprecated, use resource_limits
            cleanup_after_seconds: Seconds after which inactive workspaces are cleaned up
            docker_client: Optional pre-configured Docker client (for testing)
            resource_limits: ResourceLimits object for CPU, memory, disk, and timeout settings.
                           If provided, overrides mem_limit and cpu_quota.
        """
        if docker_client is not None:
            self.client = docker_client
        elif DOCKER_AVAILABLE:
            self.client = docker.from_env()
        else:
            raise DockerNotAvailableError()

        self.base_image = base_image
        self.data_dir = Path(data_dir)
        self.network_mode = network_mode

        # Use ResourceLimits if provided, otherwise fall back to legacy params
        if resource_limits is not None:
            self.resource_limits = resource_limits
            docker_config = resource_limits.to_docker_config()
            self.mem_limit = docker_config["mem_limit"]
            self.cpu_quota = docker_config["nano_cpus"] // 10000  # Convert back to quota
            self.cleanup_after_seconds = resource_limits.idle_timeout_seconds
        else:
            self.resource_limits = None
            self.mem_limit = mem_limit
            self.cpu_quota = cpu_quota
            self.cleanup_after_seconds = cleanup_after_seconds

        # In-memory tracking (replace with DB in production)
        self.workspaces: dict[str, Workspace] = {}
        self._lock = asyncio.Lock()

    def _get_workspace_path(self, workspace_id: str) -> Path:
        """Get the filesystem path for a workspace's data volume."""
        return self.data_dir / workspace_id

    def _generate_id(self) -> str:
        """Generate a unique workspace ID."""
        return f"ws-{uuid.uuid4().hex[:12]}"

    def _get_container_name(self, workspace_id: str) -> str:
        """Get the Docker container name for a workspace."""
        return f"c4-{workspace_id}"

    def _get_ready_workspace(self, workspace_id: str) -> Workspace:
        """Get a workspace and verify it's ready for operations.

        Args:
            workspace_id: Workspace ID to retrieve

        Returns:
            Workspace object if ready

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
            WorkspaceNotReadyError: If workspace isn't ready
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            raise WorkspaceNotFoundError(workspace_id)

        if not workspace.is_usable():
            raise WorkspaceNotReadyError(workspace_id, workspace.status.value)

        return workspace

    async def create(
        self,
        user_id: str,
        git_url: str,
        branch: str = "main",
    ) -> Workspace:
        """Create a new workspace with git repo cloned.

        Creates a Docker container with the workspace directory mounted
        as a volume. The git repository is cloned inside the container.

        Args:
            user_id: User ID who owns the workspace
            git_url: Git repository URL to clone
            branch: Branch to checkout (default: main)

        Returns:
            Created Workspace object

        Raises:
            WorkspaceCreationError: If creation fails
        """
        workspace_id = self._generate_id()
        workspace_path = self._get_workspace_path(workspace_id)
        container_name = self._get_container_name(workspace_id)

        # Create workspace object
        workspace = Workspace(
            id=workspace_id,
            user_id=user_id,
            git_url=git_url,
            branch=branch,
            status=WorkspaceStatus.CREATING,
            created_at=datetime.now(UTC),
        )

        async with self._lock:
            self.workspaces[workspace_id] = workspace

        try:
            # Ensure data directory exists
            workspace_path.mkdir(parents=True, exist_ok=True)

            # Run container
            loop = asyncio.get_event_loop()
            container = await loop.run_in_executor(
                None,
                lambda: self.client.containers.run(
                    image=self.base_image,
                    name=container_name,
                    detach=True,
                    volumes={str(workspace_path): {"bind": "/workspace", "mode": "rw"}},
                    environment={
                        "GIT_URL": git_url,
                        "GIT_BRANCH": branch,
                        "WORKSPACE_ID": workspace_id,
                    },
                    mem_limit=self.mem_limit,
                    cpu_quota=self.cpu_quota,
                    nano_cpus=self.resource_limits.to_docker_config()["nano_cpus"] if self.resource_limits else None,
                    network_mode=self.network_mode,
                    labels={
                        "c4.workspace_id": workspace_id,
                        "c4.user_id": user_id,
                        "c4.git_url": git_url,
                        "c4.branch": branch,
                    },
                    working_dir="/workspace",
                    # Keep container running
                    command="tail -f /dev/null",
                ),
            )

            workspace.container_id = container.id

            # Clone git repository inside container
            clone_result = await self._exec_in_container(
                container,
                f"git clone --branch {branch} --depth 1 {git_url} .",
                timeout=120,
            )

            if clone_result.exit_code != 0:
                workspace.status = WorkspaceStatus.ERROR
                workspace.error_message = f"Git clone failed: {clone_result.stderr}"
                # Clean up container on failure
                await self._remove_container(container)
                raise WorkspaceCreationError(f"Git clone failed: {clone_result.stderr}")

            workspace.status = WorkspaceStatus.READY
            return workspace

        except (ImageNotFound, ContainerError, APIError) as e:
            workspace.status = WorkspaceStatus.ERROR
            workspace.error_message = str(e)
            # Clean up on failure
            if workspace_path.exists():
                shutil.rmtree(workspace_path, ignore_errors=True)
            raise WorkspaceCreationError(str(e))

        except WorkspaceCreationError:
            raise

        except Exception as e:
            workspace.status = WorkspaceStatus.ERROR
            workspace.error_message = str(e)
            raise WorkspaceCreationError(str(e))

    async def _exec_in_container(
        self,
        container: Container,
        command: str,
        timeout: int = 60,
    ) -> ExecResult:
        """Execute a command inside a container.

        Args:
            container: Docker container object
            command: Shell command to execute
            timeout: Timeout in seconds

        Returns:
            ExecResult with output and status
        """
        loop = asyncio.get_event_loop()
        start_time = time.time()

        try:
            # Execute command with timeout
            exit_code, output = await asyncio.wait_for(
                loop.run_in_executor(
                    None,
                    lambda: container.exec_run(
                        cmd=["sh", "-c", command],
                        workdir="/workspace",
                        demux=True,
                    ),
                ),
                timeout=timeout,
            )

            duration = time.time() - start_time

            stdout = output[0].decode() if output[0] else ""
            stderr = output[1].decode() if output[1] else ""

            return ExecResult(
                exit_code=exit_code,
                stdout=stdout,
                stderr=stderr,
                timed_out=False,
                duration_seconds=duration,
            )

        except asyncio.TimeoutError:
            duration = time.time() - start_time
            return ExecResult(
                exit_code=-1,
                stdout="",
                stderr="Command timed out",
                timed_out=True,
                duration_seconds=duration,
            )

    async def _remove_container(self, container: Container) -> None:
        """Remove a container, stopping it first if necessary."""
        loop = asyncio.get_event_loop()
        try:
            await loop.run_in_executor(
                None,
                lambda: container.stop(timeout=10),
            )
        except Exception:
            pass  # Container may already be stopped

        try:
            await loop.run_in_executor(
                None,
                lambda: container.remove(force=True),
            )
        except Exception:
            pass  # Container may already be removed

    async def destroy(self, workspace_id: str) -> bool:
        """Destroy a workspace and clean up resources.

        Stops and removes the Docker container, then deletes the
        workspace data directory.

        Args:
            workspace_id: ID of workspace to destroy

        Returns:
            True if destroyed successfully, False otherwise
        """
        async with self._lock:
            workspace = self.workspaces.get(workspace_id)
            if not workspace:
                return False

            # Stop and remove container
            if workspace.container_id:
                try:
                    loop = asyncio.get_event_loop()
                    container = await loop.run_in_executor(
                        None,
                        lambda: self.client.containers.get(workspace.container_id),
                    )
                    await self._remove_container(container)
                except NotFound:
                    pass  # Container already removed

            # Remove workspace directory
            workspace_path = self._get_workspace_path(workspace_id)
            if workspace_path.exists():
                shutil.rmtree(workspace_path, ignore_errors=True)

            # Remove from registry
            del self.workspaces[workspace_id]
            return True

    async def get(self, workspace_id: str) -> Workspace | None:
        """Get workspace by ID.

        Args:
            workspace_id: Workspace ID to retrieve

        Returns:
            Workspace if found, None otherwise
        """
        return self.workspaces.get(workspace_id)

    async def list_by_user(self, user_id: str) -> list[Workspace]:
        """List all workspaces for a user.

        Args:
            user_id: User ID to filter by

        Returns:
            List of workspaces owned by the user
        """
        return [ws for ws in self.workspaces.values() if ws.user_id == user_id]

    async def exec(
        self,
        workspace_id: str,
        command: str,
        timeout: int = 60,
    ) -> ExecResult:
        """Execute a command in the workspace.

        Runs the command inside the workspace's Docker container.

        Args:
            workspace_id: Workspace ID to execute in
            command: Shell command to execute
            timeout: Timeout in seconds (default: 60)

        Returns:
            ExecResult with output and status

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
            WorkspaceNotReadyError: If workspace isn't ready
        """
        workspace = self._get_ready_workspace(workspace_id)

        if not workspace.container_id:
            raise WorkspaceNotReadyError(workspace_id, "no container")

        # Mark as running
        old_status = workspace.status
        workspace.status = WorkspaceStatus.RUNNING

        try:
            loop = asyncio.get_event_loop()
            container = await loop.run_in_executor(
                None,
                lambda: self.client.containers.get(workspace.container_id),
            )

            result = await self._exec_in_container(container, command, timeout)

            # Update last activity time in metadata
            workspace.metadata["last_activity"] = datetime.now(UTC).isoformat()

            return result

        finally:
            # Restore status
            workspace.status = old_status

    async def read_file(self, workspace_id: str, path: str) -> str:
        """Read a file from the workspace.

        Uses cat command inside the container to read file contents.

        Args:
            workspace_id: Workspace ID
            path: Relative path to file within workspace

        Returns:
            File contents as string

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
            FileNotFoundError: If file doesn't exist
        """
        workspace = self._get_ready_workspace(workspace_id)

        if not workspace.container_id:
            raise WorkspaceNotReadyError(workspace_id, "no container")

        # Security: validate path doesn't escape workspace
        if ".." in path or path.startswith("/"):
            raise FileNotFoundError(f"Invalid path: {path}")

        loop = asyncio.get_event_loop()
        container = await loop.run_in_executor(
            None,
            lambda: self.client.containers.get(workspace.container_id),
        )

        result = await self._exec_in_container(
            container,
            f"cat '{path}'",
            timeout=30,
        )

        if result.exit_code != 0:
            if "No such file" in result.stderr or result.exit_code == 1:
                raise FileNotFoundError(f"File not found: {path}")
            raise IOError(f"Failed to read file: {result.stderr}")

        return result.stdout

    async def write_file(
        self,
        workspace_id: str,
        path: str,
        content: str,
    ) -> bool:
        """Write a file to the workspace.

        Uses shell commands inside the container to write file contents.

        Args:
            workspace_id: Workspace ID
            path: Relative path to file within workspace
            content: File content to write

        Returns:
            True if written successfully

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
        """
        workspace = self._get_ready_workspace(workspace_id)

        if not workspace.container_id:
            raise WorkspaceNotReadyError(workspace_id, "no container")

        # Security: validate path doesn't escape workspace
        if ".." in path or path.startswith("/"):
            return False

        loop = asyncio.get_event_loop()
        container = await loop.run_in_executor(
            None,
            lambda: self.client.containers.get(workspace.container_id),
        )

        # Create parent directories
        parent_dir = str(Path(path).parent)
        if parent_dir and parent_dir != ".":
            await self._exec_in_container(
                container,
                f"mkdir -p '{parent_dir}'",
                timeout=30,
            )

        # Write file using here-doc to handle special characters
        # Escape single quotes in content
        escaped_content = content.replace("'", "'\"'\"'")
        result = await self._exec_in_container(
            container,
            f"printf '%s' '{escaped_content}' > '{path}'",
            timeout=30,
        )

        return result.exit_code == 0

    async def health_check(self, workspace_id: str) -> bool:
        """Check if workspace is healthy.

        Verifies the container is running and responsive.

        Args:
            workspace_id: Workspace ID to check

        Returns:
            True if workspace is healthy, False otherwise
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            return False

        if not workspace.container_id:
            return False

        if not workspace.is_active():
            return False

        try:
            loop = asyncio.get_event_loop()
            container = await loop.run_in_executor(
                None,
                lambda: self.client.containers.get(workspace.container_id),
            )

            # Check container is running
            status = await loop.run_in_executor(
                None,
                lambda: container.status,
            )

            if status != "running":
                return False

            # Quick command to verify responsiveness
            result = await self._exec_in_container(container, "echo ok", timeout=5)
            return result.exit_code == 0

        except NotFound:
            return False
        except Exception:
            return False

    async def get_stats(self, workspace_id: str) -> WorkspaceStats | None:
        """Get resource usage statistics.

        Retrieves CPU, memory, and disk usage from the Docker container.

        Args:
            workspace_id: Workspace ID

        Returns:
            WorkspaceStats if available, None otherwise
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace or not workspace.container_id:
            return None

        try:
            loop = asyncio.get_event_loop()
            container = await loop.run_in_executor(
                None,
                lambda: self.client.containers.get(workspace.container_id),
            )

            stats = await loop.run_in_executor(
                None,
                lambda: container.stats(stream=False),
            )

            # Calculate CPU percentage
            cpu_delta = stats["cpu_stats"]["cpu_usage"]["total_usage"] - stats["precpu_stats"]["cpu_usage"]["total_usage"]
            system_delta = stats["cpu_stats"]["system_cpu_usage"] - stats["precpu_stats"]["system_cpu_usage"]
            cpu_percent = (cpu_delta / system_delta) * 100 if system_delta else 0

            # Calculate memory usage
            memory_usage = stats["memory_stats"].get("usage", 0)
            memory_mb = memory_usage / (1024 * 1024)

            # Calculate disk usage from workspace directory
            workspace_path = self._get_workspace_path(workspace_id)
            disk_mb = 0.0
            if workspace_path.exists():
                try:
                    total_size = sum(f.stat().st_size for f in workspace_path.rglob("*") if f.is_file())
                    disk_mb = total_size / (1024 * 1024)
                except Exception:
                    pass

            return WorkspaceStats(
                cpu_percent=cpu_percent,
                memory_mb=memory_mb,
                disk_mb=disk_mb,
            )

        except NotFound:
            return None
        except Exception:
            return None

    async def cleanup_stale_workspaces(self) -> int:
        """Clean up workspaces that have been inactive for too long.

        Destroys workspaces that haven't had activity for longer than
        cleanup_after_seconds.

        Returns:
            Number of workspaces cleaned up
        """
        now = datetime.now(UTC)
        cleaned = 0

        workspace_ids = list(self.workspaces.keys())
        for workspace_id in workspace_ids:
            workspace = self.workspaces.get(workspace_id)
            if not workspace:
                continue

            # Check last activity or creation time
            last_activity_str = workspace.metadata.get("last_activity")
            if last_activity_str:
                last_activity = datetime.fromisoformat(last_activity_str)
            else:
                last_activity = workspace.created_at

            # Ensure timezone aware comparison
            if last_activity.tzinfo is None:
                last_activity = last_activity.replace(tzinfo=UTC)

            age_seconds = (now - last_activity).total_seconds()
            if age_seconds > self.cleanup_after_seconds:
                if await self.destroy(workspace_id):
                    cleaned += 1

        return cleaned

    async def cleanup_all(self) -> int:
        """Clean up all workspaces.

        Returns:
            Number of workspaces destroyed
        """
        count = 0
        workspace_ids = list(self.workspaces.keys())

        for workspace_id in workspace_ids:
            if await self.destroy(workspace_id):
                count += 1

        return count

    def list_containers(self) -> list[dict]:
        """List all C4 workspace containers.

        Returns:
            List of container info dicts with id, name, status, labels
        """
        containers = self.client.containers.list(
            all=True,
            filters={"label": "c4.workspace_id"},
        )

        return [
            {
                "id": c.id,
                "name": c.name,
                "status": c.status,
                "labels": c.labels,
            }
            for c in containers
        ]
