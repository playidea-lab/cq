"""Unit tests for GraphRouter - AgentRouter-compatible graph-based router.

Tests cover:
1. Basic interface compatibility with AgentRouter
2. Graph-based routing using AgentGraph
3. Rule-based routing using RuleEngine
4. Fallback behavior for unknown domains
"""

from __future__ import annotations

from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from c4.supervisor.agent_graph.graph import AgentGraph
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentHandsOffTo,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    ChainExtension,
    ChainExtensionAction,
    Condition,
    Domain,
    DomainDefinition,
    DomainRequiredSkills,
    Override,
    OverrideAction,
    WorkflowSelect,
    WorkflowStep,
)
from c4.supervisor.agent_graph.router import GraphRouter
from c4.supervisor.agent_graph.rules import RuleEngine


# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def sample_graph() -> AgentGraph:
    """Create a sample agent graph with skills, agents, and domains."""
    graph = AgentGraph()

    # Add agents (in reverse order for handoff edges)
    agents = [
        AgentDefinition(
            agent=Agent(
                id="code-reviewer",
                name="Code Reviewer",
                persona=AgentPersona(role="Reviewer", expertise="Code review"),
                skills=AgentSkills(primary=["code-review"]),
                relationships=AgentRelationships(),
            )
        ),
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
                            when="Tests written",
                            passes="Test results",
                            weight=0.8,
                        )
                    ]
                ),
            )
        ),
        AgentDefinition(
            agent=Agent(
                id="backend-dev",
                name="Backend Developer",
                persona=AgentPersona(role="Backend Dev", expertise="Backend"),
                skills=AgentSkills(primary=["python-coding"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="test-automator",
                            when="Code complete",
                            passes="Implementation",
                            weight=0.9,
                        )
                    ]
                ),
            )
        ),
        AgentDefinition(
            agent=Agent(
                id="frontend-dev",
                name="Frontend Developer",
                persona=AgentPersona(role="Frontend Dev", expertise="Frontend"),
                skills=AgentSkills(primary=["typescript-coding"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="test-automator",
                            when="UI complete",
                            passes="Components",
                            weight=0.85,
                        )
                    ]
                ),
            )
        ),
        AgentDefinition(
            agent=Agent(
                id="debugger",
                name="Debugger",
                persona=AgentPersona(role="Debugger", expertise="Debugging"),
                skills=AgentSkills(primary=["debugging"]),
                relationships=AgentRelationships(),
            )
        ),
    ]

    for agent in agents:
        graph.add_agent(agent)

    # Add domains
    domains = [
        DomainDefinition(
            domain=Domain(
                id="web-backend",
                name="Web Backend",
                description="Backend development",
                required_skills=DomainRequiredSkills(core=["python-coding"]),
                workflow=[
                    WorkflowStep(
                        step=1,
                        role="primary",
                        select=WorkflowSelect(by="agent", prefer_agent="backend-dev"),
                        purpose="Implement backend",
                    ),
                    WorkflowStep(
                        step=2,
                        role="quality",
                        select=WorkflowSelect(by="agent", prefer_agent="code-reviewer"),
                        purpose="Review code",
                    ),
                ],
            )
        ),
        DomainDefinition(
            domain=Domain(
                id="web-frontend",
                name="Web Frontend",
                description="Frontend development",
                required_skills=DomainRequiredSkills(core=["typescript-coding"]),
                workflow=[
                    WorkflowStep(
                        step=1,
                        role="primary",
                        select=WorkflowSelect(by="agent", prefer_agent="frontend-dev"),
                        purpose="Implement frontend",
                    ),
                ],
            )
        ),
    ]

    for domain in domains:
        graph.add_domain(domain)

    return graph


@pytest.fixture
def sample_rule_engine() -> RuleEngine:
    """Create a sample rule engine with overrides and extensions."""
    engine = RuleEngine()

    # Add override for debug tasks
    engine.add_override(
        Override(
            name="debug-override",
            priority=90,
            condition=Condition(has_keyword=["debug", "fix bug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debug tasks use debugger",
        )
    )

    # Add chain extension for security-related tasks
    engine.add_chain_extension(
        ChainExtension(
            name="test-extension",
            condition=Condition(domain="web-backend"),
            action=ChainExtensionAction(add_to_chain="test-automator", position="last"),
        )
    )

    return engine


@pytest.fixture
def router(sample_graph: AgentGraph, sample_rule_engine: RuleEngine) -> GraphRouter:
    """Create a GraphRouter with sample graph and rules."""
    return GraphRouter(graph=sample_graph, rule_engine=sample_rule_engine)


# ============================================================================
# Test GraphRouter Initialization
# ============================================================================


class TestGraphRouterInit:
    """Tests for GraphRouter initialization."""

    def test_init_with_graph_and_engine(
        self, sample_graph: AgentGraph, sample_rule_engine: RuleEngine
    ) -> None:
        """GraphRouter can be initialized with graph and rule engine."""
        router = GraphRouter(graph=sample_graph, rule_engine=sample_rule_engine)
        assert router.graph is sample_graph
        assert router.rule_engine is sample_rule_engine

    def test_init_with_graph_only(self, sample_graph: AgentGraph) -> None:
        """GraphRouter can be initialized with just a graph."""
        router = GraphRouter(graph=sample_graph)
        assert router.graph is sample_graph
        assert router.rule_engine is not None  # Creates default engine

    def test_init_empty(self) -> None:
        """GraphRouter can be initialized empty."""
        router = GraphRouter()
        assert router.graph is not None  # Creates empty graph
        assert router.rule_engine is not None  # Creates empty engine


# ============================================================================
# Test get_recommended_agent()
# ============================================================================


class TestGetRecommendedAgent:
    """Tests for GraphRouter.get_recommended_agent()."""

    def test_get_recommended_agent_known_domain(self, router: GraphRouter) -> None:
        """get_recommended_agent returns config for known domain."""
        config = router.get_recommended_agent("web-backend")

        assert config.primary == "backend-dev"
        assert "backend-dev" in config.chain

    def test_get_recommended_agent_builds_chain(self, router: GraphRouter) -> None:
        """get_recommended_agent builds chain from graph handoffs."""
        config = router.get_recommended_agent("web-backend")

        # Chain should follow handoff edges
        # backend-dev -> test-automator -> code-reviewer
        assert config.chain[0] == "backend-dev"
        # test-automator should be in chain (from extension or handoff)
        assert "test-automator" in config.chain

    def test_get_recommended_agent_unknown_domain(self, router: GraphRouter) -> None:
        """get_recommended_agent returns fallback for unknown domain."""
        config = router.get_recommended_agent("unknown-domain")

        # Should return some fallback
        assert config.primary is not None
        assert len(config.chain) >= 1

    def test_get_recommended_agent_none_domain(self, router: GraphRouter) -> None:
        """get_recommended_agent handles None domain."""
        config = router.get_recommended_agent(None)

        assert config.primary is not None
        assert len(config.chain) >= 1


# ============================================================================
# Test get_agent_for_task_type()
# ============================================================================


class TestGetAgentForTaskType:
    """Tests for GraphRouter.get_agent_for_task_type()."""

    def test_get_agent_for_task_type_with_override(self, router: GraphRouter) -> None:
        """get_agent_for_task_type returns override agent for matching task."""
        # The rule engine has override for "debug" keyword
        agent = router.get_agent_for_task_type(
            task_type=None,
            domain="web-backend",
            title="Debug login issue",
        )

        assert agent == "debugger"

    def test_get_agent_for_task_type_no_override(self, router: GraphRouter) -> None:
        """get_agent_for_task_type returns domain primary when no override."""
        agent = router.get_agent_for_task_type(
            task_type=None,
            domain="web-backend",
            title="Add new API endpoint",
        )

        assert agent == "backend-dev"

    def test_get_agent_for_task_type_uses_task_type(self, router: GraphRouter) -> None:
        """get_agent_for_task_type can use task_type for matching."""
        # Without special task_type override, falls back to domain
        agent = router.get_agent_for_task_type(
            task_type="feature",
            domain="web-frontend",
        )

        assert agent == "frontend-dev"


# ============================================================================
# Test get_chain_for_domain()
# ============================================================================


class TestGetChainForDomain:
    """Tests for GraphRouter.get_chain_for_domain()."""

    def test_get_chain_for_domain_returns_list(self, router: GraphRouter) -> None:
        """get_chain_for_domain returns list of agent IDs."""
        chain = router.get_chain_for_domain("web-backend")

        assert isinstance(chain, list)
        assert len(chain) >= 1
        assert all(isinstance(a, str) for a in chain)

    def test_get_chain_for_domain_primary_first(self, router: GraphRouter) -> None:
        """get_chain_for_domain puts primary agent first."""
        chain = router.get_chain_for_domain("web-backend")

        assert chain[0] == "backend-dev"

    def test_get_chain_for_domain_unknown(self, router: GraphRouter) -> None:
        """get_chain_for_domain returns fallback for unknown domain."""
        chain = router.get_chain_for_domain("nonexistent")

        assert isinstance(chain, list)
        assert len(chain) >= 1


# ============================================================================
# Test Rule Integration
# ============================================================================


class TestRuleIntegration:
    """Tests for rule-based routing integration."""

    def test_chain_extension_applied(self, router: GraphRouter) -> None:
        """Chain extensions are applied to domain chains."""
        # The rule engine has extension adding test-automator for web-backend
        config = router.get_recommended_agent("web-backend")

        assert "test-automator" in config.chain

    def test_override_takes_precedence(self, router: GraphRouter) -> None:
        """Override rules take precedence over graph routing."""
        # Debug keyword should trigger debugger override
        agent = router.get_agent_for_task_type(
            task_type=None,
            domain="web-backend",
            title="Fix bug in authentication",  # Contains "fix bug"
        )

        assert agent == "debugger"


# ============================================================================
# Test AgentChainConfig Compatibility
# ============================================================================


class TestAgentChainConfigCompat:
    """Tests for AgentChainConfig compatibility."""

    def test_config_has_required_attributes(self, router: GraphRouter) -> None:
        """Returned config has all required attributes."""
        config = router.get_recommended_agent("web-backend")

        assert hasattr(config, "primary")
        assert hasattr(config, "chain")
        assert hasattr(config, "description")
        assert hasattr(config, "handoff_instructions")

    def test_config_primary_in_chain(self, router: GraphRouter) -> None:
        """Config primary is always in chain."""
        config = router.get_recommended_agent("web-backend")

        assert config.primary in config.chain


# ============================================================================
# Test Loading from Directory
# ============================================================================


class TestLoadFromDirectory:
    """Tests for GraphRouter.from_directory() class method."""

    def test_from_directory_loads_yaml(self) -> None:
        """from_directory loads YAML files from directory."""
        with TemporaryDirectory() as tmpdir:
            tmppath = Path(tmpdir)

            # Create minimal skill YAML
            skills_dir = tmppath / "skills"
            skills_dir.mkdir()
            (skills_dir / "python-coding.yaml").write_text("""
skill:
  id: python-coding
  name: Python Coding
  description: Write Python code
  capabilities:
    - Write Python
  triggers:
    keywords:
      - python
""")

            # Create minimal agent YAML
            agents_dir = tmppath / "personas"
            agents_dir.mkdir()
            (agents_dir / "backend-dev.yaml").write_text("""
agent:
  id: backend-dev
  name: Backend Developer
  persona:
    role: Developer
    expertise: Backend
  skills:
    primary:
      - python-coding
  relationships: {}
""")

            # Create minimal domain YAML
            domains_dir = tmppath / "domains"
            domains_dir.mkdir()
            (domains_dir / "web-backend.yaml").write_text("""
domain:
  id: web-backend
  name: Web Backend
  description: Backend development
  required_skills:
    core:
      - python-coding
  workflow:
    - step: 1
      role: primary
      select:
        by: agent
        prefer_agent: backend-dev
      purpose: Implement backend
""")

            router = GraphRouter.from_directory(tmppath)

            # Should load and be functional
            config = router.get_recommended_agent("web-backend")
            assert config.primary == "backend-dev"


# ============================================================================
# Test Legacy Fallback Support
# ============================================================================


class TestLegacyFallback:
    """Tests for GraphRouter legacy fallback to DOMAIN_AGENT_MAP."""

    def test_empty_graph_uses_legacy(self) -> None:
        """Empty graph uses DOMAIN_AGENT_MAP fallback."""
        router = GraphRouter()  # Empty graph

        # Should use legacy web-frontend config
        config = router.get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"
        assert "frontend-developer" in config.chain

    def test_empty_graph_uses_legacy_task_overrides(self) -> None:
        """Empty graph uses TASK_TYPE_AGENT_OVERRIDES fallback."""
        router = GraphRouter()  # Empty graph

        # Should use legacy debug override
        agent = router.get_agent_for_task_type("debug", "web-backend")
        assert agent == "debugger"

    def test_fallback_disabled_returns_default(self) -> None:
        """With fallback disabled, returns default agent."""
        router = GraphRouter(use_legacy_fallback=False)

        # Should not use legacy, returns default
        config = router.get_recommended_agent("web-frontend")
        assert config.primary == "general-purpose"

    def test_graph_with_domains_no_fallback(
        self, sample_graph: AgentGraph, sample_rule_engine: RuleEngine
    ) -> None:
        """Graph with domains doesn't use legacy fallback."""
        router = GraphRouter(graph=sample_graph, rule_engine=sample_rule_engine)

        # Should use graph domain, not legacy
        config = router.get_recommended_agent("web-backend")
        assert config.primary == "backend-dev"

    def test_get_all_domains_includes_legacy(self) -> None:
        """get_all_domains includes legacy domains when fallback enabled."""
        router = GraphRouter()  # Empty graph

        domains = router.get_all_domains()
        assert "web-frontend" in domains
        assert "web-backend" in domains
        assert "ml-dl" in domains

    def test_legacy_unknown_domain_fallback(self) -> None:
        """Unknown domain falls back to 'unknown' legacy config."""
        router = GraphRouter()  # Empty graph

        config = router.get_recommended_agent("nonexistent-domain")
        assert config.primary == "general-purpose"

    def test_legacy_none_domain_fallback(self) -> None:
        """None domain falls back to 'unknown' legacy config."""
        router = GraphRouter()  # Empty graph

        config = router.get_recommended_agent(None)
        assert config.primary == "general-purpose"

    def test_legacy_handoff_instructions(self) -> None:
        """Handoff instructions are preserved from legacy."""
        router = GraphRouter()  # Empty graph

        instructions = router.get_handoff_instructions("web-frontend")
        # Legacy has handoff instructions for web-frontend
        assert len(instructions) > 0
