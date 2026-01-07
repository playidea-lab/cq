"""C4 Event Bus - Event utilities and querying for daemon operations"""

from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from c4.models import Event, EventType


class EventBus:
    """
    Event utilities for C4 daemon.

    Note: Event emission is handled by StateMachine.emit_event().
    This class provides query and utility methods for events.
    """

    def __init__(self, events_dir: Path):
        self.events_dir = events_dir

    def get_events(
        self,
        since: datetime | None = None,
        event_type: "EventType | None" = None,
        limit: int = 100,
    ) -> list["Event"]:
        """
        Get events from the event log.

        Args:
            since: Only return events after this timestamp
            event_type: Filter by event type
            limit: Maximum number of events to return

        Returns:
            List of events, newest first
        """
        import json

        from c4.models import Event

        events: list[Event] = []

        if not self.events_dir.exists():
            return events

        # Get all event files, sorted by name (which includes timestamp)
        event_files = sorted(self.events_dir.glob("*.json"), reverse=True)

        for event_file in event_files:
            if len(events) >= limit:
                break

            try:
                data = json.loads(event_file.read_text())
                event = Event.model_validate(data)

                # Apply filters
                if since and event.ts < since:
                    continue
                if event_type and event.type != event_type:
                    continue

                events.append(event)
            except (json.JSONDecodeError, ValueError):
                # Skip invalid event files
                continue

        return events

    def get_latest_event(self, event_type: "EventType | None" = None) -> "Event | None":
        """Get the most recent event, optionally filtered by type"""
        events = self.get_events(event_type=event_type, limit=1)
        return events[0] if events else None

    def count_events(
        self,
        since: datetime | None = None,
        event_type: "EventType | None" = None,
    ) -> int:
        """Count events matching criteria"""
        return len(self.get_events(since=since, event_type=event_type, limit=10000))

    def get_event_summary(self) -> dict[str, int]:
        """Get count of events by type"""
        from collections import Counter

        events = self.get_events(limit=10000)
        type_counts = Counter(e.type.value for e in events)
        return dict(type_counts)
