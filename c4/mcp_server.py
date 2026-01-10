"""C4D MCP Server - Main server implementation with MCP tools"""

import json
import logging
from datetime import datetime
from pathlib import Path
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from .constants import MAX_REPAIR_DEPTH, REPAIR_PREFIX, REPAIR_PREFIX_LEN
from .daemon import SupervisorLoopManager, WorkerManager
from .discovery import (
    DesignStore,
    Domain,
    EARSPattern,
    EARSRequirement,
    FeatureSpec,
    SpecStore,
    VerificationRequirement,
)
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
from .store import SQLiteLockStore, SQLiteStateStore, SQLiteTaskStore, StateStore
from .supervisor.agent_router import get_recommended_agent
from .validation import ValidationRunner

logger = logging.getLogger(__name__)


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
        self._worker_manager: WorkerManager | None = None
        self._supervisor_loop_manager: SupervisorLoopManager | None = None
        self._state_store = state_store
        self._lock_store: SQLiteLockStore | None = None
        self._task_store: SQLiteTaskStore | None = None
        self._spec_store: SpecStore | None = None

    # =========================================================================
    # Initialization
    # =========================================================================

    def _get_default_store(self) -> StateStore:
        """Get the default state store (SQLite)"""
        if self._state_store is not None:
            return self._state_store
        return SQLiteStateStore(self.c4_dir / "c4.db")

    def is_initialized(self) -> bool:
        """Check if C4 is initialized in this project"""
        # Check both SQLite (new) and JSON (legacy) for backward compatibility
        return (self.c4_dir / "c4.db").exists() or (self.c4_dir / "state.json").exists()

    def initialize(
        self,
        project_id: str | None = None,
        with_default_checkpoints: bool = True,
    ) -> C4State:
        """
        Initialize C4 in the project directory.

        Args:
            project_id: Project identifier (defaults to directory name)
            with_default_checkpoints: If True, add default checkpoints (CP-REVIEW, CP-FINAL)
        """
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

        # Initialize state machine with SQLite store (default)
        self.state_machine = StateMachine(self.c4_dir, store=self._get_default_store())
        state = self.state_machine.initialize_state(project_id)

        # Create config with optional default checkpoints
        checkpoints = []
        if with_default_checkpoints:
            from c4.models.checkpoint import DEFAULT_CHECKPOINTS

            checkpoints = list(DEFAULT_CHECKPOINTS)  # Copy to avoid mutation

        self._config = C4Config(project_id=project_id, checkpoints=checkpoints)
        self._save_config()

        # Transition to DISCOVERY
        self.state_machine.transition("c4_init")

        return state

    def load(self) -> None:
        """Load existing C4 project"""
        if not self.is_initialized():
            raise FileNotFoundError(f"C4 not initialized in {self.root}")

        # Migrate from state.json to SQLite if needed
        self._migrate_to_sqlite_if_needed()

        self.state_machine = StateMachine(self.c4_dir, store=self._get_default_store())
        self.state_machine.load_state()
        self._load_config()
        self._load_tasks()

    def _migrate_to_sqlite_if_needed(self) -> None:
        """Migrate from state.json to SQLite if needed"""
        state_json = self.c4_dir / "state.json"
        db_path = self.c4_dir / "c4.db"

        # Only migrate if state.json exists and c4.db doesn't
        if state_json.exists() and not db_path.exists():
            import json

            # 1. Read JSON state
            data = json.loads(state_json.read_text())
            state = C4State.model_validate(data)

            # 2. Save to SQLite
            store = SQLiteStateStore(db_path)
            store.save(state)

            # 3. Backup original (don't delete)
            backup_path = self.c4_dir / "state.json.bak"
            state_json.rename(backup_path)

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

    def _sync_verification_to_config(self, verification: "VerificationRequirement") -> None:
        """Sync a verification requirement to config.yaml.

        This ensures verifications collected during discovery are available
        for runtime verification during checkpoint review.
        """
        from c4.models.config import VerificationItem

        # Check if already exists (by name)
        existing_names = {item.name for item in self.config.verifications.items}
        if verification.name in existing_names:
            return  # Already synced

        # Add to config
        item = VerificationItem(
            type=verification.type,
            name=verification.name,
            config=verification.config,
            enabled=verification.enabled,
        )
        self.config.verifications.items.append(item)

        # Enable verifications if not already
        if not self.config.verifications.enabled:
            self.config.verifications.enabled = True

        self._save_config()

    def _apply_domain_default_verifications(self, domain: str) -> list[str]:
        """Apply default verifications for a domain.

        Returns list of verification names that were added.
        """
        from c4.models.config import VerificationItem
        from c4.supervisor.verifier import DOMAIN_DEFAULT_VERIFICATIONS

        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get(domain, [])
        if not defaults:
            return []

        added = []
        existing_names = {item.name for item in self.config.verifications.items}

        for default in defaults:
            if default["name"] in existing_names:
                continue

            item = VerificationItem(
                type=default["type"],
                name=default["name"],
                config=default.get("config", {}),
                enabled=True,
            )
            self.config.verifications.items.append(item)
            added.append(default["name"])

        if added:
            if not self.config.verifications.enabled:
                self.config.verifications.enabled = True
            self._save_config()

        return added

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
    def lock_store(self) -> SQLiteLockStore:
        """Get SQLite lock store for atomic lock operations"""
        if self._lock_store is None:
            self._lock_store = SQLiteLockStore(self.c4_dir / "c4.db")
        return self._lock_store

    @property
    def worker_manager(self) -> WorkerManager:
        """Get worker manager, creating if necessary"""
        if self._worker_manager is None:
            if self.state_machine is None:
                raise RuntimeError("C4 not loaded")
            self._worker_manager = WorkerManager(self.state_machine, self.config)
        return self._worker_manager

    def _touch_worker(self, worker_id: str | None) -> None:
        """
        Update worker's last_seen timestamp (implicit heartbeat).

        This should be called on every MCP tool call that includes a worker_id
        to keep the worker marked as active. Prevents stale worker recovery
        from reclaiming tasks from active workers.
        """
        if worker_id and self._worker_manager is not None:
            try:
                self._worker_manager.heartbeat(worker_id)
            except Exception as e:
                # Don't fail tool calls if heartbeat fails, but log the error
                logger.warning(f"Worker heartbeat failed for {worker_id}: {e}")

    def _get_effective_domain(self, task: Task) -> str | None:
        """
        Get effective domain for a task (Phase 4: Agent routing).

        Priority:
        1. Task-specific domain (task.domain)
        2. Project config domain (self.config.domain)
        3. None (will use 'unknown' in agent routing)

        Returns:
            Domain string or None
        """
        # Task-specific domain takes priority
        if task.domain:
            return task.domain

        # Fall back to project config domain
        if self._config and self._config.domain:
            return self._config.domain

        return None

    def _get_agent_routing(self, task: Task) -> dict[str, Any]:
        """
        Get agent routing information for a task (Phase 4).

        Returns:
            Dict with agent routing fields for TaskAssignment
        """
        domain = self._get_effective_domain(task)
        agent_config = get_recommended_agent(domain)

        return {
            "recommended_agent": agent_config.primary,
            "agent_chain": agent_config.chain,
            "domain": domain,
            "handoff_instructions": agent_config.handoff_instructions,
        }

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

    @property
    def task_store(self) -> SQLiteTaskStore:
        """Get or create the task store"""
        if self._task_store is None:
            self._task_store = SQLiteTaskStore(self.c4_dir / "c4.db")
        return self._task_store

    @property
    def spec_store(self) -> SpecStore:
        """Get or create the spec store"""
        if self._spec_store is None:
            self._spec_store = SpecStore(self.c4_dir)
        return self._spec_store

    def _load_tasks(self) -> None:
        """Load tasks from SQLite (with migration from tasks.json if needed)"""
        project_id = self.state_machine.state.project_id

        # Migrate from tasks.json if SQLite is empty but tasks.json exists
        tasks_file = self.c4_dir / "tasks.json"
        if not self.task_store.exists(project_id) and tasks_file.exists():
            count = self.task_store.migrate_from_json(project_id, tasks_file)
            if count > 0:
                # Backup original tasks.json
                backup_path = self.c4_dir / "tasks.json.bak"
                if not backup_path.exists():
                    tasks_file.rename(backup_path)

        # Load from SQLite
        self._tasks = self.task_store.load_all(project_id)

    def _save_tasks(self) -> None:
        """Save all tasks to SQLite"""
        if self.state_machine is None:
            return
        project_id = self.state_machine.state.project_id
        self.task_store.save_all(project_id, self._tasks)

    def _save_task(self, task: Task) -> None:
        """Save a single task to SQLite (more efficient than save_all)"""
        if self.state_machine is None:
            return
        project_id = self.state_machine.state.project_id
        self.task_store.save(project_id, task)
        # Also update in-memory cache
        self._tasks[task.id] = task

    def get_task(self, task_id: str) -> Task | None:
        """Get a task by ID (from cache, falls back to SQLite)"""
        # Try cache first
        if task_id in self._tasks:
            return self._tasks[task_id]
        # Fall back to SQLite
        if self.state_machine is None:
            return None
        project_id = self.state_machine.state.project_id
        task = self.task_store.get(project_id, task_id)
        if task:
            self._tasks[task_id] = task  # Update cache
        return task

    def add_task(self, task: Task) -> None:
        """Add a task to the registry"""
        self._tasks[task.id] = task
        if task.id not in self.state_machine.state.queue.pending:
            self.state_machine.state.queue.pending.append(task.id)
        self._save_task(task)  # Use single-task save
        self.state_machine.save_state()

    # =========================================================================
    # MCP Tool Implementations
    # =========================================================================

    def c4_status(self) -> dict[str, Any]:
        """Get current C4 project status"""
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized", "initialized": False}

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

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        # Re-load state to get latest (prevent race conditions with other workers)
        self.state_machine.load_state()
        state = self.state_machine.state

        # Clean up expired scope locks (prevents stale locks from blocking task assignment)
        self.lock_store.cleanup_expired(state.project_id)

        # Ensure we're in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Register worker if not exists
        if not self.worker_manager.is_registered(worker_id):
            self.worker_manager.register(worker_id)

        # Check if worker already has an in_progress task (resume after crash/restart)
        for task_id, assigned_worker in list(state.queue.in_progress.items()):  # list() to allow mutation
            if assigned_worker == worker_id:
                task = self.get_task(task_id)
                if task:
                    # Verify scope lock is still valid for this worker
                    if task.scope:
                        # Check SQLite lock store (authoritative) for lock status
                        lock_owner = self.lock_store.get_lock_owner(
                            state.project_id, task.scope
                        )
                        if lock_owner is None or lock_owner != worker_id:
                            # Lock expired or taken by another worker - cannot resume
                            # Move task back to pending for reassignment by OTHER workers
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)  # Add to front of queue
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            # Also release SQLite lock if we owned it
                            self.lock_store.release_scope_lock(state.project_id, task.scope)
                            self.state_machine.save_state()
                            # Return None - let another worker pick up the task
                            # (don't try to re-acquire as same worker)
                            return None
                    else:
                        # For scope=None tasks, verify task state consistency
                        if task.assigned_to != worker_id or task.status != TaskStatus.IN_PROGRESS:
                            # Task state inconsistent with queue - reset for reassignment
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            self.state_machine.save_state()
                            continue

                    # Re-sync worker state
                    self.worker_manager.set_busy(
                        worker_id, task_id, task.scope, task.branch
                    )

                    # Refresh lock TTL for resumed work (with result check)
                    if task.scope:
                        if not self.lock_store.refresh_scope_lock(
                            state.project_id, task.scope, worker_id, self.config.scope_lock_ttl_sec
                        ):
                            # Lock refresh failed - task may have been taken
                            # Move back to pending for safe reassignment
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            self.state_machine.save_state()
                            continue

                    # Get agent routing info (Phase 4)
                    agent_routing = self._get_agent_routing(task)

                    return TaskAssignment(
                        task_id=task_id,
                        title=task.title,
                        scope=task.scope,
                        dod=task.dod,
                        validations=task.validations,
                        branch=task.branch or "",
                        **agent_routing,
                    )

        # Find available task from pending (sorted by priority, highest first)
        project_id = state.project_id
        ttl = self.config.scope_lock_ttl_sec

        # Get all pending tasks and sort by priority (descending)
        pending_tasks = []
        for task_id in state.queue.pending:
            task = self.get_task(task_id)
            if task:
                pending_tasks.append(task)
        pending_tasks.sort(key=lambda t: t.priority, reverse=True)

        for task in pending_tasks:
            task_id = task.id

            # Check dependencies first (non-locking check)
            deps_met = all(
                dep_id in state.queue.done for dep_id in task.dependencies
            )
            if not deps_met:
                continue

            # Try to acquire scope lock ATOMICALLY using SQLite
            # This prevents race conditions between workers
            if task.scope:
                lock_acquired = self.lock_store.acquire_scope_lock(
                    project_id, task.scope, worker_id, ttl
                )
                if not lock_acquired:
                    # Another worker has the lock - skip this task
                    continue

            # Lock acquired (or no scope) - now safe to assign
            # Use atomic_modify to prevent race conditions with c4_submit
            from c4.store import SQLiteStateStore

            store = self.state_machine.store
            task_branch = f"{self.config.work_branch_prefix}{task_id}"
            assigned = False

            if isinstance(store, SQLiteStateStore):
                with store.atomic_modify(project_id) as state:
                    # Double-check task is still pending
                    if task_id in state.queue.pending:
                        # Assign task (ATOMIC)
                        state.queue.pending.remove(task_id)
                        state.queue.in_progress[task_id] = worker_id

                        # Ensure worker exists in state (fix race condition)
                        if worker_id not in state.workers:
                            from c4.models import WorkerInfo

                            now = datetime.now()
                            state.workers[worker_id] = WorkerInfo(
                                worker_id=worker_id,
                                state="busy",
                                task_id=task_id,
                                scope=task.scope,
                                branch=task_branch,
                                joined_at=now,
                                last_seen=now,
                            )
                        else:
                            # Update existing worker state
                            state.workers[worker_id].state = "busy"
                            state.workers[worker_id].task_id = task_id
                            state.workers[worker_id].scope = task.scope
                            state.workers[worker_id].branch = task_branch
                            state.workers[worker_id].last_seen = datetime.now()

                        assigned = True

                    # Update cached state
                    self.state_machine._state = state
            else:
                # Fallback for non-SQLite stores
                self.state_machine.load_state()
                state = self.state_machine.state
                if task_id in state.queue.pending:
                    state.queue.pending.remove(task_id)
                    state.queue.in_progress[task_id] = worker_id
                    self.worker_manager.set_busy(
                        worker_id, task_id, task.scope, task_branch
                    )
                    self.state_machine.save_state()
                    assigned = True

            if not assigned:
                # Task was assigned by another worker - release our lock
                if task.scope:
                    self.lock_store.release_scope_lock(project_id, task.scope)
                continue

            # Update task in SQLite (outside atomic block but still atomic per-task)
            task.status = TaskStatus.IN_PROGRESS
            task.assigned_to = worker_id
            task.branch = task_branch
            self._save_task(task)

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

            # Get agent routing info (Phase 4)
            agent_routing = self._get_agent_routing(task)

            return TaskAssignment(
                task_id=task_id,
                title=task.title,
                scope=task.scope,
                dod=task.dod,
                validations=task.validations,
                branch=task_branch,
                **agent_routing,
            )

        return None

    def c4_submit(
        self,
        task_id: str,
        commit_sha: str,
        validation_results: list[dict],
        worker_id: str | None = None,
    ) -> SubmitResponse:
        """Report task completion with validation results"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        # Parse and validate results first (doesn't need state lock)
        results = [ValidationResult.model_validate(r) for r in validation_results]
        all_passed = all(r.status == "pass" for r in results)
        if not all_passed:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message="Some validations failed",
            )

        # Get task info (from tasks.json, not state)
        task = self.get_task(task_id)
        project_id = self.state_machine.state.project_id

        # Use atomic_modify for thread-safe state update
        # This prevents race conditions when multiple workers submit concurrently
        from c4.store import SQLiteStateStore

        store = self.state_machine.store
        if not isinstance(store, SQLiteStateStore):
            # Fallback for non-SQLite stores (e.g., tests with LocalFileStore)
            return self._c4_submit_legacy(
                task_id, commit_sha, results, worker_id, task
            )

        # Atomic state modification
        error_response: SubmitResponse | None = None
        actual_worker_id: str | None = None

        with store.atomic_modify(project_id) as state:
            # Validate task exists and is in progress
            if task_id not in state.queue.in_progress:
                if task_id in state.queue.done:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="get_next_task",
                        message=f"Task {task_id} already completed by another worker",
                    )
                else:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="fix_failures",
                        message=f"Task {task_id} is not in progress",
                    )
                # Still need to exit context properly - state will be saved as-is
                if error_response:
                    # Update cached state and return early
                    self.state_machine._state = state
                    # We can't return from inside context, so we'll check after

            if error_response is None:
                # Validate worker assignment
                assigned_worker = state.queue.in_progress.get(task_id)
                if worker_id and assigned_worker != worker_id:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="get_next_task",
                        message=f"Task {task_id} is assigned to {assigned_worker}, not {worker_id}",
                    )

            if error_response is None:
                # All validations passed - proceed with state modification
                actual_worker_id = state.queue.in_progress[task_id]

                # Move to done (ATOMIC - this is the critical section)
                del state.queue.in_progress[task_id]
                state.queue.done.append(task_id)

                # Update worker state (ensure worker exists first)
                if actual_worker_id in state.workers:
                    state.workers[actual_worker_id].state = "idle"
                    state.workers[actual_worker_id].task_id = None
                    state.workers[actual_worker_id].scope = None
                    state.workers[actual_worker_id].last_seen = datetime.now()
                else:
                    # Worker not in state (shouldn't happen but handle gracefully)
                    from c4.models import WorkerInfo

                    state.workers[actual_worker_id] = WorkerInfo(
                        worker_id=actual_worker_id,
                        state="idle",
                        joined_at=datetime.now(),
                        last_seen=datetime.now(),
                    )

                # Update metrics
                state.metrics.tasks_completed += 1

                # Update last validation
                state.last_validation = {r.name: r.status for r in results}

            # Update cached state in state_machine
            self.state_machine._state = state

        # Handle error responses after atomic block
        if error_response:
            return error_response

        # Non-atomic operations (safe to do outside the lock)
        # Update commit info in SQLite - status is DERIVED from c4_state.queue
        # (single source of truth), so we only update commit_sha here
        self.task_store.update_commit_info(
            project_id,
            task_id,
            commit_sha=commit_sha
        )

        # Release scope lock
        if task and task.scope:
            self.lock_store.release_scope_lock(project_id, task.scope)

        # Emit event
        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            actual_worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )

        # Get fresh state reference for final checks
        state = self.state_machine.state

        # Check if checkpoint reached
        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            self._add_to_checkpoint_queue(cp_id, results)
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

    def _c4_submit_legacy(
        self,
        task_id: str,
        commit_sha: str,
        results: list[ValidationResult],
        worker_id: str | None,
        task: Task | None,
    ) -> SubmitResponse:
        """Legacy submit for non-SQLite stores (backward compatibility)"""
        self.state_machine.load_state()
        state = self.state_machine.state

        if task_id not in state.queue.in_progress:
            if task_id in state.queue.done:
                return SubmitResponse(
                    success=False,
                    next_action="get_next_task",
                    message=f"Task {task_id} already completed",
                )
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message=f"Task {task_id} is not in progress",
            )

        assigned_worker = state.queue.in_progress.get(task_id)
        if worker_id and assigned_worker != worker_id:
            return SubmitResponse(
                success=False,
                next_action="get_next_task",
                message=f"Task {task_id} is assigned to {assigned_worker}",
            )

        actual_worker_id = state.queue.in_progress[task_id]
        del state.queue.in_progress[task_id]
        state.queue.done.append(task_id)

        if task:
            task.status = TaskStatus.DONE
            task.commit_sha = commit_sha
            self._save_task(task)

        if task and task.scope:
            self.lock_store.release_scope_lock(state.project_id, task.scope)

        self.worker_manager.set_idle(actual_worker_id)
        state.metrics.tasks_completed += 1
        state.last_validation = {r.name: r.status for r in results}

        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            actual_worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )
        self.state_machine.save_state()

        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            self._add_to_checkpoint_queue(cp_id, results)
            self.state_machine.enter_checkpoint(cp_id)
            return SubmitResponse(
                success=True,
                next_action="await_checkpoint",
                message=f"Checkpoint {cp_id} queued",
            )

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
        dependencies: list[str] | None = None,
        domain: str | None = None,
        priority: int = 0,
    ) -> dict[str, Any]:
        """Add a new task with optional dependencies.

        Args:
            task_id: Unique task identifier (e.g., "T-001")
            title: Task title
            scope: File/directory scope for lock (e.g., "src/auth/")
            dod: Definition of Done
            dependencies: List of task IDs that must complete first
            domain: Domain for agent routing (e.g., "web-frontend")
            priority: Higher priority tasks are assigned first (default: 0)
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        task = Task(
            id=task_id,
            title=title,
            scope=scope,
            dod=dod,
            dependencies=dependencies or [],
            domain=domain,
            priority=priority,
        )
        self.add_task(task)

        return {
            "success": True,
            "task_id": task_id,
            "dependencies": task.dependencies,
        }

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

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        state = self.state_machine.state

        # Prevent infinite REPAIR nesting (max 2 levels: REPAIR-REPAIR-{task})
        # Use prefix-based check to avoid false positives like "MY-REPAIR-FEATURE"
        repair_depth = 0
        temp_id = task_id
        while temp_id.startswith(REPAIR_PREFIX):
            repair_depth += 1
            temp_id = temp_id[REPAIR_PREFIX_LEN:]

        if repair_depth >= MAX_REPAIR_DEPTH:
            return {
                "success": False,
                "error": f"Max repair nesting exceeded ({repair_depth} >= {MAX_REPAIR_DEPTH})",
                "message": f"Task {task_id} has already been repaired {repair_depth} times. Manual intervention required.",
                "task_id": task_id,
            }

        # Validate task is actually in progress before marking blocked
        if task_id not in state.queue.in_progress:
            return {
                "success": False,
                "error": f"Task {task_id} is not in progress",
                "message": "Cannot mark a task as blocked if it's not currently in progress",
            }

        # Verify worker ownership - only the assigned worker can mark task as blocked
        assigned_worker = state.queue.in_progress.get(task_id)
        if assigned_worker != worker_id:
            return {
                "success": False,
                "error": f"Task {task_id} is assigned to {assigned_worker}, not {worker_id}",
                "message": "Cannot mark a task as blocked if you are not the assigned worker",
            }

        # Move task from in_progress (will be picked up after repair)
        del state.queue.in_progress[task_id]

        # Get task and release scope lock
        task = self.get_task(task_id)
        if task and task.scope:
            self.lock_store.release_scope_lock(state.project_id, task.scope)

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

    def c4_start(self) -> dict[str, Any]:
        """
        Transition from PLAN/HALTED to EXECUTE state.
        This starts the worker loop execution.
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value if state.status else "unknown"

        # Check if transition is valid
        if not self.state_machine.can_transition("c4_run"):
            return {
                "success": False,
                "error": f"Cannot start from current state: {current_status}",
                "current_status": current_status,
                "hint": "Must be in PLAN or HALTED state to start execution",
            }

        # Perform the transition
        self.state_machine.transition("c4_run")
        new_state = self.state_machine.state

        # Auto-start supervisor loop for background checkpoint processing
        supervisor_started = False
        if not self.is_supervisor_loop_running:
            try:
                self.start_supervisor_loop()
                supervisor_started = True
            except Exception as e:
                # Log but don't fail - supervisor can be started manually
                logger.warning(f"Failed to auto-start supervisor loop: {e}")

        return {
            "success": True,
            "message": f"Transitioned from {current_status} to EXECUTE",
            "status": new_state.status.value,
            "pending_tasks": len(new_state.queue.pending),
            "supervisor_loop_started": supervisor_started,
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

    # =========================================================================
    # Discovery & Specification Management
    # =========================================================================

    def c4_save_spec(
        self,
        feature: str,
        requirements: list[dict],
        domain: str,
        description: str | None = None,
    ) -> dict[str, Any]:
        """
        Save feature specification to .c4/specs/.

        Args:
            feature: Feature name (e.g., "user-auth")
            requirements: List of EARS requirements [{id, pattern, text}]
            domain: Domain name (e.g., "web-frontend")
            description: Optional feature description

        Returns:
            Dictionary with save result
        """
        try:
            # Parse domain
            domain_enum = Domain(domain)

            # Create feature spec
            spec = FeatureSpec(
                feature=feature,
                domain=domain_enum,
                description=description,
            )

            # Add requirements
            for req_data in requirements:
                req = EARSRequirement(
                    id=req_data["id"],
                    pattern=EARSPattern(req_data.get("pattern", "ubiquitous")),
                    text=req_data["text"],
                    domain=domain_enum,
                )
                spec.requirements.append(req)

            # Save to store
            spec_file = self.spec_store.save(spec)

            return {
                "success": True,
                "feature": feature,
                "domain": domain,
                "requirements_count": len(spec.requirements),
                "file_path": str(spec_file),
            }

        except ValueError as e:
            return {"success": False, "error": f"Invalid domain: {e}"}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_list_specs(self) -> dict[str, Any]:
        """
        List all feature specifications.

        Returns:
            Dictionary with feature list
        """
        try:
            features = self.spec_store.list_features()

            # Get summary for each feature
            specs_summary = []
            for feature in features:
                spec = self.spec_store.load(feature)
                if spec:
                    specs_summary.append({
                        "feature": spec.feature,
                        "domain": spec.domain.value,
                        "requirements_count": len(spec.requirements),
                        "description": spec.description,
                    })

            return {
                "success": True,
                "count": len(features),
                "features": specs_summary,
            }

        except Exception as e:
            return {"success": False, "error": str(e), "features": []}

    def c4_get_spec(self, feature: str) -> dict[str, Any]:
        """
        Get a specific feature specification.

        Args:
            feature: Feature name

        Returns:
            Dictionary with spec details
        """
        try:
            spec = self.spec_store.load(feature)
            if spec is None:
                return {"success": False, "error": f"Feature '{feature}' not found"}

            return {
                "success": True,
                "feature": spec.feature,
                "domain": spec.domain.value,
                "description": spec.description,
                "requirements": [
                    {
                        "id": req.id,
                        "pattern": req.pattern.value,
                        "text": req.text,
                    }
                    for req in spec.requirements
                ],
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_add_verification(
        self,
        feature: str,
        verification_type: str,
        name: str,
        reason: str,
        priority: int = 2,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """
        Add a verification requirement to a feature spec from conversation context.

        Use this when the user requests specific verification or when conversation
        context suggests verification needs (e.g., "성능 검증 필요해", "보안 테스트 해줘").

        Args:
            feature: Feature name (must exist)
            verification_type: One of: http, browser, cli, metrics, visual, dryrun
            name: Human-readable name for the verification
            reason: Why this verification is needed (from conversation context)
            priority: 1=critical, 2=normal, 3=optional (default: 2)
            config: Verification-specific configuration

        Returns:
            Dictionary with success status and verification details

        Example:
            c4_add_verification(
                feature="user-auth",
                verification_type="http",
                name="Login API Response Time",
                reason="User requested performance verification for login",
                priority=1,
                config={"url": "/api/login", "max_response_time": 500}
            )
        """
        # Validate verification type
        valid_types = ["http", "browser", "cli", "metrics", "visual", "dryrun"]
        if verification_type not in valid_types:
            return {
                "success": False,
                "error": f"Invalid verification type: {verification_type}",
                "valid_types": valid_types,
            }

        # Load existing spec
        spec = self.spec_store.load(feature)
        if spec is None:
            return {
                "success": False,
                "error": f"Feature '{feature}' not found",
                "hint": "Create the feature with c4_save_spec first",
            }

        try:
            # Add verification requirement
            verification = spec.add_verification(
                verification_type=verification_type,
                name=name,
                reason=reason,
                priority=priority,
                **(config or {}),
            )

            # Save updated spec
            self.spec_store.save(spec)

            # Also sync to config.yaml for runtime verification
            self._sync_verification_to_config(verification)

            return {
                "success": True,
                "feature": feature,
                "verification": {
                    "type": verification.type,
                    "name": verification.name,
                    "reason": verification.reason,
                    "priority": verification.priority,
                    "config": verification.config,
                },
                "total_verifications": len(spec.verification_requirements),
                "config_synced": True,
                "message": f"Added {verification_type} verification: {name} (synced to config.yaml)",
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_get_feature_verifications(self, feature: str) -> dict[str, Any]:
        """
        Get all verification requirements for a feature.

        Args:
            feature: Feature name

        Returns:
            Dictionary with verification requirements
        """
        spec = self.spec_store.load(feature)
        if spec is None:
            return {
                "success": False,
                "error": f"Feature '{feature}' not found",
            }

        return {
            "success": True,
            "feature": feature,
            "verifications": [
                {
                    "type": v.type,
                    "name": v.name,
                    "reason": v.reason,
                    "priority": v.priority,
                    "config": v.config,
                    "enabled": v.enabled,
                }
                for v in spec.verification_requirements
            ],
            "config_format": spec.get_verifications_for_config(),
        }

    def c4_discovery_complete(self) -> dict[str, Any]:
        """
        Mark discovery phase as complete, transition to DESIGN.

        Returns:
            Dictionary with transition result
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value

        # Verify we're in DISCOVERY state
        if state.status != ProjectStatus.DISCOVERY:
            return {
                "success": False,
                "error": f"Not in DISCOVERY state (current: {current_status})",
                "hint": "c4_discovery_complete can only be called from DISCOVERY state",
            }

        # Check if any specs have been created
        specs = self.spec_store.list_features()
        if not specs:
            return {
                "success": False,
                "error": "No specifications found",
                "hint": "Create at least one specification with c4_save_spec before completing discovery",
            }

        try:
            # Collect unique domains from all specs and apply default verifications
            domains_found: set[str] = set()
            default_verifications_added: list[str] = []

            for spec_name in specs:
                spec = self.spec_store.load(spec_name)
                if spec and spec.domain:
                    domain_value = spec.domain.value if hasattr(spec.domain, "value") else str(spec.domain)
                    domains_found.add(domain_value)

            # Apply domain defaults for each unique domain
            for domain in domains_found:
                added = self._apply_domain_default_verifications(domain)
                default_verifications_added.extend(added)

            # Also set domain in config if single domain project
            if len(domains_found) == 1:
                self._config.domain = list(domains_found)[0]
                self._save_config()

            # Transition to DESIGN
            self.state_machine.transition("discovery_complete")

            result = {
                "success": True,
                "previous_status": current_status,
                "new_status": self.state_machine.state.status.value,
                "specs_count": len(specs),
                "domains": list(domains_found),
                "message": "Discovery phase complete. Ready for design review.",
            }

            if default_verifications_added:
                result["default_verifications_added"] = default_verifications_added
                result["message"] += f" Added {len(default_verifications_added)} domain default verification(s)."

            return result

        except StateTransitionError as e:
            return {"success": False, "error": str(e)}

    # =========================================================================
    # Design Phase Tools
    # =========================================================================

    @property
    def design_store(self) -> "DesignStore":
        """Get or create design store instance."""
        if not hasattr(self, "_design_store"):
            from c4.discovery.design import DesignStore

            self._design_store = DesignStore(self.c4_dir / "specs")
        return self._design_store

    def c4_save_design(
        self,
        feature: str,
        domain: str,
        selected_option: str | None = None,
        options: list[dict] | None = None,
        components: list[dict] | None = None,
        decisions: list[dict] | None = None,
        mermaid_diagram: str | None = None,
        constraints: list[str] | None = None,
        nfr: dict[str, str] | None = None,
        description: str | None = None,
    ) -> dict[str, Any]:
        """
        Save design specification for a feature.

        Args:
            feature: Feature name (must match an existing spec)
            domain: Domain name (e.g., "web-frontend")
            selected_option: ID of the selected architecture option
            options: List of architecture options [{id, name, description, complexity, pros, cons, recommended}]
            components: List of components [{name, type, description, responsibilities, dependencies}]
            decisions: List of design decisions [{id, question, decision, rationale, alternatives_considered}]
            mermaid_diagram: Mermaid diagram string
            constraints: List of technical constraints
            nfr: Non-functional requirements dict

        Returns:
            Dictionary with save result
        """
        try:
            from c4.discovery.design import (
                ArchitectureOption,
                ComponentDesign,
                DesignSpec,
            )
            from c4.discovery.models import Domain as DomainEnum

            # Parse domain
            domain_enum = DomainEnum(domain)

            # Create design spec
            spec = DesignSpec(
                feature=feature,
                domain=domain_enum,
                description=description,
                selected_option=selected_option,
                mermaid_diagram=mermaid_diagram,
                constraints=constraints or [],
                nfr=nfr or {},
            )

            # Add architecture options
            if options:
                for opt_data in options:
                    opt = ArchitectureOption(
                        id=opt_data.get("id", f"option-{len(spec.architecture_options)+1}"),
                        name=opt_data.get("name", "Unnamed Option"),
                        description=opt_data.get("description", ""),
                        complexity=opt_data.get("complexity", "medium"),
                        pros=opt_data.get("pros", []),
                        cons=opt_data.get("cons", []),
                        recommended=opt_data.get("recommended", False),
                    )
                    spec.add_option(opt)

            # Add components
            if components:
                for comp_data in components:
                    comp = ComponentDesign(
                        name=comp_data.get("name", ""),
                        type=comp_data.get("type", "component"),
                        description=comp_data.get("description", ""),
                        responsibilities=comp_data.get("responsibilities", []),
                        dependencies=comp_data.get("dependencies", []),
                        interfaces=comp_data.get("interfaces", []),
                    )
                    spec.add_component(comp)

            # Add decisions
            if decisions:
                for dec_data in decisions:
                    spec.add_decision(
                        id=dec_data.get("id", f"DEC-{len(spec.decisions)+1}"),
                        question=dec_data.get("question", ""),
                        decision=dec_data.get("decision", ""),
                        rationale=dec_data.get("rationale", ""),
                        alternatives=dec_data.get("alternatives_considered", []),
                    )

            # Save design
            yaml_path, md_path = self.design_store.save(spec)

            return {
                "success": True,
                "feature": feature,
                "domain": domain,
                "yaml_path": str(yaml_path),
                "md_path": str(md_path),
                "options_count": len(spec.architecture_options),
                "components_count": len(spec.components),
                "decisions_count": len(spec.decisions),
            }

        except ValueError as e:
            return {"success": False, "error": f"Invalid domain: {e}"}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_get_design(self, feature: str) -> dict[str, Any]:
        """
        Get design specification for a feature.

        Args:
            feature: Feature name to retrieve

        Returns:
            Dictionary with design details or error
        """
        try:
            spec = self.design_store.load(feature)

            if spec is None:
                return {
                    "success": False,
                    "error": f"Design not found for feature: {feature}",
                    "hint": "Create a design with c4_save_design first",
                }

            return {
                "success": True,
                "feature": spec.feature,
                "domain": spec.domain.value,
                "description": spec.description,
                "selected_option": spec.selected_option,
                "options": [
                    {
                        "id": opt.id,
                        "name": opt.name,
                        "description": opt.description,
                        "complexity": opt.complexity,
                        "pros": opt.pros,
                        "cons": opt.cons,
                        "recommended": opt.recommended,
                    }
                    for opt in spec.architecture_options
                ],
                "components": [
                    {
                        "name": comp.name,
                        "type": comp.type,
                        "description": comp.description,
                        "responsibilities": comp.responsibilities,
                        "dependencies": comp.dependencies,
                    }
                    for comp in spec.components
                ],
                "decisions": [
                    {
                        "id": dec.id,
                        "question": dec.question,
                        "decision": dec.decision,
                        "rationale": dec.rationale,
                    }
                    for dec in spec.decisions
                ],
                "mermaid_diagram": spec.mermaid_diagram,
                "constraints": spec.constraints,
                "nfr": spec.nfr,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_list_designs(self) -> dict[str, Any]:
        """
        List all features with design specifications.

        Returns:
            Dictionary with list of features
        """
        try:
            features = self.design_store.list_features_with_design()
            designs = []

            for feature_name in features:
                spec = self.design_store.load(feature_name)
                if spec:
                    designs.append({
                        "feature": spec.feature,
                        "domain": spec.domain.value,
                        "selected_option": spec.selected_option,
                        "has_diagram": spec.mermaid_diagram is not None,
                        "components_count": len(spec.components),
                        "decisions_count": len(spec.decisions),
                    })

            return {
                "success": True,
                "count": len(designs),
                "designs": designs,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_design_complete(self) -> dict[str, Any]:
        """
        Mark design phase as complete, transition to PLAN.

        Returns:
            Dictionary with transition result
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value

        # Verify we're in DESIGN state
        if state.status != ProjectStatus.DESIGN:
            return {
                "success": False,
                "error": f"Not in DESIGN state (current: {current_status})",
                "hint": "c4_design_complete can only be called from DESIGN state",
            }

        # Check if any designs have been created
        designs = self.design_store.list_features_with_design()
        if not designs:
            return {
                "success": False,
                "error": "No design specifications found",
                "hint": "Create at least one design with c4_save_design before completing design phase",
            }

        # Check if designs have selected options
        incomplete_designs = []
        for feature_name in designs:
            spec = self.design_store.load(feature_name)
            if spec and spec.architecture_options and not spec.selected_option:
                incomplete_designs.append(feature_name)

        if incomplete_designs:
            return {
                "success": False,
                "error": f"Designs without selected option: {incomplete_designs}",
                "hint": "Select an architecture option for each design before completing",
            }

        try:
            # Transition to PLAN
            self.state_machine.transition("design_approved")

            return {
                "success": True,
                "previous_status": current_status,
                "new_status": self.state_machine.state.status.value,
                "designs_count": len(designs),
                "message": "Design phase complete. Ready for planning.",
            }

        except StateTransitionError as e:
            return {"success": False, "error": str(e)}

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
                        "worker_id": {
                            "type": "string",
                            "description": "Worker ID submitting the task (for ownership verification)",
                        },
                    },
                    "required": ["task_id", "commit_sha", "validation_results"],
                },
            ),
            Tool(
                name="c4_add_todo",
                description="Add a new task to the queue with optional dependencies",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "task_id": {"type": "string", "description": "Unique task ID (e.g., T-001)"},
                        "title": {"type": "string", "description": "Task title"},
                        "scope": {"type": "string", "description": "File/directory scope for lock"},
                        "dod": {"type": "string", "description": "Definition of Done"},
                        "dependencies": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Task IDs that must complete first (e.g., ['T-001', 'T-002'])",
                        },
                        "domain": {
                            "type": "string",
                            "description": "Domain for agent routing (web-frontend, web-backend, etc.)",
                        },
                        "priority": {
                            "type": "integer",
                            "description": "Higher priority tasks assigned first (default: 0)",
                        },
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
                name="c4_start",
                description="Start execution by transitioning from PLAN/HALTED to EXECUTE state",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
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
            # Discovery & Specification Tools
            Tool(
                name="c4_save_spec",
                description="Save feature specification to .c4/specs/. Used during discovery phase to persist EARS requirements.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "feature": {
                            "type": "string",
                            "description": "Feature name (e.g., 'user-auth', 'dashboard')",
                        },
                        "requirements": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "id": {"type": "string", "description": "Requirement ID (e.g., 'REQ-001')"},
                                    "pattern": {
                                        "type": "string",
                                        "enum": ["ubiquitous", "state-driven", "event-driven", "optional", "unwanted"],
                                        "description": "EARS pattern type",
                                    },
                                    "text": {"type": "string", "description": "Full EARS requirement text"},
                                },
                                "required": ["id", "text"],
                            },
                            "description": "List of EARS requirements",
                        },
                        "domain": {
                            "type": "string",
                            "enum": ["web-frontend", "web-backend", "fullstack", "ml-dl", "mobile-app", "infra", "library", "unknown"],
                            "description": "Project domain",
                        },
                        "description": {
                            "type": "string",
                            "description": "Optional feature description",
                        },
                    },
                    "required": ["feature", "requirements", "domain"],
                },
            ),
            Tool(
                name="c4_list_specs",
                description="List all feature specifications in .c4/specs/",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
                },
            ),
            Tool(
                name="c4_get_spec",
                description="Get a specific feature specification by name",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "feature": {
                            "type": "string",
                            "description": "Feature name to retrieve",
                        },
                    },
                    "required": ["feature"],
                },
            ),
            Tool(
                name="c4_discovery_complete",
                description="Mark discovery phase as complete and transition to DESIGN state. Requires at least one specification to be saved.",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
                },
            ),
            # Design Phase Tools
            Tool(
                name="c4_save_design",
                description="Save design specification for a feature including architecture options, components, and decisions.",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "feature": {
                            "type": "string",
                            "description": "Feature name (e.g., 'user-auth')",
                        },
                        "domain": {
                            "type": "string",
                            "enum": ["web-frontend", "web-backend", "fullstack", "ml-dl", "mobile-app", "infra", "library", "unknown"],
                            "description": "Project domain",
                        },
                        "description": {
                            "type": "string",
                            "description": "Optional feature description",
                        },
                        "selected_option": {
                            "type": "string",
                            "description": "ID of selected architecture option",
                        },
                        "options": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "id": {"type": "string"},
                                    "name": {"type": "string"},
                                    "description": {"type": "string"},
                                    "complexity": {"type": "string", "enum": ["low", "medium", "high"]},
                                    "pros": {"type": "array", "items": {"type": "string"}},
                                    "cons": {"type": "array", "items": {"type": "string"}},
                                    "recommended": {"type": "boolean"},
                                },
                                "required": ["id", "name", "description"],
                            },
                            "description": "Architecture options",
                        },
                        "components": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "name": {"type": "string"},
                                    "type": {"type": "string"},
                                    "description": {"type": "string"},
                                    "responsibilities": {"type": "array", "items": {"type": "string"}},
                                    "dependencies": {"type": "array", "items": {"type": "string"}},
                                    "interfaces": {"type": "array", "items": {"type": "string"}},
                                },
                                "required": ["name", "type", "description"],
                            },
                            "description": "Component designs",
                        },
                        "decisions": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "id": {"type": "string"},
                                    "question": {"type": "string"},
                                    "decision": {"type": "string"},
                                    "rationale": {"type": "string"},
                                    "alternatives_considered": {"type": "array", "items": {"type": "string"}},
                                },
                                "required": ["id", "question", "decision", "rationale"],
                            },
                            "description": "Design decisions",
                        },
                        "mermaid_diagram": {
                            "type": "string",
                            "description": "Mermaid diagram source",
                        },
                        "constraints": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Technical constraints",
                        },
                        "nfr": {
                            "type": "object",
                            "additionalProperties": {"type": "string"},
                            "description": "Non-functional requirements (e.g., {'latency': '<500ms'})",
                        },
                    },
                    "required": ["feature", "domain"],
                },
            ),
            Tool(
                name="c4_get_design",
                description="Get design specification for a feature",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "feature": {
                            "type": "string",
                            "description": "Feature name to retrieve",
                        },
                    },
                    "required": ["feature"],
                },
            ),
            Tool(
                name="c4_list_designs",
                description="List all features with design specifications",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
                },
            ),
            Tool(
                name="c4_design_complete",
                description="Mark design phase as complete and transition to PLAN state. Requires at least one design with selected option.",
                inputSchema={
                    "type": "object",
                    "properties": {},
                    "required": [],
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
                    import os
                    import shutil

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

                    # Clear daemon cache (all projects - simpler and safer)
                    cache_cleared = clear_daemon_cache()

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
                    arguments.get("worker_id"),  # Optional worker_id for ownership verification
                )
                result = result.model_dump()
            elif name == "c4_add_todo":
                result = daemon.c4_add_todo(
                    task_id=arguments["task_id"],
                    title=arguments["title"],
                    scope=arguments.get("scope"),
                    dod=arguments["dod"],
                    dependencies=arguments.get("dependencies"),
                    domain=arguments.get("domain"),
                    priority=arguments.get("priority", 0),
                )
            elif name == "c4_checkpoint":
                result = daemon.c4_checkpoint(
                    arguments["checkpoint_id"],
                    arguments["decision"],
                    arguments["notes"],
                    arguments.get("required_changes"),
                )
                result = result.model_dump()
            elif name == "c4_start":
                result = daemon.c4_start()
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
            # Discovery & Specification Tools
            elif name == "c4_save_spec":
                result = daemon.c4_save_spec(
                    feature=arguments["feature"],
                    requirements=arguments["requirements"],
                    domain=arguments["domain"],
                    description=arguments.get("description"),
                )
            elif name == "c4_list_specs":
                result = daemon.c4_list_specs()
            elif name == "c4_get_spec":
                result = daemon.c4_get_spec(arguments["feature"])
            elif name == "c4_discovery_complete":
                result = daemon.c4_discovery_complete()
            # Design Phase Tools
            elif name == "c4_save_design":
                result = daemon.c4_save_design(
                    feature=arguments["feature"],
                    domain=arguments["domain"],
                    selected_option=arguments.get("selected_option"),
                    options=arguments.get("options"),
                    components=arguments.get("components"),
                    decisions=arguments.get("decisions"),
                    mermaid_diagram=arguments.get("mermaid_diagram"),
                    constraints=arguments.get("constraints"),
                    nfr=arguments.get("nfr"),
                    description=arguments.get("description"),
                )
            elif name == "c4_get_design":
                result = daemon.c4_get_design(arguments["feature"])
            elif name == "c4_list_designs":
                result = daemon.c4_list_designs()
            elif name == "c4_design_complete":
                result = daemon.c4_design_complete()
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
