"""Tests for telemetry client."""

import pytest

from c4.telemetry.client import TelemetryClient, get_anonymous_id
from c4.telemetry.schema import TelemetryEvent


class TestGetAnonymousId:
    """Tests for get_anonymous_id function."""

    def test_returns_string(self) -> None:
        """Test that anonymous ID is a string."""
        result = get_anonymous_id()
        assert isinstance(result, str)

    def test_returns_consistent_id(self) -> None:
        """Test that same device gets same ID."""
        id1 = get_anonymous_id()
        id2 = get_anonymous_id()
        assert id1 == id2

    def test_id_has_expected_length(self) -> None:
        """Test that ID is 16 characters (truncated SHA-256)."""
        result = get_anonymous_id()
        assert len(result) == 16

    def test_id_is_hex(self) -> None:
        """Test that ID contains only hex characters."""
        result = get_anonymous_id()
        assert all(c in "0123456789abcdef" for c in result)


class TestTelemetryClient:
    """Tests for TelemetryClient class."""

    def test_init_default(self) -> None:
        """Test default initialization."""
        client = TelemetryClient()
        assert client.enabled is True

    def test_init_disabled(self) -> None:
        """Test initialization with telemetry disabled."""
        client = TelemetryClient(enabled=False)
        assert client.enabled is False

    def test_enabled_setter(self) -> None:
        """Test enabling/disabling telemetry."""
        client = TelemetryClient()
        client.enabled = False
        assert client.enabled is False
        client.enabled = True
        assert client.enabled is True

    @pytest.mark.anyio
    async def test_send_buffers_event(self) -> None:
        """Test that send buffers events."""
        client = TelemetryClient()
        event = TelemetryEvent(
            event_type="tool_call",
            anonymous_id="test123",
            tool_name="c4_quick",
        )
        await client.send(event)
        buffered = client.flush()
        assert len(buffered) == 1
        assert buffered[0] == event

    @pytest.mark.anyio
    async def test_send_disabled_no_buffer(self) -> None:
        """Test that disabled client doesn't buffer."""
        client = TelemetryClient(enabled=False)
        event = TelemetryEvent(
            event_type="tool_call",
            anonymous_id="test123",
        )
        await client.send(event)
        buffered = client.flush()
        assert len(buffered) == 0

    def test_send_sync_buffers_event(self) -> None:
        """Test synchronous send."""
        client = TelemetryClient()
        event = TelemetryEvent(
            event_type="task_complete",
            anonymous_id="test456",
        )
        client.send_sync(event)
        buffered = client.flush()
        assert len(buffered) == 1
        assert buffered[0] == event

    def test_send_sync_disabled_no_buffer(self) -> None:
        """Test that disabled sync send doesn't buffer."""
        client = TelemetryClient(enabled=False)
        event = TelemetryEvent(
            event_type="task_complete",
            anonymous_id="test456",
        )
        client.send_sync(event)
        buffered = client.flush()
        assert len(buffered) == 0

    def test_flush_clears_buffer(self) -> None:
        """Test that flush clears the buffer."""
        client = TelemetryClient()
        event = TelemetryEvent(
            event_type="tool_call",
            anonymous_id="test789",
        )
        client.send_sync(event)
        client.flush()
        # Second flush should be empty
        buffered = client.flush()
        assert len(buffered) == 0

    @pytest.mark.anyio
    async def test_multiple_events_buffered(self) -> None:
        """Test buffering multiple events."""
        client = TelemetryClient()
        events = [
            TelemetryEvent(event_type="tool_call", anonymous_id="a1"),
            TelemetryEvent(event_type="task_complete", anonymous_id="a2"),
            TelemetryEvent(event_type="error", anonymous_id="a3"),
        ]
        for event in events:
            await client.send(event)
        buffered = client.flush()
        assert len(buffered) == 3
        assert buffered == events

    def test_get_device_info(self) -> None:
        """Test getting device info."""
        client = TelemetryClient()
        info = client.get_device_info()
        assert "platform" in info
        assert "platform_version" in info
        assert "python_version" in info
        # Should not contain sensitive info
        assert "username" not in info
        assert "hostname" not in info
