"""Multilspy-based symbol provider for C4 LSP.

Provides semantic code analysis using real LSP servers via multilspy for:
- Symbol search (find_symbol) with workspace/symbol
- Document symbols (get_symbols_overview) with textDocument/documentSymbol
- Multi-language support (Python, TypeScript, Go, Rust, etc.)

Multilspy is a Microsoft library that wraps language servers and provides
a unified Python interface. This gives us IDE-level performance and reliability.
"""

from __future__ import annotations

import logging
import threading
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING, Any

try:
    from multilspy import SyncLanguageServer
    from multilspy.multilspy_config import MultilspyConfig

    MULTILSPY_AVAILABLE = True
except ImportError:
    MULTILSPY_AVAILABLE = False
    SyncLanguageServer = None  # type: ignore[assignment, misc]
    MultilspyConfig = None  # type: ignore[assignment, misc]

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


@dataclass
class LanguageServerInfo:
    """Information about a language server instance."""

    server: Any  # SyncLanguageServer
    language: str
    started: bool = False
    lock: threading.Lock = field(default_factory=threading.Lock)
    last_used: float = field(default_factory=lambda: __import__("time").time())
    request_count: int = 0


class MultilspyProvider:
    """Symbol provider using multilspy (real LSP servers).

    This provider uses Microsoft's multilspy library to communicate with
    actual language servers (pylsp, pyright, tsserver, gopls, etc.),
    providing IDE-level code intelligence.

    Features:
    - Multi-language support (30+ languages)
    - Built-in timeout handling
    - Background indexing by LSP servers
    - Incremental parsing and caching

    Example:
        >>> provider = MultilspyProvider("/path/to/project")
        >>> symbols = provider.find_symbol("MyClass")
        >>> overview = provider.get_symbols_overview("src/main.py")
    """

    # File extension to language mapping
    LANGUAGE_MAP: dict[str, str] = {
        ".py": "python",
        ".pyi": "python",
        ".ts": "typescript",
        ".tsx": "typescript",
        ".js": "javascript",
        ".jsx": "javascript",
        ".go": "go",
        ".rs": "rust",
        ".java": "java",
        ".rb": "ruby",
        ".cs": "csharp",
    }

    def __init__(
        self,
        project_path: str | Path,
        timeout: int = 30,
        idle_timeout: int = 300,
    ):
        """Initialize the multilspy provider.

        Args:
            project_path: Root path of the project to analyze.
            timeout: Timeout in seconds for LSP operations.
            idle_timeout: Seconds of inactivity before shutting down a server (default: 5 min).
        """
        if not MULTILSPY_AVAILABLE:
            raise ImportError("multilspy is not installed. Install with: uv add multilspy")

        self.project_path = Path(project_path).resolve()
        self.timeout = timeout
        self.idle_timeout = idle_timeout
        self._servers: dict[str, LanguageServerInfo] = {}
        self._global_lock = threading.Lock()

    def _get_server(self, language: str) -> SyncLanguageServer:
        """Get or create LSP server for the specified language.

        Args:
            language: Language identifier (e.g., "python", "typescript").

        Returns:
            A SyncLanguageServer instance for the language.

        Raises:
            RuntimeError: If server creation fails.
        """
        with self._global_lock:
            if language not in self._servers:
                try:
                    config = MultilspyConfig.from_dict(
                        {
                            "code_language": language,
                            "trace_lsp_communication": False,
                        }
                    )
                    server = SyncLanguageServer.create(
                        config,
                        logger,
                        str(self.project_path),
                        timeout=self.timeout,
                    )
                    self._servers[language] = LanguageServerInfo(
                        server=server,
                        language=language,
                    )
                    logger.info(f"Created LSP server for {language}")
                except Exception as e:
                    logger.error(f"Failed to create LSP server for {language}: {e}")
                    raise RuntimeError(f"LSP server creation failed: {e}") from e

            # Update usage tracking
            import time

            info = self._servers[language]
            info.last_used = time.time()
            info.request_count += 1

            return info.server

    def _detect_language(self, file_path: Path | str) -> str | None:
        """Detect programming language from file extension.

        Args:
            file_path: Path to the file.

        Returns:
            Language identifier or None if unsupported.
        """
        path = Path(file_path)
        return self.LANGUAGE_MAP.get(path.suffix.lower())

    def find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
        depth: int = 0,
    ) -> list[dict[str, Any]]:
        """Find symbols matching the pattern using LSP workspace/symbol.

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass", "MyClass/method").
            relative_path: Restrict search to this file or directory.
            include_body: Include symbol body in results (requires extra request).
            depth: Depth of children to include.

        Returns:
            List of symbol information dictionaries.
        """
        results: list[dict[str, Any]] = []

        # Parse pattern to extract symbol name
        parts = name_path_pattern.split("/")
        query = parts[-1]  # Use last part as search query

        # Determine which languages to query
        if relative_path:
            file_path = self.project_path / relative_path
            if file_path.is_file():
                language = self._detect_language(file_path)
                languages = [language] if language else []
            else:
                # Directory - query all available languages
                languages = list(set(self.LANGUAGE_MAP.values()))
        else:
            # No restriction - query Python by default (most common)
            languages = ["python"]

        for lang in languages:
            if not lang:
                continue

            try:
                server = self._get_server(lang)
                with server.start_server():
                    # Use workspace/symbol for global search
                    symbols = server.request_workspace_symbol(query)

                    for sym in symbols:
                        result = self._convert_workspace_symbol(sym, include_body)
                        if result:
                            # Filter by name_path_pattern
                            if self._matches_pattern(result, name_path_pattern):
                                results.append(result)

            except Exception as e:
                logger.warning(f"LSP workspace/symbol failed for {lang}: {e}")

        return results

    def get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get document symbols using LSP textDocument/documentSymbol.

        Args:
            relative_path: Path to the file relative to project root.
            depth: Depth of children to include (0 = top-level only).

        Returns:
            Dictionary with symbols grouped by kind.
        """
        file_path = self.project_path / relative_path
        language = self._detect_language(file_path)

        if not language:
            return {"error": f"Unsupported file type: {file_path.suffix}"}

        if not file_path.exists():
            return {"error": f"File not found: {relative_path}"}

        try:
            server = self._get_server(language)
            with server.start_server():
                # Open the document in the LSP server
                server.open_file(str(file_path))

                # Request document symbols
                symbols = server.request_document_symbol(str(file_path))

                return self._format_document_symbols(symbols, depth)

        except Exception as e:
            logger.error(f"Document symbol request failed: {e}")
            return {"error": str(e)}

    def _convert_workspace_symbol(
        self,
        sym: Any,
        include_body: bool = False,
    ) -> dict[str, Any] | None:
        """Convert multilspy workspace symbol to C4 format.

        Args:
            sym: UnifiedSymbolInformation from multilspy.
            include_body: Whether to include the symbol body.

        Returns:
            Dictionary with symbol information or None if conversion fails.
        """
        try:
            # Extract location info
            location = sym.location if hasattr(sym, "location") else None
            if not location:
                return None

            # Build name_path
            container = getattr(sym, "container_name", "") or ""
            name = getattr(sym, "name", "")
            name_path = f"{container}/{name}" if container else name

            result = {
                "name": name,
                "type": self._kind_to_type(getattr(sym, "kind", None)),
                "name_path": name_path,
                "location": {
                    "file_path": str(self._extract_uri(location)),
                    "line": self._extract_line(location),
                    "column": self._extract_column(location),
                },
            }

            if include_body:
                # Body retrieval would require reading the file
                # and extracting the symbol's range
                result["body"] = None  # TODO: Implement if needed

            return result

        except Exception as e:
            logger.debug(f"Failed to convert symbol: {e}")
            return None

    def _format_document_symbols(
        self,
        symbols: list[Any],
        depth: int,
    ) -> dict[str, Any]:
        """Format document symbols into grouped structure.

        Args:
            symbols: List of DocumentSymbol or SymbolInformation.
            depth: Depth of children to include.

        Returns:
            Dictionary with symbols grouped by kind.
        """
        result: dict[str, list[dict[str, Any]]] = {
            "classes": [],
            "functions": [],
            "methods": [],
            "variables": [],
            "constants": [],
            "other": [],
        }

        def process_symbol(sym: Any, current_depth: int = 0) -> dict[str, Any] | None:
            try:
                name = getattr(sym, "name", "")
                kind = getattr(sym, "kind", None)
                kind_str = self._kind_to_type(kind)

                symbol_dict: dict[str, Any] = {
                    "name": name,
                    "type": kind_str,
                    "line": self._extract_symbol_line(sym),
                }

                # Process children if depth allows
                children = getattr(sym, "children", []) or []
                if children and current_depth < depth:
                    symbol_dict["children"] = [c for c in (process_symbol(child, current_depth + 1) for child in children) if c is not None]

                return symbol_dict

            except Exception as e:
                logger.debug(f"Failed to process symbol: {e}")
                return None

        for sym in symbols:
            processed = process_symbol(sym)
            if processed:
                kind_str = processed.get("type", "other")
                if kind_str == "class":
                    result["classes"].append(processed)
                elif kind_str == "function":
                    result["functions"].append(processed)
                elif kind_str == "method":
                    result["methods"].append(processed)
                elif kind_str in ("variable", "field"):
                    result["variables"].append(processed)
                elif kind_str == "constant":
                    result["constants"].append(processed)
                else:
                    result["other"].append(processed)

        # Remove empty categories
        return {k: v for k, v in result.items() if v}

    def _matches_pattern(self, symbol: dict[str, Any], pattern: str) -> bool:
        """Check if symbol matches the name_path pattern.

        Args:
            symbol: Symbol dictionary.
            pattern: Pattern like "MyClass" or "MyClass/method".

        Returns:
            True if the symbol matches the pattern.
        """
        name_path = symbol.get("name_path", "")
        name = symbol.get("name", "")

        # Simple matching: pattern is suffix of name_path or equals name
        if "/" in pattern:
            return name_path.endswith(pattern) or pattern in name_path
        else:
            return name == pattern or pattern in name

    def _kind_to_type(self, kind: Any) -> str:
        """Convert LSP SymbolKind to string type.

        Args:
            kind: SymbolKind enum value.

        Returns:
            String representation of the symbol type.
        """
        if kind is None:
            return "unknown"

        # SymbolKind values (LSP spec)
        kind_map = {
            1: "file",
            2: "module",
            3: "namespace",
            4: "package",
            5: "class",
            6: "method",
            7: "property",
            8: "field",
            9: "constructor",
            10: "enum",
            11: "interface",
            12: "function",
            13: "variable",
            14: "constant",
            15: "string",
            16: "number",
            17: "boolean",
            18: "array",
            19: "object",
            20: "key",
            21: "null",
            22: "enum_member",
            23: "struct",
            24: "event",
            25: "operator",
            26: "type_parameter",
        }

        kind_value = kind.value if hasattr(kind, "value") else kind
        return kind_map.get(kind_value, "unknown")

    def _extract_uri(self, location: Any) -> Path:
        """Extract file path from LSP location."""
        if hasattr(location, "uri"):
            uri = location.uri
            if uri.startswith("file://"):
                return Path(uri[7:])
            return Path(uri)
        return Path("")

    def _extract_line(self, location: Any) -> int:
        """Extract line number from LSP location."""
        if hasattr(location, "range"):
            return location.range.start.line
        return 0

    def _extract_column(self, location: Any) -> int:
        """Extract column number from LSP location."""
        if hasattr(location, "range"):
            return location.range.start.character
        return 0

    def _extract_symbol_line(self, sym: Any) -> int:
        """Extract line number from document symbol."""
        if hasattr(sym, "range"):
            return sym.range.start.line
        if hasattr(sym, "selection_range"):
            return sym.selection_range.start.line
        if hasattr(sym, "location"):
            return self._extract_line(sym.location)
        return 0

    def cleanup_idle_servers(self) -> int:
        """Shutdown servers that have been idle for longer than idle_timeout.

        Returns:
            Number of servers shut down.
        """
        import time

        now = time.time()
        to_remove: list[str] = []

        with self._global_lock:
            for lang, info in self._servers.items():
                if now - info.last_used > self.idle_timeout:
                    to_remove.append(lang)

            for lang in to_remove:
                info = self._servers.pop(lang)
                logger.info(f"Shutting down idle {lang} server (unused for {self.idle_timeout}s)")

        return len(to_remove)

    def get_stats(self) -> dict[str, Any]:
        """Get statistics about active servers.

        Returns:
            Dictionary with server statistics.
        """
        import time

        now = time.time()
        stats: dict[str, Any] = {
            "active_servers": len(self._servers),
            "servers": {},
        }

        with self._global_lock:
            for lang, info in self._servers.items():
                stats["servers"][lang] = {
                    "started": info.started,
                    "request_count": info.request_count,
                    "idle_seconds": int(now - info.last_used),
                }

        return stats

    def shutdown(self) -> None:
        """Shutdown all LSP servers."""
        with self._global_lock:
            for info in self._servers.values():
                try:
                    if info.started and info.server:
                        # Server shutdown is handled by context manager
                        pass
                except Exception as e:
                    logger.warning(f"Error shutting down {info.language} server: {e}")

            self._servers.clear()
            logger.info("All LSP servers shut down")

    def __enter__(self) -> "MultilspyProvider":
        """Context manager entry."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Context manager exit - shutdown servers."""
        self.shutdown()


# Convenience function for MCP tool interface
def find_symbol_multilspy(
    name_path_pattern: str,
    relative_path: str = "",
    include_body: bool = False,
    project_path: str | None = None,
    timeout: int = 30,
) -> list[dict[str, Any]]:
    """MCP tool wrapper for find_symbol using multilspy.

    Args:
        name_path_pattern: Pattern to match (e.g., "MyClass/my_method").
        relative_path: Restrict search to this path.
        include_body: Include symbol body in results.
        project_path: Project root path.
        timeout: Maximum execution time in seconds.

    Returns:
        List of symbol info dictionaries.
    """
    if not MULTILSPY_AVAILABLE:
        return []

    try:
        with MultilspyProvider(
            project_path=project_path or ".",
            timeout=timeout,
        ) as provider:
            return provider.find_symbol(
                name_path_pattern=name_path_pattern,
                relative_path=relative_path,
                include_body=include_body,
            )
    except Exception as e:
        logger.error(f"find_symbol_multilspy failed: {e}")
        return []


def get_symbols_overview_multilspy(
    relative_path: str,
    depth: int = 0,
    project_path: str | None = None,
    timeout: int = 30,
) -> dict[str, Any]:
    """MCP tool wrapper for get_symbols_overview using multilspy.

    Args:
        relative_path: Path to the file relative to project root.
        depth: Depth of children to include.
        project_path: Project root path.
        timeout: Maximum execution time in seconds.

    Returns:
        Dictionary with symbols grouped by kind.
    """
    if not MULTILSPY_AVAILABLE:
        return {"error": "multilspy not available"}

    try:
        with MultilspyProvider(
            project_path=project_path or ".",
            timeout=timeout,
        ) as provider:
            return provider.get_symbols_overview(
                relative_path=relative_path,
                depth=depth,
            )
    except Exception as e:
        logger.error(f"get_symbols_overview_multilspy failed: {e}")
        return {"error": str(e)}
