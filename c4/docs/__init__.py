"""C4 Self-Documenting Platform.

Provides code analysis, spec mapping, gap analysis, and documentation generation.
"""

from .analyzer import CodeAnalyzer, Symbol, SymbolKind

__all__ = ["CodeAnalyzer", "Symbol", "SymbolKind"]
