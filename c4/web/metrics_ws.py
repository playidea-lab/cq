"""WebSocket-based real-time metrics streaming.

Provides real-time metric updates via WebSocket:
- State machine status changes
- Task progress updates
- Worker status changes
"""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Callable

from starlette.websockets import WebSocket, WebSocketDisconnect

logger = logging.getLogger(__name__)


@dataclass
class MetricsMessage:
    """A metrics update message."""

    event_type: str
    data: dict[str, Any]
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    def to_json(self) -> str:
        """Convert to JSON string."""
        return json.dumps(
            {
                "event_type": self.event_type,
                "data": self.data,
                "timestamp": self.timestamp,
            }
        )


class MetricsWebSocket:
    """WebSocket manager for real-time metrics streaming.

    Manages WebSocket connections and broadcasts state changes
    to all connected clients.

    Usage:
        metrics_ws = MetricsWebSocket()

        # In FastAPI route:
        @app.websocket("/ws/metrics")
        async def metrics_endpoint(websocket: WebSocket):
            await metrics_ws.handle_connection(websocket)

        # When state changes:
        await metrics_ws.broadcast_state_change(new_state)
    """

    def __init__(self) -> None:
        """Initialize the WebSocket manager."""
        self._connections: set[WebSocket] = set()
        self._lock = asyncio.Lock()

    @property
    def connection_count(self) -> int:
        """Number of active connections."""
        return len(self._connections)

    async def handle_connection(self, websocket: WebSocket) -> None:
        """Handle a WebSocket connection lifecycle.

        Args:
            websocket: The WebSocket connection to handle
        """
        await self._connect(websocket)
        try:
            await self._listen(websocket)
        except WebSocketDisconnect:
            pass
        finally:
            await self._disconnect(websocket)

    async def _connect(self, websocket: WebSocket) -> None:
        """Accept and register a WebSocket connection.

        Args:
            websocket: The WebSocket to connect
        """
        await websocket.accept()
        async with self._lock:
            self._connections.add(websocket)

        client_info = self._get_client_info(websocket)
        logger.info(f"WebSocket connected: {client_info} (total: {self.connection_count})")

        # Send initial welcome message
        welcome = MetricsMessage(
            event_type="connected",
            data={"message": "Connected to C4 metrics stream"},
        )
        await websocket.send_text(welcome.to_json())

    async def _disconnect(self, websocket: WebSocket) -> None:
        """Unregister a WebSocket connection.

        Args:
            websocket: The WebSocket to disconnect
        """
        async with self._lock:
            self._connections.discard(websocket)

        client_info = self._get_client_info(websocket)
        logger.info(f"WebSocket disconnected: {client_info} (total: {self.connection_count})")

    async def _listen(self, websocket: WebSocket) -> None:
        """Listen for messages from client (keep-alive).

        Args:
            websocket: The WebSocket to listen on
        """
        while True:
            # Wait for messages (ping/pong or commands)
            data = await websocket.receive_text()
            # For now, we just acknowledge any message
            if data == "ping":
                await websocket.send_text(json.dumps({"event_type": "pong"}))

    async def broadcast(self, message: MetricsMessage) -> int:
        """Broadcast a message to all connected clients.

        Args:
            message: The message to broadcast

        Returns:
            Number of clients the message was sent to
        """
        if not self._connections:
            return 0

        json_data = message.to_json()
        sent_count = 0
        disconnected: list[WebSocket] = []

        async with self._lock:
            for websocket in self._connections:
                try:
                    await websocket.send_text(json_data)
                    sent_count += 1
                except Exception as e:
                    logger.warning(f"Failed to send to WebSocket: {e}")
                    disconnected.append(websocket)

            # Clean up disconnected clients
            for ws in disconnected:
                self._connections.discard(ws)

        return sent_count

    async def broadcast_state_change(self, state_data: dict[str, Any]) -> int:
        """Broadcast a state change to all clients.

        Args:
            state_data: The new state data

        Returns:
            Number of clients notified
        """
        message = MetricsMessage(
            event_type="state_change",
            data=state_data,
        )
        count = await self.broadcast(message)
        if count > 0:
            logger.debug(f"Broadcast state change to {count} clients")
        return count

    async def broadcast_task_update(
        self, task_id: str, status: str, extra: dict[str, Any] | None = None
    ) -> int:
        """Broadcast a task status update.

        Args:
            task_id: The task ID
            status: New task status
            extra: Additional data

        Returns:
            Number of clients notified
        """
        data = {"task_id": task_id, "status": status}
        if extra:
            data.update(extra)

        message = MetricsMessage(event_type="task_update", data=data)
        return await self.broadcast(message)

    async def broadcast_worker_update(
        self, worker_id: str, state: str, task_id: str | None = None
    ) -> int:
        """Broadcast a worker status update.

        Args:
            worker_id: The worker ID
            state: Worker state (idle, busy, etc.)
            task_id: Current task ID if any

        Returns:
            Number of clients notified
        """
        message = MetricsMessage(
            event_type="worker_update",
            data={
                "worker_id": worker_id,
                "state": state,
                "task_id": task_id,
            },
        )
        return await self.broadcast(message)

    async def close_all(self) -> None:
        """Close all WebSocket connections."""
        async with self._lock:
            for websocket in list(self._connections):
                try:
                    await websocket.close()
                except Exception:
                    pass
            self._connections.clear()

        logger.info("All WebSocket connections closed")

    def _get_client_info(self, websocket: WebSocket) -> str:
        """Get client info string for logging.

        Args:
            websocket: The WebSocket connection

        Returns:
            Client info string
        """
        client = websocket.client
        if client:
            return f"{client.host}:{client.port}"
        return "unknown"


# Global instance for app-wide use
_metrics_ws: MetricsWebSocket | None = None


def get_metrics_ws() -> MetricsWebSocket:
    """Get or create the global MetricsWebSocket instance.

    Returns:
        The global MetricsWebSocket instance
    """
    global _metrics_ws
    if _metrics_ws is None:
        _metrics_ws = MetricsWebSocket()
    return _metrics_ws


def create_state_change_callback(metrics_ws: MetricsWebSocket) -> Callable[..., None]:
    """Create a callback function for state machine changes.

    This callback can be registered with StateMachine to automatically
    broadcast state changes to all WebSocket clients.

    Args:
        metrics_ws: The MetricsWebSocket instance

    Returns:
        Callback function for state changes
    """

    def callback(event: str, old_status: str, new_status: str, **kwargs: Any) -> None:
        """Callback for state machine transitions."""
        # Use asyncio to schedule the broadcast
        try:
            loop = asyncio.get_event_loop()
            if loop.is_running():
                asyncio.create_task(
                    metrics_ws.broadcast_state_change(
                        {
                            "event": event,
                            "old_status": old_status,
                            "new_status": new_status,
                            **kwargs,
                        }
                    )
                )
        except RuntimeError:
            # No event loop running, skip broadcast
            pass

    return callback
