"""MCP Tool Handlers.

This package contains grouped tool handlers for the C4 MCP server.
Each module registers its handlers with the global tool_registry.

Import this package to register all handlers.
"""

# Import all handler modules to register them with the registry
from . import (
    agents,
    design,
    discovery,
    files,
    memory,
    state,
    supervisor,
    symbols,
    tasks,
    validation,
    worktree,
)

__all__ = [
    "agents",
    "design",
    "discovery",
    "files",
    "memory",
    "state",
    "supervisor",
    "symbols",
    "tasks",
    "validation",
    "worktree",
]
