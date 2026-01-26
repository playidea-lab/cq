"""Integration tests for DDD-CLEANCODE fields persistence in SQLiteTaskStore."""

from __future__ import annotations

import tempfile
from pathlib import Path

import pytest

from c4.models.ddd import (
    BoundaryMap,
    CheckpointDefinition,
    CodePlacement,
    ContractSpec,
    ApiSpec,
    Goal,
    QualityGate,
    RequiredTests,
)
from c4.models.task import Task
from c4.store.sqlite import SQLiteTaskStore


@pytest.fixture
def temp_db() -> Path:
    """Create a temporary database file."""
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        return Path(f.name)


@pytest.fixture
def task_store(temp_db: Path) -> SQLiteTaskStore:
    """Create a SQLiteTaskStore with temporary database."""
    return SQLiteTaskStore(temp_db)


@pytest.fixture
def fully_specified_task() -> Task:
    """Create a fully specified task with all DDD-CLEANCODE fields."""
    return Task(
        id="T-001-0",
        title="Implement user registration",
        scope="src/auth/",
        dod="Legacy DoD for compatibility",
        dependencies=[],
        goal=Goal(
            done="POST /v1/users returns 201 with created user",
            out_of_scope="Email verification (T-002)",
        ),
        contract_spec=ContractSpec(
            apis=[
                ApiSpec(
                    name="UserService.register",
                    input="email: str, password: str",
                    output="User",
                    errors=["DuplicateEmail", "WeakPassword"],
                    side_effects="Creates user record in DB",
                ),
                ApiSpec(
                    name="UserService.validate_email",
                    input="email: str",
                    output="bool",
                ),
            ],
            tests=RequiredTests(
                success=["test_register_creates_user", "test_register_returns_user_data"],
                failure=["test_register_duplicate_email", "test_register_weak_password"],
                boundary=["test_register_max_email_length", "test_register_unicode_name"],
            ),
        ),
        boundary_map=BoundaryMap(
            target_domain="auth",
            target_layer="app",
            allowed_imports=["stdlib", "pydantic", "domain.user"],
            forbidden_imports=["sqlalchemy", "httpx", "fastapi"],
            public_export="src/api/v1/users.py",
        ),
        code_placement=CodePlacement(
            create=["src/auth/service.py", "src/auth/domain/user.py"],
            modify=["src/api/v1/users.py", "src/api/v1/__init__.py"],
            tests=["tests/unit/auth/test_service.py", "tests/unit/auth/test_user.py"],
        ),
        quality_gates=[
            QualityGate(name="format", command="uv run ruff format --check .", required=True),
            QualityGate(name="lint", command="uv run ruff check .", required=True),
            QualityGate(name="typecheck", command="uv run pyright .", required=False),
            QualityGate(name="unit", command="uv run pytest tests/unit/auth/ -v", required=True),
        ],
        checkpoints=CheckpointDefinition(
            cp1_skeleton="파일 생성 + 테스트 골격 (실패 OK)",
            cp2_green="register() 성공 테스트 통과",
            cp3_harden="실패/경계 테스트 추가 및 통과",
        ),
    )


class TestDDDFieldsPersistence:
    """Test that DDD-CLEANCODE fields are properly saved and loaded."""

    def test_save_and_load_preserves_goal(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """Goal field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify
        assert loaded is not None
        assert loaded.goal is not None
        assert loaded.goal.done == "POST /v1/users returns 201 with created user"
        assert loaded.goal.out_of_scope == "Email verification (T-002)"

    def test_save_and_load_preserves_contract_spec(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """ContractSpec field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify APIs
        assert loaded is not None
        assert loaded.contract_spec is not None
        assert len(loaded.contract_spec.apis) == 2
        assert loaded.contract_spec.apis[0].name == "UserService.register"
        assert loaded.contract_spec.apis[0].input == "email: str, password: str"
        assert loaded.contract_spec.apis[0].output == "User"
        assert "DuplicateEmail" in loaded.contract_spec.apis[0].errors
        assert loaded.contract_spec.apis[0].side_effects == "Creates user record in DB"

        # Verify Tests
        assert loaded.contract_spec.tests.success == [
            "test_register_creates_user",
            "test_register_returns_user_data",
        ]
        assert loaded.contract_spec.tests.failure == [
            "test_register_duplicate_email",
            "test_register_weak_password",
        ]
        assert "test_register_max_email_length" in loaded.contract_spec.tests.boundary

    def test_save_and_load_preserves_boundary_map(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """BoundaryMap field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify
        assert loaded is not None
        assert loaded.boundary_map is not None
        assert loaded.boundary_map.target_domain == "auth"
        assert loaded.boundary_map.target_layer == "app"
        assert "stdlib" in loaded.boundary_map.allowed_imports
        assert "pydantic" in loaded.boundary_map.allowed_imports
        assert "sqlalchemy" in loaded.boundary_map.forbidden_imports
        assert "httpx" in loaded.boundary_map.forbidden_imports
        assert loaded.boundary_map.public_export == "src/api/v1/users.py"

    def test_save_and_load_preserves_code_placement(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """CodePlacement field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify
        assert loaded is not None
        assert loaded.code_placement is not None
        assert "src/auth/service.py" in loaded.code_placement.create
        assert "src/auth/domain/user.py" in loaded.code_placement.create
        assert "src/api/v1/users.py" in loaded.code_placement.modify
        assert "tests/unit/auth/test_service.py" in loaded.code_placement.tests

    def test_save_and_load_preserves_quality_gates(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """QualityGates field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify
        assert loaded is not None
        assert len(loaded.quality_gates) == 4

        gate_by_name = {g.name: g for g in loaded.quality_gates}
        assert gate_by_name["format"].command == "uv run ruff format --check ."
        assert gate_by_name["format"].required is True
        assert gate_by_name["lint"].command == "uv run ruff check ."
        assert gate_by_name["typecheck"].required is False
        assert gate_by_name["unit"].command == "uv run pytest tests/unit/auth/ -v"

    def test_save_and_load_preserves_checkpoints(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """Checkpoints field is preserved after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify
        assert loaded is not None
        assert loaded.checkpoints is not None
        assert "파일 생성" in loaded.checkpoints.cp1_skeleton
        assert "테스트 골격" in loaded.checkpoints.cp1_skeleton
        assert "성공 테스트 통과" in loaded.checkpoints.cp2_green
        assert "실패/경계 테스트" in loaded.checkpoints.cp3_harden

    def test_save_and_load_all_preserves_ddd_fields(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """All DDD fields are preserved when using load_all()."""
        project_id = "test-project"

        # Create second task
        task2 = Task(
            id="T-002-0",
            title="Email verification",
            dod="Implement email verification",
            dependencies=["T-001-0"],
            goal=Goal(done="Email sent on registration", out_of_scope="SMS verification"),
        )

        # Save both
        task_store.save(project_id, fully_specified_task)
        task_store.save(project_id, task2)

        # Load all
        all_tasks = task_store.load_all(project_id)

        # Verify
        assert len(all_tasks) == 2
        assert "T-001-0" in all_tasks
        assert "T-002-0" in all_tasks

        # Verify T-001-0 DDD fields
        t1 = all_tasks["T-001-0"]
        assert t1.goal is not None
        assert t1.contract_spec is not None
        assert t1.boundary_map is not None
        assert t1.code_placement is not None
        assert len(t1.quality_gates) == 4
        assert t1.checkpoints is not None

        # Verify T-002-0 DDD fields
        t2 = all_tasks["T-002-0"]
        assert t2.goal is not None
        assert t2.goal.done == "Email sent on registration"

    def test_task_without_ddd_fields_loads_with_defaults(
        self, task_store: SQLiteTaskStore
    ) -> None:
        """Tasks without DDD fields load with None/empty defaults."""
        project_id = "test-project"

        # Create minimal task (legacy style)
        minimal_task = Task(
            id="T-003-0",
            title="Legacy task",
            dod="Simple DoD",
            dependencies=[],
        )

        # Save
        task_store.save(project_id, minimal_task)

        # Load
        loaded = task_store.get(project_id, minimal_task.id)

        # Verify defaults
        assert loaded is not None
        assert loaded.goal is None
        assert loaded.contract_spec is None
        assert loaded.boundary_map is None
        assert loaded.code_placement is None
        assert loaded.quality_gates == []
        assert loaded.checkpoints is None

    def test_is_fully_specified_after_load(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """is_fully_specified() returns True after save/load cycle."""
        project_id = "test-project"

        # Verify before save
        assert fully_specified_task.is_fully_specified() is True

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)

        # Verify after load
        assert loaded is not None
        assert loaded.is_fully_specified() is True

    def test_to_worker_packet_after_load(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """to_worker_packet() returns complete packet after save/load cycle."""
        project_id = "test-project"

        # Save
        task_store.save(project_id, fully_specified_task)

        # Load
        loaded = task_store.get(project_id, fully_specified_task.id)
        assert loaded is not None

        # Get worker packet (returns WorkerPacket Pydantic model)
        # WorkerPacket contains only DDD-CLEANCODE specification fields
        packet = loaded.to_worker_packet()

        # Verify packet structure (no id/title - those are Task-level fields)
        assert packet.goal is not None
        assert packet.goal.done == "POST /v1/users returns 201 with created user"
        assert packet.contract_spec is not None
        assert packet.contract_spec.apis[0].name == "UserService.register"
        assert packet.boundary_map is not None
        assert packet.boundary_map.target_domain == "auth"
        assert packet.code_placement is not None
        assert packet.code_placement.create[0] == "src/auth/service.py"
        assert len(packet.quality_gates) == 4
        assert packet.checkpoints is not None
        assert "파일 생성" in packet.checkpoints.cp1_skeleton

        # Verify packet completeness
        assert packet.is_fully_specified() is True


class TestDDDFieldsUpdateCycle:
    """Test update scenarios for DDD-CLEANCODE fields."""

    def test_update_preserves_ddd_fields(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """Updating status preserves DDD fields."""
        project_id = "test-project"

        # Save initial
        task_store.save(project_id, fully_specified_task)

        # Update status
        task_store.update_status(
            project_id,
            fully_specified_task.id,
            status="in_progress",
            assigned_to="worker-1",
            branch="c4/w-T-001-0",
        )

        # Load and verify DDD fields preserved
        loaded = task_store.get(project_id, fully_specified_task.id)

        assert loaded is not None
        assert loaded.assigned_to == "worker-1"
        assert loaded.branch == "c4/w-T-001-0"

        # DDD fields should be intact
        assert loaded.goal is not None
        assert loaded.goal.done == "POST /v1/users returns 201 with created user"
        assert loaded.contract_spec is not None
        assert len(loaded.contract_spec.apis) == 2
        assert loaded.boundary_map is not None
        assert loaded.boundary_map.target_domain == "auth"

    def test_update_commit_info_preserves_ddd_fields(
        self, task_store: SQLiteTaskStore, fully_specified_task: Task
    ) -> None:
        """Updating commit info preserves DDD fields."""
        project_id = "test-project"

        # Save initial
        task_store.save(project_id, fully_specified_task)

        # Update commit info
        task_store.update_commit_info(
            project_id,
            fully_specified_task.id,
            commit_sha="abc123def456",
            branch="c4/w-T-001-0",
        )

        # Load and verify
        loaded = task_store.get(project_id, fully_specified_task.id)

        assert loaded is not None
        assert loaded.commit_sha == "abc123def456"
        assert loaded.branch == "c4/w-T-001-0"

        # DDD fields should be intact
        assert loaded.is_fully_specified() is True
        assert loaded.checkpoints is not None
        assert "파일 생성" in loaded.checkpoints.cp1_skeleton
