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

from c4.lsp.jedi_provider import (
    JEDI_AVAILABLE,
    JediSymbolProvider,
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

    def __init__(
        self,
        project_path: str | Path,
        timeout: int = 30,
        prefer_multilspy: bool = True,
    ):
        """Initialize the unified provider.

        Args:
            project_path: Root path of the project to analyze.
            timeout: Timeout in seconds for operations.
            prefer_multilspy: If True, try multilspy first; if False, always use Jedi.
        """
        self.project_path = Path(project_path).resolve()
        self.timeout = timeout
        self.prefer_multilspy = prefer_multilspy

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
        self._jedi: JediSymbolProvider | None = None
        if JEDI_AVAILABLE:
            self._jedi = JediSymbolProvider(project_path=str(self.project_path))
            logger.info("Jedi provider initialized")

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

        # Try multilspy first
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

        # Fallback to Jedi (Python only)
        if JEDI_AVAILABLE:
            try:
                # Jedi only works for Python files
                if relative_path:
                    path = self.project_path / relative_path
                    if path.is_file() and self._detect_language(path) != "python":
                        logger.debug(f"Skipping Jedi for non-Python file: {path}")
                        return results

                # Use the MCP wrapper which handles the interface differences
                jedi_results = jedi_find_symbol(
                    name_path_pattern=name_path_pattern,
                    relative_path=relative_path,
                    include_body=include_body,
                    project_path=str(self.project_path),
                    timeout=self.timeout,
                )
                if jedi_results:
                    results = jedi_results
                    logger.debug(f"Jedi found {len(results)} symbols")
            except Exception as e:
                logger.warning(f"Jedi find_symbol failed: {e}")

        return results

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
                if result and "error" not in result:
                    logger.debug(f"multilspy returned overview for {relative_path}")
                    return result
            except Exception as e:
                logger.warning(f"multilspy get_symbols_overview failed: {e}")

        # Fallback to Jedi (Python only)
        if is_python and JEDI_AVAILABLE:
            try:
                # Use the MCP wrapper which handles the interface differences
                result = jedi_get_symbols_overview(
                    relative_path=relative_path,
                    depth=depth,
                    project_path=str(self.project_path),
                )
                if result:
                    logger.debug(f"Jedi returned overview for {relative_path}")
                    return result
            except Exception as e:
                logger.warning(f"Jedi get_symbols_overview failed: {e}")
                return {"error": str(e)}

        if not is_python and not self.multilspy_available:
            return {"error": f"No provider available for {language} files"}

        return {"error": "No symbols found"}

    def shutdown(self) -> None:
        """Shutdown all providers."""
        if self._multilspy:
            try:
                self._multilspy.shutdown()
            except Exception as e:
                logger.warning(f"Error shutting down multilspy: {e}")

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


def get_provider(project_path: str | None = None, timeout: int = 30) -> UnifiedSymbolProvider:
    """Get or create the unified provider singleton.

    Args:
        project_path: Project root path. Uses current directory if not specified.
        timeout: Timeout in seconds for operations.

    Returns:
        UnifiedSymbolProvider instance.
    """
    global _provider_instance

    path = Path(project_path or ".").resolve()

    # Create new instance if needed
    if _provider_instance is None or _provider_instance.project_path != path:
        if _provider_instance:
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
