"""EARS-based requirements specification models and storage."""

import re
import shutil
import tempfile
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any, Optional

import yaml
from pydantic import BaseModel, Field

from c4.discovery.models import Domain


def _utc_now() -> datetime:
    """Get current UTC datetime with timezone info."""
    return datetime.now(timezone.utc)


class EARSPattern(str, Enum):
    """EARS (Easy Approach to Requirements Syntax) patterns."""

    UBIQUITOUS = "ubiquitous"  # The system shall...
    STATE_DRIVEN = "state-driven"  # While <condition>, the system shall...
    EVENT_DRIVEN = "event-driven"  # When <trigger>, the system shall...
    OPTIONAL = "optional"  # Where <feature>, the system shall...
    UNWANTED = "unwanted"  # If <condition>, the system shall...


class EARSRequirement(BaseModel):
    """A single requirement in EARS format."""

    id: str
    pattern: EARSPattern
    text: str  # Full EARS text
    domain: Optional[Domain] = None
    priority: int = Field(default=2, ge=1, le=5)
    testable: bool = True
    notes: Optional[str] = None

    @classmethod
    def parse(cls, id: str, text: str) -> "EARSRequirement":
        """Parse EARS text and detect pattern."""
        text_lower = text.lower().strip()

        if text_lower.startswith("while ") or text_lower.startswith("during "):
            pattern = EARSPattern.STATE_DRIVEN
        elif text_lower.startswith("when ") or text_lower.startswith("if user"):
            pattern = EARSPattern.EVENT_DRIVEN
        elif text_lower.startswith("where ") or "if available" in text_lower:
            pattern = EARSPattern.OPTIONAL
        elif text_lower.startswith("if ") or text_lower.startswith("unless"):
            pattern = EARSPattern.UNWANTED
        else:
            pattern = EARSPattern.UBIQUITOUS

        return cls(id=id, pattern=pattern, text=text)


# Domain-specific extension patterns
class UIStatePattern(BaseModel):
    """Web/App UI state pattern."""

    component: str
    when_condition: str
    display: str


class AsyncFlowPattern(BaseModel):
    """Async flow pattern for API calls."""

    trigger: str
    loading_action: str
    success_action: str
    error_action: str


class DataRequirement(BaseModel):
    """ML/DL data requirement."""

    name: str
    schema: dict[str, Any]
    distribution: Optional[str] = None
    quality: Optional[str] = None


class ModelRequirement(BaseModel):
    """ML/DL model requirement."""

    name: str
    architecture: Optional[str] = None
    metrics: dict[str, str] = Field(default_factory=dict)  # metric_name -> condition
    constraints: list[str] = Field(default_factory=list)


class ExperimentProtocol(BaseModel):
    """ML/DL experiment protocol."""

    name: str
    hypothesis: str
    variables: dict[str, str] = Field(default_factory=dict)
    procedure: str
    success_criteria: str


class PerformanceRequirement(BaseModel):
    """Performance requirement."""

    metric: str
    condition: str  # e.g., "p95 < 500ms"


class VerificationRequirement(BaseModel):
    """Verification requirement collected from conversation context.

    These are added when:
    1. User explicitly requests verification (e.g., "성능 검증 필요해")
    2. Discovery interview identifies verification needs
    3. Domain-specific requirements suggest verification

    Example:
        - type: http
          name: "API Response Time"
          reason: "User requested performance verification"
          config:
            url: "/api/data"
            max_response_time: 500
    """

    type: str  # http, browser, cli, metrics, visual, dryrun
    name: str  # Human-readable name
    reason: str  # Why this verification was added (conversation context)
    config: dict[str, Any] = Field(default_factory=dict)
    priority: int = Field(default=2, ge=1, le=3)  # 1=critical, 2=normal, 3=optional
    enabled: bool = True

    @classmethod
    def from_user_request(
        cls,
        verification_type: str,
        name: str,
        reason: str,
        **config: Any,
    ) -> "VerificationRequirement":
        """Create from user request in conversation."""
        return cls(
            type=verification_type,
            name=name,
            reason=f"User request: {reason}",
            config=config,
        )

    @classmethod
    def from_domain_default(
        cls,
        verification_type: str,
        name: str,
        domain: str,
    ) -> "VerificationRequirement":
        """Create from domain default."""
        return cls(
            type=verification_type,
            name=name,
            reason=f"Domain default for {domain}",
            priority=3,  # Lower priority for defaults
        )


class FeatureSpec(BaseModel):
    """Complete feature specification."""

    feature: str
    version: str = "1.0"
    domain: Domain
    description: Optional[str] = None
    created_at: datetime = Field(default_factory=_utc_now)
    updated_at: datetime = Field(default_factory=_utc_now)

    # EARS requirements
    requirements: list[EARSRequirement] = Field(default_factory=list)

    # Domain extensions
    ui_states: list[UIStatePattern] = Field(default_factory=list)
    async_flows: list[AsyncFlowPattern] = Field(default_factory=list)
    data_requirements: list[DataRequirement] = Field(default_factory=list)
    model_requirements: list[ModelRequirement] = Field(default_factory=list)
    experiments: list[ExperimentProtocol] = Field(default_factory=list)
    performance: list[PerformanceRequirement] = Field(default_factory=list)

    # Verification requirements (from conversation context)
    verification_requirements: list[VerificationRequirement] = Field(default_factory=list)

    def add_requirement(self, id: str, text: str) -> EARSRequirement:
        """Add a new EARS requirement."""
        req = EARSRequirement.parse(id, text)
        req.domain = self.domain
        self.requirements.append(req)
        self.updated_at = _utc_now()
        return req

    def add_verification(
        self,
        verification_type: str,
        name: str,
        reason: str,
        priority: int = 2,
        **config: Any,
    ) -> VerificationRequirement:
        """Add a verification requirement from conversation context.

        Args:
            verification_type: http, browser, cli, metrics, visual, dryrun
            name: Human-readable name
            reason: Why this verification is needed (from conversation)
            priority: 1=critical, 2=normal, 3=optional
            **config: Verification-specific configuration

        Returns:
            Created VerificationRequirement
        """
        req = VerificationRequirement(
            type=verification_type,
            name=name,
            reason=reason,
            priority=priority,
            config=config,
        )
        self.verification_requirements.append(req)
        self.updated_at = _utc_now()
        return req

    def get_verifications_for_config(self) -> list[dict[str, Any]]:
        """Get verification requirements in config.yaml format.

        Returns:
            List ready to be merged into config.yaml verifications.items
        """
        return [
            {
                "type": v.type,
                "name": v.name,
                "config": v.config,
                "enabled": v.enabled,
            }
            for v in self.verification_requirements
            if v.enabled
        ]

    def to_yaml(self) -> str:
        """Export to YAML format."""
        data = self.model_dump(mode="json", exclude_none=True)
        # Convert datetime to ISO format
        data["created_at"] = self.created_at.isoformat()
        data["updated_at"] = self.updated_at.isoformat()
        return yaml.dump(data, allow_unicode=True, sort_keys=False, default_flow_style=False)

    @classmethod
    def from_yaml(cls, yaml_str: str) -> "FeatureSpec":
        """Load from YAML string."""
        data = yaml.safe_load(yaml_str)
        return cls(**data)


class SpecStore:
    """Storage for feature specifications in .c4/specs/ directory."""

    def __init__(self, c4_dir: Path):
        self.c4_dir = c4_dir
        self.specs_dir = c4_dir / "specs"

    def ensure_dir(self) -> None:
        """Ensure specs directory exists."""
        self.specs_dir.mkdir(parents=True, exist_ok=True)

    def get_feature_dir(self, feature_name: str) -> Path:
        """Get directory for a specific feature.

        Raises:
            ValueError: If feature name contains path traversal attempts.
        """
        # Validate: reject path traversal attempts
        if ".." in feature_name or feature_name.startswith("/"):
            raise ValueError(f"Invalid feature name: {feature_name}")

        # Normalize feature name for directory
        safe_name = re.sub(r"[^\w\-]", "-", feature_name.lower())
        return self.specs_dir / safe_name

    def save(self, spec: FeatureSpec) -> Path:
        """Save feature specification to YAML file atomically.

        Uses write-to-temp-then-rename pattern to prevent partial writes.
        """
        self.ensure_dir()
        feature_dir = self.get_feature_dir(spec.feature)
        feature_dir.mkdir(parents=True, exist_ok=True)

        spec_file = feature_dir / "requirements.yaml"

        # Atomic write: write to temp file, then rename
        with tempfile.NamedTemporaryFile(
            mode="w",
            suffix=".yaml",
            dir=feature_dir,
            delete=False,
            encoding="utf-8",
        ) as tmp:
            tmp.write(spec.to_yaml())
            tmp_path = Path(tmp.name)

        # Atomic rename (on same filesystem)
        tmp_path.replace(spec_file)
        return spec_file

    def load(self, feature_name: str) -> Optional[FeatureSpec]:
        """Load feature specification from YAML file."""
        feature_dir = self.get_feature_dir(feature_name)
        spec_file = feature_dir / "requirements.yaml"

        if not spec_file.exists():
            return None

        yaml_str = spec_file.read_text(encoding="utf-8")
        return FeatureSpec.from_yaml(yaml_str)

    def list_features(self) -> list[str]:
        """List all feature names with specs."""
        if not self.specs_dir.exists():
            return []

        features = []
        for item in self.specs_dir.iterdir():
            if item.is_dir() and (item / "requirements.yaml").exists():
                features.append(item.name)
        return features

    def delete(self, feature_name: str) -> bool:
        """Delete feature specification."""
        feature_dir = self.get_feature_dir(feature_name)
        if feature_dir.exists():
            shutil.rmtree(feature_dir)
            return True
        return False


# EARS pattern templates for different domains
EARS_TEMPLATES: dict[Domain, dict[str, str]] = {
    Domain.WEB_FRONTEND: {
        "click_action": "When user clicks {element}, the system shall {action}",
        "form_submit": "When user submits {form}, the system shall {validate_and_action}",
        "loading_state": "While {data} is loading, the system shall display {loading_indicator}",
        "error_display": "If {error_condition}, the system shall display {error_message}",
        "responsive": "Where viewport width < {breakpoint}, the system shall {responsive_action}",
    },
    Domain.WEB_BACKEND: {
        "api_endpoint": (
            "When {http_method} request to {endpoint}, "
            "the system shall {response_action}"
        ),
        "auth_check": (
            "If user is not authenticated, "
            "the system shall return 401 Unauthorized"
        ),
        "validation": (
            "If request body is invalid, "
            "the system shall return 400 with validation errors"
        ),
        "rate_limit": (
            "When rate limit exceeded, "
            "the system shall return 429 Too Many Requests"
        ),
    },
    Domain.ML_DL: {
        "model_accuracy": "The model shall achieve {metric} >= {threshold} on {dataset}",
        "inference_time": "The system shall complete inference in < {latency}",
        "data_validation": "When data fails validation, the system shall log error and skip record",
        "training_checkpoint": "While training, the system shall save checkpoint every {interval}",
    },
    Domain.MOBILE_APP: {
        "offline_mode": "While network is unavailable, the system shall {offline_action}",
        "background_sync": "When app enters foreground, the system shall sync pending changes",
        "permission_denied": "If user denies {permission}, the system shall {fallback_action}",
        "push_notification": (
            "When push notification received, "
            "the system shall {notification_action}"
        ),
    },
    Domain.INFRA: {
        "scaling": "When {metric} > {threshold} for {duration}, the system shall scale {direction}",
        "health_check": "The system shall respond to health checks within {timeout}",
        "failover": "If primary instance fails, the system shall failover to {backup}",
        "backup": "The system shall create backups every {interval}",
    },
}
