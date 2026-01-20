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
    from c4.supervisor.agent_graph.graph import AgentGraph
    from c4.supervisor.agent_graph.models import (
        ChainExtension,
        Condition,
        Override,
        Rules,
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
        return [ext for ext in self._chain_extensions if evaluate(ext.condition, task)]

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

    def apply_overrides(
        self,
        task: TaskLike,
        graph: AgentGraph | None = None,
    ) -> str | None:
        """Apply override rules to determine the primary agent.

        Finds the highest-priority matching override and applies its action.
        Supports: set_primary, require_agent.

        Args:
            task: The task to match against
            graph: Optional AgentGraph to validate agent existence

        Returns:
            Agent ID to use as primary, or None if no override matches
        """
        override = self.find_matching_override(task)
        if override is None:
            return None

        action = override.action

        # set_primary takes highest priority
        if action.set_primary:
            if graph is None or graph.get_node(action.set_primary):
                return action.set_primary

        # require_agent also sets the primary
        if action.require_agent:
            if graph is None or graph.get_node(action.require_agent):
                return action.require_agent

        return None

    def extend_chain(
        self,
        chain: list[str],
        task: TaskLike,
    ) -> list[str]:
        """Extend an agent chain based on matching chain extension rules.

        Applies all matching chain extensions to add agents to the chain.
        Supports positions: first, after_primary, before_last, last.
        Also supports ensure_in_chain for guaranteeing agent presence.

        Args:
            chain: The current agent chain (list of agent IDs)
            task: The task to match against

        Returns:
            Extended agent chain with additional agents
        """
        result = list(chain)  # Make a copy
        extensions = self.find_matching_chain_extensions(task)

        for ext in extensions:
            action = ext.action

            # Handle ensure_in_chain first
            if action.ensure_in_chain:
                for agent_id in action.ensure_in_chain:
                    if agent_id not in result:
                        # Add at default position (before_last)
                        if len(result) >= 2:
                            result.insert(-1, agent_id)
                        else:
                            result.append(agent_id)

            # Handle add_to_chain with position
            if action.add_to_chain and action.add_to_chain not in result:
                agent_id = action.add_to_chain
                position = action.position or "before_last"

                if position == "first":
                    result.insert(0, agent_id)
                elif position == "after_primary":
                    if len(result) >= 1:
                        result.insert(1, agent_id)
                    else:
                        result.append(agent_id)
                elif position == "before_last":
                    if len(result) >= 1:
                        result.insert(-1, agent_id)
                    else:
                        result.append(agent_id)
                elif position == "last":
                    result.append(agent_id)

        return result


def apply_overrides(
    rules: Rules | RuleEngine,
    task: TaskLike,
    graph: AgentGraph | None = None,
) -> str | None:
    """Apply override rules to determine the primary agent.

    This is a module-level convenience function that works with
    both Rules model and RuleEngine instance.

    Args:
        rules: Either a Rules model or a RuleEngine instance
        task: The task to match against
        graph: Optional AgentGraph to validate agent existence

    Returns:
        Agent ID to use as primary, or None if no override matches

    Example:
        >>> from c4.supervisor.agent_graph.models import Rules, Override, OverrideAction, Condition
        >>> rules = Rules(overrides=[
        ...     Override(
        ...         name="debug",
        ...         priority=90,
        ...         condition=Condition(has_keyword=["debug"]),
        ...         action=OverrideAction(set_primary="debugger"),
        ...         reason="Use debugger for debug tasks"
        ...     )
        ... ])
        >>> task = Task(title="Debug login issue")
        >>> apply_overrides(rules, task)
        'debugger'
    """
    if isinstance(rules, RuleEngine):
        return rules.apply_overrides(task, graph)

    # Rules model - create a temporary engine
    engine = RuleEngine()
    if rules.overrides:
        for override in rules.overrides:
            engine.add_override(override)

    return engine.apply_overrides(task, graph)


def extend_chain(
    rules: Rules | RuleEngine,
    chain: list[str],
    task: TaskLike,
) -> list[str]:
    """Extend an agent chain based on chain extension rules.

    This is a module-level convenience function that works with
    both Rules model and RuleEngine instance.

    Args:
        rules: Either a Rules model or a RuleEngine instance
        chain: The current agent chain (list of agent IDs)
        task: The task to match against

    Returns:
        Extended agent chain

    Example:
        >>> from c4.supervisor.agent_graph.models import (
        ...     Rules, ChainExtension, ChainExtensionAction, Condition
        ... )
        >>> action = ChainExtensionAction(
        ...     add_to_chain="security-auditor", position="before_last"
        ... )
        >>> rules = Rules(chain_extensions=[
        ...     ChainExtension(
        ...         name="security",
        ...         condition=Condition(has_keyword=["auth", "security"]),
        ...         action=action
        ...     )
        ... ])
        >>> chain = ["backend-dev", "code-reviewer"]
        >>> task = Task(title="Add authentication")
        >>> extend_chain(rules, chain, task)
        ['backend-dev', 'security-auditor', 'code-reviewer']
    """
    if isinstance(rules, RuleEngine):
        return rules.extend_chain(chain, task)

    # Rules model - create a temporary engine
    engine = RuleEngine()
    if rules.chain_extensions:
        for ext in rules.chain_extensions:
            engine.add_chain_extension(ext)

    return engine.extend_chain(chain, task)
