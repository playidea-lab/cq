"""Semantic Search Engine for Code and Documentation.

Provides natural language search over codebase:
- TF-IDF based semantic similarity
- Keyword extraction and expansion
- Multi-language support (Python, TypeScript, Go, Rust)
- Ranked results with relevance scoring
"""

from __future__ import annotations

import math
import re
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any

from c4.docs.analyzer import CodeAnalyzer, Symbol, SymbolKind


class SearchScope(Enum):
    """Search scope options."""

    ALL = "all"  # Search everything
    SYMBOLS = "symbols"  # Only symbols (functions, classes, etc.)
    DOCS = "docs"  # Only docstrings/comments
    CODE = "code"  # Only code content
    FILES = "files"  # File names only


@dataclass
class SearchHit:
    """A search result hit."""

    file_path: str
    line_number: int
    score: float
    content: str
    symbol_name: str | None = None
    symbol_kind: SymbolKind | None = None
    context: list[str] = field(default_factory=list)
    highlights: list[tuple[int, int]] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "file_path": self.file_path,
            "line_number": self.line_number,
            "score": round(self.score, 4),
            "content": self.content,
            "symbol_name": self.symbol_name,
            "symbol_kind": self.symbol_kind.value if self.symbol_kind else None,
            "context": self.context,
            "highlights": self.highlights,
            "metadata": self.metadata,
        }


@dataclass
class SearchResult:
    """Search result container."""

    query: str
    total_hits: int
    hits: list[SearchHit]
    query_time_ms: float
    expanded_terms: list[str] = field(default_factory=list)
    suggestions: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "query": self.query,
            "total_hits": self.total_hits,
            "hits": [h.to_dict() for h in self.hits],
            "query_time_ms": round(self.query_time_ms, 2),
            "expanded_terms": self.expanded_terms,
            "suggestions": self.suggestions,
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append(f"# Search Results: `{self.query}`")
        lines.append("")
        lines.append(f"Found **{self.total_hits}** results in {self.query_time_ms:.1f}ms")

        if self.expanded_terms:
            lines.append(f"\n**Expanded terms:** {', '.join(self.expanded_terms)}")

        lines.append("")

        for i, hit in enumerate(self.hits[:20], 1):
            kind_str = f"[{hit.symbol_kind.value}]" if hit.symbol_kind else ""
            lines.append(f"### {i}. {hit.file_path}:{hit.line_number} {kind_str}")
            lines.append(f"**Score:** {hit.score:.3f}")
            if hit.symbol_name:
                lines.append(f"**Symbol:** `{hit.symbol_name}`")
            lines.append("")
            lines.append("```")
            lines.append(hit.content)
            lines.append("```")
            lines.append("")

        if self.suggestions:
            lines.append("### Did you mean?")
            for s in self.suggestions[:5]:
                lines.append(f"- `{s}`")

        return "\n".join(lines)


class SemanticSearcher:
    """TF-IDF based semantic search engine.

    Features:
    - Natural language queries
    - Synonym expansion
    - Fuzzy matching
    - Ranked results

    Example:
        searcher = SemanticSearcher(project_root=".")
        searcher.index()

        # Natural language search
        results = searcher.search("how to authenticate users")

        # Code-specific search
        results = searcher.search("database connection", scope=SearchScope.CODE)
    """

    # Common programming synonyms for query expansion
    SYNONYMS = {
        "auth": ["authentication", "authorize", "login", "signin"],
        "user": ["account", "profile", "member"],
        "db": ["database", "sql", "query", "orm"],
        "api": ["endpoint", "route", "handler", "rest"],
        "err": ["error", "exception", "fail", "catch"],
        "req": ["request", "http", "fetch"],
        "res": ["response", "result", "output"],
        "config": ["configuration", "settings", "options", "env"],
        "test": ["spec", "unittest", "pytest", "jest"],
        "async": ["await", "promise", "concurrent", "parallel"],
        "cache": ["redis", "memcache", "store", "persist"],
        "log": ["logger", "logging", "debug", "trace"],
        "validate": ["validation", "check", "verify", "sanitize"],
        "parse": ["parser", "decode", "deserialize", "convert"],
        "serialize": ["encode", "json", "stringify", "marshal"],
        "create": ["add", "insert", "new", "generate"],
        "read": ["get", "fetch", "retrieve", "find", "query"],
        "update": ["modify", "edit", "patch", "change"],
        "delete": ["remove", "destroy", "drop", "clear"],
        "list": ["array", "collection", "items", "all"],
    }

    # Stop words to filter out
    STOP_WORDS = {
        "the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
        "have", "has", "had", "do", "does", "did", "will", "would", "could",
        "should", "may", "might", "must", "shall", "can", "need", "dare",
        "ought", "used", "to", "of", "in", "for", "on", "with", "at", "by",
        "from", "as", "into", "through", "during", "before", "after",
        "above", "below", "between", "under", "again", "further", "then",
        "once", "here", "there", "when", "where", "why", "how", "all", "each",
        "few", "more", "most", "other", "some", "such", "no", "nor", "not",
        "only", "own", "same", "so", "than", "too", "very", "just", "and",
        "but", "if", "or", "because", "until", "while", "although", "this",
        "that", "these", "those", "i", "me", "my", "we", "our", "you", "your",
        "it", "its", "they", "them", "what", "which", "who", "whom",
    }

    def __init__(
        self,
        project_root: str | Path = ".",
        analyzer: CodeAnalyzer | None = None,
    ) -> None:
        """Initialize the semantic searcher.

        Args:
            project_root: Root directory of the project
            analyzer: Optional CodeAnalyzer instance to reuse
        """
        self.project_root = Path(project_root).resolve()
        self._analyzer = analyzer or CodeAnalyzer()
        self._indexed = False

        # TF-IDF index
        self._documents: dict[str, str] = {}  # doc_id -> content
        self._doc_metadata: dict[str, dict[str, Any]] = {}  # doc_id -> metadata
        self._term_freq: dict[str, Counter] = {}  # doc_id -> term -> count
        self._doc_freq: Counter = Counter()  # term -> doc count
        self._total_docs = 0

        # Symbol index for faster lookups
        self._symbol_docs: dict[str, str] = {}  # symbol_name -> doc_id
        self._all_symbols: set[str] = set()

    def index(
        self,
        include_patterns: list[str] | None = None,
        exclude_patterns: list[str] | None = None,
    ) -> int:
        """Index the codebase for semantic search.

        Args:
            include_patterns: Glob patterns to include
            exclude_patterns: Glob patterns to exclude

        Returns:
            Number of documents indexed
        """
        exclude_patterns = exclude_patterns or [
            "**/node_modules/**",
            "**/__pycache__/**",
            "**/.git/**",
            "**/venv/**",
            "**/.venv/**",
            "**/dist/**",
            "**/build/**",
            "**/*.min.js",
            "**/*.min.css",
        ]

        # Reset index
        self._documents.clear()
        self._doc_metadata.clear()
        self._term_freq.clear()
        self._doc_freq.clear()
        self._symbol_docs.clear()
        self._all_symbols.clear()
        self._total_docs = 0

        # Index code files
        self._analyzer.add_directory(
            self.project_root,
            recursive=True,
            exclude_patterns=exclude_patterns,
        )

        # Build document index
        count = 0
        for file_path in self._get_files(exclude_patterns):
            try:
                content = Path(file_path).read_text(encoding="utf-8")
                self._index_file(file_path, content)
                count += 1
            except Exception:
                continue

        # Index symbols
        for symbol in self._analyzer.get_all_symbols():
            self._index_symbol(symbol)

        # Calculate IDF
        self._total_docs = len(self._documents)
        self._indexed = True

        return count

    def _get_files(self, exclude_patterns: list[str]) -> list[str]:
        """Get all files to index."""
        extensions = {
            ".py", ".ts", ".tsx", ".js", ".jsx",
            ".go", ".rs", ".java", ".kt", ".swift",
            ".md", ".rst", ".txt", ".yaml", ".yml", ".json",
        }

        files = []
        for ext in extensions:
            for path in self.project_root.rglob(f"*{ext}"):
                if not path.is_file():
                    continue

                path_str = str(path)
                excluded = False
                for pattern in exclude_patterns:
                    if Path(path_str).match(pattern):
                        excluded = True
                        break

                if not excluded:
                    files.append(path_str)

        return files

    def _index_file(self, file_path: str, content: str) -> None:
        """Index a file for search."""
        lines = content.split("\n")

        for i, line in enumerate(lines):
            line = line.strip()
            if not line:
                continue

            # Create document ID
            doc_id = f"{file_path}:{i + 1}"

            # Tokenize and calculate TF
            tokens = self._tokenize(line)
            if not tokens:
                continue

            self._documents[doc_id] = line
            self._doc_metadata[doc_id] = {
                "file_path": file_path,
                "line_number": i + 1,
            }

            # Term frequency
            tf = Counter(tokens)
            self._term_freq[doc_id] = tf

            # Document frequency
            for term in set(tokens):
                self._doc_freq[term] += 1

    def _index_symbol(self, symbol: Symbol) -> None:
        """Index a symbol for search."""
        doc_id = f"{symbol.location.file_path}:{symbol.location.start_line}"

        # Build searchable content
        parts = [symbol.name]
        if symbol.docstring:
            parts.append(symbol.docstring)
        if symbol.signature:
            parts.append(symbol.signature)

        content = " ".join(parts)
        tokens = self._tokenize(content)

        if tokens:
            # Update existing doc or create new one
            if doc_id in self._documents:
                # Merge tokens
                existing_tf = self._term_freq.get(doc_id, Counter())
                new_tf = Counter(tokens)
                self._term_freq[doc_id] = existing_tf + new_tf

                # Update doc freq for new terms
                existing_terms = set(existing_tf.keys())
                for term in set(tokens) - existing_terms:
                    self._doc_freq[term] += 1
            else:
                self._documents[doc_id] = content
                self._term_freq[doc_id] = Counter(tokens)
                for term in set(tokens):
                    self._doc_freq[term] += 1

            # Update metadata
            if doc_id not in self._doc_metadata:
                self._doc_metadata[doc_id] = {}

            self._doc_metadata[doc_id].update({
                "file_path": symbol.location.file_path,
                "line_number": symbol.location.start_line,
                "symbol_name": symbol.name,
                "symbol_kind": symbol.kind,
            })

            # Symbol index
            self._symbol_docs[symbol.name] = doc_id
            self._all_symbols.add(symbol.name)

    def _tokenize(self, text: str) -> list[str]:
        """Tokenize text for indexing/search.

        Args:
            text: Text to tokenize

        Returns:
            List of normalized tokens
        """
        # Convert to lowercase
        text = text.lower()

        # Split camelCase and PascalCase
        text = re.sub(r"([a-z])([A-Z])", r"\1 \2", text)

        # Split snake_case
        text = text.replace("_", " ")

        # Remove special characters, keep alphanumeric
        text = re.sub(r"[^a-z0-9\s]", " ", text)

        # Split and filter
        tokens = text.split()
        tokens = [t for t in tokens if len(t) > 1 and t not in self.STOP_WORDS]

        return tokens

    def _expand_query(self, tokens: list[str]) -> list[str]:
        """Expand query terms with synonyms.

        Args:
            tokens: Original query tokens

        Returns:
            Expanded list of tokens
        """
        expanded = list(tokens)

        for token in tokens:
            # Direct synonym lookup
            if token in self.SYNONYMS:
                expanded.extend(self.SYNONYMS[token])
            else:
                # Reverse lookup
                for key, synonyms in self.SYNONYMS.items():
                    if token in synonyms:
                        expanded.append(key)
                        expanded.extend(s for s in synonyms if s != token)
                        break

        return list(set(expanded))

    def _calculate_tfidf(
        self,
        doc_id: str,
        query_tokens: list[str],
    ) -> float:
        """Calculate TF-IDF score for a document.

        Args:
            doc_id: Document ID
            query_tokens: Query tokens

        Returns:
            TF-IDF score
        """
        if doc_id not in self._term_freq:
            return 0.0

        doc_tf = self._term_freq[doc_id]
        score = 0.0

        for term in query_tokens:
            tf = doc_tf.get(term, 0)
            if tf == 0:
                continue

            # IDF with smoothing
            df = self._doc_freq.get(term, 0)
            if df == 0:
                continue

            idf = math.log((self._total_docs + 1) / (df + 1)) + 1

            # TF-IDF
            score += (1 + math.log(tf)) * idf

        # Normalize by document length
        doc_length = sum(doc_tf.values())
        if doc_length > 0:
            score /= math.sqrt(doc_length)

        return score

    def search(
        self,
        query: str,
        scope: SearchScope = SearchScope.ALL,
        limit: int = 20,
        min_score: float = 0.1,
        expand_synonyms: bool = True,
        context_lines: int = 2,
    ) -> SearchResult:
        """Search the codebase using natural language.

        Args:
            query: Natural language search query
            scope: What to search (all, symbols, docs, code, files)
            limit: Maximum number of results
            min_score: Minimum relevance score (0-1)
            expand_synonyms: Whether to expand query with synonyms
            context_lines: Number of context lines to include

        Returns:
            SearchResult with ranked hits
        """
        import time
        start_time = time.time()

        if not self._indexed:
            self.index()

        # Tokenize query
        query_tokens = self._tokenize(query)
        if not query_tokens:
            return SearchResult(
                query=query,
                total_hits=0,
                hits=[],
                query_time_ms=0,
            )

        # Expand with synonyms
        expanded_tokens = []
        if expand_synonyms:
            expanded_tokens = self._expand_query(query_tokens)
            all_tokens = list(set(query_tokens + expanded_tokens))
        else:
            all_tokens = query_tokens

        # Score all documents
        scored_docs: list[tuple[str, float]] = []

        for doc_id in self._documents:
            metadata = self._doc_metadata.get(doc_id, {})

            # Apply scope filter
            if scope == SearchScope.SYMBOLS and "symbol_name" not in metadata:
                continue
            elif scope == SearchScope.DOCS and metadata.get("symbol_kind") not in (
                None, SymbolKind.CLASS, SymbolKind.FUNCTION, SymbolKind.METHOD
            ):
                # Only include documented items
                doc_content = self._documents.get(doc_id, "")
                if not any(kw in doc_content.lower() for kw in ["\"\"\"", "'''", "//", "/*"]):
                    continue
            elif scope == SearchScope.FILES:
                # Only match file paths
                file_path = metadata.get("file_path", "")
                if not any(t in file_path.lower() for t in all_tokens):
                    continue

            score = self._calculate_tfidf(doc_id, all_tokens)

            # Boost exact matches
            content = self._documents.get(doc_id, "").lower()
            if query.lower() in content:
                score *= 1.5

            # Boost symbol name matches
            symbol_name = metadata.get("symbol_name", "")
            if symbol_name and any(t in symbol_name.lower() for t in query_tokens):
                score *= 1.3

            if score >= min_score:
                scored_docs.append((doc_id, score))

        # Sort by score
        scored_docs.sort(key=lambda x: x[1], reverse=True)
        scored_docs = scored_docs[:limit]

        # Build hits
        hits = []
        for doc_id, score in scored_docs:
            metadata = self._doc_metadata.get(doc_id, {})
            content = self._documents.get(doc_id, "")

            # Get context lines
            context = self._get_context(
                metadata.get("file_path", ""),
                metadata.get("line_number", 1),
                context_lines,
            )

            # Find highlight positions
            highlights = []
            content_lower = content.lower()
            for token in query_tokens:
                start = 0
                while True:
                    pos = content_lower.find(token, start)
                    if pos == -1:
                        break
                    highlights.append((pos, pos + len(token)))
                    start = pos + 1

            hit = SearchHit(
                file_path=metadata.get("file_path", ""),
                line_number=metadata.get("line_number", 1),
                score=score,
                content=content,
                symbol_name=metadata.get("symbol_name"),
                symbol_kind=metadata.get("symbol_kind"),
                context=context,
                highlights=highlights,
            )
            hits.append(hit)

        elapsed_ms = (time.time() - start_time) * 1000

        # Generate suggestions for low/no results
        suggestions = []
        if len(hits) < 3:
            suggestions = self._get_suggestions(query_tokens)

        return SearchResult(
            query=query,
            total_hits=len(hits),
            hits=hits,
            query_time_ms=elapsed_ms,
            expanded_terms=expanded_tokens if expand_synonyms else [],
            suggestions=suggestions,
        )

    def _get_context(
        self,
        file_path: str,
        line_number: int,
        context_lines: int,
    ) -> list[str]:
        """Get surrounding context lines.

        Args:
            file_path: Path to file
            line_number: Target line number
            context_lines: Number of lines before/after

        Returns:
            List of context lines
        """
        try:
            path = Path(file_path)
            if not path.exists():
                return []

            lines = path.read_text(encoding="utf-8").split("\n")
            start = max(0, line_number - context_lines - 1)
            end = min(len(lines), line_number + context_lines)

            return [
                f"{i + 1}: {lines[i]}"
                for i in range(start, end)
            ]
        except Exception:
            return []

    def _get_suggestions(self, query_tokens: list[str]) -> list[str]:
        """Get search suggestions based on query.

        Args:
            query_tokens: Original query tokens

        Returns:
            List of suggested search terms
        """
        suggestions = []

        # Find similar symbol names
        for token in query_tokens:
            for symbol in self._all_symbols:
                symbol_lower = symbol.lower()
                if token in symbol_lower or symbol_lower.startswith(token[:3]):
                    suggestions.append(symbol)
                    if len(suggestions) >= 5:
                        break

        return list(set(suggestions))[:5]

    def find_related(
        self,
        symbol_name: str,
        limit: int = 10,
    ) -> list[SearchHit]:
        """Find symbols related to a given symbol.

        Args:
            symbol_name: Name of the symbol
            limit: Maximum number of results

        Returns:
            List of related symbols
        """
        # Get the symbol's content
        doc_id = self._symbol_docs.get(symbol_name)
        if not doc_id:
            return []

        content = self._documents.get(doc_id, "")
        tokens = self._tokenize(content)

        if not tokens:
            return []

        # Score other symbols
        scored = []
        for other_name, other_doc_id in self._symbol_docs.items():
            if other_name == symbol_name:
                continue

            score = self._calculate_tfidf(other_doc_id, tokens)
            if score > 0:
                scored.append((other_doc_id, score))

        scored.sort(key=lambda x: x[1], reverse=True)

        # Build hits
        hits = []
        for doc_id, score in scored[:limit]:
            metadata = self._doc_metadata.get(doc_id, {})
            hits.append(SearchHit(
                file_path=metadata.get("file_path", ""),
                line_number=metadata.get("line_number", 1),
                score=score,
                content=self._documents.get(doc_id, ""),
                symbol_name=metadata.get("symbol_name"),
                symbol_kind=metadata.get("symbol_kind"),
            ))

        return hits

    def search_by_type(
        self,
        kind: SymbolKind,
        query: str | None = None,
        limit: int = 20,
    ) -> list[SearchHit]:
        """Search for symbols of a specific type.

        Args:
            kind: Symbol kind to search for
            query: Optional text query
            limit: Maximum results

        Returns:
            List of matching symbols
        """
        hits = []

        for doc_id, metadata in self._doc_metadata.items():
            if metadata.get("symbol_kind") != kind:
                continue

            content = self._documents.get(doc_id, "")

            # If query provided, filter by it
            if query:
                query_tokens = self._tokenize(query)
                score = self._calculate_tfidf(doc_id, query_tokens)
                if score < 0.1:
                    continue
            else:
                score = 1.0

            hits.append(SearchHit(
                file_path=metadata.get("file_path", ""),
                line_number=metadata.get("line_number", 1),
                score=score,
                content=content,
                symbol_name=metadata.get("symbol_name"),
                symbol_kind=kind,
            ))

        hits.sort(key=lambda x: x.score, reverse=True)
        return hits[:limit]


# MCP Tool Definitions
MCP_SEMANTIC_TOOLS = [
    {
        "name": "semantic_search",
        "description": "Search the codebase using natural language. Finds code, symbols, and documentation matching your query.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Natural language search query (e.g., 'how to authenticate users', 'database connection handling')",
                },
                "scope": {
                    "type": "string",
                    "description": "What to search: all, symbols, docs, code, files",
                    "enum": ["all", "symbols", "docs", "code", "files"],
                    "default": "all",
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 20)",
                    "default": 20,
                },
                "expand_synonyms": {
                    "type": "boolean",
                    "description": "Expand query with programming synonyms (default: true)",
                    "default": True,
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
        "name": "find_related_symbols",
        "description": "Find symbols that are semantically related to a given symbol.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the symbol to find related items for",
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 10)",
                    "default": 10,
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
        "name": "search_by_type",
        "description": "Search for all symbols of a specific type (class, function, interface, etc.)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "kind": {
                    "type": "string",
                    "description": "Symbol type to search for",
                    "enum": ["class", "function", "method", "interface", "type_alias", "enum", "constant", "variable"],
                },
                "query": {
                    "type": "string",
                    "description": "Optional text query to filter results",
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
            "required": ["kind"],
        },
    },
]
