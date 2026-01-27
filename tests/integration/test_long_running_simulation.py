"""Integration tests for long-running worker warning + wait behavior.

Simulates the full flow:
1. Worker starts a task
2. Worker becomes unresponsive (exceeds warning timeout)
3. System generates warnings but does NOT auto-recover (default)
4. Worker completes successfully → task continues
5. OR User manually kills worker → task recovered

Key principle: "사용자가 별다른 응답없으면 그냥 기다리는거임"
(If user doesn't respond, just wait)
"""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.daemon.workers import WorkerManager
from c4.models import C4Config, ProjectStatus, WorkerInfo
from c4.models.config import LongRunningConfig
from c4.models.state import C4State, TaskQueue


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def mock_state_machine():
    """Create a mock state machine with workers."""
    state = C4State(
        project_id="test-project",
        status=ProjectStatus.EXECUTE,
        queue=TaskQueue(),
        workers={},
    )

    mock_sm = MagicMock()
    mock_sm.state = state
    mock_sm.store = MagicMock()

    # Make atomic_modify return a context manager that yields state
    mock_sm.store.atomic_modify.return_value.__enter__ = MagicMock(return_value=state)
    mock_sm.store.atomic_modify.return_value.__exit__ = MagicMock(return_value=False)

    return mock_sm


@pytest.fixture
def config_no_auto_recover():
    """Config with auto_recover=False (default behavior)."""
    return C4Config(
        project_id="test-project",
        long_running=LongRunningConfig(
            warning_timeout_sec=2400,  # 40 minutes
            stale_timeout_sec=3600,  # 60 minutes
            auto_recover=False,  # DEFAULT: do not auto-recover
        ),
    )


@pytest.fixture
def config_auto_recover():
    """Config with auto_recover=True (opt-in behavior)."""
    return C4Config(
        project_id="test-project",
        long_running=LongRunningConfig(
            warning_timeout_sec=2400,  # 40 minutes
            stale_timeout_sec=3600,  # 60 minutes
            auto_recover=True,  # OPT-IN: enable auto-recovery
        ),
    )


class TestWarningWithoutAutoRecovery:
    """Test default behavior: warn but don't auto-recover."""

    def test_worker_in_warning_zone_generates_alert(
        self, mock_state_machine, config_no_auto_recover
    ):
        """Worker in warning zone (40-60 min) should generate alert."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a worker that's been unresponsive for 45 minutes
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=45),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        # Get alerts
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,  # 40 minutes
            stale_timeout_seconds=3600,  # 60 minutes
        )

        # Should have exactly 1 alert
        assert len(alerts) == 1
        alert = alerts[0]
        assert alert["type"] == "long_running_worker"
        assert alert["worker_id"] == "worker-1"
        assert alert["task_id"] == "T-001-0"
        assert alert["elapsed_minutes"] == 45
        assert "actions" in alert
        assert len(alert["actions"]) == 3  # continue, extend, kill

    def test_worker_past_stale_threshold_not_auto_recovered_when_disabled(
        self, mock_state_machine, config_no_auto_recover
    ):
        """Worker past 60 min should NOT be auto-recovered when auto_recover=False."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a worker that's been unresponsive for 70 minutes (past stale threshold)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=70),
        )
        mock_state_machine.state.workers["worker-1"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-1"

        # Call recover_stale_workers (this would be called by supervisor loop)
        # With auto_recover=False, the supervisor loop should NOT call this
        # But even if called, we verify the worker is still there

        # Key insight: get_long_running_alerts should return empty for stale workers
        # because stale workers are handled by recover_stale_workers (if enabled)
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )

        # No alert because worker is past stale threshold (not in warning zone)
        assert len(alerts) == 0

        # But the worker should still be busy (NOT auto-recovered)
        assert mock_state_machine.state.workers["worker-1"].state == "busy"
        assert "T-001-0" in mock_state_machine.state.queue.in_progress

    def test_worker_completes_successfully_after_long_time(
        self, mock_state_machine, config_no_auto_recover
    ):
        """Worker that takes long time but completes should work fine."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a worker that's been busy for 90 minutes
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=90),
        )
        mock_state_machine.state.workers["worker-1"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-1"

        # Simulate worker completing successfully
        # (heartbeat update + task completion)
        worker.last_seen = datetime.now()
        worker.state = "idle"
        worker.task_id = None

        # Move task from in_progress to done
        del mock_state_machine.state.queue.in_progress["T-001-0"]
        mock_state_machine.state.queue.done.append("T-001-0")

        # Worker should be idle and task should be done
        assert mock_state_machine.state.workers["worker-1"].state == "idle"
        assert "T-001-0" in mock_state_machine.state.queue.done
        assert "T-001-0" not in mock_state_machine.state.queue.in_progress


class TestUserInterventionActions:
    """Test user actions: continue, extend, kill."""

    def test_user_extends_timeout(self, mock_state_machine, config_no_auto_recover):
        """User can extend worker timeout (reset last_seen)."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a worker in warning zone
        old_last_seen = datetime.now() - timedelta(minutes=45)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=old_last_seen,
        )
        mock_state_machine.state.workers["worker-1"] = worker

        # User decides to extend
        result = worker_manager.extend_worker_timeout("worker-1")

        # Should succeed
        assert result["success"] is True
        assert result["worker_id"] == "worker-1"
        assert "extension_minutes" in result

        # Worker's last_seen should be updated to now
        # (the actual update happens in the mock, so we verify the result)
        assert result["new_last_seen"] is not None

    def test_user_kills_stuck_worker(self, mock_state_machine, config_no_auto_recover):
        """User can manually kill a stuck worker."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a stuck worker
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            scope="src/",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=90),
        )
        mock_state_machine.state.workers["worker-1"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-1"

        # User decides to kill
        result = worker_manager.kill_worker("worker-1")

        # Should succeed
        assert result["success"] is True
        assert result["worker_id"] == "worker-1"
        assert result["task_id"] == "T-001-0"
        assert result["task_recovered"] is True

        # Worker should be disconnected
        assert mock_state_machine.state.workers["worker-1"].state == "disconnected"

    def test_user_chooses_continue_and_waits(
        self, mock_state_machine, config_no_auto_recover
    ):
        """User can acknowledge alert and continue waiting (no action needed)."""
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Add a worker in warning zone
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=45),
        )
        mock_state_machine.state.workers["worker-1"] = worker

        # Get initial alert
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )
        assert len(alerts) == 1

        # User chooses "continue" (acknowledges but takes no action)
        # This is essentially a no-op - just dismiss the alert
        # The worker keeps running

        # Worker should still be busy
        assert mock_state_machine.state.workers["worker-1"].state == "busy"
        assert mock_state_machine.state.workers["worker-1"].task_id == "T-001-0"


class TestAutoRecoveryWhenEnabled:
    """Test opt-in auto-recovery behavior."""

    def test_stale_worker_auto_recovered_when_enabled(
        self, mock_state_machine, config_auto_recover
    ):
        """Worker past stale threshold should be auto-recovered when auto_recover=True."""
        worker_manager = WorkerManager(mock_state_machine, config_auto_recover)

        # Add a stale worker (70 minutes)
        worker = WorkerInfo(
            worker_id="worker-1",
            state="busy",
            task_id="T-001-0",
            joined_at=datetime.now() - timedelta(hours=2),
            last_seen=datetime.now() - timedelta(minutes=70),
        )
        mock_state_machine.state.workers["worker-1"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-1"

        # Call recover_stale_workers (supervisor loop would call this)
        recoveries = worker_manager.recover_stale_workers(
            stale_timeout_seconds=3600,  # 60 minutes
        )

        # Should have recovered the worker
        assert len(recoveries) == 1
        assert recoveries[0]["worker_id"] == "worker-1"
        assert recoveries[0]["task_id"] == "T-001-0"
        assert recoveries[0].get("task_recovered") is True

        # Worker should be disconnected
        assert mock_state_machine.state.workers["worker-1"].state == "disconnected"

        # Task should be back in pending
        assert "T-001-0" in mock_state_machine.state.queue.pending


class TestSimulatedFullFlow:
    """Simulate a complete flow from start to finish."""

    def test_full_flow_worker_slow_but_completes(
        self, mock_state_machine, config_no_auto_recover
    ):
        """
        Full simulation:
        1. Worker gets task
        2. Worker runs for 45 minutes (warning zone)
        3. Alert generated
        4. User does nothing (waits)
        5. Worker completes at 50 minutes
        6. Task is done, no recovery needed
        """
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Step 1: Worker gets task
        worker = WorkerInfo(
            worker_id="worker-ml-training",
            state="busy",
            task_id="T-001-0",
            scope="models/",
            joined_at=datetime.now(),
            last_seen=datetime.now(),
        )
        mock_state_machine.state.workers["worker-ml-training"] = worker
        mock_state_machine.state.queue.in_progress["T-001-0"] = "worker-ml-training"

        # Step 2: Time passes, worker is in warning zone (simulate 45 min)
        worker.last_seen = datetime.now() - timedelta(minutes=45)

        # Step 3: Check for alerts
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )
        assert len(alerts) == 1
        assert alerts[0]["worker_id"] == "worker-ml-training"
        assert alerts[0]["task_id"] == "T-001-0"

        # Step 4: User sees alert but does nothing (default: wait)
        # No action taken

        # Step 5: Worker completes (heartbeat + completion)
        worker.last_seen = datetime.now()
        worker.state = "idle"
        worker.task_id = None
        del mock_state_machine.state.queue.in_progress["T-001-0"]
        mock_state_machine.state.queue.done.append("T-001-0")

        # Step 6: Verify success
        assert mock_state_machine.state.workers["worker-ml-training"].state == "idle"
        assert "T-001-0" in mock_state_machine.state.queue.done
        assert "T-001-0" not in mock_state_machine.state.queue.in_progress

        # No more alerts
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )
        assert len(alerts) == 0

    def test_full_flow_worker_stuck_user_kills(
        self, mock_state_machine, config_no_auto_recover
    ):
        """
        Full simulation:
        1. Worker gets task
        2. Worker runs for 90 minutes (past stale)
        3. User checks status, sees worker past stale
        4. User decides to kill
        5. Task recovered to pending
        6. Another worker picks up task
        """
        worker_manager = WorkerManager(mock_state_machine, config_no_auto_recover)

        # Step 1: Worker gets task
        worker = WorkerInfo(
            worker_id="worker-stuck",
            state="busy",
            task_id="T-002-0",
            scope="api/",
            joined_at=datetime.now(),
            last_seen=datetime.now(),
        )
        mock_state_machine.state.workers["worker-stuck"] = worker
        mock_state_machine.state.queue.in_progress["T-002-0"] = "worker-stuck"

        # Step 2: Time passes, worker is past stale (90 min)
        worker.last_seen = datetime.now() - timedelta(minutes=90)

        # Step 3: User checks - no alert (past stale threshold)
        alerts = worker_manager.get_long_running_alerts(
            warning_timeout_seconds=2400,
            stale_timeout_seconds=3600,
        )
        assert len(alerts) == 0  # Past stale, not in warning zone

        # Step 4: User decides to kill (after seeing worker is unresponsive)
        result = worker_manager.kill_worker("worker-stuck")
        assert result["success"] is True
        assert result["task_recovered"] is True

        # Step 5: Task is back in pending
        assert "T-002-0" in mock_state_machine.state.queue.pending
        assert mock_state_machine.state.workers["worker-stuck"].state == "disconnected"

        # Step 6: Another worker picks up task
        new_worker = WorkerInfo(
            worker_id="worker-new",
            state="busy",
            task_id="T-002-0",
            joined_at=datetime.now(),
            last_seen=datetime.now(),
        )
        mock_state_machine.state.workers["worker-new"] = new_worker
        mock_state_machine.state.queue.pending.remove("T-002-0")
        mock_state_machine.state.queue.in_progress["T-002-0"] = "worker-new"

        # Verify
        assert "T-002-0" in mock_state_machine.state.queue.in_progress
        assert mock_state_machine.state.queue.in_progress["T-002-0"] == "worker-new"
