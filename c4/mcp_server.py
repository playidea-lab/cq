"""C4D MCP Server - Main server implementation with MCP tools"""

import json
from datetime import datetime
from pathlib import Path
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from .daemon import LockManager, SupervisorLoopManager, WorkerManager
from .models import (
    C4Config,
    C4State,
    CheckpointQueueItem,
    CheckpointResponse,
    EventType,
    ProjectStatus,
    RepairQueueItem,
    SubmitResponse,
    Task,
    TaskAssignment,
    TaskStatus,
    ValidationResult,
)
from .state_machine import StateMachine, StateTransitionError
from .store import StateStore
from .validation import ValidationRunner


class C4Daemon:
    """C4 Daemon - manages project state and task orchestration"""

    def __init__(
        self,
        project_root: Path | None = None,
        state_store: StateStore | None = None,
    ):
        import os
        # Priority: explicit param > env var > cwd
        if project_root:
            self.root = project_root
        elif os.environ.get("C4_PROJECT_ROOT"):
            self.root = Path(os.environ["C4_PROJECT_ROOT"])
        else:
            self.root = Path.cwd()
        self.c4_dir = self.root / ".c4"
        self.state_machine: StateMachine | None = None
        self._config: C4Config | None = None
        self._tasks: dict[str, Task] = {}
        self._validation_runner: ValidationRunner | None = None
        self._lock_manager: LockManager | None = None
        self._worker_manager: WorkerManager | None = None
        self._supervisor_loop_manager: SupervisorLoopManager | None = None
        self._state_store = state_store

    # =========================================================================
    # Initialization
    # =========================================================================

    def is_initialized(self) -> bool:
        """Check if C4 is initialized in this project"""
        return (self.c4_dir / "state.json").exists()

    def initialize(self, project_id: str | None = None) -> C4State:
        """Initialize C4 in the project directory"""
        if project_id is None:
            project_id = self.root.name

        # Create directories
        self.c4_dir.mkdir(parents=True, exist_ok=True)
        (self.c4_dir / "locks").mkdir(exist_ok=True)
        (self.c4_dir / "events").mkdir(exist_ok=True)
        (self.c4_dir / "bundles").mkdir(exist_ok=True)
        (self.c4_dir / "workers").mkdir(exist_ok=True)
        (self.c4_dir / "runs").mkdir(exist_ok=True)
        (self.c4_dir / "runs" / "tests").mkdir(exist_ok=True)
        (self.c4_dir / "runs" / "logs").mkdir(exist_ok=True)

        # Create docs directory
        (self.root / "docs").mkdir(exist_ok=True)

        # Initialize state machine (with optional custom store)
        self.state_machine = StateMachine(self.c4_dir, store=self._state_store)
        state = self.state_machine.initialize_state(project_id)

        # Create default config
        self._config = C4Config(project_id=project_id)
        self._save_config()

        # Transition to PLAN
        self.state_machine.transition("c4_init")

        return state

    def load(self) -> None:
        """Load existing C4 project"""
        if not self.is_initialized():
            raise FileNotFoundError(f"C4 not initialized in {self.root}")

        self.state_machine = StateMachine(self.c4_dir, store=self._state_store)
        self.state_machine.load_state()
        self._load_config()
        self._load_tasks()

    # =========================================================================
    # Config Management
    # =========================================================================

    def _load_config(self) -> None:
        """Load config from config.yaml"""
        config_file = self.c4_dir / "config.yaml"
        if config_file.exists():
            import yaml

            data = yaml.safe_load(config_file.read_text())
            self._config = C4Config.model_validate(data)
        else:
            # Create default config
            self._config = C4Config(project_id=self.root.name)
            self._save_config()

    def _save_config(self) -> None:
        """Save config to config.yaml"""
        if self._config is None:
            return
        import yaml

        config_file = self.c4_dir / "config.yaml"
        config_file.write_text(yaml.dump(self._config.model_dump(), default_flow_style=False))

    @property
    def config(self) -> C4Config:
        if self._config is None:
            self._load_config()
        return self._config  # type: ignore

    @property
    def validation_runner(self) -> ValidationRunner:
        """Get validation runner, creating if necessary"""
        if self._validation_runner is None:
            self._validation_runner = ValidationRunner(self.root, self.config)
        return self._validation_runner

    @property
    def lock_manager(self) -> LockManager:
        """Get lock manager, creating if necessary"""
        if self._lock_manager is None:
            if self.state_machine is None:
                raise RuntimeError("C4 not loaded")
            self._lock_manager = LockManager(self.state_machine, self.config)
        return self._lock_manager

    @property
    def worker_manager(self) -> WorkerManager:
        """Get worker manager, creating if necessary"""
        if self._worker_manager is None:
            if self.state_machine is None:
                raise RuntimeError("C4 not loaded")
            self._worker_manager = WorkerManager(self.state_machine, self.config)
        return self._worker_manager

    @property
    def supervisor_loop_manager(self) -> SupervisorLoopManager:
        """Get supervisor loop manager, creating if necessary"""
        if self._supervisor_loop_manager is None:
            self._supervisor_loop_manager = SupervisorLoopManager(self)
        return self._supervisor_loop_manager

    def start_supervisor_loop(
        self,
        poll_interval: float = 1.0,
        max_retries: int = 3,
        supervisor_timeout: int = 300,
    ) -> None:
        """Start the background supervisor loop"""
        self.supervisor_loop_manager.start(
            poll_interval=poll_interval,
            max_retries=max_retries,
            supervisor_timeout=supervisor_timeout,
        )

    def stop_supervisor_loop(self) -> None:
        """Stop the background supervisor loop"""
        self.supervisor_loop_manager.stop()

    @property
    def is_supervisor_loop_running(self) -> bool:
        """Check if supervisor loop is running"""
        return self._supervisor_loop_manager is not None and self._supervisor_loop_manager.is_running

    # =========================================================================
    # Task Management
    # =========================================================================

    def _load_tasks(self) -> None:
        """Load tasks from tasks.json"""
        tasks_file = self.c4_dir / "tasks.json"
        if tasks_file.exists():
            data = json.loads(tasks_file.read_text())
            self._tasks = {t["id"]: Task.model_validate(t) for t in data}

    def _save_tasks(self) -> None:
        """Save tasks to tasks.json"""
        tasks_file = self.c4_dir / "tasks.json"
        tasks_file.write_text(
            json.dumps([t.model_dump() for t in self._tasks.values()], indent=2)
        )

    def get_task(self, task_id: str) -> Task | None:
        """Get a task by ID"""
        return self._tasks.get(task_id)

    def add_task(self, task: Task) -> None:
        """Add a task to the registry"""
        self._tasks[task.id] = task
        if task.id not in self.state_machine.state.queue.pending:
            self.state_machine.state.queue.pending.append(task.id)
        self._save_tasks()
        self.state_machine.save_state()

    # =========================================================================
    # MCP Tool Implementations
    # =========================================================================

    def c4_status(self) -> dict[str, Any]:
        """Get current C4 project status"""
        if self.state_machine is None:
            return {"error": "C4 not initialized", "initialized": False}

        state = self.state_machine.state
        return {
            "initialized": True,
            "project_id": state.project_id,
            "status": state.status.value,
            "execution_mode": state.execution_mode.value if state.execution_mode else None,
            "checkpoint": {
                "current": state.checkpoint.current,
                "state": state.checkpoint.state,
            },
            "queue": {
                "pending": len(state.queue.pending),
                "in_progress": len(state.queue.in_progress),
                "done": len(state.queue.done),
                "pending_ids": state.queue.pending[:5],  # First 5
                "in_progress_map": state.queue.in_progress,
            },
            "workers": {
                wid: {"state": w.state, "task_id": w.task_id}
                for wid, w in state.workers.items()
            },
            "metrics": state.metrics.model_dump(),
            # Async queues for automation
            "checkpoint_queue": [
                {"checkpoint_id": item.checkpoint_id, "triggered_at": item.triggered_at}
                for item in state.checkpoint_queue
            ],
            "repair_queue": [
                {"task_id": item.task_id, "attempts": item.attempts}
                for item in state.repair_queue
            ],
            "supervisor_loop_running": self.is_supervisor_loop_running,
        }

    def c4_get_task(self, worker_id: str) -> TaskAssignment | None:
        """Request next task assignment for a worker"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Ensure we're in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Register worker if not exists
        if not self.worker_manager.is_registered(worker_id):
            self.worker_manager.register(worker_id)

        # Find available task
        for task_id in state.queue.pending[:]:  # Copy to avoid mutation during iteration
            task = self.get_task(task_id)
            if task is None:
                continue

            if self.lock_manager.can_assign_task(task, worker_id):
                # Assign task
                state.queue.pending.remove(task_id)
                state.queue.in_progress[task_id] = worker_id

                # Update task
                task.status = TaskStatus.IN_PROGRESS
                task.assigned_to = worker_id
                task.branch = f"{self.config.work_branch_prefix}{task_id}"
                self._save_tasks()

                # Acquire scope lock
                if task.scope:
                    self.lock_manager.acquire(task.scope, worker_id)

                # Update worker
                self.worker_manager.set_busy(
                    worker_id, task_id, task.scope, task.branch
                )

                # Emit event
                self.state_machine.emit_event(
                    EventType.TASK_ASSIGNED,
                    "c4d",
                    {
                        "task_id": task_id,
                        "worker_id": worker_id,
                        "scope": task.scope,
                    },
                )

                self.state_machine.save_state()

                return TaskAssignment(
                    task_id=task_id,
                    title=task.title,
                    scope=task.scope,
                    dod=task.dod,
                    validations=task.validations,
                    branch=task.branch,
                )

        return None

    def c4_submit(
        self,
        task_id: str,
        commit_sha: str,
        validation_results: list[dict],
    ) -> SubmitResponse:
        """Report task completion with validation results"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Validate task exists and is in progress
        if task_id not in state.queue.in_progress:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message=f"Task {task_id} is not in progress",
            )

        # Parse validation results
        results = [ValidationResult.model_validate(r) for r in validation_results]

        # Check all validations passed
        all_passed = all(r.status == "pass" for r in results)
        if not all_passed:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message="Some validations failed",
            )

        # Get worker and task
        worker_id = state.queue.in_progress[task_id]
        task = self.get_task(task_id)

        # Move to done
        del state.queue.in_progress[task_id]
        state.queue.done.append(task_id)

        # Update task
        if task:
            task.status = TaskStatus.DONE
            task.commit_sha = commit_sha
            self._save_tasks()

        # Release scope lock
        if task and task.scope:
            self.lock_manager.release(task.scope)

        # Update worker
        self.worker_manager.set_idle(worker_id)

        # Update metrics
        state.metrics.tasks_completed += 1

        # Update last validation
        state.last_validation = {r.name: r.status for r in results}

        # Emit event
        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )

        self.state_machine.save_state()

        # Check if checkpoint reached
        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            # NON-BLOCKING: Add to checkpoint queue for async processing
            self._add_to_checkpoint_queue(cp_id, results)
            # Enter checkpoint state
            self.state_machine.enter_checkpoint(cp_id)
            return SubmitResponse(
                success=True,
                next_action="await_checkpoint",
                message=f"Checkpoint {cp_id} queued for review",
            )

        # Check if all done
        if not state.queue.pending and not state.queue.in_progress:
            return SubmitResponse(
                success=True,
                next_action="complete",
                message="All tasks completed",
            )

        return SubmitResponse(
            success=True,
            next_action="get_next_task",
            message="Task completed successfully",
        )

    def _add_to_checkpoint_queue(
        self, checkpoint_id: str, validation_results: list[ValidationResult]
    ) -> None:
        """Add checkpoint to queue for async supervisor processing"""
        if self.state_machine is None:
            return

        state = self.state_machine.state

        # Check if already in queue (avoid duplicates)
        if any(item.checkpoint_id == checkpoint_id for item in state.checkpoint_queue):
            return

        item = CheckpointQueueItem(
            checkpoint_id=checkpoint_id,
            triggered_at=datetime.now().isoformat(),
            tasks_completed=list(state.queue.done),
            validation_results=validation_results,
        )
        state.checkpoint_queue.append(item)
        self.state_machine.save_state()

    def c4_add_todo(
        self,
        task_id: str,
        title: str,
        scope: str | None,
        dod: str,
    ) -> dict[str, Any]:
        """Add a new task (used by REQUEST_CHANGES)"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        task = Task(
            id=task_id,
            title=title,
            scope=scope,
            dod=dod,
        )
        self.add_task(task)

        return {"success": True, "task_id": task_id}

    def c4_checkpoint(
        self,
        checkpoint_id: str,
        decision: str,
        notes: str,
        required_changes: list[str] | None = None,
    ) -> CheckpointResponse:
        """Record supervisor checkpoint decision"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Validate we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return CheckpointResponse(
                success=False,
                message=f"Not in CHECKPOINT state (current: {state.status.value})",
            )

        # Emit decision event
        self.state_machine.emit_event(
            EventType.SUPERVISOR_DECISION,
            "supervisor",
            {
                "checkpoint_id": checkpoint_id,
                "decision": decision,
                "notes": notes,
                "required_changes": required_changes,
            },
        )

        # Process decision
        try:
            if decision == "APPROVE":
                # Add to passed checkpoints to prevent re-triggering
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Check if this is the final checkpoint
                is_final = not state.queue.pending
                if is_final:
                    self.state_machine.transition("approve_final")
                else:
                    self.state_machine.transition("approve")
                state.metrics.checkpoints_passed += 1

            elif decision == "REQUEST_CHANGES":
                # Mark checkpoint as passed to prevent re-triggering
                # (supervisor has reviewed; RC tasks are the follow-up)
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Add required changes as tasks
                if required_changes:
                    for i, change in enumerate(required_changes):
                        task_id = f"RC-{checkpoint_id}-{i+1:02d}"
                        self.c4_add_todo(
                            task_id=task_id,
                            title=change,
                            scope=None,
                            dod=change,
                        )
                self.state_machine.transition("request_changes")

            elif decision == "REPLAN":
                self.state_machine.transition("replan")

            else:
                return CheckpointResponse(
                    success=False,
                    message=f"Invalid decision: {decision}",
                )

            # Clear checkpoint state
            state.checkpoint.current = None
            state.checkpoint.state = "pending"

            return CheckpointResponse(success=True)

        except StateTransitionError as e:
            return CheckpointResponse(success=False, message=str(e))

    def c4_mark_blocked(
        self,
        task_id: str,
        worker_id: str,
        failure_signature: str,
        attempts: int,
        last_error: str = "",
    ) -> dict[str, Any]:
        """
        Mark a task as blocked after max retry attempts.
        Adds the task to repair queue for supervisor guidance.

        Args:
            task_id: ID of the blocked task
            worker_id: ID of the worker that was working on the task
            failure_signature: Error signature from validation failures
            attempts: Number of fix attempts made
            last_error: Last error message

        Returns:
            Dictionary with success status and message
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Move task from in_progress back to pending (will be picked up after repair)
        if task_id in state.queue.in_progress:
            del state.queue.in_progress[task_id]

        # Get task and release scope lock
        task = self.get_task(task_id)
        if task and task.scope:
            self.lock_manager.release(task.scope)

        # Update worker state
        if self.worker_manager.is_registered(worker_id):
            self.worker_manager.set_idle(worker_id)

        # Add to repair queue
        item = RepairQueueItem(
            task_id=task_id,
            worker_id=worker_id,
            failure_signature=failure_signature,
            attempts=attempts,
            blocked_at=datetime.now().isoformat(),
            last_error=last_error,
        )
        state.repair_queue.append(item)

        # Emit event
        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,  # Reuse event type for blocked
            worker_id,
            {
                "task_id": task_id,
                "blocked": True,
                "failure_signature": failure_signature,
                "attempts": attempts,
            },
        )

        self.state_machine.save_state()

        return {
            "success": True,
            "message": f"Task {task_id} marked as blocked and added to repair queue",
            "repair_queue_size": len(state.repair_queue),
        }

    def c4_run_validation(
        self,
        names: list[str] | None = None,
        fail_fast: bool = True,
        timeout: int = 300,
    ) -> dict[str, Any]:
        """
        Run validation commands (lint, test, etc.)

        Args:
            names: Specific validations to run. If None, runs all required.
            fail_fast: Stop on first failure
            timeout: Timeout per validation in seconds

        Returns:
            Dictionary with validation results
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        try:
            if names:
                runs = self.validation_runner.run_validations(
                    names, fail_fast=fail_fast, timeout=timeout
                )
            else:
                runs = self.validation_runner.run_all_required(
                    timeout_per_validation=timeout
                )

            # Convert to results
            results = []
            all_passed = True
            for run in runs:
                result = run.to_result()
                results.append(result.model_dump())
                if not run.passed:
                    all_passed = False

            # Update state with last validation results
            self.state_machine.state.last_validation = {
                r["name"]: r["status"] for r in results
            }
            self.state_machine.save_state()

            # Emit event
            self.state_machine.emit_event(
                EventType.VALIDATION_RUN,
                "worker",
                {
                    "validations": [r["name"] for r in results],
                    "all_passed": all_passed,
                    "results": results,
                },
            )

            return {
                "success": all_passed,
                "results": results,
                "summary": {
                    "total": len(results),
                    "passed": sum(1 for r in results if r["status"] == "pass"),
                    "failed": sum(1 for r in results if r["status"] == "fail"),
                },
            }

        except ValueError as e:
            return {"success": False, "error": str(e), "results": []}
        except Exception as e:
            return {"success": False, "error": str(e), "results": []}

    def check_and_trigger_checkpoint(self) -> dict[str, Any] | None:
        """
        Check if checkpoint conditions are met and trigger if so.

        Returns:
            Checkpoint info if triggered, None otherwise
        """
        if self.state_machine is None:
            return None

        state = self.state_machine.state

        # Only check in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Check gate conditions
        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            # Enter checkpoint state
            self.state_machine.enter_checkpoint(cp_id)

            return {
                "checkpoint_id": cp_id,
                "triggered": True,
                "message": f"Checkpoint {cp_id} conditions met, entering CHECKPOINT state",
            }

        return None

    # =========================================================================
    # Supervisor Integration
    # =========================================================================

    def create_checkpoint_bundle(self, checkpoint_id: str | None = None) -> Path:
        """
        Create a bundle for supervisor review.

        Args:
            checkpoint_id: Checkpoint ID (uses current if not specified)

        Returns:
            Path to the created bundle directory
        """
        from .bundle import BundleCreator

        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Use current checkpoint if not specified
        if checkpoint_id is None:
            checkpoint_id = state.checkpoint.current
        if checkpoint_id is None:
            raise ValueError("No checkpoint ID specified or active")

        # Get completed tasks
        tasks_completed = list(state.queue.done)

        # Get last validation results
        validation_results = []
        if state.last_validation:
            for name, status in state.last_validation.items():
                validation_results.append({"name": name, "status": status})

        # Create bundle
        bundle_creator = BundleCreator(self.root, self.c4_dir)
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id=checkpoint_id,
            tasks_completed=tasks_completed,
            validation_results=validation_results,
        )

        return bundle_dir

    def run_supervisor_review(
        self,
        bundle_dir: Path | None = None,
        use_mock: bool = False,
        mock_decision: str = "APPROVE",
        timeout: int = 300,
        max_retries: int = 3,
    ) -> dict[str, Any]:
        """
        Run supervisor review on a checkpoint bundle.

        Args:
            bundle_dir: Path to bundle (creates new if None)
            use_mock: Use mock supervisor instead of real Claude CLI
            mock_decision: Decision for mock supervisor
            timeout: Timeout for real supervisor
            max_retries: Max retries for real supervisor

        Returns:
            Dictionary with supervisor decision and processing result
        """
        from .models import SupervisorDecision
        from .supervisor import Supervisor, SupervisorError

        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Ensure we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return {
                "success": False,
                "error": f"Not in CHECKPOINT state (current: {state.status.value})",
            }

        # Create bundle if not provided
        if bundle_dir is None:
            bundle_dir = self.create_checkpoint_bundle()

        # Run supervisor
        supervisor = Supervisor(self.root, prompts_dir=self.root / "prompts")

        try:
            if use_mock:
                decision_enum = SupervisorDecision(mock_decision)
                response = supervisor.run_supervisor_mock(
                    bundle_dir,
                    mock_decision=decision_enum,
                    mock_notes=f"Mock {mock_decision} decision",
                    mock_changes=["Mock change 1"] if mock_decision == "REQUEST_CHANGES" else None,
                )
            else:
                response = supervisor.run_supervisor(
                    bundle_dir,
                    timeout=timeout,
                    max_retries=max_retries,
                )

            # Process the decision
            return self.process_supervisor_decision(response)

        except SupervisorError as e:
            return {"success": False, "error": str(e)}

    def process_supervisor_decision(self, response: Any) -> dict[str, Any]:
        """
        Process supervisor decision and update state accordingly.

        Args:
            response: SupervisorResponse from supervisor

        Returns:
            Dictionary with processing result
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Apply decision via c4_checkpoint
        checkpoint_response = self.c4_checkpoint(
            checkpoint_id=response.checkpoint_id,
            decision=response.decision.value,
            notes=response.notes,
            required_changes=response.required_changes if response.required_changes else None,
        )

        return {
            "success": checkpoint_response.success,
            "decision": response.decision.value,
            "checkpoint_id": response.checkpoint_id,
            "notes": response.notes,
            "required_changes": response.required_changes,
            "new_status": self.state_machine.state.status.value,
            "message": checkpoint_response.message,
        }


# =============================================================================
# MCP Server Setup
# =============================================================================


def create_server(project_root: Path | None = None) -> Server:
    """Create the MCP server with all tools registered"""
    import os

    server = Server("c4d")

    # Cache of daemons per project root
    _daemon_cache: dict[str, C4Daemon] = {}

    def get_daemon() -> C4Daemon:
        """Get or create a daemon for the current project root"""
        # Determine project root: env var > param > cwd
        if os.environ.get("C4_PROJECT_ROOT"):
            root = Path(os.environ["C4_PROJECT_ROOT"])
        elif project_root:
            root = project_root
        else:
            root = Path.cwd()

        root_str = str(root.resolve())

        if root_str not in _daemon_cache:
            daemon = C4Daemon(root)
            if daemon.is_initialized():
                daemon.load()
            _daemon_cache[root_str] = daemon

        return _daemon_cache[root_str]

    # For backward compatibility
    daemon = get_daemon()

    def clear_daemon_cache(project_root_str: str | None = None) -> bool:
        """Clear daemon cache for a specific project or all projects"""
        if project_root_str:
            if project_root_str in _daemon_cache:
                del _daemon_cache[project_root_str]
                return True
            return False
        else:
            _daemon_cache.clear()
            return True

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        return [
            Tool(
                name="c4_status",
                description="Get current C4 project status including state, queue, and workers",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
                },
            ),
            Tool(
                name="c4_clear",
                description="Clear C4 state completely. Deletes .c4 directory and clears daemon cache. Use for development/debugging.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "confirm": {
                            "type": "boolean",
                            "description": "Must be true to confirm deletion",
                        },
                        "keep_config": {
                            "type": "boolean",
                            "description": "Keep config.yaml (default: false)",
                            "default": False,
                        },
                    },
                    "required": ["confirm"],
                },
            ),
            Tool(
                name="c4_get_task",
                description="Request next task assignment for a worker.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "worker_id": {
                            "type": "string",
                            "description": "Unique identifier for the worker",
                        },
                    },
                    "required": ["worker_id"],
                },
            ),
            Tool(
                name="c4_submit",
                description="Report task completion with validation results",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "task_id": {
                            "type": "string",
                            "description": "ID of the completed task",
                        },
                        "commit_sha": {
                            "type": "string",
                            "description": "Git commit SHA of the work",
                        },
                        "validation_results": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "name": {"type": "string"},
                                    "status": {"type": "string", "enum": ["pass", "fail"]},
                                    "message": {"type": "string"},
                                },
                                "required": ["name", "status"],
                            },
                            "description": "Results of validation runs (lint, test, etc.)",
                        },
                    },
                    "required": ["task_id", "commit_sha", "validation_results"],
                },
            ),
            Tool(
                name="c4_add_todo",
                description="Add a new task to the queue (used for REQUEST_CHANGES)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "task_id": {"type": "string"},
                        "title": {"type": "string"},
                        "scope": {"type": "string"},
                        "dod": {"type": "string", "description": "Definition of Done"},
                    },
                    "required": ["task_id", "title", "dod"],
                },
            ),
            Tool(
                name="c4_checkpoint",
                description="Record supervisor checkpoint decision",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "checkpoint_id": {"type": "string"},
                        "decision": {
                            "type": "string",
                            "enum": ["APPROVE", "REQUEST_CHANGES", "REPLAN"],
                        },
                        "notes": {"type": "string"},
                        "required_changes": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "List of required changes (for REQUEST_CHANGES)",
                        },
                    },
                    "required": ["checkpoint_id", "decision", "notes"],
                },
            ),
            Tool(
                name="c4_run_validation",
                description="Run validation commands (lint, test) and return results.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "names": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Validations to run (e.g., ['lint', 'unit'])",
                        },
                        "fail_fast": {
                            "type": "boolean",
                            "description": "Stop on first failure (default: true)",
                            "default": True,
                        },
                        "timeout": {
                            "type": "integer",
                            "description": "Timeout per validation in seconds (default: 300)",
                            "default": 300,
                        },
                    },
                    "required": [],
                },
            ),
            Tool(
                name="c4_mark_blocked",
                description="Mark a task as blocked after max retry attempts. Adds to repair queue for supervisor guidance.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "task_id": {
                            "type": "string",
                            "description": "ID of the blocked task",
                        },
                        "worker_id": {
                            "type": "string",
                            "description": "ID of the worker that was working on the task",
                        },
                        "failure_signature": {
                            "type": "string",
                            "description": "Error signature from validation failures",
                        },
                        "attempts": {
                            "type": "integer",
                            "description": "Number of fix attempts made",
                        },
                        "last_error": {
                            "type": "string",
                            "description": "Last error message",
                        },
                    },
                    "required": ["task_id", "worker_id", "failure_signature", "attempts"],
                },
            ),
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict) -> list[TextContent]:
        try:
            # Get daemon dynamically for the current project
            daemon = get_daemon()

            if name == "c4_status":
                result = daemon.c4_status()
            elif name == "c4_clear":
                if not arguments.get("confirm"):
                    result = {"error": "Must set confirm=true to clear C4 state"}
                else:
                    import shutil
                    import os

                    # Get current project root
                    if os.environ.get("C4_PROJECT_ROOT"):
                        root = Path(os.environ["C4_PROJECT_ROOT"])
                    elif project_root:
                        root = project_root
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

                    # Clear daemon cache
                    root_str = str(root.resolve())
                    cache_cleared = clear_daemon_cache(root_str)

                    result = {
                        "success": True,
                        "deleted": deleted_items,
                        "cache_cleared": cache_cleared,
                        "project_root": str(root),
                        "message": "C4 state cleared. Run /c4-init to reinitialize.",
                    }
            elif name == "c4_get_task":
                result = daemon.c4_get_task(arguments["worker_id"])
                if result:
                    result = result.model_dump()
            elif name == "c4_submit":
                result = daemon.c4_submit(
                    arguments["task_id"],
                    arguments["commit_sha"],
                    arguments["validation_results"],
                )
                result = result.model_dump()
            elif name == "c4_add_todo":
                result = daemon.c4_add_todo(
                    arguments["task_id"],
                    arguments["title"],
                    arguments.get("scope"),
                    arguments["dod"],
                )
            elif name == "c4_checkpoint":
                result = daemon.c4_checkpoint(
                    arguments["checkpoint_id"],
                    arguments["decision"],
                    arguments["notes"],
                    arguments.get("required_changes"),
                )
                result = result.model_dump()
            elif name == "c4_run_validation":
                result = daemon.c4_run_validation(
                    names=arguments.get("names"),
                    fail_fast=arguments.get("fail_fast", True),
                    timeout=arguments.get("timeout", 300),
                )
            elif name == "c4_mark_blocked":
                result = daemon.c4_mark_blocked(
                    task_id=arguments["task_id"],
                    worker_id=arguments["worker_id"],
                    failure_signature=arguments["failure_signature"],
                    attempts=arguments["attempts"],
                    last_error=arguments.get("last_error", ""),
                )
            else:
                result = {"error": f"Unknown tool: {name}"}

            return [TextContent(type="text", text=json.dumps(result, indent=2, default=str))]

        except Exception as e:
            return [TextContent(type="text", text=json.dumps({"error": str(e)}))]

    return server


async def main():
    """Run the MCP server"""
    from mcp.server import InitializationOptions
    from mcp.types import ServerCapabilities, ToolsCapability

    server = create_server()
    init_options = InitializationOptions(
        server_name="c4d",
        server_version="0.1.0",
        capabilities=ServerCapabilities(
            tools=ToolsCapability(listChanged=False),
        ),
    )
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, init_options)


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
