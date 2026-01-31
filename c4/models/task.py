"""C4 Task Models - Task and validation result schemas"""

from typing import Literal

from pydantic import BaseModel, Field

from .ddd import (
    BoundaryMap,
    CheckpointDefinition,
    CodePlacement,
    ContractSpec,
    DoDItem,
    Goal,
    QualityGate,
    WorkerPacket,
)
from .enums import TaskStatus, TaskType


class ValidationResult(BaseModel):
    """Result of a validation run"""

    name: str
    status: Literal["pass", "fail"]
    duration_ms: int | None = None
    message: str | None = None
    coverage: float | None = None


class Task(BaseModel):
    """Task definition for C4 system.

    Supports Review-as-Task workflow with versioned task IDs:
    - Implementation tasks: T-{number}-{version} (e.g., T-001-0)
    - Review tasks: R-{number}-{version} (e.g., R-001-0)

    Phase 7+ (DDD-CLEANCODE): Extended with structured specifications
    - goal: Clear done/out-of-scope definition
    - contract_spec: API contracts and test requirements
    - boundary_map: DDD layer constraints
    - code_placement: File locations
    - quality_gates: Validation commands
    - checkpoints: CP1/CP2/CP3 milestones
    - dod_items: Parsed DoD checklist
    """

    id: str
    title: str
    scope: str | None = None
    priority: int = 0
    dod: str  # Definition of Done (legacy, kept for compatibility)
    validations: list[str] = Field(default_factory=lambda: ["lint", "unit"])
    dependencies: list[str] = Field(default_factory=list)
    status: TaskStatus = TaskStatus.PENDING
    assigned_to: str | None = None
    branch: str | None = None
    commit_sha: str | None = None
    # Phase 4: Agent routing - task-specific domain override
    domain: str | None = None
    task_type: str | None = None  # For skill matching (e.g., "review", "debug", "security")

    # Review-as-Task fields
    type: TaskType = TaskType.IMPLEMENTATION
    base_id: str | None = None  # Base task number, e.g., "001" from T-001-0
    version: int = 0  # Version number, 0 = original, 1+ = revisions
    parent_id: str | None = None  # Parent task ID (R-001-0 -> T-001-0)
    completed_by: str | None = None  # Worker who completed parent task
    review_comments: str | None = None  # Comments from REQUEST_CHANGES

    # Economic mode: model selection for cost optimization
    model: str = Field(
        default="opus",
        pattern="^(sonnet|opus|haiku)$",
        description="Claude model for this task (sonnet, opus, haiku). Default: opus",
    )

    # Checkpoint-as-Task fields
    phase_id: str | None = None  # Phase identifier (e.g., "001", "phase-1")
    required_tasks: list[str] = Field(default_factory=list)  # Tasks to verify (CP only)
    review_decision: str | None = None  # APPROVE, REQUEST_CHANGES, REPLAN (R/CP only)

    # Unified Queue: Multiple completion support (for CHECKPOINT type)
    required_completions: int = Field(
        default=1,
        ge=1,
        description="Number of completions required (CP tasks default to 2)",
    )
    completion_count: int = Field(
        default=0,
        ge=0,
        description="Current completion count",
    )
    completed_by_sessions: list[str] = Field(
        default_factory=list,
        description="Session/worker IDs that completed this task",
    )

    # Repair-as-Task fields
    original_task_id: str | None = Field(
        None,
        description="Original blocked task ID (for REPAIR type)",
    )
    failure_signature: str | None = Field(
        None,
        description="Error signature from validation failures (for REPAIR type)",
    )
    repair_guidance: str | None = Field(
        None,
        description="AI-generated repair guidance (for REPAIR type)",
    )

    # ==========================================================================
    # DDD-CLEANCODE Fields (Phase 7+)
    # ==========================================================================

    # Goal specification
    goal: Goal | None = Field(None, description="Clear done/out-of-scope definition")

    # Contract specification
    contract_spec: ContractSpec | None = Field(
        None, description="API contracts and test requirements"
    )

    # Boundary constraints
    boundary_map: BoundaryMap | None = Field(
        None, description="DDD layer constraints and import rules"
    )

    # File placement
    code_placement: CodePlacement | None = Field(
        None, description="File locations for implementation and tests"
    )

    # Quality gates (extends validations)
    quality_gates: list[QualityGate] = Field(
        default_factory=list, description="Detailed validation commands"
    )

    # Checkpoint definitions
    checkpoints: CheckpointDefinition | None = Field(
        None, description="CP1/CP2/CP3 milestone definitions"
    )

    # Parsed DoD items
    dod_items: list[DoDItem] = Field(
        default_factory=list, description="Parsed DoD checklist items"
    )

    # Current checkpoint progress
    current_checkpoint: Literal["cp1", "cp2", "cp3", "done"] | None = Field(
        None, description="Current checkpoint stage"
    )

    # ==========================================================================
    # Methods
    # ==========================================================================

    def to_worker_packet(self) -> WorkerPacket:
        """Convert task to WorkerPacket for structured processing."""
        return WorkerPacket(
            goal=self.goal,
            contract_spec=self.contract_spec,
            boundary_map=self.boundary_map,
            code_placement=self.code_placement,
            quality_gates=self.quality_gates,
            checkpoints=self.checkpoints,
            dod_items=self.dod_items,
        )

    def is_fully_specified(self) -> bool:
        """Check if task has complete DDD-CLEANCODE specifications."""
        return self.to_worker_packet().is_fully_specified()

    def get_dod_completion_percentage(self) -> float:
        """Get DoD completion percentage."""
        if not self.dod_items:
            return 0.0
        completed = sum(1 for item in self.dod_items if item.completed)
        return (completed / len(self.dod_items)) * 100

    def get_files_to_create(self) -> list[str]:
        """Get list of files to create from code_placement."""
        if self.code_placement:
            return self.code_placement.create
        return []

    def get_files_to_modify(self) -> list[str]:
        """Get list of files to modify from code_placement."""
        if self.code_placement:
            return self.code_placement.modify
        return []

    def get_test_files(self) -> list[str]:
        """Get list of test files from code_placement."""
        if self.code_placement:
            return self.code_placement.tests
        return []

    def get_all_affected_files(self) -> list[str]:
        """Get all files that will be affected by this task."""
        files = []
        files.extend(self.get_files_to_create())
        files.extend(self.get_files_to_modify())
        files.extend(self.get_test_files())
        return files
