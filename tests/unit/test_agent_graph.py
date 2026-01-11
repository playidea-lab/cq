"""Unit tests for AgentGraph - NetworkX-based graph implementation.

Tests cover:
1. Node management (add_skill, add_agent, add_domain)
2. Edge management (add_edge, automatic edge creation)
3. Node/edge querying (get_node, get_edges, get_all_nodes)
4. Property accessors (skills, agents, domains)
5. Edge type filtering
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph.graph import AgentGraph, EdgeType, NodeType
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentHandsOffTo,
    AgentPersona,
    AgentPersonality,
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

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def graph() -> AgentGraph:
    """Create a fresh AgentGraph instance."""
    return AgentGraph()


@pytest.fixture
def sample_skill() -> SkillDefinition:
    """Create a sample skill definition."""
    return SkillDefinition(
        skill=Skill(
            id="python-coding",
            name="Python Coding",
            description="Writing Python code following best practices",
            capabilities=["write Python code", "debug Python code"],
            triggers=SkillTriggers(keywords=["python", "py"]),
            prerequisites=["basic-programming"],
            complementary_skills=["testing"],
        )
    )


@pytest.fixture
def sample_skill_testing() -> SkillDefinition:
    """Create a testing skill definition."""
    return SkillDefinition(
        skill=Skill(
            id="testing",
            name="Testing",
            description="Writing and running tests",
            capabilities=["write unit tests", "run pytest"],
            triggers=SkillTriggers(keywords=["test", "pytest"]),
        )
    )


@pytest.fixture
def sample_agent() -> AgentDefinition:
    """Create a sample agent definition."""
    return AgentDefinition(
        agent=Agent(
            id="backend-dev",
            name="Backend Developer",
            persona=AgentPersona(
                role="Backend Developer",
                expertise="Python backend development",
                personality=AgentPersonality(style="methodical"),
            ),
            skills=AgentSkills(
                primary=["python-coding"],
                secondary=["testing"],
            ),
            relationships=AgentRelationships(
                hands_off_to=[
                    AgentHandsOffTo(
                        agent="code-reviewer",
                        when="Code is complete",
                        passes="Implementation details",
                        weight=0.8,
                    )
                ]
            ),
        )
    )


@pytest.fixture
def sample_agent_reviewer() -> AgentDefinition:
    """Create a code reviewer agent definition."""
    return AgentDefinition(
        agent=Agent(
            id="code-reviewer",
            name="Code Reviewer",
            persona=AgentPersona(
                role="Code Reviewer",
                expertise="Code quality and best practices",
            ),
            skills=AgentSkills(primary=["code-review"]),
            relationships=AgentRelationships(),
        )
    )


@pytest.fixture
def sample_domain() -> DomainDefinition:
    """Create a sample domain definition."""
    return DomainDefinition(
        domain=Domain(
            id="web-backend",
            name="Web Backend",
            description="Backend web development domain",
            required_skills=DomainRequiredSkills(
                core=["python-coding"],
                optional=["testing"],
            ),
            workflow=[
                WorkflowStep(
                    step=1,
                    role="primary",
                    select=WorkflowSelect(by="agent", prefer_agent="backend-dev"),
                    purpose="Implement the backend logic",
                ),
                WorkflowStep(
                    step=2,
                    role="quality",
                    select=WorkflowSelect(by="skill", skills=["code-review"]),
                    purpose="Review the code",
                ),
            ],
        )
    )


# ============================================================================
# Test NodeType and EdgeType Enums
# ============================================================================


class TestEnums:
    """Test NodeType and EdgeType enumerations."""

    def test_node_type_values(self) -> None:
        """NodeType should have skill, agent, domain values."""
        assert NodeType.SKILL == "skill"
        assert NodeType.AGENT == "agent"
        assert NodeType.DOMAIN == "domain"

    def test_edge_type_values(self) -> None:
        """EdgeType should have all required edge types."""
        assert EdgeType.HAS_SKILL == "has_skill"
        assert EdgeType.HANDS_OFF_TO == "hands_off_to"
        assert EdgeType.PREFERS == "prefers"
        assert EdgeType.TRIGGERS == "triggers"
        assert EdgeType.REQUIRES == "requires"
        assert EdgeType.COMPLEMENTS == "complements"


# ============================================================================
# Test add_skill()
# ============================================================================


class TestAddSkill:
    """Tests for AgentGraph.add_skill()."""

    def test_add_skill_creates_node(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """add_skill should create a node with type=skill."""
        graph.add_skill(sample_skill)

        node = graph.get_node("python-coding")
        assert node is not None
        assert node["type"] == NodeType.SKILL
        assert node["name"] == "Python Coding"

    def test_add_skill_appears_in_skills_list(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """Added skill should appear in graph.skills property."""
        graph.add_skill(sample_skill)

        assert "python-coding" in graph.skills

    def test_add_skill_creates_requires_edges(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
    ) -> None:
        """add_skill should create REQUIRES edges for prerequisites."""
        # First add the prerequisite skill
        prereq_skill = SkillDefinition(
            skill=Skill(
                id="basic-programming",
                name="Basic Programming",
                description="Basic programming concepts",
                capabilities=["basic coding"],
                triggers=SkillTriggers(keywords=["programming"]),
            )
        )
        graph.add_skill(prereq_skill)
        graph.add_skill(sample_skill)

        edges = graph.get_edges("python-coding", EdgeType.REQUIRES)
        assert len(edges) == 1
        assert edges[0]["to"] == "basic-programming"

    def test_add_skill_creates_complements_edges(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_skill_testing: SkillDefinition,
    ) -> None:
        """add_skill should create COMPLEMENTS edges for complementary_skills."""
        graph.add_skill(sample_skill_testing)
        graph.add_skill(sample_skill)

        edges = graph.get_edges("python-coding", EdgeType.COMPLEMENTS)
        assert len(edges) == 1
        assert edges[0]["to"] == "testing"


# ============================================================================
# Test add_agent()
# ============================================================================


class TestAddAgent:
    """Tests for AgentGraph.add_agent()."""

    def test_add_agent_creates_node(
        self, graph: AgentGraph, sample_agent: AgentDefinition
    ) -> None:
        """add_agent should create a node with type=agent."""
        graph.add_agent(sample_agent)

        node = graph.get_node("backend-dev")
        assert node is not None
        assert node["type"] == NodeType.AGENT
        assert node["name"] == "Backend Developer"

    def test_add_agent_appears_in_agents_list(
        self, graph: AgentGraph, sample_agent: AgentDefinition
    ) -> None:
        """Added agent should appear in graph.agents property."""
        graph.add_agent(sample_agent)

        assert "backend-dev" in graph.agents

    def test_add_agent_creates_has_skill_edges_for_primary(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_agent: AgentDefinition,
    ) -> None:
        """add_agent should create HAS_SKILL edges for primary skills."""
        graph.add_skill(sample_skill)
        graph.add_agent(sample_agent)

        edges = graph.get_edges("backend-dev", EdgeType.HAS_SKILL)
        skill_ids = [e["to"] for e in edges]
        assert "python-coding" in skill_ids

    def test_add_agent_creates_has_skill_edges_for_secondary(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_skill_testing: SkillDefinition,
        sample_agent: AgentDefinition,
    ) -> None:
        """add_agent should create HAS_SKILL edges for secondary skills."""
        graph.add_skill(sample_skill)
        graph.add_skill(sample_skill_testing)
        graph.add_agent(sample_agent)

        edges = graph.get_edges("backend-dev", EdgeType.HAS_SKILL)
        skill_ids = [e["to"] for e in edges]
        assert "testing" in skill_ids

    def test_add_agent_creates_hands_off_to_edges(
        self,
        graph: AgentGraph,
        sample_agent: AgentDefinition,
        sample_agent_reviewer: AgentDefinition,
    ) -> None:
        """add_agent should create HANDS_OFF_TO edges."""
        graph.add_agent(sample_agent_reviewer)
        graph.add_agent(sample_agent)

        edges = graph.get_edges("backend-dev", EdgeType.HANDS_OFF_TO)
        assert len(edges) == 1
        assert edges[0]["to"] == "code-reviewer"
        assert edges[0]["weight"] == 0.8
        assert edges[0]["when"] == "Code is complete"


# ============================================================================
# Test add_domain()
# ============================================================================


class TestAddDomain:
    """Tests for AgentGraph.add_domain()."""

    def test_add_domain_creates_node(
        self, graph: AgentGraph, sample_domain: DomainDefinition
    ) -> None:
        """add_domain should create a node with type=domain."""
        graph.add_domain(sample_domain)

        node = graph.get_node("web-backend")
        assert node is not None
        assert node["type"] == NodeType.DOMAIN
        assert node["name"] == "Web Backend"

    def test_add_domain_appears_in_domains_list(
        self, graph: AgentGraph, sample_domain: DomainDefinition
    ) -> None:
        """Added domain should appear in graph.domains property."""
        graph.add_domain(sample_domain)

        assert "web-backend" in graph.domains

    def test_add_domain_creates_prefers_edges(
        self,
        graph: AgentGraph,
        sample_agent: AgentDefinition,
        sample_domain: DomainDefinition,
    ) -> None:
        """add_domain should create PREFERS edges for prefer_agent in workflow."""
        graph.add_agent(sample_agent)
        graph.add_domain(sample_domain)

        edges = graph.get_edges("web-backend", EdgeType.PREFERS)
        assert len(edges) == 1
        assert edges[0]["to"] == "backend-dev"


# ============================================================================
# Test add_edge()
# ============================================================================


class TestAddEdge:
    """Tests for AgentGraph.add_edge()."""

    def test_add_edge_creates_edge(self, graph: AgentGraph) -> None:
        """add_edge should create an edge between nodes."""
        # Add nodes first (manually for this test)
        graph._graph.add_node("skill-a", type=NodeType.SKILL)
        graph._graph.add_node("skill-b", type=NodeType.SKILL)

        graph.add_edge("skill-a", "skill-b", EdgeType.COMPLEMENTS)

        edges = graph.get_edges("skill-a", EdgeType.COMPLEMENTS)
        assert len(edges) == 1
        assert edges[0]["to"] == "skill-b"

    def test_add_edge_with_attributes(self, graph: AgentGraph) -> None:
        """add_edge should store additional attributes."""
        graph._graph.add_node("agent-a", type=NodeType.AGENT)
        graph._graph.add_node("agent-b", type=NodeType.AGENT)

        graph.add_edge(
            "agent-a", "agent-b", EdgeType.HANDS_OFF_TO, weight=0.9, when="Task done"
        )

        edges = graph.get_edges("agent-a", EdgeType.HANDS_OFF_TO)
        assert edges[0]["weight"] == 0.9
        assert edges[0]["when"] == "Task done"


# ============================================================================
# Test get_node()
# ============================================================================


class TestGetNode:
    """Tests for AgentGraph.get_node()."""

    def test_get_node_returns_node_data(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """get_node should return node attributes."""
        graph.add_skill(sample_skill)

        node = graph.get_node("python-coding")
        assert node is not None
        assert node["type"] == NodeType.SKILL
        assert "definition" in node

    def test_get_node_returns_none_for_missing(self, graph: AgentGraph) -> None:
        """get_node should return None for non-existent nodes."""
        assert graph.get_node("nonexistent") is None


# ============================================================================
# Test get_edges()
# ============================================================================


class TestGetEdges:
    """Tests for AgentGraph.get_edges()."""

    def test_get_edges_returns_all_outgoing(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_skill_testing: SkillDefinition,
        sample_agent: AgentDefinition,
    ) -> None:
        """get_edges without filter returns all outgoing edges."""
        graph.add_skill(sample_skill)
        graph.add_skill(sample_skill_testing)
        graph.add_agent(sample_agent)

        # Agent has HAS_SKILL edges to both skills
        all_edges = graph.get_edges("backend-dev")
        assert len(all_edges) >= 2  # At least 2 HAS_SKILL edges

    def test_get_edges_filters_by_type(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_agent: AgentDefinition,
        sample_agent_reviewer: AgentDefinition,
    ) -> None:
        """get_edges with edge_type filter returns only matching edges."""
        graph.add_skill(sample_skill)
        graph.add_agent(sample_agent_reviewer)
        graph.add_agent(sample_agent)

        # Filter to only HANDS_OFF_TO
        handoff_edges = graph.get_edges("backend-dev", EdgeType.HANDS_OFF_TO)
        assert len(handoff_edges) == 1
        assert all(e["edge_type"] == EdgeType.HANDS_OFF_TO for e in handoff_edges)

    def test_get_edges_returns_empty_for_no_matches(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """get_edges returns empty list when no edges match."""
        graph.add_skill(sample_skill)

        edges = graph.get_edges("python-coding", EdgeType.HANDS_OFF_TO)
        assert edges == []


# ============================================================================
# Test get_all_nodes()
# ============================================================================


class TestGetAllNodes:
    """Tests for AgentGraph.get_all_nodes()."""

    def test_get_all_nodes_returns_all(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_agent: AgentDefinition,
        sample_domain: DomainDefinition,
    ) -> None:
        """get_all_nodes without filter returns all node IDs."""
        graph.add_skill(sample_skill)
        graph.add_agent(sample_agent)
        graph.add_domain(sample_domain)

        all_nodes = graph.get_all_nodes()
        assert "python-coding" in all_nodes
        assert "backend-dev" in all_nodes
        assert "web-backend" in all_nodes

    def test_get_all_nodes_filters_by_type(
        self,
        graph: AgentGraph,
        sample_skill: SkillDefinition,
        sample_agent: AgentDefinition,
    ) -> None:
        """get_all_nodes with node_type filter returns only matching nodes."""
        graph.add_skill(sample_skill)
        graph.add_agent(sample_agent)

        skill_nodes = graph.get_all_nodes(NodeType.SKILL)
        assert "python-coding" in skill_nodes
        assert "backend-dev" not in skill_nodes


# ============================================================================
# Test Property Accessors
# ============================================================================


class TestPropertyAccessors:
    """Tests for property accessors (skills, agents, domains)."""

    def test_skills_property(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """skills property returns all skill node IDs."""
        graph.add_skill(sample_skill)
        assert graph.skills == ["python-coding"]

    def test_agents_property(
        self, graph: AgentGraph, sample_agent: AgentDefinition
    ) -> None:
        """agents property returns all agent node IDs."""
        graph.add_agent(sample_agent)
        assert graph.agents == ["backend-dev"]

    def test_domains_property(
        self, graph: AgentGraph, sample_domain: DomainDefinition
    ) -> None:
        """domains property returns all domain node IDs."""
        graph.add_domain(sample_domain)
        assert graph.domains == ["web-backend"]

    def test_properties_return_empty_for_empty_graph(
        self, graph: AgentGraph
    ) -> None:
        """Properties return empty lists for empty graph."""
        assert graph.skills == []
        assert graph.agents == []
        assert graph.domains == []


# ============================================================================
# Test Edge Automatic Creation
# ============================================================================


class TestAutoEdgeCreation:
    """Tests for automatic edge creation logic."""

    def test_skill_requires_edge_only_if_target_exists(
        self, graph: AgentGraph, sample_skill: SkillDefinition
    ) -> None:
        """REQUIRES edge created only if prerequisite skill exists in graph."""
        # Add skill without its prerequisite
        graph.add_skill(sample_skill)

        # No REQUIRES edge because "basic-programming" not in graph
        edges = graph.get_edges("python-coding", EdgeType.REQUIRES)
        assert len(edges) == 0

    def test_agent_has_skill_edge_only_if_skill_exists(
        self, graph: AgentGraph, sample_agent: AgentDefinition
    ) -> None:
        """HAS_SKILL edge created only if skill exists in graph."""
        # Add agent without adding skills first
        graph.add_agent(sample_agent)

        # No HAS_SKILL edges because skills not in graph
        edges = graph.get_edges("backend-dev", EdgeType.HAS_SKILL)
        assert len(edges) == 0

    def test_agent_hands_off_edge_only_if_target_exists(
        self, graph: AgentGraph, sample_agent: AgentDefinition
    ) -> None:
        """HANDS_OFF_TO edge created only if target agent exists in graph."""
        # Add agent without adding the reviewer first
        graph.add_agent(sample_agent)

        # No HANDS_OFF_TO edges because "code-reviewer" not in graph
        edges = graph.get_edges("backend-dev", EdgeType.HANDS_OFF_TO)
        assert len(edges) == 0

    def test_domain_prefers_edge_only_if_agent_exists(
        self, graph: AgentGraph, sample_domain: DomainDefinition
    ) -> None:
        """PREFERS edge created only if agent exists in graph."""
        # Add domain without adding the preferred agent first
        graph.add_domain(sample_domain)

        # No PREFERS edges because "backend-dev" not in graph
        edges = graph.get_edges("web-backend", EdgeType.PREFERS)
        assert len(edges) == 0
