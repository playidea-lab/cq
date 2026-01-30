"""C4 LSP Server Module.

Provides Language Server Protocol support for code intelligence
powered by multilspy (real LSP servers) with jedi as fallback.
"""

from c4.lsp.jedi_provider import (
    JEDI_AVAILABLE,
    JediSymbolProvider,
    LSPSymbolKind,
    SymbolInfo,
    SymbolLocation,
    SymbolType,
)
from c4.lsp.multilspy_provider import MULTILSPY_AVAILABLE, MultilspyProvider
from c4.lsp.server import C4LSPServer
from c4.lsp.unified_provider import (
    UnifiedSymbolProvider,
    find_symbol_unified,
    get_symbols_overview_unified,
)

__all__ = [
    "C4LSPServer",
    # Jedi (fallback)
    "JEDI_AVAILABLE",
    "JediSymbolProvider",
    "LSPSymbolKind",
    "SymbolInfo",
    "SymbolLocation",
    "SymbolType",
    # Multilspy (primary)
    "MULTILSPY_AVAILABLE",
    "MultilspyProvider",
    # Unified (recommended)
    "UnifiedSymbolProvider",
    "find_symbol_unified",
    "get_symbols_overview_unified",
]
