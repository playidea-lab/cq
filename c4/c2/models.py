"""
c2 Pydantic models for document lifecycle management.

Covers: Project, Source, ReadingNote, Artifact, Workspace state.
Reuses auto_review models where applicable.
"""

import datetime
from enum import Enum
from typing import Optional

from pydantic import BaseModel, Field

# --- Enums ---


class ProjectType(str, Enum):
    ACADEMIC_PAPER = "academic_paper"
    PROPOSAL = "proposal"
    REPORT = "report"


class Relevance(str, Enum):
    HIGH = "H"
    MEDIUM = "M"
    LOW = "L"


class SourceStatus(str, Enum):
    DISCOVERED = "발견"
    READING = "읽기중"
    COMPLETED = "완료"


class SourceType(str, Enum):
    PAPER = "paper"
    REPORT = "report"
    WEB = "web"
    INTERNAL = "internal"


class SectionStatus(str, Enum):
    NOT_STARTED = "미시작"
    DRAFTING = "초안"
    REVISING = "수정중"
    COMPLETE = "완료"


class ReviewType(str, Enum):
    EXTERNAL = "external"
    SELF_REVIEW = "self_review"
    COMPLETENESS = "completeness"


class ReviewReflectionStatus(str, Enum):
    PENDING = "미반영"
    PARTIAL = "부분반영"
    DONE = "반영완료"


# --- Source / Discover ---


class Source(BaseModel):
    """A discovered source (paper, report, web page, etc.)."""

    id: str = Field(..., description="Short identifier, e.g. 'smith2024'")
    title: str = Field(..., description="Source title")
    authors: list[str] = Field(default_factory=list)
    year: Optional[int] = None
    type: SourceType = SourceType.PAPER
    url: str = Field(default="")
    relevance: Relevance = Relevance.MEDIUM
    status: SourceStatus = SourceStatus.DISCOVERED
    keywords: list[str] = Field(default_factory=list)
    notes: str = Field(default="", description="1-line summary")
    discovered_at: Optional[datetime.date] = None


# --- Reading Note ---


class ReadingPass(BaseModel):
    """Result of a single reading pass."""

    pass_number: int = Field(..., ge=1, le=3)
    summary: str = Field(default="")
    claims: list[str] = Field(default_factory=list)
    method_notes: str = Field(default="")
    results_notes: str = Field(default="")
    limitations: str = Field(default="")
    connection_to_project: str = Field(default="")


class ReadingNote(BaseModel):
    """Structured reading note for a source."""

    source_id: str
    source_title: str = ""
    relevance: Relevance = Relevance.MEDIUM
    passes: list[ReadingPass] = Field(default_factory=list)
    key_quotes: list[str] = Field(default_factory=list)
    memo: str = Field(default="")

    @property
    def max_pass(self) -> int:
        return max((p.pass_number for p in self.passes), default=0)


# --- Write ---


class SectionState(BaseModel):
    """Status of a document section."""

    name: str
    status: SectionStatus = SectionStatus.NOT_STARTED
    notes: str = Field(default="")


class ClaimEvidence(BaseModel):
    """Mapping from a claim to its evidence."""

    claim: str
    evidence_source: str = Field(default="", description="c4 experiment or source id")
    result: str = Field(default="")
    location: str = Field(default="", description="Figure/Table/Section reference")


# --- Review ---


class ReviewRecord(BaseModel):
    """A single review entry."""

    date: Optional[datetime.date] = None
    reviewer: str = Field(default="self")
    type: ReviewType = ReviewType.SELF_REVIEW
    summary: str = Field(default="", description="Key feedback summary")
    reflection_status: ReviewReflectionStatus = ReviewReflectionStatus.PENDING


# --- Change Log ---


class ChangeEntry(BaseModel):
    """A single change log entry."""

    date: Optional[datetime.date] = None
    domain: str = Field(default="", description="discover/read/write/review")
    action: str = Field(default="")
    decision: str = Field(default="")


# --- Workspace ---


class WorkspaceState(BaseModel):
    """Full state of a c2 project workspace (mirrors c2_workspace.md)."""

    project_name: str
    project_type: ProjectType = ProjectType.ACADEMIC_PAPER
    goal: str = Field(default="")
    created_at: Optional[datetime.date] = None
    last_session: str = Field(default="", description="date - brief action")

    # Discover
    sources: list[Source] = Field(default_factory=list)

    # Read
    reading_notes: list[ReadingNote] = Field(default_factory=list)

    # Write
    sections: list[SectionState] = Field(default_factory=list)
    claim_evidence: list[ClaimEvidence] = Field(default_factory=list)

    # Review
    reviews: list[ReviewRecord] = Field(default_factory=list)

    # Meta
    open_questions: list[str] = Field(default_factory=list)
    changelog: list[ChangeEntry] = Field(default_factory=list)


# --- Persona Learning ---


class EditPattern(BaseModel):
    """A pattern extracted from user edits."""

    category: str = Field(
        ..., description="tone / structure / wording / deletion / addition"
    )
    description: str = Field(..., description="What the user changed and why")
    frequency: int = Field(default=1)
    examples: list[str] = Field(default_factory=list)


class ProfileDiff(BaseModel):
    """Proposed changes to the user profile."""

    tone_updates: list[str] = Field(default_factory=list)
    structure_updates: list[str] = Field(default_factory=list)
    new_patterns: list[EditPattern] = Field(default_factory=list)
    summary: str = Field(default="")


# --- Artifact ---


class Artifact(BaseModel):
    """A versioned output artifact."""

    name: str
    version: str = Field(default="v1")
    path: str = Field(default="")
    type: str = Field(default="", description="pdf / md / tex / docx")
    created_at: Optional[datetime.date] = None
    notes: str = Field(default="")
