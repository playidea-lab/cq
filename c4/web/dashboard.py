"""Team Dashboard Service.

Provides APIs for team dashboard:
- Team project list
- Realtime progress status
- Worker/review status
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

from c4.models import C4State, ProjectStatus
from c4.store import StateStore


@dataclass
class ProjectSummary:
    """Summary of a C4 project for dashboard display."""

    project_id: str
    name: str
    status: str
    tasks_pending: int
    tasks_in_progress: int
    tasks_done: int
    workers_active: int
    workers_idle: int
    last_activity: str | None
    checkpoint_state: str | None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "project_id": self.project_id,
            "name": self.name,
            "status": self.status,
            "tasks": {
                "pending": self.tasks_pending,
                "in_progress": self.tasks_in_progress,
                "done": self.tasks_done,
                "total": self.tasks_pending + self.tasks_in_progress + self.tasks_done,
            },
            "workers": {
                "active": self.workers_active,
                "idle": self.workers_idle,
                "total": self.workers_active + self.workers_idle,
            },
            "last_activity": self.last_activity,
            "checkpoint_state": self.checkpoint_state,
        }


@dataclass
class WorkerStatus:
    """Status of a worker in the team."""

    worker_id: str
    state: str  # "idle", "busy", "stale"
    task_id: str | None
    scope: str | None
    branch: str | None
    last_seen: str | None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "worker_id": self.worker_id,
            "state": self.state,
            "task_id": self.task_id,
            "scope": self.scope,
            "branch": self.branch,
            "last_seen": self.last_seen,
        }


@dataclass
class ReviewStatus:
    """Status of reviews/checkpoints in the team."""

    checkpoint_id: str | None
    state: str  # "pending", "in_review", "approved", "changes_requested"
    tasks_completed: int
    checkpoints_passed: int
    checkpoints_total: int
    repair_queue_size: int

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "checkpoint_id": self.checkpoint_id,
            "state": self.state,
            "tasks_completed": self.tasks_completed,
            "checkpoints": {
                "passed": self.checkpoints_passed,
                "total": self.checkpoints_total,
            },
            "repair_queue_size": self.repair_queue_size,
        }


@dataclass
class RealtimeStatus:
    """Realtime status update for dashboard."""

    timestamp: str
    project_id: str
    status: str
    progress: float  # 0.0 - 1.0
    tasks: dict[str, int]
    workers: list[WorkerStatus]
    review: ReviewStatus
    events: list[dict[str, Any]] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "timestamp": self.timestamp,
            "project_id": self.project_id,
            "status": self.status,
            "progress": self.progress,
            "tasks": self.tasks,
            "workers": [w.to_dict() for w in self.workers],
            "review": self.review.to_dict(),
            "events": self.events,
        }


class DashboardService:
    """Service for team dashboard data.

    Provides:
    - list_projects(): Get all team projects
    - get_realtime_status(project_id): Get realtime status for a project
    - get_worker_status(project_id): Get all workers' status
    - get_review_status(project_id): Get review/checkpoint status
    """

    def __init__(
        self,
        store: StateStore,
        c4_root: Path | None = None,
    ):
        """Initialize dashboard service.

        Args:
            store: State store for loading project states
            c4_root: Root directory for C4 projects (default: ~/.c4)
        """
        self.store = store
        self.c4_root = c4_root or Path.home() / ".c4"

    def list_projects(self) -> list[ProjectSummary]:
        """List all team projects with summary info.

        Returns:
            List of ProjectSummary objects
        """
        projects: list[ProjectSummary] = []

        # Scan projects directory
        projects_dir = self.c4_root / "projects"
        if not projects_dir.exists():
            return projects

        for project_dir in projects_dir.iterdir():
            if not project_dir.is_dir():
                continue

            project_id = project_dir.name
            state = self._load_project_state(project_id)
            if state is None:
                continue

            summary = self._build_project_summary(project_id, state)
            projects.append(summary)

        return projects

    def get_realtime_status(self, project_id: str) -> RealtimeStatus | None:
        """Get realtime status for a project.

        Args:
            project_id: Project identifier

        Returns:
            RealtimeStatus or None if project not found
        """
        state = self._load_project_state(project_id)
        if state is None:
            return None

        workers = self._get_worker_statuses(state)
        review = self._get_review_status(state)

        total_tasks = (
            len(state.queue.pending)
            + len(state.queue.in_progress)
            + len(state.queue.done)
        )
        progress = len(state.queue.done) / total_tasks if total_tasks > 0 else 0.0

        return RealtimeStatus(
            timestamp=datetime.now().isoformat(),
            project_id=project_id,
            status=state.status.value if state.status else "unknown",
            progress=progress,
            tasks={
                "pending": len(state.queue.pending),
                "in_progress": len(state.queue.in_progress),
                "done": len(state.queue.done),
            },
            workers=workers,
            review=review,
            events=self._get_recent_events(state),
        )

    def get_worker_status(self, project_id: str) -> list[WorkerStatus]:
        """Get all workers' status for a project.

        Args:
            project_id: Project identifier

        Returns:
            List of WorkerStatus objects
        """
        state = self._load_project_state(project_id)
        if state is None:
            return []

        return self._get_worker_statuses(state)

    def get_review_status(self, project_id: str) -> ReviewStatus | None:
        """Get review/checkpoint status for a project.

        Args:
            project_id: Project identifier

        Returns:
            ReviewStatus or None if project not found
        """
        state = self._load_project_state(project_id)
        if state is None:
            return None

        return self._get_review_status(state)

    def _load_project_state(self, project_id: str) -> C4State | None:
        """Load project state from store."""
        try:
            return self.store.load(project_id)
        except Exception:
            return None

    def _build_project_summary(
        self, project_id: str, state: C4State
    ) -> ProjectSummary:
        """Build project summary from state."""
        workers_active = sum(
            1 for w in state.workers.values() if w.state == "busy"
        )
        workers_idle = sum(
            1 for w in state.workers.values() if w.state == "idle"
        )

        last_activity = None
        if state.workers:
            last_seen_times = [
                w.last_seen for w in state.workers.values() if w.last_seen
            ]
            if last_seen_times:
                last_activity = max(last_seen_times).isoformat()

        return ProjectSummary(
            project_id=project_id,
            name=project_id,  # Could be enhanced with project metadata
            status=state.status.value if state.status else "unknown",
            tasks_pending=len(state.queue.pending),
            tasks_in_progress=len(state.queue.in_progress),
            tasks_done=len(state.queue.done),
            workers_active=workers_active,
            workers_idle=workers_idle,
            last_activity=last_activity,
            checkpoint_state=state.checkpoint.state if state.checkpoint else None,
        )

    def _get_worker_statuses(self, state: C4State) -> list[WorkerStatus]:
        """Get worker statuses from project state."""
        workers: list[WorkerStatus] = []

        for worker_id, info in state.workers.items():
            workers.append(
                WorkerStatus(
                    worker_id=worker_id,
                    state=info.state,
                    task_id=info.task_id,
                    scope=info.scope,
                    branch=info.branch,
                    last_seen=info.last_seen.isoformat() if info.last_seen else None,
                )
            )

        return workers

    def _get_review_status(self, state: C4State) -> ReviewStatus:
        """Get review status from project state."""
        # Count total checkpoints from config gates
        checkpoints_total = len(state.passed_checkpoints) + (
            1 if state.checkpoint.current else 0
        )

        # Determine review state
        review_state = "idle"
        if state.status == ProjectStatus.CHECKPOINT:
            review_state = "in_review"
        elif state.checkpoint.state == "approved":
            review_state = "approved"
        elif state.checkpoint.state == "changes_requested":
            review_state = "changes_requested"

        return ReviewStatus(
            checkpoint_id=state.checkpoint.current,
            state=review_state,
            tasks_completed=len(state.queue.done),
            checkpoints_passed=len(state.passed_checkpoints),
            checkpoints_total=checkpoints_total,
            repair_queue_size=len(state.repair_queue),
        )

    def _get_recent_events(
        self, state: C4State, limit: int = 10
    ) -> list[dict[str, Any]]:
        """Get recent events from project state."""
        events: list[dict[str, Any]] = []

        # Add checkpoint queue items as events
        for item in state.checkpoint_queue[-limit:]:
            events.append(
                {
                    "type": "checkpoint_triggered",
                    "timestamp": item.triggered_at,
                    "data": {
                        "checkpoint_id": item.checkpoint_id,
                        "tasks_completed": len(item.tasks_completed),
                    },
                }
            )

        # Add repair queue items as events
        for item in state.repair_queue[-limit:]:
            events.append(
                {
                    "type": "task_blocked",
                    "timestamp": datetime.now().isoformat(),  # No timestamp in model
                    "data": {
                        "task_id": item.task_id,
                        "failure_signature": item.failure_signature,
                        "attempts": item.attempts,
                    },
                }
            )

        # Sort by timestamp and limit
        events.sort(key=lambda e: e.get("timestamp", ""), reverse=True)
        return events[:limit]


def create_dashboard_service(c4_root: Path | None = None) -> DashboardService:
    """Factory function to create DashboardService.

    Args:
        c4_root: Root directory for C4 projects (default: ~/.c4)

    Returns:
        Configured DashboardService instance
    """
    from c4.store import SQLiteStateStore

    c4_root = c4_root or Path.home() / ".c4"
    # Use the global state.db for multi-project dashboard
    store = SQLiteStateStore(c4_root / "state.db")
    return DashboardService(store, c4_root)
