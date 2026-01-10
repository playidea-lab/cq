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
