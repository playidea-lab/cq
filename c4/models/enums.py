"""C4 Enums - All enumeration types for the C4 system"""

from enum import Enum


class ProjectStatus(str, Enum):
    """Top-level project states"""

    INIT = "INIT"
    DISCOVERY = "DISCOVERY"  # New: Domain detection + interview
    DESIGN = "DESIGN"  # New: Architecture design
    PLAN = "PLAN"
    EXECUTE = "EXECUTE"
    CHECKPOINT = "CHECKPOINT"
    COMPLETE = "COMPLETE"
    HALTED = "HALTED"
    ERROR = "ERROR"


class ExecutionMode(str, Enum):
    """Sub-states within EXECUTE"""

    RUNNING = "running"
    PAUSED = "paused"
    REPAIR = "repair"


class TaskStatus(str, Enum):
    """Task queue status"""

    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    DONE = "done"
    BLOCKED = "blocked"


class TaskType(str, Enum):
    """Task type for unified task queue system.

    All task types are processed through the same worker loop:
    - IMPLEMENTATION: T-XXX-N (구현 작업)
    - REVIEW: R-XXX-N (단일 태스크 검토, 수직적)
    - CHECKPOINT: CP-XXX (줄기 합류점 검토, 수평적, 2회 완료 필요)
    - REPAIR: RPR-XXX (실패 태스크 수정)
    """

    IMPLEMENTATION = "impl"  # Implementation task (T-XXX-N)
    REVIEW = "review"  # Review task (R-XXX-N)
    CHECKPOINT = "checkpoint"  # Checkpoint task (CP-XXX)
    REPAIR = "repair"  # Repair task (RPR-XXX)


class SupervisorDecision(str, Enum):
    """Supervisor checkpoint decisions"""

    APPROVE = "APPROVE"
    REQUEST_CHANGES = "REQUEST_CHANGES"
    REPLAN = "REPLAN"


class EventType(str, Enum):
    """Event types for event log"""

    LEADER_STARTED = "LEADER_STARTED"
    WORKER_JOINED = "WORKER_JOINED"
    WORKER_LEFT = "WORKER_LEFT"
    TASK_ASSIGNED = "TASK_ASSIGNED"
    WORKER_SUBMITTED = "WORKER_SUBMITTED"
    VALIDATION_STARTED = "VALIDATION_STARTED"
    VALIDATION_FINISHED = "VALIDATION_FINISHED"
    VALIDATION_RUN = "VALIDATION_RUN"
    CHECKPOINT_REQUIRED = "CHECKPOINT_REQUIRED"
    SUPERVISOR_DECISION = "SUPERVISOR_DECISION"
    STATE_CHANGED = "STATE_CHANGED"
    ERROR = "ERROR"
