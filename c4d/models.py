"""C4D Data Models - Pydantic schemas for state, tasks, events"""

from datetime import datetime
from enum import Enum
from typing import Literal

from pydantic import BaseModel, Field


# =============================================================================
# Enums
# =============================================================================


class ProjectStatus(str, Enum):
    """Top-level project states"""

    INIT = "INIT"
    PLAN = "PLAN"
    EXECUTE = "EXECUTE"
    CHECKPOINT = "CHECKPOINT"
    COMPLETE = "COMPLETE"
    HALTED = "HALTED"
    ERROR = "ERROR"


class ExecutionMode(str, Enum):
    """Sub-states within EXECUTE"""

    RUNNING = "running"
    PAUSED = "paused"
    REPAIR = "repair"


class TaskStatus(str, Enum):
    """Task queue status"""

    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    DONE = "done"
    BLOCKED = "blocked"


class SupervisorDecision(str, Enum):
    """Supervisor checkpoint decisions"""

    APPROVE = "APPROVE"
    REQUEST_CHANGES = "REQUEST_CHANGES"
    REPLAN = "REPLAN"


class EventType(str, Enum):
    """Event types for event log"""

    LEADER_STARTED = "LEADER_STARTED"
    WORKER_JOINED = "WORKER_JOINED"
    TASK_ASSIGNED = "TASK_ASSIGNED"
    WORKER_SUBMITTED = "WORKER_SUBMITTED"
    VALIDATION_STARTED = "VALIDATION_STARTED"
    VALIDATION_FINISHED = "VALIDATION_FINISHED"
    VALIDATION_RUN = "VALIDATION_RUN"  # Combined validation run event
    CHECKPOINT_REQUIRED = "CHECKPOINT_REQUIRED"
    SUPERVISOR_DECISION = "SUPERVISOR_DECISION"
    STATE_CHANGED = "STATE_CHANGED"
    ERROR = "ERROR"


# =============================================================================
# Task Models
# =============================================================================


class ValidationResult(BaseModel):
    """Result of a validation run"""

    name: str
    status: Literal["pass", "fail"]
    duration_ms: int | None = None
    message: str | None = None
    coverage: float | None = None


class Task(BaseModel):
    """Task definition parsed from todo.md"""

    id: str
    title: str
    scope: str | None = None
    priority: int = 0
    dod: str  # Definition of Done
    validations: list[str] = Field(default_factory=lambda: ["lint", "unit"])
    dependencies: list[str] = Field(default_factory=list)
    status: TaskStatus = TaskStatus.PENDING
    assigned_to: str | None = None
    branch: str | None = None
    commit_sha: str | None = None


# =============================================================================
# Worker Models
# =============================================================================


class WorkerInfo(BaseModel):
    """Worker state information"""

    worker_id: str
    state: Literal["idle", "busy", "disconnected"]
    task_id: str | None = None
    scope: str | None = None
    branch: str | None = None
    joined_at: datetime
    last_seen: datetime | None = None


# =============================================================================
# Lock Models
# =============================================================================


class LeaderLock(BaseModel):
    """Leader lock information"""

    owner: str = "c4d"
    pid: int
    started_at: datetime


class ScopeLock(BaseModel):
    """Scope lock for concurrent worker protection"""

    owner: str  # worker_id
    scope: str
    expires_at: datetime


class LocksState(BaseModel):
    """All locks state"""

    leader: LeaderLock | None = None
    scopes: dict[str, ScopeLock] = Field(default_factory=dict)


# =============================================================================
# Checkpoint Models
# =============================================================================


class CheckpointState(BaseModel):
    """Current checkpoint state"""

    current: str | None = None  # e.g., "CP0", "CP1"
    state: Literal["pending", "in_progress", "passed", "failed"] = "pending"


class CheckpointConfig(BaseModel):
    """Checkpoint gate configuration"""

    id: str  # e.g., "CP0"
    required_tasks: list[str] = Field(default_factory=list)
    required_validations: list[str] = Field(default_factory=lambda: ["lint", "unit"])
    auto_approve: bool = False


# =============================================================================
# Queue Models
# =============================================================================


class TaskQueue(BaseModel):
    """Task queue state"""

    pending: list[str] = Field(default_factory=list)
    in_progress: dict[str, str] = Field(default_factory=dict)  # task_id → worker_id
    done: list[str] = Field(default_factory=list)


# =============================================================================
# Metrics
# =============================================================================


class Metrics(BaseModel):
    """Runtime metrics"""

    events_emitted: int = 0
    validations_run: int = 0
    tasks_completed: int = 0
    checkpoints_passed: int = 0


# =============================================================================
# Main State
# =============================================================================


class C4State(BaseModel):
    """Main state.json schema - Single Source of Truth"""

    project_id: str
    status: ProjectStatus = ProjectStatus.INIT
    execution_mode: ExecutionMode | None = None
    checkpoint: CheckpointState = Field(default_factory=CheckpointState)
    queue: TaskQueue = Field(default_factory=TaskQueue)
    workers: dict[str, WorkerInfo] = Field(default_factory=dict)
    locks: LocksState = Field(default_factory=LocksState)
    last_validation: dict[str, str] | None = None  # validation_name → "pass"/"fail"
    metrics: Metrics = Field(default_factory=Metrics)
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)


# =============================================================================
# Event Models
# =============================================================================


class Event(BaseModel):
    """Event log entry"""

    id: str  # 6-digit sequential ID
    ts: datetime = Field(default_factory=datetime.now)
    type: EventType
    actor: str  # "c4d", "worker-1", etc.
    data: dict = Field(default_factory=dict)


# =============================================================================
# Config Models
# =============================================================================


class ValidationConfig(BaseModel):
    """Validation command configuration"""

    commands: dict[str, str] = Field(
        default_factory=lambda: {
            "lint": "npm run lint",
            "unit": "npm test",
            "e2e": "npm run e2e",
        }
    )
    required: list[str] = Field(default_factory=lambda: ["lint", "unit"])


class BudgetConfig(BaseModel):
    """Budget limits"""

    max_iterations_per_task: int = 7
    max_failures_same_signature: int = 3


class C4Config(BaseModel):
    """config.yaml schema"""

    project_id: str
    default_branch: str = "main"
    work_branch_prefix: str = "c4/w-"
    poll_interval_ms: int = 1000
    max_idle_minutes: int = 0  # 0 = unlimited
    scope_lock_ttl_sec: int = 3600
    validation: ValidationConfig = Field(default_factory=ValidationConfig)
    checkpoints: list[CheckpointConfig] = Field(default_factory=list)
    budgets: BudgetConfig = Field(default_factory=BudgetConfig)


# =============================================================================
# MCP Tool Response Models
# =============================================================================


class TaskAssignment(BaseModel):
    """Response for c4_get_task()"""

    task_id: str
    title: str
    scope: str | None
    dod: str
    validations: list[str]
    branch: str


class SubmitResponse(BaseModel):
    """Response for c4_submit()"""

    success: bool
    next_action: Literal["get_next_task", "await_checkpoint", "fix_failures", "complete"]
    message: str | None = None


class CheckpointResponse(BaseModel):
    """Response for c4_checkpoint()"""

    success: bool
    message: str | None = None
