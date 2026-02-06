"""Git context capture - commit, branch, and dirty state.

Absorbed from piq/piqr/git_extractor.py.
"""

from __future__ import annotations

import subprocess


def capture_git_context(cwd: str | None = None) -> dict:
    """Capture current git repository state.

    Args:
        cwd: Working directory (default: current)

    Returns:
        Dict with commit, branch, dirty, remote info
    """
    ctx: dict = {}

    try:
        ctx["commit"] = _run_git(["rev-parse", "HEAD"], cwd).strip()[:12]
    except Exception:
        ctx["commit"] = None

    try:
        ctx["branch"] = _run_git(["rev-parse", "--abbrev-ref", "HEAD"], cwd).strip()
    except Exception:
        ctx["branch"] = None

    try:
        status = _run_git(["status", "--porcelain"], cwd).strip()
        ctx["dirty"] = len(status) > 0
        if status:
            ctx["dirty_files"] = len(status.splitlines())
    except Exception:
        ctx["dirty"] = None

    try:
        ctx["remote_url"] = _run_git(
            ["config", "--get", "remote.origin.url"], cwd
        ).strip()
    except Exception:
        ctx["remote_url"] = None

    return ctx


def _run_git(args: list[str], cwd: str | None = None) -> str:
    """Run a git command and return output."""
    result = subprocess.run(
        ["git"] + args,
        capture_output=True,
        text=True,
        cwd=cwd,
        timeout=5,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr)
    return result.stdout
