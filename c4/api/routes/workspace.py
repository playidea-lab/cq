"""Workspace API Routes.

Provides endpoints for managing isolated workspace environments:
- POST /create - Create a new workspace from git repo
- GET /list - List user's workspaces
- GET /{workspace_id} - Get workspace details
- DELETE /{workspace_id} - Delete a workspace
- GET /{workspace_id}/status - Get workspace status and resource usage
- POST /{workspace_id}/exec - Execute command in workspace

Security:
- All endpoints require authentication (JWT or API key)
- Users can only access their own workspaces (authorization check)
"""

from __future__ import annotations

import logging
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status

from c4.services.activity import ActivityCollector, create_activity_collector
from c4.workspace import (
    LocalWorkspaceManager,
    WorkspaceCreationError,
    WorkspaceManager,
    WorkspaceNotReadyError,
)

from ..auth import User, get_current_user
from ..models import (
    WorkspaceCreateRequest,
    WorkspaceExecRequest,
    WorkspaceExecResponse,
    WorkspaceListResponse,
    WorkspaceResponse,
    WorkspaceStatusResponse,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/workspace", tags=["Workspace"])


# ============================================================================
# Dependency Injection
# ============================================================================


# Global workspace manager instance (for production, replace with proper DI)
_workspace_manager: WorkspaceManager | None = None


def get_workspace_manager() -> WorkspaceManager:
    """Get the workspace manager instance.

    Returns:
        WorkspaceManager implementation (LocalWorkspaceManager by default)
    """
    global _workspace_manager
    if _workspace_manager is None:
        _workspace_manager = LocalWorkspaceManager()
    return _workspace_manager


def set_workspace_manager(manager: WorkspaceManager) -> None:
    """Set the workspace manager instance (for testing).

    Args:
        manager: WorkspaceManager implementation to use
    """
    global _workspace_manager
    _workspace_manager = manager


# Type aliases for dependency injection
CurrentUser = Annotated[User, Depends(get_current_user)]
Manager = Annotated[WorkspaceManager, Depends(get_workspace_manager)]


def get_activity_collector() -> ActivityCollector:
    """Get ActivityCollector instance."""
    return create_activity_collector()


Activity = Annotated[ActivityCollector, Depends(get_activity_collector)]


# ============================================================================
# Helper Functions
# ============================================================================


def workspace_to_response(workspace) -> WorkspaceResponse:
    """Convert Workspace dataclass to WorkspaceResponse.

    Args:
        workspace: Workspace dataclass from workspace module

    Returns:
        WorkspaceResponse Pydantic model
    """
    return WorkspaceResponse(
        id=workspace.id,
        user_id=workspace.user_id,
        git_url=workspace.git_url,
        branch=workspace.branch,
        status=workspace.status.value,
        created_at=workspace.created_at,
        container_id=workspace.container_id,
        error_message=workspace.error_message,
    )


async def get_authorized_workspace(
    workspace_id: str,
    user: User,
    manager: WorkspaceManager,
):
    """Get workspace and verify user authorization.

    Args:
        workspace_id: Workspace ID to retrieve
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Workspace if found and authorized

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    workspace = await manager.get(workspace_id)
    if not workspace:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Workspace not found: {workspace_id}",
        )
    if workspace.user_id != user.user_id:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not authorized to access this workspace",
        )
    return workspace


# ============================================================================
# Endpoints
# ============================================================================


@router.post(
    "/create",
    response_model=WorkspaceResponse,
    status_code=status.HTTP_201_CREATED,
    summary="Create Workspace",
    description="Create a new workspace by cloning a git repository.",
)
async def create_workspace(
    request: WorkspaceCreateRequest,
    user: CurrentUser,
    manager: Manager,
) -> WorkspaceResponse:
    """Create a new workspace for the authenticated user.

    The workspace will clone the specified git repository and checkout
    the specified branch. The workspace is isolated to the current user.

    Args:
        request: Workspace creation parameters
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Created workspace details

    Raises:
        HTTPException: 400 if workspace creation fails
    """
    try:
        workspace = await manager.create(
            user_id=user.user_id,
            git_url=request.git_url,
            branch=request.branch,
        )
        logger.info(f"Created workspace {workspace.id} for user {user.user_id}")
        return workspace_to_response(workspace)

    except WorkspaceCreationError as e:
        logger.warning(f"Workspace creation failed: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e


@router.get(
    "/list",
    response_model=WorkspaceListResponse,
    summary="List Workspaces",
    description="List all workspaces owned by the authenticated user.",
)
async def list_workspaces(
    user: CurrentUser,
    manager: Manager,
) -> WorkspaceListResponse:
    """List all workspaces for the authenticated user.

    Args:
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        List of workspaces and total count
    """
    workspaces = await manager.list_by_user(user.user_id)
    return WorkspaceListResponse(
        workspaces=[workspace_to_response(ws) for ws in workspaces],
        total=len(workspaces),
    )


@router.get(
    "/{workspace_id}",
    response_model=WorkspaceResponse,
    summary="Get Workspace",
    description="Get details of a specific workspace.",
)
async def get_workspace(
    workspace_id: str,
    user: CurrentUser,
    manager: Manager,
) -> WorkspaceResponse:
    """Get workspace details by ID.

    Only the workspace owner can access this endpoint.

    Args:
        workspace_id: Workspace identifier
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Workspace details

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    workspace = await get_authorized_workspace(workspace_id, user, manager)
    return workspace_to_response(workspace)


@router.delete(
    "/{workspace_id}",
    summary="Delete Workspace",
    description="Delete a workspace and clean up all resources.",
)
async def delete_workspace(
    workspace_id: str,
    user: CurrentUser,
    manager: Manager,
) -> dict:
    """Delete a workspace.

    Only the workspace owner can delete it. This will:
    - Stop any running processes
    - Remove the workspace directory
    - Clean up all associated resources

    Args:
        workspace_id: Workspace identifier
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Success message

    Raises:
        HTTPException: 404 if not found, 403 if not authorized, 500 if deletion fails
    """
    await get_authorized_workspace(workspace_id, user, manager)

    success = await manager.destroy(workspace_id)
    if not success:
        logger.error(f"Failed to destroy workspace {workspace_id}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to delete workspace: {workspace_id}",
        )

    logger.info(f"Deleted workspace {workspace_id} for user {user.user_id}")
    return {"success": True, "message": f"Workspace {workspace_id} deleted"}


@router.get(
    "/{workspace_id}/status",
    response_model=WorkspaceStatusResponse,
    summary="Get Workspace Status",
    description="Get workspace status including resource usage.",
)
async def get_workspace_status(
    workspace_id: str,
    user: CurrentUser,
    manager: Manager,
) -> WorkspaceStatusResponse:
    """Get workspace status and resource usage.

    Returns current status and resource metrics:
    - CPU usage percentage
    - Memory usage in MB
    - Disk usage in MB
    - Health status

    Args:
        workspace_id: Workspace identifier
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Workspace status and resource metrics

    Raises:
        HTTPException: 404 if not found, 403 if not authorized
    """
    workspace = await get_authorized_workspace(workspace_id, user, manager)

    stats = await manager.get_stats(workspace_id)
    is_healthy = await manager.health_check(workspace_id)

    return WorkspaceStatusResponse(
        id=workspace.id,
        status=workspace.status.value,
        cpu_percent=stats.cpu_percent if stats else None,
        memory_mb=stats.memory_mb if stats else None,
        disk_mb=stats.disk_mb if stats else None,
        is_healthy=is_healthy,
    )


@router.post(
    "/{workspace_id}/exec",
    response_model=WorkspaceExecResponse,
    summary="Execute Command",
    description="Execute a command in the workspace.",
)
async def exec_in_workspace(
    workspace_id: str,
    request: WorkspaceExecRequest,
    user: CurrentUser,
    manager: Manager,
) -> WorkspaceExecResponse:
    """Execute a command in the workspace.

    The command runs in the workspace root directory with the
    specified timeout. Commands that exceed the timeout are killed.

    Args:
        workspace_id: Workspace identifier
        request: Command execution parameters
        user: Current authenticated user
        manager: Workspace manager instance

    Returns:
        Command execution results

    Raises:
        HTTPException: 404 if not found, 403 if not authorized, 409 if workspace not ready
    """
    await get_authorized_workspace(workspace_id, user, manager)

    try:
        result = await manager.exec(workspace_id, request.command, request.timeout)
        return WorkspaceExecResponse(
            exit_code=result.exit_code,
            stdout=result.stdout,
            stderr=result.stderr,
            timed_out=result.timed_out,
            duration_seconds=result.duration_seconds,
        )

    except WorkspaceNotReadyError as e:
        logger.warning(f"Workspace {workspace_id} not ready for exec: {e}")
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"Workspace not ready: status={e.status}",
        ) from e
