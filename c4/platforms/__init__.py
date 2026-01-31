"""C4 Platform Support - Multi-platform CLI launcher and command validation.

This module provides:
- Platform-specific CLI launcher configuration
- Command validation for each platform
- Template generation for missing commands
- Global/project platform configuration management
"""

import os
from pathlib import Path
from typing import Any

import yaml

# =============================================================================
# Platform Configuration
# =============================================================================

# Supported platforms and their launch commands
# Note: "." means current directory (passed after os.chdir to project_path)
PLATFORM_COMMANDS: dict[str, list[str]] = {
    "claude": ["claude", "."],
    "cursor": ["cursor", "."],
    "codex": ["codex", "."],
    "gemini": ["gemini", "."],
    "opencode": ["opencode", "."],
}

# Platform-specific command directories
PLATFORM_COMMAND_DIRS: dict[str, str] = {
    "claude": ".claude/commands",
    "cursor": ".cursor/commands",
    "codex": ".codex/agents",
    "gemini": ".gemini/commands",
}

# Required commands that each platform should implement
REQUIRED_COMMANDS: list[str] = [
    "c4-status",
    "c4-init",
    "c4-plan",
    "c4-run",
    "c4-stop",
    "c4-checkpoint",
    "c4-submit",
    "c4-validate",
    "c4-add-task",
    "c4-clear",
]

# Commands that are complex and should be manually adapted per platform
COMPLEX_COMMANDS: list[str] = [
    "c4-plan",
    "c4-run",
    "c4-checkpoint",
    "c4-submit",
]


# =============================================================================
# Platform Resolution
# =============================================================================


def get_default_platform(project_path: Path | None = None) -> str:
    """Get the default platform based on configuration hierarchy.

    Priority (highest to lowest):
    1. Project config (.c4/config.yaml)
    2. Global config (~/.c4/config.yaml)
    3. Environment variable (C4_PLATFORM)
    4. Default value ("claude")

    Args:
        project_path: Project directory path. Defaults to cwd.

    Returns:
        Platform name (e.g., "claude", "cursor", "codex")
    """
    if project_path is None:
        project_path = Path.cwd()

    # 1. Project config
    project_config = project_path / ".c4" / "config.yaml"
    if project_config.exists():
        try:
            config = yaml.safe_load(project_config.read_text())
            if config and (platform := config.get("platform")):
                return platform
        except yaml.YAMLError:
            pass

    # 2. Global config
    global_config = Path.home() / ".c4" / "config.yaml"
    if global_config.exists():
        try:
            config = yaml.safe_load(global_config.read_text())
            if config and (platform := config.get("platform")):
                return platform
        except yaml.YAMLError:
            pass

    # 3. Environment variable
    if platform := os.environ.get("C4_PLATFORM"):
        return platform

    # 4. Default
    return "claude"


def get_platform_command(platform: str) -> list[str] | None:
    """Get the shell command to launch a platform.

    Args:
        platform: Platform name

    Returns:
        List of command arguments, or None if platform unknown
    """
    return PLATFORM_COMMANDS.get(platform)


def list_platforms() -> list[str]:
    """Get list of supported platforms."""
    return list(PLATFORM_COMMANDS.keys())


# =============================================================================
# Configuration Management
# =============================================================================


def set_platform_config(
    platform: str,
    global_config: bool = False,
    project_path: Path | None = None,
) -> Path:
    """Set the default platform in config file.

    Args:
        platform: Platform name to set as default
        global_config: If True, set in ~/.c4/config.yaml, else project config
        project_path: Project directory for project config

    Returns:
        Path to the updated config file

    Raises:
        ValueError: If platform is not supported
    """
    if platform not in PLATFORM_COMMANDS:
        raise ValueError(f"Unknown platform: {platform}. Supported: {list_platforms()}")

    if global_config:
        config_path = Path.home() / ".c4" / "config.yaml"
    else:
        if project_path is None:
            project_path = Path.cwd()
        config_path = project_path / ".c4" / "config.yaml"

    # Load existing config or create new
    config: dict[str, Any] = {}
    if config_path.exists():
        try:
            existing = yaml.safe_load(config_path.read_text())
            if existing:
                config = existing
        except yaml.YAMLError:
            pass

    # Update platform
    config["platform"] = platform

    # Write back
    config_path.parent.mkdir(parents=True, exist_ok=True)
    config_path.write_text(yaml.dump(config, default_flow_style=False, allow_unicode=True))

    return config_path


def get_config_info(project_path: Path | None = None) -> dict[str, Any]:
    """Get current configuration info for display.

    Args:
        project_path: Project directory path

    Returns:
        Dict with global_platform, project_platform, effective_platform, source
    """
    if project_path is None:
        project_path = Path.cwd()

    info: dict[str, Any] = {
        "global_platform": None,
        "project_platform": None,
        "env_platform": os.environ.get("C4_PLATFORM"),
        "effective_platform": None,
        "source": "default",
    }

    # Global config
    global_config = Path.home() / ".c4" / "config.yaml"
    if global_config.exists():
        try:
            config = yaml.safe_load(global_config.read_text())
            if config:
                info["global_platform"] = config.get("platform")
        except yaml.YAMLError:
            pass

    # Project config
    project_config = project_path / ".c4" / "config.yaml"
    if project_config.exists():
        try:
            config = yaml.safe_load(project_config.read_text())
            if config:
                info["project_platform"] = config.get("platform")
        except yaml.YAMLError:
            pass

    # Determine effective platform and source
    if info["project_platform"]:
        info["effective_platform"] = info["project_platform"]
        info["source"] = "project"
    elif info["global_platform"]:
        info["effective_platform"] = info["global_platform"]
        info["source"] = "global"
    elif info["env_platform"]:
        info["effective_platform"] = info["env_platform"]
        info["source"] = "environment"
    else:
        info["effective_platform"] = "claude"
        info["source"] = "default"

    return info


# =============================================================================
# Command Validation
# =============================================================================


def get_command_dir(project_path: Path, platform: str) -> Path:
    """Get the command directory for a platform.

    Args:
        project_path: Project root directory
        platform: Platform name

    Returns:
        Path to command directory
    """
    rel_path = PLATFORM_COMMAND_DIRS.get(platform, f".{platform}/commands")
    return project_path / rel_path


def validate_platform_commands(
    project_path: Path,
    platform: str,
) -> dict[str, list[str]]:
    """Validate that required commands exist for a platform.

    Args:
        project_path: Project root directory
        platform: Platform name to validate

    Returns:
        Dict with 'missing' and 'found' command lists
    """
    cmd_dir = get_command_dir(project_path, platform)

    # For codex, command files might have different extensions
    if platform == "codex":
        extensions = [".md", ".yaml", ".yml"]
    else:
        extensions = [".md"]

    found: list[str] = []
    missing: list[str] = []

    for cmd in REQUIRED_COMMANDS:
        cmd_exists = False
        for ext in extensions:
            if (cmd_dir / f"{cmd}{ext}").exists():
                cmd_exists = True
                break

        if cmd_exists:
            found.append(cmd)
        else:
            missing.append(cmd)

    return {"found": found, "missing": missing}


def generate_command_template(
    project_path: Path,
    platform: str,
    command: str,
    reference_platform: str = "claude",
) -> Path | None:
    """Generate a template for a missing command.

    For simple commands, creates a basic template.
    For complex commands, copies from reference platform with a note to customize.

    Args:
        project_path: Project root directory
        platform: Target platform
        command: Command name (e.g., "c4-status")
        reference_platform: Platform to copy complex commands from

    Returns:
        Path to created template, or None if reference doesn't exist
    """
    target_dir = get_command_dir(project_path, platform)
    target_dir.mkdir(parents=True, exist_ok=True)

    target_file = target_dir / f"{command}.md"

    # Check if reference exists for complex commands
    if command in COMPLEX_COMMANDS:
        ref_dir = get_command_dir(project_path, reference_platform)
        ref_file = ref_dir / f"{command}.md"

        if ref_file.exists():
            # Copy with customization note
            content = ref_file.read_text()
            header = f"""<!--
  C4 Platform: {platform}
  Based on: {reference_platform} version

  TODO: Customize this command for {platform}
  - Check MCP tool call syntax
  - Update platform-specific instructions
  - Test in {platform} environment
-->

"""
            target_file.write_text(header + content)
            return target_file
        else:
            return None

    # For simple commands, generate basic template
    template = _get_simple_command_template(command, platform)
    target_file.write_text(template)
    return target_file


def _get_simple_command_template(command: str, platform: str) -> str:
    """Get template content for simple commands."""

    templates: dict[str, str] = {
        "c4-status": """# C4 Project Status

Show the current C4 project status.

## Instructions

1. Call `c4_status()` MCP tool to get project status
2. Display the result in a formatted way:
   - Project ID and current state
   - Task queue (pending, in_progress, done)
   - Active workers
   - Metrics

## Usage

```
/c4-status
```
""",
        "c4-init": """# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

1. Run C4 initialization for the current project
2. Verify .c4/ directory was created
3. Check if Claude Code restart is needed (if .mcp.json was newly created)

## Usage

```
/c4-init
/c4-init myproject  # with custom project ID
```
""",
        "c4-stop": """# C4 Stop Execution

Stop the current C4 execution.

## Instructions

1. Call `c4_status()` to check current state
2. If in EXECUTE state, call `c4_stop()` to halt execution
3. Confirm the state changed to HALTED

## Usage

```
/c4-stop
```
""",
        "c4-validate": """# C4 Run Validation

Run validation commands for the current task.

## Instructions

1. Call `c4_run_validation(names)` with validation names
2. Common validations: ["lint", "unit", "e2e"]
3. Display results

## Usage

```
/c4-validate
/c4-validate lint unit
```
""",
        "c4-add-task": """# C4 Add Task

Add a new task to the C4 queue.

## Instructions

1. Collect task information:
   - task_id: Unique identifier
   - title: Task title
   - dod: Definition of Done (must be specific and verifiable)
   - scope: Affected files/directories
2. Call `c4_add_todo()` MCP tool

## Usage

```
/c4-add-task T-001 --title "Implement login" --dod "Login returns JWT token"
```
""",
        "c4-clear": """# C4 Clear State

Clear C4 state and start fresh.

## Instructions

1. Confirm with user before clearing
2. Call `c4_clear(confirm=True)` MCP tool
3. Optionally keep config with `keep_config=True`

## Usage

```
/c4-clear
/c4-clear --keep-config
```
""",
    }

    if command in templates:
        return templates[command]

    # Generic template for unknown commands
    return f"""# {command}

<!-- Template for {platform} platform -->

## Instructions

TODO: Implement this command for {platform}

## Usage

```
/{command}
```
"""


# =============================================================================
# Platform Setup
# =============================================================================


def setup_platform(
    project_path: Path,
    platform: str,
    generate_templates: bool = True,
) -> dict[str, Any]:
    """Set up a platform for C4 usage.

    Creates necessary directories and validates/generates commands.

    Args:
        project_path: Project root directory
        platform: Platform to set up
        generate_templates: Whether to generate templates for missing commands

    Returns:
        Dict with setup results
    """
    results: dict[str, Any] = {
        "platform": platform,
        "command_dir": str(get_command_dir(project_path, platform)),
        "validation": validate_platform_commands(project_path, platform),
        "generated": [],
        "skipped": [],
    }

    if generate_templates and results["validation"]["missing"]:
        for cmd in results["validation"]["missing"]:
            template_path = generate_command_template(project_path, platform, cmd)
            if template_path:
                results["generated"].append(cmd)
            else:
                results["skipped"].append(cmd)

    return results
