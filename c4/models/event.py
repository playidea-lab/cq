"""C4 Event Models - Event log entry schema"""

from datetime import datetime

from pydantic import BaseModel, Field

from .enums import EventType


class Event(BaseModel):
    """Event log entry"""

    id: str  # 6-digit sequential ID
    ts: datetime = Field(default_factory=datetime.now)
    type: EventType
    actor: str  # "c4d", "worker-1", etc.
    data: dict = Field(default_factory=dict)
