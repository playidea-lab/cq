"""C4 Workspace - Isolated workspace management for cloud workers."""

from .local import LocalWorkspaceManager
from .manager import (
    WorkspaceCreationError,
    WorkspaceError,
    WorkspaceManager,
    WorkspaceNotFoundError,
    WorkspaceNotReadyError,
)
from .models import ExecResult, Workspace, WorkspaceStats, WorkspaceStatus

# DockerWorkspaceManager is conditionally imported
# Requires: pip install c4[docker]
try:
    from .docker_backend import (  # noqa: F401
        DockerNotAvailableError,
        DockerWorkspaceManager,
    )

    _DOCKER_EXPORTS = ["DockerWorkspaceManager", "DockerNotAvailableError"]
except ImportError:
    _DOCKER_EXPORTS = []

__all__ = [
    "ExecResult",
    "LocalWorkspaceManager",
    "Workspace",
    "WorkspaceCreationError",
    "WorkspaceError",
    "WorkspaceManager",
    "WorkspaceNotFoundError",
    "WorkspaceNotReadyError",
    "WorkspaceStats",
    "WorkspaceStatus",
    *_DOCKER_EXPORTS,
]
