"""Supervisor tool handlers.

Handles: c4_ensure_supervisor, c4_checkpoint
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_ensure_supervisor")
def handle_ensure_supervisor(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Ensure supervisor loop is running for AI review."""
    return daemon.c4_ensure_supervisor(
        force_restart=arguments.get("force_restart", False)
    )


@register_tool("c4_checkpoint")
def handle_checkpoint(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Record supervisor checkpoint decision."""
    result = daemon.c4_checkpoint(
        arguments["checkpoint_id"],
        arguments["decision"],
        arguments["notes"],
        arguments.get("required_changes"),
    )

    # Record observation for profile learning
    try:
        daemon.profile_observer.record_checkpoint(
            decision=arguments["decision"],
            notes=arguments["notes"],
            required_changes=arguments.get("required_changes"),
        )
    except Exception:
        pass  # Non-critical

    return result.model_dump()
