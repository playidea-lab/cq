"""CLI entry point for C4 LSP Server.

Usage:
    uv run python -m c4.lsp              # stdio mode
    uv run python -m c4.lsp --tcp        # TCP mode (localhost:2087)
    uv run python -m c4.lsp --tcp --port 8080  # custom port
"""

from c4.lsp.server import main

if __name__ == "__main__":
    main()
