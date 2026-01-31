"""C4 Core API Routes.

Provides endpoints for core C4 orchestration:
- GET /status - Get current C4 status
- POST /get-task - Get task assignment for worker
- POST /submit - Submit completed task
- POST /add-task - Add new task to queue
- POST /start - Transition to EXECUTE state
- POST /checkpoint - Record checkpoint decision
- POST /mark-blocked - Mark task as blocked
"""

import logging
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import get_daemon, require_execute_state, require_initialized_project
from ..models import (
    AddTaskRequest,
    CheckpointRequest,
    CheckpointResponse,
    ErrorResponse,
    GetTaskRequest,
    GetTaskResponse,
    StartResponse,
    StatusResponse,
    SubmitRequest,
    SubmitResponse,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/c4", tags=["C4 Core"])


@router.get(
    "/status",
    response_model=StatusResponse,
    responses={500: {"model": ErrorResponse}},
    summary="Get C4 Status",
    description="Returns current C4 state, task queue summary, and active workers.",
)
async def get_status(daemon: C4Daemon = Depends(get_daemon)) -> StatusResponse:
    """Get current C4 project status."""
    try:
        result = daemon.c4_status()
        return StatusResponse(
            state=result.get("state", "UNKNOWN"),
            queue=result.get("queue", {}),
            workers=result.get("workers", {}),
            project_root=result.get("project_root"),
        )
    except Exception as e:
        logger.error(f"Failed to get status: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/get-task",
    response_model=GetTaskResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Get Task Assignment",
    description="Request a task assignment for a worker.",
)
async def get_task(
    request: GetTaskRequest,
    daemon: C4Daemon = Depends(require_execute_state),
) -> GetTaskResponse:
    """Get next available task for a worker."""
    try:
        result = daemon.c4_get_task(request.worker_id)

        # Check if task was assigned
        if "task_id" in result:
            return GetTaskResponse(
                task_id=result.get("task_id"),
                title=result.get("title"),
                dod=result.get("dod"),
                scope=result.get("scope"),
                domain=result.get("domain"),
                dependencies=result.get("dependencies", []),
            )
        else:
            return GetTaskResponse(
                message=result.get("message", "No tasks available"),
            )
    except Exception as e:
        logger.error(f"Failed to get task: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/submit",
    response_model=SubmitResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Submit Completed Task",
    description="Submit a completed task with validation results.",
)
async def submit_task(
    request: SubmitRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> SubmitResponse:
    """Submit a completed task with validation results."""
    try:
        # Convert validation results to dict format
        validation_results = [
            {"name": v.name, "status": v.status.value, "message": v.message}
            for v in request.validation_results
        ]

        result = daemon.c4_submit(
            task_id=request.task_id,
            commit_sha=request.commit_sha,
            validation_results=validation_results,
            worker_id=request.worker_id,
        )

        return SubmitResponse(
            success=result.get("success", False),
            message=result.get("message", ""),
            next_task=result.get("next_task"),
        )
    except Exception as e:
        logger.error(f"Failed to submit task: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/add-task",
    responses={400: {"model": ErrorResponse}},
    summary="Add Task to Queue",
    description="Add a new task to the task queue with optional dependencies and priority.",
)
async def add_task(
    request: AddTaskRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Add a new task to the queue."""
    try:
        result = daemon.c4_add_todo(
            task_id=request.task_id,
            title=request.title,
            dod=request.dod,
            scope=request.scope,
            domain=request.domain,
            priority=request.priority,
            dependencies=request.dependencies,
        )
        return result
    except Exception as e:
        logger.error(f"Failed to add task: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/start",
    response_model=StartResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Start Execution",
    description="Transition from PLAN/HALTED state to EXECUTE state.",
)
async def start_execution(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> StartResponse:
    """Start execution by transitioning to EXECUTE state."""
    try:
        result = daemon.c4_start()
        return StartResponse(
            success=result.get("success", False),
            new_state=result.get("state", "UNKNOWN"),
            message=result.get("message", ""),
        )
    except Exception as e:
        logger.error(f"Failed to start execution: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/checkpoint",
    response_model=CheckpointResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Record Checkpoint Decision",
    description="Record a supervisor checkpoint decision (APPROVE, REQUEST_CHANGES, REPLAN).",
)
async def record_checkpoint(
    request: CheckpointRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> CheckpointResponse:
    """Record a checkpoint decision."""
    try:
        result = daemon.c4_checkpoint(
            checkpoint_id=request.checkpoint_id,
            decision=request.decision.value,
            notes=request.notes,
            required_changes=request.required_changes,
        )
        return CheckpointResponse(
            success=result.get("success", False),
            message=result.get("message", ""),
            new_state=result.get("new_state"),
        )
    except Exception as e:
        logger.error(f"Failed to record checkpoint: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/mark-blocked",
    responses={400: {"model": ErrorResponse}},
    summary="Mark Task as Blocked",
    description="Mark a task as blocked after max retry attempts. Adds to repair queue.",
)
async def mark_blocked(
    task_id: str,
    worker_id: str,
    failure_signature: str,
    attempts: int,
    last_error: str | None = None,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Mark a task as blocked."""
    try:
        result = daemon.c4_mark_blocked(
            task_id=task_id,
            worker_id=worker_id,
            failure_signature=failure_signature,
            attempts=attempts,
            last_error=last_error,
        )
        return result
    except Exception as e:
        logger.error(f"Failed to mark task as blocked: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


# NOTE: /ensure-supervisor endpoint removed
# SupervisorLoop has been replaced by unified queue architecture.
# Checkpoint processing is now handled via CP-XXX tasks.
# Repair processing is now handled via RPR-XXX tasks.
