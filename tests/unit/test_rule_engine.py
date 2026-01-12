"""Unit tests for RuleEngine - Rule-based routing evaluation.

Tests cover:
1. Basic condition evaluation (task_type, domain, has_keyword, file_pattern)
2. Logical operators (all, any, not)
3. Override matching with priority
4. Chain extension matching
5. GraphRouter integration with rules
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph import (
    AgentGraph,
    GraphRouter,
    RuleContext,
    RuleEngine,
    SkillMatcher,
    TaskContext,
)
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    ChainExtension,
    ChainExtensionAction,
    Condition,
    Override,
    OverrideAction,
    RuleDefinition,
    Rules,
    Skill,
    SkillDefinition,
    SkillTriggers,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def rule_engine() -> RuleEngine:
    """Create an empty RuleEngine."""
    return RuleEngine()


@pytest.fixture
def debug_override() -> Override:
    """Create a debug task type override."""
    return Override(
        name="debug-override",
        priority=100,
        condition=Condition(task_type=["debug", "fix-bug"]),
        action=OverrideAction(set_primary="debugger"),
        reason="Debugging tasks use debugger",
    )


@pytest.fixture
def security_override() -> Override:
    """Create a security keyword override."""
    return Override(
        name="security-override",
        priority=95,
        condition=Condition(has_keyword=["auth", "password", "token"]),
        action=OverrideAction(add_to_chain="security-auditor", position="before_last"),
        reason="Security-sensitive code needs security review",
    )


@pytest.fixture
def production_extension() -> ChainExtension:
    """Create a production deployment chain extension."""
    return ChainExtension(
        name="production-extension",
        condition=Condition(has_keyword=["production", "deploy"]),
        action=ChainExtensionAction(
            add_to_chain="security-auditor",
            position="before_last",
        ),
    )


# ============================================================================
# Test Basic Condition Evaluation
# ============================================================================


class TestBasicConditions:
    """Tests for basic condition evaluation."""

    def test_task_type_match_single(self, rule_engine: RuleEngine) -> None:
        """Should match single task type."""
        condition = Condition(task_type="debug")
        context = RuleContext(task_type="debug")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_task_type_match_list(self, rule_engine: RuleEngine) -> None:
        """Should match task type in list."""
        condition = Condition(task_type=["debug", "fix-bug", "hotfix"])
        context = RuleContext(task_type="fix-bug")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_task_type_no_match(self, rule_engine: RuleEngine) -> None:
        """Should not match different task type."""
        condition = Condition(task_type="debug")
        context = RuleContext(task_type="feature")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is False

    def test_domain_match(self, rule_engine: RuleEngine) -> None:
        """Should match domain."""
        condition = Condition(domain="web-backend")
        context = RuleContext(domain="web-backend")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_domain_match_list(self, rule_engine: RuleEngine) -> None:
        """Should match domain in list."""
        condition = Condition(domain=["web-backend", "web-frontend"])
        context = RuleContext(domain="web-frontend")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_has_keyword_match(self, rule_engine: RuleEngine) -> None:
        """Should match keyword in title."""
        condition = Condition(has_keyword=["auth", "password"])
        context = RuleContext(title="Fix authentication bug")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_has_keyword_in_description(self, rule_engine: RuleEngine) -> None:
        """Should match keyword in description."""
        condition = Condition(has_keyword=["password"])
        context = RuleContext(
            title="Fix security issue",
            description="The password validation is broken",
        )

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_file_pattern_match(self, rule_engine: RuleEngine) -> None:
        """Should match file pattern."""
        condition = Condition(file_pattern=["*.py", "*.ts"])
        context = RuleContext(scope="api/endpoints.py")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_file_pattern_glob_match(self, rule_engine: RuleEngine) -> None:
        """Should match glob file pattern."""
        condition = Condition(file_pattern=["**/auth/**"])
        context = RuleContext(scope="src/auth/login.py")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True


# ============================================================================
# Test Logical Operators
# ============================================================================


class TestLogicalOperators:
    """Tests for logical operator evaluation."""

    def test_all_operator_both_match(self, rule_engine: RuleEngine) -> None:
        """ALL operator should match when all conditions match."""
        condition = Condition(
            all=[
                Condition(task_type="debug"),
                Condition(domain="web-backend"),
            ]
        )
        context = RuleContext(task_type="debug", domain="web-backend")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_all_operator_one_fails(self, rule_engine: RuleEngine) -> None:
        """ALL operator should fail when one condition fails."""
        condition = Condition(
            all=[
                Condition(task_type="debug"),
                Condition(domain="web-backend"),
            ]
        )
        context = RuleContext(task_type="debug", domain="ml-dl")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is False

    def test_any_operator_one_matches(self, rule_engine: RuleEngine) -> None:
        """ANY operator should match when one condition matches."""
        condition = Condition(
            any=[
                Condition(task_type="debug"),
                Condition(has_keyword=["error"]),
            ]
        )
        context = RuleContext(title="Fix error handling")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_any_operator_none_match(self, rule_engine: RuleEngine) -> None:
        """ANY operator should fail when no conditions match."""
        condition = Condition(
            any=[
                Condition(task_type="debug"),
                Condition(has_keyword=["error"]),
            ]
        )
        context = RuleContext(task_type="feature", title="Add new feature")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is False

    def test_not_operator(self, rule_engine: RuleEngine) -> None:
        """NOT operator should negate condition."""
        condition = Condition(
            not_=Condition(task_type="debug")
        )
        context = RuleContext(task_type="feature")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is True

    def test_not_operator_negates_match(self, rule_engine: RuleEngine) -> None:
        """NOT operator should negate matching condition."""
        condition = Condition(
            not_=Condition(task_type="debug")
        )
        context = RuleContext(task_type="debug")

        result = rule_engine._evaluate_condition(condition, context)
        assert result is False


# ============================================================================
# Test Override Matching
# ============================================================================


class TestOverrideMatching:
    """Tests for override rule matching."""

    def test_find_matching_override(
        self, rule_engine: RuleEngine, debug_override: Override
    ) -> None:
        """Should find matching override."""
        rule_engine.add_override(debug_override)
        context = RuleContext(task_type="debug")

        result = rule_engine.find_matching_override(context)

        assert result is not None
        assert result.name == "debug-override"
        assert result.action.set_primary == "debugger"

    def test_find_no_matching_override(
        self, rule_engine: RuleEngine, debug_override: Override
    ) -> None:
        """Should return None when no override matches."""
        rule_engine.add_override(debug_override)
        context = RuleContext(task_type="feature")

        result = rule_engine.find_matching_override(context)

        assert result is None

    def test_priority_order(
        self,
        rule_engine: RuleEngine,
        debug_override: Override,
        security_override: Override,
    ) -> None:
        """Higher priority override should match first."""
        rule_engine.add_override(security_override)  # priority 95
        rule_engine.add_override(debug_override)  # priority 100

        # Both could match, but debug (100) has higher priority
        context = RuleContext(task_type="debug", title="Debug auth issue")
        result = rule_engine.find_matching_override(context)

        assert result is not None
        assert result.name == "debug-override"

    def test_overrides_sorted_by_priority(
        self,
        rule_engine: RuleEngine,
        debug_override: Override,
        security_override: Override,
    ) -> None:
        """Overrides should be sorted by priority."""
        rule_engine.add_override(security_override)  # priority 95
        rule_engine.add_override(debug_override)  # priority 100

        overrides = rule_engine.overrides
        assert overrides[0].priority == 100
        assert overrides[1].priority == 95


# ============================================================================
# Test Chain Extension Matching
# ============================================================================


class TestChainExtensionMatching:
    """Tests for chain extension rule matching."""

    def test_find_matching_extension(
        self, rule_engine: RuleEngine, production_extension: ChainExtension
    ) -> None:
        """Should find matching chain extension."""
        rule_engine.add_chain_extension(production_extension)
        context = RuleContext(title="Deploy to production")

        result = rule_engine.find_matching_chain_extensions(context)

        assert len(result) == 1
        assert result[0].name == "production-extension"

    def test_find_multiple_extensions(
        self, rule_engine: RuleEngine, production_extension: ChainExtension
    ) -> None:
        """Should find all matching chain extensions."""
        another_extension = ChainExtension(
            name="deploy-extension",
            condition=Condition(has_keyword=["deploy"]),
            action=ChainExtensionAction(
                ensure_in_chain=["test-automator"],
            ),
        )
        rule_engine.add_chain_extension(production_extension)
        rule_engine.add_chain_extension(another_extension)

        context = RuleContext(title="Deploy to production")
        result = rule_engine.find_matching_chain_extensions(context)

        assert len(result) == 2


# ============================================================================
# Test GraphRouter Integration
# ============================================================================


@pytest.fixture
def graph_with_agents() -> AgentGraph:
    """Create a graph with some agents."""
    g = AgentGraph()

    # Add a skill
    python_skill = SkillDefinition(
        skill=Skill(
            id="python-coding",
            name="Python Coding",
            description="Writing Python code and modules",
            capabilities=["write python code"],
            triggers=SkillTriggers(keywords=["python"]),
        )
    )
    g.add_skill(python_skill)

    # Add agents
    backend_dev = AgentDefinition(
        agent=Agent(
            id="backend-dev",
            name="Backend Developer",
            persona=AgentPersona(role="Backend specialist", expertise="Python, APIs"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(backend_dev)

    debugger = AgentDefinition(
        agent=Agent(
            id="debugger",
            name="Debugger",
            persona=AgentPersona(role="Bug hunter", expertise="Debugging"),
            skills=AgentSkills(primary=["python-coding"]),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(debugger)

    return g


class TestGraphRouterWithRules:
    """Tests for GraphRouter with RuleEngine integration."""

    def test_rule_override_takes_priority(
        self,
        graph_with_agents: AgentGraph,
        debug_override: Override,
    ) -> None:
        """Rule override should take priority over skill matching."""
        rule_engine = RuleEngine()
        rule_engine.add_override(debug_override)

        skill_matcher = SkillMatcher(graph_with_agents)
        router = GraphRouter(
            skill_matcher=skill_matcher,
            graph=graph_with_agents,
            rule_engine=rule_engine,
        )

        # This would match python-coding skill, but rule override wins
        task = TaskContext(
            title="Debug Python bug",
            task_type="debug",
        )
        config = router.get_recommended_agent("web-backend", task=task)

        assert config.primary == "debugger"

    def test_routing_result_includes_rule_info(
        self,
        graph_with_agents: AgentGraph,
        debug_override: Override,
    ) -> None:
        """RoutingResult should include rule information."""
        rule_engine = RuleEngine()
        rule_engine.add_override(debug_override)

        router = GraphRouter(
            graph=graph_with_agents,
            rule_engine=rule_engine,
        )

        task = TaskContext(title="Fix bug", task_type="debug")
        result = router.get_recommended_agent_with_details("web-backend", task=task)

        assert result.routing_method == "rule"
        assert result.matched_rule == "debug-override"
        assert result.rule_reason == "Debugging tasks use debugger"

    def test_chain_extension_applied(
        self,
        graph_with_agents: AgentGraph,
        production_extension: ChainExtension,
    ) -> None:
        """Chain extension should be applied to routing result."""
        rule_engine = RuleEngine()
        rule_engine.add_chain_extension(production_extension)

        router = GraphRouter(
            graph=graph_with_agents,
            rule_engine=rule_engine,
        )

        task = TaskContext(title="Deploy to production")
        config = router.get_recommended_agent("web-backend", task=task)

        assert "security-auditor" in config.chain

    def test_rule_engine_property(
        self,
        graph_with_agents: AgentGraph,
    ) -> None:
        """Should expose rule_engine property."""
        rule_engine = RuleEngine()
        router = GraphRouter(
            graph=graph_with_agents,
            rule_engine=rule_engine,
        )

        assert router.rule_engine is rule_engine

    def test_router_without_rule_engine(
        self,
        graph_with_agents: AgentGraph,
    ) -> None:
        """Router should work without rule engine."""
        router = GraphRouter(graph=graph_with_agents)

        assert router.rule_engine is None

        task = TaskContext(title="Python feature")
        config = router.get_recommended_agent("web-backend", task=task)

        # Should use skill matching
        assert config is not None


# ============================================================================
# Test RuleEngine Clear and Properties
# ============================================================================


class TestRuleEngineManagement:
    """Tests for RuleEngine management methods."""

    def test_clear_rules(
        self,
        rule_engine: RuleEngine,
        debug_override: Override,
        production_extension: ChainExtension,
    ) -> None:
        """Clear should remove all rules."""
        rule_engine.add_override(debug_override)
        rule_engine.add_chain_extension(production_extension)

        assert len(rule_engine.overrides) == 1
        assert len(rule_engine.chain_extensions) == 1

        rule_engine.clear()

        assert len(rule_engine.overrides) == 0
        assert len(rule_engine.chain_extensions) == 0

    def test_add_rules_from_definition(
        self, rule_engine: RuleEngine
    ) -> None:
        """Should add rules from RuleDefinition."""
        rule_def = RuleDefinition(
            rules=Rules(
                overrides=[
                    Override(
                        name="test-override",
                        priority=50,
                        condition=Condition(task_type="test"),
                        action=OverrideAction(set_primary="test-agent"),
                        reason="Test reason",
                    )
                ],
                chain_extensions=[
                    ChainExtension(
                        name="test-extension",
                        condition=Condition(has_keyword=["test"]),
                        action=ChainExtensionAction(add_to_chain="test-agent"),
                    )
                ],
            )
        )

        rule_engine.add_rules(rule_def)

        assert len(rule_engine.overrides) == 1
        assert len(rule_engine.chain_extensions) == 1


# ============================================================================
# Test AgentGraphLoader.load_directory()
# ============================================================================


class TestLoaderLoadDirectory:
    """Tests for AgentGraphLoader.load_directory() method."""

    def test_load_directory_returns_tuple(self) -> None:
        """load_directory should return (AgentGraph, RuleEngine) tuple."""
        from c4.supervisor.agent_graph import AgentGraph, AgentGraphLoader, RuleEngine

        loader = AgentGraphLoader()  # Uses default examples/ directory
        result = loader.load_directory()

        assert isinstance(result, tuple)
        assert len(result) == 2
        assert isinstance(result[0], AgentGraph)
        assert isinstance(result[1], RuleEngine)

    def test_load_directory_populates_graph(self) -> None:
        """load_directory should populate graph with skills, agents, domains."""
        from c4.supervisor.agent_graph import AgentGraphLoader

        loader = AgentGraphLoader()
        graph, _ = loader.load_directory()

        # Graph should have nodes (skills, agents, domains from examples/)
        assert len(graph.skills) > 0
        assert len(graph.agents) > 0

    def test_load_directory_with_router(self) -> None:
        """load_directory result should work with GraphRouter."""
        from c4.supervisor.agent_graph import AgentGraphLoader, GraphRouter

        loader = AgentGraphLoader()
        graph, rule_engine = loader.load_directory()

        router = GraphRouter(graph=graph, rule_engine=rule_engine)
        assert router.graph is graph
        assert router.rule_engine is rule_engine
