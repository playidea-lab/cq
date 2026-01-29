"""Notification hooks for C4 events.

These hooks integrate the notification system with C4 events
such as task completion, checkpoint reaching, and worker staleness.
"""

import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from c4.models import ValidationResult

logger = logging.getLogger(__name__)


def on_task_complete(
    task_id: str,
    success: bool,
    validation_results: list["ValidationResult"] | None = None,
    summary: str | None = None,
) -> bool:
    """Send notification when a task completes.

    Args:
        task_id: The completed task ID.
        success: Whether the task succeeded.
        validation_results: Optional list of validation results.
        summary: Optional summary message.

    Returns:
        True if notification was sent successfully.
    """
    from c4.notification.manager import notify_task_complete

    # Build summary from validation results if not provided
    if summary is None and validation_results:
        failed = [r for r in validation_results if r.status == "fail"]
        if failed:
            summary = f"{task_id}: {len(failed)} validation(s) failed"
        else:
            summary = f"{task_id}: All validations passed"

    try:
        return notify_task_complete(task_id, success, summary)
    except Exception as e:
        logger.debug(f"Failed to send task completion notification: {e}")
        return False


def on_checkpoint_reached(
    checkpoint_id: str,
    task_ids: list[str] | None = None,
) -> bool:
    """Send notification when a checkpoint is reached.

    Args:
        checkpoint_id: The checkpoint ID.
        task_ids: Optional list of related task IDs.

    Returns:
        True if notification was sent successfully.
    """
    from c4.notification.manager import notify_checkpoint

    if task_ids:
        message = f"{checkpoint_id} requires review ({len(task_ids)} tasks)"
    else:
        message = f"{checkpoint_id} requires review"

    try:
        return notify_checkpoint(checkpoint_id, message)
    except Exception as e:
        logger.debug(f"Failed to send checkpoint notification: {e}")
        return False


def on_worker_stale(
    worker_id: str,
    task_id: str | None = None,
    elapsed_seconds: float = 0,
) -> bool:
    """Send notification when a worker becomes stale.

    Args:
        worker_id: The stale worker ID.
        task_id: Optional task ID the worker was working on.
        elapsed_seconds: How long the worker has been unresponsive.

    Returns:
        True if notification was sent successfully.
    """
    from c4.notification.manager import notify_worker_event

    try:
        return notify_worker_event("stale", worker_id, task_id)
    except Exception as e:
        logger.debug(f"Failed to send worker stale notification: {e}")
        return False
