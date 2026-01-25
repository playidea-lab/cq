"""Unit tests for agent_graph models.

Tests cover:
1. Valid data parsing for each model
2. ValidationError on missing required fields
3. Loading example YAML files
"""

from pathlib import Path

import pytest
import yaml
from pydantic import ValidationError

from c4.supervisor.agent_graph.models import (
    Agent,
    AgentBehaviors,
    # Agent models
    AgentDefinition,
    AgentHandsOffTo,
    AgentInstructions,
    AgentPersona,
    AgentPersonality,
    AgentReceivesFrom,
    AgentRelationships,
    AgentSkills,
    ChainExtension,
    ChainExtensionAction,
    Condition,
    Domain,
    # Domain models
    DomainDefinition,
    DomainRequiredSkills,
    DomainRule,
    Override,
    OverrideAction,
    # Rule models
    RuleDefinition,
    Rules,
    Selection,
    Skill,
    # Skill models
    SkillDefinition,
    SkillTriggers,
    WorkflowSelect,
    WorkflowStep,
)

# ============================================================================
# Fixtures
# ============================================================================


EXAMPLES_DIR = Path(__file__).parent.parent.parent / "c4/supervisor/agent_graph/examples"


@pytest.fixture
def skill_yaml_path() -> Path:
    return EXAMPLES_DIR / "skills" / "debugging.yaml"


@pytest.fixture
def agent_yaml_path() -> Path:
    return EXAMPLES_DIR / "personas" / "debugger.yaml"


@pytest.fixture
def domain_yaml_path() -> Path:
    return EXAMPLES_DIR / "domains" / "web-backend.yaml"


@pytest.fixture
def rule_yaml_path() -> Path:
    return EXAMPLES_DIR / "rules" / "routing.yaml"


# ============================================================================
# Skill Model Tests
# ============================================================================


class TestSkillModels:
    """Tests for Skill-related Pydantic models."""

    def test_skill_triggers_with_keywords(self):
        """SkillTriggers should accept keywords."""
        triggers = SkillTriggers(keywords=["bug", "error", "crash"])
        assert triggers.keywords == ["bug", "error", "crash"]
        assert triggers.task_types is None
        assert triggers.file_patterns is None

    def test_skill_triggers_with_task_types(self):
        """SkillTriggers should accept task_types."""
        triggers = SkillTriggers(task_types=["debug", "fix-bug"])
        assert triggers.task_types == ["debug", "fix-bug"]

    def test_skill_triggers_with_file_patterns(self):
        """SkillTriggers should accept file_patterns."""
        triggers = SkillTriggers(file_patterns=["*.log", "*.trace"])
        assert triggers.file_patterns == ["*.log", "*.trace"]

    def test_skill_triggers_with_all_fields(self):
        """SkillTriggers should accept all trigger types."""
        triggers = SkillTriggers(
            keywords=["bug"],
            task_types=["debug"],
            file_patterns=["*.log"],
        )
        assert triggers.keywords == ["bug"]
        assert triggers.task_types == ["debug"]
        assert triggers.file_patterns == ["*.log"]

    def test_skill_minimal(self):
        """Skill should accept minimal required fields."""
        skill = Skill(
            id="debugging",
            name="Debugging",
            description="Debug and fix issues in code",
            capabilities=["error-tracing"],
            triggers=SkillTriggers(keywords=["bug"]),
        )
        assert skill.id == "debugging"
        assert skill.name == "Debugging"
        assert skill.capabilities == ["error-tracing"]

    def test_skill_full(self):
        """Skill should accept all fields."""
        skill = Skill(
            id="debugging",
            name="Debugging & Error Analysis",
            description="Debug and analyze errors in code",
            capabilities=["error-tracing", "stack-analysis"],
            triggers=SkillTriggers(keywords=["bug", "error"]),
            tools=["debugger", "profiler"],
            complementary_skills=["profiling", "logging"],
            prerequisites=["basics"],
            leads_to=["testing"],
        )
        assert skill.tools == ["debugger", "profiler"]
        assert skill.complementary_skills == ["profiling", "logging"]
        assert skill.prerequisites == ["basics"]
        assert skill.leads_to == ["testing"]

    def test_skill_missing_required_field(self):
        """Skill should raise ValidationError for missing required fields."""
        with pytest.raises(ValidationError) as exc_info:
            Skill(
                id="debugging",
                name="Debugging",
                # missing description, capabilities, triggers
            )
        errors = exc_info.value.errors()
        assert len(errors) >= 3  # At least 3 required fields missing

    def test_skill_definition(self):
        """SkillDefinition should wrap Skill."""
        skill_def = SkillDefinition(
            skill=Skill(
                id="debugging",
                name="Debugging",
                description="Debug issues",
                capabilities=["error-tracing"],
                triggers=SkillTriggers(keywords=["bug"]),
            )
        )
        assert skill_def.skill.id == "debugging"

    def test_skill_definition_missing_skill(self):
        """SkillDefinition should raise ValidationError if skill missing."""
        with pytest.raises(ValidationError):
            SkillDefinition()

    def test_load_skill_yaml(self, skill_yaml_path: Path):
        """Should load skill from example YAML file."""
        with open(skill_yaml_path) as f:
            data = yaml.safe_load(f)
        skill_def = SkillDefinition.model_validate(data)
        assert skill_def.skill.id == "debugging"
        assert skill_def.skill.name == "Debugging & Error Analysis"
        assert "error-tracing" in skill_def.skill.capabilities


# ============================================================================
# Agent Model Tests
# ============================================================================


class TestAgentModels:
    """Tests for Agent-related Pydantic models."""

    def test_agent_personality(self):
        """AgentPersonality should accept all style enums."""
        personality = AgentPersonality(
            style="methodical",
            communication="precise",
            approach="root-cause",
        )
        assert personality.style == "methodical"
        assert personality.communication == "precise"
        assert personality.approach == "root-cause"

    def test_agent_persona_minimal(self):
        """AgentPersona should accept minimal required fields."""
        persona = AgentPersona(
            role="Engineer",
            expertise="Software development",
        )
        assert persona.role == "Engineer"
        assert persona.expertise == "Software development"
        assert persona.personality is None

    def test_agent_persona_full(self):
        """AgentPersona should accept all fields."""
        persona = AgentPersona(
            role="Senior Debug Engineer",
            expertise="10 years debugging experience",
            personality=AgentPersonality(
                style="methodical",
                communication="precise",
                approach="root-cause",
            ),
        )
        assert persona.personality.style == "methodical"

    def test_agent_skills_minimal(self):
        """AgentSkills should accept minimal required fields."""
        skills = AgentSkills(primary=["debugging"])
        assert skills.primary == ["debugging"]
        assert skills.secondary is None

    def test_agent_skills_full(self):
        """AgentSkills should accept all fields."""
        skills = AgentSkills(
            primary=["debugging", "profiling"],
            secondary=["logging", "testing"],
        )
        assert skills.secondary == ["logging", "testing"]

    def test_agent_receives_from(self):
        """AgentReceivesFrom should accept agents and context."""
        receives = AgentReceivesFrom(
            agents=["developer", "tester"],
            context="Error reports",
        )
        assert receives.agents == ["developer", "tester"]
        assert receives.context == "Error reports"

    def test_agent_hands_off_to(self):
        """AgentHandsOffTo should accept agent, when, passes, weight."""
        handoff = AgentHandsOffTo(
            agent="code-reviewer",
            when="After fix",
            passes="Changes and impact",
            weight=0.9,
        )
        assert handoff.agent == "code-reviewer"
        assert handoff.weight == 0.9

    def test_agent_hands_off_to_default_weight(self):
        """AgentHandsOffTo should default weight to 0.5."""
        handoff = AgentHandsOffTo(
            agent="code-reviewer",
            when="After fix",
            passes="Changes",
        )
        assert handoff.weight == 0.5

    def test_agent_relationships(self):
        """AgentRelationships should accept receives_from and hands_off_to."""
        relationships = AgentRelationships(
            receives_from=[AgentReceivesFrom(agents=["developer"], context="Bug report")],
            hands_off_to=[
                AgentHandsOffTo(
                    agent="code-reviewer",
                    when="After fix",
                    passes="Changes",
                )
            ],
        )
        assert len(relationships.receives_from) == 1
        assert len(relationships.hands_off_to) == 1

    def test_agent_instructions(self):
        """AgentInstructions should accept on_receive and on_handoff."""
        instructions = AgentInstructions(
            on_receive="1. Analyze\n2. Debug",
            on_handoff="## Summary\n...",
        )
        assert "Analyze" in instructions.on_receive
        assert "Summary" in instructions.on_handoff

    def test_agent_behaviors(self):
        """AgentBehaviors should accept all behavior fields."""
        behaviors = AgentBehaviors(
            on_ambiguity="Ask clarifying questions",
            on_conflict="Prioritize safety",
            diagram_preference="mermaid",
        )
        assert behaviors.on_ambiguity == "Ask clarifying questions"

    def test_agent_minimal(self):
        """Agent should accept minimal required fields."""
        agent = Agent(
            id="debugger",
            name="Debugger",
            persona=AgentPersona(role="Engineer", expertise="Debugging"),
            skills=AgentSkills(primary=["debugging"]),
            relationships=AgentRelationships(),
        )
        assert agent.id == "debugger"
        assert agent.name == "Debugger"

    def test_agent_full(self):
        """Agent should accept all fields."""
        agent = Agent(
            id="debugger",
            name="Debugger",
            persona=AgentPersona(
                role="Senior Debug Engineer",
                expertise="10 years debugging",
                personality=AgentPersonality(
                    style="methodical",
                    communication="precise",
                    approach="root-cause",
                ),
            ),
            skills=AgentSkills(
                primary=["debugging"],
                secondary=["logging"],
            ),
            relationships=AgentRelationships(
                receives_from=[AgentReceivesFrom(agents=["developer"], context="Bug report")],
                hands_off_to=[
                    AgentHandsOffTo(
                        agent="code-reviewer",
                        when="After fix",
                        passes="Changes",
                        weight=0.9,
                    )
                ],
            ),
            instructions=AgentInstructions(
                on_receive="Analyze the bug",
                on_handoff="Provide summary",
            ),
            behaviors=AgentBehaviors(
                on_ambiguity="Reproduce first",
                on_conflict="Safety first",
            ),
        )
        assert agent.instructions.on_receive == "Analyze the bug"

    def test_agent_missing_required_field(self):
        """Agent should raise ValidationError for missing required fields."""
        with pytest.raises(ValidationError) as exc_info:
            Agent(
                id="debugger",
                name="Debugger",
                # missing persona, skills, relationships
            )
        errors = exc_info.value.errors()
        assert len(errors) >= 3

    def test_agent_definition(self):
        """AgentDefinition should wrap Agent."""
        agent_def = AgentDefinition(
            agent=Agent(
                id="debugger",
                name="Debugger",
                persona=AgentPersona(role="Engineer", expertise="Debugging"),
                skills=AgentSkills(primary=["debugging"]),
                relationships=AgentRelationships(),
            )
        )
        assert agent_def.agent.id == "debugger"

    def test_load_agent_yaml(self, agent_yaml_path: Path):
        """Should load agent from example YAML file."""
        with open(agent_yaml_path) as f:
            data = yaml.safe_load(f)
        agent_def = AgentDefinition.model_validate(data)
        assert agent_def.agent.id == "debugger"
        assert agent_def.agent.name == "Debugger"
        assert agent_def.agent.persona.role == "Senior Debug Engineer"
        assert "debugging" in agent_def.agent.skills.primary


# ============================================================================
# Domain Model Tests
# ============================================================================


class TestDomainModels:
    """Tests for Domain-related Pydantic models."""

    def test_domain_required_skills_minimal(self):
        """DomainRequiredSkills should accept minimal required fields."""
        skills = DomainRequiredSkills(core=["api-design"])
        assert skills.core == ["api-design"]
        assert skills.optional is None

    def test_domain_required_skills_full(self):
        """DomainRequiredSkills should accept all fields."""
        skills = DomainRequiredSkills(
            core=["api-design", "backend-architecture"],
            optional=["security", "caching"],
        )
        assert skills.optional == ["security", "caching"]

    def test_workflow_select_by_skill(self):
        """WorkflowSelect should accept skill-based selection."""
        select = WorkflowSelect(
            by="skill",
            skills=["api-design"],
            prefer_agent="backend-architect",
        )
        assert select.by == "skill"
        assert select.skills == ["api-design"]
        assert select.prefer_agent == "backend-architect"

    def test_workflow_select_by_agent(self):
        """WorkflowSelect should accept agent-based selection."""
        select = WorkflowSelect(
            by="agent",
            prefer_agent="backend-architect",
        )
        assert select.by == "agent"
        assert select.prefer_agent == "backend-architect"

    def test_workflow_step_minimal(self):
        """WorkflowStep should accept minimal required fields."""
        step = WorkflowStep(
            step=1,
            role="primary",
            select=WorkflowSelect(by="skill", skills=["api-design"]),
            purpose="Design API",
        )
        assert step.step == 1
        assert step.role == "primary"
        assert step.condition is None

    def test_workflow_step_with_condition(self):
        """WorkflowStep should accept condition."""
        step = WorkflowStep(
            step=1,
            role="quality",
            select=WorkflowSelect(by="skill", skills=["testing"]),
            purpose="Test code",
            condition="when changes > 100 lines",
        )
        assert step.condition == "when changes > 100 lines"

    def test_domain_rule(self):
        """DomainRule should accept if and then."""
        rule = DomainRule(
            if_="task_has_keyword(['auth'])",
            then="add_to_chain(security-auditor)",
        )
        assert rule.if_ == "task_has_keyword(['auth'])"
        assert rule.then == "add_to_chain(security-auditor)"

    def test_domain_minimal(self):
        """Domain should accept minimal required fields."""
        domain = Domain(
            id="web-backend",
            name="Web Backend",
            description="Backend development domain",
            required_skills=DomainRequiredSkills(core=["api-design"]),
            workflow=[
                WorkflowStep(
                    step=1,
                    role="primary",
                    select=WorkflowSelect(by="skill", skills=["api-design"]),
                    purpose="Design API",
                )
            ],
        )
        assert domain.id == "web-backend"
        assert len(domain.workflow) == 1

    def test_domain_with_rules(self):
        """Domain should accept rules."""
        domain = Domain(
            id="web-backend",
            name="Web Backend",
            description="Backend development domain",
            required_skills=DomainRequiredSkills(core=["api-design"]),
            workflow=[
                WorkflowStep(
                    step=1,
                    role="primary",
                    select=WorkflowSelect(by="skill", skills=["api-design"]),
                    purpose="Design API",
                )
            ],
            rules=[
                DomainRule(
                    if_="task_has_keyword(['auth'])",
                    then="add_to_chain(security-auditor)",
                )
            ],
        )
        assert len(domain.rules) == 1

    def test_domain_missing_required_field(self):
        """Domain should raise ValidationError for missing required fields."""
        with pytest.raises(ValidationError):
            Domain(
                id="web-backend",
                name="Web Backend",
                # missing description, required_skills, workflow
            )

    def test_domain_definition(self):
        """DomainDefinition should wrap Domain."""
        domain_def = DomainDefinition(
            domain=Domain(
                id="web-backend",
                name="Web Backend",
                description="Backend development",
                required_skills=DomainRequiredSkills(core=["api-design"]),
                workflow=[
                    WorkflowStep(
                        step=1,
                        role="primary",
                        select=WorkflowSelect(by="skill", skills=["api-design"]),
                        purpose="Design API",
                    )
                ],
            )
        )
        assert domain_def.domain.id == "web-backend"

    def test_load_domain_yaml(self, domain_yaml_path: Path):
        """Should load domain from example YAML file."""
        with open(domain_yaml_path) as f:
            data = yaml.safe_load(f)
        domain_def = DomainDefinition.model_validate(data)
        assert domain_def.domain.id == "web-backend"
        assert domain_def.domain.name == "Web Backend Development"
        assert "api-design" in domain_def.domain.required_skills.core


# ============================================================================
# Rule Model Tests
# ============================================================================


class TestRuleModels:
    """Tests for Rule-related Pydantic models."""

    def test_condition_task_type_string(self):
        """Condition should accept task_type as string."""
        condition = Condition(task_type="debug")
        assert condition.task_type == "debug"

    def test_condition_task_type_list(self):
        """Condition should accept task_type as list."""
        condition = Condition(task_type=["debug", "fix-bug"])
        assert condition.task_type == ["debug", "fix-bug"]

    def test_condition_domain(self):
        """Condition should accept domain."""
        condition = Condition(domain="web-backend")
        assert condition.domain == "web-backend"

    def test_condition_has_keyword(self):
        """Condition should accept has_keyword."""
        condition = Condition(has_keyword=["bug", "error"])
        assert condition.has_keyword == ["bug", "error"]

    def test_condition_file_pattern(self):
        """Condition should accept file_pattern."""
        condition = Condition(file_pattern=["*.log", "*.trace"])
        assert condition.file_pattern == ["*.log", "*.trace"]

    def test_condition_any(self):
        """Condition should accept any (OR)."""
        condition = Condition(
            any=[
                Condition(task_type="debug"),
                Condition(has_keyword=["bug"]),
            ]
        )
        assert len(condition.any) == 2

    def test_condition_all(self):
        """Condition should accept all (AND)."""
        condition = Condition(
            all=[
                Condition(domain="api"),
                Condition(task_type="implement"),
            ]
        )
        assert len(condition.all) == 2

    def test_condition_not(self):
        """Condition should accept not (negation)."""
        condition = Condition(not_=Condition(task_type="test"))
        assert condition.not_.task_type == "test"

    def test_override_action_set_primary(self):
        """OverrideAction should accept set_primary."""
        action = OverrideAction(set_primary="debugger")
        assert action.set_primary == "debugger"

    def test_override_action_add_to_chain(self):
        """OverrideAction should accept add_to_chain with position."""
        action = OverrideAction(
            add_to_chain="security-auditor",
            position="before_last",
        )
        assert action.add_to_chain == "security-auditor"
        assert action.position == "before_last"

    def test_override_action_require_agent(self):
        """OverrideAction should accept require_agent."""
        action = OverrideAction(require_agent="security-auditor")
        assert action.require_agent == "security-auditor"

    def test_override(self):
        """Override should accept all required fields."""
        override = Override(
            name="debug-always-debugger",
            priority=100,
            condition=Condition(task_type=["debug", "fix-bug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debugging tasks always start with debugger",
        )
        assert override.name == "debug-always-debugger"
        assert override.priority == 100
        assert override.action.set_primary == "debugger"

    def test_chain_extension_action_add_to_chain(self):
        """ChainExtensionAction should accept add_to_chain."""
        action = ChainExtensionAction(
            add_to_chain="api-documenter",
            position="after_primary",
        )
        assert action.add_to_chain == "api-documenter"
        assert action.position == "after_primary"

    def test_chain_extension_action_ensure_in_chain(self):
        """ChainExtensionAction should accept ensure_in_chain."""
        action = ChainExtensionAction(ensure_in_chain=["ml-engineer", "test-automator"])
        assert action.ensure_in_chain == ["ml-engineer", "test-automator"]

    def test_chain_extension(self):
        """ChainExtension should accept all required fields."""
        extension = ChainExtension(
            name="api-needs-docs",
            condition=Condition(domain="api"),
            action=ChainExtensionAction(
                add_to_chain="api-documenter",
                position="after_primary",
            ),
        )
        assert extension.name == "api-needs-docs"

    def test_selection(self):
        """Selection should accept all required fields."""
        selection = Selection(
            name="prefer-specialist",
            description="Prefer more specialized agent",
            strategy="1. skill_match_count\n2. primary_skill_match",
            when="multiple_agents_match",
        )
        assert selection.name == "prefer-specialist"
        assert selection.when == "multiple_agents_match"

    def test_rules(self):
        """Rules should accept overrides, chain_extensions, selection."""
        rules = Rules(
            overrides=[
                Override(
                    name="debug-rule",
                    priority=100,
                    condition=Condition(task_type="debug"),
                    action=OverrideAction(set_primary="debugger"),
                    reason="Debugging requires debugger",
                )
            ],
            chain_extensions=[
                ChainExtension(
                    name="api-docs",
                    condition=Condition(domain="api"),
                    action=ChainExtensionAction(add_to_chain="api-documenter"),
                )
            ],
            selection=[
                Selection(
                    name="prefer-specialist",
                    description="Prefer specialist",
                    strategy="skill_match_count",
                )
            ],
        )
        assert len(rules.overrides) == 1
        assert len(rules.chain_extensions) == 1
        assert len(rules.selection) == 1

    def test_rule_definition(self):
        """RuleDefinition should wrap Rules."""
        rule_def = RuleDefinition(
            rules=Rules(
                overrides=[
                    Override(
                        name="debug-rule",
                        priority=100,
                        condition=Condition(task_type="debug"),
                        action=OverrideAction(set_primary="debugger"),
                        reason="Debugging requires debugger",
                    )
                ]
            )
        )
        assert len(rule_def.rules.overrides) == 1

    def test_load_rule_yaml(self, rule_yaml_path: Path):
        """Should load rules from example YAML file."""
        with open(rule_yaml_path) as f:
            data = yaml.safe_load(f)
        rule_def = RuleDefinition.model_validate(data)
        assert len(rule_def.rules.overrides) >= 1
        # Check that expected rules exist (order-independent)
        override_names = [o.name for o in rule_def.rules.overrides]
        assert "debug-always-debugger" in override_names
        assert "review-always-code-reviewer" in override_names


# ============================================================================
# Cross-Model Integration Tests
# ============================================================================


class TestModelIntegration:
    """Tests for model integration and loading all example files."""

    def test_load_all_skill_examples(self):
        """Should load all skill example files."""
        skills_dir = EXAMPLES_DIR / "skills"
        for yaml_file in skills_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            skill_def = SkillDefinition.model_validate(data)
            assert skill_def.skill.id is not None
            assert len(skill_def.skill.capabilities) > 0

    def test_load_all_agent_examples(self):
        """Should load all agent example files."""
        personas_dir = EXAMPLES_DIR / "personas"
        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            agent_def = AgentDefinition.model_validate(data)
            assert agent_def.agent.id is not None
            assert len(agent_def.agent.skills.primary) > 0

    def test_load_all_domain_examples(self):
        """Should load all domain example files."""
        domains_dir = EXAMPLES_DIR / "domains"
        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            domain_def = DomainDefinition.model_validate(data)
            assert domain_def.domain.id is not None
            assert len(domain_def.domain.workflow) > 0

    def test_load_all_rule_examples(self):
        """Should load all rule example files."""
        rules_dir = EXAMPLES_DIR / "rules"
        for yaml_file in rules_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            rule_def = RuleDefinition.model_validate(data)
            # At least one of overrides, chain_extensions, or selection
            assert (
                rule_def.rules.overrides is not None
                or rule_def.rules.chain_extensions is not None
                or rule_def.rules.selection is not None
            )
