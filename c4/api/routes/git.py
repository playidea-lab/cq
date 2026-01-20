"""Git API Routes.

Provides endpoints for Git operations:
- GET /status - Get git status
- POST /commit - Create a commit
"""

import logging
import subprocess
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import get_project_root, require_initialized_project
from ..models import (
    ErrorResponse,
    GitCommitRequest,
    GitCommitResponse,
    GitStatusResponse,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/git", tags=["Git"])


def _run_git_command(args: list[str], cwd: str) -> tuple[bool, str]:
    """Run a git command and return (success, output)."""
    try:
        result = subprocess.run(
            ["git", *args],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30,
        )
        return result.returncode == 0, result.stdout.strip() or result.stderr.strip()
    except subprocess.TimeoutExpired:
        return False, "Command timed out"
    except Exception as e:
        return False, str(e)


@router.get(
    "/status",
    response_model=GitStatusResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Get Git Status",
    description="Get current git repository status.",
)
async def get_git_status(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> GitStatusResponse:
    """Get git repository status."""
    try:
        project_root = str(get_project_root())

        # Get current branch
        success, branch = _run_git_command(["branch", "--show-current"], project_root)
        if not success:
            branch = "unknown"

        # Check if clean
        success, status_output = _run_git_command(["status", "--porcelain"], project_root)

        staged = []
        modified = []
        untracked = []

        if success and status_output:
            for line in status_output.split("\n"):
                if not line:
                    continue

                status_code = line[:2]
                filename = line[3:].strip()

                if status_code[0] in "MADRC":
                    staged.append(filename)
                if status_code[1] == "M":
                    modified.append(filename)
                elif status_code == "??":
                    untracked.append(filename)

        return GitStatusResponse(
            branch=branch,
            is_clean=not (staged or modified or untracked),
            staged=staged,
            modified=modified,
            untracked=untracked,
        )
    except Exception as e:
        logger.error(f"Failed to get git status: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/commit",
    response_model=GitCommitResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Create Git Commit",
    description="Create a git commit with task-related message.",
)
async def create_commit(
    request: GitCommitRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> GitCommitResponse:
    """Create a git commit."""
    try:
        project_root = str(get_project_root())

        # Check if there are changes to commit
        success, status_output = _run_git_command(["status", "--porcelain"], project_root)
        if not status_output:
            return GitCommitResponse(
                success=False,
                message="No changes to commit",
            )

        # Stage all changes
        success, _ = _run_git_command(["add", "-A"], project_root)
        if not success:
            return GitCommitResponse(
                success=False,
                message="Failed to stage changes",
            )

        # Create commit message
        message = request.message or f"{request.task_id}: Task completed"

        # Create commit
        success, output = _run_git_command(["commit", "-m", message], project_root)
        if not success:
            return GitCommitResponse(
                success=False,
                message=f"Failed to create commit: {output}",
            )

        # Get commit SHA
        success, sha = _run_git_command(["rev-parse", "HEAD"], project_root)

        return GitCommitResponse(
            success=True,
            commit_sha=sha if success else None,
            message=f"Created commit: {sha[:8] if sha else 'unknown'}",
        )
    except Exception as e:
        logger.error(f"Failed to create commit: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/log",
    responses={400: {"model": ErrorResponse}},
    summary="Get Git Log",
    description="Get recent git commits.",
)
async def get_git_log(
    limit: int = 10,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Get recent git commits."""
    try:
        project_root = str(get_project_root())

        # Get log
        success, log_output = _run_git_command(
            ["log", f"-{limit}", "--oneline", "--no-decorate"],
            project_root,
        )

        if not success:
            return {"commits": [], "error": log_output}

        commits = []
        for line in log_output.split("\n"):
            if line:
                parts = line.split(" ", 1)
                if len(parts) == 2:
                    commits.append({"sha": parts[0], "message": parts[1]})

        return {"commits": commits}
    except Exception as e:
        logger.error(f"Failed to get git log: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/diff",
    responses={400: {"model": ErrorResponse}},
    summary="Get Git Diff",
    description="Get current uncommitted changes.",
)
async def get_git_diff(
    staged: bool = False,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Get git diff."""
    try:
        project_root = str(get_project_root())

        args = ["diff"]
        if staged:
            args.append("--staged")

        success, diff_output = _run_git_command(args, project_root)

        return {
            "diff": diff_output if success else "",
            "has_changes": bool(diff_output),
        }
    except Exception as e:
        logger.error(f"Failed to get git diff: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
