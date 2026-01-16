"""C4D Safety Guards - Protection against infinite loops and runaway processes"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

from ..constants import MAX_ITERATIONS_PER_TASK, TASK_TIMEOUT_SEC

logger = logging.getLogger(__name__)


# Default limits (override from constants.py where applicable)
DEFAULT_MAX_ITERATIONS_PER_TASK = MAX_ITERATIONS_PER_TASK
DEFAULT_TASK_TIMEOUT_SEC = TASK_TIMEOUT_SEC
MAX_TOTAL_ITERATIONS = 100
MAX_CONSECUTIVE_FAILURES = 5
SESSION_TIMEOUT_SECONDS = 14400  # 4 hours


@dataclass
class TaskStats:
    """Statistics for a single task"""

    task_id: str
    started_at: datetime = field(default_factory=datetime.now)
    iterations: int = 0
    failures: int = 0
    last_activity: datetime = field(default_factory=datetime.now)


@dataclass
class SafetyState:
    """State for safety monitoring"""

    session_started: datetime = field(default_factory=datetime.now)
    total_iterations: int = 0
    consecutive_failures: int = 0
    task_stats: dict[str, TaskStats] = field(default_factory=dict)
    last_task_id: str | None = None


class SafetyGuard:
    """
    Safety guard for preventing runaway automation.

    Monitors:
    - Per-task iteration limits
    - Total session iteration limits
    - Consecutive failure limits
    - Task timeouts
    - Session timeouts
    """

    def __init__(
        self,
        max_iterations_per_task: int = DEFAULT_MAX_ITERATIONS_PER_TASK,
        max_total_iterations: int = MAX_TOTAL_ITERATIONS,
        max_consecutive_failures: int = MAX_CONSECUTIVE_FAILURES,
        task_timeout_seconds: int = DEFAULT_TASK_TIMEOUT_SEC,
        session_timeout_seconds: int = SESSION_TIMEOUT_SECONDS,
    ):
        self.max_iterations_per_task = max_iterations_per_task
        self.max_total_iterations = max_total_iterations
        self.max_consecutive_failures = max_consecutive_failures
        self.task_timeout_seconds = task_timeout_seconds
        self.session_timeout_seconds = session_timeout_seconds
        self._state = SafetyState()

    def reset(self) -> None:
        """Reset all safety state"""
        self._state = SafetyState()
        logger.info("Safety guard reset")

    def start_task(self, task_id: str) -> None:
        """Record start of work on a task"""
        if task_id not in self._state.task_stats:
            self._state.task_stats[task_id] = TaskStats(task_id=task_id)
        self._state.last_task_id = task_id
        logger.debug(f"Started tracking task: {task_id}")

    def record_iteration(self, task_id: str | None = None) -> None:
        """Record an iteration (attempt to complete a task step)"""
        task_id = task_id or self._state.last_task_id
        self._state.total_iterations += 1

        if task_id and task_id in self._state.task_stats:
            stats = self._state.task_stats[task_id]
            stats.iterations += 1
            stats.last_activity = datetime.now()

        logger.debug(
            f"Iteration recorded. Task: {task_id}, "
            f"Total: {self._state.total_iterations}"
        )

    def record_failure(self, task_id: str | None = None) -> None:
        """Record a failure"""
        task_id = task_id or self._state.last_task_id
        self._state.consecutive_failures += 1

        if task_id and task_id in self._state.task_stats:
            self._state.task_stats[task_id].failures += 1

        logger.debug(
            f"Failure recorded. Consecutive: {self._state.consecutive_failures}"
        )

    def record_success(self) -> None:
        """Record a success (resets consecutive failure counter)"""
        self._state.consecutive_failures = 0
        logger.debug("Success recorded, consecutive failures reset")

    def check_can_continue(self, task_id: str | None = None) -> tuple[bool, str]:
        """
        Check if automation can continue.

        Args:
            task_id: Current task ID to check

        Returns:
            Tuple of (can_continue, reason_if_not)
        """
        task_id = task_id or self._state.last_task_id

        # Check session timeout
        session_elapsed = datetime.now() - self._state.session_started
        if session_elapsed.total_seconds() > self.session_timeout_seconds:
            return False, f"Session timeout ({self.session_timeout_seconds}s exceeded)"

        # Check total iterations
        if self._state.total_iterations >= self.max_total_iterations:
            return False, f"Max total iterations ({self.max_total_iterations}) reached"

        # Check consecutive failures
        if self._state.consecutive_failures >= self.max_consecutive_failures:
            return False, f"Max consecutive failures ({self.max_consecutive_failures}) reached"

        # Check task-specific limits
        if task_id and task_id in self._state.task_stats:
            stats = self._state.task_stats[task_id]

            # Task iterations
            if stats.iterations >= self.max_iterations_per_task:
                max_iter = self.max_iterations_per_task
                return False, f"Max iterations per task ({max_iter}) reached for {task_id}"

            # Task timeout
            task_elapsed = datetime.now() - stats.started_at
            if task_elapsed.total_seconds() > self.task_timeout_seconds:
                return False, f"Task timeout ({self.task_timeout_seconds}s) exceeded for {task_id}"

        return True, ""

    def get_status(self) -> dict[str, Any]:
        """Get current safety status"""
        session_elapsed = datetime.now() - self._state.session_started

        return {
            "session_started": self._state.session_started.isoformat(),
            "session_elapsed_seconds": int(session_elapsed.total_seconds()),
            "total_iterations": self._state.total_iterations,
            "consecutive_failures": self._state.consecutive_failures,
            "limits": {
                "max_iterations_per_task": self.max_iterations_per_task,
                "max_total_iterations": self.max_total_iterations,
                "max_consecutive_failures": self.max_consecutive_failures,
                "task_timeout_seconds": self.task_timeout_seconds,
                "session_timeout_seconds": self.session_timeout_seconds,
            },
            "task_count": len(self._state.task_stats),
            "can_continue": self.check_can_continue()[0],
        }
