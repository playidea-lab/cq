"""C4 Git Hooks Management - Install and manage Git hooks for C4 workflow.

This module provides Git hooks that integrate with the C4 workflow:
- pre-commit: Run lint validation before committing
- commit-msg: Validate commit messages contain Task IDs
"""

from __future__ import annotations

import os
import stat
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


# =============================================================================
# Hook Templates
# =============================================================================

PRE_COMMIT_HOOK = """#!/bin/bash
# C4 Git Hook: pre-commit
# Runs lint validation before allowing commit

set -e

# Skip if not in a C4 project
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

# Run lint validation
echo "C4: Running pre-commit validation..."
if command -v uv &> /dev/null; then
    uv run ruff check . --fix 2>/dev/null || {
        echo "C4: Lint failed. Please fix the issues and try again."
        exit 1
    }
else
    echo "C4: Warning - uv not found, skipping lint"
fi

echo "C4: Pre-commit validation passed."
exit 0
"""

COMMIT_MSG_HOOK = """#!/bin/bash
# C4 Git Hook: commit-msg
# Validates commit messages contain Task IDs (e.g., [T-001])

COMMIT_MSG_FILE="$1"
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

# Skip if not in a C4 project
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

# Task ID pattern: [T-XXX], [R-XXX], [CP-XXX], etc.
TASK_PATTERN='\\[T-[0-9]+-[0-9]+\\]|\\[R-[0-9]+-[0-9]+\\]|\\[CP-[0-9]+\\]'

# Check for task ID
if echo "$COMMIT_MSG" | grep -qE "$TASK_PATTERN"; then
    exit 0
fi

# Mode from environment or config
MODE="${C4_COMMIT_MSG_MODE:-warn}"

if [[ "$MODE" == "strict" ]]; then
    echo ""
    echo "C4: ERROR - Commit message must include a Task ID."
    echo "    Format: [T-XXX-N] or [R-XXX-N] or [CP-XXX]"
    echo "    Example: [T-001-0] feat: implement login"
    echo ""
    echo "    Your message: $COMMIT_MSG"
    echo ""
    exit 1
fi

# Warn mode (default)
echo ""
echo "C4: WARNING - Commit message does not include a Task ID."
echo "    Consider using: [T-XXX-N] message"
echo ""
exit 0
"""

POST_COMMIT_HOOK = """#!/bin/bash
# C4 Git Hook: post-commit
# Sync C4 state after commit and generate event file for daemon processing

# Skip if not in a C4 project
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

# Get commit info
COMMIT_SHA=$(git rev-parse HEAD)
COMMIT_MSG=$(git log -1 --pretty=%B)
CHANGED_FILES=$(git diff-tree --no-commit-id --name-only -r HEAD | tr '\\n' ',' | sed 's/,$//')
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Extract task ID if present (supports T-XXX-N, R-XXX-N, CP-XXX formats)
TASK_PATTERN='\\[T-[0-9A-Za-z]+-[0-9]+(-[0-9]+)?\\]'
TASK_PATTERN="${TASK_PATTERN}|\\[R-[0-9A-Za-z]+-[0-9]+(-[0-9]+)?\\]"
TASK_PATTERN="${TASK_PATTERN}|\\[CP-[0-9]+\\]"
TASK_ID=$(echo "$COMMIT_MSG" | grep -oE "$TASK_PATTERN" | head -1 | tr -d '[]')

if [[ -n "$TASK_ID" ]]; then
    echo "C4: Commit associated with task $TASK_ID"
fi

# Create events directory if it doesn't exist
mkdir -p .c4/events

# Generate event file for daemon processing
SHORT_SHA="${COMMIT_SHA:0:7}"
EVENT_FILE=".c4/events/git-${SHORT_SHA}.json"

# Write event JSON (null if TASK_ID is empty)
if [[ -n "$TASK_ID" ]]; then
    TASK_ID_JSON="\"$TASK_ID\""
else
    TASK_ID_JSON="null"
fi

cat > "$EVENT_FILE" << EOF
{
  "type": "git_commit",
  "sha": "$COMMIT_SHA",
  "task_id": $TASK_ID_JSON,
  "files": "$CHANGED_FILES",
  "timestamp": "$TIMESTAMP"
}
EOF

echo "C4: Event file created at $EVENT_FILE"

exit 0
"""


def get_git_hooks_dir() -> Path | None:
    """Find the .git/hooks directory for the current repository.

    Returns:
        Path to .git/hooks directory, or None if not in a git repo.
    """
    try:
        # Walk up to find .git directory
        current = Path.cwd()
        while current != current.parent:
            git_dir = current / ".git"
            if git_dir.is_dir():
                return git_dir / "hooks"
            current = current.parent
        return None
    except Exception:
        return None


def install_hook(hook_name: str, content: str, force: bool = False) -> tuple[bool, str]:
    """Install a single Git hook.

    Args:
        hook_name: Name of the hook (e.g., "pre-commit")
        content: Content of the hook script
        force: If True, overwrite existing hooks

    Returns:
        Tuple of (success, message)
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hooks_dir.mkdir(parents=True, exist_ok=True)
    hook_path = hooks_dir / hook_name

    # Check for existing hook
    if hook_path.exists() and not force:
        # Check if it's our hook
        existing_content = hook_path.read_text()
        if "C4 Git Hook" in existing_content:
            return True, f"{hook_name}: Already installed (C4)"
        else:
            return False, f"{hook_name}: Existing hook found (use --force to overwrite)"

    # Write hook
    hook_path.write_text(content)

    # Make executable
    hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    return True, f"{hook_name}: Installed"


def uninstall_hook(hook_name: str) -> tuple[bool, str]:
    """Uninstall a single Git hook.

    Args:
        hook_name: Name of the hook to uninstall

    Returns:
        Tuple of (success, message)
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hook_path = hooks_dir / hook_name

    if not hook_path.exists():
        return True, f"{hook_name}: Not installed"

    # Check if it's our hook
    content = hook_path.read_text()
    if "C4 Git Hook" not in content:
        return False, f"{hook_name}: Not a C4 hook (skipped)"

    hook_path.unlink()
    return True, f"{hook_name}: Uninstalled"


def get_hook_status(hook_name: str) -> dict:
    """Get status of a Git hook.

    Args:
        hook_name: Name of the hook

    Returns:
        Dict with status information
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return {"installed": False, "is_c4": False, "error": "Not in git repo"}

    hook_path = hooks_dir / hook_name

    if not hook_path.exists():
        return {"installed": False, "is_c4": False}

    content = hook_path.read_text()
    is_c4 = "C4 Git Hook" in content
    is_executable = os.access(hook_path, os.X_OK)

    return {
        "installed": True,
        "is_c4": is_c4,
        "executable": is_executable,
        "path": str(hook_path),
    }


# =============================================================================
# High-level API
# =============================================================================

HOOKS = {
    "pre-commit": PRE_COMMIT_HOOK,
    "commit-msg": COMMIT_MSG_HOOK,
    "post-commit": POST_COMMIT_HOOK,
}


def install_all_hooks(force: bool = False) -> dict[str, tuple[bool, str]]:
    """Install all C4 Git hooks.

    Args:
        force: If True, overwrite existing hooks

    Returns:
        Dict mapping hook name to (success, message)
    """
    results = {}
    for hook_name, content in HOOKS.items():
        results[hook_name] = install_hook(hook_name, content, force)
    return results


def uninstall_all_hooks() -> dict[str, tuple[bool, str]]:
    """Uninstall all C4 Git hooks.

    Returns:
        Dict mapping hook name to (success, message)
    """
    results = {}
    for hook_name in HOOKS:
        results[hook_name] = uninstall_hook(hook_name)
    return results


def get_all_hook_status() -> dict[str, dict]:
    """Get status of all C4 Git hooks.

    Returns:
        Dict mapping hook name to status dict
    """
    results = {}
    for hook_name in HOOKS:
        results[hook_name] = get_hook_status(hook_name)
    return results
