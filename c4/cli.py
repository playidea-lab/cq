"""C4D CLI - Command line interface for c4d daemon and c4 project management"""

import json
import os
import shlex
import shutil
import signal
import subprocess
import sys
from pathlib import Path

import typer
from rich.console import Console
from rich.table import Table

from . import commands as c4_commands
from . import git_hooks
from .config.credentials import ENV_VAR_MAPPING, SUPPORTED_PROVIDERS, CredentialsManager
from .hooks import get_c4_install_dir, install_all_hooks
from .mcp_server import C4Daemon
from .models import ProjectStatus, Task
from .platforms import (
    PLATFORM_COMMANDS,
    get_config_info,
    get_default_platform,
    get_platform_command,
    list_platforms,
    set_platform_config,
    setup_platform,
)
from .state_machine import StateTransitionError

# =============================================================================
# CLI Apps
# =============================================================================

# c4d daemon management CLI
app = typer.Typer(
    name="c4d",
    help="C4 Daemon management",
    no_args_is_help=True,
)

# c4 project management CLI
c4_app = typer.Typer(
    name="c4",
    help="C4 Project management",
    no_args_is_help=False,  # Allow running without subcommand
    invoke_without_command=True,
)

console = Console()


# =============================================================================
# c4 Smart Entry Point (no subcommand)
# =============================================================================


@c4_app.callback()
def c4_main(
    ctx: typer.Context,
    path: Path = typer.Option(
        None,
        "--path",
        "-p",
        help="Project directory (defaults to current directory)",
    ),
    platform: str = typer.Option(
        None,
        "--platform",
        help="Target platform (claude, cursor, codex, gemini, opencode)",
    ),
):
    """C4 - AI Project Orchestration.

    Without subcommand: auto-init if needed, then start the target platform.

    Examples:
        c4                      # Auto-init + start default platform
        c4 --platform cursor    # Start Cursor
        c4 --path /my/project   # Specify project path
        c4 status               # Show status (subcommand)
        c4 init                 # Just initialize (subcommand)
        c4 config platform cursor  # Set default platform
    """
    # Set C4_PROJECT_ROOT for subcommands if --path is provided
    if path is not None:
        os.environ["C4_PROJECT_ROOT"] = str(path.resolve())

    # If a subcommand is invoked, let it handle things
    if ctx.invoked_subcommand is not None:
        return

    # Resolve project path
    project_path = (path or Path.cwd()).resolve()
    c4_dir = project_path / ".c4"

    # Step 1: Auto-initialize if needed
    if not c4_dir.exists():
        console.print(f"[yellow]Initializing C4 in {project_path}...[/yellow]")
        console.print()

        # Temporarily set env for init
        old_env = os.environ.get("C4_PROJECT_ROOT")
        os.environ["C4_PROJECT_ROOT"] = str(project_path)

        try:
            # Call init logic directly (without invoking CLI)
            daemon = C4Daemon()
            daemon.initialize()
            _create_mcp_config(project_path)
            _create_project_settings(project_path)
            install_all_hooks()

            console.print("[green]C4 initialized![/green]")
            console.print()
        finally:
            if old_env is not None:
                os.environ["C4_PROJECT_ROOT"] = old_env
            elif "C4_PROJECT_ROOT" in os.environ:
                del os.environ["C4_PROJECT_ROOT"]
    else:
        console.print(f"[green]C4 already initialized in {project_path}[/green]")
        console.print()

    # Step 2: Determine target platform
    target_platform = platform or get_default_platform(project_path)
    cmd = get_platform_command(target_platform)

    if cmd is None:
        console.print(f"[red]Error:[/red] Unknown platform: {target_platform}")
        console.print(f"Supported platforms: {', '.join(list_platforms())}")
        raise typer.Exit(1)

    # Step 3: Start the platform
    console.print(f"[blue]Starting {target_platform}...[/blue]")

    # Change to project directory and start platform
    os.chdir(project_path)

    try:
        subprocess.run(cmd, check=False)
    except FileNotFoundError:
        console.print(f"[red]Error:[/red] '{cmd[0]}' command not found")
        console.print(f"Make sure {target_platform} CLI is installed")
        raise typer.Exit(1)


# =============================================================================
# c4d Commands (Daemon Management)
# =============================================================================


@app.command()
def start(
    project: Path = typer.Option(
        Path.cwd(),
        "--project",
        "-p",
        help="Project directory",
    ),
    foreground: bool = typer.Option(
        False,
        "--foreground",
        "-f",
        help="Run in foreground (don't daemonize)",
    ),
):
    """Start the C4 daemon (MCP server)"""
    c4_dir = project / ".c4"

    if not c4_dir.exists():
        console.print("[red]Error:[/red] C4 not initialized. Run 'c4 init' first.")
        raise typer.Exit(1)

    pid_file = c4_dir / "daemon.pid"
    log_file = c4_dir / "daemon.log"

    # Check if already running
    if pid_file.exists():
        pid = int(pid_file.read_text().strip())
        try:
            os.kill(pid, 0)  # Check if process exists
            console.print(f"[yellow]Warning:[/yellow] Daemon already running (PID: {pid})")
            raise typer.Exit(1)
        except ProcessLookupError:
            pid_file.unlink()  # Stale pid file

    if foreground:
        console.print("[green]Starting C4 daemon in foreground...[/green]")
        console.print(f"Project: {project}")
        console.print(f"Log: {log_file}")

        # Run MCP server directly
        import asyncio

        from .mcp_server import main

        asyncio.run(main())
    else:
        # Daemonize
        console.print("[green]Starting C4 daemon...[/green]")

        # Start as subprocess
        cmd = [sys.executable, "-m", "c4d.mcp_server"]
        env = os.environ.copy()
        env["C4_PROJECT_ROOT"] = str(project)

        with open(log_file, "a") as log:
            proc = subprocess.Popen(
                cmd,
                stdout=log,
                stderr=log,
                cwd=project,
                env=env,
                start_new_session=True,
            )

        # Write PID file
        pid_file.write_text(str(proc.pid))

        console.print(f"[green]Daemon started[/green] (PID: {proc.pid})")
        console.print(f"Log: {log_file}")


@app.command()
def stop(
    project: Path = typer.Option(
        Path.cwd(),
        "--project",
        "-p",
        help="Project directory",
    ),
):
    """Stop the C4 daemon"""
    c4_dir = project / ".c4"
    pid_file = c4_dir / "daemon.pid"

    if not pid_file.exists():
        console.print("[yellow]Daemon not running[/yellow]")
        raise typer.Exit(0)

    pid = int(pid_file.read_text().strip())

    try:
        os.kill(pid, signal.SIGTERM)
        console.print(f"[green]Daemon stopped[/green] (PID: {pid})")
        pid_file.unlink()
    except ProcessLookupError:
        console.print("[yellow]Daemon was not running (stale PID file)[/yellow]")
        pid_file.unlink()
    except PermissionError:
        console.print(f"[red]Error:[/red] Permission denied to stop daemon (PID: {pid})")
        raise typer.Exit(1)


@app.command()
def status(
    project: Path = typer.Option(
        Path.cwd(),
        "--project",
        "-p",
        help="Project directory",
    ),
):
    """Check daemon status"""
    c4_dir = project / ".c4"
    pid_file = c4_dir / "daemon.pid"

    if not pid_file.exists():
        console.print("[yellow]Daemon not running[/yellow]")
        raise typer.Exit(0)

    pid = int(pid_file.read_text().strip())

    try:
        os.kill(pid, 0)
        console.print(f"[green]Daemon running[/green] (PID: {pid})")
    except ProcessLookupError:
        console.print("[yellow]Daemon not running (stale PID file)[/yellow]")
        pid_file.unlink()


# =============================================================================
# c4 Commands (Project Management)
# =============================================================================


# =============================================================================
# Init Helper Functions
# =============================================================================


def _init_git_repo(project_path: Path) -> dict[str, bool]:
    """Initialize Git repository with .gitignore and initial commit.

    Returns:
        Dict with status of each operation:
        - git_init: True if git init was executed (or repo already existed)
        - gitignore: True if .gitignore was created/updated
        - initial_commit: True if initial commit was created
    """
    result = {"git_init": False, "gitignore": False, "initial_commit": False}
    git_dir = project_path / ".git"

    # Step 1: git init (if not already a repo)
    if not git_dir.exists():
        try:
            subprocess.run(
                ["git", "init"],
                cwd=project_path,
                capture_output=True,
                check=True,
            )
            result["git_init"] = True
        except subprocess.CalledProcessError as e:
            console.print(f"[yellow]Warning:[/yellow] git init failed: {e.stderr}")
            return result
        except FileNotFoundError:
            console.print("[yellow]Warning:[/yellow] git not found, skipping Git setup")
            return result
    else:
        result["git_init"] = True  # Already initialized

    # Step 2: Create .gitignore (if not exists)
    gitignore_path = project_path / ".gitignore"
    c4_ignore_patterns = [
        "# C4 local files",
        ".c4/locks/",
        ".c4/daemon.pid",
        ".c4/daemon.log",
        ".c4/workers/",
        "",
        "# Common patterns",
        "__pycache__/",
        "*.pyc",
        ".venv/",
        "venv/",
        ".env",
        ".env.local",
        "*.log",
        ".DS_Store",
        "node_modules/",
        "dist/",
        "build/",
    ]

    if not gitignore_path.exists():
        gitignore_path.write_text("\n".join(c4_ignore_patterns) + "\n")
        result["gitignore"] = True
    else:
        # Check if C4 patterns are already in .gitignore
        existing = gitignore_path.read_text()
        if ".c4/locks/" not in existing:
            # Append C4 patterns
            with gitignore_path.open("a") as f:
                f.write("\n\n# C4 local files (auto-added)\n")
                f.write(".c4/locks/\n")
                f.write(".c4/daemon.pid\n")
                f.write(".c4/daemon.log\n")
                f.write(".c4/workers/\n")
            result["gitignore"] = True
        else:
            result["gitignore"] = True  # Already has C4 patterns

    # Step 3: Initial commit (if no commits exist)
    try:
        # Check if there are any commits
        check_commits = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=project_path,
            capture_output=True,
        )

        if check_commits.returncode != 0:
            # No commits yet, create initial commit
            # Stage .gitignore at minimum
            subprocess.run(
                ["git", "add", ".gitignore"],
                cwd=project_path,
                capture_output=True,
                check=True,
            )

            # Check if .c4 exists and add config (not db)
            c4_config = project_path / ".c4" / "config.yaml"
            if c4_config.exists():
                subprocess.run(
                    ["git", "add", ".c4/config.yaml"],
                    cwd=project_path,
                    capture_output=True,
                )

            # Create initial commit
            subprocess.run(
                ["git", "commit", "-m", "Initial commit (C4 initialized)"],
                cwd=project_path,
                capture_output=True,
                check=True,
            )
            result["initial_commit"] = True
        else:
            result["initial_commit"] = True  # Already has commits
    except subprocess.CalledProcessError:
        # Commit might fail if nothing to commit, that's OK
        result["initial_commit"] = True

    return result


def _detect_piq_project(project_path: Path) -> Path | None:
    """Detect if this is a PIQ-enabled project and find PIQ installation.

    Returns:
        Path to PIQ installation directory, or None if not a PIQ project.
    """
    # Check 1: pyproject.toml has piq dependency
    pyproject = project_path / "pyproject.toml"
    if pyproject.exists():
        content = pyproject.read_text()
        if "piq" in content.lower():
            # Check if piq is installed in this project's venv
            piq_path = project_path / ".venv" / "lib"
            if piq_path.exists():
                return project_path

    # Check 2: packages/piq exists (monorepo structure)
    piq_pkg = project_path / "packages" / "piq"
    if piq_pkg.exists():
        return piq_pkg

    # Check 3: PIQ_ROOT environment variable
    piq_root = os.environ.get("PIQ_ROOT")
    if piq_root and Path(piq_root).exists():
        return Path(piq_root)

    # Check 4: Common sibling location (../piq)
    sibling_piq = project_path.parent / "piq"
    if (sibling_piq / "packages" / "piq").exists():
        return sibling_piq / "packages" / "piq"

    return None


def _create_mcp_config(
    project_path: Path,
    with_lsp: bool = True,
    with_daemon: bool = True,
) -> bool:
    """Create .mcp.json in project root.

    Includes C4 MCP server, and optionally PIQ MCP server if detected.

    Args:
        project_path: Project directory path
        with_lsp: Enable LSP server for IDE features (hover, completion)
        with_daemon: Enable daemon mode for health monitoring
    """
    mcp_file = project_path / ".mcp.json"
    c4_install_dir = get_c4_install_dir()

    # Build environment variables
    env = {"C4_PROJECT_ROOT": str(project_path)}
    if with_lsp:
        env["C4_LSP_ENABLED"] = "true"
    if with_daemon:
        env["C4_DAEMON_ENABLED"] = "true"

    config = {
        "mcpServers": {
            "c4": {
                "type": "stdio",
                "command": "uv",
                "args": [
                    "--directory",
                    str(c4_install_dir),
                    "run",
                    "python",
                    "-m",
                    "c4.mcp_server",
                ],
                "env": env,
            }
        }
    }

    # Auto-detect and add PIQ MCP server if this is a PIQ project
    piq_path = _detect_piq_project(project_path)
    if piq_path:
        config["mcpServers"]["piq"] = {
            "type": "stdio",
            "command": "uv",
            "args": [
                "--directory",
                str(piq_path),
                "run",
                "python",
                "-m",
                "piq.mcp.server",
            ],
            "env": {
                "PIQ_PROJECT_ROOT": str(project_path),
            },
        }

    mcp_file.write_text(json.dumps(config, indent=2))
    return True


def _create_project_settings(project_path: Path) -> bool:
    """Create .claude/settings.json with permissions.allow.

    This uses project-local settings instead of ~/.claude.json to avoid
    conflicts with Claude Code's runtime config management.

    Pattern-based Bash permissions prevent settings.local.json bloat
    by pre-approving common commands for autonomous execution.
    """
    settings_dir = project_path / ".claude"
    settings_dir.mkdir(parents=True, exist_ok=True)

    settings_file = settings_dir / "settings.json"
    settings = {
        "permissions": {
            "allow": [
                # MCP tools (wildcard for all tools from each server)
                "mcp__c4__*",
                "mcp__piq__*",
                "mcp__serena__*",
                "mcp__plugin_serena_serena__*",
                # Package managers
                "Bash(uv:*)",
                "Bash(pip:*)",
                "Bash(npm:*)",
                "Bash(npx:*)",
                "Bash(pnpm:*)",
                "Bash(yarn:*)",
                "Bash(cargo:*)",
                "Bash(go:*)",
                # Git operations
                "Bash(git:*)",
                # Python/Testing
                "Bash(python:*)",
                "Bash(python3:*)",
                "Bash(pytest:*)",
                # File system (read)
                "Bash(ls:*)",
                "Bash(cat:*)",
                "Bash(head:*)",
                "Bash(tail:*)",
                "Bash(find:*)",
                "Bash(grep:*)",
                "Bash(rg:*)",
                "Bash(tree:*)",
                "Bash(wc:*)",
                "Bash(file:*)",
                "Bash(stat:*)",
                "Bash(readlink:*)",
                "Bash(realpath:*)",
                "Bash(which:*)",
                "Bash(whereis:*)",
                # File system (write)
                "Bash(mkdir:*)",
                "Bash(touch:*)",
                "Bash(cp:*)",
                "Bash(mv:*)",
                "Bash(rm:*)",
                "Bash(chmod:*)",
                "Bash(chown:*)",
                # Text processing
                "Bash(echo:*)",
                "Bash(printf:*)",
                "Bash(sort:*)",
                "Bash(uniq:*)",
                "Bash(cut:*)",
                "Bash(awk:*)",
                "Bash(sed:*)",
                "Bash(tr:*)",
                "Bash(diff:*)",
                "Bash(jq:*)",
                # Utilities
                "Bash(xargs:*)",
                "Bash(timeout:*)",
                "Bash(date:*)",
                "Bash(env:*)",
                "Bash(export:*)",
                "Bash(source:*)",
                "Bash(basename:*)",
                "Bash(dirname:*)",
                # Network
                "Bash(curl:*)",
                "Bash(wget:*)",
                # Process
                "Bash(ps:*)",
                "Bash(lsof:*)",
                "Bash(pkill:*)",
                "Bash(kill:*)",
                # Archive
                "Bash(tar:*)",
                "Bash(zip:*)",
                "Bash(unzip:*)",
                "Bash(gzip:*)",
                # Database
                "Bash(sqlite3:*)",
                "Bash(psql:*)",
                # Docker
                "Bash(docker:*)",
                "Bash(docker-compose:*)",
                # Web search/fetch
                "WebSearch",
                "WebFetch(domain:github.com)",
                "WebFetch(domain:raw.githubusercontent.com)",
                "WebFetch(domain:api.github.com)",
                # File operations for this project
                f"Write({project_path}/**)",
                f"Edit({project_path}/**)",
                f"MultiEdit({project_path}/**)",
                f"Read({project_path}/**)",
            ],
            # Auto-accept file edits for autonomous C4 worker operations
            "defaultMode": "acceptEdits",
        },
        "enableAllProjectMcpServers": True,
    }

    settings_file.write_text(json.dumps(settings, indent=2))
    return True


def _setup_standards_symlinks(project_path: Path) -> None:
    """Link .c4/standards to .claude/rules for backward compatibility."""
    standards_dir = project_path / ".c4" / "standards"
    rules_dir = project_path / ".claude" / "rules"

    if not standards_dir.exists():
        return

    rules_dir.mkdir(parents=True, exist_ok=True)

    # Check if standards_dir contains any .md files
    md_files = list(standards_dir.glob("*.md"))
    if not md_files:
        return

    console.print("[dim]Linking .c4/standards to .claude/rules...[/dim]")

    for standard_file in md_files:
        rule_link = rules_dir / standard_file.name

        # Remove existing link/file to update
        if rule_link.exists() or rule_link.is_symlink():
            try:
                if rule_link.is_symlink() or rule_link.is_file():
                     rule_link.unlink()
            except OSError:
                continue

        try:
            # Try relative symlink
            os.symlink(
                os.path.relpath(standard_file, rules_dir),
                rule_link
            )
        except OSError:
            # Fallback to copy
            shutil.copy2(standard_file, rule_link)


@c4_app.command()
def init(
    project_id: str = typer.Option(
        None,
        "--project-id",
        help="Project ID (defaults to directory name)",
    ),
    project_path: Path = typer.Option(
        None,
        "--path",
        "-p",
        help="Project directory (defaults to current directory)",
    ),
    skip_hooks: bool = typer.Option(
        False,
        "--skip-hooks",
        help="Skip installing Claude Code hooks",
    ),
    skip_commands: bool = typer.Option(
        False,
        "--skip-commands",
        help="Skip installing C4 slash commands to ~/.claude/commands/",
    ),
    template: str = typer.Option(
        None,
        "--template",
        "-t",
        help="Initialize with a template (e.g., image-classification, llm-finetuning)",
    ),
    with_git_hooks: bool = typer.Option(
        True,
        "--with-git-hooks/--no-git-hooks",
        help="Install Git hooks (pre-commit, commit-msg) for validation",
    ),
    with_lsp: bool = typer.Option(
        True,
        "--with-lsp/--no-lsp",
        help="Configure LSP server for IDE integration (hover, completion)",
    ),
    with_daemon: bool = typer.Option(
        True,
        "--with-daemon/--no-daemon",
        help="Enable daemon mode for background health monitoring",
    ),
):
    """Initialize C4 in a project (all-in-one setup)

    This command performs complete C4 initialization:
    - Creates .c4/ directory with state files
    - Creates .mcp.json for MCP server configuration
    - Creates .claude/settings.json for MCP auto-approval
    - Sets up ~/.claude.json allowedTools (with Bash(*))
    - Installs Claude Code hooks (stop hook, security hook)
    - Installs C4 slash commands to ~/.claude/commands/ (global)

    Use --template to initialize with a pre-configured ML project template.
    Use --skip-commands to skip installing global slash commands.
    """
    # Resolve project path
    if project_path is None:
        project_path = Path(os.environ.get("C4_PROJECT_ROOT", Path.cwd()))
    project_path = project_path.resolve()

    # Temporarily set env for C4Daemon
    old_env = os.environ.get("C4_PROJECT_ROOT")
    os.environ["C4_PROJECT_ROOT"] = str(project_path)

    try:
        daemon = C4Daemon()

        # Check if already initialized
        mcp_existed = (project_path / ".mcp.json").exists()

        if daemon.is_initialized():
            console.print("[yellow]Warning:[/yellow] C4 already initialized")
            console.print("[dim]Re-running setup to ensure all configurations are correct...[/dim]")
        else:
            # Step 1: Initialize .c4/ directory
            console.print("[dim]Creating .c4/ directory...[/dim]")
            daemon.initialize(project_id)

        # Step 2: Initialize Git repository
        console.print("[dim]Setting up Git repository...[/dim]")
        git_result = _init_git_repo(project_path)
        if git_result["git_init"]:
            if git_result["initial_commit"]:
                console.print("[dim]  Git initialized with initial commit[/dim]")
            else:
                console.print("[dim]  Git repository ready[/dim]")

        # Step 3: Create .mcp.json
        console.print("[dim]Creating .mcp.json...[/dim]")
        _create_mcp_config(project_path, with_lsp=with_lsp, with_daemon=with_daemon)

        # Step 4: Create .claude/settings.json with permissions
        console.print("[dim]Creating .claude/settings.json (permissions)...[/dim]")
        _create_project_settings(project_path)
        _setup_standards_symlinks(project_path)

        # Step 5: Install hooks (unless skipped)
        if not skip_hooks:
            console.print("[dim]Installing Claude Code hooks...[/dim]")
            hook_results = install_all_hooks()
            if not all(hook_results.values()):
                console.print(
                    "[yellow]Warning:[/yellow] Some Claude Code hooks may not have been installed"
                )

            # Install Git hooks (if enabled)
            if with_git_hooks:
                console.print("[dim]Installing Git hooks...[/dim]")
                git_hook_results = git_hooks.install_all_hooks(project_path)
                for hook_name, (success, msg) in git_hook_results.items():
                    if success:
                        console.print(f"  [green]✓[/green] {hook_name}")
                    else:
                        console.print(f"  [yellow]![/yellow] {hook_name}: {msg}")
            else:
                console.print("[dim]Skipping Git hooks (--no-git-hooks)[/dim]")

        # Step 6: Install C4 slash commands (unless skipped)
        if not skip_commands:
            console.print("[dim]Installing C4 slash commands to ~/.claude/commands/...[/dim]")
            cmd_results = c4_commands.install_all_commands()
            installed_count = sum(1 for success, _ in cmd_results.values() if success)
            updated_count = sum(
                1 for success, msg in cmd_results.values() if success and "Installed:" in msg
            )
            if installed_count > 0:
                console.print(
                    f"  [green]✓[/green] {installed_count} commands "
                    f"({updated_count} updated, {installed_count - updated_count} already up to date)"
                )
            else:
                console.print("[yellow]Warning:[/yellow] No C4 commands were installed")

        # Step 7: Apply template if specified
        template_applied = False
        if template:
            console.print(f"[dim]Applying template '{template}'...[/dim]")
            try:
                from c4.templates import TemplateRegistry

                template_class = TemplateRegistry.get(template)
                if template_class is None:
                    console.print(f"[yellow]Warning:[/yellow] Template '{template}' not found")
                    console.print("[dim]Available templates: c4 template list[/dim]")
                else:
                    # Create template instance and generate project
                    template_instance = template_class()

                    # Generate with defaults
                    result = template_instance.generate_project(
                        output_dir=project_path,
                        project_name=project_id or project_path.name,
                    )
                    if result.success:
                        template_applied = True
                        console.print(f"[green]✓[/green] Template '{template}' applied")
                    else:
                        console.print(
                            f"[yellow]Warning:[/yellow] Template generation failed: "
                            f"{result.message}"
                        )
            except Exception as e:
                console.print(f"[yellow]Warning:[/yellow] Template error: {e}")

        # Success message
        console.print()
        console.print("[green bold]✅ C4 initialized![/green bold]")
        console.print()
        console.print(f"[bold]Project:[/bold] {project_path.name}")
        console.print(f"[bold]Path:[/bold] {project_path}")
        console.print()

        console.print("[bold]Created/Updated:[/bold]")
        console.print("  .c4/                    - C4 state directory")
        if git_result["git_init"]:
            console.print("  .git/                   - Git repository")
        if git_result["gitignore"]:
            console.print("  .gitignore              - Git ignore patterns")
        console.print("  .mcp.json               - MCP server config")
        console.print("  .claude/settings.json   - Permissions & MCP auto-approval")
        if not skip_hooks:
            console.print("  ~/.claude/hooks/        - Stop & security hooks")
        if with_git_hooks:
            console.print("  .git/hooks/             - Git hooks (pre-commit, commit-msg)")
        if template_applied:
            console.print(f"  (template files)        - From '{template}' template")
        console.print()

        console.print("[bold]🔧 Features Enabled:[/bold]")
        if not skip_hooks:
            console.print("  ✓ Bash Security Hook: Blocks dangerous commands")
            console.print("  ✓ Stop Hook: Prevents exit during active work")
        if with_git_hooks:
            console.print("  ✓ Git Hooks: Lint validation, Task ID tracking")
        if with_lsp:
            console.print("  ✓ LSP Server: Hover docs, task completion")
        if with_daemon:
            console.print("  ✓ Daemon Mode: Background health monitoring")
        console.print()

        # Restart notice if .mcp.json was newly created
        if not mcp_existed:
            console.print("[yellow bold]⚠️  Restart Claude Code to load MCP server[/yellow bold]")
            console.print()

        console.print("[bold]Next steps:[/bold]")
        console.print("  /c4-plan    - Create execution plan from docs")
        console.print("  /c4-status  - Check project status")
        console.print("  /c4-run     - Start task execution")

    finally:
        # Restore environment
        if old_env is not None:
            os.environ["C4_PROJECT_ROOT"] = old_env
        elif "C4_PROJECT_ROOT" in os.environ:
            del os.environ["C4_PROJECT_ROOT"]


@c4_app.command("status")
def c4_status():
    """Show current C4 project status"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized. Run 'c4 init' first.")
        raise typer.Exit(1)

    daemon.load()
    _show_status(daemon)


def _show_status(daemon: C4Daemon):
    """Helper to display status"""
    status = daemon.c4_status()

    # Header
    console.print()
    console.print(f"[bold]C4 Project:[/bold] {status['project_id']}")
    console.print()

    # State info
    state_color = {
        "INIT": "dim",
        "PLAN": "blue",
        "EXECUTE": "green",
        "CHECKPOINT": "yellow",
        "COMPLETE": "green bold",
        "HALTED": "red",
        "ERROR": "red bold",
    }.get(status["status"], "white")

    console.print(f"[bold]Status:[/bold] [{state_color}]{status['status']}[/{state_color}]")

    if status.get("execution_mode"):
        console.print(f"[bold]Mode:[/bold] {status['execution_mode']}")

    if status["checkpoint"]["current"]:
        cp_current = status["checkpoint"]["current"]
        cp_state = status["checkpoint"]["state"]
        console.print(f"[bold]Checkpoint:[/bold] {cp_current} ({cp_state})")

    console.print()

    # Queue table
    table = Table(title="Task Queue")
    table.add_column("Status", style="cyan")
    table.add_column("Count", justify="right")
    table.add_column("Details")

    table.add_row(
        "Pending",
        str(status["queue"]["pending"]),
        ", ".join(status["queue"]["pending_ids"]) if status["queue"]["pending_ids"] else "-",
    )
    table.add_row(
        "In Progress",
        str(status["queue"]["in_progress"]),
        ", ".join(f"{k}→{v}" for k, v in status["queue"]["in_progress_map"].items()) or "-",
    )
    table.add_row("Done", str(status["queue"]["done"]), "-")

    console.print(table)
    console.print()

    # Workers
    if status["workers"]:
        worker_table = Table(title="Workers")
        worker_table.add_column("ID")
        worker_table.add_column("State")
        worker_table.add_column("Task")

        for wid, info in status["workers"].items():
            worker_table.add_row(wid, info["state"], info["task_id"] or "-")

        console.print(worker_table)
        console.print()

    # Metrics
    metrics = status['metrics']
    console.print(
        f"[dim]Events: {metrics['events_emitted']} | "
        f"Tasks completed: {metrics['tasks_completed']} | "
        f"Checkpoints passed: {metrics['checkpoints_passed']}[/dim]"
    )

    # Token & Cost summary
    if metrics.get('total_prompt_tokens', 0) > 0:
        cost_color = "green" if metrics['total_cost_usd'] < 1.0 else "yellow"
        if metrics['total_cost_usd'] > 10.0:
            cost_color = "red"

        console.print(
            f"[dim]Tokens: {metrics['total_prompt_tokens']:,} (in) / {metrics['total_completion_tokens']:,} (out) | "
            f"Est. Cost: [{cost_color}]${metrics['total_cost_usd']:.4f}[/{cost_color}][/dim]"
        )


@c4_app.command()
def run():
    """Start execution (PLAN → EXECUTE)"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized. Run 'c4 init' first.")
        raise typer.Exit(1)

    daemon.load()

    # Check current state
    current = daemon.state_machine.state.status
    if current == ProjectStatus.EXECUTE:
        console.print("[yellow]Already in EXECUTE state[/yellow]")
        _show_status(daemon)
        return

    if not daemon.state_machine.can_transition("c4_run"):
        console.print(f"[red]Error:[/red] Cannot run from state {current.value}")
        console.print(f"Allowed commands: {daemon.state_machine.get_allowed_commands()}")
        raise typer.Exit(1)

    # Transition
    try:
        daemon.state_machine.transition("c4_run")
        console.print("[green]Execution started![/green]")
        _show_status(daemon)
    except StateTransitionError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@c4_app.command("stop")
def halt():
    """Stop execution (→ HALTED)"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    if not daemon.state_machine.can_transition("c4_stop"):
        state_val = daemon.state_machine.state.status.value
        console.print(f"[red]Error:[/red] Cannot stop from state {state_val}")
        raise typer.Exit(1)

    try:
        daemon.state_machine.transition("c4_stop")
        console.print("[yellow]Execution halted[/yellow]")
        _show_status(daemon)
    except StateTransitionError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@c4_app.command()
def plan():
    """Enter/re-enter PLAN mode"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized. Run 'c4 init' first.")
        raise typer.Exit(1)

    daemon.load()

    current = daemon.state_machine.state.status
    if current == ProjectStatus.PLAN:
        console.print("[yellow]Already in PLAN mode[/yellow]")
        return

    if not daemon.state_machine.can_transition("c4_plan"):
        console.print(f"[red]Error:[/red] Cannot enter PLAN from state {current.value}")
        raise typer.Exit(1)

    try:
        daemon.state_machine.transition("c4_plan")
        console.print("[blue]Entered PLAN mode[/blue]")
    except StateTransitionError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@c4_app.command()
def rollback(
    checkpoint: str = typer.Argument(
        None,
        help="Checkpoint tag to rollback to (e.g., c4/CP-001)",
    ),
    list_checkpoints: bool = typer.Option(
        False,
        "--list",
        "-l",
        help="List available checkpoints",
    ),
    force: bool = typer.Option(
        False,
        "--force",
        "-f",
        help="Skip confirmation prompt",
    ),
    soft: bool = typer.Option(
        False,
        "--soft",
        help="Use soft reset (keep changes staged)",
    ),
):
    """Rollback to a checkpoint.

    Lists available checkpoints and allows rollback to a specific point.

    Examples:
        c4 rollback --list              # List checkpoints
        c4 rollback c4/CP-001           # Rollback to checkpoint (with confirmation)
        c4 rollback c4/CP-001 --force   # Skip confirmation
        c4 rollback c4/CP-001 --soft    # Keep changes staged
    """
    from .daemon.git_ops import GitOperations

    project_path = Path.cwd()
    git_ops = GitOperations(project_path)

    if not git_ops.is_git_repo():
        console.print("[red]Error:[/red] Not a Git repository")
        raise typer.Exit(1)

    # List checkpoints
    if list_checkpoints or checkpoint is None:
        checkpoints = git_ops.list_checkpoint_tags()

        if not checkpoints:
            console.print("[yellow]No checkpoints found[/yellow]")
            console.print()
            console.print("[dim]Checkpoints are created when supervisors approve work.[/dim]")
            console.print("[dim]Use supervisor approval to create checkpoints.[/dim]")
            return

        console.print()
        console.print("[bold]Available Checkpoints[/bold]")
        console.print()

        table = Table(show_header=True)
        table.add_column("Tag", style="cyan")
        table.add_column("SHA", style="dim")
        table.add_column("Date")
        table.add_column("Description")

        for cp in checkpoints:
            table.add_row(
                cp["tag"],
                cp["sha"],
                cp["date"][:19] if len(cp["date"]) > 19 else cp["date"],
                cp["message"],
            )

        console.print(table)
        console.print()
        console.print("[dim]Usage: c4 rollback <tag> [--force][/dim]")
        return

    # Validate checkpoint exists
    tag_info = git_ops.get_tag_info(checkpoint)
    if not tag_info:
        console.print(f"[red]Error:[/red] Checkpoint '{checkpoint}' not found")
        console.print()
        console.print("Use 'c4 rollback --list' to see available checkpoints")
        raise typer.Exit(1)

    # Show what will be rolled back
    console.print()
    console.print(f"[bold]Rollback to:[/bold] {checkpoint}")
    console.print(f"[bold]Target SHA:[/bold] {tag_info['sha']}")
    console.print(f"[bold]Description:[/bold] {tag_info['message']}")
    console.print()

    # Show commits that will be undone
    commits = git_ops.get_commits_since_tag(checkpoint)
    if commits:
        console.print(f"[yellow]⚠ {len(commits)} commit(s) will be undone:[/yellow]")
        console.print()
        for commit in commits[:10]:  # Show max 10
            console.print(f"  [dim]{commit['sha']}[/dim] {commit['message']}")
        if len(commits) > 10:
            console.print(f"  [dim]... and {len(commits) - 10} more[/dim]")
        console.print()
    else:
        console.print("[green]Already at this checkpoint[/green]")
        return

    # Confirmation
    if not force:
        reset_type = "soft" if soft else "hard"
        console.print(f"[bold red]This will perform a {reset_type} reset![/bold red]")
        if not soft:
            console.print("[red]All uncommitted changes will be lost.[/red]")
        console.print()

        confirm = typer.confirm("Are you sure you want to rollback?")
        if not confirm:
            console.print("[yellow]Rollback cancelled[/yellow]")
            raise typer.Exit(0)

    # Perform rollback
    result = git_ops.rollback_to_checkpoint(checkpoint, hard=not soft)

    if result.success:
        console.print()
        console.print(f"[green bold]✅ {result.message}[/green bold]")
        console.print()
        console.print("[dim]Note: You may need to restart Claude Code to sync state.[/dim]")
    else:
        console.print(f"[red]Error:[/red] {result.message}")
        raise typer.Exit(1)


# =============================================================================
# Authentication Commands
# =============================================================================

auth_app = typer.Typer(help="Authentication commands")
c4_app.add_typer(auth_app, name="auth")


@c4_app.command()
def login(
    provider: str = typer.Option(
        "github",
        "--provider",
        "-p",
        help="OAuth provider (github, google)",
    ),
    no_browser: bool = typer.Option(
        False,
        "--no-browser",
        help="Don't open browser automatically",
    ),
):
    """Login to C4 Cloud.

    Opens browser for OAuth authentication with the selected provider.

    Examples:
        c4 login                    # Login with GitHub (default)
        c4 login --provider google  # Login with Google
        c4 login --no-browser       # Print URL instead of opening
    """
    import os

    from .auth import SessionManager
    from .auth.oauth import OAuthConfig, OAuthFlow
    from .auth.session import Session

    # Check for Supabase URL
    supabase_url = os.environ.get("SUPABASE_URL")
    if not supabase_url:
        # Try loading from config
        config_path = Path.home() / ".c4" / "cloud.yaml"
        if config_path.exists():
            import yaml

            try:
                config = yaml.safe_load(config_path.read_text())
                supabase_url = config.get("supabase_url")
            except Exception:
                pass

    if not supabase_url:
        console.print("[red]Error:[/red] Supabase URL not configured")
        console.print()
        console.print("Set SUPABASE_URL environment variable or create ~/.c4/cloud.yaml:")
        console.print('  supabase_url: "https://your-project.supabase.co"')
        raise typer.Exit(1)

    # Check if already logged in
    session_manager = SessionManager()
    if session_manager.is_logged_in():
        session = session_manager.load()
        console.print(f"[yellow]Already logged in as:[/yellow] {session.email}")
        if not typer.confirm("Login again?"):
            raise typer.Exit(0)

    # Configure OAuth
    oauth_config = OAuthConfig(
        supabase_url=supabase_url,
        scopes=["repo"] if provider == "github" else None,
    )
    oauth_flow = OAuthFlow(oauth_config)

    auth_url = oauth_flow.get_authorization_url(provider)

    if no_browser:
        console.print()
        console.print("[bold]Open this URL in your browser:[/bold]")
        console.print()
        console.print(f"  {auth_url}")
        console.print()
    else:
        console.print(f"[blue]Opening browser for {provider} login...[/blue]")

    def on_waiting() -> None:
        console.print()
        console.print("[dim]Waiting for authentication...[/dim]")
        console.print("[dim]Press Ctrl+C to cancel[/dim]")

    try:
        result = oauth_flow.run(
            provider=provider,
            open_browser=not no_browser,
            on_waiting=on_waiting,
        )
    except KeyboardInterrupt:
        console.print()
        console.print("[yellow]Login cancelled[/yellow]")
        raise typer.Exit(0)

    if not result.success:
        console.print(f"[red]Login failed:[/red] {result.error}")
        raise typer.Exit(1)

    # Create and save session
    if result.access_token:
        session = Session(
            access_token=result.access_token,
            refresh_token=result.refresh_token or "",
            provider=provider,
        )
        session_manager.save(session)

        console.print()
        console.print("[green bold]Login successful![/green bold]")
        console.print()
        console.print(f"[dim]Provider: {provider}[/dim]")
        console.print("[dim]Session saved to ~/.c4/session.json[/dim]")
    else:
        console.print("[red]Error:[/red] No access token received")
        raise typer.Exit(1)


@c4_app.command()
def logout():
    """Logout from C4 Cloud.

    Clears the stored session and tokens.
    """
    from .auth import SessionManager

    session_manager = SessionManager()

    if not session_manager.is_logged_in():
        console.print("[yellow]Not logged in[/yellow]")
        return

    session = session_manager.load()
    email = session.email if session else "unknown"

    if session_manager.clear():
        console.print(f"[green]Logged out:[/green] {email}")
    else:
        console.print("[red]Error:[/red] Failed to clear session")
        raise typer.Exit(1)


@auth_app.command("status")
def auth_status():
    """Check authentication status."""
    from .auth import SessionManager

    session_manager = SessionManager()
    session = session_manager.load()

    console.print()
    if session is None:
        console.print("[yellow]Not logged in[/yellow]")
        console.print()
        console.print("[dim]Run 'c4 login' to authenticate[/dim]")
        return

    if session.is_expired:
        console.print("[red]Session expired[/red]")
        console.print()
        console.print("[dim]Run 'c4 login' to re-authenticate[/dim]")
        return

    console.print("[green]Logged in[/green]")
    console.print()
    console.print(f"[bold]Email:[/bold] {session.email or 'N/A'}")
    console.print(f"[bold]Provider:[/bold] {session.provider or 'N/A'}")
    console.print(f"[bold]User ID:[/bold] {session.user_id or 'N/A'}")

    if session.expires_at:
        expires_in = session.expires_in_seconds
        if expires_in > 3600:
            expires_str = f"{expires_in // 3600}h {(expires_in % 3600) // 60}m"
        elif expires_in > 60:
            expires_str = f"{expires_in // 60}m"
        else:
            expires_str = f"{expires_in}s"
        console.print(f"[bold]Expires in:[/bold] {expires_str}")

    console.print()
    console.print(f"[dim]Session file: {session_manager.session_file}[/dim]")


@auth_app.command("refresh")
def auth_refresh():
    """Refresh authentication token."""
    console.print("[yellow]Token refresh not yet implemented[/yellow]")
    console.print("[dim]Use 'c4 login' to re-authenticate[/dim]")


# =============================================================================
# Worker Subcommands
# =============================================================================

worker_app = typer.Typer(help="Worker commands")
c4_app.add_typer(worker_app, name="worker")


@worker_app.command("join")
def worker_join(
    worker_id: str = typer.Option(
        None,
        "--id",
        help="Worker ID (auto-generated if not provided)",
    ),
):
    """Join as a worker"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    if daemon.state_machine.state.status != ProjectStatus.EXECUTE:
        state_val = daemon.state_machine.state.status.value
        console.print(f"[red]Error:[/red] Cannot join workers in state {state_val}")
        console.print("Run 'c4 run' first to start execution")
        raise typer.Exit(1)

    # Generate worker ID if not provided
    if worker_id is None:
        import uuid

        worker_id = f"worker-{uuid.uuid4().hex[:8]}"

    _worker = daemon.worker_manager.register(worker_id)
    console.print(f"[green]Joined as worker:[/green] {worker_id}")
    console.print()
    console.print("Ready to receive tasks. Use MCP tools:")
    console.print("  c4_get_task() - Get next task")
    console.print("  c4_submit()   - Submit completed task")


@worker_app.command("submit")
def worker_submit(
    task_id: str = typer.Argument(..., help="Task ID"),
    commit: str = typer.Option(..., "--commit", "-c", help="Git commit SHA"),
):
    """Submit a completed task (manual mode)"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    # Manual submit without validation results
    result = daemon.c4_submit(
        task_id,
        commit,
        [{"name": "manual", "status": "pass"}],
    )

    if result.success:
        console.print(f"[green]Task submitted:[/green] {task_id}")
        console.print(f"Next action: {result.next_action}")
    else:
        console.print(f"[red]Error:[/red] {result.message}")
        raise typer.Exit(1)


# =============================================================================
# Task Management
# =============================================================================


@c4_app.command("add-task")
def add_task(
    task_id: str = typer.Argument(..., help="Task ID"),
    title: str = typer.Option(..., "--title", "-t", help="Task title"),
    dod: str = typer.Option(..., "--dod", "-d", help="Definition of Done"),
    scope: str = typer.Option(None, "--scope", "-s", help="Scope (for locking)"),
):
    """Add a task to the queue"""
    daemon = C4Daemon()  # Uses C4_PROJECT_ROOT env var or cwd

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    task = Task(
        id=task_id,
        title=title,
        dod=dod,
        scope=scope,
    )
    daemon.add_task(task)

    console.print(f"[green]Task added:[/green] {task_id}")
    console.print(f"Title: {title}")
    console.print(f"DoD: {dod}")
    if scope:
        console.print(f"Scope: {scope}")


# =============================================================================
# Config Commands
# =============================================================================

config_app = typer.Typer(help="Configuration management")
c4_app.add_typer(config_app, name="config")


@config_app.callback(invoke_without_command=True)
def config_main(
    ctx: typer.Context,
    show: bool = typer.Option(
        False,
        "--show",
        "-s",
        help="Show current configuration",
    ),
):
    """Manage C4 configuration.

    Without arguments: show current configuration.

    Examples:
        c4 config --show              # Show all config
        c4 config platform cursor     # Set project default platform
        c4 config --global platform cursor  # Set global default
    """
    if ctx.invoked_subcommand is not None:
        return

    # Default action: show config
    info = get_config_info()

    console.print()
    console.print("[bold]C4 Configuration[/bold]")
    console.print()

    table = Table(show_header=True)
    table.add_column("Source", style="cyan")
    table.add_column("Platform")
    table.add_column("Active", style="green")

    # Global
    global_platform = info["global_platform"] or "[dim]-[/dim]"
    global_active = "*" if info["source"] == "global" else ""
    table.add_row("Global (~/.c4/config.yaml)", global_platform, global_active)

    # Project
    project_platform = info["project_platform"] or "[dim]-[/dim]"
    project_active = "*" if info["source"] == "project" else ""
    table.add_row("Project (.c4/config.yaml)", project_platform, project_active)

    # Environment
    env_platform = info["env_platform"] or "[dim]-[/dim]"
    env_active = "*" if info["source"] == "environment" else ""
    table.add_row("Environment (C4_PLATFORM)", env_platform, env_active)

    # Default
    default_active = "*" if info["source"] == "default" else ""
    table.add_row("Default", "claude", default_active)

    console.print(table)
    console.print()
    console.print(f"[bold]Effective platform:[/bold] {info['effective_platform']}")
    console.print()
    console.print(f"[dim]Supported platforms: {', '.join(list_platforms())}[/dim]")


@config_app.command("platform")
def config_platform(
    platform_name: str = typer.Argument(..., help="Platform name to set as default"),
    is_global: bool = typer.Option(
        False,
        "--global",
        "-g",
        help="Set in global config (~/.c4/config.yaml)",
    ),
):
    """Set the default platform.

    Examples:
        c4 config platform cursor           # Project default
        c4 config platform cursor --global  # Global default
    """
    try:
        config_path = set_platform_config(
            platform=platform_name,
            global_config=is_global,
        )
        scope = "global" if is_global else "project"
        console.print(f"[green]Set {scope} platform:[/green] {platform_name}")
        console.print(f"[dim]Config: {config_path}[/dim]")
    except ValueError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


# =============================================================================
# API Key Management
# =============================================================================

api_key_app = typer.Typer(help="API key management")
config_app.add_typer(api_key_app, name="api-key")


@api_key_app.command("set")
def api_key_set(
    provider: str = typer.Argument(
        ...,
        help=f"Provider name ({', '.join(SUPPORTED_PROVIDERS)})",
    ),
    api_key: str = typer.Option(
        ...,
        "--key",
        "-k",
        prompt="API Key",
        hide_input=True,
        help="API key value (prompted if not provided)",
    ),
    is_global: bool = typer.Option(
        True,
        "--global/--project",
        "-g/-p",
        help="Store in global (~/.c4) or project (.c4) config",
    ),
):
    """Set an API key for a provider.

    API keys are stored securely with restricted file permissions (600).
    Priority: environment variable > project config > global config.

    Examples:
        c4 config api-key set anthropic --key sk-ant-xxx
        c4 config api-key set anthropic  # Prompts for key
        c4 config api-key set openai --project --key sk-xxx
    """
    creds = CredentialsManager()

    try:
        creds_path = creds.set_api_key(provider, api_key, is_global=is_global)
        scope = "global" if is_global else "project"
        masked = creds.mask_api_key(api_key)
        console.print(f"[green]Set {provider} API key ({scope}):[/green] {masked}")
        console.print(f"[dim]Stored in: {creds_path}[/dim]")
    except ValueError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@api_key_app.command("list")
def api_key_list():
    """List configured API keys.

    Shows all configured providers and their source (env, project, global).
    API keys are masked for security.

    Examples:
        c4 config api-key list
    """
    creds = CredentialsManager()
    providers = creds.list_configured_providers()

    console.print()
    console.print("[bold]Configured API Keys[/bold]")
    console.print()

    if not providers:
        console.print("[yellow]No API keys configured[/yellow]")
        console.print()
        console.print("[dim]Set an API key with:[/dim]")
        console.print("  c4 config api-key set anthropic --key YOUR_KEY")
        console.print()
        console.print(f"[dim]Supported providers: {', '.join(SUPPORTED_PROVIDERS)}[/dim]")
        return

    table = Table(show_header=True)
    table.add_column("Provider", style="cyan")
    table.add_column("Source")
    table.add_column("Key (masked)")
    table.add_column("Active", style="green")

    for provider, source in sorted(providers.items()):
        masked_key = creds.get_masked_api_key(provider) or "[dim]-[/dim]"
        source_color = {"env": "yellow", "project": "blue", "global": "dim"}
        source_display = f"[{source_color.get(source, 'white')}]{source}[/{source_color.get(source, 'white')}]"

        # Check if this provider is actually active (highest priority)
        is_active = creds.get_api_key(provider) is not None
        active_mark = "*" if is_active else ""

        table.add_row(provider, source_display, masked_key, active_mark)

    console.print(table)
    console.print()
    console.print("[dim]Priority: env > project > global[/dim]")
    console.print(f"[dim]Supported: {', '.join(SUPPORTED_PROVIDERS)}[/dim]")


@api_key_app.command("delete")
def api_key_delete(
    provider: str = typer.Argument(
        ...,
        help=f"Provider name ({', '.join(SUPPORTED_PROVIDERS)})",
    ),
    is_global: bool = typer.Option(
        True,
        "--global/--project",
        "-g/-p",
        help="Delete from global (~/.c4) or project (.c4) config",
    ),
    force: bool = typer.Option(
        False,
        "--force",
        "-f",
        help="Skip confirmation prompt",
    ),
):
    """Delete an API key.

    Note: This only deletes from config files, not environment variables.

    Examples:
        c4 config api-key delete anthropic
        c4 config api-key delete openai --project
        c4 config api-key delete anthropic --force
    """
    creds = CredentialsManager()
    scope = "global" if is_global else "project"

    # Check if key exists
    creds_path = creds.global_path if is_global else creds.project_path
    if not creds_path.exists():
        console.print(f"[yellow]No {scope} credentials file found[/yellow]")
        return

    if not force:
        confirm = typer.confirm(f"Delete {provider} API key from {scope} config?")
        if not confirm:
            console.print("[yellow]Cancelled[/yellow]")
            raise typer.Exit(0)

    if creds.delete_api_key(provider, is_global=is_global):
        console.print(f"[green]Deleted {provider} API key from {scope} config[/green]")
    else:
        console.print(f"[yellow]No {provider} API key found in {scope} config[/yellow]")


@api_key_app.command("get")
def api_key_get(
    provider: str = typer.Argument(
        "anthropic",
        help=f"Provider name ({', '.join(SUPPORTED_PROVIDERS)})",
    ),
    show_full: bool = typer.Option(
        False,
        "--full",
        help="Show full API key (use with caution)",
    ),
):
    """Get an API key (masked by default).

    Shows the effective API key considering priority:
    env > project > global

    Examples:
        c4 config api-key get anthropic
        c4 config api-key get openai --full
    """
    creds = CredentialsManager()
    providers = creds.list_configured_providers()

    if provider not in providers:
        console.print(f"[yellow]No {provider} API key configured[/yellow]")
        console.print()
        console.print(f"[dim]Set with: c4 config api-key set {provider}[/dim]")
        raise typer.Exit(1)

    source = providers[provider]
    if show_full:
        api_key = creds.get_api_key(provider)
        console.print(f"[cyan]{provider}[/cyan] ({source}): {api_key}")
    else:
        masked = creds.get_masked_api_key(provider)
        console.print(f"[cyan]{provider}[/cyan] ({source}): {masked}")
        console.print("[dim]Use --full to show complete key[/dim]")


# =============================================================================
# Environment Variable Export Command
# =============================================================================


@c4_app.command("env")
def env_export(
    provider: str = typer.Argument(
        None,
        help="Specific provider to export (defaults to all)",
    ),
    output_format: str = typer.Option(
        "export",
        "--format",
        "-f",
        help="Output format: export, json, dotenv, fish",
    ),
    quiet: bool = typer.Option(
        False,
        "--quiet",
        "-q",
        help="Output only the export statements (for eval)",
    ),
):
    """Export API keys as environment variables.

    Exports stored API keys as shell environment variable statements.
    Use with eval to set them in your current shell:

        eval $(c4 env)

    Or add to your shell profile for permanent setup:

        echo 'eval $(c4 env 2>/dev/null)' >> ~/.bashrc

    Examples:
        c4 env                          # Export all keys (bash/zsh)
        c4 env anthropic                # Export specific provider
        c4 env --format=dotenv > .env   # Create .env file
        c4 env --format=fish            # Fish shell format
        c4 env --format=json            # JSON format
        eval $(c4 env)                  # Apply to current shell
    """
    # Create stderr console for messages that shouldn't interfere with eval
    err_console = Console(stderr=True, force_terminal=False)

    creds = CredentialsManager()
    providers = creds.list_configured_providers()

    # Filter to specific provider if requested
    if provider:
        provider = provider.lower()
        if provider not in providers:
            if not quiet:
                err_console.print(f"[yellow]No {provider} API key configured[/yellow]")
            raise typer.Exit(1)
        providers = {provider: providers[provider]}

    if not providers:
        if not quiet:
            err_console.print("[yellow]No API keys configured[/yellow]")
            err_console.print("[dim]Set an API key with: c4 config api-key set anthropic[/dim]")
        raise typer.Exit(0)

    # Build key-value pairs (only from config files, not env)
    exports: dict[str, str] = {}
    for prov, source in providers.items():
        # Only export keys stored in config files (not already in env)
        if source in ("project", "global"):
            api_key = creds.get_api_key(prov)
            if api_key:
                env_var = ENV_VAR_MAPPING.get(prov)
                if env_var:
                    exports[env_var] = api_key

    if not exports:
        if not quiet:
            err_console.print("[yellow]No API keys to export (all from env or none configured)[/yellow]")
        raise typer.Exit(0)

    # Output based on format
    if output_format == "export":
        # Bash/Zsh format - use shlex.quote() to prevent shell injection
        for env_var, api_key in exports.items():
            print(f"export {env_var}={shlex.quote(api_key)}")

    elif output_format == "fish":
        # Fish shell format - use single quotes with escaping
        for env_var, api_key in exports.items():
            # In fish, single quotes prevent variable expansion
            # Escape backslashes and single quotes
            escaped = api_key.replace("\\", "\\\\").replace("'", "\\'")
            print(f"set -gx {env_var} '{escaped}'")

    elif output_format == "dotenv":
        # .env file format - escape backslashes and double quotes
        for env_var, api_key in exports.items():
            escaped = api_key.replace("\\", "\\\\").replace('"', '\\"')
            print(f'{env_var}="{escaped}"')

    elif output_format == "json":
        # JSON format
        print(json.dumps(exports, indent=2))

    else:
        err_console.print(f"[red]Unknown format: {output_format}[/red]")
        err_console.print("[dim]Supported: export, fish, dotenv, json[/dim]")
        raise typer.Exit(1)

    # Show info message to stderr (won't affect eval)
    if not quiet:
        err_console.print(f"[dim]# Exported {len(exports)} API key(s)[/dim]")


# =============================================================================
# UI Server Command
# =============================================================================


@c4_app.command("ui")
def ui_server(
    port: int = typer.Option(
        4000,
        "--port",
        "-p",
        help="Server port",
    ),
    host: str = typer.Option(
        "localhost",
        "--host",
        "-h",
        help="Server host",
    ),
    no_browser: bool = typer.Option(
        False,
        "--no-browser",
        help="Don't open browser automatically",
    ),
):
    """Start local UI server.

    Starts a web-based dashboard for C4 at http://localhost:4000.

    Examples:
        c4 ui                   # Start on default port 4000
        c4 ui --port 8080       # Use custom port
        c4 ui --no-browser      # Don't open browser
    """
    from .ui import run_ui_server

    console.print(f"[blue]Starting C4 UI server on http://{host}:{port}[/blue]")

    if not no_browser:
        console.print("[dim]Opening browser...[/dim]")

    try:
        run_ui_server(
            port=port,
            host=host,
            open_browser=not no_browser,
        )
    except KeyboardInterrupt:
        console.print()
        console.print("[yellow]UI server stopped[/yellow]")


# =============================================================================
# Platform Setup Commands
# =============================================================================


@c4_app.command("platforms")
def platforms_cmd(
    setup: str = typer.Option(
        None,
        "--setup",
        help="Set up a specific platform (creates templates)",
    ),
    validate: str = typer.Option(
        None,
        "--validate",
        help="Validate commands for a platform",
    ),
    list_all: bool = typer.Option(
        False,
        "--list",
        "-l",
        help="List supported platforms",
    ),
):
    """Manage platform configurations.

    Examples:
        c4 platforms --list                # List supported platforms
        c4 platforms --validate cursor     # Check cursor commands
        c4 platforms --setup cursor        # Set up cursor with templates
    """
    if list_all:
        console.print()
        console.print("[bold]Supported Platforms[/bold]")
        console.print()
        for name, cmd in PLATFORM_COMMANDS.items():
            console.print(f"  [cyan]{name}[/cyan]: {' '.join(cmd)}")
        console.print()
        return

    project_path = Path.cwd()

    if validate:
        from .platforms import validate_platform_commands

        result = validate_platform_commands(project_path, validate)
        console.print()
        console.print(f"[bold]Platform: {validate}[/bold]")
        console.print()

        if result["found"]:
            console.print("[green]Found commands:[/green]")
            for cmd in result["found"]:
                console.print(f"  [green]+[/green] {cmd}")

        if result["missing"]:
            console.print()
            console.print("[yellow]Missing commands:[/yellow]")
            for cmd in result["missing"]:
                console.print(f"  [yellow]-[/yellow] {cmd}")
            console.print()
            console.print(f"[dim]Run 'c4 platforms --setup {validate}' to generate templates[/dim]")
        else:
            console.print()
            console.print("[green]All required commands found![/green]")
        return

    if setup:
        result = setup_platform(project_path, setup, generate_templates=True)
        console.print()
        console.print(f"[bold]Setting up platform: {setup}[/bold]")
        console.print(f"Command directory: {result['command_dir']}")
        console.print()

        if result["generated"]:
            console.print("[green]Generated templates:[/green]")
            for cmd in result["generated"]:
                console.print(f"  [green]+[/green] {cmd}.md")

        if result["skipped"]:
            console.print()
            console.print("[yellow]Skipped (no reference):[/yellow]")
            for cmd in result["skipped"]:
                console.print(f"  [yellow]![/yellow] {cmd}")

        if result["validation"]["found"]:
            console.print()
            console.print("[dim]Already existing:[/dim]")
            for cmd in result["validation"]["found"]:
                console.print(f"  [dim]=[/dim] {cmd}")

        console.print()
        console.print("[yellow]Note:[/yellow] Review and customize generated templates")
        console.print("[dim]Reference: .claude/commands/ (Claude Code version)[/dim]")
        return

    # Default: show help
    console.print("Use --list, --validate, or --setup option")
    console.print("Run 'c4 platforms --help' for details")


# =============================================================================
# Registry Commands
# =============================================================================

registry_app = typer.Typer(help="Registry management commands")
c4_app.add_typer(registry_app, name="registry")


@registry_app.command("build")
def registry_build(
    project_path: Path = typer.Option(
        None,
        "--path",
        "-p",
        help="Project directory (defaults to current directory)",
    ),
    target: str = typer.Option(
        "claude",
        "--target",
        "-t",
        help="Target platform (claude, all)",
    ),
):
    """Build registry artifacts for specific platforms.

    Converts YAML definitions into platform-specific formats (e.g., Markdown for Claude).
    """
    from c4.system.registry.builder import RegistryBuilder

    root = (project_path or Path.cwd()).resolve()
    console.print(f"[bold]Building registry for {root.name}...[/bold]")

    builder = RegistryBuilder(root)
    generated = []

    if target in ("claude", "all"):
        console.print("[dim]Building for Claude Code...[/dim]")
        files = builder.build_for_claude()
        generated.extend(files)

    if generated:
        console.print(f"[green]Successfully generated {len(generated)} files:[/green]")
        for f in generated[:5]:
            console.print(f"  - {f}")
        if len(generated) > 5:
            console.print(f"  ... and {len(generated) - 5} more")
    else:
        console.print("[yellow]No files generated (check logs or definitions).[/yellow]")


# =============================================================================
# Skill Management Commands
# =============================================================================

skill_app = typer.Typer(help="Skill management commands")
c4_app.add_typer(skill_app, name="skill")


@skill_app.command("list")
def skill_list(
    directory: Path = typer.Option(
        None,
        "--dir",
        "-d",
        help="Skill directory (defaults to built-in skills)",
    ),
    domain: str = typer.Option(
        None,
        "--domain",
        help="Filter by domain (e.g., ml-dl, web-frontend)",
    ),
    show_all: bool = typer.Option(
        False,
        "--all",
        "-a",
        help="Show all skills including deprecated",
    ),
):
    """List available skills.

    Examples:
        c4 skill list                      # List all built-in skills
        c4 skill list --domain ml-dl       # Filter by domain
        c4 skill list --dir .c4/skills     # List project skills
    """
    from c4.supervisor.agent_graph import EXAMPLES_DIR
    from c4.supervisor.agent_graph.loader import AgentGraphLoader

    skill_dir = directory or (EXAMPLES_DIR / "skills")

    if not skill_dir.exists():
        console.print(f"[red]Error:[/red] Directory not found: {skill_dir}")
        raise typer.Exit(1)

    loader = AgentGraphLoader(skill_dir.parent)

    try:
        skills = loader.load_skills()
    except Exception as e:
        console.print(f"[red]Error loading skills:[/red] {e}")
        raise typer.Exit(1)

    if not skills:
        console.print("[yellow]No skills found[/yellow]")
        return

    # Filter by domain
    if domain:
        skills = [s for s in skills if domain in s.skill.domains or "universal" in s.skill.domains]

    # Filter deprecated
    if not show_all:
        skills = [s for s in skills if not (s.skill.metadata and s.skill.metadata.deprecated)]

    console.print()
    console.print(f"[bold]Skills ({len(skills)})[/bold]")
    console.print()

    table = Table(show_header=True)
    table.add_column("ID", style="cyan")
    table.add_column("Name")
    table.add_column("Impact", style="yellow")
    table.add_column("Domains")
    table.add_column("Version", style="dim")

    for skill_def in sorted(skills, key=lambda s: s.skill.id):
        skill = skill_def.skill
        domains_str = ", ".join(skill.domains[:3])
        if len(skill.domains) > 3:
            domains_str += f" (+{len(skill.domains) - 3})"

        version = skill.metadata.version if skill.metadata else "1.0.0"
        deprecated = " (deprecated)" if skill.metadata and skill.metadata.deprecated else ""

        table.add_row(
            skill.id + deprecated,
            skill.name,
            skill.impact.value,
            domains_str,
            version,
        )

    console.print(table)
    console.print()
    console.print(f"[dim]Source: {skill_dir}[/dim]")


@skill_app.command("validate")
def skill_validate(
    path: Path = typer.Argument(
        ...,
        help="Path to skill file or directory to validate",
    ),
    check_deps: bool = typer.Option(
        False,
        "--check-deps",
        help="Check skill dependencies (Level 3)",
    ),
    verbose: bool = typer.Option(
        False,
        "--verbose",
        "-v",
        help="Show all issues including info level",
    ),
):
    """Validate skill definitions.

    Runs multi-level validation:
    - Level 1 (Required): Schema + Pydantic validation, triggers
    - Level 2 (Recommended): Rules, description quality, examples
    - Level 3 (Optional): Dependencies (--check-deps)

    Examples:
        c4 skill validate skills/debugging.yaml
        c4 skill validate skills/ --check-deps
        c4 skill validate .c4/skills/ --verbose
    """
    from c4.supervisor.agent_graph.skill_validator import (
        SkillValidator,
        ValidationLevel,
    )

    validator = SkillValidator()

    if path.is_file():
        result = validator.validate_file(path, check_deps=check_deps)

        status = "[green]PASS[/green]" if result.is_valid else "[red]FAIL[/red]"
        console.print()
        console.print(f"{status} {result.skill_id or path.name}")
        console.print()

        for issue in result.issues:
            if not verbose and issue.level == ValidationLevel.INFO:
                continue
            color = {"error": "red", "warning": "yellow", "info": "dim"}[issue.level.value]
            console.print(f"  [{color}]{issue}[/{color}]")
            if issue.suggestion:
                console.print(f"    [dim]Suggestion: {issue.suggestion}[/dim]")

        if result.is_valid:
            console.print()
            console.print("[green]All required checks passed[/green]")
        raise typer.Exit(0 if result.is_valid else 1)

    elif path.is_dir():
        results = validator.validate_directory(path, check_deps=check_deps)

        if not results:
            console.print("[yellow]No skill files found[/yellow]")
            return

        total_errors = sum(r.error_count for r in results)
        total_warnings = sum(r.warning_count for r in results)
        valid_count = sum(1 for r in results if r.is_valid)

        console.print()
        console.print(f"[bold]Validation Results ({len(results)} skills)[/bold]")
        console.print()

        for result in results:
            status = "[green]+[/green]" if result.is_valid else "[red]x[/red]"
            console.print(f"  {status} {result.skill_id or result.skill_path}")

            for issue in result.issues:
                if not verbose and issue.level == ValidationLevel.INFO:
                    continue
                if issue.level == ValidationLevel.ERROR:
                    console.print(f"      [red]{issue}[/red]")
                elif issue.level == ValidationLevel.WARNING:
                    console.print(f"      [yellow]{issue}[/yellow]")
                elif verbose:
                    console.print(f"      [dim]{issue}[/dim]")

        console.print()
        console.print(
            f"Summary: {valid_count}/{len(results)} passed, {total_errors} errors, {total_warnings} warnings"
        )

        # Check for circular dependencies
        cycles = validator.check_circular_dependencies(path)
        if cycles:
            console.print()
            console.print("[red]Circular dependencies detected:[/red]")
            for skill_id, cycle in cycles:
                console.print(f"  {' -> '.join(cycle)}")

        raise typer.Exit(0 if total_errors == 0 else 1)

    else:
        console.print(f"[red]Error:[/red] Path not found: {path}")
        raise typer.Exit(1)


@skill_app.command("info")
def skill_info(
    skill_id: str = typer.Argument(..., help="Skill ID to show info for"),
    directory: Path = typer.Option(
        None,
        "--dir",
        "-d",
        help="Skill directory (defaults to built-in skills)",
    ),
):
    """Show detailed information about a skill.

    Examples:
        c4 skill info debugging
        c4 skill info experiment-tracking --dir .c4/skills
    """
    from c4.supervisor.agent_graph import EXAMPLES_DIR
    from c4.supervisor.agent_graph.loader import AgentGraphLoader

    skill_dir = directory or (EXAMPLES_DIR / "skills")

    if not skill_dir.exists():
        console.print(f"[red]Error:[/red] Directory not found: {skill_dir}")
        raise typer.Exit(1)

    loader = AgentGraphLoader(skill_dir.parent)
    skill_def = loader.load_skill_by_id(skill_id)

    if not skill_def:
        console.print(f"[red]Error:[/red] Skill not found: {skill_id}")
        console.print()
        console.print("[dim]Use 'c4 skill list' to see available skills[/dim]")
        raise typer.Exit(1)

    skill = skill_def.skill

    console.print()
    console.print(f"[bold cyan]{skill.id}[/bold cyan] - {skill.name}")
    console.print()
    console.print("[bold]Description:[/bold]")
    console.print(f"  {skill.description}")
    console.print()

    # Impact & Category
    console.print(f"[bold]Impact:[/bold] {skill.impact.value}")
    if skill.category:
        console.print(f"[bold]Category:[/bold] {skill.category.value}")
    console.print(f"[bold]Domains:[/bold] {', '.join(skill.domains)}")
    console.print()

    # Capabilities
    console.print("[bold]Capabilities:[/bold]")
    for cap in skill.capabilities:
        console.print(f"  - {cap}")
    console.print()

    # Triggers
    console.print("[bold]Triggers:[/bold]")
    if skill.triggers.keywords:
        console.print(f"  Keywords: {', '.join(skill.triggers.keywords)}")
    if skill.triggers.task_types:
        console.print(f"  Task Types: {', '.join(skill.triggers.task_types)}")
    if skill.triggers.file_patterns:
        console.print(f"  File Patterns: {', '.join(skill.triggers.file_patterns)}")
    console.print()

    # Rules
    if skill.rules:
        console.print(f"[bold]Rules ({len(skill.rules)}):[/bold]")
        for rule in skill.rules:
            impact_color = {"critical": "red", "high": "yellow", "medium": "white", "low": "dim"}
            console.print(
                f"  [{impact_color[rule.impact.value]}]{rule.id}[/{impact_color[rule.impact.value]}]: {rule.description[:60]}..."
            )
        console.print()

    # Dependencies
    if skill.dependencies:
        console.print("[bold]Dependencies:[/bold]")
        if skill.dependencies.required:
            console.print(f"  Required: {', '.join(skill.dependencies.required)}")
        if skill.dependencies.optional:
            console.print(f"  Optional: {', '.join(skill.dependencies.optional)}")
        console.print()

    # Metadata
    if skill.metadata:
        console.print("[bold]Metadata:[/bold]")
        console.print(f"  Version: {skill.metadata.version}")
        if skill.metadata.author:
            console.print(f"  Author: {skill.metadata.author}")
        if skill.metadata.tags:
            console.print(f"  Tags: {', '.join(skill.metadata.tags)}")
        if skill.metadata.deprecated:
            console.print("  [red]DEPRECATED[/red]")
            if skill.metadata.deprecated_by:
                console.print(f"  Replaced by: {skill.metadata.deprecated_by}")

    # Related skills
    if skill.complementary_skills or skill.leads_to or skill.prerequisites:
        console.print()
        console.print("[bold]Related Skills:[/bold]")
        if skill.prerequisites:
            console.print(f"  Prerequisites: {', '.join(skill.prerequisites)}")
        if skill.complementary_skills:
            console.print(f"  Complementary: {', '.join(skill.complementary_skills)}")
        if skill.leads_to:
            console.print(f"  Leads to: {', '.join(skill.leads_to)}")


# =============================================================================
# Template Management Commands
# =============================================================================

template_app = typer.Typer(help="ML/DL template management")
c4_app.add_typer(template_app, name="template")


@template_app.command("list")
def template_list(
    category: str = typer.Option(
        None,
        "--category",
        "-c",
        help="Filter by category (image, text, detection, generative)",
    ),
    show_all: bool = typer.Option(
        False,
        "--all",
        "-a",
        help="Show all templates including experimental",
    ),
):
    """List available ML/DL templates.

    Examples:
        c4 template list                    # List all templates
        c4 template list --category image   # Filter by category
    """
    from c4.templates import TemplateRegistry

    templates = TemplateRegistry.list_all()

    if not templates:
        console.print("[yellow]No templates registered[/yellow]")
        return

    # Filter by category
    if category:
        templates = [t for t in templates if category.lower() in t.category.value.lower()]

    console.print()
    console.print(f"[bold]ML/DL Templates ({len(templates)})[/bold]")
    console.print()

    table = Table(show_header=True)
    table.add_column("ID", style="cyan")
    table.add_column("Name")
    table.add_column("Category", style="yellow")
    table.add_column("Version", style="dim")

    for info in sorted(templates, key=lambda t: t.id):
        # Handle both enum and string category values
        category = (
            info.category.value
            if hasattr(info.category, "value")
            else str(info.category)
        )
        table.add_row(
            info.id,
            info.name,
            category,
            info.version,
        )

    console.print(table)
    console.print()
    console.print("[dim]Use 'c4 template info <id>' for details[/dim]")


@template_app.command("info")
def template_info(
    template_id: str = typer.Argument(..., help="Template ID to show info for"),
):
    """Show detailed information about a template.

    Examples:
        c4 template info image-classification
        c4 template info llm-finetuning
    """
    from c4.templates import TemplateRegistry

    template = TemplateRegistry.get(template_id)

    if not template:
        console.print(f"[red]Error:[/red] Template not found: {template_id}")
        console.print()
        console.print("[dim]Use 'c4 template list' to see available templates[/dim]")
        raise typer.Exit(1)

    config = template.config

    console.print()
    console.print(f"[bold cyan]{config.id}[/bold cyan] - {config.name}")
    console.print()
    console.print("[bold]Description:[/bold]")
    console.print(f"  {config.description}")
    console.print()

    console.print(f"[bold]Category:[/bold] {config.category.value}")
    console.print(f"[bold]Version:[/bold] {config.version}")
    console.print()

    # Parameters
    console.print("[bold]Parameters:[/bold]")
    for param in config.parameters:
        required = "[red]*[/red]" if param.required else ""
        default = f" (default: {param.default})" if param.default is not None else ""
        console.print(f"  {required}{param.name}: {param.description}{default}")
    console.print()

    # PIQ Knowledge and Tags
    if config.piq_knowledge_refs:
        console.print("[bold]PIQ Knowledge References:[/bold]")
        console.print(f"  {', '.join(config.piq_knowledge_refs)}")
        console.print()

    if config.tags:
        console.print("[bold]Tags:[/bold]")
        console.print(f"  {', '.join(config.tags)}")
        console.print()

    if config.dependencies:
        console.print("[bold]Dependencies:[/bold]")
        for dep in config.dependencies[:5]:
            console.print(f"  - {dep}")
        if len(config.dependencies) > 5:
            console.print(f"  ... and {len(config.dependencies) - 5} more")
        console.print()

    console.print("[dim]Use 'c4 template create <id>' to create a project[/dim]")


@template_app.command("create")
def template_create(
    template_id: str = typer.Argument(..., help="Template ID to use"),
    output_dir: Path = typer.Option(
        Path.cwd(),
        "--output",
        "-o",
        help="Output directory for the project",
    ),
    project_name: str = typer.Option(
        None,
        "--name",
        "-n",
        help="Project name (defaults to template default)",
    ),
    architecture: str = typer.Option(
        None,
        "--arch",
        "-a",
        help="Model architecture to use",
    ),
    dataset: str = typer.Option(
        None,
        "--dataset",
        "-d",
        help="Dataset name or path",
    ),
    num_classes: int = typer.Option(
        None,
        "--num-classes",
        help="Number of output classes",
    ),
    pretrained: bool = typer.Option(
        True,
        "--pretrained/--no-pretrained",
        help="Use pretrained weights",
    ),
    piq_enabled: bool = typer.Option(
        False,
        "--piq",
        help="Enable PIQ knowledge integration",
    ),
    force: bool = typer.Option(
        False,
        "--force",
        "-f",
        help="Overwrite existing files",
    ),
):
    """Create a new ML/DL project from a template.

    Examples:
        c4 template create image-classification --name my-classifier
        c4 template create object-detection --arch yolov8n --dataset coco
        c4 template create llm-finetuning --arch llama-2-7b --piq
    """
    from c4.templates import TemplateRegistry

    template = TemplateRegistry.get(template_id)

    if not template:
        console.print(f"[red]Error:[/red] Template not found: {template_id}")
        raise typer.Exit(1)

    # Build parameters
    params = {
        "pretrained": pretrained,
        "piq_enabled": piq_enabled,
    }

    if project_name:
        params["project_name"] = project_name
    if architecture:
        params["architecture"] = architecture
    if dataset:
        params["dataset"] = dataset
    if num_classes:
        params["num_classes"] = num_classes

    # Validate parameters
    validation = template.validate(params)
    if not validation.is_valid:
        console.print("[red]Validation errors:[/red]")
        for error in validation.errors:
            console.print(f"  - {error}")
        raise typer.Exit(1)

    if validation.warnings:
        console.print("[yellow]Warnings:[/yellow]")
        for warning in validation.warnings:
            console.print(f"  - {warning}")
        console.print()

    # Generate project
    console.print(f"[blue]Creating project from template: {template_id}[/blue]")
    console.print()

    try:
        project = template.generate_project(output_dir, params)

        console.print("[green bold]✅ Project created![/green bold]")
        console.print()
        console.print(f"[bold]Directory:[/bold] {output_dir}")
        console.print()
        console.print("[bold]Generated files:[/bold]")
        for file_path in sorted(project.files.keys()):
            console.print(f"  - {file_path}")
        console.print()

        # Show tasks
        tasks = template.generate_tasks(params)
        console.print(f"[bold]C4 Tasks ({len(tasks)}):[/bold]")
        for task in tasks[:5]:
            console.print(f"  - {task.id}: {task.title}")
        if len(tasks) > 5:
            console.print(f"  ... and {len(tasks) - 5} more")
        console.print()

        console.print("[bold]Next steps:[/bold]")
        console.print("  1. cd " + str(output_dir))
        console.print("  2. pip install -r requirements.txt")
        console.print("  3. c4 init && c4 run")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@template_app.command("validate")
def template_validate(
    template_id: str = typer.Argument(..., help="Template ID to validate"),
    config_file: Path = typer.Option(
        None,
        "--config",
        "-c",
        help="Path to config YAML file to validate",
    ),
):
    """Validate a template configuration.

    Examples:
        c4 template validate image-classification
        c4 template validate llm-finetuning --config config.yaml
    """
    from c4.templates import TemplateRegistry

    template = TemplateRegistry.get(template_id)

    if not template:
        console.print(f"[red]Error:[/red] Template not found: {template_id}")
        raise typer.Exit(1)

    # Load config if provided
    params = {}
    if config_file:
        import yaml

        if not config_file.exists():
            console.print(f"[red]Error:[/red] Config file not found: {config_file}")
            raise typer.Exit(1)

        with open(config_file) as f:
            params = yaml.safe_load(f) or {}

    # Validate
    validation = template.validate(params)

    console.print()
    console.print(f"[bold]Validating template: {template_id}[/bold]")
    console.print()

    if validation.errors:
        console.print("[red]Errors:[/red]")
        for error in validation.errors:
            console.print(f"  ✗ {error}")

    if validation.warnings:
        console.print("[yellow]Warnings:[/yellow]")
        for warning in validation.warnings:
            console.print(f"  ⚠ {warning}")

    if validation.suggestions:
        console.print("[blue]Suggestions:[/blue]")
        for suggestion in validation.suggestions:
            console.print(f"  💡 {suggestion}")

    console.print()
    if validation.is_valid:
        console.print("[green bold]✅ Validation passed[/green bold]")
    else:
        console.print("[red bold]✗ Validation failed[/red bold]")
        raise typer.Exit(1)


# =============================================================================
# Team Management Commands
# =============================================================================

team_app = typer.Typer(help="Team collaboration commands")
c4_app.add_typer(team_app, name="team")


@team_app.command("list")
def team_list():
    """List your teams.

    Shows all teams you are a member of.

    Examples:
        c4 team list
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        console.print("[dim]Set SUPABASE_URL and SUPABASE_KEY in .env[/dim]")
        raise typer.Exit(1)

    try:
        _service = create_team_service()  # Reserved for future use
        # For now, show placeholder - need user auth context
        console.print("[yellow]Note:[/yellow] Team listing requires authentication")
        console.print()
        console.print("To configure team access:")
        console.print("  1. Set SUPABASE_URL and SUPABASE_KEY in .env")
        console.print("  2. Run 'c4 login' to authenticate")
        console.print("  3. Use 'c4 team list' to see your teams")
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("create")
def team_create(
    name: str = typer.Argument(..., help="Team name"),
    slug: str = typer.Option(
        None,
        "--slug",
        "-s",
        help="URL-friendly team identifier (auto-generated if not provided)",
    ),
):
    """Create a new team.

    Creates a team and makes you the owner.

    Examples:
        c4 team create "My Team"
        c4 team create "My Team" --slug my-team
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    try:
        service = create_team_service()
        team = service.create_team(name=name, slug=slug)
        console.print(f"[green]Team created:[/green] {team.name}")
        console.print(f"  ID: {team.id}")
        console.print(f"  Slug: {team.slug}")
        console.print(f"  Plan: {team.plan.value}")
        console.print()
        console.print("[dim]Invite members with:[/dim]")
        console.print(f"  c4 team invite {team.id} <email>")
    except Exception as e:
        console.print(f"[red]Error creating team:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("info")
def team_info(
    team_id: str = typer.Argument(..., help="Team ID or slug"),
):
    """Show team details.

    Examples:
        c4 team info <team-id>
        c4 team info my-team-slug
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    try:
        service = create_team_service()

        # Try by ID first, then by slug
        try:
            team = service.get_team(team_id)
        except Exception:
            team = service.get_team_by_slug(team_id)

        console.print()
        console.print(f"[bold]{team.name}[/bold]")
        console.print(f"  ID: {team.id}")
        console.print(f"  Slug: {team.slug}")
        console.print(f"  Owner: {team.owner_id}")
        console.print(f"  Plan: {team.plan.value}")
        console.print(f"  Created: {team.created_at}")

        # Show members
        members = service.get_team_members(team.id)
        console.print()
        console.print(f"[bold]Members ({len(members)})[/bold]")

        table = Table(show_header=True)
        table.add_column("Email")
        table.add_column("Role")
        table.add_column("Joined")

        for member in members:
            table.add_row(
                member.email or member.user_id,
                member.role.value,
                member.joined_at[:10] if member.joined_at else "-",
            )

        console.print(table)
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("members")
def team_members(
    team_id: str = typer.Argument(..., help="Team ID"),
):
    """List team members.

    Examples:
        c4 team members <team-id>
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    try:
        service = create_team_service()
        members = service.get_team_members(team_id)

        console.print()
        console.print(f"[bold]Team Members ({len(members)})[/bold]")

        table = Table(show_header=True)
        table.add_column("ID", style="dim")
        table.add_column("Email")
        table.add_column("Role", style="cyan")
        table.add_column("Joined")

        for member in members:
            table.add_row(
                member.id[:8] + "...",
                member.email or member.user_id,
                member.role.value,
                member.joined_at[:10] if member.joined_at else "-",
            )

        console.print(table)
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("invite")
def team_invite(
    team_id: str = typer.Argument(..., help="Team ID"),
    email: str = typer.Argument(..., help="Email to invite"),
    role: str = typer.Option(
        "member",
        "--role",
        "-r",
        help="Role: owner, admin, member, viewer",
    ),
):
    """Invite a member to the team.

    Examples:
        c4 team invite <team-id> user@example.com
        c4 team invite <team-id> user@example.com --role admin
    """
    try:
        from .services.teams import TeamRole, create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    try:
        # Parse role
        try:
            team_role = TeamRole(role.lower())
        except ValueError:
            console.print(f"[red]Invalid role:[/red] {role}")
            console.print("[dim]Valid roles: owner, admin, member, viewer[/dim]")
            raise typer.Exit(1)

        service = create_team_service()
        invite = service.create_invite(team_id, email, team_role)

        console.print(f"[green]Invitation sent to:[/green] {email}")
        console.print(f"  Role: {invite.role.value}")
        console.print(f"  Token: {invite.token[:8]}...")
        console.print(f"  Expires: {invite.expires_at}")
        console.print()
        console.print("[dim]Share this invite link:[/dim]")
        console.print(f"  https://c4.dev/invite/{invite.token}")
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("join")
def team_join(
    token: str = typer.Argument(..., help="Invitation token"),
):
    """Accept a team invitation.

    Examples:
        c4 team join <invite-token>
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    try:
        service = create_team_service()
        member = service.accept_invite(token)

        console.print("[green]Successfully joined team![/green]")
        console.print(f"  Team ID: {member.team_id}")
        console.print(f"  Role: {member.role.value}")
        console.print()
        console.print("[dim]View team details with:[/dim]")
        console.print(f"  c4 team info {member.team_id}")
    except Exception as e:
        console.print(f"[red]Error joining team:[/red] {e}")
        raise typer.Exit(1)


@team_app.command("leave")
def team_leave(
    team_id: str = typer.Argument(..., help="Team ID"),
    confirm: bool = typer.Option(
        False,
        "--yes",
        "-y",
        help="Skip confirmation",
    ),
):
    """Leave a team.

    Examples:
        c4 team leave <team-id>
        c4 team leave <team-id> --yes
    """
    try:
        from .services.teams import create_team_service
    except ImportError as e:
        console.print(f"[red]Error:[/red] Supabase not configured: {e}")
        raise typer.Exit(1)

    if not confirm:
        confirm = typer.confirm(f"Are you sure you want to leave team {team_id}?")
        if not confirm:
            console.print("[yellow]Cancelled[/yellow]")
            raise typer.Exit(0)

    try:
        _service = create_team_service()  # Reserved for future use
        # Note: This requires knowing the current user's member ID
        console.print("[yellow]Note:[/yellow] Team leave requires user authentication context")
        console.print("[dim]Use the web UI to leave teams for now[/dim]")
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


# =============================================================================
# Migration Commands
# =============================================================================

migrate_app = typer.Typer(help="Data migration commands")
c4_app.add_typer(migrate_app, name="migrate")


@migrate_app.command("export")
def migrate_export(
    output: Path = typer.Option(
        None,
        "--output",
        "-o",
        help="Output file path (default: .c4/export.json)",
    ),
    project_id: str = typer.Option(
        None,
        "--project-id",
        help="Project ID (auto-detected from .c4)",
    ),
):
    """Export local project data for migration.

    Exports state, tasks, and locks to a JSON file that can be
    imported to another backend (e.g., Supabase).

    Examples:
        c4 migrate export
        c4 migrate export --output backup.json
    """
    from .store.migration import MigrationManager

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    if not c4_dir.exists():
        console.print("[red]Error:[/red] No .c4 directory found")
        console.print("[dim]Run 'c4 init' first[/dim]")
        raise typer.Exit(1)

    db_path = c4_dir / "c4.db"
    if not db_path.exists():
        console.print("[red]Error:[/red] No local database found")
        raise typer.Exit(1)

    # Determine project ID
    if not project_id:
        project_id = str(c4_dir.parent.resolve())

    # Set output path
    if not output:
        output = c4_dir / "export.json"

    try:
        manager = MigrationManager(db_path)
        export_data = manager.export_to_file(project_id, output)

        console.print("[green]Export successful![/green]")
        console.print(f"  Project: {export_data.project_id}")
        console.print(f"  Tasks: {len(export_data.tasks)}")
        console.print(f"  Locks: {len(export_data.locks)}")
        console.print(f"  Output: {output}")
        console.print()
        console.print("[dim]Import with:[/dim]")
        console.print(f"  c4 migrate import {output}")
    except Exception as e:
        console.print(f"[red]Export failed:[/red] {e}")
        raise typer.Exit(1)


@migrate_app.command("import")
def migrate_import(
    input_file: Path = typer.Argument(..., help="Input JSON file"),
    no_backup: bool = typer.Option(
        False,
        "--no-backup",
        help="Skip backup before import",
    ),
):
    """Import project data from export file.

    Imports state, tasks, and locks from a JSON export file.
    Creates a backup before import by default.

    Examples:
        c4 migrate import backup.json
        c4 migrate import backup.json --no-backup
    """
    from .store.migration import MigrationManager

    if not input_file.exists():
        console.print(f"[red]Error:[/red] File not found: {input_file}")
        raise typer.Exit(1)

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    if not c4_dir.exists():
        console.print("[red]Error:[/red] No .c4 directory found")
        console.print("[dim]Run 'c4 init' first[/dim]")
        raise typer.Exit(1)

    db_path = c4_dir / "c4.db"

    try:
        manager = MigrationManager(db_path)
        snapshot = manager.import_from_file(input_file, create_backup=not no_backup)

        console.print("[green]Import successful![/green]")
        console.print(f"  Snapshot: {snapshot.snapshot_id}")
        console.print(f"  Tasks imported: {snapshot.tasks_count}")
        console.print(f"  Locks imported: {snapshot.locks_count}")
        console.print(f"  Status: {snapshot.status}")
        if snapshot.backup_path:
            console.print(f"  Backup: {snapshot.backup_path}")
        console.print()
        console.print("[dim]Rollback with:[/dim]")
        console.print(f"  c4 migrate rollback {snapshot.snapshot_id}")
    except Exception as e:
        console.print(f"[red]Import failed:[/red] {e}")
        raise typer.Exit(1)


@migrate_app.command("rollback")
def migrate_rollback(
    snapshot_id: str = typer.Argument(
        None,
        help="Snapshot ID to rollback to (default: most recent)",
    ),
):
    """Rollback to a previous state.

    Restores the database from a backup created during import.

    Examples:
        c4 migrate rollback                    # Most recent
        c4 migrate rollback import-20250124-1  # Specific snapshot
    """
    from .store.migration import MigrationManager

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    if not c4_dir.exists():
        console.print("[red]Error:[/red] No .c4 directory found")
        raise typer.Exit(1)

    db_path = c4_dir / "c4.db"

    try:
        manager = MigrationManager(db_path)

        if manager.rollback(snapshot_id):
            console.print("[green]Rollback successful![/green]")
            if snapshot_id:
                console.print(f"  Restored to: {snapshot_id}")
            else:
                console.print("  Restored to: most recent backup")
        else:
            console.print("[red]Rollback failed[/red]")
            raise typer.Exit(1)
    except Exception as e:
        console.print(f"[red]Rollback failed:[/red] {e}")
        raise typer.Exit(1)


@migrate_app.command("backups")
def migrate_backups():
    """List available backups.

    Shows all migration backups that can be used for rollback.

    Examples:
        c4 migrate backups
    """
    from .store.migration import MigrationManager

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    if not c4_dir.exists():
        console.print("[red]Error:[/red] No .c4 directory found")
        raise typer.Exit(1)

    db_path = c4_dir / "c4.db"

    try:
        manager = MigrationManager(db_path)
        backups = manager.list_backups()
        snapshots = manager.list_snapshots()

        if not backups and not snapshots:
            console.print("[yellow]No backups found[/yellow]")
            return

        console.print()
        console.print("[bold]Backups[/bold]")

        if backups:
            table = Table(show_header=True)
            table.add_column("Name")
            table.add_column("Size")
            table.add_column("Created")

            for backup in backups:
                size_mb = backup["size_bytes"] / (1024 * 1024)
                table.add_row(
                    backup["name"],
                    f"{size_mb:.2f} MB",
                    backup["created_at"][:19],
                )

            console.print(table)
        else:
            console.print("[dim]No backup files[/dim]")

        console.print()
        console.print("[bold]Snapshots[/bold]")

        if snapshots:
            table = Table(show_header=True)
            table.add_column("ID")
            table.add_column("Direction")
            table.add_column("Status")
            table.add_column("Tasks")
            table.add_column("Created")

            for snap in snapshots:
                table.add_row(
                    snap.snapshot_id,
                    f"{snap.source} → {snap.target}",
                    snap.status,
                    str(snap.tasks_count),
                    snap.timestamp[:19],
                )

            console.print(table)
        else:
            console.print("[dim]No migration snapshots[/dim]")
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(1)


@migrate_app.command("to-cloud")
def migrate_to_cloud(
    team_id: str = typer.Argument(..., help="Target team ID"),
    project_id: str = typer.Option(
        None,
        "--project-id",
        help="Project ID (auto-detected from .c4)",
    ),
):
    """Migrate local project to Supabase cloud.

    Exports local SQLite data and uploads to team's cloud storage.
    Includes state, tasks, and locks.

    Examples:
        c4 migrate to-cloud <team-id>
    """
    from .store.migration import sync_with_supabase

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    if not c4_dir.exists():
        console.print("[red]Error:[/red] No .c4 directory found")
        raise typer.Exit(1)

    db_path = c4_dir / "c4.db"
    if not db_path.exists():
        console.print("[red]Error:[/red] No local database found")
        raise typer.Exit(1)

    # Determine project ID
    if not project_id:
        project_id = str(c4_dir.parent.resolve())

    console.print("[bold]Migrating local project to Supabase cloud...[/bold]")
    console.print()

    try:
        # Use the synchronous wrapper for direct migration
        console.print("[blue]Step 1/2:[/blue] Creating backup and exporting data...")
        snapshot = sync_with_supabase(
            db_path=db_path,
            project_id=project_id,
            team_id=team_id,
            direction="upload",
        )

        console.print(f"  ✓ Backup created: {snapshot.backup_path}")
        console.print(f"  ✓ Exported {snapshot.tasks_count} tasks, {snapshot.locks_count} locks")

        console.print("[blue]Step 2/2:[/blue] Uploading to Supabase...")
        console.print(f"  ✓ Status: {snapshot.status}")

        console.print()
        console.print("[green]Migration complete![/green]")
        console.print(f"  Project: {project_id}")
        console.print(f"  Team: {team_id}")
        console.print(f"  Snapshot ID: {snapshot.snapshot_id}")
        console.print()
        console.print("[dim]Update .c4/config.yaml to use Supabase backend:[/dim]")
        console.print("  store:")
        console.print("    backend: supabase")
        console.print(f"    team_id: {team_id}")
        console.print()
        console.print("[dim]To rollback if needed:[/dim]")
        console.print(f"  c4 migrate rollback {snapshot.snapshot_id}")

    except ImportError:
        console.print("[red]Error:[/red] Supabase not installed")
        console.print("[dim]Install with: uv add 'c4[cloud]'[/dim]")
        raise typer.Exit(1)
    except Exception as e:
        console.print(f"[red]Migration failed:[/red] {e}")
        raise typer.Exit(1)


@migrate_app.command("from-cloud")
def migrate_from_cloud(
    team_id: str = typer.Argument(..., help="Source team ID"),
    project_id: str = typer.Option(
        None,
        "--project-id",
        help="Project ID (auto-detected from .c4)",
    ),
    no_backup: bool = typer.Option(False, "--no-backup", help="Skip backup"),
):
    """Download project from Supabase cloud to local.

    Downloads state, tasks, and locks from team's cloud storage.

    Examples:
        c4 migrate from-cloud <team-id>
    """
    from .store.migration import sync_with_supabase

    # Find .c4 directory
    c4_dir = Path(os.environ.get("C4_PROJECT_ROOT", ".")) / ".c4"
    c4_dir.mkdir(parents=True, exist_ok=True)

    db_path = c4_dir / "c4.db"

    # Determine project ID
    if not project_id:
        project_id = str(c4_dir.parent.resolve())

    console.print("[bold]Downloading project from Supabase cloud...[/bold]")
    console.print()

    try:
        console.print("[blue]Step 1/2:[/blue] Connecting to Supabase...")
        snapshot = sync_with_supabase(
            db_path=db_path,
            project_id=project_id,
            team_id=team_id,
            direction="download",
        )

        console.print("[blue]Step 2/2:[/blue] Importing data...")
        console.print(f"  ✓ Imported {snapshot.tasks_count} tasks, {snapshot.locks_count} locks")
        if snapshot.backup_path:
            console.print(f"  ✓ Backup created: {snapshot.backup_path}")

        console.print()
        console.print("[green]Download complete![/green]")
        console.print(f"  Project: {project_id}")
        console.print(f"  Team: {team_id}")
        console.print(f"  Snapshot ID: {snapshot.snapshot_id}")

    except ImportError:
        console.print("[red]Error:[/red] Supabase not installed")
        console.print("[dim]Install with: uv add 'c4[cloud]'[/dim]")
        raise typer.Exit(1)
    except Exception as e:
        console.print(f"[red]Download failed:[/red] {e}")
        raise typer.Exit(1)


# =============================================================================
# Main entry points
# =============================================================================



# =============================================================================
# Git Hooks Commands
# =============================================================================

hooks_app = typer.Typer(help="Git hooks management for C4 workflow")
c4_app.add_typer(hooks_app, name="hooks")


@hooks_app.command("install")
def hooks_install(
    force: bool = typer.Option(False, "--force", "-f", help="Overwrite existing hooks"),
):
    """Install C4 Git hooks.

    Installs pre-commit (lint), commit-msg (Task ID validation), and post-commit hooks.

    Examples:
        c4 hooks install
        c4 hooks install --force
    """
    from .git_hooks import install_all_hooks

    console.print("[bold]Installing C4 Git hooks...[/bold]")
    console.print()

    results = install_all_hooks(force=force)

    success_count = 0
    for hook_name, (success, message) in results.items():
        if success:
            console.print(f"  [green]✓[/green] {message}")
            success_count += 1
        else:
            console.print(f"  [yellow]![/yellow] {message}")

    console.print()
    if success_count == len(results):
        console.print("[green]All hooks installed successfully![/green]")
    else:
        console.print(f"[yellow]{success_count}/{len(results)} hooks installed[/yellow]")


@hooks_app.command("uninstall")
def hooks_uninstall():
    """Uninstall C4 Git hooks.

    Only removes hooks that were installed by C4.

    Examples:
        c4 hooks uninstall
    """
    from .git_hooks import uninstall_all_hooks

    console.print("[bold]Uninstalling C4 Git hooks...[/bold]")
    console.print()

    results = uninstall_all_hooks()

    for hook_name, (success, message) in results.items():
        if success:
            console.print(f"  [green]✓[/green] {message}")
        else:
            console.print(f"  [yellow]![/yellow] {message}")

    console.print()
    console.print("[green]Done![/green]")


@hooks_app.command("status")
def hooks_status():
    """Show status of C4 Git hooks.

    Examples:
        c4 hooks status
    """
    from .git_hooks import get_all_hook_status, get_git_hooks_dir

    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        console.print("[red]Error:[/red] Not in a git repository")
        raise typer.Exit(1)

    console.print("[bold]C4 Git Hooks Status[/bold]")
    console.print(f"[dim]Hooks directory: {hooks_dir}[/dim]")
    console.print()

    results = get_all_hook_status()

    for hook_name, status in results.items():
        if not status.get("installed"):
            console.print(f"  {hook_name}: [dim]Not installed[/dim]")
        elif status.get("is_c4"):
            exec_status = "[green]executable[/green]" if status.get("executable") else "[yellow]not executable[/yellow]"
            console.print(f"  {hook_name}: [green]Installed (C4)[/green] - {exec_status}")
        else:
            console.print(f"  {hook_name}: [yellow]Installed (external)[/yellow]")

    console.print()
    console.print("[dim]Use 'c4 hooks install' to install missing hooks[/dim]")


def main_c4d():
    """Entry point for c4d command"""
    app()


def main_c4():
    """Entry point for c4 command"""
    c4_app()


if __name__ == "__main__":
    app()
