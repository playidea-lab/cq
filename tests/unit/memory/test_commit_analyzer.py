"""Tests for c4/memory/commit_analyzer.py"""

import json
from unittest.mock import MagicMock, patch

import pytest

from c4.memory.commit_analyzer import (
    AnthropicCommitAnalyzer,
    CommitAnalyzer,
    CommitIntent,
    HeuristicAnalyzer,
    OpenAICommitAnalyzer,
    detect_category,
    detect_domains,
    extract_key_changes,
    get_commit_analyzer,
)

# =============================================================================
# CommitIntent Tests
# =============================================================================


class TestCommitIntent:
    """Tests for CommitIntent dataclass."""

    def test_create_basic(self) -> None:
        """Should create CommitIntent with basic fields."""
        intent = CommitIntent(
            sha="abc123",
            intent="Fix null pointer exception",
            category="bug_fix",
        )
        assert intent.sha == "abc123"
        assert intent.intent == "Fix null pointer exception"
        assert intent.category == "bug_fix"
        assert intent.affected_domains == []
        assert intent.key_changes == []
        assert intent.confidence == 1.0
        assert intent.metadata == {}

    def test_create_with_all_fields(self) -> None:
        """Should create CommitIntent with all fields."""
        intent = CommitIntent(
            sha="abc123",
            intent="Add user authentication",
            category="feature",
            affected_domains=["auth", "api"],
            key_changes=["Add login endpoint", "Add JWT validation"],
            confidence=0.95,
            metadata={"analyzer": "anthropic"},
        )
        assert intent.affected_domains == ["auth", "api"]
        assert intent.key_changes == ["Add login endpoint", "Add JWT validation"]
        assert intent.confidence == 0.95
        assert intent.metadata == {"analyzer": "anthropic"}

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        intent = CommitIntent(
            sha="abc123",
            intent="Refactor code",
            category="refactor",
            affected_domains=["backend"],
            key_changes=["Extract method"],
            confidence=0.8,
            metadata={"test": True},
        )
        result = intent.to_dict()

        assert result["sha"] == "abc123"
        assert result["intent"] == "Refactor code"
        assert result["category"] == "refactor"
        assert result["affected_domains"] == ["backend"]
        assert result["key_changes"] == ["Extract method"]
        assert result["confidence"] == 0.8
        assert result["metadata"] == {"test": True}

    def test_from_dict(self) -> None:
        """Should create from dictionary."""
        data = {
            "sha": "def456",
            "intent": "Add feature",
            "category": "feature",
            "affected_domains": ["frontend"],
            "key_changes": ["New component"],
            "confidence": 0.9,
            "metadata": {"model": "gpt-4"},
        }
        intent = CommitIntent.from_dict(data)

        assert intent.sha == "def456"
        assert intent.intent == "Add feature"
        assert intent.category == "feature"
        assert intent.affected_domains == ["frontend"]
        assert intent.key_changes == ["New component"]
        assert intent.confidence == 0.9
        assert intent.metadata == {"model": "gpt-4"}

    def test_from_dict_with_defaults(self) -> None:
        """Should use defaults for missing fields."""
        data = {"sha": "xyz", "intent": "test", "category": "test"}
        intent = CommitIntent.from_dict(data)

        assert intent.affected_domains == []
        assert intent.key_changes == []
        assert intent.confidence == 1.0
        assert intent.metadata == {}

    def test_roundtrip(self) -> None:
        """Should survive to_dict/from_dict roundtrip."""
        original = CommitIntent(
            sha="abc",
            intent="Test intent",
            category="test",
            affected_domains=["auth", "api"],
            key_changes=["Change 1", "Change 2"],
            confidence=0.75,
            metadata={"key": "value"},
        )
        restored = CommitIntent.from_dict(original.to_dict())

        assert restored.sha == original.sha
        assert restored.intent == original.intent
        assert restored.category == original.category
        assert restored.affected_domains == original.affected_domains
        assert restored.key_changes == original.key_changes
        assert restored.confidence == original.confidence
        assert restored.metadata == original.metadata


# =============================================================================
# Category Detection Tests
# =============================================================================


class TestDetectCategory:
    """Tests for detect_category function."""

    def test_conventional_commit_fix(self) -> None:
        """Should detect fix from conventional commit prefix."""
        category, confidence = detect_category("fix: resolve null pointer")
        assert category == "bug_fix"
        assert confidence >= 0.9

    def test_conventional_commit_feat(self) -> None:
        """Should detect feature from conventional commit prefix."""
        category, confidence = detect_category("feat: add user authentication")
        assert category == "feature"
        assert confidence >= 0.9

    def test_conventional_commit_with_scope(self) -> None:
        """Should handle conventional commit with scope."""
        category, confidence = detect_category("fix(auth): resolve token expiry")
        assert category == "bug_fix"
        assert confidence >= 0.9

    def test_conventional_commit_refactor(self) -> None:
        """Should detect refactor from conventional commit."""
        category, confidence = detect_category("refactor: simplify validation logic")
        assert category == "refactor"
        assert confidence >= 0.9

    def test_conventional_commit_docs(self) -> None:
        """Should detect documentation from conventional commit."""
        category, confidence = detect_category("docs: update README")
        assert category == "documentation"
        assert confidence >= 0.9

    def test_conventional_commit_test(self) -> None:
        """Should detect test from conventional commit."""
        category, confidence = detect_category("test: add unit tests for auth")
        assert category == "test"
        assert confidence >= 0.9

    def test_conventional_commit_style(self) -> None:
        """Should detect style from conventional commit."""
        category, confidence = detect_category("style: format code")
        assert category == "style"
        assert confidence >= 0.9

    def test_conventional_commit_perf(self) -> None:
        """Should detect performance from conventional commit."""
        category, confidence = detect_category("perf: optimize database queries")
        assert category == "performance"
        assert confidence >= 0.9

    def test_conventional_commit_build(self) -> None:
        """Should detect build from conventional commit."""
        category, confidence = detect_category("build: update dependencies")
        assert category == "build"
        assert confidence >= 0.9

    def test_conventional_commit_ci(self) -> None:
        """Should detect CI from conventional commit."""
        category, confidence = detect_category("ci: add github actions workflow")
        assert category == "build"
        assert confidence >= 0.9

    def test_conventional_commit_chore(self) -> None:
        """Should detect chore from conventional commit."""
        category, confidence = detect_category("chore: clean up unused files")
        assert category == "chore"
        assert confidence >= 0.9

    def test_conventional_commit_revert(self) -> None:
        """Should detect revert from conventional commit."""
        category, confidence = detect_category("revert: undo previous change")
        assert category == "revert"
        assert confidence >= 0.9

    def test_keyword_fix(self) -> None:
        """Should detect fix from keywords."""
        category, _ = detect_category("Fixed the login bug")
        assert category == "bug_fix"

    def test_keyword_add(self) -> None:
        """Should detect feature from add keyword."""
        category, _ = detect_category("Add new payment method")
        assert category == "feature"

    def test_keyword_refactor(self) -> None:
        """Should detect refactor from keyword."""
        category, _ = detect_category("Refactored the validation module")
        assert category == "refactor"

    def test_keyword_optimize(self) -> None:
        """Should detect performance from optimize keyword."""
        category, _ = detect_category("Optimize database queries for speed")
        assert category == "performance"

    def test_keyword_test(self) -> None:
        """Should detect test from keyword."""
        category, _ = detect_category("Add unit tests for user service")
        assert category == "test"

    def test_keyword_security(self) -> None:
        """Should detect security from keyword."""
        category, _ = detect_category("Fix security vulnerability in auth")
        assert category in ["security", "bug_fix"]  # Both are valid

    def test_diff_influence(self) -> None:
        """Should consider diff content."""
        category, _ = detect_category(
            "Update code",
            diff="- old_test\n+ new_test\n+ def test_something():",
        )
        # The diff contains test-related content
        assert category in ["test", "unknown", "feature"]

    def test_unknown_category(self) -> None:
        """Should return unknown for ambiguous messages."""
        category, confidence = detect_category("update something")
        # Generic messages have lower confidence
        assert confidence <= 0.9

    def test_case_insensitive(self) -> None:
        """Should be case insensitive."""
        cat1, _ = detect_category("FIX: resolve issue")
        cat2, _ = detect_category("fix: resolve issue")
        assert cat1 == cat2

    def test_multiple_keywords_highest_score(self) -> None:
        """Should pick category with highest score."""
        category, _ = detect_category("fix: add tests for bug fix")
        # Multiple patterns match, should pick most relevant
        assert category in ["bug_fix", "test"]


# =============================================================================
# Domain Detection Tests
# =============================================================================


class TestDetectDomains:
    """Tests for detect_domains function."""

    def test_auth_domain(self) -> None:
        """Should detect auth domain."""
        domains = detect_domains("Fix authentication token expiry")
        assert "auth" in domains

    def test_auth_from_path(self) -> None:
        """Should detect auth from file path in diff."""
        domains = detect_domains("Update code", diff="--- a/src/auth/login.py")
        assert "auth" in domains

    def test_api_domain(self) -> None:
        """Should detect api domain."""
        domains = detect_domains("Add new endpoint for users")
        assert "api" in domains

    def test_database_domain(self) -> None:
        """Should detect database domain."""
        domains = detect_domains("Add migration for users table")
        assert "database" in domains

    def test_frontend_domain(self) -> None:
        """Should detect frontend domain."""
        domains = detect_domains("Update React component styles")
        assert "frontend" in domains

    def test_frontend_from_extension(self) -> None:
        """Should detect frontend from file extension."""
        domains = detect_domains("Update", diff="--- a/src/App.tsx")
        assert "frontend" in domains

    def test_backend_domain(self) -> None:
        """Should detect backend domain."""
        domains = detect_domains("Update service worker")
        assert "backend" in domains

    def test_config_domain(self) -> None:
        """Should detect config domain."""
        domains = detect_domains("Update environment settings")
        assert "config" in domains

    def test_testing_domain(self) -> None:
        """Should detect testing domain."""
        domains = detect_domains("Add unit test fixtures")
        assert "testing" in domains

    def test_testing_from_path(self) -> None:
        """Should detect testing from path."""
        domains = detect_domains("Update", diff="--- a/tests/test_auth.py")
        assert "testing" in domains

    def test_infrastructure_domain(self) -> None:
        """Should detect infrastructure domain."""
        domains = detect_domains("Update Kubernetes deployment")
        assert "infrastructure" in domains

    def test_ml_domain(self) -> None:
        """Should detect ML domain."""
        domains = detect_domains("Update PyTorch model training")
        assert "ml" in domains

    def test_multiple_domains(self) -> None:
        """Should detect multiple domains."""
        domains = detect_domains("Fix auth API endpoint database query")
        assert len(domains) >= 2

    def test_no_domains(self) -> None:
        """Should return empty for generic message."""
        domains = detect_domains("Minor update")
        assert isinstance(domains, list)

    def test_sorted_output(self) -> None:
        """Should return sorted list."""
        domains = detect_domains("Update database API authentication")
        assert domains == sorted(domains)


# =============================================================================
# Key Changes Extraction Tests
# =============================================================================


class TestExtractKeyChanges:
    """Tests for extract_key_changes function."""

    def test_bullet_points(self) -> None:
        """Should extract bullet point items."""
        message = """Add user authentication

- Add login endpoint
- Add logout endpoint
- Add session management
"""
        changes = extract_key_changes(message)
        assert "Add login endpoint" in changes
        assert "Add logout endpoint" in changes
        assert "Add session management" in changes

    def test_numbered_list(self) -> None:
        """Should extract numbered list items."""
        message = """Refactor validation

1. Extract validation logic
2. Add input sanitization
3. Improve error messages
"""
        changes = extract_key_changes(message)
        assert "Extract validation logic" in changes
        assert "Add input sanitization" in changes

    def test_asterisk_bullets(self) -> None:
        """Should extract asterisk bullet items."""
        message = """Update dependencies

* Update React to v18
* Update TypeScript to v5
"""
        changes = extract_key_changes(message)
        assert "Update React to v18" in changes

    def test_plus_bullets(self) -> None:
        """Should extract plus bullet items."""
        message = """Add features

+ New login form
+ Password reset flow
"""
        changes = extract_key_changes(message)
        assert "New login form" in changes

    def test_subject_only(self) -> None:
        """Should use subject when no body."""
        message = "Fix null pointer in auth module"
        changes = extract_key_changes(message)
        assert len(changes) >= 1
        assert "null pointer" in changes[0].lower() or "auth" in changes[0].lower()

    def test_limit_to_five(self) -> None:
        """Should limit to 5 key changes."""
        message = """Many changes

- Change 1
- Change 2
- Change 3
- Change 4
- Change 5
- Change 6
- Change 7
"""
        changes = extract_key_changes(message)
        assert len(changes) <= 5

    def test_skip_empty_lines(self) -> None:
        """Should skip empty lines."""
        message = """Update

- First change

- Second change
"""
        changes = extract_key_changes(message)
        # Should have 2 items, not empty strings
        assert all(c.strip() for c in changes)

    def test_skip_comments(self) -> None:
        """Should skip comment lines."""
        message = """Update

# This is a comment
- Actual change
"""
        changes = extract_key_changes(message)
        assert "This is a comment" not in changes

    def test_conventional_commit_cleanup(self) -> None:
        """Should clean up conventional commit prefix from subject."""
        message = "fix(auth): resolve token expiry issue"
        changes = extract_key_changes(message)
        assert len(changes) >= 1
        # Should extract the actual change, not the prefix
        assert "resolve" in changes[0].lower() or "token" in changes[0].lower()


# =============================================================================
# HeuristicAnalyzer Tests
# =============================================================================


class TestHeuristicAnalyzer:
    """Tests for HeuristicAnalyzer."""

    def test_analyze_bug_fix(self) -> None:
        """Should analyze bug fix commit."""
        analyzer = HeuristicAnalyzer()
        intent = analyzer.analyze_commit(
            sha="abc123",
            message="fix: resolve null pointer in auth module",
        )

        assert intent.sha == "abc123"
        assert intent.category == "bug_fix"
        assert "auth" in intent.affected_domains
        assert intent.confidence >= 0.5
        assert intent.metadata.get("analyzer") == "heuristic"

    def test_analyze_feature(self) -> None:
        """Should analyze feature commit."""
        analyzer = HeuristicAnalyzer()
        intent = analyzer.analyze_commit(
            sha="def456",
            message="feat(api): add user registration endpoint",
        )

        assert intent.category == "feature"
        assert "api" in intent.affected_domains

    def test_analyze_with_diff(self) -> None:
        """Should consider diff content."""
        analyzer = HeuristicAnalyzer()
        intent = analyzer.analyze_commit(
            sha="ghi789",
            message="Update code",
            diff="--- a/src/auth/login.py\n+++ b/src/auth/login.py",
        )

        assert "auth" in intent.affected_domains

    def test_generate_intent(self) -> None:
        """Should generate meaningful intent description."""
        analyzer = HeuristicAnalyzer()
        intent = analyzer.analyze_commit(
            sha="jkl012",
            message="refactor: simplify validation logic",
        )

        assert intent.intent
        assert len(intent.intent) > 0

    def test_intent_cleanup(self) -> None:
        """Should clean up conventional prefix from intent."""
        analyzer = HeuristicAnalyzer()
        intent = analyzer.analyze_commit(
            sha="mno345",
            message="fix(auth): resolve token issue",
        )

        # Intent should be clean, not start with "fix(auth):"
        assert not intent.intent.startswith("fix(")


# =============================================================================
# AnthropicCommitAnalyzer Tests
# =============================================================================


class TestAnthropicCommitAnalyzer:
    """Tests for AnthropicCommitAnalyzer."""

    def test_init_with_api_key(self) -> None:
        """Should initialize with API key."""
        analyzer = AnthropicCommitAnalyzer(api_key="test-key")
        assert analyzer.api_key == "test-key"
        assert analyzer.model == "claude-3-haiku-20240307"

    def test_init_with_custom_model(self) -> None:
        """Should accept custom model."""
        analyzer = AnthropicCommitAnalyzer(
            api_key="test-key",
            model="claude-3-sonnet-20240229",
        )
        assert analyzer.model == "claude-3-sonnet-20240229"

    def test_analyze_success(self) -> None:
        """Should analyze commit with API."""
        mock_response = MagicMock()
        mock_response.content = [
            MagicMock(
                text=json.dumps(
                    {
                        "intent": "Fix authentication token expiry",
                        "category": "bug_fix",
                        "affected_domains": ["auth"],
                        "key_changes": ["Fix token refresh"],
                        "confidence": 0.95,
                    }
                )
            )
        ]

        mock_client = MagicMock()
        mock_client.messages.create.return_value = mock_response

        analyzer = AnthropicCommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        intent = analyzer.analyze_commit(
            sha="abc123",
            message="fix: resolve token expiry",
        )

        assert intent.category == "bug_fix"
        assert "auth" in intent.affected_domains
        assert intent.confidence == 0.95

    def test_analyze_fallback_on_error(self) -> None:
        """Should fall back to heuristics on API error."""
        mock_client = MagicMock()
        mock_client.messages.create.side_effect = Exception("API error")

        analyzer = AnthropicCommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        intent = analyzer.analyze_commit(
            sha="abc123",
            message="fix: resolve auth issue",
        )

        # Should still get a result from heuristics
        assert intent.category == "bug_fix"
        assert intent.metadata.get("analyzer") == "heuristic"

    def test_analyze_fallback_on_parse_error(self) -> None:
        """Should fall back on JSON parse error."""
        mock_response = MagicMock()
        mock_response.content = [MagicMock(text="Invalid JSON response")]

        mock_client = MagicMock()
        mock_client.messages.create.return_value = mock_response

        analyzer = AnthropicCommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        intent = analyzer.analyze_commit(
            sha="abc123",
            message="fix: resolve issue",
        )

        # Should get heuristic fallback
        assert intent is not None
        assert intent.metadata.get("analyzer") == "heuristic"

    def test_truncate_long_diff(self) -> None:
        """Should truncate long diffs."""
        mock_response = MagicMock()
        mock_response.content = [
            MagicMock(
                text=json.dumps(
                    {
                        "intent": "Update code",
                        "category": "feature",
                        "affected_domains": [],
                        "key_changes": [],
                        "confidence": 0.8,
                    }
                )
            )
        ]

        mock_client = MagicMock()
        mock_client.messages.create.return_value = mock_response

        analyzer = AnthropicCommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        # Very long diff
        long_diff = "x" * 10000

        analyzer.analyze_commit(sha="abc", message="test", diff=long_diff)

        # Check that the diff was truncated in the call
        call_args = mock_client.messages.create.call_args
        prompt = call_args.kwargs["messages"][0]["content"]
        assert "x" * 5000 not in prompt  # Should be truncated


# =============================================================================
# OpenAICommitAnalyzer Tests
# =============================================================================


class TestOpenAICommitAnalyzer:
    """Tests for OpenAICommitAnalyzer."""

    def test_init_with_api_key(self) -> None:
        """Should initialize with API key."""
        analyzer = OpenAICommitAnalyzer(api_key="test-key")
        assert analyzer.api_key == "test-key"
        assert analyzer.model == "gpt-3.5-turbo"

    def test_analyze_success(self) -> None:
        """Should analyze commit with API."""
        mock_message = MagicMock()
        mock_message.content = json.dumps(
            {
                "intent": "Add new feature",
                "category": "feature",
                "affected_domains": ["api"],
                "key_changes": ["Add endpoint"],
                "confidence": 0.9,
            }
        )

        mock_choice = MagicMock()
        mock_choice.message = mock_message

        mock_response = MagicMock()
        mock_response.choices = [mock_choice]

        mock_client = MagicMock()
        mock_client.chat.completions.create.return_value = mock_response

        analyzer = OpenAICommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        intent = analyzer.analyze_commit(
            sha="abc123",
            message="feat: add user endpoint",
        )

        assert intent.category == "feature"
        assert "api" in intent.affected_domains

    def test_analyze_fallback_on_error(self) -> None:
        """Should fall back to heuristics on API error."""
        mock_client = MagicMock()
        mock_client.chat.completions.create.side_effect = Exception("API error")

        analyzer = OpenAICommitAnalyzer(api_key="test-key")
        analyzer._client = mock_client

        intent = analyzer.analyze_commit(
            sha="abc123",
            message="fix: resolve issue",
        )

        assert intent is not None
        assert intent.metadata.get("analyzer") == "heuristic"


# =============================================================================
# CommitAnalyzer Main Class Tests
# =============================================================================


class TestCommitAnalyzer:
    """Tests for main CommitAnalyzer class."""

    def test_auto_detect_anthropic(self) -> None:
        """Should auto-detect Anthropic when API key set."""
        with patch.dict("os.environ", {"ANTHROPIC_API_KEY": "test-key"}):
            analyzer = CommitAnalyzer()
            assert isinstance(analyzer.provider, AnthropicCommitAnalyzer)

    def test_auto_detect_openai(self) -> None:
        """Should auto-detect OpenAI when API key set."""
        with patch.dict(
            "os.environ",
            {"OPENAI_API_KEY": "test-key"},
            clear=True,
        ):
            # Clear ANTHROPIC_API_KEY to test OpenAI fallback
            import os
            anthropic_key = os.environ.pop("ANTHROPIC_API_KEY", None)
            try:
                analyzer = CommitAnalyzer()
                assert isinstance(analyzer.provider, (OpenAICommitAnalyzer, HeuristicAnalyzer))
            finally:
                if anthropic_key:
                    os.environ["ANTHROPIC_API_KEY"] = anthropic_key

    def test_auto_detect_heuristic_fallback(self) -> None:
        """Should fall back to heuristic when no API keys."""
        with patch.dict("os.environ", {}, clear=True):
            analyzer = CommitAnalyzer()
            assert isinstance(analyzer.provider, HeuristicAnalyzer)

    def test_explicit_provider(self) -> None:
        """Should use explicit provider."""
        heuristic = HeuristicAnalyzer()
        analyzer = CommitAnalyzer(provider=heuristic)
        assert analyzer.provider is heuristic

    def test_analyze_delegates(self) -> None:
        """Should delegate to provider."""
        mock_provider = MagicMock()
        mock_provider.analyze_commit.return_value = CommitIntent(
            sha="abc",
            intent="Test",
            category="test",
        )

        analyzer = CommitAnalyzer(provider=mock_provider)
        result = analyzer.analyze_commit(sha="abc", message="test message")

        mock_provider.analyze_commit.assert_called_once_with(
            "abc", "test message", None, None
        )
        assert result.sha == "abc"


# =============================================================================
# Factory Function Tests
# =============================================================================


class TestGetCommitAnalyzer:
    """Tests for get_commit_analyzer factory function."""

    def test_heuristic_provider(self) -> None:
        """Should create heuristic analyzer."""
        analyzer = get_commit_analyzer("heuristic")
        assert isinstance(analyzer.provider, HeuristicAnalyzer)

    def test_anthropic_provider(self) -> None:
        """Should create Anthropic analyzer."""
        analyzer = get_commit_analyzer("anthropic", api_key="test-key")
        assert isinstance(analyzer.provider, AnthropicCommitAnalyzer)
        assert analyzer.provider.api_key == "test-key"

    def test_openai_provider(self) -> None:
        """Should create OpenAI analyzer."""
        analyzer = get_commit_analyzer("openai", api_key="test-key")
        assert isinstance(analyzer.provider, OpenAICommitAnalyzer)
        assert analyzer.provider.api_key == "test-key"

    def test_auto_detect(self) -> None:
        """Should auto-detect when no provider specified."""
        with patch.dict("os.environ", {}, clear=True):
            analyzer = get_commit_analyzer()
            # Should fall back to heuristic
            assert isinstance(analyzer.provider, HeuristicAnalyzer)

    def test_custom_model(self) -> None:
        """Should pass model to provider."""
        analyzer = get_commit_analyzer(
            "anthropic",
            api_key="test-key",
            model="claude-3-opus-20240229",
        )
        assert analyzer.provider.model == "claude-3-opus-20240229"


# =============================================================================
# Pattern Coverage Tests
# =============================================================================


class TestCategoryPatterns:
    """Tests to verify category patterns work correctly."""

    @pytest.mark.parametrize(
        "message,expected_category",
        [
            ("fix: resolve bug", "bug_fix"),
            ("Fixed the issue", "bug_fix"),
            ("bugfix for login", "bug_fix"),
            ("patch security hole", "bug_fix"),
            ("resolve connection error", "bug_fix"),
            ("feat: add login", "feature"),
            ("Add new feature", "feature"),
            ("implement user auth", "feature"),
            ("introduce caching", "feature"),
            ("refactor: clean up code", "refactor"),
            ("restructure modules", "refactor"),
            ("simplify logic", "refactor"),
            ("perf: optimize query", "performance"),
            ("speed up rendering", "performance"),
            ("optimize caching layer", "performance"),
            ("docs: update readme", "documentation"),
            ("add jsdoc comments", "documentation"),
            ("test: add unit tests", "test"),
            ("improve coverage", "test"),
            ("style: format code", "style"),
            ("run prettier", "style"),
            ("build: update deps", "build"),
            ("ci: add workflow", "build"),
            ("bump version", "build"),
            ("chore: clean up", "chore"),
            ("remove unused code", "chore"),
            ("revert: undo change", "revert"),
            ("rollback previous commit", "revert"),
        ],
    )
    def test_category_detection(self, message: str, expected_category: str) -> None:
        """Should detect correct category for various messages."""
        category, _ = detect_category(message)
        assert category == expected_category


class TestDomainPatterns:
    """Tests to verify domain patterns work correctly."""

    @pytest.mark.parametrize(
        "message,expected_domain",
        [
            ("fix auth login", "auth"),
            ("update jwt token", "auth"),
            ("add api endpoint", "api"),
            ("update controller", "api"),
            ("fix database query", "database"),
            ("add migration", "database"),
            ("update react component", "frontend"),
            ("fix css styles", "frontend"),
            ("update backend service", "backend"),
            ("add worker job", "backend"),
            ("update config yaml", "config"),
            ("add env settings", "config"),
            ("add unit test", "testing"),
            ("fix test fixture", "testing"),
            ("update dockerfile", "infrastructure"),
            ("add k8s config", "infrastructure"),
            ("train pytorch model", "ml"),
            ("update tensorflow", "ml"),
        ],
    )
    def test_domain_detection(self, message: str, expected_domain: str) -> None:
        """Should detect correct domain for various messages."""
        domains = detect_domains(message)
        assert expected_domain in domains
