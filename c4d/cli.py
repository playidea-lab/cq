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
        console.print(f"[green]Starting C4 daemon in foreground...[/green]")
        console.print(f"Project: {project}")
        console.print(f"Log: {log_file}")

        # Run MCP server directly
        from .mcp_server import main
        import asyncio

        asyncio.run(main())
    else:
        # Daemonize
        console.print(f"[green]Starting C4 daemon...[/green]")

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


@c4_app.command()
def init(
    project_id: str = typer.Option(
        None,
        "--project-id",
        help="Project ID (defaults to directory name)",
    ),
):
    """Initialize C4 in the current project"""
    daemon = C4Daemon(Path.cwd())

    if daemon.is_initialized():
        console.print("[yellow]Warning:[/yellow] C4 already initialized")
        # Load and show status
        daemon.load()
        _show_status(daemon)
        return

    state = daemon.initialize(project_id)

    console.print(f"[green]C4 initialized![/green]")
    console.print(f"Project ID: {state.project_id}")
    console.print(f"Status: {state.status.value}")
    console.print()
    console.print("Created:")
    console.print("  .c4/           - C4 data directory")
    console.print("  docs/          - Plan documents")
    console.print()
    console.print("Next steps:")
    console.print("  1. Create docs/PLAN.md, docs/CHECKPOINTS.md, docs/DONE.md")
    console.print("  2. Create todo.md with tasks")
    console.print("  3. Run 'c4 run' to start execution")


@c4_app.command("status")
def c4_status():
    """Show current C4 project status"""
    daemon = C4Daemon(Path.cwd())

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
        console.print(f"[bold]Checkpoint:[/bold] {status['checkpoint']['current']} ({status['checkpoint']['state']})")

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
    daemon = C4Daemon(Path.cwd())

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


@c4_app.command()
def stop():
    """Stop execution (→ HALTED)"""
    daemon = C4Daemon(Path.cwd())

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    if not daemon.state_machine.can_transition("c4_stop"):
        console.print(f"[red]Error:[/red] Cannot stop from state {daemon.state_machine.state.status.value}")
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
    daemon = C4Daemon(Path.cwd())

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
    daemon = C4Daemon(Path.cwd())

    if not daemon.is_initialized():
        console.print("[red]Error:[/red] C4 not initialized")
        raise typer.Exit(1)

    daemon.load()

    if daemon.state_machine.state.status != ProjectStatus.EXECUTE:
        console.print(f"[red]Error:[/red] Cannot join workers in state {daemon.state_machine.state.status.value}")
        console.print("Run 'c4 run' first to start execution")
        raise typer.Exit(1)

    # Generate worker ID if not provided
    if worker_id is None:
        import uuid
        worker_id = f"worker-{uuid.uuid4().hex[:8]}"

    worker = daemon.register_worker(worker_id)
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
    daemon = C4Daemon(Path.cwd())

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
    daemon = C4Daemon(Path.cwd())

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
