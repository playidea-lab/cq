"""C4 Self-Documenting Platform.

Provides comprehensive code analysis, spec mapping, gap analysis, and documentation generation:
- CodeAnalyzer: AST-based symbol extraction (functions, classes, methods)
- SemanticSearcher: TF-IDF based natural language search
- CallGraphAnalyzer: Function call relationship analysis
- GapAnalyzer: Requirements-to-implementation mapping
- TestGenerator: EARS-based test generation
"""

from .analyzer import (
    CodeAnalyzer,
    Symbol,
    SymbolKind,
    Reference,
    Dependency,
    Location,
)
from .semantic_search import (
    SemanticSearcher,
    SearchResult,
    SearchHit,
    SearchScope,
)
from .call_graph import (
    CallGraphAnalyzer,
    CallNode,
    CallEdge,
    CallPath,
    CallGraphStats,
    RelationType,
)
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
    "Reference",
    "Dependency",
    "Location",
    # Semantic Search
    "SemanticSearcher",
    "SearchResult",
    "SearchHit",
    "SearchScope",
    # Call Graph
    "CallGraphAnalyzer",
    "CallNode",
    "CallEdge",
    "CallPath",
    "CallGraphStats",
    "RelationType",
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
