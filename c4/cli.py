"""C4D CLI - Command line interface for c4d daemon and c4 project management"""

import json
import os
import signal
import subprocess
import sys
from pathlib import Path

import typer
from rich.console import Console
from rich.table import Table

from . import git_hooks
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


def _create_mcp_config(project_path: Path) -> bool:
    """Create .mcp.json in project root."""
    mcp_file = project_path / ".mcp.json"
    c4_install_dir = get_c4_install_dir()

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
                "env": {"C4_PROJECT_ROOT": str(project_path)},
            }
        }
    }

    mcp_file.write_text(json.dumps(config, indent=2))
    return True


def _create_project_settings(project_path: Path) -> bool:
    """Create .claude/settings.json with permissions.allow.

    This uses project-local settings instead of ~/.claude.json to avoid
    conflicts with Claude Code's runtime config management.
    """
    settings_dir = project_path / ".claude"
    settings_dir.mkdir(parents=True, exist_ok=True)

    settings_file = settings_dir / "settings.json"
    settings = {
        "permissions": {
            "allow": [
                # MCP tools (wildcard for all tools from each server)
                "mcp__c4__*",
                "mcp__serena__*",
                "mcp__plugin_serena_serena__*",
                # Shell commands (no parentheses = allow all)
                "Bash",
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
):
    """Initialize C4 in a project (all-in-one setup)

    This command performs complete C4 initialization:
    - Creates .c4/ directory with state files
    - Creates .mcp.json for MCP server configuration
    - Creates .claude/settings.json for MCP auto-approval
    - Sets up ~/.claude.json allowedTools (with Bash(*))
    - Installs Claude Code hooks (stop hook, security hook)
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
        _create_mcp_config(project_path)

        # Step 4: Create .claude/settings.json with permissions
        console.print("[dim]Creating .claude/settings.json (permissions)...[/dim]")
        _create_project_settings(project_path)

        # Step 5: Install hooks (unless skipped)
        if not skip_hooks:
            console.print("[dim]Installing Claude Code hooks...[/dim]")
            hook_results = install_all_hooks()
            if not all(hook_results.values()):
                console.print(
                    "[yellow]Warning:[/yellow] Some Claude Code hooks may not have been installed"
                )

            # Install Git hooks
            console.print("[dim]Installing Git hooks...[/dim]")
            git_hook_results = git_hooks.install_all_hooks(project_path)
            for hook_name, (success, msg) in git_hook_results.items():
                if success:
                    console.print(f"  [green]✓[/green] {hook_name}")
                else:
                    console.print(f"  [yellow]![/yellow] {hook_name}: {msg}")

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
            console.print("  .git/hooks/             - Git hooks (pre-commit, commit-msg)")
        console.print()

        console.print("[bold]🔒 Security & Workflow:[/bold]")
        console.print("  - Bash Security Hook: Blocks dangerous commands")
        console.print("  - Stop Hook: Prevents exit during active work")
        console.print("  - Git Hooks: Lint validation, Task ID tracking")
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
        cp_current = status['checkpoint']['current']
        cp_state = status['checkpoint']['state']
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
    console.print(f"[dim]Events: {status['metrics']['events_emitted']} | "
                  f"Tasks completed: {status['metrics']['tasks_completed']} | "
                  f"Checkpoints passed: {status['metrics']['checkpoints_passed']}[/dim]")


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
        console.print(f"Summary: {valid_count}/{len(results)} passed, {total_errors} errors, {total_warnings} warnings")

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
            console.print(f"  [{impact_color[rule.impact.value]}]{rule.id}[/{impact_color[rule.impact.value]}]: {rule.description[:60]}...")
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
# Main entry points
# =============================================================================


def main_c4d():
    """Entry point for c4d command"""
    app()


def main_c4():
    """Entry point for c4 command"""
    c4_app()


if __name__ == "__main__":
    app()
