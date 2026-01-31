"""Tests for parallel LSP operations.

Tests the LSPWorkerPool and parallel symbol search capabilities.
"""

import time
from unittest.mock import MagicMock, patch

import pytest

from c4.lsp.worker_pool import (
    LSPWorkerPool,
    PoolStats,
    TaskPriority,
    WorkerTask,
    get_lsp_worker_pool,
    reset_global_pool,
)


class TestLSPWorkerPool:
    """Tests for LSPWorkerPool."""

    @pytest.fixture
    def pool(self):
        """Create a fresh pool for each test."""
        p = LSPWorkerPool(max_workers=2, default_timeout=5.0)
        yield p
        p.stop(wait=False)

    def test_pool_creation(self, pool):
        """Test pool initialization."""
        assert pool._max_workers == 2
        assert pool._default_timeout == 5.0
        assert not pool._started

    def test_pool_auto_start(self, pool):
        """Test pool auto-starts on first submit."""
        future = pool.submit(lambda: 42)
        assert pool._started
        result = future.result(timeout=1.0)
        assert result == 42

    def test_pool_manual_start_stop(self, pool):
        """Test manual start/stop."""
        pool.start()
        assert pool._started
        assert pool.is_running

        pool.stop(wait=True)
        assert not pool._started
        assert not pool.is_running

    def test_submit_with_priority(self, pool):
        """Test submitting tasks with different priorities."""
        results = []

        # Submit low priority first
        pool.submit(lambda: results.append("low"), priority=TaskPriority.LOW)
        # Submit high priority second
        pool.submit(lambda: results.append("high"), priority=TaskPriority.HIGH)

        # Wait for completion
        time.sleep(0.5)

        # High priority should complete (order may vary due to threading)
        assert "high" in results
        assert "low" in results

    def test_submit_with_args_kwargs(self, pool):
        """Test submitting tasks with arguments."""
        def add(a, b, multiplier=1):
            return (a + b) * multiplier

        future = pool.submit(add, 2, 3, multiplier=2)
        result = future.result(timeout=1.0)
        assert result == 10

    def test_pool_stats(self, pool):
        """Test statistics collection."""
        pool.submit(lambda: 1).result(timeout=1.0)
        pool.submit(lambda: 2).result(timeout=1.0)

        stats = pool.get_stats()
        assert stats.total_submitted >= 2
        assert stats.total_completed >= 2

    def test_pool_status(self, pool):
        """Test status reporting."""
        pool.start()
        status = pool.get_status()

        assert status["started"] is True
        assert status["max_workers"] == 2
        assert "total_submitted" in status

    def test_exception_handling(self, pool):
        """Test that exceptions are propagated."""
        def failing():
            raise ValueError("Test error")

        future = pool.submit(failing)

        with pytest.raises(ValueError, match="Test error"):
            future.result(timeout=1.0)

    def test_concurrent_execution(self, pool):
        """Test that tasks run concurrently."""
        start_times = []

        def record_start():
            start_times.append(time.time())
            time.sleep(0.2)
            return True

        # Submit multiple tasks
        futures = [pool.submit(record_start) for _ in range(2)]

        # Wait for all
        for f in futures:
            f.result(timeout=2.0)

        # Tasks should have started close together (concurrent)
        if len(start_times) >= 2:
            time_diff = abs(start_times[1] - start_times[0])
            assert time_diff < 0.15  # Should start within 150ms of each other


class TestWorkerTask:
    """Tests for WorkerTask dataclass."""

    def test_task_creation(self):
        """Test task initialization."""
        task = WorkerTask(func=lambda: None)
        assert task.priority == TaskPriority.NORMAL
        assert task.timeout is None
        assert task.created_at > 0

    def test_task_priority_ordering(self):
        """Test tasks are ordered by priority."""
        high = WorkerTask(func=lambda: None, priority=TaskPriority.HIGH)
        low = WorkerTask(func=lambda: None, priority=TaskPriority.LOW)

        assert high < low  # Higher priority (lower number) comes first

    def test_task_same_priority_ordering(self):
        """Test tasks with same priority are ordered by creation time."""
        task1 = WorkerTask(func=lambda: None, priority=TaskPriority.NORMAL)
        time.sleep(0.01)
        task2 = WorkerTask(func=lambda: None, priority=TaskPriority.NORMAL)

        assert task1 < task2  # Earlier created comes first


class TestPoolStats:
    """Tests for PoolStats dataclass."""

    def test_initial_stats(self):
        """Test initial stats are zero."""
        stats = PoolStats()
        assert stats.total_submitted == 0
        assert stats.total_completed == 0
        assert stats.total_failed == 0
        assert stats.total_timed_out == 0

    def test_success_rate(self):
        """Test success rate calculation."""
        stats = PoolStats(total_submitted=10, total_completed=8, total_failed=2)
        d = stats.to_dict()
        assert d["success_rate"] == 80.0

    def test_success_rate_zero_submitted(self):
        """Test success rate with no tasks."""
        stats = PoolStats()
        d = stats.to_dict()
        # Should not divide by zero
        assert d["success_rate"] == 0.0


class TestGlobalPool:
    """Tests for global pool management."""

    def setup_method(self):
        """Reset global pool before each test."""
        reset_global_pool()

    def teardown_method(self):
        """Clean up after tests."""
        reset_global_pool()

    def test_get_pool_singleton(self):
        """Test that get_lsp_worker_pool returns same instance."""
        pool1 = get_lsp_worker_pool()
        pool2 = get_lsp_worker_pool()
        assert pool1 is pool2

    def test_reset_global_pool(self):
        """Test resetting the global pool."""
        pool1 = get_lsp_worker_pool()
        pool1.start()

        reset_global_pool()

        pool2 = get_lsp_worker_pool()
        assert pool1 is not pool2
        assert not pool2._started


class TestParallelSymbolSearch:
    """Tests for parallel symbol search operations."""

    @pytest.fixture
    def mock_provider(self):
        """Create a mock unified provider."""
        with patch("c4.lsp.unified_provider.UnifiedSymbolProvider") as MockProvider:
            provider = MagicMock()
            MockProvider.return_value = provider
            yield provider

    def test_parallel_file_analysis_concept(self):
        """Test concept of parallel file analysis.

        This tests the pattern that would be used for parallel workspace search.
        """
        pool = LSPWorkerPool(max_workers=4)

        def analyze_file(file_path: str) -> list:
            # Simulate file analysis
            time.sleep(0.05)
            return [{"name": f"symbol_from_{file_path}"}]

        try:
            pool.start()

            # Simulate parallel analysis of multiple files
            files = [f"file{i}.py" for i in range(4)]
            futures = [
                pool.submit(analyze_file, f, priority=TaskPriority.NORMAL)
                for f in files
            ]

            # Collect results
            results = []
            for future in futures:
                result = future.result(timeout=2.0)
                results.extend(result)

            assert len(results) == 4
            assert all("symbol_from_" in r["name"] for r in results)

        finally:
            pool.stop(wait=False)

    def test_parallel_with_timeout(self):
        """Test that slow tasks timeout properly."""
        pool = LSPWorkerPool(max_workers=2, default_timeout=0.5)

        def slow_task():
            time.sleep(2.0)
            return "done"

        try:
            pool.start()
            future = pool.submit(slow_task)

            # Should timeout waiting for result
            with pytest.raises(Exception):  # TimeoutError or similar
                future.result(timeout=0.3)

        finally:
            pool.stop(wait=False)

    def test_mixed_priority_execution(self):
        """Test mixed priority task execution."""
        pool = LSPWorkerPool(max_workers=1)  # Single worker for predictable order
        execution_order = []

        def record_task(name):
            execution_order.append(name)
            return name

        try:
            pool.start()

            # Submit in reverse priority order
            futures = [
                pool.submit(record_task, "low", priority=TaskPriority.LOW),
                pool.submit(record_task, "normal", priority=TaskPriority.NORMAL),
                pool.submit(record_task, "high", priority=TaskPriority.HIGH),
                pool.submit(record_task, "critical", priority=TaskPriority.CRITICAL),
            ]

            # Wait for all
            for f in futures:
                f.result(timeout=2.0)

            # All tasks should have executed
            assert len(execution_order) == 4
            assert set(execution_order) == {"low", "normal", "high", "critical"}

        finally:
            pool.stop(wait=False)


class TestDefaultMaxWorkers:
    """Tests for DEFAULT_MAX_WORKERS configuration."""

    def test_default_max_workers_value(self):
        """Test DEFAULT_MAX_WORKERS is set based on CPU cores."""
        import os
        from c4.lsp.worker_pool import DEFAULT_MAX_WORKERS

        expected = min(os.cpu_count() or 4, 8)
        assert DEFAULT_MAX_WORKERS == expected
        assert DEFAULT_MAX_WORKERS >= 1
        assert DEFAULT_MAX_WORKERS <= 8

    def test_get_pool_uses_default_workers(self):
        """Test that get_lsp_worker_pool uses DEFAULT_MAX_WORKERS."""
        from c4.lsp.worker_pool import DEFAULT_MAX_WORKERS

        reset_global_pool()
        pool = get_lsp_worker_pool()

        try:
            assert pool._max_workers == DEFAULT_MAX_WORKERS
        finally:
            reset_global_pool()

    def test_get_pool_custom_workers(self):
        """Test that custom worker count can be specified."""
        reset_global_pool()
        pool = get_lsp_worker_pool(max_workers=2)

        try:
            assert pool._max_workers == 2
        finally:
            reset_global_pool()


class TestJediProviderParallelSearch:
    """Tests for parallel symbol search in JediSymbolProvider."""

    @pytest.fixture
    def temp_workspace(self, tmp_path):
        """Create a temporary workspace with Python files."""
        # Create multiple Python files
        for i in range(15):
            file_path = tmp_path / f"module_{i}.py"
            file_path.write_text(f"""
def function_{i}():
    '''Function {i} docstring.'''
    pass

class Class_{i}:
    def method_{i}(self):
        pass
""")
        return tmp_path

    def test_parallel_search_enabled_for_large_searches(self, temp_workspace):
        """Test that parallel search is used for many files."""
        try:
            from c4.lsp.jedi_provider import JediSymbolProvider, JEDI_AVAILABLE
        except ImportError:
            pytest.skip("jedi not available")

        if not JEDI_AVAILABLE:
            pytest.skip("jedi not available")

        provider = JediSymbolProvider(project_path=temp_workspace)

        # Search should use parallel for > 10 files
        results = provider._search_workspace(
            target_name="function_5",
            parent_names=[],
            is_absolute=False,
            parallel=True,
        )

        # Should find the function
        assert len(results) >= 1
        assert any(r.name == "function_5" for r in results)

    def test_sequential_search_for_small_searches(self, temp_workspace):
        """Test that sequential search is used for few files."""
        try:
            from c4.lsp.jedi_provider import JediSymbolProvider, JEDI_AVAILABLE
        except ImportError:
            pytest.skip("jedi not available")

        if not JEDI_AVAILABLE:
            pytest.skip("jedi not available")

        provider = JediSymbolProvider(project_path=temp_workspace)

        # With parallel=False, should use sequential
        results = provider._search_workspace(
            target_name="function_5",
            parent_names=[],
            is_absolute=False,
            parallel=False,
        )

        assert len(results) >= 1

    def test_analyze_single_file_thread_safe(self, tmp_path):
        """Test that _analyze_single_file is thread-safe."""
        try:
            from c4.lsp.jedi_provider import JediSymbolProvider, JEDI_AVAILABLE
        except ImportError:
            pytest.skip("jedi not available")

        if not JEDI_AVAILABLE:
            pytest.skip("jedi not available")

        test_file = tmp_path / "test.py"
        test_file.write_text("""
def target_function():
    pass
""")

        provider = JediSymbolProvider(project_path=tmp_path)

        # Call from multiple threads
        import concurrent.futures

        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            futures = [
                executor.submit(
                    provider._analyze_single_file,
                    test_file,
                    "target_function",
                    5000,
                )
                for _ in range(4)
            ]

            results = [f.result() for f in futures]

        # All should return the same result
        for result in results:
            assert len(result) >= 1
            assert result[0].name == "target_function"
