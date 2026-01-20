"""Unit tests for WebSocket connection module."""

from __future__ import annotations

import json
from datetime import datetime
from unittest.mock import AsyncMock, MagicMock

import pytest

from c4.connection import (
    ConnectionConfig,
    ConnectionState,
    WebSocketClient,
    WebSocketMessage,
)
from c4.connection.websocket_client import ConnectionManager, MessageType


class TestConnectionConfig:
    """Tests for ConnectionConfig dataclass."""

    def test_default_config(self):
        """Test default configuration values."""
        config = ConnectionConfig()

        assert config.host == "127.0.0.1"
        assert config.port == 4000
        assert config.path == "/ws"
        assert config.reconnect is True
        assert config.reconnect_interval == 2.0
        assert config.max_reconnect_attempts == 5
        assert config.ping_interval == 30.0
        assert config.timeout == 10.0

    def test_custom_config(self):
        """Test custom configuration values."""
        config = ConnectionConfig(
            host="192.168.1.100",
            port=8080,
            path="/websocket",
            reconnect=False,
            timeout=30.0,
        )

        assert config.host == "192.168.1.100"
        assert config.port == 8080
        assert config.path == "/websocket"
        assert config.reconnect is False
        assert config.timeout == 30.0

    def test_url_property(self):
        """Test URL property generation."""
        config = ConnectionConfig(
            host="localhost",
            port=3000,
            path="/ws/v1",
        )

        assert config.url == "ws://localhost:3000/ws/v1"

    def test_default_url(self):
        """Test default URL."""
        config = ConnectionConfig()
        assert config.url == "ws://127.0.0.1:4000/ws"


class TestWebSocketMessage:
    """Tests for WebSocketMessage dataclass."""

    def test_create_message(self):
        """Test creating a basic message."""
        msg = WebSocketMessage(type=MessageType.PING)

        assert msg.type == MessageType.PING
        assert msg.payload == {}
        assert msg.message_id is None
        assert isinstance(msg.timestamp, datetime)

    def test_create_message_with_payload(self):
        """Test creating a message with payload."""
        msg = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload={"status": "running", "workers": 3},
            message_id="msg-123",
        )

        assert msg.type == MessageType.STATUS_UPDATE
        assert msg.payload["status"] == "running"
        assert msg.payload["workers"] == 3
        assert msg.message_id == "msg-123"

    def test_to_json(self):
        """Test JSON serialization."""
        timestamp = datetime(2025, 6, 15, 12, 0, 0)
        msg = WebSocketMessage(
            type=MessageType.SUBSCRIBE,
            payload={"topic": "status"},
            timestamp=timestamp,
            message_id="msg-456",
        )

        json_str = msg.to_json()
        data = json.loads(json_str)

        assert data["type"] == "subscribe"
        assert data["payload"] == {"topic": "status"}
        assert data["timestamp"] == "2025-06-15T12:00:00"
        assert data["message_id"] == "msg-456"

    def test_from_json(self):
        """Test JSON deserialization."""
        json_str = json.dumps(
            {
                "type": "task_update",
                "payload": {"task_id": "T-001", "action": "completed"},
                "timestamp": "2025-06-15T12:00:00",
                "message_id": "msg-789",
            }
        )

        msg = WebSocketMessage.from_json(json_str)

        assert msg.type == MessageType.TASK_UPDATE
        assert msg.payload["task_id"] == "T-001"
        assert msg.payload["action"] == "completed"
        assert msg.timestamp == datetime(2025, 6, 15, 12, 0, 0)
        assert msg.message_id == "msg-789"

    def test_roundtrip_serialization(self):
        """Test to_json -> from_json roundtrip."""
        original = WebSocketMessage(
            type=MessageType.WORKER_UPDATE,
            payload={"worker_id": "w-001", "state": "idle"},
            message_id="test-roundtrip",
        )

        json_str = original.to_json()
        restored = WebSocketMessage.from_json(json_str)

        assert restored.type == original.type
        assert restored.payload == original.payload
        assert restored.message_id == original.message_id


class TestMessageType:
    """Tests for MessageType enum."""

    def test_client_message_types(self):
        """Test client -> server message types."""
        assert MessageType.SUBSCRIBE == "subscribe"
        assert MessageType.UNSUBSCRIBE == "unsubscribe"
        assert MessageType.PING == "ping"
        assert MessageType.COMMAND == "command"

    def test_server_message_types(self):
        """Test server -> client message types."""
        assert MessageType.STATUS_UPDATE == "status_update"
        assert MessageType.TASK_UPDATE == "task_update"
        assert MessageType.WORKER_UPDATE == "worker_update"
        assert MessageType.CHECKPOINT_UPDATE == "checkpoint_update"
        assert MessageType.PONG == "pong"
        assert MessageType.ERROR == "error"
        assert MessageType.ACK == "ack"


class TestConnectionState:
    """Tests for ConnectionState enum."""

    def test_state_values(self):
        """Test all connection state values."""
        assert ConnectionState.DISCONNECTED == "disconnected"
        assert ConnectionState.CONNECTING == "connecting"
        assert ConnectionState.CONNECTED == "connected"
        assert ConnectionState.RECONNECTING == "reconnecting"
        assert ConnectionState.ERROR == "error"


class TestWebSocketClient:
    """Tests for WebSocketClient class."""

    def test_init_default(self):
        """Test client initialization with defaults."""
        client = WebSocketClient()

        assert client.config.host == "127.0.0.1"
        assert client.config.port == 4000
        assert client.state == ConnectionState.DISCONNECTED
        assert not client.is_connected

    def test_init_with_config(self):
        """Test client initialization with custom config."""
        config = ConnectionConfig(host="remote", port=8080)
        client = WebSocketClient(config)

        assert client.config.host == "remote"
        assert client.config.port == 8080

    def test_state_property(self):
        """Test state property returns current state."""
        client = WebSocketClient()
        assert client.state == ConnectionState.DISCONNECTED

        client._state = ConnectionState.CONNECTED
        assert client.state == ConnectionState.CONNECTED

    def test_is_connected_property(self):
        """Test is_connected property."""
        client = WebSocketClient()

        assert not client.is_connected

        client._state = ConnectionState.CONNECTED
        assert client.is_connected

        client._state = ConnectionState.CONNECTING
        assert not client.is_connected

    def test_on_message_registers_handler(self):
        """Test on_message registers a handler."""
        client = WebSocketClient()

        handler = MagicMock()
        client.on_message(MessageType.STATUS_UPDATE, handler)

        assert MessageType.STATUS_UPDATE in client._handlers
        assert handler in client._handlers[MessageType.STATUS_UPDATE]

    def test_on_message_multiple_handlers(self):
        """Test registering multiple handlers for same type."""
        client = WebSocketClient()

        handler1 = MagicMock()
        handler2 = MagicMock()
        client.on_message(MessageType.TASK_UPDATE, handler1)
        client.on_message(MessageType.TASK_UPDATE, handler2)

        assert len(client._handlers[MessageType.TASK_UPDATE]) == 2

    def test_off_message_removes_handler(self):
        """Test off_message removes a handler."""
        client = WebSocketClient()

        handler = MagicMock()
        client.on_message(MessageType.STATUS_UPDATE, handler)
        client.off_message(MessageType.STATUS_UPDATE, handler)

        assert handler not in client._handlers.get(MessageType.STATUS_UPDATE, [])

    def test_off_message_nonexistent(self):
        """Test off_message handles non-existent handler gracefully."""
        client = WebSocketClient()

        handler = MagicMock()
        # Should not raise
        client.off_message(MessageType.STATUS_UPDATE, handler)

    @pytest.mark.asyncio
    async def test_dispatch_message_calls_handlers(self):
        """Test _dispatch_message calls registered handlers."""
        client = WebSocketClient()

        handler1 = MagicMock()
        handler2 = MagicMock()
        client.on_message(MessageType.STATUS_UPDATE, handler1)
        client.on_message(MessageType.STATUS_UPDATE, handler2)

        msg = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload={"status": "running"},
        )
        await client._dispatch_message(msg)

        handler1.assert_called_once_with(msg)
        handler2.assert_called_once_with(msg)

    @pytest.mark.asyncio
    async def test_dispatch_message_async_handler(self):
        """Test _dispatch_message handles async handlers."""
        client = WebSocketClient()

        async_handler = AsyncMock()
        client.on_message(MessageType.TASK_UPDATE, async_handler)

        msg = WebSocketMessage(
            type=MessageType.TASK_UPDATE,
            payload={"task_id": "T-001"},
        )
        await client._dispatch_message(msg)

        async_handler.assert_called_once_with(msg)

    @pytest.mark.asyncio
    async def test_dispatch_message_handles_errors(self):
        """Test _dispatch_message handles handler errors gracefully."""
        client = WebSocketClient()

        def failing_handler(msg):
            raise ValueError("Test error")

        client.on_message(MessageType.ERROR, failing_handler)

        msg = WebSocketMessage(type=MessageType.ERROR)
        # Should not raise
        await client._dispatch_message(msg)

    @pytest.mark.asyncio
    async def test_send_not_connected(self):
        """Test send returns False when not connected."""
        client = WebSocketClient()

        msg = WebSocketMessage(type=MessageType.PING)
        result = await client.send(msg)

        assert result is False

    @pytest.mark.asyncio
    async def test_subscribe_adds_to_subscriptions(self):
        """Test subscribe adds topic to subscriptions set."""
        client = WebSocketClient()
        client._state = ConnectionState.CONNECTED
        client._websocket = MagicMock()
        client._websocket.send_str = AsyncMock()

        await client.subscribe("status")

        assert "status" in client._subscriptions

    @pytest.mark.asyncio
    async def test_unsubscribe_removes_subscription(self):
        """Test unsubscribe removes topic from subscriptions."""
        client = WebSocketClient()
        client._state = ConnectionState.CONNECTED
        client._websocket = MagicMock()
        client._websocket.send_str = AsyncMock()
        client._subscriptions.add("status")

        await client.unsubscribe("status")

        assert "status" not in client._subscriptions

    @pytest.mark.asyncio
    async def test_send_command(self):
        """Test send_command sends command message."""
        client = WebSocketClient()
        client._state = ConnectionState.CONNECTED
        client._websocket = MagicMock()
        client._websocket.send_str = AsyncMock()

        result = await client.send_command("run", {"force": True})

        assert result is True
        client._websocket.send_str.assert_called_once()

        # Check the sent message
        call_args = client._websocket.send_str.call_args[0][0]
        data = json.loads(call_args)
        assert data["type"] == "command"
        assert data["payload"]["command"] == "run"
        assert data["payload"]["args"]["force"] is True

    @pytest.mark.asyncio
    async def test_disconnect_cleans_up(self):
        """Test disconnect cleans up resources."""
        client = WebSocketClient()
        client._state = ConnectionState.CONNECTED
        mock_websocket = MagicMock()
        mock_websocket.close = AsyncMock()
        mock_session = MagicMock()
        mock_session.close = AsyncMock()
        client._websocket = mock_websocket
        client._session = mock_session
        client._ping_task = MagicMock()
        client._receive_task = MagicMock()

        await client.disconnect()

        assert client._state == ConnectionState.DISCONNECTED
        mock_websocket.close.assert_called_once()
        mock_session.close.assert_called_once()


class TestConnectionManager:
    """Tests for ConnectionManager class."""

    def test_init(self):
        """Test manager initialization."""
        manager = ConnectionManager()

        assert manager.connection_count == 0
        assert manager._connections == {}
        assert manager._subscriptions == {}

    @pytest.mark.asyncio
    async def test_connect_registers_client(self):
        """Test connect registers a client."""
        manager = ConnectionManager()
        ws = MagicMock()

        await manager.connect("client-1", ws)

        assert manager.connection_count == 1
        assert "client-1" in manager._connections

    @pytest.mark.asyncio
    async def test_disconnect_removes_client(self):
        """Test disconnect removes a client."""
        manager = ConnectionManager()
        ws = MagicMock()

        await manager.connect("client-1", ws)
        await manager.disconnect("client-1")

        assert manager.connection_count == 0
        assert "client-1" not in manager._connections

    @pytest.mark.asyncio
    async def test_disconnect_removes_from_subscriptions(self):
        """Test disconnect removes client from all subscriptions."""
        manager = ConnectionManager()
        ws = MagicMock()

        await manager.connect("client-1", ws)
        manager.subscribe("client-1", "status")
        manager.subscribe("client-1", "tasks")

        await manager.disconnect("client-1")

        assert "client-1" not in manager.get_subscribers("status")
        assert "client-1" not in manager.get_subscribers("tasks")

    def test_subscribe(self):
        """Test subscribing to a topic."""
        manager = ConnectionManager()

        manager.subscribe("client-1", "status")

        assert "client-1" in manager.get_subscribers("status")

    def test_subscribe_multiple_clients(self):
        """Test multiple clients subscribing to same topic."""
        manager = ConnectionManager()

        manager.subscribe("client-1", "status")
        manager.subscribe("client-2", "status")

        subscribers = manager.get_subscribers("status")
        assert "client-1" in subscribers
        assert "client-2" in subscribers

    def test_unsubscribe(self):
        """Test unsubscribing from a topic."""
        manager = ConnectionManager()
        manager.subscribe("client-1", "status")

        manager.unsubscribe("client-1", "status")

        assert "client-1" not in manager.get_subscribers("status")

    def test_unsubscribe_nonexistent_topic(self):
        """Test unsubscribing from non-existent topic."""
        manager = ConnectionManager()

        # Should not raise
        manager.unsubscribe("client-1", "nonexistent")

    @pytest.mark.asyncio
    async def test_broadcast(self):
        """Test broadcasting to subscribers."""
        manager = ConnectionManager()
        ws1 = MagicMock()
        ws1.send_str = AsyncMock()
        ws2 = MagicMock()
        ws2.send_str = AsyncMock()

        await manager.connect("client-1", ws1)
        await manager.connect("client-2", ws2)
        manager.subscribe("client-1", "status")
        manager.subscribe("client-2", "status")

        msg = WebSocketMessage(
            type=MessageType.STATUS_UPDATE,
            payload={"status": "running"},
        )
        count = await manager.broadcast("status", msg)

        assert count == 2
        ws1.send_str.assert_called_once()
        ws2.send_str.assert_called_once()

    @pytest.mark.asyncio
    async def test_broadcast_no_subscribers(self):
        """Test broadcasting to topic with no subscribers."""
        manager = ConnectionManager()

        msg = WebSocketMessage(type=MessageType.STATUS_UPDATE)
        count = await manager.broadcast("status", msg)

        assert count == 0

    @pytest.mark.asyncio
    async def test_broadcast_handles_send_errors(self):
        """Test broadcast handles send errors gracefully."""
        manager = ConnectionManager()
        ws = MagicMock()
        ws.send_str = AsyncMock(side_effect=Exception("Send failed"))

        await manager.connect("client-1", ws)
        manager.subscribe("client-1", "status")

        msg = WebSocketMessage(type=MessageType.STATUS_UPDATE)
        count = await manager.broadcast("status", msg)

        # Should still return 0 (no successful sends)
        assert count == 0

    @pytest.mark.asyncio
    async def test_send_to(self):
        """Test sending to specific client."""
        manager = ConnectionManager()
        ws = MagicMock()
        ws.send_str = AsyncMock()

        await manager.connect("client-1", ws)

        msg = WebSocketMessage(type=MessageType.ACK)
        result = await manager.send_to("client-1", msg)

        assert result is True
        ws.send_str.assert_called_once()

    @pytest.mark.asyncio
    async def test_send_to_nonexistent(self):
        """Test sending to non-existent client."""
        manager = ConnectionManager()

        msg = WebSocketMessage(type=MessageType.ACK)
        result = await manager.send_to("nonexistent", msg)

        assert result is False

    def test_get_subscribers_returns_copy(self):
        """Test get_subscribers returns a copy."""
        manager = ConnectionManager()
        manager.subscribe("client-1", "status")

        subscribers = manager.get_subscribers("status")
        subscribers.add("client-2")

        # Original should not be modified
        assert "client-2" not in manager.get_subscribers("status")
