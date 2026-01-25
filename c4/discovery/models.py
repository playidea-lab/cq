"""Domain detection models and data structures."""

from enum import Enum
from typing import Optional

from pydantic import BaseModel, Field


class Domain(str, Enum):
    """Supported development domains."""

    WEB_FRONTEND = "web-frontend"
    WEB_BACKEND = "web-backend"
    FULLSTACK = "fullstack"
    ML_DL = "ml-dl"
    MOBILE_APP = "mobile-app"
    INFRA = "infra"
    LIBRARY = "library"
    FIRMWARE = "firmware"
    UNKNOWN = "unknown"

    @classmethod
    def from_string(cls, value: str) -> "Domain":
        """Parse domain from string, case-insensitive."""
        normalized = value.lower().replace("_", "-")
        for domain in cls:
            if domain.value == normalized:
                return domain
        return cls.UNKNOWN


class DomainSignal(BaseModel):
    """A signal that indicates a particular domain."""

    domain: Domain
    confidence: float = Field(ge=0.0, le=1.0)
    reason: str
    files_matched: list[str] = Field(default_factory=list)


class DomainDetectionResult(BaseModel):
    """Result of domain detection analysis."""

    primary_domain: Domain
    confidence: float = Field(ge=0.0, le=1.0)
    signals: list[DomainSignal] = Field(default_factory=list)
    detected_domains: list[Domain] = Field(default_factory=list)
    detected_frameworks: list[str] = Field(default_factory=list)
    is_empty_project: bool = False

    @property
    def is_mixed_domain(self) -> bool:
        """Check if multiple domains were detected with high confidence."""
        return len(self.detected_domains) > 1

    @property
    def is_piq_project(self) -> bool:
        """Check if this is a PiQ ML experiment project."""
        return "piq" in self.detected_frameworks


class ProjectOverview(BaseModel):
    """High-level project overview collected from user or inferred."""

    description: str
    domain: Domain
    additional_domains: list[Domain] = Field(default_factory=list)
    key_features: list[str] = Field(default_factory=list)
    tech_stack: list[str] = Field(default_factory=list)

    @property
    def all_domains(self) -> list[Domain]:
        """Get all domains including primary and additional."""
        domains = [self.domain]
        for d in self.additional_domains:
            if d not in domains:
                domains.append(d)
        return domains


class FeatureInfo(BaseModel):
    """Information about a feature to be developed."""

    name: str
    description: str
    domain: Domain
    priority: int = Field(default=1, ge=1, le=5)  # 1=highest
    is_core: bool = False  # User explicitly mentioned
    requires_detailed_spec: bool = True


class InterviewState(BaseModel):
    """State of the discovery interview process."""

    project_overview: Optional[ProjectOverview] = None
    confirmed_domain: bool = False
    features: list[FeatureInfo] = Field(default_factory=list)
    current_feature_index: int = 0
    completed: bool = False
