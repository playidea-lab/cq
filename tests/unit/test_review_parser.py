"""Tests for c4.review_parser module"""

import tempfile
from pathlib import Path

import pytest

from c4.review_parser import (
    ReviewIssue,
    filter_issues_by_severity,
    issues_to_required_changes,
    issues_to_task_titles,
    parse_review_content,
    parse_review_report,
)


class TestParseReviewContent:
    """Tests for parse_review_content function"""

    def test_parse_empty_content(self):
        """Empty content returns empty list"""
        assert parse_review_content("") == []

    def test_parse_single_critical_issue(self):
        """Parse a single critical issue"""
        content = """
## Critical Issues

### [C-001] SQL Injection vulnerability
- **파일**: `src/db.py:42`
- **설명**: User input not sanitized
"""
        issues = parse_review_content(content)

        assert len(issues) == 1
        assert issues[0].id == "C-001"
        assert issues[0].severity == "Critical"
        assert issues[0].title == "SQL Injection vulnerability"
        assert issues[0].file == "src/db.py:42"
        assert issues[0].description == "User input not sanitized"

    def test_parse_multiple_severities(self):
        """Parse issues across multiple severity levels"""
        content = """
## Critical Issues

### [C-001] Critical bug

## High Issues

### [H-001] High priority issue
### [H-002] Another high issue

## Medium Issues

### [M-001] Medium issue

## Low Issues

### [L-001] Low priority
"""
        issues = parse_review_content(content)

        assert len(issues) == 5
        assert issues[0].severity == "Critical"
        assert issues[1].severity == "High"
        assert issues[2].severity == "High"
        assert issues[3].severity == "Medium"
        assert issues[4].severity == "Low"

    def test_parse_issue_without_file(self):
        """Issue without file info is still parsed"""
        content = """
## High Issues

### [H-001] Issue without file path
"""
        issues = parse_review_content(content)

        assert len(issues) == 1
        assert issues[0].file is None

    def test_parse_case_insensitive_severity(self):
        """Severity headers are case insensitive"""
        content = """
## CRITICAL ISSUES

### [C-001] Test issue
"""
        issues = parse_review_content(content)

        assert len(issues) == 1
        assert issues[0].severity == "Critical"


class TestParseReviewReport:
    """Tests for parse_review_report function with file I/O"""

    def test_parse_nonexistent_file(self):
        """Nonexistent file returns empty list"""
        result = parse_review_report(Path("/nonexistent/path.md"))
        assert result == []

    def test_parse_real_file(self):
        """Parse a real markdown file"""
        content = """# Code Review Report

## Critical Issues

### [C-001] Test vulnerability
- **파일**: `test.py:1`
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".md", delete=False) as f:
            f.write(content)
            f.flush()

            issues = parse_review_report(Path(f.name))

            assert len(issues) == 1
            assert issues[0].id == "C-001"


class TestFilterIssuesBySeverity:
    """Tests for filter_issues_by_severity function"""

    @pytest.fixture
    def sample_issues(self) -> list[ReviewIssue]:
        """Create sample issues for testing"""
        return [
            ReviewIssue(id="C-001", severity="Critical", title="Critical 1"),
            ReviewIssue(id="H-001", severity="High", title="High 1"),
            ReviewIssue(id="H-002", severity="High", title="High 2"),
            ReviewIssue(id="M-001", severity="Medium", title="Medium 1"),
            ReviewIssue(id="L-001", severity="Low", title="Low 1"),
        ]

    def test_filter_critical_only(self, sample_issues):
        """Filter for Critical only"""
        result = filter_issues_by_severity(sample_issues, "Critical")
        assert len(result) == 1
        assert all(i.severity == "Critical" for i in result)

    def test_filter_high_and_above(self, sample_issues):
        """Filter for High and Critical"""
        result = filter_issues_by_severity(sample_issues, "High")
        assert len(result) == 3
        assert all(i.severity in ["Critical", "High"] for i in result)

    def test_filter_medium_and_above(self, sample_issues):
        """Filter for Medium, High, and Critical"""
        result = filter_issues_by_severity(sample_issues, "Medium")
        assert len(result) == 4
        assert all(i.severity != "Low" for i in result)

    def test_filter_all(self, sample_issues):
        """Filter includes all when Low is threshold"""
        result = filter_issues_by_severity(sample_issues, "Low")
        assert len(result) == 5

    def test_invalid_severity_raises(self, sample_issues):
        """Invalid severity raises ValueError"""
        with pytest.raises(ValueError, match="Invalid severity"):
            filter_issues_by_severity(sample_issues, "Invalid")


class TestIssuesToTaskTitles:
    """Tests for issues_to_task_titles function"""

    def test_convert_to_titles(self):
        """Convert issues to task titles"""
        issues = [
            ReviewIssue(id="C-001", severity="Critical", title="Fix SQL injection"),
            ReviewIssue(id="H-001", severity="High", title="Add input validation"),
        ]

        titles = issues_to_task_titles(issues, min_severity="High")

        assert len(titles) == 2
        assert titles[0] == "[Critical] Fix SQL injection"
        assert titles[1] == "[High] Add input validation"

    def test_filter_by_severity(self):
        """Only includes issues at or above min_severity"""
        issues = [
            ReviewIssue(id="C-001", severity="Critical", title="Critical"),
            ReviewIssue(id="M-001", severity="Medium", title="Medium"),
            ReviewIssue(id="L-001", severity="Low", title="Low"),
        ]

        titles = issues_to_task_titles(issues, min_severity="High")

        assert len(titles) == 1
        assert "[Critical]" in titles[0]


class TestIssuesToRequiredChanges:
    """Tests for issues_to_required_changes function"""

    def test_convert_with_file_info(self):
        """Include file info in change description"""
        issues = [
            ReviewIssue(
                id="C-001",
                severity="Critical",
                title="Fix bug",
                file="src/main.py:42",
            ),
        ]

        changes = issues_to_required_changes(issues, min_severity="Critical")

        assert len(changes) == 1
        assert "[Critical] Fix bug (src/main.py:42)" == changes[0]

    def test_convert_without_file_info(self):
        """Works without file info"""
        issues = [
            ReviewIssue(id="H-001", severity="High", title="Add tests"),
        ]

        changes = issues_to_required_changes(issues, min_severity="High")

        assert changes == ["[High] Add tests"]
