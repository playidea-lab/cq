"""C4 Workspace Manager - Abstract base class for workspace management."""

from __future__ import annotations

from abc import ABC, abstractmethod

from .models import ExecResult, Workspace, WorkspaceStats


class WorkspaceManager(ABC):
    """Abstract base class for workspace management.

    This class defines the interface for managing isolated workspace
    environments. Implementations can use different backends:
    - LocalWorkspaceManager: Local filesystem (dev/testing)
    - DockerWorkspaceManager: Docker containers (production)
    - KubernetesWorkspaceManager: K8s pods (cloud-native)

    All methods are async for scalability and non-blocking I/O.

    Example:
        manager = LocalWorkspaceManager(base_dir="/tmp/workspaces")

        # Create workspace
        workspace = await manager.create(
            user_id="user-123",
            git_url="https://github.com/user/repo",
            branch="main"
        )

        # Execute command
        result = await manager.exec(workspace.id, "pytest tests/")

        # Clean up
        await manager.destroy(workspace.id)
    """

    @abstractmethod
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
            WorkspaceError: If creation fails
        """
        ...

    @abstractmethod
    async def destroy(self, workspace_id: str) -> bool:
        """Destroy a workspace and clean up resources.

        Args:
            workspace_id: ID of workspace to destroy

        Returns:
            True if destroyed successfully, False otherwise
        """
        ...

    @abstractmethod
    async def get(self, workspace_id: str) -> Workspace | None:
        """Get workspace by ID.

        Args:
            workspace_id: Workspace ID to retrieve

        Returns:
            Workspace if found, None otherwise
        """
        ...

    @abstractmethod
    async def list_by_user(self, user_id: str) -> list[Workspace]:
        """List all workspaces for a user.

        Args:
            user_id: User ID to filter by

        Returns:
            List of workspaces owned by the user
        """
        ...

    @abstractmethod
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
        ...

    @abstractmethod
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
        ...

    @abstractmethod
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
        ...

    @abstractmethod
    async def health_check(self, workspace_id: str) -> bool:
        """Check if workspace is healthy.

        Args:
            workspace_id: Workspace ID to check

        Returns:
            True if workspace is healthy, False otherwise
        """
        ...

    @abstractmethod
    async def get_stats(self, workspace_id: str) -> WorkspaceStats | None:
        """Get resource usage statistics.

        Args:
            workspace_id: Workspace ID

        Returns:
            WorkspaceStats if available, None otherwise
        """
        ...


class WorkspaceError(Exception):
    """Base exception for workspace errors."""

    pass


class WorkspaceNotFoundError(WorkspaceError):
    """Raised when workspace is not found."""

    def __init__(self, workspace_id: str):
        self.workspace_id = workspace_id
        super().__init__(f"Workspace not found: {workspace_id}")


class WorkspaceNotReadyError(WorkspaceError):
    """Raised when workspace is not ready for operations."""

    def __init__(self, workspace_id: str, status: str):
        self.workspace_id = workspace_id
        self.status = status
        super().__init__(f"Workspace {workspace_id} not ready: status={status}")


class WorkspaceCreationError(WorkspaceError):
    """Raised when workspace creation fails."""

    def __init__(self, reason: str):
        self.reason = reason
        super().__init__(f"Failed to create workspace: {reason}")
