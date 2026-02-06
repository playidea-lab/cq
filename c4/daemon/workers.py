"""C4 Worker Manager - Worker lifecycle management for multi-worker support"""

import logging
import re
from datetime import datetime
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.models import C4Config, TaskQueue, WorkerInfo
    from c4.state_machine import StateMachine

logger = logging.getLogger(__name__)

# Worker ID format: worker-[8 hex chars from uuid]
WORKER_ID_PATTERN = re.compile(r"^worker-[a-f0-9]{8}$")


class WorkerManager:
    """Manages worker registration and lifecycle"""

    def __init__(self, state_machine: "StateMachine", config: "C4Config"):
        self.state_machine = state_machine
        self.config = config

    @property
    def _workers(self) -> dict[str, "WorkerInfo"]:
        """Get the workers dict from state"""
        return self.state_machine.state.workers

    def _recover_task_to_pending(
        self, queue: "TaskQueue", task_id: str | None
    ) -> bool:
        """Move a task from in_progress back to pending queue.

        This is a common pattern used when a worker crashes, is killed,
        or becomes stale while processing a task.

        Args:
            queue: The task queue to modify
            task_id: The task ID to recover (None is safe - returns False)

        Returns:
            True if task was recovered, False if task_id was None or not in progress
        """
        if task_id and task_id in queue.in_progress:
            del queue.in_progress[task_id]
            queue.pending.insert(0, task_id)  # Add to front for priority
            return True
        return False

    def register(self, worker_id: str) -> "WorkerInfo":
        """Register a new worker.

        Validates worker ID format and detects duplicate registrations.

        Args:
            worker_id: Worker ID in format "worker-[a-f0-9]{8}" (uuid-based)

        Returns:
            WorkerInfo for the registered worker

        Raises:
            ValueError: If worker_id format is invalid or already registered
        """
        from c4.models import EventType, WorkerInfo

        # Validate worker ID format (must be UUID-based)
        if not WORKER_ID_PATTERN.match(worker_id):
            raise ValueError(
                f"Invalid worker_id format: '{worker_id}'. "
                f"Must match pattern 'worker-[a-f0-9]{{8}}' (generated from uuid.uuid4().hex[:8]). "
                f"Example: 'worker-a1b2c3d4'"
            )

        # Detect duplicate worker ID registration
        if worker_id in self._workers:
            existing = self._workers[worker_id]
            logger.warning(
                f"Worker ID collision detected: '{worker_id}' already registered. "
                f"State: {existing.state}, Last seen: {existing.last_seen}"
            )
            raise ValueError(
                f"Worker ID '{worker_id}' is already registered. "
                f"This likely indicates multiple instances using the same ID. "
                f"Each worker MUST generate a unique ID using uuid.uuid4().hex[:8]"
            )

        now = datetime.now()

        # Auto-detect GPU capability
        gpu_capable = self._detect_gpu_capability()

        worker = WorkerInfo(
            worker_id=worker_id,
            state="idle",
            joined_at=now,
            last_seen=now,
            gpu_capable=gpu_capable,
        )

        self._workers[worker_id] = worker
        self.state_machine.emit_event(
            EventType.WORKER_JOINED,
            worker_id,
            {"worker_id": worker_id},
        )
        self.state_machine.save_state()

        logger.info(f"Worker registered successfully: {worker_id}")
        return worker

    @staticmethod
    def _detect_gpu_capability() -> bool:
        """Detect if the current machine has GPU capability."""
        try:
            from c4.gpu.monitor import GpuMonitor

            monitor = GpuMonitor()
            gpus = monitor.detect()
            return any(g.backend != "cpu" for g in gpus)
        except Exception:
            return False

    def unregister(self, worker_id: str, lock_store: "Any | None" = None) -> bool:
        """
        Unregister a worker and release any assigned tasks.

        If the worker was busy with a task, the task is moved back to the
        pending queue to prevent zombie assignments.

        Args:
            worker_id: The worker ID to unregister
            lock_store: Optional lock store for releasing scope locks

        Returns:
            True if worker was removed, False if not found
        """
        from c4.models import EventType

        if worker_id not in self._workers:
            return False

        worker = self._workers[worker_id]
        recovered_task = None

        # If worker was busy, recover the task
        if worker.state == "busy" and worker.task_id:
            recovered_task = worker.task_id
            scope = worker.scope
            queue = self.state_machine.state.queue

            # Move task from in_progress back to pending
            self._recover_task_to_pending(queue, worker.task_id)

            # Release scope lock if applicable
            if scope and lock_store:
                try:
                    lock_store.release_scope_lock(
                        self.state_machine.state.project_id, scope
                    )
                except Exception:
                    pass  # Best effort

        del self._workers[worker_id]
        self.state_machine.emit_event(
            EventType.WORKER_LEFT,
            worker_id,
            {"worker_id": worker_id, "recovered_task": recovered_task},
        )
        self.state_machine.save_state()
        return True

    def heartbeat(self, worker_id: str) -> bool:
        """
        Update worker's last_seen timestamp atomically.

        Uses atomic_modify to prevent overwriting other workers' queue changes.
        This fixes a critical bug where heartbeat could overwrite task state
        with stale cached data.

        Returns:
            True if updated, False if worker not found
        """
        store = self.state_machine.store
        project_id = self.state_machine.state.project_id

        # Use atomic_modify to safely update only worker's last_seen
        try:
            with store.atomic_modify(project_id) as state:
                if worker_id not in state.workers:
                    return False
                state.workers[worker_id].last_seen = datetime.now()
                # Update cached state
                self.state_machine._state = state
            return True
        except Exception:
            # If atomic_modify fails, don't update anything
            return False

    def get_worker(self, worker_id: str) -> "WorkerInfo | None":
        """Get worker info by ID"""
        return self._workers.get(worker_id)

    def is_registered(self, worker_id: str) -> bool:
        """Check if worker is registered"""
        return worker_id in self._workers

    def set_busy(
        self,
        worker_id: str,
        task_id: str,
        scope: str | None = None,
        branch: str | None = None,
    ) -> bool:
        """
        Mark worker as busy with a task.

        Returns:
            True if updated, False if worker not found
        """
        if worker_id not in self._workers:
            return False

        worker = self._workers[worker_id]
        worker.state = "busy"
        worker.task_id = task_id
        worker.scope = scope
        worker.branch = branch
        worker.last_seen = datetime.now()
        return True

    def set_idle(self, worker_id: str) -> bool:
        """
        Mark worker as idle (no task).

        Returns:
            True if updated, False if worker not found
        """
        if worker_id not in self._workers:
            return False

        worker = self._workers[worker_id]
        worker.state = "idle"
        worker.task_id = None
        worker.scope = None
        worker.last_seen = datetime.now()
        return True

    def get_status(self) -> dict[str, Any]:
        """Get current worker status for debugging/monitoring"""
        now = datetime.now()
        worker_info = {}

        for worker_id, worker in self._workers.items():
            idle_seconds = (now - worker.last_seen).total_seconds()
            worker_info[worker_id] = {
                "state": worker.state,
                "task_id": worker.task_id,
                "scope": worker.scope,
                "branch": worker.branch,
                "joined_at": worker.joined_at.isoformat(),
                "last_seen": worker.last_seen.isoformat(),
                "idle_seconds": idle_seconds,
            }

        return {
            "total_workers": len(self._workers),
            "idle": sum(1 for w in self._workers.values() if w.state == "idle"),
            "busy": sum(1 for w in self._workers.values() if w.state == "busy"),
            "workers": worker_info,
        }

    def cleanup_stale(self, max_idle_minutes: int) -> list[str]:
        """
        Remove workers that have been idle too long.

        Args:
            max_idle_minutes: Maximum idle time before removal

        Returns:
            List of worker IDs that were removed
        """
        if max_idle_minutes <= 0:
            return []

        from datetime import timedelta

        now = datetime.now()
        max_idle = timedelta(minutes=max_idle_minutes)
        stale = []

        for worker_id, worker in list(self._workers.items()):
            if worker.state == "idle" and (now - worker.last_seen) > max_idle:
                stale.append(worker_id)
                del self._workers[worker_id]

        if stale:
            self.state_machine.save_state()

        return stale

    def recover_stale_workers(
        self,
        stale_timeout_seconds: int,
        lock_store: Any = None,
    ) -> list[dict[str, Any]]:
        """
        Recover tasks from workers that have been inactive too long.

        This handles workers that crashed while busy. The tasks are moved
        back to pending queue and scope locks are released.

        Uses atomic_modify to prevent race conditions with task assignment.

        Args:
            stale_timeout_seconds: Time in seconds after which a busy worker is considered stale
            lock_store: Optional SQLite lock store for releasing scope locks

        Returns:
            List of recovery actions taken (for logging/monitoring)
        """
        from datetime import timedelta

        now = datetime.now()
        stale_threshold = timedelta(seconds=stale_timeout_seconds)
        recoveries: list[dict[str, Any]] = []

        store = self.state_machine.store
        project_id = self.state_machine.state.project_id

        # Use atomic_modify to safely update queue and workers
        with store.atomic_modify(project_id) as state:
            queue = state.queue

            for worker_id, worker in list(state.workers.items()):
                if worker.state != "busy":
                    continue

                elapsed = now - worker.last_seen
                if elapsed <= stale_threshold:
                    continue

                # Worker is stale - recover task
                task_id = worker.task_id
                scope = worker.scope

                recovery_info: dict[str, Any] = {
                    "worker_id": worker_id,
                    "task_id": task_id,
                    "scope": scope,
                    "elapsed_seconds": elapsed.total_seconds(),
                }

                # Move task back to pending if it's in progress
                if self._recover_task_to_pending(queue, task_id):
                    recovery_info["task_recovered"] = True

                # Release scope lock (outside atomic block is OK - lock store is separate)
                if scope and lock_store:
                    try:
                        lock_store.release_scope_lock(project_id, scope)
                        recovery_info["lock_released"] = True
                    except Exception as e:
                        recovery_info["lock_release_error"] = str(e)

                # Mark worker as disconnected
                worker.state = "disconnected"
                worker.task_id = None
                worker.scope = None

                recoveries.append(recovery_info)

            # Update cached state
            self.state_machine._state = state

        return recoveries


    def get_long_running_alerts(
        self,
        warning_timeout_seconds: int,
        stale_timeout_seconds: int,
    ) -> list[dict[str, Any]]:
        """
        Get alerts for workers that have exceeded the warning timeout.

        These are workers that are still busy but haven't sent a heartbeat
        for longer than warning_timeout_seconds (but less than stale_timeout_seconds).

        Users can respond to these alerts via c4_handle_long_running with:
        - "continue": Acknowledge and keep waiting
        - "extend": Reset the worker's last_seen (extend timeout by another cycle)
        - "kill": Mark worker as stale and recover the task

        Args:
            warning_timeout_seconds: Time after which to show warning
            stale_timeout_seconds: Time after which worker is auto-recovered

        Returns:
            List of alert objects with worker/task info and available actions
        """
        from datetime import timedelta

        now = datetime.now()
        warning_threshold = timedelta(seconds=warning_timeout_seconds)
        stale_threshold = timedelta(seconds=stale_timeout_seconds)
        alerts: list[dict[str, Any]] = []

        for worker_id, worker in self._workers.items():
            if worker.state != "busy":
                continue

            if worker.last_seen is None:
                continue

            elapsed = now - worker.last_seen
            elapsed_seconds = elapsed.total_seconds()

            # Only alert if in warning zone (between warning and stale)
            if warning_threshold < elapsed <= stale_threshold:
                remaining_seconds = stale_timeout_seconds - elapsed_seconds
                alerts.append({
                    "type": "long_running_worker",
                    "worker_id": worker_id,
                    "task_id": worker.task_id,
                    "scope": worker.scope,
                    "elapsed_minutes": int(elapsed_seconds / 60),
                    "elapsed_seconds": int(elapsed_seconds),
                    "remaining_seconds": int(remaining_seconds),
                    "remaining_minutes": int(remaining_seconds / 60),
                    "message": (
                        f"Worker '{worker_id}' has been unresponsive for "
                        f"{int(elapsed_seconds / 60)} minutes while working on {worker.task_id}. "
                        f"Auto-recovery in {int(remaining_seconds / 60)} minutes."
                    ),
                    "actions": [
                        {
                            "action": "continue",
                            "description": "Normal long-running task - keep waiting",
                        },
                        {
                            "action": "extend",
                            "description": "Extend timeout by 60 minutes",
                        },
                        {
                            "action": "kill",
                            "description": "Worker is stuck - recover task",
                        },
                    ],
                })

        return alerts

    def extend_worker_timeout(
        self,
        worker_id: str,
        extension_seconds: int = 3600,
    ) -> dict[str, Any]:
        """
        Extend a worker's timeout by resetting its last_seen timestamp.

        This effectively gives the worker more time to complete its task
        without being marked as stale.

        Args:
            worker_id: The worker to extend
            extension_seconds: How many seconds to extend (default 60 minutes)

        Returns:
            Result dict with status and new timeout info
        """
        store = self.state_machine.store
        project_id = self.state_machine.state.project_id

        try:
            with store.atomic_modify(project_id) as state:
                if worker_id not in state.workers:
                    return {
                        "success": False,
                        "error": f"Worker '{worker_id}' not found",
                    }

                worker = state.workers[worker_id]
                if worker.state != "busy":
                    return {
                        "success": False,
                        "error": f"Worker '{worker_id}' is not busy (state: {worker.state})",
                    }

                # Reset last_seen to now (effectively extending the timeout)
                old_last_seen = worker.last_seen
                worker.last_seen = datetime.now()

                # Update cached state
                self.state_machine._state = state

            return {
                "success": True,
                "worker_id": worker_id,
                "task_id": worker.task_id,
                "old_last_seen": old_last_seen.isoformat() if old_last_seen else None,
                "new_last_seen": worker.last_seen.isoformat(),
                "extension_minutes": extension_seconds // 60,
                "message": f"Timeout extended by {extension_seconds // 60} minutes",
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
            }

    def kill_worker(
        self,
        worker_id: str,
        lock_store: Any = None,
    ) -> dict[str, Any]:
        """
        Force-kill a worker and recover its task.

        This is used when a user determines that a worker is truly stuck
        and wants to recover the task for reassignment.

        Args:
            worker_id: The worker to kill
            lock_store: Optional lock store for releasing scope locks

        Returns:
            Result dict with status and recovery info
        """
        store = self.state_machine.store
        project_id = self.state_machine.state.project_id

        try:
            with store.atomic_modify(project_id) as state:
                if worker_id not in state.workers:
                    return {
                        "success": False,
                        "error": f"Worker '{worker_id}' not found",
                    }

                worker = state.workers[worker_id]
                task_id = worker.task_id
                scope = worker.scope
                queue = state.queue

                recovery_info: dict[str, Any] = {
                    "success": True,
                    "worker_id": worker_id,
                    "task_id": task_id,
                    "scope": scope,
                    "previous_state": worker.state,
                }

                # Move task back to pending if it's in progress
                if self._recover_task_to_pending(queue, task_id):
                    recovery_info["task_recovered"] = True
                    recovery_info["message"] = (
                        f"Worker killed. Task {task_id} moved back to pending queue."
                    )
                else:
                    recovery_info["task_recovered"] = False
                    recovery_info["message"] = "Worker killed. No task to recover."

                # Mark worker as disconnected
                worker.state = "disconnected"
                worker.task_id = None
                worker.scope = None

                # Update cached state
                self.state_machine._state = state

            # Release scope lock (outside atomic block - lock store is separate)
            if scope and lock_store:
                try:
                    lock_store.release_scope_lock(project_id, scope)
                    recovery_info["lock_released"] = True
                except Exception as e:
                    recovery_info["lock_release_error"] = str(e)

            return recovery_info

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
            }
