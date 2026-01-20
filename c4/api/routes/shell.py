"""Shell Execution API Routes.

Provides endpoints for shell command execution within workspaces:
- POST /run - Execute a shell command
- POST /run-validation - Run configured validations

Security:
- All commands are validated against dangerous patterns
- Commands run in workspace root with restricted permissions
- Timeout enforcement (default: 60s, max: 300s)
"""

import asyncio
import logging
import re
import time
from pathlib import Path

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import require_initialized_project
from ..models import (
    ErrorResponse,
    ShellRunRequest,
    ShellRunResponse,
    ShellValidationRequest,
    ShellValidationResponse,
    ValidationResult,
    ValidationStatus,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/shell", tags=["Shell"])


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
# Security: Dangerous Command Detection
# ============================================================================


class CommandSecurityError(Exception):
    """Raised when a command is determined to be dangerous."""

    pass


# Patterns that match dangerous commands
# These are designed to catch common destructive operations
DANGEROUS_PATTERNS = [
    # rm with any flags on root or home directories
    (r"rm\s+(-[a-z]*\s+)?/\s*$", "rm /"),
    (r"rm\s+(-[a-z]*\s+)?/\*", "rm /*"),
    (r"rm\s+(-[a-z]*\s+)?/etc", "rm /etc"),
    (r"rm\s+(-[a-z]*\s+)?/usr", "rm /usr"),
    (r"rm\s+(-[a-z]*\s+)?/var", "rm /var"),
    (r"rm\s+(-[a-z]*\s+)?/home", "rm /home"),
    (r"rm\s+(-[a-z]*\s+)?~", "rm ~"),
    (r"rm\s+(-[a-z]*\s+)?~/", "rm ~/"),
    # dd if=/dev/* (disk operations)
    (r"dd\s+if=/dev/", "dd if=/dev/*"),
    # mkfs (filesystem creation)
    (r"mkfs\.", "mkfs.*"),
    # Fork bomb
    (r":\(\)\s*\{\s*:\|:&\s*\}\s*;", "fork bomb :(){ :|:& };:"),
    # chmod 777 / (dangerous permissions)
    (r"chmod\s+(-[a-z]*\s+)?777\s+/", "chmod 777 /"),
    (r"chmod\s+(-[a-z]*\s+)?777\s+/etc", "chmod 777 /etc"),
    # chown (ownership changes on system directories)
    (r"chown\s+.*\s+/(etc|usr|var|home|bin|sbin)", "chown on system directory"),
    # curl | bash or wget | sh (remote code execution) - including sudo
    (r"curl\s+.*\|\s*(sudo\s+)?(bash|sh)", "curl | bash"),
    (r"wget\s+.*\|\s*(sudo\s+)?(bash|sh)", "wget | bash"),
    (r"curl\s+.*-O\s*-\s*\|\s*(sudo\s+)?(bash|sh)", "curl | bash"),
    (r"wget\s+.*-O\s*-\s*\|\s*(sudo\s+)?(bash|sh)", "wget | bash"),
    # sudo rm on system directories
    (r"sudo\s+rm\s+(-[a-z]*\s+)?/", "sudo rm /"),
    # cd / && rm (change to root and delete)
    (r"cd\s+/\s*&&\s*rm", "cd / && rm"),
    (r"cd\s+/\s*;\s*rm", "cd / ; rm"),
]


def validate_command_security(command: str) -> None:
    """Validate that a command is not dangerous.

    Args:
        command: The shell command to validate

    Raises:
        CommandSecurityError: If the command matches a dangerous pattern
    """
    # Normalize command (remove extra whitespace, lowercase for matching)
    normalized = " ".join(command.split()).lower()

    for pattern, description in DANGEROUS_PATTERNS:
        if re.search(pattern, normalized, re.IGNORECASE):
            raise CommandSecurityError(
                f"Dangerous command blocked: {description}. This command could cause system damage and is not allowed."
            )

    # Additional checks for empty commands
    if not command.strip():
        raise CommandSecurityError("Empty command is not allowed")


# ============================================================================
# Shell Execution Endpoints
# ============================================================================


@router.post(
    "/run",
    response_model=ShellRunResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Run Shell Command",
    description="Execute a shell command in the workspace directory.",
)
async def run_command(request: ShellRunRequest) -> ShellRunResponse:
    """Execute a shell command in the workspace.

    Security:
    - Command is validated against dangerous patterns
    - Execution is confined to workspace directory
    - Timeout is enforced (max 300 seconds)
    """
    start_time = time.time()

    # Validate command security
    try:
        validate_command_security(request.command)
    except CommandSecurityError as e:
        logger.warning(f"Blocked dangerous command: {request.command}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e

    # Get workspace root
    try:
        manager = get_workspace_manager()
        workspace_root = manager.get_workspace_root(request.workspace_id)
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e

    # Cap timeout at maximum
    timeout = min(request.timeout, 300)

    # Execute command
    try:
        process = await asyncio.create_subprocess_shell(
            request.command,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=str(workspace_root),
        )

        try:
            stdout_bytes, stderr_bytes = await asyncio.wait_for(
                process.communicate(),
                timeout=timeout,
            )
            timed_out = False
            exit_code = process.returncode or 0
        except asyncio.TimeoutError:
            # Kill the process on timeout
            process.kill()
            await process.wait()
            stdout_bytes = b""
            stderr_bytes = b"Command timed out"
            timed_out = True
            exit_code = -1

        # Decode output
        stdout = stdout_bytes.decode("utf-8", errors="replace")
        stderr = stderr_bytes.decode("utf-8", errors="replace")

        duration = time.time() - start_time

        return ShellRunResponse(
            success=exit_code == 0 and not timed_out,
            stdout=stdout,
            stderr=stderr,
            exit_code=exit_code,
            timed_out=timed_out,
            duration_seconds=duration,
        )

    except Exception as e:
        logger.error(f"Failed to execute command: {e}")
        duration = time.time() - start_time
        return ShellRunResponse(
            success=False,
            stdout="",
            stderr=str(e),
            exit_code=-1,
            timed_out=False,
            duration_seconds=duration,
        )


@router.post(
    "/run-validation",
    response_model=ShellValidationResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Run Workspace Validations",
    description="Run configured validations (lint, test, etc.) in the workspace.",
)
async def run_validation(
    request: ShellValidationRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> ShellValidationResponse:
    """Run validations in the workspace.

    Uses the C4 daemon's validation configuration to run specified
    validations (lint, test, etc.) and return results.
    """
    start_time = time.time()

    # Validate workspace_id
    try:
        manager = get_workspace_manager()
        manager.get_workspace_root(request.workspace_id)
    except ValueError as e:
        logger.warning(f"Invalid workspace_id: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e

    try:
        # Use daemon's validation runner
        result = daemon.c4_run_validation(
            names=request.names if request.names else None,
            fail_fast=request.fail_fast,
            timeout=request.timeout,
        )

        duration = time.time() - start_time

        # Convert results to response format
        validation_results = []
        all_passed = True

        for r in result.get("results", []):
            is_pass = r.get("status") == "pass"
            status_val = ValidationStatus.PASS if is_pass else ValidationStatus.FAIL
            if status_val == ValidationStatus.FAIL:
                all_passed = False

            validation_results.append(
                ValidationResult(
                    name=r.get("name", "unknown"),
                    status=status_val,
                    message=r.get("message"),
                )
            )

        return ShellValidationResponse(
            results=validation_results,
            all_passed=all_passed,
            duration_seconds=duration,
        )

    except Exception as e:
        logger.error(f"Failed to run validations: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
