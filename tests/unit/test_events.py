"""Tests for c4.bridge.events.EventCollector."""

from __future__ import annotations

from c4.bridge.events import EventCollector


class TestEventCollector:
    """EventCollector unit tests."""

    def test_emit_and_attach(self) -> None:
        ec = EventCollector()
        ec.emit("c2.document.parsed", "c4.c2", {"file_path": "/a/b.pdf"})

        result = ec.attach({"success": True})
        assert "_events" in result
        assert len(result["_events"]) == 1

        ev = result["_events"][0]
        assert ev["type"] == "c2.document.parsed"
        assert ev["source"] == "c4.c2"
        assert ev["data"]["file_path"] == "/a/b.pdf"
        assert ev["project_id"] == ""

    def test_attach_no_events(self) -> None:
        ec = EventCollector()
        result = ec.attach({"success": True})
        assert "_events" not in result

    def test_multiple_events(self) -> None:
        ec = EventCollector()
        ec.emit("a.event", "src-a", {"k": 1})
        ec.emit("b.event", "src-b", {"k": 2}, project_id="proj-1")

        result = ec.attach({})
        assert len(result["_events"]) == 2
        assert result["_events"][0]["type"] == "a.event"
        assert result["_events"][1]["type"] == "b.event"
        assert result["_events"][1]["project_id"] == "proj-1"

    def test_emit_default_data(self) -> None:
        ec = EventCollector()
        ec.emit("test.event", "test")

        result = ec.attach({})
        assert result["_events"][0]["data"] == {}

    def test_attach_preserves_existing_keys(self) -> None:
        ec = EventCollector()
        ec.emit("x", "y")

        result = ec.attach({"foo": "bar", "count": 42})
        assert result["foo"] == "bar"
        assert result["count"] == 42
        assert "_events" in result
