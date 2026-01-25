"""C4 MCP Tools.

Provides MCP (Model Context Protocol) tools for Claude integration:
- Documentation tools (Context7-like)
- Gap analysis tools
- Code analysis tools (semantic search, call graph)
"""

from .docs_server import (
    DocGenerator,
    DocumentationEntry,
    ExampleEntry,
)
from .code_tools import (
    CodeToolsHandler,
    get_code_tools,
)

__all__ = [
    "DocGenerator",
    "DocumentationEntry",
    "ExampleEntry",
    "CodeToolsHandler",
    "get_code_tools",
]
