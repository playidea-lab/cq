"""Process-isolated Jedi worker for safe timeout handling.

Key features:
- Worker runs Jedi in separate process
- Source code (str) sent to worker, results (dict) returned
- Timeout triggers SIGTERM/SIGKILL for clean termination
- No ghost threads, no GC recursion errors
- Worker state machine: INIT → HEALTHY → BUSY → DEAD

This solves the fundamental limitations of ThreadPoolExecutor:
1. Threads cannot be forcefully terminated (ghost threads)
2. Jedi objects are not picklable (can't use ProcessPoolExecutor directly)
3. GC recursion errors during cleanup after timeout

The solution: Send source code (str) to worker process, worker creates
Jedi objects internally, returns serializable results (dict).
"""

from __future__ import annotations

import logging
import queue
import threading
import time
import uuid
from enum import Enum
from multiprocessing import Process, Queue
from typing import Any

logger = logging.getLogger(__name__)


class WorkerState(Enum):
    """Worker process state machine states."""

    INIT = "init"
    HEALTHY = "healthy"
    BUSY = "busy"
    DEAD = "dead"


class WorkerDiedError(Exception):
    """Raised when worker process dies unexpectedly."""

    pass


class JediWorkerProcess:
    """Single worker process for Jedi operations.

    Lifecycle:
    1. INIT: Created but not started
    2. HEALTHY: Running and ready for work
    3. BUSY: Processing a request
    4. DEAD: Terminated (timeout, error, or explicit termination)

    Usage:
        worker = JediWorkerProcess("/path/to/project", timeout=5.0)
        worker.start()
        try:
            result = worker.execute({"op": "get_names", "source": "x = 1"})
        except TimeoutError:
            pass  # Worker is now DEAD
        finally:
            worker.terminate()
    """

    def __init__(self, repo_root: str, timeout: float = 30.0):
        """Initialize worker process.

        Args:
            repo_root: Project root path for Jedi context.
            timeout: Maximum execution time in seconds.
        """
        self.repo_root = repo_root
        self.timeout = timeout
        self._process: Process | None = None
        self._input_queue: Queue[dict[str, Any] | None] = Queue()
        self._output_queue: Queue[dict[str, Any]] = Queue()
        self._state: WorkerState = WorkerState.INIT
        self._current_request_id: str | None = None

    @property
    def state(self) -> WorkerState:
        """Get current worker state, checking if process is alive."""
        if self._process and not self._process.is_alive():
            self._state = WorkerState.DEAD
        return self._state

    @property
    def pid(self) -> int | None:
        """Get worker process ID."""
        return self._process.pid if self._process else None

    def start(self) -> None:
        """Start worker process with project context."""
        if self._state != WorkerState.INIT:
            raise RuntimeError(f"Cannot start worker in state {self._state}")

        from c4.lsp.jedi_worker_entry import worker_main

        self._process = Process(
            target=worker_main,
            args=(self._input_queue, self._output_queue, self.repo_root),
            daemon=True,
        )
        self._process.start()
        self._state = WorkerState.HEALTHY
        logger.debug(f"Jedi worker started: pid={self._process.pid}")

    def execute(self, request: dict[str, Any]) -> dict[str, Any]:
        """Execute Jedi operation with timeout and survival check.

        Args:
            request: Operation request dict with keys:
                - op: Operation name (get_names, completions, goto)
                - source: Source code string
                - path: Optional file path
                - options: Operation-specific options

        Returns:
            Response dict with keys:
                - id: Request ID (matches request)
                - ok: Success boolean
                - result: Operation result (if ok)
                - error: Error info (if not ok)
                - stats: Execution stats

        Raises:
            RuntimeError: If worker not healthy
            TimeoutError: If operation times out
            WorkerDiedError: If worker dies during operation
        """
        if self.state != WorkerState.HEALTHY:
            raise RuntimeError(f"Worker not healthy: {self.state}")

        request_id = str(uuid.uuid4())
        request["id"] = request_id
        self._current_request_id = request_id
        self._state = WorkerState.BUSY

        try:
            self._input_queue.put(request)
            return self._wait_for_response(request_id, self.timeout)
        except (TimeoutError, WorkerDiedError):
            self._state = WorkerState.DEAD
            raise
        finally:
            if self._state == WorkerState.BUSY:
                self._state = WorkerState.HEALTHY
            self._current_request_id = None

    def _wait_for_response(self, request_id: str, timeout: float) -> dict[str, Any]:
        """Wait for response while checking worker survival.

        Uses polling with short intervals to detect:
        1. Worker death (process no longer alive)
        2. Response arrival
        3. Timeout expiration

        Args:
            request_id: Expected response ID
            timeout: Maximum wait time in seconds

        Returns:
            Response dict

        Raises:
            WorkerDiedError: If worker dies while waiting
            TimeoutError: If timeout expires
        """
        deadline = time.time() + timeout
        poll_interval = 0.1

        while time.time() < deadline:
            # Check if worker died
            if self._process and not self._process.is_alive():
                raise WorkerDiedError(f"Worker died while processing {request_id}")

            # Check for response
            try:
                response = self._output_queue.get(timeout=poll_interval)
                if response.get("id") == request_id:
                    return response
                else:
                    # Response for different request (shouldn't happen in single-threaded use)
                    logger.warning(
                        f"Received response for {response.get('id')}, expected {request_id}"
                    )
            except queue.Empty:
                continue

        raise TimeoutError(f"Request {request_id} timed out after {timeout}s")

    def terminate(self) -> None:
        """Terminate worker process with 2-stage shutdown.

        Stage 1: SIGTERM (graceful shutdown)
        Stage 2: SIGKILL if still alive after 1 second
        Stage 3: Join to prevent zombie

        This ensures no ghost processes regardless of what Jedi is doing.
        """
        if self._process is None:
            return

        pid = self._process.pid
        logger.debug(f"Terminating Jedi worker: pid={pid}")

        # Stage 1: SIGTERM (graceful)
        try:
            self._process.terminate()
        except Exception as e:
            logger.debug(f"Error during terminate: {e}")

        # Stage 2: Wait briefly
        try:
            self._process.join(timeout=1.0)
        except Exception as e:
            logger.debug(f"Error during join after terminate: {e}")

        # Stage 3: SIGKILL if still alive
        if self._process.is_alive():
            logger.debug(f"Worker {pid} didn't terminate, sending SIGKILL")
            try:
                self._process.kill()
            except Exception as e:
                logger.debug(f"Error during kill: {e}")

            try:
                self._process.join(timeout=1.0)
            except Exception as e:
                logger.debug(f"Error during join after kill: {e}")

        # Cleanup
        self._process = None
        self._state = WorkerState.DEAD
        logger.debug(f"Jedi worker terminated: pid={pid}")

    def __del__(self) -> None:
        """Ensure process is terminated on garbage collection."""
        try:
            self.terminate()
        except Exception:
            pass


class JediWorkerPool:
    """Pool of Jedi workers with state-based management.

    Manages a pool of worker processes:
    - Lazily creates workers on demand
    - Recycles dead workers (timeout/error)
    - Thread-safe for concurrent requests

    Usage:
        pool = JediWorkerPool("/path/to/project", max_workers=2, timeout=3.0)
        try:
            result = pool.execute({"op": "get_names", "source": "x = 1"})
        except TimeoutError:
            pass  # Worker was recycled, next request uses new worker
        finally:
            pool.shutdown()
    """

    def __init__(
        self,
        repo_root: str,
        max_workers: int = 2,
        timeout: float = 3.0,
    ):
        """Initialize worker pool.

        Args:
            repo_root: Project root path for Jedi context.
            max_workers: Maximum concurrent workers.
            timeout: Timeout per operation in seconds.
                     Jedi is best-effort: use short timeouts (1-3s recommended).
        """
        self.repo_root = repo_root
        self._max_workers = max_workers
        self._timeout = timeout
        self._workers: list[JediWorkerProcess] = []
        self._lock = threading.Lock()
        self._shutdown = False

    @property
    def worker_count(self) -> int:
        """Get current worker count."""
        with self._lock:
            return len(self._workers)

    @property
    def healthy_count(self) -> int:
        """Get healthy worker count."""
        with self._lock:
            return sum(1 for w in self._workers if w.state == WorkerState.HEALTHY)

    def execute(self, request: dict[str, Any]) -> dict[str, Any]:
        """Execute operation synchronously on an available worker.

        Args:
            request: Operation request dict.

        Returns:
            Response dict.

        Raises:
            RuntimeError: If pool is shut down or no workers available.
            TimeoutError: If operation times out.
            WorkerDiedError: If worker dies during operation.
        """
        if self._shutdown:
            raise RuntimeError("Worker pool is shut down")

        worker = self._get_healthy_worker()
        try:
            return worker.execute(request)
        except (TimeoutError, WorkerDiedError) as e:
            logger.warning(f"Worker failed, recycling: {e}")
            self._recycle_worker(worker)
            raise

    def _get_healthy_worker(self) -> JediWorkerProcess:
        """Get or create a healthy worker.

        Returns:
            A worker in HEALTHY state.

        Raises:
            RuntimeError: If no workers available and at max capacity.
        """
        with self._lock:
            # Clean up dead workers first
            alive_workers = []
            for w in self._workers:
                if w.state == WorkerState.DEAD:
                    try:
                        w.terminate()
                    except Exception:
                        pass
                else:
                    alive_workers.append(w)
            self._workers = alive_workers

            # Find existing healthy worker
            for w in self._workers:
                if w.state == WorkerState.HEALTHY:
                    return w

            # Create new worker if under limit
            if len(self._workers) < self._max_workers:
                worker = JediWorkerProcess(self.repo_root, self._timeout)
                worker.start()
                self._workers.append(worker)
                return worker

            # All workers busy, wait for one to become available
            # In practice, this shouldn't happen with proper concurrency control
            raise RuntimeError("No available workers and at max capacity")

    def _recycle_worker(self, worker: JediWorkerProcess) -> None:
        """Terminate and remove a worker.

        The next request will create a fresh worker.

        Args:
            worker: Worker to recycle.
        """
        with self._lock:
            try:
                worker.terminate()
            except Exception as e:
                logger.debug(f"Error terminating worker: {e}")

            if worker in self._workers:
                self._workers.remove(worker)

    def shutdown(self) -> None:
        """Shutdown all workers in the pool."""
        with self._lock:
            self._shutdown = True
            for w in self._workers:
                try:
                    w.terminate()
                except Exception as e:
                    logger.debug(f"Error shutting down worker: {e}")
            self._workers.clear()
            logger.info("Jedi worker pool shut down")

    def __enter__(self) -> "JediWorkerPool":
        """Context manager entry."""
        return self

    def __exit__(self, *args: Any) -> None:
        """Context manager exit - shutdown pool."""
        self.shutdown()

    def __del__(self) -> None:
        """Ensure pool is shut down on garbage collection."""
        try:
            self.shutdown()
        except Exception:
            pass
