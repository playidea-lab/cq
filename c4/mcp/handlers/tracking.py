"""Direct mode tracking tool handlers.

Handles: c4_claim, c4_report
Lightweight alternatives to c4_get_task/c4_submit for main-session work.
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_claim")
def handle_claim(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Claim a task for direct execution (no worker protocol)."""
    return daemon.c4_claim(arguments["task_id"])


@register_tool("c4_report")
def handle_report(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Report task completion for direct mode."""
    result = daemon.c4_report(
        task_id=arguments["task_id"],
        summary=arguments["summary"],
        files_changed=arguments.get("files_changed"),
    )

    # Record observation for profile learning
    try:
        daemon.profile_observer.record_report(
            summary=arguments["summary"],
            files_changed=arguments.get("files_changed"),
        )
    except Exception:
        pass  # Non-critical

    return result
