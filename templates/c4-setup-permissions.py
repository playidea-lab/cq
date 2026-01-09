#!/usr/bin/env python3
"""C4 Permission Setup Script - Sets allowedTools in ~/.claude.json"""

import json
import sys
from pathlib import Path


def setup_permissions(project_path: str) -> None:
    """Set up C4 automation permissions for a project."""
    config_path = Path.home() / ".claude.json"

    # Load existing config
    if config_path.exists():
        config = json.loads(config_path.read_text())
    else:
        config = {}

    # Ensure projects structure exists
    if "projects" not in config:
        config["projects"] = {}
    if project_path not in config["projects"]:
        config["projects"][project_path] = {}

    # Set allowedTools - EXACTLY these, no modifications
    # Bash(*) is safe because c4-bash-security-hook.sh blocks dangerous commands
    config["projects"][project_path]["allowedTools"] = [
        # File operations (project only)
        f"Write(/{project_path}/**)",
        f"Edit(/{project_path}/**)",
        f"Read(/{project_path}/**)",

        # Bash - ALL commands allowed (Security Hook handles blocking)
        "Bash(*)",

        # MCP tools
        "mcp__c4",
        "mcp__serena",
        "mcp__plugin_serena_serena",
    ]

    # Save config
    config_path.write_text(json.dumps(config, indent=2))
    print(f"✅ Permissions set for {project_path}")
    print(f"   - Bash(*) enabled (Security Hook protects dangerous commands)")
    print(f"   - MCP tools: mcp__c4, mcp__serena")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: c4-setup-permissions.py <project_path>")
        sys.exit(1)

    project_path = sys.argv[1]
    setup_permissions(project_path)
