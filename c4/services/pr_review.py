"""PR Review Service - AI-powered code review.

This service analyzes PR diffs using Claude API and generates
structured code review feedback.
"""

from __future__ import annotations

import json
import logging
import re
from dataclasses import dataclass, field

from c4.integrations.github_app import GitHubAppClient, PRInfo, ReviewComment, ReviewResult

logger = logging.getLogger(__name__)


# =============================================================================
# Review Result Types
# =============================================================================


@dataclass
class CodeIssue:
    """A single code issue identified during review."""

    severity: str  # critical, warning, suggestion, praise
    file_path: str
    line: int | None
    title: str
    description: str
    suggestion: str | None = None


@dataclass
class ReviewAnalysis:
    """Complete analysis of a PR."""

    summary: str
    issues: list[CodeIssue] = field(default_factory=list)
    suggestions: list[str] = field(default_factory=list)
    security_concerns: list[str] = field(default_factory=list)
    test_coverage: str | None = None
    overall_quality: str | None = None  # good, acceptable, needs_work
    labels_to_add: list[str] = field(default_factory=list)


# =============================================================================
# Review Prompts
# =============================================================================

REVIEW_SYSTEM_PROMPT = """You are an expert code reviewer. Analyze the provided PR diff and give constructive feedback.

Focus on:
1. Code quality and best practices
2. Potential bugs or logic errors
3. Security vulnerabilities (SQL injection, XSS, etc.)
4. Performance issues
5. Test coverage if test files are included

Be specific and actionable. Reference exact file paths and line numbers when possible.

Respond in JSON format with this structure:
{
  "summary": "Brief 1-2 sentence summary of the changes",
  "overall_quality": "good|acceptable|needs_work",
  "issues": [
    {
      "severity": "critical|warning|suggestion|praise",
      "file_path": "path/to/file.py",
      "line": 42,
      "title": "Issue title",
      "description": "Detailed description",
      "suggestion": "How to fix (optional)"
    }
  ],
  "suggestions": ["General improvement suggestions"],
  "security_concerns": ["Security issues if any"],
  "test_coverage": "Assessment of test coverage",
  "labels": ["bug", "enhancement", "etc - suggest appropriate labels"]
}

Be constructive, not harsh. Praise good code when you see it."""


# =============================================================================
# PR Review Service
# =============================================================================


class PRReviewService:
    """Service for AI-powered PR code review.

    This service coordinates between the GitHub App client and the LLM
    to provide automated code reviews.
    """

    MAX_DIFF_SIZE = 50000  # 50KB default limit

    def __init__(
        self,
        github_client: GitHubAppClient,
        model: str = "claude-sonnet-4-20250514",
        max_diff_size: int | None = None,
    ) -> None:
        """Initialize PR review service.

        Args:
            github_client: GitHub App client for API operations
            model: LLM model to use for review
            max_diff_size: Maximum diff size to process
        """
        self.github_client = github_client
        self.model = model
        self.max_diff_size = max_diff_size or self.MAX_DIFF_SIZE

    async def review_pr(self, pr_info: PRInfo) -> ReviewResult:
        """Review a PR and post comments.

        Args:
            pr_info: PR information from webhook

        Returns:
            ReviewResult with status
        """
        # Get diff
        diff = self.github_client.get_pr_diff(pr_info)
        if not diff:
            return ReviewResult(
                success=False,
                message="Failed to retrieve PR diff",
            )

        # Check size
        if len(diff) > self.max_diff_size:
            logger.warning(f"PR diff too large: {len(diff)} bytes (max: {self.max_diff_size})")
            return ReviewResult(
                success=False,
                message=f"PR diff too large ({len(diff)} bytes). Max: {self.max_diff_size} bytes",
            )

        # Analyze with LLM
        analysis = await self._analyze_diff(diff, pr_info)
        if not analysis:
            return ReviewResult(
                success=False,
                message="Failed to analyze PR diff",
            )

        # Create review comments
        comments = self._create_comments(analysis)

        # Determine review event based on issues
        event = self._determine_review_event(analysis)

        # Post review
        result = self.github_client.create_review(
            pr_info=pr_info,
            body=self._format_review_body(analysis),
            event=event,
            comments=comments,
        )

        # Add labels if suggested
        if result.success and analysis.labels_to_add:
            self.github_client.add_labels(pr_info, analysis.labels_to_add)

        return result

    async def _analyze_diff(self, diff: str, pr_info: PRInfo) -> ReviewAnalysis | None:
        """Analyze diff using LLM.

        Args:
            diff: PR diff content
            pr_info: PR information for context

        Returns:
            ReviewAnalysis or None on error
        """
        user_prompt = f"""Review this pull request:

**Title:** {pr_info.title}
**Author:** {pr_info.author}
**Base:** {pr_info.base_branch} <- **Head:** {pr_info.head_branch}

**Diff:**
```diff
{diff}
```

Provide a thorough code review in JSON format."""

        try:
            response = await self._call_llm(user_prompt)
            return self._parse_analysis(response)
        except Exception as e:
            logger.error(f"Failed to analyze diff: {e}")
            return None

    async def _call_llm(self, prompt: str) -> str:
        """Call LLM API for analysis.

        Uses LiteLLM for provider abstraction.

        Args:
            prompt: User prompt

        Returns:
            LLM response text
        """
        try:
            import litellm

            response = await litellm.acompletion(
                model=self.model,
                messages=[
                    {"role": "system", "content": REVIEW_SYSTEM_PROMPT},
                    {"role": "user", "content": prompt},
                ],
                temperature=0.1,
                max_tokens=4096,
            )

            return response.choices[0].message.content or ""

        except ImportError:
            # Fallback to anthropic client
            return await self._call_anthropic(prompt)

    async def _call_anthropic(self, prompt: str) -> str:
        """Fallback to direct Anthropic API call.

        Args:
            prompt: User prompt

        Returns:
            API response text
        """
        try:
            import anthropic

            client = anthropic.AsyncAnthropic()

            response = await client.messages.create(
                model=self.model,
                max_tokens=4096,
                system=REVIEW_SYSTEM_PROMPT,
                messages=[{"role": "user", "content": prompt}],
            )

            return response.content[0].text if response.content else ""

        except ImportError:
            raise ImportError("Either 'litellm' or 'anthropic' package is required for PR review. Install with: uv add litellm")

    def _parse_analysis(self, response: str) -> ReviewAnalysis | None:
        """Parse LLM response into ReviewAnalysis.

        Args:
            response: Raw LLM response

        Returns:
            Parsed ReviewAnalysis or None
        """
        # Extract JSON from response (may be wrapped in markdown code block)
        json_match = re.search(r"```(?:json)?\s*([\s\S]*?)\s*```", response)
        if json_match:
            json_str = json_match.group(1)
        else:
            # Try parsing entire response as JSON
            json_str = response

        try:
            data = json.loads(json_str)

            issues = []
            for issue_data in data.get("issues", []):
                issues.append(
                    CodeIssue(
                        severity=issue_data.get("severity", "suggestion"),
                        file_path=issue_data.get("file_path", ""),
                        line=issue_data.get("line"),
                        title=issue_data.get("title", ""),
                        description=issue_data.get("description", ""),
                        suggestion=issue_data.get("suggestion"),
                    )
                )

            return ReviewAnalysis(
                summary=data.get("summary", ""),
                issues=issues,
                suggestions=data.get("suggestions", []),
                security_concerns=data.get("security_concerns", []),
                test_coverage=data.get("test_coverage"),
                overall_quality=data.get("overall_quality"),
                labels_to_add=data.get("labels", []),
            )

        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse review JSON: {e}")
            # Create basic analysis from raw response
            return ReviewAnalysis(
                summary=response[:500] if response else "Unable to parse review",
                overall_quality="acceptable",
            )

    def _create_comments(self, analysis: ReviewAnalysis) -> list[ReviewComment]:
        """Convert analysis issues to review comments.

        Args:
            analysis: Review analysis

        Returns:
            List of ReviewComment objects
        """
        comments = []

        for issue in analysis.issues:
            if not issue.file_path or not issue.line:
                continue

            body = f"**{issue.severity.upper()}**: {issue.title}\n\n{issue.description}"
            if issue.suggestion:
                body += f"\n\n**Suggestion:** {issue.suggestion}"

            comments.append(
                ReviewComment(
                    path=issue.file_path,
                    line=issue.line,
                    body=body,
                )
            )

        return comments

    def _determine_review_event(self, analysis: ReviewAnalysis) -> str:
        """Determine review event type based on analysis.

        Args:
            analysis: Review analysis

        Returns:
            Review event: COMMENT, APPROVE, or REQUEST_CHANGES
        """
        # Check for critical issues
        critical_count = sum(1 for i in analysis.issues if i.severity == "critical")

        if critical_count > 0 or analysis.security_concerns:
            return "REQUEST_CHANGES"

        if analysis.overall_quality == "good":
            return "APPROVE"

        return "COMMENT"

    def _format_review_body(self, analysis: ReviewAnalysis) -> str:
        """Format main review body.

        Args:
            analysis: Review analysis

        Returns:
            Formatted review body markdown
        """
        body = f"## C4 Automated Code Review\n\n{analysis.summary}\n\n"

        # Quality badge
        quality_emoji = {
            "good": ":white_check_mark:",
            "acceptable": ":yellow_circle:",
            "needs_work": ":red_circle:",
        }
        emoji = quality_emoji.get(analysis.overall_quality or "acceptable", ":yellow_circle:")
        body += f"**Overall Quality:** {emoji} {analysis.overall_quality or 'N/A'}\n\n"

        # Security concerns
        if analysis.security_concerns:
            body += "### :warning: Security Concerns\n\n"
            for concern in analysis.security_concerns:
                body += f"- {concern}\n"
            body += "\n"

        # Issue summary
        if analysis.issues:
            critical = sum(1 for i in analysis.issues if i.severity == "critical")
            warnings = sum(1 for i in analysis.issues if i.severity == "warning")
            suggestions = sum(1 for i in analysis.issues if i.severity == "suggestion")
            praise = sum(1 for i in analysis.issues if i.severity == "praise")

            body += "### Issues Summary\n\n"
            if critical:
                body += f"- :red_circle: Critical: {critical}\n"
            if warnings:
                body += f"- :yellow_circle: Warnings: {warnings}\n"
            if suggestions:
                body += f"- :blue_circle: Suggestions: {suggestions}\n"
            if praise:
                body += f"- :green_circle: Praise: {praise}\n"
            body += "\n"

        # General suggestions
        if analysis.suggestions:
            body += "### Suggestions\n\n"
            for suggestion in analysis.suggestions:
                body += f"- {suggestion}\n"
            body += "\n"

        # Test coverage
        if analysis.test_coverage:
            body += f"### Test Coverage\n\n{analysis.test_coverage}\n\n"

        body += "---\n*Automated review by C4*"

        return body
