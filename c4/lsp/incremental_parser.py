"""Incremental parsing using Tree-sitter.

This module provides incremental parsing capabilities for large files,
only re-parsing changed portions instead of the entire file.

Features:
- Language detection from file extension
- Incremental parsing (edit-aware)
- Symbol extraction from syntax tree
- Integration with existing symbol cache

Supported languages:
- Python (.py)
- JavaScript (.js, .jsx, .mjs)
- TypeScript (.ts, .tsx)
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from pathlib import Path
from threading import RLock
from typing import Any

logger = logging.getLogger(__name__)

# Language extension mapping
LANGUAGE_EXTENSIONS: dict[str, str] = {
    ".py": "python",
    ".pyw": "python",
    ".js": "javascript",
    ".jsx": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".mts": "typescript",
    ".cts": "typescript",
}

# LSP Symbol Kinds (subset)
SYMBOL_KIND_MAP = {
    "module": 2,
    "class_definition": 5,
    "function_definition": 12,
    "method_definition": 6,
    "variable": 13,
    "constant": 14,
    "property": 7,
    "interface": 11,
    "enum": 10,
}


@dataclass
class ParsedSymbol:
    """Represents a symbol extracted from the syntax tree."""

    name: str
    kind: str
    line: int
    column: int
    end_line: int
    end_column: int
    children: list[ParsedSymbol] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to LSP-compatible dictionary."""
        result = {
            "name": self.name,
            "kind": SYMBOL_KIND_MAP.get(self.kind, 13),  # Default to variable
            "location": {
                "line": self.line,
                "column": self.column,
                "end_line": self.end_line,
                "end_column": self.end_column,
            },
        }
        if self.children:
            result["children"] = [c.to_dict() for c in self.children]
        return result


@dataclass
class ParseResult:
    """Result of parsing a file."""

    language: str
    symbols: list[ParsedSymbol]
    tree: Any | None = None  # Tree-sitter tree object
    errors: list[str] = field(default_factory=list)

    @property
    def has_errors(self) -> bool:
        """Check if parsing had errors."""
        return len(self.errors) > 0


class TreeSitterParser:
    """Incremental parser using Tree-sitter.

    Maintains parse trees for files and supports incremental updates
    when file content changes.
    """

    def __init__(self) -> None:
        self._parsers: dict[str, Any] = {}  # language -> Parser
        self._trees: dict[str, Any] = {}  # file_path -> Tree
        self._contents: dict[str, str] = {}  # file_path -> content
        self._lock = RLock()
        self._initialized = False
        self._available_languages: set[str] = set()

        # Try to initialize
        self._init_parsers()

    def _init_parsers(self) -> None:
        """Initialize tree-sitter parsers for each language."""
        try:
            import tree_sitter_javascript
            import tree_sitter_python
            import tree_sitter_typescript
            from tree_sitter import Language, Parser

            self._initialized = True

            # Python
            try:
                python_lang = Language(tree_sitter_python.language())
                parser = Parser(python_lang)
                self._parsers["python"] = parser
                self._available_languages.add("python")
                logger.debug("Tree-sitter Python parser initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize Python parser: {e}")

            # JavaScript
            try:
                js_lang = Language(tree_sitter_javascript.language())
                parser = Parser(js_lang)
                self._parsers["javascript"] = parser
                self._available_languages.add("javascript")
                logger.debug("Tree-sitter JavaScript parser initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize JavaScript parser: {e}")

            # TypeScript
            try:
                ts_lang = Language(tree_sitter_typescript.language_typescript())
                parser = Parser(ts_lang)
                self._parsers["typescript"] = parser
                self._available_languages.add("typescript")
                logger.debug("Tree-sitter TypeScript parser initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize TypeScript parser: {e}")

            logger.info(
                f"Tree-sitter initialized with languages: {self._available_languages}"
            )

        except ImportError as e:
            logger.warning(
                f"Tree-sitter not available: {e}. "
                "Install with: uv add tree-sitter tree-sitter-python "
                "tree-sitter-javascript tree-sitter-typescript --optional lsp"
            )
            self._initialized = False

    @property
    def is_available(self) -> bool:
        """Check if tree-sitter is available."""
        return self._initialized and len(self._available_languages) > 0

    def supports_language(self, language: str) -> bool:
        """Check if a language is supported."""
        return language in self._available_languages

    def detect_language(self, file_path: str | Path) -> str | None:
        """Detect language from file extension.

        Args:
            file_path: Path to the file

        Returns:
            Language name or None if not supported
        """
        path = Path(file_path)
        suffix = path.suffix.lower()
        return LANGUAGE_EXTENSIONS.get(suffix)

    def parse(
        self,
        file_path: str,
        content: str,
        *,
        incremental: bool = True,
    ) -> ParseResult:
        """Parse file content and extract symbols.

        Args:
            file_path: Path to the file (for language detection and caching)
            content: File content to parse
            incremental: Use incremental parsing if tree exists

        Returns:
            ParseResult with symbols and optional tree
        """
        language = self.detect_language(file_path)

        if not language or not self.supports_language(language):
            return ParseResult(
                language=language or "unknown",
                symbols=[],
                errors=[f"Language not supported: {language}"],
            )

        with self._lock:
            parser = self._parsers.get(language)
            if not parser:
                return ParseResult(
                    language=language,
                    symbols=[],
                    errors=[f"Parser not available for {language}"],
                )

            try:
                content_bytes = content.encode("utf-8")

                # Check for incremental parsing
                old_tree = None
                if incremental and file_path in self._trees:
                    old_content = self._contents.get(file_path, "")
                    if old_content and old_content != content:
                        old_tree = self._trees[file_path]
                        # Apply edit to old tree for incremental parse
                        # This is a simplified edit - full impl would track actual edits
                        old_tree.edit(
                            start_byte=0,
                            old_end_byte=len(old_content.encode("utf-8")),
                            new_end_byte=len(content_bytes),
                            start_point=(0, 0),
                            old_end_point=self._get_end_point(old_content),
                            new_end_point=self._get_end_point(content),
                        )

                # Parse (API changed: can't pass None as second argument)
                if old_tree is not None:
                    tree = parser.parse(content_bytes, old_tree)
                else:
                    tree = parser.parse(content_bytes)

                # Store for future incremental parsing
                self._trees[file_path] = tree
                self._contents[file_path] = content

                # Extract symbols
                symbols = self._extract_symbols(tree.root_node, language)

                return ParseResult(
                    language=language,
                    symbols=symbols,
                    tree=tree,
                )

            except Exception as e:
                logger.error(f"Parse error for {file_path}: {e}")
                return ParseResult(
                    language=language,
                    symbols=[],
                    errors=[str(e)],
                )

    def _get_end_point(self, content: str) -> tuple[int, int]:
        """Get end point (row, column) for content."""
        lines = content.split("\n")
        if not lines:
            return (0, 0)
        return (len(lines) - 1, len(lines[-1]))

    def _extract_symbols(
        self,
        node: Any,
        language: str,
        depth: int = 0,
    ) -> list[ParsedSymbol]:
        """Extract symbols from syntax tree node.

        Args:
            node: Tree-sitter node
            language: Language for symbol type mapping
            depth: Current depth (for limiting recursion)

        Returns:
            List of parsed symbols
        """
        if depth > 10:  # Limit recursion depth
            return []

        symbols: list[ParsedSymbol] = []

        # Symbol types to extract by language
        symbol_types = self._get_symbol_types(language)

        for child in node.children:
            if child.type in symbol_types:
                symbol = self._node_to_symbol(child, language)
                if symbol:
                    # Recursively extract children
                    symbol.children = self._extract_symbols(child, language, depth + 1)
                    symbols.append(symbol)
            else:
                # Continue searching in non-symbol nodes
                symbols.extend(self._extract_symbols(child, language, depth))

        return symbols

    def _get_symbol_types(self, language: str) -> set[str]:
        """Get symbol node types for a language."""
        if language == "python":
            return {
                "class_definition",
                "function_definition",
                "decorated_definition",
                "assignment",
            }
        elif language in ("javascript", "typescript"):
            return {
                "class_declaration",
                "function_declaration",
                "method_definition",
                "arrow_function",
                "variable_declaration",
                "lexical_declaration",
                "interface_declaration",
                "type_alias_declaration",
                "enum_declaration",
                "export_statement",
            }
        return set()

    def _node_to_symbol(self, node: Any, language: str) -> ParsedSymbol | None:
        """Convert tree-sitter node to ParsedSymbol.

        Args:
            node: Tree-sitter node
            language: Language for name extraction

        Returns:
            ParsedSymbol or None if name cannot be extracted
        """
        name = self._extract_name(node, language)
        if not name:
            return None

        # For export_statement, use the kind of the inner declaration
        kind = node.type
        if kind == "export_statement" and language in ("javascript", "typescript"):
            for child in node.children:
                if child.type in self._get_symbol_types(language):
                    kind = child.type
                    break

        return ParsedSymbol(
            name=name,
            kind=kind,
            line=node.start_point[0],
            column=node.start_point[1],
            end_line=node.end_point[0],
            end_column=node.end_point[1],
        )

    def _extract_name(self, node: Any, language: str) -> str | None:
        """Extract symbol name from node.

        Args:
            node: Tree-sitter node
            language: Language for name extraction rules

        Returns:
            Symbol name or None
        """
        # Handle decorated definitions (Python)
        if node.type == "decorated_definition":
            for child in node.children:
                if child.type in ("class_definition", "function_definition"):
                    return self._extract_name(child, language)
            return None

        # Python
        if language == "python":
            if node.type in ("class_definition", "function_definition"):
                for child in node.children:
                    if child.type == "identifier":
                        return child.text.decode("utf-8")
            elif node.type == "assignment":
                for child in node.children:
                    if child.type == "identifier":
                        return child.text.decode("utf-8")
                    if child.type == "expression_list":
                        # First identifier in expression list
                        for subchild in child.children:
                            if subchild.type == "identifier":
                                return subchild.text.decode("utf-8")
                        break

        # JavaScript/TypeScript
        elif language in ("javascript", "typescript"):
            name_nodes = ["identifier", "property_identifier", "type_identifier"]

            # Handle export_statement by extracting from declaration child
            if node.type == "export_statement":
                for child in node.children:
                    if child.type in self._get_symbol_types(language):
                        return self._extract_name(child, language)
                return None

            for child in node.children:
                if child.type in name_nodes:
                    return child.text.decode("utf-8")
                # Handle variable declarators
                if child.type == "variable_declarator":
                    for subchild in child.children:
                        if subchild.type in name_nodes:
                            return subchild.text.decode("utf-8")

        return None

    def invalidate(self, file_path: str) -> bool:
        """Invalidate cached tree for a file.

        Args:
            file_path: Path to the file

        Returns:
            True if cache entry was removed
        """
        with self._lock:
            removed = False
            if file_path in self._trees:
                del self._trees[file_path]
                removed = True
            if file_path in self._contents:
                del self._contents[file_path]
            return removed

    def clear(self) -> int:
        """Clear all cached trees.

        Returns:
            Number of entries cleared
        """
        with self._lock:
            count = len(self._trees)
            self._trees.clear()
            self._contents.clear()
            return count

    def get_status(self) -> dict[str, Any]:
        """Get parser status for monitoring."""
        with self._lock:
            return {
                "initialized": self._initialized,
                "available_languages": list(self._available_languages),
                "cached_files": len(self._trees),
            }


# Global parser instance
_global_parser: TreeSitterParser | None = None
_parser_lock = RLock()


def get_tree_sitter_parser() -> TreeSitterParser:
    """Get or create the global tree-sitter parser.

    Returns:
        Global TreeSitterParser instance
    """
    global _global_parser

    with _parser_lock:
        if _global_parser is None:
            _global_parser = TreeSitterParser()

    return _global_parser


def reset_global_parser() -> None:
    """Reset the global parser instance (for testing)."""
    global _global_parser

    with _parser_lock:
        if _global_parser is not None:
            _global_parser.clear()
        _global_parser = None
