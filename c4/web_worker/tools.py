"""Tool definitions for the AgenticWorker.

Defines the tools available to the Claude agent for interacting with
the C4 workspace through the API.
"""

from typing import Any

# Tool definitions following Claude's Tool Use schema
TOOLS: list[dict[str, Any]] = [
    {
        "name": "read_file",
        "description": "Read file contents from the workspace",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "File path relative to workspace root",
                }
            },
            "required": ["path"],
        },
    },
    {
        "name": "write_file",
        "description": "Write content to a file in the workspace. Creates parent directories if needed.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "File path relative to workspace root",
                },
                "content": {
                    "type": "string",
                    "description": "Content to write to the file",
                },
            },
            "required": ["path", "content"],
        },
    },
    {
        "name": "run_shell",
        "description": "Run a shell command in the workspace. Commands are executed in the workspace root directory.",
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {
                    "type": "string",
                    "description": "Shell command to execute",
                },
                "timeout": {
                    "type": "integer",
                    "description": "Timeout in seconds (default: 60, max: 300)",
                    "default": 60,
                },
            },
            "required": ["command"],
        },
    },
    {
        "name": "search_files",
        "description": "Search for files by name pattern (glob) or content (grep)",
        "input_schema": {
            "type": "object",
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Search pattern - glob pattern for file names or regex for content",
                },
                "search_type": {
                    "type": "string",
                    "enum": ["glob", "grep"],
                    "description": "Type of search: 'glob' for file names, 'grep' for file content",
                },
                "path": {
                    "type": "string",
                    "description": "Directory path to search in (relative to workspace root)",
                    "default": ".",
                },
            },
            "required": ["pattern", "search_type"],
        },
    },
    {
        "name": "list_directory",
        "description": "List files and directories in a path",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Directory path relative to workspace root",
                    "default": ".",
                },
                "recursive": {
                    "type": "boolean",
                    "description": "Whether to list recursively",
                    "default": False,
                },
            },
        },
    },
]


def get_tool_by_name(name: str) -> dict[str, Any] | None:
    """Get tool definition by name.

    Args:
        name: Tool name

    Returns:
        Tool definition dict or None if not found
    """
    for tool in TOOLS:
        if tool["name"] == name:
            return tool
    return None


def validate_tool_input(name: str, input_data: dict[str, Any]) -> tuple[bool, str]:
    """Validate tool input against schema.

    Args:
        name: Tool name
        input_data: Input data to validate

    Returns:
        Tuple of (is_valid, error_message)
    """
    tool = get_tool_by_name(name)
    if not tool:
        return False, f"Unknown tool: {name}"

    schema = tool.get("input_schema", {})
    required = schema.get("required", [])

    # Check required fields
    for field in required:
        if field not in input_data:
            return False, f"Missing required field: {field}"

    return True, ""


# Tool names as constants for type safety
class ToolName:
    """Tool name constants."""

    READ_FILE = "read_file"
    WRITE_FILE = "write_file"
    RUN_SHELL = "run_shell"
    SEARCH_FILES = "search_files"
    LIST_DIRECTORY = "list_directory"
