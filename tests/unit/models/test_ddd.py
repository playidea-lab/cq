"""Tests for DDD-CLEANCODE models."""


from c4.models.ddd import (
    ApiSpec,
    BoundaryMap,
    CheckpointDefinition,
    CodePlacement,
    ContractSpec,
    DoDItem,
    Goal,
    QualityGate,
    RequiredTests,
    WorkBreakdownCriteria,
    WorkerPacket,
)


class TestGoal:
    """Tests for Goal model."""

    def test_create_goal_with_done_only(self):
        """Goal can be created with just done field."""
        goal = Goal(done="POST /v1/users 호출 시 User가 생성된다")
        assert goal.done == "POST /v1/users 호출 시 User가 생성된다"
        assert goal.out_of_scope is None

    def test_create_goal_with_out_of_scope(self):
        """Goal can include out_of_scope."""
        goal = Goal(
            done="사용자 등록 완료",
            out_of_scope="이메일 검증은 T-002에서",
        )
        assert goal.out_of_scope == "이메일 검증은 T-002에서"


class TestApiSpec:
    """Tests for ApiSpec model."""

    def test_create_api_spec_minimal(self):
        """ApiSpec with minimal fields."""
        api = ApiSpec(
            name="UserService.register",
            input="email: str, password: str",
            output="User",
        )
        assert api.name == "UserService.register"
        assert api.errors == []
        assert api.side_effects is None

    def test_create_api_spec_full(self):
        """ApiSpec with all fields."""
        api = ApiSpec(
            name="UserService.register",
            input="email: str, password: str",
            output="User | None",
            errors=["DuplicateEmail", "WeakPassword"],
            side_effects="DB에 user 레코드 생성",
        )
        assert len(api.errors) == 2
        assert api.side_effects is not None


class TestRequiredTests:
    """Tests for RequiredTests model."""

    def test_create_test_spec(self):
        """RequiredTests requires success, failure, boundary."""
        spec = RequiredTests(
            success=["test_register_success"],
            failure=["test_register_duplicate"],
            boundary=["test_register_max_length"],
        )
        assert len(spec.success) == 1
        assert len(spec.failure) == 1
        assert len(spec.boundary) == 1


class TestContractSpec:
    """Tests for ContractSpec model."""

    def test_create_contract_spec(self):
        """ContractSpec with APIs and tests."""
        spec = ContractSpec(
            apis=[
                ApiSpec(
                    name="UserService.register",
                    input="email: str",
                    output="User",
                )
            ],
            tests=RequiredTests(
                success=["test_success"],
                failure=["test_failure"],
                boundary=["test_boundary"],
            ),
        )
        assert len(spec.apis) == 1
        assert spec.tests is not None


class TestBoundaryMap:
    """Tests for BoundaryMap model."""

    def test_create_boundary_map_minimal(self):
        """BoundaryMap with required fields."""
        boundary = BoundaryMap(
            target_domain="auth",
            target_layer="app",
        )
        assert boundary.target_domain == "auth"
        assert boundary.target_layer == "app"
        assert "stdlib" in boundary.allowed_imports

    def test_create_boundary_map_full(self):
        """BoundaryMap with all constraints."""
        boundary = BoundaryMap(
            target_domain="auth",
            target_layer="domain",
            allowed_imports=["stdlib", "pydantic", "domain.user"],
            forbidden_imports=["sqlalchemy", "httpx"],
            public_export="src/api/v1/users.py",
        )
        assert "sqlalchemy" in boundary.forbidden_imports
        assert boundary.public_export is not None


class TestCodePlacement:
    """Tests for CodePlacement model."""

    def test_create_code_placement(self):
        """CodePlacement with file lists."""
        placement = CodePlacement(
            create=["src/auth/service.py"],
            modify=["src/api/v1/users.py"],
            tests=["tests/unit/auth/test_service.py"],
        )
        assert len(placement.create) == 1
        assert len(placement.modify) == 1
        assert len(placement.tests) == 1


class TestQualityGate:
    """Tests for QualityGate model."""

    def test_create_quality_gate_defaults(self):
        """QualityGate with defaults."""
        gate = QualityGate(
            name="lint",
            command="uv run ruff check .",
        )
        assert gate.required is True
        assert gate.timeout_seconds == 300

    def test_create_quality_gate_custom(self):
        """QualityGate with custom settings."""
        gate = QualityGate(
            name="slow_test",
            command="uv run pytest -v",
            required=False,
            timeout_seconds=600,
        )
        assert gate.required is False
        assert gate.timeout_seconds == 600


class TestCheckpointDefinition:
    """Tests for CheckpointDefinition model."""

    def test_create_checkpoint_definition(self):
        """CheckpointDefinition with all stages."""
        cp = CheckpointDefinition(
            cp1_skeleton="파일 배치 + 테스트 골격",
            cp2_green="register() 성공 테스트 통과",
            cp3_harden="실패/경계 테스트 추가",
        )
        assert "골격" in cp.cp1_skeleton
        assert "통과" in cp.cp2_green
        assert "테스트" in cp.cp3_harden


class TestDoDItem:
    """Tests for DoDItem model."""

    def test_create_dod_item_defaults(self):
        """DoDItem with defaults."""
        item = DoDItem(text="UserService.register() 구현")
        assert item.completed is False
        assert item.category == "impl"

    def test_create_dod_item_completed(self):
        """DoDItem marked as completed."""
        item = DoDItem(
            text="test_success 통과",
            completed=True,
            category="test",
        )
        assert item.completed is True
        assert item.category == "test"


class TestWorkBreakdownCriteria:
    """Tests for WorkBreakdownCriteria model."""

    def test_default_criteria(self):
        """Default criteria values."""
        criteria = WorkBreakdownCriteria()
        assert criteria.max_duration_days == 2
        assert criteria.max_public_apis == 3
        assert criteria.max_tests == 9
        assert criteria.max_files_modified == 5

    def test_check_split_needed_no_split(self):
        """No split needed for small task."""
        criteria = WorkBreakdownCriteria()
        reasons = criteria.check_split_needed(
            file_count=3,
            api_count=2,
            test_count=6,
            domain_count=1,
        )
        assert len(reasons) == 0

    def test_check_split_needed_too_many_files(self):
        """Split needed for too many files."""
        criteria = WorkBreakdownCriteria()
        reasons = criteria.check_split_needed(
            file_count=10,
            api_count=2,
            test_count=6,
            domain_count=1,
        )
        assert len(reasons) == 1
        assert "files" in reasons[0].lower()

    def test_check_split_needed_multiple_reasons(self):
        """Split needed for multiple reasons."""
        criteria = WorkBreakdownCriteria()
        reasons = criteria.check_split_needed(
            file_count=10,
            api_count=5,
            test_count=15,
            domain_count=3,
        )
        assert len(reasons) == 4


class TestWorkerPacket:
    """Tests for WorkerPacket model."""

    def test_create_empty_packet(self):
        """Empty WorkerPacket."""
        packet = WorkerPacket()
        assert packet.goal is None
        assert packet.is_fully_specified() is False

    def test_is_fully_specified_true(self):
        """Fully specified WorkerPacket."""
        packet = WorkerPacket(
            goal=Goal(done="완료 조건"),
            contract_spec=ContractSpec(
                apis=[ApiSpec(name="API", input="x", output="y")],
                tests=RequiredTests(
                    success=["s1"],
                    failure=["f1"],
                    boundary=["b1"],
                ),
            ),
            boundary_map=BoundaryMap(target_domain="auth", target_layer="app"),
            code_placement=CodePlacement(create=["file.py"]),
            quality_gates=[QualityGate(name="lint", command="ruff")],
            checkpoints=CheckpointDefinition(
                cp1_skeleton="cp1",
                cp2_green="cp2",
                cp3_harden="cp3",
            ),
        )
        assert packet.is_fully_specified() is True

    def test_get_completion_percentage_empty(self):
        """Completion percentage with no items."""
        packet = WorkerPacket()
        assert packet.get_completion_percentage() == 0.0

    def test_get_completion_percentage_partial(self):
        """Completion percentage with partial completion."""
        packet = WorkerPacket(
            dod_items=[
                DoDItem(text="item1", completed=True),
                DoDItem(text="item2", completed=False),
                DoDItem(text="item3", completed=True),
                DoDItem(text="item4", completed=False),
            ]
        )
        assert packet.get_completion_percentage() == 50.0
