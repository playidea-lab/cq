"""Task operations for C4 Daemon.

This module contains task CRUD operations extracted from C4Daemon:
- c4_get_task: Request next task assignment for a worker
- c4_submit: Report task completion with validation results
- c4_add_todo: Add a new task with optional dependencies
- c4_mark_blocked: Mark a task as blocked after max retry attempts

These operations are delegated from C4Daemon for modularity.
"""

import logging
from datetime import datetime
from typing import TYPE_CHECKING, Any

from ..constants import MAX_REPAIR_DEPTH, REPAIR_PREFIX, REPAIR_PREFIX_LEN
from ..models import (
    CheckpointQueueItem,
    EventType,
    RepairQueueItem,
    SubmitResponse,
    Task,
    TaskAssignment,
    TaskStatus,
    TaskType,
    ValidationResult,
)
from .git_ops import GitOperations

if TYPE_CHECKING:
    from .c4_daemon import C4Daemon

logger = logging.getLogger(__name__)


class TaskOps:
    """Task operations handler for C4 Daemon.

    Provides task CRUD operations with proper state management,
    scope locking, and event emission.
    """

    def __init__(self, daemon: "C4Daemon"):
        """Initialize TaskOps with parent daemon reference.

        Args:
            daemon: Parent C4Daemon instance for state and config access
        """
        self._daemon = daemon

    # =========================================================================
    # Task Persistence
    # =========================================================================

    def load_tasks(self) -> None:
        """Load tasks from SQLite (with migration from tasks.json if needed)."""
        project_id = self._daemon.state_machine.state.project_id

        # Migrate from tasks.json if SQLite is empty but tasks.json exists
        tasks_file = self._daemon.c4_dir / "tasks.json"
        if not self._daemon.task_store.exists(project_id) and tasks_file.exists():
            count = self._daemon.task_store.migrate_from_json(project_id, tasks_file)
            if count > 0:
                # Backup original tasks.json
                backup_path = self._daemon.c4_dir / "tasks.json.bak"
                if not backup_path.exists():
                    tasks_file.rename(backup_path)

        # Load from SQLite
        self._daemon._tasks = self._daemon.task_store.load_all(project_id)

    def save_tasks(self) -> None:
        """Save all tasks to SQLite."""
        if self._daemon.state_machine is None:
            return
        project_id = self._daemon.state_machine.state.project_id
        self._daemon.task_store.save_all(project_id, self._daemon._tasks)

    def save_task(self, task: Task) -> None:
        """Save a single task to SQLite (more efficient than save_all).

        Args:
            task: Task to save
        """
        if self._daemon.state_machine is None:
            return
        project_id = self._daemon.state_machine.state.project_id
        self._daemon.task_store.save(project_id, task)
        # Also update in-memory cache
        self._daemon._tasks[task.id] = task

    # =========================================================================
    # c4_get_task
    # =========================================================================

    def get_task(
        self, worker_id: str, model_filter: str | None = None
    ) -> TaskAssignment | None:
        """Request next task assignment for a worker.

        Args:
            worker_id: Unique worker identifier
            model_filter: Only return tasks with this model (sonnet, opus, haiku).
                If None, returns any available task (default behavior).

        Returns:
            TaskAssignment with task details, or None if no tasks available
        """
        daemon = self._daemon
        if daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Auto-ensure supervisor loop is running for AI review
        daemon._ensure_supervisor_running()

        # Implicit heartbeat - keep worker marked as active
        daemon._touch_worker(worker_id)

        # Re-load state to get latest (prevent race conditions with other workers)
        daemon.state_machine.load_state()
        # Also refresh task cache from SQLite (fixes stale cache after direct DB edits)
        self.load_tasks()
        state = daemon.state_machine.state

        # Sync tasks whose branches have been merged (fixes Git-C4 state sync)
        daemon._sync_merged_tasks()

        # Clean up expired scope locks (prevents stale locks from blocking task assignment)
        daemon.lock_store.cleanup_expired(state.project_id)

        # Ensure we're in EXECUTE state
        from ..models import ProjectStatus
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Register worker if not exists
        if not daemon.worker_manager.is_registered(worker_id):
            daemon.worker_manager.register(worker_id)

        # Check if worker already has an in_progress task (resume after crash/restart)
        resumed_task = self._try_resume_task(worker_id, state)
        if resumed_task is not None:
            return resumed_task

        # Find available task from pending (sorted by priority, highest first)
        return self._find_and_assign_task(worker_id, state, model_filter)

    def _try_resume_task(self, worker_id: str, state: Any) -> TaskAssignment | None:
        """Try to resume an in_progress task for a worker.

        Returns:
            TaskAssignment if task was resumed, None otherwise
        """
        daemon = self._daemon

        for task_id, assigned_worker in list(state.queue.in_progress.items()):
            if assigned_worker != worker_id:
                continue

            task = daemon.get_task(task_id)
            if not task:
                continue

            # Verify scope lock is still valid for this worker
            if task.scope:
                lock_owner = daemon.lock_store.get_lock_owner(
                    state.project_id, task.scope
                )
                if lock_owner is None or lock_owner != worker_id:
                    # Lock expired or taken by another worker - cannot resume
                    self._release_task_to_pending(task_id, task, state)
                    return None
            else:
                # For scope=None tasks, verify task state consistency
                if task.assigned_to != worker_id or task.status != TaskStatus.IN_PROGRESS:
                    self._release_task_to_pending(task_id, task, state)
                    continue

            # Re-sync worker state
            daemon.worker_manager.set_busy(
                worker_id, task_id, task.scope, task.branch
            )

            # Refresh lock TTL for resumed work (with result check)
            if task.scope:
                if not daemon.lock_store.refresh_scope_lock(
                    state.project_id, task.scope, worker_id, daemon.config.scope_lock_ttl_sec
                ):
                    self._release_task_to_pending(task_id, task, state)
                    continue

            # Get agent routing info
            agent_routing = daemon._get_agent_routing(task)

            # Check for existing worktree for resumed task
            worktree_path = self._get_or_create_worktree(worker_id, task)

            return TaskAssignment(
                task_id=task_id,
                title=task.title,
                scope=task.scope,
                dod=task.dod,
                validations=task.validations,
                branch=task.branch or "",
                worktree_path=worktree_path,
                model=task.model,
                **agent_routing,
            )

        return None

    def _release_task_to_pending(self, task_id: str, task: Task, state: Any) -> None:
        """Release a task back to pending for reassignment."""
        daemon = self._daemon

        del state.queue.in_progress[task_id]
        state.queue.pending.insert(0, task_id)
        task.status = TaskStatus.PENDING
        task.assigned_to = None
        self.save_task(task)

        if task.scope:
            daemon.lock_store.release_scope_lock(state.project_id, task.scope)

        daemon.state_machine.save_state()

    def _get_or_create_worktree(
        self, worker_id: str, task: Task
    ) -> str | None:
        """Get or create worktree for a task.

        Returns:
            Worktree path string or None
        """
        daemon = self._daemon
        git_ops = GitOperations(daemon.root)

        if not git_ops.is_git_repo() or not daemon.config.worktree.enabled:
            return None

        wt_path = git_ops.get_worktree_path(worker_id)
        if wt_path.exists():
            return str(wt_path)

        # Try to create worktree for resumed task
        worktree_result = git_ops.create_worktree(
            worker_id=worker_id,
            branch=task.branch or "",
        )
        if worktree_result.success:
            return str(wt_path)

        return None

    def _find_and_assign_task(
        self, worker_id: str, state: Any, model_filter: str | None
    ) -> TaskAssignment | None:
        """Find and assign an available task to a worker.

        Args:
            worker_id: Worker to assign task to
            state: Current C4 state
            model_filter: Optional model filter

        Returns:
            TaskAssignment if task was assigned, None otherwise
        """
        daemon = self._daemon
        project_id = state.project_id
        ttl = daemon.config.scope_lock_ttl_sec

        # Get all pending tasks and sort by priority (descending)
        pending_tasks = []
        for task_id in state.queue.pending:
            task = daemon.get_task(task_id)
            if task:
                pending_tasks.append(task)
        pending_tasks.sort(key=lambda t: t.priority, reverse=True)

        for task in pending_tasks:
            task_id = task.id

            # Economic mode: filter by model if specified
            if model_filter and task.model != model_filter:
                continue

            # Check dependencies first (non-locking check)
            deps_met = all(
                dep_id in state.queue.done for dep_id in task.dependencies
            )
            if not deps_met:
                continue

            # Peer Review: exclude original worker from repair tasks
            original_worker = daemon._get_original_worker_for_repair(task_id)
            if original_worker and original_worker == worker_id:
                continue

            # Try to acquire scope lock ATOMICALLY using SQLite
            if task.scope:
                lock_acquired = daemon.lock_store.acquire_scope_lock(
                    project_id, task.scope, worker_id, ttl
                )
                if not lock_acquired:
                    continue

            # Assign the task
            assignment = self._assign_task(worker_id, task, state)
            if assignment:
                return assignment
            else:
                # Release lock if assignment failed
                if task.scope:
                    daemon.lock_store.release_scope_lock(project_id, task.scope)

        return None

    def _assign_task(
        self, worker_id: str, task: Task, state: Any
    ) -> TaskAssignment | None:
        """Assign a specific task to a worker.

        Args:
            worker_id: Worker to assign task to
            task: Task to assign
            state: Current C4 state

        Returns:
            TaskAssignment if successful, None otherwise
        """
        daemon = self._daemon
        task_id = task.id
        project_id = state.project_id
        store = daemon.state_machine.store

        # Determine branch: Review tasks use parent's branch
        if task.type == TaskType.REVIEW and task.parent_id:
            parent_task = daemon.get_task(task.parent_id)
            if parent_task and parent_task.branch:
                task_branch = parent_task.branch
                is_review_using_parent_branch = True
            else:
                task_branch = f"{daemon.config.work_branch_prefix}{task.parent_id}"
                is_review_using_parent_branch = True
        else:
            task_branch = f"{daemon.config.work_branch_prefix}{task_id}"
            is_review_using_parent_branch = False

        assigned = False

        with store.atomic_modify(project_id) as mod_state:
            # Double-check task is still pending
            if task_id in mod_state.queue.pending:
                # Assign task (ATOMIC)
                mod_state.queue.pending.remove(task_id)
                mod_state.queue.in_progress[task_id] = worker_id

                # Ensure worker exists in state
                self._ensure_worker_in_state(mod_state, worker_id, task_id, task, task_branch)
                assigned = True

            # Update cached state in state_machine
            daemon.state_machine._state = mod_state

        if not assigned:
            return None

        # Update task in SQLite
        task.status = TaskStatus.IN_PROGRESS
        task.assigned_to = worker_id
        task.branch = task_branch
        self.save_task(task)

        # Create worktree for isolated multi-worker support
        worktree_path = self._create_worktree_for_task(
            worker_id, task, task_branch, is_review_using_parent_branch
        )

        # Emit event
        daemon.state_machine.emit_event(
            EventType.TASK_ASSIGNED,
            "c4d",
            {
                "task_id": task_id,
                "worker_id": worker_id,
                "scope": task.scope,
                "worktree_path": worktree_path,
            },
        )

        # Get agent routing info
        agent_routing = daemon._get_agent_routing(task)

        return TaskAssignment(
            task_id=task_id,
            title=task.title,
            scope=task.scope,
            dod=task.dod,
            validations=task.validations,
            branch=task_branch,
            worktree_path=worktree_path,
            model=task.model,
            **agent_routing,
        )

    def _ensure_worker_in_state(
        self, state: Any, worker_id: str, task_id: str, task: Task, task_branch: str
    ) -> None:
        """Ensure worker exists in state with correct info."""
        from ..models import WorkerInfo

        if worker_id not in state.workers:
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
            state.workers[worker_id].state = "busy"
            state.workers[worker_id].task_id = task_id
            state.workers[worker_id].scope = task.scope
            state.workers[worker_id].branch = task_branch
            state.workers[worker_id].last_seen = datetime.now()

    def _create_worktree_for_task(
        self,
        worker_id: str,
        task: Task,
        task_branch: str,
        is_review_using_parent_branch: bool,
    ) -> str | None:
        """Create worktree for task if enabled.

        Returns:
            Worktree path string or None
        """
        daemon = self._daemon
        git_ops = GitOperations(daemon.root)

        if not git_ops.is_git_repo() or not daemon.config.worktree.enabled:
            return None

        work_branch = daemon.config.get_work_branch()

        if not is_review_using_parent_branch:
            # Create worktree with new branch from work_branch
            worktree_result = git_ops.create_worktree(
                worker_id=worker_id,
                branch=task_branch,
                base_branch=work_branch,
            )
            if worktree_result.success:
                wt_path = git_ops.get_worktree_path(worker_id)
                logger.info(f"Created worktree for {worker_id} at {wt_path}")
                return str(wt_path)
            else:
                # Fallback: create branch only (legacy behavior)
                logger.warning(
                    f"Worktree creation failed for {worker_id}: "
                    f"{worktree_result.message}. Using branch only."
                )
                branch_result = daemon._create_task_branch_from_work(
                    git_ops, task_branch, work_branch
                )
                if not branch_result.success:
                    logger.warning(
                        f"Failed to create task branch {task_branch}: "
                        f"{branch_result.message}"
                    )
        else:
            # Review task: reuse worker's existing worktree if exists
            wt_path = git_ops.get_worktree_path(worker_id)
            if wt_path.exists():
                logger.info(f"Review task {task.id} using worktree {wt_path}")
                return str(wt_path)
            else:
                logger.info(
                    f"Review task {task.id} using parent branch "
                    f"{task_branch} (no worktree)"
                )

        return None

    # =========================================================================
    # c4_submit
    # =========================================================================

    def submit(
        self,
        task_id: str,
        commit_sha: str,
        validation_results: list[dict],
        worker_id: str | None = None,
        review_result: str | None = None,
        review_comments: str | None = None,
    ) -> SubmitResponse:
        """Report task completion with validation results.

        For review tasks (TaskType.REVIEW), additional parameters:
        - review_result: "APPROVE" or "REQUEST_CHANGES"
        - review_comments: Comments for REQUEST_CHANGES (becomes DoD for next version)

        Args:
            task_id: ID of the completed task
            commit_sha: Git commit SHA of the work
            validation_results: List of validation result dicts
            worker_id: Worker ID submitting the task
            review_result: Review decision for review tasks
            review_comments: Comments for review tasks

        Returns:
            SubmitResponse with success status and next action
        """
        daemon = self._daemon
        if daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Auto-ensure supervisor loop is running for AI review
        daemon._ensure_supervisor_running()

        # Implicit heartbeat - keep worker marked as active
        daemon._touch_worker(worker_id)

        # Parse and validate results first
        results = [ValidationResult.model_validate(r) for r in validation_results]
        all_passed = all(r.status == "pass" for r in results)
        if not all_passed:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message="Some validations failed",
            )

        # Get task info
        task = daemon.get_task(task_id)
        project_id = daemon.state_machine.state.project_id

        # Perform atomic state modification
        error_response, actual_worker_id = self._atomic_submit(
            task_id, worker_id, results, project_id
        )

        if error_response:
            return error_response

        # Non-atomic operations after successful state update
        return self._complete_submit(
            task_id, task, commit_sha, results, actual_worker_id,
            review_result, review_comments, project_id
        )

    def _atomic_submit(
        self,
        task_id: str,
        worker_id: str | None,
        results: list[ValidationResult],
        project_id: str,
    ) -> tuple[SubmitResponse | None, str | None]:
        """Perform atomic state modification for submit.

        Returns:
            Tuple of (error_response, actual_worker_id)
        """
        daemon = self._daemon
        store = daemon.state_machine.store

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

                # Move to done (ATOMIC)
                del state.queue.in_progress[task_id]
                state.queue.done.append(task_id)

                # Update worker state
                self._update_worker_state_on_complete(state, actual_worker_id)

                # Update metrics
                state.metrics.tasks_completed += 1

                # Update last validation
                state.last_validation = {r.name: r.status for r in results}

            # Update cached state in state_machine
            daemon.state_machine._state = state

        return error_response, actual_worker_id

    def _update_worker_state_on_complete(self, state: Any, worker_id: str) -> None:
        """Update worker state after task completion."""
        from ..models import WorkerInfo

        if worker_id in state.workers:
            state.workers[worker_id].state = "idle"
            state.workers[worker_id].task_id = None
            state.workers[worker_id].scope = None
            state.workers[worker_id].last_seen = datetime.now()
        else:
            state.workers[worker_id] = WorkerInfo(
                worker_id=worker_id,
                state="idle",
                joined_at=datetime.now(),
                last_seen=datetime.now(),
            )

    def _complete_submit(
        self,
        task_id: str,
        task: Task | None,
        commit_sha: str,
        results: list[ValidationResult],
        actual_worker_id: str | None,
        review_result: str | None,
        review_comments: str | None,
        project_id: str,
    ) -> SubmitResponse:
        """Complete submit with non-atomic operations.

        Returns:
            SubmitResponse with next action
        """
        daemon = self._daemon

        # Update task status in SQLite
        daemon.task_store.update_status(
            project_id,
            task_id,
            status="done",
            commit_sha=commit_sha,
        )

        # Plan file sync
        daemon._update_plan_task_status(task_id, "done")

        # Invalidate task cache
        if task_id in daemon._tasks:
            del daemon._tasks[task_id]

        # Release scope lock
        if task and task.scope:
            daemon.lock_store.release_scope_lock(project_id, task.scope)

        # Emit event
        daemon.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            actual_worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )

        # Send notification
        from ..notification import NotificationManager
        task_title = task.title if task else task_id
        NotificationManager.notify(
            title="C4 Task Complete",
            message=f"{task_id}: {task_title}",
            urgency="normal",
        )

        # GitHub Auto-Commit
        if (
            daemon.config.github.enabled
            and daemon.config.github.auto_commit
            and task
            and task.type == TaskType.IMPLEMENTATION
        ):
            daemon._trigger_auto_commit(task_id, task.title, actual_worker_id)

        # Review-as-Task handling
        if daemon.config.review_as_task and task:
            if task.type == TaskType.IMPLEMENTATION:
                daemon._generate_review_task(task, actual_worker_id)
            elif task.type == TaskType.REVIEW:
                review_response = daemon._handle_review_completion(
                    task, review_result, review_comments, actual_worker_id
                )
                if review_response:
                    return review_response

        # Checkpoint-as-Task handling
        if daemon.config.checkpoint_as_task and task and task.type == TaskType.CHECKPOINT:
            cp_response = daemon._handle_checkpoint_completion(
                task, review_result, review_comments, actual_worker_id
            )
            if cp_response:
                return cp_response

        # Auto-cleanup worktree
        if daemon.config.worktree.enabled and daemon.config.worktree.auto_cleanup:
            if actual_worker_id:
                self._cleanup_worktree(actual_worker_id, task_id)

        # Check checkpoint or completion
        return self._check_completion_state(results)

    def _cleanup_worktree(self, worker_id: str, task_id: str) -> None:
        """Clean up worktree after task completion."""
        daemon = self._daemon
        git_ops = GitOperations(daemon.root)

        if git_ops.is_git_repo():
            cleanup_result = git_ops.remove_worktree(worker_id)
            if cleanup_result.success:
                logger.info(
                    f"Auto-cleaned worktree for {worker_id} after task {task_id}"
                )
            else:
                logger.warning(
                    f"Failed to cleanup worktree for {worker_id}: "
                    f"{cleanup_result.message}"
                )

    def _check_completion_state(
        self, results: list[ValidationResult]
    ) -> SubmitResponse:
        """Check if checkpoint reached or all done.

        Returns:
            Appropriate SubmitResponse
        """
        daemon = self._daemon
        state = daemon.state_machine.state

        # Check if checkpoint reached
        cp_id = daemon.state_machine.check_gate_conditions(daemon.config)
        if cp_id:
            self._add_to_checkpoint_queue(cp_id, results)
            daemon.state_machine.enter_checkpoint(cp_id)
            return SubmitResponse(
                success=True,
                next_action="await_checkpoint",
                message=f"Checkpoint {cp_id} queued for AI review (automatic)",
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
        """Add checkpoint to queue for async supervisor processing."""
        daemon = self._daemon
        if daemon.state_machine is None:
            return

        state = daemon.state_machine.state

        # Check if already in queue
        if any(item.checkpoint_id == checkpoint_id for item in state.checkpoint_queue):
            return

        item = CheckpointQueueItem(
            checkpoint_id=checkpoint_id,
            triggered_at=datetime.now().isoformat(),
            tasks_completed=list(state.queue.done),
            validation_results=validation_results,
        )
        state.checkpoint_queue.append(item)
        daemon.state_machine.save_state()

    # =========================================================================
    # c4_add_todo
    # =========================================================================

    def add_todo(
        self,
        task_id: str,
        title: str,
        scope: str | None,
        dod: str,
        dependencies: list[str] | None = None,
        domain: str | None = None,
        priority: int = 0,
        model: str = "opus",
    ) -> dict[str, Any]:
        """Add a new task with optional dependencies.

        Supports versioned task IDs for Review-as-Task workflow:
        - T-001 -> T-001-0 (auto-append version 0)
        - T-001-0 -> T-001-0 (keep as-is)
        - R-001-0 -> R-001-0 (review tasks)

        Args:
            task_id: Unique task identifier (e.g., "T-001" or "T-001-0")
            title: Task title
            scope: File/directory scope for lock (e.g., "src/auth/")
            dod: Definition of Done
            dependencies: List of task IDs that must complete first
            domain: Domain for agent routing (e.g., "web-frontend")
            priority: Higher priority tasks are assigned first (default: 0)
            model: Claude model for this task (sonnet, opus, haiku). Default: opus

        Returns:
            Dict with success status and task info
        """
        daemon = self._daemon
        if daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Parse and normalize task ID for Review-as-Task
        normalized_id, base_id, version, task_type = daemon._parse_task_id(task_id)

        # Normalize dependency IDs as well
        normalized_deps: list[str] = []
        if dependencies:
            for dep_id in dependencies:
                norm_dep_id, _, _, _ = daemon._parse_task_id(dep_id)
                normalized_deps.append(norm_dep_id)

        task = Task(
            id=normalized_id,
            title=title,
            scope=scope,
            dod=dod,
            dependencies=normalized_deps,
            domain=domain,
            priority=priority,
            model=model,
            # Review-as-Task fields
            type=task_type,
            base_id=base_id,
            version=version,
        )
        daemon.add_task(task)

        # Plan file sync
        daemon._sync_to_plan_file()

        # Validate DoD quality
        warnings = daemon._validate_dod_quality(dod)

        result: dict[str, Any] = {
            "success": True,
            "task_id": normalized_id,
            "dependencies": task.dependencies,
            "model": task.model,
        }

        if warnings:
            result["warnings"] = warnings
            result["hint"] = (
                "Use Worker Packet format for better task specification. "
                "See docs/PLAN-DDD-CLEANCODE-guide.md"
            )

        return result

    # =========================================================================
    # c4_mark_blocked
    # =========================================================================

    def mark_blocked(
        self,
        task_id: str,
        worker_id: str,
        failure_signature: str,
        attempts: int,
        last_error: str = "",
    ) -> dict[str, Any]:
        """Mark a task as blocked after max retry attempts.

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
        daemon = self._daemon
        if daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Implicit heartbeat
        daemon._touch_worker(worker_id)

        state = daemon.state_machine.state

        # Prevent infinite REPAIR nesting
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

        # Validate task is actually in progress
        if task_id not in state.queue.in_progress:
            return {
                "success": False,
                "error": f"Task {task_id} is not in progress",
                "message": "Cannot mark a task as blocked if it's not currently in progress",
            }

        # Verify worker ownership
        assigned_worker = state.queue.in_progress.get(task_id)
        if assigned_worker != worker_id:
            return {
                "success": False,
                "error": f"Task {task_id} is assigned to {assigned_worker}, not {worker_id}",
                "message": "Cannot mark a task as blocked if you are not the assigned worker",
            }

        # Move task from in_progress
        del state.queue.in_progress[task_id]

        # Get task and release scope lock
        task = daemon.get_task(task_id)
        if task and task.scope:
            daemon.lock_store.release_scope_lock(state.project_id, task.scope)

        # Update worker state
        if daemon.worker_manager.is_registered(worker_id):
            daemon.worker_manager.set_idle(worker_id)

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
        daemon.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            worker_id,
            {
                "task_id": task_id,
                "blocked": True,
                "failure_signature": failure_signature,
                "attempts": attempts,
            },
        )

        daemon.state_machine.save_state()

        return {
            "success": True,
            "message": f"Task {task_id} marked as blocked and added to repair queue",
            "repair_queue_size": len(state.repair_queue),
        }
