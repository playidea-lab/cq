"""Unit tests for DynamicChainBuilder - Task-aware chain construction.

Tests cover:
1. Role detection from task keywords
2. Basic chain building with handoffs
3. Chain building with required roles
4. Path optimization
5. Edge cases
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph import (
    AgentGraph,
    ChainBuildContext,
    DynamicChainBuilder,
    TaskContext,
)
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    Skill,
    SkillDefinition,
    SkillTriggers,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def graph_with_agents() -> AgentGraph:
    """Create a graph with agents and handoff relationships."""
    from c4.supervisor.agent_graph.graph import EdgeType

    g = AgentGraph()

    # Add a skill
    python_skill = SkillDefinition(
        skill=Skill(
            id="python-coding",
            name="Python Coding",
            description="Writing Python code",
            capabilities=["write python code"],
            triggers=SkillTriggers(keywords=["python"]),
        )
    )
    g.add_skill(python_skill)

    # First, add all agents without relationships
    # This ensures all nodes exist before we add handoff edges
    code_reviewer = AgentDefinition(
        agent=Agent(
            id="code-reviewer",
            name="Code Reviewer",
            persona=AgentPersona(role="Reviewer", expertise="Code quality"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(code_reviewer)

    test_automator = AgentDefinition(
        agent=Agent(
            id="test-automator",
            name="Test Automator",
            persona=AgentPersona(role="Tester", expertise="Testing"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(test_automator)

    security_auditor = AgentDefinition(
        agent=Agent(
            id="security-auditor",
            name="Security Auditor",
            persona=AgentPersona(role="Security", expertise="Security"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(security_auditor)

    python_pro = AgentDefinition(
        agent=Agent(
            id="python-pro",
            name="Python Pro",
            persona=AgentPersona(role="Developer", expertise="Python"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(python_pro)

    backend_architect = AgentDefinition(
        agent=Agent(
            id="backend-architect",
            name="Backend Architect",
            persona=AgentPersona(role="Architect", expertise="System design"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(backend_architect)

    payment_integration = AgentDefinition(
        agent=Agent(
            id="payment-integration",
            name="Payment Integration",
            persona=AgentPersona(role="Payment", expertise="Payments"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(payment_integration)

    # Now add handoff edges manually
    # backend-architect -> python-pro -> test-automator -> code-reviewer
    g.add_edge("backend-architect", "python-pro", EdgeType.HANDS_OFF_TO, weight=0.9)
    g.add_edge("backend-architect", "security-auditor", EdgeType.HANDS_OFF_TO, weight=0.7)
    g.add_edge("python-pro", "test-automator", EdgeType.HANDS_OFF_TO, weight=0.85)
    g.add_edge("test-automator", "code-reviewer", EdgeType.HANDS_OFF_TO, weight=0.8)
    g.add_edge("security-auditor", "code-reviewer", EdgeType.HANDS_OFF_TO, weight=0.75)
    g.add_edge("payment-integration", "security-auditor", EdgeType.HANDS_OFF_TO, weight=0.9)

    return g


@pytest.fixture
def builder(graph_with_agents: AgentGraph) -> DynamicChainBuilder:
    """Create a DynamicChainBuilder with the test graph."""
    return DynamicChainBuilder(graph_with_agents)


# ============================================================================
# Test Role Detection
# ============================================================================


class TestRoleDetection:
    """Tests for detecting required roles from task content."""

    def test_detect_security_keywords(self, builder: DynamicChainBuilder) -> None:
        """Should detect security-auditor from security keywords."""
        task = TaskContext(title="Fix authentication vulnerability")
        roles = builder.detect_required_roles(task)
        assert "security-auditor" in roles

    def test_detect_payment_keywords(self, builder: DynamicChainBuilder) -> None:
        """Should detect payment-integration from payment keywords."""
        task = TaskContext(title="Integrate Stripe payment")
        roles = builder.detect_required_roles(task)
        assert "payment-integration" in roles

    def test_detect_test_keywords(self, builder: DynamicChainBuilder) -> None:
        """Should detect test-automator from test keywords."""
        task = TaskContext(title="Write unit tests for API")
        roles = builder.detect_required_roles(task)
        assert "test-automator" in roles

    def test_detect_multiple_roles(self, builder: DynamicChainBuilder) -> None:
        """Should detect multiple roles from combined keywords."""
        task = TaskContext(
            title="Add payment feature", description="Include security review and tests"
        )
        roles = builder.detect_required_roles(task)
        assert "payment-integration" in roles
        assert "security-auditor" in roles
        assert "test-automator" in roles

    def test_detect_from_description(self, builder: DynamicChainBuilder) -> None:
        """Should detect roles from description field."""
        task = TaskContext(title="Add feature", description="Must pass security audit")
        roles = builder.detect_required_roles(task)
        assert "security-auditor" in roles

    def test_no_detection_for_unrelated_task(self, builder: DynamicChainBuilder) -> None:
        """Should return empty set for task without special keywords."""
        task = TaskContext(title="Refactor user model")
        roles = builder.detect_required_roles(task)
        # code-reviewer might match "refactor"
        assert "payment-integration" not in roles
        assert "security-auditor" not in roles

    def test_case_insensitive_detection(self, builder: DynamicChainBuilder) -> None:
        """Detection should be case insensitive."""
        task = TaskContext(title="SECURITY VULNERABILITY FIX")
        roles = builder.detect_required_roles(task)
        assert "security-auditor" in roles


# ============================================================================
# Test Basic Chain Building
# ============================================================================


class TestBasicChainBuilding:
    """Tests for basic chain building following handoffs."""

    def test_build_chain_follows_handoffs(self, builder: DynamicChainBuilder) -> None:
        """Should follow handoff edges by weight."""
        chain = builder.build_chain("backend-architect")

        # Should follow highest-weight path
        assert chain[0] == "backend-architect"
        assert "python-pro" in chain  # weight 0.9
        assert len(chain) > 1

    def test_build_chain_respects_max_length(self, builder: DynamicChainBuilder) -> None:
        """Should respect max_length limit."""
        context = ChainBuildContext(max_length=2)
        chain = builder.build_chain("backend-architect", context)
        assert len(chain) <= 2

    def test_build_chain_prevents_cycles(self, builder: DynamicChainBuilder) -> None:
        """Should not include same agent twice."""
        chain = builder.build_chain("backend-architect")
        assert len(chain) == len(set(chain))  # No duplicates

    def test_build_chain_single_agent(
        self, builder: DynamicChainBuilder, graph_with_agents: AgentGraph
    ) -> None:
        """Should handle agent with no handoffs."""
        chain = builder.build_chain("code-reviewer")
        assert chain == ["code-reviewer"]


# ============================================================================
# Test Chain Building with Required Roles
# ============================================================================


class TestChainWithRequiredRoles:
    """Tests for chain building that includes required roles."""

    def test_include_required_role(self, builder: DynamicChainBuilder) -> None:
        """Should include required role in chain."""
        context = ChainBuildContext(required_roles={"security-auditor"})
        chain = builder.build_chain("backend-architect", context)
        assert "security-auditor" in chain

    def test_include_multiple_required_roles(self, builder: DynamicChainBuilder) -> None:
        """Should include all required roles."""
        context = ChainBuildContext(required_roles={"security-auditor", "test-automator"})
        chain = builder.build_chain("backend-architect", context)
        assert "security-auditor" in chain
        assert "test-automator" in chain

    def test_required_role_from_task_detection(self, builder: DynamicChainBuilder) -> None:
        """Should auto-detect required roles from task."""
        task = TaskContext(title="Add payment with security review")
        context = ChainBuildContext(task=task)
        chain = builder.build_chain("backend-architect", context)
        assert "security-auditor" in chain
        assert "payment-integration" in chain

    def test_exclude_agents(self, builder: DynamicChainBuilder) -> None:
        """Should exclude specified agents from chain."""
        context = ChainBuildContext(exclude_agents={"python-pro"})
        chain = builder.build_chain("backend-architect", context)
        assert "python-pro" not in chain


# ============================================================================
# Test Path Building
# ============================================================================


class TestPathBuilding:
    """Tests for building chains to specific targets."""

    def test_build_chain_with_path_direct(self, builder: DynamicChainBuilder) -> None:
        """Should find direct path to target."""
        chain = builder.build_chain_with_path("backend-architect", "python-pro")
        assert chain is not None
        assert chain[0] == "backend-architect"
        assert "python-pro" in chain

    def test_build_chain_with_path_indirect(self, builder: DynamicChainBuilder) -> None:
        """Should find indirect path to target."""
        chain = builder.build_chain_with_path("backend-architect", "code-reviewer")
        assert chain is not None
        assert chain[0] == "backend-architect"
        assert "code-reviewer" in chain

    def test_build_chain_with_no_path(self, builder: DynamicChainBuilder) -> None:
        """Should still include target as required role if no direct path."""
        # code-reviewer has no outgoing edges, so no path to backend-architect
        chain = builder.build_chain_with_path("code-reviewer", "payment-integration")
        assert chain is not None
        assert "payment-integration" in chain


# ============================================================================
# Test Chain Optimization
# ============================================================================


class TestChainOptimization:
    """Tests for chain optimization."""

    def test_optimize_keeps_primary(self, builder: DynamicChainBuilder) -> None:
        """Should always keep primary agent."""
        chain = ["backend-architect", "python-pro", "test-automator"]
        optimized = builder.optimize_chain(chain)
        assert optimized[0] == "backend-architect"

    def test_optimize_keeps_required_roles(self, builder: DynamicChainBuilder) -> None:
        """Should keep required roles."""
        chain = ["backend-architect", "python-pro", "security-auditor"]
        optimized = builder.optimize_chain(chain, required_roles={"security-auditor"})
        assert "security-auditor" in optimized

    def test_optimize_respects_max_length(self, builder: DynamicChainBuilder) -> None:
        """Should respect max_length."""
        chain = ["a", "b", "c", "d", "e"]
        optimized = builder.optimize_chain(chain, max_length=3)
        assert len(optimized) <= 3

    def test_optimize_single_agent(self, builder: DynamicChainBuilder) -> None:
        """Should handle single-agent chain."""
        chain = ["backend-architect"]
        optimized = builder.optimize_chain(chain)
        assert optimized == ["backend-architect"]


# ============================================================================
# Test Role Keywords Management
# ============================================================================


class TestRoleKeywords:
    """Tests for role keyword management."""

    def test_get_role_keywords(self, builder: DynamicChainBuilder) -> None:
        """Should return copy of role keywords."""
        keywords = builder.role_keywords
        assert "security-auditor" in keywords
        assert "security" in keywords["security-auditor"]

    def test_add_role_keywords_new(self, builder: DynamicChainBuilder) -> None:
        """Should add keywords for new role."""
        builder.add_role_keywords("custom-agent", ["custom", "special"])
        keywords = builder.role_keywords
        assert "custom-agent" in keywords
        assert "custom" in keywords["custom-agent"]

    def test_add_role_keywords_existing(self, builder: DynamicChainBuilder) -> None:
        """Should merge keywords for existing role."""
        builder.add_role_keywords("security-auditor", ["owasp"])
        keywords = builder.role_keywords
        assert "owasp" in keywords["security-auditor"]
        assert "security" in keywords["security-auditor"]  # Original preserved


# ============================================================================
# Test Custom Role Keywords
# ============================================================================


class TestCustomRoleKeywords:
    """Tests for custom role keyword mapping."""

    def test_custom_keywords(self, graph_with_agents: AgentGraph) -> None:
        """Should use custom keywords."""
        custom_keywords = {
            "python-pro": ["python", "py", "django"],
        }
        builder = DynamicChainBuilder(graph_with_agents, role_keywords=custom_keywords)

        task = TaskContext(title="Django migration")
        roles = builder.detect_required_roles(task)
        assert "python-pro" in roles

    def test_empty_custom_keywords(self, graph_with_agents: AgentGraph) -> None:
        """Should handle empty custom keywords."""
        builder = DynamicChainBuilder(graph_with_agents, role_keywords={})

        task = TaskContext(title="Security vulnerability")
        roles = builder.detect_required_roles(task)
        assert len(roles) == 0  # No keywords to match
