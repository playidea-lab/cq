"""Unified symbol provider with multilspy primary and Jedi fallback.

This module provides a unified interface for symbol operations that:
1. Tries multilspy first (real LSP servers, 30+ languages)
2. Falls back to Jedi for Python if multilspy fails or is unavailable

This ensures maximum compatibility while leveraging the best available
code intelligence for each situation.
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from c4.lsp.cache import get_symbol_cache
from c4.lsp.jedi_provider import (
    JEDI_AVAILABLE,
    JediSymbolProvider,
    find_symbol_isolated,
    get_symbols_overview_isolated,
    shutdown_jedi_worker_pool,
)
from c4.lsp.jedi_provider import (
    find_symbol_mcp as jedi_find_symbol,
)
from c4.lsp.jedi_provider import (
    get_symbols_overview_mcp as jedi_get_symbols_overview,
)
from c4.lsp.multilspy_provider import (
    MULTILSPY_AVAILABLE,
    MultilspyProvider,
)

logger = logging.getLogger(__name__)


class UnifiedSymbolProvider:
    """Unified provider with multilspy primary, Jedi fallback.

    This provider implements a fallback chain:
    1. multilspy (if available and language supported)
    2. Jedi (for Python, always available)

    The provider automatically detects the language from file extensions
    and routes requests to the appropriate backend.

    Example:
        >>> provider = UnifiedSymbolProvider("/path/to/project")
        >>> # Uses multilspy for TypeScript, Jedi for Python
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

    # Jedi timeout: best-effort, short timeout
    # Process-isolated workers can be killed on timeout, so short is safe
    JEDI_TIMEOUT = 2.0

    def __init__(
        self,
        project_path: str | Path,
        timeout: int = 30,
        prefer_multilspy: bool = True,  # Try multilspy first, fallback to Jedi
        use_isolated_jedi: bool = True,  # Use process-isolated Jedi workers
    ):
        """Initialize the unified provider.

        Args:
            project_path: Root path of the project to analyze.
            timeout: Timeout in seconds for operations.
            prefer_multilspy: If True, try multilspy first; if False, always use Jedi.
            use_isolated_jedi: If True, use process-isolated Jedi workers (recommended).
        """
        self.project_path = Path(project_path).resolve()
        self.timeout = timeout
        self.prefer_multilspy = prefer_multilspy
        self.use_isolated_jedi = use_isolated_jedi

        # Initialize multilspy if available and preferred
        self._multilspy: MultilspyProvider | None = None
        if MULTILSPY_AVAILABLE and prefer_multilspy:
            try:
                self._multilspy = MultilspyProvider(
                    project_path=self.project_path,
                    timeout=timeout,
                )
                logger.info("Multilspy provider initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize multilspy, using Jedi: {e}")
                self._multilspy = None

        # Initialize Jedi provider (always available for Python)
        # For isolated mode, we don't need JediSymbolProvider instance
        self._jedi: JediSymbolProvider | None = None
        if JEDI_AVAILABLE and not use_isolated_jedi:
            self._jedi = JediSymbolProvider(project_path=str(self.project_path))
            logger.info("Jedi provider initialized (thread-based)")
        elif JEDI_AVAILABLE and use_isolated_jedi:
            # Initialize the worker pool lazily on first use
            logger.info("Jedi provider configured (process-isolated)")

    @property
    def multilspy_available(self) -> bool:
        """Check if multilspy is available and initialized."""
        return self._multilspy is not None

    @property
    def jedi_available(self) -> bool:
        """Check if Jedi is available and initialized."""
        return self._jedi is not None

    def _detect_language(self, file_path: Path | str) -> str | None:
        """Detect programming language from file extension.

        Args:
            file_path: Path to the file.

        Returns:
            Language identifier or None if unsupported.
        """
        path = Path(file_path)
        return self.LANGUAGE_MAP.get(path.suffix.lower())

    def _is_python_only(self, relative_path: str) -> bool:
        """Check if the path is Python-only (for Jedi fallback).

        Args:
            relative_path: Relative path to check.

        Returns:
            True if the path contains only Python files.
        """
        if not relative_path:
            return False

        path = self.project_path / relative_path
        if path.is_file():
            return self._detect_language(path) == "python"

        return False

    def find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
        depth: int = 0,
    ) -> list[dict[str, Any]]:
        """Find symbols matching the pattern.

        Uses multilspy first if available, falls back to Jedi for Python.
        Jedi fallback uses process-isolated workers for reliable timeout handling.

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass", "MyClass/method").
            relative_path: Restrict search to this file or directory.
            include_body: Include symbol body in results.
            depth: Depth of children to include.

        Returns:
            List of symbol information dictionaries.
        """
        results: list[dict[str, Any]] = []

        # Determine if we should use Jedi directly
        use_jedi_only = not self.multilspy_available or not self.prefer_multilspy or self._is_python_only(relative_path)

        # Try multilspy first (Tier 1: real LSP, fast, accurate)
        if self.multilspy_available and not use_jedi_only:
            try:
                results = self._multilspy.find_symbol(  # type: ignore[union-attr]
                    name_path_pattern=name_path_pattern,
                    relative_path=relative_path,
                    include_body=include_body,
                    depth=depth,
                )
                if results:
                    logger.debug(f"multilspy found {len(results)} symbols")
                    return results
            except Exception as e:
                logger.warning(f"multilspy find_symbol failed, trying Jedi: {e}")

        # Tier 2: Jedi fallback (Python only)
        if JEDI_AVAILABLE:
            try:
                # Jedi only works for Python files
                if relative_path:
                    path = self.project_path / relative_path
                    if path.is_file() and self._detect_language(path) != "python":
                        logger.debug(f"Skipping Jedi for non-Python file: {path}")
                        return results

                # Use process-isolated worker if configured (recommended)
                if self.use_isolated_jedi and relative_path:
                    isolated_results = self._find_symbol_isolated(
                        name_path_pattern=name_path_pattern,
                        relative_path=relative_path,
                        include_body=include_body,
                    )
                    if isolated_results:
                        results = isolated_results
                elif relative_path:
                    # Fallback to thread-based MCP wrapper (only with specific path)
                    jedi_results = jedi_find_symbol(
                        name_path_pattern=name_path_pattern,
                        relative_path=relative_path,
                        include_body=include_body,
                        project_path=str(self.project_path),
                        timeout=self.timeout,
                    )
                    if jedi_results:
                        results = jedi_results
                else:
                    # Workspace-wide search without relative_path is disabled
                    # because thread-based timeout cannot be enforced reliably.
                    # Use relative_path to limit search scope.
                    logger.warning(
                        f"Workspace-wide symbol search disabled (no relative_path). "
                        f"Pattern: {name_path_pattern}. Please provide a relative_path."
                    )
                    return []

                if results:
                    logger.debug(f"Jedi found {len(results)} symbols")
            except Exception as e:
                logger.warning(f"Jedi find_symbol failed: {e}")

        return results

    def _find_symbol_isolated(
        self,
        name_path_pattern: str,
        relative_path: str,
        include_body: bool = False,
    ) -> list[dict[str, Any]]:
        """Find symbols using process-isolated Jedi worker.

        This method provides reliable timeout handling by running Jedi
        in a separate process that can be killed on timeout.

        Args:
            name_path_pattern: Pattern to match.
            relative_path: File path relative to project root.
            include_body: Include symbol body (not yet implemented).

        Returns:
            List of symbol dictionaries.
        """
        path = self.project_path / relative_path

        if not path.is_file():
            return []

        try:
            source = path.read_text(encoding="utf-8")
        except Exception as e:
            logger.debug(f"Failed to read file {path}: {e}")
            return []

        return find_symbol_isolated(
            name_path_pattern=name_path_pattern,
            source=source,
            file_path=str(path),
            project_path=str(self.project_path),
            timeout=self.JEDI_TIMEOUT,
        )

    def get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get document symbols for a file.

        Uses multilspy first if available, falls back to Jedi for Python.

        Args:
            relative_path: Path to the file relative to project root.
            depth: Depth of children to include (0 = top-level only).

        Returns:
            Dictionary with symbols grouped by kind.
        """
        file_path = self.project_path / relative_path

        if not file_path.exists():
            return {"error": f"File not found: {relative_path}"}

        language = self._detect_language(file_path)
        is_python = language == "python"

        # Try multilspy first (for any supported language)
        if self.multilspy_available and self.prefer_multilspy:
            try:
                result = self._multilspy.get_symbols_overview(  # type: ignore[union-attr]
                    relative_path=relative_path,
                    depth=depth,
                )
                # Check if result has meaningful content (not just empty/unknown symbols)
                if result and "error" not in result:
                    has_meaningful_content = any(
                        len(items) > 0 and any(s.get("name") for s in items)
                        for key, items in result.items()
                        if isinstance(items, list)
                    )
                    if has_meaningful_content:
                        logger.debug(f"multilspy returned overview for {relative_path}")
                        return result
                    logger.debug("multilspy returned empty/invalid result, trying Jedi fallback")
            except Exception as e:
                logger.warning(f"multilspy get_symbols_overview failed: {e}")

        # Tier 2: Jedi fallback (Python only)
        if is_python and JEDI_AVAILABLE:
            try:
                # Use process-isolated worker if configured (recommended)
                if self.use_isolated_jedi:
                    result = self._get_symbols_overview_isolated(
                        relative_path=relative_path,
                        depth=depth,
                    )
                else:
                    # Fallback to thread-based MCP wrapper
                    result = jedi_get_symbols_overview(
                        relative_path=relative_path,
                        depth=depth,
                        project_path=str(self.project_path),
                    )

                if result and "error" not in result:
                    # Convert Jedi format (symbols_by_kind) to standard format
                    symbols_by_kind = result.get("symbols_by_kind", {})
                    normalized = {
                        "classes": symbols_by_kind.get("class", []),
                        "functions": symbols_by_kind.get("function", []),
                        "methods": symbols_by_kind.get("method", []),
                        "variables": symbols_by_kind.get("variable", []),
                        "constants": symbols_by_kind.get("constant", []),
                    }
                    # Add any other kinds to 'other'
                    other = []
                    for kind, items in symbols_by_kind.items():
                        if kind not in ("class", "function", "method", "variable", "constant"):
                            other.extend(items)
                    if other:
                        normalized["other"] = other
                    # Remove empty categories
                    normalized = {k: v for k, v in normalized.items() if v}
                    logger.debug(f"Jedi returned overview for {relative_path}")
                    return normalized
            except Exception as e:
                logger.warning(f"Jedi get_symbols_overview failed: {e}")
                return {"error": str(e)}

        # No provider available for this language
        if not is_python and not self.multilspy_available:
            return {"error": f"No provider available for {language} files"}

        return {"error": "No symbols found"}

    def _get_symbols_overview_isolated(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get symbols overview using process-isolated Jedi worker.

        Args:
            relative_path: File path relative to project root.
            depth: Depth of children to include.

        Returns:
            Dictionary with symbols grouped by kind.
        """
        path = self.project_path / relative_path

        if not path.is_file():
            return {"error": f"File not found: {relative_path}"}

        try:
            source = path.read_text(encoding="utf-8")
        except Exception as e:
            return {"error": f"Failed to read file: {e}"}

        return get_symbols_overview_isolated(
            source=source,
            file_path=str(path),
            project_path=str(self.project_path),
            depth=depth,
            timeout=self.JEDI_TIMEOUT,
        )

    def invalidate_cache(self, file_path: str | None = None) -> int:
        """Invalidate cache entries.

        This method should be called when files change externally
        (e.g., git checkout, external editor saves).

        Args:
            file_path: Specific file to invalidate. If None, invalidates all entries.

        Returns:
            Number of entries invalidated.
        """
        cache = get_symbol_cache()

        if file_path:
            # Resolve to absolute path
            abs_path = str((self.project_path / file_path).resolve())
            invalidated = 1 if cache.invalidate(abs_path) else 0
            if invalidated:
                logger.debug(f"Invalidated cache for {file_path}")
            return invalidated
        else:
            # Invalidate all entries
            count = cache.clear()
            logger.info(f"Invalidated all cache entries ({count} cleared)")
            return count

    def invalidate_all(self) -> int:
        """Invalidate all cache entries.

        Convenience method that calls invalidate_cache(None).
        Useful for:
        - git checkout/switch operations
        - External file system changes
        - Manual cache reset

        Returns:
            Number of entries cleared.
        """
        return self.invalidate_cache(None)

    def get_cache_status(self) -> dict[str, Any]:
        """Get cache status for monitoring.

        Returns:
            Dictionary with cache statistics.
        """
        cache = get_symbol_cache()
        return cache.get_status()

    def shutdown(self) -> None:
        """Shutdown all providers including worker pools."""
        if self._multilspy:
            try:
                self._multilspy.shutdown()
            except Exception as e:
                logger.warning(f"Error shutting down multilspy: {e}")

        # Shutdown Jedi worker pool if we're using isolated mode
        if self.use_isolated_jedi:
            try:
                shutdown_jedi_worker_pool()
            except Exception as e:
                logger.warning(f"Error shutting down Jedi worker pool: {e}")

        self._multilspy = None
        self._jedi = None
        logger.info("Unified provider shut down")

    def __enter__(self) -> "UnifiedSymbolProvider":
        """Context manager entry."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Context manager exit - shutdown providers."""
        self.shutdown()


# Singleton instance for MCP tools
_provider_instance: UnifiedSymbolProvider | None = None
_provider_lock = __import__("threading").Lock()


def get_provider(project_path: str | None = None, timeout: int = 30) -> UnifiedSymbolProvider:
    """Get or create the unified provider singleton.

    Thread-safe singleton pattern using double-checked locking.

    Args:
        project_path: Project root path. Uses current directory if not specified.
        timeout: Timeout in seconds for operations.

    Returns:
        UnifiedSymbolProvider instance.
    """
    global _provider_instance

    path = Path(project_path or ".").resolve()

    # Fast path: check without lock if instance exists and matches
    if _provider_instance is not None and _provider_instance.project_path == path:
        return _provider_instance

    # Slow path: acquire lock and check again (double-checked locking)
    with _provider_lock:
        # Re-check after acquiring lock (another thread may have created it)
        if _provider_instance is not None and _provider_instance.project_path == path:
            return _provider_instance

        # Create new instance
        if _provider_instance is not None:
            _provider_instance.shutdown()
        _provider_instance = UnifiedSymbolProvider(project_path=path, timeout=timeout)
        return _provider_instance


# MCP tool wrapper functions
def find_symbol_unified(
    name_path_pattern: str,
    relative_path: str = "",
    include_body: bool = False,
    project_path: str | None = None,
    timeout: int = 30,
) -> list[dict[str, Any]]:
    """MCP tool wrapper for find_symbol with unified provider.

    This function provides a drop-in replacement for the Jedi-based
    find_symbol_mcp, but with multilspy support.

    Args:
        name_path_pattern: Pattern to match (e.g., "MyClass/my_method").
        relative_path: Restrict search to this path.
        include_body: Include symbol body in results.
        project_path: Project root path.
        timeout: Maximum execution time in seconds.

    Returns:
        List of symbol info dictionaries.
    """
    try:
        provider = get_provider(project_path, timeout)
        return provider.find_symbol(
            name_path_pattern=name_path_pattern,
            relative_path=relative_path,
            include_body=include_body,
        )
    except Exception as e:
        logger.error(f"find_symbol_unified failed: {e}")
        # Ultimate fallback to Jedi
        return jedi_find_symbol(
            name_path_pattern=name_path_pattern,
            relative_path=relative_path,
            include_body=include_body,
            project_path=project_path,
            timeout=timeout,
        )


def get_symbols_overview_unified(
    relative_path: str,
    depth: int = 0,
    project_path: str | None = None,
    timeout: int = 30,
) -> dict[str, Any]:
    """MCP tool wrapper for get_symbols_overview with unified provider.

    This function provides a drop-in replacement for the Jedi-based
    get_symbols_overview_mcp, but with multilspy support.

    Args:
        relative_path: Path to the file relative to project root.
        depth: Depth of children to include.
        project_path: Project root path.
        timeout: Maximum execution time in seconds.

    Returns:
        Dictionary with symbols grouped by kind.
    """
    try:
        provider = get_provider(project_path, timeout)
        return provider.get_symbols_overview(
            relative_path=relative_path,
            depth=depth,
        )
    except Exception as e:
        logger.error(f"get_symbols_overview_unified failed: {e}")
        # Ultimate fallback to Jedi
        return jedi_get_symbols_overview(
            relative_path=relative_path,
            depth=depth,
            project_path=project_path,
        )


def shutdown_provider() -> None:
    """Shutdown the singleton provider."""
    global _provider_instance
    if _provider_instance:
        _provider_instance.shutdown()
        _provider_instance = None
