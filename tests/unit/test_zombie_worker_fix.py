"""Tests for zombie worker bug fixes."""

from pathlib import Path
from unittest.mock import patch

import pytest

from c4.mcp_server import C4Daemon
from c4.models import Task, TaskStatus


@pytest.fixture
def daemon(tmp_path: Path) -> C4Daemon:
    """Create a test daemon with initialized state."""
    daemon = C4Daemon(project_root=tmp_path)
    daemon.initialize(project_id="test-zombie", with_default_checkpoints=False)
    return daemon


def test_sync_merged_tasks_updates_worker_state(daemon: C4Daemon):
    """Test that _sync_merged_tasks() sets worker to idle when task is merged."""
    # Register a worker
    worker_id = "worker-abcd1234"
    daemon.worker_manager.register(worker_id)

    # Create and assign a task
    task = Task(
        id="T-001-0",
        title="Test task",
        dod="Complete test",
        status=TaskStatus.IN_PROGRESS,
        assigned_to=worker_id,
        branch="c4/w-T-001-0",
    )
    daemon._save_task(task)
    daemon.state_machine.state.queue.in_progress["T-001-0"] = worker_id

    # Mark worker as busy
    daemon.worker_manager.set_busy(worker_id, "T-001-0", branch="c4/w-T-001-0")

    # Verify worker is busy
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker is not None
    assert worker.state == "busy"
    assert worker.task_id == "T-001-0"

    # Mock git to report branch as merged
    with patch('c4.daemon.c4_daemon.GitOperations') as MockGitOps:
        mock_git_instance = MockGitOps.return_value
        mock_git_instance.get_merged_task_branches.return_value = ["c4/w-T-001-0"]

        # Sync merged tasks
        synced = daemon._sync_merged_tasks()
        assert synced == 1

        # Verify task is done
        task = daemon.get_task("T-001-0")
        assert task is not None
        assert task.status == TaskStatus.DONE
        assert task.assigned_to is None

        # Verify worker is idle (BUG FIX)
        worker = daemon.worker_manager.get_worker(worker_id)
        assert worker is not None
        assert worker.state == "idle", "Worker should be idle after task is merged"
        assert worker.task_id is None, "Worker should have no task_id when idle"


def test_sync_merged_tasks_worker_not_found(daemon: C4Daemon):
    """Test that _sync_merged_tasks() handles missing worker gracefully."""
    # Create a task assigned to a non-existent worker
    task = Task(
        id="T-002-0",
        title="Test task",
        dod="Complete test",
        status=TaskStatus.IN_PROGRESS,
        assigned_to="worker-deadbeef",  # Not registered
        branch="c4/w-T-002-0",
    )
    daemon._save_task(task)
    daemon.state_machine.state.queue.in_progress["T-002-0"] = "worker-deadbeef"

    # Mock git to report branch as merged
    with patch('c4.daemon.c4_daemon.GitOperations') as MockGitOps:
        mock_git_instance = MockGitOps.return_value
        mock_git_instance.get_merged_task_branches.return_value = ["c4/w-T-002-0"]

        # Sync should complete without error
        synced = daemon._sync_merged_tasks()
        assert synced == 1

        # Task should still be moved to done
        task = daemon.get_task("T-002-0")
        assert task is not None
        assert task.status == TaskStatus.DONE
        assert task.assigned_to is None


def test_sync_fixes_busy_worker_with_done_task(daemon: C4Daemon):
    """Test that _sync_state_consistency() fixes busy worker when task is already done."""
    # Register a worker
    worker_id = "worker-cafe1234"
    daemon.worker_manager.register(worker_id)

    # Create a task that's already done
    task = Task(
        id="T-003-0",
        title="Done task",
        dod="Complete test",
        status=TaskStatus.DONE,
        assigned_to=None,
        branch="c4/w-T-003-0",
    )
    daemon._save_task(task)
    # Task is in done queue
    daemon.state_machine.state.queue.done.append("T-003-0")

    # But worker is still busy with this task (zombie state)
    daemon.worker_manager.set_busy(worker_id, "T-003-0", branch="c4/w-T-003-0")
    daemon.state_machine.save_state()  # Persist worker state to database

    # Verify worker is busy (zombie)
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker is not None
    assert worker.state == "busy"
    assert worker.task_id == "T-003-0"

    # Run consistency sync
    result = daemon._sync_state_consistency()

    # Verify worker is now idle
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker is not None
    assert worker.state == "idle", "Worker should be idle when task is done"
    assert worker.task_id is None

    # Check that fix was recorded
    assert f"{worker_id}:done" in result["fixed"]


def test_sync_fixes_busy_worker_with_missing_task(daemon: C4Daemon):
    """Test that _sync_state_consistency() fixes busy worker when task doesn't exist."""
    # Register a worker
    worker_id = "worker-beef1234"
    daemon.worker_manager.register(worker_id)

    # Worker is busy with a task that doesn't exist anywhere
    daemon.worker_manager.set_busy(worker_id, "T-999-0", branch="c4/w-T-999-0")
    daemon.state_machine.save_state()  # Persist worker state to database

    # Verify worker is busy (zombie)
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker is not None
    assert worker.state == "busy"
    assert worker.task_id == "T-999-0"

    # Run consistency sync
    result = daemon._sync_state_consistency()

    # Verify worker is now idle
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker is not None
    assert worker.state == "idle", "Worker should be idle when task is missing"
    assert worker.task_id is None

    # Check that fix was recorded
    assert f"{worker_id}:missing" in result["fixed"]
