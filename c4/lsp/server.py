"""C4 LSP Server - Language Server Protocol implementation.

A pygls-based LSP server that provides code intelligence using
C4's tree-sitter based CodeAnalyzer.
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

try:
    from lsprotocol import types as lsp
    from pygls.lsp.server import LanguageServer

    PYGLS_AVAILABLE = True
except ImportError:
    PYGLS_AVAILABLE = False

from c4.docs.analyzer import CodeAnalyzer, SymbolKind

if TYPE_CHECKING:
    from c4.docs.analyzer import Location, Symbol

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
            raise ImportError(
                "pygls is required for LSP support. "
                "Install with: uv add pygls"
            )

        self._server = LanguageServer(name, version)
        self._analyzer = CodeAnalyzer()
        self._workspace_root: Path | None = None
        self._indexed_files: set[str] = set()

        self._register_features()

    @property
    def server(self) -> "LanguageServer":
        """Get the underlying pygls server."""
        return self._server

    @property
    def analyzer(self) -> CodeAnalyzer:
        """Get the code analyzer."""
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
            # Index workspace on startup
            if self._workspace_root:
                self._index_workspace()

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

            # Get the full text (we're using Full sync)
            if params.content_changes:
                text = params.content_changes[-1].text
                self._analyzer.add_file(file_path, text)

        @server.feature(lsp.TEXT_DOCUMENT_DID_SAVE)
        def did_save(params: lsp.DidSaveTextDocumentParams) -> None:
            """Handle document save."""
            uri = params.text_document.uri
            file_path = uri.replace("file://", "")

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

    def _index_workspace(self) -> None:
        """Index all files in the workspace."""
        if not self._workspace_root:
            return

        logger.info(f"Indexing workspace: {self._workspace_root}")
        count = self._analyzer.add_directory(self._workspace_root)
        logger.info(f"Indexed {count} files")

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

    def _handle_hover(self, params: lsp.HoverParams) -> lsp.Hover | None:
        """Handle textDocument/hover request."""
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
