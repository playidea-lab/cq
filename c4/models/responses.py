"""C4 Response Models - MCP tool response schemas"""

from typing import Literal

from pydantic import BaseModel


class TaskAssignment(BaseModel):
    """Response for c4_get_task()

    Includes agent routing information (Phase 4) for automatic
    agent selection and chaining based on task domain.

    Includes worktree path for multi-worker isolation.
    Each worker gets its own isolated working directory at:
    .c4/worktrees/{worker_id}/
    """

    task_id: str
    title: str
    scope: str | None
    dod: str
    validations: list[str]
    branch: str
    # Phase 4: Agent routing fields
    recommended_agent: str | None = None
    agent_chain: list[str] | None = None
    domain: str | None = None
    handoff_instructions: str | None = None
    # Worktree isolation field
    worktree_path: str | None = None


class SubmitResponse(BaseModel):
    """Response for c4_submit()"""

    success: bool
    next_action: Literal[
        "get_next_task", "await_checkpoint", "fix_failures", "complete", "escalate"
    ]
    message: str | None = None


class CheckpointResponse(BaseModel):
    """Response for c4_checkpoint()"""

    success: bool
    message: str | None = None
