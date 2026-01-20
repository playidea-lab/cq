"""Agent Graph System - Graph-based agent routing with 4-layer architecture.

Layers:
1. Skills - Atomic capabilities that agents can possess
2. Agents - Personas with skills and relationships
3. Domains - Problem areas with workflows
4. Rules - Routing overrides and chain extensions

Schema files are in the schema/ subdirectory.
Example YAML files are in the examples/ subdirectory.
"""

from pathlib import Path

from c4.supervisor.agent_graph.chain_builder import (
    ChainBuildContext,
    DynamicChainBuilder,
)
from c4.supervisor.agent_graph.external_loader import (
    ConflictResolution,
    ExternalLoaderConfig,
    ExternalSkillLoader,
    LoadedSkill,
    SkillLoadResult,
    SkillSource,
    load_all_skills,
)
from c4.supervisor.agent_graph.graph import AgentGraph, EdgeType, NodeType
from c4.supervisor.agent_graph.loader import (
    AgentGraphLoader,
    FileNotFoundError,
    LoaderError,
    ModelValidationError,
    SchemaValidationError,
    YAMLParseError,
)
from c4.supervisor.agent_graph.models import (
    # Agent models
    Agent,
    AgentBehaviors,
    AgentDefinition,
    AgentHandsOffTo,
    AgentInstructions,
    AgentPersona,
    AgentPersonality,
    AgentReceivesFrom,
    AgentRelationships,
    AgentSkills,
    # Rule models
    ChainExtension,
    ChainExtensionAction,
    Condition,
    # Domain models
    Domain,
    DomainDefinition,
    DomainRequiredSkills,
    DomainRule,
    # Skill models (V2 extended)
    DomainSpecificConfig,
    ImpactLevel,
    Override,
    OverrideAction,
    RuleDefinition,
    Rules,
    Selection,
    Skill,
    SkillCategory,
    SkillDefinition,
    SkillDependencies,
    SkillMetadata,
    SkillRule,
    SkillTriggers,
    WorkflowSelect,
    WorkflowStep,
)
from c4.supervisor.agent_graph.router import (
    GraphRouter,
    RoutingResult,
)
from c4.supervisor.agent_graph.rule_engine import (
    RuleContext,
    RuleEngine,
)
from c4.supervisor.agent_graph.skill_matcher import (
    AgentMatch,
    SkillMatch,
    SkillMatcher,
    TaskContext,
    TaskLike,
)
from c4.supervisor.agent_graph.skill_md_parser import (
    ParsedSkillMd,
    SkillMdParser,
    parse_skill_md,
)
from c4.supervisor.agent_graph.skill_validator import (
    SkillValidator,
    ValidationIssue,
    ValidationLevel,
    ValidationResult,
)

SCHEMA_DIR = Path(__file__).parent / "schema"
EXAMPLES_DIR = Path(__file__).parent / "examples"
SKILLS_DIR = Path(__file__).parent / "skills"

__all__ = [
    # Graph
    "AgentGraph",
    "NodeType",
    "EdgeType",
    # Loader
    "AgentGraphLoader",
    "LoaderError",
    "FileNotFoundError",
    "SchemaValidationError",
    "YAMLParseError",
    "ModelValidationError",
    # Router
    "GraphRouter",
    "RoutingResult",
    # Rule Engine
    "RuleEngine",
    "RuleContext",
    # Skill Matcher
    "SkillMatcher",
    "SkillMatch",
    "TaskContext",
    "TaskLike",
    "AgentMatch",
    # Chain Builder
    "DynamicChainBuilder",
    "ChainBuildContext",
    # Directories
    "SCHEMA_DIR",
    "EXAMPLES_DIR",
    "SKILLS_DIR",
    # Skill Validator
    "SkillValidator",
    "ValidationIssue",
    "ValidationLevel",
    "ValidationResult",
    # SKILL.md Parser
    "ParsedSkillMd",
    "SkillMdParser",
    "parse_skill_md",
    # External Loader
    "ConflictResolution",
    "ExternalLoaderConfig",
    "ExternalSkillLoader",
    "LoadedSkill",
    "SkillLoadResult",
    "SkillSource",
    "load_all_skills",
    # Skill models (V2 extended)
    "DomainSpecificConfig",
    "ImpactLevel",
    "Skill",
    "SkillCategory",
    "SkillDefinition",
    "SkillDependencies",
    "SkillMetadata",
    "SkillRule",
    "SkillTriggers",
    # Agent models
    "Agent",
    "AgentBehaviors",
    "AgentDefinition",
    "AgentHandsOffTo",
    "AgentInstructions",
    "AgentPersona",
    "AgentPersonality",
    "AgentReceivesFrom",
    "AgentRelationships",
    "AgentSkills",
    # Domain models
    "Domain",
    "DomainDefinition",
    "DomainRequiredSkills",
    "DomainRule",
    "WorkflowSelect",
    "WorkflowStep",
    # Rule models
    "ChainExtension",
    "ChainExtensionAction",
    "Condition",
    "Override",
    "OverrideAction",
    "RuleDefinition",
    "Rules",
    "Selection",
]
