"""Local Workspace Manager - Filesystem-based workspace management for dev/testing."""

from __future__ import annotations

import asyncio
import shutil
import uuid
from datetime import UTC, datetime
from pathlib import Path

from .manager import (
    WorkspaceCreationError,
    WorkspaceManager,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from .models import ExecResult, Workspace, WorkspaceStats, WorkspaceStatus


class LocalWorkspaceManager(WorkspaceManager):
    """Local filesystem-based workspace manager for development and testing.

    This implementation uses the local filesystem to create isolated
    workspace directories. Git repositories are cloned using the local
    git binary. Commands are executed via subprocess.

    Note: This is intended for development and testing only.
    For production, use Docker or Kubernetes-based implementations.

    Example:
        manager = LocalWorkspaceManager(base_dir="/tmp/c4-workspaces")

        # Create workspace
        ws = await manager.create("user-1", "https://github.com/user/repo")

        # Execute command
        result = await manager.exec(ws.id, "ls -la")

        # Clean up
        await manager.destroy(ws.id)
    """

    def __init__(self, base_dir: Path | str = "/tmp/c4-workspaces"):
        """Initialize local workspace manager.

        Args:
            base_dir: Base directory for workspace storage
        """
        self.base_dir = Path(base_dir)
        self.workspaces: dict[str, Workspace] = {}
        self._lock = asyncio.Lock()

    def _get_workspace_path(self, workspace_id: str) -> Path:
        """Get the filesystem path for a workspace."""
        return self.base_dir / workspace_id

    def _generate_id(self) -> str:
        """Generate a unique workspace ID."""
        return f"ws-{uuid.uuid4().hex[:12]}"

    async def create(
        self,
        user_id: str,
        git_url: str,
        branch: str = "main",
    ) -> Workspace:
        """Create a new workspace with git repo cloned.

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
            # Ensure base directory exists
            self.base_dir.mkdir(parents=True, exist_ok=True)

            # Clone git repository
            clone_cmd = f"git clone --branch {branch} --depth 1 {git_url} {workspace_path}"
            process = await asyncio.create_subprocess_shell(
                clone_cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            _, stderr = await asyncio.wait_for(process.communicate(), timeout=120)

            if process.returncode != 0:
                error_msg = stderr.decode() if stderr else "Unknown git clone error"
                workspace.status = WorkspaceStatus.ERROR
                workspace.error_message = error_msg
                raise WorkspaceCreationError(f"Git clone failed: {error_msg}")

            # Update workspace status
            workspace.status = WorkspaceStatus.READY
            return workspace

        except asyncio.TimeoutError:
            workspace.status = WorkspaceStatus.ERROR
            workspace.error_message = "Git clone timed out"
            raise WorkspaceCreationError("Git clone timed out after 120 seconds")

        except WorkspaceCreationError:
            raise

        except Exception as e:
            workspace.status = WorkspaceStatus.ERROR
            workspace.error_message = str(e)
            raise WorkspaceCreationError(str(e))

    async def destroy(self, workspace_id: str) -> bool:
        """Destroy a workspace and clean up resources.

        Args:
            workspace_id: ID of workspace to destroy

        Returns:
            True if destroyed successfully, False otherwise
        """
        async with self._lock:
            workspace = self.workspaces.get(workspace_id)
            if not workspace:
                return False

            workspace_path = self._get_workspace_path(workspace_id)

            try:
                # Remove workspace directory
                if workspace_path.exists():
                    shutil.rmtree(workspace_path)

                # Remove from registry
                del self.workspaces[workspace_id]
                return True

            except Exception:
                return False

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
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            raise WorkspaceNotFoundError(workspace_id)

        if not workspace.is_usable():
            raise WorkspaceNotReadyError(workspace_id, workspace.status.value)

        workspace_path = self._get_workspace_path(workspace_id)

        # Mark as running
        old_status = workspace.status
        workspace.status = WorkspaceStatus.RUNNING

        start_time = datetime.now(UTC)
        timed_out = False

        try:
            process = await asyncio.create_subprocess_shell(
                command,
                cwd=workspace_path,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )

            try:
                stdout, stderr = await asyncio.wait_for(
                    process.communicate(),
                    timeout=timeout,
                )
            except asyncio.TimeoutError:
                process.kill()
                await process.wait()
                timed_out = True
                stdout = b""
                stderr = b"Command timed out"

            duration = (datetime.now(UTC) - start_time).total_seconds()

            return ExecResult(
                exit_code=process.returncode if not timed_out else -1,
                stdout=stdout.decode() if stdout else "",
                stderr=stderr.decode() if stderr else "",
                timed_out=timed_out,
                duration_seconds=duration,
            )

        finally:
            # Restore status
            workspace.status = old_status

    async def read_file(self, workspace_id: str, path: str) -> str:
        """Read a file from the workspace.

        Args:
            workspace_id: Workspace ID
            path: Relative path to file within workspace

        Returns:
            File contents as string

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
            FileNotFoundError: If file doesn't exist
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            raise WorkspaceNotFoundError(workspace_id)

        workspace_path = self._get_workspace_path(workspace_id)
        file_path = workspace_path / path

        # Security: ensure path is within workspace
        try:
            file_path.resolve().relative_to(workspace_path.resolve())
        except ValueError:
            raise FileNotFoundError(f"Path escapes workspace: {path}")

        if not file_path.exists():
            raise FileNotFoundError(f"File not found: {path}")

        return file_path.read_text()

    async def write_file(
        self,
        workspace_id: str,
        path: str,
        content: str,
    ) -> bool:
        """Write a file to the workspace.

        Args:
            workspace_id: Workspace ID
            path: Relative path to file within workspace
            content: File content to write

        Returns:
            True if written successfully

        Raises:
            WorkspaceNotFoundError: If workspace doesn't exist
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            raise WorkspaceNotFoundError(workspace_id)

        workspace_path = self._get_workspace_path(workspace_id)
        file_path = workspace_path / path

        # Security: ensure path is within workspace
        try:
            file_path.resolve().relative_to(workspace_path.resolve())
        except ValueError:
            return False

        # Create parent directories if needed
        file_path.parent.mkdir(parents=True, exist_ok=True)
        file_path.write_text(content)
        return True

    async def health_check(self, workspace_id: str) -> bool:
        """Check if workspace is healthy.

        Args:
            workspace_id: Workspace ID to check

        Returns:
            True if workspace is healthy, False otherwise
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            return False

        workspace_path = self._get_workspace_path(workspace_id)

        # Check workspace directory exists
        if not workspace_path.exists():
            return False

        # Check status is active
        if not workspace.is_active():
            return False

        return True

    async def get_stats(self, workspace_id: str) -> WorkspaceStats | None:
        """Get resource usage statistics.

        Args:
            workspace_id: Workspace ID

        Returns:
            WorkspaceStats if available, None otherwise
        """
        workspace = self.workspaces.get(workspace_id)
        if not workspace:
            return None

        workspace_path = self._get_workspace_path(workspace_id)

        if not workspace_path.exists():
            return None

        # Calculate disk usage
        total_size = 0
        try:
            for entry in workspace_path.rglob("*"):
                if entry.is_file():
                    total_size += entry.stat().st_size
        except Exception:
            total_size = 0

        disk_mb = total_size / (1024 * 1024)

        # Local implementation doesn't track CPU/memory
        return WorkspaceStats(
            cpu_percent=0.0,
            memory_mb=0.0,
            disk_mb=disk_mb,
        )

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
