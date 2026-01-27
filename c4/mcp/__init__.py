"""C4 MCP Tools.

Provides MCP (Model Context Protocol) tools for Claude integration:
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

__all__ = [
    "DocGenerator",
    "DocumentationEntry",
    "ExampleEntry",
    "CodeToolsHandler",
    "get_code_tools",
]
