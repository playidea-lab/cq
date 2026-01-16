"""C4 Connection - WebSocket client and server for real-time communication."""

from .websocket_client import (
    ConnectionConfig,
    ConnectionState,
    WebSocketClient,
    WebSocketMessage,
)

__all__ = [
    "ConnectionConfig",
    "ConnectionState",
    "WebSocketClient",
    "WebSocketMessage",
]
