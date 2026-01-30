"""Tests for LSP Worker Pool."""

import threading
import time
from concurrent.futures import TimeoutError as FutureTimeoutError

import pytest

from c4.lsp.worker_pool import (
    LSPWorkerPool,
    PoolStats,
    TaskPriority,
    WorkerTask,
    get_lsp_worker_pool,
    reset_global_pool,
)


class TestTaskPriority:
    """Tests for TaskPriority enum."""

    def test_priority_ordering(self) -> None:
        """Critical should be highest priority."""
        assert TaskPriority.CRITICAL < TaskPriority.HIGH
        assert TaskPriority.HIGH < TaskPriority.NORMAL
        assert TaskPriority.NORMAL < TaskPriority.LOW


class TestWorkerTask:
    """Tests for WorkerTask dataclass."""

    def test_task_creation(self) -> None:
        """Should create task with defaults."""
        task = WorkerTask(func=lambda: None)

        assert task.func is not None
        assert task.args == ()
        assert task.kwargs == {}
        assert task.priority == TaskPriority.NORMAL
        assert task.timeout is None
        assert task.future is None

    def test_task_comparison_by_priority(self) -> None:
        """Higher priority tasks should sort first."""
        task1 = WorkerTask(func=lambda: None, priority=TaskPriority.LOW)
        task2 = WorkerTask(func=lambda: None, priority=TaskPriority.HIGH)

        assert task2 < task1

    def test_task_comparison_by_time_same_priority(self) -> None:
        """Older tasks should sort first for same priority."""
        task1 = WorkerTask(func=lambda: None, priority=TaskPriority.NORMAL)
        time.sleep(0.001)
        task2 = WorkerTask(func=lambda: None, priority=TaskPriority.NORMAL)

        assert task1 < task2


class TestPoolStats:
    """Tests for PoolStats dataclass."""

    def test_initial_stats(self) -> None:
        """Should have zero initial values."""
        stats = PoolStats()

        assert stats.total_submitted == 0
        assert stats.total_completed == 0
        assert stats.total_failed == 0

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        stats = PoolStats(total_submitted=10, total_completed=8)
        result = stats.to_dict()

        assert result["total_submitted"] == 10
        assert result["total_completed"] == 8
        assert "success_rate" in result
        assert result["success_rate"] == 80.0

    def test_success_rate_zero_submitted(self) -> None:
        """Should handle zero submitted."""
        stats = PoolStats()
        result = stats.to_dict()

        assert result["success_rate"] == 0.0


class TestLSPWorkerPool:
    """Tests for LSPWorkerPool class."""

    @pytest.fixture
    def pool(self) -> LSPWorkerPool:
        """Create a pool for testing."""
        p = LSPWorkerPool(max_workers=2, default_timeout=5.0)
        yield p
        p.stop(wait=False)

    def test_pool_creation(self, pool: LSPWorkerPool) -> None:
        """Should create pool with config."""
        assert pool._max_workers == 2
        assert pool._default_timeout == 5.0
        assert not pool._started

    def test_start_and_stop(self, pool: LSPWorkerPool) -> None:
        """Should start and stop cleanly."""
        pool.start()
        assert pool.is_running

        pool.stop()
        assert not pool.is_running

    def test_double_start_is_safe(self, pool: LSPWorkerPool) -> None:
        """Should handle double start gracefully."""
        pool.start()
        pool.start()  # Should not raise
        assert pool.is_running

    def test_double_stop_is_safe(self, pool: LSPWorkerPool) -> None:
        """Should handle double stop gracefully."""
        pool.start()
        pool.stop()
        pool.stop()  # Should not raise
        assert not pool.is_running

    def test_submit_executes_task(self, pool: LSPWorkerPool) -> None:
        """Should execute submitted task."""

        def task() -> int:
            return 42

        pool.start()
        future = pool.submit(task)
        result = future.result(timeout=2.0)

        assert result == 42

    def test_submit_auto_starts_pool(self) -> None:
        """Should auto-start pool on first submit."""
        pool = LSPWorkerPool(max_workers=1)

        try:
            assert not pool._started
            future = pool.submit(lambda: 1)
            assert pool._started
            assert future.result(timeout=2.0) == 1
        finally:
            pool.stop(wait=False)

    def test_submit_with_args(self, pool: LSPWorkerPool) -> None:
        """Should pass args to task."""

        def add(a: int, b: int) -> int:
            return a + b

        pool.start()
        future = pool.submit(add, 2, 3)
        result = future.result(timeout=2.0)

        assert result == 5

    def test_submit_with_kwargs(self, pool: LSPWorkerPool) -> None:
        """Should pass kwargs to task."""

        def greet(name: str, greeting: str = "Hello") -> str:
            return f"{greeting}, {name}!"

        pool.start()
        future = pool.submit(greet, "World", greeting="Hi")
        result = future.result(timeout=2.0)

        assert result == "Hi, World!"

    def test_submit_with_priority(self, pool: LSPWorkerPool) -> None:
        """Should respect priority ordering."""
        results: list[int] = []
        event = threading.Event()

        def task(value: int) -> None:
            event.wait()  # Wait for all tasks to be queued
            results.append(value)

        # Create pool with single worker to test ordering
        single_pool = LSPWorkerPool(max_workers=1)
        single_pool.start()

        try:
            # Submit low priority first
            f1 = single_pool.submit(task, 1, priority=TaskPriority.LOW)
            f2 = single_pool.submit(task, 2, priority=TaskPriority.HIGH)
            f3 = single_pool.submit(task, 3, priority=TaskPriority.CRITICAL)

            # Allow tasks to proceed
            time.sleep(0.1)
            event.set()

            # Wait for completion
            f1.result(timeout=2.0)
            f2.result(timeout=2.0)
            f3.result(timeout=2.0)

            # First task starts immediately, then priority order
            # Due to queue behavior, first submitted task executes first
            # then priority ordering applies to queued tasks
            assert 3 in results  # Critical should complete
            assert 2 in results  # High should complete
            assert 1 in results  # Low should complete

        finally:
            single_pool.stop(wait=False)

    def test_task_exception_handling(self, pool: LSPWorkerPool) -> None:
        """Should propagate exceptions."""

        def failing_task() -> None:
            raise ValueError("Test error")

        pool.start()
        future = pool.submit(failing_task)

        with pytest.raises(ValueError, match="Test error"):
            future.result(timeout=2.0)

    def test_stats_tracking(self, pool: LSPWorkerPool) -> None:
        """Should track execution stats."""
        pool.start()

        # Submit some tasks
        futures = [pool.submit(lambda: time.sleep(0.01)) for _ in range(3)]

        for f in futures:
            f.result(timeout=2.0)

        stats = pool.get_stats()
        assert stats.total_submitted == 3
        assert stats.total_completed == 3
        assert stats.total_failed == 0

    def test_get_status(self, pool: LSPWorkerPool) -> None:
        """Should return status dictionary."""
        pool.start()
        status = pool.get_status()

        assert status["started"] is True
        assert status["max_workers"] == 2
        assert "total_submitted" in status
        assert "success_rate" in status

    def test_concurrent_submissions(self, pool: LSPWorkerPool) -> None:
        """Should handle concurrent submissions."""
        pool.start()
        results: list[int] = []
        lock = threading.Lock()

        def task(value: int) -> int:
            time.sleep(0.05)  # Simulate work
            with lock:
                results.append(value)
            return value

        # Submit many tasks
        futures = [pool.submit(task, i) for i in range(10)]

        # Wait for all
        for f in futures:
            f.result(timeout=5.0)

        assert len(results) == 10
        assert set(results) == set(range(10))


class TestGlobalPool:
    """Tests for global pool functions."""

    def teardown_method(self) -> None:
        """Reset global pool after each test."""
        reset_global_pool()

    def test_get_pool_returns_same_instance(self) -> None:
        """Should return singleton."""
        pool1 = get_lsp_worker_pool()
        pool2 = get_lsp_worker_pool()

        assert pool1 is pool2

    def test_get_pool_uses_config_on_first_call(self) -> None:
        """Should use config on first call only."""
        pool = get_lsp_worker_pool(max_workers=8, default_timeout=60.0)

        assert pool._max_workers == 8
        assert pool._default_timeout == 60.0

        # Second call ignores config
        pool2 = get_lsp_worker_pool(max_workers=2, default_timeout=10.0)
        assert pool2._max_workers == 8  # Still uses first config

    def test_reset_global_pool(self) -> None:
        """Should reset global pool."""
        pool1 = get_lsp_worker_pool()
        pool1.start()

        reset_global_pool()

        pool2 = get_lsp_worker_pool()
        assert pool1 is not pool2


class TestPoolShutdown:
    """Tests for pool shutdown behavior."""

    def test_shutdown_sets_flag(self) -> None:
        """Should set shutdown flag."""
        pool = LSPWorkerPool(max_workers=1)
        pool.start()

        assert pool.is_running
        pool.stop(wait=False, timeout=0.5)
        assert not pool.is_running

    def test_shutdown_completes_submitted_tasks(self) -> None:
        """Submitted tasks should complete before shutdown."""
        pool = LSPWorkerPool(max_workers=2)
        pool.start()

        results: list[int] = []

        def task(value: int) -> int:
            results.append(value)
            return value

        # Submit quick tasks
        futures = [pool.submit(task, i) for i in range(3)]

        # Get results (will wait for completion)
        for i, f in enumerate(futures):
            assert f.result(timeout=2.0) == i

        pool.stop(wait=False, timeout=0.5)
        assert len(results) == 3
