"""C4 Hooks Management - Install and register Claude Code hooks."""

import json
import shutil
from pathlib import Path

# =============================================================================
# Stop Hook Content (inline to avoid template dependency)
# =============================================================================

STOP_HOOK_SH = """#!/bin/bash
# C4 Stop Hook - Block exit if work remains

# C4 not initialized -> allow exit
if [[ ! -f ".c4/state.json" ]]; then
    exit 0
fi

# Check state via Python script
result=$(python3 ~/.claude/hooks/c4-stop-hook.py 2>/dev/null)
exit_code=$?

if [[ $exit_code -eq 2 ]]; then
    cat << EOF
{
    "decision": "block",
    "reason": "$result",
    "instructions": "There are pending tasks. Continue working with /c4-worker"
}
EOF
    exit 2
fi

exit 0
"""


def get_c4_install_dir() -> Path:
    """Get C4 installation directory."""
    # Try install path file first
    install_path_file = Path.home() / ".c4-install-path"
    if install_path_file.exists():
        return Path(install_path_file.read_text().strip())

    # Default locations
    default_paths = [
        Path.home() / ".c4",
        Path.home() / "git" / "c4",
    ]
    for p in default_paths:
        if (p / "c4").is_dir():
            return p

    # Fallback: use this file's location
    return Path(__file__).parent.parent


def install_stop_hook() -> bool:
    """Install stop.sh and c4-stop-hook.py to ~/.claude/hooks/"""
    hooks_dir = Path.home() / ".claude" / "hooks"
    hooks_dir.mkdir(parents=True, exist_ok=True)

    # Install stop.sh
    stop_sh = hooks_dir / "stop.sh"
    stop_sh.write_text(STOP_HOOK_SH)
    stop_sh.chmod(0o755)

    # Copy c4-stop-hook.py from templates
    c4_install_dir = get_c4_install_dir()
    src = c4_install_dir / "templates" / "c4-stop-hook.py"
    dst = hooks_dir / "c4-stop-hook.py"

    if src.exists():
        shutil.copy(src, dst)
        dst.chmod(0o755)
        return True
    else:
        # Fallback: create minimal version
        dst.write_text('''#!/usr/bin/env python3
"""C4 Stop Hook - Check if work remains."""
import json
import sys
from pathlib import Path

state_file = Path(".c4/state.json")
if not state_file.exists():
    sys.exit(0)

try:
    state = json.loads(state_file.read_text())
except Exception:
    sys.exit(0)

status = state.get("status", "")

if status == "EXECUTE":
    tasks = state.get("tasks", [])
    pending = [t for t in tasks if t.get("status") == "pending"]
    in_progress = [t for t in tasks if t.get("status") == "in_progress"]
    if pending or in_progress:
        print(f"{len(pending)} pending, {len(in_progress)} in_progress tasks remain")
        sys.exit(2)

if status == "CHECKPOINT":
    cp_queue = state.get("checkpoint_queue", [])
    if cp_queue:
        print(f"Checkpoint - AI review in progress (automatic)")
        sys.exit(2)

sys.exit(0)
''')
        dst.chmod(0o755)
        return True


def install_security_hook() -> bool:
    """Install bash security hook to ~/.claude/hooks/"""
    hooks_dir = Path.home() / ".claude" / "hooks"
    hooks_dir.mkdir(parents=True, exist_ok=True)

    c4_install_dir = get_c4_install_dir()
    src = c4_install_dir / "templates" / "c4-bash-security-hook.sh"
    dst = hooks_dir / "c4-bash-security-hook.sh"

    if src.exists():
        shutil.copy(src, dst)
        dst.chmod(0o755)
        return True

    return False


def register_hooks() -> bool:
    """Register hooks in ~/.claude/settings.json"""
    settings_path = Path.home() / ".claude" / "settings.json"
    settings_path.parent.mkdir(parents=True, exist_ok=True)

    # Load existing settings
    if settings_path.exists():
        try:
            settings = json.loads(settings_path.read_text())
        except json.JSONDecodeError:
            settings = {}
    else:
        settings = {}

    # Initialize hooks structure (use PascalCase for Claude Code)
    if "hooks" not in settings:
        settings["hooks"] = {}
    if "PreToolUse" not in settings["hooks"]:
        settings["hooks"]["PreToolUse"] = []

    # Bash security hook config
    bash_hook = {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "~/.claude/hooks/c4-bash-security-hook.sh"}],
    }

    # Remove existing Bash hook, add new one
    settings["hooks"]["PreToolUse"] = [
        h for h in settings["hooks"]["PreToolUse"] if h.get("matcher") != "Bash"
    ]
    settings["hooks"]["PreToolUse"].append(bash_hook)

    # Save settings
    settings_path.write_text(json.dumps(settings, indent=2))
    return True


def install_all_hooks() -> dict:
    """Install all C4 hooks. Returns status dict."""
    results = {
        "stop_hook": install_stop_hook(),
        "security_hook": install_security_hook(),
        "hooks_registered": register_hooks(),
    }
    return results
