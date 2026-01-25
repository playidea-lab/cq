"""TypeScript/JavaScript parser for code symbol extraction.

Uses regex-based parsing for basic symbol extraction.
For more advanced use cases, consider using tree-sitter-typescript.
"""

from __future__ import annotations

import re
from pathlib import Path

from .models import (
    SymbolInfo,
    SymbolKind,
    SymbolLocation,
    SymbolTable,
)


class TypeScriptParser:
    """Parse TypeScript/JavaScript source code and extract symbols.

    This is a lightweight regex-based parser for common patterns.
    For production use with complex codebases, consider tree-sitter.
    """

    # Regex patterns for TypeScript/JavaScript constructs
    PATTERNS = {
        # Classes: class Name extends Base implements Interface
        "class": re.compile(
            r"^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)"
            r"(?:\s+extends\s+[\w.<>,\s]+)?"
            r"(?:\s+implements\s+[\w.<>,\s]+)?\s*\{",
            re.MULTILINE,
        ),
        # Interfaces: interface Name extends Base
        "interface": re.compile(
            r"^(?:export\s+)?interface\s+(\w+)"
            r"(?:\s*<[^>]+>)?"
            r"(?:\s+extends\s+[\w.<>,\s]+)?\s*\{",
            re.MULTILINE,
        ),
        # Type aliases: type Name = ...
        "type_alias": re.compile(
            r"^(?:export\s+)?type\s+(\w+)(?:\s*<[^>]+>)?\s*=",
            re.MULTILINE,
        ),
        # Enums: enum Name
        "enum": re.compile(
            r"^(?:export\s+)?(?:const\s+)?enum\s+(\w+)\s*\{",
            re.MULTILINE,
        ),
        # Functions: function name() or async function name()
        "function": re.compile(
            r"^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*"
            r"(?:<[^>]+>)?\s*\([^)]*\)",
            re.MULTILINE,
        ),
        # Arrow functions: const name = () => or const name = async () =>
        "arrow_function": re.compile(
            r"^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*"
            r"(?::\s*[^=]+)?\s*=\s*(?:async\s+)?\([^)]*\)\s*(?::\s*[^=]+)?\s*=>",
            re.MULTILINE,
        ),
        # Constants: const NAME = or export const NAME =
        "constant": re.compile(
            r"^(?:export\s+)?const\s+([A-Z][A-Z0-9_]*)\s*(?::[^=]+)?\s*=",
            re.MULTILINE,
        ),
        # Variables: const/let/var name =
        "variable": re.compile(
            r"^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::[^=]+)?\s*=",
            re.MULTILINE,
        ),
        # Methods in class body: methodName() or async methodName()
        "method": re.compile(
            r"^\s+(?:public\s+|private\s+|protected\s+)?(?:static\s+)?"
            r"(?:async\s+)?(?:get\s+|set\s+)?(\w+)\s*"
            r"(?:<[^>]+>)?\s*\([^)]*\)\s*(?::\s*[^{]+)?\s*\{",
            re.MULTILINE,
        ),
        # Imports (including `import type`)
        "import": re.compile(
            r"^import\s+(?:type\s+)?(?:(?:\{[^}]+\}|\*\s+as\s+\w+|\w+)"
            r"(?:\s*,\s*(?:\{[^}]+\}|\*\s+as\s+\w+))?\s+from\s+)?['\"]([^'\"]+)['\"]",
            re.MULTILINE,
        ),
        # Exports
        "export": re.compile(
            r"^export\s+(?:default\s+)?(?:class|interface|type|enum|function|const|let|var)\s+(\w+)",
            re.MULTILINE,
        ),
    }

    # JSDoc comment pattern
    JSDOC_PATTERN = re.compile(
        r"/\*\*\s*([\s\S]*?)\*/\s*(?=(?:export\s+)?(?:class|interface|type|enum|function|const|async))",
        re.MULTILINE,
    )

    def __init__(self) -> None:
        """Initialize the parser."""
        self._current_file: str = ""

    def parse_file(self, file_path: str | Path) -> SymbolTable:
        """Parse a TypeScript/JavaScript file and return its symbol table.

        Args:
            file_path: Path to the file.

        Returns:
            SymbolTable with all symbols found.
        """
        file_path = Path(file_path)
        self._current_file = str(file_path)

        # Determine language from extension
        ext = file_path.suffix.lower()
        if ext in (".ts", ".tsx"):
            language = "typescript"
        elif ext in (".js", ".jsx", ".mjs", ".cjs"):
            language = "javascript"
        else:
            language = "typescript"  # Default

        table = SymbolTable(
            file_path=str(file_path),
            language=language,
        )

        try:
            source = file_path.read_text(encoding="utf-8")
            self._parse_source_impl(source, table)
        except Exception as e:
            table.errors.append(f"Parse error: {e}")

        return table

    def parse_source(
        self,
        source: str,
        filename: str = "<string>",
        language: str = "typescript",
    ) -> SymbolTable:
        """Parse TypeScript/JavaScript source code string.

        Args:
            source: Source code.
            filename: Virtual filename.
            language: "typescript" or "javascript".

        Returns:
            SymbolTable with all symbols found.
        """
        self._current_file = filename

        table = SymbolTable(
            file_path=filename,
            language=language,
        )

        try:
            self._parse_source_impl(source, table)
        except Exception as e:
            table.errors.append(f"Parse error: {e}")

        return table

    def _parse_source_impl(self, source: str, table: SymbolTable) -> None:
        """Internal implementation of source parsing."""
        lines = source.split("\n")

        # Extract JSDoc comments for documentation
        jsdocs = self._extract_jsdocs(source)

        # Parse classes
        for match in self.PATTERNS["class"].finditer(source):
            name = match.group(1)
            line = source[: match.start()].count("\n") + 1
            doc = jsdocs.get(line - 1)  # JSDoc is on previous line

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.CLASS,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=match.start() - source.rfind("\n", 0, match.start()) - 1,
                ),
                doc=doc,
                signature=self._extract_line(lines, line - 1),
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)

        # Parse interfaces
        for match in self.PATTERNS["interface"].finditer(source):
            name = match.group(1)
            line = source[: match.start()].count("\n") + 1
            doc = jsdocs.get(line - 1)

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.INTERFACE,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                doc=doc,
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)

        # Parse type aliases
        for match in self.PATTERNS["type_alias"].finditer(source):
            name = match.group(1)
            line = source[: match.start()].count("\n") + 1

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.TYPE_ALIAS,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)

        # Parse enums
        for match in self.PATTERNS["enum"].finditer(source):
            name = match.group(1)
            line = source[: match.start()].count("\n") + 1

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.ENUM,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)

        # Parse functions
        for match in self.PATTERNS["function"].finditer(source):
            name = match.group(1)
            line = source[: match.start()].count("\n") + 1
            doc = jsdocs.get(line - 1)
            is_async = "async" in match.group(0)

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.FUNCTION,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                doc=doc,
                signature=self._extract_line(lines, line - 1),
                is_async=is_async,
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)

        # Parse arrow functions (only if not already matched as constant)
        matched_names = {s.name for s in table.symbols.values()}
        for match in self.PATTERNS["arrow_function"].finditer(source):
            name = match.group(1)
            if name in matched_names:
                continue
            if name.isupper():  # Skip if it's a constant
                continue

            line = source[: match.start()].count("\n") + 1
            is_async = "async" in match.group(0)

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.FUNCTION,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                signature=self._extract_line(lines, line - 1),
                is_async=is_async,
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)
            matched_names.add(name)

        # Parse constants
        for match in self.PATTERNS["constant"].finditer(source):
            name = match.group(1)
            if name in matched_names:
                continue

            line = source[: match.start()].count("\n") + 1

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.CONSTANT,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=line,
                    start_col=0,
                ),
                is_exported="export" in match.group(0),
            )
            table.add_symbol(symbol)
            matched_names.add(name)

        # Parse imports
        for match in self.PATTERNS["import"].finditer(source):
            module = match.group(1)
            table.imports.append(module)

        # Parse exports for export list
        for match in self.PATTERNS["export"].finditer(source):
            name = match.group(1)
            if name and name not in table.exports:
                table.exports.append(name)

    def _extract_jsdocs(self, source: str) -> dict[int, str]:
        """Extract JSDoc comments and their line numbers.

        Returns:
            Dict mapping line number (of symbol) to JSDoc content.
        """
        jsdocs: dict[int, str] = {}

        for match in self.JSDOC_PATTERN.finditer(source):
            content = match.group(1).strip()
            # Clean up JSDoc asterisks
            lines = []
            for line in content.split("\n"):
                line = line.strip()
                if line.startswith("*"):
                    line = line[1:].strip()
                lines.append(line)
            doc = " ".join(lines).strip()

            # Find the line number after the comment
            end_pos = match.end()
            line = source[:end_pos].count("\n") + 1
            jsdocs[line] = doc

        return jsdocs

    def _extract_line(self, lines: list[str], index: int) -> str:
        """Extract a line by index, handling bounds."""
        if 0 <= index < len(lines):
            return lines[index].strip()
        return ""
