"""C4 Realtime module for Supabase Realtime integration."""

from .manager import (
    ChannelState,
    RealtimeCallback,
    RealtimeChannel,
    RealtimeConfig,
    RealtimeManager,
)
from .sync import (
    SyncConfig,
    SyncEvent,
    WorkerInfo,
    WorkerSync,
    create_worker_sync,
)

__all__ = [
    # Manager
    "ChannelState",
    "RealtimeCallback",
    "RealtimeChannel",
    "RealtimeConfig",
    "RealtimeManager",
    # Sync
    "SyncConfig",
    "SyncEvent",
    "WorkerInfo",
    "WorkerSync",
    "create_worker_sync",
]
