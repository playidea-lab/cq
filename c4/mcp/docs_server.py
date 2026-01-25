"""MCP Documentation Tools (Context7-like).

Provides documentation generation and querying tools:
- query_docs: Search documentation by keyword/topic
- get_api_reference: Get detailed API documentation for a symbol
- search_examples: Find usage examples in codebase
- get_changelog: Get change history for a file or symbol
- create_snapshot: Create versioned documentation snapshot
- list_snapshots: List all documentation snapshots
- compare_snapshots: Compare two documentation versions
- get_snapshot: Get documentation for a specific version
"""

from __future__ import annotations

import hashlib
import json
import shutil
import subprocess
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any

from c4.docs.analyzer import CodeAnalyzer, Symbol, SymbolKind


class DocFormat(Enum):
    """Documentation output format."""

    MARKDOWN = "markdown"
    JSON = "json"
    HTML = "html"


@dataclass
class DocumentationEntry:
    """A documentation entry for a symbol."""

    name: str
    kind: SymbolKind
    qualified_name: str
    file_path: str
    line_number: int
    signature: str | None
    docstring: str | None
    parent: str | None = None
    children: list[str] = field(default_factory=list)
    references_count: int = 0
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "kind": self.kind.value,
            "qualified_name": self.qualified_name,
            "file_path": self.file_path,
            "line_number": self.line_number,
            "signature": self.signature,
            "docstring": self.docstring,
            "parent": self.parent,
            "children": self.children,
            "references_count": self.references_count,
            "metadata": self.metadata,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []

        # Header
        kind_emoji = {
            SymbolKind.CLASS: "📦",
            SymbolKind.FUNCTION: "⚙️",
            SymbolKind.METHOD: "🔧",
            SymbolKind.VARIABLE: "📝",
            SymbolKind.CONSTANT: "🔒",
            SymbolKind.INTERFACE: "📋",
            SymbolKind.TYPE_ALIAS: "🏷️",
            SymbolKind.ENUM: "📊",
        }.get(self.kind, "📄")

        lines.append(f"## {kind_emoji} {self.qualified_name}")
        lines.append("")

        # Location
        lines.append(f"**Location:** `{self.file_path}:{self.line_number}`")
        lines.append("")

        # Signature
        if self.signature:
            lines.append("```python")
            lines.append(self.signature)
            lines.append("```")
            lines.append("")

        # Docstring
        if self.docstring:
            lines.append("### Description")
            lines.append(self.docstring)
            lines.append("")

        # Children
        if self.children:
            lines.append("### Members")
            for child in self.children:
                lines.append(f"- `{child}`")
            lines.append("")

        # References
        if self.references_count > 0:
            lines.append(f"**References:** {self.references_count} usages found")
            lines.append("")

        return "\n".join(lines)


@dataclass
class ExampleEntry:
    """A usage example from the codebase."""

    symbol_name: str
    file_path: str
    line_number: int
    context: str
    usage_type: str = "usage"  # "usage", "import", "definition"
    surrounding_lines: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "symbol_name": self.symbol_name,
            "file_path": self.file_path,
            "line_number": self.line_number,
            "context": self.context,
            "usage_type": self.usage_type,
            "surrounding_lines": self.surrounding_lines,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"### {self.file_path}:{self.line_number}")
        lines.append("")
        lines.append("```python")
        if self.surrounding_lines:
            for line in self.surrounding_lines:
                lines.append(line)
        else:
            lines.append(self.context)
        lines.append("```")
        lines.append("")
        return "\n".join(lines)


@dataclass
class ChangelogEntry:
    """A changelog entry from git history."""

    commit_hash: str
    author: str
    date: str
    message: str
    file_path: str | None = None
    additions: int = 0
    deletions: int = 0
    diff_snippet: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "commit_hash": self.commit_hash,
            "author": self.author,
            "date": self.date,
            "message": self.message,
            "file_path": self.file_path,
            "additions": self.additions,
            "deletions": self.deletions,
            "diff_snippet": self.diff_snippet,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"### {self.commit_hash[:7]} - {self.date}")
        lines.append(f"**Author:** {self.author}")
        lines.append(f"**Message:** {self.message}")
        if self.file_path:
            lines.append(f"**File:** {self.file_path}")
        if self.additions or self.deletions:
            lines.append(f"**Changes:** +{self.additions} -{self.deletions}")
        if self.diff_snippet:
            lines.append("")
            lines.append("```diff")
            lines.append(self.diff_snippet)
            lines.append("```")
        lines.append("")
        return "\n".join(lines)


@dataclass
class DocSnapshot:
    """A versioned documentation snapshot."""

    version: str
    created_at: str
    commit_hash: str | None
    description: str | None
    files_count: int
    symbols_count: int
    content_hash: str
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "version": self.version,
            "created_at": self.created_at,
            "commit_hash": self.commit_hash,
            "description": self.description,
            "files_count": self.files_count,
            "symbols_count": self.symbols_count,
            "content_hash": self.content_hash,
            "metadata": self.metadata,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"## 📸 Snapshot: {self.version}")
        lines.append("")
        lines.append(f"**Created:** {self.created_at}")
        if self.commit_hash:
            lines.append(f"**Commit:** {self.commit_hash[:7]}")
        if self.description:
            lines.append(f"**Description:** {self.description}")
        lines.append(f"**Files:** {self.files_count}")
        lines.append(f"**Symbols:** {self.symbols_count}")
        lines.append(f"**Hash:** {self.content_hash[:12]}...")
        lines.append("")
        return "\n".join(lines)


@dataclass
class SnapshotDiff:
    """Diff between two documentation snapshots."""

    from_version: str
    to_version: str
    added_symbols: list[str]
    removed_symbols: list[str]
    modified_symbols: list[str]
    added_files: list[str]
    removed_files: list[str]
    summary: str

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "from_version": self.from_version,
            "to_version": self.to_version,
            "added_symbols": self.added_symbols,
            "removed_symbols": self.removed_symbols,
            "modified_symbols": self.modified_symbols,
            "added_files": self.added_files,
            "removed_files": self.removed_files,
            "summary": self.summary,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"# Documentation Diff: {self.from_version} → {self.to_version}")
        lines.append("")
        lines.append(f"**Summary:** {self.summary}")
        lines.append("")

        if self.added_symbols:
            lines.append("## ➕ Added Symbols")
            for sym in self.added_symbols:
                lines.append(f"- `{sym}`")
            lines.append("")

        if self.removed_symbols:
            lines.append("## ➖ Removed Symbols")
            for sym in self.removed_symbols:
                lines.append(f"- `{sym}`")
            lines.append("")

        if self.modified_symbols:
            lines.append("## 🔄 Modified Symbols")
            for sym in self.modified_symbols:
                lines.append(f"- `{sym}`")
            lines.append("")

        if self.added_files:
            lines.append("## 📄 Added Files")
            for f in self.added_files:
                lines.append(f"- `{f}`")
            lines.append("")

        if self.removed_files:
            lines.append("## 🗑️ Removed Files")
            for f in self.removed_files:
                lines.append(f"- `{f}`")
            lines.append("")

        return "\n".join(lines)


@dataclass
class QueryResult:
    """Result of a documentation query."""

    query: str
    total_results: int
    entries: list[DocumentationEntry]
    query_time_ms: float = 0.0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "query": self.query,
            "total_results": self.total_results,
            "entries": [e.to_dict() for e in self.entries],
            "query_time_ms": self.query_time_ms,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"# Documentation Search: `{self.query}`")
        lines.append("")
        lines.append(f"Found **{self.total_results}** results")
        lines.append("")
        for entry in self.entries:
            lines.append(entry.to_markdown())
        return "\n".join(lines)


class DocGenerator:
    """Documentation generator using CodeAnalyzer.

    Provides Context7-like documentation querying:
    - query_docs(): Search documentation by keyword
    - get_api_reference(): Get detailed API docs for a symbol
    - search_examples(): Find usage examples
    - get_changelog(): Get change history

    Example:
        generator = DocGenerator(project_root=".")
        generator.index_codebase()

        # Search for documentation
        result = generator.query_docs("UserService")

        # Get API reference
        api_doc = generator.get_api_reference("UserService")

        # Find examples
        examples = generator.search_examples("UserService")

        # Get changelog
        changes = generator.get_changelog("src/services/user.py")
    """

    def __init__(
        self,
        project_root: str | Path = ".",
        include_patterns: list[str] | None = None,
        exclude_patterns: list[str] | None = None,
    ) -> None:
        """Initialize the documentation generator.

        Args:
            project_root: Root directory of the project
            include_patterns: Glob patterns for files to include
            exclude_patterns: Glob patterns for files to exclude
        """
        self.project_root = Path(project_root).resolve()
        self.include_patterns = include_patterns or ["**/*.py", "**/*.ts", "**/*.tsx"]
        self.exclude_patterns = exclude_patterns or [
            "**/node_modules/**",
            "**/__pycache__/**",
            "**/.git/**",
            "**/venv/**",
            "**/.venv/**",
            "**/dist/**",
            "**/build/**",
        ]
        self._analyzer = CodeAnalyzer()
        self._indexed = False
        self._documentation_cache: dict[str, DocumentationEntry] = {}
        self._snapshots_dir = self.project_root / ".c4" / "docs" / "snapshots"

    def index_codebase(self) -> int:
        """Index the entire codebase for documentation.

        Returns:
            Number of files indexed
        """
        count = self._analyzer.add_directory(
            self.project_root,
            recursive=True,
            exclude_patterns=self.exclude_patterns,
        )
        self._build_documentation_cache()
        self._indexed = True
        return count

    def index_file(self, file_path: str | Path) -> None:
        """Index a single file.

        Args:
            file_path: Path to the file to index
        """
        self._analyzer.add_file(file_path)
        self._build_documentation_cache()
        self._indexed = True

    def _build_documentation_cache(self) -> None:
        """Build documentation cache from analyzed symbols."""
        self._documentation_cache.clear()

        for symbol in self._analyzer.get_all_symbols():
            # Get references count
            refs = self._analyzer.find_references(symbol.name)
            refs_count = len([r for r in refs if r.ref_kind == "usage"])

            # Get children names
            children_names = [c.name for c in symbol.children]

            entry = DocumentationEntry(
                name=symbol.name,
                kind=symbol.kind,
                qualified_name=symbol.qualified_name,
                file_path=symbol.location.file_path,
                line_number=symbol.location.start_line,
                signature=symbol.signature,
                docstring=symbol.docstring,
                parent=symbol.parent,
                children=children_names,
                references_count=refs_count,
                metadata=symbol.metadata,
            )

            # Cache by both name and qualified name
            self._documentation_cache[symbol.name] = entry
            if symbol.parent:
                self._documentation_cache[symbol.qualified_name] = entry

    def query_docs(
        self,
        query: str,
        kind: SymbolKind | None = None,
        limit: int = 20,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Search documentation by keyword/topic.

        Args:
            query: Search query (keyword, symbol name, topic)
            kind: Optional filter by symbol kind
            limit: Maximum number of results
            format: Output format (markdown, json)

        Returns:
            Documentation search results in specified format
        """
        import time
        start_time = time.time()

        # Search for matching symbols
        symbols = self._analyzer.find_symbol(query, kind=kind, exact_match=False)
        symbols = symbols[:limit]

        # Build documentation entries
        entries = []
        for symbol in symbols:
            refs = self._analyzer.find_references(symbol.name)
            refs_count = len([r for r in refs if r.ref_kind == "usage"])

            entry = DocumentationEntry(
                name=symbol.name,
                kind=symbol.kind,
                qualified_name=symbol.qualified_name,
                file_path=symbol.location.file_path,
                line_number=symbol.location.start_line,
                signature=symbol.signature,
                docstring=symbol.docstring,
                parent=symbol.parent,
                children=[c.name for c in symbol.children],
                references_count=refs_count,
            )
            entries.append(entry)

        elapsed_ms = (time.time() - start_time) * 1000

        result = QueryResult(
            query=query,
            total_results=len(entries),
            entries=entries,
            query_time_ms=round(elapsed_ms, 2),
        )

        if format == DocFormat.JSON:
            return result.to_dict()
        return result.to_markdown()

    def get_api_reference(
        self,
        symbol_name: str,
        include_children: bool = True,
        include_references: bool = False,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Get detailed API documentation for a symbol.

        Args:
            symbol_name: Name of the symbol (class, function, etc.)
            include_children: Whether to include child symbols
            include_references: Whether to include usage references
            format: Output format (markdown, json)

        Returns:
            Detailed API documentation in specified format
        """
        # Find the symbol
        symbols = self._analyzer.find_symbol(symbol_name, exact_match=True)
        if not symbols:
            # Try partial match
            symbols = self._analyzer.find_symbol(symbol_name, exact_match=False)

        if not symbols:
            return {"error": f"Symbol '{symbol_name}' not found"} if format == DocFormat.JSON else f"Symbol '{symbol_name}' not found"

        # Use the first match
        symbol = symbols[0]

        # Get references
        refs = self._analyzer.find_references(symbol.name)
        refs_count = len(refs)

        # Build documentation entry
        entry = DocumentationEntry(
            name=symbol.name,
            kind=symbol.kind,
            qualified_name=symbol.qualified_name,
            file_path=symbol.location.file_path,
            line_number=symbol.location.start_line,
            signature=symbol.signature,
            docstring=symbol.docstring,
            parent=symbol.parent,
            children=[c.name for c in symbol.children] if include_children else [],
            references_count=refs_count,
            metadata=symbol.metadata,
        )

        result: dict[str, Any] = entry.to_dict()

        # Add references if requested
        if include_references:
            result["references"] = [
                {
                    "file": ref.location.file_path,
                    "line": ref.location.start_line,
                    "context": ref.context,
                }
                for ref in refs[:20]  # Limit references
            ]

        if format == DocFormat.JSON:
            return result

        # Build markdown
        lines = [entry.to_markdown()]

        if include_references and refs:
            lines.append("### References")
            lines.append("")
            for ref in refs[:10]:
                lines.append(f"- `{ref.location.file_path}:{ref.location.start_line}`: {ref.context}")
            if len(refs) > 10:
                lines.append(f"- ... and {len(refs) - 10} more")
            lines.append("")

        return "\n".join(lines)

    def search_examples(
        self,
        symbol_name: str,
        limit: int = 10,
        context_lines: int = 3,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Find usage examples in the codebase.

        Args:
            symbol_name: Name of the symbol to find examples for
            limit: Maximum number of examples
            context_lines: Number of surrounding lines to include
            format: Output format (markdown, json)

        Returns:
            Usage examples in specified format
        """
        refs = self._analyzer.find_references(symbol_name)

        # Filter to usage refs (not definitions)
        usage_refs = [r for r in refs if r.ref_kind == "usage"][:limit]

        examples = []
        for ref in usage_refs:
            # Get surrounding lines
            surrounding = self._get_surrounding_lines(
                ref.location.file_path,
                ref.location.start_line,
                context_lines,
            )

            example = ExampleEntry(
                symbol_name=symbol_name,
                file_path=ref.location.file_path,
                line_number=ref.location.start_line,
                context=ref.context,
                usage_type=ref.ref_kind,
                surrounding_lines=surrounding,
            )
            examples.append(example)

        if format == DocFormat.JSON:
            return {
                "symbol": symbol_name,
                "total_examples": len(examples),
                "examples": [e.to_dict() for e in examples],
            }

        # Build markdown
        lines = []
        lines.append(f"# Usage Examples: `{symbol_name}`")
        lines.append("")
        lines.append(f"Found **{len(examples)}** examples")
        lines.append("")
        for example in examples:
            lines.append(example.to_markdown())

        return "\n".join(lines)

    def _get_surrounding_lines(
        self,
        file_path: str,
        line_number: int,
        context: int,
    ) -> list[str]:
        """Get surrounding lines from a file.

        Args:
            file_path: Path to the file
            line_number: Target line number (1-indexed)
            context: Number of lines before and after

        Returns:
            List of surrounding lines with line numbers
        """
        try:
            path = Path(file_path)
            if not path.is_absolute():
                path = self.project_root / path

            if not path.exists():
                return []

            lines = path.read_text(encoding="utf-8").split("\n")

            start = max(0, line_number - context - 1)
            end = min(len(lines), line_number + context)

            result = []
            for i in range(start, end):
                prefix = ">>>" if i == line_number - 1 else "   "
                result.append(f"{prefix} {i + 1}: {lines[i]}")

            return result
        except Exception:
            return []

    def get_changelog(
        self,
        file_path: str | None = None,
        symbol_name: str | None = None,
        limit: int = 10,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Get change history for a file or symbol.

        Args:
            file_path: Path to file (relative to project root)
            symbol_name: Name of symbol to track changes for
            limit: Maximum number of changelog entries
            format: Output format (markdown, json)

        Returns:
            Change history in specified format
        """
        if symbol_name and not file_path:
            # Find file containing the symbol
            symbols = self._analyzer.find_symbol(symbol_name, exact_match=True)
            if symbols:
                file_path = symbols[0].location.file_path

        if not file_path:
            return {"error": "No file or symbol specified"} if format == DocFormat.JSON else "No file or symbol specified"

        # Get git log for the file
        entries = self._get_git_log(file_path, limit)

        if format == DocFormat.JSON:
            return {
                "file_path": file_path,
                "symbol_name": symbol_name,
                "total_entries": len(entries),
                "entries": [e.to_dict() for e in entries],
            }

        # Build markdown
        lines = []
        target = symbol_name or file_path
        lines.append(f"# Changelog: `{target}`")
        lines.append("")
        lines.append(f"Showing **{len(entries)}** recent changes")
        lines.append("")
        for entry in entries:
            lines.append(entry.to_markdown())

        return "\n".join(lines)

    def _get_git_log(self, file_path: str, limit: int) -> list[ChangelogEntry]:
        """Get git log for a file.

        Args:
            file_path: Path to the file
            limit: Maximum number of entries

        Returns:
            List of changelog entries
        """
        try:
            # Run git log
            result = subprocess.run(
                [
                    "git", "log",
                    f"-{limit}",
                    "--format=%H|%an|%ai|%s",
                    "--numstat",
                    "--",
                    file_path,
                ],
                cwd=self.project_root,
                capture_output=True,
                text=True,
                timeout=10,
            )

            if result.returncode != 0:
                return []

            entries = []
            lines = result.stdout.strip().split("\n")

            i = 0
            while i < len(lines):
                line = lines[i].strip()
                if not line:
                    i += 1
                    continue

                # Parse commit line
                if "|" in line:
                    parts = line.split("|", 3)
                    if len(parts) >= 4:
                        commit_hash, author, date, message = parts

                        # Parse stats (next line if available)
                        additions = 0
                        deletions = 0
                        if i + 1 < len(lines):
                            stat_line = lines[i + 1].strip()
                            if stat_line and "\t" in stat_line:
                                stat_parts = stat_line.split("\t")
                                if len(stat_parts) >= 2:
                                    try:
                                        additions = int(stat_parts[0]) if stat_parts[0] != "-" else 0
                                        deletions = int(stat_parts[1]) if stat_parts[1] != "-" else 0
                                    except ValueError:
                                        pass
                                i += 1

                        entries.append(ChangelogEntry(
                            commit_hash=commit_hash,
                            author=author,
                            date=date.split(" ")[0],  # Just the date part
                            message=message,
                            file_path=file_path,
                            additions=additions,
                            deletions=deletions,
                        ))

                i += 1

            return entries
        except Exception:
            return []

    def generate_full_docs(
        self,
        output_dir: str | Path | None = None,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str:
        """Generate full documentation for the codebase.

        Args:
            output_dir: Directory to write documentation files
            format: Output format (markdown, json)

        Returns:
            Path to generated documentation or the documentation content
        """
        if not self._indexed:
            self.index_codebase()

        # Group symbols by file
        symbols_by_file: dict[str, list[Symbol]] = {}
        for symbol in self._analyzer.get_all_symbols():
            file_path = symbol.location.file_path
            if file_path not in symbols_by_file:
                symbols_by_file[file_path] = []
            symbols_by_file[file_path].append(symbol)

        lines = []
        lines.append("# API Documentation")
        lines.append("")
        lines.append(f"Generated at: {datetime.now(timezone.utc).isoformat()}")
        lines.append("")

        for file_path in sorted(symbols_by_file.keys()):
            symbols = symbols_by_file[file_path]

            lines.append(f"## {file_path}")
            lines.append("")

            # Group by kind
            classes = [s for s in symbols if s.kind == SymbolKind.CLASS]
            functions = [s for s in symbols if s.kind == SymbolKind.FUNCTION]

            if classes:
                lines.append("### Classes")
                lines.append("")
                for cls in classes:
                    lines.append(f"#### {cls.name}")
                    if cls.docstring:
                        lines.append(cls.docstring)
                    if cls.children:
                        lines.append("")
                        lines.append("**Methods:**")
                        for child in cls.children:
                            lines.append(f"- `{child.name}`")
                    lines.append("")

            if functions:
                lines.append("### Functions")
                lines.append("")
                for func in functions:
                    if func.signature:
                        lines.append(f"#### `{func.signature}`")
                    else:
                        lines.append(f"#### {func.name}")
                    if func.docstring:
                        lines.append(func.docstring)
                    lines.append("")

        content = "\n".join(lines)

        if output_dir:
            output_path = Path(output_dir)
            output_path.mkdir(parents=True, exist_ok=True)

            if format == DocFormat.MARKDOWN:
                doc_file = output_path / "API.md"
                doc_file.write_text(content, encoding="utf-8")
                return str(doc_file)
            elif format == DocFormat.JSON:
                doc_file = output_path / "API.json"
                # Convert to JSON structure
                data = {
                    "generated_at": datetime.now(timezone.utc).isoformat(),
                    "files": {},
                }
                for file_path, symbols in symbols_by_file.items():
                    data["files"][file_path] = [
                        {
                            "name": s.name,
                            "kind": s.kind.value,
                            "signature": s.signature,
                            "docstring": s.docstring,
                            "line": s.location.start_line,
                        }
                        for s in symbols
                    ]
                doc_file.write_text(json.dumps(data, indent=2), encoding="utf-8")
                return str(doc_file)

        return content

    def create_snapshot(
        self,
        version: str,
        description: str | None = None,
    ) -> DocSnapshot:
        """Create a versioned documentation snapshot.

        Args:
            version: Version identifier (e.g., "v1.0.0", "2024-01-15")
            description: Optional description of the snapshot

        Returns:
            DocSnapshot with metadata about the created snapshot
        """
        if not self._indexed:
            self.index_codebase()

        # Create snapshots directory
        self._snapshots_dir.mkdir(parents=True, exist_ok=True)
        version_dir = self._snapshots_dir / version

        if version_dir.exists():
            raise ValueError(f"Snapshot version '{version}' already exists")

        version_dir.mkdir(parents=True, exist_ok=True)

        # Get current git commit
        commit_hash = self._get_current_commit()

        # Generate documentation data
        symbols_by_file: dict[str, list[dict[str, Any]]] = {}
        all_symbols = self._analyzer.get_all_symbols()

        for symbol in all_symbols:
            file_path = symbol.location.file_path
            if file_path not in symbols_by_file:
                symbols_by_file[file_path] = []
            symbols_by_file[file_path].append({
                "name": symbol.name,
                "kind": symbol.kind.value,
                "qualified_name": symbol.qualified_name,
                "signature": symbol.signature,
                "docstring": symbol.docstring,
                "line": symbol.location.start_line,
                "parent": symbol.parent,
                "children": [c.name for c in symbol.children],
            })

        # Calculate content hash
        content_str = json.dumps(symbols_by_file, sort_keys=True)
        content_hash = hashlib.sha256(content_str.encode()).hexdigest()

        # Save documentation data
        data = {
            "version": version,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "commit_hash": commit_hash,
            "description": description,
            "files": symbols_by_file,
        }
        (version_dir / "docs.json").write_text(
            json.dumps(data, indent=2, ensure_ascii=False),
            encoding="utf-8",
        )

        # Generate markdown documentation
        md_content = self.generate_full_docs(format=DocFormat.MARKDOWN)
        (version_dir / "API.md").write_text(md_content, encoding="utf-8")

        # Generate HTML documentation
        html_content = self._generate_html_docs(symbols_by_file, version, description)
        (version_dir / "index.html").write_text(html_content, encoding="utf-8")

        # Save metadata
        snapshot = DocSnapshot(
            version=version,
            created_at=datetime.now(timezone.utc).isoformat(),
            commit_hash=commit_hash,
            description=description,
            files_count=len(symbols_by_file),
            symbols_count=len(all_symbols),
            content_hash=content_hash,
        )
        (version_dir / "metadata.json").write_text(
            json.dumps(snapshot.to_dict(), indent=2),
            encoding="utf-8",
        )

        return snapshot

    def list_snapshots(self) -> list[DocSnapshot]:
        """List all documentation snapshots.

        Returns:
            List of DocSnapshot objects sorted by creation date (newest first)
        """
        snapshots = []

        if not self._snapshots_dir.exists():
            return snapshots

        for version_dir in self._snapshots_dir.iterdir():
            if not version_dir.is_dir():
                continue

            metadata_file = version_dir / "metadata.json"
            if not metadata_file.exists():
                continue

            try:
                data = json.loads(metadata_file.read_text(encoding="utf-8"))
                snapshots.append(DocSnapshot(
                    version=data.get("version", version_dir.name),
                    created_at=data.get("created_at", ""),
                    commit_hash=data.get("commit_hash"),
                    description=data.get("description"),
                    files_count=data.get("files_count", 0),
                    symbols_count=data.get("symbols_count", 0),
                    content_hash=data.get("content_hash", ""),
                    metadata=data.get("metadata", {}),
                ))
            except (json.JSONDecodeError, KeyError):
                continue

        # Sort by creation date (newest first)
        snapshots.sort(key=lambda s: s.created_at, reverse=True)
        return snapshots

    def get_snapshot(
        self,
        version: str,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Get documentation for a specific snapshot version.

        Args:
            version: Version identifier
            format: Output format (markdown, json, html)

        Returns:
            Documentation content in specified format
        """
        version_dir = self._snapshots_dir / version

        if not version_dir.exists():
            return {"error": f"Snapshot '{version}' not found"}

        if format == DocFormat.HTML:
            html_file = version_dir / "index.html"
            if html_file.exists():
                return html_file.read_text(encoding="utf-8")
            return {"error": "HTML documentation not found"}

        if format == DocFormat.MARKDOWN:
            md_file = version_dir / "API.md"
            if md_file.exists():
                return md_file.read_text(encoding="utf-8")
            return f"Markdown documentation not found for version '{version}'"

        # JSON format
        docs_file = version_dir / "docs.json"
        if docs_file.exists():
            return json.loads(docs_file.read_text(encoding="utf-8"))
        return {"error": "Documentation data not found"}

    def compare_snapshots(
        self,
        from_version: str,
        to_version: str,
        format: DocFormat = DocFormat.MARKDOWN,
    ) -> str | dict[str, Any]:
        """Compare two documentation snapshots.

        Args:
            from_version: Base version to compare from
            to_version: Target version to compare to
            format: Output format (markdown or json)

        Returns:
            Diff between the two versions
        """
        from_dir = self._snapshots_dir / from_version
        to_dir = self._snapshots_dir / to_version

        if not from_dir.exists():
            return {"error": f"Snapshot '{from_version}' not found"}
        if not to_dir.exists():
            return {"error": f"Snapshot '{to_version}' not found"}

        # Load both snapshots
        from_docs = json.loads((from_dir / "docs.json").read_text(encoding="utf-8"))
        to_docs = json.loads((to_dir / "docs.json").read_text(encoding="utf-8"))

        from_files = set(from_docs.get("files", {}).keys())
        to_files = set(to_docs.get("files", {}).keys())

        # File-level changes
        added_files = list(to_files - from_files)
        removed_files = list(from_files - to_files)

        # Symbol-level changes
        from_symbols: dict[str, dict[str, Any]] = {}
        to_symbols: dict[str, dict[str, Any]] = {}

        for file_path, symbols in from_docs.get("files", {}).items():
            for sym in symbols:
                key = f"{file_path}:{sym['qualified_name']}"
                from_symbols[key] = sym

        for file_path, symbols in to_docs.get("files", {}).items():
            for sym in symbols:
                key = f"{file_path}:{sym['qualified_name']}"
                to_symbols[key] = sym

        added_symbols = [s.split(":")[-1] for s in (set(to_symbols.keys()) - set(from_symbols.keys()))]
        removed_symbols = [s.split(":")[-1] for s in (set(from_symbols.keys()) - set(to_symbols.keys()))]

        # Find modified symbols (same qualified name but different content)
        modified_symbols = []
        for key in set(from_symbols.keys()) & set(to_symbols.keys()):
            from_sym = from_symbols[key]
            to_sym = to_symbols[key]
            # Check for signature or docstring changes
            if from_sym.get("signature") != to_sym.get("signature") or \
               from_sym.get("docstring") != to_sym.get("docstring"):
                modified_symbols.append(key.split(":")[-1])

        # Build summary
        parts = []
        if added_symbols:
            parts.append(f"{len(added_symbols)} added")
        if removed_symbols:
            parts.append(f"{len(removed_symbols)} removed")
        if modified_symbols:
            parts.append(f"{len(modified_symbols)} modified")
        if added_files:
            parts.append(f"{len(added_files)} new files")
        if removed_files:
            parts.append(f"{len(removed_files)} deleted files")

        summary = ", ".join(parts) if parts else "No changes"

        diff = SnapshotDiff(
            from_version=from_version,
            to_version=to_version,
            added_symbols=added_symbols,
            removed_symbols=removed_symbols,
            modified_symbols=modified_symbols,
            added_files=added_files,
            removed_files=removed_files,
            summary=summary,
        )

        if format == DocFormat.JSON:
            return diff.to_dict()
        return diff.to_markdown()

    def delete_snapshot(self, version: str) -> bool:
        """Delete a documentation snapshot.

        Args:
            version: Version identifier to delete

        Returns:
            True if deleted, False if not found
        """
        version_dir = self._snapshots_dir / version
        if version_dir.exists():
            shutil.rmtree(version_dir)
            return True
        return False

    def _get_current_commit(self) -> str | None:
        """Get the current git commit hash."""
        try:
            result = subprocess.run(
                ["git", "rev-parse", "HEAD"],
                cwd=self.project_root,
                capture_output=True,
                text=True,
                timeout=5,
            )
            if result.returncode == 0:
                return result.stdout.strip()
        except Exception:
            pass
        return None

    def _generate_html_docs(
        self,
        symbols_by_file: dict[str, list[dict[str, Any]]],
        version: str,
        description: str | None = None,
    ) -> str:
        """Generate static HTML documentation.

        Args:
            symbols_by_file: Symbols grouped by file path
            version: Version identifier
            description: Optional description

        Returns:
            HTML content
        """
        generated_at = datetime.now(timezone.utc).isoformat()

        # Build file list
        file_items = []
        for file_path in sorted(symbols_by_file.keys()):
            symbols = symbols_by_file[file_path]
            file_id = file_path.replace("/", "_").replace(".", "_")
            file_items.append(f'<li><a href="#{file_id}">{file_path}</a> ({len(symbols)} symbols)</li>')

        # Build file sections
        file_sections = []
        for file_path in sorted(symbols_by_file.keys()):
            symbols = symbols_by_file[file_path]
            file_id = file_path.replace("/", "_").replace(".", "_")

            classes = [s for s in symbols if s["kind"] == "class"]
            functions = [s for s in symbols if s["kind"] == "function"]

            section_parts = [f'<section id="{file_id}">']
            section_parts.append(f"<h2>{file_path}</h2>")

            if classes:
                section_parts.append("<h3>Classes</h3>")
                for cls in classes:
                    section_parts.append(f"<h4>📦 {cls['name']}</h4>")
                    if cls.get("docstring"):
                        section_parts.append(f"<p>{cls['docstring']}</p>")
                    if cls.get("children"):
                        section_parts.append("<p><strong>Methods:</strong></p><ul>")
                        for child in cls["children"]:
                            section_parts.append(f"<li><code>{child}</code></li>")
                        section_parts.append("</ul>")

            if functions:
                section_parts.append("<h3>Functions</h3>")
                for func in functions:
                    sig = func.get("signature") or func["name"]
                    section_parts.append(f"<h4>⚙️ <code>{sig}</code></h4>")
                    if func.get("docstring"):
                        section_parts.append(f"<p>{func['docstring']}</p>")

            section_parts.append("</section>")
            file_sections.append("\n".join(section_parts))

        # Build full HTML
        html = f'''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>API Documentation - {version}</title>
    <style>
        :root {{
            --primary: #2563eb;
            --bg: #f8fafc;
            --text: #1e293b;
            --border: #e2e8f0;
            --code-bg: #f1f5f9;
        }}
        * {{ box-sizing: border-box; margin: 0; padding: 0; }}
        body {{
            font-family: system-ui, -apple-system, sans-serif;
            background: var(--bg);
            color: var(--text);
            line-height: 1.6;
            padding: 2rem;
        }}
        header {{
            border-bottom: 1px solid var(--border);
            padding-bottom: 1rem;
            margin-bottom: 2rem;
        }}
        h1 {{ color: var(--primary); font-size: 2rem; }}
        h2 {{ margin-top: 2rem; padding-bottom: 0.5rem; border-bottom: 1px solid var(--border); }}
        h3 {{ margin-top: 1.5rem; color: #475569; }}
        h4 {{ margin-top: 1rem; }}
        nav {{ background: white; padding: 1rem; border-radius: 8px; margin-bottom: 2rem; }}
        nav ul {{ list-style: none; columns: 2; }}
        nav li {{ margin: 0.25rem 0; }}
        nav a {{ color: var(--primary); text-decoration: none; }}
        nav a:hover {{ text-decoration: underline; }}
        code {{ background: var(--code-bg); padding: 0.125rem 0.375rem; border-radius: 4px; font-size: 0.9em; }}
        pre {{ background: var(--code-bg); padding: 1rem; border-radius: 8px; overflow-x: auto; }}
        section {{ background: white; padding: 1.5rem; border-radius: 8px; margin-bottom: 1.5rem; }}
        footer {{ margin-top: 2rem; text-align: center; color: #64748b; font-size: 0.875rem; }}
    </style>
</head>
<body>
    <header>
        <h1>📚 API Documentation</h1>
        <p><strong>Version:</strong> {version}</p>
        {f"<p><strong>Description:</strong> {description}</p>" if description else ""}
        <p><strong>Generated:</strong> {generated_at}</p>
    </header>

    <nav>
        <h3>Files</h3>
        <ul>
            {"".join(file_items)}
        </ul>
    </nav>

    <main>
        {"".join(file_sections)}
    </main>

    <footer>
        <p>Generated by C4 Documentation Tools</p>
    </footer>
</body>
</html>'''
        return html


# MCP Tool Definitions
# These are used by the MCP server to expose documentation tools

MCP_TOOLS = [
    {
        "name": "query_docs",
        "description": "Search documentation by keyword/topic. Returns matching symbols with their documentation.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Search query (keyword, symbol name, topic)",
                },
                "kind": {
                    "type": "string",
                    "description": "Filter by symbol kind (class, function, method, etc.)",
                    "enum": ["class", "function", "method", "variable", "constant", "interface", "type_alias", "enum"],
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 20)",
                    "default": 20,
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
            "required": ["query"],
        },
    },
    {
        "name": "get_api_reference",
        "description": "Get detailed API documentation for a specific symbol (class, function, etc.)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the symbol to get documentation for",
                },
                "include_children": {
                    "type": "boolean",
                    "description": "Include child symbols (methods, properties)",
                    "default": True,
                },
                "include_references": {
                    "type": "boolean",
                    "description": "Include usage references",
                    "default": False,
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
            "required": ["symbol_name"],
        },
    },
    {
        "name": "search_examples",
        "description": "Find usage examples for a symbol in the codebase",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the symbol to find examples for",
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of examples (default: 10)",
                    "default": 10,
                },
                "context_lines": {
                    "type": "integer",
                    "description": "Number of surrounding lines to include (default: 3)",
                    "default": 3,
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
            "required": ["symbol_name"],
        },
    },
    {
        "name": "get_changelog",
        "description": "Get change history for a file or symbol using git",
        "inputSchema": {
            "type": "object",
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Path to the file (relative to project root)",
                },
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the symbol to track changes for",
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of changelog entries (default: 10)",
                    "default": 10,
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
        },
    },
    {
        "name": "create_snapshot",
        "description": "Create a versioned documentation snapshot. Saves current documentation state for later retrieval or comparison.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "version": {
                    "type": "string",
                    "description": "Version identifier (e.g., 'v1.0.0', '2024-01-15')",
                },
                "description": {
                    "type": "string",
                    "description": "Optional description of the snapshot",
                },
            },
            "required": ["version"],
        },
    },
    {
        "name": "list_snapshots",
        "description": "List all documentation snapshots. Returns metadata about available versions.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
        },
    },
    {
        "name": "get_snapshot",
        "description": "Get documentation for a specific snapshot version.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "version": {
                    "type": "string",
                    "description": "Version identifier of the snapshot",
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown, json, or html)",
                    "enum": ["markdown", "json", "html"],
                    "default": "markdown",
                },
            },
            "required": ["version"],
        },
    },
    {
        "name": "compare_snapshots",
        "description": "Compare two documentation snapshots. Shows added, removed, and modified symbols between versions.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "from_version": {
                    "type": "string",
                    "description": "Base version to compare from",
                },
                "to_version": {
                    "type": "string",
                    "description": "Target version to compare to",
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
            "required": ["from_version", "to_version"],
        },
    },
]


def handle_mcp_tool_call(
    tool_name: str,
    arguments: dict[str, Any],
    doc_generator: DocGenerator,
) -> str | dict[str, Any]:
    """Handle MCP tool call for documentation tools.

    Args:
        tool_name: Name of the tool to call
        arguments: Tool arguments
        doc_generator: DocGenerator instance

    Returns:
        Tool result
    """
    format_str = arguments.get("format", "markdown")
    format_enum = DocFormat.JSON if format_str == "json" else DocFormat.MARKDOWN

    if tool_name == "query_docs":
        kind = None
        if "kind" in arguments:
            kind_map = {
                "class": SymbolKind.CLASS,
                "function": SymbolKind.FUNCTION,
                "method": SymbolKind.METHOD,
                "variable": SymbolKind.VARIABLE,
                "constant": SymbolKind.CONSTANT,
                "interface": SymbolKind.INTERFACE,
                "type_alias": SymbolKind.TYPE_ALIAS,
                "enum": SymbolKind.ENUM,
            }
            kind = kind_map.get(arguments["kind"])

        return doc_generator.query_docs(
            query=arguments["query"],
            kind=kind,
            limit=arguments.get("limit", 20),
            format=format_enum,
        )

    elif tool_name == "get_api_reference":
        return doc_generator.get_api_reference(
            symbol_name=arguments["symbol_name"],
            include_children=arguments.get("include_children", True),
            include_references=arguments.get("include_references", False),
            format=format_enum,
        )

    elif tool_name == "search_examples":
        return doc_generator.search_examples(
            symbol_name=arguments["symbol_name"],
            limit=arguments.get("limit", 10),
            context_lines=arguments.get("context_lines", 3),
            format=format_enum,
        )

    elif tool_name == "get_changelog":
        return doc_generator.get_changelog(
            file_path=arguments.get("file_path"),
            symbol_name=arguments.get("symbol_name"),
            limit=arguments.get("limit", 10),
            format=format_enum,
        )

    elif tool_name == "create_snapshot":
        try:
            snapshot = doc_generator.create_snapshot(
                version=arguments["version"],
                description=arguments.get("description"),
            )
            if format_enum == DocFormat.JSON:
                return snapshot.to_dict()
            return snapshot.to_markdown()
        except ValueError as e:
            return {"error": str(e)}

    elif tool_name == "list_snapshots":
        snapshots = doc_generator.list_snapshots()
        if format_enum == DocFormat.JSON:
            return {"snapshots": [s.to_dict() for s in snapshots]}
        if not snapshots:
            return "No documentation snapshots found."
        result = "# Documentation Snapshots\n\n"
        for snapshot in snapshots:
            result += snapshot.to_markdown() + "\n"
        return result

    elif tool_name == "get_snapshot":
        format_str_snapshot = arguments.get("format", "markdown")
        if format_str_snapshot == "html":
            format_enum_snapshot = DocFormat.HTML
        elif format_str_snapshot == "json":
            format_enum_snapshot = DocFormat.JSON
        else:
            format_enum_snapshot = DocFormat.MARKDOWN
        return doc_generator.get_snapshot(
            version=arguments["version"],
            format=format_enum_snapshot,
        )

    elif tool_name == "compare_snapshots":
        return doc_generator.compare_snapshots(
            from_version=arguments["from_version"],
            to_version=arguments["to_version"],
            format=format_enum,
        )

    else:
        return {"error": f"Unknown tool: {tool_name}"}
