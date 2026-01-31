"""C4 LSP Server - Language Server Protocol implementation.

A pygls-based LSP server that provides code intelligence using
C4's tree-sitter based CodeAnalyzer.
"""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from typing import TYPE_CHECKING

try:
    from lsprotocol import types as lsp
    from pygls.server import LanguageServer  # pygls 1.3.x API

    PYGLS_AVAILABLE = True
except ImportError:
    PYGLS_AVAILABLE = False

import re

from c4.docs.analyzer import CodeAnalyzer, SymbolKind
from c4.lsp.cache import get_symbol_cache
from c4.lsp.jedi_provider import JEDI_AVAILABLE, JediSymbolProvider

if TYPE_CHECKING:
    from c4.docs.analyzer import Location, Symbol
    from c4.models.task import Task
    from c4.store.sqlite import SQLiteTaskStore

logger = logging.getLogger(__name__)


def _symbol_kind_to_lsp(kind: SymbolKind) -> int:
    """Convert C4 SymbolKind to LSP SymbolKind."""
    mapping = {
        SymbolKind.MODULE: 2,
        SymbolKind.CLASS: 5,
        SymbolKind.METHOD: 6,
        SymbolKind.PROPERTY: 7,
        SymbolKind.FUNCTION: 12,
        SymbolKind.VARIABLE: 13,
        SymbolKind.CONSTANT: 14,
        SymbolKind.INTERFACE: 11,
        SymbolKind.ENUM: 10,
        SymbolKind.TYPE_ALIAS: 26,  # TypeParameter
        SymbolKind.IMPORT: 2,  # Module
        SymbolKind.PARAMETER: 13,  # Variable
    }
    return mapping.get(kind, 13)  # Default to Variable


def _symbol_kind_to_completion_kind(kind: SymbolKind) -> int:
    """Convert C4 SymbolKind to LSP CompletionItemKind."""
    # LSP CompletionItemKind values
    mapping = {
        SymbolKind.MODULE: 9,  # Module
        SymbolKind.CLASS: 7,  # Class
        SymbolKind.METHOD: 2,  # Method
        SymbolKind.PROPERTY: 10,  # Property
        SymbolKind.FUNCTION: 3,  # Function
        SymbolKind.VARIABLE: 6,  # Variable
        SymbolKind.CONSTANT: 21,  # Constant
        SymbolKind.INTERFACE: 8,  # Interface
        SymbolKind.ENUM: 13,  # Enum
        SymbolKind.TYPE_ALIAS: 25,  # TypeParameter
        SymbolKind.IMPORT: 9,  # Module
        SymbolKind.PARAMETER: 6,  # Variable
    }
    return mapping.get(kind, 6)  # Default to Variable


def _location_to_lsp_range(loc: Location) -> "lsp.Range":
    """Convert C4 Location to LSP Range."""
    return lsp.Range(
        start=lsp.Position(line=loc.start_line - 1, character=loc.start_column),
        end=lsp.Position(line=loc.end_line - 1, character=loc.end_column),
    )


def _location_to_lsp_location(loc: Location) -> "lsp.Location":
    """Convert C4 Location to LSP Location."""
    return lsp.Location(
        uri=f"file://{loc.file_path}",
        range=_location_to_lsp_range(loc),
    )


class C4LSPServer:
    """C4 Language Server Protocol Server.

    Provides code intelligence features:
    - textDocument/hover: Symbol information on hover
    - textDocument/definition: Go to definition
    - textDocument/references: Find all references
    - textDocument/documentSymbol: Document outline
    - workspace/symbol: Workspace symbol search

    Example:
        server = C4LSPServer()
        server.start_io()  # stdio mode
        # or
        server.start_tcp("localhost", 2087)  # TCP mode
    """

    def __init__(self, name: str = "c4-lsp", version: str = "0.1.0") -> None:
        """Initialize the LSP server.

        Args:
            name: Server name
            version: Server version
        """
        if not PYGLS_AVAILABLE:
            raise ImportError("pygls is required for LSP support. Install with: uv add pygls")

        self._server = LanguageServer(name, version)
        self._analyzer = CodeAnalyzer()
        self._workspace_root: Path | None = None
        self._indexed_files: set[str] = set()

        # Jedi provider for semantic analysis
        self._jedi_provider: JediSymbolProvider | None = None

        # Async indexing state
        self._indexing_in_progress: bool = False
        self._indexed_count: int = 0

        # C4 Task Store (lazy initialized)
        self._task_store: "SQLiteTaskStore | None" = None
        self._c4_project_id: str | None = None

        # Task ID pattern: T-XXX, T-XXX-N, R-XXX, R-XXX-N, CP-XXX
        self._task_id_pattern = re.compile(r"\b(T-\d{3}(?:-\d+)?|R-\d{3}(?:-\d+)?|CP-\d{3})\b")

        self._register_features()

    @property
    def server(self) -> "LanguageServer":
        """Get the underlying pygls server."""
        return self._server

    @property
    def analyzer(self) -> CodeAnalyzer:
        """Get the code analyzer."""
        return self._analyzer

    @property
    def jedi_provider(self) -> JediSymbolProvider | None:
        """Get the jedi symbol provider."""
        return self._jedi_provider
        return self._analyzer

    def _register_features(self) -> None:
        """Register LSP feature handlers."""
        server = self._server

        @server.feature(lsp.INITIALIZE)
        def initialize(params: lsp.InitializeParams) -> lsp.InitializeResult:
            """Handle initialize request."""
            if params.root_uri:
                root = params.root_uri.replace("file://", "")
                self._workspace_root = Path(root)
                logger.info(f"Workspace root: {self._workspace_root}")

                # Initialize jedi provider
                if JEDI_AVAILABLE:
                    try:
                        self._jedi_provider = JediSymbolProvider(project_path=self._workspace_root)
                        logger.info("Jedi symbol provider initialized")
                    except Exception as e:
                        logger.warning(f"Failed to initialize jedi provider: {e}")

            return lsp.InitializeResult(
                capabilities=lsp.ServerCapabilities(
                    text_document_sync=lsp.TextDocumentSyncOptions(
                        open_close=True,
                        change=lsp.TextDocumentSyncKind.Full,
                        save=lsp.SaveOptions(include_text=True),
                    ),
                    hover_provider=True,
                    definition_provider=True,
                    references_provider=True,
                    document_symbol_provider=True,
                    workspace_symbol_provider=True,
                    completion_provider=lsp.CompletionOptions(
                        trigger_characters=[".", "_"],
                        resolve_provider=True,
                    ),
                ),
                server_info=lsp.ServerInfo(
                    name=server.name,
                    version=server.version,
                ),
            )

        @server.feature(lsp.INITIALIZED)
        def initialized(params: lsp.InitializedParams) -> None:
            """Handle initialized notification."""
            logger.info("LSP server initialized")
            # Index workspace asynchronously on startup
            if self._workspace_root:
                asyncio.create_task(self._index_workspace_async())

        @server.feature(lsp.TEXT_DOCUMENT_DID_OPEN)
        def did_open(params: lsp.DidOpenTextDocumentParams) -> None:
            """Handle document open."""
            uri = params.text_document.uri
            text = params.text_document.text
            file_path = uri.replace("file://", "")

            logger.debug(f"Document opened: {file_path}")
            self._analyzer.add_file(file_path, text)
            self._indexed_files.add(file_path)

        @server.feature(lsp.TEXT_DOCUMENT_DID_CHANGE)
        def did_change(params: lsp.DidChangeTextDocumentParams) -> None:
            """Handle document change."""
            uri = params.text_document.uri
            file_path = uri.replace("file://", "")

            # Invalidate symbol cache for this file
            cache = get_symbol_cache()
            cache.invalidate(file_path)

            # Get the full text (we're using Full sync)
            if params.content_changes:
                text = params.content_changes[-1].text
                self._analyzer.add_file(file_path, text)

        @server.feature(lsp.TEXT_DOCUMENT_DID_SAVE)
        def did_save(params: lsp.DidSaveTextDocumentParams) -> None:
            """Handle document save."""
            uri = params.text_document.uri
            file_path = uri.replace("file://", "")

            # Invalidate symbol cache for this file
            cache = get_symbol_cache()
            cache.invalidate(file_path)

            if params.text:
                self._analyzer.add_file(file_path, params.text)
            logger.debug(f"Document saved: {file_path}")

        @server.feature(lsp.TEXT_DOCUMENT_DID_CLOSE)
        def did_close(params: lsp.DidCloseTextDocumentParams) -> None:
            """Handle document close."""
            uri = params.text_document.uri
            file_path = uri.replace("file://", "")
            logger.debug(f"Document closed: {file_path}")

        @server.feature(lsp.TEXT_DOCUMENT_HOVER)
        def hover(params: lsp.HoverParams) -> lsp.Hover | None:
            """Handle hover request."""
            return self._handle_hover(params)

        @server.feature(lsp.TEXT_DOCUMENT_DEFINITION)
        def definition(
            params: lsp.DefinitionParams,
        ) -> list[lsp.Location] | None:
            """Handle go to definition request."""
            return self._handle_definition(params)

        @server.feature(lsp.TEXT_DOCUMENT_REFERENCES)
        def references(
            params: lsp.ReferenceParams,
        ) -> list[lsp.Location] | None:
            """Handle find references request."""
            return self._handle_references(params)

        @server.feature(lsp.TEXT_DOCUMENT_DOCUMENT_SYMBOL)
        def document_symbol(
            params: lsp.DocumentSymbolParams,
        ) -> list[lsp.DocumentSymbol] | None:
            """Handle document symbol request."""
            return self._handle_document_symbol(params)

        @server.feature(lsp.WORKSPACE_SYMBOL)
        def workspace_symbol(
            params: lsp.WorkspaceSymbolParams,
        ) -> list[lsp.SymbolInformation] | None:
            """Handle workspace symbol request."""
            return self._handle_workspace_symbol(params)

        @server.feature(lsp.TEXT_DOCUMENT_COMPLETION)
        def completion(
            params: lsp.CompletionParams,
        ) -> lsp.CompletionList | None:
            """Handle completion request."""
            return self._handle_completion(params)

        @server.feature(lsp.COMPLETION_ITEM_RESOLVE)
        def completion_resolve(
            item: lsp.CompletionItem,
        ) -> lsp.CompletionItem:
            """Handle completion item resolve request."""
            return self._handle_completion_resolve(item)

    async def _index_workspace_async(self) -> None:
        """Index all files in the workspace asynchronously.

        Uses non-blocking approach to allow LSP to respond to requests
        while indexing is in progress.
        """
        if not self._workspace_root:
            return

        if self._indexing_in_progress:
            logger.warning("Indexing already in progress, skipping")
            return

        self._indexing_in_progress = True
        self._indexed_count = 0

        # Directories to exclude from indexing
        exclude_dirs = {
            "node_modules",
            "__pycache__",
            ".git",
            "venv",
            ".venv",
            ".pytest_cache",
            ".mypy_cache",
            ".ruff_cache",
            ".tox",
            "dist",
            "build",
            ".eggs",
            ".c4",
            ".claude",
        }

        logger.info(f"Indexing workspace (async): {self._workspace_root}")

        try:
            # Collect files to index (with exclusions)
            files_to_index: list[Path] = []
            for path in self._workspace_root.rglob("*.py"):
                # Check if any part of the path is in exclude_dirs
                if set(path.parts) & exclude_dirs:
                    continue
                files_to_index.append(path)

            logger.info(f"Found {len(files_to_index)} Python files to index")

            # Index in batches to yield control
            batch_size = 10
            for i, path in enumerate(files_to_index):
                try:
                    self._analyzer.add_file(path)
                    self._indexed_count += 1
                except Exception as e:
                    logger.warning(f"Failed to index {path}: {e}")

                # Yield to event loop periodically
                if (i + 1) % batch_size == 0:
                    await asyncio.sleep(0)

            logger.info(f"Indexing complete: {self._indexed_count} files indexed")
        except Exception as e:
            logger.error(f"Indexing failed: {e}")
        finally:
            self._indexing_in_progress = False

    def _get_word_at_position(
        self,
        uri: str,
        position: lsp.Position,
    ) -> str | None:
        """Get the word at a given position in a document."""
        file_path = uri.replace("file://", "")

        # Get file content from analyzer
        content = self._analyzer._file_contents.get(file_path)
        if not content:
            return None

        lines = content.split("\n")
        if position.line >= len(lines):
            return None

        line = lines[position.line]
        if position.character >= len(line):
            return None

        # Find word boundaries
        start = position.character
        end = position.character

        while start > 0 and (line[start - 1].isalnum() or line[start - 1] == "_"):
            start -= 1

        while end < len(line) and (line[end].isalnum() or line[end] == "_"):
            end += 1

        if start == end:
            return None

        return line[start:end]

    def _get_task_id_at_position(self, uri: str, position: "lsp.Position") -> str | None:
        """Extract task ID at the given position if present.

        Looks for patterns like T-001, T-001-0, R-001, R-001-0, CP-001.
        """
        file_path = uri.replace("file://", "")

        # Get file content from analyzer
        content = self._analyzer._file_contents.get(file_path)
        if not content:
            return None

        lines = content.split("\n")
        if position.line >= len(lines):
            return None

        line = lines[position.line]
        char_pos = position.character

        # Find all task ID matches in the line
        for match in self._task_id_pattern.finditer(line):
            start, end = match.span()
            if start <= char_pos <= end:
                return match.group(1)

        return None

    def _get_task_store(self) -> "SQLiteTaskStore | None":
        """Lazy initialize and return the task store."""
        if self._task_store is None:
            try:
                from c4.store.sqlite import SQLiteTaskStore

                # Find .c4 directory from workspace root
                c4_dir = None
                if self._workspace_root:
                    c4_dir = self._workspace_root / ".c4"
                    if not c4_dir.exists():
                        c4_dir = None

                if c4_dir is None:
                    # Try current directory
                    c4_dir = Path.cwd() / ".c4"
                    if not c4_dir.exists():
                        return None

                db_path = c4_dir / "c4.db"
                if not db_path.exists():
                    return None

                self._task_store = SQLiteTaskStore(str(db_path))

                # Load project_id from config
                config_path = c4_dir / "config.yaml"
                if config_path.exists():
                    import yaml

                    with open(config_path) as f:
                        config = yaml.safe_load(f)
                        self._c4_project_id = config.get("project_id", "")
            except Exception as e:
                logger.debug(f"Failed to initialize task store: {e}")
                return None

        return self._task_store

    def _get_task_info(self, task_id: str) -> "Task | None":
        """Get task information from C4 store."""
        store = self._get_task_store()
        if store is None or self._c4_project_id is None:
            return None

        try:
            return store.get(self._c4_project_id, task_id)
        except Exception as e:
            logger.debug(f"Failed to get task {task_id}: {e}")
            return None

    def _format_task_hover(self, task_id: str) -> str:
        """Format task information as Markdown hover content."""
        task = self._get_task_info(task_id)

        if task is None:
            return f"**Task**: `{task_id}`\n\n*Task not found*"

        parts = []

        # Task ID and title
        parts.append(f"**Task**: `{task.id}`")
        parts.append(f"# {task.title}")

        # Status
        status_emoji = {
            "pending": "⏳",
            "in_progress": "🔄",
            "done": "✅",
            "blocked": "❌",
        }
        status = task.status.value if hasattr(task.status, "value") else str(task.status)
        emoji = status_emoji.get(status, "❓")
        parts.append(f"\n**Status**: {emoji} {status}")

        # Assignee
        if task.assigned_to:
            parts.append(f"**Assigned to**: {task.assigned_to}")

        # Domain and type
        if task.domain:
            parts.append(f"**Domain**: {task.domain}")
        if task.task_type:
            parts.append(f"**Type**: {task.task_type}")

        # DoD (truncated if too long)
        if task.dod:
            dod_preview = task.dod[:500]
            if len(task.dod) > 500:
                dod_preview += "..."
            parts.append(f"\n---\n**Definition of Done**:\n{dod_preview}")

        # Dependencies
        if task.dependencies:
            deps = ", ".join(f"`{d}`" for d in task.dependencies[:5])
            if len(task.dependencies) > 5:
                deps += f" (+{len(task.dependencies) - 5} more)"
            parts.append(f"\n**Dependencies**: {deps}")

        return "\n".join(parts)

    def _get_task_completion_prefix(self, uri: str, position: "lsp.Position") -> str | None:
        """Check if cursor is at a task ID prefix for completion.

        Returns the prefix (e.g., 'T-', 'T-00', 'R-', 'CP-') if found, None otherwise.
        """
        # Convert URI to path
        if uri.startswith("file://"):
            file_path = uri[7:]
        else:
            file_path = uri

        # Get file content from analyzer
        content = self._analyzer._file_contents.get(file_path)
        if content is None:
            return None

        lines = content.split("\n")
        if position.line >= len(lines):
            return None

        line = lines[position.line]
        if position.character > len(line):
            return None

        # Look backward from cursor to find prefix
        text_before = line[: position.character]

        # Match T-, T-XXX, R-, R-XXX, CP-, CP-XXX patterns at end
        task_prefix_pattern = re.compile(r"(T-\d*|R-\d*|CP-\d*)$")
        match = task_prefix_pattern.search(text_before)

        if match:
            return match.group(1)

        return None

    def _get_task_completions(self, prefix: str) -> list["lsp.CompletionItem"]:
        """Get task completion items matching the prefix.

        Args:
            prefix: Task ID prefix (e.g., 'T-', 'T-00', 'R-', 'CP-')

        Returns:
            List of completion items for matching tasks
        """
        store = self._get_task_store()
        if store is None or self._c4_project_id is None:
            return []

        try:
            tasks = store.load_all(self._c4_project_id)
        except Exception as e:
            logger.debug(f"Failed to load tasks: {e}")
            return []

        items = []
        prefix_upper = prefix.upper()

        for task in tasks:
            # Filter by prefix
            if not task.id.upper().startswith(prefix_upper):
                continue

            # Only show pending and in_progress tasks (most relevant for completion)
            status = task.status.value if hasattr(task.status, "value") else str(task.status)
            if status not in ("pending", "in_progress"):
                continue

            # Status emoji for detail
            status_emoji = {
                "pending": "⏳",
                "in_progress": "🔄",
            }.get(status, "")

            item = lsp.CompletionItem(
                label=task.id,
                kind=lsp.CompletionItemKind.Reference,
                detail=f"{status_emoji} {task.title}",
                sort_text=f"0_{task.id}",  # Sort tasks before symbols
                insert_text=task.id,
                data={
                    "type": "c4_task",
                    "task_id": task.id,
                },
            )
            items.append(item)

        return items

    def _format_task_completion_doc(self, task: "Task") -> str:
        """Format task details for completion documentation."""
        parts = []

        parts.append(f"**{task.id}**: {task.title}")

        status = task.status.value if hasattr(task.status, "value") else str(task.status)
        status_emoji = {
            "pending": "⏳",
            "in_progress": "🔄",
            "done": "✅",
            "blocked": "❌",
        }.get(status, "")
        parts.append(f"\n**Status**: {status_emoji} {status}")

        if task.assigned_to:
            parts.append(f"**Assigned to**: {task.assigned_to}")

        if task.domain:
            parts.append(f"**Domain**: {task.domain}")

        if task.dod:
            dod_preview = task.dod[:300]
            if len(task.dod) > 300:
                dod_preview += "..."
            parts.append(f"\n---\n**DoD**:\n{dod_preview}")

        return "\n".join(parts)

    def _handle_hover(self, params: lsp.HoverParams) -> lsp.Hover | None:
        """Handle textDocument/hover request.

        Supports:
        - C4 Task IDs (T-XXX, R-XXX, CP-XXX patterns)
        - Code symbols (classes, functions, variables)
        """
        # First, check for task ID at position
        task_id = self._get_task_id_at_position(
            params.text_document.uri,
            params.position,
        )
        if task_id:
            content = self._format_task_hover(task_id)
            return lsp.Hover(
                contents=lsp.MarkupContent(
                    kind=lsp.MarkupKind.Markdown,
                    value=content,
                ),
            )

        # Fall back to symbol hover
        word = self._get_word_at_position(
            params.text_document.uri,
            params.position,
        )
        if not word:
            return None

        # Find the symbol
        symbols = self._analyzer.find_symbol(word, exact_match=True)
        if not symbols:
            return None

        symbol = symbols[0]

        # Build hover content
        parts = []

        # Symbol kind and name
        kind_name = symbol.kind.value.title()
        parts.append(f"**{kind_name}**: `{symbol.qualified_name}`")

        # Signature if available
        if symbol.signature:
            parts.append(f"\n```python\n{symbol.signature}\n```")

        # Docstring if available
        if symbol.docstring:
            parts.append(f"\n---\n{symbol.docstring}")

        # Location
        parts.append(f"\n*Defined in {symbol.location.file_path}:{symbol.location.start_line}*")

        content = "\n".join(parts)

        return lsp.Hover(
            contents=lsp.MarkupContent(
                kind=lsp.MarkupKind.Markdown,
                value=content,
            ),
        )

    def _handle_definition(
        self,
        params: lsp.DefinitionParams,
    ) -> list[lsp.Location] | None:
        """Handle textDocument/definition request."""
        word = self._get_word_at_position(
            params.text_document.uri,
            params.position,
        )
        if not word:
            return None

        symbols = self._analyzer.find_symbol(word, exact_match=True)
        if not symbols:
            return None

        return [_location_to_lsp_location(s.location) for s in symbols]

    def _handle_references(
        self,
        params: lsp.ReferenceParams,
    ) -> list[lsp.Location] | None:
        """Handle textDocument/references request."""
        word = self._get_word_at_position(
            params.text_document.uri,
            params.position,
        )
        if not word:
            return None

        references = self._analyzer.find_references(word)
        if not references:
            return None

        return [_location_to_lsp_location(r.location) for r in references]

    def _handle_document_symbol(
        self,
        params: lsp.DocumentSymbolParams,
    ) -> list[lsp.DocumentSymbol] | None:
        """Handle textDocument/documentSymbol request."""
        file_path = params.text_document.uri.replace("file://", "")
        symbols = self._analyzer.get_file_symbols(file_path)

        if not symbols:
            return None

        def to_document_symbol(symbol: Symbol) -> lsp.DocumentSymbol:
            children = [to_document_symbol(c) for c in symbol.children]
            return lsp.DocumentSymbol(
                name=symbol.name,
                kind=lsp.SymbolKind(_symbol_kind_to_lsp(symbol.kind)),
                range=_location_to_lsp_range(symbol.location),
                selection_range=_location_to_lsp_range(symbol.location),
                detail=symbol.signature,
                children=children if children else None,
            )

        # Filter to top-level symbols only (no parent)
        top_level = [s for s in symbols if s.parent is None]
        return [to_document_symbol(s) for s in top_level]

    def _handle_workspace_symbol(
        self,
        params: lsp.WorkspaceSymbolParams,
    ) -> list[lsp.SymbolInformation] | None:
        """Handle workspace/symbol request."""
        query = params.query
        if not query:
            return None

        symbols = self._analyzer.find_symbol(query, exact_match=False)
        if not symbols:
            return None

        result = []
        for symbol in symbols[:100]:  # Limit results
            result.append(
                lsp.SymbolInformation(
                    name=symbol.name,
                    kind=lsp.SymbolKind(_symbol_kind_to_lsp(symbol.kind)),
                    location=_location_to_lsp_location(symbol.location),
                    container_name=symbol.parent,
                )
            )

        return result

    def _get_prefix_at_position(
        self,
        uri: str,
        position: lsp.Position,
    ) -> str:
        """Get the word prefix at a given position (for completion)."""
        file_path = uri.replace("file://", "")

        content = self._analyzer._file_contents.get(file_path)
        if not content:
            return ""

        lines = content.split("\n")
        if position.line >= len(lines):
            return ""

        line = lines[position.line]
        if position.character > len(line):
            return ""

        # Find prefix start (walk backwards)
        start = position.character
        while start > 0 and (line[start - 1].isalnum() or line[start - 1] == "_"):
            start -= 1

        return line[start : position.character]

    def _handle_completion(
        self,
        params: lsp.CompletionParams,
    ) -> lsp.CompletionList | None:
        """Handle textDocument/completion request."""
        items = []

        # Check for task ID prefix (T-, R-, CP-)
        task_prefix = self._get_task_completion_prefix(
            params.text_document.uri,
            params.position,
        )
        if task_prefix:
            task_items = self._get_task_completions(task_prefix)
            items.extend(task_items)

        # Get symbol prefix
        prefix = self._get_prefix_at_position(
            params.text_document.uri,
            params.position,
        )

        # Get all symbols and filter by prefix
        all_symbols = self._analyzer.get_all_symbols()

        seen = set()  # Avoid duplicates

        for symbol in all_symbols:
            # Filter by prefix (case-insensitive)
            if prefix and not symbol.name.lower().startswith(prefix.lower()):
                continue

            # Skip duplicates
            if symbol.name in seen:
                continue
            seen.add(symbol.name)

            # Create completion item
            item = lsp.CompletionItem(
                label=symbol.name,
                kind=lsp.CompletionItemKind(_symbol_kind_to_completion_kind(symbol.kind)),
                detail=symbol.signature or f"{symbol.kind.value} in {symbol.location.file_path}",
                documentation=lsp.MarkupContent(
                    kind=lsp.MarkupKind.Markdown,
                    value=symbol.docstring or "",
                )
                if symbol.docstring
                else None,
                insert_text=symbol.name,
                data={
                    "name": symbol.name,
                    "file_path": symbol.location.file_path,
                    "line": symbol.location.start_line,
                },
            )
            items.append(item)

            # Limit results
            if len(items) >= 50:
                break

        # Return None if no items
        if not items:
            return None

        return lsp.CompletionList(
            is_incomplete=len(items) >= 50,
            items=items,
        )

    def _handle_completion_resolve(
        self,
        item: lsp.CompletionItem,
    ) -> lsp.CompletionItem:
        """Handle completionItem/resolve request.

        Provides additional details for a completion item.
        """
        if not item.data:
            return item

        # Handle C4 task completion items
        if item.data.get("type") == "c4_task":
            task_id = item.data.get("task_id")
            if task_id:
                task = self._get_task_info(task_id)
                if task:
                    item.documentation = lsp.MarkupContent(
                        kind=lsp.MarkupKind.Markdown,
                        value=self._format_task_completion_doc(task),
                    )
            return item

        # Handle symbol completion items
        name = item.data.get("name")
        if not name:
            return item

        # Find the symbol for more details
        symbols = self._analyzer.find_symbol(name, exact_match=True)
        if not symbols:
            return item

        symbol = symbols[0]

        # Build detailed documentation
        parts = []

        # Signature
        if symbol.signature:
            parts.append(f"```python\n{symbol.signature}\n```")

        # Docstring
        if symbol.docstring:
            parts.append(f"\n{symbol.docstring}")

        # Location
        loc = symbol.location
        parts.append(f"\n---\n*Defined in {loc.file_path}:{loc.start_line}*")

        if parts:
            item.documentation = lsp.MarkupContent(
                kind=lsp.MarkupKind.Markdown,
                value="\n".join(parts),
            )

        return item

    def start_io(self) -> None:
        """Start the server in stdio mode."""
        logger.info("Starting C4 LSP server (stdio)")
        self._server.start_io()

    def start_tcp(self, host: str = "localhost", port: int = 2087) -> None:
        """Start the server in TCP mode.

        Args:
            host: Host to bind to
            port: Port to listen on
        """
        logger.info(f"Starting C4 LSP server (TCP {host}:{port})")
        self._server.start_tcp(host, port)

    def stop(self) -> None:
        """Stop the LSP server.

        Signals the server to shut down gracefully.
        """
        logger.info("Stopping C4 LSP server")
        try:
            # pygls server shutdown
            if hasattr(self._server, "shutdown"):
                self._server.shutdown()
            elif hasattr(self._server, "stop"):
                self._server.stop()
        except Exception as e:
            logger.warning(f"Error during server shutdown: {e}")

    def set_workspace_root(self, root: Path) -> None:
        """Set the workspace root directory.

        This is used when starting the server programmatically
        without going through the LSP initialize handshake.

        Args:
            root: Path to the workspace root
        """
        self._workspace_root = Path(root)
        logger.info(f"Workspace root set to: {self._workspace_root}")

        # Initialize Jedi provider with workspace
        if JEDI_AVAILABLE:
            try:
                self._jedi_provider = JediSymbolProvider(project_path=self._workspace_root)
                logger.info("Jedi provider initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize Jedi provider: {e}")

    # MCP Tool Methods

    def find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
    ) -> list[dict]:
        """Find symbols matching the name path pattern.

        This is an MCP tool method for symbol search using jedi.

        Name path patterns:
        - Simple name: "method_name" - matches any symbol with that name
        - Relative path: "ClassName/method_name" - matches method in class
        - Absolute path: "/ClassName/method_name" - exact match from root

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
            relative_path: Restrict search to this file or directory
            include_body: Include symbol body in results

        Returns:
            List of symbol info dictionaries
        """
        if not self._jedi_provider:
            return []

        if relative_path and self._workspace_root:
            full_path = self._workspace_root / relative_path
            if full_path.is_file():
                try:
                    source = full_path.read_text(encoding="utf-8")
                    symbols = self._jedi_provider.find_symbol(
                        name_path_pattern,
                        source=source,
                        file_path=str(full_path),
                        include_body=include_body,
                    )
                except Exception as e:
                    logger.warning(f"Error reading file {full_path}: {e}")
                    return []
            else:
                symbols = self._jedi_provider.find_symbol(name_path_pattern)
        else:
            symbols = self._jedi_provider.find_symbol(name_path_pattern)

        return [s.to_dict() for s in symbols]

    def get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict:
        """Get an overview of symbols in a file.

        This is an MCP tool method for getting file symbol overview using jedi.

        Args:
            relative_path: Path to the file (relative to workspace root)
            depth: Depth of children to include (0 = top-level only)

        Returns:
            Dictionary with symbols grouped by kind
        """
        if not self._jedi_provider:
            return {"error": "jedi provider not available"}

        if not self._workspace_root:
            return {"error": "workspace root not set"}

        full_path = self._workspace_root / relative_path

        if not full_path.exists():
            return {"error": f"File not found: {relative_path}"}

        symbols = self._jedi_provider.get_symbols_overview(str(full_path), depth=depth)

        # Group by kind
        grouped: dict[str, list[dict]] = {}
        for symbol in symbols:
            kind = symbol.kind.value
            if kind not in grouped:
                grouped[kind] = []
            grouped[kind].append(symbol.to_dict())

        return {
            "file": relative_path,
            "symbols_by_kind": grouped,
            "total_count": len(symbols),
        }


def main() -> None:
    """CLI entry point for the LSP server."""
    import argparse

    parser = argparse.ArgumentParser(description="C4 LSP Server")
    parser.add_argument(
        "--tcp",
        action="store_true",
        help="Use TCP instead of stdio",
    )
    parser.add_argument(
        "--host",
        default="localhost",
        help="Host to bind to (TCP mode)",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=2087,
        help="Port to listen on (TCP mode)",
    )
    parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Enable verbose logging",
    )

    args = parser.parse_args()

    # Configure logging
    level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    )

    server = C4LSPServer()

    if args.tcp:
        server.start_tcp(args.host, args.port)
    else:
        server.start_io()


if __name__ == "__main__":
    main()
