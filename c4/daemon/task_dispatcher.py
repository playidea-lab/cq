"""Task Dispatcher - Priority-based task assignment with peer review support."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.models import C4State
    from c4.state_machine import StateMachine


class TaskPriority(int, Enum):
    """Task priority levels (higher = more urgent)."""

    CRITICAL = 100  # System failures, blocking issues
    HIGH = 80  # Repair tasks, checkpoint blockers
    NORMAL = 50  # Regular tasks
    LOW = 20  # Nice-to-have, refactoring
    BACKGROUND = 0  # Documentation, cleanup


@dataclass
class TaskAssignment:
    """Result of task assignment."""

    task_id: str
    worker_id: str
    priority: int
    is_repair: bool = False
    original_worker_id: str | None = None
    assigned_at: datetime = field(default_factory=datetime.now)


@dataclass
class AssignmentResult:
    """Result of assignment attempt."""

    success: bool
    assignment: TaskAssignment | None = None
    reason: str | None = None


class TaskDispatcher:
    """
    Dispatches tasks to workers with priority-based assignment.

    Features:
    - Priority queue ordering
    - Peer review: repair tasks go to different workers
    - Worker skill matching (optional)
    - Load balancing across idle workers

    Example:
        dispatcher = TaskDispatcher(state_machine)
        result = dispatcher.assign_next_task("worker-123")
        if result.success:
            print(f"Assigned {result.assignment.task_id}")
    """

    def __init__(
        self,
        state_machine: "StateMachine",
        enable_peer_review: bool = True,
    ):
        """Initialize dispatcher.

        Args:
            state_machine: State machine for state access
            enable_peer_review: Enable peer review for repair tasks
        """
        self._state_machine = state_machine
        self._peer_review_enabled = enable_peer_review
        self._task_priorities: dict[str, int] = {}
        self._task_history: dict[str, list[str]] = {}  # task_id -> [worker_ids]

    @property
    def state(self) -> "C4State":
        """Get current state."""
        return self._state_machine.state

    # =========================================================================
    # Priority Management
    # =========================================================================

    def set_priority(self, task_id: str, priority: TaskPriority | int) -> None:
        """Set priority for a task.

        Args:
            task_id: Task identifier
            priority: Priority level (TaskPriority enum or int)
        """
        if isinstance(priority, TaskPriority):
            priority = priority.value
        self._task_priorities[task_id] = priority

    def get_priority(self, task_id: str) -> int:
        """Get priority for a task (default: NORMAL)."""
        return self._task_priorities.get(task_id, TaskPriority.NORMAL.value)

    def set_repair_priority(self, task_id: str) -> None:
        """Mark a task as repair (high priority)."""
        self._task_priorities[task_id] = TaskPriority.HIGH.value

    # =========================================================================
    # Task History (for Peer Review)
    # =========================================================================

    def record_assignment(self, task_id: str, worker_id: str) -> None:
        """Record that a worker attempted a task.

        Args:
            task_id: Task identifier
            worker_id: Worker who attempted the task
        """
        if task_id not in self._task_history:
            self._task_history[task_id] = []
        if worker_id not in self._task_history[task_id]:
            self._task_history[task_id].append(worker_id)

    def get_previous_workers(self, task_id: str) -> list[str]:
        """Get list of workers who previously attempted a task."""
        return self._task_history.get(task_id, [])

    def is_repair_task(self, task_id: str) -> bool:
        """Check if a task is a repair (previously attempted)."""
        return len(self.get_previous_workers(task_id)) > 0

    # =========================================================================
    # Task Assignment
    # =========================================================================

    def get_prioritized_tasks(self) -> list[str]:
        """Get pending tasks sorted by priority (highest first).

        Returns:
            List of task IDs sorted by priority
        """
        pending = self.state.queue.pending
        return sorted(pending, key=lambda t: self.get_priority(t), reverse=True)

    def find_eligible_task(self, worker_id: str) -> str | None:
        """Find the highest priority task eligible for a worker.

        For peer review: if a task is a repair, the original worker
        is excluded from receiving it again.

        Args:
            worker_id: Worker requesting a task

        Returns:
            Task ID or None if no eligible tasks
        """
        prioritized = self.get_prioritized_tasks()

        for task_id in prioritized:
            # Check peer review constraint
            if self._peer_review_enabled and self.is_repair_task(task_id):
                previous_workers = self.get_previous_workers(task_id)
                if worker_id in previous_workers:
                    # This worker already attempted - skip for peer review
                    continue

            return task_id

        return None

    def assign_next_task(self, worker_id: str) -> AssignmentResult:
        """Assign the next available task to a worker.

        Considers:
        - Task priority
        - Peer review constraints (repair tasks go to different workers)
        - Worker availability

        Args:
            worker_id: Worker requesting a task

        Returns:
            AssignmentResult with assignment details or failure reason
        """
        # Check worker exists and is idle
        worker = self.state.workers.get(worker_id)
        if not worker:
            return AssignmentResult(
                success=False,
                reason=f"Worker {worker_id} not registered",
            )

        if worker.state != "idle":
            return AssignmentResult(
                success=False,
                reason=f"Worker {worker_id} is not idle (state: {worker.state})",
            )

        # Find eligible task
        task_id = self.find_eligible_task(worker_id)
        if not task_id:
            # Check if there are tasks but worker is excluded
            pending = self.state.queue.pending
            if pending and self._peer_review_enabled:
                # All tasks may be repairs from this worker
                all_repairs_from_worker = all(
                    worker_id in self.get_previous_workers(t) for t in pending
                )
                if all_repairs_from_worker:
                    return AssignmentResult(
                        success=False,
                        reason="All pending tasks require peer review (different worker)",
                    )

            return AssignmentResult(
                success=False,
                reason="No pending tasks",
            )

        # Create assignment
        is_repair = self.is_repair_task(task_id)
        original_worker = (
            self.get_previous_workers(task_id)[0] if is_repair else None
        )

        assignment = TaskAssignment(
            task_id=task_id,
            worker_id=worker_id,
            priority=self.get_priority(task_id),
            is_repair=is_repair,
            original_worker_id=original_worker,
        )

        # Record this assignment
        self.record_assignment(task_id, worker_id)

        return AssignmentResult(
            success=True,
            assignment=assignment,
        )

    def assign_repair_task(
        self,
        task_id: str,
        original_worker_id: str,
    ) -> AssignmentResult:
        """Assign a repair task to a different worker (peer review).

        This is called when a task fails and needs repair.
        The original worker is excluded from receiving it.

        Args:
            task_id: Failed task to repair
            original_worker_id: Worker who originally failed

        Returns:
            AssignmentResult with assignment or failure
        """
        # Record original worker
        self.record_assignment(task_id, original_worker_id)

        # Set repair priority
        self.set_repair_priority(task_id)

        # Find an idle worker different from original
        for worker_id, worker in self.state.workers.items():
            if worker.state != "idle":
                continue
            if worker_id == original_worker_id:
                continue

            assignment = TaskAssignment(
                task_id=task_id,
                worker_id=worker_id,
                priority=TaskPriority.HIGH.value,
                is_repair=True,
                original_worker_id=original_worker_id,
            )

            self.record_assignment(task_id, worker_id)

            return AssignmentResult(
                success=True,
                assignment=assignment,
            )

        return AssignmentResult(
            success=False,
            reason="No other idle workers available for peer review",
        )

    # =========================================================================
    # Load Balancing
    # =========================================================================

    def get_worker_load(self) -> dict[str, int]:
        """Get current task count per worker.

        Returns:
            Dict mapping worker_id to active task count
        """
        load: dict[str, int] = {}
        for worker_id, worker in self.state.workers.items():
            load[worker_id] = 1 if worker.state == "busy" else 0
        return load

    def get_least_loaded_worker(self) -> str | None:
        """Get the worker with the least load.

        Returns:
            Worker ID or None if no idle workers
        """
        idle_workers = [
            w_id
            for w_id, w in self.state.workers.items()
            if w.state == "idle"
        ]

        if not idle_workers:
            return None

        # For now, just return first idle (could add more sophisticated balancing)
        return idle_workers[0]

    # =========================================================================
    # Statistics
    # =========================================================================

    def get_stats(self) -> dict[str, Any]:
        """Get dispatcher statistics.

        Returns:
            Dict with dispatcher metrics
        """
        pending = self.state.queue.pending
        repairs = [t for t in pending if self.is_repair_task(t)]

        return {
            "pending_tasks": len(pending),
            "repair_tasks": len(repairs),
            "peer_review_enabled": self._peer_review_enabled,
            "priorities": {
                "critical": len([
                    t for t in pending
                    if self.get_priority(t) >= TaskPriority.CRITICAL.value
                ]),
                "high": len([
                    t for t in pending
                    if TaskPriority.HIGH.value <= self.get_priority(t) < TaskPriority.CRITICAL.value
                ]),
                "normal": len([
                    t for t in pending
                    if TaskPriority.LOW.value < self.get_priority(t) < TaskPriority.HIGH.value
                ]),
                "low": len([
                    t for t in pending
                    if self.get_priority(t) <= TaskPriority.LOW.value
                ]),
            },
            "worker_load": self.get_worker_load(),
        }
