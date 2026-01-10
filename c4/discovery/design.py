"""Design specification models and storage for architecture decisions."""

import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

import yaml
from pydantic import BaseModel, Field

from c4.discovery.models import Domain


def _utc_now() -> datetime:
    """Get current UTC datetime with timezone info."""
    return datetime.now(timezone.utc)


class ArchitectureOption(BaseModel):
    """A single architecture option for user to choose from."""

    id: str  # e.g., "option-a", "option-b"
    name: str  # e.g., "Session-based Auth", "JWT Token Auth"
    description: str
    complexity: str = "medium"  # low, medium, high
    pros: list[str] = Field(default_factory=list)
    cons: list[str] = Field(default_factory=list)
    recommended: bool = False


class ComponentDesign(BaseModel):
    """Design for a single component/module."""

    name: str
    type: str  # e.g., "frontend", "backend", "service", "database"
    description: str
    responsibilities: list[str] = Field(default_factory=list)
    dependencies: list[str] = Field(default_factory=list)
    interfaces: list[str] = Field(default_factory=list)


class DataFlowStep(BaseModel):
    """A step in a data flow diagram."""

    from_component: str
    to_component: str
    action: str  # e.g., "POST /api/login", "query users table"
    data: Optional[str] = None  # e.g., "JWT token", "user credentials"


class DesignDecision(BaseModel):
    """A recorded design decision."""

    id: str
    question: str  # What was the question?
    decision: str  # What was decided?
    rationale: str  # Why was it decided?
    alternatives_considered: list[str] = Field(default_factory=list)
    timestamp: datetime = Field(default_factory=_utc_now)


class DesignSpec(BaseModel):
    """Complete design specification for a feature."""

    feature: str
    version: str = "1.0"
    domain: Domain
    description: Optional[str] = None
    created_at: datetime = Field(default_factory=_utc_now)
    updated_at: datetime = Field(default_factory=_utc_now)

    # Architecture options presented to user
    architecture_options: list[ArchitectureOption] = Field(default_factory=list)
    selected_option: Optional[str] = None  # ID of selected option

    # Component design
    components: list[ComponentDesign] = Field(default_factory=list)

    # Data flow
    data_flows: list[DataFlowStep] = Field(default_factory=list)

    # Design decisions
    decisions: list[DesignDecision] = Field(default_factory=list)

    # Mermaid diagram (stored as string)
    mermaid_diagram: Optional[str] = None

    # Technical constraints
    constraints: list[str] = Field(default_factory=list)

    # Non-functional requirements
    nfr: dict[str, str] = Field(default_factory=dict)  # e.g., {"latency": "<500ms"}

    def add_option(self, option: ArchitectureOption) -> None:
        """Add an architecture option."""
        self.architecture_options.append(option)
        self.updated_at = _utc_now()

    def select_option(self, option_id: str) -> bool:
        """Select an architecture option by ID."""
        if any(opt.id == option_id for opt in self.architecture_options):
            self.selected_option = option_id
            self.updated_at = _utc_now()
            return True
        return False

    def add_decision(
        self,
        id: str,
        question: str,
        decision: str,
        rationale: str,
        alternatives: Optional[list[str]] = None,
    ) -> DesignDecision:
        """Record a design decision."""
        dec = DesignDecision(
            id=id,
            question=question,
            decision=decision,
            rationale=rationale,
            alternatives_considered=alternatives or [],
        )
        self.decisions.append(dec)
        self.updated_at = _utc_now()
        return dec

    def add_component(self, component: ComponentDesign) -> None:
        """Add a component design."""
        self.components.append(component)
        self.updated_at = _utc_now()

    def to_yaml(self) -> str:
        """Export to YAML format."""
        data = self.model_dump(mode="json", exclude_none=True)
        # Convert datetime to ISO format
        data["created_at"] = self.created_at.isoformat()
        data["updated_at"] = self.updated_at.isoformat()
        # Decision timestamps are already ISO strings from model_dump(mode="json")
        return yaml.dump(data, allow_unicode=True, sort_keys=False, default_flow_style=False)

    def to_markdown(self) -> str:
        """Export to Markdown format for human reading."""
        lines = [
            f"# Design: {self.feature}",
            "",
            f"**Domain**: {self.domain.value}",
            f"**Version**: {self.version}",
            f"**Updated**: {self.updated_at.isoformat()}",
            "",
        ]

        if self.description:
            lines.extend([self.description, ""])

        # Architecture Options
        if self.architecture_options:
            lines.extend(["## Architecture Options", ""])
            for opt in self.architecture_options:
                selected = " (Selected)" if opt.id == self.selected_option else ""
                recommended = " **(Recommended)**" if opt.recommended else ""
                lines.append(f"### {opt.name}{recommended}{selected}")
                lines.extend([
                    "",
                    f"**Complexity**: {opt.complexity}",
                    "",
                    opt.description,
                    "",
                ])
                if opt.pros:
                    lines.append("**Pros:**")
                    for pro in opt.pros:
                        lines.append(f"- {pro}")
                    lines.append("")
                if opt.cons:
                    lines.append("**Cons:**")
                    for con in opt.cons:
                        lines.append(f"- {con}")
                    lines.append("")

        # Components
        if self.components:
            lines.extend(["## Components", ""])
            for comp in self.components:
                lines.extend([
                    f"### {comp.name} ({comp.type})",
                    "",
                    comp.description,
                    "",
                ])
                if comp.responsibilities:
                    lines.append("**Responsibilities:**")
                    for resp in comp.responsibilities:
                        lines.append(f"- {resp}")
                    lines.append("")
                if comp.dependencies:
                    lines.append(f"**Dependencies:** {', '.join(comp.dependencies)}")
                    lines.append("")

        # Mermaid Diagram
        if self.mermaid_diagram:
            lines.extend([
                "## Architecture Diagram",
                "",
                "```mermaid",
                self.mermaid_diagram,
                "```",
                "",
            ])

        # Design Decisions
        if self.decisions:
            lines.extend(["## Design Decisions", ""])
            for dec in self.decisions:
                lines.extend([
                    f"### {dec.id}: {dec.question}",
                    "",
                    f"**Decision:** {dec.decision}",
                    "",
                    f"**Rationale:** {dec.rationale}",
                    "",
                ])
                if dec.alternatives_considered:
                    lines.append("**Alternatives Considered:**")
                    for alt in dec.alternatives_considered:
                        lines.append(f"- {alt}")
                    lines.append("")

        # Constraints
        if self.constraints:
            lines.extend(["## Technical Constraints", ""])
            for c in self.constraints:
                lines.append(f"- {c}")
            lines.append("")

        # NFRs
        if self.nfr:
            lines.extend(["## Non-Functional Requirements", ""])
            for key, val in self.nfr.items():
                lines.append(f"- **{key}**: {val}")
            lines.append("")

        return "\n".join(lines)

    @classmethod
    def from_yaml(cls, yaml_str: str) -> "DesignSpec":
        """Load from YAML string."""
        data = yaml.safe_load(yaml_str)
        return cls(**data)


class DesignStore:
    """Storage for design specifications in .c4/specs/{feature}/ directory."""

    def __init__(self, specs_dir: Path):
        self.specs_dir = specs_dir

    def get_feature_dir(self, feature_name: str) -> Path:
        """Get directory for a specific feature.

        Raises:
            ValueError: If feature name contains path traversal attempts.
        """
        # Validate: reject path traversal attempts
        if ".." in feature_name or feature_name.startswith("/"):
            raise ValueError(f"Invalid feature name: {feature_name}")

        # Normalize feature name for directory
        import re

        safe_name = re.sub(r"[^\w\-]", "-", feature_name.lower())
        return self.specs_dir / safe_name

    def save(self, spec: DesignSpec) -> tuple[Path, Path]:
        """Save design specification to both YAML and Markdown files atomically.

        Returns:
            Tuple of (yaml_path, markdown_path)
        """
        feature_dir = self.get_feature_dir(spec.feature)
        feature_dir.mkdir(parents=True, exist_ok=True)

        yaml_file = feature_dir / "design.yaml"
        md_file = feature_dir / "design.md"

        # Atomic write for YAML
        with tempfile.NamedTemporaryFile(
            mode="w",
            suffix=".yaml",
            dir=feature_dir,
            delete=False,
            encoding="utf-8",
        ) as tmp:
            tmp.write(spec.to_yaml())
            tmp_yaml = Path(tmp.name)
        tmp_yaml.replace(yaml_file)

        # Atomic write for Markdown
        with tempfile.NamedTemporaryFile(
            mode="w",
            suffix=".md",
            dir=feature_dir,
            delete=False,
            encoding="utf-8",
        ) as tmp:
            tmp.write(spec.to_markdown())
            tmp_md = Path(tmp.name)
        tmp_md.replace(md_file)

        return yaml_file, md_file

    def load(self, feature_name: str) -> Optional[DesignSpec]:
        """Load design specification from YAML file."""
        feature_dir = self.get_feature_dir(feature_name)
        yaml_file = feature_dir / "design.yaml"

        if not yaml_file.exists():
            return None

        yaml_str = yaml_file.read_text(encoding="utf-8")
        return DesignSpec.from_yaml(yaml_str)

    def exists(self, feature_name: str) -> bool:
        """Check if design exists for a feature."""
        feature_dir = self.get_feature_dir(feature_name)
        return (feature_dir / "design.yaml").exists()

    def list_features_with_design(self) -> list[str]:
        """List all features that have design specs."""
        if not self.specs_dir.exists():
            return []

        features = []
        for item in self.specs_dir.iterdir():
            if item.is_dir() and (item / "design.yaml").exists():
                features.append(item.name)
        return features
