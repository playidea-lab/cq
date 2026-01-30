"""Worktree management tool handlers.

Handles: c4_worktree_status, c4_worktree_cleanup
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_worktree_status")
def handle_worktree_status(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get worktree status for all workers or a specific worker."""
    return daemon.c4_worktree_status(
        worker_id=arguments.get("worker_id"),
    )


@register_tool("c4_worktree_cleanup")
def handle_worktree_cleanup(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Clean up worktrees, optionally keeping active workers."""
    return daemon.c4_worktree_cleanup(
        keep_active=arguments.get("keep_active", True),
    )
