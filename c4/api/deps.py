"""Dependency injection for C4 API.

Provides FastAPI dependencies for:
- C4Daemon instance management
- Authentication (JWT and API key)
- Rate limiting
"""

import logging
from functools import lru_cache
from pathlib import Path

from fastapi import Depends, HTTPException, status

# Re-export auth dependencies for convenience
from c4.api.auth import (
    AuthConfig,
    AuthenticationError,
    AuthorizationError,
    CurrentUser,
    OptionalUser,
    User,
    get_auth_config,
    get_current_user,
    get_optional_user,
    require_auth,
)
from c4.mcp_server import C4Daemon

logger = logging.getLogger(__name__)

__all__ = [
    # Daemon dependencies
    "get_project_root",
    "get_daemon_singleton",
    "get_daemon",
    "clear_daemon_cache",
    "require_initialized_project",
    "require_execute_state",
    # Auth dependencies
    "AuthConfig",
    "AuthenticationError",
    "AuthorizationError",
    "CurrentUser",
    "OptionalUser",
    "User",
    "get_auth_config",
    "get_current_user",
    "get_optional_user",
    "require_auth",
]

# Global daemon instance (singleton per project)
_daemon_cache: dict[str, C4Daemon] = {}


def get_project_root() -> Path:
    """Get the project root from environment or current directory.

    Returns:
        Path to project root
    """
    import os

    # Check environment variable first
    if env_root := os.environ.get("C4_PROJECT_ROOT"):
        return Path(env_root)

    # Default to current working directory
    return Path.cwd()


@lru_cache(maxsize=1)
def get_daemon_singleton() -> C4Daemon:
    """Get or create the singleton C4Daemon instance.

    This is cached to ensure only one daemon instance exists per process.

    Returns:
        C4Daemon instance

    Raises:
        HTTPException: If daemon initialization fails
    """
    project_root = get_project_root()

    try:
        daemon = C4Daemon(str(project_root))

        # Load existing project if .c4 directory exists
        if (project_root / ".c4").exists():
            daemon.load()
            logger.info(f"Loaded existing C4 project at {project_root}")
        else:
            logger.info(f"C4 project not initialized at {project_root}")

        return daemon
    except Exception as e:
        logger.error(f"Failed to initialize C4Daemon: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to initialize C4 daemon: {str(e)}",
        ) from e


def get_daemon() -> C4Daemon:
    """FastAPI dependency to get C4Daemon.

    Use this in route handlers:
        @router.get("/status")
        async def get_status(daemon: C4Daemon = Depends(get_daemon)):
            return daemon.c4_status()

    Returns:
        C4Daemon instance
    """
    return get_daemon_singleton()


def clear_daemon_cache() -> None:
    """Clear the daemon singleton cache.

    Useful for testing or when project root changes.
    """
    get_daemon_singleton.cache_clear()
    logger.info("Daemon cache cleared")


def require_initialized_project(daemon: C4Daemon = Depends(get_daemon)) -> C4Daemon:
    """Dependency that requires C4 project to be initialized.

    Use for routes that need an existing project:
        @router.post("/get-task")
        async def get_task(daemon: C4Daemon = Depends(require_initialized_project)):
            ...

    Returns:
        C4Daemon instance

    Raises:
        HTTPException: If project is not initialized
    """
    if not daemon.is_initialized():
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="C4 project not initialized. Run 'c4 init' first.",
        )
    return daemon


def require_execute_state(daemon: C4Daemon = Depends(require_initialized_project)) -> C4Daemon:
    """Dependency that requires C4 to be in EXECUTE state.

    Use for routes that need execution mode:
        @router.post("/get-task")
        async def get_task(daemon: C4Daemon = Depends(require_execute_state)):
            ...

    Returns:
        C4Daemon instance

    Raises:
        HTTPException: If not in EXECUTE state
    """
    status_info = daemon.c4_status()
    if status_info.get("state") != "EXECUTE":
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"C4 must be in EXECUTE state. Current state: {status_info.get('state')}",
        )
    return daemon
