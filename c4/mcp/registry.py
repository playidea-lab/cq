"""MCP Tool Registry - Registration and dispatching of MCP tools.

This module provides a centralized registry for MCP tools, eliminating
the large if-elif chain in call_tool.

Usage:
    from c4.mcp.registry import tool_registry, register_tool

    @register_tool("c4_status")
    def handle_status(daemon, arguments):
        return daemon.c4_status()

    # In call_tool:
    result = tool_registry.dispatch(name, daemon, arguments)
"""

from typing import Any, Callable, Protocol

# Type for tool handler function
ToolHandler = Callable[["C4DaemonProtocol", dict[str, Any]], Any]


class C4DaemonProtocol(Protocol):
    """Protocol defining C4Daemon interface for type checking."""

    def c4_status(self) -> dict[str, Any]:
        ...

    def c4_get_task(self, worker_id: str) -> Any:
        ...

    def c4_submit(
        self,
        task_id: str,
        commit_sha: str,
        validation_results: list[dict],
        worker_id: str | None = None,
    ) -> Any:
        ...

    # Add more methods as needed for type checking


class ToolRegistry:
    """Registry for MCP tool handlers.

    Provides:
    - Tool registration via decorator
    - Centralized dispatch
    - Tool listing for documentation
    """

    def __init__(self) -> None:
        self._handlers: dict[str, ToolHandler] = {}

    def register(self, name: str) -> Callable[[ToolHandler], ToolHandler]:
        """Decorator to register a tool handler.

        Args:
            name: Tool name (e.g., "c4_status")

        Returns:
            Decorator function
        """

        def decorator(handler: ToolHandler) -> ToolHandler:
            self._handlers[name] = handler
            return handler

        return decorator

    def dispatch(
        self, name: str, daemon: Any, arguments: dict[str, Any]
    ) -> dict[str, Any]:
        """Dispatch a tool call to its registered handler.

        Args:
            name: Tool name
            daemon: C4Daemon instance
            arguments: Tool arguments

        Returns:
            Handler result or error dict

        Raises:
            KeyError: If tool is not registered
        """
        if name not in self._handlers:
            return {"error": f"Unknown tool: {name}"}

        handler = self._handlers[name]
        return handler(daemon, arguments)

    def list_tools(self) -> list[str]:
        """List all registered tool names."""
        return list(self._handlers.keys())

    def is_registered(self, name: str) -> bool:
        """Check if a tool is registered."""
        return name in self._handlers


# Global registry instance
tool_registry = ToolRegistry()

# Convenience decorator
register_tool = tool_registry.register
