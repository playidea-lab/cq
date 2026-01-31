"""Telemetry event schema for C4 usage tracking."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any


@dataclass
class TelemetryEvent:
    """Represents a single telemetry event.

    Attributes:
        event_type: Type of event (e.g., "tool_call", "task_complete", "error")
        anonymous_id: Device hash for identification without login
        timestamp: When the event occurred (UTC)
        tool_name: Name of the tool if applicable (e.g., "c4_quick", "c4_submit")
        metadata: Additional data (success/failure, duration - never code content)
    """

    event_type: str
    anonymous_id: str
    timestamp: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    tool_name: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        """Validate event fields."""
        if not self.event_type:
            raise ValueError("event_type cannot be empty")
        if not self.anonymous_id:
            raise ValueError("anonymous_id cannot be empty")

    def to_dict(self) -> dict[str, Any]:
        """Convert event to dictionary for serialization."""
        return {
            "event_type": self.event_type,
            "anonymous_id": self.anonymous_id,
            "timestamp": self.timestamp.isoformat(),
            "tool_name": self.tool_name,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> TelemetryEvent:
        """Create event from dictionary."""
        timestamp = data.get("timestamp")
        if isinstance(timestamp, str):
            timestamp = datetime.fromisoformat(timestamp)
        elif timestamp is None:
            timestamp = datetime.now(timezone.utc)

        return cls(
            event_type=data["event_type"],
            anonymous_id=data["anonymous_id"],
            timestamp=timestamp,
            tool_name=data.get("tool_name"),
            metadata=data.get("metadata", {}),
        )
