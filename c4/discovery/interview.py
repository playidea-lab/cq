"""Interview Flow Engine for C4 Discovery Phase.

This module provides the interview workflow that:
1. Generates questions based on detected domain
2. Collects user answers through structured questions
3. Generates EARS-based specifications from answers
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from .models import Domain, FeatureInfo, ProjectOverview


class InterviewPhase(str, Enum):
    """Phases of the interview process."""

    DOMAIN_CONFIRM = "domain_confirm"  # Confirm detected domain
    PROJECT_OVERVIEW = "project_overview"  # Get project overview
    CORE_FEATURES = "core_features"  # Identify core features
    FEATURE_DETAILS = "feature_details"  # Detail each feature
    TECH_STACK = "tech_stack"  # Technology choices
    VALIDATION = "validation"  # Testing and validation
    CHECKPOINTS = "checkpoints"  # Checkpoint configuration
    COMPLETE = "complete"


@dataclass
class InterviewQuestion:
    """A single interview question with options."""

    question: str
    header: str
    options: list[dict[str, str]]
    multi_select: bool = False
    phase: InterviewPhase = InterviewPhase.PROJECT_OVERVIEW
    context_key: str = ""  # Key to store answer in context

    def to_ask_user_format(self) -> dict[str, Any]:
        """Convert to AskUserQuestion format."""
        return {
            "question": self.question,
            "header": self.header,
            "options": self.options,
            "multiSelect": self.multi_select,
        }


@dataclass
class InterviewContext:
    """Context collected during the interview."""

    domain: Domain = Domain.UNKNOWN
    domain_confirmed: bool = False
    project_description: str = ""
    core_features: list[str] = field(default_factory=list)
    feature_details: dict[str, FeatureInfo] = field(default_factory=dict)
    tech_stack: dict[str, str] = field(default_factory=dict)
    validation_tools: list[str] = field(default_factory=list)
    checkpoint_strategy: str = "phase"
    additional_context: dict[str, Any] = field(default_factory=dict)


class InterviewEngine:
    """Engine for conducting structured interviews.

    This engine generates domain-specific questions and collects
    answers to build project specifications.
    """

    def __init__(self) -> None:
        """Initialize interview engine."""
        self.context = InterviewContext()
        self.current_phase = InterviewPhase.DOMAIN_CONFIRM
        self.current_feature_index = 0

    def set_detected_domain(self, domain: Domain) -> None:
        """Set the detected domain from domain detector."""
        self.context.domain = domain

    def get_current_phase(self) -> InterviewPhase:
        """Get current interview phase."""
        return self.current_phase

    def is_complete(self) -> bool:
        """Check if interview is complete."""
        return self.current_phase == InterviewPhase.COMPLETE

    def get_next_questions(self) -> list[InterviewQuestion]:
        """Get questions for the current phase."""
        if self.current_phase == InterviewPhase.DOMAIN_CONFIRM:
            return self._get_domain_confirm_questions()
        elif self.current_phase == InterviewPhase.PROJECT_OVERVIEW:
            return self._get_project_overview_questions()
        elif self.current_phase == InterviewPhase.CORE_FEATURES:
            return self._get_core_features_questions()
        elif self.current_phase == InterviewPhase.FEATURE_DETAILS:
            return self._get_feature_detail_questions()
        elif self.current_phase == InterviewPhase.TECH_STACK:
            return self._get_tech_stack_questions()
        elif self.current_phase == InterviewPhase.VALIDATION:
            return self._get_validation_questions()
        elif self.current_phase == InterviewPhase.CHECKPOINTS:
            return self._get_checkpoint_questions()
        return []

    def process_answers(self, answers: dict[str, Any]) -> None:
        """Process answers from the current phase and advance."""
        if self.current_phase == InterviewPhase.DOMAIN_CONFIRM:
            self._process_domain_confirm(answers)
        elif self.current_phase == InterviewPhase.PROJECT_OVERVIEW:
            self._process_project_overview(answers)
        elif self.current_phase == InterviewPhase.CORE_FEATURES:
            self._process_core_features(answers)
        elif self.current_phase == InterviewPhase.FEATURE_DETAILS:
            self._process_feature_details(answers)
        elif self.current_phase == InterviewPhase.TECH_STACK:
            self._process_tech_stack(answers)
        elif self.current_phase == InterviewPhase.VALIDATION:
            self._process_validation(answers)
        elif self.current_phase == InterviewPhase.CHECKPOINTS:
            self._process_checkpoints(answers)

    def _advance_phase(self) -> None:
        """Advance to the next phase."""
        phase_order = list(InterviewPhase)
        current_index = phase_order.index(self.current_phase)
        if current_index < len(phase_order) - 1:
            self.current_phase = phase_order[current_index + 1]

    # =========================================================================
    # Domain Confirmation
    # =========================================================================

    def _get_domain_confirm_questions(self) -> list[InterviewQuestion]:
        """Get domain confirmation questions."""
        domain_options = [
            {"label": "웹 프론트엔드", "description": "React, Vue, 브라우저 기반 UI"},
            {"label": "웹 백엔드", "description": "API 서버, 데이터베이스"},
            {"label": "풀스택", "description": "프론트엔드 + 백엔드"},
            {"label": "ML/DL", "description": "머신러닝, 딥러닝, 데이터 분석"},
            {"label": "모바일 앱", "description": "iOS, Android, React Native, Flutter"},
            {"label": "인프라/DevOps", "description": "Terraform, Docker, CI/CD"},
            {"label": "라이브러리", "description": "재사용 가능한 패키지"},
            {"label": "펌웨어/임베디드", "description": "하드웨어 제어, IoT"},
        ]

        # Put detected domain first with (감지됨) marker
        detected_label = self._domain_to_label(self.context.domain)
        if detected_label:
            for i, opt in enumerate(domain_options):
                if opt["label"] == detected_label:
                    opt["label"] = f"{detected_label} (감지됨)"
                    # Move to first position
                    domain_options.insert(0, domain_options.pop(i))
                    break

        return [
            InterviewQuestion(
                question="이 프로젝트의 도메인이 맞나요?",
                header="도메인",
                options=domain_options,
                multi_select=True,  # Allow multiple for fullstack, etc.
                phase=InterviewPhase.DOMAIN_CONFIRM,
                context_key="domain",
            )
        ]

    def _domain_to_label(self, domain: Domain) -> str:
        """Convert domain enum to Korean label."""
        mapping = {
            Domain.WEB_FRONTEND: "웹 프론트엔드",
            Domain.WEB_BACKEND: "웹 백엔드",
            Domain.FULLSTACK: "풀스택",
            Domain.ML_DL: "ML/DL",
            Domain.MOBILE_APP: "모바일 앱",
            Domain.INFRA: "인프라/DevOps",
            Domain.LIBRARY: "라이브러리",
            Domain.FIRMWARE: "펌웨어/임베디드",
        }
        return mapping.get(domain, "")

    def _label_to_domain(self, label: str) -> Domain:
        """Convert Korean label to domain enum."""
        # Remove (감지됨) marker if present
        clean_label = label.replace(" (감지됨)", "")
        mapping = {
            "웹 프론트엔드": Domain.WEB_FRONTEND,
            "웹 백엔드": Domain.WEB_BACKEND,
            "풀스택": Domain.FULLSTACK,
            "ML/DL": Domain.ML_DL,
            "모바일 앱": Domain.MOBILE_APP,
            "인프라/DevOps": Domain.INFRA,
            "라이브러리": Domain.LIBRARY,
            "펌웨어/임베디드": Domain.FIRMWARE,
        }
        return mapping.get(clean_label, Domain.UNKNOWN)

    def _process_domain_confirm(self, answers: dict[str, Any]) -> None:
        """Process domain confirmation answer."""
        domain_answer = answers.get("domain", "")
        if isinstance(domain_answer, list):
            # Multi-select: if both frontend and backend, it's fullstack
            domains = [self._label_to_domain(d) for d in domain_answer]
            if Domain.WEB_FRONTEND in domains and Domain.WEB_BACKEND in domains:
                self.context.domain = Domain.FULLSTACK
            elif domains:
                self.context.domain = domains[0]
        else:
            self.context.domain = self._label_to_domain(domain_answer)

        self.context.domain_confirmed = True
        self._advance_phase()

    # =========================================================================
    # Project Overview
    # =========================================================================

    def _get_project_overview_questions(self) -> list[InterviewQuestion]:
        """Get project overview questions."""
        return [
            InterviewQuestion(
                question="프로젝트의 규모는 어떤가요?",
                header="규모",
                options=[
                    {"label": "소형 (1-2주)", "description": "단일 기능, 프로토타입"},
                    {"label": "중형 (1-2달)", "description": "여러 기능, MVP"},
                    {"label": "대형 (3달+)", "description": "복잡한 시스템, 팀 프로젝트"},
                ],
                phase=InterviewPhase.PROJECT_OVERVIEW,
                context_key="project_size",
            ),
        ]

    def _process_project_overview(self, answers: dict[str, Any]) -> None:
        """Process project overview answers."""
        self.context.additional_context["project_size"] = answers.get(
            "project_size", "중형 (1-2달)"
        )
        self._advance_phase()

    # =========================================================================
    # Core Features
    # =========================================================================

    def _get_core_features_questions(self) -> list[InterviewQuestion]:
        """Get core features identification questions based on domain."""
        domain = self.context.domain

        if domain in [Domain.WEB_FRONTEND, Domain.FULLSTACK]:
            return self._get_web_frontend_feature_questions()
        elif domain == Domain.WEB_BACKEND:
            return self._get_web_backend_feature_questions()
        elif domain == Domain.ML_DL:
            return self._get_ml_feature_questions()
        elif domain == Domain.MOBILE_APP:
            return self._get_mobile_feature_questions()
        elif domain == Domain.INFRA:
            return self._get_infra_feature_questions()
        else:
            return self._get_generic_feature_questions()

    def _get_web_frontend_feature_questions(self) -> list[InterviewQuestion]:
        """Get web frontend specific feature questions."""
        return [
            InterviewQuestion(
                question="어떤 UI 기능이 필요한가요? (복수 선택)",
                header="UI기능",
                options=[
                    {"label": "인증/로그인", "description": "사용자 인증, 권한 관리"},
                    {"label": "대시보드", "description": "데이터 시각화, 통계"},
                    {"label": "폼/입력", "description": "데이터 입력, 유효성 검사"},
                    {"label": "리스트/테이블", "description": "데이터 목록, 페이지네이션"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="ui_features",
            ),
            InterviewQuestion(
                question="특별한 인터랙션이 필요한가요?",
                header="인터랙션",
                options=[
                    {"label": "없음", "description": "표준 웹 인터랙션"},
                    {"label": "캔버스/그래픽", "description": "Canvas 2D/3D, 애니메이션"},
                    {"label": "웹캠/미디어", "description": "카메라, 마이크 사용"},
                    {"label": "실시간 동기화", "description": "WebSocket, 실시간 업데이트"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="interactions",
            ),
        ]

    def _get_web_backend_feature_questions(self) -> list[InterviewQuestion]:
        """Get web backend specific feature questions."""
        return [
            InterviewQuestion(
                question="어떤 백엔드 기능이 필요한가요? (복수 선택)",
                header="API기능",
                options=[
                    {"label": "REST API", "description": "CRUD 엔드포인트"},
                    {"label": "인증/JWT", "description": "사용자 인증, 토큰 관리"},
                    {"label": "데이터베이스", "description": "PostgreSQL, MySQL, SQLite"},
                    {"label": "파일 업로드", "description": "이미지, 문서 처리"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="api_features",
            ),
        ]

    def _get_ml_feature_questions(self) -> list[InterviewQuestion]:
        """Get ML/DL specific feature questions."""
        return [
            InterviewQuestion(
                question="어떤 ML 작업이 필요한가요?",
                header="ML작업",
                options=[
                    {"label": "분류", "description": "Classification, 카테고리 예측"},
                    {"label": "회귀", "description": "Regression, 수치 예측"},
                    {"label": "시계열", "description": "Time series, 예측"},
                    {"label": "컴퓨터 비전", "description": "이미지 처리, 객체 탐지"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="ml_tasks",
            ),
            InterviewQuestion(
                question="데이터 파이프라인이 필요한가요?",
                header="파이프라인",
                options=[
                    {"label": "필요 없음", "description": "단순 실험, 노트북"},
                    {"label": "기본 파이프라인", "description": "전처리 → 학습 → 평가"},
                    {"label": "MLOps", "description": "실험 추적, 모델 레지스트리"},
                ],
                phase=InterviewPhase.CORE_FEATURES,
                context_key="pipeline",
            ),
        ]

    def _get_mobile_feature_questions(self) -> list[InterviewQuestion]:
        """Get mobile app specific feature questions."""
        return [
            InterviewQuestion(
                question="어떤 모바일 기능이 필요한가요? (복수 선택)",
                header="모바일기능",
                options=[
                    {"label": "인증/로그인", "description": "소셜 로그인, 생체 인증"},
                    {"label": "카메라/갤러리", "description": "사진 촬영, 이미지 선택"},
                    {"label": "푸시 알림", "description": "FCM, APNS"},
                    {"label": "오프라인 지원", "description": "로컬 저장, 동기화"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="mobile_features",
            ),
        ]

    def _get_infra_feature_questions(self) -> list[InterviewQuestion]:
        """Get infra/DevOps specific feature questions."""
        return [
            InterviewQuestion(
                question="어떤 인프라 작업이 필요한가요? (복수 선택)",
                header="인프라작업",
                options=[
                    {"label": "클라우드 프로비저닝", "description": "AWS, GCP, Azure"},
                    {"label": "컨테이너화", "description": "Docker, Kubernetes"},
                    {"label": "CI/CD", "description": "GitHub Actions, GitLab CI"},
                    {"label": "모니터링", "description": "로깅, 알림"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="infra_features",
            ),
        ]

    def _get_generic_feature_questions(self) -> list[InterviewQuestion]:
        """Get generic feature questions for unknown domains."""
        return [
            InterviewQuestion(
                question="이 프로젝트의 핵심 기능은 무엇인가요?",
                header="핵심기능",
                options=[
                    {"label": "데이터 처리", "description": "입력 → 변환 → 출력"},
                    {"label": "사용자 인터페이스", "description": "UI/UX, 사용자 상호작용"},
                    {"label": "자동화", "description": "작업 자동화, 스케줄링"},
                    {"label": "통합", "description": "시스템 간 연동"},
                ],
                multi_select=True,
                phase=InterviewPhase.CORE_FEATURES,
                context_key="generic_features",
            ),
        ]

    def _process_core_features(self, answers: dict[str, Any]) -> None:
        """Process core features answers."""
        for key, value in answers.items():
            if isinstance(value, list):
                self.context.core_features.extend(value)
            else:
                self.context.core_features.append(value)
            self.context.additional_context[key] = value

        self._advance_phase()

    # =========================================================================
    # Feature Details
    # =========================================================================

    def _get_feature_detail_questions(self) -> list[InterviewQuestion]:
        """Get detailed questions for each core feature.

        Returns empty list if no more features to process.
        Phase advancement is handled by process_answers, not here.
        """
        if self.current_feature_index >= len(self.context.core_features):
            # No more features - return empty (no side effects in getter)
            return []

        feature = self.context.core_features[self.current_feature_index]
        return [
            InterviewQuestion(
                question=f"'{feature}' 기능을 상세화할까요?",
                header="상세화",
                options=[
                    {"label": "예, 상세화", "description": "세부 요구사항 정의"},
                    {"label": "기본으로", "description": "기본 구현만"},
                    {"label": "건너뛰기", "description": "나중에 정의"},
                ],
                phase=InterviewPhase.FEATURE_DETAILS,
                context_key=f"feature_detail_{self.current_feature_index}",
            ),
        ]

    def _process_feature_details(self, answers: dict[str, Any]) -> None:
        """Process feature detail answers."""
        # Handle edge case: no features to process
        if not self.context.core_features:
            self._advance_phase()
            return

        key = f"feature_detail_{self.current_feature_index}"
        detail_choice = answers.get(key, "기본으로")

        if self.current_feature_index < len(self.context.core_features):
            feature = self.context.core_features[self.current_feature_index]
            # Priority: 1=highest, 5=lowest. "예, 상세화"=1, "기본으로"=2, "건너뛰기"=3
            priority = 2 if detail_choice == "기본으로" else (1 if "상세화" in detail_choice else 3)
            self.context.feature_details[feature] = FeatureInfo(
                name=feature,
                description="",
                priority=priority,
                domain=self.context.domain,
            )

        self.current_feature_index += 1
        if self.current_feature_index >= len(self.context.core_features):
            self._advance_phase()

    # =========================================================================
    # Tech Stack
    # =========================================================================

    def _get_tech_stack_questions(self) -> list[InterviewQuestion]:
        """Get technology stack questions based on domain."""
        domain = self.context.domain

        if domain in [Domain.WEB_FRONTEND, Domain.FULLSTACK]:
            return [
                InterviewQuestion(
                    question="프레임워크를 선택해주세요",
                    header="프레임워크",
                    options=[
                        {"label": "React (권장)", "description": "생태계 풍부, 채용 많음"},
                        {"label": "Vue", "description": "쉬운 학습곡선"},
                        {"label": "Vanilla JS", "description": "프레임워크 없이"},
                        {"label": "Svelte", "description": "빠른 성능"},
                    ],
                    phase=InterviewPhase.TECH_STACK,
                    context_key="framework",
                ),
                InterviewQuestion(
                    question="언어를 선택해주세요",
                    header="언어",
                    options=[
                        {"label": "TypeScript (권장)", "description": "타입 안정성"},
                        {"label": "JavaScript", "description": "빠른 시작"},
                    ],
                    phase=InterviewPhase.TECH_STACK,
                    context_key="language",
                ),
            ]
        elif domain == Domain.WEB_BACKEND:
            return [
                InterviewQuestion(
                    question="백엔드 프레임워크를 선택해주세요",
                    header="백엔드",
                    options=[
                        {"label": "FastAPI (권장)", "description": "Python, 빠른 API 개발"},
                        {"label": "Express.js", "description": "Node.js 표준"},
                        {"label": "Django", "description": "Python, 풀 스택"},
                        {"label": "NestJS", "description": "TypeScript, 구조화된 백엔드"},
                    ],
                    phase=InterviewPhase.TECH_STACK,
                    context_key="backend_framework",
                ),
            ]
        elif domain == Domain.ML_DL:
            return [
                InterviewQuestion(
                    question="ML 프레임워크를 선택해주세요",
                    header="ML프레임워크",
                    options=[
                        {"label": "PyTorch (권장)", "description": "연구, 유연함"},
                        {"label": "scikit-learn", "description": "전통 ML, 간단한 모델"},
                        {"label": "TensorFlow", "description": "프로덕션, TPU 지원"},
                        {"label": "JAX", "description": "고성능, 함수형"},
                    ],
                    phase=InterviewPhase.TECH_STACK,
                    context_key="ml_framework",
                ),
            ]
        else:
            return [
                InterviewQuestion(
                    question="주 프로그래밍 언어를 선택해주세요",
                    header="언어",
                    options=[
                        {"label": "Python", "description": "범용, 빠른 개발"},
                        {"label": "TypeScript", "description": "타입 안정성"},
                        {"label": "Go", "description": "고성능, 간결함"},
                        {"label": "Rust", "description": "시스템 프로그래밍"},
                    ],
                    phase=InterviewPhase.TECH_STACK,
                    context_key="language",
                ),
            ]

    def _process_tech_stack(self, answers: dict[str, Any]) -> None:
        """Process tech stack answers."""
        for key, value in answers.items():
            self.context.tech_stack[key] = value
        self._advance_phase()

    # =========================================================================
    # Validation
    # =========================================================================

    def _get_validation_questions(self) -> list[InterviewQuestion]:
        """Get validation and testing questions."""
        domain = self.context.domain
        language = self.context.tech_stack.get("language", "")

        if "Python" in language or domain == Domain.ML_DL:
            return [
                InterviewQuestion(
                    question="테스트 도구를 선택해주세요 (복수 선택)",
                    header="테스트",
                    options=[
                        {"label": "pytest (권장)", "description": "Python 표준 테스트"},
                        {"label": "Ruff", "description": "린팅 + 포매팅"},
                        {"label": "mypy", "description": "타입 체크"},
                        {"label": "필요 없음", "description": "테스트 스킵"},
                    ],
                    multi_select=True,
                    phase=InterviewPhase.VALIDATION,
                    context_key="test_tools",
                ),
            ]
        else:
            return [
                InterviewQuestion(
                    question="테스트 도구를 선택해주세요 (복수 선택)",
                    header="테스트",
                    options=[
                        {"label": "Vitest (권장)", "description": "빠른 유닛 테스트"},
                        {"label": "ESLint + Prettier", "description": "린팅 + 포매팅"},
                        {"label": "Playwright", "description": "E2E 테스트"},
                        {"label": "필요 없음", "description": "테스트 스킵"},
                    ],
                    multi_select=True,
                    phase=InterviewPhase.VALIDATION,
                    context_key="test_tools",
                ),
            ]

    def _process_validation(self, answers: dict[str, Any]) -> None:
        """Process validation answers."""
        tools = answers.get("test_tools", [])
        if isinstance(tools, list):
            self.context.validation_tools = tools
        else:
            self.context.validation_tools = [tools]
        self._advance_phase()

    # =========================================================================
    # Checkpoints
    # =========================================================================

    def _get_checkpoint_questions(self) -> list[InterviewQuestion]:
        """Get checkpoint configuration questions."""
        return [
            InterviewQuestion(
                question="체크포인트 전략을 선택해주세요",
                header="체크포인트",
                options=[
                    {"label": "기능별 (권장)", "description": "각 기능 완료 시 리뷰"},
                    {"label": "Phase별", "description": "Phase 완료 시 리뷰"},
                    {"label": "없음", "description": "마지막에 한 번만 리뷰"},
                ],
                phase=InterviewPhase.CHECKPOINTS,
                context_key="checkpoint_strategy",
            ),
        ]

    def _process_checkpoints(self, answers: dict[str, Any]) -> None:
        """Process checkpoint answers."""
        self.context.checkpoint_strategy = answers.get("checkpoint_strategy", "기능별 (권장)")
        self._advance_phase()

    # =========================================================================
    # Result Generation
    # =========================================================================

    def get_state_dict(self) -> dict[str, Any]:
        """Get current interview state as dictionary for persistence."""
        return {
            "phase": self.current_phase.value,
            "domain": self.context.domain.value,
            "domain_confirmed": self.context.domain_confirmed,
            "project_description": self.context.project_description,
            "core_features": self.context.core_features,
            "tech_stack": self.context.tech_stack,
            "validation_tools": self.context.validation_tools,
            "checkpoint_strategy": self.context.checkpoint_strategy,
            "additional_context": self.context.additional_context,
            "current_feature_index": self.current_feature_index,
        }

    def get_project_overview(self) -> ProjectOverview:
        """Generate project overview from collected context."""
        return ProjectOverview(
            description=self.context.project_description,
            domain=self.context.domain,
            additional_domains=[],
            key_features=self.context.core_features,
            tech_stack=list(self.context.tech_stack.values()),
        )

    def get_features(self) -> list[FeatureInfo]:
        """Get list of FeatureInfo objects from context."""
        features = []
        for f in self.context.core_features:
            if f in self.context.feature_details:
                features.append(self.context.feature_details[f])
            else:
                features.append(
                    FeatureInfo(
                        name=f,
                        description="",
                        priority=2,
                        domain=self.context.domain,
                    )
                )
        return features

    def restore_from_state_dict(self, state: dict[str, Any]) -> None:
        """Restore interview state from dictionary."""
        self.current_phase = InterviewPhase(state.get("phase", "domain_confirm"))
        self.context.domain = Domain(state.get("domain", "unknown"))
        self.context.domain_confirmed = state.get("domain_confirmed", False)
        self.context.project_description = state.get("project_description", "")
        self.context.core_features = state.get("core_features", [])
        self.context.tech_stack = state.get("tech_stack", {})
        self.context.validation_tools = state.get("validation_tools", [])
        self.context.checkpoint_strategy = state.get("checkpoint_strategy", "feature")
        self.context.additional_context = state.get("additional_context", {})
        self.current_feature_index = state.get("current_feature_index", 0)
