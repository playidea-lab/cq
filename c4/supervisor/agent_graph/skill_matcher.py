"""SkillMatcher - Task-to-skill extraction and agent matching.

This module provides skill-based routing by:
1. Extracting required skills from a task based on skill triggers
2. Finding the best agents for those skills

Usage:
    >>> from c4.supervisor.agent_graph import AgentGraph, SkillMatcher
    >>> graph = AgentGraph()
    >>> # ... load skills and agents ...
    >>> matcher = SkillMatcher(graph)
    >>> skills = matcher.extract_required_skills(task)
    >>> agents = matcher.find_best_agents(skills)
"""

from __future__ import annotations

import fnmatch
from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.graph import AgentGraph


class TaskLike(Protocol):
    """Protocol for task-like objects that can be matched against skills.

    Supports both simple dict-like access and full dataclass objects.
    """

    @property
    def title(self) -> str:
        """Task title."""
        ...

    @property
    def description(self) -> str:
        """Task description."""
        ...

    @property
    def task_type(self) -> str | None:
        """Task type (e.g., 'feature', 'bugfix', 'refactor')."""
        ...

    @property
    def scope(self) -> str | None:
        """File or directory scope for the task."""
        ...


@dataclass
class TaskContext:
    """Simple task context for skill matching.

    Provides a concrete implementation of TaskLike for cases where
    you only have basic task information.

    Args:
        title: Task title (required)
        description: Task description (default: "")
        task_type: Task type (default: None)
        scope: File/directory scope (default: None)

    Example:
        >>> task = TaskContext(title="Fix Python bug", task_type="bugfix")
        >>> matcher.extract_required_skills(task)
        ['python-coding', 'debugging']
    """

    title: str
    description: str = ""
    task_type: str | None = None
    scope: str | None = None

    def __post_init__(self) -> None:
        """Validate task context."""
        if not self.title or not self.title.strip():
            raise ValueError("title cannot be empty")


@dataclass
class AgentMatch:
    """Represents an agent match with scoring details.

    Attributes:
        agent_id: The matched agent's ID
        score: Total match score (higher is better)
        matched_skills: List of skills that matched
        primary_match_count: Number of primary skill matches
        secondary_match_count: Number of secondary skill matches
    """

    agent_id: str
    score: float
    matched_skills: list[str]
    primary_match_count: int = 0
    secondary_match_count: int = 0


class SkillMatcher:
    """Matches tasks to skills and finds best agents.

    The SkillMatcher uses skill triggers (keywords, task_types, file_patterns)
    to determine which skills a task requires, then finds agents that possess
    those skills.

    Scoring:
    - Each matched skill adds 1.0 to the score
    - Primary skill matches add an additional 0.5 bonus
    - Agents are sorted by score descending

    Args:
        graph: AgentGraph instance with skills and agents

    Example:
        >>> matcher = SkillMatcher(graph)
        >>> task = TaskContext(title="Debug Django API", scope="src/api.py")
        >>> skills = matcher.extract_required_skills(task)
        >>> # skills = ['python-coding', 'api-design', 'debugging']
        >>> agents = matcher.find_best_agents(skills)
        >>> # agents = [AgentMatch(agent_id='backend-dev', score=2.5, ...)]
    """

    # Bonus score for primary skill matches
    PRIMARY_SKILL_BONUS = 0.5

    def __init__(self, graph: AgentGraph) -> None:
        """Initialize SkillMatcher with an agent graph.

        Args:
            graph: AgentGraph instance containing skills and agents
        """
        self._graph = graph

    def extract_required_skills(self, task: TaskLike) -> list[str]:
        """Extract required skills from a task based on skill triggers.

        Analyzes the task's title, description, task_type, and scope to
        determine which skills are needed. Skills are matched using their
        trigger definitions:
        - keywords: Matched against title and description (case-insensitive)
        - task_types: Matched against task_type (case-insensitive)
        - file_patterns: Matched against scope using glob patterns

        Args:
            task: Task-like object with title, description, task_type, scope

        Returns:
            List of skill IDs that match the task. Empty list if no matches.

        Example:
            >>> task = TaskContext(title="Fix bug in login.py", task_type="bugfix")
            >>> skills = matcher.extract_required_skills(task)
            >>> # Returns skills that trigger on "bug", "login", "*.py", or "bugfix"
        """
        required_skills: list[str] = []
        text = f"{task.title} {task.description}".lower()

        for skill_id in self._graph.skills:
            if self._skill_matches_task(skill_id, task, text):
                required_skills.append(skill_id)

        return required_skills

    def _skill_matches_task(
        self, skill_id: str, task: TaskLike, text: str
    ) -> bool:
        """Check if a skill matches the task based on its triggers.

        Args:
            skill_id: ID of the skill to check
            task: The task being matched
            text: Lowercase concatenation of title and description

        Returns:
            True if any trigger matches
        """
        node = self._graph.get_node(skill_id)
        if not node:
            return False

        definition = node.get("definition")
        if not definition:
            return False

        triggers = definition.skill.triggers

        # Check keywords
        if triggers.keywords:
            for keyword in triggers.keywords:
                if keyword.lower() in text:
                    return True

        # Check task_types
        if triggers.task_types and task.task_type:
            task_type_lower = task.task_type.lower()
            for trigger_type in triggers.task_types:
                if trigger_type.lower() == task_type_lower:
                    return True

        # Check file_patterns
        if triggers.file_patterns and task.scope:
            for pattern in triggers.file_patterns:
                if fnmatch.fnmatch(task.scope, pattern):
                    return True

        return False

    def find_best_agents(
        self,
        required_skills: list[str],
        domain: str | None = None,
    ) -> list[AgentMatch]:
        """Find agents that best match the required skills.

        Scores agents based on how many required skills they possess.
        Primary skills receive a bonus. Results are sorted by score
        descending.

        Args:
            required_skills: List of skill IDs to match
            domain: Optional domain filter (not implemented yet)

        Returns:
            List of AgentMatch objects sorted by score (highest first).
            Empty list if no skills provided or no agents match.

        Example:
            >>> agents = matcher.find_best_agents(['python-coding', 'debugging'])
            >>> for agent in agents:
            ...     print(f"{agent.agent_id}: {agent.score} ({agent.matched_skills})")
            backend-dev: 2.5 (['python-coding', 'debugging'])
            debugger: 1.5 (['debugging'])
        """
        if not required_skills:
            return []

        matches: list[AgentMatch] = []
        required_set = set(required_skills)

        for agent_id in self._graph.agents:
            agent_skills = set(self._graph.find_skills_for_agent(agent_id))
            matched = agent_skills & required_set

            if matched:
                primary_skills = self._get_primary_skills(agent_id)
                primary_matched = matched & primary_skills
                secondary_matched = matched - primary_matched

                # Calculate score
                base_score = len(matched)
                primary_bonus = len(primary_matched) * self.PRIMARY_SKILL_BONUS
                score = base_score + primary_bonus

                matches.append(
                    AgentMatch(
                        agent_id=agent_id,
                        score=score,
                        matched_skills=sorted(matched),
                        primary_match_count=len(primary_matched),
                        secondary_match_count=len(secondary_matched),
                    )
                )

        # Sort by score descending, then by agent_id for stable sorting
        matches.sort(key=lambda m: (-m.score, m.agent_id))
        return matches

    def _get_primary_skills(self, agent_id: str) -> set[str]:
        """Get primary skills for an agent.

        Args:
            agent_id: Agent ID to look up

        Returns:
            Set of primary skill IDs
        """
        node = self._graph.get_node(agent_id)
        if not node:
            return set()

        definition = node.get("definition")
        if not definition:
            return set()

        return set(definition.agent.skills.primary)

    def get_recommended_agent(
        self,
        task: TaskLike,
        domain: str | None = None,
    ) -> str | None:
        """Get the single best agent for a task.

        Convenience method that extracts skills and returns the top match.

        Args:
            task: Task to match
            domain: Optional domain filter

        Returns:
            Agent ID of the best match, or None if no match found

        Example:
            >>> agent = matcher.get_recommended_agent(task)
            >>> print(agent)  # "backend-dev"
        """
        skills = self.extract_required_skills(task)
        if not skills:
            return None

        agents = self.find_best_agents(skills, domain)
        if not agents:
            return None

        return agents[0].agent_id
