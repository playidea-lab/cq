"""Telemetry client for sending events to Supabase."""

from __future__ import annotations

import asyncio
import hashlib
import logging
import os
import platform
import socket
import threading
import uuid
from pathlib import Path
from typing import TYPE_CHECKING

from c4.telemetry.schema import TelemetryEvent

if TYPE_CHECKING:
    from supabase import Client

logger = logging.getLogger(__name__)

# Configuration
TELEMETRY_TABLE = "c4_telemetry"
DEFAULT_BATCH_SIZE = 10
DEFAULT_FLUSH_INTERVAL = 30  # seconds


def get_anonymous_id() -> str:
    """Generate a stable anonymous ID for the device.

    Uses a combination of:
    - MAC address (via uuid.getnode())
    - Hostname

    The result is hashed to ensure privacy while maintaining consistency
    across sessions on the same machine.

    Returns:
        A SHA-256 hash string (first 16 characters) identifying this device.
    """
    try:
        # Get MAC address as integer
        mac = uuid.getnode()
        # Get hostname
        hostname = socket.gethostname()
        # Combine for uniqueness
        device_string = f"{mac}-{hostname}"
    except Exception:
        # Fallback: use a random ID stored in a file
        device_string = _get_or_create_fallback_id()

    # Hash for privacy
    hash_digest = hashlib.sha256(device_string.encode()).hexdigest()
    return hash_digest[:16]


def _get_or_create_fallback_id() -> str:
    """Get or create a fallback ID stored in user's home directory."""
    fallback_path = Path.home() / ".c4" / ".device_id"
    try:
        if fallback_path.exists():
            return fallback_path.read_text().strip()
        else:
            fallback_path.parent.mkdir(parents=True, exist_ok=True)
            device_id = str(uuid.uuid4())
            fallback_path.write_text(device_id)
            return device_id
    except Exception:
        # Last resort: generate a new ID each time (not ideal but safe)
        return str(uuid.uuid4())


class TelemetryClient:
    """Client for sending telemetry events to Supabase.

    Events are buffered and sent in batches for efficiency.
    Failures are logged but never block the main application.

    Example:
        >>> client = TelemetryClient()
        >>> event = TelemetryEvent(
        ...     event_type="tool_call",
        ...     anonymous_id=get_anonymous_id(),
        ...     tool_name="c4_quick",
        ...     metadata={"duration_ms": 150}
        ... )
        >>> await client.send(event)
    """

    def __init__(
        self,
        *,
        enabled: bool | None = None,
        batch_size: int = DEFAULT_BATCH_SIZE,
        flush_interval: int = DEFAULT_FLUSH_INTERVAL,
    ) -> None:
        """Initialize the telemetry client.

        Args:
            enabled: Whether telemetry is enabled. Defaults to TELEMETRY_ENABLED env var.
            batch_size: Number of events to buffer before auto-flush.
            flush_interval: Seconds between auto-flushes (0 to disable).
        """
        # Check enabled state from env if not explicitly set
        if enabled is None:
            env_enabled = os.getenv("TELEMETRY_ENABLED", "true").lower()
            enabled = env_enabled in ("true", "1", "yes")

        self._enabled = enabled
        self._batch_size = batch_size
        self._flush_interval = flush_interval
        self._buffer: list[TelemetryEvent] = []
        self._lock = threading.RLock()
        self._client: Client | None = None
        self._session_id = str(uuid.uuid4())[:8]
        self._c4_version = self._get_c4_version()
        self._flush_task: asyncio.Task | None = None

        # Initialize Supabase client if enabled
        if self._enabled:
            self._client = self._init_supabase()

    def _get_c4_version(self) -> str:
        """Get C4 version from package."""
        try:
            from c4 import __version__
            return __version__
        except Exception:
            return "unknown"

    def _init_supabase(self) -> Client | None:
        """Initialize Supabase client from environment variables.

        Returns:
            Supabase client or None if not configured.
        """
        url = os.getenv("SUPABASE_URL")
        key = os.getenv("SUPABASE_KEY")

        if not url or not key:
            logger.debug("Supabase not configured - telemetry will buffer locally only")
            return None

        try:
            from supabase import create_client
            client = create_client(url, key)
            logger.debug("Telemetry Supabase client initialized")
            return client
        except Exception as e:
            logger.warning(f"Failed to initialize Supabase client: {e}")
            return None

    @property
    def enabled(self) -> bool:
        """Whether telemetry collection is enabled."""
        return self._enabled

    @enabled.setter
    def enabled(self, value: bool) -> None:
        """Set telemetry enabled state."""
        self._enabled = value
        if value and self._client is None:
            self._client = self._init_supabase()

    @property
    def buffer_size(self) -> int:
        """Current number of buffered events."""
        with self._lock:
            return len(self._buffer)

    async def send(self, event: TelemetryEvent) -> None:
        """Send a telemetry event (buffered).

        Events are buffered and sent in batches for efficiency.
        Auto-flushes when batch_size is reached.

        Args:
            event: The telemetry event to send.
        """
        if not self._enabled:
            return

        with self._lock:
            self._buffer.append(event)
            should_flush = len(self._buffer) >= self._batch_size

        if should_flush:
            await self._flush()

    def send_sync(self, event: TelemetryEvent) -> None:
        """Synchronous version of send for non-async contexts.

        Args:
            event: The telemetry event to send.
        """
        if not self._enabled:
            return

        with self._lock:
            self._buffer.append(event)
            should_flush = len(self._buffer) >= self._batch_size

        if should_flush:
            # Run flush in background thread to not block
            threading.Thread(target=self._flush_sync, daemon=True).start()

    def _flush_sync(self) -> None:
        """Synchronous flush for threading context."""
        try:
            asyncio.run(self._flush())
        except Exception as e:
            logger.debug(f"Sync flush failed: {e}")

    async def _flush(self) -> None:
        """Flush buffered events to Supabase.

        Failures are logged but never raise exceptions.
        """
        with self._lock:
            if not self._buffer:
                return
            events = self._buffer.copy()
            self._buffer.clear()

        if not self._client:
            logger.debug(f"No Supabase client - discarding {len(events)} events")
            return

        try:
            # Convert events to database rows
            rows = [self._event_to_row(e) for e in events]

            # Insert to Supabase
            self._client.table(TELEMETRY_TABLE).insert(rows).execute()

            logger.debug(f"Telemetry: sent {len(events)} events")

        except Exception as e:
            # Never fail - telemetry should not affect main app
            logger.warning(f"Telemetry flush failed: {e}")

    def _event_to_row(self, event: TelemetryEvent) -> dict:
        """Convert TelemetryEvent to database row format."""
        return {
            "event_type": event.event_type,
            "anonymous_id": event.anonymous_id,
            "event_timestamp": event.timestamp.isoformat(),
            "tool_name": event.tool_name,
            "metadata": event.metadata,
            "session_id": self._session_id,
            "c4_version": self._c4_version,
            "platform": platform.system().lower(),
        }

    def flush(self) -> list[TelemetryEvent]:
        """Flush and return buffered events (for testing).

        Returns:
            List of buffered events (clears the buffer).
        """
        with self._lock:
            events = self._buffer.copy()
            self._buffer.clear()
        return events

    async def flush_async(self) -> int:
        """Flush buffered events to server.

        Returns:
            Number of events flushed.
        """
        with self._lock:
            count = len(self._buffer)

        if count > 0:
            await self._flush()

        return count

    def get_device_info(self) -> dict[str, str]:
        """Get basic device info for telemetry context.

        Returns:
            Dictionary with platform info (no sensitive data).
        """
        return {
            "platform": platform.system(),
            "platform_version": platform.version(),
            "python_version": platform.python_version(),
        }

    async def start_auto_flush(self) -> None:
        """Start background auto-flush task."""
        if self._flush_interval <= 0:
            return

        async def _auto_flush_loop():
            while True:
                await asyncio.sleep(self._flush_interval)
                try:
                    await self._flush()
                except Exception as e:
                    logger.debug(f"Auto-flush failed: {e}")

        self._flush_task = asyncio.create_task(_auto_flush_loop())

    async def stop(self) -> None:
        """Stop auto-flush and flush remaining events."""
        if self._flush_task:
            self._flush_task.cancel()
            try:
                await self._flush_task
            except asyncio.CancelledError:
                pass

        # Final flush
        await self._flush()


# Global client instance (lazy initialization)
_global_client: TelemetryClient | None = None


def get_telemetry_client() -> TelemetryClient:
    """Get or create the global telemetry client.

    Returns:
        The global TelemetryClient instance.
    """
    global _global_client
    if _global_client is None:
        _global_client = TelemetryClient()
    return _global_client


def track_tool_call(
    tool_name: str,
    *,
    success: bool = True,
    duration_ms: int | None = None,
    error: str | None = None,
) -> None:
    """Convenience function to track a tool call.

    Args:
        tool_name: Name of the MCP tool (e.g., "c4_quick", "c4_submit")
        success: Whether the call succeeded
        duration_ms: Call duration in milliseconds
        error: Error message if failed (truncated to 100 chars)
    """
    client = get_telemetry_client()
    if not client.enabled:
        return

    metadata: dict = {"success": success}
    if duration_ms is not None:
        metadata["duration_ms"] = duration_ms
    if error:
        metadata["error"] = error[:100]  # Truncate to avoid large payloads

    event = TelemetryEvent(
        event_type="tool_call",
        anonymous_id=get_anonymous_id(),
        tool_name=tool_name,
        metadata=metadata,
    )

    client.send_sync(event)


def track_state_change(
    from_state: str,
    to_state: str,
) -> None:
    """Convenience function to track state machine transition.

    Args:
        from_state: Previous project state
        to_state: New project state
    """
    client = get_telemetry_client()
    if not client.enabled:
        return

    event = TelemetryEvent(
        event_type="state_change",
        anonymous_id=get_anonymous_id(),
        metadata={
            "from_state": from_state,
            "to_state": to_state,
        },
    )

    client.send_sync(event)
