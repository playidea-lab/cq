"""GraphRouter - Graph-based agent routing with skill matching.

This module provides a GraphRouter that extends AgentRouter with:
1. Skill-based routing via SkillMatcher
2. AgentGraph integration for relationship-aware routing
3. Backward compatibility with legacy domain-based routing

Usage:
    >>> from c4.supervisor.agent_graph import GraphRouter, SkillMatcher, TaskContext
    >>> router = GraphRouter(skill_matcher=matcher)
    >>> config = router.get_recommended_agent("web-backend", task=task)
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

from c4.supervisor.agent_router import (
    AgentChainConfig,
    AgentRouter,
    Domain,
)

if TYPE_CHECKING:
    from c4.models.config import AgentConfig
    from c4.supervisor.agent_graph.graph import AgentGraph
    from c4.supervisor.agent_graph.skill_matcher import SkillMatcher, TaskLike


@dataclass
class RoutingResult:
    """Result of a routing decision with metadata.

    Attributes:
        config: The agent chain configuration
        routing_method: How the agent was selected ("skill", "domain", "task_type")
        matched_skills: Skills that matched (if skill-based routing)
        skill_score: Score from skill matching (if applicable)
    """

    config: AgentChainConfig
    routing_method: str
    matched_skills: list[str] | None = None
    skill_score: float | None = None


class GraphRouter:
    """Graph-based agent router with skill matching support.

    Extends AgentRouter functionality with:
    - Skill-based routing: Match tasks to agents via skill triggers
    - Domain fallback: Fall back to domain routing if no skill match
    - Task type overrides: Respect task type → agent mappings
    - Full backward compatibility: Works without SkillMatcher

    Routing Priority:
    1. Task type override (e.g., "debug" → "debugger")
    2. Skill-based routing (if task and SkillMatcher provided)
    3. Domain-based routing (fallback)

    Args:
        config: Optional AgentConfig for custom chains/overrides
        skill_matcher: Optional SkillMatcher for skill-based routing
        graph: Optional AgentGraph (used by SkillMatcher if not provided)

    Example:
        >>> # With skill matching
        >>> matcher = SkillMatcher(graph)
        >>> router = GraphRouter(skill_matcher=matcher)
        >>> task = TaskContext(title="Fix Python API bug", scope="api.py")
        >>> config = router.get_recommended_agent("web-backend", task=task)
        >>> # Uses skill matching → backend-dev or debugger

        >>> # Without skill matching (legacy behavior)
        >>> router = GraphRouter()
        >>> config = router.get_recommended_agent("web-backend")
        >>> # Uses domain mapping → backend-architect
    """

    def __init__(
        self,
        config: AgentConfig | None = None,
        skill_matcher: SkillMatcher | None = None,
        graph: AgentGraph | None = None,
    ) -> None:
        """Initialize GraphRouter.

        Args:
            config: Optional AgentConfig for custom domain chains
            skill_matcher: Optional SkillMatcher for skill-based routing
            graph: Optional AgentGraph (creates SkillMatcher if not provided)
        """
        self._legacy_router = AgentRouter(config=config)
        self._skill_matcher = skill_matcher
        self._graph = graph

        # If graph provided but no matcher, create one
        if graph is not None and skill_matcher is None:
            from c4.supervisor.agent_graph.skill_matcher import SkillMatcher

            self._skill_matcher = SkillMatcher(graph)

    @property
    def skill_matcher(self) -> SkillMatcher | None:
        """Get the skill matcher instance."""
        return self._skill_matcher

    @property
    def graph(self) -> AgentGraph | None:
        """Get the agent graph instance."""
        return self._graph

    def get_recommended_agent(
        self,
        domain: str | Domain | None,
        task: TaskLike | None = None,
    ) -> AgentChainConfig:
        """Get recommended agent chain configuration.

        Routing priority:
        1. If task has task_type that matches an override → use override agent
        2. If task provided and SkillMatcher available → try skill matching
        3. Fall back to domain-based routing

        Args:
            domain: Domain string, Domain enum, or None
            task: Optional task for skill-based routing

        Returns:
            AgentChainConfig with primary agent and chain

        Example:
            >>> # Skill-based routing
            >>> task = TaskContext(title="Debug Python API", task_type="bugfix")
            >>> config = router.get_recommended_agent("web-backend", task=task)

            >>> # Domain-only routing (legacy)
            >>> config = router.get_recommended_agent("web-backend")
        """
        # 1. Check task type override first
        if task is not None and hasattr(task, "task_type") and task.task_type:
            override_agent = self._get_task_type_override(task.task_type)
            if override_agent:
                return self._build_config_for_agent(override_agent, domain)

        # 2. Try skill-based routing if task and matcher available
        if task is not None and self._skill_matcher is not None:
            skill_result = self._try_skill_routing(task, domain)
            if skill_result is not None:
                return skill_result

        # 3. Fall back to domain-based routing
        return self._legacy_router.get_recommended_agent(domain)

    def get_recommended_agent_with_details(
        self,
        domain: str | Domain | None,
        task: TaskLike | None = None,
    ) -> RoutingResult:
        """Get recommended agent with routing details.

        Same as get_recommended_agent but returns additional metadata
        about how the routing decision was made.

        Args:
            domain: Domain string, Domain enum, or None
            task: Optional task for skill-based routing

        Returns:
            RoutingResult with config and routing metadata
        """
        # 1. Check task type override first
        if task is not None and hasattr(task, "task_type") and task.task_type:
            override_agent = self._get_task_type_override(task.task_type)
            if override_agent:
                config = self._build_config_for_agent(override_agent, domain)
                return RoutingResult(
                    config=config,
                    routing_method="task_type",
                )

        # 2. Try skill-based routing
        if task is not None and self._skill_matcher is not None:
            skills = self._skill_matcher.extract_required_skills(task)
            if skills:
                agents = self._skill_matcher.find_best_agents(skills)
                if agents:
                    best_match = agents[0]
                    config = self._build_config_for_agent(best_match.agent_id, domain)
                    return RoutingResult(
                        config=config,
                        routing_method="skill",
                        matched_skills=best_match.matched_skills,
                        skill_score=best_match.score,
                    )

        # 3. Fall back to domain-based routing
        config = self._legacy_router.get_recommended_agent(domain)
        return RoutingResult(
            config=config,
            routing_method="domain",
        )

    def _get_task_type_override(self, task_type: str) -> str | None:
        """Check if task type has an agent override.

        Args:
            task_type: Task type string (e.g., "debug", "security")

        Returns:
            Override agent name or None
        """
        task_type_lower = task_type.lower().replace("_", "-")
        overrides = self._legacy_router.merged_overrides
        return overrides.get(task_type_lower)

    def _try_skill_routing(
        self,
        task: TaskLike,
        domain: str | Domain | None,
    ) -> AgentChainConfig | None:
        """Try to route based on skill matching.

        Args:
            task: Task to match
            domain: Domain for fallback chain building

        Returns:
            AgentChainConfig if skill match found, None otherwise
        """
        if self._skill_matcher is None:
            return None

        skills = self._skill_matcher.extract_required_skills(task)
        if not skills:
            return None

        agents = self._skill_matcher.find_best_agents(skills)
        if not agents:
            return None

        # Use best matching agent
        best_agent = agents[0].agent_id
        return self._build_config_for_agent(best_agent, domain)

    def _build_config_for_agent(
        self,
        agent_id: str,
        domain: str | Domain | None,
    ) -> AgentChainConfig:
        """Build an AgentChainConfig for a specific agent.

        Uses the graph to build a chain if available, otherwise
        returns a simple single-agent chain.

        Args:
            agent_id: Primary agent ID
            domain: Domain for chain lookup

        Returns:
            AgentChainConfig with agent and chain
        """
        # Try to get chain from domain config first
        domain_config = self._legacy_router.get_recommended_agent(domain)

        # If the agent matches domain primary, use that config
        if domain_config.primary == agent_id:
            return domain_config

        # Build a new config with the skill-matched agent
        # Try to get chain from graph if available
        chain = [agent_id]
        if self._graph is not None:
            graph_chain = self._build_chain_from_graph(agent_id, max_length=5)
            if graph_chain:
                chain = graph_chain

        return AgentChainConfig(
            primary=agent_id,
            chain=chain,
            description=f"Skill-matched agent: {agent_id}",
            handoff_instructions=domain_config.handoff_instructions,
        )

    def _build_chain_from_graph(
        self,
        primary_agent: str,
        max_length: int = 10,
    ) -> list[str]:
        """Build an agent chain starting from the primary agent.

        Follows HANDS_OFF_TO edges in order of weight (highest first),
        preventing cycles and respecting the maximum chain length.

        Args:
            primary_agent: The starting agent ID
            max_length: Maximum number of agents in the chain

        Returns:
            List of agent IDs forming the chain
        """
        if self._graph is None:
            return [primary_agent]

        chain: list[str] = []
        visited: set[str] = set()
        current = primary_agent

        while len(chain) < max_length:
            chain.append(current)
            visited.add(current)

            # Find handoff targets sorted by weight
            targets = self._graph.find_handoff_targets(current)

            # Find first unvisited target
            next_agent = None
            for target_id, _weight in targets:
                if target_id not in visited:
                    next_agent = target_id
                    break

            if next_agent is None:
                break

            current = next_agent

        return chain

    # =========================================================================
    # Delegate methods to legacy router for backward compatibility
    # =========================================================================

    def get_agent_for_task_type(
        self,
        task_type: str | None,
        domain: str | Domain | None = None,
    ) -> str:
        """Get specific agent for a task type.

        Delegates to legacy router for backward compatibility.
        """
        return self._legacy_router.get_agent_for_task_type(task_type, domain)

    def get_chain_for_domain(self, domain: str | Domain | None) -> list[str]:
        """Get agent chain list for a domain."""
        return self._legacy_router.get_chain_for_domain(domain)

    def get_handoff_instructions(self, domain: str | Domain | None) -> str:
        """Get handoff instructions for a domain's agent chain."""
        return self._legacy_router.get_handoff_instructions(domain)

    def get_all_domains(self) -> list[str]:
        """Get list of all supported domains."""
        return self._legacy_router.get_all_domains()

    # =========================================================================
    # Graph-specific methods
    # =========================================================================

    def find_agents_for_skill(self, skill_id: str) -> list[str]:
        """Find agents that have a specific skill.

        Args:
            skill_id: Skill ID to search for

        Returns:
            List of agent IDs with that skill

        Raises:
            ValueError: If no graph is loaded
        """
        if self._graph is None:
            raise ValueError("No graph loaded. Cannot query skills.")
        return self._graph.find_agents_with_skill(skill_id)

    def get_path_between_agents(
        self,
        from_agent: str,
        to_agent: str,
    ) -> list[str] | None:
        """Find the shortest handoff path between two agents.

        Args:
            from_agent: Source agent ID
            to_agent: Target agent ID

        Returns:
            List of agent IDs forming the path, or None

        Raises:
            ValueError: If no graph is loaded
        """
        if self._graph is None:
            raise ValueError("No graph loaded. Cannot query paths.")
        return self._graph.get_path(from_agent, to_agent)
