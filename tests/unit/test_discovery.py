"""Tests for C4 Discovery Module."""

import tempfile
from pathlib import Path

import pytest

from c4.discovery import (
    Domain,
    DomainDetector,
    EARSPattern,
    EARSRequirement,
    FeatureSpec,
    InterviewContext,
    InterviewEngine,
    InterviewPhase,
    SpecStore,
)


class TestDomainDetector:
    """Test domain detection logic."""

    @pytest.fixture
    def temp_project(self):
        """Create a temporary project directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            yield Path(tmpdir)

    def test_detect_empty_project(self, temp_project):
        """Test detection on empty project."""
        detector = DomainDetector(temp_project)
        result = detector.detect()
        assert result.primary_domain == Domain.UNKNOWN
        assert result.is_empty_project

    def test_detect_web_frontend_react(self, temp_project):
        """Test detection of React project."""
        # Create package.json with react
        package_json = temp_project / "package.json"
        package_json.write_text('{"dependencies": {"react": "^18.0.0"}}')

        detector = DomainDetector(temp_project)
        result = detector.detect()

        assert result.primary_domain == Domain.WEB_FRONTEND
        assert not result.is_empty_project
        assert result.confidence > 0.5

    def test_detect_web_backend_fastapi(self, temp_project):
        """Test detection of FastAPI project."""
        pyproject = temp_project / "pyproject.toml"
        pyproject.write_text(
            '[project]\nname = "myapi"\ndependencies = ["fastapi"]'
        )

        detector = DomainDetector(temp_project)
        result = detector.detect()

        assert result.primary_domain == Domain.WEB_BACKEND
        assert not result.is_empty_project

    def test_detect_ml_pytorch(self, temp_project):
        """Test detection of PyTorch ML project."""
        pyproject = temp_project / "pyproject.toml"
        pyproject.write_text(
            '[project]\nname = "mlproject"\ndependencies = ["torch"]'
        )

        detector = DomainDetector(temp_project)
        result = detector.detect()

        assert result.primary_domain == Domain.ML_DL
        assert not result.is_empty_project

    def test_detect_infra_terraform(self, temp_project):
        """Test detection of Terraform project."""
        tf_file = temp_project / "main.tf"
        tf_file.write_text('resource "aws_instance" "web" {}')

        detector = DomainDetector(temp_project)
        result = detector.detect()

        assert result.primary_domain == Domain.INFRA

    def test_detect_mobile_flutter(self, temp_project):
        """Test detection of Flutter project."""
        pubspec = temp_project / "pubspec.yaml"
        pubspec.write_text("name: myapp\nflutter:\n  sdk: flutter")

        detector = DomainDetector(temp_project)
        result = detector.detect()

        assert result.primary_domain == Domain.MOBILE_APP

    def test_infer_domain_from_description(self, temp_project):
        """Test inferring domain from description."""
        detector = DomainDetector(temp_project)

        assert detector.infer_domain_from_description(
            "웹 대시보드를 React로 만들고 싶어"
        ) == Domain.WEB_FRONTEND

        assert detector.infer_domain_from_description(
            "FastAPI로 REST API 서버를 구축하려고 해"
        ) == Domain.WEB_BACKEND

        assert detector.infer_domain_from_description(
            "PyTorch로 이미지 분류 모델을 학습할 거야"
        ) == Domain.ML_DL

        assert detector.infer_domain_from_description(
            "AWS 인프라를 Terraform으로 관리하고 싶어"
        ) == Domain.INFRA


class TestEARSRequirement:
    """Test EARS pattern parsing."""

    def test_parse_ubiquitous(self):
        """Test parsing ubiquitous pattern."""
        text = "The system shall display user data"
        req = EARSRequirement.parse("REQ-001", text)

        assert req.pattern == EARSPattern.UBIQUITOUS
        assert "display user data" in req.text

    def test_parse_event_driven(self):
        """Test parsing event-driven (When) pattern."""
        text = "When user clicks login, the system shall authenticate"
        req = EARSRequirement.parse("REQ-002", text)

        assert req.pattern == EARSPattern.EVENT_DRIVEN
        assert "authenticate" in req.text

    def test_parse_state_driven(self):
        """Test parsing state-driven (While) pattern."""
        text = "While loading data, the system shall show spinner"
        req = EARSRequirement.parse("REQ-003", text)

        assert req.pattern == EARSPattern.STATE_DRIVEN
        assert "show spinner" in req.text

    def test_parse_unwanted(self):
        """Test parsing unwanted behavior (If) pattern."""
        text = "If network fails, the system shall retry 3 times"
        req = EARSRequirement.parse("REQ-004", text)

        assert req.pattern == EARSPattern.UNWANTED
        assert "retry" in req.text

    def test_parse_optional(self):
        """Test parsing optional feature (Where) pattern."""
        text = "Where dark mode is enabled, the system shall use dark theme"
        req = EARSRequirement.parse("REQ-005", text)

        assert req.pattern == EARSPattern.OPTIONAL
        assert "dark theme" in req.text

    def test_model_fields(self):
        """Test EARSRequirement model fields."""
        req = EARSRequirement(
            id="REQ-001",
            pattern=EARSPattern.EVENT_DRIVEN,
            text="When user submits form, the system shall validate input",
        )

        assert req.id == "REQ-001"
        assert req.pattern == EARSPattern.EVENT_DRIVEN
        assert "validate input" in req.text


class TestFeatureSpec:
    """Test feature specification."""

    def test_create_feature_spec(self):
        """Test creating a feature spec."""
        spec = FeatureSpec(
            feature="user-auth",
            description="User authentication",
            domain=Domain.WEB_FRONTEND,
            requirements=[
                EARSRequirement(
                    id="REQ-001",
                    pattern=EARSPattern.EVENT_DRIVEN,
                    text="When user submits login, the system shall authenticate",
                )
            ],
        )

        assert spec.feature == "user-auth"
        assert spec.domain == Domain.WEB_FRONTEND
        assert len(spec.requirements) == 1

    def test_to_yaml(self):
        """Test YAML serialization."""
        spec = FeatureSpec(
            feature="user-auth",
            description="User authentication",
            domain=Domain.WEB_FRONTEND,
            requirements=[
                EARSRequirement(
                    id="REQ-001",
                    pattern=EARSPattern.UBIQUITOUS,
                    text="The system shall validate credentials",
                )
            ],
        )
        yaml_str = spec.to_yaml()

        assert "user-auth" in yaml_str
        assert "REQ-001" in yaml_str

    def test_add_requirement(self):
        """Test adding a requirement to spec."""
        spec = FeatureSpec(
            feature="user-auth",
            domain=Domain.WEB_FRONTEND,
        )
        req = spec.add_requirement("REQ-001", "The system shall validate credentials")

        assert req.id == "REQ-001"
        assert req.pattern == EARSPattern.UBIQUITOUS
        assert len(spec.requirements) == 1


class TestSpecStore:
    """Test spec storage."""

    @pytest.fixture
    def temp_c4_dir(self):
        """Create a temporary .c4 directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            c4_dir = Path(tmpdir) / ".c4"
            c4_dir.mkdir()
            yield c4_dir

    def test_init_creates_specs_dir(self, temp_c4_dir):
        """Test that ensure_dir creates specs directory."""
        store = SpecStore(temp_c4_dir)
        store.ensure_dir()
        assert store.specs_dir.exists()

    def test_save_and_load_spec(self, temp_c4_dir):
        """Test saving and loading a spec."""
        store = SpecStore(temp_c4_dir)

        spec = FeatureSpec(
            feature="user-auth",
            description="User authentication",
            domain=Domain.WEB_FRONTEND,
            requirements=[
                EARSRequirement(
                    id="REQ-001",
                    pattern=EARSPattern.UBIQUITOUS,
                    text="The system shall authenticate users",
                )
            ],
        )

        store.save(spec)
        loaded = store.load("user-auth")

        assert loaded is not None
        assert loaded.feature == "user-auth"
        assert loaded.domain == Domain.WEB_FRONTEND
        assert len(loaded.requirements) == 1

    def test_list_features(self, temp_c4_dir):
        """Test listing features."""
        store = SpecStore(temp_c4_dir)

        spec1 = FeatureSpec(
            feature="feature-a",
            description="Feature A",
            domain=Domain.WEB_FRONTEND,
        )
        spec2 = FeatureSpec(
            feature="feature-b",
            description="Feature B",
            domain=Domain.WEB_BACKEND,
        )

        store.save(spec1)
        store.save(spec2)

        features = store.list_features()
        assert len(features) == 2
        assert "feature-a" in features
        assert "feature-b" in features

    def test_delete_spec(self, temp_c4_dir):
        """Test deleting a spec."""
        store = SpecStore(temp_c4_dir)

        spec = FeatureSpec(
            feature="to-delete",
            description="To be deleted",
            domain=Domain.WEB_FRONTEND,
        )
        store.save(spec)

        assert store.load("to-delete") is not None

        store.delete("to-delete")

        assert store.load("to-delete") is None

    def test_path_traversal_protection(self, temp_c4_dir):
        """Test path traversal is blocked via normalization and resolve check."""
        store = SpecStore(temp_c4_dir)
        store.ensure_dir()

        # Empty names should fail
        with pytest.raises(ValueError, match="cannot be empty"):
            store.get_feature_dir("")

        with pytest.raises(ValueError, match="cannot be empty"):
            store.get_feature_dir("   ")

        # Path traversal attempts are normalized safely
        # "../etc/passwd" becomes "--etc-passwd" which is safe
        path = store.get_feature_dir("../etc/passwd")
        assert "--etc-passwd" in str(path)
        assert str(path).startswith(str(store.specs_dir.resolve()))

        # Absolute paths are normalized (/ becomes -)
        path = store.get_feature_dir("/absolute/path")
        assert "-absolute-path" in str(path)

        # Dots are normalized to dashes
        path = store.get_feature_dir(".")
        assert str(path).endswith("-")

        path = store.get_feature_dir("..")
        assert str(path).endswith("--")

        # Valid names should work
        path = store.get_feature_dir("valid-feature")
        assert "valid-feature" in str(path)


class TestInterviewEngine:
    """Test interview flow engine."""

    def test_initial_phase(self):
        """Test initial interview phase."""
        engine = InterviewEngine()
        assert engine.get_current_phase() == InterviewPhase.DOMAIN_CONFIRM

    def test_set_detected_domain(self):
        """Test setting detected domain."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.WEB_FRONTEND)
        assert engine.context.domain == Domain.WEB_FRONTEND

    def test_get_domain_confirm_questions(self):
        """Test getting domain confirmation questions."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.WEB_FRONTEND)

        questions = engine.get_next_questions()
        assert len(questions) > 0
        assert questions[0].header == "도메인"

        # The detected domain should be first
        first_option = questions[0].options[0]
        assert "(감지됨)" in first_option["label"]

    def test_process_domain_confirm(self):
        """Test processing domain confirmation."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.WEB_FRONTEND)

        engine.process_answers({"domain": "웹 프론트엔드 (감지됨)"})

        assert engine.context.domain_confirmed
        assert engine.context.domain == Domain.WEB_FRONTEND
        assert engine.get_current_phase() == InterviewPhase.PROJECT_OVERVIEW

    def test_fullstack_detection_from_multiple_selection(self):
        """Test fullstack domain from multiple selections."""
        engine = InterviewEngine()

        engine.process_answers({
            "domain": ["웹 프론트엔드", "웹 백엔드"]
        })

        assert engine.context.domain == Domain.FULLSTACK

    def test_complete_interview_flow(self):
        """Test complete interview flow."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.WEB_FRONTEND)

        # Phase 1: Domain confirm
        engine.process_answers({"domain": "웹 프론트엔드"})
        assert engine.get_current_phase() == InterviewPhase.PROJECT_OVERVIEW

        # Phase 2: Project overview
        engine.process_answers({"project_size": "중형 (1-2달)"})
        assert engine.get_current_phase() == InterviewPhase.CORE_FEATURES

        # Phase 3: Core features
        engine.process_answers({
            "ui_features": ["인증/로그인", "대시보드"],
            "interactions": ["없음"],
        })
        assert engine.get_current_phase() == InterviewPhase.FEATURE_DETAILS
        assert len(engine.context.core_features) > 0

        # Phase 4: Feature details (skip for simplicity)
        for _ in range(len(engine.context.core_features)):
            if engine.get_current_phase() == InterviewPhase.FEATURE_DETAILS:
                idx = engine.current_feature_index
                engine.process_answers({f"feature_detail_{idx}": "기본으로"})

        assert engine.get_current_phase() == InterviewPhase.TECH_STACK

        # Phase 5: Tech stack
        engine.process_answers({
            "framework": "React (권장)",
            "language": "TypeScript (권장)",
        })
        assert engine.get_current_phase() == InterviewPhase.VALIDATION

        # Phase 6: Validation
        engine.process_answers({"test_tools": ["Vitest (권장)", "ESLint + Prettier"]})
        assert engine.get_current_phase() == InterviewPhase.CHECKPOINTS

        # Phase 7: Checkpoints
        engine.process_answers({"checkpoint_strategy": "기능별 (권장)"})
        assert engine.get_current_phase() == InterviewPhase.COMPLETE
        assert engine.is_complete()

    def test_get_interview_state(self):
        """Test getting interview state for persistence."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.ML_DL)
        engine.process_answers({"domain": "ML/DL"})

        state = engine.get_state_dict()

        assert state["phase"] == "project_overview"
        assert state["domain"] == "ml-dl"
        assert state["domain_confirmed"]

    def test_restore_from_state(self):
        """Test restoring interview from saved state."""
        # Create and advance an engine
        engine1 = InterviewEngine()
        engine1.set_detected_domain(Domain.WEB_BACKEND)
        engine1.process_answers({"domain": "웹 백엔드"})
        engine1.process_answers({"project_size": "소형 (1-2주)"})

        state = engine1.get_state_dict()

        # Restore to new engine
        engine2 = InterviewEngine()
        engine2.restore_from_state_dict(state)

        assert engine2.get_current_phase() == InterviewPhase.CORE_FEATURES
        assert engine2.context.domain == Domain.WEB_BACKEND
        assert engine2.context.domain_confirmed

    def test_get_project_overview(self):
        """Test generating project overview from context."""
        engine = InterviewEngine()
        engine.set_detected_domain(Domain.WEB_FRONTEND)
        engine.context.core_features = ["인증/로그인", "대시보드"]
        engine.context.tech_stack = {
            "framework": "React",
            "language": "TypeScript",
        }

        overview = engine.get_project_overview()

        assert overview.domain == Domain.WEB_FRONTEND
        assert len(overview.key_features) == 2
        assert "React" in overview.tech_stack


class TestInterviewContext:
    """Test interview context."""

    def test_default_values(self):
        """Test default context values."""
        ctx = InterviewContext()

        assert ctx.domain == Domain.UNKNOWN
        assert not ctx.domain_confirmed
        assert ctx.core_features == []
        assert ctx.tech_stack == {}
        assert ctx.validation_tools == []
        assert ctx.checkpoint_strategy == "phase"
