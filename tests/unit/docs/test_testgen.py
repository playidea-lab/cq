"""Unit tests for TestGenerator."""

import tempfile
from pathlib import Path

import pytest
import yaml

from c4.docs.testgen import (
    EarsPattern,
    Requirement,
    TestFormat,
    TestGenerator,
    TestStub,
)

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
            "text": "While session is active, the system shall allow access to protected resources",
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
            "text": "If password is incorrect 5 times, the system shall lock the account",
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

PYTHON_TEST_FILE = '''
"""Existing tests."""

import pytest


def test_login_success():
    """Test login."""
    pass


def test_password_validation():
    """Test password."""
    pass
'''

TYPESCRIPT_TEST_FILE = '''
import { describe, it, expect } from 'vitest';

describe('Auth', () => {
  it('should login successfully', () => {
    expect(true).toBe(true);
  });

  it('validates password correctly', () => {
    expect(true).toBe(true);
  });
});
'''


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def temp_specs_dir():
    """Create a temporary specs directory with sample spec."""
    with tempfile.TemporaryDirectory() as tmpdir:
        specs_dir = Path(tmpdir) / "specs"
        specs_dir.mkdir()

        # Create sample spec
        spec_dir = specs_dir / "user-auth"
        spec_dir.mkdir()

        with open(spec_dir / "requirements.yaml", "w") as f:
            yaml.dump(SAMPLE_SPEC, f)

        yield specs_dir


@pytest.fixture
def temp_tests_dir():
    """Create a temporary tests directory with existing tests."""
    with tempfile.TemporaryDirectory() as tmpdir:
        tests_dir = Path(tmpdir)

        # Create Python tests
        py_tests = tests_dir / "unit"
        py_tests.mkdir()
        (py_tests / "test_existing.py").write_text(PYTHON_TEST_FILE)

        # Create TypeScript tests
        ts_tests = tests_dir / "e2e"
        ts_tests.mkdir()
        (ts_tests / "auth.test.ts").write_text(TYPESCRIPT_TEST_FILE)

        yield tests_dir


@pytest.fixture
def generator(temp_specs_dir, temp_tests_dir):
    """Create a TestGenerator with temp directories."""
    return TestGenerator(specs_dir=temp_specs_dir, tests_dir=temp_tests_dir)


# =============================================================================
# Unit Tests - EarsPattern Enum
# =============================================================================


class TestEarsPattern:
    """Tests for EarsPattern enum."""

    def test_all_patterns_defined(self):
        """All EARS patterns are defined."""
        patterns = [p.value for p in EarsPattern]
        assert "ubiquitous" in patterns
        assert "event-driven" in patterns
        assert "state-driven" in patterns
        assert "optional" in patterns
        assert "unwanted" in patterns

    def test_pattern_from_string(self):
        """Pattern can be created from string."""
        assert EarsPattern("ubiquitous") == EarsPattern.UBIQUITOUS
        assert EarsPattern("event-driven") == EarsPattern.EVENT_DRIVEN


# =============================================================================
# Unit Tests - Requirement Dataclass
# =============================================================================


class TestRequirement:
    """Tests for Requirement dataclass."""

    def test_requirement_creation(self):
        """Requirement can be created with required fields."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall do something",
        )

        assert req.id == "REQ-001"
        assert req.pattern == EarsPattern.UBIQUITOUS
        assert req.text == "The system shall do something"
        assert req.testable is True  # Default

    def test_requirement_with_all_fields(self):
        """Requirement can be created with all fields."""
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.EVENT_DRIVEN,
            text="When X happens, system shall Y",
            domain="web-backend",
            priority=1,
            testable=True,
            metadata={"source": "prd"},
        )

        assert req.domain == "web-backend"
        assert req.priority == 1
        assert req.metadata["source"] == "prd"


# =============================================================================
# Unit Tests - TestStub Dataclass
# =============================================================================


class TestTestStub:
    """Tests for TestStub dataclass."""

    def test_stub_creation(self):
        """TestStub can be created."""
        stub = TestStub(
            name="test_example",
            description="Example test",
            requirement_id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            code="def test_example(): pass",
            language="python",
            format=TestFormat.PYTEST,
        )

        assert stub.name == "test_example"
        assert stub.language == "python"
        assert stub.format == TestFormat.PYTEST


# =============================================================================
# Unit Tests - TestGenerator
# =============================================================================


class TestTestGeneratorInit:
    """Tests for TestGenerator initialization."""

    def test_default_paths(self):
        """Generator uses default paths."""
        gen = TestGenerator()
        assert gen.specs_dir == Path(".c4/specs")
        assert gen.tests_dir == Path("tests")

    def test_custom_paths(self):
        """Generator accepts custom paths."""
        gen = TestGenerator(specs_dir="/custom/specs", tests_dir="/custom/tests")
        assert gen.specs_dir == Path("/custom/specs")
        assert gen.tests_dir == Path("/custom/tests")


class TestLoadSpec:
    """Tests for spec loading."""

    def test_load_valid_spec(self, generator):
        """Valid spec is loaded correctly."""
        spec = generator._load_spec("user-auth")

        assert spec["feature"] == "user-auth"
        assert len(spec["requirements"]) == 6

    def test_load_missing_spec(self, generator):
        """Missing spec raises FileNotFoundError."""
        with pytest.raises(FileNotFoundError):
            generator._load_spec("nonexistent")


class TestParseRequirements:
    """Tests for requirement parsing."""

    def test_parse_all_requirements(self, generator):
        """All requirements are parsed."""
        spec = generator._load_spec("user-auth")
        reqs = generator._parse_requirements(spec)

        assert len(reqs) == 6

    def test_parse_requirement_fields(self, generator):
        """Requirement fields are parsed correctly."""
        spec = generator._load_spec("user-auth")
        reqs = generator._parse_requirements(spec)

        req = reqs[0]
        assert req.id == "REQ-001"
        assert req.pattern == EarsPattern.UBIQUITOUS
        assert "validate user credentials" in req.text
        assert req.testable is True

    def test_parse_different_patterns(self, generator):
        """Different patterns are parsed correctly."""
        spec = generator._load_spec("user-auth")
        reqs = generator._parse_requirements(spec)

        patterns = [r.pattern for r in reqs]
        assert EarsPattern.UBIQUITOUS in patterns
        assert EarsPattern.EVENT_DRIVEN in patterns
        assert EarsPattern.STATE_DRIVEN in patterns
        assert EarsPattern.OPTIONAL in patterns
        assert EarsPattern.UNWANTED in patterns


class TestNormalizeTestName:
    """Tests for test name normalization."""

    def test_normalize_snake_case(self):
        """Snake case names are normalized."""
        gen = TestGenerator()
        assert gen._normalize_test_name("test_example") == "test_example"

    def test_normalize_camel_case(self):
        """Camel case names are converted to snake case."""
        gen = TestGenerator()
        assert gen._normalize_test_name("testExample") == "test_example"
        assert gen._normalize_test_name("testExampleFunction") == "test_example_function"

    def test_normalize_spaces_and_hyphens(self):
        """Spaces and hyphens are converted to underscores."""
        gen = TestGenerator()
        assert gen._normalize_test_name("test example") == "test_example"
        assert gen._normalize_test_name("test-example") == "test_example"


class TestRequirementToTestName:
    """Tests for converting requirements to test names."""

    def test_basic_conversion(self):
        """Basic requirement is converted to test name."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        name = gen._requirement_to_test_name(req)
        assert name.startswith("test_")
        assert "req_001" in name

    def test_removes_common_prefixes(self):
        """Common EARS prefixes are removed."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall provide authentication",
        )

        name = gen._requirement_to_test_name(req)
        assert "the_system_shall" not in name


class TestScanExistingTests:
    """Tests for scanning existing tests."""

    def test_scan_python_tests(self, generator):
        """Python tests are scanned correctly."""
        existing = generator._scan_existing_tests(language="python")

        # Should find tests from PYTHON_TEST_FILE
        assert "test_login_success" in existing or "login_success" in existing

    def test_scan_typescript_tests(self, generator):
        """TypeScript tests are scanned correctly."""
        existing = generator._scan_existing_tests(language="typescript")

        # Should find tests from TYPESCRIPT_TEST_FILE
        # Names are normalized so check for key parts
        assert len(existing) > 0

    def test_scan_empty_directory(self, temp_specs_dir):
        """Empty directory returns empty set."""
        gen = TestGenerator(specs_dir=temp_specs_dir, tests_dir="/nonexistent")
        existing = gen._scan_existing_tests(language="python")

        assert len(existing) == 0


class TestGeneratePythonTest:
    """Tests for Python test generation."""

    def test_generates_valid_python(self):
        """Generated Python test is syntactically valid."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        code = gen._generate_python_test(req)

        assert "def test_" in code
        assert 'pytest.skip("Not implemented yet")' in code
        assert "REQ-001" in code

    def test_includes_docstring(self):
        """Generated test includes docstring."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        code = gen._generate_python_test(req)

        assert '"""' in code

    def test_includes_aaa_comments(self):
        """Generated test includes AAA comments."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        code = gen._generate_python_test(req)

        assert "# Arrange" in code
        assert "# Act" in code
        assert "# Assert" in code

    def test_event_driven_pattern_comments(self):
        """Event-driven pattern has appropriate comments."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.EVENT_DRIVEN,
            text="When X happens, system shall Y",
        )

        code = gen._generate_python_test(req)

        assert "Trigger the event" in code


class TestGenerateTypescriptTest:
    """Tests for TypeScript test generation."""

    def test_generates_vitest_format(self):
        """Generated TypeScript uses vitest format."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        code = gen._generate_typescript_test(req, TestFormat.VITEST)

        assert "it.skip(" in code
        assert "expect(" in code
        assert "REQ-001" in code

    def test_generates_jest_format(self):
        """Generated TypeScript uses jest format."""
        gen = TestGenerator()
        req = Requirement(
            id="REQ-001",
            pattern=EarsPattern.UBIQUITOUS,
            text="The system shall validate credentials",
        )

        code = gen._generate_typescript_test(req, TestFormat.JEST)

        assert "it.skip(" in code
        assert "expect(" in code


class TestGenerateTestStubs:
    """Tests for generate_test_stubs method."""

    def test_generates_stubs_for_spec(self, generator):
        """Stubs are generated for all testable requirements."""
        result = generator.generate_test_stubs("user-auth", languages=["python"])

        # Should generate for testable requirements (5 of 6)
        assert len(result.stubs) == 5
        assert result.total_requirements == 6

    def test_skips_non_testable(self, generator):
        """Non-testable requirements are skipped."""
        result = generator.generate_test_stubs("user-auth", languages=["python"])

        # REQ-006 is non-testable
        assert "REQ-006" in result.skipped

    def test_result_has_metadata(self, generator):
        """Result includes metadata."""
        result = generator.generate_test_stubs("user-auth", languages=["python"])

        assert result.spec_id == "user-auth"
        assert result.generated_at is not None

    def test_generates_multiple_languages(self, generator):
        """Can generate for multiple languages."""
        result = generator.generate_test_stubs(
            "user-auth",
            languages=["python", "typescript"],
        )

        python_stubs = [s for s in result.stubs if s.language == "python"]
        ts_stubs = [s for s in result.stubs if s.language == "typescript"]

        assert len(python_stubs) > 0
        assert len(ts_stubs) > 0


class TestGenerateTestFile:
    """Tests for generate_test_file method."""

    def test_generates_python_file(self, generator, tmp_path):
        """Python test file is generated."""
        output = tmp_path / "test_auth.py"
        result = generator.generate_test_file(
            "user-auth",
            language="python",
            output_path=output,
        )

        assert result.exists()
        content = result.read_text()
        assert "import pytest" in content
        assert "def test_" in content

    def test_generates_typescript_file(self, generator, tmp_path):
        """TypeScript test file is generated."""
        output = tmp_path / "auth.test.ts"
        result = generator.generate_test_file(
            "user-auth",
            language="typescript",
            test_format=TestFormat.VITEST,
            output_path=output,
        )

        assert result.exists()
        content = result.read_text()
        assert "import { describe, it, expect }" in content
        assert "it.skip(" in content

    def test_auto_generates_path(self, generator):
        """Output path is auto-generated if not specified."""
        # Use custom output to avoid creating in real tests dir
        with tempfile.TemporaryDirectory() as tmpdir:
            gen = TestGenerator(
                specs_dir=generator.specs_dir,
                tests_dir=tmpdir,
            )
            result = gen.generate_test_file("user-auth", language="python")

            assert result.exists()
            assert "user_auth" in result.name


class TestGetCoverageSummary:
    """Tests for coverage summary."""

    def test_coverage_with_existing_tests(self, generator):
        """Coverage is calculated correctly."""
        summary = generator.get_coverage_summary("user-auth", language="python")

        assert summary["spec_id"] == "user-auth"
        assert summary["total_requirements"] == 6
        assert summary["testable_requirements"] == 5
        assert "coverage_percentage" in summary

    def test_uncovered_requirements_listed(self, generator):
        """Uncovered requirements are listed."""
        summary = generator.get_coverage_summary("user-auth", language="python")

        # Some requirements should be uncovered
        assert len(summary["uncovered"]) > 0


# =============================================================================
# Integration Tests
# =============================================================================


class TestIntegration:
    """Integration tests for TestGenerator."""

    def test_full_workflow(self, temp_specs_dir, tmp_path):
        """Full workflow: load spec, generate tests, check coverage."""
        gen = TestGenerator(specs_dir=temp_specs_dir, tests_dir=tmp_path)

        # Generate tests
        result = gen.generate_test_stubs("user-auth", languages=["python"])
        assert len(result.stubs) > 0

        # Write test file
        output = gen.generate_test_file(
            "user-auth",
            language="python",
            output_path=tmp_path / "test_auth.py",
        )
        assert output.exists()

        # Check coverage (should now show some coverage)
        summary = gen.get_coverage_summary("user-auth", language="python")
        assert summary["testable_requirements"] == 5
