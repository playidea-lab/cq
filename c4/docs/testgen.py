"""Test Generator for EARS Specifications.

Generates test stubs from EARS requirements in Python (pytest)
and TypeScript (vitest/jest) formats.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any

import yaml


class TestFormat(Enum):
    """Test framework format."""

    PYTEST = "pytest"
    VITEST = "vitest"
    JEST = "jest"


class EarsPattern(Enum):
    """EARS requirement pattern types."""

    UBIQUITOUS = "ubiquitous"
    EVENT_DRIVEN = "event-driven"
    STATE_DRIVEN = "state-driven"
    OPTIONAL = "optional"
    UNWANTED = "unwanted"


@dataclass
class Requirement:
    """An EARS requirement."""

    id: str
    pattern: EarsPattern
    text: str
    domain: str = ""
    priority: int = 0
    testable: bool = True
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class TestStub:
    """A generated test stub."""

    name: str
    description: str
    requirement_id: str
    pattern: EarsPattern
    code: str
    language: str  # "python" or "typescript"
    format: TestFormat


@dataclass
class TestGenerationResult:
    """Result of test generation."""

    spec_id: str
    stubs: list[TestStub]
    skipped: list[str]  # Requirement IDs skipped (duplicates or non-testable)
    generated_at: str
    total_requirements: int


class TestGenerator:
    """Test Generator for EARS Specifications.

    Generates test stubs from EARS requirements supporting:
    - Python pytest format
    - TypeScript vitest/jest format

    Example:
        generator = TestGenerator(specs_dir=".c4/specs")
        result = generator.generate_test_stubs("user-auth")

        # Write Python tests
        for stub in result.stubs:
            if stub.language == "python":
                print(stub.code)

        # Write TypeScript tests
        for stub in result.stubs:
            if stub.language == "typescript":
                print(stub.code)
    """

    # Test naming patterns
    PYTHON_TEST_PATTERN = re.compile(r"def\s+test_(\w+)")
    TYPESCRIPT_TEST_PATTERN = re.compile(r"(?:it|test)\s*\(\s*['\"]([^'\"]+)['\"]")

    def __init__(
        self,
        specs_dir: str | Path = ".c4/specs",
        tests_dir: str | Path = "tests",
    ) -> None:
        """Initialize the test generator.

        Args:
            specs_dir: Directory containing spec files
            tests_dir: Directory containing existing tests
        """
        self.specs_dir = Path(specs_dir)
        self.tests_dir = Path(tests_dir)
        self._existing_tests: set[str] = set()

    def _load_spec(self, spec_id: str) -> dict[str, Any]:
        """Load a specification file.

        Args:
            spec_id: Specification identifier (directory name)

        Returns:
            Parsed spec data

        Raises:
            FileNotFoundError: If spec file doesn't exist
        """
        spec_path = self.specs_dir / spec_id / "requirements.yaml"

        if not spec_path.exists():
            raise FileNotFoundError(f"Spec not found: {spec_path}")

        with open(spec_path, encoding="utf-8") as f:
            return yaml.safe_load(f)

    def _parse_requirements(self, spec_data: dict[str, Any]) -> list[Requirement]:
        """Parse requirements from spec data.

        Args:
            spec_data: Parsed YAML spec data

        Returns:
            List of Requirement objects
        """
        requirements = []

        for req_data in spec_data.get("requirements", []):
            try:
                pattern = EarsPattern(req_data.get("pattern", "ubiquitous"))
            except ValueError:
                pattern = EarsPattern.UBIQUITOUS

            req = Requirement(
                id=req_data.get("id", ""),
                pattern=pattern,
                text=req_data.get("text", ""),
                domain=req_data.get("domain", ""),
                priority=req_data.get("priority", 0),
                testable=req_data.get("testable", True),
            )
            requirements.append(req)

        return requirements

    def _scan_existing_tests(
        self,
        language: str = "python",
        subdirs: list[str] | None = None,
    ) -> set[str]:
        """Scan existing tests to avoid duplicates.

        Args:
            language: "python" or "typescript"
            subdirs: Subdirectories to scan (default: all)

        Returns:
            Set of existing test names (normalized)
        """
        existing = set()

        if not self.tests_dir.exists():
            return existing

        # Determine file patterns
        if language == "python":
            patterns = ["**/test_*.py", "**/*_test.py"]
            test_pattern = self.PYTHON_TEST_PATTERN
        else:
            patterns = ["**/*.test.ts", "**/*.spec.ts", "**/*.test.js", "**/*.spec.js"]
            test_pattern = self.TYPESCRIPT_TEST_PATTERN

        # Scan subdirectories
        search_dirs = (
            [self.tests_dir / subdir for subdir in subdirs]
            if subdirs
            else [self.tests_dir]
        )

        for search_dir in search_dirs:
            if not search_dir.exists():
                continue

            for pattern in patterns:
                for test_file in search_dir.glob(pattern):
                    try:
                        content = test_file.read_text(encoding="utf-8")
                        matches = test_pattern.findall(content)
                        for match in matches:
                            # Normalize test name
                            normalized = self._normalize_test_name(match)
                            existing.add(normalized)
                    except Exception:
                        continue

        return existing

    def _normalize_test_name(self, name: str) -> str:
        """Normalize a test name for comparison.

        Args:
            name: Test name to normalize

        Returns:
            Normalized name (lowercase, underscores)
        """
        # Convert camelCase to snake_case
        name = re.sub(r"([a-z])([A-Z])", r"\1_\2", name)
        # Replace spaces and hyphens with underscores
        name = re.sub(r"[\s\-]+", "_", name)
        # Lowercase and strip
        return name.lower().strip()

    def _requirement_to_test_name(self, req: Requirement) -> str:
        """Convert a requirement to a test name.

        Args:
            req: Requirement object

        Returns:
            Test function/case name
        """
        # Extract key phrase from requirement text
        text = req.text.lower()

        # Remove common EARS prefixes
        text = re.sub(r"^(the system shall|when|while|if|where)\s+", "", text)
        text = re.sub(r"^(be able to|have|provide|support|allow)\s+", "", text)

        # Clean up and convert to snake_case
        text = re.sub(r"[^\w\s]", "", text)
        words = text.split()[:6]  # Take first 6 words
        name = "_".join(words)

        # Add requirement ID suffix
        req_suffix = req.id.lower().replace("-", "_")

        return f"test_{name}_{req_suffix}"

    def _generate_python_test(self, req: Requirement) -> str:
        """Generate a pytest test stub.

        Args:
            req: Requirement to generate test for

        Returns:
            Python test code
        """
        test_name = self._requirement_to_test_name(req)
        docstring = self._generate_test_docstring(req)
        arrange_act_assert = self._generate_aaa_comments(req)

        return f'''def {test_name}():
    """{docstring}"""
{arrange_act_assert}
    # TODO: Implement test for {req.id}
    pytest.skip("Not implemented yet")
'''

    def _generate_typescript_test(
        self,
        req: Requirement,
        test_format: TestFormat = TestFormat.VITEST,
    ) -> str:
        """Generate a TypeScript test stub.

        Args:
            req: Requirement to generate test for
            test_format: vitest or jest

        Returns:
            TypeScript test code
        """
        test_name = self._requirement_to_test_name(req)
        # Convert snake_case to readable description
        description = test_name.replace("test_", "").replace("_", " ")
        docstring = self._generate_test_docstring(req)
        arrange_act_assert = self._generate_aaa_comments_ts(req)

        if test_format == TestFormat.VITEST:
            skip_fn = "it.skip"
        else:
            skip_fn = "it.skip"

        return f'''{skip_fn}("{description}", () => {{
  // {docstring}
{arrange_act_assert}
  // TODO: Implement test for {req.id}
  expect(true).toBe(false);
}});
'''

    def _generate_test_docstring(self, req: Requirement) -> str:
        """Generate a test docstring from requirement.

        Args:
            req: Requirement object

        Returns:
            Docstring text
        """
        pattern_prefix = {
            EarsPattern.UBIQUITOUS: "Verify that",
            EarsPattern.EVENT_DRIVEN: "When triggered,",
            EarsPattern.STATE_DRIVEN: "While in state,",
            EarsPattern.OPTIONAL: "If enabled,",
            EarsPattern.UNWANTED: "Ensure prevention of",
        }

        prefix = pattern_prefix.get(req.pattern, "Verify that")
        # Truncate long requirement text
        text = req.text[:100] + "..." if len(req.text) > 100 else req.text

        return f"{prefix} {text}\n\n    Requirement: {req.id} ({req.pattern.value})"

    def _generate_aaa_comments(self, req: Requirement) -> str:
        """Generate Arrange-Act-Assert comments based on pattern.

        Args:
            req: Requirement object

        Returns:
            Python AAA comment block
        """
        if req.pattern == EarsPattern.EVENT_DRIVEN:
            return """    # Arrange: Set up preconditions
    # ...

    # Act: Trigger the event
    # ...

    # Assert: Verify the expected outcome
    # ...
"""
        elif req.pattern == EarsPattern.STATE_DRIVEN:
            return """    # Arrange: Set up the required state
    # ...

    # Act: Perform action while in state
    # ...

    # Assert: Verify behavior during state
    # ...
"""
        elif req.pattern == EarsPattern.OPTIONAL:
            return """    # Arrange: Enable the optional feature
    # ...

    # Act: Perform the action
    # ...

    # Assert: Verify optional behavior is active
    # ...
"""
        elif req.pattern == EarsPattern.UNWANTED:
            return """    # Arrange: Set up conditions for unwanted behavior
    # ...

    # Act: Attempt to trigger unwanted behavior
    # ...

    # Assert: Verify prevention/handling of unwanted case
    # ...
"""
        else:  # UBIQUITOUS
            return """    # Arrange: Set up test data
    # ...

    # Act: Execute the functionality
    # ...

    # Assert: Verify the requirement is met
    # ...
"""

    def _generate_aaa_comments_ts(self, req: Requirement) -> str:
        """Generate Arrange-Act-Assert comments for TypeScript.

        Args:
            req: Requirement object

        Returns:
            TypeScript AAA comment block
        """
        if req.pattern == EarsPattern.EVENT_DRIVEN:
            return """  // Arrange: Set up preconditions
  // ...

  // Act: Trigger the event
  // ...

  // Assert: Verify the expected outcome
  // ..."""
        elif req.pattern == EarsPattern.STATE_DRIVEN:
            return """  // Arrange: Set up the required state
  // ...

  // Act: Perform action while in state
  // ...

  // Assert: Verify behavior during state
  // ..."""
        elif req.pattern == EarsPattern.OPTIONAL:
            return """  // Arrange: Enable the optional feature
  // ...

  // Act: Perform the action
  // ...

  // Assert: Verify optional behavior is active
  // ..."""
        elif req.pattern == EarsPattern.UNWANTED:
            return """  // Arrange: Set up conditions for unwanted behavior
  // ...

  // Act: Attempt to trigger unwanted behavior
  // ...

  // Assert: Verify prevention/handling of unwanted case
  // ..."""
        else:  # UBIQUITOUS
            return """  // Arrange: Set up test data
  // ...

  // Act: Execute the functionality
  // ...

  // Assert: Verify the requirement is met
  // ..."""

    def _generate_file_header_python(self, spec_id: str, spec_data: dict) -> str:
        """Generate Python test file header.

        Args:
            spec_id: Specification ID
            spec_data: Specification data

        Returns:
            Python file header
        """
        feature = spec_data.get("feature", spec_id)
        description = spec_data.get("description", "")

        return f'''"""Generated tests for {feature}.

{description}

Auto-generated from EARS requirements.
Generated at: {datetime.now(timezone.utc).isoformat()}
"""

import pytest

'''

    def _generate_file_header_typescript(
        self,
        spec_id: str,
        spec_data: dict,
        test_format: TestFormat = TestFormat.VITEST,
    ) -> str:
        """Generate TypeScript test file header.

        Args:
            spec_id: Specification ID
            spec_data: Specification data
            test_format: vitest or jest

        Returns:
            TypeScript file header
        """
        feature = spec_data.get("feature", spec_id)
        description = spec_data.get("description", "")

        if test_format == TestFormat.VITEST:
            imports = "import { describe, it, expect } from 'vitest';"
        else:
            imports = "// Jest is configured globally"

        return f'''/**
 * Generated tests for {feature}.
 *
 * {description}
 *
 * Auto-generated from EARS requirements.
 * Generated at: {datetime.now(timezone.utc).isoformat()}
 */

{imports}

'''

    def generate_test_stubs(
        self,
        spec_id: str,
        languages: list[str] | None = None,
        test_format: TestFormat = TestFormat.VITEST,
        check_duplicates: bool = True,
    ) -> TestGenerationResult:
        """Generate test stubs for a specification.

        Args:
            spec_id: Specification identifier
            languages: Languages to generate ("python", "typescript")
            test_format: TypeScript test format (vitest or jest)
            check_duplicates: Check for existing tests

        Returns:
            TestGenerationResult with generated stubs
        """
        if languages is None:
            languages = ["python"]

        # Load specification
        spec_data = self._load_spec(spec_id)
        requirements = self._parse_requirements(spec_data)

        # Scan existing tests if checking duplicates
        existing_tests: dict[str, set[str]] = {}
        if check_duplicates:
            for lang in languages:
                existing_tests[lang] = self._scan_existing_tests(language=lang)

        stubs: list[TestStub] = []
        skipped: list[str] = []

        for req in requirements:
            # Skip non-testable requirements
            if not req.testable:
                skipped.append(req.id)
                continue

            test_name = self._requirement_to_test_name(req)
            normalized = self._normalize_test_name(test_name)

            for lang in languages:
                # Check for duplicates
                if check_duplicates and normalized in existing_tests.get(lang, set()):
                    skipped.append(req.id)
                    continue

                # Generate stub based on language
                if lang == "python":
                    code = self._generate_python_test(req)
                    stub = TestStub(
                        name=test_name,
                        description=req.text,
                        requirement_id=req.id,
                        pattern=req.pattern,
                        code=code,
                        language="python",
                        format=TestFormat.PYTEST,
                    )
                else:
                    code = self._generate_typescript_test(req, test_format)
                    stub = TestStub(
                        name=test_name,
                        description=req.text,
                        requirement_id=req.id,
                        pattern=req.pattern,
                        code=code,
                        language="typescript",
                        format=test_format,
                    )

                stubs.append(stub)

        return TestGenerationResult(
            spec_id=spec_id,
            stubs=stubs,
            skipped=list(set(skipped)),
            generated_at=datetime.now(timezone.utc).isoformat(),
            total_requirements=len(requirements),
        )

    def generate_test_file(
        self,
        spec_id: str,
        language: str = "python",
        test_format: TestFormat = TestFormat.VITEST,
        output_path: str | Path | None = None,
        check_duplicates: bool = True,
    ) -> Path:
        """Generate a complete test file for a specification.

        Args:
            spec_id: Specification identifier
            language: "python" or "typescript"
            test_format: TypeScript test format
            output_path: Custom output path (auto-generated if None)
            check_duplicates: Check for existing tests

        Returns:
            Path to generated test file
        """
        # Load spec for header
        spec_data = self._load_spec(spec_id)

        # Generate stubs
        result = self.generate_test_stubs(
            spec_id=spec_id,
            languages=[language],
            test_format=test_format,
            check_duplicates=check_duplicates,
        )

        # Build file content
        if language == "python":
            header = self._generate_file_header_python(spec_id, spec_data)
            file_ext = ".py"
        else:
            header = self._generate_file_header_typescript(
                spec_id, spec_data, test_format
            )
            file_ext = ".test.ts" if test_format == TestFormat.VITEST else ".spec.ts"

        # Combine stubs
        content = header
        for stub in result.stubs:
            if stub.language == language:
                content += stub.code + "\n"

        # Determine output path
        if output_path is None:
            safe_name = spec_id.replace("-", "_").replace(" ", "_")
            if language == "python":
                output_path = self.tests_dir / "generated" / f"test_{safe_name}{file_ext}"
            else:
                output_path = self.tests_dir / "generated" / f"{safe_name}{file_ext}"
        else:
            output_path = Path(output_path)

        # Create directory if needed
        output_path.parent.mkdir(parents=True, exist_ok=True)

        # Write file
        output_path.write_text(content, encoding="utf-8")

        return output_path

    def get_coverage_summary(
        self,
        spec_id: str,
        language: str = "python",
    ) -> dict[str, Any]:
        """Get test coverage summary for a specification.

        Args:
            spec_id: Specification identifier
            language: Language to check

        Returns:
            Coverage summary dict
        """
        spec_data = self._load_spec(spec_id)
        requirements = self._parse_requirements(spec_data)
        existing_tests = self._scan_existing_tests(language=language)

        covered = 0
        uncovered = []

        for req in requirements:
            if not req.testable:
                continue

            test_name = self._requirement_to_test_name(req)
            normalized = self._normalize_test_name(test_name)

            if normalized in existing_tests:
                covered += 1
            else:
                uncovered.append(req.id)

        total_testable = len([r for r in requirements if r.testable])
        coverage_pct = (covered / total_testable * 100) if total_testable > 0 else 0

        return {
            "spec_id": spec_id,
            "total_requirements": len(requirements),
            "testable_requirements": total_testable,
            "covered": covered,
            "uncovered": uncovered,
            "coverage_percentage": round(coverage_pct, 2),
        }
