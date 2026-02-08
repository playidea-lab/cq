"""C4 Bridge — JSON-RPC over TCP for Go-Python communication.

This module provides a lightweight TCP server that accepts JSON-RPC requests
from the Go MCP server and delegates them to Python implementations:

- **LSP**: Symbol search, editing, renaming via tree-sitter/multilspy/Jedi
- **Knowledge**: Hybrid vector + FTS5 search over Obsidian-style markdown docs
- **GPU**: CUDA/MPS detection and job scheduling

Protocol: newline-delimited JSON over TCP (no protoc/grpcio dependency).
"""

__version__ = "0.1.0"
