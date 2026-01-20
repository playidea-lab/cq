"""GraphRouter - Graph-based agent routing with skill matching and rules.

This module provides a GraphRouter that extends AgentRouter with:
1. Rule-based routing via RuleEngine (highest priority)
2. Skill-based routing via SkillMatcher
3. AgentGraph integration for relationship-aware routing
4. Backward compatibility with legacy domain-based routing

Usage:
    >>> from c4.supervisor.agent_graph import GraphRouter, SkillMatcher, TaskContext
    >>> router = GraphRouter(skill_matcher=matcher, rule_engine=engine)
    >>> config = router.get_recommended_agent("web-backend", task=task)
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

from c4.discovery.models import Domain
from c4.supervisor.agent_router import (
    AgentChainConfig,
    AgentRouter,
)

if TYPE_CHECKING:
    from c4.models.config import AgentConfig
    from c4.supervisor.agent_graph.graph import AgentGraph
    from c4.supervisor.agent_graph.rule_engine import RuleContext, RuleEngine
    from c4.supervisor.agent_graph.skill_matcher import SkillMatcher, TaskLike


@dataclass
class RoutingResult:
    """Result of a routing decision with metadata.

    Attributes:
        config: The agent chain configuration
        routing_method: How the agent was selected ("rule", "skill", "domain", "task_type")
        matched_skills: Skills that matched (if skill-based routing)
        skill_score: Score from skill matching (if applicable)
        matched_rule: Name of matched rule (if rule-based routing)
        rule_reason: Reason from matched rule (if rule-based routing)
    """

    config: AgentChainConfig
    routing_method: str
    matched_skills: list[str] | None = None
    skill_score: float | None = None
    matched_rule: str | None = None
    rule_reason: str | None = None


class GraphRouter:
    """Graph-based agent router with skill matching and rule engine support.

    Extends AgentRouter functionality with:
    - Rule-based routing: Evaluate override rules with complex conditions
    - Skill-based routing: Match tasks to agents via skill triggers
    - Domain fallback: Fall back to domain routing if no skill match
    - Task type overrides: Respect task type → agent mappings
    - Full backward compatibility: Works without SkillMatcher or RuleEngine

    Routing Priority:
    1. Rule override (if RuleEngine and task provided)
    2. Task type override (e.g., "debug" → "debugger")
    3. Skill-based routing (if task and SkillMatcher provided)
    4. Domain-based routing (fallback)

    Args:
        config: Optional AgentConfig for custom chains/overrides
        skill_matcher: Optional SkillMatcher for skill-based routing
        graph: Optional AgentGraph (used by SkillMatcher if not provided)
        rule_engine: Optional RuleEngine for rule-based routing

    Example:
        >>> # With skill matching and rules
        >>> matcher = SkillMatcher(graph)
        >>> engine = RuleEngine()
        >>> router = GraphRouter(skill_matcher=matcher, rule_engine=engine)
        >>> task = TaskContext(title="Fix Python API bug", scope="api.py")
        >>> config = router.get_recommended_agent("web-backend", task=task)
        >>> # Uses rules → skill matching → backend-dev or debugger

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
        rule_engine: RuleEngine | None = None,
    ) -> None:
        """Initialize GraphRouter.

        Args:
            config: Optional AgentConfig for custom domain chains
            skill_matcher: Optional SkillMatcher for skill-based routing
            graph: Optional AgentGraph (creates SkillMatcher if not provided)
            rule_engine: Optional RuleEngine for rule-based routing
        """
        self._legacy_router = AgentRouter(config=config)
        self._skill_matcher = skill_matcher
        self._graph = graph
        self._rule_engine = rule_engine

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

    @property
    def rule_engine(self) -> RuleEngine | None:
        """Get the rule engine instance."""
        return self._rule_engine

    @property
    def use_legacy_fallback(self) -> bool:
        """Check if using legacy domain-only routing.

        Returns True if no skill matcher or rule engine is configured,
        meaning the router will only use domain-based routing.
        """
        return self._skill_matcher is None and self._rule_engine is None

    def _create_rule_context(
        self,
        task: TaskLike,
        domain: str | Domain | None,
    ) -> RuleContext:
        """Create a RuleContext from task and domain.

        Args:
            task: Task information
            domain: Domain string or enum

        Returns:
            RuleContext for rule evaluation
        """
        from c4.supervisor.agent_graph.rule_engine import RuleContext

        domain_str = domain.value if isinstance(domain, Domain) else domain
        return RuleContext(
            task_type=getattr(task, "task_type", None),
            domain=domain_str,
            title=getattr(task, "title", ""),
            description=getattr(task, "description", ""),
            scope=getattr(task, "scope", ""),
        )

    def _apply_chain_extensions(
        self,
        config: AgentChainConfig,
        task: TaskLike,
        domain: str | Domain | None,
    ) -> AgentChainConfig:
        """Apply chain extension rules to a config.

        Args:
            config: Current config
            task: Task for matching
            domain: Domain for context

        Returns:
            Modified AgentChainConfig with extensions applied
        """
        if self._rule_engine is None:
            return config

        context = self._create_rule_context(task, domain)
        extensions = self._rule_engine.find_matching_chain_extensions(context)

        if not extensions:
            return config

        chain = list(config.chain)
        for ext in extensions:
            action = ext.action

            # Handle add_to_chain
            if action.add_to_chain and action.add_to_chain not in chain:
                position = action.position or "before_last"
                if position == "first":
                    chain.insert(0, action.add_to_chain)
                elif position == "after_primary":
                    chain.insert(1, action.add_to_chain)
                elif position == "before_last":
                    if len(chain) > 1:
                        chain.insert(-1, action.add_to_chain)
                    else:
                        chain.append(action.add_to_chain)
                else:  # last
                    chain.append(action.add_to_chain)

            # Handle ensure_in_chain
            if action.ensure_in_chain:
                for agent in action.ensure_in_chain:
                    if agent not in chain:
                        chain.append(agent)

        return AgentChainConfig(
            primary=config.primary,
            chain=chain,
            description=config.description,
            handoff_instructions=config.handoff_instructions,
        )

    def get_recommended_agent(
        self,
        domain: str | Domain | None,
        task: TaskLike | None = None,
    ) -> AgentChainConfig:
        """Get recommended agent chain configuration.

        Routing priority:
        1. If RuleEngine has matching override → use rule-defined agent
        2. If task has task_type that matches an override → use override agent
        3. If task provided and SkillMatcher available → try skill matching
        4. Fall back to domain-based routing

        Chain extensions are applied after routing decision.

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
        # 1. Check rule engine overrides first
        if task is not None and self._rule_engine is not None:
            rule_result = self._try_rule_routing(task, domain)
            if rule_result is not None:
                return rule_result

        # 2. Check task type override
        if task is not None and hasattr(task, "task_type") and task.task_type:
            override_agent = self._get_task_type_override(task.task_type)
            if override_agent:
                config = self._build_config_for_agent(override_agent, domain)
                if task is not None and self._rule_engine is not None:
                    config = self._apply_chain_extensions(config, task, domain)
                return config

        # 3. Try skill-based routing if task and matcher available
        if task is not None and self._skill_matcher is not None:
            skill_result = self._try_skill_routing(task, domain)
            if skill_result is not None:
                if self._rule_engine is not None:
                    skill_result = self._apply_chain_extensions(skill_result, task, domain)
                return skill_result

        # 4. Fall back to domain-based routing
        config = self._legacy_router.get_recommended_agent(domain)
        if task is not None and self._rule_engine is not None:
            config = self._apply_chain_extensions(config, task, domain)
        return config

    def _try_rule_routing(
        self,
        task: TaskLike,
        domain: str | Domain | None,
    ) -> AgentChainConfig | None:
        """Try to route based on rule engine.

        Args:
            task: Task to match
            domain: Domain for context

        Returns:
            AgentChainConfig if rule match found, None otherwise
        """
        if self._rule_engine is None:
            return None

        context = self._create_rule_context(task, domain)
        override = self._rule_engine.find_matching_override(context)

        if override is None:
            return None

        # Build config from override action
        if override.action.set_primary:
            config = self._build_config_for_agent(override.action.set_primary, domain)
        else:
            # No set_primary, use domain default
            config = self._legacy_router.get_recommended_agent(domain)

        # Apply add_to_chain from override
        if override.action.add_to_chain:
            chain = list(config.chain)
            if override.action.add_to_chain not in chain:
                position = override.action.position or "before_last"
                if position == "first":
                    chain.insert(0, override.action.add_to_chain)
                elif position == "after_primary":
                    chain.insert(1, override.action.add_to_chain)
                elif position == "before_last":
                    if len(chain) > 1:
                        chain.insert(-1, override.action.add_to_chain)
                    else:
                        chain.append(override.action.add_to_chain)
                else:  # last
                    chain.append(override.action.add_to_chain)
            config = AgentChainConfig(
                primary=config.primary,
                chain=chain,
                description=f"Rule override: {override.name}",
                handoff_instructions=config.handoff_instructions,
            )

        # Apply chain extensions
        config = self._apply_chain_extensions(config, task, domain)

        return config

    def get_recommended_agent_with_details(
        self,
        domain: str | Domain | None,
        task: TaskLike | None = None,
    ) -> RoutingResult:
        """Get recommended agent with routing details.

        Same as get_recommended_agent but returns additional metadata
        about how the routing decision was made.

        Routing priority:
        1. Rule override (if RuleEngine and task provided)
        2. Task type override (e.g., "debug" → "debugger")
        3. Skill-based routing (if task and SkillMatcher provided)
        4. Domain-based routing (fallback)

        Args:
            domain: Domain string, Domain enum, or None
            task: Optional task for skill-based routing

        Returns:
            RoutingResult with config and routing metadata
        """
        # 1. Check rule engine overrides first
        if task is not None and self._rule_engine is not None:
            context = self._create_rule_context(task, domain)
            override = self._rule_engine.find_matching_override(context)
            if override is not None:
                config = self._try_rule_routing(task, domain)
                if config is not None:
                    return RoutingResult(
                        config=config,
                        routing_method="rule",
                        matched_rule=override.name,
                        rule_reason=override.reason,
                    )

        # 2. Check task type override
        if task is not None and hasattr(task, "task_type") and task.task_type:
            override_agent = self._get_task_type_override(task.task_type)
            if override_agent:
                config = self._build_config_for_agent(override_agent, domain)
                if self._rule_engine is not None:
                    config = self._apply_chain_extensions(config, task, domain)
                return RoutingResult(
                    config=config,
                    routing_method="task_type",
                )

        # 3. Try skill-based routing
        if task is not None and self._skill_matcher is not None:
            skills = self._skill_matcher.extract_required_skills(task)
            if skills:
                agents = self._skill_matcher.find_best_agents(skills)
                if agents:
                    best_match = agents[0]
                    config = self._build_config_for_agent(best_match.agent_id, domain)
                    if self._rule_engine is not None:
                        config = self._apply_chain_extensions(config, task, domain)
                    return RoutingResult(
                        config=config,
                        routing_method="skill",
                        matched_skills=best_match.matched_skills,
                        skill_score=best_match.score,
                    )

        # 4. Fall back to domain-based routing
        config = self._legacy_router.get_recommended_agent(domain)
        if task is not None and self._rule_engine is not None:
            config = self._apply_chain_extensions(config, task, domain)
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
