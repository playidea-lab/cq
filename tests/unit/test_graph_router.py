"""Unit tests for GraphRouter - AgentRouter-compatible interface.

Tests cover:
1. GraphRouter initialization (with/without graph)
2. get_recommended_agent (graph mode, fallback mode, edge cases)
3. get_agent_for_task_type (task overrides, domain fallback)
4. get_chain_for_domain
5. get_handoff_instructions
6. get_all_domains
7. Graph-only features (find_agents_for_skill, get_path_between_agents)
8. AgentChainConfig dataclass
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
    Skill,
    SkillDefinition,
    SkillTriggers,
    WorkflowSelect,
    WorkflowStep,
)
from c4.supervisor.agent_graph.router import (
    AgentChainConfig,
    GraphRouter,
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
def sample_graph() -> AgentGraph:
    """Create a sample graph with skills, agents, and domains."""
    graph = AgentGraph()

    # Add skills
    graph.add_skill(
        SkillDefinition(
            skill=Skill(
                id="python-coding",
                name="Python Coding",
                description="Writing Python code",
                capabilities=["python"],
                triggers=SkillTriggers(keywords=["python"]),
            )
        )
    )
    graph.add_skill(
        SkillDefinition(
            skill=Skill(
                id="testing",
                name="Testing",
                description="Writing tests",
                capabilities=["pytest"],
                triggers=SkillTriggers(keywords=["test"]),
            )
        )
    )
    graph.add_skill(
        SkillDefinition(
            skill=Skill(
                id="code-review",
                name="Code Review",
                description="Reviewing code",
                capabilities=["review"],
                triggers=SkillTriggers(keywords=["review"]),
            )
        )
    )

    # Add agents (in reverse order for handoff edges)
    graph.add_agent(
        AgentDefinition(
            agent=Agent(
                id="code-reviewer",
                name="Code Reviewer",
                persona=AgentPersona(role="Reviewer", expertise="Code review"),
                skills=AgentSkills(primary=["code-review"]),
                relationships=AgentRelationships(),
            )
        )
    )
    graph.add_agent(
        AgentDefinition(
            agent=Agent(
                id="test-automator",
                name="Test Automator",
                persona=AgentPersona(role="Tester", expertise="Testing"),
                skills=AgentSkills(primary=["testing"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="code-reviewer",
                            when="Tests complete",
                            passes="Test results",
                            weight=0.8,
                        )
                    ]
                ),
            )
        )
    )
    graph.add_agent(
        AgentDefinition(
            agent=Agent(
                id="backend-dev",
                name="Backend Developer",
                persona=AgentPersona(role="Developer", expertise="Backend"),
                skills=AgentSkills(primary=["python-coding"], secondary=["testing"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="test-automator",
                            when="Implementation done",
                            passes="Code and specs",
                            weight=0.9,
                        )
                    ]
                ),
            )
        )
    )

    # Add domain with workflow
    graph.add_domain(
        DomainDefinition(
            domain=Domain(
                id="web-backend",
                name="Web Backend",
                description="Backend web development",
                required_skills=DomainRequiredSkills(core=["python-coding"]),
                workflow=[
                    WorkflowStep(
                        step=1,
                        role="primary",
                        select=WorkflowSelect(by="agent", prefer_agent="backend-dev"),
                        purpose="Implement backend",
                    )
                ],
            )
        )
    )

    return graph


@pytest.fixture
def router_with_graph(sample_graph: AgentGraph) -> GraphRouter:
    """Create a GraphRouter with a sample graph."""
    return GraphRouter(graph=sample_graph)


@pytest.fixture
def router_no_graph() -> GraphRouter:
    """Create a GraphRouter without a graph (fallback mode)."""
    return GraphRouter()


# ============================================================================
# Test AgentChainConfig Dataclass
# ============================================================================


class TestAgentChainConfig:
    """Tests for AgentChainConfig dataclass."""

    def test_chain_config_basic(self) -> None:
        """AgentChainConfig should store primary and chain."""
        config = AgentChainConfig(primary="backend-dev")
        assert config.primary == "backend-dev"
        assert config.chain == ["backend-dev"]

    def test_chain_config_with_chain(self) -> None:
        """AgentChainConfig should preserve chain if provided."""
        config = AgentChainConfig(
            primary="backend-dev",
            chain=["backend-dev", "test-automator", "code-reviewer"],
        )
        assert config.primary == "backend-dev"
        assert config.chain == ["backend-dev", "test-automator", "code-reviewer"]

    def test_chain_config_adds_primary_to_chain(self) -> None:
        """AgentChainConfig should add primary to chain if missing."""
        config = AgentChainConfig(
            primary="backend-dev",
            chain=["test-automator", "code-reviewer"],
        )
        assert config.chain == ["backend-dev", "test-automator", "code-reviewer"]

    def test_chain_config_with_description(self) -> None:
        """AgentChainConfig should store description and handoff_instructions."""
        config = AgentChainConfig(
            primary="backend-dev",
            description="Backend development chain",
            handoff_instructions="Pass code and specs",
        )
        assert config.description == "Backend development chain"
        assert config.handoff_instructions == "Pass code and specs"


# ============================================================================
# Test GraphRouter Initialization
# ============================================================================


class TestGraphRouterInit:
    """Tests for GraphRouter initialization."""

    def test_init_with_graph(self, sample_graph: AgentGraph) -> None:
        """GraphRouter should store graph reference."""
        router = GraphRouter(graph=sample_graph)
        assert router.graph is sample_graph
        assert router.has_graph is True

    def test_init_without_graph(self) -> None:
        """GraphRouter should work without graph (fallback mode)."""
        router = GraphRouter()
        assert router.graph is None
        assert router.has_graph is False

    def test_init_with_custom_overrides(self) -> None:
        """GraphRouter should merge custom task overrides."""
        router = GraphRouter(task_overrides={"my-task": "my-agent"})
        assert router._task_overrides["my-task"] == "my-agent"
        # Built-in overrides should still exist
        assert router._task_overrides["debug"] == "debugger"

    def test_init_with_custom_default_agent(self) -> None:
        """GraphRouter should use custom default agent."""
        router = GraphRouter(default_agent="custom-agent")
        assert router._default_agent == "custom-agent"


# ============================================================================
# Test get_recommended_agent - Graph Mode
# ============================================================================


class TestGetRecommendedAgentGraphMode:
    """Tests for get_recommended_agent with graph."""

    def test_get_recommended_agent_found(self, router_with_graph: GraphRouter) -> None:
        """get_recommended_agent should return config for known domain."""
        config = router_with_graph.get_recommended_agent("web-backend")
        assert config.primary == "backend-dev"
        assert "backend-dev" in config.chain

    def test_get_recommended_agent_chain_built(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_recommended_agent should build chain from handoffs."""
        config = router_with_graph.get_recommended_agent("web-backend")
        # Chain should follow handoff relationships
        assert config.chain == ["backend-dev", "test-automator", "code-reviewer"]

    def test_get_recommended_agent_none_domain(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_recommended_agent should return default for None domain."""
        config = router_with_graph.get_recommended_agent(None)
        assert config.primary == "general-purpose"

    def test_get_recommended_agent_unknown_domain_fallback(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_recommended_agent should fallback to legacy for unknown domain."""
        config = router_with_graph.get_recommended_agent("some-unknown-domain")
        # Should get default from legacy router
        assert config.primary is not None


# ============================================================================
# Test get_recommended_agent - Fallback Mode
# ============================================================================


class TestGetRecommendedAgentFallbackMode:
    """Tests for get_recommended_agent without graph (fallback)."""

    def test_fallback_known_domain(self, router_no_graph: GraphRouter) -> None:
        """Fallback mode should work for known domains."""
        config = router_no_graph.get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"
        assert "frontend-developer" in config.chain

    def test_fallback_unknown_domain(self, router_no_graph: GraphRouter) -> None:
        """Fallback mode should handle unknown domains."""
        config = router_no_graph.get_recommended_agent("unknown")
        assert config.primary == "general-purpose"


# ============================================================================
# Test get_agent_for_task_type
# ============================================================================


class TestGetAgentForTaskType:
    """Tests for get_agent_for_task_type."""

    def test_task_type_override(self, router_with_graph: GraphRouter) -> None:
        """Task type should override domain default."""
        agent = router_with_graph.get_agent_for_task_type("debug", "web-backend")
        assert agent == "debugger"

    def test_task_type_override_security(self, router_with_graph: GraphRouter) -> None:
        """Security task should get security-auditor."""
        agent = router_with_graph.get_agent_for_task_type("security", "web-frontend")
        assert agent == "security-auditor"

    def test_task_type_none_uses_domain(self, router_with_graph: GraphRouter) -> None:
        """None task type should fall back to domain primary."""
        agent = router_with_graph.get_agent_for_task_type(None, "web-backend")
        assert agent == "backend-dev"

    def test_task_type_case_insensitive(self, router_with_graph: GraphRouter) -> None:
        """Task type matching should be case insensitive."""
        agent = router_with_graph.get_agent_for_task_type("DEBUG", "web-backend")
        assert agent == "debugger"

    def test_task_type_normalizes_underscores(
        self, router_with_graph: GraphRouter
    ) -> None:
        """Task type should normalize underscores to hyphens."""
        agent = router_with_graph.get_agent_for_task_type("fix_bug", "web-backend")
        assert agent == "debugger"


# ============================================================================
# Test get_chain_for_domain
# ============================================================================


class TestGetChainForDomain:
    """Tests for get_chain_for_domain."""

    def test_get_chain_returns_list(self, router_with_graph: GraphRouter) -> None:
        """get_chain_for_domain should return chain list."""
        chain = router_with_graph.get_chain_for_domain("web-backend")
        assert isinstance(chain, list)
        assert len(chain) >= 1
        assert chain[0] == "backend-dev"

    def test_get_chain_fallback(self, router_no_graph: GraphRouter) -> None:
        """get_chain_for_domain should work in fallback mode."""
        chain = router_no_graph.get_chain_for_domain("web-frontend")
        assert "frontend-developer" in chain


# ============================================================================
# Test get_handoff_instructions
# ============================================================================


class TestGetHandoffInstructions:
    """Tests for get_handoff_instructions."""

    def test_get_handoff_instructions(self, router_with_graph: GraphRouter) -> None:
        """get_handoff_instructions should return string."""
        instructions = router_with_graph.get_handoff_instructions("web-backend")
        assert isinstance(instructions, str)

    def test_get_handoff_instructions_fallback(
        self, router_no_graph: GraphRouter
    ) -> None:
        """get_handoff_instructions should work in fallback mode."""
        instructions = router_no_graph.get_handoff_instructions("web-frontend")
        assert isinstance(instructions, str)


# ============================================================================
# Test get_all_domains
# ============================================================================


class TestGetAllDomains:
    """Tests for get_all_domains."""

    def test_get_all_domains_includes_graph_domains(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_all_domains should include graph-defined domains."""
        domains = router_with_graph.get_all_domains()
        assert "web-backend" in domains

    def test_get_all_domains_includes_legacy_domains(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_all_domains should include legacy built-in domains."""
        domains = router_with_graph.get_all_domains()
        # Legacy domains should be included
        assert "web-frontend" in domains
        assert "ml-dl" in domains

    def test_get_all_domains_sorted(self, router_with_graph: GraphRouter) -> None:
        """get_all_domains should return sorted list."""
        domains = router_with_graph.get_all_domains()
        assert domains == sorted(domains)


# ============================================================================
# Test Graph-Only Features
# ============================================================================


class TestGraphOnlyFeatures:
    """Tests for graph-only features."""

    def test_find_agents_for_skill_with_graph(
        self, router_with_graph: GraphRouter
    ) -> None:
        """find_agents_for_skill should return agents with skill."""
        agents = router_with_graph.find_agents_for_skill("python-coding")
        assert "backend-dev" in agents

    def test_find_agents_for_skill_no_graph(self, router_no_graph: GraphRouter) -> None:
        """find_agents_for_skill should return empty without graph."""
        agents = router_no_graph.find_agents_for_skill("python-coding")
        assert agents == []

    def test_get_path_between_agents_with_graph(
        self, router_with_graph: GraphRouter
    ) -> None:
        """get_path_between_agents should find handoff path."""
        path = router_with_graph.get_path_between_agents("backend-dev", "code-reviewer")
        assert path is not None
        assert path[0] == "backend-dev"
        assert path[-1] == "code-reviewer"

    def test_get_path_between_agents_no_graph(
        self, router_no_graph: GraphRouter
    ) -> None:
        """get_path_between_agents should return None without graph."""
        path = router_no_graph.get_path_between_agents("backend-dev", "code-reviewer")
        assert path is None


# ============================================================================
# Test Module-Level Functions
# ============================================================================


class TestModuleFunctions:
    """Tests for module-level convenience functions."""

    def test_get_default_router(self) -> None:
        """get_default_router should return GraphRouter instance."""
        router = get_default_router()
        assert isinstance(router, GraphRouter)

    def test_set_default_router(self, router_with_graph: GraphRouter) -> None:
        """set_default_router should update default router."""
        original = get_default_router()
        try:
            set_default_router(router_with_graph)
            assert get_default_router() is router_with_graph
        finally:
            set_default_router(original)

    def test_get_recommended_agent_function(self) -> None:
        """get_recommended_agent function should work."""
        config = get_recommended_agent("web-frontend")
        assert config.primary is not None

    def test_get_agent_for_task_type_function(self) -> None:
        """get_agent_for_task_type function should work."""
        agent = get_agent_for_task_type("debug")
        assert agent == "debugger"

    def test_get_chain_for_domain_function(self) -> None:
        """get_chain_for_domain function should work."""
        chain = get_chain_for_domain("web-frontend")
        assert isinstance(chain, list)
        assert len(chain) >= 1
