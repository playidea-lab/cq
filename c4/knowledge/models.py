"""Knowledge data models."""

from __future__ import annotations

from datetime import datetime, timezone
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class HypothesisStatus(str, Enum):
    """Hypothesis verification status."""

    PROPOSED = "proposed"
    TESTING = "testing"
    SUPPORTED = "supported"
    REFUTED = "refuted"
    INCONCLUSIVE = "inconclusive"


class ExperimentResult(BaseModel):
    """Experiment outcome."""

    metrics: dict[str, float] = Field(default_factory=dict)
    success: bool = True
    error_message: str | None = None
    duration_seconds: float = 0.0
    gpu_memory_peak_gb: float | None = None


class Observation(BaseModel):
    """An observation during an experiment."""

    timestamp: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )
    content: str = ""
    source: str = "auto"  # auto, user, llm


class ExperimentKnowledge(BaseModel):
    """A complete experiment knowledge record."""

    id: str = ""
    task_id: str = ""
    title: str = ""
    hypothesis: str = ""
    hypothesis_status: HypothesisStatus = HypothesisStatus.PROPOSED
    config: dict[str, Any] = Field(default_factory=dict)
    result: ExperimentResult = Field(default_factory=ExperimentResult)
    observations: list[Observation] = Field(default_factory=list)
    lessons_learned: list[str] = Field(default_factory=list)
    tags: list[str] = Field(default_factory=list)
    domain: str = ""
    created_at: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )


class Pattern(BaseModel):
    """A mined pattern from experiments."""

    id: str = ""
    name: str = ""
    description: str = ""
    domain: str = ""
    confidence: float = 0.0  # 0.0 - 1.0
    evidence_count: int = 0
    evidence_ids: list[str] = Field(default_factory=list)
    config_pattern: dict[str, Any] = Field(default_factory=dict)
    created_at: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )
