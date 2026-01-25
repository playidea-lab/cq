"""Unit tests for GapAnalyzer."""

import json
import tempfile
from pathlib import Path

import pytest
import yaml

from c4.docs.gap import (
    GapAnalysisResult,
    GapAnalyzer,
    ImplementationStatus,
    Priority,
    RequirementGap,
)
from c4.docs.testgen import EarsPattern, Requirement

# =============================================================================
# Test Data
# =============================================================================

SAMPLE_SPEC = {
    "feature": "user-auth",
    "version": "1.0",
    "domain": "web-backend",
    "description": "User authentication system",
    "requirements": [
        {
            "id": "REQ-001",
            "pattern": "ubiquitous",
            "text": "The system shall validate user credentials",
            "domain": "web-backend",
            "priority": 1,
            "testable": True,
        },
        {
            "id": "REQ-002",
            "pattern": "event-driven",
            "text": "When user submits login form, the system shall verify password",
            "domain": "web-backend",
            "priority": 1,
            "testable": True,
        },
        {
            "id": "REQ-003",
            "pattern": "state-driven",
            "text": "While session is active, the system shall allow access",
            "domain": "web-backend",
            "priority": 2,
            "testable": True,
        },
        {
            "id": "REQ-004",
            "pattern": "optional",
            "text": "If MFA is enabled, the system shall require two-factor authentication",
            "domain": "web-backend",
            "priority": 2,
            "testable": True,
        },
        {
            "id": "REQ-005",
            "pattern": "unwanted",
            "text": "If password is incorrect 5 times, the system shall lock account",
            "domain": "web-backend",
            "priority": 1,
            "testable": True,
        },
        {
            "id": "REQ-006",
            "pattern": "ubiquitous",
            "text": "Non-testable requirement for documentation",
            "domain": "web-backend",
            "priority": 3,
            "testable": False,
        },
    ],
}

SAMPLE_SOURCE_CODE = '''
"""User authentication module."""

def validate_credentials(username, password):
    """Validate user credentials."""
    if not username or not password:
        return False
    return check_password(username, password)


def verify_password(password_hash, password):
    """Verify password against hash."""
    return password_hash == hash_password(password)


class SessionManager:
    """Manage user sessions."""

    def __init__(self):
        self.active_sessions = {}

    def is_active(self, session_id):
        """Check if session is active."""
        return session_id in self.active_sessions

    def allow_access(self, session_id):
        """Allow access for active session."""
        if self.is_active(session_id):
            return True
        return False


def handle_mfa_enabled(user):
    """Handle MFA when enabled."""
    if user.mfa_enabled:
        return require_two_factor()
    return True


def lock_account(user_id, attempts):
    """Lock account after failed attempts."""
    if attempts >= 5:
        prevent_login(user_id)
        return True
    return False
'''

PYTHON_TEST_FILE = '''
"""Existing tests."""

import pytest


def test_validate_credentials():
    """Test credential validation."""
    pass


def test_verify_password():
    """Test password verification."""
    pass
'''


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def temp_project():
    """Create a temporary project structure with specs and source."""
    with tempfile.TemporaryDirectory() as tmpdir:
        tmpdir = Path(tmpdir)

        # Create specs directory
        specs_dir = tmpdir / ".c4" / "specs" / "user-auth"
        specs_dir.mkdir(parents=True)

        with open(specs_dir / "requirements.yaml", "w") as f:
            yaml.dump(SAMPLE_SPEC, f)

        # Create source directory
        source_dir = tmpdir / "src"
        source_dir.mkdir()
        (source_dir / "auth.py").write_text(SAMPLE_SOURCE_CODE)

        # Create tests directory
        tests_dir = tmpdir / "tests" / "unit"
        tests_dir.mkdir(parents=True)
        (tests_dir / "test_auth.py").write_text(PYTHON_TEST_FILE)

        yield {
            "root": tmpdir,
            "specs_dir": tmpdir / ".c4" / "specs",
            "source_dir": source_dir,
            "tests_dir": tmpdir / "tests",
        }


@pytest.fixture
def analyzer(temp_project):
    """Create a GapAnalyzer with temp directories."""
    return GapAnalyzer(
        specs_dir=temp_project["specs_dir"],
        source_dir=temp_project["source_dir"],
        tests_dir=temp_project["tests_dir"],
    )


@pytest.fixture
def empty_analyzer():
    """Create a GapAnalyzer with non-existent directories."""
    return GapAnalyzer(
        specs_dir="/nonexistent/specs",
        source_dir="/nonexistent/src",
        tests_dir="/nonexistent/tests",
    )


# =============================================================================
# Unit Tests - Enums
# =============================================================================


class TestImplementationStatus:
    """Tests for ImplementationStatus enum."""

    def test_all_statuses_defined(self):
        """All implementation statuses are defined."""
        statuses = [s.value for s in ImplementationStatus]
        assert "not_implemented" in statuses
        assert "partially_implemented" in statuses
        assert "implemented" in statuses
        assert "tested" in statuses


class TestPriority:
    """Tests for Priority enum."""

    def test_all_priorities_defined(self):
        """All priority levels are defined."""
        assert Priority.CRITICAL.value == 1
        assert Priority.HIGH.value == 2
        assert Priority.MEDIUM.value == 3
        assert Priority.LOW.value == 4


# =============================================================================
# Unit Tests - RequirementGap Dataclass
# =============================================================================


class TestRequirementGap:
    """Tests for RequirementGap dataclass."""

    def test_gap_creation(self):
        """RequirementGap can be created with required fields."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall do something",
        )

        gap = RequirementGap(
            requirement=req,
            implementation_status=ImplementationStatus.NOT_IMPLEMENTED,
            test_coverage=False,
        )

        assert gap.requirement.id == "REQ-001"
        assert gap.implementation_status == ImplementationStatus.NOT_IMPLEMENTED
        assert gap.test_coverage is False
        assert gap.matched_symbols == []
        assert gap.suggested_priority == Priority.MEDIUM


# =============================================================================
# Unit Tests - GapAnalyzer Initialization
# =============================================================================


class TestGapAnalyzerInit:
    """Tests for GapAnalyzer initialization."""

    def test_default_paths(self):
        """Analyzer uses default paths."""
        analyzer = GapAnalyzer()
        assert analyzer.specs_dir == Path(".c4/specs")
        assert analyzer.source_dir == Path("src")
        assert analyzer.tests_dir == Path("tests")

    def test_custom_paths(self, temp_project):
        """Analyzer accepts custom paths."""
        analyzer = GapAnalyzer(
            specs_dir=temp_project["specs_dir"],
            source_dir=temp_project["source_dir"],
            tests_dir=temp_project["tests_dir"],
        )
        assert analyzer.specs_dir == temp_project["specs_dir"]


# =============================================================================
# Unit Tests - Spec Loading
# =============================================================================


class TestLoadSpec:
    """Tests for spec loading."""

    def test_load_valid_spec(self, analyzer):
        """Valid spec is loaded correctly."""
        spec = analyzer._load_spec("user-auth")

        assert spec["feature"] == "user-auth"
        assert len(spec["requirements"]) == 6

    def test_load_missing_spec(self, analyzer):
        """Missing spec raises FileNotFoundError."""
        with pytest.raises(FileNotFoundError):
            analyzer._load_spec("nonexistent")


# =============================================================================
# Unit Tests - Requirement Parsing
# =============================================================================


class TestParseRequirements:
    """Tests for requirement parsing."""

    def test_parse_all_requirements(self, analyzer):
        """All requirements are parsed."""
        spec = analyzer._load_spec("user-auth")
        reqs = analyzer._parse_requirements(spec)

        assert len(reqs) == 6

    def test_parse_requirement_fields(self, analyzer):
        """Requirement fields are parsed correctly."""
        spec = analyzer._load_spec("user-auth")
        reqs = analyzer._parse_requirements(spec)

        req = reqs[0]
        assert req.id == "REQ-001"
        assert req.pattern == EarsPattern.UBIQUITOUS
        assert "validate user credentials" in req.text
        assert req.testable is True


# =============================================================================
# Unit Tests - Keyword Extraction
# =============================================================================


class TestKeywordExtraction:
    """Tests for keyword extraction from requirements."""

    def test_extract_keywords(self, analyzer):
        """Keywords are extracted from requirement text."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate user credentials",
        )

        keywords = analyzer._extract_keywords_from_requirement(req)

        assert "validate" in keywords
        assert "user" in keywords
        assert "credentials" in keywords
        assert "req_001" in keywords  # ID normalized

    def test_removes_stop_words(self, analyzer):
        """Stop words are removed from keywords."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall have the ability to validate",
        )

        keywords = analyzer._extract_keywords_from_requirement(req)

        assert "the" not in keywords
        assert "shall" not in keywords


# =============================================================================
# Unit Tests - Implementation Search
# =============================================================================


class TestImplementationSearch:
    """Tests for implementation search."""

    def test_find_implemented_requirement(self, analyzer, temp_project):
        """Finds implementation for a requirement."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        source_files = list(temp_project["source_dir"].rglob("*.py"))
        status, symbols, files = analyzer._search_implementation(req, source_files)

        assert status in (
            ImplementationStatus.IMPLEMENTED,
            ImplementationStatus.PARTIALLY_IMPLEMENTED,
        )
        assert len(files) > 0

    def test_not_implemented_requirement(self, analyzer, temp_project):
        """Detects unimplemented requirement."""
        req = Requirement(
            id="REQ-999",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall do something completely unrelated xyz123",
        )

        source_files = list(temp_project["source_dir"].rglob("*.py"))
        status, symbols, files = analyzer._search_implementation(req, source_files)

        assert status == ImplementationStatus.NOT_IMPLEMENTED


# =============================================================================
# Unit Tests - Priority Calculation
# =============================================================================


class TestPriorityCalculation:
    """Tests for priority calculation."""

    def test_unwanted_pattern_higher_priority(self, analyzer):
        """Unwanted pattern gets higher priority."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UNWANTED,
            text="If error occurs, system shall handle it",
            priority=2,
        )

        priority = analyzer._calculate_priority(
            req, ImplementationStatus.NOT_IMPLEMENTED, has_test=False
        )

        # Unwanted + not implemented should be high priority
        assert priority in (Priority.CRITICAL, Priority.HIGH)

    def test_optional_pattern_lower_priority(self, analyzer):
        """Optional pattern gets lower priority."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.OPTIONAL,
            text="If enabled, system shall provide extra feature",
            priority=3,
        )

        priority = analyzer._calculate_priority(
            req, ImplementationStatus.IMPLEMENTED, has_test=True
        )

        assert priority in (Priority.MEDIUM, Priority.LOW)


# =============================================================================
# Unit Tests - Gap Analysis
# =============================================================================


class TestAnalyzeGaps:
    """Tests for analyze_gaps method."""

    def test_analyze_gaps_returns_result(self, analyzer):
        """analyze_gaps returns GapAnalysisResult."""
        result = analyzer.analyze_gaps("user-auth")

        assert isinstance(result, GapAnalysisResult)
        assert result.spec_id == "user-auth"
        assert result.feature == "user-auth"
        assert result.total_requirements == 6

    def test_analyze_gaps_counts(self, analyzer):
        """analyze_gaps calculates correct counts."""
        result = analyzer.analyze_gaps("user-auth")

        # Should have 5 testable requirements
        assert result.testable_requirements == 5

        # Total should match
        total = (
            result.implemented_count
            + result.partially_implemented_count
            + result.not_implemented_count
            + result.tested_count
        )
        assert total == result.testable_requirements

    def test_analyze_gaps_coverage(self, analyzer):
        """analyze_gaps calculates coverage percentages."""
        result = analyzer.analyze_gaps("user-auth")

        assert 0 <= result.implementation_coverage <= 100
        assert 0 <= result.test_coverage <= 100

    def test_analyze_gaps_includes_gaps(self, analyzer):
        """analyze_gaps includes gap details."""
        result = analyzer.analyze_gaps("user-auth")

        assert len(result.gaps) == 5  # 5 testable requirements

        for gap in result.gaps:
            assert isinstance(gap, RequirementGap)
            assert gap.requirement.id.startswith("REQ-")


# =============================================================================
# Unit Tests - Unimplemented Requirements
# =============================================================================


class TestGetUnimplementedRequirements:
    """Tests for get_unimplemented_requirements method."""

    def test_returns_list(self, analyzer):
        """get_unimplemented_requirements returns a list."""
        unimplemented = analyzer.get_unimplemented_requirements("user-auth")

        assert isinstance(unimplemented, list)

    def test_sorted_by_priority(self, analyzer):
        """Unimplemented requirements are sorted by priority."""
        unimplemented = analyzer.get_unimplemented_requirements("user-auth")

        if len(unimplemented) > 1:
            priorities = [r["priority"] for r in unimplemented]
            assert priorities == sorted(priorities)

    def test_includes_required_fields(self, analyzer):
        """Each unimplemented requirement has required fields."""
        unimplemented = analyzer.get_unimplemented_requirements("user-auth")

        for req in unimplemented:
            assert "id" in req
            assert "text" in req
            assert "status" in req
            assert "priority" in req
            assert "priority_label" in req


# =============================================================================
# Unit Tests - Report Generation
# =============================================================================


class TestGenerateReport:
    """Tests for generate_report method."""

    def test_generate_json_report(self, analyzer):
        """JSON report is valid JSON."""
        result = analyzer.analyze_gaps("user-auth")
        report = analyzer.generate_report(result, format="json")

        # Should be valid JSON
        parsed = json.loads(report)
        assert parsed["spec_id"] == "user-auth"
        assert "summary" in parsed
        assert "gaps" in parsed

    def test_generate_markdown_report(self, analyzer):
        """Markdown report contains expected sections."""
        result = analyzer.analyze_gaps("user-auth")
        report = analyzer.generate_report(result, format="markdown")

        assert "# Gap Analysis Report: user-auth" in report
        assert "## Summary" in report
        assert "## Detailed Gaps" in report
        assert "Implementation Coverage" in report

    def test_invalid_format_raises_error(self, analyzer):
        """Invalid format raises ValueError."""
        result = analyzer.analyze_gaps("user-auth")

        with pytest.raises(ValueError, match="Unknown format"):
            analyzer.generate_report(result, format="invalid")


class TestGenerateJsonReport:
    """Tests for JSON report generation."""

    def test_json_report_structure(self, analyzer):
        """JSON report has correct structure."""
        result = analyzer.analyze_gaps("user-auth")
        report = json.loads(analyzer._generate_json_report(result))

        assert "spec_id" in report
        assert "feature" in report
        assert "domain" in report
        assert "analyzed_at" in report
        assert "summary" in report
        assert "files_analyzed" in report
        assert "gaps" in report

    def test_json_summary_fields(self, analyzer):
        """JSON summary has all required fields."""
        result = analyzer.analyze_gaps("user-auth")
        report = json.loads(analyzer._generate_json_report(result))

        summary = report["summary"]
        assert "total_requirements" in summary
        assert "testable_requirements" in summary
        assert "implementation_coverage" in summary
        assert "test_coverage" in summary
        assert "implemented" in summary
        assert "not_implemented" in summary


class TestGenerateMarkdownReport:
    """Tests for Markdown report generation."""

    def test_markdown_has_header(self, analyzer):
        """Markdown report has proper header."""
        result = analyzer.analyze_gaps("user-auth")
        report = analyzer._generate_markdown_report(result)

        assert report.startswith("# Gap Analysis Report:")

    def test_markdown_has_table(self, analyzer):
        """Markdown report has summary table."""
        result = analyzer.analyze_gaps("user-auth")
        report = analyzer._generate_markdown_report(result)

        assert "| Metric | Value |" in report
        assert "|--------|-------|" in report

    def test_markdown_has_status_emoji(self, analyzer):
        """Markdown report uses status emoji."""
        result = analyzer.analyze_gaps("user-auth")
        report = analyzer._generate_markdown_report(result)

        # Should have at least one status emoji
        assert any(emoji in report for emoji in ["✅", "✓", "⚠️", "❌"])


# =============================================================================
# Unit Tests - List Specs
# =============================================================================


class TestListSpecs:
    """Tests for list_specs method."""

    def test_list_specs_returns_list(self, analyzer):
        """list_specs returns a list."""
        specs = analyzer.list_specs()

        assert isinstance(specs, list)
        assert len(specs) == 1  # user-auth

    def test_list_specs_content(self, analyzer):
        """list_specs returns correct content."""
        specs = analyzer.list_specs()

        assert specs[0]["id"] == "user-auth"
        assert specs[0]["feature"] == "user-auth"
        assert specs[0]["domain"] == "web-backend"
        assert specs[0]["requirements_count"] == 6

    def test_list_specs_empty_directory(self, empty_analyzer):
        """list_specs returns empty list for non-existent directory."""
        specs = empty_analyzer.list_specs()

        assert specs == []


# =============================================================================
# Integration Tests
# =============================================================================


class TestIntegration:
    """Integration tests for GapAnalyzer."""

    def test_full_workflow(self, temp_project):
        """Full workflow: analyze, get unimplemented, generate reports."""
        analyzer = GapAnalyzer(
            specs_dir=temp_project["specs_dir"],
            source_dir=temp_project["source_dir"],
            tests_dir=temp_project["tests_dir"],
        )

        # Analyze gaps
        result = analyzer.analyze_gaps("user-auth")
        assert result.total_requirements == 6

        # Get unimplemented
        unimplemented = analyzer.get_unimplemented_requirements("user-auth")
        assert isinstance(unimplemented, list)

        # Generate JSON report
        json_report = analyzer.generate_report(result, format="json")
        parsed = json.loads(json_report)
        assert parsed["spec_id"] == "user-auth"

        # Generate Markdown report
        md_report = analyzer.generate_report(result, format="markdown")
        assert "# Gap Analysis Report" in md_report

    def test_multiple_specs(self, temp_project):
        """Can handle multiple specs."""
        # Add another spec
        another_spec_dir = temp_project["specs_dir"] / "another-feature"
        another_spec_dir.mkdir(parents=True)

        another_spec = {
            "feature": "another-feature",
            "domain": "web-frontend",
            "requirements": [
                {
                    "id": "REQ-A01",
                    "pattern": "ubiquitous",
                    "text": "Another requirement",
                    "testable": True,
                }
            ],
        }

        with open(another_spec_dir / "requirements.yaml", "w") as f:
            yaml.dump(another_spec, f)

        analyzer = GapAnalyzer(
            specs_dir=temp_project["specs_dir"],
            source_dir=temp_project["source_dir"],
            tests_dir=temp_project["tests_dir"],
        )

        # List should show both specs
        specs = analyzer.list_specs()
        spec_ids = [s["id"] for s in specs]
        assert "user-auth" in spec_ids
        assert "another-feature" in spec_ids

        # Can analyze both
        result1 = analyzer.analyze_gaps("user-auth")
        result2 = analyzer.analyze_gaps("another-feature")

        assert result1.spec_id == "user-auth"
        assert result2.spec_id == "another-feature"
