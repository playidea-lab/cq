"""Jedi-based symbol provider for C4 LSP.

Provides semantic code analysis using Jedi for:
- Symbol search (find_symbol)
- Document symbols (get_symbols_overview)
- Workspace-wide symbol search
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING

try:
    import jedi
    import jedi.cache
    import jedi.settings

    # Disable subprocess caching to prevent recursion errors during GC cleanup
    # The InferenceStateSubprocess.__del__ method can trigger recursion errors
    # when Python's garbage collector cleans up the cache
    jedi.settings.call_signatures_validity = 0.0  # Disable signature caching
    jedi.settings.auto_import_modules = []  # Don't auto-import standard library

    # Monkey-patch InferenceStateSubprocess.__del__ to prevent GC recursion errors
    # This is necessary because Jedi's subprocess cleanup can trigger recursion
    # when Python's garbage collector runs during low recursion limit periods
    try:
        from jedi.inference.compiled.subprocess import InferenceStateSubprocess

        # Replace __del__ with a no-op to prevent GC recursion errors
        InferenceStateSubprocess.__del__ = lambda self: None  # type: ignore[method-assign]
    except (ImportError, AttributeError):
        pass  # Jedi version doesn't have this class or attribute

    def _clear_jedi_cache() -> None:
        """Clear Jedi's internal caches to prevent GC recursion errors."""
        try:
            jedi.cache.clear_time_caches()
        except Exception:
            pass

    JEDI_AVAILABLE = True
except ImportError:
    JEDI_AVAILABLE = False
    jedi = None  # type: ignore[assignment]

if TYPE_CHECKING:
    from jedi.api.classes import Name

from .cache import SymbolCache, get_symbol_cache

logger = logging.getLogger(__name__)


# Operation-specific timeout configuration (in seconds)
# These are tiered based on expected operation complexity
TIMEOUT_CONFIG = {
    "completion": 0.5,       # Fast: autocomplete suggestions
    "definition": 2.0,       # Medium: go to definition
    "references": 5.0,       # Slower: find all references
    "workspace_symbol": 10.0, # Slowest: search across workspace
    "document_symbols": 3.0,  # Medium: symbols in current file
    "find_symbol": 30.0,     # Variable: depends on scope
}

def get_timeout(operation: str, default: float = 30.0) -> float:
    """Get the timeout for a specific operation.

    Args:
        operation: The operation name (e.g., "completion", "references")
        default: Default timeout if operation not found

    Returns:
        Timeout in seconds
    """
    return TIMEOUT_CONFIG.get(operation, default)


class SymbolType(str, Enum):
    """Symbol types for categorization."""

    CLASS = "class"
    FUNCTION = "function"
    METHOD = "method"
    PROPERTY = "property"
    VARIABLE = "variable"
    CONSTANT = "constant"
    MODULE = "module"
    PARAMETER = "parameter"
    UNKNOWN = "unknown"


class LSPSymbolKind(int, Enum):
    """LSP Symbol kinds (subset of lsprotocol.types.SymbolKind)."""

    FILE = 1
    MODULE = 2
    NAMESPACE = 3
    PACKAGE = 4
    CLASS = 5
    METHOD = 6
    PROPERTY = 7
    FIELD = 8
    CONSTRUCTOR = 9
    ENUM = 10
    INTERFACE = 11
    FUNCTION = 12
    VARIABLE = 13
    CONSTANT = 14
    STRING = 15
    NUMBER = 16
    BOOLEAN = 17
    ARRAY = 18


@dataclass
class SymbolLocation:
    """Location information for a symbol."""

    file_path: str
    line: int
    column: int
    end_line: int | None = None
    end_column: int | None = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "file_path": self.file_path,
            "line": self.line,
            "column": self.column,
            "end_line": self.end_line,
            "end_column": self.end_column,
        }


@dataclass
class SymbolInfo:
    """Information about a symbol."""

    name: str
    kind: SymbolType
    location: SymbolLocation
    qualified_name: str | None = None
    parent_name: str | None = None
    signature: str | None = None
    docstring: str | None = None

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "kind": self.kind.value,
            "location": self.location.to_dict(),
            "qualified_name": self.qualified_name,
            "parent_name": self.parent_name,
            "signature": self.signature,
            "docstring": self.docstring,
        }


def _jedi_type_to_symbol_type(jedi_type: str, parent_type: str | None = None) -> SymbolType:
    """Convert jedi type string to SymbolType.

    Jedi returns 'function' for both functions and methods. We distinguish
    methods by checking if they have a class parent.
    """
    type_mapping = {
        "class": SymbolType.CLASS,
        "function": SymbolType.FUNCTION,  # Will be overridden for methods
        "method": SymbolType.METHOD,
        "property": SymbolType.PROPERTY,
        "module": SymbolType.MODULE,
        "param": SymbolType.PARAMETER,
        "statement": SymbolType.VARIABLE,
        "instance": SymbolType.VARIABLE,
    }

    symbol_type = type_mapping.get(jedi_type, SymbolType.UNKNOWN)

    # If jedi says 'function' but parent is a class, it's a method
    if jedi_type == "function" and parent_type == "class":
        return SymbolType.METHOD

    return symbol_type


def _symbol_type_to_lsp_kind(symbol_type: SymbolType) -> LSPSymbolKind:
    """Convert SymbolType to LSP SymbolKind."""
    mapping = {
        SymbolType.CLASS: LSPSymbolKind.CLASS,
        SymbolType.FUNCTION: LSPSymbolKind.FUNCTION,
        SymbolType.METHOD: LSPSymbolKind.METHOD,
        SymbolType.PROPERTY: LSPSymbolKind.PROPERTY,
        SymbolType.VARIABLE: LSPSymbolKind.VARIABLE,
        SymbolType.CONSTANT: LSPSymbolKind.CONSTANT,
        SymbolType.MODULE: LSPSymbolKind.MODULE,
        SymbolType.PARAMETER: LSPSymbolKind.VARIABLE,
    }
    return mapping.get(symbol_type, LSPSymbolKind.VARIABLE)


class JediSymbolProvider:
    """Jedi-based symbol provider for semantic code analysis.

    Provides:
    - find_symbol: Find symbols by name path pattern
    - get_symbols_overview: Get all symbols in a file
    - workspace_symbols: Search symbols across the workspace

    Uses a two-stage cache for performance:
    - Stage 1: Raw symbols from Jedi (content-hash based)
    - Stage 2: Processed symbols for MCP output
    """

    def __init__(
        self,
        project_path: str | Path | None = None,
        cache: SymbolCache | None = None,
    ) -> None:
        """Initialize the Jedi symbol provider.

        Args:
            project_path: Root path for the project (for proper import resolution)
            cache: Optional symbol cache (uses global cache if not provided)
        """
        if not JEDI_AVAILABLE:
            raise ImportError(
                "jedi is required for symbol search. " "Install with: uv add jedi"
            )

        self._project_path = Path(project_path) if project_path else None
        self._project: jedi.Project | None = None
        self._cache = cache or get_symbol_cache()

        if self._project_path:
            try:
                # Disable caching at module level to prevent GC recursion errors
                jedi.settings.cache_directory = None  # No disk cache

                # Disable smart_sys_path to prevent Jedi from following
                # external library imports (causes recursion issues)
                self._project = jedi.Project(
                    path=str(self._project_path),
                    added_sys_path=[],  # Don't add extra paths
                    smart_sys_path=False,  # Don't analyze sys.path
                )
                logger.info(f"Jedi project initialized at {self._project_path}")
            except Exception as e:
                logger.warning(f"Failed to create jedi project: {e}")

    def _get_script(
        self, source: str, path: str | None = None
    ) -> "jedi.Script":
        """Create a Jedi Script for the given source."""
        path_str = str(path) if path else None
        return jedi.Script(code=source, path=path_str, project=self._project)

    def _name_to_symbol_info(
        self, name: "Name", parent_type: str | None = None
    ) -> SymbolInfo | None:
        """Convert a Jedi Name to SymbolInfo."""
        try:
            # Get location
            if name.module_path is None:
                return None

            line = name.line or 1
            column = name.column or 0

            location = SymbolLocation(
                file_path=str(name.module_path),
                line=line,
                column=column,
            )

            # Determine symbol type
            jedi_type = name.type
            symbol_type = _jedi_type_to_symbol_type(jedi_type, parent_type)

            # Get signature for functions/methods
            signature = None
            if symbol_type in (SymbolType.FUNCTION, SymbolType.METHOD):
                try:
                    signatures = name.get_signatures()
                    if signatures:
                        signature = str(signatures[0])
                except Exception:
                    pass

            # Get docstring
            docstring = None
            try:
                docstring = name.docstring(raw=True)
            except Exception:
                pass

            return SymbolInfo(
                name=name.name,
                kind=symbol_type,
                location=location,
                qualified_name=name.full_name,
                parent_name=parent_type,
                signature=signature,
                docstring=docstring if docstring else None,
            )
        except Exception as e:
            logger.debug(f"Failed to convert name to symbol: {e}")
            return None

    def find_symbol(
        self,
        name_path_pattern: str,
        source: str | None = None,
        file_path: str | None = None,
        include_body: bool = False,
    ) -> list[SymbolInfo]:
        """Find symbols matching the name path pattern.

        Name path patterns:
        - Simple name: "method_name" - matches any symbol with that name
        - Relative path: "ClassName/method_name" - matches method in class
        - Absolute path: "/ClassName/method_name" - exact match from root

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
            source: Source code to search in (if not provided, searches project)
            file_path: File path for context
            include_body: Whether to include symbol body (not implemented yet)

        Returns:
            List of matching SymbolInfo objects
        """
        if not JEDI_AVAILABLE:
            return []

        results: list[SymbolInfo] = []

        # Parse the pattern
        pattern_parts = name_path_pattern.strip("/").split("/")
        is_absolute = name_path_pattern.startswith("/")
        target_name = pattern_parts[-1]
        parent_names = pattern_parts[:-1] if len(pattern_parts) > 1 else []

        # If source is provided, search in that file
        if source and file_path:
            script = self._get_script(source, file_path)
            try:
                names = script.get_names(all_scopes=True, definitions=True)
                for name in names:
                    # Filter by target name
                    if name.name != target_name:
                        continue

                    # Check parent chain if pattern has parents
                    if parent_names:
                        parent = name.parent()
                        parent_type = parent.type if parent else None
                        parent_name_match = parent and parent.name == parent_names[-1]

                        if is_absolute:
                            # Must match full parent chain
                            current = name.parent()
                            matched = True
                            for expected_parent in reversed(parent_names):
                                if not current or current.name != expected_parent:
                                    matched = False
                                    break
                                current = current.parent()
                            if not matched:
                                continue
                        else:
                            # Just check immediate parent
                            if not parent_name_match:
                                continue
                    else:
                        parent = name.parent()
                        parent_type = parent.type if parent else None

                    # Convert to SymbolInfo
                    symbol = self._name_to_symbol_info(name, parent_type)
                    if symbol:
                        results.append(symbol)

            except RecursionError:
                # Jedi can hit recursion limits on complex import chains
                logger.debug(f"Recursion limit hit for {file_path}, skipping")
            except Exception as e:
                # Log at debug level to avoid log spam during workspace search
                logger.debug(f"Error searching symbols in {file_path}: {e}")

        # If no source, search in project files
        elif self._project_path:
            results = self._search_workspace(target_name, parent_names, is_absolute)

        return results

    def _search_workspace(
        self,
        target_name: str,
        parent_names: list[str],
        is_absolute: bool,
        max_files: int = 500,
        max_file_lines: int = 5000,
        max_consecutive_errors: int = 10,
    ) -> list[SymbolInfo]:
        """Search for symbols across workspace files.

        Args:
            target_name: Symbol name to find
            parent_names: Parent path components
            is_absolute: Whether pattern is absolute
            max_files: Maximum files to search (prevents runaway)
            max_file_lines: Skip files larger than this
            max_consecutive_errors: Stop after this many consecutive errors
        """
        results: list[SymbolInfo] = []

        if not self._project_path:
            return results

        files_searched = 0
        consecutive_errors = 0
        skip_dirs = {"__pycache__", ".git", "node_modules", ".venv", "venv", ".tox", "build", "dist", ".eggs", "*.egg-info"}

        # Search Python files
        for py_file in self._project_path.rglob("*.py"):
            # Limit total files searched
            if files_searched >= max_files:
                logger.debug(f"Reached max files limit ({max_files}), stopping search")
                break

            # Skip common directories
            if any(part in py_file.parts for part in skip_dirs):
                continue

            try:
                # Check file size first (quick check)
                stat = py_file.stat()
                if stat.st_size > max_file_lines * 100:  # Rough estimate: 100 bytes/line
                    logger.debug(f"Skipping large file: {py_file}")
                    continue

                source = py_file.read_text(encoding="utf-8")

                # More accurate line count check
                if source.count('\n') > max_file_lines:
                    logger.debug(f"Skipping file with too many lines: {py_file}")
                    continue

                files_searched += 1

                # Search for symbols in this file
                # Note: We don't lower recursion limit as Jedi needs the default limit
                # Protection is via: 1) timeout, 2) consecutive error counter
                file_results = self.find_symbol(
                    f"{'/' if is_absolute else ''}{'/'.join(parent_names + [target_name])}",
                    source=source,
                    file_path=str(py_file),
                )
                results.extend(file_results)
                consecutive_errors = 0  # Reset on success

            except RecursionError:
                consecutive_errors += 1
                logger.debug(f"Recursion limit hit searching {py_file}")
                if consecutive_errors >= max_consecutive_errors:
                    logger.warning(
                        f"Too many consecutive errors ({consecutive_errors}), stopping search"
                    )
                    break
            except Exception as e:
                consecutive_errors += 1
                logger.debug(f"Error searching {py_file}: {e}")
                if consecutive_errors >= max_consecutive_errors:
                    logger.warning(
                        f"Too many consecutive errors ({consecutive_errors}), stopping search"
                    )
                    break

        # Clear Jedi cache to prevent GC recursion errors
        if JEDI_AVAILABLE:
            _clear_jedi_cache()

        return results

    def get_symbols_overview(
        self,
        file_path: str,
        source: str | None = None,
        depth: int = 0,
        use_cache: bool = True,
    ) -> list[SymbolInfo]:
        """Get an overview of symbols in a file.

        Uses a two-stage cache for performance:
        1. Stage 1 (raw): Cached symbols from Jedi (content-hash based)
        2. Stage 2 (processed): Ready-to-use SymbolInfo objects

        Args:
            file_path: Path to the file
            source: Source code (if not provided, reads from file)
            depth: Depth of children to include (0 = top-level only)
            use_cache: Whether to use the symbol cache (default: True)

        Returns:
            List of SymbolInfo objects
        """
        if not JEDI_AVAILABLE:
            return []

        if source is None:
            try:
                source = Path(file_path).read_text(encoding="utf-8")
            except Exception as e:
                logger.warning(f"Failed to read file {file_path}: {e}")
                return []

        # Check cache if enabled
        content_hash = self._cache.compute_hash(source) if use_cache else ""

        if use_cache:
            # Try to get from cache (stage 2: processed)
            cached = self._cache.get(file_path, content_hash, stage="processed")
            if cached is not None:
                # Reconstruct SymbolInfo objects from cached dicts
                return self._dicts_to_symbols(cached, depth)

        # Cache miss - compute symbols
        results: list[SymbolInfo] = []
        script = self._get_script(source, file_path)

        try:
            # Get all names with definitions
            names = script.get_names(all_scopes=True, definitions=True)

            # Build parent-child relationships
            seen_names: set[tuple[str, int, int]] = set()

            for name in names:
                # Skip duplicates
                key = (name.name, name.line or 0, name.column or 0)
                if key in seen_names:
                    continue
                seen_names.add(key)

                # Get parent info
                parent = name.parent()
                parent_type = parent.type if parent else None
                parent_name = parent.name if parent else None

                # For depth=0, only include top-level (no class/function parent)
                if depth == 0:
                    if parent_type in ("class", "function"):
                        continue

                symbol = self._name_to_symbol_info(name, parent_type)
                if symbol:
                    symbol.parent_name = parent_name
                    results.append(symbol)

            # Store in cache
            if use_cache and results:
                raw_dicts = [s.to_dict() for s in results]
                self._cache.put(
                    file_path,
                    content_hash,
                    raw_symbols=raw_dicts,
                    processed_symbols=raw_dicts,
                )

        except Exception as e:
            logger.warning(f"Error getting symbols overview: {e}")

        return results

    def _dicts_to_symbols(
        self, cached: list[dict], depth: int
    ) -> list[SymbolInfo]:
        """Convert cached dictionaries back to SymbolInfo objects.

        Args:
            cached: List of cached symbol dictionaries
            depth: Depth filter to apply

        Returns:
            List of SymbolInfo objects
        """
        results: list[SymbolInfo] = []
        for d in cached:
            # Filter by depth if needed
            if depth == 0 and d.get("parent_name"):
                # Skip nested symbols for depth=0
                parent_kind = d.get("parent_kind")
                if parent_kind in ("class", "function"):
                    continue

            loc = d.get("location", {})
            location = SymbolLocation(
                file_path=loc.get("file_path", ""),
                line=loc.get("line", 0),
                column=loc.get("column", 0),
                end_line=loc.get("end_line"),
                end_column=loc.get("end_column"),
            )

            kind_str = d.get("kind", "unknown")
            try:
                kind = SymbolType(kind_str)
            except ValueError:
                kind = SymbolType.UNKNOWN

            symbol = SymbolInfo(
                name=d.get("name", ""),
                kind=kind,
                location=location,
                qualified_name=d.get("qualified_name"),
                parent_name=d.get("parent_name"),
                signature=d.get("signature"),
                docstring=d.get("docstring"),
            )
            results.append(symbol)

        return results

    def workspace_symbols(
        self,
        query: str,
        max_results: int = 100,
    ) -> list[SymbolInfo]:
        """Search for symbols across the workspace.

        Args:
            query: Search query (partial name match)
            max_results: Maximum number of results

        Returns:
            List of matching SymbolInfo objects
        """
        if not JEDI_AVAILABLE or not self._project_path:
            return []

        results: list[SymbolInfo] = []
        query_lower = query.lower()

        for py_file in self._project_path.rglob("*.py"):
            if len(results) >= max_results:
                break

            # Skip common directories
            if any(
                part in py_file.parts
                for part in ["__pycache__", ".git", "node_modules", ".venv", "venv"]
            ):
                continue

            try:
                source = py_file.read_text(encoding="utf-8")
                script = self._get_script(source, str(py_file))
                names = script.get_names(all_scopes=True, definitions=True)

                for name in names:
                    if len(results) >= max_results:
                        break

                    # Filter by query (case-insensitive partial match)
                    if query_lower not in name.name.lower():
                        continue

                    parent = name.parent()
                    parent_type = parent.type if parent else None
                    symbol = self._name_to_symbol_info(name, parent_type)
                    if symbol:
                        results.append(symbol)

            except Exception as e:
                logger.debug(f"Error searching {py_file}: {e}")

        return results


# Convenience functions for MCP tool interface
def find_symbol_mcp(
    name_path_pattern: str,
    relative_path: str = "",
    include_body: bool = False,
    project_path: str | None = None,
    timeout: float | None = None,
    max_file_lines: int = 10000,
) -> list[dict]:
    """MCP tool wrapper for find_symbol.

    Args:
        name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
        relative_path: Restrict search to this path
        include_body: Include symbol body in results
        project_path: Project root path
        timeout: Maximum execution time in seconds (default from TIMEOUT_CONFIG)
        max_file_lines: Skip files larger than this (default: 10000)

    Returns:
        List of symbol info dictionaries
    """
    # Use tiered timeout: workspace search vs single file
    if timeout is None:
        timeout = get_timeout("workspace_symbol" if not relative_path else "find_symbol")
    import concurrent.futures

    if not JEDI_AVAILABLE:
        return []

    def _find_symbols():
        provider = JediSymbolProvider(project_path=project_path)

        if relative_path:
            full_path = Path(project_path or ".") / relative_path
            if full_path.is_file():
                # Skip large files
                line_count = sum(1 for _ in full_path.open(encoding="utf-8", errors="ignore"))
                if line_count > max_file_lines:
                    logger.warning(f"Skipping large file ({line_count} lines): {relative_path}")
                    return []

                source = full_path.read_text(encoding="utf-8")
                symbols = provider.find_symbol(
                    name_path_pattern,
                    source=source,
                    file_path=str(full_path),
                    include_body=include_body,
                )
            else:
                symbols = provider.find_symbol(name_path_pattern)
        else:
            symbols = provider.find_symbol(name_path_pattern)

        return [s.to_dict() for s in symbols]

    # Execute with timeout
    with concurrent.futures.ThreadPoolExecutor(max_workers=1) as executor:
        future = executor.submit(_find_symbols)
        try:
            return future.result(timeout=timeout)
        except concurrent.futures.TimeoutError:
            logger.error(f"find_symbol_mcp timed out after {timeout}s for pattern: {name_path_pattern}")
            return []


def get_symbols_overview_mcp(
    relative_path: str,
    depth: int = 0,
    project_path: str | None = None,
    timeout: float | None = None,
    max_file_lines: int = 10000,
) -> dict:
    """MCP tool wrapper for get_symbols_overview.

    Args:
        relative_path: Path to the file (relative to project root)
        depth: Depth of children to include
        project_path: Project root path
        timeout: Maximum execution time in seconds (default from TIMEOUT_CONFIG)
        max_file_lines: Skip files larger than this (default: 10000)

    Returns:
        Dictionary with symbols grouped by kind
    """
    # Use tiered timeout for document symbols
    if timeout is None:
        timeout = get_timeout("document_symbols")
    import concurrent.futures

    if not JEDI_AVAILABLE:
        return {"error": "jedi not available"}

    full_path = Path(project_path or ".") / relative_path

    if not full_path.exists():
        return {"error": f"File not found: {relative_path}"}

    # Skip large files
    line_count = sum(1 for _ in full_path.open(encoding="utf-8", errors="ignore"))
    if line_count > max_file_lines:
        return {"error": f"File too large ({line_count} lines). Max: {max_file_lines}"}

    def _get_overview():
        provider = JediSymbolProvider(project_path=project_path)
        symbols = provider.get_symbols_overview(str(full_path), depth=depth)

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

    # Execute with timeout
    with concurrent.futures.ThreadPoolExecutor(max_workers=1) as executor:
        future = executor.submit(_get_overview)
        try:
            return future.result(timeout=timeout)
        except concurrent.futures.TimeoutError:
            logger.error(f"get_symbols_overview_mcp timed out after {timeout}s for: {relative_path}")
            return {"error": f"Operation timed out after {timeout} seconds"}
