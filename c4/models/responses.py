"""C4 Response Models - MCP tool response schemas"""

from typing import Literal

from pydantic import BaseModel


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
