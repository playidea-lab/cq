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
    description: str = ""  # Human-readable description
    required_tasks: list[str] = Field(default_factory=list)
    required_validations: list[str] = Field(default_factory=lambda: ["lint", "unit"])
    auto_approve: bool = True  # Default: AI auto-review. Set False for human review


# Default checkpoints created on c4 init
DEFAULT_CHECKPOINTS: list[CheckpointConfig] = [
    CheckpointConfig(
        id="CP-REVIEW",
        description="코드 리뷰 완료 후 Supervisor 검토",
        required_tasks=[],
        required_validations=["lint"],
        auto_approve=False,
    ),
    CheckpointConfig(
        id="CP-FINAL",
        description="모든 작업 완료 후 최종 검토",
        required_tasks=[],
        required_validations=["lint", "unit"],
        auto_approve=False,
    ),
]
