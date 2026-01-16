"""Unit tests for agent_graph visualizer module."""

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
    Skill,
    SkillDefinition,
    SkillTriggers,
)
from c4.supervisor.agent_graph.visualizer import (
    GraphVisualizer,
    highlight_path,
    to_mermaid,
)


@pytest.fixture
def sample_graph() -> AgentGraph:
    """Create a sample graph for testing."""
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

    # Add agents
    graph.add_agent(
        AgentDefinition(
            agent=Agent(
                id="backend-dev",
                name="Backend Developer",
                persona=AgentPersona(role="Developer", expertise="Backend"),
                skills=AgentSkills(primary=["python-coding"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="test-automator",
                            when="After implementation",
                            passes="Code",
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
                id="test-automator",
                name="Test Automator",
                persona=AgentPersona(role="Tester", expertise="Testing"),
                skills=AgentSkills(primary=["testing"]),
                relationships=AgentRelationships(
                    hands_off_to=[
                        AgentHandsOffTo(
                            agent="code-reviewer",
                            when="After testing",
                            passes="Results",
                            weight=0.7,
                        )
                    ]
                ),
            )
        )
    )
    graph.add_agent(
        AgentDefinition(
            agent=Agent(
                id="code-reviewer",
                name="Code Reviewer",
                persona=AgentPersona(role="Reviewer", expertise="Review"),
                skills=AgentSkills(primary=["testing"]),
                relationships=AgentRelationships(),
            )
        )
    )

    return graph


class TestGraphVisualizer:
    """Tests for GraphVisualizer class."""

    def test_to_mermaid_returns_string(self, sample_graph: AgentGraph) -> None:
        """to_mermaid returns a string."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid()
        assert isinstance(result, str)
        assert result.startswith("graph TD")

    def test_to_mermaid_includes_subgraphs(self, sample_graph: AgentGraph) -> None:
        """to_mermaid includes subgraphs for each node type."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid()

        assert "subgraph Skills" in result
        assert "subgraph Agents" in result
        assert "python-coding" in result
        assert "backend-dev" in result

    def test_to_mermaid_includes_edges(self, sample_graph: AgentGraph) -> None:
        """to_mermaid includes edges between nodes."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid()

        # HAS_SKILL edge from agent to skill
        assert "backend-dev" in result
        assert "python-coding" in result

    def test_to_mermaid_filter_agents(self, sample_graph: AgentGraph) -> None:
        """to_mermaid with filter_type='agents' shows only agents."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid(filter_type="agents")

        # Should have agents
        assert "backend-dev" in result
        assert "test-automator" in result

        # Should not have skills subgraph
        assert "subgraph Skills" not in result

    def test_to_mermaid_filter_skills(self, sample_graph: AgentGraph) -> None:
        """to_mermaid with filter_type='skills' shows only skills."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid(filter_type="skills")

        # Should have skills
        assert "python-coding" in result
        assert "testing" in result

        # Should not have agents subgraph
        assert "subgraph Agents" not in result

    def test_to_mermaid_direction(self, sample_graph: AgentGraph) -> None:
        """to_mermaid respects direction parameter."""
        visualizer = GraphVisualizer(sample_graph)

        result_td = visualizer.to_mermaid(direction="TD")
        assert result_td.startswith("graph TD")

        result_lr = visualizer.to_mermaid(direction="LR")
        assert result_lr.startswith("graph LR")

    def test_to_mermaid_no_labels(self, sample_graph: AgentGraph) -> None:
        """to_mermaid with include_labels=False omits edge labels."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.to_mermaid(include_labels=False)

        # Should not have pipe-delimited labels
        # Count pipes - should be none for labels
        label_pattern = "|has|"
        assert label_pattern not in result

    def test_highlight_path(self, sample_graph: AgentGraph) -> None:
        """highlight_path includes highlight class for path nodes."""
        visualizer = GraphVisualizer(sample_graph)
        result = visualizer.highlight_path("backend-dev", "code-reviewer")

        # Should have highlight class definition
        assert "classDef highlight" in result

        # Should have class assignment for path nodes
        assert "class" in result
        assert "highlight" in result

    def test_highlight_path_no_path(self, sample_graph: AgentGraph) -> None:
        """highlight_path works even when no path exists."""
        visualizer = GraphVisualizer(sample_graph)
        # code-reviewer has no outgoing handoffs
        result = visualizer.highlight_path("code-reviewer", "backend-dev")

        # Should still generate valid mermaid
        assert result.startswith("graph TD")
        assert "classDef highlight" in result


class TestModuleFunctions:
    """Tests for module-level convenience functions."""

    def test_to_mermaid_function(self, sample_graph: AgentGraph) -> None:
        """to_mermaid function works as expected."""
        result = to_mermaid(sample_graph)
        assert isinstance(result, str)
        assert result.startswith("graph TD")

    def test_to_mermaid_function_with_options(self, sample_graph: AgentGraph) -> None:
        """to_mermaid function accepts options."""
        result = to_mermaid(
            sample_graph,
            filter_type="agents",
            include_labels=False,
            direction="LR",
        )
        assert result.startswith("graph LR")
        assert "backend-dev" in result

    def test_highlight_path_function(self, sample_graph: AgentGraph) -> None:
        """highlight_path function works as expected."""
        result = highlight_path(sample_graph, "backend-dev", "test-automator")
        assert isinstance(result, str)
        assert "highlight" in result


class TestEmptyGraph:
    """Tests for empty graph visualization."""

    def test_empty_graph(self) -> None:
        """Empty graph produces valid mermaid."""
        graph = AgentGraph()
        visualizer = GraphVisualizer(graph)
        result = visualizer.to_mermaid()

        assert result.startswith("graph TD")
        # Should not have subgraphs for empty graph
        assert "subgraph" not in result


class TestNodeStyles:
    """Tests for node styling."""

    def test_skill_style_applied(self, sample_graph: AgentGraph) -> None:
        """Skills get the skill style class."""
        result = to_mermaid(sample_graph)
        assert "python-coding[Python Coding]:::skill" in result

    def test_agent_style_applied(self, sample_graph: AgentGraph) -> None:
        """Agents get the agent style class."""
        result = to_mermaid(sample_graph)
        assert "backend-dev[Backend Developer]:::agent" in result

    def test_style_definitions_included(self, sample_graph: AgentGraph) -> None:
        """Style class definitions are included."""
        result = to_mermaid(sample_graph)
        assert "classDef skill" in result
        assert "classDef agent" in result
        assert "classDef domain" in result
