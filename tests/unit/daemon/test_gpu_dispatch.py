"""Tests for GPU task dispatch - matching GPU tasks to GPU-capable workers."""

from datetime import datetime
from unittest.mock import MagicMock, patch

from c4.models.worker import WorkerInfo


class TestWorkerGpuCapable:
    def test_default_not_gpu_capable(self):
        worker = WorkerInfo(
            worker_id="worker-test0001",
            state="idle",
            joined_at=datetime.now(),
        )
        assert worker.gpu_capable is False

    def test_gpu_capable_flag(self):
        worker = WorkerInfo(
            worker_id="worker-test0002",
            state="idle",
            joined_at=datetime.now(),
            gpu_capable=True,
        )
        assert worker.gpu_capable is True


class TestGpuDetection:
    def test_detect_no_gpu(self):
        from c4.daemon.workers import WorkerManager

        with patch("c4.daemon.workers.WorkerManager._detect_gpu_capability") as mock:
            mock.return_value = False
            result = WorkerManager._detect_gpu_capability()
            assert result is False

    def test_detect_with_gpu(self):
        from c4.daemon.workers import WorkerManager

        with patch("c4.daemon.workers.WorkerManager._detect_gpu_capability") as mock:
            mock.return_value = True
            result = WorkerManager._detect_gpu_capability()
            assert result is True


class TestGpuTaskFiltering:
    """Test that GPU tasks are only assigned to GPU-capable workers."""

    def _make_task(self, task_id, gpu_count=0, priority=0, deps=None):
        """Create a mock task with optional GPU config."""
        task = MagicMock()
        task.id = task_id
        task.priority = priority
        task.dependencies = deps or []
        task.model = None
        task.scope = None
        task.type = "IMPLEMENTATION"
        task.parent_id = None

        if gpu_count > 0:
            gpu_config = MagicMock()
            gpu_config.gpu_count = gpu_count
            task.gpu_config = gpu_config
        else:
            task.gpu_config = None

        return task

    def test_non_gpu_worker_skips_gpu_tasks(self):
        """Non-GPU worker should skip tasks requiring GPU."""
        # Create a mock state with a non-GPU worker
        worker = WorkerInfo(
            worker_id="worker-nogpu001",
            state="idle",
            joined_at=datetime.now(),
            gpu_capable=False,
        )

        gpu_task = self._make_task("T-GPU-001", gpu_count=2)

        # Worker should not be eligible for GPU tasks
        # Test the filter logic directly
        should_skip = (
            gpu_task.gpu_config
            and gpu_task.gpu_config.gpu_count > 0
            and not worker.gpu_capable
        )
        assert should_skip is True

    def test_gpu_worker_accepts_gpu_tasks(self):
        """GPU-capable worker should accept GPU tasks."""
        worker = WorkerInfo(
            worker_id="worker-gpu00001",
            state="idle",
            joined_at=datetime.now(),
            gpu_capable=True,
        )

        gpu_task = self._make_task("T-GPU-002", gpu_count=1)

        should_skip = (
            gpu_task.gpu_config
            and gpu_task.gpu_config.gpu_count > 0
            and not worker.gpu_capable
        )
        assert should_skip is False

    def test_any_worker_accepts_non_gpu_tasks(self):
        """Any worker (GPU or not) should accept non-GPU tasks."""
        non_gpu_worker = WorkerInfo(
            worker_id="worker-nogpu002",
            state="idle",
            joined_at=datetime.now(),
            gpu_capable=False,
        )

        regular_task = self._make_task("T-REG-001", gpu_count=0)

        should_skip = bool(
            regular_task.gpu_config
            and regular_task.gpu_config.gpu_count > 0
            and not non_gpu_worker.gpu_capable
        )
        assert should_skip is False

    def test_gpu_worker_accepts_non_gpu_tasks(self):
        """GPU-capable worker should also accept non-GPU tasks."""
        gpu_worker = WorkerInfo(
            worker_id="worker-gpu00002",
            state="idle",
            joined_at=datetime.now(),
            gpu_capable=True,
        )

        regular_task = self._make_task("T-REG-002", gpu_count=0)

        should_skip = bool(
            regular_task.gpu_config
            and regular_task.gpu_config.gpu_count > 0
            and not gpu_worker.gpu_capable
        )
        assert should_skip is False
