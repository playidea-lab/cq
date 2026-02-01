"""Task management tool handlers.

Handles: c4_get_task, c4_submit, c4_add_todo, c4_mark_blocked
"""

from typing import Any

from ..registry import register_tool


def _get_enforce_mode_hint(daemon: Any) -> dict[str, Any] | None:
    """Get enforce_mode hint from config (Pydantic model)."""
    if not hasattr(daemon, "config") or not hasattr(daemon.config, "enforce_mode"):
        return None

    enforce_mode = daemon.config.enforce_mode
    if not enforce_mode.enabled:
        return None

    return {
        "message": enforce_mode.hints.message,
        "blocked_patterns": enforce_mode.docs.blocked_patterns,
    }


@register_tool("c4_get_task")
def handle_get_task(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Request next task assignment for a worker."""
    result = daemon.c4_get_task(arguments["worker_id"])
    if result:
        response = result.model_dump()
        # Add enforce_mode hint
        hint = _get_enforce_mode_hint(daemon)
        if hint:
            response["enforce_mode"] = hint
        return response
    return {}


@register_tool("c4_submit")
def handle_submit(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Report task completion with validation results."""
    result = daemon.c4_submit(
        arguments["task_id"],
        arguments["commit_sha"],
        arguments["validation_results"],
        arguments.get("worker_id"),  # Optional for ownership verification
    )
    return result.model_dump()


@register_tool("c4_add_todo")
def handle_add_todo(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Add a new task to the queue."""
    return daemon.c4_add_todo(
        task_id=arguments["task_id"],
        title=arguments["title"],
        scope=arguments.get("scope"),
        dod=arguments["dod"],
        dependencies=arguments.get("dependencies"),
        domain=arguments.get("domain"),
        priority=arguments.get("priority", 0),
    )


@register_tool("c4_mark_blocked")
def handle_mark_blocked(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Mark a task as blocked after max retry attempts."""
    return daemon.c4_mark_blocked(
        task_id=arguments["task_id"],
        worker_id=arguments["worker_id"],
        failure_signature=arguments["failure_signature"],
        attempts=arguments["attempts"],
        last_error=arguments.get("last_error", ""),
    )
