"""C4 MCP Server - Main server implementation.

This module provides the MCP server setup and request handling.
Tool handlers are registered via the registry pattern in c4.mcp.handlers.
"""

import json
import logging
from pathlib import Path
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from ..daemon import C4Daemon

# Import handlers to register them
from . import handlers  # noqa: F401
from .registry import tool_registry
from .tool_schemas import get_tool_definitions

logger = logging.getLogger(__name__)

# Module-level daemon cache
_daemon_cache: dict[str, C4Daemon] = {}


def get_daemon(project_root: Path | None = None) -> C4Daemon:
    """Get or create a daemon for the current project root.

    Args:
        project_root: Optional project root path. If not provided,
                     uses C4_PROJECT_ROOT env var or current directory.

    Returns:
        C4Daemon instance for the project.
    """
    import os

    # Determine project root: env var > param > cwd
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
    elif project_root:
        root = project_root
    else:
        root = Path.cwd()

    root_str = str(root.resolve())

    if root_str not in _daemon_cache:
        daemon = C4Daemon(root)
        if daemon.is_initialized():
            daemon.load()
        _daemon_cache[root_str] = daemon

    return _daemon_cache[root_str]


def clear_daemon_cache(project_root_str: str | None = None) -> bool:
    """Clear daemon cache for a specific project or all projects.

    Args:
        project_root_str: Optional project root path string. If None,
                         clears all cached daemons.

    Returns:
        True if cache was cleared, False if specific project not found.
    """
    if project_root_str:
        if project_root_str in _daemon_cache:
            del _daemon_cache[project_root_str]
            return True
        return False
    else:
        _daemon_cache.clear()
        return True


def create_server(project_root: Path | None = None) -> Server:
    """Create the MCP server with all tools registered.

    Args:
        project_root: Optional default project root.

    Returns:
        Configured MCP Server instance.
    """
    server = Server("c4d")

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        """List all available C4 MCP tools."""
        return get_tool_definitions()

    @server.call_tool()
    async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
        """Handle tool calls by dispatching to registered handlers."""
        try:
            # Get daemon dynamically for the current project
            daemon = get_daemon(project_root)

            # Use registry for dispatch
            if tool_registry.is_registered(name):
                result = tool_registry.dispatch(name, daemon, arguments)
            else:
                result = {"error": f"Unknown tool: {name}"}

            # Convert result to JSON string
            if hasattr(result, "model_dump"):
                result = result.model_dump()

            return [TextContent(type="text", text=json.dumps(result, default=str))]

        except Exception as e:
            logger.error(f"MCP tool call error: {e}")
            return [TextContent(type="text", text=json.dumps({"error": str(e)}))]

    return server


async def main():
    """Run the MCP server."""
    from mcp.server import InitializationOptions
    from mcp.types import ServerCapabilities, ToolsCapability

    server = create_server()
    init_options = InitializationOptions(
        server_name="c4d",
        server_version="0.1.0",
        capabilities=ServerCapabilities(
            tools=ToolsCapability(listChanged=False),
        ),
    )
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, init_options)


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
