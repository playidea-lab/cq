"""File operation tool handlers.

Handles: c4_read_file, c4_create_text_file, c4_list_dir, c4_find_file,
         c4_search_for_pattern, c4_replace_content
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_read_file")
def handle_read_file(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Read a file or portion of it."""
    return daemon.c4_read_file(
        relative_path=arguments.get("relative_path", ""),
        start_line=arguments.get("start_line", 0),
        end_line=arguments.get("end_line"),
    )


@register_tool("c4_create_text_file")
def handle_create_text_file(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Create or overwrite a text file."""
    return daemon.c4_create_text_file(
        relative_path=arguments.get("relative_path", ""),
        content=arguments.get("content", ""),
    )


@register_tool("c4_list_dir")
def handle_list_dir(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """List files and directories."""
    return daemon.c4_list_dir(
        relative_path=arguments.get("relative_path", "."),
        recursive=arguments.get("recursive", False),
    )


@register_tool("c4_find_file")
def handle_find_file(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Find files matching a glob pattern."""
    return daemon.c4_find_file(
        file_mask=arguments.get("file_mask", "*"),
        relative_path=arguments.get("relative_path", "."),
    )


@register_tool("c4_search_for_pattern")
def handle_search_for_pattern(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Search for a regex pattern in files."""
    return daemon.c4_search_for_pattern(
        pattern=arguments.get("pattern", ""),
        relative_path=arguments.get("relative_path", "."),
        glob_pattern=arguments.get("glob_pattern"),
        context_lines=arguments.get("context_lines", 0),
    )


@register_tool("c4_replace_content")
def handle_replace_content(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Replace content in a file using literal or regex matching."""
    return daemon.c4_replace_content(
        relative_path=arguments.get("relative_path", ""),
        needle=arguments.get("needle", ""),
        replacement=arguments.get("replacement", ""),
        mode=arguments.get("mode", "literal"),
        allow_multiple=arguments.get("allow_multiple", False),
    )
