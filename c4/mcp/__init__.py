"""C4 MCP Tools.

Provides MCP (Model Context Protocol) tools for Claude integration:
- Documentation tools (Context7-like)
- Gap analysis tools
- Code analysis tools (semantic search, call graph)
- Tool registry and handlers for C4 MCP server
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

__all__ = [
    "DocGenerator",
    "DocumentationEntry",
    "ExampleEntry",
    "CodeToolsHandler",
    "get_code_tools",
    "register_tool",
    "tool_registry",
]
