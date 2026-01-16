"""C4 Web Dashboard Module.

Provides team dashboard functionality:
- Team project list
- Realtime progress status
- Worker/review status
"""

from c4.web.dashboard import (
    DashboardService,
    ProjectSummary,
    RealtimeStatus,
    ReviewStatus,
    WorkerStatus,
)

__all__ = [
    "DashboardService",
    "ProjectSummary",
    "RealtimeStatus",
    "WorkerStatus",
    "ReviewStatus",
]
