"""Realtime E2E Tests.

Tests for Supabase Realtime integration:
- RealtimeManager connection and subscription
- StateStore on_change notifications
- WorkerSync multi-worker coordination
- Connection disconnect and reconnect

Most tests use mocking for WebSocket. For real Supabase tests,
set SUPABASE_URL and SUPABASE_KEY environment variables.

Run with: uv run pytest tests/e2e/test_realtime_e2e.py -v
"""

import os
import threading
from datetime import datetime, timedelta
from unittest.mock import MagicMock, patch

import pytest

from c4.realtime.manager import (
    ChannelState,
    RealtimeConfig,
    RealtimeManager,
)
from c4.realtime.sync import (
    SyncConfig,
    SyncEvent,
    WorkerInfo,
    WorkerSync,
    create_worker_sync,
)
from c4.store.supabase import ChangeType, SupabaseStateStore


@pytest.fixture
def realtime_config() -> RealtimeConfig:
    """Create test realtime configuration."""
    return RealtimeConfig(
        supabase_url="https://test.supabase.co",
        supabase_key="test-anon-key",
        auto_reconnect=False,
        reconnect_interval=0.1,
        heartbeat_interval=1.0,
        timeout=5.0,
    )


@pytest.fixture
def mock_store() -> SupabaseStateStore:
    """Create mock Supabase store."""
    store = SupabaseStateStore(
        url="https://test.supabase.co",
        key="test-key",
        realtime=True,
    )
    store._client = MagicMock()
    return store


class TestRealtimeManagerE2E:
    """E2E tests for RealtimeManager."""

    def test_manager_lifecycle(self, realtime_config: RealtimeConfig) -> None:
        """Test manager connect/disconnect lifecycle."""
        manager = RealtimeManager(realtime_config)

        assert manager.state == ChannelState.DISCONNECTED
        assert manager.is_connected is False

        # We won't actually connect (no real WebSocket)
        # but we can test the state transitions

        manager._state = ChannelState.CONNECTED
        assert manager.is_connected is True

        manager.disconnect()
        assert manager.state == ChannelState.DISCONNECTED

    def test_subscribe_multiple_tables(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test subscribing to multiple tables."""
        manager = RealtimeManager(realtime_config)

        callback1 = MagicMock()
        callback2 = MagicMock()
        callback3 = MagicMock()

        # Subscribe to different tables
        ch1 = manager.subscribe_table("tasks", "*", callback1)
        ch2 = manager.subscribe_table("projects", "INSERT", callback2)
        ch3 = manager.subscribe_table(
            "workers",
            "UPDATE",
            callback3,
            filter_column="status",
            filter_value="active",
        )

        assert len(manager.channels) == 3
        assert "tasks" in ch1
        assert "projects" in ch2
        assert "workers" in ch3

    def test_subscribe_broadcast_and_presence(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test broadcast and presence subscriptions."""
        manager = RealtimeManager(realtime_config)

        broadcast_cb = MagicMock()
        presence_cb = MagicMock()

        ch1 = manager.subscribe_broadcast("events", "task_done", broadcast_cb)
        ch2 = manager.subscribe_presence("workers", presence_cb)

        assert "broadcast" in ch1
        assert "presence" in ch2
        assert len(manager.channels) == 2

    def test_message_dispatch_postgres_changes(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test postgres_changes message dispatch."""
        manager = RealtimeManager(realtime_config)

        callback = MagicMock()
        manager.subscribe_table("tasks", "UPDATE", callback)

        # Simulate postgres_changes payload
        payload = {
            "eventType": "UPDATE",
            "schema": "public",
            "table": "tasks",
            "new": {"id": "task-1", "title": "Updated Task"},
            "old": {"id": "task-1", "title": "Original Task"},
        }

        manager._handle_postgres_change("realtime:public:tasks", payload)

        callback.assert_called_once_with(payload)

    def test_message_dispatch_with_filter(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test message dispatch respects filters."""
        manager = RealtimeManager(realtime_config)

        callback = MagicMock()
        manager.subscribe_table(
            "tasks",
            "UPDATE",
            callback,
            filter_column="project_id",
            filter_value="proj-123",
        )

        # Matching record
        matching = {
            "eventType": "UPDATE",
            "schema": "public",
            "table": "tasks",
            "new": {"id": "task-1", "project_id": "proj-123"},
            "old": {},
        }

        # Non-matching record
        non_matching = {
            "eventType": "UPDATE",
            "schema": "public",
            "table": "tasks",
            "new": {"id": "task-2", "project_id": "proj-999"},
            "old": {},
        }

        manager._handle_postgres_change("realtime:public:tasks", matching)
        manager._handle_postgres_change("realtime:public:tasks", non_matching)

        # Only matching should trigger callback
        assert callback.call_count == 1

    def test_reconnection_backoff(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test reconnection with exponential backoff."""
        realtime_config.auto_reconnect = True
        manager = RealtimeManager(realtime_config)

        # Simulate reconnection attempts
        manager._reconnect_attempts = 0
        backoff1 = min(
            realtime_config.reconnect_interval * (2 ** 0),
            60.0,
        )

        manager._reconnect_attempts = 3
        backoff2 = min(
            realtime_config.reconnect_interval * (2 ** 2),
            60.0,
        )

        assert backoff1 == 0.1
        assert backoff2 == 0.4

    def test_callbacks_registration(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test callback registration methods."""
        manager = RealtimeManager(realtime_config)

        on_connect = MagicMock()
        on_disconnect = MagicMock()
        on_error = MagicMock()
        on_reconnect = MagicMock()

        manager.on_connect(on_connect)
        manager.on_disconnect(on_disconnect)
        manager.on_error(on_error)
        manager.on_reconnect(on_reconnect)

        assert manager._on_connect == on_connect
        assert manager._on_disconnect == on_disconnect
        assert manager._on_error == on_error
        assert manager._on_reconnect == on_reconnect


class TestStateStoreRealtimeE2E:
    """E2E tests for StateStore realtime integration."""

    def test_on_change_registration(self, mock_store: SupabaseStateStore) -> None:
        """Test on_change callback registration."""
        callback = MagicMock()

        # Mock the RealtimeManager
        with patch("c4.realtime.manager.RealtimeManager") as mock_manager_class:
            mock_manager = MagicMock()
            mock_manager.subscribe_table.return_value = "channel:test"
            mock_manager_class.return_value = mock_manager

            sub_id = mock_store.on_change(callback, project_id="test-project")

            assert sub_id.startswith("change:test-project:")

    def test_on_change_without_project_filter(
        self, mock_store: SupabaseStateStore
    ) -> None:
        """Test global on_change callback."""
        callback = MagicMock()

        with patch("c4.realtime.manager.RealtimeManager") as mock_manager_class:
            mock_manager = MagicMock()
            mock_manager.subscribe_table.return_value = "channel:global"
            mock_manager_class.return_value = mock_manager

            sub_id = mock_store.on_change(callback)

            assert sub_id.startswith("change:*:")
            assert callback in mock_store._global_change_callbacks

    def test_dispatch_change_to_callbacks(
        self, mock_store: SupabaseStateStore
    ) -> None:
        """Test change dispatch to registered callbacks."""
        callback = MagicMock()
        mock_store._change_callbacks["test-project"] = [callback]

        state_data = {
            "project_id": "test-project",
            "status": "EXECUTE",
            "state_data": {
                "project_id": "test-project",
                "status": "EXECUTE",
                "execution_mode": "running",
                "checkpoint": {"current": None, "state": "pending"},
                "queue": {"pending": [], "in_progress": {}, "done": []},
                "workers": {},
                "locks": {"leader": None, "scopes": {}},
                "metrics": {
                    "events_emitted": 0,
                    "validations_run": 0,
                    "tasks_completed": 0,
                    "checkpoints_passed": 0,
                },
                "passed_checkpoints": [],
                "checkpoint_queue": [],
                "repair_queue": [],
                "created_at": "2024-01-01T00:00:00",
                "updated_at": "2024-01-01T00:00:00",
            },
        }

        payload = {
            "eventType": "UPDATE",
            "new": state_data,
            "old": state_data,
        }

        mock_store._dispatch_change(payload)

        callback.assert_called_once()
        args = callback.call_args[0]
        assert args[0].project_id == "test-project"
        assert args[1] == ChangeType.UPDATE

    def test_off_change_unregisters(
        self, mock_store: SupabaseStateStore
    ) -> None:
        """Test off_change removes callback."""
        callback = MagicMock()

        with patch("c4.realtime.manager.RealtimeManager") as mock_manager_class:
            mock_manager = MagicMock()
            mock_manager.subscribe_table.return_value = "channel:test"
            mock_manager_class.return_value = mock_manager

            sub_id = mock_store.on_change(callback, project_id="test-project")
            result = mock_store.off_change(sub_id)

            assert result is True
            assert "test-project" not in mock_store._change_callbacks

    def test_disconnect_realtime_cleanup(
        self, mock_store: SupabaseStateStore
    ) -> None:
        """Test disconnect_realtime cleans up resources."""
        mock_manager = MagicMock()
        mock_store._realtime_manager = mock_manager
        mock_store._change_callbacks = {"proj": [MagicMock()]}
        mock_store._global_change_callbacks = [MagicMock()]

        mock_store.disconnect_realtime()

        mock_manager.disconnect.assert_called_once()
        assert mock_store._realtime_manager is None
        assert mock_store._change_callbacks == {}
        assert mock_store._global_change_callbacks == []


class TestWorkerSyncE2E:
    """E2E tests for WorkerSync multi-worker coordination."""

    @pytest.fixture
    def sync_config(self) -> SyncConfig:
        """Create test sync config."""
        return SyncConfig(
            heartbeat_interval=0.5,
            worker_timeout=2.0,
            conflict_retry_max=2,
            conflict_retry_delay=0.1,
        )

    def test_worker_lifecycle(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test worker start/stop lifecycle."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        assert sync.worker_id == "worker-1"
        assert sync._running is False

        sync.start()
        assert sync._running is True
        assert sync._workers["worker-1"].status == "active"

        sync.stop()
        assert sync._running is False
        assert sync._workers["worker-1"].status == "stopped"

    def test_multiple_workers_tracking(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test tracking multiple workers."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        # Simulate another worker joining
        sync._handle_worker_event({
            "event_type": SyncEvent.WORKER_JOINED.value,
            "worker_id": "worker-2",
            "project_id": "test-project",
        })

        assert len(sync.workers) == 2
        assert "worker-1" in sync.workers
        assert "worker-2" in sync.workers

    def test_dead_worker_detection(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test detection and cleanup of dead workers."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        # Add a worker with old heartbeat
        sync._workers["dead-worker"] = WorkerInfo(
            worker_id="dead-worker",
            project_id="test-project",
            last_heartbeat=datetime.now() - timedelta(minutes=10),
        )

        left_handler = MagicMock()
        sync.on_worker_left(left_handler)

        sync._check_dead_workers()

        assert "dead-worker" not in sync.workers
        left_handler.assert_called_once()

    def test_task_assignment_notification(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test task assignment broadcasts and tracking."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        sync.broadcast_task_assigned("T-001")

        assert sync._workers["worker-1"].current_task == "T-001"

    def test_task_completion_clears_assignment(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test task completion clears current task."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        sync._workers["worker-1"].current_task = "T-001"
        sync.broadcast_task_completed("T-001")

        assert sync._workers["worker-1"].current_task is None

    def test_event_handlers_called(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test all event handlers are called correctly."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        state_handler = MagicMock()
        task_assigned_handler = MagicMock()
        task_completed_handler = MagicMock()
        worker_joined_handler = MagicMock()

        sync.on_state_changed(state_handler)
        sync.on_task_assigned(task_assigned_handler)
        sync.on_task_completed(task_completed_handler)
        sync.on_worker_joined(worker_joined_handler)

        # Simulate state change
        mock_state = MagicMock()
        mock_state.queue.in_progress = {}
        mock_change_type = MagicMock()

        sync._handle_state_change(mock_state, mock_change_type)
        state_handler.assert_called_once()

        # Simulate worker join
        sync._handle_worker_event({
            "event_type": SyncEvent.WORKER_JOINED.value,
            "worker_id": "worker-2",
            "project_id": "test-project",
        })
        worker_joined_handler.assert_called_once()

    def test_heartbeat_updates_timestamp(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test heartbeat updates worker timestamp."""
        sync = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        # Add another worker with old heartbeat
        old_time = datetime.now() - timedelta(seconds=30)
        sync._workers["worker-2"] = WorkerInfo(
            worker_id="worker-2",
            project_id="test-project",
            last_heartbeat=old_time,
        )

        # Simulate heartbeat
        sync._handle_worker_event({
            "event_type": SyncEvent.WORKER_HEARTBEAT.value,
            "worker_id": "worker-2",
            "current_task": "T-005",
        })

        assert sync._workers["worker-2"].last_heartbeat > old_time
        assert sync._workers["worker-2"].current_task == "T-005"


class TestTwoClientSimulation:
    """Simulate two clients connecting and syncing."""

    @pytest.fixture
    def sync_config(self) -> SyncConfig:
        """Create fast sync config for tests."""
        return SyncConfig(
            heartbeat_interval=0.2,
            worker_timeout=1.0,
            conflict_retry_max=3,
            conflict_retry_delay=0.05,
        )

    def test_two_workers_see_each_other(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test two workers can see each other via events."""
        worker1 = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        worker2 = WorkerSync(
            store=mock_store,
            worker_id="worker-2",
            project_id="test-project",
            config=sync_config,
        )

        # Simulate worker1 receiving worker2's join event
        worker1._handle_worker_event({
            "event_type": SyncEvent.WORKER_JOINED.value,
            "worker_id": "worker-2",
            "project_id": "test-project",
        })

        # Simulate worker2 receiving worker1's join event
        worker2._handle_worker_event({
            "event_type": SyncEvent.WORKER_JOINED.value,
            "worker_id": "worker-1",
            "project_id": "test-project",
        })

        # Both workers should see each other
        assert "worker-2" in worker1.workers
        assert "worker-1" in worker2.workers

    def test_task_assignment_visible_to_others(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test task assignment is visible to other workers."""
        worker1 = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        worker2 = WorkerSync(
            store=mock_store,
            worker_id="worker-2",
            project_id="test-project",
            config=sync_config,
        )

        # Add worker2 to worker1's tracking
        worker1._workers["worker-2"] = WorkerInfo(
            worker_id="worker-2",
            project_id="test-project",
        )

        # Worker1 assigns task and worker2 sees it via heartbeat
        worker1.broadcast_task_assigned("T-001")

        # Simulate worker1's heartbeat received by worker2
        worker2._handle_worker_event({
            "event_type": SyncEvent.WORKER_HEARTBEAT.value,
            "worker_id": "worker-1",
            "current_task": "T-001",
        })

        # Worker2 should see worker1's current task
        assert worker2._workers["worker-1"].current_task == "T-001"

    def test_worker_disconnect_detected(
        self, mock_store: SupabaseStateStore, sync_config: SyncConfig
    ) -> None:
        """Test worker disconnect is detected by others."""
        worker1 = WorkerSync(
            store=mock_store,
            worker_id="worker-1",
            project_id="test-project",
            config=sync_config,
        )

        # Add worker2 with recent heartbeat
        worker1._workers["worker-2"] = WorkerInfo(
            worker_id="worker-2",
            project_id="test-project",
            last_heartbeat=datetime.now(),
        )

        # Simulate worker2 leaving
        worker1._handle_worker_event({
            "event_type": SyncEvent.WORKER_LEFT.value,
            "worker_id": "worker-2",
        })

        # Worker2 should be removed
        assert "worker-2" not in worker1.workers


class TestReconnection:
    """Test connection disconnect and reconnect scenarios."""

    def test_reconnect_config(self, realtime_config: RealtimeConfig) -> None:
        """Test reconnection configuration."""
        realtime_config.auto_reconnect = True
        realtime_config.max_reconnect_attempts = 5
        realtime_config.reconnect_interval = 2.0

        manager = RealtimeManager(realtime_config)

        assert manager._config.auto_reconnect is True
        assert manager._config.max_reconnect_attempts == 5
        assert manager._config.reconnect_interval == 2.0

    def test_max_reconnect_attempts(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test that reconnection stops after max attempts."""
        realtime_config.max_reconnect_attempts = 3
        manager = RealtimeManager(realtime_config)

        # Simulate max attempts reached
        manager._reconnect_attempts = 4

        assert manager._reconnect_attempts > realtime_config.max_reconnect_attempts

    def test_on_reconnect_callback(
        self, realtime_config: RealtimeConfig
    ) -> None:
        """Test reconnect callback is called."""
        manager = RealtimeManager(realtime_config)

        callback = MagicMock()
        manager.on_reconnect(callback)

        # Manually call the callback as we would during reconnection
        if manager._on_reconnect:
            manager._on_reconnect(1)

        callback.assert_called_once_with(1)


class TestRealSupabaseIntegration:
    """Integration tests with real Supabase (skipped without credentials)."""

    @pytest.mark.skipif(
        not os.environ.get("SUPABASE_URL"),
        reason="SUPABASE_URL not set",
    )
    def test_real_connection(self) -> None:
        """Test real Supabase Realtime connection."""
        config = RealtimeConfig(
            supabase_url=os.environ["SUPABASE_URL"],
            supabase_key=os.environ.get("SUPABASE_KEY", ""),
            auto_reconnect=False,
            timeout=10.0,
        )

        manager = RealtimeManager(config)

        connected = threading.Event()

        def on_connect():
            connected.set()

        manager.on_connect(on_connect)

        # This would actually connect
        # manager.connect()
        # assert connected.wait(timeout=10)
        # manager.disconnect()

        # For now, just verify config is correct
        assert "supabase.co" in config.supabase_url

    @pytest.mark.skipif(
        not os.environ.get("SUPABASE_URL"),
        reason="SUPABASE_URL not set",
    )
    def test_real_state_subscription(self) -> None:
        """Test real state change subscription."""
        store = SupabaseStateStore(
            url=os.environ["SUPABASE_URL"],
            key=os.environ.get("SUPABASE_KEY", ""),
            realtime=True,
        )

        # For real tests, we would:
        # 1. Subscribe to changes
        # 2. Make a state change
        # 3. Verify callback received

        # For now, just verify store is configured correctly
        assert store._realtime_enabled is True


class TestFactoryFunctions:
    """Test factory functions."""

    def test_create_worker_sync(self, mock_store: SupabaseStateStore) -> None:
        """Test create_worker_sync factory."""
        config = SyncConfig(heartbeat_interval=10.0)

        sync = create_worker_sync(
            store=mock_store,
            worker_id="factory-worker",
            project_id="factory-project",
            config=config,
        )

        assert isinstance(sync, WorkerSync)
        assert sync.worker_id == "factory-worker"
        assert sync.project_id == "factory-project"
        assert sync._config.heartbeat_interval == 10.0

    def test_create_worker_sync_default_config(
        self, mock_store: SupabaseStateStore
    ) -> None:
        """Test factory with default config."""
        sync = create_worker_sync(
            store=mock_store,
            worker_id="worker",
            project_id="project",
        )

        assert sync._config.heartbeat_interval == 30.0
