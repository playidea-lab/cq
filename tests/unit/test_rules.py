"""Unit tests for agent_graph rules module."""

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
# Task Tests
# ============================================================================


class TestTask:
    """Tests for Task dataclass."""

    def test_task_creation_minimal(self) -> None:
        """Task can be created with just a title."""
        task = Task(title="Test task")
        assert task.title == "Test task"
        assert task.description == ""
        assert task.task_type is None
        assert task.domain is None
        assert task.scope is None

    def test_task_creation_full(self) -> None:
        """Task can be created with all fields."""
        task = Task(
            title="Add login feature",
            description="Implement user authentication",
            task_type="feature",
            domain="web-frontend",
            scope="src/auth/",
        )
        assert task.title == "Add login feature"
        assert task.description == "Implement user authentication"
        assert task.task_type == "feature"
        assert task.domain == "web-frontend"
        assert task.scope == "src/auth/"

    def test_task_empty_title_raises(self) -> None:
        """Task with empty title raises ValueError."""
        with pytest.raises(ValueError, match="title cannot be empty"):
            Task(title="")


# ============================================================================
# Atomic Condition Tests
# ============================================================================


class TestTaskTypeCondition:
    """Tests for task_type condition evaluation."""

    def test_task_type_string_match(self) -> None:
        """Single task_type string matches."""
        cond = Condition(task_type="feature")
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True

    def test_task_type_string_no_match(self) -> None:
        """Single task_type string doesn't match."""
        cond = Condition(task_type="bugfix")
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is False

    def test_task_type_list_match(self) -> None:
        """Task type list matches if any matches."""
        cond = Condition(task_type=["feature", "enhancement"])
        task = Task(title="Add login", task_type="enhancement")
        assert evaluate(cond, task) is True

    def test_task_type_case_insensitive(self) -> None:
        """Task type matching is case-insensitive."""
        cond = Condition(task_type="FEATURE")
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True

    def test_task_type_none_in_task(self) -> None:
        """Task with None task_type doesn't match."""
        cond = Condition(task_type="feature")
        task = Task(title="Add login")
        assert evaluate(cond, task) is False


class TestDomainCondition:
    """Tests for domain condition evaluation."""

    def test_domain_string_match(self) -> None:
        """Single domain string matches."""
        cond = Condition(domain="web-frontend")
        task = Task(title="Add login", domain="web-frontend")
        assert evaluate(cond, task) is True

    def test_domain_string_no_match(self) -> None:
        """Single domain string doesn't match."""
        cond = Condition(domain="web-backend")
        task = Task(title="Add login", domain="web-frontend")
        assert evaluate(cond, task) is False

    def test_domain_list_match(self) -> None:
        """Domain list matches if any matches."""
        cond = Condition(domain=["web-frontend", "mobile-app"])
        task = Task(title="Add login", domain="mobile-app")
        assert evaluate(cond, task) is True

    def test_domain_case_insensitive(self) -> None:
        """Domain matching is case-insensitive."""
        cond = Condition(domain="WEB-FRONTEND")
        task = Task(title="Add login", domain="web-frontend")
        assert evaluate(cond, task) is True


class TestHasKeywordCondition:
    """Tests for has_keyword condition evaluation."""

    def test_keyword_in_title(self) -> None:
        """Keyword found in title."""
        cond = Condition(has_keyword=["login", "auth"])
        task = Task(title="Add login feature")
        assert evaluate(cond, task) is True

    def test_keyword_in_description(self) -> None:
        """Keyword found in description."""
        cond = Condition(has_keyword=["authentication"])
        task = Task(title="Add feature", description="Implement authentication")
        assert evaluate(cond, task) is True

    def test_keyword_not_found(self) -> None:
        """Keyword not found in title or description."""
        cond = Condition(has_keyword=["database"])
        task = Task(title="Add login", description="User authentication")
        assert evaluate(cond, task) is False

    def test_keyword_case_insensitive(self) -> None:
        """Keyword matching is case-insensitive."""
        cond = Condition(has_keyword=["LOGIN"])
        task = Task(title="Add login feature")
        assert evaluate(cond, task) is True

    def test_keyword_partial_match(self) -> None:
        """Keyword partial match works."""
        cond = Condition(has_keyword=["auth"])
        task = Task(title="Add authentication")
        assert evaluate(cond, task) is True

    def test_empty_keywords_matches(self) -> None:
        """Empty keywords list matches everything."""
        cond = Condition(has_keyword=[])
        task = Task(title="Any task")
        assert evaluate(cond, task) is True


class TestFilePatternCondition:
    """Tests for file_pattern condition evaluation."""

    def test_file_pattern_exact_match(self) -> None:
        """Exact file pattern match."""
        cond = Condition(file_pattern=["src/auth.py"])
        task = Task(title="Fix auth", scope="src/auth.py")
        assert evaluate(cond, task) is True

    def test_file_pattern_glob_match(self) -> None:
        """Glob pattern match with wildcard."""
        cond = Condition(file_pattern=["src/*.py"])
        task = Task(title="Fix auth", scope="src/auth.py")
        assert evaluate(cond, task) is True

    def test_file_pattern_double_star_match(self) -> None:
        """Double-star glob pattern match."""
        cond = Condition(file_pattern=["src/**/*.py"])
        task = Task(title="Fix auth", scope="src/utils/helpers.py")
        # This is tricky - fnmatch doesn't support ** like pathlib
        # Our implementation has special handling for this
        assert evaluate(cond, task) is True

    def test_file_pattern_directory_match(self) -> None:
        """Directory pattern match."""
        cond = Condition(file_pattern=["src/auth/*"])
        task = Task(title="Fix auth", scope="src/auth/login.py")
        assert evaluate(cond, task) is True

    def test_file_pattern_no_match(self) -> None:
        """File pattern doesn't match."""
        cond = Condition(file_pattern=["tests/*"])
        task = Task(title="Fix auth", scope="src/auth.py")
        assert evaluate(cond, task) is False

    def test_file_pattern_none_scope(self) -> None:
        """Task with None scope doesn't match."""
        cond = Condition(file_pattern=["src/*"])
        task = Task(title="Fix auth")
        assert evaluate(cond, task) is False

    def test_file_pattern_empty_matches(self) -> None:
        """Empty patterns list matches everything."""
        cond = Condition(file_pattern=[])
        task = Task(title="Any task")
        assert evaluate(cond, task) is True


# ============================================================================
# Logical Operator Tests
# ============================================================================


class TestNotCondition:
    """Tests for NOT condition evaluation."""

    def test_not_true_becomes_false(self) -> None:
        """NOT true = false."""
        inner = Condition(task_type="feature")
        cond = Condition(not_=inner)
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is False

    def test_not_false_becomes_true(self) -> None:
        """NOT false = true."""
        inner = Condition(task_type="bugfix")
        cond = Condition(not_=inner)
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True

    def test_double_not(self) -> None:
        """Double NOT = original."""
        inner = Condition(task_type="feature")
        middle = Condition(not_=inner)
        cond = Condition(not_=middle)
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True


class TestAllCondition:
    """Tests for AND (all) condition evaluation."""

    def test_all_empty_matches(self) -> None:
        """Empty all list matches (vacuously true)."""
        cond = Condition(all=[])
        task = Task(title="Any task")
        assert evaluate(cond, task) is True

    def test_all_single_match(self) -> None:
        """Single condition in all matches."""
        cond = Condition(all=[Condition(task_type="feature")])
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True

    def test_all_multiple_all_match(self) -> None:
        """All conditions match."""
        cond = Condition(
            all=[
                Condition(task_type="feature"),
                Condition(domain="web-frontend"),
            ]
        )
        task = Task(title="Add login", task_type="feature", domain="web-frontend")
        assert evaluate(cond, task) is True

    def test_all_one_fails(self) -> None:
        """One condition fails, all fails."""
        cond = Condition(
            all=[
                Condition(task_type="feature"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add login", task_type="feature", domain="web-frontend")
        assert evaluate(cond, task) is False


class TestAnyCondition:
    """Tests for OR (any) condition evaluation."""

    def test_any_empty_matches(self) -> None:
        """Empty any list matches (vacuously true)."""
        cond = Condition(any=[])
        task = Task(title="Any task")
        assert evaluate(cond, task) is True

    def test_any_single_match(self) -> None:
        """Single condition in any matches."""
        cond = Condition(any=[Condition(task_type="feature")])
        task = Task(title="Add login", task_type="feature")
        assert evaluate(cond, task) is True

    def test_any_one_matches(self) -> None:
        """One condition matches, any succeeds."""
        cond = Condition(
            any=[
                Condition(task_type="bugfix"),
                Condition(domain="web-frontend"),
            ]
        )
        task = Task(title="Add login", task_type="feature", domain="web-frontend")
        assert evaluate(cond, task) is True

    def test_any_none_match(self) -> None:
        """No conditions match, any fails."""
        cond = Condition(
            any=[
                Condition(task_type="bugfix"),
                Condition(domain="web-backend"),
            ]
        )
        task = Task(title="Add login", task_type="feature", domain="web-frontend")
        assert evaluate(cond, task) is False


class TestComplexConditions:
    """Tests for complex nested conditions."""

    def test_nested_all_any(self) -> None:
        """Nested all containing any."""
        # (type=feature OR type=enhancement) AND domain=web-frontend
        cond = Condition(
            all=[
                Condition(
                    any=[
                        Condition(task_type="feature"),
                        Condition(task_type="enhancement"),
                    ]
                ),
                Condition(domain="web-frontend"),
            ]
        )
        task = Task(title="Add login", task_type="enhancement", domain="web-frontend")
        assert evaluate(cond, task) is True

    def test_nested_any_all(self) -> None:
        """Nested any containing all."""
        # (type=feature AND domain=frontend) OR (type=bugfix AND has "urgent")
        cond = Condition(
            any=[
                Condition(
                    all=[
                        Condition(task_type="feature"),
                        Condition(domain="web-frontend"),
                    ]
                ),
                Condition(
                    all=[
                        Condition(task_type="bugfix"),
                        Condition(has_keyword=["urgent"]),
                    ]
                ),
            ]
        )
        # First branch matches
        task1 = Task(title="Add login", task_type="feature", domain="web-frontend")
        assert evaluate(cond, task1) is True

        # Second branch matches
        task2 = Task(title="Fix urgent bug", task_type="bugfix")
        assert evaluate(cond, task2) is True

        # Neither matches
        task3 = Task(title="Normal bugfix", task_type="bugfix")
        assert evaluate(cond, task3) is False

    def test_not_with_any(self) -> None:
        """NOT with any (De Morgan's law)."""
        # NOT (type=feature OR type=enhancement)
        cond = Condition(
            not_=Condition(
                any=[
                    Condition(task_type="feature"),
                    Condition(task_type="enhancement"),
                ]
            )
        )
        task = Task(title="Fix bug", task_type="bugfix")
        assert evaluate(cond, task) is True

    def test_implicit_and_with_multiple_fields(self) -> None:
        """Multiple fields in same condition = implicit AND."""
        cond = Condition(
            task_type="feature",
            domain="web-frontend",
            has_keyword=["login"],
        )
        # All match
        task1 = Task(
            title="Add login",
            task_type="feature",
            domain="web-frontend",
        )
        assert evaluate(cond, task1) is True

        # One doesn't match
        task2 = Task(
            title="Add signup",
            task_type="feature",
            domain="web-frontend",
        )
        assert evaluate(cond, task2) is False


class TestEmptyCondition:
    """Tests for empty conditions."""

    def test_empty_condition_matches_all(self) -> None:
        """Empty condition matches everything (vacuously true)."""
        cond = Condition()
        task = Task(title="Any task")
        assert evaluate(cond, task) is True


# ============================================================================
# RuleEngine Tests
# ============================================================================


class TestRuleEngine:
    """Tests for RuleEngine class."""

    def test_empty_engine(self) -> None:
        """Empty engine returns no matches."""
        engine = RuleEngine()
        task = Task(title="Any task")
        assert engine.find_matching_override(task) is None
        assert engine.find_matching_chain_extensions(task) == []

    def test_add_override(self) -> None:
        """Override can be added and found."""
        engine = RuleEngine()
        override = Override(
            name="debug-override",
            priority=90,
            condition=Condition(has_keyword=["debug", "fix bug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debug tasks use debugger",
        )
        engine.add_override(override)

        task = Task(title="Fix bug in login")
        match = engine.find_matching_override(task)
        assert match is not None
        assert match.name == "debug-override"
        assert match.action.set_primary == "debugger"

    def test_override_priority_order(self) -> None:
        """Higher priority overrides are returned first."""
        engine = RuleEngine()

        low_priority = Override(
            name="low-priority",
            priority=10,
            condition=Condition(has_keyword=["bug"]),
            action=OverrideAction(set_primary="backend-dev"),
            reason="Low priority",
        )
        high_priority = Override(
            name="high-priority",
            priority=90,
            condition=Condition(has_keyword=["bug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="High priority",
        )

        engine.add_override(low_priority)
        engine.add_override(high_priority)

        task = Task(title="Fix bug")
        match = engine.find_matching_override(task)
        assert match is not None
        assert match.name == "high-priority"

    def test_add_chain_extension(self) -> None:
        """Chain extension can be added and found."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="security-check",
            condition=Condition(has_keyword=["auth", "security"]),
            action=ChainExtensionAction(
                add_to_chain="security-auditor",
                position="before_last",
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add authentication")
        matches = engine.find_matching_chain_extensions(task)
        assert len(matches) == 1
        assert matches[0].name == "security-check"

    def test_multiple_chain_extensions_match(self) -> None:
        """Multiple chain extensions can match."""
        engine = RuleEngine()

        ext1 = ChainExtension(
            name="test-check",
            condition=Condition(file_pattern=["src/*"]),
            action=ChainExtensionAction(add_to_chain="test-runner"),
        )
        ext2 = ChainExtension(
            name="lint-check",
            condition=Condition(file_pattern=["src/*"]),
            action=ChainExtensionAction(add_to_chain="linter"),
        )

        engine.add_chain_extension(ext1)
        engine.add_chain_extension(ext2)

        task = Task(title="Update code", scope="src/main.py")
        matches = engine.find_matching_chain_extensions(task)
        assert len(matches) == 2

    def test_evaluate_condition_wrapper(self) -> None:
        """evaluate_condition wrapper works."""
        engine = RuleEngine()
        cond = Condition(task_type="feature")
        task = Task(title="Add login", task_type="feature")
        assert engine.evaluate_condition(cond, task) is True


# ============================================================================
# Protocol Compatibility Tests
# ============================================================================


class TestTaskLikeProtocol:
    """Tests for TaskLike protocol compatibility."""

    def test_custom_task_like_object(self) -> None:
        """Custom object implementing TaskLike works."""

        class CustomTask:
            @property
            def task_type(self) -> str:
                return "feature"

            @property
            def domain(self) -> str:
                return "web-frontend"

            @property
            def title(self) -> str:
                return "Custom task"

            @property
            def description(self) -> str:
                return "Custom description"

            @property
            def scope(self) -> str | None:
                return "src/custom.py"

        custom = CustomTask()
        cond = Condition(task_type="feature", domain="web-frontend")
        assert evaluate(cond, custom) is True


# ============================================================================
# Action Application Tests
# ============================================================================


class TestApplyOverrides:
    """Tests for RuleEngine.apply_overrides() action application."""

    def test_apply_overrides_set_primary(self) -> None:
        """set_primary action returns the specified agent."""
        engine = RuleEngine()
        override = Override(
            name="debug-override",
            priority=90,
            condition=Condition(has_keyword=["debug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debug tasks use debugger",
        )
        engine.add_override(override)

        task = Task(title="Debug login issue")
        result = engine.apply_overrides(task)
        assert result == "debugger"

    def test_apply_overrides_no_match_returns_none(self) -> None:
        """No matching override returns None."""
        engine = RuleEngine()
        override = Override(
            name="debug-override",
            priority=90,
            condition=Condition(has_keyword=["debug"]),
            action=OverrideAction(set_primary="debugger"),
            reason="Debug tasks use debugger",
        )
        engine.add_override(override)

        task = Task(title="Add new feature")
        result = engine.apply_overrides(task)
        assert result is None

    def test_apply_overrides_require_agent(self) -> None:
        """require_agent action returns the specified agent."""
        engine = RuleEngine()
        override = Override(
            name="test-required",
            priority=90,
            condition=Condition(file_pattern=["tests/*"]),
            action=OverrideAction(require_agent="test-automator"),
            reason="Test files need test-automator",
        )
        engine.add_override(override)

        task = Task(title="Add tests", scope="tests/unit/test_login.py")
        result = engine.apply_overrides(task)
        assert result == "test-automator"


class TestExtendChain:
    """Tests for RuleEngine.extend_chain() action application."""

    def test_extend_chain_adds_agent_at_end(self) -> None:
        """Chain extension adds agent at 'last' position."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="reviewer-check",
            condition=Condition(task_type="feature"),
            action=ChainExtensionAction(
                add_to_chain="code-reviewer",
                position="last",
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add login", task_type="feature")
        original_chain = ["backend-dev", "test-automator"]
        result = engine.extend_chain(original_chain, task)

        assert result == ["backend-dev", "test-automator", "code-reviewer"]

    def test_extend_chain_adds_agent_at_first(self) -> None:
        """Chain extension adds agent at 'first' position."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="architect-first",
            condition=Condition(has_keyword=["architecture"]),
            action=ChainExtensionAction(
                add_to_chain="architect",
                position="first",
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Redesign architecture")
        original_chain = ["backend-dev", "code-reviewer"]
        result = engine.extend_chain(original_chain, task)

        assert result == ["architect", "backend-dev", "code-reviewer"]

    def test_extend_chain_adds_agent_after_primary(self) -> None:
        """Chain extension adds agent at 'after_primary' position."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="security-after-primary",
            condition=Condition(has_keyword=["auth"]),
            action=ChainExtensionAction(
                add_to_chain="security-auditor",
                position="after_primary",
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Implement auth flow")
        original_chain = ["backend-dev", "test-automator", "code-reviewer"]
        result = engine.extend_chain(original_chain, task)

        assert result == ["backend-dev", "security-auditor", "test-automator", "code-reviewer"]

    def test_extend_chain_adds_agent_before_last(self) -> None:
        """Chain extension adds agent at 'before_last' position."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="test-before-review",
            condition=Condition(domain="web-backend"),
            action=ChainExtensionAction(
                add_to_chain="test-automator",
                position="before_last",
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add API", domain="web-backend")
        original_chain = ["backend-dev", "code-reviewer"]
        result = engine.extend_chain(original_chain, task)

        assert result == ["backend-dev", "test-automator", "code-reviewer"]

    def test_extend_chain_no_matching_extensions(self) -> None:
        """No matching extensions returns original chain unchanged."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="test-check",
            condition=Condition(has_keyword=["test"]),
            action=ChainExtensionAction(add_to_chain="test-automator"),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add feature")  # No 'test' keyword
        original_chain = ["backend-dev", "code-reviewer"]
        result = engine.extend_chain(original_chain, task)

        assert result == original_chain

    def test_extend_chain_multiple_extensions(self) -> None:
        """Multiple matching extensions all apply."""
        engine = RuleEngine()
        ext1 = ChainExtension(
            name="security-check",
            condition=Condition(has_keyword=["auth"]),
            action=ChainExtensionAction(add_to_chain="security-auditor", position="last"),
        )
        ext2 = ChainExtension(
            name="test-check",
            condition=Condition(has_keyword=["auth"]),
            action=ChainExtensionAction(add_to_chain="test-automator", position="last"),
        )
        engine.add_chain_extension(ext1)
        engine.add_chain_extension(ext2)

        task = Task(title="Implement auth")
        original_chain = ["backend-dev"]
        result = engine.extend_chain(original_chain, task)

        # Both extensions add to 'last', so order depends on extension order
        assert result == ["backend-dev", "security-auditor", "test-automator"]

    def test_extend_chain_avoids_duplicates(self) -> None:
        """Chain extension avoids adding duplicate agents."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="reviewer-check",
            condition=Condition(task_type="feature"),
            action=ChainExtensionAction(add_to_chain="code-reviewer", position="last"),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add login", task_type="feature")
        original_chain = ["backend-dev", "code-reviewer"]  # Already has code-reviewer
        result = engine.extend_chain(original_chain, task)

        # Should not add duplicate
        assert result == ["backend-dev", "code-reviewer"]

    def test_extend_chain_ensure_in_chain(self) -> None:
        """ensure_in_chain action ensures agents are present."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="require-reviewers",
            condition=Condition(domain="library"),
            action=ChainExtensionAction(
                ensure_in_chain=["code-reviewer", "api-documenter"],
            ),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Update library", domain="library")
        original_chain = ["python-pro", "code-reviewer"]  # Already has code-reviewer
        result = engine.extend_chain(original_chain, task)

        # Should add api-documenter but not duplicate code-reviewer
        assert "python-pro" in result
        assert "code-reviewer" in result
        assert "api-documenter" in result
        assert result.count("code-reviewer") == 1

    def test_extend_chain_empty_chain(self) -> None:
        """Chain extension on empty chain works."""
        engine = RuleEngine()
        extension = ChainExtension(
            name="add-first",
            condition=Condition(task_type="feature"),
            action=ChainExtensionAction(add_to_chain="backend-dev", position="first"),
        )
        engine.add_chain_extension(extension)

        task = Task(title="Add login", task_type="feature")
        original_chain: list[str] = []
        result = engine.extend_chain(original_chain, task)

        assert result == ["backend-dev"]
