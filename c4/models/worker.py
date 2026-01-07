"""C4 Worker Models - Worker state information"""

from datetime import datetime
from typing import Literal

from pydantic import BaseModel


class WorkerInfo(BaseModel):
    """Worker state information"""

    worker_id: str
    state: Literal["idle", "busy", "disconnected"]
    task_id: str | None = None
    scope: str | None = None
    branch: str | None = None
    joined_at: datetime
    last_seen: datetime | None = None
