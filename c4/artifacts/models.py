"""Artifact data models."""

from __future__ import annotations

from datetime import datetime, timezone

from pydantic import BaseModel, Field


class ArtifactRecord(BaseModel):
    """A stored artifact record."""

    task_id: str
    name: str
    type: str = Field(
        default="output",
        pattern="^(source|data|output)$",
    )
    content_hash: str  # SHA256
    size_bytes: int = 0
    version: int = 1
    local_path: str = ""
    created_at: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )


class ArtifactVersion(BaseModel):
    """Version history entry for an artifact."""

    version: int
    content_hash: str
    size_bytes: int = 0
    created_at: str = ""
