"""DAG to C4 Task conversion layer.

Converts a directed acyclic graph of GPU/ML jobs into C4 tasks with
proper dependencies and GPU configuration.
"""

from __future__ import annotations

import logging
from typing import Any

from c4.models.task import GpuTaskConfig

logger = logging.getLogger(__name__)


class DagNode:
    """A node in a DAG representing a single job step.

    Attributes:
        id: Unique node identifier.
        title: Human-readable title.
        command: Shell command to execute.
        dependencies: List of node IDs this node depends on.
        gpu_count: Number of GPUs required (0 = CPU only).
        min_vram_gb: Minimum VRAM per GPU.
        parallelism: GPU parallelism strategy (single, ddp, fsdp).
        timeout_minutes: Execution timeout.
        env: Environment variables.
    """

    def __init__(
        self,
        id: str,
        title: str = "",
        command: str = "",
        dependencies: list[str] | None = None,
        gpu_count: int = 0,
        min_vram_gb: int = 8,
        parallelism: str = "single",
        timeout_minutes: int = 60,
        env: dict[str, str] | None = None,
    ) -> None:
        self.id = id
        self.title = title or id
        self.command = command
        self.dependencies = dependencies or []
        self.gpu_count = gpu_count
        self.min_vram_gb = min_vram_gb
        self.parallelism = parallelism
        self.timeout_minutes = timeout_minutes
        self.env = env or {}


class Dag:
    """A directed acyclic graph of job nodes.

    Usage:
        dag = Dag(name="training-pipeline")
        dag.add_node(DagNode(id="preprocess", command="python preprocess.py"))
        dag.add_node(DagNode(id="train", command="python train.py", gpu_count=2,
                             dependencies=["preprocess"]))
        dag.add_node(DagNode(id="eval", command="python eval.py",
                             dependencies=["train"]))

        tasks = dag_to_tasks(dag, project_prefix="ML")
    """

    def __init__(self, name: str = "dag") -> None:
        self.name = name
        self.nodes: dict[str, DagNode] = {}

    def add_node(self, node: DagNode) -> None:
        """Add a node to the DAG.

        Raises:
            ValueError: If node ID already exists or has unknown dependencies.
        """
        if node.id in self.nodes:
            raise ValueError(f"Duplicate node ID: {node.id}")
        self.nodes[node.id] = node

    def validate(self) -> list[str]:
        """Validate DAG structure. Returns list of error messages."""
        errors: list[str] = []

        # Check dependencies exist
        for node in self.nodes.values():
            for dep in node.dependencies:
                if dep not in self.nodes:
                    errors.append(f"Node '{node.id}' depends on unknown node '{dep}'")

        # Check for cycles (DFS)
        visited: set[str] = set()
        in_stack: set[str] = set()

        def _dfs(nid: str) -> bool:
            if nid in in_stack:
                return True  # Cycle found
            if nid in visited:
                return False
            visited.add(nid)
            in_stack.add(nid)
            for dep in self.nodes[nid].dependencies:
                if dep in self.nodes and _dfs(dep):
                    return True
            in_stack.discard(nid)
            return False

        for nid in self.nodes:
            if _dfs(nid):
                errors.append(f"Cycle detected involving node '{nid}'")
                break

        return errors

    def topological_order(self) -> list[str]:
        """Return node IDs in topological order."""
        in_degree: dict[str, int] = {nid: 0 for nid in self.nodes}
        for node in self.nodes.values():
            for dep in node.dependencies:
                if dep in self.nodes:
                    in_degree[node.id] = in_degree.get(node.id, 0)

        # Kahn's algorithm
        for node in self.nodes.values():
            for dep in node.dependencies:
                if dep in in_degree:
                    in_degree[node.id] += 1

        # Recalculate properly
        in_degree = {nid: 0 for nid in self.nodes}
        reverse_deps: dict[str, list[str]] = {nid: [] for nid in self.nodes}
        for node in self.nodes.values():
            for dep in node.dependencies:
                if dep in self.nodes:
                    in_degree[node.id] += 1
                    reverse_deps[dep].append(node.id)

        queue = [nid for nid, deg in in_degree.items() if deg == 0]
        order: list[str] = []

        while queue:
            queue.sort()  # Deterministic ordering
            nid = queue.pop(0)
            order.append(nid)
            for follower in reverse_deps.get(nid, []):
                in_degree[follower] -= 1
                if in_degree[follower] == 0:
                    queue.append(follower)

        return order


def dag_to_tasks(
    dag: Dag,
    project_prefix: str = "DAG",
    base_number: int = 1,
    priority_base: int = 100,
) -> list[dict[str, Any]]:
    """Convert DAG to C4 task definitions.

    Args:
        dag: The DAG to convert.
        project_prefix: Prefix for generated task IDs.
        base_number: Starting task number.
        priority_base: Base priority (earlier tasks get higher priority).

    Returns:
        List of task definition dicts ready for c4_add_todo.

    Raises:
        ValueError: If DAG validation fails.
    """
    errors = dag.validate()
    if errors:
        raise ValueError(f"Invalid DAG: {'; '.join(errors)}")

    order = dag.topological_order()
    node_to_task_id: dict[str, str] = {}

    # Assign task IDs
    for i, nid in enumerate(order):
        task_num = base_number + i
        node_to_task_id[nid] = f"T-{project_prefix}-{task_num:03d}-0"

    tasks: list[dict[str, Any]] = []
    for i, nid in enumerate(order):
        node = dag.nodes[nid]
        task_id = node_to_task_id[nid]

        # Map dependencies to task IDs
        dep_task_ids = [node_to_task_id[d] for d in node.dependencies if d in node_to_task_id]

        # Build GPU config if needed
        gpu_config = None
        if node.gpu_count > 0:
            gpu_config = GpuTaskConfig(
                gpu_count=node.gpu_count,
                min_vram_gb=node.min_vram_gb,
                parallelism=node.parallelism,
                timeout_minutes=node.timeout_minutes,
            )

        task = {
            "id": task_id,
            "title": node.title,
            "dod": f"- [ ] Execute: {node.command}\n- [ ] Exit code 0\n- [ ] No errors in stderr",
            "dependencies": dep_task_ids,
            "priority": priority_base - i,  # Earlier tasks get higher priority
            "gpu_config": gpu_config.model_dump() if gpu_config else None,
            "metadata": {
                "dag_name": dag.name,
                "dag_node_id": nid,
                "command": node.command,
                "env": node.env,
            },
        }
        tasks.append(task)

    logger.info(
        "Converted DAG '%s' to %d tasks (%d with GPU)",
        dag.name,
        len(tasks),
        sum(1 for t in tasks if t["gpu_config"]),
    )
    return tasks
