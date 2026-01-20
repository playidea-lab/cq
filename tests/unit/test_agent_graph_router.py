"""Unit tests for GraphRouter - Graph-based agent routing.

Tests cover:
1. AgentChainConfig initialization
2. GraphRouter initialization and properties
3. get_recommended_agent() with graph mode
4. get_recommended_agent() fallback mode
5. get_agent_for_task_type() with overrides
6. Module-level convenience functions
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph.graph import AgentGraph
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentHandsOffTo,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    Domain,
    DomainDefinition,
    DomainRequiredSkills,
    WorkflowSelect,
    WorkflowStep,
)
from c4.supervisor.agent_graph.router import GraphRouter
from c4.supervisor.agent_router import (
    TASK_TYPE_AGENT_OVERRIDES,
    AgentChainConfig,
    get_agent_for_task_type,
    get_chain_for_domain,
    get_default_router,
    get_recommended_agent,
    set_default_router,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def graph_with_domain() -> AgentGraph:
    """Create a graph with a domain and agents."""
    graph = AgentGraph()

    # Create agents
    backend = AgentDefinition(
        agent=Agent(
            id="backend-architect",
            name="Backend Architect",
            persona=AgentPersona(role="Architect", expertise="Backend systems"),
            skills=AgentSkills(primary=["api-design"]),
            relationships=AgentRelationships(
                hands_off_to=[
                    AgentHandsOffTo(
                        agent="code-reviewer",
                        when="Implementation done",
                        passes="Code and tests",
                        weight=0.9,
                    )
                ]
            ),
        )
    )

    reviewer = AgentDefinition(
        agent=Agent(
            id="code-reviewer",
            name="Code Reviewer",
            persona=AgentPersona(role="Reviewer", expertise="Code quality"),
            skills=AgentSkills(primary=["code-review"]),
            relationships=AgentRelationships(),
        )
    )

    # Create domain
    domain = DomainDefinition(
        domain=Domain(
            id="web-backend",
            name="Web Backend",
            description="Backend web development",
            required_skills=DomainRequiredSkills(core=["api-design"]),
            workflow=[
                WorkflowStep(
                    step=1,
                    role="primary",
                    select=WorkflowSelect(by="agent", prefer_agent="backend-architect"),
                    purpose="Design and implement backend",
                ),
            ],
        )
    )

    # Add to graph
    graph.add_agent(reviewer)
    graph.add_agent(backend)
    graph.add_domain(domain)

    return graph


@pytest.fixture
def router_with_graph(graph_with_domain: AgentGraph) -> GraphRouter:
    """Create a GraphRouter with a loaded graph."""
    return GraphRouter(graph=graph_with_domain)


@pytest.fixture
def router_fallback() -> GraphRouter:
    """Create a GraphRouter in fallback mode (no graph)."""
    return GraphRouter()


# ============================================================================
# Test AgentChainConfig
# ============================================================================


class TestAgentChainConfig:
    """Tests for AgentChainConfig dataclass."""

    def test_chain_config_basic(self) -> None:
        """AgentChainConfig should initialize with primary and chain."""
        config = AgentChainConfig(
            primary="backend-architect",
            chain=["backend-architect", "code-reviewer"],
            description="Backend chain",
        )

        assert config.primary == "backend-architect"
        assert config.chain == ["backend-architect", "code-reviewer"]
        assert config.description == "Backend chain"

    def test_chain_config_adds_primary_to_chain(self) -> None:
        """AgentChainConfig should add primary to chain if missing."""
        config = AgentChainConfig(
            primary="backend-architect",
            chain=["code-reviewer"],  # primary not in chain
        )

        assert config.chain == ["backend-architect", "code-reviewer"]
        assert config.chain[0] == config.primary

    def test_chain_config_empty_chain_uses_primary(self) -> None:
        """AgentChainConfig should use primary as chain if chain is empty."""
        config = AgentChainConfig(primary="backend-architect")

        assert config.chain == ["backend-architect"]

    def test_chain_config_keeps_primary_in_chain(self) -> None:
        """AgentChainConfig should keep primary in chain if already there."""
        config = AgentChainConfig(
            primary="backend-architect",
            chain=["backend-architect", "code-reviewer"],
        )

        # Should not duplicate primary
        assert config.chain == ["backend-architect", "code-reviewer"]


# ============================================================================
# Test GraphRouter Initialization
# ============================================================================


class TestGraphRouterInit:
    """Tests for GraphRouter initialization."""

    def test_init_with_graph(self, graph_with_domain: AgentGraph) -> None:
        """GraphRouter should initialize with a graph."""
        router = GraphRouter(graph=graph_with_domain)

        assert router.graph is graph_with_domain
        assert router.has_graph is True

    def test_init_without_graph(self) -> None:
        """GraphRouter should initialize in fallback mode without graph."""
        router = GraphRouter()

        assert router.graph is None
        assert router.has_graph is False

    def test_init_custom_task_overrides(self) -> None:
        """GraphRouter should accept custom task overrides."""
        custom = {"my-task": "my-agent"}
        router = GraphRouter(task_overrides=custom)

        # Custom override should be present
        assert "my-task" in router._task_overrides
        assert router._task_overrides["my-task"] == "my-agent"

        # Default overrides should still be present
        assert "debug" in router._task_overrides

    def test_init_default_agent(self) -> None:
        """GraphRouter should accept custom default agent."""
        router = GraphRouter(default_agent="my-default")

        assert router._default_agent == "my-default"


# ============================================================================
# Test get_recommended_agent() - Graph Mode
# ============================================================================


class TestGetRecommendedAgentGraph:
    """Tests for get_recommended_agent() with graph."""

    def test_get_recommended_agent_finds_domain(self, router_with_graph: GraphRouter) -> None:
        """get_recommended_agent should return correct agent for known domain."""
        config = router_with_graph.get_recommended_agent("web-backend")

        assert config.primary == "backend-architect"
        assert "backend-architect" in config.chain

    def test_get_recommended_agent_builds_chain(self, router_with_graph: GraphRouter) -> None:
        """get_recommended_agent should build agent chain from graph."""
        config = router_with_graph.get_recommended_agent("web-backend")

        # Chain should follow handoffs
        assert config.chain == ["backend-architect", "code-reviewer"]

    def test_get_recommended_agent_none_domain(self, router_with_graph: GraphRouter) -> None:
        """get_recommended_agent should return default for None domain."""
        config = router_with_graph.get_recommended_agent(None)

        assert config.primary == "general-purpose"
        assert config.chain == ["general-purpose"]

    def test_get_recommended_agent_normalizes_domain(self, router_with_graph: GraphRouter) -> None:
        """get_recommended_agent should normalize domain strings."""
        # Test with underscores (should convert to dashes)
        config = router_with_graph.get_recommended_agent("web_backend")

        assert config.primary == "backend-architect"


# ============================================================================
# Test get_recommended_agent() - Fallback Mode
# ============================================================================


class TestGetRecommendedAgentFallback:
    """Tests for get_recommended_agent() in fallback mode."""

    def test_fallback_uses_legacy_router(self, router_fallback: GraphRouter) -> None:
        """get_recommended_agent should use legacy router when no graph."""
        config = router_fallback.get_recommended_agent("web-backend")

        # Should return something from legacy router
        assert config.primary is not None
        assert len(config.chain) > 0

    def test_fallback_unknown_domain(self, router_fallback: GraphRouter) -> None:
        """get_recommended_agent should handle unknown domain in fallback."""
        config = router_fallback.get_recommended_agent("unknown-domain-xyz")

        # Should return some default from legacy router
        assert config.primary is not None


# ============================================================================
# Test get_agent_for_task_type()
# ============================================================================


class TestGetAgentForTaskType:
    """Tests for get_agent_for_task_type()."""

    def test_task_type_override(self, router_with_graph: GraphRouter) -> None:
        """get_agent_for_task_type should use task type overrides."""
        agent = router_with_graph.get_agent_for_task_type("debug")

        assert agent == "debugger"

    def test_task_type_normalizes(self, router_with_graph: GraphRouter) -> None:
        """get_agent_for_task_type should normalize task types."""
        # Underscores and case should be handled
        agent = router_with_graph.get_agent_for_task_type("FIX_BUG")

        assert agent == "debugger"

    def test_task_type_with_domain_fallback(self, router_with_graph: GraphRouter) -> None:
        """get_agent_for_task_type should use domain when no task match."""
        agent = router_with_graph.get_agent_for_task_type(
            "unknown-task",
            domain="web-backend",
        )

        assert agent == "backend-architect"

    def test_task_type_none_uses_domain(self, router_with_graph: GraphRouter) -> None:
        """get_agent_for_task_type should use domain when task_type is None."""
        agent = router_with_graph.get_agent_for_task_type(None, domain="web-backend")

        assert agent == "backend-architect"


# ============================================================================
# Test Other GraphRouter Methods
# ============================================================================


class TestGraphRouterMethods:
    """Tests for other GraphRouter methods."""

    def test_get_chain_for_domain(self, router_with_graph: GraphRouter) -> None:
        """get_chain_for_domain should return just the chain list."""
        chain = router_with_graph.get_chain_for_domain("web-backend")

        assert chain == ["backend-architect", "code-reviewer"]

    def test_get_handoff_instructions(self, router_with_graph: GraphRouter) -> None:
        """get_handoff_instructions should return instructions string."""
        instructions = router_with_graph.get_handoff_instructions("web-backend")

        # Should return a string (may be empty)
        assert isinstance(instructions, str)

    def test_get_all_domains(self, router_with_graph: GraphRouter) -> None:
        """get_all_domains should return all domains."""
        domains = router_with_graph.get_all_domains()

        assert "web-backend" in domains
        assert isinstance(domains, list)

    def test_find_agents_for_skill_with_graph(self, router_with_graph: GraphRouter) -> None:
        """find_agents_for_skill should use graph when available."""
        # Need to add skill to graph first
        agents = router_with_graph.find_agents_for_skill("api-design")

        # May be empty if skill not fully linked in graph
        assert isinstance(agents, list)

    def test_find_agents_for_skill_no_graph(self, router_fallback: GraphRouter) -> None:
        """find_agents_for_skill should return empty without graph."""
        agents = router_fallback.find_agents_for_skill("any-skill")

        assert agents == []

    def test_get_path_between_agents_with_graph(self, router_with_graph: GraphRouter) -> None:
        """get_path_between_agents should use graph when available."""
        path = router_with_graph.get_path_between_agents("backend-architect", "code-reviewer")

        assert path == ["backend-architect", "code-reviewer"]

    def test_get_path_between_agents_no_graph(self, router_fallback: GraphRouter) -> None:
        """get_path_between_agents should return None without graph."""
        path = router_fallback.get_path_between_agents("a", "b")

        assert path is None


# ============================================================================
# Test Module-Level Functions
# ============================================================================


class TestModuleFunctions:
    """Tests for module-level convenience functions."""

    def test_get_default_router(self) -> None:
        """get_default_router should return a GraphRouter."""
        router = get_default_router()

        assert isinstance(router, GraphRouter)

    def test_set_default_router(self) -> None:
        """set_default_router should set custom router as default."""
        custom_router = GraphRouter(default_agent="custom-agent")
        set_default_router(custom_router)

        router = get_default_router()
        assert router._default_agent == "custom-agent"

        # Reset for other tests
        set_default_router(GraphRouter())

    def test_get_recommended_agent_function(self) -> None:
        """get_recommended_agent() function should use default router."""
        config = get_recommended_agent("web-backend")

        assert config.primary is not None

    def test_get_agent_for_task_type_function(self) -> None:
        """get_agent_for_task_type() function should use default router."""
        agent = get_agent_for_task_type("debug")

        assert agent == "debugger"

    def test_get_chain_for_domain_function(self) -> None:
        """get_chain_for_domain() function should use default router."""
        chain = get_chain_for_domain("web-backend")

        assert isinstance(chain, list)
        assert len(chain) > 0


# ============================================================================
# Test Task Type Overrides
# ============================================================================


class TestTaskTypeOverrides:
    """Tests for TASK_TYPE_AGENT_OVERRIDES constant."""

    def test_overrides_has_debug_tasks(self) -> None:
        """TASK_TYPE_AGENT_OVERRIDES should have debug tasks."""
        assert TASK_TYPE_AGENT_OVERRIDES["debug"] == "debugger"
        assert TASK_TYPE_AGENT_OVERRIDES["debugging"] == "debugger"
        assert TASK_TYPE_AGENT_OVERRIDES["fix-bug"] == "debugger"

    def test_overrides_has_security_tasks(self) -> None:
        """TASK_TYPE_AGENT_OVERRIDES should have security tasks."""
        assert TASK_TYPE_AGENT_OVERRIDES["security"] == "security-auditor"
        assert TASK_TYPE_AGENT_OVERRIDES["vulnerability"] == "security-auditor"

    def test_overrides_has_documentation_tasks(self) -> None:
        """TASK_TYPE_AGENT_OVERRIDES should have documentation tasks."""
        assert TASK_TYPE_AGENT_OVERRIDES["docs"] == "api-documenter"
        assert TASK_TYPE_AGENT_OVERRIDES["documentation"] == "api-documenter"

    def test_overrides_has_testing_tasks(self) -> None:
        """TASK_TYPE_AGENT_OVERRIDES should have testing tasks."""
        assert TASK_TYPE_AGENT_OVERRIDES["test"] == "test-automator"
        assert TASK_TYPE_AGENT_OVERRIDES["testing"] == "test-automator"

    def test_overrides_has_refactoring_tasks(self) -> None:
        """TASK_TYPE_AGENT_OVERRIDES should have refactoring tasks."""
        assert TASK_TYPE_AGENT_OVERRIDES["refactor"] == "code-refactorer"
        assert TASK_TYPE_AGENT_OVERRIDES["cleanup"] == "code-refactorer"
