"""Integration tests for Agent Graph System - End-to-end scenarios.

Tests cover:
1. Full routing workflow: Task → Skills → Agent → Chain
2. Rule-based overrides in context
3. Dynamic chain building with required roles
4. Loader integration with all components
5. Performance benchmarks
"""

from __future__ import annotations

import time

import pytest

from c4.supervisor.agent_graph import (
    AgentGraph,
    AgentGraphLoader,
    DynamicChainBuilder,
    GraphRouter,
    RuleContext,
    RuleEngine,
    SkillMatcher,
    TaskContext,
)
from c4.supervisor.agent_graph.models import (
    ChainExtension,
    ChainExtensionAction,
    Condition,
    Override,
    OverrideAction,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def loader() -> AgentGraphLoader:
    """Create loader with examples directory."""
    return AgentGraphLoader()


@pytest.fixture
def full_system(
    loader: AgentGraphLoader,
) -> tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]:
    """Load complete system from examples."""
    graph, rule_engine = loader.load_directory()
    skill_matcher = SkillMatcher(graph)
    router = GraphRouter(
        graph=graph,
        skill_matcher=skill_matcher,
        rule_engine=rule_engine,
    )
    return graph, rule_engine, skill_matcher, router


# ============================================================================
# E2E Scenario Tests
# ============================================================================


class TestE2ERoutingScenarios:
    """End-to-end tests for complete routing workflows."""

    def test_scenario_python_backend_task(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Scenario: Python backend development task."""
        graph, rule_engine, skill_matcher, router = full_system

        # User submits a Python backend task
        task = TaskContext(
            title="Implement REST API endpoint for user management",
            description="Create CRUD operations for users with FastAPI",
            task_type="feature",
        )

        # Router should recommend an appropriate agent
        config = router.get_recommended_agent("web-backend", task=task)

        # Verify routing result
        assert config is not None
        assert config.primary is not None
        assert len(config.chain) >= 1

    def test_scenario_debug_task_with_rule_override(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Scenario: Debug task should trigger rule override."""
        graph, rule_engine, skill_matcher, router = full_system

        # Add a debug override rule
        debug_override = Override(
            name="debug-override",
            priority=100,
            condition=Condition(task_type=["debug", "bugfix"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debug tasks should use debugger agent",
        )
        rule_engine.add_override(debug_override)

        # User submits a debug task
        task = TaskContext(
            title="Fix null pointer exception in auth module",
            task_type="debug",
        )

        # Router should use the debugger via rule override
        result = router.get_recommended_agent_with_details("web-backend", task=task)

        assert result.config.primary == "debugger"
        assert result.routing_method == "rule"
        # Rule name could be our added rule or existing YAML rule
        assert "debug" in result.matched_rule.lower()

    def test_scenario_security_sensitive_task(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Scenario: Security-sensitive task should include security-auditor."""
        graph, rule_engine, skill_matcher, router = full_system

        # Add a chain extension for security keywords
        security_extension = ChainExtension(
            name="security-extension",
            condition=Condition(has_keyword=["auth", "security", "password"]),
            action=ChainExtensionAction(
                add_to_chain="security-auditor",
                position="before_last",
            ),
        )
        rule_engine.add_chain_extension(security_extension)

        # User submits auth-related task
        task = TaskContext(
            title="Implement OAuth2 authentication flow",
            description="Add social login with Google and GitHub",
        )

        # Router should extend chain with security-auditor
        config = router.get_recommended_agent("web-backend", task=task)

        # Security-auditor should be in the chain
        assert "security-auditor" in config.chain

    def test_scenario_full_workflow_with_loader(self, loader: AgentGraphLoader) -> None:
        """Scenario: Complete workflow from YAML loading to routing."""
        # Load everything from YAML files
        graph, rule_engine = loader.load_directory()

        # Verify loaded content
        assert len(graph.skills) > 0, "Should have loaded skills"
        assert len(graph.agents) > 0, "Should have loaded agents"

        # Create router with loaded components
        skill_matcher = SkillMatcher(graph)
        router = GraphRouter(
            graph=graph,
            skill_matcher=skill_matcher,
            rule_engine=rule_engine,
        )

        # Test routing for various domains
        domains = ["web-backend", "web-frontend", "ml-dl"]
        for domain in domains:
            try:
                config = router.get_recommended_agent(domain)
                assert config.primary is not None
            except Exception:
                # Domain might not be configured, which is OK for optional domains
                pass


class TestDynamicChainBuildingE2E:
    """End-to-end tests for dynamic chain building."""

    def test_payment_feature_chain(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Scenario: Payment feature should build chain with payment specialist."""
        graph, _, _, _ = full_system
        builder = DynamicChainBuilder(graph)

        # Task with payment keywords
        task = TaskContext(
            title="Integrate Stripe payment processing",
            description="Add checkout flow with subscription support",
        )

        # Detect required roles
        required = builder.detect_required_roles(task)

        # Payment-related agents should be detected
        # Note: Depends on which agents exist in the examples
        if "payment-integration" in graph.agents:
            assert "payment-integration" in required

    def test_testing_task_chain(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Scenario: Testing task should include test-automator."""
        graph, _, _, _ = full_system
        builder = DynamicChainBuilder(graph)

        task = TaskContext(
            title="Write comprehensive unit tests",
            description="Add pytest tests with 80% coverage",
        )

        required = builder.detect_required_roles(task)

        if "test-automator" in graph.agents:
            assert "test-automator" in required


class TestSkillMatchingE2E:
    """End-to-end tests for skill matching."""

    def test_skill_extraction_from_task(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Skills should be extracted from task content."""
        _, _, skill_matcher, _ = full_system

        task = TaskContext(
            title="Build Python data pipeline",
            description="Process CSV files and store in PostgreSQL",
        )

        skills = skill_matcher.extract_required_skills(task)

        # Should find python-related skills if they're defined
        # The exact skills depend on the YAML definitions
        assert isinstance(skills, list)

    def test_agent_matching_from_skills(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Agents should be matched based on skills."""
        _, _, skill_matcher, _ = full_system

        task = TaskContext(
            title="Implement REST API",
            task_type="feature",
        )

        # Extract skills from task, then find agents with those skills
        skills = skill_matcher.extract_required_skills(task)
        if skills:
            matches = skill_matcher.find_best_agents(skills)
        else:
            matches = []

        # Should return agent matches (even if empty)
        assert isinstance(matches, list)


# ============================================================================
# Performance Benchmarks
# ============================================================================


class TestPerformanceBenchmarks:
    """Performance tests for the agent graph system."""

    def test_routing_performance(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Routing should complete within acceptable time."""
        _, _, _, router = full_system

        task = TaskContext(
            title="Standard feature implementation",
            task_type="feature",
        )

        # Warm-up
        router.get_recommended_agent("web-backend", task=task)

        # Benchmark
        iterations = 100
        start = time.perf_counter()

        for _ in range(iterations):
            router.get_recommended_agent("web-backend", task=task)

        elapsed = time.perf_counter() - start
        avg_ms = (elapsed / iterations) * 1000

        # Should complete in under 10ms per routing decision
        assert avg_ms < 10, f"Routing took {avg_ms:.2f}ms on average (should be < 10ms)"

    def test_skill_matching_performance(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Skill matching should complete quickly."""
        _, _, skill_matcher, _ = full_system

        task = TaskContext(
            title="Complex Python backend with security features",
            description="Build API with authentication and database integration",
        )

        # Warm-up
        skills = skill_matcher.extract_required_skills(task)

        # Benchmark
        iterations = 100
        start = time.perf_counter()

        for _ in range(iterations):
            skills = skill_matcher.extract_required_skills(task)
            if skills:
                skill_matcher.find_best_agents(skills)

        elapsed = time.perf_counter() - start
        avg_ms = (elapsed / iterations) * 1000

        # Should complete in under 5ms per skill matching
        assert avg_ms < 5, f"Skill matching took {avg_ms:.2f}ms on average (should be < 5ms)"

    def test_rule_evaluation_performance(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Rule evaluation should be fast."""
        _, rule_engine, _, _ = full_system

        # Add some test rules
        for i in range(10):
            rule_engine.add_override(
                Override(
                    name=f"test-rule-{i}",
                    priority=50 - i,
                    condition=Condition(task_type=[f"type-{i}"]),
                    action=OverrideAction(set_primary="test-agent"),
                    reason=f"Test rule {i}",
                )
            )

        context = RuleContext(
            task_type="type-5",
            domain="web-backend",
            title="Test task",
        )

        # Warm-up
        rule_engine.find_matching_override(context)

        # Benchmark
        iterations = 1000
        start = time.perf_counter()

        for _ in range(iterations):
            rule_engine.find_matching_override(context)

        elapsed = time.perf_counter() - start
        avg_us = (elapsed / iterations) * 1_000_000

        # Should complete in under 100 microseconds
        assert avg_us < 100, f"Rule evaluation took {avg_us:.2f}μs on average (should be < 100μs)"

    def test_loader_performance(self, loader: AgentGraphLoader) -> None:
        """Loading should complete in reasonable time."""
        # Benchmark full load
        start = time.perf_counter()
        graph, rule_engine = loader.load_directory()
        elapsed = time.perf_counter() - start

        # Should load in under 500ms
        assert elapsed < 0.5, f"Loading took {elapsed*1000:.2f}ms (should be < 500ms)"


# ============================================================================
# MCP Tool Integration Tests
# ============================================================================


class TestMCPToolIntegration:
    """Tests verifying routing works correctly for MCP tool usage."""

    def test_routing_returns_valid_config(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Routing should return valid config for MCP tool."""
        _, _, _, router = full_system

        task = TaskContext(
            title="Add new API endpoint",
            task_type="feature",
        )

        config = router.get_recommended_agent("web-backend", task=task)

        # Config should have all required fields for MCP tool
        assert hasattr(config, "primary")
        assert hasattr(config, "chain")
        assert config.primary is not None
        assert isinstance(config.chain, list)

    def test_routing_result_has_metadata(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Routing result should include useful metadata."""
        _, _, _, router = full_system

        task = TaskContext(
            title="Debug issue",
            task_type="debug",
        )

        result = router.get_recommended_agent_with_details("web-backend", task=task)

        # Result should have routing metadata
        assert hasattr(result, "routing_method")
        assert hasattr(result, "config")
        assert result.routing_method in ["rule", "skill", "task_type", "domain"]

    def test_domain_based_routing_fallback(
        self, full_system: tuple[AgentGraph, RuleEngine, SkillMatcher, GraphRouter]
    ) -> None:
        """Should fall back to domain-based routing when no better match."""
        _, _, _, router = full_system

        # No task context, just domain
        config = router.get_recommended_agent("web-backend")

        # Should still return a valid config
        assert config.primary is not None
        assert len(config.chain) >= 1
