"""Knowledge tool handlers for MCP.

New (v2): c4_knowledge_search, c4_knowledge_record, c4_knowledge_get
Legacy (delegated): c4_experiment_search, c4_experiment_record, c4_pattern_suggest
"""

import os
from pathlib import Path
from typing import Any

from ..registry import register_tool


def _get_document_store():
    """Get Obsidian-style document store."""
    from c4.knowledge.documents import DocumentStore

    root = Path(os.environ.get("C4_PROJECT_ROOT", "."))
    return DocumentStore(base_path=root / ".c4" / "knowledge")


def _get_searcher():
    """Get hybrid searcher."""
    from c4.knowledge.search import KnowledgeSearcher

    root = Path(os.environ.get("C4_PROJECT_ROOT", "."))
    return KnowledgeSearcher(base_path=root / ".c4" / "knowledge")


# =============================================================================
# New v2 handlers
# =============================================================================


@register_tool("c4_knowledge_search")
def handle_knowledge_search(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Hybrid search over knowledge documents (vector + FTS5).

    Args (via arguments):
        query: Search query string (required)
        top_k: Max results (default: 10)
        filters: Optional dict with type, domain, hypothesis_status

    Returns:
        Search results with RRF scores.
    """
    query = arguments.get("query")
    if not query:
        return {"error": "query is required"}

    top_k = arguments.get("top_k", 10)
    filters = arguments.get("filters")

    try:
        searcher = _get_searcher()
        results = searcher.search(query, top_k=top_k, filters=filters)
        return {
            "count": len(results),
            "results": results,
        }
    except Exception as e:
        return {"error": f"Search failed: {e}"}


@register_tool("c4_knowledge_record")
def handle_knowledge_record(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Create a knowledge document (experiment/pattern/insight/hypothesis).

    Args (via arguments):
        doc_type: "experiment", "pattern", "insight", "hypothesis" (required)
        title: Document title (required)
        body: Markdown body content
        ... other metadata fields (domain, tags, hypothesis, etc.)

    Returns:
        Created document ID.
    """
    doc_type = arguments.get("doc_type")
    title = arguments.get("title")

    if not doc_type:
        return {"error": "doc_type is required"}
    if not title:
        return {"error": "title is required"}

    valid_types = {"experiment", "pattern", "insight", "hypothesis"}
    if doc_type not in valid_types:
        return {"error": f"Invalid doc_type: {doc_type}. Must be one of {valid_types}"}

    body = arguments.get("body", "")

    # Whitelist: only known KnowledgeDocument fields are accepted as metadata
    _VALID_METADATA_FIELDS = {
        "title", "domain", "tags", "task_id",
        "hypothesis", "hypothesis_status", "parent_experiment",
        "compared_to", "builds_on",
        "confidence", "evidence_count", "evidence_ids",
        "insight_type", "source_count",
        "status", "evidence_for", "evidence_against",
        "id", "created_at",
    }
    metadata = {
        k: v for k, v in arguments.items()
        if k in _VALID_METADATA_FIELDS
    }

    try:
        store = _get_document_store()
        doc_id = store.create(doc_type, metadata, body=body)
        return {
            "success": True,
            "doc_id": doc_id,
            "message": f"Document created: {doc_id}",
        }
    except Exception as e:
        return {"error": f"Recording failed: {e}"}


@register_tool("c4_knowledge_get")
def handle_knowledge_get(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get a knowledge document by ID (full content + backlinks).

    Args (via arguments):
        doc_id: Document ID (required)

    Returns:
        Full document content with backlinks.
    """
    doc_id = arguments.get("doc_id")
    if not doc_id:
        return {"error": "doc_id is required"}

    try:
        store = _get_document_store()
        doc = store.get(doc_id)
        if doc is None:
            return {"error": f"Document not found: {doc_id}"}

        backlinks = store.get_backlinks(doc_id)

        result = doc.model_dump()
        result["backlinks"] = backlinks
        return result
    except Exception as e:
        return {"error": f"Get failed: {e}"}


# =============================================================================
# Legacy handlers (delegate to v2)
# =============================================================================


@register_tool("c4_experiment_search")
def handle_experiment_search(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Search experiment knowledge (legacy - delegates to c4_knowledge_search)."""
    query = arguments.get("query")
    if not query:
        return {"error": "query is required"}

    top_k = arguments.get("top_k", 5)
    domain = arguments.get("domain")

    filters = {"type": "experiment"}
    if domain:
        filters["domain"] = domain

    try:
        searcher = _get_searcher()
        results = searcher.search(query, top_k=top_k, filters=filters)
        return {
            "count": len(results),
            "experiments": results,
        }
    except Exception as e:
        return {"error": f"Search failed: {e}"}


@register_tool("c4_experiment_record")
def handle_experiment_record(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Record experiment result (legacy - delegates to c4_knowledge_record)."""
    title = arguments.get("title")
    if not title:
        return {"error": "title is required"}

    result = handle_knowledge_record(daemon, {
        "doc_type": "experiment",
        "title": title,
        "task_id": arguments.get("task_id", ""),
        "hypothesis": arguments.get("hypothesis", ""),
        "tags": arguments.get("tags", []),
        "domain": arguments.get("domain", ""),
        "body": _build_experiment_body(arguments),
    })
    # Backward compat: legacy callers expect "experiment_id"
    if "doc_id" in result:
        result["experiment_id"] = result["doc_id"]
    return result


@register_tool("c4_pattern_suggest")
def handle_pattern_suggest(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get pattern-based suggestions (uses v2 search for patterns)."""
    domain = arguments.get("domain")
    limit = arguments.get("limit", 10)

    try:
        searcher = _get_searcher()
        filters: dict[str, str] = {"type": "pattern"}
        if domain:
            filters["domain"] = domain

        results = searcher.search("pattern", top_k=limit, filters=filters)

        return {
            "pattern_count": len(results),
            "patterns": results,
        }
    except Exception as e:
        return {"error": f"Pattern suggestion failed: {e}"}


def _build_experiment_body(arguments: dict[str, Any]) -> str:
    """Build Markdown body from legacy experiment arguments."""
    parts = [f"# {arguments.get('title', 'Experiment')}"]

    if arguments.get("hypothesis"):
        parts.append(f"\n## Hypothesis\n{arguments['hypothesis']}")

    config = arguments.get("config", {})
    if config:
        config_lines = [f"- {k}: {v}" for k, v in config.items()]
        parts.append("\n## Config\n" + "\n".join(config_lines))

    result = arguments.get("result", {})
    if result:
        metrics = result.get("metrics", {})
        if metrics:
            metric_lines = [f"- {k}: {v}" for k, v in metrics.items()]
            parts.append("\n## Result\n" + "\n".join(metric_lines))
        parts.append(f"\n- success: {result.get('success', True)}")

    lessons = arguments.get("lessons_learned", [])
    if lessons:
        lesson_lines = [f"- {item}" for item in lessons]
        parts.append("\n## Lessons Learned\n" + "\n".join(lesson_lines))

    return "\n".join(parts)
