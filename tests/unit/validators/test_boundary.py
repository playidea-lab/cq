"""Tests for boundary validator."""

from pathlib import Path

from c4.models.ddd import BoundaryMap
from c4.validators.boundary import (
    ImportViolation,
    format_violations_report,
    validate_boundary,
    validate_file_imports,
)


class TestValidateFileImports:
    """Tests for validate_file_imports function."""

    def test_allowed_stdlib_import(self, tmp_path: Path):
        """Standard library imports are allowed."""
        test_file = tmp_path / "test.py"
        test_file.write_text("""
import os
import sys
from pathlib import Path
from typing import Optional
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib"],
            forbidden_imports=[],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 0

    def test_allowed_pydantic_import(self, tmp_path: Path):
        """Pydantic imports are allowed when specified."""
        test_file = tmp_path / "test.py"
        test_file.write_text("""
from pydantic import BaseModel
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib", "pydantic"],
            forbidden_imports=[],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 0

    def test_forbidden_import_detected(self, tmp_path: Path):
        """Forbidden imports are detected."""
        test_file = tmp_path / "test.py"
        test_file.write_text("""
import sqlalchemy
from httpx import Client
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="domain",
            allowed_imports=["stdlib"],
            forbidden_imports=["sqlalchemy", "httpx"],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 2
        assert any("sqlalchemy" in v.module for v in violations)
        assert any("httpx" in v.module for v in violations)

    def test_unlisted_import_rejected(self, tmp_path: Path):
        """Imports not in allowed list are rejected."""
        test_file = tmp_path / "test.py"
        test_file.write_text("""
import requests
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="domain",
            allowed_imports=["stdlib"],
            forbidden_imports=[],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 1
        assert "not in allowed list" in violations[0].reason

    def test_custom_allowed_import(self, tmp_path: Path):
        """Custom allowed imports work."""
        test_file = tmp_path / "test.py"
        test_file.write_text("""
from myapp.domain.user import User
from myapp.domain.common import BaseEntity
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib", "myapp.domain"],
            forbidden_imports=[],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 0

    def test_nonexistent_file(self, tmp_path: Path):
        """Nonexistent file returns empty violations."""
        nonexistent = tmp_path / "nonexistent.py"
        boundary = BoundaryMap(target_domain="test", target_layer="app")

        violations = validate_file_imports(nonexistent, boundary)
        assert len(violations) == 0

    def test_non_python_file(self, tmp_path: Path):
        """Non-Python files are skipped."""
        test_file = tmp_path / "test.txt"
        test_file.write_text("import sqlalchemy")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="domain",
            forbidden_imports=["sqlalchemy"],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 0


class TestValidateBoundary:
    """Tests for validate_boundary function."""

    def test_validate_multiple_files(self, tmp_path: Path):
        """Validate multiple files at once."""
        (tmp_path / "file1.py").write_text("import os")
        (tmp_path / "file2.py").write_text("import requests")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib"],
            forbidden_imports=[],
        )

        files = list(tmp_path.glob("*.py"))
        result = validate_boundary(files, boundary)

        # file1.py should pass, file2.py should fail
        assert result.valid is False
        assert len(result.violations) == 1

    def test_all_files_pass(self, tmp_path: Path):
        """All files passing returns valid=True."""
        (tmp_path / "file1.py").write_text("import os")
        (tmp_path / "file2.py").write_text("import sys")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib"],
        )

        files = list(tmp_path.glob("*.py"))
        result = validate_boundary(files, boundary)

        assert result.valid is True
        assert len(result.violations) == 0

    def test_warning_for_no_public_export(self, tmp_path: Path):
        """Warning when public_export not defined."""
        (tmp_path / "test.py").write_text("import os")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="app",
            allowed_imports=["stdlib"],
            public_export=None,  # Not defined
        )

        files = list(tmp_path.glob("*.py"))
        result = validate_boundary(files, boundary)

        assert len(result.warnings) > 0
        assert any("public_export" in w for w in result.warnings)


class TestFormatViolationsReport:
    """Tests for format_violations_report function."""

    def test_format_no_violations(self):
        """Format empty violations list."""
        report = format_violations_report([])
        assert "✅" in report
        assert "No boundary violations" in report

    def test_format_with_violations(self):
        """Format violations into report."""
        violations = [
            ImportViolation(
                file="src/auth/service.py",
                line=5,
                module="sqlalchemy",
                reason="Forbidden import",
            ),
            ImportViolation(
                file="src/auth/service.py",
                line=10,
                module="httpx",
                reason="Forbidden import",
            ),
        ]

        report = format_violations_report(violations)

        assert "❌" in report
        assert "2 boundary violations" in report
        assert "src/auth/service.py" in report
        assert "sqlalchemy" in report
        assert "httpx" in report

    def test_format_groups_by_file(self):
        """Violations are grouped by file."""
        violations = [
            ImportViolation(
                file="file1.py",
                line=1,
                module="mod1",
                reason="reason1",
            ),
            ImportViolation(
                file="file2.py",
                line=2,
                module="mod2",
                reason="reason2",
            ),
            ImportViolation(
                file="file1.py",
                line=3,
                module="mod3",
                reason="reason3",
            ),
        ]

        report = format_violations_report(violations)

        # File1 should appear before its violations
        file1_idx = report.find("file1.py")
        mod1_idx = report.find("mod1")
        mod3_idx = report.find("mod3")

        assert file1_idx < mod1_idx
        assert file1_idx < mod3_idx


class TestEdgeCases:
    """Edge case tests."""

    def test_syntax_error_in_file(self, tmp_path: Path):
        """Handle syntax errors gracefully."""
        test_file = tmp_path / "broken.py"
        test_file.write_text("def broken(:\n    pass")

        boundary = BoundaryMap(target_domain="test", target_layer="app")
        violations = validate_file_imports(test_file, boundary)

        # Should have a parse error violation
        assert len(violations) == 1
        assert "parse" in violations[0].reason.lower()

    def test_empty_file(self, tmp_path: Path):
        """Handle empty Python file."""
        test_file = tmp_path / "empty.py"
        test_file.write_text("")

        boundary = BoundaryMap(target_domain="test", target_layer="app")
        violations = validate_file_imports(test_file, boundary)

        assert len(violations) == 0

    def test_comments_only_file(self, tmp_path: Path):
        """Handle file with only comments."""
        test_file = tmp_path / "comments.py"
        test_file.write_text("""
# This is a comment
# import sqlalchemy  # This should not be detected
'''"import requests"'''
""")

        boundary = BoundaryMap(
            target_domain="test",
            target_layer="domain",
            forbidden_imports=["sqlalchemy", "requests"],
        )

        violations = validate_file_imports(test_file, boundary)
        assert len(violations) == 0  # Comments and strings are not real imports
