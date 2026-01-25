"""Data models for Code Symbol Analysis Engine."""

from __future__ import annotations

from enum import Enum

from pydantic import BaseModel, Field


class SymbolKind(str, Enum):
    """Symbol type enumeration (LSP-compatible subset)."""

    MODULE = "module"
    CLASS = "class"
    FUNCTION = "function"
    METHOD = "method"
    PROPERTY = "property"
    VARIABLE = "variable"
    CONSTANT = "constant"
    PARAMETER = "parameter"
    INTERFACE = "interface"
    ENUM = "enum"
    ENUM_MEMBER = "enum_member"
    TYPE_ALIAS = "type_alias"
    IMPORT = "import"
    UNKNOWN = "unknown"


class SymbolLocation(BaseModel):
    """Location of a symbol in source code."""

    file_path: str
    start_line: int
    start_col: int = 0
    end_line: int | None = None
    end_col: int | None = None

    def __str__(self) -> str:
        """Format as file:line:col."""
        return f"{self.file_path}:{self.start_line}:{self.start_col}"


class SymbolInfo(BaseModel):
    """Information about a code symbol."""

    name: str
    kind: SymbolKind
    location: SymbolLocation
    doc: str | None = None  # Docstring or comment
    signature: str | None = None  # Function/method signature
    parent: str | None = None  # Parent symbol name path
    children: list[str] = Field(default_factory=list)  # Child symbol names
    references: list[SymbolLocation] = Field(default_factory=list)
    imports: list[str] = Field(default_factory=list)  # For modules
    decorators: list[str] = Field(default_factory=list)  # For functions/classes
    type_hint: str | None = None  # Type annotation
    is_async: bool = False
    is_exported: bool = True  # Public API

    @property
    def name_path(self) -> str:
        """Full qualified name path."""
        if self.parent:
            return f"{self.parent}/{self.name}"
        return self.name

    @property
    def is_private(self) -> bool:
        """Check if symbol is private (starts with _)."""
        return self.name.startswith("_")


class SymbolTable(BaseModel):
    """Symbol table for a single file."""

    file_path: str
    language: str  # "python", "typescript", "javascript"
    symbols: dict[str, SymbolInfo] = Field(default_factory=dict)
    imports: list[str] = Field(default_factory=list)
    exports: list[str] = Field(default_factory=list)
    errors: list[str] = Field(default_factory=list)  # Parse errors

    def add_symbol(self, symbol: SymbolInfo) -> None:
        """Add a symbol to the table."""
        self.symbols[symbol.name_path] = symbol

    def get_symbol(self, name_path: str) -> SymbolInfo | None:
        """Get a symbol by name path."""
        return self.symbols.get(name_path)

    def find_by_kind(self, kind: SymbolKind) -> list[SymbolInfo]:
        """Find all symbols of a specific kind."""
        return [s for s in self.symbols.values() if s.kind == kind]

    def find_by_name(self, name: str, substring: bool = False) -> list[SymbolInfo]:
        """Find symbols by name."""
        if substring:
            return [s for s in self.symbols.values() if name in s.name]
        return [s for s in self.symbols.values() if s.name == name]


class CodebaseIndex(BaseModel):
    """Index of all symbols in a codebase."""

    root_path: str
    tables: dict[str, SymbolTable] = Field(default_factory=dict)  # file_path -> table
    last_updated: str | None = None

    def add_table(self, table: SymbolTable) -> None:
        """Add a file's symbol table."""
        self.tables[table.file_path] = table

    def get_table(self, file_path: str) -> SymbolTable | None:
        """Get symbol table for a file."""
        return self.tables.get(file_path)

    def find_symbol(
        self,
        name_path: str,
        file_path: str | None = None,
    ) -> list[tuple[str, SymbolInfo]]:
        """Find symbol by name path across codebase.

        Returns list of (file_path, SymbolInfo) tuples.
        """
        results = []

        tables = [self.tables[file_path]] if file_path else self.tables.values()

        for table in tables:
            if table is None:
                continue
            # Exact match
            if name_path in table.symbols:
                results.append((table.file_path, table.symbols[name_path]))
            else:
                # Suffix match (e.g., "MyClass/my_method" matches "module/MyClass/my_method")
                for full_path, symbol in table.symbols.items():
                    if full_path.endswith(name_path):
                        results.append((table.file_path, symbol))

        return results

    def find_by_kind(self, kind: SymbolKind) -> list[tuple[str, SymbolInfo]]:
        """Find all symbols of a kind across codebase."""
        results = []
        for table in self.tables.values():
            for symbol in table.find_by_kind(kind):
                results.append((table.file_path, symbol))
        return results

    def find_references(
        self,
        symbol_name: str,
    ) -> list[SymbolLocation]:
        """Find all references to a symbol."""
        refs = []
        for table in self.tables.values():
            for symbol in table.symbols.values():
                if symbol.name == symbol_name:
                    refs.extend(symbol.references)
        return refs

    @property
    def stats(self) -> dict[str, int]:
        """Get statistics about the index."""
        total_symbols = sum(len(t.symbols) for t in self.tables.values())
        by_kind: dict[str, int] = {}
        for table in self.tables.values():
            for symbol in table.symbols.values():
                by_kind[symbol.kind.value] = by_kind.get(symbol.kind.value, 0) + 1

        return {
            "files": len(self.tables),
            "symbols": total_symbols,
            **by_kind,
        }


class ReferenceInfo(BaseModel):
    """Information about a reference to a symbol."""

    symbol_name: str
    location: SymbolLocation
    context: str | None = None  # Surrounding code snippet
    reference_kind: str = "usage"  # "usage", "import", "definition"


class AnalysisResult(BaseModel):
    """Result of code analysis."""

    success: bool
    index: CodebaseIndex | None = None
    errors: list[str] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)
    duration_ms: float | None = None
