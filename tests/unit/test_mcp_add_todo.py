"""Tests for c4_add_todo MCP tool with DDD-CLEANCODE fields."""

from __future__ import annotations

import warnings
from unittest.mock import MagicMock

import pytest

from c4.mcp_server import C4Daemon
from c4.models.task import Task


@pytest.fixture
def mock_state_machine() -> MagicMock:
    """Create a mock state machine."""
    mock = MagicMock()
    mock.tasks = {}
    return mock


@pytest.fixture
def daemon(mock_state_machine: MagicMock) -> C4Daemon:
    """Create a C4Daemon with mock state machine."""
    d = C4Daemon()
    d.state_machine = mock_state_machine
    d._tasks = {}

    def add_task(task: Task) -> None:
        d._tasks[task.id] = task

    d.add_task = add_task  # type: ignore
    return d


class TestC4AddTodoBasic:
    """Basic c4_add_todo tests."""

    def test_add_todo_basic(self, daemon: C4Daemon) -> None:
        """Basic task creation works."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope="src/",
            dod="Implement feature",
        )

        assert result["success"] is True
        assert result["task_id"] == "T-001-0"
        assert "T-001-0" in daemon._tasks  # type: ignore

    def test_add_todo_with_dependencies(self, daemon: C4Daemon) -> None:
        """Dependencies are normalized."""
        result = daemon.c4_add_todo(
            task_id="T-002",
            title="Dependent task",
            scope=None,
            dod="Depends on T-001",
            dependencies=["T-001"],
        )

        assert result["success"] is True
        assert result["dependencies"] == ["T-001-0"]


class TestC4AddTodoDDDCLEANCODE:
    """Tests for DDD-CLEANCODE fields."""

    def test_add_todo_with_goal(self, daemon: C4Daemon) -> None:
        """Goal specification is stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            goal={
                "done": "POST /v1/users returns 201",
                "out_of_scope": "Email verification",
            },
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.goal is not None
        assert task.goal.done == "POST /v1/users returns 201"
        assert task.goal.out_of_scope == "Email verification"

    def test_add_todo_with_contract_spec(self, daemon: C4Daemon) -> None:
        """Contract specification is stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            contract_spec={
                "apis": [
                    {
                        "name": "UserService.register",
                        "input": "email: str, password: str",
                        "output": "User",
                        "errors": ["DuplicateEmail", "WeakPassword"],
                    }
                ],
                "tests": {
                    "success": ["test_register_ok"],
                    "failure": ["test_register_duplicate"],
                    "boundary": ["test_register_max_length"],
                },
            },
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.contract_spec is not None
        assert len(task.contract_spec.apis) == 1
        assert task.contract_spec.apis[0].name == "UserService.register"
        assert task.contract_spec.tests.success == ["test_register_ok"]

    def test_add_todo_with_boundary_map(self, daemon: C4Daemon) -> None:
        """Boundary map is stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            boundary_map={
                "target_domain": "auth",
                "target_layer": "app",
                "allowed_imports": ["stdlib", "pydantic"],
                "forbidden_imports": ["sqlalchemy", "httpx"],
                "public_export": "src/api/v1/users.py",
            },
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.boundary_map is not None
        assert task.boundary_map.target_domain == "auth"
        assert task.boundary_map.target_layer == "app"
        assert "sqlalchemy" in task.boundary_map.forbidden_imports

    def test_add_todo_with_code_placement(self, daemon: C4Daemon) -> None:
        """Code placement is stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            code_placement={
                "create": ["src/auth/service.py", "src/auth/domain/user.py"],
                "modify": ["src/api/v1/users.py"],
                "tests": ["tests/unit/auth/test_service.py"],
            },
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.code_placement is not None
        assert "src/auth/service.py" in task.code_placement.create
        assert task.get_files_to_create() == ["src/auth/service.py", "src/auth/domain/user.py"]

    def test_add_todo_with_quality_gates(self, daemon: C4Daemon) -> None:
        """Quality gates are stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            quality_gates=[
                {"name": "lint", "command": "uv run ruff check .", "required": True},
                {"name": "test", "command": "uv run pytest", "required": True},
            ],
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert len(task.quality_gates) == 2
        assert task.quality_gates[0].name == "lint"
        assert task.quality_gates[0].required is True

    def test_add_todo_with_checkpoints(self, daemon: C4Daemon) -> None:
        """Checkpoints are stored."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            checkpoints={
                "cp1_skeleton": "파일 생성 + 테스트 골격",
                "cp2_green": "핵심 테스트 통과",
                "cp3_harden": "에러/경계 테스트 추가",
            },
        )

        assert result["success"] is True
        task = daemon._tasks["T-001-0"]  # type: ignore
        assert task.checkpoints is not None
        assert "파일 생성" in task.checkpoints.cp1_skeleton

    def test_add_todo_fully_specified(self, daemon: C4Daemon) -> None:
        """Fully specified task returns fully_specified=True."""
        result = daemon.c4_add_todo(
            task_id="T-001",
            title="Test task",
            scope=None,
            dod="legacy dod",
            goal={"done": "Done", "out_of_scope": "Not done"},
            contract_spec={
                "apis": [{"name": "Foo.bar", "input": "x", "output": "y"}],
                "tests": {"success": ["test_ok"], "failure": ["test_fail"], "boundary": ["test_edge"]},
            },
            boundary_map={
                "target_domain": "core",
                "target_layer": "app",
                "allowed_imports": ["stdlib"],
                "forbidden_imports": ["external"],
            },
            code_placement={
                "create": ["src/foo.py"],
                "modify": [],
                "tests": ["tests/test_foo.py"],
            },
            quality_gates=[{"name": "lint", "command": "ruff", "required": True}],
            checkpoints={"cp1_skeleton": "A", "cp2_green": "B", "cp3_harden": "C"},
        )

        assert result["success"] is True
        assert result["fully_specified"] is True


class TestC4AddTodoDeprecation:
    """Tests for deprecation warning."""

    def test_dod_without_goal_emits_warning(self, daemon: C4Daemon) -> None:
        """Using dod without goal emits DeprecationWarning."""
        with warnings.catch_warnings(record=True) as w:
            warnings.simplefilter("always")
            daemon.c4_add_todo(
                task_id="T-001",
                title="Legacy task",
                scope=None,
                dod="old style dod",
            )

            assert len(w) == 1
            assert issubclass(w[0].category, DeprecationWarning)
            assert "dod" in str(w[0].message).lower()
            assert "deprecated" in str(w[0].message).lower()

    def test_dod_with_goal_no_warning(self, daemon: C4Daemon) -> None:
        """Using dod with goal does not emit warning."""
        with warnings.catch_warnings(record=True) as w:
            warnings.simplefilter("always")
            daemon.c4_add_todo(
                task_id="T-001",
                title="New task",
                scope=None,
                dod="for compatibility",
                goal={"done": "Done", "out_of_scope": "Not done"},
            )

            # Filter only DeprecationWarning related to dod
            dod_warnings = [x for x in w if "dod" in str(x.message).lower()]
            assert len(dod_warnings) == 0
