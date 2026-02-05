"""C4 slash commands for Claude Code.

This module provides functions to install C4 commands to ~/.claude/commands/
for global availability in Claude Code.
"""

import hashlib
import logging
from pathlib import Path

logger = logging.getLogger(__name__)

# Core C4 commands to install
CORE_COMMANDS = [
    "c4-init.md",
    "c4-plan.md",
    "c4-run.md",
    "c4-status.md",
    "c4-stop.md",
    "c4-submit.md",
    "c4-validate.md",
    "c4-checkpoint.md",
    "c4-add-task.md",
    "c4-clear.md",
    "c4-swarm.md",
    "c4-release.md",
    "c4-interview.md",
]

# C4 command version - increment when commands are updated
C4_COMMANDS_VERSION = "1.0.0"


def get_commands_dir() -> Path:
    """Get the C4 commands package directory."""
    return Path(__file__).parent


def get_target_dir() -> Path:
    """Get the target directory for installing commands (~/.claude/commands/)."""
    return Path.home() / ".claude" / "commands"


def get_command_hash(content: str) -> str:
    """Calculate hash of command content for version comparison."""
    return hashlib.sha256(content.encode()).hexdigest()[:12]


def get_installed_version(target_file: Path) -> str | None:
    """Extract C4 version from installed command file.

    Returns version string if found, None otherwise.
    """
    if not target_file.exists():
        return None

    try:
        content = target_file.read_text()
        # Look for version marker in file
        for line in content.split("\n")[:10]:
            if "C4_VERSION:" in line:
                return line.split("C4_VERSION:")[-1].strip()
        return None
    except Exception:
        return None


def should_update(source_content: str, target_file: Path) -> bool:
    """Check if target file should be updated.

    Returns True if:
    - Target doesn't exist
    - Target has different content hash
    - Target has older version
    """
    if not target_file.exists():
        return True

    try:
        target_content = target_file.read_text()
        return get_command_hash(source_content) != get_command_hash(target_content)
    except Exception:
        return True


def install_command(
    command_name: str,
    force: bool = False,
) -> tuple[bool, str]:
    """Install a single command to ~/.claude/commands/.

    Args:
        command_name: Name of the command file (e.g., 'c4-plan.md')
        force: If True, overwrite even if target exists

    Returns:
        Tuple of (success, message)
    """
    source_dir = get_commands_dir()
    target_dir = get_target_dir()
    source_file = source_dir / command_name
    target_file = target_dir / command_name

    if not source_file.exists():
        return False, f"Source command not found: {command_name}"

    try:
        source_content = source_file.read_text()

        # Check if update needed
        if not force and not should_update(source_content, target_file):
            return True, f"Already up to date: {command_name}"

        # Create target directory if needed
        target_dir.mkdir(parents=True, exist_ok=True)

        # Write command file
        target_file.write_text(source_content)
        return True, f"Installed: {command_name}"

    except Exception as e:
        return False, f"Failed to install {command_name}: {e}"


def install_all_commands(
    force: bool = False,
    commands: list[str] | None = None,
) -> dict[str, tuple[bool, str]]:
    """Install all C4 commands to ~/.claude/commands/.

    Args:
        force: If True, overwrite all existing files
        commands: List of specific commands to install (defaults to CORE_COMMANDS)

    Returns:
        Dict mapping command name to (success, message) tuple
    """
    if commands is None:
        commands = CORE_COMMANDS

    results = {}
    for cmd in commands:
        results[cmd] = install_command(cmd, force=force)

    return results


def uninstall_command(command_name: str) -> tuple[bool, str]:
    """Remove a C4 command from ~/.claude/commands/.

    Only removes files that appear to be C4 commands (contain 'C4' marker).
    """
    target_dir = get_target_dir()
    target_file = target_dir / command_name

    if not target_file.exists():
        return True, f"Not installed: {command_name}"

    try:
        content = target_file.read_text()
        # Safety check: only remove files that look like C4 commands
        if "C4" not in content and "c4" not in content.lower():
            return False, f"Not a C4 command (skipped): {command_name}"

        target_file.unlink()
        return True, f"Uninstalled: {command_name}"
    except Exception as e:
        return False, f"Failed to uninstall {command_name}: {e}"


def uninstall_all_commands() -> dict[str, tuple[bool, str]]:
    """Remove all C4 commands from ~/.claude/commands/."""
    results = {}
    for cmd in CORE_COMMANDS:
        results[cmd] = uninstall_command(cmd)
    return results


def get_command_status() -> dict[str, dict]:
    """Get status of all C4 commands.

    Returns dict with:
    - installed: bool
    - up_to_date: bool
    - source_hash: str
    - target_hash: str (if installed)
    """
    source_dir = get_commands_dir()
    target_dir = get_target_dir()
    status = {}

    for cmd in CORE_COMMANDS:
        source_file = source_dir / cmd
        target_file = target_dir / cmd

        if not source_file.exists():
            status[cmd] = {
                "installed": False,
                "up_to_date": False,
                "error": "Source not found",
            }
            continue

        source_content = source_file.read_text()
        source_hash = get_command_hash(source_content)

        if target_file.exists():
            try:
                target_content = target_file.read_text()
                target_hash = get_command_hash(target_content)
                status[cmd] = {
                    "installed": True,
                    "up_to_date": source_hash == target_hash,
                    "source_hash": source_hash,
                    "target_hash": target_hash,
                }
            except Exception as e:
                status[cmd] = {
                    "installed": True,
                    "up_to_date": False,
                    "error": str(e),
                }
        else:
            status[cmd] = {
                "installed": False,
                "up_to_date": False,
                "source_hash": source_hash,
            }

    return status
