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
    Override,
    OverrideAction,
    RuleDefinition,
    Rules,
    Selection,
    # Skill models
    Skill,
    SkillDefinition,
    SkillTriggers,
    WorkflowSelect,
    WorkflowStep,
)

SCHEMA_DIR = Path(__file__).parent / "schema"
EXAMPLES_DIR = Path(__file__).parent / "examples"

__all__ = [
    # Directories
    "SCHEMA_DIR",
    "EXAMPLES_DIR",
    # Skill models
    "Skill",
    "SkillDefinition",
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
