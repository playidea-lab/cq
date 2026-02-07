"""Knowledge hook - auto-save experiment results to Knowledge Store on task completion.

Triggers on AFTER_COMPLETE for tasks with execution_stats.
"""

from __future__ import annotations

import logging
from typing import Any

from c4.hooks.base import BaseHook, HookContext, HookPhase

logger = logging.getLogger(__name__)


class KnowledgeHook(BaseHook):
    """Save experiment results to Knowledge Store after task completion.

    When a task completes with execution_stats (from @c4_track), this hook
    automatically records the experiment in the Knowledge Store for future
    pattern mining and similarity search.
    """

    @property
    def name(self) -> str:
        return "knowledge_auto_save"

    @property
    def phase(self) -> HookPhase:
        return HookPhase.AFTER_COMPLETE

    def execute(self, context: HookContext) -> bool:
        """Save execution stats to Knowledge Store.

        Expects context.task_data to contain:
            - execution_stats: dict with metrics, code_features, etc.
            - title: task title (optional)
        """
        execution_stats = context.get("execution_stats")
        if not execution_stats:
            logger.debug("No execution_stats for task %s, skipping knowledge save", context.task_id)
            return True  # Not a failure, just nothing to do

        try:
            record = _build_experiment_record(context.task_id, context.task_data, execution_stats)
            _save_to_knowledge_store(record)
            logger.info("Knowledge saved for task %s", context.task_id)
            return True
        except Exception as e:
            logger.warning("Failed to save knowledge for task %s: %s", context.task_id, e)
            return False


def _build_experiment_record(
    task_id: str,
    task_data: dict[str, Any],
    execution_stats: dict[str, Any],
) -> dict[str, Any]:
    """Build a knowledge record from task completion data."""
    return {
        "task_id": task_id,
        "title": task_data.get("title", task_id),
        "domain": task_data.get("domain", "unknown"),
        "metrics": execution_stats.get("metrics", {}),
        "code_features": execution_stats.get("code_features", {}),
        "data_profile": execution_stats.get("data_profile", {}),
        "git_context": execution_stats.get("git_context", {}),
        "run_time_sec": execution_stats.get("run_time_sec", 0),
    }


def _save_to_knowledge_store(record: dict[str, Any]) -> None:
    """Save record to the knowledge store as a Markdown document."""
    from c4.knowledge.documents import DocumentStore

    store = DocumentStore()

    metrics = record.get("metrics", {})
    metrics_lines = [f"- {k}: {v}" for k, v in metrics.items()]
    body = f"# {record['title']}\n\n## Metrics\n" + "\n".join(metrics_lines) if metrics_lines else f"# {record['title']}"

    # Determine domain from task data or fall back to "unknown"
    domain = record.get("domain", "unknown")

    # Check for existing document with same task_id to avoid duplicates
    existing = store.list_documents(limit=1000)
    existing_doc = next((d for d in existing if d.get("task_id") == record["task_id"]), None)

    # Build metadata including code_features and data_profile in body
    code_features = record.get("code_features", {})
    data_profile = record.get("data_profile", {})

    if code_features:
        cf_lines = [f"- {k}: {v}" for k, v in code_features.items()]
        body += "\n\n## Code Features\n" + "\n".join(cf_lines)
    if data_profile:
        dp_lines = [f"- {k}: {v}" for k, v in data_profile.items()]
        body += "\n\n## Data Profile\n" + "\n".join(dp_lines)

    metadata = {
        "title": record["title"],
        "task_id": record["task_id"],
        "domain": domain,
        "tags": list(code_features.get("imports", []))[:5],
    }

    if existing_doc:
        doc_id = existing_doc["id"]
        store.update(doc_id, metadata=metadata, body=body)
    else:
        doc_id = store.create("experiment", metadata, body=body)

    # Auto-index embedding for semantic search (best-effort)
    try:
        from c4.knowledge.embeddings import KnowledgeEmbedder

        embedder = KnowledgeEmbedder()
        doc = store.get(doc_id)
        if doc:
            embedder.index_document(doc_id, doc.model_dump())
        embedder.close()
    except Exception:
        pass  # Embedding failure doesn't block hook
