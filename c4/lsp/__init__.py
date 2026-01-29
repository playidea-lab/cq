"""C4 LSP Server - Language Server Protocol support for IDE integration

Provides LSP-based communication between C4 and IDEs for:
- Real-time project status updates
- Task queue notifications
- File change tracking

Usage:
    # stdio mode (for IDE direct connection)
    from c4.lsp import LSPServer
    server = LSPServer(c4_dir)
    server.run_stdio()

    # TCP mode (for daemon embedding)
    server = LSPServer(c4_dir)
    await server.run_tcp(host="127.0.0.1", port=8765)
"""

from .server import LSPServer

__all__ = [
    "LSPServer",
]
