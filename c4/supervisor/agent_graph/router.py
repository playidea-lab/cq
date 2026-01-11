"""GraphRouter - Graph-based agent routing with AgentRouter-compatible interface.

This module provides a GraphRouter class that uses AgentGraph for intelligent
agent selection and chain building, while maintaining compatibility with
the existing AgentRouter interface.

Features:
- Domain-based agent recommendation using graph structure
- Task type overrides for specialized agents
- Agent chain building following HANDS_OFF_TO relationships
- Fallback to legacy AgentRouter when graph is not loaded

Usage:
    # With graph
    graph = AgentGraphLoader().load_directory("path/to/agents")
    router = GraphRouter(graph=graph)
    config = router.get_recommended_agent("web-frontend")

    # Fallback mode (no graph)
    router = GraphRouter()  # Uses legacy AgentRouter internally
    config = router.get_recommended_agent("web-frontend")
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.graph import AgentGraph


@dataclass
class AgentChainConfig:
    """Configuration for agent chain execution.

    Compatible with c4.supervisor.agent_router.AgentChainConfig.

    Attributes:
        primary: The main agent for this domain (first in chain)
        chain: Ordered list of agents to execute sequentially
        description: Human-readable description of this agent combination
        handoff_instructions: Instructions for context passing between agents
    """

    primary: str
    chain: list[str] = field(default_factory=list)
    description: str = ""
    handoff_instructions: str = ""

    def __post_init__(self) -> None:
        """Ensure primary is first in chain if chain is provided."""
        if self.chain and self.primary not in self.chain:
            self.chain = [self.primary] + self.chain
        elif not self.chain:
            self.chain = [self.primary]


# Task type → Agent overrides (same as legacy AgentRouter)
TASK_TYPE_AGENT_OVERRIDES: dict[str, str] = {
    # Debugging tasks
    "debug": "debugger",
    "debugging": "debugger",
    "fix-bug": "debugger",
    # Performance tasks
    "performance": "performance-engineer",
    "optimization": "performance-engineer",
    "profiling": "performance-engineer",
    # Security tasks
    "security": "security-auditor",
    "vulnerability": "security-auditor",
    "audit": "security-auditor",
    # Database tasks
    "database": "database-optimizer",
    "query": "database-optimizer",
    "migration": "database-optimizer",
    # Documentation tasks
    "docs": "api-documenter",
    "documentation": "api-documenter",
    "readme": "api-documenter",
    # Refactoring tasks
    "refactor": "code-refactorer",
    "cleanup": "code-refactorer",
    "restructure": "code-refactorer",
    # Testing tasks
    "test": "test-automator",
    "testing": "test-automator",
    "coverage": "test-automator",
    # DevOps tasks
    "deploy": "deployment-engineer",
    "ci-cd": "deployment-engineer",
    "pipeline": "deployment-engineer",
    # GraphQL tasks
    "graphql": "graphql-architect",
    "schema": "graphql-architect",
    # Payment tasks
    "payment": "payment-integration",
    "stripe": "payment-integration",
    "billing": "payment-integration",
    # Data tasks
    "data-pipeline": "data-engineer",
    "etl": "data-engineer",
    "analytics": "data-scientist",
}


class GraphRouter:
    """Graph-based agent router with AgentRouter-compatible interface.

    Uses AgentGraph for intelligent routing decisions while maintaining
    full compatibility with the existing AgentRouter API.

    The router can operate in two modes:
    1. Graph mode: Uses AgentGraph for domain-based routing
    2. Fallback mode: Delegates to legacy AgentRouter when no graph is provided

    Example:
        >>> from c4.supervisor.agent_graph import AgentGraph, AgentGraphLoader
        >>> graph = AgentGraphLoader().load_directory("agents/")
        >>> router = GraphRouter(graph=graph)
        >>> config = router.get_recommended_agent("web-frontend")
        >>> print(config.primary)  # e.g., "frontend-developer"

        >>> # Fallback mode
        >>> router = GraphRouter()
        >>> config = router.get_recommended_agent("web-frontend")
    """

    def __init__(
        self,
        graph: AgentGraph | None = None,
        task_overrides: dict[str, str] | None = None,
        default_agent: str = "general-purpose",
        max_chain_length: int = 10,
    ) -> None:
        """Initialize the GraphRouter.

        Args:
            graph: AgentGraph instance for graph-based routing.
                   If None, uses legacy AgentRouter fallback.
            task_overrides: Custom task type → agent overrides.
                           Merged with defaults (custom takes precedence).
            default_agent: Fallback agent when no match found.
            max_chain_length: Maximum agents in a chain (default 10).
        """
        self._graph = graph
        self._default_agent = default_agent
        self._max_chain_length = max_chain_length

        # Merge task overrides
        self._task_overrides = dict(TASK_TYPE_AGENT_OVERRIDES)
        if task_overrides:
            self._task_overrides.update(task_overrides)

        # Lazy-loaded legacy router for fallback
        self._legacy_router: Any = None

    @property
    def graph(self) -> AgentGraph | None:
        """Get the underlying AgentGraph."""
        return self._graph

    @property
    def has_graph(self) -> bool:
        """Check if graph-based routing is available."""
        return self._graph is not None

    def _get_legacy_router(self) -> Any:
        """Get or create legacy AgentRouter for fallback."""
        if self._legacy_router is None:
            from c4.supervisor.agent_router import AgentRouter

            self._legacy_router = AgentRouter()
        return self._legacy_router

    def get_recommended_agent(self, domain: str | None) -> AgentChainConfig:
        """Get recommended agent chain configuration for a domain.

        Uses graph-based routing if available, otherwise falls back
        to legacy AgentRouter.

        Args:
            domain: Domain string (e.g., "web-frontend", "web-backend").
                   If None, returns default/unknown config.

        Returns:
            AgentChainConfig with primary agent and full chain.

        Example:
            >>> config = router.get_recommended_agent("web-frontend")
            >>> config.primary
            'frontend-developer'
            >>> config.chain
            ['frontend-developer', 'test-automator', 'code-reviewer']
        """
        if self._graph is None:
            # Fallback to legacy router
            legacy_config = self._get_legacy_router().get_recommended_agent(domain)
            return AgentChainConfig(
                primary=legacy_config.primary,
                chain=legacy_config.chain,
                description=legacy_config.description,
                handoff_instructions=legacy_config.handoff_instructions,
            )

        if domain is None:
            return AgentChainConfig(
                primary=self._default_agent,
                chain=[self._default_agent],
                description="Default agent for unknown domain",
            )

        # Normalize domain string
        domain_str = domain.lower().replace("_", "-")

        # Try to find domain in graph
        domain_info = self._graph.find_agents_for_domain(domain_str)

        if domain_info["primary"]:
            # Found domain with preferred agents
            primary = domain_info["primary"]
            chain = self._graph.build_chain(
                primary,
                max_chain_length=self._max_chain_length,
            )

            # Get domain node for description
            domain_node = self._graph.get_node(domain_str)
            description = ""
            if domain_node:
                description = domain_node.get("description", "")

            return AgentChainConfig(
                primary=primary,
                chain=chain if chain else [primary],
                description=description,
                handoff_instructions=self._get_handoff_instructions(primary),
            )

        # Domain not found in graph - fallback to legacy
        legacy_config = self._get_legacy_router().get_recommended_agent(domain)
        return AgentChainConfig(
            primary=legacy_config.primary,
            chain=legacy_config.chain,
            description=legacy_config.description,
            handoff_instructions=legacy_config.handoff_instructions,
        )

    def get_agent_for_task_type(
        self,
        task_type: str | None,
        domain: str | None = None,
    ) -> str:
        """Get specific agent for a task type, with domain fallback.

        Task type overrides take precedence over domain defaults.
        This allows specialized agents for specific task patterns.

        Args:
            task_type: Type of task (e.g., "debug", "security", "refactor").
            domain: Fallback domain if no task type match.

        Returns:
            Agent name string.

        Example:
            >>> router.get_agent_for_task_type("debug", "web-backend")
            'debugger'
            >>> router.get_agent_for_task_type(None, "web-backend")
            'backend-architect'
        """
        # Check task type overrides first
        if task_type:
            task_type_lower = task_type.lower().replace("_", "-")
            if task_type_lower in self._task_overrides:
                return self._task_overrides[task_type_lower]

        # Fall back to domain primary agent
        config = self.get_recommended_agent(domain)
        return config.primary

    def get_chain_for_domain(self, domain: str | None) -> list[str]:
        """Get just the agent chain list for a domain.

        Args:
            domain: Domain string (e.g., "web-frontend").

        Returns:
            List of agent names in execution order.

        Example:
            >>> router.get_chain_for_domain("web-frontend")
            ['frontend-developer', 'test-automator', 'code-reviewer']
        """
        config = self.get_recommended_agent(domain)
        return config.chain

    def get_handoff_instructions(self, domain: str | None) -> str:
        """Get handoff instructions for a domain's agent chain.

        Args:
            domain: Domain string (e.g., "web-frontend").

        Returns:
            Instructions string for context passing between agents.
        """
        config = self.get_recommended_agent(domain)
        return config.handoff_instructions

    def get_all_domains(self) -> list[str]:
        """Get list of all supported domains.

        Returns both graph-defined domains and legacy built-in domains.
        """
        domains = set()

        # Add graph domains
        if self._graph:
            domains.update(self._graph.domains)

        # Add legacy domains
        legacy_router = self._get_legacy_router()
        domains.update(legacy_router.get_all_domains())

        return sorted(domains)

    def _get_handoff_instructions(self, agent_id: str) -> str:
        """Get handoff instructions from agent's relationships."""
        if not self._graph:
            return ""

        node = self._graph.get_node(agent_id)
        if not node:
            return ""

        # Get from agent definition if available
        definition = node.get("definition")
        if definition and hasattr(definition, "agent"):
            agent = definition.agent
            if agent.relationships and agent.relationships.hands_off_to:
                # Combine all handoff contexts
                instructions = []
                for handoff in agent.relationships.hands_off_to:
                    if handoff.passes:
                        instructions.append(f"Pass: {handoff.passes}")
                return " ".join(instructions)

        return ""

    def find_agents_for_skill(self, skill_id: str) -> list[str]:
        """Find all agents that have a specific skill.

        This is a graph-only feature not available in legacy AgentRouter.

        Args:
            skill_id: The skill ID to search for.

        Returns:
            List of agent IDs that have the skill, or empty list if
            graph not available.
        """
        if not self._graph:
            return []
        return self._graph.find_agents_with_skill(skill_id)

    def get_path_between_agents(
        self,
        from_agent: str,
        to_agent: str,
    ) -> list[str] | None:
        """Find the handoff path between two agents.

        This is a graph-only feature not available in legacy AgentRouter.

        Args:
            from_agent: Source agent ID.
            to_agent: Target agent ID.

        Returns:
            List of agent IDs forming the path, or None if no path
            exists or graph not available.
        """
        if not self._graph:
            return None
        return self._graph.get_path(from_agent, to_agent)


# =============================================================================
# Module-level convenience functions
# =============================================================================

_default_router: GraphRouter | None = None


def get_default_router() -> GraphRouter:
    """Get or create the default GraphRouter.

    Returns a router in fallback mode (no graph loaded).
    Use set_default_router() to configure with a graph.
    """
    global _default_router
    if _default_router is None:
        _default_router = GraphRouter()
    return _default_router


def set_default_router(router: GraphRouter) -> None:
    """Set a custom router as the default."""
    global _default_router
    _default_router = router


def get_recommended_agent(domain: str | None) -> AgentChainConfig:
    """Get recommended agent configuration using default router."""
    return get_default_router().get_recommended_agent(domain)


def get_agent_for_task_type(
    task_type: str | None,
    domain: str | None = None,
) -> str:
    """Get agent for task type using default router."""
    return get_default_router().get_agent_for_task_type(task_type, domain)


def get_chain_for_domain(domain: str | None) -> list[str]:
    """Get agent chain for domain using default router."""
    return get_default_router().get_chain_for_domain(domain)
