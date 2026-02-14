"""Tests for c4.bridge.events.EventCollector and event type constants."""

from __future__ import annotations

from c4.bridge.events import (
    C2_DOCUMENT_PARSED,
    C2_TEXT_EXTRACTED,
    KNOWLEDGE_RECORDED,
    RESEARCH_RECORDED,
    RESEARCH_STARTED,
    SRC_C2,
    SRC_KNOWLEDGE,
    SRC_RESEARCH,
    EventCollector,
)


class TestEventCollector:
    """EventCollector unit tests."""

    def test_emit_and_attach(self) -> None:
        ec = EventCollector()
        ec.emit(C2_DOCUMENT_PARSED, SRC_C2, {"file_path": "/a/b.pdf"})

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


class TestEventConstants:
    """Verify event type constants are consistent."""

    def test_c2_event_types(self) -> None:
        assert C2_DOCUMENT_PARSED == "c2.document.parsed"
        assert C2_TEXT_EXTRACTED == "c2.text.extracted"

    def test_knowledge_event_types(self) -> None:
        assert KNOWLEDGE_RECORDED == "knowledge.recorded"

    def test_research_event_types(self) -> None:
        assert RESEARCH_STARTED == "research.started"
        assert RESEARCH_RECORDED == "research.recorded"

    def test_source_constants(self) -> None:
        assert SRC_C2 == "c4.c2"
        assert SRC_KNOWLEDGE == "c4.knowledge"
        assert SRC_RESEARCH == "c4.research"
