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


def test_max_idle_minutes_default_is_60(daemon: C4Daemon):
    """Test that max_idle_minutes defaults to 60 minutes."""
    assert daemon.config.max_idle_minutes == 60


def test_cleanup_stale_removes_old_idle_workers(daemon: C4Daemon):
    """Test that cleanup_stale() removes idle workers after max_idle_minutes."""
    from datetime import datetime, timedelta

    # Register workers (must use hex-only IDs)
    worker1 = "worker-aaaaaaaa"
    worker2 = "worker-bbbbbbbb"

    daemon.worker_manager.register(worker1)
    daemon.worker_manager.register(worker2)

    # Make worker1 old (91 minutes idle)
    w1 = daemon.worker_manager.get_worker(worker1)
    w1.last_seen = datetime.now() - timedelta(minutes=91)
    w1.state = "idle"
    daemon.state_machine.save_state()

    # worker2 is recent
    daemon.worker_manager.heartbeat(worker2)

    # Run cleanup with 90 minute threshold
    removed = daemon.worker_manager.cleanup_stale(max_idle_minutes=90)

    # Verify worker1 was removed
    assert worker1 in removed
    assert daemon.worker_manager.get_worker(worker1) is None

    # Verify worker2 still exists
    assert daemon.worker_manager.get_worker(worker2) is not None


def test_recover_stale_busy_workers(daemon: C4Daemon):
    """Test that recover_stale_workers() recovers tasks from unresponsive busy workers."""
    from datetime import datetime, timedelta

    # Register worker and assign a task (must use hex-only ID)
    worker_id = "worker-cccccccc"
    daemon.worker_manager.register(worker_id)

    # Create and assign a task
    from c4.models import Task, TaskStatus
    task = Task(
        id="T-999-0",
        title="Test task",
        dod="Complete test",
        status=TaskStatus.IN_PROGRESS,
        assigned_to=worker_id,
    )
    daemon._save_task(task)
    daemon.state_machine.state.queue.in_progress["T-999-0"] = worker_id
    daemon.worker_manager.set_busy(worker_id, "T-999-0")

    # Make worker stale (last seen 2 hours ago)
    worker = daemon.worker_manager.get_worker(worker_id)
    worker.last_seen = datetime.now() - timedelta(hours=2)
    daemon.state_machine.save_state()

    # Run recovery with 1 hour timeout
    recoveries = daemon.worker_manager.recover_stale_workers(
        stale_timeout_seconds=3600,  # 1 hour
        lock_store=daemon.lock_store,
    )

    # Verify task was recovered
    assert len(recoveries) == 1
    assert recoveries[0]["worker_id"] == worker_id
    assert recoveries[0]["task_id"] == "T-999-0"
    assert recoveries[0]["task_recovered"] is True

    # Verify task is back in pending queue
    assert "T-999-0" not in daemon.state_machine.state.queue.in_progress
    assert "T-999-0" in daemon.state_machine.state.queue.pending

    # Verify worker is marked as disconnected
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker.state == "disconnected"


def test_worker_ttl_eviction(daemon: C4Daemon):
    """Test that TTL-based eviction removes workers with expired last_seen."""
    from datetime import datetime, timedelta

    # Register workers (must use hex-only IDs)
    worker1 = "worker-eeeeeeee"
    worker2 = "worker-ffffffff"

    daemon.worker_manager.register(worker1)
    daemon.worker_manager.register(worker2)

    # Make worker1 expired (last seen 35 minutes ago, TTL is 30 minutes)
    w1 = daemon.worker_manager.get_worker(worker1)
    w1.last_seen = datetime.now() - timedelta(minutes=35)
    daemon.state_machine.save_state()

    # worker2 is recent
    daemon.worker_manager.heartbeat(worker2)

    # Run consistency sync (should trigger TTL eviction)
    result = daemon._sync_state_consistency()

    # Verify worker1 was evicted
    assert f"{worker1}:ttl-evict-idle" in result["fixed"]
    assert daemon.worker_manager.get_worker(worker1) is None

    # Verify worker2 still exists
    assert daemon.worker_manager.get_worker(worker2) is not None


def test_last_seen_updated_on_get_task(daemon: C4Daemon):
    """Test that last_seen is updated when worker calls c4_get_task."""

    # Register a worker
    worker_id = "worker-dddddddd"
    daemon.worker_manager.register(worker_id)

    # Get initial last_seen
    worker = daemon.worker_manager.get_worker(worker_id)
    initial_last_seen = worker.last_seen

    # Wait a bit and call get_task (which triggers heartbeat)
    import time
    time.sleep(0.1)

    # Transition to EXECUTE state so get_task works
    daemon.state_machine.transition("skip_discovery", "test")
    daemon.state_machine.transition("c4_run", "test")

    # Call get_task (should update last_seen via heartbeat)
    daemon.c4_get_task(worker_id)

    # Verify last_seen was updated
    worker = daemon.worker_manager.get_worker(worker_id)
    assert worker.last_seen > initial_last_seen


def test_purge_removes_all_stale_workers(daemon: C4Daemon):
    """Test that purge_stale_workers() performs comprehensive cleanup."""
    from datetime import datetime, timedelta

    from c4.models import Task, TaskStatus

    # Register multiple workers with different states
    worker_idle_stale = "worker-a1a1a1a1"  # Idle, stale
    worker_idle_active = "worker-b2b2b2b2"  # Idle, active
    worker_busy_zombie = "worker-c3c3c3c3"  # Busy but task is done (zombie)
    worker_busy_stale = "worker-d4d4d4d4"  # Busy, stale (unresponsive)
    worker_busy_active = "worker-e5e5e5e5"  # Busy, active

    daemon.worker_manager.register(worker_idle_stale)
    daemon.worker_manager.register(worker_idle_active)
    daemon.worker_manager.register(worker_busy_zombie)
    daemon.worker_manager.register(worker_busy_stale)
    daemon.worker_manager.register(worker_busy_active)

    # Set up idle stale worker (91 minutes idle)
    w1 = daemon.worker_manager.get_worker(worker_idle_stale)
    w1.state = "idle"
    w1.last_seen = datetime.now() - timedelta(minutes=91)

    # idle_active is recent (default)

    # Set up busy zombie worker (task is done)
    task_done = Task(
        id="T-100-0",
        title="Done task",
        dod="Done",
        status=TaskStatus.DONE,
    )
    daemon._save_task(task_done)
    daemon.state_machine.state.queue.done.append("T-100-0")
    daemon.worker_manager.set_busy(worker_busy_zombie, "T-100-0")

    # Set up busy stale worker (task exists but worker unresponsive)
    task_active = Task(
        id="T-101-0",
        title="Active task",
        dod="Active",
        status=TaskStatus.IN_PROGRESS,
        assigned_to=worker_busy_stale,
    )
    daemon._save_task(task_active)
    daemon.state_machine.state.queue.in_progress["T-101-0"] = worker_busy_stale
    daemon.worker_manager.set_busy(worker_busy_stale, "T-101-0")
    w4 = daemon.worker_manager.get_worker(worker_busy_stale)
    w4.last_seen = datetime.now() - timedelta(hours=2)  # Stale

    # Set up busy active worker
    task_active2 = Task(
        id="T-102-0",
        title="Active task 2",
        dod="Active",
        status=TaskStatus.IN_PROGRESS,
        assigned_to=worker_busy_active,
    )
    daemon._save_task(task_active2)
    daemon.state_machine.state.queue.in_progress["T-102-0"] = worker_busy_active
    daemon.worker_manager.set_busy(worker_busy_active, "T-102-0")

    # Save state
    daemon.state_machine.save_state()

    # Run purge
    result = daemon.purge_stale_workers(max_idle_minutes=90)

    # Verify results
    assert result["success"] is True

    # worker_idle_stale (91 mins old) is removed by TTL eviction (30 min threshold) not idle cleanup
    assert (
        worker_idle_stale in result["idle_removed"]
        or f"{worker_idle_stale}:ttl-evict-idle" in result["zombie_fixes"]
    ), f"Expected {worker_idle_stale} to be removed, got: {result}"

    # Zombie worker should be fixed
    assert f"{worker_busy_zombie}:done" in result["zombie_fixes"]

    # Stale busy worker is recovered by either TTL eviction or recover_stale_workers
    assert (
        worker_busy_stale in result["stale_recovered"]
        or f"{worker_busy_stale}:ttl-evict-busy" in result["zombie_fixes"]
    ), f"Expected {worker_busy_stale} to be recovered, got: {result}"

    # Verify workers removed or fixed
    assert daemon.worker_manager.get_worker(worker_idle_stale) is None  # Removed by TTL
    # Zombie worker is set to idle (not removed - could continue working)
    zombie = daemon.worker_manager.get_worker(worker_busy_zombie)
    assert zombie is not None and zombie.state == "idle"
    # Stale worker is disconnected or removed
    stale = daemon.worker_manager.get_worker(worker_busy_stale)
    assert stale is None or stale.state == "disconnected"

    # Verify active workers still exist
    assert daemon.worker_manager.get_worker(worker_idle_active) is not None
    assert daemon.worker_manager.get_worker(worker_busy_active) is not None

    # Verify total count (3+ cleanups)
    assert result["total_cleaned"] >= 3  # At least 3 workers cleaned


def test_sync_fixes_orphaned_in_progress_in_tasks_db(daemon: C4Daemon):
    """Test that _sync_state_consistency() resets orphaned tasks in c4_tasks.

    Scenario: c4_tasks has tasks with status=in_progress and assigned_to=worker-X,
    but c4_state.queue.in_progress is empty (workers were already evicted).
    These orphaned tasks should be reset to pending.
    """
    # Create tasks directly in c4_tasks with in_progress status
    task1 = Task(
        id="T-ORPHAN-001-0",
        title="Orphaned task 1",
        dod="Test orphan",
        status=TaskStatus.IN_PROGRESS,
        assigned_to="worker-dead0001",
    )
    task2 = Task(
        id="T-ORPHAN-002-0",
        title="Orphaned task 2",
        dod="Test orphan",
        status=TaskStatus.IN_PROGRESS,
        assigned_to="worker-dead0002",
    )
    daemon._save_task(task1)
    daemon._save_task(task2)

    # Do NOT add to c4_state.queue.in_progress — simulating worker eviction
    # c4_state thinks queue is clean, but c4_tasks still has in_progress

    # Verify precondition: c4_state has no in_progress
    assert len(daemon.state_machine.state.queue.in_progress) == 0

    # Verify precondition: c4_tasks has 2 in_progress
    queue_stats = daemon.task_store.get_queue_stats("test-zombie")
    assert queue_stats["in_progress_count"] == 2

    # Run consistency sync
    result = daemon._sync_state_consistency()

    # Verify orphans were fixed
    assert "T-ORPHAN-001-0:orphaned_reset" in result["fixed"]
    assert "T-ORPHAN-002-0:orphaned_reset" in result["fixed"]

    # Refresh task cache from DB (sync wrote directly to task_store)
    daemon._load_tasks()

    # Verify tasks are now pending in c4_tasks
    t1 = daemon.get_task("T-ORPHAN-001-0")
    assert t1.status == TaskStatus.PENDING
    assert t1.assigned_to is None

    t2 = daemon.get_task("T-ORPHAN-002-0")
    assert t2.status == TaskStatus.PENDING
    assert t2.assigned_to is None

    # Verify tasks are in c4_state.queue.pending
    assert "T-ORPHAN-001-0" in daemon.state_machine.state.queue.pending
    assert "T-ORPHAN-002-0" in daemon.state_machine.state.queue.pending

    # Verify c4_tasks shows 0 in_progress
    queue_stats = daemon.task_store.get_queue_stats("test-zombie")
    assert queue_stats["in_progress_count"] == 0


def test_sync_orphan_does_not_reset_tracked_tasks(daemon: C4Daemon):
    """Ensure orphan detection doesn't reset tasks that ARE in c4_state.queue."""
    worker_id = "worker-aaaa1111"
    daemon.worker_manager.register(worker_id)

    task = Task(
        id="T-TRACKED-001-0",
        title="Tracked task",
        dod="Test",
        status=TaskStatus.IN_PROGRESS,
        assigned_to=worker_id,
    )
    daemon._save_task(task)

    # This task IS tracked in c4_state
    daemon.state_machine.state.queue.in_progress["T-TRACKED-001-0"] = worker_id
    daemon.worker_manager.set_busy(worker_id, "T-TRACKED-001-0")
    daemon.state_machine.save_state()

    # Run consistency sync
    result = daemon._sync_state_consistency()

    # Should NOT be orphan-reset
    orphan_fixes = [f for f in result["fixed"] if "orphaned_reset" in f]
    assert len(orphan_fixes) == 0

    # Task should still be in_progress
    t = daemon.get_task("T-TRACKED-001-0")
    assert t.status == TaskStatus.IN_PROGRESS
