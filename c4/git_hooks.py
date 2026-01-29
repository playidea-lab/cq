"""C4 Git Hooks - Install and manage Git hooks for C4 integration.

This module provides Git hooks that integrate with C4:
- pre-commit: Runs lint validation before commits
- commit-msg: Validates Task ID in commit messages
- post-commit: Updates task commit_sha in the database
"""

import stat
from pathlib import Path

# =============================================================================
# Hook Templates
# =============================================================================

PRE_COMMIT_HOOK = '''#!/bin/bash
# C4 Git Hook: pre-commit
# Runs lint validation on staged files before allowing commit

set -e

# C4 프로젝트가 아니면 스킵
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

# Get staged Python files
STAGED_PY=$(git diff --cached --name-only --diff-filter=ACM | grep -E '\\.py$' || true)

if [[ -z "$STAGED_PY" ]]; then
    echo "C4: No Python files staged, skipping lint."
    exit 0
fi

echo "C4: Running pre-commit validation on staged files..."
if command -v uv &> /dev/null; then
    # Check only staged files
    echo "$STAGED_PY" | xargs uv run ruff check --fix 2>/dev/null || {
        echo "C4: Lint failed. Please fix the issues and try again."
        exit 1
    }
else
    echo "C4: Warning - uv not found, skipping lint"
fi

echo "C4: Pre-commit validation passed."
exit 0
'''

COMMIT_MSG_HOOK = '''#!/bin/bash
# C4 Git Hook: commit-msg
# 1. Task ID 검증
# 2. Task 상태 업데이트 (선택적)

COMMIT_MSG_FILE="$1"
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

# C4 프로젝트가 아니면 스킵
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

# Task ID 추출 패턴: [T-XXX-N], [R-XXX-N], [CP-XXX]
TASK_PATTERN='\\[T-[0-9]+-[0-9]+\\]|\\[R-[0-9]+-[0-9]+\\]|\\[CP-[0-9]+\\]'
TASK_ID=$(echo "$COMMIT_MSG" | grep -oE "$TASK_PATTERN" | head -1 | tr -d '[]')

if [[ -n "$TASK_ID" ]]; then
    echo "C4: Task ID found: $TASK_ID"
    exit 0
fi

# Task ID 없음 - 경고 또는 차단
MODE="${C4_COMMIT_MSG_MODE:-warn}"
if [[ "$MODE" == "strict" ]]; then
    echo ""
    echo "C4: ERROR - Commit message must include a Task ID."
    echo "    Format: [T-XXX-N] or [R-XXX-N] or [CP-XXX]"
    echo "    Example: [T-001-0] Implement feature X"
    exit 1
fi

echo ""
echo "C4: WARNING - Commit message does not include a Task ID."
echo "    Consider using: [T-XXX-N] message"
exit 0
'''

POST_COMMIT_HOOK = '''#!/bin/bash
# C4 Git Hook: post-commit
# Task commit_sha 자동 업데이트

# C4 프로젝트가 아니면 스킵
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

COMMIT_SHA=$(git rev-parse HEAD)
COMMIT_MSG=$(git log -1 --pretty=%B)

# Task ID 추출
PATTERN='\\[T-[0-9]+-[0-9]+\\]|\\[R-[0-9]+-[0-9]+\\]'
TASK_ID=$(echo "$COMMIT_MSG" | grep -oE "$PATTERN" | head -1 | tr -d '[]')

if [[ -n "$TASK_ID" ]]; then
    echo "C4: Commit $COMMIT_SHA associated with task $TASK_ID"

    # SQLite 직접 업데이트 (MCP 없이도 동작)
    # c4_tasks 테이블의 task_json 내부 commit_sha 업데이트
    if [[ -f ".c4/tasks.db" ]]; then
        SQL="UPDATE c4_tasks SET task_json = json_set(task_json, '$.commit_sha', '$COMMIT_SHA') "
        SQL+="WHERE task_id='$TASK_ID' AND status='in_progress';"
        sqlite3 .c4/tasks.db "$SQL" 2>/dev/null || true
    fi
fi

exit 0
'''


# =============================================================================
# Installation Functions
# =============================================================================

def get_git_hooks_dir(project_path: Path | None = None) -> Path | None:
    """Get .git/hooks directory for the project.

    Args:
        project_path: Project root directory. Defaults to current directory.

    Returns:
        Path to .git/hooks/ or None if not a git repository.
    """
    if project_path is None:
        project_path = Path.cwd()

    git_dir = project_path / ".git"
    if not git_dir.is_dir():
        return None

    hooks_dir = git_dir / "hooks"
    return hooks_dir


def install_hook(
    hook_name: str,
    hook_content: str,
    project_path: Path | None = None,
    force: bool = False,
) -> tuple[bool, str]:
    """Install a single Git hook.

    Args:
        hook_name: Name of the hook (e.g., 'pre-commit')
        hook_content: Content of the hook script
        project_path: Project root directory
        force: Overwrite existing hook if True

    Returns:
        Tuple of (success: bool, message: str)
    """
    hooks_dir = get_git_hooks_dir(project_path)

    if hooks_dir is None:
        return False, "Not a Git repository"

    # Create hooks directory if needed
    hooks_dir.mkdir(parents=True, exist_ok=True)

    hook_path = hooks_dir / hook_name

    # Check for existing hook
    if hook_path.exists() and not force:
        # Check if it's a C4 hook
        content = hook_path.read_text()
        if "C4 Git Hook" not in content:
            return False, f"Existing non-C4 hook at {hook_path}. Use force=True to overwrite."
        # It's a C4 hook, safe to overwrite

    # Write the hook
    hook_path.write_text(hook_content)

    # Make executable
    hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    return True, f"Installed {hook_name}"


def install_all_hooks(
    project_path: Path | None = None,
    force: bool = False,
) -> dict[str, tuple[bool, str]]:
    """Install all C4 Git hooks.

    Args:
        project_path: Project root directory
        force: Overwrite existing hooks if True

    Returns:
        Dictionary mapping hook name to (success, message) tuple
    """
    hooks = {
        "pre-commit": PRE_COMMIT_HOOK,
        "commit-msg": COMMIT_MSG_HOOK,
        "post-commit": POST_COMMIT_HOOK,
    }

    results = {}
    for hook_name, hook_content in hooks.items():
        results[hook_name] = install_hook(
            hook_name, hook_content, project_path, force
        )

    return results


def uninstall_hook(
    hook_name: str,
    project_path: Path | None = None,
) -> tuple[bool, str]:
    """Uninstall a single Git hook (only if it's a C4 hook).

    Args:
        hook_name: Name of the hook
        project_path: Project root directory

    Returns:
        Tuple of (success: bool, message: str)
    """
    hooks_dir = get_git_hooks_dir(project_path)

    if hooks_dir is None:
        return False, "Not a Git repository"

    hook_path = hooks_dir / hook_name

    if not hook_path.exists():
        return True, f"Hook {hook_name} not found (already uninstalled)"

    # Only remove C4 hooks
    content = hook_path.read_text()
    if "C4 Git Hook" not in content:
        return False, f"Hook {hook_name} is not a C4 hook, skipping"

    hook_path.unlink()
    return True, f"Uninstalled {hook_name}"


def uninstall_all_hooks(
    project_path: Path | None = None,
) -> dict[str, tuple[bool, str]]:
    """Uninstall all C4 Git hooks.

    Args:
        project_path: Project root directory

    Returns:
        Dictionary mapping hook name to (success, message) tuple
    """
    hook_names = ["pre-commit", "commit-msg", "post-commit"]

    results = {}
    for hook_name in hook_names:
        results[hook_name] = uninstall_hook(hook_name, project_path)

    return results


def check_hooks_installed(project_path: Path | None = None) -> dict[str, bool]:
    """Check which C4 Git hooks are installed.

    Args:
        project_path: Project root directory

    Returns:
        Dictionary mapping hook name to installation status
    """
    hooks_dir = get_git_hooks_dir(project_path)
    hook_names = ["pre-commit", "commit-msg", "post-commit"]

    if hooks_dir is None:
        return {name: False for name in hook_names}

    results = {}
    for hook_name in hook_names:
        hook_path = hooks_dir / hook_name
        if hook_path.exists():
            content = hook_path.read_text()
            results[hook_name] = "C4 Git Hook" in content
        else:
            results[hook_name] = False

    return results
