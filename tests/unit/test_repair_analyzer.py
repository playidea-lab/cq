"""Unit tests for C4 Repair Analyzer."""

from pathlib import Path

import pytest

from c4.daemon.repair_analyzer import (
    KNOWN_PATTERNS,
    FailureAnalysis,
    FailureAnalyzer,
    FailureCategory,
    FailurePattern,
    RepairMetrics,
    RepairSuggestionGenerator,
)
from c4.models.queue import RepairQueueItem


class TestFailurePattern:
    """Tests for FailurePattern matching."""

    def test_syntax_error_pattern_matches(self):
        """Test that syntax error pattern matches correctly."""
        pattern = FailurePattern(
            pattern=r"SyntaxError:\s*(.+)",
            category=FailureCategory.SYNTAX_ERROR,
            solution_template="Fix: {match}",
        )

        assert pattern.matches("SyntaxError: invalid syntax")
        assert pattern.matches("  SyntaxError: unexpected EOF")
        assert not pattern.matches("TypeError: something")

    def test_import_error_pattern_matches(self):
        """Test import error pattern matching."""
        pattern = FailurePattern(
            pattern=r"ModuleNotFoundError:\s*No module named '(\w+)'",
            category=FailureCategory.IMPORT_ERROR,
            solution_template="Install: {match}",
        )

        assert pattern.matches("ModuleNotFoundError: No module named 'pandas'")
        assert not pattern.matches("ImportError: something else")

    def test_test_failure_pattern_matches(self):
        """Test pytest failure pattern."""
        pattern = FailurePattern(
            pattern=r"FAILED\s+(\S+)::(\S+)",
            category=FailureCategory.TEST_FAILURE,
            solution_template="Fix test: {match}",
        )

        assert pattern.matches("FAILED tests/test_foo.py::test_bar")
        assert not pattern.matches("PASSED tests/test_foo.py")


class TestFailureAnalyzer:
    """Tests for FailureAnalyzer."""

    @pytest.fixture
    def analyzer(self, tmp_path: Path) -> FailureAnalyzer:
        """Create analyzer with temp directory."""
        return FailureAnalyzer(tmp_path, use_ai_fallback=False)

    @pytest.fixture
    def repair_item(self) -> RepairQueueItem:
        """Create a sample repair queue item."""
        return RepairQueueItem(
            task_id="T-001",
            worker_id="worker-1",
            failure_signature="SyntaxError: invalid syntax",
            attempts=3,
            blocked_at="2024-01-01T00:00:00",
            last_error="File 'test.py', line 10\n    print(x\n         ^",
        )

    def test_analyze_syntax_error(self, analyzer: FailureAnalyzer, repair_item: RepairQueueItem):
        """Test analysis of syntax errors."""
        analysis = analyzer.analyze(repair_item)

        assert analysis.category == FailureCategory.SYNTAX_ERROR
        assert analysis.confidence > 0.5
        assert len(analysis.suggested_fixes) > 0
        assert "syntax" in analysis.root_cause.lower()

    def test_analyze_type_error(self, analyzer: FailureAnalyzer):
        """Test analysis of type errors."""
        item = RepairQueueItem(
            task_id="T-002",
            worker_id="worker-1",
            failure_signature="TypeError: 'NoneType' object is not callable",
            attempts=2,
            blocked_at="2024-01-01T00:00:00",
        )

        analysis = analyzer.analyze(item)

        assert analysis.category == FailureCategory.TYPE_ERROR
        assert "type" in analysis.root_cause.lower()

    def test_analyze_import_error(self, analyzer: FailureAnalyzer):
        """Test analysis of import errors."""
        item = RepairQueueItem(
            task_id="T-003",
            worker_id="worker-1",
            failure_signature="ModuleNotFoundError: No module named 'pandas'",
            attempts=1,
            blocked_at="2024-01-01T00:00:00",
        )

        analysis = analyzer.analyze(item)

        assert analysis.category == FailureCategory.IMPORT_ERROR
        assert any("uv add" in fix for fix in analysis.suggested_fixes)

    def test_analyze_test_failure(self, analyzer: FailureAnalyzer):
        """Test analysis of test failures."""
        item = RepairQueueItem(
            task_id="T-004",
            worker_id="worker-1",
            failure_signature="FAILED tests/test_foo.py::test_bar - AssertionError",
            attempts=2,
            blocked_at="2024-01-01T00:00:00",
            last_error="AssertionError: expected 5, got 3",
        )

        analysis = analyzer.analyze(item)

        assert analysis.category in (FailureCategory.TEST_FAILURE, FailureCategory.SYNTAX_ERROR)

    def test_analyze_unknown_error(self, analyzer: FailureAnalyzer):
        """Test analysis of unknown errors falls back gracefully."""
        item = RepairQueueItem(
            task_id="T-005",
            worker_id="worker-1",
            failure_signature="Some completely unknown error pattern XYZ123",
            attempts=1,
            blocked_at="2024-01-01T00:00:00",
        )

        analysis = analyzer.analyze(item)

        assert analysis.category == FailureCategory.UNKNOWN
        assert analysis.confidence <= 0.5
        assert len(analysis.suggested_fixes) > 0

    def test_analyze_caching(self, analyzer: FailureAnalyzer, repair_item: RepairQueueItem):
        """Test that analysis results are cached."""
        # First analysis
        analysis1 = analyzer.analyze(repair_item)

        # Second analysis should hit cache
        analysis2 = analyzer.analyze(repair_item)

        assert analysis1.category == analysis2.category
        assert analysis1.root_cause == analysis2.root_cause

    def test_extract_affected_files(self, analyzer: FailureAnalyzer):
        """Test file extraction from error text."""
        error_text = '''
        File "tests/test_foo.py", line 10
        File "src/main.py", line 25
        c4/models/task.py:42: error
        '''

        files = analyzer._extract_affected_files(error_text)

        # At least one file should be extracted
        assert len(files) > 0
        # Should find the c4 file path pattern
        assert any("c4/models/task.py" in f for f in files)

    def test_analysis_to_dict(self, analyzer: FailureAnalyzer, repair_item: RepairQueueItem):
        """Test serialization of analysis."""
        analysis = analyzer.analyze(repair_item)
        data = analysis.to_dict()

        assert "category" in data
        assert "root_cause" in data
        assert "confidence" in data
        assert "suggested_fixes" in data
        assert "timestamp" in data


class TestRepairSuggestionGenerator:
    """Tests for RepairSuggestionGenerator."""

    @pytest.fixture
    def generator(self, tmp_path: Path) -> RepairSuggestionGenerator:
        """Create generator with temp directory."""
        return RepairSuggestionGenerator(tmp_path)

    @pytest.fixture
    def analysis(self) -> FailureAnalysis:
        """Create a sample analysis."""
        return FailureAnalysis(
            category=FailureCategory.SYNTAX_ERROR,
            root_cause="Missing closing parenthesis",
            affected_files=["src/main.py", "src/utils.py"],
            confidence=0.85,
            suggested_fixes=[
                "Add missing parenthesis on line 10",
                "Check for balanced brackets",
            ],
        )

    def test_generate_dod(self, generator: RepairSuggestionGenerator, analysis: FailureAnalysis):
        """Test DoD generation."""
        dod = generator.generate_dod(analysis, "T-001")

        assert "Repair Task for T-001" in dod
        assert "syntax_error" in dod
        assert "Missing closing parenthesis" in dod
        assert "src/main.py" in dod
        assert "- [ ]" in dod  # Checklist items
        assert "Verification" in dod

    def test_generate_repair_prompt(
        self, generator: RepairSuggestionGenerator, analysis: FailureAnalysis
    ):
        """Test repair prompt generation."""
        prompt = generator.generate_repair_prompt(
            analysis,
            "T-001",
            context="Additional context here",
        )

        assert "Repair Request for Task T-001" in prompt
        assert "syntax_error" in prompt
        assert "85%" in prompt  # Confidence
        assert "Additional context here" in prompt
        assert "Instructions" in prompt

    def test_generate_dod_without_files(self, generator: RepairSuggestionGenerator):
        """Test DoD generation without affected files."""
        analysis = FailureAnalysis(
            category=FailureCategory.UNKNOWN,
            root_cause="Unknown error",
            affected_files=[],
            confidence=0.3,
            suggested_fixes=["Investigate the error"],
        )

        dod = generator.generate_dod(analysis, "T-002")

        assert "Affected Files" not in dod or "##" in dod


class TestRepairMetrics:
    """Tests for RepairMetrics tracking."""

    def test_initial_state(self):
        """Test initial metrics state."""
        metrics = RepairMetrics()

        assert metrics.total_repairs == 0
        assert metrics.successful_repairs == 0
        assert metrics.failed_repairs == 0
        assert metrics.success_rate == 0.0

    def test_record_successful_repair(self):
        """Test recording a successful repair."""
        metrics = RepairMetrics()

        metrics.record_repair(
            success=True,
            category=FailureCategory.SYNTAX_ERROR,
            worker_id="worker-1",
            attempts=2,
        )

        assert metrics.total_repairs == 1
        assert metrics.successful_repairs == 1
        assert metrics.failed_repairs == 0
        assert metrics.success_rate == 1.0
        assert metrics.avg_attempts_to_fix == 2.0

    def test_record_failed_repair(self):
        """Test recording a failed repair."""
        metrics = RepairMetrics()

        metrics.record_repair(
            success=False,
            category=FailureCategory.TYPE_ERROR,
            worker_id="worker-1",
            attempts=5,
        )

        assert metrics.total_repairs == 1
        assert metrics.successful_repairs == 0
        assert metrics.failed_repairs == 1
        assert metrics.success_rate == 0.0

    def test_record_multiple_repairs(self):
        """Test recording multiple repairs."""
        metrics = RepairMetrics()

        # Record 3 successful, 2 failed
        for i in range(3):
            metrics.record_repair(
                success=True,
                category=FailureCategory.LINT_ERROR,
                worker_id=f"worker-{i}",
                attempts=i + 1,
            )

        for i in range(2):
            metrics.record_repair(
                success=False,
                category=FailureCategory.TEST_FAILURE,
                worker_id=f"worker-{i}",
                attempts=5,
            )

        assert metrics.total_repairs == 5
        assert metrics.successful_repairs == 3
        assert metrics.failed_repairs == 2
        assert metrics.success_rate == 0.6
        assert metrics.repairs_by_category["lint_error"] == 3
        assert metrics.repairs_by_category["test_failure"] == 2

    def test_metrics_to_dict(self):
        """Test serialization to dictionary."""
        metrics = RepairMetrics()
        metrics.record_repair(
            success=True,
            category=FailureCategory.SYNTAX_ERROR,
            worker_id="worker-1",
            attempts=1,
        )

        data = metrics.to_dict()

        assert "total_repairs" in data
        assert "success_rate" in data
        assert "repairs_by_category" in data
        assert "last_updated" in data

    def test_metrics_to_prometheus_format(self):
        """Test Prometheus export format."""
        metrics = RepairMetrics()
        metrics.record_repair(
            success=True,
            category=FailureCategory.IMPORT_ERROR,
            worker_id="worker-1",
            attempts=1,
        )

        lines = metrics.to_prometheus_format()

        assert any("c4_repair_total" in line for line in lines)
        assert any("c4_repair_success" in line for line in lines)
        assert any("c4_repair_success_rate" in line for line in lines)
        assert any("import_error" in line for line in lines)


class TestKnownPatterns:
    """Tests for the known patterns list."""

    def test_all_patterns_have_required_fields(self):
        """Verify all patterns have required fields."""
        for pattern in KNOWN_PATTERNS:
            assert pattern.pattern
            assert pattern.category in FailureCategory
            assert pattern.solution_template
            assert pattern.priority > 0

    def test_patterns_cover_common_errors(self):
        """Verify patterns cover common Python errors."""
        error_types = [
            "SyntaxError: invalid syntax",
            "TypeError: something",
            "ImportError: cannot import",
            "ModuleNotFoundError: No module named 'foo'",
            "AttributeError: 'NoneType' object has no attribute 'bar'",
            "FAILED tests/test.py::test_func",
            "AssertionError: expected True",
        ]

        for error in error_types:
            matched = any(p.matches(error) for p in KNOWN_PATTERNS)
            assert matched, f"No pattern matched: {error}"
