"""C4 Daemon - Core daemon components for multi-worker orchestration.

Note (Unified Queue Architecture):
SupervisorLoop has been removed. Checkpoint and repair tasks are now
processed through the unified task queue as CP-XXX and RPR-XXX task types.
"""

from .events import EventBus
from .git_ops import GitOperations, GitResult
from .health import (
    HealthMonitor,
    HealthMonitorConfig,
    OverallHealth,
    ServiceHealth,
    ServiceStatus,
    check_port_available,
    check_service_reachable,
)
from .lifecycle import DaemonInfo, DaemonLifecycle, DaemonStatus
from .safety import SafetyGuard
from .task_dispatcher import (
    AssignmentResult,
    TaskAssignment,
    TaskDispatcher,
    TaskPriority,
)
from .workers import WorkerManager

__all__ = [
    "DaemonInfo",
    "DaemonLifecycle",
    "DaemonStatus",
    "EventBus",
    "GitOperations",
    "GitResult",
    "HealthMonitor",
    "HealthMonitorConfig",
    "OverallHealth",
    "SafetyGuard",
    "ServiceHealth",
    "ServiceStatus",
    "TaskAssignment",
    "AssignmentResult",
    "TaskDispatcher",
    "TaskPriority",
    "WorkerManager",
    "check_port_available",
    "check_service_reachable",
]
