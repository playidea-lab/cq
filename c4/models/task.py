"""C4 Task Models - Task and validation result schemas"""

from typing import Literal

from pydantic import BaseModel, Field

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
    """

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
    # Phase 4: Agent routing - task-specific domain override
    domain: str | None = None

    # Review-as-Task fields
    type: TaskType = TaskType.IMPLEMENTATION
    base_id: str | None = None  # Base task number, e.g., "001" from T-001-0
    version: int = 0  # Version number, 0 = original, 1+ = revisions
    parent_id: str | None = None  # Parent task ID (R-001-0 -> T-001-0)
    completed_by: str | None = None  # Worker who completed parent task
    review_comments: str | None = None  # Comments from REQUEST_CHANGES
