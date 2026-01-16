"""RuleEngine for evaluating conditions against tasks.

This module provides the evaluation logic for Condition models defined in models.py.
It supports:
- task_type: Match task type(s)
- domain: Match domain(s)
- has_keyword: Match if any keyword present in task title/description
- file_pattern: Match file patterns (glob) against task scope
- any: OR - match if any sub-condition matches
- all: AND - match if all sub-conditions match
- not_: NOT - negate sub-condition
"""

from __future__ import annotations

import fnmatch
from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.models import (
        ChainExtension,
        Condition,
        Override,
    )


class TaskLike(Protocol):
    """Protocol for objects that can be evaluated against conditions.

    Any object with these attributes can be used as a task.
    """

    @property
    def task_type(self) -> str | None:
        """The type of the task (e.g., 'feature', 'bugfix', 'refactor')."""
        ...

    @property
    def domain(self) -> str | None:
        """The domain of the task (e.g., 'web-frontend', 'web-backend')."""
        ...

    @property
    def title(self) -> str:
        """The title of the task."""
        ...

    @property
    def description(self) -> str:
        """The description of the task."""
        ...

    @property
    def scope(self) -> str | None:
        """The file/directory scope of the task."""
        ...


@dataclass
class Task:
    """Simple task data class for evaluation.

    This is a concrete implementation of TaskLike for testing and simple usage.
    """

    title: str
    description: str = ""
    task_type: str | None = None
    domain: str | None = None
    scope: str | None = None

    def __post_init__(self) -> None:
        """Ensure title is not empty."""
        if not self.title:
            raise ValueError("Task title cannot be empty")


def evaluate(condition: Condition, task: TaskLike) -> bool:
    """Evaluate a condition against a task.

    This is the main entry point for condition evaluation.
    It handles all condition types recursively.

    Args:
        condition: The Condition to evaluate
        task: The task to evaluate against

    Returns:
        True if the condition matches, False otherwise

    Examples:
        >>> from c4.supervisor.agent_graph.models import Condition
        >>> cond = Condition(task_type="feature")
        >>> task = Task(title="Add login", task_type="feature")
        >>> evaluate(cond, task)
        True
    """
    # Handle logical operators first (they take precedence)
    if condition.not_ is not None:
        return _evaluate_not(condition.not_, task)

    if condition.all is not None:
        return _evaluate_all(condition.all, task)

    if condition.any is not None:
        return _evaluate_any(condition.any, task)

    # Evaluate atomic conditions - all specified conditions must match (implicit AND)
    results: list[bool] = []

    if condition.task_type is not None:
        results.append(_evaluate_task_type(condition.task_type, task))

    if condition.domain is not None:
        results.append(_evaluate_domain(condition.domain, task))

    if condition.has_keyword is not None:
        results.append(_evaluate_has_keyword(condition.has_keyword, task))

    if condition.file_pattern is not None:
        results.append(_evaluate_file_pattern(condition.file_pattern, task))

    # If no conditions specified, return True (vacuously true)
    if not results:
        return True

    # All specified conditions must match
    return all(results)


def _evaluate_not(condition: Condition, task: TaskLike) -> bool:
    """Evaluate NOT condition.

    Args:
        condition: The condition to negate
        task: The task to evaluate against

    Returns:
        True if the condition does NOT match
    """
    return not evaluate(condition, task)


def _evaluate_all(conditions: list[Condition], task: TaskLike) -> bool:
    """Evaluate AND (all) condition.

    Args:
        conditions: List of conditions that must all match
        task: The task to evaluate against

    Returns:
        True if ALL conditions match
    """
    if not conditions:
        return True  # Vacuously true

    return all(evaluate(cond, task) for cond in conditions)


def _evaluate_any(conditions: list[Condition], task: TaskLike) -> bool:
    """Evaluate OR (any) condition.

    Args:
        conditions: List of conditions where at least one must match
        task: The task to evaluate against

    Returns:
        True if ANY condition matches
    """
    if not conditions:
        return True  # Vacuously true

    return any(evaluate(cond, task) for cond in conditions)


def _evaluate_task_type(
    task_type: str | list[str],
    task: TaskLike,
) -> bool:
    """Evaluate task_type condition.

    Args:
        task_type: Expected task type(s)
        task: The task to evaluate against

    Returns:
        True if task.task_type matches any of the expected types
    """
    if task.task_type is None:
        return False

    if isinstance(task_type, str):
        return task.task_type.lower() == task_type.lower()

    # List of task types - match if any matches
    return any(task.task_type.lower() == t.lower() for t in task_type)


def _evaluate_domain(
    domain: str | list[str],
    task: TaskLike,
) -> bool:
    """Evaluate domain condition.

    Args:
        domain: Expected domain(s)
        task: The task to evaluate against

    Returns:
        True if task.domain matches any of the expected domains
    """
    if task.domain is None:
        return False

    if isinstance(domain, str):
        return task.domain.lower() == domain.lower()

    # List of domains - match if any matches
    return any(task.domain.lower() == d.lower() for d in domain)


def _evaluate_has_keyword(
    keywords: list[str],
    task: TaskLike,
) -> bool:
    """Evaluate has_keyword condition.

    Searches for keywords in task title and description (case-insensitive).

    Args:
        keywords: List of keywords to search for
        task: The task to evaluate against

    Returns:
        True if ANY keyword is found in title or description
    """
    if not keywords:
        return True  # No keywords to match

    # Combine title and description for searching
    text = f"{task.title} {task.description}".lower()

    return any(keyword.lower() in text for keyword in keywords)


def _evaluate_file_pattern(
    patterns: list[str],
    task: TaskLike,
) -> bool:
    """Evaluate file_pattern condition using glob matching.

    Matches patterns against task.scope.

    Args:
        patterns: List of glob patterns to match
        task: The task to evaluate against

    Returns:
        True if task.scope matches ANY of the patterns
    """
    if not patterns:
        return True  # No patterns to match

    if task.scope is None:
        return False

    scope = task.scope

    for pattern in patterns:
        if fnmatch.fnmatch(scope, pattern):
            return True
        # Also try with ** for directory matching
        if pattern.endswith("/**") or pattern.endswith("/*"):
            base_pattern = pattern.rstrip("/*")
            if scope.startswith(base_pattern):
                return True

    return False


class RuleEngine:
    """Engine for evaluating rules against tasks.

    This class provides a higher-level API for rule evaluation,
    including support for overrides and chain extensions.

    Example:
        >>> from c4.supervisor.agent_graph.models import Condition, Override, OverrideAction
        >>> engine = RuleEngine()
        >>> override = Override(
        ...     name="debug-override",
        ...     priority=90,
        ...     condition=Condition(has_keyword=["debug", "fix bug"]),
        ...     action=OverrideAction(set_primary="debugger"),
        ...     reason="Debugging tasks should use debugger agent"
        ... )
        >>> engine.add_override(override)
        >>> task = Task(title="Fix bug in login", task_type="bugfix")
        >>> match = engine.find_matching_override(task)
        >>> match.action.set_primary
        'debugger'
    """

    def __init__(self) -> None:
        """Initialize an empty rule engine."""

        self._overrides: list[Override] = []
        self._chain_extensions: list[ChainExtension] = []

    def add_override(self, override: Override) -> None:
        """Add an override rule.

        Overrides are sorted by priority (highest first) when added.

        Args:
            override: The override rule to add
        """
        self._overrides.append(override)
        # Keep sorted by priority (highest first)
        self._overrides.sort(key=lambda x: x.priority, reverse=True)

    def add_chain_extension(self, extension: ChainExtension) -> None:
        """Add a chain extension rule.

        Args:
            extension: The chain extension rule to add
        """
        self._chain_extensions.append(extension)

    def find_matching_override(self, task: TaskLike) -> Override | None:
        """Find the highest-priority matching override for a task.

        Args:
            task: The task to match against

        Returns:
            The highest-priority matching override, or None if none match
        """
        for override in self._overrides:
            if evaluate(override.condition, task):
                return override
        return None

    def find_matching_chain_extensions(
        self,
        task: TaskLike,
    ) -> list[ChainExtension]:
        """Find all matching chain extensions for a task.

        Args:
            task: The task to match against

        Returns:
            List of all matching chain extensions
        """
        return [
            ext for ext in self._chain_extensions if evaluate(ext.condition, task)
        ]

    def evaluate_condition(self, condition: Condition, task: TaskLike) -> bool:
        """Evaluate a condition against a task.

        This is a convenience wrapper around the module-level evaluate function.

        Args:
            condition: The Condition to evaluate
            task: The task to evaluate against

        Returns:
            True if the condition matches, False otherwise
        """
        return evaluate(condition, task)
