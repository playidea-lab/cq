"""Capture rules for different tool types and command outputs.

This module provides specialized capture rules for extracting structured
information from tool outputs, particularly git commits.

Usage:
    from c4.memory.capture_rules import (
        is_git_commit_output,
        parse_git_commit,
        GitCommitMetadata,
    )

    if is_git_commit_output(tool_name, output):
        metadata = parse_git_commit(output)
        # metadata.sha, metadata.message, metadata.changed_files, etc.
"""

import re
from dataclasses import dataclass, field
from typing import Any


@dataclass
class GitCommitMetadata:
    """Metadata extracted from a git commit.

    Attributes:
        sha: The commit SHA (short or full).
        message: The commit message.
        branch: The branch the commit was made on (if available).
        changed_files: List of files that were changed.
        insertions: Number of lines inserted.
        deletions: Number of lines deleted.
        author: The commit author (if available).
        raw_diff: The raw diff output (if available).
    """

    sha: str
    message: str
    branch: str = ""
    changed_files: list[str] = field(default_factory=list)
    insertions: int = 0
    deletions: int = 0
    author: str = ""
    raw_diff: str = ""

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization.

        Returns:
            Dictionary representation of the commit metadata.
        """
        return {
            "sha": self.sha,
            "message": self.message,
            "branch": self.branch,
            "changed_files": self.changed_files,
            "insertions": self.insertions,
            "deletions": self.deletions,
            "author": self.author,
        }


# Patterns for detecting git commit output
GIT_COMMIT_PATTERNS = [
    # Standard git commit output: [branch sha] message
    r"\[(?P<branch>[\w\-/\.]+)\s+(?P<sha>[a-f0-9]{7,40})\]\s+(?P<message>.+)",
    # Git commit --amend or similar: [branch sha (amend)] message
    r"\[(?P<branch>[\w\-/\.]+)\s+(?P<sha>[a-f0-9]{7,40})\s+\([^)]+\)\]\s+(?P<message>.+)",
]

# Pattern for file change statistics
FILE_CHANGE_PATTERN = r"^\s*(?P<file>.+?)\s*\|\s*(?P<changes>\d+|Bin)"
STAT_SUMMARY_PATTERN = r"(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?"

# Pattern for diff file headers
DIFF_FILE_PATTERN = r"^diff --git a/(.+) b/(.+)$"
DIFF_STAT_LINE = r"^\s*(.+?)\s*\|\s*(\d+)\s*[+\-]*$"


def is_git_commit_output(tool_name: str, output: str) -> bool:
    """Check if the output looks like a git commit result.

    Detects git commit output from shell/bash tool execution.

    Args:
        tool_name: The name of the tool that produced the output.
        output: The output string to check.

    Returns:
        True if the output appears to be from a git commit.

    Examples:
        >>> is_git_commit_output("Bash", "[main abc1234] Fix bug\\n1 file changed")
        True
        >>> is_git_commit_output("Bash", "ls -la output")
        False
    """
    # Only consider shell/bash tool outputs
    tool_name_lower = tool_name.lower()
    if tool_name_lower not in ("bash", "shell", "execute", "run_command", "terminal"):
        return False

    # Check for git commit patterns in the output
    output_lower = output.lower()

    # Quick negative check - if no commit-related terms, skip
    if "commit" not in output_lower and "[" not in output:
        return False

    # Check for standard git commit output format
    for pattern in GIT_COMMIT_PATTERNS:
        if re.search(pattern, output, re.MULTILINE):
            return True

    # Check for "create mode" or "file changed" which often appears in commit output
    if "file changed" in output_lower or "files changed" in output_lower:
        # Verify it has a commit-like header
        if re.search(r"\[[^\]]+\s+[a-f0-9]{7,40}\]", output):
            return True

    return False


def parse_git_commit(output: str, input_command: str | None = None) -> GitCommitMetadata | None:
    """Parse git commit output to extract metadata.

    Extracts commit SHA, message, changed files, and statistics from
    git commit output.

    Args:
        output: The git commit output string.
        input_command: Optional input command that produced this output.

    Returns:
        GitCommitMetadata if parsing succeeds, None otherwise.

    Examples:
        >>> result = parse_git_commit("[main abc1234] Fix bug\\n 1 file changed, 5 insertions(+)")
        >>> result.sha
        'abc1234'
        >>> result.message
        'Fix bug'
    """
    if not output:
        return None

    sha = ""
    message = ""
    branch = ""
    changed_files: list[str] = []
    insertions = 0
    deletions = 0

    lines = output.strip().split("\n")

    # Parse the commit header line
    for pattern in GIT_COMMIT_PATTERNS:
        match = re.search(pattern, output)
        if match:
            groups = match.groupdict()
            sha = groups.get("sha", "")
            message = groups.get("message", "")
            branch = groups.get("branch", "")
            break

    # If no match, try to extract SHA from other patterns
    if not sha:
        # Try to find SHA in format like "commit abc1234"
        sha_match = re.search(r"commit\s+([a-f0-9]{7,40})", output, re.IGNORECASE)
        if sha_match:
            sha = sha_match.group(1)

    # If still no SHA, this is not a valid commit output
    if not sha:
        return None

    # Parse file change statistics
    for line in lines:
        line = line.strip()

        # Check for file change pattern: " file.py | 10 +"
        file_match = re.match(FILE_CHANGE_PATTERN, line)
        if file_match:
            file_path = file_match.group("file").strip()
            # Clean up the file path (remove leading/trailing whitespace)
            if file_path and not file_path.startswith("("):
                changed_files.append(file_path)
            continue

        # Check for "create mode" or "delete mode" lines
        mode_match = re.match(r"^\s*(create|delete)\s+mode\s+\d+\s+(.+)$", line)
        if mode_match:
            file_path = mode_match.group(2).strip()
            if file_path and file_path not in changed_files:
                changed_files.append(file_path)
            continue

        # Check for rename/copy lines
        rename_match = re.match(r"^\s*rename\s+(.+)\s+=>\s+(.+)\s+\(\d+%\)$", line)
        if rename_match:
            # Add the new name
            new_name = rename_match.group(2).strip()
            if new_name and new_name not in changed_files:
                changed_files.append(new_name)
            continue

    # Parse summary statistics
    stat_match = re.search(STAT_SUMMARY_PATTERN, output)
    if stat_match:
        if stat_match.group(2):
            insertions = int(stat_match.group(2))
        if stat_match.group(3):
            deletions = int(stat_match.group(3))

    # Try to parse diff output if present
    diff_lines = []
    in_diff = False
    for line in lines:
        if line.startswith("diff --git"):
            in_diff = True
            diff_match = re.match(DIFF_FILE_PATTERN, line)
            if diff_match:
                file_b = diff_match.group(2)
                if file_b not in changed_files:
                    changed_files.append(file_b)
        if in_diff:
            diff_lines.append(line)

    raw_diff = "\n".join(diff_lines) if diff_lines else ""

    # Extract author from input command if available
    author = ""
    if input_command:
        author_match = re.search(r'--author[=\s]+["\']?([^"\']+)["\']?', input_command)
        if author_match:
            author = author_match.group(1)

    return GitCommitMetadata(
        sha=sha,
        message=message,
        branch=branch,
        changed_files=changed_files,
        insertions=insertions,
        deletions=deletions,
        author=author,
        raw_diff=raw_diff[:5000] if raw_diff else "",  # Limit diff size
    )


def extract_changed_files_from_diff(diff_output: str) -> list[str]:
    """Extract list of changed files from a git diff output.

    Args:
        diff_output: Raw git diff output string.

    Returns:
        List of file paths that were changed.

    Examples:
        >>> diff = "diff --git a/foo.py b/foo.py\\n+++ b/foo.py"
        >>> extract_changed_files_from_diff(diff)
        ['foo.py']
    """
    files: list[str] = []

    for line in diff_output.split("\n"):
        match = re.match(DIFF_FILE_PATTERN, line)
        if match:
            file_b = match.group(2)
            if file_b not in files:
                files.append(file_b)

    return files


def format_commit_content(metadata: GitCommitMetadata) -> str:
    """Format commit metadata as readable content for observation.

    Creates a human-readable summary of the commit for storage
    as observation content.

    Args:
        metadata: The parsed commit metadata.

    Returns:
        Formatted string summarizing the commit.
    """
    lines = [
        f"Git Commit: {metadata.sha}",
        f"Message: {metadata.message}",
    ]

    if metadata.branch:
        lines.append(f"Branch: {metadata.branch}")

    if metadata.author:
        lines.append(f"Author: {metadata.author}")

    if metadata.changed_files:
        lines.append(f"\nChanged files ({len(metadata.changed_files)}):")
        for f in metadata.changed_files[:20]:  # Limit to 20 files
            lines.append(f"  - {f}")
        if len(metadata.changed_files) > 20:
            lines.append(f"  ... and {len(metadata.changed_files) - 20} more")

    if metadata.insertions or metadata.deletions:
        lines.append(f"\nStats: +{metadata.insertions} -{metadata.deletions}")

    return "\n".join(lines)


# Capture rules registry - maps tool patterns to capture handlers
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
    # Git operations (high importance)
    "git_commit": 8,
    "Bash": 5,  # Default for bash, elevated if git commit detected
    # Navigation (medium importance)
    "list_dir": 5,
    "find_file": 5,
    # C4 MCP tools (capture for context)
    "c4_get_task": 7,
    "c4_submit": 8,
}


def get_capture_importance(tool_name: str, output: str | None = None) -> int:
    """Get capture importance for a tool, with output-based adjustments.

    If the tool is Bash and the output looks like a git commit,
    the importance is elevated to 8.

    Args:
        tool_name: The name of the tool.
        output: Optional output to analyze for special cases.

    Returns:
        Importance level (1-10).
    """
    base_importance = CAPTURE_RULES.get(tool_name, 5)

    # Check for special cases that elevate importance
    if output and tool_name.lower() in ("bash", "shell", "execute", "run_command"):
        if is_git_commit_output(tool_name, output):
            return 8  # Git commits are high importance

    return base_importance
