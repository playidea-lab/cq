#!/usr/bin/env python3
"""C4 Auto-Capture Hook - Capture tool outputs for memory system.

This script is called by Claude Code's PostToolUse hook mechanism.
It reads tool execution data from stdin and captures relevant outputs
to the C4 memory system for semantic search and context retrieval.

The hook runs asynchronously (async=true in hook config) so it does not
block the main Claude Code flow. On any error, it exits silently with 0.

Input format (JSON from stdin):
    {
        "tool_name": "read_file",
        "input": {"path": "/src/main.py"},
        "output": "def main(): ..."
    }

Capture rules:
    - read_file: Capture file contents (importance: 6)
    - search_for_pattern: Capture search results (importance: 6)
    - find_symbol: Capture symbol definitions (importance: 7)
    - get_symbols_overview: Capture symbol overviews (importance: 7)
    - user_message: Capture user messages (importance: 9)
    - file_write: Capture written files (importance: 8)

Usage:
    # In claude_desktop_config.json or claude_code settings:
    "hooks": {
        "PostToolUse": [
            {
                "command": "python3 /path/to/c4-auto-capture-hook.py",
                "async": true,
                "timeout": 5000
            }
        ]
    }

Exit codes:
    0 = Success or silent failure (never blocks main flow)
"""

import json
import os
import sys
from pathlib import Path

# Capture rules: tool_name -> importance level (1-10)
# Only tools in this map are captured
CAPTURE_RULES: dict[str, int] = {
    # Code analysis (medium-high importance)
    "read_file": 6,
    "search_for_pattern": 6,
    "find_symbol": 7,
    "get_symbols_overview": 7,
    # User interactions (high importance)
    "user_message": 9,
    # File modifications (high importance)
    "file_write": 8,
    "edit_file": 8,
    # Navigation (medium importance)
    "list_dir": 5,
    "find_file": 5,
    # C4 MCP tools (capture for context)
    "c4_get_task": 7,
    "c4_submit": 8,
}

# Maximum output size to capture (to avoid memory issues)
MAX_OUTPUT_SIZE = 50000  # 50KB


def should_capture(tool_name: str) -> bool:
    """Check if a tool's output should be captured.

    Args:
        tool_name: Name of the tool that was executed.

    Returns:
        True if the tool output should be captured.
    """
    return tool_name in CAPTURE_RULES


def get_importance(tool_name: str) -> int:
    """Get importance level for a tool.

    Args:
        tool_name: Name of the tool.

    Returns:
        Importance level (1-10), defaults to 5.
    """
    return CAPTURE_RULES.get(tool_name, 5)


def truncate_output(output: str | dict, max_size: int = MAX_OUTPUT_SIZE) -> str:
    """Truncate output to maximum size.

    Args:
        output: Tool output (string or dict).
        max_size: Maximum size in characters.

    Returns:
        Truncated output as string.
    """
    if isinstance(output, dict):
        output_str = json.dumps(output, indent=2)
    else:
        output_str = str(output)

    if len(output_str) > max_size:
        return output_str[:max_size] + "\n... [truncated]"
    return output_str


def find_project_root() -> Path | None:
    """Find project root by looking for .c4 directory.

    Returns:
        Project root path, or None if not in a C4 project.
    """
    # Check environment variable first
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
        if (root / ".c4").exists():
            return root

    # Walk up from current directory
    current = Path.cwd()
    while current != current.parent:
        if (current / ".c4").exists():
            return current
        current = current.parent

    return None


def capture_tool_output(tool_name: str, input_data: dict | str | None, output: str | dict) -> None:
    """Capture tool output to memory system.

    Args:
        tool_name: Name of the executed tool.
        input_data: Input parameters passed to the tool.
        output: Output from the tool execution.
    """
    root = find_project_root()
    if root is None:
        return  # Not in a C4 project

    db_path = root / ".c4" / "tasks.db"
    if not db_path.exists():
        return  # Database not initialized

    # Import here to avoid import errors when c4 is not installed
    try:
        from c4.memory.auto_capture import get_auto_capture_handler
    except ImportError:
        return  # c4 not installed

    # Get project ID from directory name
    project_id = root.name

    # Get handler
    handler = get_auto_capture_handler(
        project_id=project_id,
        db_path=db_path,
        enable_embeddings=False,  # No embeddings in hook for speed
    )

    # Truncate large outputs
    output_str = truncate_output(output)

    # Capture the observation
    importance = get_importance(tool_name)
    observation = handler.capture_tool_output(
        tool_name=tool_name,
        input_data=input_data,
        output=output_str,
        importance=importance,
    )

    # Store the observation
    handler.store_observation(observation)


def main() -> None:
    """Main entry point for the hook."""
    try:
        # Read JSON from stdin
        stdin_data = sys.stdin.read()
        if not stdin_data.strip():
            sys.exit(0)  # No input, nothing to do

        data = json.loads(stdin_data)

        tool_name = data.get("tool_name", "")
        input_data = data.get("input")
        output = data.get("output", "")

        # Check if this tool should be captured
        if not should_capture(tool_name):
            sys.exit(0)

        # Capture the tool output
        capture_tool_output(tool_name, input_data, output)

    except Exception:
        # Silent fail - never block main flow
        pass

    sys.exit(0)


if __name__ == "__main__":
    main()
