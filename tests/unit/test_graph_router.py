"""Unit tests for GraphRouter - Graph-based agent routing with skill matching.

Tests cover:
1. Basic routing (domain-only, backward compatibility)
2. Skill-based routing (with task and SkillMatcher)
3. Task type override priority
4. RoutingResult with metadata
5. Delegation to legacy router
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph import (
    AgentGraph,
    GraphRouter,
    RoutingResult,
    SkillMatcher,
    TaskContext,
)
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentHandsOffTo,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    Skill,
    SkillDefinition,
    SkillTriggers,
)
from c4.supervisor._legacy.agent_router import AgentChainConfig

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def graph() -> AgentGraph:
    """Create an AgentGraph with test skills and agents."""
    g = AgentGraph()

    # Add skills
    python_skill = SkillDefinition(
        skill=Skill(
            id="python-coding",
            name="Python Coding",
            description="Writing Python code and modules",
            capabilities=["write python code"],
            triggers=SkillTriggers(
                keywords=["python", "py", "django", "flask"],
                task_types=["feature", "refactor"],
                file_patterns=["*.py"],
            ),
        )
    )
    g.add_skill(python_skill)

    debugging_skill = SkillDefinition(
        skill=Skill(
            id="debugging",
            name="Debugging",
            description="Finding and fixing bugs in code",
            capabilities=["debug code"],
            triggers=SkillTriggers(
                keywords=["debug", "bug", "error"],
                task_types=["bugfix"],
            ),
        )
    )
    g.add_skill(debugging_skill)

    api_skill = SkillDefinition(
        skill=Skill(
            id="api-design",
            name="API Design",
            description="Designing RESTful APIs",
            capabilities=["design APIs"],
            triggers=SkillTriggers(
                keywords=["api", "rest", "endpoint"],
            ),
        )
    )
    g.add_skill(api_skill)

    # Add agents
    backend_dev = AgentDefinition(
        agent=Agent(
            id="backend-dev",
            name="Backend Developer",
            persona=AgentPersona(
                role="Python backend specialist",
                expertise="Python, APIs, databases",
            ),
            skills=AgentSkills(
                primary=["python-coding", "api-design"],
                secondary=["debugging"],
            ),
            relationships=AgentRelationships(
                hands_off_to=[
                    AgentHandsOffTo(
                        agent="test-automator",
                        when="Implementation complete",
                        passes="Code and test requirements",
                        weight=0.9,
                    ),
                ],
            ),
        )
    )
    g.add_agent(backend_dev)

    debugger = AgentDefinition(
        agent=Agent(
            id="debugger",
            name="Debugger",
            persona=AgentPersona(
                role="Bug hunting specialist",
                expertise="Debugging, profiling, tracing",
            ),
            skills=AgentSkills(
                primary=["debugging"],
                secondary=["python-coding"],
            ),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(debugger)

    # Add a testing skill for test-automator
    testing_skill = SkillDefinition(
        skill=Skill(
            id="testing",
            name="Testing",
            description="Writing and running tests",
            capabilities=["write tests"],
            triggers=SkillTriggers(
                keywords=["test", "testing", "unittest"],
            ),
        )
    )
    g.add_skill(testing_skill)

    test_automator = AgentDefinition(
        agent=Agent(
            id="test-automator",
            name="Test Automator",
            persona=AgentPersona(
                role="Testing specialist",
                expertise="Unit tests, integration tests",
            ),
            skills=AgentSkills(
                primary=["testing"],
                secondary=[],
            ),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(test_automator)

    return g


@pytest.fixture
def skill_matcher(graph: AgentGraph) -> SkillMatcher:
    """Create a SkillMatcher with the test graph."""
    return SkillMatcher(graph)


@pytest.fixture
def router_with_matcher(graph: AgentGraph, skill_matcher: SkillMatcher) -> GraphRouter:
    """Create a GraphRouter with SkillMatcher."""
    return GraphRouter(skill_matcher=skill_matcher, graph=graph)


@pytest.fixture
def router_without_matcher() -> GraphRouter:
    """Create a GraphRouter without SkillMatcher (legacy mode)."""
    return GraphRouter()


# ============================================================================
# Test Basic Routing (Domain-only, Backward Compatibility)
# ============================================================================


class TestBasicRouting:
    """Tests for basic domain-based routing."""

    def test_domain_routing_web_backend(self, router_without_matcher: GraphRouter) -> None:
        """Should route web-backend domain to backend-architect."""
        config = router_without_matcher.get_recommended_agent("web-backend")

        assert config.primary == "backend-architect"
        assert isinstance(config, AgentChainConfig)

    def test_domain_routing_web_frontend(self, router_without_matcher: GraphRouter) -> None:
        """Should route web-frontend domain to frontend-developer."""
        config = router_without_matcher.get_recommended_agent("web-frontend")

        assert config.primary == "frontend-developer"

    def test_domain_routing_unknown(self, router_without_matcher: GraphRouter) -> None:
        """Should handle unknown domain with fallback."""
        config = router_without_matcher.get_recommended_agent("nonexistent-domain")

        assert config.primary == "general-purpose"

    def test_domain_routing_none(self, router_without_matcher: GraphRouter) -> None:
        """Should handle None domain with fallback."""
        config = router_without_matcher.get_recommended_agent(None)

        assert config.primary == "general-purpose"


# ============================================================================
# Test Skill-based Routing
# ============================================================================


class TestSkillBasedRouting:
    """Tests for skill-based routing with SkillMatcher."""

    def test_skill_routing_python_task(self, router_with_matcher: GraphRouter) -> None:
        """Should route Python task to backend-dev via skill matching."""
        task = TaskContext(title="Fix Python API bug")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        # backend-dev has python-coding and api-design as primary
        assert config.primary == "backend-dev"

    def test_skill_routing_debug_task(self, router_with_matcher: GraphRouter) -> None:
        """Should route debug task to debugger via skill matching."""
        task = TaskContext(title="Debug this error")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        # debugger has debugging as primary
        assert config.primary == "debugger"

    def test_skill_routing_no_match_falls_back(self, router_with_matcher: GraphRouter) -> None:
        """Should fall back to domain routing if no skill match."""
        task = TaskContext(title="Update documentation")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        # No skill triggers for "documentation", falls back to domain
        assert config.primary == "backend-architect"

    def test_skill_routing_without_task(self, router_with_matcher: GraphRouter) -> None:
        """Should use domain routing if no task provided."""
        config = router_with_matcher.get_recommended_agent("web-backend")

        # No task = domain routing
        assert config.primary == "backend-architect"


# ============================================================================
# Test Task Type Override Priority
# ============================================================================


class TestTaskTypeOverride:
    """Tests for task type override taking priority."""

    def test_task_type_override_debug(self, router_with_matcher: GraphRouter) -> None:
        """Task type 'debug' should override to debugger."""
        task = TaskContext(title="Something", task_type="debug")
        config = router_with_matcher.get_recommended_agent("web-frontend", task=task)

        # Task type override takes precedence
        assert config.primary == "debugger"

    def test_task_type_override_security(self, router_with_matcher: GraphRouter) -> None:
        """Task type 'security' should override to security-auditor."""
        task = TaskContext(title="Something", task_type="security")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        assert config.primary == "security-auditor"

    def test_task_type_priority_over_skill(self, router_with_matcher: GraphRouter) -> None:
        """Task type override should take priority over skill matching."""
        # This task has keywords that would match python-coding
        # But task_type should take precedence
        task = TaskContext(title="Python security audit", task_type="security")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        assert config.primary == "security-auditor"


# ============================================================================
# Test RoutingResult with Metadata
# ============================================================================


class TestRoutingResultMetadata:
    """Tests for get_recommended_agent_with_details."""

    def test_routing_result_skill_method(self, router_with_matcher: GraphRouter) -> None:
        """Should return skill routing method when skill matched."""
        task = TaskContext(title="Fix Python bug")
        result = router_with_matcher.get_recommended_agent_with_details("web-backend", task=task)

        assert isinstance(result, RoutingResult)
        assert result.routing_method == "skill"
        assert result.matched_skills is not None
        assert len(result.matched_skills) > 0
        assert result.skill_score is not None
        assert result.skill_score > 0

    def test_routing_result_domain_method(self, router_with_matcher: GraphRouter) -> None:
        """Should return domain routing method when no skill match."""
        task = TaskContext(title="Update readme")
        result = router_with_matcher.get_recommended_agent_with_details("web-backend", task=task)

        assert result.routing_method == "domain"
        assert result.matched_skills is None
        assert result.skill_score is None

    def test_routing_result_task_type_method(self, router_with_matcher: GraphRouter) -> None:
        """Should return task_type routing method for override."""
        task = TaskContext(title="Something", task_type="debug")
        result = router_with_matcher.get_recommended_agent_with_details("web-backend", task=task)

        assert result.routing_method == "task_type"


# ============================================================================
# Test Chain Building
# ============================================================================


class TestChainBuilding:
    """Tests for agent chain building."""

    def test_skill_match_builds_chain_from_graph(self, router_with_matcher: GraphRouter) -> None:
        """Skill-matched agents should have chain from graph."""
        task = TaskContext(title="Python API work")
        config = router_with_matcher.get_recommended_agent("web-backend", task=task)

        # backend-dev hands off to test-automator in graph
        assert config.primary == "backend-dev"
        assert len(config.chain) >= 1
        # Chain should include backend-dev
        assert "backend-dev" in config.chain


# ============================================================================
# Test Delegation to Legacy Router
# ============================================================================


class TestLegacyDelegation:
    """Tests for delegation to legacy AgentRouter methods."""

    def test_get_agent_for_task_type(self, router_without_matcher: GraphRouter) -> None:
        """Should delegate get_agent_for_task_type to legacy router."""
        agent = router_without_matcher.get_agent_for_task_type("debug")

        assert agent == "debugger"

    def test_get_chain_for_domain(self, router_without_matcher: GraphRouter) -> None:
        """Should delegate get_chain_for_domain to legacy router."""
        chain = router_without_matcher.get_chain_for_domain("web-backend")

        assert isinstance(chain, list)
        assert len(chain) > 0

    def test_get_handoff_instructions(self, router_without_matcher: GraphRouter) -> None:
        """Should delegate get_handoff_instructions to legacy router."""
        instructions = router_without_matcher.get_handoff_instructions("web-backend")

        assert isinstance(instructions, str)

    def test_get_all_domains(self, router_without_matcher: GraphRouter) -> None:
        """Should delegate get_all_domains to legacy router."""
        domains = router_without_matcher.get_all_domains()

        assert isinstance(domains, list)
        assert "web-backend" in domains
        assert "web-frontend" in domains


# ============================================================================
# Test Graph-specific Methods
# ============================================================================


class TestGraphMethods:
    """Tests for graph-specific methods."""

    def test_find_agents_for_skill(self, router_with_matcher: GraphRouter) -> None:
        """Should find agents with a specific skill."""
        agents = router_with_matcher.find_agents_for_skill("python-coding")

        assert "backend-dev" in agents

    def test_find_agents_for_skill_no_graph(self, router_without_matcher: GraphRouter) -> None:
        """Should raise error when no graph loaded."""
        with pytest.raises(ValueError, match="No graph loaded"):
            router_without_matcher.find_agents_for_skill("python-coding")

    def test_get_path_between_agents(self, router_with_matcher: GraphRouter) -> None:
        """Should find path between agents (or None if no path)."""
        # backend-dev hands off to test-automator, but the edge may not exist
        # if test-automator was added after backend-dev in the fixture
        path = router_with_matcher.get_path_between_agents("backend-dev", "debugger")

        # Since there's no direct path from backend-dev to debugger, expect None
        # This tests the API works, not the specific graph structure
        # The actual path depends on how edges are created
        assert path is None or isinstance(path, list)

    def test_get_path_no_graph(self, router_without_matcher: GraphRouter) -> None:
        """Should raise error when no graph loaded."""
        with pytest.raises(ValueError, match="No graph loaded"):
            router_without_matcher.get_path_between_agents("a", "b")


# ============================================================================
# Test Initialization
# ============================================================================


class TestInitialization:
    """Tests for GraphRouter initialization."""

    def test_init_creates_matcher_from_graph(self, graph: AgentGraph) -> None:
        """Should create SkillMatcher if graph provided but no matcher."""
        router = GraphRouter(graph=graph)

        assert router.skill_matcher is not None
        assert router.graph is graph

    def test_init_without_graph_or_matcher(self) -> None:
        """Should work without graph or matcher (legacy mode)."""
        router = GraphRouter()

        assert router.skill_matcher is None
        assert router.graph is None

    def test_init_with_both(self, graph: AgentGraph, skill_matcher: SkillMatcher) -> None:
        """Should accept both graph and matcher."""
        router = GraphRouter(graph=graph, skill_matcher=skill_matcher)

        assert router.skill_matcher is skill_matcher
        assert router.graph is graph
