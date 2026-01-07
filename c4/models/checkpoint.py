"""C4 Checkpoint Models - Checkpoint state and configuration"""

from typing import Literal

from pydantic import BaseModel, Field


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
