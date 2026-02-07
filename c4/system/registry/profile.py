"""User Profile models for C4.

Defines the UserProfile schema that captures user preferences,
review style, writing habits, and domain expertise.
These are learned from observable user behavior at checkpoints.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field


class ReviewStyle(BaseModel):
    """How the user prefers code/paper reviews."""

    thoroughness: Literal["quick-scan", "balanced", "deep-dive"] = "balanced"
    tone: Literal["direct", "diplomatic", "socratic"] = "diplomatic"
    focus: list[str] = Field(default_factory=lambda: ["correctness", "clarity"])
    strictness: float = Field(
        default=0.5,
        ge=0.0,
        le=1.0,
        description="0.0=lenient, 1.0=strict. Learned from REQUEST_CHANGES ratio.",
    )
    paper_criteria: list[str] = Field(default_factory=list)


class WritingStyle(BaseModel):
    """User's writing preferences."""

    language: str = "en"
    formality: Literal["casual", "professional", "academic"] = "professional"
    verbosity: Literal["concise", "moderate", "detailed"] = "moderate"
    citation_style: str | None = None


class CommunicationPreferences(BaseModel):
    """How the user prefers communication."""

    explanation_depth: Literal["minimal", "standard", "educational"] = "standard"
    dod_detail_level: Literal["brief", "standard", "exhaustive"] = "standard"


class DomainExpertise(BaseModel):
    """User's domain knowledge levels."""

    domains: dict[str, Literal["beginner", "intermediate", "expert"]] = Field(
        default_factory=dict
    )
    research_fields: list[str] = Field(default_factory=list)


class UserProfile(BaseModel):
    """Complete user profile for C4 personalization.

    Stored at ~/.c4/profile.yaml or .c4/profiles/{user}.yaml.
    Updated automatically from checkpoint observations.
    """

    name: str = "default"
    version: int = 0
    last_updated: str | None = None
    review: ReviewStyle = Field(default_factory=ReviewStyle)
    writing: WritingStyle = Field(default_factory=WritingStyle)
    communication: CommunicationPreferences = Field(
        default_factory=CommunicationPreferences
    )
    expertise: DomainExpertise = Field(default_factory=DomainExpertise)
    persona_overrides: dict[str, str] = Field(default_factory=dict)
    learned_from: dict[str, int] = Field(default_factory=dict)
