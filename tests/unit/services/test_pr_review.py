"""Tests for PR Review Service.

Tests for:
- Review analysis parsing
- Comment generation
- Review event determination
- Review body formatting
"""

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.integrations.github_app import GitHubAppClient, PRInfo, ReviewResult
from c4.services.pr_review import (
    CodeIssue,
    PRReviewService,
    ReviewAnalysis,
)

# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def github_client():
    """Create a mock GitHub App client."""
    return MagicMock(spec=GitHubAppClient)


@pytest.fixture
def pr_info():
    """Create sample PR info."""
    return PRInfo(
        owner="owner",
        repo="repo",
        number=42,
        title="Add new feature",
        head_sha="abc123",
        base_branch="main",
        head_branch="feature",
        author="developer",
        diff_url="https://github.com/owner/repo/pull/42.diff",
        installation_id=12345,
    )


@pytest.fixture
def service(github_client):
    """Create PR review service with mock client."""
    return PRReviewService(github_client)


@pytest.fixture
def sample_diff():
    """Sample PR diff content."""
    return """diff --git a/src/main.py b/src/main.py
index abc123..def456 100644
--- a/src/main.py
+++ b/src/main.py
@@ -10,6 +10,10 @@ def main():
     print("Hello, world!")
+
+def new_function():
+    # Added new function
+    return 42
"""


@pytest.fixture
def sample_llm_response():
    """Sample LLM JSON response."""
    return json.dumps(
        {
            "summary": "Adds a new function that returns 42",
            "overall_quality": "good",
            "issues": [
                {
                    "severity": "suggestion",
                    "file_path": "src/main.py",
                    "line": 15,
                    "title": "Add docstring",
                    "description": "Consider adding a docstring to the function",
                    "suggestion": "Add a docstring explaining what the function does",
                }
            ],
            "suggestions": ["Consider adding type hints"],
            "security_concerns": [],
            "test_coverage": "No tests added",
            "labels": ["enhancement"],
        }
    )


# =============================================================================
# Service Initialization Tests
# =============================================================================


class TestPRReviewServiceInit:
    """Test PR Review Service initialization."""

    def test_init_with_defaults(self, github_client):
        """Test service initialization with default values."""
        service = PRReviewService(github_client)

        assert service.github_client is github_client
        assert service.model == "claude-sonnet-4-20250514"
        assert service.max_diff_size == 50000

    def test_init_with_custom_values(self, github_client):
        """Test service initialization with custom values."""
        service = PRReviewService(
            github_client,
            model="claude-opus-4-20250514",
            max_diff_size=100000,
        )

        assert service.model == "claude-opus-4-20250514"
        assert service.max_diff_size == 100000


# =============================================================================
# Analysis Parsing Tests
# =============================================================================


class TestAnalysisParsing:
    """Test LLM response parsing."""

    def test_parse_valid_json(self, service):
        """Test parsing valid JSON response."""
        response = json.dumps(
            {
                "summary": "Test summary",
                "overall_quality": "good",
                "issues": [
                    {
                        "severity": "warning",
                        "file_path": "test.py",
                        "line": 10,
                        "title": "Issue",
                        "description": "Description",
                    }
                ],
                "suggestions": ["Suggestion 1"],
                "security_concerns": ["Concern 1"],
                "test_coverage": "Coverage info",
                "labels": ["bug"],
            }
        )

        analysis = service._parse_analysis(response)

        assert analysis is not None
        assert analysis.summary == "Test summary"
        assert analysis.overall_quality == "good"
        assert len(analysis.issues) == 1
        assert analysis.issues[0].severity == "warning"
        assert len(analysis.suggestions) == 1
        assert len(analysis.security_concerns) == 1
        assert analysis.test_coverage == "Coverage info"
        assert analysis.labels_to_add == ["bug"]

    def test_parse_json_in_markdown_block(self, service):
        """Test parsing JSON wrapped in markdown code block."""
        response = """Here's my analysis:

```json
{
    "summary": "Analysis summary",
    "overall_quality": "acceptable",
    "issues": [],
    "suggestions": [],
    "security_concerns": [],
    "labels": []
}
```

That's my review."""

        analysis = service._parse_analysis(response)

        assert analysis is not None
        assert analysis.summary == "Analysis summary"
        assert analysis.overall_quality == "acceptable"

    def test_parse_invalid_json_returns_fallback(self, service):
        """Test that invalid JSON returns a fallback analysis."""
        response = "This is not valid JSON at all"

        analysis = service._parse_analysis(response)

        assert analysis is not None
        assert "This is not valid JSON" in analysis.summary
        assert analysis.overall_quality == "acceptable"


# =============================================================================
# Comment Generation Tests
# =============================================================================


class TestCommentGeneration:
    """Test review comment generation."""

    def test_create_comments_from_issues(self, service):
        """Test creating comments from issues with file and line."""
        analysis = ReviewAnalysis(
            summary="Test",
            issues=[
                CodeIssue(
                    severity="warning",
                    file_path="src/main.py",
                    line=10,
                    title="Issue title",
                    description="Issue description",
                    suggestion="Fix suggestion",
                ),
                CodeIssue(
                    severity="critical",
                    file_path="src/utils.py",
                    line=25,
                    title="Critical issue",
                    description="Critical description",
                ),
            ],
        )

        comments = service._create_comments(analysis)

        assert len(comments) == 2
        assert comments[0].path == "src/main.py"
        assert comments[0].line == 10
        assert "WARNING" in comments[0].body
        assert "Fix suggestion" in comments[0].body
        assert comments[1].path == "src/utils.py"
        assert "CRITICAL" in comments[1].body

    def test_skip_issues_without_file_or_line(self, service):
        """Test that issues without file or line are skipped."""
        analysis = ReviewAnalysis(
            summary="Test",
            issues=[
                CodeIssue(
                    severity="suggestion",
                    file_path="",  # No file
                    line=10,
                    title="Issue",
                    description="Description",
                ),
                CodeIssue(
                    severity="warning",
                    file_path="test.py",
                    line=None,  # No line
                    title="Issue",
                    description="Description",
                ),
            ],
        )

        comments = service._create_comments(analysis)

        assert len(comments) == 0


# =============================================================================
# Review Event Tests
# =============================================================================


class TestReviewEventDetermination:
    """Test review event type determination."""

    def test_critical_issues_request_changes(self, service):
        """Test that critical issues result in REQUEST_CHANGES."""
        analysis = ReviewAnalysis(
            summary="Test",
            issues=[
                CodeIssue(
                    severity="critical",
                    file_path="test.py",
                    line=1,
                    title="Critical",
                    description="Critical issue",
                )
            ],
        )

        event = service._determine_review_event(analysis)

        assert event == "REQUEST_CHANGES"

    def test_security_concerns_request_changes(self, service):
        """Test that security concerns result in REQUEST_CHANGES."""
        analysis = ReviewAnalysis(
            summary="Test",
            security_concerns=["SQL injection vulnerability"],
        )

        event = service._determine_review_event(analysis)

        assert event == "REQUEST_CHANGES"

    def test_good_quality_approves(self, service):
        """Test that good quality results in APPROVE."""
        analysis = ReviewAnalysis(
            summary="Test",
            overall_quality="good",
        )

        event = service._determine_review_event(analysis)

        assert event == "APPROVE"

    def test_acceptable_quality_comments(self, service):
        """Test that acceptable quality results in COMMENT."""
        analysis = ReviewAnalysis(
            summary="Test",
            overall_quality="acceptable",
        )

        event = service._determine_review_event(analysis)

        assert event == "COMMENT"

    def test_needs_work_quality_comments(self, service):
        """Test that needs_work quality results in COMMENT."""
        analysis = ReviewAnalysis(
            summary="Test",
            overall_quality="needs_work",
        )

        event = service._determine_review_event(analysis)

        assert event == "COMMENT"


# =============================================================================
# Review Body Formatting Tests
# =============================================================================


class TestReviewBodyFormatting:
    """Test review body markdown formatting."""

    def test_format_basic_body(self, service):
        """Test formatting basic review body."""
        analysis = ReviewAnalysis(
            summary="Simple PR that adds a feature",
            overall_quality="good",
        )

        body = service._format_review_body(analysis)

        assert "C4 Automated Code Review" in body
        assert "Simple PR that adds a feature" in body
        assert "good" in body
        assert ":white_check_mark:" in body

    def test_format_body_with_security_concerns(self, service):
        """Test formatting body with security concerns."""
        analysis = ReviewAnalysis(
            summary="PR with issues",
            security_concerns=["SQL injection", "XSS vulnerability"],
        )

        body = service._format_review_body(analysis)

        assert "Security Concerns" in body
        assert "SQL injection" in body
        assert "XSS vulnerability" in body

    def test_format_body_with_issues_summary(self, service):
        """Test formatting body with issues summary."""
        analysis = ReviewAnalysis(
            summary="Test",
            issues=[
                CodeIssue(
                    severity="critical",
                    file_path="a.py",
                    line=1,
                    title="C",
                    description="D",
                ),
                CodeIssue(
                    severity="critical",
                    file_path="b.py",
                    line=2,
                    title="C",
                    description="D",
                ),
                CodeIssue(
                    severity="warning",
                    file_path="c.py",
                    line=3,
                    title="W",
                    description="D",
                ),
                CodeIssue(
                    severity="praise",
                    file_path="d.py",
                    line=4,
                    title="P",
                    description="D",
                ),
            ],
        )

        body = service._format_review_body(analysis)

        assert "Critical: 2" in body
        assert "Warnings: 1" in body
        assert "Praise: 1" in body

    def test_format_body_with_suggestions(self, service):
        """Test formatting body with suggestions."""
        analysis = ReviewAnalysis(
            summary="Test",
            suggestions=["Add type hints", "Improve documentation"],
        )

        body = service._format_review_body(analysis)

        assert "Suggestions" in body
        assert "Add type hints" in body
        assert "Improve documentation" in body


# =============================================================================
# Integration-Style Tests (Mocked LLM)
# =============================================================================


class TestReviewPRFlow:
    """Test complete review PR flow with mocks."""

    @pytest.mark.asyncio
    async def test_review_pr_success(self, service, pr_info, sample_diff, sample_llm_response):
        """Test successful PR review flow."""
        # Setup mocks
        service.github_client.get_pr_diff.return_value = sample_diff
        service.github_client.create_review.return_value = ReviewResult(
            success=True,
            message="Review created",
            review_id=123,
            comments_posted=1,
        )
        service.github_client.add_labels.return_value = True

        # Mock LLM call
        with patch.object(service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = sample_llm_response

            result = await service.review_pr(pr_info)

        assert result.success is True
        service.github_client.get_pr_diff.assert_called_once_with(pr_info)
        service.github_client.create_review.assert_called_once()
        service.github_client.add_labels.assert_called_once()

    @pytest.mark.asyncio
    async def test_review_pr_diff_too_large(self, service, pr_info):
        """Test handling of diff that exceeds size limit."""
        # Create a diff larger than max size
        large_diff = "x" * (service.max_diff_size + 1000)
        service.github_client.get_pr_diff.return_value = large_diff

        result = await service.review_pr(pr_info)

        assert result.success is False
        assert "too large" in result.message.lower()
        service.github_client.create_review.assert_not_called()

    @pytest.mark.asyncio
    async def test_review_pr_no_diff(self, service, pr_info):
        """Test handling when diff retrieval fails."""
        service.github_client.get_pr_diff.return_value = None

        result = await service.review_pr(pr_info)

        assert result.success is False
        assert "diff" in result.message.lower()

    @pytest.mark.asyncio
    async def test_review_pr_no_labels_on_failure(self, service, pr_info, sample_diff, sample_llm_response):
        """Test that labels are not added when review creation fails."""
        service.github_client.get_pr_diff.return_value = sample_diff
        service.github_client.create_review.return_value = ReviewResult(
            success=False,
            message="Failed to create review",
        )

        with patch.object(service, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = sample_llm_response

            result = await service.review_pr(pr_info)

        assert result.success is False
        service.github_client.add_labels.assert_not_called()
