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
    - Bash (git commit): Capture git commits with metadata (importance: 8)

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
    # Shell commands (will be elevated for git commits)
    "Bash": 5,
    # Navigation (medium importance)
    "list_dir": 5,
    "find_file": 5,
    # C4 MCP tools (capture for context)
    "c4_get_task": 7,
    "c4_submit": 8,
}

# Maximum output size to capture (to avoid memory issues)
MAX_OUTPUT_SIZE = 50000  # 50KB


def should_capture(tool_name: str, output: str | None = None) -> bool:
    """Check if a tool's output should be captured.

    Args:
        tool_name: Name of the tool that was executed.
        output: Optional output to check for special cases (e.g., git commits).

    Returns:
        True if the tool output should be captured.
    """
    if tool_name in CAPTURE_RULES:
        return True

    # Check for git commit in Bash output (case-insensitive)
    if tool_name.lower() in ("bash", "shell") and output:
        if is_git_commit_output(output):
            return True

    return False


def is_git_commit_output(output: str) -> bool:
    """Check if output looks like a git commit result.

    Args:
        output: The output string to check.

    Returns:
        True if the output appears to be from a git commit.
    """
    import re

    # Quick negative check
    if not output or "[" not in output:
        return False

    # Standard git commit output pattern: [branch sha] message
    patterns = [
        r"\[[\w\-/\.]+\s+[a-f0-9]{7,40}\]\s+.+",  # [branch sha] message
        r"\[[\w\-/\.]+\s+[a-f0-9]{7,40}\s+\([^)]+\)\]\s+.+",  # [branch sha (amend)] message
    ]

    for pattern in patterns:
        if re.search(pattern, output):
            return True

    # Check for "file changed" which appears in commit output
    output_lower = output.lower()
    if ("file changed" in output_lower or "files changed" in output_lower):
        if re.search(r"\[[^\]]+\s+[a-f0-9]{7,40}\]", output):
            return True

    return False


def get_importance(tool_name: str, output: str | None = None) -> int:
    """Get importance level for a tool.

    Args:
        tool_name: Name of the tool.
        output: Optional output to check for special cases.

    Returns:
        Importance level (1-10), defaults to 5.
    """
    # Check for git commit in Bash output - elevated importance
    if output and tool_name.lower() in ("bash", "shell"):
        if is_git_commit_output(output):
            return 8  # Git commits are high importance

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


def parse_git_commit_metadata(output: str, input_data: dict | str | None = None) -> dict | None:
    """Parse git commit output to extract metadata.

    Args:
        output: The git commit output string.
        input_data: Optional input command data.

    Returns:
        Dictionary with commit metadata, or None if parsing fails.
    """
    import re

    if not output:
        return None

    # Parse commit header: [branch sha] message
    patterns = [
        r"\[(?P<branch>[\w\-/\.]+)\s+(?P<sha>[a-f0-9]{7,40})\]\s+(?P<message>.+)",
        r"\[(?P<branch>[\w\-/\.]+)\s+(?P<sha>[a-f0-9]{7,40})\s+\([^)]+\)\]\s+(?P<message>.+)",
    ]

    sha = ""
    message = ""
    branch = ""

    for pattern in patterns:
        match = re.search(pattern, output)
        if match:
            sha = match.group("sha")
            message = match.group("message")
            branch = match.group("branch")
            break

    if not sha:
        return None

    # Parse changed files
    changed_files = []
    insertions = 0
    deletions = 0

    for line in output.split("\n"):
        line = line.strip()

        # File change pattern: " file.py | 10 +"
        file_match = re.match(r"^\s*(.+?)\s*\|\s*(\d+|Bin)", line)
        if file_match:
            file_path = file_match.group(1).strip()
            if file_path and not file_path.startswith("("):
                changed_files.append(file_path)
            continue

        # Create/delete mode lines
        mode_match = re.match(r"^\s*(create|delete)\s+mode\s+\d+\s+(.+)$", line)
        if mode_match:
            file_path = mode_match.group(2).strip()
            if file_path and file_path not in changed_files:
                changed_files.append(file_path)
            continue

    # Parse summary statistics
    stat_match = re.search(
        r"(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?",
        output,
    )
    if stat_match:
        if stat_match.group(2):
            insertions = int(stat_match.group(2))
        if stat_match.group(3):
            deletions = int(stat_match.group(3))

    return {
        "sha": sha,
        "message": message,
        "branch": branch,
        "changed_files": changed_files,
        "insertions": insertions,
        "deletions": deletions,
    }


def format_git_commit_content(metadata: dict) -> str:
    """Format git commit metadata as readable content.

    Args:
        metadata: The parsed commit metadata.

    Returns:
        Formatted string summarizing the commit.
    """
    lines = [
        f"Git Commit: {metadata['sha']}",
        f"Message: {metadata['message']}",
    ]

    if metadata.get("branch"):
        lines.append(f"Branch: {metadata['branch']}")

    if metadata.get("changed_files"):
        files = metadata["changed_files"]
        lines.append(f"\nChanged files ({len(files)}):")
        for f in files[:20]:  # Limit to 20 files
            lines.append(f"  - {f}")
        if len(files) > 20:
            lines.append(f"  ... and {len(files) - 20} more")

    if metadata.get("insertions") or metadata.get("deletions"):
        lines.append(f"\nStats: +{metadata.get('insertions', 0)} -{metadata.get('deletions', 0)}")

    return "\n".join(lines)


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

    # Convert output to string for processing
    output_str = truncate_output(output)

    # Check for git commit and extract metadata
    commit_metadata = None
    effective_tool_name = tool_name
    if tool_name.lower() in ("bash", "shell") and is_git_commit_output(output_str):
        commit_metadata = parse_git_commit_metadata(output_str, input_data)
        if commit_metadata:
            effective_tool_name = "git_commit"
            # Use formatted content for better readability
            output_str = format_git_commit_content(commit_metadata)

    # Capture the observation
    importance = get_importance(tool_name, output_str if not commit_metadata else None)
    if commit_metadata:
        importance = 8  # Git commits are high importance

    observation = handler.capture_tool_output(
        tool_name=effective_tool_name,
        input_data=input_data,
        output=output_str,
        importance=importance,
    )

    # Add commit metadata to observation metadata
    if commit_metadata:
        observation.metadata["commit_metadata"] = commit_metadata
        observation.tags.append("git:commit")
        if commit_metadata.get("branch"):
            observation.tags.append(f"branch:{commit_metadata['branch']}")

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

        # Convert output to string for checking
        output_str = str(output) if not isinstance(output, str) else output

        # Check if this tool should be captured (including git commit detection)
        if not should_capture(tool_name, output_str):
            sys.exit(0)

        # Capture the tool output
        capture_tool_output(tool_name, input_data, output)

    except Exception:
        # Silent fail - never block main flow
        pass

    sys.exit(0)


if __name__ == "__main__":
    main()
