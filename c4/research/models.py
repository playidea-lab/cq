"""Research module Pydantic models.

Tracks paper-experiment iteration loops: projects hold multiple iterations,
each iteration records review scores, identified gaps, and experiment results.
"""

from __future__ import annotations

import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel


class ProjectStatus(str, Enum):
    ACTIVE = "active"
    PAUSED = "paused"
    COMPLETED = "completed"


class IterationStatus(str, Enum):
    REVIEWING = "reviewing"
    PLANNING = "planning"
    EXPERIMENTING = "experimenting"
    DONE = "done"


class ResearchProject(BaseModel):
    id: str
    name: str
    paper_path: str | None = None
    repo_path: str | None = None
    target_score: float = 7.0
    current_iteration: int = 0
    status: ProjectStatus = ProjectStatus.ACTIVE
    created_at: datetime.datetime | None = None
    updated_at: datetime.datetime | None = None


class Iteration(BaseModel):
    id: str
    project_id: str
    iteration_num: int
    review_score: float | None = None
    axis_scores: dict[str, Any] | None = None
    gaps: list[dict[str, Any]] | None = None
    experiments: list[dict[str, Any]] | None = None
    status: IterationStatus = IterationStatus.REVIEWING
    started_at: datetime.datetime | None = None
    completed_at: datetime.datetime | None = None
