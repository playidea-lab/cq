"""Design phase tool handlers.

Handles: c4_save_design, c4_get_design, c4_list_designs, c4_design_complete
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_save_design")
def handle_save_design(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Save design specification for a feature."""
    return daemon.c4_save_design(
        feature=arguments["feature"],
        domain=arguments["domain"],
        selected_option=arguments.get("selected_option"),
        options=arguments.get("options"),
        components=arguments.get("components"),
        decisions=arguments.get("decisions"),
        mermaid_diagram=arguments.get("mermaid_diagram"),
        constraints=arguments.get("constraints"),
        nfr=arguments.get("nfr"),
        description=arguments.get("description"),
    )


@register_tool("c4_get_design")
def handle_get_design(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get design specification for a feature."""
    return daemon.c4_get_design(arguments["feature"])


@register_tool("c4_list_designs")
def handle_list_designs(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """List all features with design specifications."""
    return daemon.c4_list_designs()


@register_tool("c4_design_complete")
def handle_design_complete(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Mark design phase as complete, transition to PLAN."""
    return daemon.c4_design_complete()
