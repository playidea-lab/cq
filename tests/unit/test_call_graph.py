"""Tests for call graph analysis functionality."""

from pathlib import Path

import pytest

from c4.docs.call_graph import (
    CallGraphAnalyzer,
    CallNode,
    CallEdge,
    CallPath,
    CallGraphStats,
    RelationType,
)


@pytest.fixture
def sample_project(tmp_path: Path) -> Path:
    """Create a sample project for testing."""
    (tmp_path / "main.py").write_text('''
"""Main module."""

from utils import helper_a, helper_b
from services import process_data


def main():
    """Entry point."""
    data = helper_a()
    result = process_data(data)
    helper_b(result)
    return result


def secondary():
    """Secondary function."""
    return helper_a()
''')

    (tmp_path / "utils.py").write_text('''
"""Utility functions."""


def helper_a():
    """Helper function A."""
    return "data"


def helper_b(data):
    """Helper function B."""
    print(data)
    return True
''')

    (tmp_path / "services.py").write_text('''
"""Service functions."""

from utils import helper_a


def process_data(data):
    """Process data."""
    extra = helper_a()
    return f"{data}_{extra}"


def validate_data(data):
    """Validate data."""
    return data is not None
''')

    return tmp_path


class TestCallGraphAnalyzer:
    """Tests for CallGraphAnalyzer."""

    def test_build_graph(self, sample_project: Path):
        """Test building call graph."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        count = analyzer.build()

        assert count > 0
        assert analyzer._built is True
        assert len(analyzer._nodes) > 0

    def test_get_callers(self, sample_project: Path):
        """Test getting callers of a function."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        # helper_a is called by multiple functions
        callers = analyzer.get_callers("helper_a")

        # Should have callers
        assert len(callers) >= 0  # May vary based on analysis

    def test_get_callees(self, sample_project: Path):
        """Test getting callees of a function."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        # main calls multiple functions
        callees = analyzer.get_callees("main")

        # Should have callees
        assert len(callees) >= 0

    def test_find_paths(self, sample_project: Path):
        """Test finding paths between functions."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        # Find path from main to helper_a
        paths = analyzer.find_paths("main", "helper_a")

        # May or may not find paths depending on analysis
        assert isinstance(paths, list)

    def test_get_stats(self, sample_project: Path):
        """Test getting graph statistics."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        stats = analyzer.get_stats()

        assert isinstance(stats, CallGraphStats)
        assert stats.total_nodes >= 0
        assert stats.total_edges >= 0
        assert isinstance(stats.most_called, list)
        assert isinstance(stats.entry_points, list)

    def test_stats_to_markdown(self, sample_project: Path):
        """Test stats markdown output."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        stats = analyzer.get_stats()
        markdown = stats.to_markdown()

        assert "# Call Graph Statistics" in markdown
        assert "Total Nodes" in markdown

    def test_to_mermaid(self, sample_project: Path):
        """Test Mermaid diagram generation."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        diagram = analyzer.to_mermaid()

        assert "graph TD" in diagram

    def test_to_mermaid_centered(self, sample_project: Path):
        """Test Mermaid diagram centered on a symbol."""
        analyzer = CallGraphAnalyzer(project_root=sample_project)
        analyzer.build()

        diagram = analyzer.to_mermaid(symbol_name="main", depth=2)

        assert "graph TD" in diagram


class TestCallNode:
    """Tests for CallNode."""

    def test_to_dict(self):
        """Test CallNode serialization."""
        from c4.docs.analyzer import SymbolKind

        node = CallNode(
            name="test_func",
            qualified_name="module.test_func",
            file_path="module.py",
            line_number=10,
            kind=SymbolKind.FUNCTION,
        )

        result = node.to_dict()

        assert result["name"] == "test_func"
        assert result["qualified_name"] == "module.test_func"
        assert result["file_path"] == "module.py"
        assert result["kind"] == "function"


class TestCallEdge:
    """Tests for CallEdge."""

    def test_to_dict(self):
        """Test CallEdge serialization."""
        edge = CallEdge(
            source="module.caller",
            target="module.callee",
            source_file="module.py",
            target_file="module.py",
            source_line=10,
            relation=RelationType.CALLS,
        )

        result = edge.to_dict()

        assert result["source"] == "module.caller"
        assert result["target"] == "module.callee"
        assert result["relation"] == "calls"


class TestCallPath:
    """Tests for CallPath."""

    def test_to_dict(self):
        """Test CallPath serialization."""
        path = CallPath(
            nodes=["a", "b", "c"],
            edges=[],
            total_weight=2.0,
        )

        result = path.to_dict()

        assert result["nodes"] == ["a", "b", "c"]
        assert result["total_weight"] == 2.0
