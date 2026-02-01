"""C4D MCP Server - Backward compatibility wrapper.

This module provides backward-compatible imports for code that imports from c4.mcp_server.
The actual implementation has been split into:
- c4/daemon/c4_daemon.py - C4Daemon class
- c4/mcp/server.py - MCP server implementation
- c4/mcp/handlers/ - Tool handlers

For new code, prefer importing from the specific modules:
    from c4.daemon import C4Daemon
    from c4.mcp import create_server, main
"""

# Re-export C4Daemon from the daemon module
from .daemon import C4Daemon, _get_workflow_guide, _use_graph_router

# Re-export server functions from the mcp module
from .mcp import clear_daemon_cache, create_server, get_daemon, main

__all__ = [
    # C4Daemon class
    "C4Daemon",
    # Server functions
    "create_server",
    "main",
    "get_daemon",
    "clear_daemon_cache",
    # Utility functions (for backward compatibility)
    "_get_workflow_guide",
    "_use_graph_router",
]
