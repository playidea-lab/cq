"""SkillValidator - Validate skill definitions against schema and quality rules.

Provides multi-level validation:
- Level 1 (Required): JSON Schema + Pydantic validation, minimum triggers
- Level 2 (Recommended): Rules present, description quality, examples
- Level 3 (Optional): Dependencies, test cases (--check-deps, --run-tests)

Usage:
    >>> validator = SkillValidator()
    >>> result = validator.validate_file(Path("skills/debugging.yaml"))
    >>> print(result.summary())
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, cast

import jsonschema
import yaml
from pydantic import ValidationError as PydanticValidationError

from c4.supervisor.agent_graph.models import SkillDefinition


class ValidationLevel(str, Enum):
    """Validation severity level."""

    ERROR = "error"  # Level 1: Must fix
    WARNING = "warning"  # Level 2: Should fix
    INFO = "info"  # Level 3: Nice to have


@dataclass
class ValidationIssue:
    """A single validation issue."""

    level: ValidationLevel
    code: str
    message: str
    path: str = ""  # JSON path to the issue location
    suggestion: str | None = None

    def __str__(self) -> str:
        prefix = {"error": "E", "warning": "W", "info": "I"}[self.level.value]
        loc = f"[{self.path}] " if self.path else ""
        return f"{prefix} {self.code}: {loc}{self.message}"


@dataclass
class ValidationResult:
    """Result of skill validation."""

    skill_id: str | None = None
    skill_path: Path | None = None
    issues: list[ValidationIssue] = field(default_factory=list)
    skill_definition: SkillDefinition | None = None

    @property
    def is_valid(self) -> bool:
        """True if no errors (Level 1 issues)."""
        return not any(i.level == ValidationLevel.ERROR for i in self.issues)

    @property
    def error_count(self) -> int:
        """Number of errors."""
        return sum(1 for i in self.issues if i.level == ValidationLevel.ERROR)

    @property
    def warning_count(self) -> int:
        """Number of warnings."""
        return sum(1 for i in self.issues if i.level == ValidationLevel.WARNING)

    @property
    def info_count(self) -> int:
        """Number of info messages."""
        return sum(1 for i in self.issues if i.level == ValidationLevel.INFO)

    def add_error(
        self, code: str, message: str, path: str = "", suggestion: str | None = None
    ) -> None:
        """Add an error (Level 1)."""
        self.issues.append(
            ValidationIssue(ValidationLevel.ERROR, code, message, path, suggestion)
        )

    def add_warning(
        self, code: str, message: str, path: str = "", suggestion: str | None = None
    ) -> None:
        """Add a warning (Level 2)."""
        self.issues.append(
            ValidationIssue(ValidationLevel.WARNING, code, message, path, suggestion)
        )

    def add_info(
        self, code: str, message: str, path: str = "", suggestion: str | None = None
    ) -> None:
        """Add an info message (Level 3)."""
        self.issues.append(
            ValidationIssue(ValidationLevel.INFO, code, message, path, suggestion)
        )

    def summary(self) -> str:
        """Generate a summary string."""
        if not self.issues:
            return f"[{self.skill_id or 'unknown'}] All checks passed"

        lines = [f"[{self.skill_id or 'unknown'}] {self.error_count} errors, {self.warning_count} warnings"]
        for issue in self.issues:
            lines.append(f"  {issue}")
        return "\n".join(lines)


class SkillValidator:
    """Validates skill definitions against schema and quality rules.

    Validation Levels:
    - Level 1 (Required): Errors that must be fixed
      - JSON Schema validation
      - Pydantic model validation
      - At least one trigger (keywords OR task_types OR file_patterns)
      - At least one capability

    - Level 2 (Recommended): Warnings for best practices
      - At least 3 rules recommended
      - Description at least 50 characters
      - example_bad/example_good pairs for rules

    - Level 3 (Optional): Info for additional checks
      - Dependencies exist (--check-deps)
      - Test cases pass (--run-tests)
    """

    def __init__(self, schema_dir: Path | None = None) -> None:
        """Initialize validator.

        Args:
            schema_dir: Directory containing skill.schema.yaml
        """
        from c4.supervisor.agent_graph import SCHEMA_DIR

        self.schema_dir = schema_dir or SCHEMA_DIR
        self._schema_cache: dict[str, Any] | None = None

    def _get_schema(self) -> dict[str, Any]:
        """Load and cache the skill schema."""
        if self._schema_cache is None:
            schema_path = self.schema_dir / "skill.schema.yaml"
            with open(schema_path, encoding="utf-8") as f:
                self._schema_cache = cast(dict[str, Any], yaml.safe_load(f))
        return self._schema_cache

    def validate_file(
        self,
        skill_path: Path,
        check_deps: bool = False,
        available_skills: set[str] | None = None,
    ) -> ValidationResult:
        """Validate a skill YAML file.

        Args:
            skill_path: Path to skill YAML file
            check_deps: Whether to check dependencies (Level 3)
            available_skills: Set of available skill IDs for dependency checking

        Returns:
            ValidationResult with all issues found
        """
        result = ValidationResult(skill_path=skill_path)

        # Step 1: Parse YAML
        try:
            with open(skill_path, encoding="utf-8") as f:
                data = yaml.safe_load(f)
        except yaml.YAMLError as e:
            result.add_error("YAML-001", f"Invalid YAML syntax: {e}")
            return result
        except FileNotFoundError:
            result.add_error("FILE-001", f"File not found: {skill_path}")
            return result

        if data is None:
            result.add_error("YAML-002", "Empty YAML file")
            return result

        # Extract skill ID early for better error messages
        if isinstance(data, dict) and "skill" in data:
            skill_data = data.get("skill", {})
            if isinstance(skill_data, dict):
                result.skill_id = skill_data.get("id")

        # Step 2: JSON Schema validation (Level 1)
        schema = self._get_schema()
        schema_errors = self._validate_json_schema(data, schema)
        for error_msg, path in schema_errors:
            result.add_error("SCHEMA-001", error_msg, path)

        if result.error_count > 0:
            return result  # Stop early if schema validation fails

        # Step 3: Pydantic model validation (Level 1)
        try:
            skill_def = SkillDefinition.model_validate(data)
            result.skill_definition = skill_def
            result.skill_id = skill_def.skill.id
        except PydanticValidationError as e:
            for err in e.errors():
                path = ".".join(str(p) for p in err.get("loc", []))
                result.add_error("MODEL-001", err["msg"], path)
            return result

        # Step 4: Level 1 - Required checks
        self._check_required(result, skill_def)

        # Step 5: Level 2 - Recommended checks
        self._check_recommended(result, skill_def)

        # Step 6: Level 3 - Optional checks (if enabled)
        if check_deps and available_skills:
            self._check_dependencies(result, skill_def, available_skills)

        return result

    def _validate_json_schema(
        self, data: dict[str, Any], schema: dict[str, Any]
    ) -> list[tuple[str, str]]:
        """Validate data against JSON schema.

        Returns:
            List of (error_message, path) tuples
        """
        validator = jsonschema.Draft202012Validator(schema)
        errors = list(validator.iter_errors(data))

        results = []
        for error in errors:
            path = ".".join(str(p) for p in error.absolute_path) if error.absolute_path else "root"
            results.append((error.message, path))
        return results

    def _check_required(self, result: ValidationResult, skill_def: SkillDefinition) -> None:
        """Level 1: Required checks."""
        skill = skill_def.skill
        triggers = skill.triggers

        # Check at least one trigger type is present
        has_trigger = (
            (triggers.keywords and len(triggers.keywords) > 0)
            or (triggers.task_types and len(triggers.task_types) > 0)
            or (triggers.file_patterns and len(triggers.file_patterns) > 0)
        )
        if not has_trigger:
            result.add_error(
                "TRIGGER-001",
                "At least one trigger type (keywords, task_types, or file_patterns) must be provided",
                "skill.triggers",
                suggestion="Add keywords like ['bug', 'error'] or task_types like ['debug', 'fix']",
            )

        # Check capabilities exist
        if not skill.capabilities or len(skill.capabilities) == 0:
            result.add_error(
                "CAP-001",
                "At least one capability must be provided",
                "skill.capabilities",
                suggestion="Add capabilities like ['error-tracing', 'log-analysis']",
            )

    def _check_recommended(self, result: ValidationResult, skill_def: SkillDefinition) -> None:
        """Level 2: Recommended checks."""
        skill = skill_def.skill

        # Check description length
        if len(skill.description) < 50:
            result.add_warning(
                "DESC-001",
                f"Description is short ({len(skill.description)} chars), 50+ recommended",
                "skill.description",
                suggestion="Provide a more detailed description of what this skill enables",
            )

        # Check rules exist
        if not skill.rules or len(skill.rules) == 0:
            result.add_warning(
                "RULE-001",
                "No rules defined, at least 1 recommended",
                "skill.rules",
                suggestion="Add rules with id, description, and impact (e.g., PERF-001)",
            )
        elif len(skill.rules) < 3:
            result.add_info(
                "RULE-002",
                f"Only {len(skill.rules)} rule(s) defined, 3+ recommended for comprehensive coverage",
                "skill.rules",
            )

        # Check rules have examples
        if skill.rules:
            for i, rule in enumerate(skill.rules):
                if not rule.example_bad or not rule.example_good:
                    result.add_info(
                        "RULE-003",
                        f"Rule {rule.id} missing example_bad/example_good pair",
                        f"skill.rules[{i}]",
                        suggestion="Add code examples to illustrate the rule",
                    )

        # Check metadata
        if not skill.metadata:
            result.add_info(
                "META-001",
                "No metadata defined",
                "skill.metadata",
                suggestion="Add version, author, and tags for better discovery",
            )
        elif skill.metadata.deprecated and not skill.metadata.deprecated_by:
            result.add_warning(
                "META-002",
                "Skill is deprecated but no replacement specified",
                "skill.metadata.deprecated_by",
                suggestion="Specify the skill ID that replaces this one",
            )

    def _check_dependencies(
        self,
        result: ValidationResult,
        skill_def: SkillDefinition,
        available_skills: set[str],
    ) -> None:
        """Level 3: Dependency checks."""
        skill = skill_def.skill

        if not skill.dependencies:
            return

        # Check required dependencies exist
        for dep_id in skill.dependencies.required:
            if dep_id not in available_skills:
                result.add_error(
                    "DEP-001",
                    f"Required dependency '{dep_id}' not found",
                    "skill.dependencies.required",
                    suggestion=f"Create skill '{dep_id}' or remove from required dependencies",
                )

        # Check optional dependencies exist (info only)
        for dep_id in skill.dependencies.optional:
            if dep_id not in available_skills:
                result.add_info(
                    "DEP-002",
                    f"Optional dependency '{dep_id}' not found",
                    "skill.dependencies.optional",
                )

    def validate_directory(
        self,
        skill_dir: Path,
        recursive: bool = True,
        check_deps: bool = False,
    ) -> list[ValidationResult]:
        """Validate all skill files in a directory.

        Args:
            skill_dir: Directory containing skill YAML files
            recursive: Whether to search subdirectories
            check_deps: Whether to check dependencies

        Returns:
            List of ValidationResult for each skill file
        """
        results: list[ValidationResult] = []

        # Find all skill files
        pattern = "**/*.yaml" if recursive else "*.yaml"
        skill_files = list(skill_dir.glob(pattern))

        # First pass: collect all skill IDs
        available_skills: set[str] = set()
        for skill_file in skill_files:
            # Skip non-skill files (e.g., _domain.yaml, _groups.yaml)
            if skill_file.name.startswith("_"):
                continue
            try:
                with open(skill_file, encoding="utf-8") as f:
                    data = yaml.safe_load(f)
                if data and "skill" in data and "id" in data["skill"]:
                    available_skills.add(data["skill"]["id"])
            except (yaml.YAMLError, KeyError):
                pass

        # Second pass: validate each file
        for skill_file in sorted(skill_files):
            if skill_file.name.startswith("_"):
                continue
            result = self.validate_file(
                skill_file,
                check_deps=check_deps,
                available_skills=available_skills if check_deps else None,
            )
            results.append(result)

        return results

    def check_circular_dependencies(
        self, skill_dir: Path
    ) -> list[tuple[str, list[str]]]:
        """Check for circular dependencies in skills.

        Args:
            skill_dir: Directory containing skill YAML files

        Returns:
            List of (skill_id, cycle_path) tuples for each cycle found
        """
        # Build dependency graph
        deps: dict[str, set[str]] = {}

        for skill_file in skill_dir.glob("**/*.yaml"):
            if skill_file.name.startswith("_"):
                continue
            try:
                with open(skill_file, encoding="utf-8") as f:
                    data = yaml.safe_load(f)
                if not data or "skill" not in data:
                    continue

                skill_data = data["skill"]
                skill_id = skill_data.get("id")
                if not skill_id:
                    continue

                deps[skill_id] = set()
                if "dependencies" in skill_data:
                    dep_data = skill_data["dependencies"]
                    if "required" in dep_data:
                        deps[skill_id].update(dep_data["required"])
                    if "optional" in dep_data:
                        deps[skill_id].update(dep_data["optional"])
            except (yaml.YAMLError, KeyError):
                pass

        # Find cycles using DFS
        cycles: list[tuple[str, list[str]]] = []
        visited: set[str] = set()
        rec_stack: set[str] = set()

        def dfs(node: str, path: list[str]) -> None:
            if node in rec_stack:
                # Found cycle
                cycle_start = path.index(node)
                cycle = path[cycle_start:] + [node]
                cycles.append((node, cycle))
                return

            if node in visited:
                return

            visited.add(node)
            rec_stack.add(node)
            path.append(node)

            for neighbor in deps.get(node, set()):
                dfs(neighbor, path)

            path.pop()
            rec_stack.remove(node)

        for skill_id in deps:
            if skill_id not in visited:
                dfs(skill_id, [])

        return cycles


def validate_skill_cli(skill_path: Path, check_deps: bool = False) -> tuple[bool, str]:
    """CLI-friendly validation function.

    Args:
        skill_path: Path to skill file or directory
        check_deps: Whether to check dependencies

    Returns:
        Tuple of (success: bool, message: str)
    """
    validator = SkillValidator()

    if skill_path.is_file():
        result = validator.validate_file(skill_path, check_deps=check_deps)
        return result.is_valid, result.summary()

    elif skill_path.is_dir():
        results = validator.validate_directory(skill_path, check_deps=check_deps)

        if not results:
            return True, "No skill files found"

        total_errors = sum(r.error_count for r in results)
        total_warnings = sum(r.warning_count for r in results)
        valid_count = sum(1 for r in results if r.is_valid)

        lines = [f"Validated {len(results)} skill(s): {valid_count} passed, {total_errors} errors, {total_warnings} warnings"]
        lines.append("")

        for result in results:
            status = "PASS" if result.is_valid else "FAIL"
            lines.append(f"  [{status}] {result.skill_id or result.skill_path}")
            for issue in result.issues:
                if issue.level != ValidationLevel.INFO:
                    lines.append(f"       {issue}")

        return total_errors == 0, "\n".join(lines)

    else:
        return False, f"Path not found: {skill_path}"
