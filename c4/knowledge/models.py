"""Knowledge data models."""

from __future__ import annotations

import re
from datetime import datetime, timezone
from enum import Enum

from pydantic import BaseModel, Field

# --- Document types for Obsidian-style Knowledge Store ---

class DocumentType(str, Enum):
    """Knowledge document types."""
    EXPERIMENT = "experiment"
    PATTERN = "pattern"
    INSIGHT = "insight"
    HYPOTHESIS = "hypothesis"


# ID prefix → document type mapping
DOC_TYPE_PREFIXES = {
    "exp": DocumentType.EXPERIMENT,
    "pat": DocumentType.PATTERN,
    "ins": DocumentType.INSIGHT,
    "hyp": DocumentType.HYPOTHESIS,
}

# Backlink regex: [[doc-id]] or [[doc-id|display text]]
BACKLINK_RE = re.compile(r"\[\[([a-z]{3}-[a-f0-9]{8})(?:\|[^\]]+)?\]\]")


class KnowledgeDocument(BaseModel):
    """Obsidian-style knowledge document with frontmatter + body.

    Markdown 파일이 SSOT. index.db/vectors.db는 파생물.
    """
    id: str = ""
    type: DocumentType = DocumentType.EXPERIMENT
    title: str = ""
    domain: str = ""
    tags: list[str] = Field(default_factory=list)

    # Experiment-specific
    task_id: str = ""
    hypothesis: str = ""
    hypothesis_status: str = ""
    parent_experiment: str | None = None
    compared_to: list[dict[str, str]] = Field(default_factory=list)
    builds_on: list[str] = Field(default_factory=list)

    # Pattern-specific
    confidence: float = 0.0
    evidence_count: int = 0
    evidence_ids: list[str] = Field(default_factory=list)

    # Insight-specific
    insight_type: str = ""
    source_count: int = 0

    # Hypothesis-specific (when type=hypothesis)
    status: str = ""
    evidence_for: list[str] = Field(default_factory=list)
    evidence_against: list[str] = Field(default_factory=list)

    # Common metadata
    created_at: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )
    updated_at: str = Field(
        default_factory=lambda: datetime.now(tz=timezone.utc).isoformat()
    )
    version: int = 1

    # Body content (Markdown, not stored in frontmatter)
    body: str = ""

    def extract_backlinks(self) -> list[str]:
        """Extract all [[backlink]] references from body."""
        return BACKLINK_RE.findall(self.body)


class KnowledgeSearchResult(BaseModel):
    """Hybrid search result combining vector + FTS scores."""
    id: str
    title: str = ""
    type: str = ""
    score: float = 0.0
    snippet: str = ""


