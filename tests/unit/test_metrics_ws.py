"""Tests for MetricsWebSocket."""

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.web.metrics_ws import (
    MetricsMessage,
    MetricsWebSocket,
    create_state_change_callback,
    get_metrics_ws,
)


class TestMetricsMessage:
    """Test MetricsMessage dataclass."""

    def test_to_json_includes_all_fields(self):
        """Should include event_type, data, and timestamp in JSON."""
        msg = MetricsMessage(
            event_type="state_change",
            data={"status": "EXECUTE"},
            timestamp="2026-01-01T00:00:00",
        )

        result = json.loads(msg.to_json())

        assert result["event_type"] == "state_change"
        assert result["data"] == {"status": "EXECUTE"}
        assert result["timestamp"] == "2026-01-01T00:00:00"

    def test_to_json_auto_generates_timestamp(self):
        """Should auto-generate timestamp if not provided."""
        msg = MetricsMessage(
            event_type="test",
            data={},
        )

        result = json.loads(msg.to_json())

        assert "timestamp" in result
        assert result["timestamp"] is not None


class TestMetricsWebSocket:
    """Test MetricsWebSocket class."""

    @pytest.fixture
    def metrics_ws(self):
        """Create a MetricsWebSocket instance."""
        return MetricsWebSocket()

    @pytest.fixture
    def mock_websocket(self):
        """Create a mock WebSocket."""
        ws = AsyncMock()
        ws.client = MagicMock()
        ws.client.host = "127.0.0.1"
        ws.client.port = 12345
        return ws

    @pytest.mark.asyncio
    async def test_connection_count_starts_at_zero(self, metrics_ws):
        """Should start with zero connections."""
        assert metrics_ws.connection_count == 0

    @pytest.mark.asyncio
    async def test_connect_adds_websocket(self, metrics_ws, mock_websocket):
        """Should add websocket to connections on connect."""
        await metrics_ws._connect(mock_websocket)

        assert metrics_ws.connection_count == 1
        mock_websocket.accept.assert_called_once()
        mock_websocket.send_text.assert_called_once()  # Welcome message

    @pytest.mark.asyncio
    async def test_disconnect_removes_websocket(self, metrics_ws, mock_websocket):
        """Should remove websocket from connections on disconnect."""
        await metrics_ws._connect(mock_websocket)
        assert metrics_ws.connection_count == 1

        await metrics_ws._disconnect(mock_websocket)

        assert metrics_ws.connection_count == 0

    @pytest.mark.asyncio
    async def test_broadcast_sends_to_all_connections(self, metrics_ws):
        """Should send message to all connected clients."""
        ws1 = AsyncMock()
        ws1.client = MagicMock(host="1.1.1.1", port=1111)
        ws2 = AsyncMock()
        ws2.client = MagicMock(host="2.2.2.2", port=2222)

        await metrics_ws._connect(ws1)
        await metrics_ws._connect(ws2)

        message = MetricsMessage(event_type="test", data={"value": 42})
        sent_count = await metrics_ws.broadcast(message)

        assert sent_count == 2
        # Each websocket should receive 2 messages: welcome + broadcast
        assert ws1.send_text.call_count == 2
        assert ws2.send_text.call_count == 2

    @pytest.mark.asyncio
    async def test_broadcast_returns_zero_when_no_connections(self, metrics_ws):
        """Should return 0 when no connections exist."""
        message = MetricsMessage(event_type="test", data={})
        sent_count = await metrics_ws.broadcast(message)

        assert sent_count == 0

    @pytest.mark.asyncio
    async def test_broadcast_removes_failed_connections(self, metrics_ws, mock_websocket):
        """Should remove connections that fail to receive."""
        await metrics_ws._connect(mock_websocket)

        # Reset mock and make next send fail
        mock_websocket.send_text.reset_mock()
        mock_websocket.send_text.side_effect = Exception("Connection closed")

        message = MetricsMessage(event_type="test", data={})
        await metrics_ws.broadcast(message)

        assert metrics_ws.connection_count == 0

    @pytest.mark.asyncio
    async def test_broadcast_state_change(self, metrics_ws, mock_websocket):
        """Should broadcast state changes with correct event type."""
        await metrics_ws._connect(mock_websocket)

        await metrics_ws.broadcast_state_change({"status": "EXECUTE"})

        # Check the second call (first is welcome message)
        calls = mock_websocket.send_text.call_args_list
        assert len(calls) == 2
        broadcast_data = json.loads(calls[1][0][0])
        assert broadcast_data["event_type"] == "state_change"
        assert broadcast_data["data"]["status"] == "EXECUTE"

    @pytest.mark.asyncio
    async def test_broadcast_task_update(self, metrics_ws, mock_websocket):
        """Should broadcast task updates with task_id and status."""
        await metrics_ws._connect(mock_websocket)

        await metrics_ws.broadcast_task_update("T-001-0", "done", {"worker": "w1"})

        calls = mock_websocket.send_text.call_args_list
        broadcast_data = json.loads(calls[1][0][0])
        assert broadcast_data["event_type"] == "task_update"
        assert broadcast_data["data"]["task_id"] == "T-001-0"
        assert broadcast_data["data"]["status"] == "done"
        assert broadcast_data["data"]["worker"] == "w1"

    @pytest.mark.asyncio
    async def test_broadcast_worker_update(self, metrics_ws, mock_websocket):
        """Should broadcast worker updates."""
        await metrics_ws._connect(mock_websocket)

        await metrics_ws.broadcast_worker_update("worker-1", "busy", "T-001-0")

        calls = mock_websocket.send_text.call_args_list
        broadcast_data = json.loads(calls[1][0][0])
        assert broadcast_data["event_type"] == "worker_update"
        assert broadcast_data["data"]["worker_id"] == "worker-1"
        assert broadcast_data["data"]["state"] == "busy"
        assert broadcast_data["data"]["task_id"] == "T-001-0"

    @pytest.mark.asyncio
    async def test_close_all_closes_all_connections(self, metrics_ws):
        """Should close all connections and clear the set."""
        ws1 = AsyncMock()
        ws1.client = MagicMock(host="1.1.1.1", port=1111)
        ws2 = AsyncMock()
        ws2.client = MagicMock(host="2.2.2.2", port=2222)

        await metrics_ws._connect(ws1)
        await metrics_ws._connect(ws2)
        assert metrics_ws.connection_count == 2

        await metrics_ws.close_all()

        assert metrics_ws.connection_count == 0
        ws1.close.assert_called_once()
        ws2.close.assert_called_once()

    @pytest.mark.asyncio
    async def test_listen_responds_to_ping(self, metrics_ws, mock_websocket):
        """Should respond with pong when receiving ping."""
        mock_websocket.receive_text.side_effect = ["ping", asyncio.CancelledError()]

        with pytest.raises(asyncio.CancelledError):
            await metrics_ws._listen(mock_websocket)

        # Find the pong response
        calls = [
            c for c in mock_websocket.send_text.call_args_list if "pong" in str(c)
        ]
        assert len(calls) == 1


class TestGetMetricsWs:
    """Test get_metrics_ws singleton function."""

    def test_returns_same_instance(self):
        """Should return the same instance on subsequent calls."""
        # Reset global state
        import c4.web.metrics_ws as module

        module._metrics_ws = None

        ws1 = get_metrics_ws()
        ws2 = get_metrics_ws()

        assert ws1 is ws2


class TestCreateStateChangeCallback:
    """Test create_state_change_callback function."""

    @pytest.mark.asyncio
    async def test_callback_broadcasts_state_change(self):
        """Should create a callback that broadcasts state changes."""
        metrics_ws = MetricsWebSocket()
        mock_ws = AsyncMock()
        mock_ws.client = MagicMock(host="127.0.0.1", port=12345)
        await metrics_ws._connect(mock_ws)

        callback = create_state_change_callback(metrics_ws)

        # Create an event loop context for the callback
        with patch("asyncio.get_event_loop") as mock_loop:
            mock_loop.return_value.is_running.return_value = True
            with patch("asyncio.create_task") as mock_create_task:
                callback("c4_run", "PLAN", "EXECUTE")
                mock_create_task.assert_called_once()
