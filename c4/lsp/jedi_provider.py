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
from .worker_pool import TaskPriority, get_lsp_worker_pool

logger = logging.getLogger(__name__)


# Global worker pool for process-isolated Jedi operations
_jedi_worker_pool = None
_jedi_worker_pool_lock = __import__("threading").Lock()


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


# Track active timed-out operations for debugging
_active_timed_out_operations: list[str] = []
_timeout_lock = __import__("threading").Lock()


def _run_with_timeout(func, timeout: float, operation_name: str, default_result):
    """Execute a function with timeout, with proper cleanup tracking.

    Note on thread termination:
    Python's ThreadPoolExecutor cannot forcefully terminate threads.
    When a timeout occurs, the thread continues running in the background
    until it completes naturally. This is a known limitation.

    For operations that may hang indefinitely, consider:
    1. Using multiprocessing (requires picklable functions)
    2. Adding internal cancellation checks within the function
    3. Setting appropriate timeouts based on expected operation time

    Args:
        func: The function to execute
        timeout: Maximum execution time in seconds
        operation_name: Name for logging purposes
        default_result: Result to return on timeout

    Returns:
        Function result or default_result on timeout
    """
    import concurrent.futures

    with concurrent.futures.ThreadPoolExecutor(max_workers=1) as executor:
        future = executor.submit(func)
        try:
            return future.result(timeout=timeout)
        except concurrent.futures.TimeoutError:
            # Track timed out operations for debugging
            with _timeout_lock:
                _active_timed_out_operations.append(operation_name)
                # Keep only last 10 entries
                if len(_active_timed_out_operations) > 10:
                    _active_timed_out_operations.pop(0)

            logger.warning(
                f"{operation_name} timed out after {timeout}s. "
                f"Thread continues in background until completion. "
                f"Consider increasing timeout or reducing scope."
            )
            return default_result


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
        parallel: bool = True,
    ) -> list[SymbolInfo]:
        """Search for symbols across workspace files.

        Uses parallel processing for improved performance when parallel=True.

        Args:
            target_name: Symbol name to find
            parent_names: Parent path components
            is_absolute: Whether pattern is absolute
            max_files: Maximum files to search (prevents runaway)
            max_file_lines: Skip files larger than this
            max_consecutive_errors: Stop after this many consecutive errors
            parallel: Whether to use parallel processing (default: True)
        """
        if not self._project_path:
            return []

        skip_dirs = {"__pycache__", ".git", "node_modules", ".venv", "venv", ".tox", "build", "dist", ".eggs", "*.egg-info"}

        # Collect files to search
        files_to_search: list[Path] = []
        for py_file in self._project_path.rglob("*.py"):
            if len(files_to_search) >= max_files:
                logger.debug(f"Reached max files limit ({max_files}), stopping collection")
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
                files_to_search.append(py_file)
            except OSError:
                continue

        if not files_to_search:
            return []

        # Build pattern string
        pattern = f"{'/' if is_absolute else ''}{'/'.join(parent_names + [target_name])}"

        if parallel and len(files_to_search) > 10:
            # Use parallel processing for larger searches
            return self._search_files_parallel(
                files_to_search,
                pattern,
                max_file_lines,
                max_consecutive_errors,
            )
        else:
            # Use sequential processing for smaller searches
            return self._search_files_sequential(
                files_to_search,
                pattern,
                max_file_lines,
                max_consecutive_errors,
            )

    def _search_files_sequential(
        self,
        files: list[Path],
        pattern: str,
        max_file_lines: int,
        max_consecutive_errors: int,
    ) -> list[SymbolInfo]:
        """Search files sequentially (original implementation)."""
        results: list[SymbolInfo] = []
        consecutive_errors = 0

        for py_file in files:
            try:
                source = py_file.read_text(encoding="utf-8")

                # More accurate line count check
                if source.count('\n') > max_file_lines:
                    logger.debug(f"Skipping file with too many lines: {py_file}")
                    continue

                file_results = self.find_symbol(
                    pattern,
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

    def _search_files_parallel(
        self,
        files: list[Path],
        pattern: str,
        max_file_lines: int,
        max_consecutive_errors: int,
    ) -> list[SymbolInfo]:
        """Search files in parallel using LSPWorkerPool.

        Submits each file analysis as a separate task to the thread pool.
        Collects results with timeout handling per file.
        """
        import concurrent.futures

        results: list[SymbolInfo] = []
        pool = get_lsp_worker_pool()

        # Ensure pool is started
        if not pool.is_running:
            pool.start()

        # Submit all files to the pool
        futures: list[tuple[Path, concurrent.futures.Future]] = []

        for py_file in files:
            future = pool.submit(
                self._analyze_single_file,
                py_file,
                pattern,
                max_file_lines,
                priority=TaskPriority.NORMAL,
                timeout=5.0,
            )
            futures.append((py_file, future))

        # Collect results with individual timeouts
        errors = 0
        for py_file, future in futures:
            try:
                file_results = future.result(timeout=5.0)
                if file_results:
                    results.extend(file_results)
                errors = 0  # Reset on success
            except concurrent.futures.TimeoutError:
                logger.debug(f"Timeout analyzing {py_file}")
                errors += 1
            except Exception as e:
                logger.debug(f"Error analyzing {py_file}: {e}")
                errors += 1

            if errors >= max_consecutive_errors:
                logger.warning(f"Too many errors ({errors}), stopping parallel search")
                # Cancel remaining futures
                for _, remaining_future in futures:
                    remaining_future.cancel()
                break

        # Clear Jedi cache to prevent GC recursion errors
        if JEDI_AVAILABLE:
            _clear_jedi_cache()

        return results

    def _analyze_single_file(
        self,
        py_file: Path,
        pattern: str,
        max_file_lines: int,
    ) -> list[SymbolInfo]:
        """Analyze a single file for symbol matches.

        This method is designed to be called from a worker thread.
        It is stateless and thread-safe.

        Args:
            py_file: Path to Python file
            pattern: Name path pattern to search for
            max_file_lines: Skip files larger than this

        Returns:
            List of matching SymbolInfo objects
        """
        try:
            source = py_file.read_text(encoding="utf-8")

            # Line count check
            if source.count('\n') > max_file_lines:
                return []

            return self.find_symbol(
                pattern,
                source=source,
                file_path=str(py_file),
            )
        except RecursionError:
            logger.debug(f"Recursion limit hit in worker: {py_file}")
            return []
        except Exception as e:
            logger.debug(f"Worker error for {py_file}: {e}")
            return []

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
            elif full_path.is_dir():
                # Directory search: iterate through Python files in directory
                symbols = []
                py_files = list(full_path.glob("**/*.py"))[:50]  # Limit to 50 files
                for py_file in py_files:
                    try:
                        line_count = sum(1 for _ in py_file.open(encoding="utf-8", errors="ignore"))
                        if line_count > max_file_lines:
                            continue
                        source = py_file.read_text(encoding="utf-8")
                        file_symbols = provider.find_symbol(
                            name_path_pattern,
                            source=source,
                            file_path=str(py_file),
                            include_body=include_body,
                        )
                        symbols.extend(file_symbols)
                    except Exception as e:
                        logger.debug(f"Skipping {py_file}: {e}")
            else:
                symbols = provider.find_symbol(name_path_pattern)
        else:
            symbols = provider.find_symbol(name_path_pattern)

        return [s.to_dict() for s in symbols]

    # Execute with timeout using helper that tracks timed-out operations
    return _run_with_timeout(
        func=_find_symbols,
        timeout=timeout,
        operation_name=f"find_symbol_mcp(pattern={name_path_pattern})",
        default_result=[],
    )


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

    # Execute with timeout using helper that tracks timed-out operations
    return _run_with_timeout(
        func=_get_overview,
        timeout=timeout,
        operation_name=f"get_symbols_overview_mcp(path={relative_path})",
        default_result={"error": f"Operation timed out after {timeout} seconds"},
    )


# ============================================================================
# Process-Isolated Worker Pool Functions
# ============================================================================
#
# These functions use the JediWorkerPool for process-isolated execution.
# Benefits:
# - No ghost threads (process can be killed)
# - No GC recursion errors (isolated process)
# - Reliable timeout handling (SIGKILL)
#
# Use these for production workloads where reliability is critical.
# ============================================================================


def get_jedi_worker_pool(repo_root: str):
    """Get or create a global Jedi worker pool.

    Thread-safe singleton pattern. The pool is created lazily on first use.

    Args:
        repo_root: Project root path for Jedi context.

    Returns:
        JediWorkerPool instance.
    """
    global _jedi_worker_pool

    from c4.lsp.jedi_worker import JediWorkerPool

    with _jedi_worker_pool_lock:
        if _jedi_worker_pool is None:
            _jedi_worker_pool = JediWorkerPool(
                repo_root=repo_root,
                max_workers=2,
                timeout=3.0,  # Best-effort: short timeout
            )
            logger.info(f"Created Jedi worker pool for {repo_root}")
        return _jedi_worker_pool


def shutdown_jedi_worker_pool() -> None:
    """Shutdown the global Jedi worker pool.

    Should be called during application shutdown.
    """
    global _jedi_worker_pool

    with _jedi_worker_pool_lock:
        if _jedi_worker_pool is not None:
            _jedi_worker_pool.shutdown()
            _jedi_worker_pool = None
            logger.info("Jedi worker pool shut down")


def find_symbol_isolated(
    name_path_pattern: str,
    source: str,
    file_path: str | None = None,
    project_path: str | None = None,
    timeout: float = 3.0,
) -> list[dict]:
    """Find symbols using process-isolated worker.

    This function uses the JediWorkerPool for safe, timeout-able execution.
    Unlike find_symbol_mcp, timeouts actually terminate the worker process,
    preventing ghost threads and GC recursion errors.

    Args:
        name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
        source: Source code to search in
        file_path: Optional file path for context
        project_path: Project root path
        timeout: Maximum execution time (default: 3.0s)

    Returns:
        List of symbol info dictionaries. Empty list on timeout/error.
    """
    if not JEDI_AVAILABLE:
        return []

    repo_root = project_path or "."

    try:
        pool = get_jedi_worker_pool(repo_root)
        result = pool.execute({
            "op": "get_names",
            "source": source,
            "path": file_path,
            "options": {"all_scopes": True, "definitions": True},
        })

        if not result.get("ok"):
            error = result.get("error", {})
            logger.debug(f"Jedi worker error: {error.get('message', 'unknown')}")
            return []

        # Filter results by pattern
        raw_symbols = result.get("result", [])
        return _filter_symbols_by_pattern(raw_symbols, name_path_pattern)

    except TimeoutError:
        logger.warning(f"find_symbol_isolated timed out for pattern={name_path_pattern}")
        return []
    except Exception as e:
        logger.debug(f"find_symbol_isolated error: {e}")
        return []


def get_symbols_overview_isolated(
    source: str,
    file_path: str | None = None,
    project_path: str | None = None,
    depth: int = 0,
    timeout: float = 3.0,
) -> dict:
    """Get symbols overview using process-isolated worker.

    This function uses the JediWorkerPool for safe, timeout-able execution.

    Args:
        source: Source code to analyze
        file_path: Optional file path for context
        project_path: Project root path
        depth: Depth of children to include (0 = top-level only)
        timeout: Maximum execution time (default: 3.0s)

    Returns:
        Dictionary with symbols grouped by kind.
    """
    if not JEDI_AVAILABLE:
        return {"error": "jedi not available"}

    repo_root = project_path or "."

    try:
        pool = get_jedi_worker_pool(repo_root)
        result = pool.execute({
            "op": "get_names",
            "source": source,
            "path": file_path,
            "options": {"all_scopes": True, "definitions": True},
        })

        if not result.get("ok"):
            error = result.get("error", {})
            return {"error": error.get("message", "unknown error")}

        # Group by kind
        raw_symbols = result.get("result", [])
        grouped: dict[str, list[dict]] = {}

        for symbol in raw_symbols:
            # Filter by depth
            if depth == 0 and symbol.get("parent_type") in ("class", "function"):
                continue

            kind = symbol.get("type", "unknown")
            if kind not in grouped:
                grouped[kind] = []
            grouped[kind].append(symbol)

        return {
            "file": file_path,
            "symbols_by_kind": grouped,
            "total_count": len(raw_symbols),
        }

    except TimeoutError:
        logger.warning(f"get_symbols_overview_isolated timed out for {file_path}")
        return {"error": "Operation timed out"}
    except Exception as e:
        logger.debug(f"get_symbols_overview_isolated error: {e}")
        return {"error": str(e)}


def _filter_symbols_by_pattern(
    symbols: list[dict],
    pattern: str,
) -> list[dict]:
    """Filter symbols by name path pattern.

    Pattern matching rules:
    - Simple name: "method_name" - matches any symbol with that name
    - Relative path: "ClassName/method_name" - matches method in class
    - Absolute path: "/ClassName/method_name" - exact match from root

    Args:
        symbols: List of symbol dictionaries from worker.
        pattern: Name path pattern to match.

    Returns:
        Filtered list of symbols.
    """
    pattern_parts = pattern.strip("/").split("/")
    is_absolute = pattern.startswith("/")
    target_name = pattern_parts[-1]
    parent_names = pattern_parts[:-1] if len(pattern_parts) > 1 else []

    results = []
    for symbol in symbols:
        # Match target name
        if symbol.get("name") != target_name:
            continue

        # Check parent if pattern has parents
        if parent_names:
            parent_name = symbol.get("parent_name")
            if is_absolute:
                # For absolute paths, we need exact parent match
                # (simplified: just check immediate parent)
                if parent_name != parent_names[-1]:
                    continue
            else:
                # Relative: check immediate parent
                if parent_name != parent_names[-1]:
                    continue

        results.append(symbol)

    return results
