"""C4 Discovery System - Domain detection, interviews, and specifications."""

from c4.discovery.design import (
    ArchitectureOption,
    ComponentDesign,
    DataFlowStep,
    DesignDecision,
    DesignSpec,
    DesignStore,
)
from c4.discovery.domain_detector import DomainDetector
from c4.discovery.interview import (
    InterviewContext,
    InterviewEngine,
    InterviewPhase,
    InterviewQuestion,
)
from c4.discovery.models import (
    Domain,
    DomainDetectionResult,
    FeatureInfo,
    InterviewState,
    ProjectOverview,
)
from c4.discovery.specs import (
    EARSPattern,
    EARSRequirement,
    FeatureSpec,
    SpecStore,
    VerificationRequirement,
)

__all__ = [
    # Domain Detection
    "Domain",
    "DomainDetector",
    "DomainDetectionResult",
    # Interview
    "InterviewEngine",
    "InterviewPhase",
    "InterviewQuestion",
    "InterviewContext",
    # Project Models
    "ProjectOverview",
    "FeatureInfo",
    "InterviewState",
    # Specifications
    "EARSPattern",
    "EARSRequirement",
    "FeatureSpec",
    "SpecStore",
    "VerificationRequirement",
    # Design
    "ArchitectureOption",
    "ComponentDesign",
    "DataFlowStep",
    "DesignDecision",
    "DesignSpec",
    "DesignStore",
]
