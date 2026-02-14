"""Event collector for piggybacking events on JSON-RPC responses.

Python sidecar methods use EventCollector to emit events that are
transparently forwarded to the Go EventBus via the BridgeProxy's
response-piggyback mechanism.

Usage::

    collector = EventCollector()
    collector.emit(C2_DOCUMENT_PARSED, SRC_C2, {"file_path": "/a/b.pdf"})
    return collector.attach({"success": True, "block_count": 5})
    # → {"success": True, "block_count": 5, "_events": [...]}
"""

from __future__ import annotations

from typing import Any

# ---------------------------------------------------------------------------
# Event type constants — keep in sync with Go EventBus consumers.
# ---------------------------------------------------------------------------
C2_DOCUMENT_PARSED = "c2.document.parsed"
C2_TEXT_EXTRACTED = "c2.text.extracted"
KNOWLEDGE_RECORDED = "knowledge.recorded"
RESEARCH_STARTED = "research.started"
RESEARCH_RECORDED = "research.recorded"
KNOWLEDGE_SEARCHED = "knowledge.searched"

# Source identifiers
SRC_C2 = "c4.c2"
SRC_KNOWLEDGE = "c4.knowledge"
SRC_RESEARCH = "c4.research"


class EventCollector:
    """Collects events and attaches them to a JSON-RPC response dict."""

    def __init__(self) -> None:
        self._events: list[dict[str, Any]] = []

    def emit(
        self,
        event_type: str,
        source: str,
        data: dict[str, Any] | None = None,
        project_id: str = "",
    ) -> None:
        """Record an event to be published by the Go EventBus."""
        self._events.append({
            "type": event_type,
            "source": source,
            "data": data or {},
            "project_id": project_id,
        })

    def attach(self, response: dict[str, Any]) -> dict[str, Any]:
        """Attach collected events to a response dict.

        Only adds ``_events`` key if there are events to send.
        Returns the same dict (mutated in place) for convenience.
        """
        if self._events:
            response["_events"] = self._events
        return response
