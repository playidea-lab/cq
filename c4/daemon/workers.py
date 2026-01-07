"""C4 Worker Manager - Worker lifecycle management for multi-worker support"""

from datetime import datetime
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.models import C4Config, WorkerInfo
    from c4.state_machine import StateMachine


class WorkerManager:
    """Manages worker registration and lifecycle"""

    def __init__(self, state_machine: "StateMachine", config: "C4Config"):
        self.state_machine = state_machine
        self.config = config

    @property
    def _workers(self) -> dict[str, "WorkerInfo"]:
        """Get the workers dict from state"""
        return self.state_machine.state.workers

    def register(self, worker_id: str) -> "WorkerInfo":
        """Register a new worker"""
        from c4.models import EventType, WorkerInfo

        now = datetime.now()
        worker = WorkerInfo(
            worker_id=worker_id,
            state="idle",
            joined_at=now,
            last_seen=now,
        )

        self._workers[worker_id] = worker
        self.state_machine.emit_event(
            EventType.WORKER_JOINED,
            worker_id,
            {"worker_id": worker_id},
        )
        self.state_machine.save_state()

        return worker

    def unregister(self, worker_id: str) -> bool:
        """
        Unregister a worker.

        Returns:
            True if worker was removed, False if not found
        """
        from c4.models import EventType

        if worker_id not in self._workers:
            return False

        del self._workers[worker_id]
        self.state_machine.emit_event(
            EventType.WORKER_LEFT,
            worker_id,
            {"worker_id": worker_id},
        )
        self.state_machine.save_state()
        return True

    def heartbeat(self, worker_id: str) -> bool:
        """
        Update worker's last_seen timestamp.

        Returns:
            True if updated, False if worker not found
        """
        if worker_id not in self._workers:
            return False

        self._workers[worker_id].last_seen = datetime.now()
        self.state_machine.save_state()
        return True

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
