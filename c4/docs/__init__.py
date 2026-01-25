"""C4 Self-Documenting Platform.

Provides code analysis, spec mapping, gap analysis, and documentation generation.
"""

from .analyzer import CodeAnalyzer, Symbol, SymbolKind
from .testgen import (
    EarsPattern,
    Requirement,
    TestFormat,
    TestGenerationResult,
    TestGenerator,
    TestStub,
)

__all__ = [
    "CodeAnalyzer",
    "Symbol",
    "SymbolKind",
    "EarsPattern",
    "Requirement",
    "TestFormat",
    "TestGenerationResult",
    "TestGenerator",
    "TestStub",
]
