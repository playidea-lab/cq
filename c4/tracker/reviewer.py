"""LLM experiment reviewer - optional post-experiment analysis.

Generates a structured review of experiment results using an LLM.
Controlled by config.tracker.llm_review flag.
"""

from __future__ import annotations

import logging
from typing import Any

logger = logging.getLogger(__name__)


def review_experiment(
    code_features: dict[str, Any],
    metrics: dict[str, float],
    data_profile: dict[str, Any],
    git_context: dict[str, Any],
    model: str = "sonnet",
) -> str | None:
    """Generate LLM review of experiment results.

    This is an optional feature controlled by config.tracker.llm_review.
    Returns a structured text review or None if LLM is unavailable.

    Args:
        code_features: AST analysis results
        metrics: Final captured metrics
        data_profile: Input data profile
        git_context: Git state info
        model: LLM model to use

    Returns:
        Review text or None
    """
    try:
        prompt = _build_review_prompt(code_features, metrics, data_profile, git_context)
        # LLM call would go here - for now return the structured prompt
        # In production, this would call Anthropic API or local LLM
        logger.debug("Experiment review prompt built (%d chars)", len(prompt))
        return None  # LLM integration deferred to Phase 4
    except Exception as e:
        logger.warning("Failed to generate experiment review: %s", e)
        return None


def _build_review_prompt(
    code_features: dict[str, Any],
    metrics: dict[str, float],
    data_profile: dict[str, Any],
    git_context: dict[str, Any],
) -> str:
    """Build structured prompt for experiment review."""
    sections = []

    if code_features:
        imports = code_features.get("imports", [])
        algorithm = code_features.get("algorithm", "unknown")
        sections.append(f"## Code Analysis\n- Algorithm: {algorithm}\n- Imports: {', '.join(imports[:10])}")

    if metrics:
        metric_lines = [f"- {k}: {v}" for k, v in sorted(metrics.items())]
        sections.append("## Metrics\n" + "\n".join(metric_lines))

    if data_profile:
        profile_lines = []
        for name, info in data_profile.items():
            shape = info.get("shape", "unknown")
            dtype = info.get("dtype", "unknown")
            profile_lines.append(f"- {name}: shape={shape}, dtype={dtype}")
        sections.append("## Data Profile\n" + "\n".join(profile_lines))

    if git_context:
        commit = git_context.get("commit_sha", "unknown")[:8]
        branch = git_context.get("branch", "unknown")
        dirty = git_context.get("dirty", False)
        sections.append(f"## Git Context\n- Branch: {branch}\n- Commit: {commit}\n- Dirty: {dirty}")

    return (
        "Review this ML experiment and provide insights:\n\n"
        + "\n\n".join(sections)
        + "\n\nProvide:\n1. Assessment of approach\n2. Suggestions for improvement\n3. Potential issues"
    )
