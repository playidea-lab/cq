"""Tests for C4D Validation Runner"""

import json
import tempfile
from pathlib import Path
from unittest.mock import patch, MagicMock
import subprocess

import pytest

from c4d.models import C4Config, ValidationConfig
from c4d.validation import (
    ValidationRunner,
    ValidationRun,
    parse_test_output,
    extract_failure_signature,
)


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        project = Path(tmpdir)
        c4_dir = project / ".c4"
        c4_dir.mkdir()
        (c4_dir / "runs").mkdir()
        (c4_dir / "runs" / "tests").mkdir()
        (c4_dir / "runs" / "logs").mkdir()
        yield project


@pytest.fixture
def config():
    """Create test config with validation commands"""
    return C4Config(
        project_id="test-project",
        validation=ValidationConfig(
            commands={
                "lint": "echo 'lint ok'",
                "unit": "echo 'tests passed'",
                "typecheck": "echo 'types ok'",
            },
            required=["lint", "unit"],
        ),
    )


@pytest.fixture
def runner(temp_project, config):
    """Create a ValidationRunner"""
    return ValidationRunner(temp_project, config)


class TestValidationRun:
    """Test ValidationRun dataclass"""

    def test_passed_property(self):
        """Test passed property based on exit_code"""
        from datetime import datetime

        run_pass = ValidationRun(
            name="test",
            command="echo 'ok'",
            exit_code=0,
            duration_ms=100,
            stdout="ok",
            stderr="",
            timestamp=datetime.now(),
        )
        assert run_pass.passed is True

        run_fail = ValidationRun(
            name="test",
            command="false",
            exit_code=1,
            duration_ms=100,
            stdout="",
            stderr="error",
            timestamp=datetime.now(),
        )
        assert run_fail.passed is False

    def test_to_result(self):
        """Test conversion to ValidationResult"""
        from datetime import datetime

        run = ValidationRun(
            name="lint",
            command="echo 'ok'",
            exit_code=0,
            duration_ms=150,
            stdout="ok",
            stderr="",
            timestamp=datetime.now(),
        )

        result = run.to_result()
        assert result.name == "lint"
        assert result.status == "pass"
        assert result.duration_ms == 150

    def test_to_result_with_error(self):
        """Test conversion with failed result"""
        from datetime import datetime

        run = ValidationRun(
            name="test",
            command="pytest",
            exit_code=1,
            duration_ms=5000,
            stdout="",
            stderr="FAILED tests/test_foo.py::test_bar",
            timestamp=datetime.now(),
        )

        result = run.to_result()
        assert result.name == "test"
        assert result.status == "fail"
        assert "FAILED" in result.message


class TestValidationRunner:
    """Test ValidationRunner class"""

    def test_init(self, runner, temp_project, config):
        """Test runner initialization"""
        assert runner.root == temp_project
        assert runner.config == config
        assert runner.tests_dir == temp_project / ".c4" / "runs" / "tests"
        assert runner.logs_dir == temp_project / ".c4" / "runs" / "logs"

    def test_run_validation_unknown_name(self, runner):
        """Test running unknown validation raises error"""
        with pytest.raises(ValueError) as exc_info:
            runner.run_validation("unknown")
        assert "Unknown validation" in str(exc_info.value)
        assert "lint" in str(exc_info.value)

    @patch("subprocess.run")
    def test_run_validation_success(self, mock_run, runner):
        """Test successful validation run"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="lint ok",
            stderr="",
        )

        result = runner.run_validation("lint")

        assert result.name == "lint"
        assert result.exit_code == 0
        assert result.passed is True
        assert result.stdout == "lint ok"
        mock_run.assert_called_once()

    @patch("subprocess.run")
    def test_run_validation_failure(self, mock_run, runner):
        """Test failed validation run"""
        mock_run.return_value = MagicMock(
            returncode=1,
            stdout="",
            stderr="lint error: missing semicolon",
        )

        result = runner.run_validation("lint")

        assert result.name == "lint"
        assert result.exit_code == 1
        assert result.passed is False
        assert "semicolon" in result.stderr

    @patch("subprocess.run")
    def test_run_validation_timeout(self, mock_run, runner):
        """Test validation timeout"""
        mock_run.side_effect = subprocess.TimeoutExpired("cmd", 5)

        result = runner.run_validation("lint", timeout=5)

        assert result.exit_code == -1
        assert "Timeout" in result.stderr

    @patch("subprocess.run")
    def test_run_validation_exception(self, mock_run, runner):
        """Test validation with general exception"""
        mock_run.side_effect = Exception("Unexpected error")

        result = runner.run_validation("lint")

        assert result.exit_code == -2
        assert "Unexpected error" in result.stderr

    @patch("subprocess.run")
    def test_run_validation_saves_results(self, mock_run, runner, temp_project):
        """Test that validation results are saved"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        runner.run_validation("lint")

        # Check files were created
        test_files = list(runner.tests_dir.glob("*_lint.json"))
        assert len(test_files) == 1

        log_files = list(runner.logs_dir.glob("*_lint.log"))
        assert len(log_files) == 1

        # Check JSON content
        data = json.loads(test_files[0].read_text())
        assert data["name"] == "lint"
        assert data["passed"] is True

    @patch("subprocess.run")
    def test_run_all_required(self, mock_run, runner):
        """Test running all required validations"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        results = runner.run_all_required()

        # Should run lint and unit (required)
        assert len(results) == 2
        assert results[0].name == "lint"
        assert results[1].name == "unit"
        assert mock_run.call_count == 2

    @patch("subprocess.run")
    def test_run_all_required_fail_fast(self, mock_run, runner):
        """Test fail-fast behavior on first failure"""
        # First call succeeds, second fails
        mock_run.side_effect = [
            MagicMock(returncode=0, stdout="ok", stderr=""),
            MagicMock(returncode=1, stdout="", stderr="test failed"),
        ]

        results = runner.run_all_required()

        # Should stop after first failure
        assert len(results) == 2
        assert results[0].passed is True
        assert results[1].passed is False

    @patch("subprocess.run")
    def test_run_validations_specific(self, mock_run, runner):
        """Test running specific validations"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        results = runner.run_validations(["lint", "typecheck"])

        assert len(results) == 2
        assert results[0].name == "lint"
        assert results[1].name == "typecheck"

    @patch("subprocess.run")
    def test_run_validations_no_fail_fast(self, mock_run, runner):
        """Test running validations without fail-fast"""
        mock_run.side_effect = [
            MagicMock(returncode=1, stdout="", stderr="lint error"),
            MagicMock(returncode=0, stdout="ok", stderr=""),
        ]

        results = runner.run_validations(["lint", "unit"], fail_fast=False)

        # Should continue despite first failure
        assert len(results) == 2
        assert results[0].passed is False
        assert results[1].passed is True

    @patch("subprocess.run")
    def test_get_last_results(self, mock_run, runner):
        """Test getting last validation results"""
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="ok",
            stderr="",
        )

        # Run some validations
        runner.run_validation("lint")
        runner.run_validation("unit")

        # Get results
        results = runner.get_last_results()

        assert "lint" in results
        assert results["lint"] == "pass"
        assert "unit" in results
        assert results["unit"] == "pass"


class TestParseTestOutput:
    """Test parse_test_output function"""

    def test_parse_pytest_output(self):
        """Test parsing pytest output"""
        stdout = """
        ============================= test session starts ==============================
        collected 10 items

        tests/test_foo.py::test_one PASSED
        tests/test_foo.py::test_two PASSED
        tests/test_foo.py::test_three FAILED
        tests/test_bar.py::test_four PASSED

        ============================== 3 passed, 1 failed in 1.23s ==============================
        """

        info = parse_test_output(stdout, "pytest")

        assert info["passed"] == 3
        assert info["failed"] == 1

    def test_parse_pytest_with_coverage(self):
        """Test parsing pytest output with coverage"""
        stdout = """
        Name                 Stmts   Miss  Cover
        ----------------------------------------
        module/foo.py          100     10    90%
        module/bar.py           50      5    90%
        ----------------------------------------
        TOTAL                  150     15    90%
        """

        info = parse_test_output(stdout, "pytest")
        assert info["coverage"] == 90

    def test_parse_pytest_with_skipped(self):
        """Test parsing pytest output with skipped tests"""
        stdout = "5 passed, 2 failed, 3 skipped in 2.00s"

        info = parse_test_output(stdout, "pytest")

        assert info["passed"] == 5
        assert info["failed"] == 2
        assert info["skipped"] == 3

    def test_parse_jest_output(self):
        """Test parsing Jest output"""
        stdout = """
        Test Suites: 2 passed, 2 total
        Tests: 10 passed, 2 total
        """

        info = parse_test_output(stdout, "jest")
        assert info["passed"] == 10


class TestExtractFailureSignature:
    """Test extract_failure_signature function"""

    def test_extract_type_error(self):
        """Test extracting TypeError signature"""
        stderr = "TypeError: 'NoneType' object is not subscriptable"

        sig = extract_failure_signature(stderr, "")
        assert sig is not None
        assert "TypeError" in sig
        assert "NoneType" in sig

    def test_extract_assertion_error(self):
        """Test extracting AssertionError signature"""
        stderr = "AssertionError: Expected 5 but got 3"

        sig = extract_failure_signature(stderr, "")
        assert sig is not None
        assert "AssertionError" in sig

    def test_extract_failed_test(self):
        """Test extracting FAILED test signature"""
        stdout = "FAILED tests/test_foo.py::test_bar - AssertionError"

        sig = extract_failure_signature("", stdout)
        assert sig is not None
        assert "FAILED" in sig

    def test_no_signature_found(self):
        """Test when no signature pattern matches"""
        stdout = "All tests passed successfully"
        stderr = ""

        sig = extract_failure_signature(stderr, stdout)
        assert sig is None

    def test_signature_truncation(self):
        """Test that long signatures are truncated"""
        stderr = "Error: " + "x" * 300

        sig = extract_failure_signature(stderr, "")
        assert sig is not None
        assert len(sig) <= 200
