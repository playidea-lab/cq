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
        "metrics": execution_stats.get("metrics", {}),
        "code_features": execution_stats.get("code_features", {}),
        "data_profile": execution_stats.get("data_profile", {}),
        "git_context": execution_stats.get("git_context", {}),
        "run_time_sec": execution_stats.get("run_time_sec", 0),
    }


def _save_to_knowledge_store(record: dict[str, Any]) -> None:
    """Save record to the local knowledge store."""
    import asyncio

    from c4.knowledge.models import ExperimentKnowledge, ExperimentResult
    from c4.knowledge.store import LocalKnowledgeStore

    experiment = ExperimentKnowledge(
        experiment_id=record["task_id"],
        title=record["title"],
        domain="ml",
        result=ExperimentResult(
            metrics=record["metrics"],
            success=True,
        ),
        observations=[],
        tags=list(record.get("code_features", {}).get("imports", []))[:5],
    )

    store = LocalKnowledgeStore()

    loop = asyncio.new_event_loop()
    try:
        loop.run_until_complete(store.save_experiment(experiment))
    finally:
        loop.close()
