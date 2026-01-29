"""Unit tests for DependencyAnalyzer."""

from __future__ import annotations

import networkx as nx

from c4.analysis.dependency import DependencyAnalyzer
from c4.models.enums import TaskStatus
from c4.models.task import Task


class MockTaskStore:
    """Mock task store for testing."""

    def __init__(self, tasks: list[Task]) -> None:
        self._tasks = tasks

    def load_all(self, project_id: str) -> list[Task]:
        return self._tasks


def create_task(task_id: str, title: str, dependencies: list[str] | None = None) -> Task:
    """Helper to create test tasks."""
    return Task(
        id=task_id,
        title=title,
        dod=f"Complete {title}",
        dependencies=dependencies or [],
        status=TaskStatus.PENDING,
    )


class TestDependencyAnalyzerBuildGraph:
    """Tests for build_graph method."""

    def test_build_graph_empty(self):
        """Test building graph with no tasks."""
        store = MockTaskStore([])
        analyzer = DependencyAnalyzer(store)

        graph = analyzer.build_graph("test-project")

        assert len(graph.nodes()) == 0
        assert len(graph.edges()) == 0

    def test_build_graph_single_task(self):
        """Test building graph with a single task."""
        tasks = [create_task("T-001-0", "Task 1")]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)

        graph = analyzer.build_graph("test-project")

        assert len(graph.nodes()) == 1
        assert "T-001-0" in graph.nodes()
        assert len(graph.edges()) == 0

    def test_build_graph_with_dependencies(self):
        """Test building graph with dependencies."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-001-0", "T-002-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)

        graph = analyzer.build_graph("test-project")

        assert len(graph.nodes()) == 3
        assert len(graph.edges()) == 3
        # T-001 -> T-002, T-001 -> T-003, T-002 -> T-003
        assert graph.has_edge("T-001-0", "T-002-0")
        assert graph.has_edge("T-001-0", "T-003-0")
        assert graph.has_edge("T-002-0", "T-003-0")

    def test_build_graph_node_attributes(self):
        """Test that nodes have correct attributes."""
        tasks = [create_task("T-001-0", "Task 1")]
        tasks[0].priority = 10
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)

        graph = analyzer.build_graph("test-project")

        node_data = graph.nodes["T-001-0"]
        assert node_data["title"] == "Task 1"
        assert node_data["priority"] == 10


class TestDependencyAnalyzerCriticalPath:
    """Tests for find_critical_path method."""

    def test_critical_path_empty_graph(self):
        """Test critical path on empty graph."""
        store = MockTaskStore([])
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        path = analyzer.find_critical_path(graph)

        assert path == []

    def test_critical_path_single_node(self):
        """Test critical path with single node."""
        tasks = [create_task("T-001-0", "Task 1")]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        path = analyzer.find_critical_path(graph)

        assert path == ["T-001-0"]

    def test_critical_path_linear_chain(self):
        """Test critical path with linear dependency chain."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-002-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        path = analyzer.find_critical_path(graph)

        assert path == ["T-001-0", "T-002-0", "T-003-0"]

    def test_critical_path_diamond_shape(self):
        """Test critical path with diamond-shaped dependencies."""
        # T-001 -> T-002 -> T-004
        # T-001 -> T-003 -> T-004
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-001-0"]),
            create_task("T-004-0", "Task 4", ["T-002-0", "T-003-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        path = analyzer.find_critical_path(graph)

        # Either T-001 -> T-002 -> T-004 or T-001 -> T-003 -> T-004
        assert len(path) == 3
        assert path[0] == "T-001-0"
        assert path[2] == "T-004-0"

    def test_critical_path_with_cycle_returns_empty(self):
        """Test that cyclic graphs return empty critical path."""
        analyzer = DependencyAnalyzer(MockTaskStore([]))

        # Manually create a cyclic graph
        graph = nx.DiGraph()
        graph.add_edge("A", "B")
        graph.add_edge("B", "C")
        graph.add_edge("C", "A")  # Creates cycle

        path = analyzer.find_critical_path(graph)

        assert path == []


class TestDependencyAnalyzerParallelGroups:
    """Tests for find_parallel_groups method."""

    def test_parallel_groups_empty_graph(self):
        """Test parallel groups on empty graph."""
        store = MockTaskStore([])
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        groups = analyzer.find_parallel_groups(graph)

        assert groups == []

    def test_parallel_groups_no_dependencies(self):
        """Test parallel groups when all tasks are independent."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2"),
            create_task("T-003-0", "Task 3"),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        groups = analyzer.find_parallel_groups(graph)

        # All tasks can run in parallel
        assert len(groups) == 1
        assert set(groups[0]) == {"T-001-0", "T-002-0", "T-003-0"}

    def test_parallel_groups_linear_chain(self):
        """Test parallel groups with linear chain."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-002-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        groups = analyzer.find_parallel_groups(graph)

        # Each task is its own group
        assert len(groups) == 3
        assert groups[0] == ["T-001-0"]
        assert groups[1] == ["T-002-0"]
        assert groups[2] == ["T-003-0"]

    def test_parallel_groups_diamond(self):
        """Test parallel groups with diamond shape."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-001-0"]),
            create_task("T-004-0", "Task 4", ["T-002-0", "T-003-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        groups = analyzer.find_parallel_groups(graph)

        assert len(groups) == 3
        assert groups[0] == ["T-001-0"]
        assert set(groups[1]) == {"T-002-0", "T-003-0"}  # These can run in parallel
        assert groups[2] == ["T-004-0"]


class TestDependencyAnalyzerBottlenecks:
    """Tests for detect_bottlenecks method."""

    def test_bottlenecks_empty_graph(self):
        """Test bottleneck detection on empty graph."""
        store = MockTaskStore([])
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        bottlenecks = analyzer.detect_bottlenecks(graph)

        assert bottlenecks == []

    def test_bottlenecks_no_high_fan_in(self):
        """Test when no tasks have high fan-in."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        bottlenecks = analyzer.detect_bottlenecks(graph, threshold=3)

        assert bottlenecks == []

    def test_bottlenecks_high_fan_in(self):
        """Test detection of task with high fan-in."""
        # T-004 depends on T-001, T-002, T-003 (fan-in = 3)
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2"),
            create_task("T-003-0", "Task 3"),
            create_task("T-004-0", "Task 4", ["T-001-0", "T-002-0", "T-003-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        bottlenecks = analyzer.detect_bottlenecks(graph, threshold=3)

        assert len(bottlenecks) == 1
        assert bottlenecks[0].task_id == "T-004-0"
        assert bottlenecks[0].fan_in == 3

    def test_bottlenecks_sorted_by_fan_in(self):
        """Test that bottlenecks are sorted by fan-in descending."""
        # Create tasks where T-005 has fan-in 4, T-004 has fan-in 3
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2"),
            create_task("T-003-0", "Task 3"),
            create_task("T-004-0", "Task 4", ["T-001-0", "T-002-0", "T-003-0"]),
            create_task("T-005-0", "Task 5", ["T-001-0", "T-002-0", "T-003-0", "T-004-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        bottlenecks = analyzer.detect_bottlenecks(graph, threshold=3)

        assert len(bottlenecks) == 2
        assert bottlenecks[0].task_id == "T-005-0"  # Higher fan-in first
        assert bottlenecks[0].fan_in == 4
        assert bottlenecks[1].task_id == "T-004-0"
        assert bottlenecks[1].fan_in == 3

    def test_bottlenecks_includes_blocking_tasks(self):
        """Test that bottleneck info includes tasks that are blocked."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2"),
            create_task("T-003-0", "Task 3"),
            create_task("T-004-0", "Task 4", ["T-001-0", "T-002-0", "T-003-0"]),
            create_task("T-005-0", "Task 5", ["T-004-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)
        graph = analyzer.build_graph("test-project")

        bottlenecks = analyzer.detect_bottlenecks(graph, threshold=3)

        assert len(bottlenecks) == 1
        assert bottlenecks[0].blocking_tasks == ["T-005-0"]


class TestDependencyAnalyzerAnalyze:
    """Tests for the analyze convenience method."""

    def test_analyze_returns_complete_result(self):
        """Test that analyze returns all analysis components."""
        tasks = [
            create_task("T-001-0", "Task 1"),
            create_task("T-002-0", "Task 2", ["T-001-0"]),
            create_task("T-003-0", "Task 3", ["T-001-0"]),
            create_task("T-004-0", "Task 4", ["T-002-0", "T-003-0"]),
        ]
        store = MockTaskStore(tasks)
        analyzer = DependencyAnalyzer(store)

        result = analyzer.analyze("test-project")

        assert result.graph is not None
        assert len(result.graph.nodes()) == 4
        assert len(result.critical_path) == 3
        assert len(result.parallel_groups) == 3
        # No bottlenecks with default threshold of 3 (max fan-in is 2)
        assert result.bottlenecks == []
