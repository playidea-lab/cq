"""Code Symbol Analysis Engine - Main coordinator."""

from __future__ import annotations

import time
from pathlib import Path
from typing import Callable

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

# File extensions and their parsers
LANGUAGE_EXTENSIONS = {
    ".py": "python",
    ".pyi": "python",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".js": "javascript",
    ".jsx": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
}

# Default ignore patterns
DEFAULT_IGNORE_PATTERNS = {
    "__pycache__",
    ".git",
    ".svn",
    ".hg",
    "node_modules",
    ".venv",
    "venv",
    ".env",
    "dist",
    "build",
    ".next",
    ".nuxt",
    "coverage",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    "*.egg-info",
}


class CodeAnalysisEngine:
    """Main engine for code symbol analysis.

    Provides unified interface for analyzing Python and TypeScript/JavaScript codebases.
    """

    def __init__(
        self,
        ignore_patterns: set[str] | None = None,
        progress_callback: Callable[[str, int, int], None] | None = None,
    ) -> None:
        """Initialize the engine.

        Args:
            ignore_patterns: Additional patterns to ignore (directories/files).
            progress_callback: Optional callback(file_path, current, total) for progress.
        """
        self._python_parser = PythonParser()
        self._typescript_parser = TypeScriptParser()
        self._ignore_patterns = DEFAULT_IGNORE_PATTERNS | (ignore_patterns or set())
        self._progress_callback = progress_callback

    def analyze_codebase(
        self,
        root_path: str | Path,
        file_patterns: list[str] | None = None,
    ) -> AnalysisResult:
        """Analyze an entire codebase.

        Args:
            root_path: Root directory of the codebase.
            file_patterns: Optional list of glob patterns to include (e.g., ["*.py"]).

        Returns:
            AnalysisResult with the complete codebase index.
        """
        start_time = time.time()
        root = Path(root_path).resolve()

        if not root.exists():
            return AnalysisResult(
                success=False,
                errors=[f"Path does not exist: {root}"],
            )

        index = CodebaseIndex(root_path=str(root))
        errors: list[str] = []
        warnings: list[str] = []

        # Collect files to analyze
        files = list(self._collect_files(root, file_patterns))
        total_files = len(files)

        # Parse each file
        for i, file_path in enumerate(files):
            if self._progress_callback:
                self._progress_callback(str(file_path), i + 1, total_files)

            try:
                table = self.analyze_file(file_path)
                if table.errors:
                    for err in table.errors:
                        warnings.append(f"{file_path}: {err}")
                index.add_table(table)
            except Exception as e:
                errors.append(f"Failed to analyze {file_path}: {e}")

        duration_ms = (time.time() - start_time) * 1000

        return AnalysisResult(
            success=len(errors) == 0,
            index=index,
            errors=errors,
            warnings=warnings,
            duration_ms=duration_ms,
        )

    def analyze_file(self, file_path: str | Path) -> SymbolTable:
        """Analyze a single file.

        Args:
            file_path: Path to the file.

        Returns:
            SymbolTable for the file.
        """
        file_path = Path(file_path)
        ext = file_path.suffix.lower()

        language = LANGUAGE_EXTENSIONS.get(ext)
        if not language:
            return SymbolTable(
                file_path=str(file_path),
                language="unknown",
                errors=[f"Unsupported file extension: {ext}"],
            )

        if language == "python":
            return self._python_parser.parse_file(file_path)
        else:
            return self._typescript_parser.parse_file(file_path)

    def analyze_source(
        self,
        source: str,
        language: str,
        filename: str = "<string>",
    ) -> SymbolTable:
        """Analyze source code string.

        Args:
            source: Source code.
            language: "python", "typescript", or "javascript".
            filename: Virtual filename.

        Returns:
            SymbolTable for the source.
        """
        if language == "python":
            return self._python_parser.parse_source(source, filename)
        elif language in ("typescript", "javascript"):
            return self._typescript_parser.parse_source(source, filename, language)
        else:
            return SymbolTable(
                file_path=filename,
                language=language,
                errors=[f"Unsupported language: {language}"],
            )

    def find_symbol(
        self,
        index: CodebaseIndex,
        name_path: str,
        file_path: str | None = None,
        kind: SymbolKind | None = None,
    ) -> list[tuple[str, SymbolInfo]]:
        """Find symbols matching a name path.

        Args:
            index: Codebase index to search.
            name_path: Symbol name path (e.g., "MyClass/my_method").
            file_path: Optional file path to restrict search.
            kind: Optional symbol kind filter.

        Returns:
            List of (file_path, SymbolInfo) tuples.
        """
        results = index.find_symbol(name_path, file_path)

        if kind:
            results = [(fp, s) for fp, s in results if s.kind == kind]

        return results

    def find_by_kind(
        self,
        index: CodebaseIndex,
        kind: SymbolKind,
        file_path: str | None = None,
    ) -> list[tuple[str, SymbolInfo]]:
        """Find all symbols of a specific kind.

        Args:
            index: Codebase index to search.
            kind: Symbol kind to find.
            file_path: Optional file path to restrict search.

        Returns:
            List of (file_path, SymbolInfo) tuples.
        """
        if file_path:
            table = index.get_table(file_path)
            if table:
                return [(file_path, s) for s in table.find_by_kind(kind)]
            return []

        return index.find_by_kind(kind)

    def find_references(
        self,
        index: CodebaseIndex,
        symbol_name: str,
        include_definitions: bool = False,
    ) -> list[ReferenceInfo]:
        """Find all references to a symbol.

        Args:
            index: Codebase index to search.
            symbol_name: Name of the symbol.
            include_definitions: Whether to include definition locations.

        Returns:
            List of ReferenceInfo objects.
        """
        refs: list[ReferenceInfo] = []

        for table in index.tables.values():
            for symbol in table.symbols.values():
                # Check if this is a definition
                if symbol.name == symbol_name:
                    if include_definitions:
                        refs.append(
                            ReferenceInfo(
                                symbol_name=symbol_name,
                                location=symbol.location,
                                context=symbol.signature or symbol.doc,
                                reference_kind="definition",
                            )
                        )
                    # Add stored references
                    for ref_loc in symbol.references:
                        refs.append(
                            ReferenceInfo(
                                symbol_name=symbol_name,
                                location=ref_loc,
                                reference_kind="usage",
                            )
                        )

                # Check imports
                for imp in table.imports:
                    if imp.endswith(f".{symbol_name}") or imp == symbol_name:
                        refs.append(
                            ReferenceInfo(
                                symbol_name=symbol_name,
                                location=SymbolLocation(
                                    file_path=table.file_path,
                                    start_line=1,  # Import location not tracked
                                    start_col=0,
                                ),
                                context=f"import {imp}",
                                reference_kind="import",
                            )
                        )

        return refs

    def get_symbol_tree(
        self,
        index: CodebaseIndex,
        file_path: str,
        max_depth: int = -1,
    ) -> dict:
        """Get hierarchical symbol tree for a file.

        Args:
            index: Codebase index.
            file_path: File to get tree for.
            max_depth: Maximum depth (-1 for unlimited).

        Returns:
            Nested dict representing symbol hierarchy.
        """
        table = index.get_table(file_path)
        if not table:
            return {}

        # Build tree from flat symbol table
        tree: dict = {
            "file": file_path,
            "language": table.language,
            "children": [],
        }

        # Group symbols by parent
        by_parent: dict[str | None, list[SymbolInfo]] = {}
        for symbol in table.symbols.values():
            parent = symbol.parent
            if parent not in by_parent:
                by_parent[parent] = []
            by_parent[parent].append(symbol)

        def build_node(symbol: SymbolInfo, depth: int) -> dict:
            node = {
                "name": symbol.name,
                "kind": symbol.kind.value,
                "location": f"{symbol.location.start_line}:{symbol.location.start_col}",
            }
            if symbol.signature:
                node["signature"] = symbol.signature
            if symbol.doc:
                node["doc"] = symbol.doc[:100] + "..." if len(symbol.doc or "") > 100 else symbol.doc

            if max_depth < 0 or depth < max_depth:
                name_path = symbol.name_path
                if name_path in by_parent:
                    node["children"] = [
                        build_node(child, depth + 1)
                        for child in sorted(by_parent[name_path], key=lambda s: s.location.start_line)
                    ]

            return node

        # Add top-level symbols
        root_symbols = by_parent.get(None, [])
        tree["children"] = [
            build_node(s, 0)
            for s in sorted(root_symbols, key=lambda s: s.location.start_line)
        ]

        return tree

    def _collect_files(
        self,
        root: Path,
        patterns: list[str] | None = None,
    ):
        """Collect files to analyze.

        Yields:
            Path objects for each file to analyze.
        """
        if patterns:
            # Use glob patterns
            for pattern in patterns:
                for file_path in root.glob(f"**/{pattern}"):
                    if self._should_include(file_path):
                        yield file_path
        else:
            # Walk directory
            for file_path in root.rglob("*"):
                if file_path.is_file() and self._should_include(file_path):
                    ext = file_path.suffix.lower()
                    if ext in LANGUAGE_EXTENSIONS:
                        yield file_path

    def _should_include(self, path: Path) -> bool:
        """Check if a path should be included in analysis."""
        parts = path.parts
        for part in parts:
            if part in self._ignore_patterns:
                return False
            # Check glob patterns
            for pattern in self._ignore_patterns:
                if "*" in pattern:
                    import fnmatch
                    if fnmatch.fnmatch(part, pattern):
                        return False
        return True


# Convenience function
def analyze_codebase(
    root_path: str | Path,
    file_patterns: list[str] | None = None,
    ignore_patterns: set[str] | None = None,
) -> AnalysisResult:
    """Analyze a codebase and return symbol index.

    Args:
        root_path: Root directory.
        file_patterns: Optional glob patterns to include.
        ignore_patterns: Additional patterns to ignore.

    Returns:
        AnalysisResult with codebase index.
    """
    engine = CodeAnalysisEngine(ignore_patterns=ignore_patterns)
    return engine.analyze_codebase(root_path, file_patterns)
