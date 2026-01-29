"""Dependency analysis for C4 tasks.

This module provides tools for analyzing task dependencies, finding critical paths,
identifying parallel execution opportunities, and detecting bottlenecks.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

import networkx as nx

if TYPE_CHECKING:
    from c4.store.base import TaskStore


@dataclass
class BottleneckInfo:
    """Information about a bottleneck task.

    Attributes:
        task_id: The ID of the bottleneck task.
        fan_in: Number of tasks that this task depends on.
        blocking_tasks: List of task IDs that are blocked by this task.
    """

    task_id: str
    fan_in: int
    blocking_tasks: list[str]


@dataclass
class AnalysisResult:
    """Result of dependency analysis.

    Attributes:
        graph: The NetworkX DiGraph representing task dependencies.
        critical_path: List of task IDs forming the longest dependency chain.
        parallel_groups: List of task ID groups that can execute in parallel.
        bottlenecks: List of BottleneckInfo for tasks with high fan-in.
    """

    graph: nx.DiGraph
    critical_path: list[str]
    parallel_groups: list[list[str]]
    bottlenecks: list[BottleneckInfo]


class DependencyAnalyzer:
    """Analyzes task dependencies for optimization opportunities.

    This class builds a dependency graph from tasks and provides methods to:
    - Find the critical path (longest dependency chain)
    - Identify parallel execution groups
    - Detect bottleneck tasks (high fan-in)

    Example:
        ```python
        analyzer = DependencyAnalyzer(task_store)
        graph = analyzer.build_graph(project_id)

        # Find critical path
        critical_path = analyzer.find_critical_path(graph)

        # Find parallel groups
        parallel_groups = analyzer.find_parallel_groups(graph)

        # Detect bottlenecks
        bottlenecks = analyzer.detect_bottlenecks(graph)
        ```
    """

    def __init__(self, task_store: TaskStore) -> None:
        """Initialize the DependencyAnalyzer.

        Args:
            task_store: The task store to retrieve tasks from.
        """
        self._task_store = task_store

    def build_graph(self, project_id: str) -> nx.DiGraph:
        """Build a dependency graph from tasks in the store.

        Creates a directed graph where:
        - Nodes are task IDs
        - Edges point from dependency to dependent (A -> B means A must complete before B)

        Args:
            project_id: The project ID to load tasks for.

        Returns:
            A NetworkX DiGraph representing task dependencies.
        """
        graph = nx.DiGraph()

        # Load all tasks from the store
        tasks = self._task_store.load_all(project_id)

        # Add all tasks as nodes with their data
        for task in tasks:
            graph.add_node(
                task.id,
                title=task.title,
                status=task.status.value if hasattr(task.status, "value") else str(task.status),
                priority=task.priority,
            )

        # Add edges for dependencies
        for task in tasks:
            for dep_id in task.dependencies:
                # Edge from dependency to this task (dep must complete first)
                if dep_id in graph:
                    graph.add_edge(dep_id, task.id)

        return graph

    def find_critical_path(self, graph: nx.DiGraph) -> list[str]:
        """Find the critical path (longest dependency chain) in the graph.

        The critical path determines the minimum time to complete all tasks,
        assuming each task takes the same amount of time.

        Args:
            graph: The dependency graph.

        Returns:
            List of task IDs forming the longest path from any source to any sink.
            Returns empty list if graph is empty or has cycles.
        """
        if not graph.nodes():
            return []

        # Check for cycles - critical path only defined for DAGs
        if not nx.is_directed_acyclic_graph(graph):
            return []

        # Find longest path using dag_longest_path
        try:
            longest_path = nx.dag_longest_path(graph)
            return list(longest_path)
        except nx.NetworkXError:
            return []

    def find_parallel_groups(self, graph: nx.DiGraph) -> list[list[str]]:
        """Find groups of tasks that can execute in parallel.

        Tasks are grouped by their topological level - tasks at the same level
        have no dependencies on each other and can run concurrently.

        Args:
            graph: The dependency graph.

        Returns:
            List of task ID groups, where each group can execute in parallel.
            Groups are ordered by topological level (earlier groups first).
            Returns empty list if graph has cycles.
        """
        if not graph.nodes():
            return []

        # Check for cycles
        if not nx.is_directed_acyclic_graph(graph):
            return []

        # Use topological generations to group tasks by level
        try:
            generations = list(nx.topological_generations(graph))
            return [list(gen) for gen in generations]
        except nx.NetworkXError:
            return []

    def detect_bottlenecks(self, graph: nx.DiGraph, threshold: int = 3) -> list[BottleneckInfo]:
        """Detect bottleneck tasks with high fan-in.

        A bottleneck is a task that many other tasks depend on (high out-degree)
        or that depends on many tasks (high in-degree >= threshold).

        Args:
            graph: The dependency graph.
            threshold: Minimum fan-in to be considered a bottleneck. Default is 3.

        Returns:
            List of BottleneckInfo for tasks with fan-in >= threshold,
            sorted by fan-in in descending order.
        """
        bottlenecks: list[BottleneckInfo] = []

        for node in graph.nodes():
            # Fan-in is the number of incoming edges (dependencies)
            fan_in = graph.in_degree(node)

            if fan_in >= threshold:
                # Find tasks that are blocked by this task (successors)
                blocking_tasks = list(graph.successors(node))

                bottlenecks.append(
                    BottleneckInfo(
                        task_id=node,
                        fan_in=fan_in,
                        blocking_tasks=blocking_tasks,
                    )
                )

        # Sort by fan_in descending
        bottlenecks.sort(key=lambda b: b.fan_in, reverse=True)

        return bottlenecks

    def analyze(self, project_id: str, bottleneck_threshold: int = 3) -> AnalysisResult:
        """Perform complete dependency analysis.

        This is a convenience method that runs all analysis methods and
        returns a combined result.

        Args:
            project_id: The project ID to analyze.
            bottleneck_threshold: Minimum fan-in for bottleneck detection.

        Returns:
            AnalysisResult containing graph, critical path, parallel groups,
            and bottlenecks.
        """
        graph = self.build_graph(project_id)

        return AnalysisResult(
            graph=graph,
            critical_path=self.find_critical_path(graph),
            parallel_groups=self.find_parallel_groups(graph),
            bottlenecks=self.detect_bottlenecks(graph, bottleneck_threshold),
        )
