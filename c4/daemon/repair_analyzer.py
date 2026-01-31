"""C4 Repair Analyzer - AI-based failure analysis and repair suggestions.

This module provides intelligent failure analysis and repair suggestions
for blocked tasks in the self-healing queue.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import subprocess
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..models.queue import RepairQueueItem

logger = logging.getLogger(__name__)


class FailureCategory(str, Enum):
    """Categories of failures for pattern matching."""

    SYNTAX_ERROR = "syntax_error"
    TYPE_ERROR = "type_error"
    IMPORT_ERROR = "import_error"
    TEST_FAILURE = "test_failure"
    LINT_ERROR = "lint_error"
    TIMEOUT = "timeout"
    PERMISSION = "permission"
    RESOURCE = "resource"
    UNKNOWN = "unknown"


@dataclass
class FailurePattern:
    """A known failure pattern with solution templates."""

    pattern: str
    category: FailureCategory
    solution_template: str
    priority: int = 50  # Higher = more important

    def matches(self, error_text: str) -> bool:
        """Check if this pattern matches the error."""
        return bool(re.search(self.pattern, error_text, re.IGNORECASE | re.MULTILINE))


@dataclass
class FailureAnalysis:
    """Result of analyzing a failure."""

    category: FailureCategory
    root_cause: str
    affected_files: list[str]
    confidence: float  # 0.0 to 1.0
    suggested_fixes: list[str]
    related_patterns: list[FailurePattern] = field(default_factory=list)
    timestamp: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "category": self.category.value,
            "root_cause": self.root_cause,
            "affected_files": self.affected_files,
            "confidence": self.confidence,
            "suggested_fixes": self.suggested_fixes,
            "timestamp": self.timestamp.isoformat(),
        }


# Common failure patterns and their solutions
KNOWN_PATTERNS = [
    FailurePattern(
        pattern=r"SyntaxError:\s*(.+)",
        category=FailureCategory.SYNTAX_ERROR,
        solution_template="Fix syntax error: {match}. Check for missing colons, parentheses, or quotes.",
        priority=90,
    ),
    FailurePattern(
        pattern=r"IndentationError:\s*(.+)",
        category=FailureCategory.SYNTAX_ERROR,
        solution_template="Fix indentation: {match}. Ensure consistent use of spaces (4) or tabs.",
        priority=85,
    ),
    FailurePattern(
        pattern=r"ImportError:\s*(.+)",
        category=FailureCategory.IMPORT_ERROR,
        solution_template="Fix import error: {match}. Check the import path and module name.",
        priority=80,
    ),
    FailurePattern(
        pattern=r"TypeError:\s*(.+)",
        category=FailureCategory.TYPE_ERROR,
        solution_template="Fix type error: {match}. Check argument types and return values.",
        priority=80,
    ),
    FailurePattern(
        pattern=r"AttributeError:\s*'(\w+)' object has no attribute '(\w+)'",
        category=FailureCategory.TYPE_ERROR,
        solution_template="Fix missing attribute '{match}' on object. Check object type or add attribute.",
        priority=75,
    ),
    FailurePattern(
        pattern=r"ImportError:\s*cannot import name '(\w+)'",
        category=FailureCategory.IMPORT_ERROR,
        solution_template="Fix import error for '{match}'. Check if the name exists in the module.",
        priority=85,
    ),
    FailurePattern(
        pattern=r"ModuleNotFoundError:\s*No module named '(\w+)'",
        category=FailureCategory.IMPORT_ERROR,
        solution_template="Install missing module: uv add {match}",
        priority=90,
    ),
    FailurePattern(
        pattern=r"FAILED\s+(\S+)::(\S+)",
        category=FailureCategory.TEST_FAILURE,
        solution_template="Fix failing test: {match}. Review test assertions and expected values.",
        priority=70,
    ),
    FailurePattern(
        pattern=r"AssertionError:\s*(.+)",
        category=FailureCategory.TEST_FAILURE,
        solution_template="Fix assertion: {match}. Check expected vs actual values.",
        priority=70,
    ),
    FailurePattern(
        pattern=r"E\d{3}\s+(.+)",  # Ruff/flake8 error codes
        category=FailureCategory.LINT_ERROR,
        solution_template="Fix lint error: {match}. Run 'uv run ruff check --fix' for auto-fix.",
        priority=60,
    ),
    FailurePattern(
        pattern=r"Timeout after (\d+)s",
        category=FailureCategory.TIMEOUT,
        solution_template="Operation timed out after {match}s. Check for infinite loops or slow operations.",
        priority=75,
    ),
    FailurePattern(
        pattern=r"PermissionError:\s*(.+)",
        category=FailureCategory.PERMISSION,
        solution_template="Fix permission error: {match}. Check file/directory permissions.",
        priority=80,
    ),
    FailurePattern(
        pattern=r"MemoryError|Out of memory",
        category=FailureCategory.RESOURCE,
        solution_template="Memory error. Reduce data size or optimize memory usage.",
        priority=85,
    ),
]


class FailureAnalyzer:
    """Analyzes failures to determine root cause and suggest fixes.

    Uses pattern matching for known errors and optionally calls AI
    for complex or unknown failures.
    """

    def __init__(
        self,
        project_root: Path,
        use_ai_fallback: bool = True,
        ai_timeout: int = 60,
    ):
        """Initialize analyzer.

        Args:
            project_root: Project root directory
            use_ai_fallback: Use AI for unknown failures
            ai_timeout: Timeout for AI calls in seconds
        """
        self.root = project_root
        self.use_ai_fallback = use_ai_fallback
        self.ai_timeout = ai_timeout
        self._cache: dict[str, FailureAnalysis] = {}

    def _get_cache_key(self, error_text: str) -> str:
        """Generate cache key from error text."""
        return hashlib.md5(error_text.encode()).hexdigest()[:16]

    def analyze(self, item: "RepairQueueItem") -> FailureAnalysis:
        """Analyze a repair queue item to determine root cause.

        Args:
            item: Repair queue item to analyze

        Returns:
            FailureAnalysis with root cause and suggestions
        """
        error_text = f"{item.failure_signature}\n{item.last_error}"
        cache_key = self._get_cache_key(error_text)

        # Check cache
        if cache_key in self._cache:
            logger.debug(f"Cache hit for failure analysis: {cache_key}")
            return self._cache[cache_key]

        # Pattern matching
        matched_patterns = []
        for pattern in KNOWN_PATTERNS:
            if pattern.matches(error_text):
                matched_patterns.append(pattern)

        # Extract affected files
        affected_files = self._extract_affected_files(error_text)

        if matched_patterns:
            # Sort by priority and use the highest
            matched_patterns.sort(key=lambda p: p.priority, reverse=True)
            best_match = matched_patterns[0]

            # Generate fix suggestions
            suggestions = self._generate_suggestions(error_text, matched_patterns)

            analysis = FailureAnalysis(
                category=best_match.category,
                root_cause=self._extract_root_cause(error_text, best_match),
                affected_files=affected_files,
                confidence=min(0.9, 0.5 + len(matched_patterns) * 0.1),
                suggested_fixes=suggestions,
                related_patterns=matched_patterns[:3],
            )
        elif self.use_ai_fallback:
            # Use AI for unknown failures
            analysis = self._analyze_with_ai(item, affected_files)
        else:
            # Generic unknown failure
            analysis = FailureAnalysis(
                category=FailureCategory.UNKNOWN,
                root_cause=item.failure_signature[:200],
                affected_files=affected_files,
                confidence=0.3,
                suggested_fixes=[
                    f"Review error: {item.failure_signature[:100]}",
                    "Check recent changes to affected files",
                    "Run validations locally to reproduce",
                ],
            )

        # Cache result
        self._cache[cache_key] = analysis
        return analysis

    def _extract_affected_files(self, error_text: str) -> list[str]:
        """Extract file paths from error text."""
        # Common patterns for file paths
        patterns = [
            r'File "([^"]+\.py)"',  # Python traceback
            r"(\S+\.py):\d+",  # file.py:123
            r"tests/\S+\.py",  # test files
            r"src/\S+\.py",  # source files
            r"c4/\S+\.py",  # c4 files
        ]

        files = set()
        for pattern in patterns:
            matches = re.findall(pattern, error_text)
            files.update(matches)

        # Filter to existing files
        existing = []
        for f in files:
            path = Path(f)
            if path.exists() or (self.root / f).exists():
                existing.append(str(f))

        return existing[:10]  # Limit to 10 files

    def _extract_root_cause(self, error_text: str, pattern: FailurePattern) -> str:
        """Extract the specific root cause from error text."""
        match = re.search(pattern.pattern, error_text, re.IGNORECASE | re.MULTILINE)
        if match:
            # Use the captured group or full match
            if match.groups():
                return f"{pattern.category.value}: {match.group(1)}"
            return f"{pattern.category.value}: {match.group(0)}"
        return pattern.category.value

    def _generate_suggestions(
        self, error_text: str, patterns: list[FailurePattern]
    ) -> list[str]:
        """Generate fix suggestions based on matched patterns."""
        suggestions = []

        for pattern in patterns[:3]:  # Top 3 patterns
            match = re.search(pattern.pattern, error_text, re.IGNORECASE)
            if match:
                # Format the template with the match
                match_text = match.group(1) if match.groups() else match.group(0)
                suggestion = pattern.solution_template.format(match=match_text)
                if suggestion not in suggestions:
                    suggestions.append(suggestion)

        # Add generic suggestions if few specific ones
        if len(suggestions) < 2:
            suggestions.extend(
                [
                    "Review the full error traceback for context",
                    "Check if dependencies are correctly installed",
                ]
            )

        return suggestions[:5]

    def _analyze_with_ai(
        self, item: "RepairQueueItem", affected_files: list[str]
    ) -> FailureAnalysis:
        """Use AI to analyze complex failures."""
        prompt = f"""Analyze this failure and provide a root cause analysis.

Task ID: {item.task_id}
Failure Signature: {item.failure_signature}
Last Error: {item.last_error}
Attempts: {item.attempts}
Affected Files: {', '.join(affected_files)}

Respond in this exact JSON format:
{{
    "category": "syntax_error|type_error|import_error|test_failure|lint_error|timeout|permission|resource|unknown",
    "root_cause": "Brief description of the root cause",
    "suggested_fixes": ["Fix 1", "Fix 2", "Fix 3"]
}}"""

        try:
            result = subprocess.run(
                ["claude", "-p", prompt, "--output-format", "json"],
                capture_output=True,
                text=True,
                timeout=self.ai_timeout,
                cwd=self.root,
            )

            if result.returncode == 0:
                try:
                    data = json.loads(result.stdout)
                    return FailureAnalysis(
                        category=FailureCategory(data.get("category", "unknown")),
                        root_cause=data.get("root_cause", item.failure_signature),
                        affected_files=affected_files,
                        confidence=0.7,
                        suggested_fixes=data.get("suggested_fixes", []),
                    )
                except (json.JSONDecodeError, ValueError):
                    logger.warning("Failed to parse AI response as JSON")

        except subprocess.TimeoutExpired:
            logger.warning("AI analysis timed out")
        except FileNotFoundError:
            logger.debug("Claude CLI not available")
        except Exception as e:
            logger.error(f"AI analysis failed: {e}")

        # Fallback
        return FailureAnalysis(
            category=FailureCategory.UNKNOWN,
            root_cause=item.failure_signature[:200],
            affected_files=affected_files,
            confidence=0.3,
            suggested_fixes=[
                "Review the error message carefully",
                "Check recent code changes",
                "Run tests locally to reproduce",
            ],
        )


class RepairSuggestionGenerator:
    """Generates actionable repair suggestions from failure analysis.

    Creates detailed DoD (Definition of Done) for repair tasks.
    """

    def __init__(self, project_root: Path):
        self.root = project_root

    def generate_dod(self, analysis: FailureAnalysis, task_id: str) -> str:
        """Generate Definition of Done for a repair task.

        Args:
            analysis: Failure analysis result
            task_id: Original task ID

        Returns:
            Formatted DoD string
        """
        lines = [
            f"# Repair Task for {task_id}",
            "",
            "## Root Cause",
            f"**Category**: {analysis.category.value}",
            f"**Issue**: {analysis.root_cause}",
            f"**Confidence**: {analysis.confidence:.0%}",
            "",
        ]

        if analysis.affected_files:
            lines.extend(
                [
                    "## Affected Files",
                    *[f"- `{f}`" for f in analysis.affected_files],
                    "",
                ]
            )

        lines.extend(
            [
                "## Definition of Done",
                "",
            ]
        )

        # Generate checklist from suggestions
        for i, fix in enumerate(analysis.suggested_fixes, 1):
            lines.append(f"- [ ] {fix}")

        # Add standard DoD items
        lines.extend(
            [
                "",
                "## Verification",
                "- [ ] All validations pass (lint, unit)",
                "- [ ] No new errors introduced",
                "- [ ] Original issue is resolved",
            ]
        )

        return "\n".join(lines)

    def generate_repair_prompt(
        self, analysis: FailureAnalysis, task_id: str, context: str | None = None
    ) -> str:
        """Generate a prompt for AI-assisted repair.

        Args:
            analysis: Failure analysis result
            task_id: Original task ID
            context: Optional additional context

        Returns:
            Prompt string for AI
        """
        prompt_parts = [
            f"# Repair Request for Task {task_id}",
            "",
            "## Failure Analysis",
            f"- **Category**: {analysis.category.value}",
            f"- **Root Cause**: {analysis.root_cause}",
            f"- **Confidence**: {analysis.confidence:.0%}",
            "",
        ]

        if analysis.affected_files:
            prompt_parts.extend(
                [
                    "## Affected Files",
                    *[f"- {f}" for f in analysis.affected_files],
                    "",
                ]
            )

        if analysis.suggested_fixes:
            prompt_parts.extend(
                [
                    "## Suggested Fixes",
                    *[f"{i}. {fix}" for i, fix in enumerate(analysis.suggested_fixes, 1)],
                    "",
                ]
            )

        if context:
            prompt_parts.extend(
                [
                    "## Additional Context",
                    context,
                    "",
                ]
            )

        prompt_parts.extend(
            [
                "## Instructions",
                "1. Analyze the root cause and affected files",
                "2. Apply the suggested fixes or implement your own solution",
                "3. Ensure all validations pass",
                "4. Document any changes made",
            ]
        )

        return "\n".join(prompt_parts)


@dataclass
class RepairMetrics:
    """Metrics for tracking repair queue performance."""

    total_repairs: int = 0
    successful_repairs: int = 0
    failed_repairs: int = 0
    avg_attempts_to_fix: float = 0.0
    repairs_by_category: dict[str, int] = field(default_factory=dict)
    repairs_by_worker: dict[str, int] = field(default_factory=dict)
    last_updated: datetime = field(default_factory=datetime.now)

    def record_repair(
        self,
        success: bool,
        category: FailureCategory,
        worker_id: str,
        attempts: int,
    ) -> None:
        """Record a repair attempt.

        Args:
            success: Whether the repair succeeded
            category: Failure category
            worker_id: Worker who performed the repair
            attempts: Number of attempts taken
        """
        self.total_repairs += 1

        if success:
            self.successful_repairs += 1
        else:
            self.failed_repairs += 1

        # Update average attempts
        if self.successful_repairs > 0:
            old_total = self.avg_attempts_to_fix * (self.successful_repairs - (1 if success else 0))
            self.avg_attempts_to_fix = (old_total + (attempts if success else 0)) / self.successful_repairs

        # Update category counts
        cat_key = category.value
        self.repairs_by_category[cat_key] = self.repairs_by_category.get(cat_key, 0) + 1

        # Update worker counts
        self.repairs_by_worker[worker_id] = self.repairs_by_worker.get(worker_id, 0) + 1

        self.last_updated = datetime.now()

    @property
    def success_rate(self) -> float:
        """Calculate repair success rate."""
        if self.total_repairs == 0:
            return 0.0
        return self.successful_repairs / self.total_repairs

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "total_repairs": self.total_repairs,
            "successful_repairs": self.successful_repairs,
            "failed_repairs": self.failed_repairs,
            "success_rate": self.success_rate,
            "avg_attempts_to_fix": self.avg_attempts_to_fix,
            "repairs_by_category": self.repairs_by_category,
            "repairs_by_worker": self.repairs_by_worker,
            "last_updated": self.last_updated.isoformat(),
        }

    def to_prometheus_format(self) -> list[str]:
        """Export metrics in Prometheus format."""
        lines = [
            f"c4_repair_total {self.total_repairs}",
            f"c4_repair_success {self.successful_repairs}",
            f"c4_repair_failed {self.failed_repairs}",
            f'c4_repair_success_rate {self.success_rate:.4f}',
            f"c4_repair_avg_attempts {self.avg_attempts_to_fix:.2f}",
        ]

        for cat, count in self.repairs_by_category.items():
            lines.append(f'c4_repair_by_category{{category="{cat}"}} {count}')

        for worker, count in self.repairs_by_worker.items():
            lines.append(f'c4_repair_by_worker{{worker="{worker}"}} {count}')

        return lines
