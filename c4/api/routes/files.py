"""File Operations API Routes.

Provides endpoints for file operations within workspaces:
- POST /read - Read file content
- POST /write - Write file content
- POST /list - List directory contents
- POST /search - Search files (glob/grep)
- DELETE /delete - Delete a file

Security:
- All paths are validated against path traversal attacks
- Operations are restricted to workspace root
"""

import fnmatch
import logging
import re
from pathlib import Path

from fastapi import APIRouter, HTTPException, status

from ..models import (
    DirectoryListRequest,
    DirectoryListResponse,
    ErrorResponse,
    FileDeleteRequest,
    FileDeleteResponse,
    FileInfo,
    FileReadRequest,
    FileReadResponse,
    FileSearchRequest,
    FileSearchResponse,
    FileWriteRequest,
    FileWriteResponse,
    SearchMatch,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/files", tags=["Files"])


# ============================================================================
# Workspace Manager (placeholder - will be replaced by container backend)
# ============================================================================


class WorkspaceManager:
    """Manages workspace directories.

    This is a placeholder implementation that uses local directories.
    Will be replaced by container-based workspace management later.
    """

    def __init__(self, base_path: Path | None = None):
        """Initialize workspace manager.

        Args:
            base_path: Base directory for all workspaces.
                      Defaults to /tmp/c4-workspaces for now.
        """
        self._base_path = base_path or Path("/tmp/c4-workspaces")
        self._base_path.mkdir(parents=True, exist_ok=True)

    def get_workspace_root(self, workspace_id: str) -> Path:
        """Get the root path for a workspace.

        Args:
            workspace_id: Workspace identifier

        Returns:
            Path to workspace root directory

        Raises:
            ValueError: If workspace_id is invalid
        """
        # Validate workspace_id
        if not workspace_id or ".." in workspace_id or "/" in workspace_id:
            raise ValueError(f"Invalid workspace_id: {workspace_id}")

        workspace_root = self._base_path / workspace_id
        workspace_root.mkdir(parents=True, exist_ok=True)
        return workspace_root


# Global workspace manager instance
_workspace_manager: WorkspaceManager | None = None


def get_workspace_manager() -> WorkspaceManager:
    """Get the workspace manager instance."""
    global _workspace_manager
    if _workspace_manager is None:
        _workspace_manager = WorkspaceManager()
    return _workspace_manager


def set_workspace_manager(manager: WorkspaceManager) -> None:
    """Set the workspace manager instance (for testing)."""
    global _workspace_manager
    _workspace_manager = manager


# ============================================================================
# Security Utilities
# ============================================================================


class PathSecurityError(Exception):
    """Raised when a path fails security validation."""

    pass


def validate_path(path: str, workspace_root: Path) -> Path:
    """Validate and resolve a path within workspace.

    Security checks:
    - No ".." components (path traversal)
    - No absolute paths
    - Resolved path must be within workspace root

    Args:
        path: Relative path to validate
        workspace_root: Workspace root directory

    Returns:
        Resolved absolute path

    Raises:
        PathSecurityError: If path fails security validation
    """
    # Check for empty path
    if not path:
        raise PathSecurityError("Path cannot be empty")

    # Check for path traversal attempts
    if ".." in path:
        raise PathSecurityError("Path traversal not allowed: '..' detected")

    # Check for absolute paths
    if path.startswith("/") or (len(path) > 1 and path[1] == ":"):
        raise PathSecurityError("Absolute paths not allowed")

    # Resolve workspace root first (handle symlinks like /var -> /private/var)
    resolved_root = workspace_root.resolve()

    # Resolve the full path
    try:
        full_path = (resolved_root / path).resolve()
    except (ValueError, OSError) as e:
        raise PathSecurityError(f"Invalid path: {e}") from e

    # Ensure resolved path is within workspace
    try:
        full_path.relative_to(resolved_root)
    except ValueError as e:
        raise PathSecurityError(
            f"Path escapes workspace boundary: {path}"
        ) from e

    return full_path


# ============================================================================
# File Operation Endpoints
# ============================================================================


@router.post(
    "/read",
    response_model=FileReadResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Read File",
    description="Read the content of a file from the workspace.",
)
async def read_file(request: FileReadRequest) -> FileReadResponse:
    """Read file content from workspace."""
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
        file_path = validate_path(request.path, workspace_root)

        if not file_path.exists():
            return FileReadResponse(
                success=False,
                path=request.path,
                error=f"File not found: {request.path}",
            )

        if not file_path.is_file():
            return FileReadResponse(
                success=False,
                path=request.path,
                error=f"Not a file: {request.path}",
            )

        # Read file content
        try:
            content = file_path.read_text(encoding="utf-8")
            size = file_path.stat().st_size
        except UnicodeDecodeError:
            return FileReadResponse(
                success=False,
                path=request.path,
                error="File is not valid UTF-8 text",
            )

        return FileReadResponse(
            success=True,
            path=request.path,
            content=content,
            size=size,
        )

    except PathSecurityError as e:
        logger.warning(f"Path security violation: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to read file: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to read file: {e}",
        ) from e


@router.post(
    "/write",
    response_model=FileWriteResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Write File",
    description="Write content to a file in the workspace.",
)
async def write_file(request: FileWriteRequest) -> FileWriteResponse:
    """Write content to a file in workspace."""
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
        file_path = validate_path(request.path, workspace_root)

        # Create parent directories if requested
        if request.create_dirs:
            file_path.parent.mkdir(parents=True, exist_ok=True)
        elif not file_path.parent.exists():
            return FileWriteResponse(
                success=False,
                path=request.path,
                error=f"Parent directory does not exist: {file_path.parent.name}",
            )

        # Write file content
        file_path.write_text(request.content, encoding="utf-8")
        size = file_path.stat().st_size

        return FileWriteResponse(
            success=True,
            path=request.path,
            size=size,
        )

    except PathSecurityError as e:
        logger.warning(f"Path security violation: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to write file: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to write file: {e}",
        ) from e


@router.post(
    "/list",
    response_model=DirectoryListResponse,
    responses={400: {"model": ErrorResponse}},
    summary="List Directory",
    description="List contents of a directory in the workspace.",
)
async def list_directory(request: DirectoryListRequest) -> DirectoryListResponse:
    """List directory contents in workspace."""
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
        # Resolve workspace_root to handle symlinks (e.g., /var -> /private/var on macOS)
        resolved_workspace_root = workspace_root.resolve()
        dir_path = validate_path(request.path, workspace_root)

        if not dir_path.exists():
            return DirectoryListResponse(
                success=False,
                path=request.path,
                error=f"Directory not found: {request.path}",
            )

        if not dir_path.is_dir():
            return DirectoryListResponse(
                success=False,
                path=request.path,
                error=f"Not a directory: {request.path}",
            )

        entries: list[FileInfo] = []

        if request.recursive:
            # Recursive listing
            for item in dir_path.rglob("*"):
                # Resolve item to handle symlinks
                resolved_item = item.resolve()
                # Skip hidden files unless requested
                if not request.include_hidden:
                    # Check if any part of the path is hidden
                    rel_parts = resolved_item.relative_to(dir_path).parts
                    if any(p.startswith(".") for p in rel_parts):
                        continue

                rel_path = str(resolved_item.relative_to(resolved_workspace_root))
                entries.append(
                    FileInfo(
                        name=item.name,
                        path=rel_path,
                        is_dir=item.is_dir(),
                        size=item.stat().st_size if item.is_file() else None,
                    )
                )
        else:
            # Non-recursive listing
            for item in dir_path.iterdir():
                # Skip hidden files unless requested
                if not request.include_hidden and item.name.startswith("."):
                    continue

                # Resolve item to handle symlinks
                resolved_item = item.resolve()
                rel_path = str(resolved_item.relative_to(resolved_workspace_root))
                entries.append(
                    FileInfo(
                        name=item.name,
                        path=rel_path,
                        is_dir=item.is_dir(),
                        size=item.stat().st_size if item.is_file() else None,
                    )
                )

        # Sort entries: directories first, then by name
        entries.sort(key=lambda e: (not e.is_dir, e.name.lower()))

        return DirectoryListResponse(
            success=True,
            path=request.path,
            entries=entries,
        )

    except PathSecurityError as e:
        logger.warning(f"Path security violation: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to list directory: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to list directory: {e}",
        ) from e


@router.post(
    "/search",
    response_model=FileSearchResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Search Files",
    description="Search for files using glob pattern or content using grep.",
)
async def search_files(request: FileSearchRequest) -> FileSearchResponse:
    """Search files in workspace using glob or grep."""
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
        # Resolve workspace_root to handle symlinks (e.g., /var -> /private/var on macOS)
        resolved_workspace_root = workspace_root.resolve()
        search_path = validate_path(request.path, workspace_root)

        if not search_path.exists():
            return FileSearchResponse(
                success=False,
                pattern=request.pattern,
                search_type=request.search_type,
                error=f"Search path not found: {request.path}",
            )

        if not search_path.is_dir():
            return FileSearchResponse(
                success=False,
                pattern=request.pattern,
                search_type=request.search_type,
                error=f"Search path is not a directory: {request.path}",
            )

        matches: list[SearchMatch] = []
        truncated = False

        if request.search_type == "glob":
            # Glob search for file names
            for item in search_path.rglob("*"):
                if len(matches) >= request.max_results:
                    truncated = True
                    break

                if item.is_file() and fnmatch.fnmatch(item.name, request.pattern):
                    # Resolve item to handle symlinks
                    resolved_item = item.resolve()
                    rel_path = str(resolved_item.relative_to(resolved_workspace_root))
                    matches.append(SearchMatch(path=rel_path))

        elif request.search_type == "grep":
            # Grep search for content
            try:
                pattern_re = re.compile(request.pattern)
            except re.error as e:
                return FileSearchResponse(
                    success=False,
                    pattern=request.pattern,
                    search_type=request.search_type,
                    error=f"Invalid regex pattern: {e}",
                )

            for item in search_path.rglob("*"):
                if len(matches) >= request.max_results:
                    truncated = True
                    break

                if not item.is_file():
                    continue

                # Try to read as text
                try:
                    content = item.read_text(encoding="utf-8")
                except (UnicodeDecodeError, OSError):
                    continue  # Skip binary or unreadable files

                # Search for pattern in content
                for line_num, line in enumerate(content.splitlines(), start=1):
                    if len(matches) >= request.max_results:
                        truncated = True
                        break

                    if pattern_re.search(line):
                        # Resolve item to handle symlinks
                        resolved_item = item.resolve()
                        rel_path = str(resolved_item.relative_to(resolved_workspace_root))
                        matches.append(
                            SearchMatch(
                                path=rel_path,
                                line_number=line_num,
                                line_content=line[:500],  # Truncate long lines
                            )
                        )

                if truncated:
                    break
        else:
            return FileSearchResponse(
                success=False,
                pattern=request.pattern,
                search_type=request.search_type,
                error=f"Invalid search_type: {request.search_type}. Use 'glob' or 'grep'.",
            )

        return FileSearchResponse(
            success=True,
            pattern=request.pattern,
            search_type=request.search_type,
            matches=matches,
            truncated=truncated,
        )

    except PathSecurityError as e:
        logger.warning(f"Path security violation: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to search files: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to search files: {e}",
        ) from e


@router.delete(
    "/delete",
    response_model=FileDeleteResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Delete File",
    description="Delete a file from the workspace.",
)
async def delete_file(request: FileDeleteRequest) -> FileDeleteResponse:
    """Delete a file from workspace."""
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
        file_path = validate_path(request.path, workspace_root)

        if not file_path.exists():
            return FileDeleteResponse(
                success=False,
                path=request.path,
                error=f"File not found: {request.path}",
            )

        if not file_path.is_file():
            return FileDeleteResponse(
                success=False,
                path=request.path,
                error=f"Not a file (use rmdir for directories): {request.path}",
            )

        # Delete the file
        file_path.unlink()

        return FileDeleteResponse(
            success=True,
            path=request.path,
        )

    except PathSecurityError as e:
        logger.warning(f"Path security violation: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to delete file: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to delete file: {e}",
        ) from e
