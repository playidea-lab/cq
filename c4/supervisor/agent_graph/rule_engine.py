"""RuleEngine - Evaluate routing rules for agent selection.

This module provides a RuleEngine that evaluates conditions and finds matching
rules for routing decisions. It supports:
- Override rules (set_primary, add_to_chain)
- Chain extensions (add agents to chain)
- Complex conditions (all, any, not)

Usage:
    >>> engine = RuleEngine()
    >>> engine.add_rules(rule_definition)
    >>> context = RuleContext(task_type="debug", domain="web-backend")
    >>> override = engine.find_matching_override(context)
    >>> if override:
    ...     print(f"Override: {override.action.set_primary}")
"""

from __future__ import annotations

import fnmatch
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.models import (
        ChainExtension,
        Condition,
        Override,
        RuleDefinition,
    )


@dataclass
class RuleContext:
    """Context for rule evaluation.

    Attributes:
        task_type: The type of task (e.g., "debug", "feature")
        domain: The domain (e.g., "web-backend", "ml-dl")
        title: Task title for keyword matching
        description: Task description for keyword matching
        scope: File scope for pattern matching
    """

    task_type: str | None = None
    domain: str | None = None
    title: str = ""
    description: str = ""
    scope: str = ""

    def get_text(self) -> str:
        """Get combined text for keyword searching."""
        return f"{self.title} {self.description}".lower()


class RuleEngine:
    """Evaluates routing rules for agent selection.

    The engine maintains a collection of Override and ChainExtension rules,
    and provides methods to find matching rules for a given context.

    Rules are evaluated as follows:
    1. Overrides are sorted by priority (highest first)
    2. First matching override wins
    3. All matching chain extensions are collected

    Example:
        >>> engine = RuleEngine()
        >>> engine.add_override(override)
        >>> engine.add_chain_extension(extension)
        >>> context = RuleContext(task_type="debug")
        >>> override = engine.find_matching_override(context)
    """

    def __init__(self) -> None:
        """Initialize an empty RuleEngine."""
        self._overrides: list[Override] = []
        self._chain_extensions: list[ChainExtension] = []

    def add_rules(self, rule_def: RuleDefinition) -> None:
        """Add rules from a RuleDefinition.

        Args:
            rule_def: RuleDefinition containing overrides and chain_extensions
        """
        if rule_def.rules.overrides:
            for override in rule_def.rules.overrides:
                self.add_override(override)
        if rule_def.rules.chain_extensions:
            for extension in rule_def.rules.chain_extensions:
                self.add_chain_extension(extension)

    def add_override(self, override: Override) -> None:
        """Add an override rule.

        Args:
            override: Override rule to add
        """
        self._overrides.append(override)
        # Keep sorted by priority (highest first)
        self._overrides.sort(key=lambda o: o.priority, reverse=True)

    def add_chain_extension(self, extension: ChainExtension) -> None:
        """Add a chain extension rule.

        Args:
            extension: ChainExtension rule to add
        """
        self._chain_extensions.append(extension)

    def find_matching_override(self, context: RuleContext) -> Override | None:
        """Find the first matching override rule.

        Overrides are evaluated in priority order (highest first).

        Args:
            context: RuleContext with task/domain information

        Returns:
            First matching Override, or None if no match
        """
        for override in self._overrides:
            if self._evaluate_condition(override.condition, context):
                return override
        return None

    def find_matching_chain_extensions(self, context: RuleContext) -> list[ChainExtension]:
        """Find all matching chain extension rules.

        Args:
            context: RuleContext with task/domain information

        Returns:
            List of matching ChainExtension rules
        """
        matching = []
        for extension in self._chain_extensions:
            if self._evaluate_condition(extension.condition, context):
                matching.append(extension)
        return matching

    def _evaluate_condition(self, condition: Condition, context: RuleContext) -> bool:
        """Evaluate a condition against a context.

        Supports:
        - task_type: Match task type(s)
        - domain: Match domain(s)
        - has_keyword: Match if any keyword present in title/description
        - file_pattern: Match file patterns against scope
        - any: OR - match if any sub-condition matches
        - all: AND - match if all sub-conditions match
        - not_: NOT - negate sub-condition

        Args:
            condition: Condition to evaluate
            context: RuleContext to evaluate against

        Returns:
            True if condition matches
        """
        # Handle logical operators first
        if condition.all is not None:
            return all(self._evaluate_condition(sub, context) for sub in condition.all)

        if condition.any is not None:
            return any(self._evaluate_condition(sub, context) for sub in condition.any)

        if condition.not_ is not None:
            return not self._evaluate_condition(condition.not_, context)

        # Collect atomic condition results
        # If no atomic conditions are set, return False
        results: list[bool] = []

        # Task type matching
        if condition.task_type is not None:
            if context.task_type is None:
                results.append(False)
            else:
                types = (
                    condition.task_type
                    if isinstance(condition.task_type, list)
                    else [condition.task_type]
                )
                results.append(context.task_type.lower() in [t.lower() for t in types])

        # Domain matching
        if condition.domain is not None:
            if context.domain is None:
                results.append(False)
            else:
                domains = (
                    condition.domain if isinstance(condition.domain, list) else [condition.domain]
                )
                results.append(context.domain.lower() in [d.lower() for d in domains])

        # Keyword matching
        if condition.has_keyword is not None:
            text = context.get_text()
            has_any_keyword = any(kw.lower() in text for kw in condition.has_keyword)
            results.append(has_any_keyword)

        # File pattern matching
        if condition.file_pattern is not None:
            if not context.scope:
                results.append(False)
            else:
                matches_pattern = any(
                    fnmatch.fnmatch(context.scope, pattern) for pattern in condition.file_pattern
                )
                results.append(matches_pattern)

        # If no conditions were set, return False
        if not results:
            return False

        # All atomic conditions must match (implicit AND)
        return all(results)

    @property
    def overrides(self) -> list[Override]:
        """Get all override rules (sorted by priority)."""
        return self._overrides.copy()

    @property
    def chain_extensions(self) -> list[ChainExtension]:
        """Get all chain extension rules."""
        return self._chain_extensions.copy()

    def clear(self) -> None:
        """Clear all rules."""
        self._overrides.clear()
        self._chain_extensions.clear()
