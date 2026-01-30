"""Discovery phase tool handlers.

Handles: c4_save_spec, c4_list_specs, c4_get_spec, c4_discovery_complete
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_save_spec")
def handle_save_spec(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Save feature specification with EARS requirements."""
    return daemon.c4_save_spec(
        feature=arguments["feature"],
        requirements=arguments["requirements"],
        domain=arguments["domain"],
        description=arguments.get("description"),
    )


@register_tool("c4_list_specs")
def handle_list_specs(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """List all feature specifications."""
    return daemon.c4_list_specs()


@register_tool("c4_get_spec")
def handle_get_spec(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get a specific feature specification."""
    return daemon.c4_get_spec(arguments["feature"])


@register_tool("c4_discovery_complete")
def handle_discovery_complete(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Mark discovery phase as complete, transition to DESIGN."""
    return daemon.c4_discovery_complete()
