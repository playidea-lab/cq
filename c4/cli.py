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

from .hooks import get_c4_install_dir, install_all_hooks
from .mcp_server import C4Daemon
from .models import ProjectStatus, Task
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
    no_args_is_help=True,
)

console = Console()


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
    """Create .claude/settings.json in project."""
    settings_dir = project_path / ".claude"
    settings_dir.mkdir(parents=True, exist_ok=True)

    settings_file = settings_dir / "settings.json"
    settings = {"enableAllProjectMcpServers": True}

    settings_file.write_text(json.dumps(settings, indent=2))
    return True


def _setup_permissions(project_path: Path) -> bool:
    """Set up allowedTools in ~/.claude.json"""
    config_path = Path.home() / ".claude.json"

    # Load existing config
    if config_path.exists():
        try:
            config = json.loads(config_path.read_text())
        except json.JSONDecodeError:
            config = {}
    else:
        config = {}

    # Ensure structure
    if "projects" not in config:
        config["projects"] = {}

    project_key = str(project_path)
    if project_key not in config["projects"]:
        config["projects"][project_key] = {}

    # Set allowedTools - EXACTLY these values, no modifications
    # MCP tools need wildcard suffix to match all tool names (e.g., mcp__c4__c4_add_todo)
    # Note: project_path already starts with /, so don't add another one
    config["projects"][project_key]["allowedTools"] = [
        f"Write({project_path}/**)",
        f"Edit({project_path}/**)",
        f"Read({project_path}/**)",
        "Bash(*)",
        "mcp__c4__*",
        "mcp__serena__*",
        "mcp__plugin_serena_serena__*",
    ]

    config_path.write_text(json.dumps(config, indent=2))
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

        # Step 2: Create .mcp.json
        console.print("[dim]Creating .mcp.json...[/dim]")
        _create_mcp_config(project_path)

        # Step 3: Create .claude/settings.json
        console.print("[dim]Creating .claude/settings.json...[/dim]")
        _create_project_settings(project_path)

        # Step 4: Set up permissions in ~/.claude.json
        console.print("[dim]Setting up permissions (Bash(*), MCP tools)...[/dim]")
        _setup_permissions(project_path)

        # Step 5: Install hooks (unless skipped)
        if not skip_hooks:
            console.print("[dim]Installing Claude Code hooks...[/dim]")
            hook_results = install_all_hooks()
            if not all(hook_results.values()):
                console.print("[yellow]Warning:[/yellow] Some hooks may not have been installed")

        # Success message
        console.print()
        console.print("[green bold]✅ C4 initialized![/green bold]")
        console.print()
        console.print(f"[bold]Project:[/bold] {project_path.name}")
        console.print(f"[bold]Path:[/bold] {project_path}")
        console.print()

        console.print("[bold]Created/Updated:[/bold]")
        console.print("  .c4/                    - C4 state directory")
        console.print("  .mcp.json               - MCP server config")
        console.print("  .claude/settings.json   - MCP auto-approval")
        console.print("  ~/.claude.json          - Bash(*) permissions")
        if not skip_hooks:
            console.print("  ~/.claude/hooks/        - Stop & security hooks")
        console.print()

        console.print("[bold]🔒 Security:[/bold]")
        console.print("  - Bash Security Hook: Blocks dangerous commands")
        console.print("  - Stop Hook: Prevents exit during active work")
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
