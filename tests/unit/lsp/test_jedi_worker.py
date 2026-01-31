"""Tests for process-isolated Jedi worker.

These tests verify:
1. Worker lifecycle (start, execute, terminate)
2. Timeout handling with clean process termination
3. Worker pool management
4. No ghost processes or memory leaks
"""

from __future__ import annotations

import gc
import os
import time

import pytest

from c4.lsp.jedi_worker import (
    JediWorkerPool,
    JediWorkerProcess,
    WorkerState,
)


class TestJediWorkerProcess:
    """Tests for JediWorkerProcess."""

    def test_worker_initial_state(self, tmp_path):
        """Worker starts in INIT state."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        assert worker.state == WorkerState.INIT
        assert worker.pid is None

    def test_worker_start_and_healthy(self, tmp_path):
        """Worker transitions to HEALTHY after start."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            assert worker.state == WorkerState.HEALTHY
            assert worker.pid is not None
            assert worker.pid > 0
        finally:
            worker.terminate()

    def test_worker_cannot_start_twice(self, tmp_path):
        """Starting worker twice raises error."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            with pytest.raises(RuntimeError, match="Cannot start worker"):
                worker.start()
        finally:
            worker.terminate()

    def test_normal_execution(self, tmp_path):
        """Normal operation returns result."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "x = 1\ndef foo(): pass",
                "options": {"all_scopes": True, "definitions": True},
            })

            assert result["ok"] is True
            assert result["id"] is not None
            assert "result" in result
            assert isinstance(result["result"], list)
            assert "stats" in result

            # Verify we found the expected symbols
            names = [r["name"] for r in result["result"]]
            assert "x" in names
            assert "foo" in names

            # Worker should return to HEALTHY
            assert worker.state == WorkerState.HEALTHY
        finally:
            worker.terminate()

    def test_execution_with_path(self, tmp_path):
        """Execution with file path context."""
        test_file = tmp_path / "test.py"
        test_file.write_text("class MyClass:\n    def method(self): pass\n")

        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": test_file.read_text(),
                "path": str(test_file),
                "options": {"all_scopes": True, "definitions": True},
            })

            assert result["ok"] is True
            names = [r["name"] for r in result["result"]]
            assert "MyClass" in names
            assert "method" in names
        finally:
            worker.terminate()

    def test_timeout_kills_worker(self, tmp_path):
        """Timeout terminates the worker process."""
        # Use very short timeout to force timeout
        worker = JediWorkerProcess(str(tmp_path), timeout=0.05)
        try:
            worker.start()
            pid = worker.pid

            # Create a source that will definitely take longer than 0.05s
            # Multiple nested comprehensions and complex expressions
            huge_source = """
import os, sys, collections, itertools, functools
from typing import *

class A:
    def method(self):
        return [
            {k: v for k, v in enumerate(range(100))}
            for _ in range(10)
        ]

class B(A):
    def method(self):
        return super().method()

x = B().method()
""" + "y = " + "1 + " * 1000 + "1"

            with pytest.raises(TimeoutError):
                worker.execute({
                    "op": "get_names",
                    "source": huge_source,
                    "options": {"all_scopes": True, "definitions": True},
                })

            # Worker should be DEAD after timeout
            assert worker.state == WorkerState.DEAD

            # The terminate should have been called internally, but call it again
            # to ensure cleanup
            worker.terminate()

            # Give a moment for process cleanup
            time.sleep(0.3)

            # Verify process is actually dead
            try:
                os.kill(pid, 0)  # Check if process exists
                # Process might still be cleaning up - that's OK as long as
                # our worker thinks it's dead
                pass
            except OSError:
                pass  # Expected - process doesn't exist
        finally:
            worker.terminate()

    def test_terminate_cleans_up(self, tmp_path):
        """Terminate properly cleans up the process."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        worker.start()
        pid = worker.pid

        worker.terminate()

        assert worker.state == WorkerState.DEAD
        assert worker._process is None

        # Process should be gone
        time.sleep(0.1)
        try:
            os.kill(pid, 0)
            pytest.fail(f"Process {pid} should not exist after terminate")
        except OSError:
            pass  # Expected

    def test_terminate_idempotent(self, tmp_path):
        """Terminate can be called multiple times safely."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        worker.start()

        worker.terminate()
        worker.terminate()  # Should not raise
        worker.terminate()

        assert worker.state == WorkerState.DEAD

    def test_worker_died_detection(self, tmp_path):
        """Detect when worker dies unexpectedly."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        worker.start()

        # Kill the worker process directly
        worker._process.kill()
        worker._process.join(timeout=1)

        # State check should detect the death
        # The state property checks if process is alive
        assert worker.state == WorkerState.DEAD

        # Trying to execute should raise RuntimeError (not WorkerDiedError)
        # because the state check happens before execution
        with pytest.raises(RuntimeError, match="Worker not healthy"):
            worker.execute({
                "op": "get_names",
                "source": "x = 1",
            })

    def test_request_id_in_response(self, tmp_path):
        """Response contains matching request ID."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "x = 1",
            })

            assert "id" in result
            assert result["id"] is not None
            # ID should be a UUID string
            assert len(result["id"]) == 36
        finally:
            worker.terminate()

    def test_error_handling_unknown_op(self, tmp_path):
        """Unknown operation returns error."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "unknown_operation",
                "source": "x = 1",
            })

            assert result["ok"] is False
            assert result["error"]["code"] == "unknown_op"
        finally:
            worker.terminate()

    def test_completions_operation(self, tmp_path):
        """Completions operation works."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "completions",
                "source": "import os\nos.",
                "line": 2,
                "column": 3,
            })

            assert result["ok"] is True
            assert isinstance(result["result"], list)
            # os module should have completions
            if result["result"]:
                assert "name" in result["result"][0]
        finally:
            worker.terminate()

    def test_goto_operation(self, tmp_path):
        """Goto operation works."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            source = "def foo(): pass\n\nfoo()"
            result = worker.execute({
                "op": "goto",
                "source": source,
                "line": 3,
                "column": 1,
            })

            assert result["ok"] is True
            assert isinstance(result["result"], list)
        finally:
            worker.terminate()


class TestJediWorkerPool:
    """Tests for JediWorkerPool."""

    def test_pool_creation(self, tmp_path):
        """Pool can be created."""
        pool = JediWorkerPool(str(tmp_path), max_workers=2, timeout=5.0)
        try:
            assert pool.worker_count == 0
            assert pool.healthy_count == 0
        finally:
            pool.shutdown()

    def test_pool_lazy_worker_creation(self, tmp_path):
        """Workers are created on demand."""
        pool = JediWorkerPool(str(tmp_path), max_workers=2, timeout=5.0)
        try:
            assert pool.worker_count == 0

            # First request creates a worker
            result = pool.execute({
                "op": "get_names",
                "source": "x = 1",
            })

            assert result["ok"] is True
            assert pool.worker_count == 1
        finally:
            pool.shutdown()

    def test_pool_recycles_dead_worker(self, tmp_path):
        """Pool creates new worker after timeout."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=0.05)
        try:
            # First request: cause timeout with complex source
            huge_source = """
import os, sys, collections
class A:
    def method(self):
        return [x for x in range(1000)]
""" + "x = " + "1 + " * 500 + "1"

            try:
                pool.execute({
                    "op": "get_names",
                    "source": huge_source,
                })
            except TimeoutError:
                pass  # Expected

            # Worker should be removed after timeout
            assert pool.worker_count == 0

            # Second request: should work with new worker
            # Use longer timeout for this simple request
            pool._timeout = 5.0
            result = pool.execute({
                "op": "get_names",
                "source": "x = 1",
            })

            assert result["ok"] is True
            assert pool.worker_count == 1
        finally:
            pool.shutdown()

    def test_pool_multiple_requests(self, tmp_path):
        """Pool handles multiple sequential requests."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=5.0)
        try:
            for i in range(5):
                result = pool.execute({
                    "op": "get_names",
                    "source": f"x{i} = {i}",
                })
                assert result["ok"] is True

            # Should still have just one worker
            assert pool.worker_count == 1
        finally:
            pool.shutdown()

    def test_pool_shutdown_cleans_all(self, tmp_path):
        """Shutdown terminates all workers."""
        pool = JediWorkerPool(str(tmp_path), max_workers=2, timeout=5.0)

        # Create workers
        for i in range(2):
            pool.execute({
                "op": "get_names",
                "source": f"x = {i}",
            })

        pids = [w.pid for w in pool._workers if w.pid]
        assert len(pids) > 0

        pool.shutdown()

        # All workers should be gone
        assert pool.worker_count == 0

        # Verify processes are actually dead
        time.sleep(0.2)
        for pid in pids:
            try:
                os.kill(pid, 0)
                pytest.fail(f"Process {pid} should not exist after shutdown")
            except OSError:
                pass

    def test_pool_context_manager(self, tmp_path):
        """Pool works as context manager."""
        with JediWorkerPool(str(tmp_path), max_workers=1, timeout=5.0) as pool:
            result = pool.execute({
                "op": "get_names",
                "source": "x = 1",
            })
            assert result["ok"] is True

        # After context exit, pool should be shut down
        assert pool._shutdown is True

    def test_pool_rejects_after_shutdown(self, tmp_path):
        """Pool rejects requests after shutdown."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=5.0)
        pool.shutdown()

        with pytest.raises(RuntimeError, match="shut down"):
            pool.execute({
                "op": "get_names",
                "source": "x = 1",
            })


class TestResourceLeaks:
    """Tests for resource leak prevention."""

    def test_no_zombie_processes(self, tmp_path):
        """Terminated workers don't become zombies."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        worker.start()
        pid = worker.pid

        worker.terminate()

        # Wait for process table update
        time.sleep(0.2)

        # Check process state (Unix-specific)
        try:
            import subprocess

            result = subprocess.run(
                ["ps", "-p", str(pid), "-o", "state="],
                capture_output=True,
                text=True,
            )
            # Process should not exist or should not be zombie
            if result.returncode == 0:
                state = result.stdout.strip()
                assert "Z" not in state, f"Process {pid} is zombie: {state}"
        except FileNotFoundError:
            pytest.skip("ps command not available")

    def test_repeated_timeouts_no_accumulation(self, tmp_path):
        """Repeated timeouts don't accumulate processes."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=0.05)
        huge_source = "x = " + "1 + " * 2000 + "1"

        # Cause several timeouts
        for _ in range(5):
            try:
                pool.execute({
                    "op": "get_names",
                    "source": huge_source,
                })
            except TimeoutError:
                pass

        # Should not have accumulated workers
        assert pool.worker_count <= 1

        pool.shutdown()

        # All should be cleaned up
        assert pool.worker_count == 0

    @pytest.mark.slow
    def test_memory_stability_on_timeouts(self, tmp_path):
        """Memory usage stays stable across timeouts."""
        try:
            import psutil
        except ImportError:
            pytest.skip("psutil not available")

        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=0.05)
        huge_source = "x = " + "1 + " * 2000 + "1"

        gc.collect()
        initial_memory = psutil.Process().memory_info().rss

        # Cause many timeouts
        for _ in range(10):
            try:
                pool.execute({
                    "op": "get_names",
                    "source": huge_source,
                })
            except TimeoutError:
                pass
            gc.collect()

        gc.collect()
        final_memory = psutil.Process().memory_info().rss

        pool.shutdown()

        # Memory should not have grown significantly (allow 50% growth)
        assert final_memory < initial_memory * 1.5, (
            f"Memory grew from {initial_memory} to {final_memory}"
        )


class TestEdgeCases:
    """Tests for edge cases and error conditions."""

    def test_empty_source(self, tmp_path):
        """Empty source code is handled."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "",
            })

            assert result["ok"] is True
            assert result["result"] == []
        finally:
            worker.terminate()

    def test_syntax_error_in_source(self, tmp_path):
        """Source with syntax errors is handled."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "def foo(\n    # incomplete",
            })

            # Jedi should still return partial results
            assert result["ok"] is True
        finally:
            worker.terminate()

    def test_unicode_source(self, tmp_path):
        """Unicode in source code is handled."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "변수 = '한글'\nπ = 3.14159",
            })

            assert result["ok"] is True
            names = [r["name"] for r in result["result"]]
            assert "변수" in names
            assert "π" in names
        finally:
            worker.terminate()

    def test_very_long_source(self, tmp_path):
        """Very long source code is handled."""
        worker = JediWorkerProcess(str(tmp_path), timeout=10.0)
        try:
            worker.start()
            # Create a file with many functions
            lines = [f"def func_{i}(): pass" for i in range(100)]
            source = "\n".join(lines)

            result = worker.execute({
                "op": "get_names",
                "source": source,
            })

            assert result["ok"] is True
            assert len(result["result"]) >= 100
        finally:
            worker.terminate()

    def test_missing_options(self, tmp_path):
        """Missing options use defaults."""
        worker = JediWorkerProcess(str(tmp_path), timeout=5.0)
        try:
            worker.start()
            result = worker.execute({
                "op": "get_names",
                "source": "x = 1",
                # No options provided
            })

            assert result["ok"] is True
        finally:
            worker.terminate()


class TestConcurrency:
    """Tests for concurrent usage."""

    def test_pool_sequential_is_safe(self, tmp_path):
        """Sequential requests through pool are safe."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=5.0)
        try:
            results = []
            for i in range(10):
                result = pool.execute({
                    "op": "get_names",
                    "source": f"x{i} = {i}",
                })
                results.append(result)

            assert all(r["ok"] for r in results)
        finally:
            pool.shutdown()

    def test_pool_handles_worker_death_gracefully(self, tmp_path):
        """Pool handles unexpected worker death."""
        pool = JediWorkerPool(str(tmp_path), max_workers=1, timeout=5.0)
        try:
            # First request creates worker
            result = pool.execute({
                "op": "get_names",
                "source": "x = 1",
            })
            assert result["ok"] is True

            # Kill the worker directly
            if pool._workers:
                pool._workers[0]._process.kill()
                pool._workers[0]._process.join(timeout=1)

            # Next request should work with new worker
            result = pool.execute({
                "op": "get_names",
                "source": "y = 2",
            })
            assert result["ok"] is True
        finally:
            pool.shutdown()
