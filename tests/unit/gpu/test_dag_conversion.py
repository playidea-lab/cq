"""Tests for DAG to C4 Task conversion."""

import pytest

from c4.gpu.dag import Dag, DagNode, dag_to_tasks


class TestDagNode:
    def test_default_node(self):
        node = DagNode(id="n1")
        assert node.id == "n1"
        assert node.title == "n1"
        assert node.gpu_count == 0
        assert node.dependencies == []

    def test_gpu_node(self):
        node = DagNode(id="train", gpu_count=2, min_vram_gb=16, parallelism="ddp")
        assert node.gpu_count == 2
        assert node.min_vram_gb == 16
        assert node.parallelism == "ddp"


class TestDag:
    def test_add_node(self):
        dag = Dag(name="test")
        dag.add_node(DagNode(id="a"))
        assert "a" in dag.nodes

    def test_duplicate_node_raises(self):
        dag = Dag()
        dag.add_node(DagNode(id="a"))
        with pytest.raises(ValueError, match="Duplicate"):
            dag.add_node(DagNode(id="a"))

    def test_validate_missing_dependency(self):
        dag = Dag()
        dag.add_node(DagNode(id="a", dependencies=["nonexistent"]))
        errors = dag.validate()
        assert len(errors) == 1
        assert "unknown node" in errors[0]

    def test_validate_cycle(self):
        dag = Dag()
        dag.add_node(DagNode(id="a", dependencies=["b"]))
        dag.add_node(DagNode(id="b", dependencies=["a"]))
        errors = dag.validate()
        assert any("Cycle" in e for e in errors)

    def test_validate_clean(self):
        dag = Dag()
        dag.add_node(DagNode(id="a"))
        dag.add_node(DagNode(id="b", dependencies=["a"]))
        errors = dag.validate()
        assert errors == []

    def test_topological_order_linear(self):
        dag = Dag()
        dag.add_node(DagNode(id="c", dependencies=["b"]))
        dag.add_node(DagNode(id="b", dependencies=["a"]))
        dag.add_node(DagNode(id="a"))
        order = dag.topological_order()
        assert order == ["a", "b", "c"]

    def test_topological_order_diamond(self):
        dag = Dag()
        dag.add_node(DagNode(id="a"))
        dag.add_node(DagNode(id="b", dependencies=["a"]))
        dag.add_node(DagNode(id="c", dependencies=["a"]))
        dag.add_node(DagNode(id="d", dependencies=["b", "c"]))
        order = dag.topological_order()
        assert order.index("a") < order.index("b")
        assert order.index("a") < order.index("c")
        assert order.index("b") < order.index("d")
        assert order.index("c") < order.index("d")


class TestDagToTasks:
    def test_simple_pipeline(self):
        dag = Dag(name="simple")
        dag.add_node(DagNode(id="preprocess", command="python preprocess.py"))
        dag.add_node(DagNode(id="train", command="python train.py",
                             dependencies=["preprocess"], gpu_count=1))

        tasks = dag_to_tasks(dag, project_prefix="ML")
        assert len(tasks) == 2

        # First task (preprocess) has no dependencies
        assert tasks[0]["dependencies"] == []
        assert tasks[0]["gpu_config"] is None

        # Second task (train) depends on first
        assert len(tasks[1]["dependencies"]) == 1
        assert tasks[1]["gpu_config"] is not None
        assert tasks[1]["gpu_config"]["gpu_count"] == 1

    def test_task_ids_format(self):
        dag = Dag()
        dag.add_node(DagNode(id="a"))
        dag.add_node(DagNode(id="b", dependencies=["a"]))

        tasks = dag_to_tasks(dag, project_prefix="TEST", base_number=5)
        assert tasks[0]["id"] == "T-TEST-005-0"
        assert tasks[1]["id"] == "T-TEST-006-0"

    def test_priority_ordering(self):
        dag = Dag()
        dag.add_node(DagNode(id="a"))
        dag.add_node(DagNode(id="b", dependencies=["a"]))
        dag.add_node(DagNode(id="c", dependencies=["b"]))

        tasks = dag_to_tasks(dag, priority_base=100)
        # Earlier tasks get higher priority
        assert tasks[0]["priority"] > tasks[1]["priority"]
        assert tasks[1]["priority"] > tasks[2]["priority"]

    def test_invalid_dag_raises(self):
        dag = Dag()
        dag.add_node(DagNode(id="a", dependencies=["missing"]))

        with pytest.raises(ValueError, match="Invalid DAG"):
            dag_to_tasks(dag)

    def test_metadata_preserved(self):
        dag = Dag(name="my-dag")
        dag.add_node(DagNode(id="step1", command="echo hello",
                             env={"CUDA_VISIBLE_DEVICES": "0"}))

        tasks = dag_to_tasks(dag)
        meta = tasks[0]["metadata"]
        assert meta["dag_name"] == "my-dag"
        assert meta["dag_node_id"] == "step1"
        assert meta["command"] == "echo hello"
        assert meta["env"]["CUDA_VISIBLE_DEVICES"] == "0"

    def test_gpu_config_details(self):
        dag = Dag()
        dag.add_node(DagNode(id="train", gpu_count=4, min_vram_gb=24,
                             parallelism="fsdp", timeout_minutes=120))

        tasks = dag_to_tasks(dag)
        gpu = tasks[0]["gpu_config"]
        assert gpu["gpu_count"] == 4
        assert gpu["min_vram_gb"] == 24
        assert gpu["parallelism"] == "fsdp"
        assert gpu["timeout_minutes"] == 120

    def test_empty_dag(self):
        dag = Dag()
        tasks = dag_to_tasks(dag)
        assert tasks == []
