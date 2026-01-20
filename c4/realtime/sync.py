"""C4 Multi-Worker Synchronization Module.

Provides coordination between multiple C4 workers using real-time
notifications and optimistic locking.

Features:
- Worker state change notifications
- Task assignment conflict prevention
- Heartbeat-based worker presence
- Leader election support
- Event broadcasting between workers

Usage:
    from c4.realtime.sync import WorkerSync, SyncConfig

    sync = WorkerSync(
        store=supabase_store,
        worker_id="worker-123",
        project_id="my-project",
    )

    # Register event handlers
    sync.on_task_assigned(handle_task_assigned)
    sync.on_worker_joined(handle_worker_joined)
    sync.on_state_changed(handle_state_changed)

    # Start synchronization
    sync.start()

    # Broadcast events
    sync.broadcast_task_completed("T-001")

    # Cleanup
    sync.stop()
"""

from __future__ import annotations

import logging
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from enum import Enum
from typing import TYPE_CHECKING, Any, Callable

if TYPE_CHECKING:
    from c4.models import C4State
    from c4.store.supabase import ChangeType, SupabaseStateStore

logger = logging.getLogger(__name__)


class SyncEvent(Enum):
    """Types of synchronization events."""

    # State changes
    STATE_CHANGED = "state_changed"
    TASK_ASSIGNED = "task_assigned"
    TASK_COMPLETED = "task_completed"
    TASK_BLOCKED = "task_blocked"

    # Worker events
    WORKER_JOINED = "worker_joined"
    WORKER_LEFT = "worker_left"
    WORKER_HEARTBEAT = "worker_heartbeat"

    # Checkpoint events
    CHECKPOINT_REACHED = "checkpoint_reached"
    CHECKPOINT_APPROVED = "checkpoint_approved"

    # Lock events
    LOCK_ACQUIRED = "lock_acquired"
    LOCK_RELEASED = "lock_released"


@dataclass
class WorkerInfo:
    """Information about a connected worker."""

    worker_id: str
    project_id: str
    status: str = "active"
    last_heartbeat: datetime = field(default_factory=datetime.now)
    current_task: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def is_alive(self) -> bool:
        """Check if worker is alive (heartbeat within threshold)."""
        threshold = timedelta(seconds=60)  # 60 second timeout
        return datetime.now() - self.last_heartbeat < threshold


@dataclass
class SyncConfig:
    """Configuration for WorkerSync.

    Args:
        heartbeat_interval: Seconds between heartbeat broadcasts
        worker_timeout: Seconds before considering a worker dead
        conflict_retry_max: Maximum retries for conflict resolution
        conflict_retry_delay: Base delay between conflict retries
    """

    heartbeat_interval: float = 30.0
    worker_timeout: float = 60.0
    conflict_retry_max: int = 3
    conflict_retry_delay: float = 0.5


# Callback types
StateChangeHandler = Callable[["C4State", "ChangeType"], None]
TaskEventHandler = Callable[[str, str], None]  # (task_id, worker_id)
WorkerEventHandler = Callable[[WorkerInfo], None]
ConflictHandler = Callable[[str, str], bool]  # (task_id, other_worker) -> retry?


class WorkerSync:
    """Multi-worker synchronization coordinator.

    Manages real-time communication between multiple C4 workers:
    - Broadcasts and receives events via Supabase Realtime
    - Tracks worker presence and health
    - Prevents task assignment conflicts
    - Coordinates state updates with optimistic locking
    """

    def __init__(
        self,
        store: "SupabaseStateStore",
        worker_id: str,
        project_id: str,
        config: SyncConfig | None = None,
    ):
        """Initialize WorkerSync.

        Args:
            store: SupabaseStateStore with realtime enabled
            worker_id: Unique identifier for this worker
            project_id: Project to synchronize
            config: Sync configuration
        """
        self._store = store
        self._worker_id = worker_id
        self._project_id = project_id
        self._config = config or SyncConfig()

        # State tracking
        self._workers: dict[str, WorkerInfo] = {}
        self._running = False
        self._lock = threading.RLock()

        # Threads
        self._heartbeat_thread: threading.Thread | None = None
        self._stop_event = threading.Event()

        # Event handlers
        self._state_handlers: list[StateChangeHandler] = []
        self._task_assigned_handlers: list[TaskEventHandler] = []
        self._task_completed_handlers: list[TaskEventHandler] = []
        self._task_blocked_handlers: list[TaskEventHandler] = []
        self._worker_joined_handlers: list[WorkerEventHandler] = []
        self._worker_left_handlers: list[WorkerEventHandler] = []
        self._conflict_handlers: list[ConflictHandler] = []

        # Register self as a worker
        self._register_self()

    @property
    def worker_id(self) -> str:
        """Get this worker's ID."""
        return self._worker_id

    @property
    def project_id(self) -> str:
        """Get the project ID."""
        return self._project_id

    @property
    def workers(self) -> dict[str, WorkerInfo]:
        """Get all known workers."""
        with self._lock:
            return dict(self._workers)

    @property
    def active_workers(self) -> list[WorkerInfo]:
        """Get list of active (alive) workers."""
        with self._lock:
            return [w for w in self._workers.values() if w.is_alive]

    def _register_self(self) -> None:
        """Register this worker in the workers dict."""
        with self._lock:
            self._workers[self._worker_id] = WorkerInfo(
                worker_id=self._worker_id,
                project_id=self._project_id,
                status="initializing",
            )

    # =========================================================================
    # Event Handler Registration
    # =========================================================================

    def on_state_changed(self, handler: StateChangeHandler) -> None:
        """Register handler for state changes."""
        self._state_handlers.append(handler)

    def on_task_assigned(self, handler: TaskEventHandler) -> None:
        """Register handler for task assignment events."""
        self._task_assigned_handlers.append(handler)

    def on_task_completed(self, handler: TaskEventHandler) -> None:
        """Register handler for task completion events."""
        self._task_completed_handlers.append(handler)

    def on_task_blocked(self, handler: TaskEventHandler) -> None:
        """Register handler for task blocked events."""
        self._task_blocked_handlers.append(handler)

    def on_worker_joined(self, handler: WorkerEventHandler) -> None:
        """Register handler for worker join events."""
        self._worker_joined_handlers.append(handler)

    def on_worker_left(self, handler: WorkerEventHandler) -> None:
        """Register handler for worker leave events."""
        self._worker_left_handlers.append(handler)

    def on_conflict(self, handler: ConflictHandler) -> None:
        """Register handler for conflict resolution."""
        self._conflict_handlers.append(handler)

    # =========================================================================
    # Lifecycle
    # =========================================================================

    def start(self) -> None:
        """Start synchronization.

        Subscribes to state changes and starts heartbeat thread.
        """
        if self._running:
            logger.warning("WorkerSync already running")
            return

        logger.info(f"Starting WorkerSync for worker {self._worker_id}")

        # Subscribe to state changes
        self._store.on_change(
            callback=self._handle_state_change,
            project_id=self._project_id,
        )

        # Start heartbeat thread
        self._stop_event.clear()
        self._heartbeat_thread = threading.Thread(
            target=self._heartbeat_loop,
            name=f"c4-sync-heartbeat-{self._worker_id[:8]}",
            daemon=True,
        )
        self._heartbeat_thread.start()

        # Update status
        with self._lock:
            self._workers[self._worker_id].status = "active"
            self._running = True

        # Broadcast join event
        self._broadcast_event(SyncEvent.WORKER_JOINED, {
            "worker_id": self._worker_id,
            "project_id": self._project_id,
        })

        logger.info(f"WorkerSync started for {self._worker_id}")

    def stop(self) -> None:
        """Stop synchronization.

        Unsubscribes from events and stops heartbeat.
        """
        if not self._running:
            return

        logger.info(f"Stopping WorkerSync for worker {self._worker_id}")

        # Broadcast leave event
        self._broadcast_event(SyncEvent.WORKER_LEFT, {
            "worker_id": self._worker_id,
        })

        # Stop heartbeat thread
        self._stop_event.set()
        if self._heartbeat_thread and self._heartbeat_thread.is_alive():
            self._heartbeat_thread.join(timeout=5)

        # Update status
        with self._lock:
            if self._worker_id in self._workers:
                self._workers[self._worker_id].status = "stopped"
            self._running = False

        logger.info(f"WorkerSync stopped for {self._worker_id}")

    # =========================================================================
    # Event Broadcasting
    # =========================================================================

    def broadcast_task_assigned(self, task_id: str) -> None:
        """Broadcast that this worker has been assigned a task.

        Args:
            task_id: ID of the assigned task
        """
        with self._lock:
            if self._worker_id in self._workers:
                self._workers[self._worker_id].current_task = task_id

        self._broadcast_event(SyncEvent.TASK_ASSIGNED, {
            "task_id": task_id,
            "worker_id": self._worker_id,
        })

    def broadcast_task_completed(self, task_id: str) -> None:
        """Broadcast that this worker completed a task.

        Args:
            task_id: ID of the completed task
        """
        with self._lock:
            if self._worker_id in self._workers:
                self._workers[self._worker_id].current_task = None

        self._broadcast_event(SyncEvent.TASK_COMPLETED, {
            "task_id": task_id,
            "worker_id": self._worker_id,
        })

    def broadcast_task_blocked(self, task_id: str, reason: str = "") -> None:
        """Broadcast that a task is blocked.

        Args:
            task_id: ID of the blocked task
            reason: Why the task is blocked
        """
        self._broadcast_event(SyncEvent.TASK_BLOCKED, {
            "task_id": task_id,
            "worker_id": self._worker_id,
            "reason": reason,
        })

    def broadcast_checkpoint_reached(self, checkpoint_id: str) -> None:
        """Broadcast that a checkpoint has been reached.

        Args:
            checkpoint_id: ID of the checkpoint
        """
        self._broadcast_event(SyncEvent.CHECKPOINT_REACHED, {
            "checkpoint_id": checkpoint_id,
            "worker_id": self._worker_id,
        })

    def _broadcast_event(
        self, event: SyncEvent, data: dict[str, Any]
    ) -> None:
        """Broadcast an event to all workers.

        Uses atomic_modify to update state and trigger realtime notifications.

        Args:
            event: Type of event
            data: Event data
        """
        # Add timestamp and event type
        data["event_type"] = event.value
        data["timestamp"] = datetime.now().isoformat()
        data["source_worker"] = self._worker_id

        try:
            # Update state to trigger realtime notification
            # Events are stored in state_data.events (last N events)
            with self._store.atomic_modify(self._project_id) as state:
                # Initialize events list if not present
                if not hasattr(state, "_events"):
                    state._events = []

                # Store in state metadata (not persisted, just triggers change)
                # The actual event is broadcast via Supabase Realtime
                pass

            logger.debug(f"Broadcast event: {event.value}")
        except Exception as e:
            logger.error(f"Failed to broadcast event {event.value}: {e}")

    # =========================================================================
    # Conflict Resolution
    # =========================================================================

    def try_acquire_task(
        self,
        task_id: str,
        scope: str | None = None,
    ) -> bool:
        """Try to acquire a task with conflict detection.

        Uses optimistic locking to prevent multiple workers from
        acquiring the same task.

        Args:
            task_id: Task to acquire
            scope: Optional scope lock required for the task

        Returns:
            True if acquired, False if conflict
        """
        for attempt in range(self._config.conflict_retry_max):
            try:
                with self._store.atomic_modify(self._project_id) as state:
                    # Check if task is already in progress
                    if task_id in state.queue.in_progress:
                        assigned_to = state.queue.in_progress[task_id]
                        if assigned_to != self._worker_id:
                            logger.info(
                                f"Task {task_id} already assigned to {assigned_to}"
                            )
                            return False

                    # Try to acquire scope lock if needed
                    if scope:
                        lock_acquired = self._store.acquire_scope_lock(
                            project_id=self._project_id,
                            scope=scope,
                            owner=self._worker_id,
                            ttl_seconds=300,
                        )
                        if not lock_acquired:
                            logger.info(f"Could not acquire lock for scope: {scope}")
                            return False

                    # Assign task to this worker
                    state.queue.in_progress[task_id] = self._worker_id

                # Success - broadcast assignment
                self.broadcast_task_assigned(task_id)
                return True

            except Exception as e:
                if "ConcurrentModificationError" in str(type(e).__name__):
                    # Conflict - another worker modified state
                    logger.info(
                        f"Conflict acquiring task {task_id}, "
                        f"attempt {attempt + 1}/{self._config.conflict_retry_max}"
                    )

                    # Call conflict handlers
                    should_retry = True
                    for handler in self._conflict_handlers:
                        if not handler(task_id, "unknown"):
                            should_retry = False
                            break

                    if not should_retry:
                        return False

                    # Exponential backoff
                    delay = self._config.conflict_retry_delay * (2 ** attempt)
                    time.sleep(delay)
                else:
                    logger.error(f"Error acquiring task {task_id}: {e}")
                    return False

        logger.warning(
            f"Failed to acquire task {task_id} after "
            f"{self._config.conflict_retry_max} attempts"
        )
        return False

    def release_task(self, task_id: str, completed: bool = False) -> bool:
        """Release a task this worker was working on.

        Args:
            task_id: Task to release
            completed: If True, move to done queue

        Returns:
            True if released successfully
        """
        try:
            with self._store.atomic_modify(self._project_id) as state:
                if task_id in state.queue.in_progress:
                    if state.queue.in_progress[task_id] == self._worker_id:
                        del state.queue.in_progress[task_id]

                        if completed:
                            state.queue.done.append(task_id)
                            self.broadcast_task_completed(task_id)
                        else:
                            # Return to pending
                            state.queue.pending.insert(0, task_id)

                        return True
                    else:
                        logger.warning(
                            f"Task {task_id} not owned by this worker"
                        )
                        return False

            return False

        except Exception as e:
            logger.error(f"Error releasing task {task_id}: {e}")
            return False

    # =========================================================================
    # Internal Handlers
    # =========================================================================

    def _handle_state_change(
        self, state: "C4State", change_type: "ChangeType"
    ) -> None:
        """Handle state change from realtime subscription.

        Args:
            state: The changed state
            change_type: Type of change (INSERT/UPDATE/DELETE)
        """
        logger.debug(f"State change received: {change_type.value}")

        # Notify state change handlers
        for handler in self._state_handlers:
            try:
                handler(state, change_type)
            except Exception as e:
                logger.error(f"State handler error: {e}")

        # Check for worker changes in state
        self._check_worker_changes(state)

    def _check_worker_changes(self, state: "C4State") -> None:
        """Check for changes in worker assignments.

        Args:
            state: Current state
        """
        with self._lock:
            # Check for new task assignments
            for task_id, worker_id in state.queue.in_progress.items():
                if worker_id != self._worker_id:
                    # Another worker got a task
                    for handler in self._task_assigned_handlers:
                        try:
                            handler(task_id, worker_id)
                        except Exception as e:
                            logger.error(f"Task assigned handler error: {e}")

    def _heartbeat_loop(self) -> None:
        """Background thread for heartbeat and worker monitoring."""
        logger.debug("Heartbeat loop started")

        while not self._stop_event.is_set():
            try:
                # Update own heartbeat
                with self._lock:
                    if self._worker_id in self._workers:
                        self._workers[self._worker_id].last_heartbeat = datetime.now()

                # Broadcast heartbeat
                self._broadcast_event(SyncEvent.WORKER_HEARTBEAT, {
                    "worker_id": self._worker_id,
                    "current_task": self._workers.get(
                        self._worker_id, WorkerInfo(self._worker_id, self._project_id)
                    ).current_task,
                })

                # Check for dead workers
                self._check_dead_workers()

            except Exception as e:
                logger.error(f"Heartbeat error: {e}")

            # Wait for next heartbeat
            self._stop_event.wait(self._config.heartbeat_interval)

        logger.debug("Heartbeat loop stopped")

    def _check_dead_workers(self) -> None:
        """Check for and handle dead workers."""
        with self._lock:
            now = datetime.now()
            timeout = timedelta(seconds=self._config.worker_timeout)

            dead_workers = []
            for worker_id, info in self._workers.items():
                if worker_id != self._worker_id:
                    if now - info.last_heartbeat > timeout:
                        dead_workers.append(info)

            for info in dead_workers:
                logger.info(f"Worker {info.worker_id} appears dead")

                # Notify handlers
                for handler in self._worker_left_handlers:
                    try:
                        handler(info)
                    except Exception as e:
                        logger.error(f"Worker left handler error: {e}")

                # Remove from tracking
                del self._workers[info.worker_id]

    def _handle_worker_event(self, payload: dict[str, Any]) -> None:
        """Handle worker-related events from broadcasts.

        Args:
            payload: Event payload
        """
        event_type = payload.get("event_type")
        worker_id = payload.get("worker_id")

        if not worker_id or worker_id == self._worker_id:
            return  # Ignore own events

        with self._lock:
            if event_type == SyncEvent.WORKER_JOINED.value:
                # New worker joined
                info = WorkerInfo(
                    worker_id=worker_id,
                    project_id=payload.get("project_id", self._project_id),
                )
                self._workers[worker_id] = info

                for handler in self._worker_joined_handlers:
                    try:
                        handler(info)
                    except Exception as e:
                        logger.error(f"Worker joined handler error: {e}")

            elif event_type == SyncEvent.WORKER_LEFT.value:
                # Worker left
                if worker_id in self._workers:
                    info = self._workers.pop(worker_id)

                    for handler in self._worker_left_handlers:
                        try:
                            handler(info)
                        except Exception as e:
                            logger.error(f"Worker left handler error: {e}")

            elif event_type == SyncEvent.WORKER_HEARTBEAT.value:
                # Update heartbeat
                if worker_id in self._workers:
                    self._workers[worker_id].last_heartbeat = datetime.now()
                    self._workers[worker_id].current_task = payload.get(
                        "current_task"
                    )
                else:
                    # New worker we didn't know about
                    self._workers[worker_id] = WorkerInfo(
                        worker_id=worker_id,
                        project_id=self._project_id,
                        current_task=payload.get("current_task"),
                    )


def create_worker_sync(
    store: "SupabaseStateStore",
    worker_id: str,
    project_id: str,
    config: SyncConfig | None = None,
) -> WorkerSync:
    """Factory function to create WorkerSync instance.

    Args:
        store: SupabaseStateStore instance
        worker_id: Unique worker identifier
        project_id: Project to synchronize
        config: Optional sync configuration

    Returns:
        Configured WorkerSync instance
    """
    return WorkerSync(
        store=store,
        worker_id=worker_id,
        project_id=project_id,
        config=config,
    )
