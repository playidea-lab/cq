"""C4 State Models - Main state schema and related models"""

from datetime import datetime
from typing import Any

from pydantic import BaseModel, Field, field_validator

from .checkpoint import CheckpointState
from .enums import ExecutionMode, ProjectStatus
from .queue import CheckpointQueueItem, RepairQueueItem
from .worker import WorkerInfo


def _extract_task_id(item: Any) -> str:
    """Extract task ID from various formats (defensive parsing).

    Handles:
    - str: Return as-is (correct format)
    - dict with 'id': Extract the ID (corrupt format from old data)
    - Other: Convert to string (fallback)
    """
    if isinstance(item, str):
        return item
    elif isinstance(item, dict) and "id" in item:
        return str(item["id"])
    else:
        return str(item)


def _extract_worker_id(value: Any) -> str:
    """Extract worker ID from various formats (defensive parsing).

    Handles:
    - str: Return as-is (correct format)
    - dict with worker info: Extract worker_id or assigned_to
    - Other: Convert to string (fallback)
    """
    if isinstance(value, str):
        return value
    elif isinstance(value, dict):
        # Try common worker ID fields
        for key in ("worker_id", "assigned_to", "owner"):
            if key in value:
                return str(value[key])
        # Fallback: return first string value
        for v in value.values():
            if isinstance(v, str):
                return v
        return str(value)
    else:
        return str(value)


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
    """Task queue state.

    Note: Validators handle corrupt data formats where full task objects
    were stored instead of just task IDs. This provides self-healing
    when loading legacy/corrupt database entries.
    """

    pending: list[str] = Field(default_factory=list)
    in_progress: dict[str, str] = Field(default_factory=dict)  # task_id → worker_id
    done: list[str] = Field(default_factory=list)

    @field_validator("pending", "done", mode="before")
    @classmethod
    def extract_task_ids_from_list(cls, v: Any) -> list[str]:
        """Extract task IDs from list, handling corrupt dict entries."""
        if not isinstance(v, list):
            return []
        return [_extract_task_id(item) for item in v]

    @field_validator("in_progress", mode="before")
    @classmethod
    def extract_worker_ids_from_dict(cls, v: Any) -> dict[str, str]:
        """Extract worker IDs from dict, handling corrupt dict entries."""
        if not isinstance(v, dict):
            return {}
        return {_extract_task_id(k): _extract_worker_id(val) for k, val in v.items()}


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
    # Async queues for automation
    checkpoint_queue: list[CheckpointQueueItem] = Field(
        default_factory=list, description="Pending checkpoints awaiting supervisor review"
    )
    repair_queue: list[RepairQueueItem] = Field(
        default_factory=list, description="Blocked tasks awaiting supervisor guidance"
    )
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)
