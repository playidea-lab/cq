"""Work Breakdown Validator - Ensure tasks are appropriately sized.

From DDD-CLEANCODE guide:
- Max 1-2 days duration
- Max 1-3 public APIs
- Max 3-9 tests
- Max 5 files modified
- Max 1 domain touched

Tasks exceeding these limits should be split.
"""

from typing import NamedTuple

from c4.models.ddd import CodePlacement, ContractSpec, WorkBreakdownCriteria
from c4.models.task import Task


class SplitRecommendation(NamedTuple):
    """Recommendation to split a task."""

    reason: str
    severity: str  # "must_split", "should_split", "consider_split"
    suggestion: str


class WorkBreakdownResult(NamedTuple):
    """Result of work breakdown analysis."""

    valid: bool
    recommendations: list[SplitRecommendation]
    metrics: dict[str, int]


# Default criteria from DDD-CLEANCODE guide
DEFAULT_CRITERIA = WorkBreakdownCriteria(
    max_duration_days=2,
    max_public_apis=3,
    max_tests=9,
    max_files_modified=5,
    max_domains_touched=1,
)


def _count_apis(contract_spec: ContractSpec | None) -> int:
    """Count public APIs in contract spec."""
    if not contract_spec or not contract_spec.apis:
        return 0
    return len(contract_spec.apis)


def _count_tests(contract_spec: ContractSpec | None) -> int:
    """Count required tests in contract spec."""
    if not contract_spec or not contract_spec.tests:
        return 0
    tests = contract_spec.tests
    return len(tests.success) + len(tests.failure) + len(tests.boundary)


def _count_files(code_placement: CodePlacement | None) -> int:
    """Count files to be modified/created."""
    if not code_placement:
        return 0
    return (
        len(code_placement.create)
        + len(code_placement.modify)
        + len(code_placement.tests)
    )


def _extract_domains(code_placement: CodePlacement | None) -> set[str]:
    """Extract domains from file paths.

    Recognizes common patterns:
    - src/{domain}/...
    - services/{domain}/...
    - tests/unit/{domain}/... (extracts domain, not "unit")
    """
    domains = set()
    if not code_placement:
        return domains

    # Folders to skip (not domains)
    SKIP_FOLDERS = {
        "src",
        "services",
        "lib",
        "app",
        "tests",
        "test",
        "unit",
        "integration",
        "e2e",
        "functional",
        "conftest",
        "",
    }

    # Only extract from implementation files, not test files
    # This prevents test path structure from affecting domain count
    impl_files = code_placement.create + code_placement.modify

    for file_path in impl_files:
        parts = file_path.replace("\\", "/").split("/")
        for i, part in enumerate(parts):
            # Skip common non-domain folders
            if part in SKIP_FOLDERS:
                continue
            # Skip file names (have extension)
            if "." in part:
                continue
            # Found a domain candidate
            domains.add(part)
            break

    return domains


def analyze_task_size(
    task: Task,
    criteria: WorkBreakdownCriteria | None = None,
) -> WorkBreakdownResult:
    """Analyze if task size is appropriate.

    Args:
        task: Task to analyze
        criteria: Size criteria (defaults to DDD-CLEANCODE recommendations)

    Returns:
        WorkBreakdownResult with recommendations
    """
    if criteria is None:
        criteria = DEFAULT_CRITERIA

    recommendations = []

    # Calculate metrics
    api_count = _count_apis(task.contract_spec)
    test_count = _count_tests(task.contract_spec)
    file_count = _count_files(task.code_placement)
    domains = _extract_domains(task.code_placement)
    domain_count = len(domains)

    metrics = {
        "api_count": api_count,
        "test_count": test_count,
        "file_count": file_count,
        "domain_count": domain_count,
    }

    # Check each criterion
    if file_count > criteria.max_files_modified:
        severity = "must_split" if file_count > criteria.max_files_modified * 2 else "should_split"
        recommendations.append(
            SplitRecommendation(
                reason=f"Too many files: {file_count} > {criteria.max_files_modified}",
                severity=severity,
                suggestion="Split by feature or layer. Group related files into separate tasks.",
            )
        )

    if api_count > criteria.max_public_apis:
        severity = "must_split" if api_count > criteria.max_public_apis * 2 else "should_split"
        recommendations.append(
            SplitRecommendation(
                reason=f"Too many APIs: {api_count} > {criteria.max_public_apis}",
                severity=severity,
                suggestion="Split by API. Each task should implement 1-3 related APIs.",
            )
        )

    if test_count > criteria.max_tests:
        severity = "should_split" if test_count > criteria.max_tests else "consider_split"
        recommendations.append(
            SplitRecommendation(
                reason=f"Too many tests: {test_count} > {criteria.max_tests}",
                severity=severity,
                suggestion="This often indicates task is too large. Consider splitting by feature.",
            )
        )

    if domain_count > criteria.max_domains_touched:
        recommendations.append(
            SplitRecommendation(
                reason=f"Crosses {domain_count} domains: {', '.join(sorted(domains))}",
                severity="must_split",
                suggestion="Single task should touch single domain. Create separate tasks for each domain.",
            )
        )

    # Additional heuristics
    if file_count > 0 and test_count == 0:
        recommendations.append(
            SplitRecommendation(
                reason="No tests specified",
                severity="consider_split",
                suggestion="Every task should include tests. Add ContractSpec.tests.",
            )
        )

    # Task is invalid if any "must_split" or "should_split" recommendations exist
    has_blocking = any(
        r.severity in ("must_split", "should_split") for r in recommendations
    )
    valid = not has_blocking

    return WorkBreakdownResult(
        valid=valid,
        recommendations=recommendations,
        metrics=metrics,
    )


def suggest_task_splits(task: Task) -> list[dict]:
    """Suggest how to split an oversized task.

    Returns:
        List of suggested sub-task specifications
    """
    suggestions = []

    # Strategy 1: Split by file groups (requires code_placement)
    if task.code_placement:
        domains = _extract_domains(task.code_placement)

        if len(domains) > 1:
            for domain in sorted(domains):
                domain_files = [
                    f for f in task.code_placement.create + task.code_placement.modify
                    if domain in f
                ]
                domain_tests = [
                    f for f in task.code_placement.tests
                    if domain in f
                ]

                if domain_files or domain_tests:
                    suggestions.append({
                        "title": f"{task.title} - {domain} domain",
                        "domain": domain,
                        "files_create": [f for f in task.code_placement.create if domain in f],
                        "files_modify": [f for f in task.code_placement.modify if domain in f],
                        "test_files": domain_tests,
                    })

    # Strategy 2: Split by API (can work without code_placement)
    if task.contract_spec and len(task.contract_spec.apis) > 3:
        api_groups: list[list] = []
        current_group: list = []

        for api in task.contract_spec.apis:
            current_group.append(api)
            if len(current_group) >= 2:  # 2 APIs per task
                api_groups.append(current_group)
                current_group = []

        if current_group:
            api_groups.append(current_group)

        for i, group in enumerate(api_groups, 1):
            api_names = [a.name for a in group]
            suggestions.append({
                "title": f"{task.title} - Part {i}",
                "apis": api_names,
                "description": f"Implement APIs: {', '.join(api_names)}",
            })

    return suggestions


def format_breakdown_report(result: WorkBreakdownResult) -> str:
    """Format work breakdown analysis into readable report."""
    lines = []

    # Header
    if result.valid:
        lines.append("✅ Task size is appropriate")
    else:
        lines.append("❌ Task should be split")

    lines.append("")

    # Metrics
    lines.append("📊 Metrics:")
    lines.append(f"   • APIs: {result.metrics['api_count']}")
    lines.append(f"   • Tests: {result.metrics['test_count']}")
    lines.append(f"   • Files: {result.metrics['file_count']}")
    lines.append(f"   • Domains: {result.metrics['domain_count']}")
    lines.append("")

    # Recommendations
    if result.recommendations:
        lines.append("📋 Recommendations:")
        for rec in result.recommendations:
            icon = "🔴" if rec.severity == "must_split" else "🟡" if rec.severity == "should_split" else "🔵"
            lines.append(f"   {icon} {rec.reason}")
            lines.append(f"      └─ {rec.suggestion}")
        lines.append("")

    # Guidelines
    lines.append("📖 DDD-CLEANCODE Guidelines:")
    lines.append(f"   • Max files: {DEFAULT_CRITERIA.max_files_modified}")
    lines.append(f"   • Max APIs: {DEFAULT_CRITERIA.max_public_apis}")
    lines.append(f"   • Max tests: {DEFAULT_CRITERIA.max_tests}")
    lines.append(f"   • Max domains: {DEFAULT_CRITERIA.max_domains_touched}")

    return "\n".join(lines)
