"""C4 Realtime Channel Manager for Supabase Realtime.

Manages Supabase Realtime channels for real-time synchronization
between multiple workers and the dashboard.

Features:
- Channel subscription/unsubscription management
- Connection state monitoring
- Automatic reconnection with exponential backoff
- Multiple channel support (postgres_changes, broadcast, presence)

Usage:
    from c4.realtime.manager import RealtimeManager, RealtimeConfig

    config = RealtimeConfig(
        supabase_url="https://xxx.supabase.co",
        supabase_key="your-anon-key",
    )

    manager = RealtimeManager(config)

    # Subscribe to table changes
    def on_task_change(payload):
        print(f"Task changed: {payload}")

    manager.subscribe_table(
        table="tasks",
        event="*",  # INSERT, UPDATE, DELETE, or *
        callback=on_task_change,
    )

    # Start listening
    manager.connect()

    # Later: cleanup
    manager.disconnect()
"""

from __future__ import annotations

import asyncio
import json
import logging
import threading
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable, TypeAlias

logger = logging.getLogger(__name__)


class ChannelState(Enum):
    """State of a realtime channel."""

    DISCONNECTED = "disconnected"
    CONNECTING = "connecting"
    CONNECTED = "connected"
    SUBSCRIBING = "subscribing"
    SUBSCRIBED = "subscribed"
    UNSUBSCRIBING = "unsubscribing"
    ERROR = "error"


# Type alias for callbacks
RealtimeCallback: TypeAlias = Callable[[dict[str, Any]], None]


@dataclass
class RealtimeConfig:
    """Configuration for RealtimeManager.

    Args:
        supabase_url: Supabase project URL
        supabase_key: Supabase anon key (or service role key)
        access_token: Optional JWT for authenticated connections
        auto_reconnect: Automatically reconnect on disconnect
        max_reconnect_attempts: Maximum reconnection attempts
        reconnect_interval: Initial reconnect interval (seconds)
        heartbeat_interval: WebSocket heartbeat interval (seconds)
        timeout: Connection timeout (seconds)
    """

    supabase_url: str
    supabase_key: str
    access_token: str | None = None
    auto_reconnect: bool = True
    max_reconnect_attempts: int = 10
    reconnect_interval: float = 1.0
    heartbeat_interval: float = 30.0
    timeout: float = 10.0


@dataclass
class RealtimeChannel:
    """Represents a subscribed channel.

    Args:
        name: Channel name (e.g., "realtime:public:tasks")
        topic: Topic within the channel
        event: Event type (INSERT, UPDATE, DELETE, *)
        callback: Function to call when event occurs
        filter_column: Optional column to filter by
        filter_value: Optional value to filter by
        state: Current channel state
    """

    name: str
    topic: str
    event: str
    callback: RealtimeCallback
    filter_column: str | None = None
    filter_value: str | None = None
    state: ChannelState = ChannelState.DISCONNECTED
    metadata: dict[str, Any] = field(default_factory=dict)


class RealtimeManager:
    """Manages Supabase Realtime connections and channels.

    This class handles:
    - WebSocket connection to Supabase Realtime
    - Channel subscription/unsubscription
    - Automatic reconnection with exponential backoff
    - Connection state monitoring
    - Heartbeat management

    Thread Safety:
        All public methods are thread-safe. Internal state is protected
        by locks. Callbacks are executed in a dedicated thread.
    """

    def __init__(self, config: RealtimeConfig):
        """Initialize RealtimeManager.

        Args:
            config: Configuration for the manager
        """
        self._config = config
        self._channels: dict[str, RealtimeChannel] = {}
        self._state = ChannelState.DISCONNECTED
        self._lock = threading.RLock()
        self._stop_event = threading.Event()

        # Connection management
        self._ws = None  # WebSocket connection (when using websockets library)
        self._ws_thread: threading.Thread | None = None
        self._heartbeat_thread: threading.Thread | None = None
        self._reconnect_attempts = 0

        # Callbacks
        self._on_connect: Callable[[], None] | None = None
        self._on_disconnect: Callable[[str], None] | None = None
        self._on_error: Callable[[Exception], None] | None = None
        self._on_reconnect: Callable[[int], None] | None = None

        # Event loop for async operations
        self._loop: asyncio.AbstractEventLoop | None = None

    @property
    def state(self) -> ChannelState:
        """Get current connection state."""
        with self._lock:
            return self._state

    @property
    def is_connected(self) -> bool:
        """Check if connected and subscribed."""
        return self.state in (ChannelState.CONNECTED, ChannelState.SUBSCRIBED)

    @property
    def channels(self) -> list[str]:
        """Get list of subscribed channel names."""
        with self._lock:
            return list(self._channels.keys())

    def get_realtime_url(self) -> str:
        """Get WebSocket URL for Supabase Realtime.

        Returns:
            WebSocket URL string
        """
        # Convert HTTP(S) URL to WebSocket URL
        url = self._config.supabase_url.replace("https://", "wss://")
        url = url.replace("http://", "ws://")

        # Append realtime path
        if not url.endswith("/"):
            url += "/"
        url += "realtime/v1/websocket"

        # Add API key
        url += f"?apikey={self._config.supabase_key}&vsn=1.0.0"

        return url

    def subscribe_table(
        self,
        table: str,
        event: str = "*",
        callback: RealtimeCallback | None = None,
        schema: str = "public",
        filter_column: str | None = None,
        filter_value: str | None = None,
    ) -> str:
        """Subscribe to postgres_changes for a table.

        Args:
            table: Table name to subscribe to
            event: Event type: INSERT, UPDATE, DELETE, or * for all
            callback: Function to call when event occurs
            schema: Database schema (default: public)
            filter_column: Optional column to filter by (eq filter)
            filter_value: Optional value to filter by

        Returns:
            Channel name (for unsubscribing)

        Example:
            # Subscribe to all changes on tasks table
            manager.subscribe_table("tasks", "*", on_task_change)

            # Subscribe to specific project's tasks
            manager.subscribe_table(
                "tasks",
                "UPDATE",
                on_task_update,
                filter_column="project_id",
                filter_value="proj-123",
            )
        """
        # Build channel name
        channel_name = f"realtime:{schema}:{table}"
        if filter_column and filter_value:
            channel_name += f":{filter_column}={filter_value}"

        topic = "postgres_changes"

        channel = RealtimeChannel(
            name=channel_name,
            topic=topic,
            event=event.upper() if event != "*" else "*",
            callback=callback or (lambda x: None),
            filter_column=filter_column,
            filter_value=filter_value,
            metadata={
                "schema": schema,
                "table": table,
                "type": "postgres_changes",
            },
        )

        with self._lock:
            self._channels[channel_name] = channel
            logger.info(f"Registered channel: {channel_name} for {event} events")

        # If already connected, subscribe immediately
        if self.is_connected:
            self._subscribe_channel(channel)

        return channel_name

    def subscribe_broadcast(
        self,
        channel_name: str,
        event: str,
        callback: RealtimeCallback,
    ) -> str:
        """Subscribe to broadcast messages.

        Broadcast is used for ephemeral, fast messages between clients
        that don't need to be persisted.

        Args:
            channel_name: Channel to subscribe to
            event: Event name to listen for
            callback: Function to call when message received

        Returns:
            Full channel name
        """
        full_name = f"broadcast:{channel_name}:{event}"

        channel = RealtimeChannel(
            name=full_name,
            topic="broadcast",
            event=event,
            callback=callback,
            metadata={
                "channel": channel_name,
                "type": "broadcast",
            },
        )

        with self._lock:
            self._channels[full_name] = channel
            logger.info(f"Registered broadcast channel: {full_name}")

        if self.is_connected:
            self._subscribe_channel(channel)

        return full_name

    def subscribe_presence(
        self,
        channel_name: str,
        callback: RealtimeCallback,
    ) -> str:
        """Subscribe to presence changes.

        Presence tracks which users/workers are online and their state.

        Args:
            channel_name: Channel to track presence on
            callback: Function to call on presence changes

        Returns:
            Full channel name
        """
        full_name = f"presence:{channel_name}"

        channel = RealtimeChannel(
            name=full_name,
            topic="presence",
            event="*",
            callback=callback,
            metadata={
                "channel": channel_name,
                "type": "presence",
            },
        )

        with self._lock:
            self._channels[full_name] = channel
            logger.info(f"Registered presence channel: {full_name}")

        if self.is_connected:
            self._subscribe_channel(channel)

        return full_name

    def unsubscribe(self, channel_name: str) -> bool:
        """Unsubscribe from a channel.

        Args:
            channel_name: Channel name returned from subscribe_*

        Returns:
            True if unsubscribed, False if not found
        """
        with self._lock:
            if channel_name not in self._channels:
                logger.warning(f"Channel not found: {channel_name}")
                return False

            channel = self._channels[channel_name]
            channel.state = ChannelState.UNSUBSCRIBING

            # Send unsubscribe message if connected
            if self.is_connected:
                self._unsubscribe_channel(channel)

            del self._channels[channel_name]
            logger.info(f"Unsubscribed from channel: {channel_name}")
            return True

    def unsubscribe_all(self) -> None:
        """Unsubscribe from all channels."""
        with self._lock:
            for channel_name in list(self._channels.keys()):
                self.unsubscribe(channel_name)

    def connect(self) -> bool:
        """Connect to Supabase Realtime.

        Establishes WebSocket connection and subscribes to all
        registered channels.

        Returns:
            True if connection initiated successfully

        Note:
            This is non-blocking. Use on_connect callback to know
            when connection is established.
        """
        with self._lock:
            if self._state != ChannelState.DISCONNECTED:
                logger.warning(f"Already in state: {self._state}")
                return False

            self._state = ChannelState.CONNECTING
            self._stop_event.clear()

        # Start WebSocket thread
        self._ws_thread = threading.Thread(
            target=self._ws_loop,
            name="c4-realtime-ws",
            daemon=True,
        )
        self._ws_thread.start()

        logger.info("Connecting to Supabase Realtime...")
        return True

    def disconnect(self) -> None:
        """Disconnect from Supabase Realtime.

        Cleanly closes WebSocket connection and stops all threads.
        """
        logger.info("Disconnecting from Supabase Realtime...")
        self._stop_event.set()

        # Close WebSocket
        if self._ws:
            try:
                # Schedule close in the event loop
                if self._loop and self._loop.is_running():
                    asyncio.run_coroutine_threadsafe(
                        self._ws.close(), self._loop
                    )
            except Exception as e:
                logger.warning(f"Error closing WebSocket: {e}")

        # Wait for threads to stop
        if self._ws_thread and self._ws_thread.is_alive():
            self._ws_thread.join(timeout=5)

        if self._heartbeat_thread and self._heartbeat_thread.is_alive():
            self._heartbeat_thread.join(timeout=2)

        with self._lock:
            self._state = ChannelState.DISCONNECTED
            for channel in self._channels.values():
                channel.state = ChannelState.DISCONNECTED

        logger.info("Disconnected from Supabase Realtime")

    def on_connect(self, callback: Callable[[], None]) -> None:
        """Set callback for successful connection.

        Args:
            callback: Function to call when connected
        """
        self._on_connect = callback

    def on_disconnect(self, callback: Callable[[str], None]) -> None:
        """Set callback for disconnection.

        Args:
            callback: Function to call with disconnect reason
        """
        self._on_disconnect = callback

    def on_error(self, callback: Callable[[Exception], None]) -> None:
        """Set callback for errors.

        Args:
            callback: Function to call with exception
        """
        self._on_error = callback

    def on_reconnect(self, callback: Callable[[int], None]) -> None:
        """Set callback for reconnection attempts.

        Args:
            callback: Function to call with attempt number
        """
        self._on_reconnect = callback

    def broadcast(
        self,
        channel_name: str,
        event: str,
        payload: dict[str, Any],
    ) -> bool:
        """Send a broadcast message.

        Args:
            channel_name: Channel to broadcast on
            event: Event name
            payload: Data to send

        Returns:
            True if message sent (connection may still fail)
        """
        if not self.is_connected:
            logger.warning("Cannot broadcast: not connected")
            return False

        message = {
            "topic": f"realtime:{channel_name}",
            "event": "broadcast",
            "payload": {
                "event": event,
                "payload": payload,
            },
            "ref": str(int(time.time() * 1000)),
        }

        return self._send_message(message)

    def track_presence(
        self,
        channel_name: str,
        state: dict[str, Any],
    ) -> bool:
        """Track presence state.

        Args:
            channel_name: Channel to track on
            state: Presence state to share

        Returns:
            True if message sent
        """
        if not self.is_connected:
            logger.warning("Cannot track presence: not connected")
            return False

        message = {
            "topic": f"realtime:{channel_name}",
            "event": "presence",
            "payload": {
                "type": "presence_state",
                "state": state,
            },
            "ref": str(int(time.time() * 1000)),
        }

        return self._send_message(message)

    # ========== Private Methods ==========

    def _ws_loop(self) -> None:
        """WebSocket event loop (runs in thread)."""
        try:
            import websockets.sync.client as ws_sync
        except ImportError:
            logger.error(
                "websockets package required. Install with: uv add websockets"
            )
            with self._lock:
                self._state = ChannelState.ERROR
            return

        while not self._stop_event.is_set():
            try:
                url = self.get_realtime_url()
                logger.debug(f"Connecting to: {url}")

                # Use sync websockets for simplicity
                with ws_sync.connect(
                    url,
                    close_timeout=5,
                    open_timeout=self._config.timeout,
                ) as websocket:
                    self._ws = websocket
                    self._on_ws_connected()

                    # Start heartbeat
                    self._start_heartbeat()

                    # Subscribe to all channels
                    with self._lock:
                        for channel in self._channels.values():
                            self._subscribe_channel(channel)

                    # Message receive loop
                    while not self._stop_event.is_set():
                        try:
                            message = websocket.recv(timeout=1.0)
                            self._on_message(message)
                        except TimeoutError:
                            continue
                        except Exception as e:
                            if not self._stop_event.is_set():
                                logger.warning(f"Receive error: {e}")
                            break

            except Exception as e:
                logger.error(f"WebSocket error: {e}")
                self._on_ws_error(e)

            # Reconnection logic
            if not self._stop_event.is_set() and self._config.auto_reconnect:
                self._attempt_reconnect()
            else:
                break

    def _on_ws_connected(self) -> None:
        """Handle successful WebSocket connection."""
        with self._lock:
            self._state = ChannelState.CONNECTED
            self._reconnect_attempts = 0

        logger.info("Connected to Supabase Realtime")

        if self._on_connect:
            try:
                self._on_connect()
            except Exception as e:
                logger.error(f"on_connect callback error: {e}")

    def _on_ws_error(self, error: Exception) -> None:
        """Handle WebSocket error."""
        with self._lock:
            self._state = ChannelState.ERROR

        if self._on_error:
            try:
                self._on_error(error)
            except Exception as e:
                logger.error(f"on_error callback error: {e}")

    def _on_message(self, raw_message: str) -> None:
        """Handle incoming WebSocket message."""
        try:
            message = json.loads(raw_message)
            event = message.get("event")
            topic = message.get("topic")
            payload = message.get("payload", {})

            logger.debug(f"Received: {event} on {topic}")

            if event == "phx_reply":
                # Phoenix reply to our messages
                self._handle_reply(message)
            elif event == "postgres_changes":
                # Database change event
                self._handle_postgres_change(topic, payload)
            elif event == "broadcast":
                # Broadcast message
                self._handle_broadcast(topic, payload)
            elif event == "presence_diff":
                # Presence change
                self._handle_presence(topic, payload)
            elif event == "system":
                # System message
                logger.debug(f"System message: {payload}")

        except json.JSONDecodeError as e:
            logger.warning(f"Invalid JSON message: {e}")
        except Exception as e:
            logger.error(f"Error handling message: {e}")

    def _handle_reply(self, message: dict) -> None:
        """Handle Phoenix reply message."""
        status = message.get("payload", {}).get("status")
        ref = message.get("ref")

        if status == "ok":
            logger.debug(f"Message {ref} acknowledged")
        elif status == "error":
            response = message.get("payload", {}).get("response", {})
            logger.error(f"Message {ref} error: {response}")

    def _handle_postgres_change(
        self, topic: str, payload: dict[str, Any]
    ) -> None:
        """Handle postgres_changes event."""
        with self._lock:
            for channel in self._channels.values():
                if channel.metadata.get("type") != "postgres_changes":
                    continue

                # Check if event matches
                event_type = payload.get("eventType", "").upper()
                if channel.event != "*" and channel.event != event_type:
                    continue

                # Check if table matches
                schema = payload.get("schema")
                table = payload.get("table")
                if (
                    channel.metadata.get("schema") != schema
                    or channel.metadata.get("table") != table
                ):
                    continue

                # Check filter if present
                if channel.filter_column and channel.filter_value:
                    record = payload.get("new") or payload.get("old", {})
                    if record.get(channel.filter_column) != channel.filter_value:
                        continue

                # Call callback
                try:
                    channel.callback(payload)
                except Exception as e:
                    logger.error(
                        f"Callback error for {channel.name}: {e}"
                    )

    def _handle_broadcast(self, topic: str, payload: dict[str, Any]) -> None:
        """Handle broadcast message."""
        event = payload.get("event")

        with self._lock:
            for channel in self._channels.values():
                if channel.metadata.get("type") != "broadcast":
                    continue

                if channel.event == event:
                    try:
                        channel.callback(payload.get("payload", {}))
                    except Exception as e:
                        logger.error(f"Broadcast callback error: {e}")

    def _handle_presence(self, topic: str, payload: dict[str, Any]) -> None:
        """Handle presence change."""
        with self._lock:
            for channel in self._channels.values():
                if channel.metadata.get("type") != "presence":
                    continue

                try:
                    channel.callback(payload)
                except Exception as e:
                    logger.error(f"Presence callback error: {e}")

    def _subscribe_channel(self, channel: RealtimeChannel) -> None:
        """Send subscription message for a channel."""
        channel.state = ChannelState.SUBSCRIBING

        if channel.metadata.get("type") == "postgres_changes":
            # Postgres changes subscription
            config = {
                "event": channel.event,
                "schema": channel.metadata.get("schema", "public"),
                "table": channel.metadata.get("table"),
            }

            if channel.filter_column and channel.filter_value:
                config["filter"] = f"{channel.filter_column}=eq.{channel.filter_value}"

            message = {
                "topic": f"realtime:{channel.metadata['schema']}:{channel.metadata['table']}",
                "event": "phx_join",
                "payload": {
                    "config": {
                        "postgres_changes": [config],
                    },
                },
                "ref": str(int(time.time() * 1000)),
            }

        elif channel.metadata.get("type") == "broadcast":
            # Broadcast subscription
            message = {
                "topic": f"realtime:{channel.metadata['channel']}",
                "event": "phx_join",
                "payload": {
                    "config": {
                        "broadcast": {"self": True},
                    },
                },
                "ref": str(int(time.time() * 1000)),
            }

        elif channel.metadata.get("type") == "presence":
            # Presence subscription
            message = {
                "topic": f"realtime:{channel.metadata['channel']}",
                "event": "phx_join",
                "payload": {
                    "config": {
                        "presence": {"key": ""},
                    },
                },
                "ref": str(int(time.time() * 1000)),
            }
        else:
            logger.warning(f"Unknown channel type: {channel.metadata}")
            return

        if self._send_message(message):
            channel.state = ChannelState.SUBSCRIBED
            logger.info(f"Subscribed to {channel.name}")

    def _unsubscribe_channel(self, channel: RealtimeChannel) -> None:
        """Send unsubscription message for a channel."""
        topic = channel.name.replace(":", "/")

        message = {
            "topic": topic,
            "event": "phx_leave",
            "payload": {},
            "ref": str(int(time.time() * 1000)),
        }

        self._send_message(message)

    def _send_message(self, message: dict) -> bool:
        """Send message through WebSocket."""
        if not self._ws:
            return False

        try:
            self._ws.send(json.dumps(message))
            return True
        except Exception as e:
            logger.error(f"Send error: {e}")
            return False

    def _start_heartbeat(self) -> None:
        """Start heartbeat thread."""
        if self._heartbeat_thread and self._heartbeat_thread.is_alive():
            return

        def heartbeat_loop():
            while not self._stop_event.is_set():
                if self.is_connected:
                    message = {
                        "topic": "phoenix",
                        "event": "heartbeat",
                        "payload": {},
                        "ref": str(int(time.time() * 1000)),
                    }
                    self._send_message(message)

                # Wait for next heartbeat
                self._stop_event.wait(self._config.heartbeat_interval)

        self._heartbeat_thread = threading.Thread(
            target=heartbeat_loop,
            name="c4-realtime-heartbeat",
            daemon=True,
        )
        self._heartbeat_thread.start()

    def _attempt_reconnect(self) -> None:
        """Attempt to reconnect with exponential backoff."""
        self._reconnect_attempts += 1

        if self._reconnect_attempts > self._config.max_reconnect_attempts:
            logger.error(
                f"Max reconnect attempts ({self._config.max_reconnect_attempts}) reached"
            )
            with self._lock:
                self._state = ChannelState.DISCONNECTED
            return

        # Calculate backoff with jitter
        backoff = min(
            self._config.reconnect_interval * (2 ** (self._reconnect_attempts - 1)),
            60.0,  # Max 60 seconds
        )

        logger.info(
            f"Reconnecting in {backoff:.1f}s "
            f"(attempt {self._reconnect_attempts}/{self._config.max_reconnect_attempts})"
        )

        if self._on_reconnect:
            try:
                self._on_reconnect(self._reconnect_attempts)
            except Exception as e:
                logger.error(f"on_reconnect callback error: {e}")

        # Wait before reconnecting
        self._stop_event.wait(backoff)
