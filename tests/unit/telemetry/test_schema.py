"""Tests for telemetry schema."""

from datetime import datetime, timezone

import pytest

from c4.telemetry.schema import TelemetryEvent


class TestTelemetryEvent:
    """Tests for TelemetryEvent dataclass."""

    def test_create_minimal_event(self) -> None:
        """Test creating event with required fields only."""
        event = TelemetryEvent(
            event_type="tool_call",
            anonymous_id="abc123",
        )
        assert event.event_type == "tool_call"
        assert event.anonymous_id == "abc123"
        assert event.tool_name is None
        assert event.metadata == {}
        assert isinstance(event.timestamp, datetime)

    def test_create_full_event(self) -> None:
        """Test creating event with all fields."""
        ts = datetime(2025, 1, 31, 12, 0, 0, tzinfo=timezone.utc)
        event = TelemetryEvent(
            event_type="task_complete",
            anonymous_id="xyz789",
            timestamp=ts,
            tool_name="c4_quick",
            metadata={"duration_ms": 1500, "success": True},
        )
        assert event.event_type == "task_complete"
        assert event.anonymous_id == "xyz789"
        assert event.timestamp == ts
        assert event.tool_name == "c4_quick"
        assert event.metadata == {"duration_ms": 1500, "success": True}

    def test_empty_event_type_raises(self) -> None:
        """Test that empty event_type raises ValueError."""
        with pytest.raises(ValueError, match="event_type cannot be empty"):
            TelemetryEvent(event_type="", anonymous_id="abc123")

    def test_empty_anonymous_id_raises(self) -> None:
        """Test that empty anonymous_id raises ValueError."""
        with pytest.raises(ValueError, match="anonymous_id cannot be empty"):
            TelemetryEvent(event_type="tool_call", anonymous_id="")

    def test_to_dict(self) -> None:
        """Test serialization to dictionary."""
        ts = datetime(2025, 1, 31, 12, 0, 0, tzinfo=timezone.utc)
        event = TelemetryEvent(
            event_type="error",
            anonymous_id="test123",
            timestamp=ts,
            tool_name="c4_submit",
            metadata={"error_code": "E001"},
        )
        result = event.to_dict()
        assert result == {
            "event_type": "error",
            "anonymous_id": "test123",
            "timestamp": "2025-01-31T12:00:00+00:00",
            "tool_name": "c4_submit",
            "metadata": {"error_code": "E001"},
        }

    def test_from_dict(self) -> None:
        """Test deserialization from dictionary."""
        data = {
            "event_type": "tool_call",
            "anonymous_id": "abc123",
            "timestamp": "2025-01-31T12:00:00+00:00",
            "tool_name": "c4_quick",
            "metadata": {"success": True},
        }
        event = TelemetryEvent.from_dict(data)
        assert event.event_type == "tool_call"
        assert event.anonymous_id == "abc123"
        assert event.tool_name == "c4_quick"
        assert event.metadata == {"success": True}
        assert event.timestamp.year == 2025

    def test_from_dict_minimal(self) -> None:
        """Test deserialization with minimal fields."""
        data = {
            "event_type": "tool_call",
            "anonymous_id": "abc123",
        }
        event = TelemetryEvent.from_dict(data)
        assert event.event_type == "tool_call"
        assert event.anonymous_id == "abc123"
        assert event.tool_name is None
        assert event.metadata == {}

    def test_roundtrip_serialization(self) -> None:
        """Test that to_dict -> from_dict preserves data."""
        original = TelemetryEvent(
            event_type="task_complete",
            anonymous_id="roundtrip",
            tool_name="c4_validate",
            metadata={"tests_passed": 10, "tests_failed": 0},
        )
        data = original.to_dict()
        restored = TelemetryEvent.from_dict(data)
        assert restored.event_type == original.event_type
        assert restored.anonymous_id == original.anonymous_id
        assert restored.tool_name == original.tool_name
        assert restored.metadata == original.metadata
