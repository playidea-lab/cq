"""Pattern Miner - Mine patterns from experiment knowledge.

Discovers successful config patterns, extracts failure lessons,
and updates hypothesis confidence based on experiment outcomes.
"""

from __future__ import annotations

import logging
from collections import Counter, defaultdict
from typing import Any

from .models import HypothesisStatus, Pattern

logger = logging.getLogger(__name__)


class PatternMiner:
    """Mine patterns from experiment results."""

    def mine_success_patterns(
        self,
        experiments: list[dict[str, Any]],
        min_support: int = 2,
    ) -> list[Pattern]:
        """Find config patterns common among successful experiments.

        Args:
            experiments: List of experiment dicts.
            min_support: Minimum number of experiments sharing a pattern.

        Returns:
            List of Pattern objects discovered.
        """
        successful = [
            e
            for e in experiments
            if e.get("result", {}).get("success", False)
        ]

        if len(successful) < min_support:
            return []

        # Count config key-value pairs across successful experiments
        kv_counts: Counter = Counter()
        kv_experiments: dict[str, list[str]] = defaultdict(list)

        for exp in successful:
            config = exp.get("config", {})
            exp_id = exp.get("id", "")
            for key, value in config.items():
                kv_key = f"{key}={value}"
                kv_counts[kv_key] += 1
                kv_experiments[kv_key].append(exp_id)

        patterns = []
        total = len(successful)

        for kv_str, count in kv_counts.items():
            if count >= min_support:
                key, value = kv_str.split("=", 1)
                confidence = count / total if total > 0 else 0.0

                pattern = Pattern(
                    name=f"success_config:{key}",
                    description=f"Config '{key}={value}' used in {count}/{total} successful experiments",
                    domain=self._infer_domain(experiments, kv_experiments[kv_str]),
                    confidence=round(confidence, 3),
                    evidence_count=count,
                    evidence_ids=kv_experiments[kv_str],
                    config_pattern={key: value},
                )
                patterns.append(pattern)

        # Sort by confidence descending
        patterns.sort(key=lambda p: p.confidence, reverse=True)
        return patterns

    def extract_failure_lessons(
        self,
        experiments: list[dict[str, Any]],
        min_occurrences: int = 2,
    ) -> list[dict[str, Any]]:
        """Extract recurring lessons from failed experiments.

        Args:
            experiments: List of experiment dicts.
            min_occurrences: Minimum times a lesson must appear.

        Returns:
            List of lesson dicts with content, count, experiment_ids.
        """
        failed = [
            e
            for e in experiments
            if not e.get("result", {}).get("success", True)
        ]

        lesson_counts: Counter = Counter()
        lesson_experiments: dict[str, list[str]] = defaultdict(list)

        for exp in failed:
            exp_id = exp.get("id", "")
            for lesson in exp.get("lessons_learned", []):
                lesson_counts[lesson] += 1
                lesson_experiments[lesson].append(exp_id)

        results = []
        for lesson, count in lesson_counts.most_common():
            if count >= min_occurrences:
                results.append({
                    "content": lesson,
                    "count": count,
                    "experiment_ids": lesson_experiments[lesson],
                })

        return results

    def update_hypothesis_confidence(
        self,
        experiments: list[dict[str, Any]],
        hypothesis: str,
    ) -> dict[str, Any]:
        """Compute confidence for a hypothesis based on experiment outcomes.

        Args:
            experiments: Experiments testing this hypothesis.
            hypothesis: The hypothesis text to evaluate.

        Returns:
            Dict with supported, refuted, inconclusive counts,
            confidence score, and suggested status.
        """
        relevant = [
            e for e in experiments if e.get("hypothesis", "") == hypothesis
        ]

        if not relevant:
            return {
                "hypothesis": hypothesis,
                "total": 0,
                "supported": 0,
                "refuted": 0,
                "inconclusive": 0,
                "confidence": 0.0,
                "suggested_status": HypothesisStatus.PROPOSED.value,
            }

        status_counts = Counter(
            e.get("hypothesis_status", "proposed") for e in relevant
        )

        supported = status_counts.get("supported", 0)
        refuted = status_counts.get("refuted", 0)
        inconclusive = status_counts.get("inconclusive", 0)
        total = len(relevant)

        # Confidence: ratio of supported vs total decisive experiments
        decisive = supported + refuted
        if decisive == 0:
            confidence = 0.0
            suggested = HypothesisStatus.INCONCLUSIVE.value
        elif supported > refuted:
            confidence = supported / decisive
            suggested = (
                HypothesisStatus.SUPPORTED.value
                if confidence >= 0.7
                else HypothesisStatus.TESTING.value
            )
        else:
            confidence = refuted / decisive
            suggested = (
                HypothesisStatus.REFUTED.value
                if confidence >= 0.7
                else HypothesisStatus.TESTING.value
            )

        return {
            "hypothesis": hypothesis,
            "total": total,
            "supported": supported,
            "refuted": refuted,
            "inconclusive": inconclusive,
            "confidence": round(confidence, 3),
            "suggested_status": suggested,
        }

    def _infer_domain(
        self,
        experiments: list[dict[str, Any]],
        exp_ids: list[str],
    ) -> str:
        """Infer domain from experiments matching given IDs."""
        domains: Counter = Counter()
        for exp in experiments:
            if exp.get("id") in exp_ids:
                domain = exp.get("domain", "")
                if domain:
                    domains[domain] += 1

        if domains:
            return domains.most_common(1)[0][0]
        return ""
