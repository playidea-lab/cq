"""C4 Daemon - Core daemon components for multi-worker orchestration.

Note (Unified Queue Architecture):
SupervisorLoop has been removed. Checkpoint and repair tasks are now
processed through the unified task queue as CP-XXX and RPR-XXX task types.
"""

from .c4_daemon import C4Daemon, _get_workflow_guide, _use_graph_router
from .code_ops import CodeOps
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
from .pr_manager import PRManager, PRResult
from .repair_analyzer import (
    FailureAnalysis,
    FailureAnalyzer,
    FailureCategory,
    RepairMetrics,
    RepairSuggestionGenerator,
)
from .safety import SafetyGuard
from .task_dispatcher import (
    AssignmentResult,
    TaskAssignment,
    TaskDispatcher,
    TaskPriority,
)
from .task_ops import TaskOps
from .workers import WorkerManager

__all__ = [
    "AssignmentResult",
    "C4Daemon",
    "_get_workflow_guide",
    "_use_graph_router",
    "CodeOps",
    "TaskOps",
    "DaemonInfo",
    "DaemonLifecycle",
    "DaemonStatus",
    "EventBus",
    "FailureAnalysis",
    "FailureAnalyzer",
    "FailureCategory",
    "GitOperations",
    "GitResult",
    "HealthMonitor",
    "HealthMonitorConfig",
    "OverallHealth",
    "PRManager",
    "PRResult",
    "RepairMetrics",
    "RepairSuggestionGenerator",
    "SafetyGuard",
    "ServiceHealth",
    "ServiceStatus",
    "TaskAssignment",
    "TaskDispatcher",
    "TaskPriority",
    "WorkerManager",
    "check_port_available",
    "check_service_reachable",
]
