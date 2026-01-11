"""GraphRouter - AgentRouter-compatible router using AgentGraph and RuleEngine.

This module provides a graph-based router that is compatible with the existing
AgentRouter interface but uses the AgentGraph for relationship-based routing
and RuleEngine for rule-based overrides.

The GraphRouter can be used as a drop-in replacement for AgentRouter while
providing more flexible and configurable agent routing.

Fallback Support:
    When no YAML definitions are loaded, GraphRouter falls back to the existing
    DOMAIN_AGENT_MAP and TASK_TYPE_AGENT_OVERRIDES from agent_router.py,
    ensuring 100% backwards compatibility.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

from c4.supervisor.agent_graph.graph import AgentGraph
from c4.supervisor.agent_graph.rules import RuleEngine, Task

if TYPE_CHECKING:
    pass


@dataclass
class AgentChainConfig:
    """Configuration for an agent chain.

    This matches the interface used by AgentRouter for compatibility.
    """

    primary: str
    chain: list[str] = field(default_factory=list)
    description: str = ""
    handoff_instructions: str = ""

    def __post_init__(self) -> None:
        """Ensure primary is in chain."""
        if self.primary and self.primary not in self.chain:
            self.chain = [self.primary] + self.chain


class GraphRouter:
    """AgentRouter-compatible router using AgentGraph and RuleEngine.

    This class provides the same interface as AgentRouter but uses:
    - AgentGraph for relationship-based routing (skills, handoffs, preferences)
    - RuleEngine for rule-based overrides and chain extensions

    Example:
        >>> graph = AgentGraph()
        >>> # ... add skills, agents, domains ...
        >>> router = GraphRouter(graph=graph)
        >>> config = router.get_recommended_agent("web-backend")
        >>> print(config.primary)  # "backend-dev"
        >>> print(config.chain)    # ["backend-dev", "test-automator", "code-reviewer"]
    """

    def __init__(
        self,
        graph: AgentGraph | None = None,
        rule_engine: RuleEngine | None = None,
        use_legacy_fallback: bool = True,
    ) -> None:
        """Initialize the GraphRouter.

        Args:
            graph: AgentGraph to use for routing. If None, creates an empty graph.
            rule_engine: RuleEngine to use for rules. If None, creates an empty engine.
            use_legacy_fallback: If True, falls back to DOMAIN_AGENT_MAP and
                TASK_TYPE_AGENT_OVERRIDES when graph has no data. Default True
                for backwards compatibility.
        """
        self._graph = graph if graph is not None else AgentGraph()
        self._rule_engine = rule_engine if rule_engine is not None else RuleEngine()
        self._use_legacy_fallback = use_legacy_fallback
        self._legacy_chains: dict[str, AgentChainConfig] | None = None
        self._legacy_overrides: dict[str, str] | None = None

    @property
    def use_legacy_fallback(self) -> bool:
        """Whether legacy fallback is enabled."""
        return self._use_legacy_fallback

    def _get_legacy_chains(self) -> dict[str, AgentChainConfig]:
        """Get legacy domain chains (lazy loaded)."""
        if self._legacy_chains is None:
            from c4.supervisor.agent_router import DOMAIN_AGENT_MAP

            # Convert to our AgentChainConfig
            self._legacy_chains = {}
            for domain, config in DOMAIN_AGENT_MAP.items():
                self._legacy_chains[domain] = AgentChainConfig(
                    primary=config.primary,
                    chain=list(config.chain),
                    description=config.description,
                    handoff_instructions=config.handoff_instructions,
                )
        return self._legacy_chains

    def _get_legacy_overrides(self) -> dict[str, str]:
        """Get legacy task type overrides (lazy loaded)."""
        if self._legacy_overrides is None:
            from c4.supervisor.agent_router import TASK_TYPE_AGENT_OVERRIDES

            self._legacy_overrides = dict(TASK_TYPE_AGENT_OVERRIDES)
        return self._legacy_overrides

    def _should_use_legacy(self) -> bool:
        """Check if we should use legacy fallback.

        Returns True if legacy fallback is enabled and the graph has no domains.
        """
        if not self._use_legacy_fallback:
            return False
        return len(self._graph.domains) == 0

    @property
    def graph(self) -> AgentGraph:
        """Get the underlying AgentGraph."""
        return self._graph

    @property
    def rule_engine(self) -> RuleEngine:
        """Get the underlying RuleEngine."""
        return self._rule_engine

    def get_recommended_agent(self, domain: str | None) -> AgentChainConfig:
        """Get the recommended agent configuration for a domain.

        This method:
        1. Checks if legacy fallback should be used (empty graph)
        2. Looks up the domain in the graph to find preferred agents
        3. Builds an agent chain following handoff relationships
        4. Applies any rule-based chain extensions

        Args:
            domain: The domain ID to get configuration for

        Returns:
            AgentChainConfig with primary agent and chain
        """
        # Use legacy fallback if graph is empty
        if self._should_use_legacy():
            return self._get_legacy_config(domain)

        # Handle None/unknown domain with fallback
        if domain is None or not self._graph.get_node(domain):
            return self._get_fallback_config(domain)

        # Get domain preferences from graph
        domain_info = self._graph.find_agents_for_domain(domain)
        primary = domain_info.get("primary")

        if not primary:
            return self._get_fallback_config(domain)

        # Build chain from primary agent using handoff relationships
        chain = self._graph.build_chain(primary)

        # Apply chain extensions from rules
        task = Task(title="(domain routing)", domain=domain)
        chain = self._rule_engine.extend_chain(chain, task)

        # Get domain node for description
        domain_node = self._graph.get_node(domain)
        description = domain_node.get("description", "") if domain_node else ""

        return AgentChainConfig(
            primary=primary,
            chain=chain,
            description=description,
            handoff_instructions=f"Domain: {domain}",
        )

    def _get_legacy_config(self, domain: str | None) -> AgentChainConfig:
        """Get configuration from legacy DOMAIN_AGENT_MAP.

        Args:
            domain: The domain ID to get configuration for

        Returns:
            AgentChainConfig from legacy map
        """
        legacy_chains = self._get_legacy_chains()

        if domain is None:
            return legacy_chains.get("unknown", AgentChainConfig(
                primary="general-purpose",
                chain=["general-purpose"],
                description="Default fallback",
                handoff_instructions="Unknown domain",
            ))

        # Normalize domain string
        domain_str = domain.lower().replace("_", "-")

        return legacy_chains.get(domain_str, legacy_chains.get("unknown", AgentChainConfig(
            primary="general-purpose",
            chain=["general-purpose"],
            description="Default fallback",
            handoff_instructions="Unknown domain",
        )))

    def get_agent_for_task_type(
        self,
        task_type: str | None,
        domain: str | None,
        title: str = "",
        description: str = "",
    ) -> str:
        """Get the recommended agent for a specific task type.

        This method:
        1. Checks for legacy task type overrides (if using legacy fallback)
        2. Checks for rule-based overrides matching the task
        3. Falls back to domain-based routing if no override matches

        Args:
            task_type: The type of task (e.g., "feature", "bugfix", "debug")
            domain: The domain of the task
            title: The task title (used for keyword matching)
            description: The task description (used for keyword matching)

        Returns:
            Agent ID to assign to the task
        """
        # Check legacy task type overrides first (if using legacy fallback)
        if self._should_use_legacy() and task_type:
            legacy_overrides = self._get_legacy_overrides()
            task_type_lower = task_type.lower().replace("_", "-")
            if task_type_lower in legacy_overrides:
                return legacy_overrides[task_type_lower]

        # Create task for rule evaluation
        task = Task(
            title=title if title else "(task type routing)",
            description=description,
            task_type=task_type,
            domain=domain,
        )

        # Check for rule-based overrides
        override_agent = self._rule_engine.apply_overrides(task, self._graph)
        if override_agent:
            return override_agent

        # Fall back to domain-based routing
        config = self.get_recommended_agent(domain)
        return config.primary

    def get_chain_for_domain(self, domain: str | None) -> list[str]:
        """Get the agent chain for a domain.

        This is a convenience method that returns just the chain
        from get_recommended_agent.

        Args:
            domain: The domain ID

        Returns:
            List of agent IDs in the chain
        """
        config = self.get_recommended_agent(domain)
        return config.chain

    def get_all_domains(self) -> list[str]:
        """Get list of all supported domains.

        Returns domains from both graph and legacy fallback.
        """
        domains = set(self._graph.domains)

        if self._use_legacy_fallback:
            domains.update(self._get_legacy_chains().keys())

        return sorted(domains)

    def get_handoff_instructions(self, domain: str | None) -> str:
        """Get handoff instructions for a domain's agent chain.

        Args:
            domain: Domain string

        Returns:
            Instructions string for context passing between agents
        """
        config = self.get_recommended_agent(domain)
        return config.handoff_instructions

    def _get_fallback_config(self, domain: str | None = None) -> AgentChainConfig:
        """Get fallback configuration when domain is unknown.

        First tries legacy fallback if enabled, then falls back to
        graph agents or a default.

        Args:
            domain: The domain that wasn't found (used for legacy fallback)

        Returns:
            AgentChainConfig with fallback agent configuration
        """
        # Try legacy fallback if enabled
        if self._use_legacy_fallback:
            return self._get_legacy_config(domain)

        # Try to find any agent in the graph
        agents = self._graph.agents
        if agents:
            primary = agents[0]
            chain = self._graph.build_chain(primary)
            return AgentChainConfig(
                primary=primary,
                chain=chain if chain else [primary],
                description="Fallback agent",
                handoff_instructions="Unknown domain, using fallback",
            )

        # No agents in graph - return a default
        return AgentChainConfig(
            primary="general-purpose",
            chain=["general-purpose"],
            description="Default fallback",
            handoff_instructions="No agents available",
        )

    @classmethod
    def from_directory(cls, directory: Path) -> GraphRouter:
        """Load a GraphRouter from YAML files in a directory.

        Expects the following structure:
        - skills/*.yaml - Skill definitions
        - personas/*.yaml - Agent definitions
        - domains/*.yaml - Domain definitions
        - rules/*.yaml - Rule definitions (optional)

        Args:
            directory: Path to the directory containing YAML files

        Returns:
            GraphRouter initialized with loaded definitions
        """
        import yaml

        from c4.supervisor.agent_graph.models import (
            AgentDefinition,
            DomainDefinition,
            SkillDefinition,
        )

        graph = AgentGraph()
        rule_engine = RuleEngine()

        # Load skills
        skills_dir = directory / "skills"
        if skills_dir.exists():
            for yaml_file in skills_dir.glob("*.yaml"):
                with open(yaml_file) as f:
                    data = yaml.safe_load(f)
                    if data and "skill" in data:
                        skill = SkillDefinition.model_validate(data)
                        graph.add_skill(skill)

        # Load agents (personas)
        agents_dir = directory / "personas"
        if agents_dir.exists():
            for yaml_file in agents_dir.glob("*.yaml"):
                with open(yaml_file) as f:
                    data = yaml.safe_load(f)
                    if data and "agent" in data:
                        agent = AgentDefinition.model_validate(data)
                        graph.add_agent(agent)

        # Load domains
        domains_dir = directory / "domains"
        if domains_dir.exists():
            for yaml_file in domains_dir.glob("*.yaml"):
                with open(yaml_file) as f:
                    data = yaml.safe_load(f)
                    if data and "domain" in data:
                        domain = DomainDefinition.model_validate(data)
                        graph.add_domain(domain)

        return cls(graph=graph, rule_engine=rule_engine)
