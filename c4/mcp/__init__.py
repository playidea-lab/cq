"""C4 MCP Tools.

Provides MCP (Model Context Protocol) tools for Claude integration:
- Documentation tools (Context7-like)
- Gap analysis tools
- Code analysis tools
"""

from .docs_server import (
    DocGenerator,
    DocumentationEntry,
    ExampleEntry,
)

__all__ = [
    "DocGenerator",
    "DocumentationEntry",
    "ExampleEntry",
]
