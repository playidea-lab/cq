"""Tests for long-running worker detection and handling."""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.daemon.workers import WorkerManager
from c4.models import C4Config, ProjectStatus, WorkerInfo
from c4.models.config import LongRunningConfig


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def mock_state_machine():
    """Create a mock state machine with workers."""
    from c4.models.state import C4State, TaskQueue

    state = C4State(
        project_id="test-project",
        status=ProjectStatus.EXECUTE,
        queue=TaskQueue(),
        workers={},
    )

    mock_sm = MagicMock()
    mock_sm.state = state
    mock_sm.store = MagicMock()
    mock_sm.store.atomic_modify = MagicMock()

    # Make atomic_modify return a context manager that yields state
    mock_sm.store.atomic_modify.return_value.__enter__ = MagicMock(return_value=state)
    mock_sm.store.atomic_modify.return_value.__exit__ = MagicMock(return_value=False)

    return mock_sm


@pytest.fixture
def config():
    """Create a test config."""
    return C4Config(
        project_id="test-project",
        long_running=LongRunningConfig(
            warning_timeout_sec=2400,  # 40 minutes
            stale_timeout_sec=3600,  # 60 minutes
        ),
    )


@pytest.fixture
def worker_manager(mock_state_machine, config):
    """Create a WorkerManager with mocked state machine."""
    return WorkerManager(mock_state_machine, config)


class TestLongRunningConfig:
    """Test LongRunningConfig model."""

    def test_default_values(self):
        """Test default configuration values."""
        config = LongRunningConfig()
        assert config.warning_timeout_sec == 2400  # 40 minutes
        assert config.stale_timeout_sec == 3600  # 60 minutes
        assert config.auto_extend is False
        assert config.auto_recover is False  # Default: no auto-recovery

    def test_custom_values(self):
        """Test custom configuration values."""
        config = LongRunningConfig(
            warning_timeout_sec=1800,
            stale_timeout_sec=3000,
            auto_extend=True,
            auto_recover=True,
        )
        assert config.warning_timeout_sec == 1800
        assert config.stale_timeout_sec == 3000
        assert config.auto_extend is True
        assert config.auto_recover is True

    def test_stale_must_exceed_warning(self):
        """Test validation that stale must exceed warning."""
        with pytest.raises(ValueError, match="must be greater than"):
            LongRunningConfig(
                warning_timeout_sec=3600,
                stale_timeout_sec=2400,  # Less than warning
            )


class TestGetLongRunningAlerts:
    """Test get_long_running_alerts method."""

    def test_no_alerts_for_healthy_workers(self, worker_manager, mock_state_machine):
        """Test no alerts when workers are within timeout."""
        # Add a healthy worker (last seen 10 minutes ago)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=1),
            last_seen=datetime.now() - timedelta(minutes=10),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )

        assert len(alerts) == 0

    def test_alert_for_warning_zone_worker(self, worker_manager, mock_state_machine):
        """Test alert when worker is in warning zone (40-60 min)."""
        # Add a worker in warning zone (45 minutes ago)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=45),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,  # 40 minutes
            stale_timeout_seconds=3600,  # 60 minutes
        )

        assert len(alerts) == 1
        alert = alerts[0]
        assert alert["type"] == "long_running_worker"
        assert alert["worker_id"] == "worker-1"
        assert alert["task_id"] == "T-001-0"
        assert alert["elapsed_minutes"] == 45
        assert len(alert["actions"]) == 3

    def test_no_alert_for_stale_worker(self, worker_manager, mock_state_machine):
        """Test no alert when worker exceeds stale timeout (handled separately)."""
        # Add a stale worker (70 minutes ago)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=70),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,  # 40 minutes
            stale_timeout_seconds=3600,  # 60 minutes
        )

        # No alert - stale workers are handled by recover_stale_workers
        assert len(alerts) == 0

    def test_no_alert_for_idle_workers(self, worker_manager, mock_state_machine):
        """Test no alerts for idle workers."""
        worker = WorkerInfo(
            worker_id="worker-1",
            state="idle",
            task_id=None,
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=50),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )

        assert len(alerts) == 0

    def test_multiple_alerts(self, worker_manager, mock_state_machine):
        """Test multiple alerts for multiple workers."""
        # Add two workers in warning zone
        for i in range(2):
            worker = WorkerInfo(
                worker_id=f"worker-{i}",
                state="busy",
                task_id=f"T-00{i}-0",
                joined_at=datetime.now() - timedelta(hours=2),
                last_seen=datetime.now() - timedelta(minutes=45),
            )
            mock_state_machine.state.workers[f"worker-{i}"] = worker

        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )

        assert len(alerts) == 2


class TestExtendWorkerTimeout:
    """Test extend_worker_timeout method."""

    def test_extend_busy_worker(self, worker_manager, mock_state_machine):
        """Test extending timeout for a busy worker."""
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=45),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        result = worker_manager.extend_worker_timeout("worker-1")

        assert result["success"] is True
        assert result["worker_id"] == "worker-1"
        assert result["task_id"] == "T-001-0"
        assert "extension_minutes" in result

    def test_extend_nonexistent_worker(self, worker_manager):
        """Test extending timeout for non-existent worker."""
        result = worker_manager.extend_worker_timeout("nonexistent")

        assert result["success"] is False
        assert "not found" in result["error"]

    def test_extend_idle_worker(self, worker_manager, mock_state_machine):
        """Test extending timeout for idle worker fails."""
        worker = WorkerInfo(
            worker_id="worker-1",
            state="idle",
            task_id=None,
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=45),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        result = worker_manager.extend_worker_timeout("worker-1")

        assert result["success"] is False
        assert "not busy" in result["error"]


class TestKillWorker:
    """Test kill_worker method."""

    def test_kill_busy_worker(self, worker_manager, mock_state_machine):
        """Test killing a busy worker and recovering task."""
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            scope="src/",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=50),
        )
        mock_state_machine.state.workers["worker-1"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-1"

        result = worker_manager.kill_worker("worker-1")

        assert result["success"] is True
        assert result["worker_id"] == "worker-1"
        assert result["task_id"] == "T-001-0"
        assert result["task_recovered"] is True
        assert worker.state == "disconnected"

    def test_kill_nonexistent_worker(self, worker_manager):
        """Test killing non-existent worker."""
        result = worker_manager.kill_worker("nonexistent")

        assert result["success"] is False
        assert "not found" in result["error"]

    def test_kill_worker_releases_scope_lock(self, worker_manager, mock_state_machine):
        """Test that killing worker releases scope lock."""
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            scope="src/api/",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=50),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        mock_lock_store = MagicMock()
        result = worker_manager.kill_worker("worker-1", lock_store=mock_lock_store)

        assert result["success"] is True
        mock_lock_store.release_scope_lock.assert_called_once_with(
            "test-project", "src/api/"
        )


class TestC4ConfigWithLongRunning:
    """Test C4Config with long_running field."""

    def test_config_has_long_running(self):
        """Test that C4Config includes long_running field."""
        config = C4Config(project_id="test")
        assert hasattr(config, "long_running")
        assert isinstance(config.long_running, LongRunningConfig)

    def test_config_custom_long_running(self):
        """Test custom long_running configuration."""
        config = C4Config(
            project_id="test",
            long_running=LongRunningConfig(
                warning_timeout_sec=1800,
                stale_timeout_sec=2700,
            ),
        )
        assert config.long_running.warning_timeout_sec == 1800
        assert config.long_running.stale_timeout_sec == 2700
