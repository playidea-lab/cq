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
    from multilspy.multilspy_logger import MultilspyLogger

    MULTILSPY_AVAILABLE = True
except ImportError:
    MULTILSPY_AVAILABLE = False
    SyncLanguageServer = None  # type: ignore[assignment, misc]
    MultilspyConfig = None  # type: ignore[assignment, misc]
    MultilspyLogger = None  # type: ignore[assignment, misc]

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


# Language server installation guides
# Used to provide helpful messages when a language server is not available
LANGUAGE_SERVER_INSTALL_GUIDE: dict[str, dict[str, str | list[str]]] = {
    "python": {
        "package": "jedi-language-server or pylsp",
        "commands": ["uv add jedi-language-server", "# or: uv add python-lsp-server"],
        "note": "Jedi fallback is available for Python",
    },
    "typescript": {
        "package": "typescript-language-server",
        "commands": ["npm install -g typescript-language-server typescript"],
        "requires": "Node.js",
    },
    "javascript": {
        "package": "typescript-language-server",
        "commands": ["npm install -g typescript-language-server typescript"],
        "requires": "Node.js",
    },
    "go": {
        "package": "gopls",
        "commands": ["go install golang.org/x/tools/gopls@latest"],
        "requires": "Go toolchain",
    },
    "rust": {
        "package": "rust-analyzer",
        "commands": ["rustup component add rust-analyzer"],
        "requires": "Rust toolchain (rustup)",
    },
    "java": {
        "package": "jdtls (Eclipse JDT Language Server)",
        "commands": ["# Install via your IDE or package manager"],
        "requires": "JDK 11+",
    },
    "ruby": {
        "package": "solargraph",
        "commands": ["gem install solargraph"],
        "requires": "Ruby",
    },
    "csharp": {
        "package": "OmniSharp",
        "commands": ["# Install via your IDE or dotnet tool"],
        "requires": ".NET SDK",
    },
}


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
                    # multilspy requires its own logger type, not Python's logging.Logger
                    multilspy_logger = MultilspyLogger()
                    server = SyncLanguageServer.create(
                        config,
                        multilspy_logger,
                        str(self.project_path),
                        timeout=self.timeout,
                    )
                    self._servers[language] = LanguageServerInfo(
                        server=server,
                        language=language,
                    )
                    logger.info(f"Created LSP server for {language}")
                except Exception as e:
                    # Provide helpful installation guide
                    guide = LANGUAGE_SERVER_INSTALL_GUIDE.get(language, {})
                    install_hint = ""
                    if guide:
                        commands = guide.get("commands", [])
                        requires = guide.get("requires", "")
                        install_hint = f" Install with: {commands[0] if commands else 'see documentation'}"
                        if requires:
                            install_hint += f" (requires {requires})"

                    error_msg = f"LSP server not available for {language}.{install_hint}"
                    logger.warning(error_msg)
                    raise RuntimeError(error_msg) from e

            # Update usage tracking
            import time

            info = self._servers[language]
            info.last_used = time.time()
            info.request_count += 1

            return info.server

    def diagnose_language_servers(self) -> dict[str, dict[str, Any]]:
        """Diagnose available language servers.

        Returns a dictionary with status and installation info for each
        supported language.

        Returns:
            Dictionary mapping language to diagnosis info:
            - available: bool - whether server is available
            - error: str | None - error message if not available
            - install_guide: dict - installation instructions
        """
        results: dict[str, dict[str, Any]] = {}

        for language in self.LANGUAGE_MAP.values():
            if language in results:
                continue  # Skip duplicates (e.g., .js and .jsx both map to javascript)

            diagnosis: dict[str, Any] = {
                "available": False,
                "error": None,
                "install_guide": LANGUAGE_SERVER_INSTALL_GUIDE.get(language, {}),
            }

            try:
                # Try to create server (will fail if not installed)
                self._get_server(language)
                diagnosis["available"] = True
                # Clean up immediately
                if language in self._servers:
                    del self._servers[language]
            except RuntimeError as e:
                diagnosis["error"] = str(e)
            except Exception as e:
                diagnosis["error"] = f"Unexpected error: {e}"

            results[language] = diagnosis

        return results

    def get_install_guide(self, language: str) -> dict[str, Any]:
        """Get installation guide for a specific language server.

        Args:
            language: Language identifier (e.g., "python", "typescript").

        Returns:
            Dictionary with installation instructions, or empty dict if unknown.
        """
        return dict(LANGUAGE_SERVER_INSTALL_GUIDE.get(language, {}))

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
                # Directory - query Python only to avoid timeout
                # Other languages can be slow to start and often not needed
                languages = ["python"]
        else:
            # No restriction - query Python by default (most common)
            languages = ["python"]

        for lang in languages:
            if not lang:
                continue

            try:
                server = self._get_server(lang)
                lang_results: list[dict] = []
                try:
                    with server.start_server():
                        symbols = server.request_workspace_symbol(query) or []
                        for sym in symbols:
                            result = self._convert_workspace_symbol(sym, include_body)
                            if result and self._matches_pattern(result, name_path_pattern):
                                lang_results.append(result)
                except Exception as shutdown_err:
                    if lang_results:
                        logger.debug(f"LSP shutdown error (ignored, got {len(lang_results)} symbols): {shutdown_err}")
                    else:
                        raise
                results.extend(lang_results)

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
            symbols = None
            try:
                with server.start_server():
                    server.open_file(str(file_path))
                    symbols = server.request_document_symbols(str(file_path))
            except Exception as shutdown_err:
                # Ignore psutil/process cleanup errors during server shutdown
                # if we already got symbols successfully
                if symbols is not None:
                    logger.debug(f"LSP server shutdown error (ignored, symbols ok): {shutdown_err}")
                else:
                    raise

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
                    "end_line": self._extract_end_line(location),
                    "end_column": self._extract_end_column(location),
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

        # multilspy may return a tuple (list, ...) for some LSP servers (e.g. rust-analyzer).
        # Normalise to a flat list of symbol items.
        if isinstance(symbols, tuple):
            flat: list[Any] = []
            for part in symbols:
                if isinstance(part, list):
                    flat.extend(part)
            symbols = flat

        def _get(sym: Any, key: str, default: Any = None) -> Any:
            """Uniform field access for both object-style and dict-style symbols."""
            if isinstance(sym, dict):
                return sym.get(key, default)
            return getattr(sym, key, default)

        def process_symbol(sym: Any, current_depth: int = 0) -> dict[str, Any] | None:
            try:
                name = _get(sym, "name", "")
                kind = _get(sym, "kind", None)
                kind_str = self._kind_to_type(kind)

                symbol_dict: dict[str, Any] = {
                    "name": name,
                    "type": kind_str,
                    "line": self._extract_symbol_line(sym),
                }

                # Process children if depth allows
                children = _get(sym, "children", []) or []
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
        """Extract line number from LSP location.

        Defensively handles None values at any level of the location hierarchy.
        """
        try:
            if location is None:
                return 0
            range_obj = getattr(location, "range", None)
            if range_obj is None:
                return 0
            start = getattr(range_obj, "start", None)
            if start is None:
                return 0
            return getattr(start, "line", 0) or 0
        except Exception:
            return 0

    def _extract_column(self, location: Any) -> int:
        """Extract column number from LSP location.

        Defensively handles None values at any level of the location hierarchy.
        """
        try:
            if location is None:
                return 0
            range_obj = getattr(location, "range", None)
            if range_obj is None:
                return 0
            start = getattr(range_obj, "start", None)
            if start is None:
                return 0
            return getattr(start, "character", 0) or 0
        except Exception:
            return 0

    def _extract_end_line(self, location: Any) -> int:
        """Extract end line number from LSP location."""
        try:
            if location is None:
                return 0
            range_obj = getattr(location, "range", None)
            if range_obj is None:
                return 0
            end = getattr(range_obj, "end", None)
            if end is None:
                return 0
            return getattr(end, "line", 0) or 0
        except Exception:
            return 0

    def _extract_end_column(self, location: Any) -> int:
        """Extract end column number from LSP location."""
        try:
            if location is None:
                return 0
            range_obj = getattr(location, "range", None)
            if range_obj is None:
                return 0
            end = getattr(range_obj, "end", None)
            if end is None:
                return 0
            return getattr(end, "character", 0) or 0
        except Exception:
            return 0

    def _extract_symbol_line(self, sym: Any) -> int:
        """Extract line number from document symbol.

        Tries multiple attributes in order of preference.
        """
        def _f(obj: Any, key: str, default: Any = None) -> Any:
            if isinstance(obj, dict):
                return obj.get(key, default)
            return getattr(obj, key, default)

        try:
            if sym is None:
                return 0
            # Try range first
            range_obj = _f(sym, "range")
            if range_obj is not None:
                start = _f(range_obj, "start")
                if start is not None:
                    return _f(start, "line", 0) or 0
            # Try selection_range
            sel_range = _f(sym, "selection_range") or _f(sym, "selectionRange")
            if sel_range is not None:
                start = _f(sel_range, "start")
                if start is not None:
                    return _f(start, "line", 0) or 0
            # Try location
            location = _f(sym, "location")
            if location is not None:
                return self._extract_line(location)
            return 0
        except Exception:
            return 0

    def find_references(
        self,
        file_path: str,
        line: int,
        column: int,
    ) -> list[dict[str, Any]]:
        """Find all references to the symbol at the given position.

        Uses multilspy's request_references (LSP textDocument/references).

        Args:
            file_path: Absolute or project-relative path to the file.
            line: Line number (0-indexed, LSP convention).
            column: Column number (0-indexed, LSP convention).

        Returns:
            List of reference locations, each with file_path, line, column,
            end_line, end_column.
        """
        abs_path = Path(file_path)
        if not abs_path.is_absolute():
            abs_path = self.project_path / file_path

        language = self._detect_language(abs_path)
        if not language:
            return []

        try:
            server = self._get_server(language)
            # multilspy expects relative path
            rel_path = str(abs_path.relative_to(self.project_path))

            locations = []
            try:
                with server.start_server():
                    server.open_file(rel_path)
                    locations = server.request_references(rel_path, line, column) or []
            except Exception as shutdown_err:
                if locations:
                    logger.debug(f"LSP shutdown error (ignored, got refs): {shutdown_err}")
                else:
                    raise

            results: list[dict[str, Any]] = []
            for loc in locations:
                ref_path = loc.get("absolutePath") or loc.get("uri", "")
                if ref_path.startswith("file://"):
                    ref_path = ref_path[7:]

                range_obj = loc.get("range", {})
                start = range_obj.get("start", {})
                end = range_obj.get("end", {})

                results.append({
                    "file_path": ref_path,
                    "line": start.get("line", 0),
                    "column": start.get("character", 0),
                    "end_line": end.get("line", 0),
                    "end_column": end.get("character", 0),
                })

            return results

        except Exception as e:
            logger.warning(f"LSP find_references failed for {language}: {e}")
            return []

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
        """Shutdown all LSP servers.

        Note: In the current architecture, servers are managed via context managers
        (start_server()) on each request. This method clears the server registry
        and logs the shutdown. If servers were kept running persistently, this
        method would need to explicitly stop them.
        """
        with self._global_lock:
            server_count = len(self._servers)
            languages = list(self._servers.keys())

            for lang, info in self._servers.items():
                try:
                    if info.server:
                        # Currently servers are ephemeral (started/stopped per request)
                        # If we move to persistent servers, add explicit shutdown here:
                        # - Stop event loop if running
                        # - Call server cleanup methods
                        logger.debug(f"Clearing {lang} server registry entry")
                except Exception as e:
                    logger.warning(f"Error cleaning up {info.language} server: {e}")

            self._servers.clear()
            if server_count > 0:
                logger.info(f"Cleared {server_count} LSP server(s): {languages}")

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


def find_references_multilspy(
    file_path: str,
    line: int,
    column: int,
    project_path: str | None = None,
    timeout: int = 30,
) -> list[dict[str, Any]]:
    """MCP tool wrapper for find_references using multilspy.

    Args:
        file_path: Path to the file (absolute or relative to project root).
        line: Line number (0-indexed).
        column: Column number (0-indexed).
        project_path: Project root path.
        timeout: Maximum execution time in seconds.

    Returns:
        List of reference location dictionaries.
    """
    if not MULTILSPY_AVAILABLE:
        return []

    try:
        with MultilspyProvider(
            project_path=project_path or ".",
            timeout=timeout,
        ) as provider:
            return provider.find_references(
                file_path=file_path,
                line=line,
                column=column,
            )
    except Exception as e:
        logger.error(f"find_references_multilspy failed: {e}")
        return []
