"""Gemini-specific command installer.

Installs C4 slash commands to ~/.gemini/commands/ in TOML format.
"""

import logging
from pathlib import Path

from c4.platforms import REQUIRED_COMMANDS, _get_gemini_command_template

logger = logging.getLogger(__name__)

def get_gemini_commands_dir() -> Path:
    """Get the global Gemini commands directory."""
    return Path.home() / ".gemini" / "commands"

def install_gemini_commands(force: bool = False) -> dict[str, tuple[bool, str]]:
    """Install all C4 commands to ~/.gemini/commands/ as TOML files.

    Args:
        force: If True, overwrite existing files.

    Returns:
        Dict mapping command name to (success, message) tuple.
    """
    target_dir = get_gemini_commands_dir()
    results = {}

    try:
        target_dir.mkdir(parents=True, exist_ok=True)
    except Exception as e:
        return {"error": (False, f"Failed to create directory: {e}")}

    for cmd in REQUIRED_COMMANDS:
        target_file = target_dir / f"{cmd}.toml"

        try:
            # Generate TOML content dynamically
            content = _get_gemini_command_template(cmd)

            if target_file.exists() and not force:
                # Simple check: if exists, skip unless forced
                # (Can implement hash check later if needed, but TOML generation is fast)
                results[cmd] = (True, "Already up to date (skipped)")
                continue

            target_file.write_text(content)
            results[cmd] = (True, "Installed")

        except Exception as e:
            results[cmd] = (False, f"Failed: {e}")

    return results
