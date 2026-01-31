"""Telemetry client for sending events to the server."""

from __future__ import annotations

import hashlib
import platform
import socket
import uuid
from pathlib import Path

from c4.telemetry.schema import TelemetryEvent


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
    """Client for sending telemetry events.

    Currently a stub implementation. The actual server integration
    will be implemented when the telemetry backend is ready.
    """

    def __init__(
        self,
        *,
        enabled: bool = True,
        endpoint: str | None = None,
    ) -> None:
        """Initialize the telemetry client.

        Args:
            enabled: Whether telemetry is enabled. Respects user preference.
            endpoint: Server endpoint URL (for future use).
        """
        self._enabled = enabled
        self._endpoint = endpoint
        self._buffer: list[TelemetryEvent] = []

    @property
    def enabled(self) -> bool:
        """Whether telemetry collection is enabled."""
        return self._enabled

    @enabled.setter
    def enabled(self, value: bool) -> None:
        """Set telemetry enabled state."""
        self._enabled = value

    async def send(self, event: TelemetryEvent) -> None:
        """Send a telemetry event to the server.

        Currently a no-op stub. Will be implemented when the
        telemetry backend is available.

        Args:
            event: The telemetry event to send.
        """
        if not self._enabled:
            return

        # Stub: just buffer the event for now
        # In the future, this will send to the telemetry server
        self._buffer.append(event)

    def send_sync(self, event: TelemetryEvent) -> None:
        """Synchronous version of send for non-async contexts.

        Args:
            event: The telemetry event to send.
        """
        if not self._enabled:
            return

        self._buffer.append(event)

    def flush(self) -> list[TelemetryEvent]:
        """Flush and return buffered events.

        Returns:
            List of buffered events (clears the buffer).
        """
        events = self._buffer.copy()
        self._buffer.clear()
        return events

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
