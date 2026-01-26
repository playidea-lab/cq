"""DDD-CLEANCODE Models - Structured task specifications for clean architecture.

This module implements the Worker Packet structure from the DDD-CLEANCODE guide:
- Goal: What to achieve and what's out of scope
- ContractSpec: APIs and test specifications
- BoundaryMap: DDD layer constraints and import rules
- CodePlacement: File locations for implementation and tests
- QualityGate: Validation commands with requirements
- CheckpointDefinition: CP1/CP2/CP3 milestones
- DoDItem: Parsed DoD checklist items
"""

from typing import Literal

from pydantic import BaseModel, Field

# =============================================================================
# Goal - What to achieve
# =============================================================================


class Goal(BaseModel):
    """Goal specification with done criteria and out-of-scope items.

    Example:
        goal:
          done: "POST /v1/users 호출 시 User가 생성된다"
          out_of_scope: "이메일 검증은 T-002에서"
    """

    done: str = Field(..., description="Clear definition of what 'done' means")
    out_of_scope: str | None = Field(
        None, description="Explicitly excluded from this task"
    )


# =============================================================================
# ContractSpec - API and Test specifications
# =============================================================================


class ApiSpec(BaseModel):
    """Single API specification.

    Example:
        - name: UserService.register
          input: "email: str, password: str"
          output: "User"
          errors: [DuplicateEmail, WeakPassword]
          side_effects: "DB에 user 레코드 생성"
    """

    name: str = Field(..., description="Fully qualified API name")
    input: str = Field(..., description="Input parameter types")
    output: str = Field(..., description="Return type")
    errors: list[str] = Field(default_factory=list, description="Possible error types")
    side_effects: str | None = Field(None, description="Side effects description")


class RequiredTests(BaseModel):
    """Test specification with success/failure/boundary cases.

    Minimum requirements from DDD-CLEANCODE guide:
    - At least 1 success test
    - At least 1 failure test
    - At least 1 boundary test
    """

    success: list[str] = Field(
        default_factory=list,
        description="Success case test names (min 1)",
        min_length=1,
    )
    failure: list[str] = Field(
        default_factory=list,
        description="Failure case test names (min 1)",
        min_length=1,
    )
    boundary: list[str] = Field(
        default_factory=list,
        description="Boundary case test names (min 1)",
        min_length=1,
    )


class ContractSpec(BaseModel):
    """Contract specification defining APIs and required tests.

    This ensures every task has clear API contracts and test coverage
    for success, failure, and boundary cases.
    """

    apis: list[ApiSpec] = Field(
        default_factory=list, description="API specifications"
    )
    tests: RequiredTests | None = Field(None, description="Required test specifications")


# =============================================================================
# BoundaryMap - DDD Layer constraints
# =============================================================================

DomainType = Literal[
    "auth",
    "user",
    "payment",
    "notification",
    "analytics",
    "common",
    "core",
    "api",
    "infra",
]

LayerType = Literal["domain", "app", "infra", "api", "presentation"]


class BoundaryMap(BaseModel):
    """DDD boundary constraints for clean architecture.

    Enforces:
    - Target domain and layer restrictions
    - Allowed/forbidden imports
    - Single public export point

    Example:
        boundary_map:
          target_domain: auth
          target_layer: app
          allowed_imports: [stdlib, pydantic, domain.user]
          forbidden_imports: [sqlalchemy, httpx, fastapi]
          public_export: src/api/v1/users.py
    """

    target_domain: str = Field(..., description="Target domain (e.g., 'auth', 'payment')")
    target_layer: LayerType = Field(..., description="Target layer in clean architecture")
    allowed_imports: list[str] = Field(
        default_factory=lambda: ["stdlib", "pydantic"],
        description="Allowed import categories/modules",
    )
    forbidden_imports: list[str] = Field(
        default_factory=list,
        description="Explicitly forbidden imports (e.g., 'sqlalchemy' in domain layer)",
    )
    public_export: str | None = Field(
        None, description="Single file where public API is exported"
    )


# =============================================================================
# CodePlacement - File locations
# =============================================================================


class CodePlacement(BaseModel):
    """File placement specification for implementation and tests.

    Guides workers on exactly where to create/modify files.

    Example:
        code_placement:
          create:
            - src/auth/service.py
            - src/auth/domain/user.py
          modify:
            - src/api/v1/users.py
          tests:
            - tests/unit/auth/test_service.py
    """

    create: list[str] = Field(
        default_factory=list, description="Files to create"
    )
    modify: list[str] = Field(
        default_factory=list, description="Files to modify"
    )
    tests: list[str] = Field(
        default_factory=list, description="Test files to create/modify"
    )


# =============================================================================
# QualityGate - Validation commands
# =============================================================================


class QualityGate(BaseModel):
    """Quality gate validation command.

    Example:
        - name: lint
          command: "uv run ruff check ."
          required: true
        - name: forbidden_imports
          command: "uv run python scripts/check_imports.py src/auth/"
          required: true
    """

    name: str = Field(..., description="Gate name (e.g., 'lint', 'typecheck')")
    command: str = Field(..., description="Command to execute")
    required: bool = Field(True, description="Whether this gate must pass")
    timeout_seconds: int = Field(300, description="Timeout for this gate")


# =============================================================================
# CheckpointDefinition - Milestone definitions
# =============================================================================


class CheckpointDefinition(BaseModel):
    """Checkpoint definitions for task progress tracking.

    Three standard checkpoints from DDD-CLEANCODE guide:
    - CP1 (Skeleton): File structure and test stubs
    - CP2 (Green): Core functionality with passing tests
    - CP3 (Harden): Edge cases and error handling

    Example:
        checkpoints:
          cp1_skeleton: "파일 배치 + 테스트 골격 (실패 OK)"
          cp2_green: "register() 성공 테스트 통과"
          cp3_harden: "실패/경계 테스트 추가"
    """

    cp1_skeleton: str = Field(
        ..., description="CP1: File structure and test stubs created"
    )
    cp2_green: str = Field(
        ..., description="CP2: Core API(s) with passing success tests"
    )
    cp3_harden: str = Field(
        ..., description="CP3: Failure and boundary tests added"
    )


# =============================================================================
# DoDItem - Parsed DoD checklist
# =============================================================================

DoDCategory = Literal["impl", "test", "gate", "review"]


class DoDItem(BaseModel):
    """Single DoD checklist item.

    Parsed from DoD string into structured items for tracking.

    Example:
        - text: "UserService.register() 구현"
          completed: false
          category: impl
        - text: "test_register_success 통과"
          completed: true
          category: test
    """

    text: str = Field(..., description="DoD item text")
    completed: bool = Field(False, description="Whether this item is completed")
    category: DoDCategory = Field("impl", description="Item category")


# =============================================================================
# WorkBreakdownCriteria - Size limits
# =============================================================================


class WorkBreakdownCriteria(BaseModel):
    """Criteria for task size limits.

    From DDD-CLEANCODE guide:
    - Max 1-2 days duration
    - Max 1-3 public APIs
    - Max 3-9 tests

    Tasks exceeding these should be split.
    """

    max_duration_days: int = Field(2, description="Maximum task duration in days")
    max_public_apis: int = Field(3, description="Maximum public APIs per task")
    max_tests: int = Field(9, description="Maximum tests per task")
    max_files_modified: int = Field(5, description="Maximum files to modify")
    max_domains_touched: int = Field(1, description="Maximum domains to touch")

    def check_split_needed(
        self,
        file_count: int,
        api_count: int,
        test_count: int,
        domain_count: int,
    ) -> list[str]:
        """Check if task should be split and return reasons."""
        reasons = []
        if file_count > self.max_files_modified:
            reasons.append(
                f"Too many files ({file_count} > {self.max_files_modified})"
            )
        if api_count > self.max_public_apis:
            reasons.append(
                f"Too many APIs ({api_count} > {self.max_public_apis})"
            )
        if test_count > self.max_tests:
            reasons.append(
                f"Too many tests ({test_count} > {self.max_tests})"
            )
        if domain_count > self.max_domains_touched:
            reasons.append(
                f"Too many domains ({domain_count} > {self.max_domains_touched})"
            )
        return reasons


# =============================================================================
# ReviewPrompts - Standard review questions
# =============================================================================

STANDARD_REVIEW_PROMPTS: list[str] = [
    "ContractSpec을 만족하는가?",
    "경계(BoundaryMap)를 침범하지 않았는가?",
    "Public API가 한 곳에서만 노출되는가?",
    "테스트가 실패/경계 케이스를 포함하는가?",
    "용어가 일관적인가?",
]


# =============================================================================
# WorkerPacket - Complete task specification
# =============================================================================


class WorkerPacket(BaseModel):
    """Complete Worker Packet following DDD-CLEANCODE guide.

    This is the target structure for fully specified tasks.
    All fields are optional for backward compatibility.
    """

    goal: Goal | None = None
    contract_spec: ContractSpec | None = None
    boundary_map: BoundaryMap | None = None
    code_placement: CodePlacement | None = None
    quality_gates: list[QualityGate] = Field(default_factory=list)
    checkpoints: CheckpointDefinition | None = None
    dod_items: list[DoDItem] = Field(default_factory=list)

    def is_fully_specified(self) -> bool:
        """Check if this packet has all required specifications."""
        return all([
            self.goal is not None,
            self.contract_spec is not None,
            self.boundary_map is not None,
            self.code_placement is not None,
            len(self.quality_gates) > 0,
            self.checkpoints is not None,
        ])

    def get_completion_percentage(self) -> float:
        """Calculate completion percentage based on dod_items."""
        if not self.dod_items:
            return 0.0
        completed = sum(1 for item in self.dod_items if item.completed)
        return (completed / len(self.dod_items)) * 100
