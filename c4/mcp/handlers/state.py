"""State management tool handlers.

Handles: c4_status, c4_clear, c4_start
"""

import shutil
from pathlib import Path
from typing import Any

from ..registry import register_tool


@register_tool("c4_status")
def handle_status(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get current C4 project status."""
    result = daemon.c4_status()

    # Add enforce_mode hints if enabled (from Pydantic config model)
    if hasattr(daemon, "config") and hasattr(daemon.config, "enforce_mode"):
        enforce_mode = daemon.config.enforce_mode
        if enforce_mode.enabled:
            result["enforce_mode"] = {
                "enabled": True,
                "message": enforce_mode.hints.message,
                "blocked_patterns": enforce_mode.docs.blocked_patterns,
                "docs_redirect": enforce_mode.docs.redirect_message,
                "prefer_c4_tools": enforce_mode.tools.prefer_c4_tools,
                "tools_redirect": enforce_mode.tools.redirect_message,
            }

    return result


@register_tool("c4_clear")
def handle_clear(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Clear C4 state (development/debugging)."""
    import os

    if not arguments.get("confirm"):
        return {"error": "Must set confirm=true to clear C4 state"}

    # Get current project root
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
    else:
        root = Path.cwd()

    c4_dir = root / ".c4"
    keep_config = arguments.get("keep_config", False)

    deleted_items = []
    if c4_dir.exists():
        if keep_config:
            # Delete everything except config.yaml
            config_backup = None
            config_file = c4_dir / "config.yaml"
            if config_file.exists():
                config_backup = config_file.read_text()

            shutil.rmtree(c4_dir)
            deleted_items.append(str(c4_dir))

            # Restore config
            if config_backup:
                c4_dir.mkdir(parents=True, exist_ok=True)
                config_file.write_text(config_backup)
                deleted_items.append("(config.yaml preserved)")
        else:
            shutil.rmtree(c4_dir)
            deleted_items.append(str(c4_dir))

    # Import here to avoid circular import
    from c4.mcp_server import clear_daemon_cache

    cache_cleared = clear_daemon_cache()

    return {
        "success": True,
        "deleted": deleted_items,
        "cache_cleared": cache_cleared,
        "project_root": str(root),
        "message": "C4 state cleared. Run /c4-init to reinitialize.",
    }


@register_tool("c4_start")
def handle_start(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Start execution (PLAN/HALTED -> EXECUTE)."""
    return daemon.c4_start()
