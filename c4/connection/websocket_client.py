"""WebSocket Client - Client for connecting to C4 daemon via WebSocket."""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Callable

logger = logging.getLogger(__name__)


class ConnectionState(str, Enum):
    """WebSocket connection states."""

    DISCONNECTED = "disconnected"
    CONNECTING = "connecting"
    CONNECTED = "connected"
    RECONNECTING = "reconnecting"
    ERROR = "error"


class MessageType(str, Enum):
    """WebSocket message types."""

    # Client -> Server
    SUBSCRIBE = "subscribe"
    UNSUBSCRIBE = "unsubscribe"
    PING = "ping"
    COMMAND = "command"

    # Server -> Client
    STATUS_UPDATE = "status_update"
    TASK_UPDATE = "task_update"
    WORKER_UPDATE = "worker_update"
    CHECKPOINT_UPDATE = "checkpoint_update"
    PONG = "pong"
    ERROR = "error"
    ACK = "ack"


@dataclass
class WebSocketMessage:
    """WebSocket message structure."""

    type: MessageType
    payload: dict[str, Any] = field(default_factory=dict)
    timestamp: datetime = field(default_factory=datetime.now)
    message_id: str | None = None

    def to_json(self) -> str:
        """Convert to JSON string."""
        return json.dumps(
            {
                "type": self.type.value,
                "payload": self.payload,
                "timestamp": self.timestamp.isoformat(),
                "message_id": self.message_id,
            }
        )

    @classmethod
    def from_json(cls, data: str) -> WebSocketMessage:
        """Create from JSON string."""
        parsed = json.loads(data)
        return cls(
            type=MessageType(parsed["type"]),
            payload=parsed.get("payload", {}),
            timestamp=datetime.fromisoformat(parsed["timestamp"]),
            message_id=parsed.get("message_id"),
        )


@dataclass
class ConnectionConfig:
    """WebSocket connection configuration."""

    host: str = "127.0.0.1"
    port: int = 4000
    path: str = "/ws"
    reconnect: bool = True
    reconnect_interval: float = 2.0
    max_reconnect_attempts: int = 5
    ping_interval: float = 30.0
    timeout: float = 10.0

    @property
    def url(self) -> str:
        """Get full WebSocket URL."""
        return f"ws://{self.host}:{self.port}{self.path}"


class WebSocketClient:
    """WebSocket client for C4 daemon communication.

    Features:
    - Automatic reconnection
    - Message subscription
    - Ping/pong heartbeat
    - Event callbacks

    Example:
        async def on_status(msg):
            print(f"Status: {msg.payload}")

        client = WebSocketClient()
        client.on_message(MessageType.STATUS_UPDATE, on_status)

        await client.connect()
        await client.subscribe("status")
    """

    def __init__(self, config: ConnectionConfig | None = None):
        """Initialize WebSocket client.

        Args:
            config: Connection configuration
        """
        self.config = config or ConnectionConfig()
        self._state = ConnectionState.DISCONNECTED
        self._websocket: Any = None  # aiohttp.ClientWebSocketResponse
        self._session: Any = None  # aiohttp.ClientSession
        self._handlers: dict[MessageType, list[Callable]] = {}
        self._reconnect_count = 0
        self._ping_task: asyncio.Task | None = None
        self._receive_task: asyncio.Task | None = None
        self._subscriptions: set[str] = set()

    @property
    def state(self) -> ConnectionState:
        """Get current connection state."""
        return self._state

    @property
    def is_connected(self) -> bool:
        """Check if connected."""
        return self._state == ConnectionState.CONNECTED

    # =========================================================================
    # Connection Management
    # =========================================================================

    async def connect(self) -> bool:
        """Connect to WebSocket server.

        Returns:
            True if connected successfully
        """
        if self._state == ConnectionState.CONNECTED:
            return True

        self._state = ConnectionState.CONNECTING
        logger.info(f"Connecting to {self.config.url}")

        try:
            import aiohttp

            self._session = aiohttp.ClientSession()
            self._websocket = await asyncio.wait_for(
                self._session.ws_connect(self.config.url),
                timeout=self.config.timeout,
            )

            self._state = ConnectionState.CONNECTED
            self._reconnect_count = 0
            logger.info("Connected successfully")

            # Start background tasks
            self._receive_task = asyncio.create_task(self._receive_loop())
            self._ping_task = asyncio.create_task(self._ping_loop())

            # Re-subscribe to previous subscriptions
            for topic in self._subscriptions:
                await self.subscribe(topic)

            return True

        except asyncio.TimeoutError:
            logger.warning("Connection timeout")
            self._state = ConnectionState.ERROR
            await self._handle_disconnect()
            return False
        except Exception as e:
            logger.error(f"Connection failed: {e}")
            self._state = ConnectionState.ERROR
            await self._handle_disconnect()
            return False

    async def disconnect(self) -> None:
        """Disconnect from WebSocket server."""
        self._state = ConnectionState.DISCONNECTED
        logger.info("Disconnecting")

        # Cancel background tasks
        if self._ping_task:
            self._ping_task.cancel()
            self._ping_task = None
        if self._receive_task:
            self._receive_task.cancel()
            self._receive_task = None

        # Close WebSocket
        if self._websocket:
            await self._websocket.close()
            self._websocket = None

        # Close session
        if self._session:
            await self._session.close()
            self._session = None

    async def _handle_disconnect(self) -> None:
        """Handle disconnection and attempt reconnection."""
        if self._websocket:
            await self._websocket.close()
            self._websocket = None

        if not self.config.reconnect:
            self._state = ConnectionState.DISCONNECTED
            return

        if self._reconnect_count >= self.config.max_reconnect_attempts:
            logger.error("Max reconnection attempts reached")
            self._state = ConnectionState.ERROR
            return

        self._state = ConnectionState.RECONNECTING
        self._reconnect_count += 1
        logger.info(f"Reconnecting ({self._reconnect_count}/{self.config.max_reconnect_attempts})")

        await asyncio.sleep(self.config.reconnect_interval)
        await self.connect()

    # =========================================================================
    # Message Handling
    # =========================================================================

    def on_message(
        self,
        message_type: MessageType,
        handler: Callable[[WebSocketMessage], Any],
    ) -> None:
        """Register a message handler.

        Args:
            message_type: Type of message to handle
            handler: Callback function
        """
        if message_type not in self._handlers:
            self._handlers[message_type] = []
        self._handlers[message_type].append(handler)

    def off_message(
        self,
        message_type: MessageType,
        handler: Callable[[WebSocketMessage], Any],
    ) -> None:
        """Remove a message handler.

        Args:
            message_type: Type of message
            handler: Callback to remove
        """
        if message_type in self._handlers:
            try:
                self._handlers[message_type].remove(handler)
            except ValueError:
                pass

    async def _dispatch_message(self, message: WebSocketMessage) -> None:
        """Dispatch message to registered handlers."""
        handlers = self._handlers.get(message.type, [])
        for handler in handlers:
            try:
                result = handler(message)
                if asyncio.iscoroutine(result):
                    await result
            except Exception as e:
                logger.error(f"Handler error: {e}")

    async def _receive_loop(self) -> None:
        """Background task to receive messages."""
        import aiohttp

        try:
            async for msg in self._websocket:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    try:
                        message = WebSocketMessage.from_json(msg.data)
                        await self._dispatch_message(message)
                    except json.JSONDecodeError:
                        logger.warning(f"Invalid JSON: {msg.data}")
                elif msg.type == aiohttp.WSMsgType.CLOSED:
                    logger.info("Connection closed by server")
                    await self._handle_disconnect()
                    break
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    logger.error(f"WebSocket error: {self._websocket.exception()}")
                    await self._handle_disconnect()
                    break
        except asyncio.CancelledError:
            pass
        except Exception as e:
            logger.error(f"Receive loop error: {e}")
            await self._handle_disconnect()

    async def _ping_loop(self) -> None:
        """Background task for ping/pong heartbeat."""
        try:
            while self._state == ConnectionState.CONNECTED:
                await asyncio.sleep(self.config.ping_interval)
                await self.send(WebSocketMessage(type=MessageType.PING))
        except asyncio.CancelledError:
            pass

    # =========================================================================
    # Messaging
    # =========================================================================

    async def send(self, message: WebSocketMessage) -> bool:
        """Send a message to the server.

        Args:
            message: Message to send

        Returns:
            True if sent successfully
        """
        if not self.is_connected or not self._websocket:
            logger.warning("Cannot send: not connected")
            return False

        try:
            await self._websocket.send_str(message.to_json())
            return True
        except Exception as e:
            logger.error(f"Send failed: {e}")
            return False

    async def subscribe(self, topic: str) -> bool:
        """Subscribe to a topic.

        Args:
            topic: Topic to subscribe (status, tasks, workers, checkpoints)

        Returns:
            True if subscription sent
        """
        self._subscriptions.add(topic)
        return await self.send(
            WebSocketMessage(
                type=MessageType.SUBSCRIBE,
                payload={"topic": topic},
            )
        )

    async def unsubscribe(self, topic: str) -> bool:
        """Unsubscribe from a topic.

        Args:
            topic: Topic to unsubscribe

        Returns:
            True if unsubscription sent
        """
        self._subscriptions.discard(topic)
        return await self.send(
            WebSocketMessage(
                type=MessageType.UNSUBSCRIBE,
                payload={"topic": topic},
            )
        )

    async def send_command(self, command: str, args: dict[str, Any] | None = None) -> bool:
        """Send a command to the server.

        Args:
            command: Command name (e.g., "run", "stop", "plan")
            args: Command arguments

        Returns:
            True if command sent
        """
        return await self.send(
            WebSocketMessage(
                type=MessageType.COMMAND,
                payload={
                    "command": command,
                    "args": args or {},
                },
            )
        )


class ConnectionManager:
    """Manages WebSocket connections for multiple clients.

    Used by the server to broadcast messages to connected clients.
    """

    def __init__(self):
        """Initialize connection manager."""
        self._connections: dict[str, Any] = {}  # client_id -> websocket
        self._subscriptions: dict[str, set[str]] = {}  # topic -> {client_ids}

    async def connect(self, client_id: str, websocket: Any) -> None:
        """Register a new connection.

        Args:
            client_id: Unique client identifier
            websocket: WebSocket connection
        """
        self._connections[client_id] = websocket
        logger.info(f"Client connected: {client_id}")

    async def disconnect(self, client_id: str) -> None:
        """Remove a connection.

        Args:
            client_id: Client identifier
        """
        if client_id in self._connections:
            del self._connections[client_id]

        # Remove from all subscriptions
        for topic in self._subscriptions:
            self._subscriptions[topic].discard(client_id)

        logger.info(f"Client disconnected: {client_id}")

    def subscribe(self, client_id: str, topic: str) -> None:
        """Subscribe a client to a topic.

        Args:
            client_id: Client identifier
            topic: Topic name
        """
        if topic not in self._subscriptions:
            self._subscriptions[topic] = set()
        self._subscriptions[topic].add(client_id)

    def unsubscribe(self, client_id: str, topic: str) -> None:
        """Unsubscribe a client from a topic.

        Args:
            client_id: Client identifier
            topic: Topic name
        """
        if topic in self._subscriptions:
            self._subscriptions[topic].discard(client_id)

    async def broadcast(self, topic: str, message: WebSocketMessage) -> int:
        """Broadcast a message to all subscribers of a topic.

        Args:
            topic: Topic name
            message: Message to broadcast

        Returns:
            Number of clients message was sent to
        """
        if topic not in self._subscriptions:
            return 0

        count = 0
        for client_id in self._subscriptions[topic]:
            if client_id in self._connections:
                try:
                    ws = self._connections[client_id]
                    await ws.send_str(message.to_json())
                    count += 1
                except Exception as e:
                    logger.error(f"Broadcast to {client_id} failed: {e}")

        return count

    async def send_to(self, client_id: str, message: WebSocketMessage) -> bool:
        """Send a message to a specific client.

        Args:
            client_id: Client identifier
            message: Message to send

        Returns:
            True if sent successfully
        """
        if client_id not in self._connections:
            return False

        try:
            ws = self._connections[client_id]
            await ws.send_str(message.to_json())
            return True
        except Exception as e:
            logger.error(f"Send to {client_id} failed: {e}")
            return False

    @property
    def connection_count(self) -> int:
        """Get number of active connections."""
        return len(self._connections)

    def get_subscribers(self, topic: str) -> set[str]:
        """Get client IDs subscribed to a topic."""
        return self._subscriptions.get(topic, set()).copy()
