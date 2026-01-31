"""C4 Web Dashboard Module.

Provides team dashboard functionality:
- Team project list
- Realtime progress status
- Worker/review status
- WebSocket metrics streaming
"""

from c4.web.dashboard import (
    DashboardService,
    ProjectSummary,
    RealtimeStatus,
    ReviewStatus,
    WorkerStatus,
)
from c4.web.metrics_ws import (
    MetricsMessage,
    MetricsWebSocket,
    create_state_change_callback,
    get_metrics_ws,
)

__all__ = [
    "DashboardService",
    "MetricsMessage",
    "MetricsWebSocket",
    "ProjectSummary",
    "RealtimeStatus",
    "ReviewStatus",
    "WorkerStatus",
    "create_state_change_callback",
    "get_metrics_ws",
]
