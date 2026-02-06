"""Knowledge Aggregator - Aggregate experiment results for insights.

Computes success rates, extracts common patterns from successful
experiments, and generates best-practice recommendations.
"""

from __future__ import annotations

import logging
from collections import Counter
from typing import Any

logger = logging.getLogger(__name__)


class KnowledgeAggregator:
    """Aggregate experiment knowledge into actionable insights."""

    def compute_success_rate(
        self, experiments: list[dict[str, Any]], domain: str | None = None
    ) -> dict[str, Any]:
        """Compute success/failure statistics for experiments.

        Args:
            experiments: List of experiment dicts from KnowledgeStore.
            domain: Optional domain filter.

        Returns:
            Dict with total, success_count, failure_count, success_rate.
        """
        if domain:
            experiments = [e for e in experiments if e.get("domain") == domain]

        total = len(experiments)
        if total == 0:
            return {
                "total": 0,
                "success_count": 0,
                "failure_count": 0,
                "success_rate": 0.0,
            }

        success_count = sum(
            1
            for e in experiments
            if e.get("result", {}).get("success", False)
        )
        failure_count = total - success_count

        return {
            "total": total,
            "success_count": success_count,
            "failure_count": failure_count,
            "success_rate": success_count / total if total > 0 else 0.0,
        }

    def extract_common_configs(
        self,
        experiments: list[dict[str, Any]],
        success_only: bool = True,
    ) -> dict[str, list[tuple[str, int]]]:
        """Extract commonly used config keys and values.

        Args:
            experiments: List of experiment dicts.
            success_only: Only consider successful experiments.

        Returns:
            Dict mapping config key to list of (value, count) tuples,
            sorted by frequency.
        """
        if success_only:
            experiments = [
                e
                for e in experiments
                if e.get("result", {}).get("success", False)
            ]

        key_value_counts: dict[str, Counter] = {}
        for exp in experiments:
            config = exp.get("config", {})
            for key, value in config.items():
                if key not in key_value_counts:
                    key_value_counts[key] = Counter()
                key_value_counts[key][str(value)] += 1

        return {
            key: counter.most_common()
            for key, counter in key_value_counts.items()
        }

    def get_best_practices(
        self, experiments: list[dict[str, Any]]
    ) -> list[dict[str, Any]]:
        """Generate best-practice recommendations from experiments.

        Extracts lessons from successful experiments and
        warnings from failed ones.

        Args:
            experiments: List of experiment dicts.

        Returns:
            List of recommendation dicts with type, content, source_count.
        """
        recommendations: list[dict[str, Any]] = []

        # Collect lessons from successful experiments
        success_lessons: Counter = Counter()
        for exp in experiments:
            if exp.get("result", {}).get("success", False):
                for lesson in exp.get("lessons_learned", []):
                    success_lessons[lesson] += 1

        for lesson, count in success_lessons.most_common(10):
            recommendations.append({
                "type": "best_practice",
                "content": lesson,
                "source_count": count,
            })

        # Collect warnings from failed experiments
        failure_lessons: Counter = Counter()
        for exp in experiments:
            if not exp.get("result", {}).get("success", True):
                for lesson in exp.get("lessons_learned", []):
                    failure_lessons[lesson] += 1

        for lesson, count in failure_lessons.most_common(5):
            recommendations.append({
                "type": "warning",
                "content": lesson,
                "source_count": count,
            })

        return recommendations

    def generate_failure_report(
        self, experiments: list[dict[str, Any]]
    ) -> dict[str, Any]:
        """Generate a summary report of experiment failures.

        Args:
            experiments: List of experiment dicts.

        Returns:
            Dict with failure_count, common_errors, affected_domains.
        """
        failed = [
            e
            for e in experiments
            if not e.get("result", {}).get("success", True)
        ]

        error_counts: Counter = Counter()
        domain_counts: Counter = Counter()

        for exp in failed:
            result = exp.get("result", {})
            error_msg = result.get("error_message", "unknown")
            if error_msg:
                error_counts[error_msg] += 1
            domain = exp.get("domain", "unknown")
            if domain:
                domain_counts[domain] += 1

        return {
            "failure_count": len(failed),
            "common_errors": error_counts.most_common(10),
            "affected_domains": domain_counts.most_common(),
        }
