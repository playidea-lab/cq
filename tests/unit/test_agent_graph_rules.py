"""Unit tests for RuleEngine - Condition evaluation against tasks.

Tests cover:
1. Task dataclass
2. Atomic conditions (task_type, domain, has_keyword, file_pattern)
3. Logical operators (all, any, not_)
4. RuleEngine operations (add_override, find_matching_override, etc.)
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph.models import (
    ChainExtension,
    ChainExtensionAction,
    Condition,
    Override,
    OverrideAction,
)
from c4.supervisor.agent_graph.rules import (
    RuleEngine,
    Task,
    evaluate,
)

# ============================================================================
# Test Task Dataclass
# ============================================================================


class TestTask:
    """Tests for Task dataclass."""

    def test_task_basic_creation(self) -> None:
        """Task should be created with title."""
        task = Task(title="Fix bug")

        assert task.title == "Fix bug"
        assert task.description == ""
        assert task.task_type is None

    def test_task_full_creation(self) -> None:
        """Task should accept all fields."""
        task = Task(
            title="Add feature",
            description="Add login feature",
            task_type="feature",
            domain="web-backend",
            scope="src/auth/",
        )

        assert task.title == "Add feature"
        assert task.description == "Add login feature"
        assert task.task_type == "feature"
        assert task.domain == "web-backend"
        assert task.scope == "src/auth/"

    def test_task_empty_title_raises(self) -> None:
        """Task should raise ValueError for empty title."""
        with pytest.raises(ValueError, match="title cannot be empty"):
            Task(title="")


# ============================================================================
# Test Atomic Conditions
# ============================================================================


class TestEvaluateTaskType:
    """Tests for task_type condition evaluation."""

    def test_task_type_single_match(self) -> None:
        """task_type should match single value."""
        condition = Condition(task_type="feature")
        task = Task(title="Add login", task_type="feature")

        assert evaluate(condition, task) is True

    def test_task_type_single_no_match(self) -> None:
        """task_type should not match different value."""
        condition = Condition(task_type="feature")
        task = Task(title="Fix bug", task_type="bugfix")

        assert evaluate(condition, task) is False

    def test_task_type_case_insensitive(self) -> None:
        """task_type matching should be case-insensitive."""
        condition = Condition(task_type="Feature")
        task = Task(title="Add login", task_type="FEATURE")

        assert evaluate(condition, task) is True

    def test_task_type_list_match(self) -> None:
        """task_type list should match if any matches."""
        condition = Condition(task_type=["feature", "enhancement"])
        task = Task(title="Add login", task_type="enhancement")

        assert evaluate(condition, task) is True

    def test_task_type_list_no_match(self) -> None:
        """task_type list should not match if none match."""
        condition = Condition(task_type=["feature", "enhancement"])
        task = Task(title="Fix bug", task_type="bugfix")

        assert evaluate(condition, task) is False

    def test_task_type_none_no_match(self) -> None:
        """task_type should not match when task has None."""
        condition = Condition(task_type="feature")
        task = Task(title="Some task")  # task_type is None

        assert evaluate(condition, task) is False


class TestEvaluateDomain:
    """Tests for domain condition evaluation."""

    def test_domain_single_match(self) -> None:
        """domain should match single value."""
        condition = Condition(domain="web-backend")
        task = Task(title="Add API", domain="web-backend")

        assert evaluate(condition, task) is True

    def test_domain_single_no_match(self) -> None:
        """domain should not match different value."""
        condition = Condition(domain="web-backend")
        task = Task(title="Add UI", domain="web-frontend")

        assert evaluate(condition, task) is False

    def test_domain_case_insensitive(self) -> None:
        """domain matching should be case-insensitive."""
        condition = Condition(domain="WEB-BACKEND")
        task = Task(title="Add API", domain="web-backend")

        assert evaluate(condition, task) is True

    def test_domain_list_match(self) -> None:
        """domain list should match if any matches."""
        condition = Condition(domain=["web-backend", "api"])
        task = Task(title="Add API", domain="api")

        assert evaluate(condition, task) is True

    def test_domain_none_no_match(self) -> None:
        """domain should not match when task has None."""
        condition = Condition(domain="web-backend")
        task = Task(title="Some task")  # domain is None

        assert evaluate(condition, task) is False


class TestEvaluateHasKeyword:
    """Tests for has_keyword condition evaluation."""

    def test_keyword_in_title(self) -> None:
        """has_keyword should match keyword in title."""
        condition = Condition(has_keyword=["login"])
        task = Task(title="Fix login bug")

        assert evaluate(condition, task) is True

    def test_keyword_in_description(self) -> None:
        """has_keyword should match keyword in description."""
        condition = Condition(has_keyword=["authentication"])
        task = Task(title="Fix bug", description="Issues with authentication")

        assert evaluate(condition, task) is True

    def test_keyword_case_insensitive(self) -> None:
        """has_keyword should be case-insensitive."""
        condition = Condition(has_keyword=["LOGIN"])
        task = Task(title="Fix login bug")

        assert evaluate(condition, task) is True

    def test_keyword_any_match(self) -> None:
        """has_keyword should match if any keyword matches."""
        condition = Condition(has_keyword=["login", "auth", "security"])
        task = Task(title="Fix security issue")

        assert evaluate(condition, task) is True

    def test_keyword_no_match(self) -> None:
        """has_keyword should not match if no keyword found."""
        condition = Condition(has_keyword=["login", "auth"])
        task = Task(title="Fix database query")

        assert evaluate(condition, task) is False

    def test_keyword_empty_list(self) -> None:
        """has_keyword with empty list should match (vacuously true)."""
        condition = Condition(has_keyword=[])
        task = Task(title="Any task")

        assert evaluate(condition, task) is True


class TestEvaluateFilePattern:
    """Tests for file_pattern condition evaluation."""

    def test_file_pattern_exact_match(self) -> None:
        """file_pattern should match exact file."""
        condition = Condition(file_pattern=["src/auth.py"])
        task = Task(title="Fix auth", scope="src/auth.py")

        assert evaluate(condition, task) is True

    def test_file_pattern_glob_match(self) -> None:
        """file_pattern should match glob patterns."""
        condition = Condition(file_pattern=["*.py"])
        task = Task(title="Fix code", scope="auth.py")

        assert evaluate(condition, task) is True

    def test_file_pattern_directory_match(self) -> None:
        """file_pattern should match directory patterns."""
        condition = Condition(file_pattern=["src/**"])
        task = Task(title="Fix code", scope="src/auth/login.py")

        assert evaluate(condition, task) is True

    def test_file_pattern_any_match(self) -> None:
        """file_pattern should match if any pattern matches."""
        condition = Condition(file_pattern=["*.py", "*.ts"])
        task = Task(title="Fix code", scope="component.ts")

        assert evaluate(condition, task) is True

    def test_file_pattern_no_match(self) -> None:
        """file_pattern should not match if no pattern matches."""
        condition = Condition(file_pattern=["*.py"])
        task = Task(title="Fix code", scope="component.ts")

        assert evaluate(condition, task) is False

    def test_file_pattern_none_scope(self) -> None:
        """file_pattern should not match when task has no scope."""
        condition = Condition(file_pattern=["*.py"])
        task = Task(title="Fix code")  # scope is None

        assert evaluate(condition, task) is False

    def test_file_pattern_empty_list(self) -> None:
        """file_pattern with empty list should match (vacuously true)."""
        condition = Condition(file_pattern=[])
        task = Task(title="Any task", scope="any.py")

        assert evaluate(condition, task) is True


# ============================================================================
# Test Logical Operators
# ============================================================================


class TestEvaluateNot:
    """Tests for not_ (NOT) condition evaluation."""

    def test_not_negates_true(self) -> None:
        """not_ should negate true to false."""
        inner = Condition(task_type="feature")
        condition = Condition(not_=inner)
        task = Task(title="Add feature", task_type="feature")

        assert evaluate(condition, task) is False

    def test_not_negates_false(self) -> None:
        """not_ should negate false to true."""
        inner = Condition(task_type="feature")
        condition = Condition(not_=inner)
        task = Task(title="Fix bug", task_type="bugfix")

        assert evaluate(condition, task) is True


class TestEvaluateAll:
    """Tests for all (AND) condition evaluation."""

    def test_all_empty_is_true(self) -> None:
        """all with empty list should be vacuously true."""
        condition = Condition(all=[])
        task = Task(title="Any task")

        assert evaluate(condition, task) is True

    def test_all_single_true(self) -> None:
        """all with single true condition should be true."""
        condition = Condition(all=[Condition(task_type="feature")])
        task = Task(title="Add feature", task_type="feature")

        assert evaluate(condition, task) is True

    def test_all_multiple_all_true(self) -> None:
        """all should be true when all conditions match."""
        condition = Condition(
            all=[
                Condition(task_type="feature"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add API", task_type="feature", domain="web-backend")

        assert evaluate(condition, task) is True

    def test_all_multiple_one_false(self) -> None:
        """all should be false when any condition fails."""
        condition = Condition(
            all=[
                Condition(task_type="feature"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add UI", task_type="feature", domain="web-frontend")

        assert evaluate(condition, task) is False


class TestEvaluateAny:
    """Tests for any (OR) condition evaluation."""

    def test_any_empty_is_true(self) -> None:
        """any with empty list should be vacuously true."""
        condition = Condition(any=[])
        task = Task(title="Any task")

        assert evaluate(condition, task) is True

    def test_any_single_true(self) -> None:
        """any with single true condition should be true."""
        condition = Condition(any=[Condition(task_type="feature")])
        task = Task(title="Add feature", task_type="feature")

        assert evaluate(condition, task) is True

    def test_any_multiple_one_true(self) -> None:
        """any should be true when any condition matches."""
        condition = Condition(
            any=[
                Condition(task_type="feature"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add UI", task_type="bugfix", domain="web-backend")

        assert evaluate(condition, task) is True

    def test_any_multiple_none_true(self) -> None:
        """any should be false when no conditions match."""
        condition = Condition(
            any=[
                Condition(task_type="feature"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add UI", task_type="bugfix", domain="web-frontend")

        assert evaluate(condition, task) is False


# ============================================================================
# Test Combined Conditions
# ============================================================================


class TestEvaluateCombined:
    """Tests for combined condition evaluation."""

    def test_empty_condition_matches_all(self) -> None:
        """Empty condition should match all tasks (vacuously true)."""
        condition = Condition()
        task = Task(title="Any task")

        assert evaluate(condition, task) is True

    def test_multiple_atomic_conditions_implicit_and(self) -> None:
        """Multiple atomic conditions should act as implicit AND."""
        condition = Condition(
            task_type="feature",
            domain="web-backend",
        )
        task = Task(title="Add API", task_type="feature", domain="web-backend")

        assert evaluate(condition, task) is True

    def test_multiple_atomic_one_fails(self) -> None:
        """Multiple atomic conditions should fail if any fails."""
        condition = Condition(
            task_type="feature",
            domain="web-backend",
        )
        task = Task(title="Add UI", task_type="feature", domain="web-frontend")

        assert evaluate(condition, task) is False


# ============================================================================
# Test RuleEngine
# ============================================================================


class TestRuleEngine:
    """Tests for RuleEngine class."""

    @pytest.fixture
    def engine(self) -> RuleEngine:
        """Create a fresh RuleEngine instance."""
        return RuleEngine()

    @pytest.fixture
    def debug_override(self) -> Override:
        """Create a debug override."""
        return Override(
            name="debug-override",
            priority=90,
            condition=Condition(has_keyword=["debug", "fix bug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debugging tasks should use debugger agent",
        )

    @pytest.fixture
    def security_override(self) -> Override:
        """Create a security override."""
        return Override(
            name="security-override",
            priority=100,
            condition=Condition(has_keyword=["security", "vulnerability"]),
            action=OverrideAction(set_primary="security-auditor"),
            reason="Security tasks should use security auditor",
        )

    def test_add_override(self, engine: RuleEngine, debug_override: Override) -> None:
        """add_override should add override to engine."""
        engine.add_override(debug_override)

        assert len(engine._overrides) == 1

    def test_add_override_sorted_by_priority(
        self,
        engine: RuleEngine,
        debug_override: Override,
        security_override: Override,
    ) -> None:
        """add_override should maintain priority order."""
        engine.add_override(debug_override)  # priority 90
        engine.add_override(security_override)  # priority 100

        # Higher priority should be first
        assert engine._overrides[0].name == "security-override"
        assert engine._overrides[1].name == "debug-override"

    def test_find_matching_override_match(
        self, engine: RuleEngine, debug_override: Override
    ) -> None:
        """find_matching_override should return matching override."""
        engine.add_override(debug_override)
        task = Task(title="Debug login issue")

        result = engine.find_matching_override(task)

        assert result is not None
        assert result.name == "debug-override"
        assert result.action.set_primary == "debugger"

    def test_find_matching_override_no_match(
        self, engine: RuleEngine, debug_override: Override
    ) -> None:
        """find_matching_override should return None when no match."""
        engine.add_override(debug_override)
        task = Task(title="Add new feature")

        result = engine.find_matching_override(task)

        assert result is None

    def test_find_matching_override_priority(
        self,
        engine: RuleEngine,
        debug_override: Override,
        security_override: Override,
    ) -> None:
        """find_matching_override should return highest priority match."""
        engine.add_override(debug_override)
        engine.add_override(security_override)
        task = Task(title="Debug security vulnerability")

        result = engine.find_matching_override(task)

        # Security has higher priority
        assert result is not None
        assert result.name == "security-override"

    def test_add_chain_extension(self, engine: RuleEngine) -> None:
        """add_chain_extension should add extension to engine."""
        extension = ChainExtension(
            name="add-tester",
            condition=Condition(has_keyword=["test"]),
            action=ChainExtensionAction(add_to_chain="test-automator"),
        )
        engine.add_chain_extension(extension)

        assert len(engine._chain_extensions) == 1

    def test_find_matching_chain_extensions(self, engine: RuleEngine) -> None:
        """find_matching_chain_extensions should return all matching extensions."""
        ext1 = ChainExtension(
            name="add-tester",
            condition=Condition(has_keyword=["test"]),
            action=ChainExtensionAction(add_to_chain="test-automator"),
        )
        ext2 = ChainExtension(
            name="add-reviewer",
            condition=Condition(has_keyword=["review"]),
            action=ChainExtensionAction(add_to_chain="code-reviewer"),
        )
        ext3 = ChainExtension(
            name="add-security",
            condition=Condition(has_keyword=["test"]),  # Also matches "test"
            action=ChainExtensionAction(add_to_chain="security-auditor"),
        )

        engine.add_chain_extension(ext1)
        engine.add_chain_extension(ext2)
        engine.add_chain_extension(ext3)

        task = Task(title="Write test cases")
        matches = engine.find_matching_chain_extensions(task)

        assert len(matches) == 2
        names = [m.name for m in matches]
        assert "add-tester" in names
        assert "add-security" in names

    def test_evaluate_condition(
        self, engine: RuleEngine, debug_override: Override
    ) -> None:
        """evaluate_condition should delegate to module evaluate()."""
        condition = Condition(task_type="feature")
        task = Task(title="Add feature", task_type="feature")

        result = engine.evaluate_condition(condition, task)

        assert result is True
