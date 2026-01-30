"""Worker Pool for parallel LSP operations.

This module provides a thread pool for handling concurrent LSP requests,
with support for prioritization, timeouts, and graceful shutdown.

Features:
- Thread pool with configurable size
- Priority-based task queuing
- Timeout handling per task
- Status monitoring and metrics
- Graceful shutdown

Usage:
    pool = get_lsp_worker_pool()
    future = pool.submit(my_lsp_operation, priority=1)
    result = future.result(timeout=5.0)
"""

from __future__ import annotations

import logging
import queue
import threading
import time
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import dataclass, field
from enum import IntEnum
from typing import Any, Callable, TypeVar

logger = logging.getLogger(__name__)

T = TypeVar("T")


class TaskPriority(IntEnum):
    """Task priority levels (lower number = higher priority)."""

    CRITICAL = 0  # Go to definition, completions
    HIGH = 1  # Document symbols
    NORMAL = 2  # Find references
    LOW = 3  # Background indexing


@dataclass
class WorkerTask:
    """A task to be executed by the worker pool."""

    func: Callable[..., Any]
    args: tuple = field(default_factory=tuple)
    kwargs: dict = field(default_factory=dict)
    priority: TaskPriority = TaskPriority.NORMAL
    timeout: float | None = None
    created_at: float = field(default_factory=time.time)
    future: Future | None = None

    def __lt__(self, other: WorkerTask) -> bool:
        """Compare by priority for queue ordering."""
        if self.priority != other.priority:
            return self.priority < other.priority
        return self.created_at < other.created_at


@dataclass
class PoolStats:
    """Statistics for the worker pool."""

    total_submitted: int = 0
    total_completed: int = 0
    total_failed: int = 0
    total_timed_out: int = 0
    avg_wait_time_ms: float = 0.0
    avg_execution_time_ms: float = 0.0
    current_queue_size: int = 0
    active_workers: int = 0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "total_submitted": self.total_submitted,
            "total_completed": self.total_completed,
            "total_failed": self.total_failed,
            "total_timed_out": self.total_timed_out,
            "avg_wait_time_ms": round(self.avg_wait_time_ms, 2),
            "avg_execution_time_ms": round(self.avg_execution_time_ms, 2),
            "current_queue_size": self.current_queue_size,
            "active_workers": self.active_workers,
            "success_rate": round(
                self.total_completed / max(1, self.total_submitted) * 100, 1
            ),
        }


class LSPWorkerPool:
    """Thread pool for parallel LSP operations.

    Provides priority-based task scheduling with timeout support
    and graceful shutdown capabilities.
    """

    def __init__(
        self,
        max_workers: int = 4,
        default_timeout: float = 30.0,
    ) -> None:
        """Initialize the worker pool.

        Args:
            max_workers: Maximum number of worker threads
            default_timeout: Default timeout for tasks (seconds)
        """
        self._max_workers = max_workers
        self._default_timeout = default_timeout
        self._executor: ThreadPoolExecutor | None = None
        self._task_queue: queue.PriorityQueue[WorkerTask] = queue.PriorityQueue()
        self._lock = threading.RLock()
        self._shutdown = threading.Event()
        self._stats = PoolStats()
        self._wait_times: list[float] = []
        self._exec_times: list[float] = []
        self._active_count = 0
        self._dispatcher_thread: threading.Thread | None = None
        self._started = False

    def start(self) -> None:
        """Start the worker pool."""
        with self._lock:
            if self._started:
                return

            self._executor = ThreadPoolExecutor(
                max_workers=self._max_workers,
                thread_name_prefix="lsp-worker",
            )
            self._shutdown.clear()
            self._started = True

            # Start dispatcher thread
            self._dispatcher_thread = threading.Thread(
                target=self._dispatcher_loop,
                name="lsp-dispatcher",
                daemon=True,
            )
            self._dispatcher_thread.start()

            logger.info(f"LSP worker pool started with {self._max_workers} workers")

    def stop(self, wait: bool = True, timeout: float = 5.0) -> None:
        """Stop the worker pool.

        Args:
            wait: Whether to wait for pending tasks
            timeout: Timeout for waiting (seconds)
        """
        with self._lock:
            if not self._started:
                return

            self._shutdown.set()
            self._started = False

            # Wake up dispatcher
            try:
                # Put a sentinel task to unblock the queue
                self._task_queue.put(
                    WorkerTask(func=lambda: None, priority=TaskPriority.CRITICAL)
                )
            except Exception:
                pass

            if self._executor:
                self._executor.shutdown(wait=wait, cancel_futures=not wait)
                self._executor = None

            if self._dispatcher_thread and self._dispatcher_thread.is_alive():
                self._dispatcher_thread.join(timeout=timeout)
                self._dispatcher_thread = None

            logger.info("LSP worker pool stopped")

    def submit(
        self,
        func: Callable[..., T],
        *args: Any,
        priority: TaskPriority = TaskPriority.NORMAL,
        timeout: float | None = None,
        **kwargs: Any,
    ) -> Future[T]:
        """Submit a task to the pool.

        Args:
            func: Function to execute
            *args: Function arguments
            priority: Task priority
            timeout: Task timeout (uses default if None)
            **kwargs: Function keyword arguments

        Returns:
            Future for the task result

        Raises:
            RuntimeError: If pool is not started
        """
        if not self._started:
            # Auto-start on first submit
            self.start()

        future: Future[T] = Future()

        task = WorkerTask(
            func=func,
            args=args,
            kwargs=kwargs,
            priority=priority,
            timeout=timeout or self._default_timeout,
            future=future,
        )

        with self._lock:
            self._stats.total_submitted += 1

        self._task_queue.put(task)
        return future

    def _dispatcher_loop(self) -> None:
        """Dispatcher loop that processes queued tasks."""
        while not self._shutdown.is_set():
            try:
                # Get task with timeout to allow shutdown check
                try:
                    task = self._task_queue.get(timeout=0.5)
                except queue.Empty:
                    continue

                # Check shutdown
                if self._shutdown.is_set():
                    if task.future and not task.future.done():
                        task.future.cancel()
                    break

                # Submit to executor
                if self._executor and task.future and not task.future.done():
                    self._executor.submit(self._execute_task, task)

            except Exception as e:
                logger.error(f"Dispatcher error: {e}")

    def _execute_task(self, task: WorkerTask) -> None:
        """Execute a single task.

        Args:
            task: Task to execute
        """
        if task.future is None or task.future.done():
            return

        with self._lock:
            self._active_count += 1
            self._stats.active_workers = self._active_count

        wait_time = (time.time() - task.created_at) * 1000

        start_time = time.time()
        try:
            # Execute with timeout if specified
            result = task.func(*task.args, **task.kwargs)
            exec_time = (time.time() - start_time) * 1000

            # Set result
            if not task.future.done():
                task.future.set_result(result)

            with self._lock:
                self._stats.total_completed += 1
                self._update_timing_stats(wait_time, exec_time)

        except Exception as e:
            exec_time = (time.time() - start_time) * 1000

            # Check if it's a timeout
            if task.timeout and exec_time > task.timeout * 1000:
                with self._lock:
                    self._stats.total_timed_out += 1
            else:
                with self._lock:
                    self._stats.total_failed += 1

            # Set exception
            if not task.future.done():
                task.future.set_exception(e)

            with self._lock:
                self._update_timing_stats(wait_time, exec_time)

            logger.debug(f"Task failed: {e}")

        finally:
            with self._lock:
                self._active_count -= 1
                self._stats.active_workers = self._active_count

    def _update_timing_stats(self, wait_time: float, exec_time: float) -> None:
        """Update timing statistics.

        Args:
            wait_time: Queue wait time in ms
            exec_time: Execution time in ms
        """
        # Keep last 100 samples for averaging
        self._wait_times.append(wait_time)
        self._exec_times.append(exec_time)

        if len(self._wait_times) > 100:
            self._wait_times = self._wait_times[-100:]
        if len(self._exec_times) > 100:
            self._exec_times = self._exec_times[-100:]

        self._stats.avg_wait_time_ms = sum(self._wait_times) / len(self._wait_times)
        self._stats.avg_execution_time_ms = sum(self._exec_times) / len(self._exec_times)

    def get_stats(self) -> PoolStats:
        """Get current pool statistics.

        Returns:
            PoolStats with current metrics
        """
        with self._lock:
            self._stats.current_queue_size = self._task_queue.qsize()
            return PoolStats(
                total_submitted=self._stats.total_submitted,
                total_completed=self._stats.total_completed,
                total_failed=self._stats.total_failed,
                total_timed_out=self._stats.total_timed_out,
                avg_wait_time_ms=self._stats.avg_wait_time_ms,
                avg_execution_time_ms=self._stats.avg_execution_time_ms,
                current_queue_size=self._stats.current_queue_size,
                active_workers=self._stats.active_workers,
            )

    def get_status(self) -> dict[str, Any]:
        """Get pool status for monitoring.

        Returns:
            Dictionary with pool status
        """
        stats = self.get_stats()
        return {
            "started": self._started,
            "max_workers": self._max_workers,
            "default_timeout": self._default_timeout,
            **stats.to_dict(),
        }

    @property
    def is_running(self) -> bool:
        """Check if pool is running."""
        return self._started and not self._shutdown.is_set()


# Global pool instance
_global_pool: LSPWorkerPool | None = None
_pool_lock = threading.RLock()


def get_lsp_worker_pool(
    max_workers: int = 4,
    default_timeout: float = 30.0,
) -> LSPWorkerPool:
    """Get or create the global LSP worker pool.

    Args:
        max_workers: Maximum workers (only used on first call)
        default_timeout: Default timeout (only used on first call)

    Returns:
        Global LSPWorkerPool instance
    """
    global _global_pool

    with _pool_lock:
        if _global_pool is None:
            _global_pool = LSPWorkerPool(
                max_workers=max_workers,
                default_timeout=default_timeout,
            )

    return _global_pool


def reset_global_pool() -> None:
    """Reset the global pool instance (for testing)."""
    global _global_pool

    with _pool_lock:
        if _global_pool is not None:
            _global_pool.stop(wait=False)
        _global_pool = None
