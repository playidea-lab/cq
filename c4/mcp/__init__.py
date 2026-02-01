"""C4 MCP Tools.

Provides MCP (Model Context Protocol) tools for Claude integration:
- MCP server implementation (server.py)
- Tool registry and handlers (registry.py, handlers/)
- Documentation tools (Context7-like)
- Gap analysis tools
- Code analysis tools (semantic search, call graph)
"""

from .code_tools import (
    CodeToolsHandler,
    get_code_tools,
)
from .docs_server import (
    DocGenerator,
    DocumentationEntry,
    ExampleEntry,
)
from .registry import register_tool, tool_registry
from .server import clear_daemon_cache, create_server, get_daemon, main

__all__ = [
    # Server
    "create_server",
    "get_daemon",
    "clear_daemon_cache",
    "main",
    # Registry
    "register_tool",
    "tool_registry",
    # Documentation tools
    "DocGenerator",
    "DocumentationEntry",
    "ExampleEntry",
    # Code tools
    "CodeToolsHandler",
    "get_code_tools",
]
