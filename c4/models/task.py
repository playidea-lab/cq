"""C4 Task Models - Task and validation result schemas"""

from typing import Literal

from pydantic import BaseModel, Field

from .enums import TaskStatus


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
