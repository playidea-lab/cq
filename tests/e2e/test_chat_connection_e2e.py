"""E2E Tests for Chat API and Local Connection

Tests the complete workflow of:
1. Chat API (MCP tools) - c4_status, c4_get_task, c4_submit, etc.
2. Local Connection (WebSocket client) - ConnectionConfig, WebSocketClient, etc.

These tests verify integration between components without requiring
actual WebSocket server or Claude CLI.
"""

from __future__ import annotations

import json
import tempfile
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.connection import (
    ConnectionConfig,
    ConnectionState,
    WebSocketClient,
    WebSocketMessage,
)
from c4.connection.websocket_client import ConnectionManager, MessageType
from c4.mcp_server import C4Daemon
from c4.models import (
    Task,
    ValidationConfig,
)

# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon(temp_project):
    """Create an initialized daemon."""
    d = C4Daemon(temp_project)
    d.initialize("e2e-test-project", with_default_checkpoints=False)
    return d


@pytest.fixture
def daemon_in_plan(daemon):
    """Create a daemon in PLAN state with tasks."""
    # Skip discovery phase
    daemon.state_machine.transition("skip_discovery")

    # Configure validations
    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'lint ok'",
            "unit": "echo 'tests passed'",
        },
        required=["lint", "unit"],
    )

    # Add tasks
    daemon.add_task(Task(id="T-001", title="First Task", dod="Complete the first task"))
    daemon.add_task(Task(id="T-002", title="Second Task", dod="Complete the second task"))

    daemon._save_config()
    return daemon


@pytest.fixture
def daemon_in_execute(daemon_in_plan):
    """Create a daemon in EXECUTE state."""
    daemon_in_plan.state_machine.transition("c4_run")
    return daemon_in_plan


@pytest.fixture
def ws_client():
    """Create a WebSocket client."""
    config = ConnectionConfig(
        host="127.0.0.1",
        port=4000,
        path="/ws",
    )
    return WebSocketClient(config)


@pytest.fixture
def connection_manager():
    """Create a connection manager."""
    return ConnectionManager()


# =============================================================================
# Chat API E2E Tests (MCP Tools)
# =============================================================================


class TestChatAPIStatus:
    """E2E tests for c4_status API."""

    def test_status_returns_project_info(self, daemon):
        """Test c4_status returns complete project information."""
        result = daemon.c4_status()

        assert result["initialized"] is True
        assert result["project_id"] == "e2e-test-project"
        assert "status" in result
        assert "queue" in result
        assert "workers" in result

    def test_status_reflects_state_changes(self, daemon_in_plan):
        """Test status updates after state transitions."""
        # Initial state
        status1 = daemon_in_plan.c4_status()
        assert status1["status"] == "PLAN"

        # After transition to EXECUTE
        daemon_in_plan.state_machine.transition("c4_run")
        status2 = daemon_in_plan.c4_status()
        assert status2["status"] == "EXECUTE"

    def test_status_shows_queue_info(self, daemon_in_plan):
        """Test queue information in status."""
        status = daemon_in_plan.c4_status()

        assert status["queue"]["pending"] == 2
        assert "T-001" in status["queue"]["pending_ids"]
        assert "T-002" in status["queue"]["pending_ids"]


class TestChatAPITaskWorkflow:
    """E2E tests for task assignment and submission workflow."""

    def test_get_task_assigns_to_worker(self, daemon_in_execute):
        """Test c4_get_task assigns task to worker."""
        assignment = daemon_in_execute.c4_get_task("worker-e2e-1")

        assert assignment is not None
        assert assignment.task_id == "T-001"
        assert assignment.branch is not None

        # Verify worker assignment via status queue
        status = daemon_in_execute.c4_status()
        assert status["queue"]["in_progress_map"].get("T-001") == "worker-e2e-1"

    def test_get_task_returns_none_when_empty(self, daemon_in_execute):
        """Test c4_get_task returns None when no tasks available."""
        # Get all tasks
        daemon_in_execute.c4_get_task("worker-1")
        daemon_in_execute.c4_get_task("worker-2")

        # No more tasks
        assignment = daemon_in_execute.c4_get_task("worker-3")
        assert assignment is None

    @patch("subprocess.run")
    def test_submit_completes_task(self, mock_run, daemon_in_execute):
        """Test c4_submit marks task as completed."""
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Get and complete task
        assignment = daemon_in_execute.c4_get_task("worker-e2e-1")

        result = daemon_in_execute.c4_submit(
            task_id=assignment.task_id,
            commit_sha="abc123def",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
            worker_id="worker-e2e-1",
        )

        assert result.success is True
        assert result.next_action in ["get_next_task", "complete"]

        # Verify task is now done via status
        status = daemon_in_execute.c4_status()
        assert status["queue"]["done"] >= 1
        assert "T-001" not in status["queue"]["in_progress_map"]

    @patch("subprocess.run")
    def test_full_worker_cycle(self, mock_run, daemon_in_execute):
        """Test complete worker cycle: get → validate → submit."""
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Worker gets task
        assignment = daemon_in_execute.c4_get_task("worker-cycle-1")
        assert assignment is not None

        # Worker runs validation
        val_result = daemon_in_execute.c4_run_validation()
        assert val_result["success"] is True

        # Worker submits
        submit_result = daemon_in_execute.c4_submit(
            task_id=assignment.task_id,
            commit_sha="cycle123",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
            worker_id="worker-cycle-1",
        )
        assert submit_result.success is True

        # Verify task is done (done is a count, not a list)
        status = daemon_in_execute.c4_status()
        assert status["queue"]["done"] >= 1
        assert "T-001" not in status["queue"]["in_progress_map"]


class TestChatAPIMultiWorker:
    """E2E tests for multi-worker scenarios."""

    def test_different_workers_get_different_tasks(self, daemon_in_execute):
        """Test parallel workers get different tasks."""
        assignment1 = daemon_in_execute.c4_get_task("worker-1")
        assignment2 = daemon_in_execute.c4_get_task("worker-2")

        assert assignment1 is not None
        assert assignment2 is not None
        assert assignment1.task_id != assignment2.task_id

    def test_same_worker_cannot_get_multiple_tasks(self, daemon_in_execute):
        """Test same worker can't hold multiple tasks simultaneously."""
        assignment1 = daemon_in_execute.c4_get_task("worker-same")
        assignment2 = daemon_in_execute.c4_get_task("worker-same")

        # Second assignment should return the same task or None
        if assignment2 is not None:
            assert assignment2.task_id == assignment1.task_id


class TestChatAPIValidation:
    """E2E tests for validation workflow."""

    @patch("subprocess.run")
    def test_validation_runs_all_commands(self, mock_run, daemon_in_execute):
        """Test c4_run_validation runs configured commands."""
        mock_run.return_value = MagicMock(returncode=0, stdout="validation ok", stderr="")

        result = daemon_in_execute.c4_run_validation()

        assert result["success"] is True
        assert len(result["results"]) >= 1

    @patch("subprocess.run")
    def test_validation_failure_tracked(self, mock_run, daemon_in_execute):
        """Test validation failures are properly tracked."""
        mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="lint error")

        result = daemon_in_execute.c4_run_validation()

        assert result["success"] is False


# =============================================================================
# Local Connection E2E Tests (WebSocket)
# =============================================================================


class TestConnectionConfig:
    """E2E tests for connection configuration."""

    def test_config_builds_correct_url(self):
        """Test URL generation from config."""
        config = ConnectionConfig(
            host="localhost",
            port=8080,
            path="/api/ws",
        )

        assert config.url == "ws://localhost:8080/api/ws"

    def test_default_config_values(self):
        """Test default configuration values."""
        config = ConnectionConfig()

        assert config.host == "127.0.0.1"
        assert config.port == 4000
        assert config.path == "/ws"
        assert config.reconnect is True
        assert config.timeout == 10.0


class TestWebSocketMessageSerialization:
    """E2E tests for message serialization."""

    def test_message_to_json_roundtrip(self):
        """Test message serialization and deserialization."""
        original = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload={"status": "running", "tasks": 5},
            message_id="msg-001",
        )

        json_str = original.to_json()
        restored = WebSocketMessage.from_json(json_str)

        assert restored.type == original.type
        assert restored.payload == original.payload
        assert restored.message_id == original.message_id

    def test_message_types_serialized_correctly(self):
        """Test all message types serialize correctly."""
        for msg_type in MessageType:
            msg = WebSocketMessage(type=msg_type)
            json_str = msg.to_json()
            data = json.loads(json_str)
            assert data["type"] == msg_type.value


class TestWebSocketClientState:
    """E2E tests for WebSocket client state management."""

    def test_client_starts_disconnected(self, ws_client):
        """Test client initializes in disconnected state."""
        assert ws_client.state == ConnectionState.DISCONNECTED
        assert not ws_client.is_connected

    def test_client_registers_handlers(self, ws_client):
        """Test message handler registration."""
        handler = MagicMock()
        ws_client.on_message(MessageType.STATUS_UPDATE, handler)

        assert MessageType.STATUS_UPDATE in ws_client._handlers
        assert handler in ws_client._handlers[MessageType.STATUS_UPDATE]

    def test_client_unregisters_handlers(self, ws_client):
        """Test message handler unregistration."""
        handler = MagicMock()
        ws_client.on_message(MessageType.STATUS_UPDATE, handler)
        ws_client.off_message(MessageType.STATUS_UPDATE, handler)

        assert handler not in ws_client._handlers.get(MessageType.STATUS_UPDATE, [])

    @pytest.mark.asyncio
    async def test_dispatch_calls_handlers(self, ws_client):
        """Test message dispatch calls registered handlers."""
        handler = MagicMock()
        ws_client.on_message(MessageType.TASK_UPDATE, handler)

        msg = WebSocketMessage(
            type=MessageType.TASK_UPDATE,
            payload={"task_id": "T-001", "status": "assigned"},
        )
        await ws_client._dispatch_message(msg)

        handler.assert_called_once_with(msg)

    @pytest.mark.asyncio
    async def test_dispatch_handles_async_handlers(self, ws_client):
        """Test dispatch works with async handlers."""
        async_handler = AsyncMock()
        ws_client.on_message(MessageType.WORKER_UPDATE, async_handler)

        msg = WebSocketMessage(
            type=MessageType.WORKER_UPDATE,
            payload={"worker_id": "w-1"},
        )
        await ws_client._dispatch_message(msg)

        async_handler.assert_called_once_with(msg)


class TestConnectionManager:
    """E2E tests for connection manager."""

    @pytest.mark.asyncio
    async def test_manager_tracks_connections(self, connection_manager):
        """Test manager tracks client connections."""
        ws = MagicMock()

        await connection_manager.connect("client-1", ws)
        assert connection_manager.connection_count == 1

        await connection_manager.connect("client-2", ws)
        assert connection_manager.connection_count == 2

    @pytest.mark.asyncio
    async def test_manager_removes_disconnected(self, connection_manager):
        """Test manager removes disconnected clients."""
        ws = MagicMock()

        await connection_manager.connect("client-1", ws)
        await connection_manager.disconnect("client-1")

        assert connection_manager.connection_count == 0

    def test_manager_subscription_tracking(self, connection_manager):
        """Test topic subscription management."""
        connection_manager.subscribe("client-1", "status")
        connection_manager.subscribe("client-1", "tasks")
        connection_manager.subscribe("client-2", "status")

        status_subscribers = connection_manager.get_subscribers("status")
        assert "client-1" in status_subscribers
        assert "client-2" in status_subscribers

        tasks_subscribers = connection_manager.get_subscribers("tasks")
        assert "client-1" in tasks_subscribers
        assert "client-2" not in tasks_subscribers

    @pytest.mark.asyncio
    async def test_manager_broadcast_to_subscribers(self, connection_manager):
        """Test broadcasting to topic subscribers."""
        ws1 = MagicMock()
        ws1.send_str = AsyncMock()
        ws2 = MagicMock()
        ws2.send_str = AsyncMock()

        await connection_manager.connect("client-1", ws1)
        await connection_manager.connect("client-2", ws2)
        connection_manager.subscribe("client-1", "status")
        connection_manager.subscribe("client-2", "status")

        msg = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload={"status": "running"},
        )
        count = await connection_manager.broadcast("status", msg)

        assert count == 2
        ws1.send_str.assert_called_once()
        ws2.send_str.assert_called_once()

    @pytest.mark.asyncio
    async def test_manager_send_to_specific_client(self, connection_manager):
        """Test sending to specific client."""
        ws = MagicMock()
        ws.send_str = AsyncMock()

        await connection_manager.connect("client-1", ws)

        msg = WebSocketMessage(type=MessageType.ACK)
        result = await connection_manager.send_to("client-1", msg)

        assert result is True
        ws.send_str.assert_called_once()

    @pytest.mark.asyncio
    async def test_disconnect_removes_from_subscriptions(self, connection_manager):
        """Test disconnect cleans up subscriptions."""
        ws = MagicMock()

        await connection_manager.connect("client-1", ws)
        connection_manager.subscribe("client-1", "status")
        connection_manager.subscribe("client-1", "tasks")

        await connection_manager.disconnect("client-1")

        assert "client-1" not in connection_manager.get_subscribers("status")
        assert "client-1" not in connection_manager.get_subscribers("tasks")


# =============================================================================
# Integration Tests: Chat API + Connection
# =============================================================================


class TestChatConnectionIntegration:
    """Integration tests combining Chat API and Connection."""

    @pytest.mark.asyncio
    async def test_status_update_message_format(self, daemon_in_execute):
        """Test status can be converted to WebSocket message format."""
        status = daemon_in_execute.c4_status()

        msg = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload=status,
        )

        json_str = msg.to_json()
        data = json.loads(json_str)

        assert data["type"] == "status_update"
        assert data["payload"]["project_id"] == "e2e-test-project"
        assert data["payload"]["status"] == "EXECUTE"

    @pytest.mark.asyncio
    async def test_task_update_message_format(self, daemon_in_execute):
        """Test task assignment can be converted to WebSocket message."""
        worker_id = "worker-msg-test"
        assignment = daemon_in_execute.c4_get_task(worker_id)

        # Get actual worker_id from status (TaskAssignment doesn't have worker_id)
        status = daemon_in_execute.c4_status()
        assigned_worker = status["queue"]["in_progress_map"].get(assignment.task_id)

        msg = WebSocketMessage(
            type=MessageType.TASK_UPDATE,
            payload={
                "action": "assigned",
                "task_id": assignment.task_id,
                "worker_id": assigned_worker,
                "branch": assignment.branch,
            },
        )

        json_str = msg.to_json()
        data = json.loads(json_str)

        assert data["type"] == "task_update"
        assert data["payload"]["action"] == "assigned"
        assert data["payload"]["task_id"] == "T-001"
        assert data["payload"]["worker_id"] == worker_id

    @pytest.mark.asyncio
    async def test_simulated_realtime_flow(self, daemon_in_execute, connection_manager):
        """Simulate real-time updates via connection manager."""
        # Setup mock websocket
        ws = MagicMock()
        ws.send_str = AsyncMock()

        await connection_manager.connect("ui-client", ws)
        connection_manager.subscribe("ui-client", "status")
        connection_manager.subscribe("ui-client", "tasks")

        # Worker gets task
        worker_id = "worker-rt"
        assignment = daemon_in_execute.c4_get_task(worker_id)

        # Get actual worker_id from status (TaskAssignment doesn't have worker_id)
        status = daemon_in_execute.c4_status()
        assigned_worker = status["queue"]["in_progress_map"].get(assignment.task_id)

        # Simulate task update broadcast
        task_msg = WebSocketMessage(
            type=MessageType.TASK_UPDATE,
            payload={
                "action": "assigned",
                "task_id": assignment.task_id,
                "worker_id": assigned_worker,
            },
        )
        await connection_manager.broadcast("tasks", task_msg)

        # Verify UI client received update
        ws.send_str.assert_called()
        call_data = json.loads(ws.send_str.call_args[0][0])
        assert call_data["type"] == "task_update"
        assert call_data["payload"]["task_id"] == "T-001"
        assert call_data["payload"]["worker_id"] == worker_id


class TestEndToEndScenarios:
    """Complete E2E scenarios combining all components."""

    @patch("subprocess.run")
    @pytest.mark.asyncio
    async def test_full_scenario_with_ui_updates(
        self, mock_run, daemon_in_execute, connection_manager
    ):
        """Test complete scenario: worker cycle with UI updates."""
        mock_run.return_value = MagicMock(returncode=0, stdout="ok", stderr="")

        # Setup UI client
        ui_ws = MagicMock()
        ui_ws.send_str = AsyncMock()
        await connection_manager.connect("ui-client", ui_ws)
        connection_manager.subscribe("ui-client", "status")
        connection_manager.subscribe("ui-client", "tasks")

        # Worker cycle
        assignment = daemon_in_execute.c4_get_task("worker-full")

        # Broadcast task assigned
        await connection_manager.broadcast(
            "tasks",
            WebSocketMessage(
                type=MessageType.TASK_UPDATE,
                payload={"action": "assigned", "task_id": assignment.task_id},
            ),
        )

        # Worker completes
        daemon_in_execute.c4_submit(
            task_id=assignment.task_id,
            commit_sha="full123",
            validation_results=[
                {"name": "lint", "status": "pass"},
                {"name": "unit", "status": "pass"},
            ],
            worker_id="worker-full",
        )

        # Broadcast task completed
        await connection_manager.broadcast(
            "tasks",
            WebSocketMessage(
                type=MessageType.TASK_UPDATE,
                payload={"action": "completed", "task_id": assignment.task_id},
            ),
        )

        # Broadcast status update
        status = daemon_in_execute.c4_status()
        await connection_manager.broadcast(
            "status",
            WebSocketMessage(type=MessageType.STATUS_UPDATE, payload=status),
        )

        # Verify multiple broadcasts occurred
        assert ui_ws.send_str.call_count >= 3
