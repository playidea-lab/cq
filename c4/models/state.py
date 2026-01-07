"""C4 State Models - Main state schema and related models"""

from datetime import datetime

from pydantic import BaseModel, Field

from .checkpoint import CheckpointState
from .enums import ExecutionMode, ProjectStatus
from .worker import WorkerInfo


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


class TaskQueue(BaseModel):
    """Task queue state"""

    pending: list[str] = Field(default_factory=list)
    in_progress: dict[str, str] = Field(default_factory=dict)  # task_id → worker_id
    done: list[str] = Field(default_factory=list)


class Metrics(BaseModel):
    """Runtime metrics"""

    events_emitted: int = 0
    validations_run: int = 0
    tasks_completed: int = 0
    checkpoints_passed: int = 0


class C4State(BaseModel):
    """Main state.json schema - Single Source of Truth"""

    project_id: str
    status: ProjectStatus = ProjectStatus.INIT
    execution_mode: ExecutionMode | None = None
    checkpoint: CheckpointState = Field(default_factory=CheckpointState)
    passed_checkpoints: list[str] = Field(default_factory=list)  # List of passed checkpoint IDs
    queue: TaskQueue = Field(default_factory=TaskQueue)
    workers: dict[str, WorkerInfo] = Field(default_factory=dict)
    locks: LocksState = Field(default_factory=LocksState)
    last_validation: dict[str, str] | None = None  # validation_name → "pass"/"fail"
    metrics: Metrics = Field(default_factory=Metrics)
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)
