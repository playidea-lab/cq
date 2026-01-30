"""Symbol operation tool handlers.

Handles: c4_replace_symbol_body, c4_insert_before_symbol, c4_insert_after_symbol,
         c4_rename_symbol, c4_find_symbol, c4_get_symbols_overview
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_replace_symbol_body")
def handle_replace_symbol_body(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Replace the body of a symbol (function, class, method)."""
    return daemon.c4_replace_symbol_body(
        name_path=arguments.get("name_path", ""),
        file_path=arguments.get("file_path"),
        new_body=arguments.get("new_body", ""),
    )


@register_tool("c4_insert_before_symbol")
def handle_insert_before_symbol(
    daemon: Any, arguments: dict[str, Any]
) -> dict[str, Any]:
    """Insert content before a symbol definition."""
    return daemon.c4_insert_before_symbol(
        name_path=arguments.get("name_path", ""),
        file_path=arguments.get("file_path"),
        content=arguments.get("content", ""),
    )


@register_tool("c4_insert_after_symbol")
def handle_insert_after_symbol(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Insert content after a symbol definition."""
    return daemon.c4_insert_after_symbol(
        name_path=arguments.get("name_path", ""),
        file_path=arguments.get("file_path"),
        content=arguments.get("content", ""),
    )


@register_tool("c4_rename_symbol")
def handle_rename_symbol(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Rename a symbol across the entire codebase."""
    return daemon.c4_rename_symbol(
        name_path=arguments.get("name_path", ""),
        file_path=arguments.get("file_path"),
        new_name=arguments.get("new_name", ""),
    )


@register_tool("c4_find_symbol")
def handle_find_symbol(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Find symbols matching a name path pattern."""
    return daemon.c4_find_symbol(
        name_path_pattern=arguments.get("name_path_pattern", ""),
        relative_path=arguments.get("relative_path", ""),
        include_body=arguments.get("include_body", False),
        depth=arguments.get("depth", 0),
    )


@register_tool("c4_get_symbols_overview")
def handle_get_symbols_overview(
    daemon: Any, arguments: dict[str, Any]
) -> dict[str, Any]:
    """Get an overview of symbols in a file."""
    return daemon.c4_get_symbols_overview(
        relative_path=arguments.get("relative_path", ""),
        depth=arguments.get("depth", 0),
    )
