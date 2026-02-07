"""Pydantic models for agent_graph system.

Models correspond 1:1 with JSON schemas in the schema/ directory:
- skill.schema.yaml -> SkillDefinition
- agent.schema.yaml -> AgentDefinition
- domain.schema.yaml -> DomainDefinition
- rule.schema.yaml -> RuleDefinition
"""

from __future__ import annotations

from enum import Enum
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

# ============================================================================
# Skill Models V2 (skill.schema.yaml) - Extended with impact, domains, rules
# ============================================================================


class ImpactLevel(str, Enum):
    """Priority level for skills and rules."""

    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


class SkillCategory(str, Enum):
    """Category of skills for grouping."""

    PERFORMANCE = "performance"
    SECURITY = "security"
    QUALITY = "quality"
    ARCHITECTURE = "architecture"
    DATA_PROCESSING = "data-processing"
    OPERATIONS = "operations"


class SkillTriggers(BaseModel):
    """Conditions that trigger a skill.

    At least one of keywords, task_types, or file_patterns must be provided.
    """

    keywords: list[str] | None = None
    task_types: list[str] | None = None
    file_patterns: list[str] | None = None


class SkillMetadata(BaseModel):
    """Metadata for versioning and discovery."""

    version: str = Field(default="1.0.0", pattern=r"^\d+\.\d+\.\d+$")
    author: str | None = None
    license: Literal["MIT", "Apache-2.0", "proprietary", "unlicensed"] = "unlicensed"
    tags: list[str] = Field(default_factory=list)
    deprecated: bool = False
    deprecated_by: str | None = Field(default=None, pattern=r"^[a-z][a-z0-9-]*$")


class SkillRule(BaseModel):
    """Embedded rule for a skill (impact guideline)."""

    id: str = Field(..., pattern=r"^[A-Z0-9]+-\d{3}$")  # e.g., PERF-001, SEC-002, A11Y-001
    description: str = Field(..., min_length=10, max_length=500)
    impact: ImpactLevel = ImpactLevel.MEDIUM
    example_bad: str | None = None
    example_good: str | None = None


class SkillDependencies(BaseModel):
    """Skill dependencies."""

    required: list[str] = Field(default_factory=list)
    optional: list[str] = Field(default_factory=list)


class DomainSpecificConfig(BaseModel):
    """Domain-specific configuration for a skill."""

    triggers: SkillTriggers | None = None
    priority_boost: float = Field(default=0.0, ge=0.0, le=2.0)


class Skill(BaseModel):
    """Defines an atomic capability that agents can possess.

    V2 extension adds: impact, category, domains, domain_specific,
    metadata, rules, dependencies.
    """

    id: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    name: str = Field(..., min_length=1, max_length=100)
    description: str = Field(..., min_length=10, max_length=500)
    capabilities: list[str] = Field(..., min_length=1)
    triggers: SkillTriggers

    # V2 New Fields: Impact & Category
    impact: ImpactLevel = ImpactLevel.MEDIUM
    category: SkillCategory | None = None

    # V2 New Fields: Multi-domain Support
    domains: list[str] = Field(default_factory=lambda: ["universal"])
    domain_specific: dict[str, DomainSpecificConfig] | None = None

    # V2 New Fields: Metadata
    metadata: SkillMetadata | None = None

    # V2 New Fields: Embedded Rules
    rules: list[SkillRule] | None = None

    # V2 New Fields: Dependencies
    dependencies: SkillDependencies | None = None

    # Original Optional fields
    tools: list[str] | None = None
    complementary_skills: list[str] | None = None
    prerequisites: list[str] | None = None
    leads_to: list[str] | None = None


class SkillDefinition(BaseModel):
    """Top-level wrapper for skill YAML files."""

    skill: Skill


# ============================================================================
# Agent Models (agent.schema.yaml)
# ============================================================================


class AgentPersonality(BaseModel):
    """Personality and working style traits."""

    style: Literal["methodical", "creative", "strategic", "pragmatic", "analytical"] | None = None
    communication: (
        Literal["precise", "verbose", "concise", "visual", "conversational", "constructive"] | None
    ) = None
    approach: (
        Literal["root-cause", "trade-off", "iterative", "systematic", "exploratory"] | None
    ) = None


class AgentPersona(BaseModel):
    """Personality and behavior characteristics."""

    role: str
    expertise: str
    personality: AgentPersonality | None = None


class AgentSkills(BaseModel):
    """Skills this agent possesses."""

    primary: list[str] = Field(default_factory=list)
    secondary: list[str] | None = None


class AgentReceivesFrom(BaseModel):
    """Describes who this agent receives work from."""

    agents: list[str]
    context: str


class AgentHandsOffTo(BaseModel):
    """Describes who this agent hands off work to."""

    agent: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    when: str
    passes: str
    weight: float = Field(default=0.5, ge=0, le=1)


class AgentRelationships(BaseModel):
    """Relationships with other agents."""

    receives_from: list[AgentReceivesFrom] | None = None
    hands_off_to: list[AgentHandsOffTo] | None = None


class WorkflowStepDef(BaseModel):
    """A structured workflow step definition in persona YAML."""

    id: str
    description: str
    default_order: int = 0


class AgentInstructions(BaseModel):
    """Behavioral instructions for the agent."""

    on_receive: str | None = None
    on_handoff: str | None = None
    workflow_steps: list[WorkflowStepDef] | None = None
    dod_template: str | None = None


class AgentBehaviors(BaseModel):
    """Special behaviors for handling edge cases."""

    on_ambiguity: str | None = None
    on_conflict: str | None = None
    diagram_preference: str | None = None


class Agent(BaseModel):
    """Defines an agent persona with skills and relationships."""

    id: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    name: str = Field(..., min_length=1, max_length=100)
    persona: AgentPersona
    skills: AgentSkills
    relationships: AgentRelationships

    # Optional fields
    instructions: AgentInstructions | None = None
    behaviors: AgentBehaviors | None = None


class AgentDefinition(BaseModel):
    """Top-level wrapper for agent YAML files."""

    agent: Agent


# ============================================================================
# Domain Models (domain.schema.yaml)
# ============================================================================


class DomainRequiredSkills(BaseModel):
    """Skills needed for this domain."""

    core: list[str] = Field(default_factory=list)
    optional: list[str] | None = None


class WorkflowSelect(BaseModel):
    """How to select the agent for a workflow step."""

    by: Literal["skill", "agent"]
    skills: list[str] | None = None
    prefer_agent: str | None = None


class WorkflowStep(BaseModel):
    """A single step in the domain workflow."""

    step: int = Field(..., ge=1)
    role: Literal["primary", "support", "quality", "security", "infrastructure", "documentation"]
    select: WorkflowSelect
    purpose: str
    condition: str | None = None


class DomainRule(BaseModel):
    """Domain-specific routing rule."""

    model_config = ConfigDict(populate_by_name=True)

    if_: str = Field(..., alias="if")
    then: str


class Domain(BaseModel):
    """Defines a problem domain with required skills and workflows."""

    id: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    name: str = Field(..., min_length=1, max_length=100)
    description: str = Field(..., min_length=10, max_length=500)
    required_skills: DomainRequiredSkills
    workflow: list[WorkflowStep] = Field(..., min_length=1)

    # Optional fields
    rules: list[DomainRule] | None = None


class DomainDefinition(BaseModel):
    """Top-level wrapper for domain YAML files."""

    domain: Domain


# ============================================================================
# Rule Models (rule.schema.yaml)
# ============================================================================


class Condition(BaseModel):
    """Condition for rule evaluation.

    Supports:
    - task_type: Match task type(s)
    - domain: Match domain(s)
    - has_keyword: Match if any keyword present
    - file_pattern: Match file patterns (glob)
    - any: OR - match if any sub-condition matches
    - all: AND - match if all sub-conditions match
    - not_: NOT - negate sub-condition
    """

    model_config = ConfigDict(populate_by_name=True)

    task_type: str | list[str] | None = None
    domain: str | list[str] | None = None
    has_keyword: list[str] | None = None
    file_pattern: list[str] | None = None

    # Logical operators (recursive)
    any: list[Condition] | None = None
    all: list[Condition] | None = None
    not_: Condition | None = Field(default=None, alias="not")


class OverrideAction(BaseModel):
    """Action to take when an override condition matches."""

    set_primary: str | None = Field(default=None, pattern=r"^[a-z][a-z0-9-]*$")
    add_to_chain: str | None = Field(default=None, pattern=r"^[a-z][a-z0-9-]*$")
    position: Literal["first", "after_primary", "before_last", "last"] | None = None
    require_agent: str | None = Field(default=None, pattern=r"^[a-z][a-z0-9-]*$")


class Override(BaseModel):
    """Override rule that takes precedence over graph-based routing."""

    name: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    priority: int = Field(..., ge=1, le=100)
    condition: Condition
    action: OverrideAction
    reason: str


class ChainExtensionAction(BaseModel):
    """Action for chain extension rules."""

    add_to_chain: str | None = Field(default=None, pattern=r"^[a-z][a-z0-9-]*$")
    position: Literal["first", "after_primary", "before_last", "last"] | None = Field(
        default="before_last"
    )
    ensure_in_chain: list[str] | None = None


class ChainExtension(BaseModel):
    """Rule for extending the agent chain."""

    name: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    condition: Condition
    action: ChainExtensionAction


class Selection(BaseModel):
    """Heuristic for agent selection when multiple agents match."""

    name: str = Field(..., pattern=r"^[a-z][a-z0-9-]*$")
    description: str
    strategy: str
    when: Literal["multiple_agents_match", "no_exact_match", "ambiguous_domain"] | None = None


class Rules(BaseModel):
    """Collection of routing rules."""

    overrides: list[Override] | None = None
    chain_extensions: list[ChainExtension] | None = None
    selection: list[Selection] | None = None


class RuleDefinition(BaseModel):
    """Top-level wrapper for rule YAML files."""

    rules: Rules
