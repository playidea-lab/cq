"""C4 Queue Models - Checkpoint and Repair queue items for async processing"""

from pydantic import BaseModel, Field

from .task import ValidationResult


class CheckpointQueueItem(BaseModel):
    """Item in the checkpoint queue awaiting supervisor review"""

    checkpoint_id: str = Field(..., description="ID of the triggered checkpoint")
    triggered_at: str = Field(..., description="ISO timestamp when checkpoint was triggered")
    tasks_completed: list[str] = Field(
        default_factory=list, description="Task IDs completed before this checkpoint"
    )
    validation_results: list[ValidationResult] = Field(
        default_factory=list, description="Validation results at checkpoint time"
    )


class RepairQueueItem(BaseModel):
    """Item in the repair queue for blocked tasks needing supervisor guidance"""

    task_id: str = Field(..., description="ID of the blocked task")
    worker_id: str = Field(..., description="Worker that was working on this task")
    failure_signature: str = Field(
        ..., description="Error signature from validation failures"
    )
    attempts: int = Field(..., description="Number of fix attempts made")
    blocked_at: str = Field(..., description="ISO timestamp when task was blocked")
    last_error: str = Field(default="", description="Last error message")
