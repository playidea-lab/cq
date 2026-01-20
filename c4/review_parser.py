"""C4 Review Parser - Parse code review reports and extract issues for task creation"""

import re
from dataclasses import dataclass
from pathlib import Path


@dataclass
class ReviewIssue:
    """Represents a single issue from a code review"""

    id: str  # e.g., "C-001", "H-003"
    severity: str  # "Critical", "High", "Medium", "Low"
    title: str  # Issue title
    file: str | None = None  # Affected file path
    description: str | None = None  # Optional description


# Severity ordering (lower index = higher priority)
SEVERITY_ORDER = ["Critical", "High", "Medium", "Low"]


def parse_review_report(report_path: Path) -> list[ReviewIssue]:
    """
    Parse a code review report (markdown format) and extract issues.

    Expected format:
    ## Critical Issues
    ### [C-001] Issue title here
    - **파일**: `path/to/file.py:line`
    - **설명**: Description text

    ## High Issues
    ### [H-001] Another issue
    ...

    Args:
        report_path: Path to the review-report.md file

    Returns:
        List of ReviewIssue objects
    """
    if not report_path.exists():
        return []

    content = report_path.read_text(encoding="utf-8")
    return parse_review_content(content)


def parse_review_content(content: str) -> list[ReviewIssue]:
    """
    Parse review content string and extract issues.

    Args:
        content: Markdown content of the review report

    Returns:
        List of ReviewIssue objects
    """
    issues: list[ReviewIssue] = []
    current_severity: str | None = None

    # Patterns for parsing
    severity_pattern = re.compile(r"^##\s+(Critical|High|Medium|Low)\s+Issues?", re.IGNORECASE)
    issue_pattern = re.compile(r"^###\s+\[([A-Z]-\d+)\]\s+(.+)$")
    file_pattern = re.compile(r"^\s*-\s+\*\*파일\*\*:\s*`(.+?)`")
    description_pattern = re.compile(r"^\s*-\s+\*\*설명\*\*:\s*(.+)")

    current_issue: dict | None = None

    for line in content.split("\n"):
        # Check for severity section
        severity_match = severity_pattern.match(line)
        if severity_match:
            # Save previous issue if exists
            if current_issue:
                issues.append(_dict_to_issue(current_issue))
                current_issue = None

            current_severity = severity_match.group(1).capitalize()
            continue

        # Check for issue header
        issue_match = issue_pattern.match(line)
        if issue_match and current_severity:
            # Save previous issue if exists
            if current_issue:
                issues.append(_dict_to_issue(current_issue))

            current_issue = {
                "id": issue_match.group(1),
                "severity": current_severity,
                "title": issue_match.group(2).strip(),
                "file": None,
                "description": None,
            }
            continue

        # Check for file info (only if we have a current issue)
        if current_issue:
            file_match = file_pattern.match(line)
            if file_match:
                current_issue["file"] = file_match.group(1)
                continue

            desc_match = description_pattern.match(line)
            if desc_match:
                current_issue["description"] = desc_match.group(1)
                continue

    # Don't forget the last issue
    if current_issue:
        issues.append(_dict_to_issue(current_issue))

    return issues


def _dict_to_issue(d: dict) -> ReviewIssue:
    """Convert a dictionary to ReviewIssue"""
    return ReviewIssue(
        id=d["id"],
        severity=d["severity"],
        title=d["title"],
        file=d.get("file"),
        description=d.get("description"),
    )


def filter_issues_by_severity(
    issues: list[ReviewIssue],
    min_severity: str = "High",
) -> list[ReviewIssue]:
    """
    Filter issues by minimum severity level.

    Args:
        issues: List of ReviewIssue objects
        min_severity: Minimum severity to include ("Critical", "High", "Medium", "Low")

    Returns:
        Filtered list of issues with severity >= min_severity
    """
    if min_severity not in SEVERITY_ORDER:
        raise ValueError(f"Invalid severity: {min_severity}. Must be one of {SEVERITY_ORDER}")

    threshold = SEVERITY_ORDER.index(min_severity)
    return [issue for issue in issues if SEVERITY_ORDER.index(issue.severity) <= threshold]


def issues_to_task_titles(
    issues: list[ReviewIssue],
    min_severity: str = "High",
) -> list[str]:
    """
    Convert issues to task title strings for c4_add_todo.

    Args:
        issues: List of ReviewIssue objects
        min_severity: Minimum severity to include

    Returns:
        List of task title strings in format "[Severity] Title"
    """
    filtered = filter_issues_by_severity(issues, min_severity)
    return [f"[{issue.severity}] {issue.title}" for issue in filtered]


def issues_to_required_changes(
    issues: list[ReviewIssue],
    min_severity: str = "High",
) -> list[str]:
    """
    Convert issues to required_changes format for c4_checkpoint REQUEST_CHANGES.

    Args:
        issues: List of ReviewIssue objects
        min_severity: Minimum severity to include

    Returns:
        List of change descriptions
    """
    filtered = filter_issues_by_severity(issues, min_severity)
    changes = []

    for issue in filtered:
        change = f"[{issue.severity}] {issue.title}"
        if issue.file:
            change += f" ({issue.file})"
        changes.append(change)

    return changes
