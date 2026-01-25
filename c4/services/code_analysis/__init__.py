"""Code Symbol Analysis Engine.

Provides unified interface for analyzing Python and TypeScript/JavaScript codebases.

Example usage:
    ```python
    from c4.services.code_analysis import (
        CodeAnalysisEngine,
        analyze_codebase,
        SymbolKind,
    )

    # Analyze entire codebase
    result = analyze_codebase("./my-project")
    if result.success:
        print(f"Analyzed {result.index.stats['files']} files")
        print(f"Found {result.index.stats['symbols']} symbols")

        # Find all classes
        classes = result.index.find_by_kind(SymbolKind.CLASS)
        for file_path, symbol in classes:
            print(f"  {symbol.name} in {file_path}:{symbol.location.start_line}")

    # Analyze single file
    engine = CodeAnalysisEngine()
    table = engine.analyze_file("./my_module.py")
    for symbol in table.symbols.values():
        print(f"{symbol.kind.value}: {symbol.name}")
    ```
"""

from .engine import (
    DEFAULT_IGNORE_PATTERNS,
    LANGUAGE_EXTENSIONS,
    CodeAnalysisEngine,
    analyze_codebase,
)
from .models import (
    AnalysisResult,
    CodebaseIndex,
    ReferenceInfo,
    SymbolInfo,
    SymbolKind,
    SymbolLocation,
    SymbolTable,
)
from .python_parser import PythonParser
from .typescript_parser import TypeScriptParser

__all__ = [
    # Engine
    "CodeAnalysisEngine",
    "analyze_codebase",
    "LANGUAGE_EXTENSIONS",
    "DEFAULT_IGNORE_PATTERNS",
    # Models
    "AnalysisResult",
    "CodebaseIndex",
    "ReferenceInfo",
    "SymbolInfo",
    "SymbolKind",
    "SymbolLocation",
    "SymbolTable",
    # Parsers
    "PythonParser",
    "TypeScriptParser",
]
