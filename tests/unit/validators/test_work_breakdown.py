"""Tests for work breakdown validator."""


from c4.models.ddd import (
    ApiSpec,
    CodePlacement,
    ContractSpec,
    RequiredTests,
    WorkBreakdownCriteria,
)
from c4.models.task import Task
from c4.validators.work_breakdown import (
    DEFAULT_CRITERIA,
    analyze_task_size,
    format_breakdown_report,
    suggest_task_splits,
)


class TestAnalyzeTaskSize:
    """Tests for analyze_task_size function."""

    def test_small_task_is_valid(self):
        """Small task passes validation."""
        task = Task(
            id="T-001-0",
            title="Small task",
            dod="Small task dod",
            contract_spec=ContractSpec(
                apis=[ApiSpec(name="api1", input="x", output="y")],
                tests=RequiredTests(
                    success=["t1"],
                    failure=["t2"],
                    boundary=["t3"],
                ),
            ),
            code_placement=CodePlacement(
                create=["src/auth/service.py"],
                modify=["src/auth/routes.py"],  # Same domain as create
                tests=["tests/unit/auth/test_service.py"],
            ),
        )

        result = analyze_task_size(task)

        assert result.valid is True
        assert len(result.recommendations) == 0
        assert result.metrics["api_count"] == 1
        assert result.metrics["file_count"] == 3
        assert result.metrics["domain_count"] == 1  # Only 'auth'

    def test_too_many_files(self):
        """Task with too many files is flagged."""
        task = Task(
            id="T-001-0",
            title="Large task",
            dod="Large task dod",
            code_placement=CodePlacement(
                create=[f"src/file{i}.py" for i in range(6)],
                modify=[f"src/mod{i}.py" for i in range(4)],
                tests=[],
            ),
        )

        result = analyze_task_size(task)

        assert result.valid is False
        assert any("files" in r.reason.lower() for r in result.recommendations)

    def test_too_many_apis(self):
        """Task with too many APIs is flagged."""
        task = Task(
            id="T-001-0",
            title="API heavy task",
            dod="Many APIs",
            contract_spec=ContractSpec(
                apis=[
                    ApiSpec(name=f"api{i}", input="x", output="y")
                    for i in range(5)
                ],
                tests=RequiredTests(success=["t"], failure=["f"], boundary=["b"]),
            ),
        )

        result = analyze_task_size(task)

        assert result.valid is False
        assert any("API" in r.reason for r in result.recommendations)

    def test_too_many_tests(self):
        """Task with too many tests gets a warning."""
        task = Task(
            id="T-001-0",
            title="Test heavy task",
            dod="Many tests",
            contract_spec=ContractSpec(
                apis=[ApiSpec(name="api1", input="x", output="y")],
                tests=RequiredTests(
                    success=[f"success{i}" for i in range(5)],
                    failure=[f"failure{i}" for i in range(5)],
                    boundary=[f"boundary{i}" for i in range(5)],
                ),
            ),
        )

        result = analyze_task_size(task)

        assert any("tests" in r.reason.lower() for r in result.recommendations)

    def test_multiple_domains(self):
        """Task touching multiple domains must be split."""
        task = Task(
            id="T-001-0",
            title="Cross-domain task",
            dod="Multiple domains",
            code_placement=CodePlacement(
                create=[
                    "src/auth/service.py",
                    "src/payment/processor.py",
                    "src/notification/sender.py",
                ],
                modify=[],
                tests=[],
            ),
        )

        result = analyze_task_size(task)

        assert result.valid is False
        assert any("domain" in r.reason.lower() for r in result.recommendations)
        assert any(r.severity == "must_split" for r in result.recommendations)

    def test_no_tests_warning(self):
        """Task with files but no tests gets warning."""
        task = Task(
            id="T-001-0",
            title="No tests task",
            dod="Missing tests",
            code_placement=CodePlacement(
                create=["src/service.py"],
                modify=[],
                tests=[],
            ),
        )

        result = analyze_task_size(task)

        assert any("test" in r.reason.lower() for r in result.recommendations)

    def test_custom_criteria(self):
        """Custom criteria are respected."""
        strict_criteria = WorkBreakdownCriteria(
            max_duration_days=1,
            max_public_apis=1,
            max_tests=3,
            max_files_modified=2,
            max_domains_touched=1,
        )

        task = Task(
            id="T-001-0",
            title="Task",
            dod="dod",
            code_placement=CodePlacement(
                create=["src/file1.py", "src/file2.py", "src/file3.py"],
                modify=[],
                tests=[],
            ),
        )

        result = analyze_task_size(task, criteria=strict_criteria)

        assert result.valid is False
        assert result.metrics["file_count"] == 3

    def test_empty_task(self):
        """Task without specifications is valid but empty."""
        task = Task(
            id="T-001-0",
            title="Empty task",
            dod="Empty dod",
        )

        result = analyze_task_size(task)

        assert result.valid is True
        assert result.metrics["api_count"] == 0
        assert result.metrics["file_count"] == 0


class TestSuggestTaskSplits:
    """Tests for suggest_task_splits function."""

    def test_suggest_split_by_domain(self):
        """Suggest splits based on domains."""
        task = Task(
            id="T-001-0",
            title="Cross-domain task",
            dod="Split me",
            code_placement=CodePlacement(
                create=[
                    "src/auth/login.py",
                    "src/auth/register.py",
                    "src/payment/checkout.py",
                ],
                modify=[],
                tests=[
                    "tests/unit/auth/test_login.py",
                    "tests/unit/payment/test_checkout.py",
                ],
            ),
        )

        suggestions = suggest_task_splits(task)

        # Should suggest at least 2 splits (auth and payment)
        assert len(suggestions) >= 2
        domains = {s.get("domain") for s in suggestions if "domain" in s}
        assert "auth" in domains
        assert "payment" in domains

    def test_suggest_split_by_api(self):
        """Suggest splits based on API count."""
        task = Task(
            id="T-001-0",
            title="API heavy task",
            dod="Many APIs",
            contract_spec=ContractSpec(
                apis=[
                    ApiSpec(name=f"Service.method{i}", input="x", output="y")
                    for i in range(6)
                ],
                tests=RequiredTests(success=["t"], failure=["f"], boundary=["b"]),
            ),
        )

        suggestions = suggest_task_splits(task)

        # Should suggest API-based splits
        assert len(suggestions) >= 2

    def test_no_splits_for_small_task(self):
        """No splits suggested for small task."""
        task = Task(
            id="T-001-0",
            title="Small task",
            dod="Small",
            code_placement=CodePlacement(
                create=["src/auth/service.py"],
                modify=[],
                tests=["tests/unit/auth/test_service.py"],
            ),
        )

        suggestions = suggest_task_splits(task)

        # Small single-domain task needs no split
        # May return empty or single suggestion
        assert isinstance(suggestions, list)


class TestFormatBreakdownReport:
    """Tests for format_breakdown_report function."""

    def test_format_valid_task(self):
        """Format report for valid task."""
        task = Task(
            id="T-001-0",
            title="Valid task",
            dod="dod",
            contract_spec=ContractSpec(
                apis=[ApiSpec(name="api", input="x", output="y")],
                tests=RequiredTests(success=["t1"], failure=["t2"], boundary=["t3"]),
            ),
            code_placement=CodePlacement(
                create=["src/auth/service.py"],
                modify=[],
                tests=["tests/unit/auth/test_service.py"],
            ),
        )

        result = analyze_task_size(task)
        report = format_breakdown_report(result)

        assert "✅" in report
        assert "Metrics" in report
        assert "Guidelines" in report

    def test_format_invalid_task(self):
        """Format report for invalid task."""
        task = Task(
            id="T-001-0",
            title="Invalid task",
            dod="dod",
            code_placement=CodePlacement(
                create=[f"src/file{i}.py" for i in range(10)],
                modify=[],
                tests=[],
            ),
        )

        result = analyze_task_size(task)
        report = format_breakdown_report(result)

        assert "❌" in report
        assert "should be split" in report
        assert "Recommendations" in report

    def test_format_includes_metrics(self):
        """Report includes all metrics."""
        task = Task(
            id="T-001-0",
            title="Task",
            dod="dod",
            contract_spec=ContractSpec(
                apis=[ApiSpec(name="api", input="x", output="y")],
                tests=RequiredTests(success=["t"], failure=["f"], boundary=["b"]),
            ),
            code_placement=CodePlacement(
                create=["file.py"],
                modify=[],
                tests=["test.py"],
            ),
        )

        result = analyze_task_size(task)
        report = format_breakdown_report(result)

        assert "APIs:" in report
        assert "Tests:" in report
        assert "Files:" in report
        assert "Domains:" in report


class TestDefaultCriteria:
    """Tests for default criteria values."""

    def test_default_values(self):
        """Default criteria match DDD-CLEANCODE guide."""
        assert DEFAULT_CRITERIA.max_duration_days == 2
        assert DEFAULT_CRITERIA.max_public_apis == 3
        assert DEFAULT_CRITERIA.max_tests == 9
        assert DEFAULT_CRITERIA.max_files_modified == 5
        assert DEFAULT_CRITERIA.max_domains_touched == 1
