"""C4 Workspace Models - Data models for workspace management."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import Enum
from typing import Any


class WorkspaceStatus(str, Enum):
    """Workspace lifecycle status.

    States:
        CREATING: Workspace is being provisioned (git clone in progress)
        READY: Workspace is ready for use (git clone complete)
        RUNNING: A command is currently executing in the workspace
        STOPPED: Workspace is stopped but can be resumed
        ERROR: Workspace encountered an error
    """

    CREATING = "creating"
    READY = "ready"
    RUNNING = "running"
    STOPPED = "stopped"
    ERROR = "error"


@dataclass
class Workspace:
    """Represents an isolated workspace environment.

    A workspace is a sandboxed environment with a git repository
    cloned and ready for execution. Workspaces are user-scoped
    and can contain multiple files and directories.

    Attributes:
        id: Unique workspace identifier
        user_id: Owner of the workspace
        git_url: Git repository URL
        branch: Git branch name (default: main)
        status: Current workspace status
        created_at: When the workspace was created
        container_id: Container/process ID (if applicable)
        error_message: Error details if status is ERROR
        metadata: Additional workspace metadata
    """

    id: str
    user_id: str
    git_url: str
    branch: str = "main"
    status: WorkspaceStatus = WorkspaceStatus.CREATING
    created_at: datetime = field(default_factory=lambda: datetime.now(UTC))
    container_id: str | None = None
    error_message: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    def is_active(self) -> bool:
        """Check if workspace is in an active state."""
        return self.status in (WorkspaceStatus.READY, WorkspaceStatus.RUNNING)

    def is_usable(self) -> bool:
        """Check if workspace can accept commands."""
        return self.status == WorkspaceStatus.READY


@dataclass
class ExecResult:
    """Result of command execution in a workspace.

    Attributes:
        exit_code: Process exit code (0 = success)
        stdout: Standard output content
        stderr: Standard error content
        timed_out: Whether execution exceeded timeout
        duration_seconds: Execution time in seconds
    """

    exit_code: int
    stdout: str
    stderr: str
    timed_out: bool = False
    duration_seconds: float = 0.0

    @property
    def success(self) -> bool:
        """Check if command executed successfully."""
        return self.exit_code == 0 and not self.timed_out


@dataclass
class WorkspaceStats:
    """Resource usage statistics for a workspace.

    Attributes:
        cpu_percent: CPU usage percentage (0-100)
        memory_mb: Memory usage in megabytes
        disk_mb: Disk usage in megabytes
    """

    cpu_percent: float
    memory_mb: float
    disk_mb: float
