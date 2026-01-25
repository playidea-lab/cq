"""C4 Self-Documenting Platform.

Provides code analysis, spec mapping, gap analysis, and documentation generation.
"""

from .analyzer import CodeAnalyzer, Symbol, SymbolKind
from .gap import (
    GapAnalysisResult,
    GapAnalyzer,
    ImplementationStatus,
    Priority,
    RequirementGap,
)
from .testgen import (
    EarsPattern,
    Requirement,
    TestFormat,
    TestGenerationResult,
    TestGenerator,
    TestStub,
)

__all__ = [
    # Analyzer
    "CodeAnalyzer",
    "Symbol",
    "SymbolKind",
    # Gap Analysis
    "GapAnalyzer",
    "GapAnalysisResult",
    "ImplementationStatus",
    "Priority",
    "RequirementGap",
    # Test Generation
    "EarsPattern",
    "Requirement",
    "TestFormat",
    "TestGenerationResult",
    "TestGenerator",
    "TestStub",
]
