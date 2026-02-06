"""Knowledge tool handlers for MCP.

Handles: c4_experiment_search, c4_experiment_record, c4_pattern_suggest
"""

import os
from pathlib import Path
from typing import Any

from ..registry import register_tool


def _get_knowledge_store():
    """Get knowledge store instance."""
    from c4.knowledge.store import LocalKnowledgeStore

    root = Path(os.environ.get("C4_PROJECT_ROOT", "."))
    return LocalKnowledgeStore(base_path=root / ".c4" / "knowledge")


def _get_aggregator():
    """Get knowledge aggregator."""
    from c4.knowledge.aggregator import KnowledgeAggregator

    return KnowledgeAggregator()


def _get_miner():
    """Get pattern miner."""
    from c4.knowledge.miner import PatternMiner

    return PatternMiner()


@register_tool("c4_experiment_search")
def handle_experiment_search(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Search experiment knowledge.

    Args (via arguments):
        query: Search query string
        top_k: Max results (default: 5)
        domain: Optional domain filter

    Returns:
        Matching experiments.
    """
    query = arguments.get("query")
    if not query:
        return {"error": "query is required"}

    top_k = arguments.get("top_k", 5)
    domain = arguments.get("domain")

    try:
        import asyncio

        store = _get_knowledge_store()
        results = asyncio.get_event_loop().run_until_complete(
            store.search(query, top_k=top_k)
        )

        if domain:
            results = [r for r in results if r.get("domain") == domain]

        return {
            "count": len(results),
            "experiments": results,
        }
    except Exception as e:
        return {"error": f"Search failed: {e}"}


@register_tool("c4_experiment_record")
def handle_experiment_record(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Record experiment result to knowledge store.

    Args (via arguments):
        task_id: Related task ID
        title: Experiment title
        hypothesis: Hypothesis being tested
        config: Experiment configuration dict
        result: Result dict (metrics, success, error_message)
        lessons_learned: List of lesson strings
        tags: List of tag strings
        domain: Domain string

    Returns:
        Saved experiment ID.
    """
    task_id = arguments.get("task_id")
    title = arguments.get("title")

    if not title:
        return {"error": "title is required"}

    experiment = {
        "task_id": task_id or "",
        "title": title,
        "hypothesis": arguments.get("hypothesis", ""),
        "config": arguments.get("config", {}),
        "result": arguments.get("result", {"success": True}),
        "lessons_learned": arguments.get("lessons_learned", []),
        "tags": arguments.get("tags", []),
        "domain": arguments.get("domain", ""),
    }

    try:
        import asyncio

        store = _get_knowledge_store()
        exp_id = asyncio.get_event_loop().run_until_complete(
            store.save_experiment(experiment)
        )

        return {
            "success": True,
            "experiment_id": exp_id,
            "message": f"Experiment recorded: {exp_id}",
        }
    except Exception as e:
        return {"error": f"Recording failed: {e}"}


@register_tool("c4_pattern_suggest")
def handle_pattern_suggest(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get pattern-based suggestions from experiment knowledge.

    Args (via arguments):
        domain: Domain to get patterns for (optional)
        include_best_practices: Include best practice recommendations (default: true)

    Returns:
        Patterns and recommendations.
    """
    domain = arguments.get("domain")
    include_best = arguments.get("include_best_practices", True)

    try:
        import asyncio

        store = _get_knowledge_store()
        aggregator = _get_aggregator()

        # Get patterns
        patterns = asyncio.get_event_loop().run_until_complete(
            store.get_patterns(domain=domain)
        )

        result: dict[str, Any] = {
            "pattern_count": len(patterns),
            "patterns": patterns[:10],
        }

        # Add best practices if requested
        if include_best:
            experiments = asyncio.get_event_loop().run_until_complete(
                store.list_experiments(domain=domain, limit=100)
            )
            stats = aggregator.compute_success_rate(experiments, domain=domain)
            recommendations = aggregator.get_best_practices(experiments)

            result["success_rate"] = stats
            result["recommendations"] = recommendations[:5]

        return result
    except Exception as e:
        return {"error": f"Pattern suggestion failed: {e}"}
