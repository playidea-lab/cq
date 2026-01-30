"""Agent routing tool handlers.

Handles: c4_test_agent_routing, c4_query_agent_graph
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_test_agent_routing")
def handle_test_agent_routing(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Test agent routing configuration."""
    return daemon.c4_test_agent_routing(
        domain=arguments.get("domain"),
        task_type=arguments.get("task_type"),
    )


@register_tool("c4_query_agent_graph")
def handle_query_agent_graph(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Query the agent graph for agents, skills, domains, paths, and chains."""
    return daemon.c4_query_agent_graph(
        query_type=arguments.get("query_type", "overview"),
        filter_by=arguments.get("filter_by"),
        filter_value=arguments.get("filter_value"),
        output_format=arguments.get("output_format", "json"),
        from_agent=arguments.get("from_agent"),
        to_agent=arguments.get("to_agent"),
    )
