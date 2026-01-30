"""Unit tests for EventBus in c4/daemon/events.py

Tests event querying, filtering, counting, and summary functions.
"""

import json
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from c4.daemon.events import EventBus
from c4.models.enums import EventType


class TestEventBusInit:
    """Tests for EventBus initialization."""

    def test_init_with_path(self, tmp_path):
        """Should initialize with given events directory path."""
        events_dir = tmp_path / "events"
        bus = EventBus(events_dir)

        assert bus.events_dir == events_dir


class TestEventBusGetEvents:
    """Tests for EventBus.get_events method."""

    @pytest.fixture
    def events_dir(self, tmp_path):
        """Create events directory."""
        events = tmp_path / "events"
        events.mkdir()
        return events

    @pytest.fixture
    def event_bus(self, events_dir):
        """Create EventBus instance."""
        return EventBus(events_dir)

    def _create_event_file(
        self,
        events_dir: Path,
        event_id: str,
        event_type: EventType,
        ts: datetime,
        actor: str = "test",
    ):
        """Helper to create event files."""
        event_data = {
            "id": event_id,
            "ts": ts.isoformat(),
            "type": event_type.value,
            "actor": actor,
            "data": {},
        }
        # Use timestamp in filename for sorting
        filename = f"{ts.strftime('%Y%m%d%H%M%S')}_{event_id}.json"
        event_file = events_dir / filename
        event_file.write_text(json.dumps(event_data))
        return event_file

    def test_get_events_returns_empty_list_when_no_events(self, event_bus):
        """Should return empty list when no event files exist."""
        result = event_bus.get_events()

        assert result == []

    def test_get_events_returns_empty_list_when_dir_not_exists(self, tmp_path):
        """Should return empty list when events directory doesn't exist."""
        bus = EventBus(tmp_path / "non_existent")

        result = bus.get_events()

        assert result == []

    def test_get_events_returns_events_sorted_by_time_desc(
        self, event_bus, events_dir
    ):
        """Should return events sorted by timestamp descending (newest first)."""
        now = datetime.now()
        older = now - timedelta(hours=1)
        oldest = now - timedelta(hours=2)

        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, oldest
        )
        self._create_event_file(
            events_dir, "002", EventType.TASK_ASSIGNED, older
        )
        self._create_event_file(
            events_dir, "003", EventType.WORKER_SUBMITTED, now
        )

        result = event_bus.get_events()

        assert len(result) == 3
        assert result[0].id == "003"  # newest first
        assert result[1].id == "002"
        assert result[2].id == "001"  # oldest last

    def test_get_events_filters_by_since_timestamp(self, event_bus, events_dir):
        """Should only return events after 'since' timestamp."""
        now = datetime.now()
        older = now - timedelta(hours=1)
        oldest = now - timedelta(hours=2)

        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, oldest
        )
        self._create_event_file(
            events_dir, "002", EventType.TASK_ASSIGNED, older
        )
        self._create_event_file(
            events_dir, "003", EventType.WORKER_SUBMITTED, now
        )

        # Filter events after 90 minutes ago
        since = now - timedelta(minutes=90)
        result = event_bus.get_events(since=since)

        assert len(result) == 2
        assert result[0].id == "003"
        assert result[1].id == "002"

    def test_get_events_filters_by_event_type(self, event_bus, events_dir):
        """Should only return events of specified type."""
        now = datetime.now()

        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, now - timedelta(hours=2)
        )
        self._create_event_file(
            events_dir, "002", EventType.TASK_ASSIGNED, now - timedelta(hours=1)
        )
        self._create_event_file(
            events_dir, "003", EventType.WORKER_JOINED, now
        )

        result = event_bus.get_events(event_type=EventType.WORKER_JOINED)

        assert len(result) == 2
        assert all(e.type == EventType.WORKER_JOINED for e in result)

    def test_get_events_respects_limit(self, event_bus, events_dir):
        """Should return at most 'limit' number of events."""
        now = datetime.now()

        for i in range(5):
            self._create_event_file(
                events_dir,
                f"00{i}",
                EventType.WORKER_JOINED,
                now - timedelta(minutes=i),
            )

        result = event_bus.get_events(limit=3)

        assert len(result) == 3

    def test_get_events_handles_invalid_json_gracefully(
        self, event_bus, events_dir
    ):
        """Should skip invalid JSON files without crashing."""
        now = datetime.now()

        # Create valid event
        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, now
        )

        # Create invalid JSON file
        invalid_file = events_dir / "invalid.json"
        invalid_file.write_text("not valid json {{{")

        result = event_bus.get_events()

        # Should only return valid event
        assert len(result) == 1
        assert result[0].id == "001"

    def test_get_events_handles_invalid_event_data_gracefully(
        self, event_bus, events_dir
    ):
        """Should skip files with invalid event data without crashing."""
        now = datetime.now()

        # Create valid event
        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, now
        )

        # Create file with invalid event data (missing required fields)
        invalid_event = events_dir / "invalid_event.json"
        invalid_event.write_text(json.dumps({"foo": "bar"}))

        result = event_bus.get_events()

        # Should only return valid event
        assert len(result) == 1


class TestEventBusGetLatestEvent:
    """Tests for EventBus.get_latest_event method."""

    @pytest.fixture
    def events_dir(self, tmp_path):
        """Create events directory."""
        events = tmp_path / "events"
        events.mkdir()
        return events

    @pytest.fixture
    def event_bus(self, events_dir):
        """Create EventBus instance."""
        return EventBus(events_dir)

    def _create_event_file(
        self,
        events_dir: Path,
        event_id: str,
        event_type: EventType,
        ts: datetime,
    ):
        """Helper to create event files."""
        event_data = {
            "id": event_id,
            "ts": ts.isoformat(),
            "type": event_type.value,
            "actor": "test",
            "data": {},
        }
        filename = f"{ts.strftime('%Y%m%d%H%M%S')}_{event_id}.json"
        event_file = events_dir / filename
        event_file.write_text(json.dumps(event_data))

    def test_get_latest_event_returns_none_when_empty(self, event_bus):
        """Should return None when no events exist."""
        result = event_bus.get_latest_event()

        assert result is None

    def test_get_latest_event_returns_most_recent(self, event_bus, events_dir):
        """Should return the most recent event."""
        now = datetime.now()

        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, now - timedelta(hours=1)
        )
        self._create_event_file(
            events_dir, "002", EventType.TASK_ASSIGNED, now
        )

        result = event_bus.get_latest_event()

        assert result is not None
        assert result.id == "002"

    def test_get_latest_event_filters_by_type(self, event_bus, events_dir):
        """Should return most recent event of specified type."""
        now = datetime.now()

        self._create_event_file(
            events_dir, "001", EventType.WORKER_JOINED, now - timedelta(hours=2)
        )
        self._create_event_file(
            events_dir, "002", EventType.TASK_ASSIGNED, now - timedelta(hours=1)
        )
        self._create_event_file(
            events_dir, "003", EventType.WORKER_JOINED, now
        )

        result = event_bus.get_latest_event(event_type=EventType.TASK_ASSIGNED)

        assert result is not None
        assert result.id == "002"
        assert result.type == EventType.TASK_ASSIGNED


class TestEventBusCountEvents:
    """Tests for EventBus.count_events method."""

    @pytest.fixture
    def events_dir(self, tmp_path):
        """Create events directory."""
        events = tmp_path / "events"
        events.mkdir()
        return events

    @pytest.fixture
    def event_bus(self, events_dir):
        """Create EventBus instance."""
        return EventBus(events_dir)

    def _create_event_file(
        self,
        events_dir: Path,
        event_id: str,
        event_type: EventType,
        ts: datetime,
    ):
        """Helper to create event files."""
        event_data = {
            "id": event_id,
            "ts": ts.isoformat(),
            "type": event_type.value,
            "actor": "test",
            "data": {},
        }
        filename = f"{ts.strftime('%Y%m%d%H%M%S')}_{event_id}.json"
        event_file = events_dir / filename
        event_file.write_text(json.dumps(event_data))

    def test_count_events_returns_zero_when_empty(self, event_bus):
        """Should return 0 when no events exist."""
        result = event_bus.count_events()

        assert result == 0

    def test_count_events_returns_correct_count(self, event_bus, events_dir):
        """Should return total count of events."""
        now = datetime.now()

        for i in range(5):
            self._create_event_file(
                events_dir,
                f"00{i}",
                EventType.WORKER_JOINED,
                now - timedelta(minutes=i),
            )

        result = event_bus.count_events()

        assert result == 5


class TestEventBusGetEventSummary:
    """Tests for EventBus.get_event_summary method."""

    @pytest.fixture
    def events_dir(self, tmp_path):
        """Create events directory."""
        events = tmp_path / "events"
        events.mkdir()
        return events

    @pytest.fixture
    def event_bus(self, events_dir):
        """Create EventBus instance."""
        return EventBus(events_dir)

    def _create_event_file(
        self,
        events_dir: Path,
        event_id: str,
        event_type: EventType,
        ts: datetime,
    ):
        """Helper to create event files."""
        event_data = {
            "id": event_id,
            "ts": ts.isoformat(),
            "type": event_type.value,
            "actor": "test",
            "data": {},
        }
        filename = f"{ts.strftime('%Y%m%d%H%M%S')}_{event_id}.json"
        event_file = events_dir / filename
        event_file.write_text(json.dumps(event_data))

    def test_get_event_summary_returns_empty_dict_when_no_events(
        self, event_bus
    ):
        """Should return empty dict when no events exist."""
        result = event_bus.get_event_summary()

        assert result == {}

    def test_get_event_summary_returns_type_counts(self, event_bus, events_dir):
        """Should return count of events grouped by type."""
        now = datetime.now()

        # 3 WORKER_JOINED events
        for i in range(3):
            self._create_event_file(
                events_dir,
                f"w{i}",
                EventType.WORKER_JOINED,
                now - timedelta(minutes=i * 10),
            )

        # 2 TASK_ASSIGNED events
        for i in range(2):
            self._create_event_file(
                events_dir,
                f"t{i}",
                EventType.TASK_ASSIGNED,
                now - timedelta(minutes=i * 10 + 5),
            )

        result = event_bus.get_event_summary()

        assert result[EventType.WORKER_JOINED.value] == 3
        assert result[EventType.TASK_ASSIGNED.value] == 2
