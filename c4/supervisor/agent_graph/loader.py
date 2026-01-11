"""AgentGraphLoader - Load and validate YAML definitions for the agent graph system.

This module provides functionality to:
1. Load skill, agent, domain, and rule definitions from YAML files
2. Validate them against JSON schemas
3. Parse them into Pydantic models

Directory structure expected:
    base_dir/
        skills/*.yaml       - Skill definitions
        personas/*.yaml     - Agent definitions
        domains/*.yaml      - Domain definitions
        rules/*.yaml        - Rule definitions
"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

import jsonschema
import yaml
from pydantic import BaseModel
from pydantic import ValidationError as PydanticValidationError

from c4.supervisor.agent_graph.models import (
    AgentDefinition,
    DomainDefinition,
    RuleDefinition,
    SkillDefinition,
)

if TYPE_CHECKING:
    from typing import Any

# Schema directory relative to this file
SCHEMA_DIR = Path(__file__).parent / "schema"


class LoaderError(Exception):
    """Base exception for loader errors."""

    pass


class FileNotFoundError(LoaderError):
    """Raised when a file or directory is not found."""

    def __init__(self, path: Path, message: str | None = None):
        self.path = path
        self.message = message or f"Path not found: {path}"
        super().__init__(self.message)


class SchemaValidationError(LoaderError):
    """Raised when YAML data fails JSON schema validation."""

    def __init__(self, file_path: Path, errors: list[str], schema_path: Path | None = None):
        self.file_path = file_path
        self.errors = errors
        self.schema_path = schema_path

        error_details = "\n  - ".join(errors)
        self.message = f"Schema validation failed for {file_path}:\n  - {error_details}"
        super().__init__(self.message)


class YAMLParseError(LoaderError):
    """Raised when YAML parsing fails."""

    def __init__(self, file_path: Path, original_error: Exception):
        self.file_path = file_path
        self.original_error = original_error
        self.message = f"Failed to parse YAML file {file_path}: {original_error}"
        super().__init__(self.message)


class ModelValidationError(LoaderError):
    """Raised when Pydantic model validation fails."""

    def __init__(self, file_path: Path, errors: list[str]):
        self.file_path = file_path
        self.errors = errors

        error_details = "\n  - ".join(errors)
        self.message = f"Model validation failed for {file_path}:\n  - {error_details}"
        super().__init__(self.message)


class AgentGraphLoader:
    """Loads and validates agent graph definitions from YAML files.

    Example usage:
        loader = AgentGraphLoader(Path(".c4/agents"))
        skills = loader.load_skills()
        agents = loader.load_agents()
        domains = loader.load_domains()
        rules = loader.load_rules()

        # Or load everything at once
        all_definitions = loader.load_all()
    """

    # Subdirectory names for each definition type
    SKILLS_DIR = "skills"
    PERSONAS_DIR = "personas"
    DOMAINS_DIR = "domains"
    RULES_DIR = "rules"

    # Schema file names
    SKILL_SCHEMA = "skill.schema.yaml"
    AGENT_SCHEMA = "agent.schema.yaml"
    DOMAIN_SCHEMA = "domain.schema.yaml"
    RULE_SCHEMA = "rule.schema.yaml"

    def __init__(self, base_dir: Path | None = None, schema_dir: Path | None = None):
        """Initialize the loader.

        Args:
            base_dir: Base directory containing skills/, personas/, domains/, rules/.
                      Defaults to the examples/ directory.
            schema_dir: Directory containing JSON schema files.
                       Defaults to the schema/ directory in the package.
        """
        from c4.supervisor.agent_graph import EXAMPLES_DIR

        self.base_dir = base_dir or EXAMPLES_DIR
        self.schema_dir = schema_dir or SCHEMA_DIR

        # Cache loaded schemas
        self._schema_cache: dict[str, dict[str, Any]] = {}

    def _get_schema(self, schema_filename: str) -> dict[str, Any]:
        """Load and cache a JSON schema from YAML file.

        Args:
            schema_filename: Name of the schema file (e.g., "skill.schema.yaml")

        Returns:
            Parsed schema as a dictionary

        Raises:
            FileNotFoundError: If schema file doesn't exist
            YAMLParseError: If schema file is invalid YAML
        """
        if schema_filename in self._schema_cache:
            return self._schema_cache[schema_filename]

        schema_path = self.schema_dir / schema_filename
        if not schema_path.exists():
            raise FileNotFoundError(schema_path, f"Schema file not found: {schema_path}")

        try:
            with open(schema_path, encoding="utf-8") as f:
                schema = cast(dict[str, Any], yaml.safe_load(f))
        except yaml.YAMLError as e:
            raise YAMLParseError(schema_path, e) from e

        self._schema_cache[schema_filename] = schema
        return schema

    def _load_yaml_file(self, file_path: Path) -> dict[str, Any]:
        """Load and parse a YAML file.

        Args:
            file_path: Path to the YAML file

        Returns:
            Parsed YAML as a dictionary

        Raises:
            FileNotFoundError: If file doesn't exist
            YAMLParseError: If file is invalid YAML
        """
        if not file_path.exists():
            raise FileNotFoundError(file_path, f"YAML file not found: {file_path}")

        try:
            with open(file_path, encoding="utf-8") as f:
                data = yaml.safe_load(f)
        except yaml.YAMLError as e:
            raise YAMLParseError(file_path, e) from e

        if data is None:
            raise YAMLParseError(file_path, ValueError("Empty YAML file"))

        return cast(dict[str, Any], data)

    def _validate_against_schema(
        self,
        data: dict[str, Any],
        schema: dict[str, Any],
        file_path: Path,
        schema_path: Path | None = None,
    ) -> None:
        """Validate data against a JSON schema.

        Args:
            data: Data to validate
            schema: JSON schema to validate against
            file_path: Path to the file being validated (for error messages)
            schema_path: Path to the schema file (for error messages)

        Raises:
            SchemaValidationError: If validation fails
        """
        validator = jsonschema.Draft202012Validator(schema)
        errors = list(validator.iter_errors(data))

        if errors:
            error_messages = []
            for error in errors:
                # Build a path string for the error location
                if error.absolute_path:
                    path = ".".join(str(p) for p in error.absolute_path)
                else:
                    path = "root"
                error_messages.append(f"[{path}] {error.message}")

            raise SchemaValidationError(file_path, error_messages, schema_path)

    def _load_definitions_from_dir(
        self,
        subdir: str,
        schema_filename: str,
        model_class: type[BaseModel],
    ) -> list[Any]:
        """Generic method to load definitions from a subdirectory.

        Args:
            subdir: Subdirectory name (e.g., "skills")
            schema_filename: Schema file name (e.g., "skill.schema.yaml")
            model_class: Pydantic model class to parse into

        Returns:
            List of parsed model instances

        Raises:
            FileNotFoundError: If directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        dir_path = self.base_dir / subdir
        if not dir_path.exists():
            raise FileNotFoundError(dir_path, f"Directory not found: {dir_path}")

        if not dir_path.is_dir():
            raise FileNotFoundError(dir_path, f"Path is not a directory: {dir_path}")

        # Get all YAML files
        yaml_files = list(dir_path.glob("*.yaml")) + list(dir_path.glob("*.yml"))
        if not yaml_files:
            return []  # Empty directory is valid

        # Load schema
        schema = self._get_schema(schema_filename)
        schema_path = self.schema_dir / schema_filename

        results = []
        for yaml_file in sorted(yaml_files):  # Sort for deterministic order
            # Load and parse YAML
            data = self._load_yaml_file(yaml_file)

            # Validate against JSON schema
            self._validate_against_schema(data, schema, yaml_file, schema_path)

            # Parse into Pydantic model
            try:
                model = model_class.model_validate(data)
            except PydanticValidationError as e:
                errors = [f"{err['loc']}: {err['msg']}" for err in e.errors()]
                raise ModelValidationError(yaml_file, errors) from e

            results.append(model)

        return results

    def load_skills(self) -> list[SkillDefinition]:
        """Load all skill definitions from the skills/ subdirectory.

        Returns:
            List of SkillDefinition objects

        Raises:
            FileNotFoundError: If skills/ directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        return self._load_definitions_from_dir(
            self.SKILLS_DIR,
            self.SKILL_SCHEMA,
            SkillDefinition,
        )

    def load_agents(self) -> list[AgentDefinition]:
        """Load all agent definitions from the personas/ subdirectory.

        Returns:
            List of AgentDefinition objects

        Raises:
            FileNotFoundError: If personas/ directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        return self._load_definitions_from_dir(
            self.PERSONAS_DIR,
            self.AGENT_SCHEMA,
            AgentDefinition,
        )

    def load_domains(self) -> list[DomainDefinition]:
        """Load all domain definitions from the domains/ subdirectory.

        Returns:
            List of DomainDefinition objects

        Raises:
            FileNotFoundError: If domains/ directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        return self._load_definitions_from_dir(
            self.DOMAINS_DIR,
            self.DOMAIN_SCHEMA,
            DomainDefinition,
        )

    def load_rules(self) -> list[RuleDefinition]:
        """Load all rule definitions from the rules/ subdirectory.

        Returns:
            List of RuleDefinition objects

        Raises:
            FileNotFoundError: If rules/ directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        return self._load_definitions_from_dir(
            self.RULES_DIR,
            self.RULE_SCHEMA,
            RuleDefinition,
        )

    def load_all(self) -> dict[str, list[Any]]:
        """Load all definitions from all subdirectories.

        Returns:
            Dictionary with keys 'skills', 'agents', 'domains', 'rules'
            containing the respective lists of definitions

        Raises:
            FileNotFoundError: If any required directory doesn't exist
            YAMLParseError: If any YAML file is invalid
            SchemaValidationError: If any file fails schema validation
            ModelValidationError: If any file fails Pydantic validation
        """
        return {
            "skills": self.load_skills(),
            "agents": self.load_agents(),
            "domains": self.load_domains(),
            "rules": self.load_rules(),
        }

    def load_skill_by_id(self, skill_id: str) -> SkillDefinition | None:
        """Load a specific skill by its ID.

        Args:
            skill_id: The skill ID to search for

        Returns:
            SkillDefinition if found, None otherwise
        """
        skills = self.load_skills()
        for skill in skills:
            if skill.skill.id == skill_id:
                return skill
        return None

    def load_agent_by_id(self, agent_id: str) -> AgentDefinition | None:
        """Load a specific agent by its ID.

        Args:
            agent_id: The agent ID to search for

        Returns:
            AgentDefinition if found, None otherwise
        """
        agents = self.load_agents()
        for agent in agents:
            if agent.agent.id == agent_id:
                return agent
        return None

    def load_domain_by_id(self, domain_id: str) -> DomainDefinition | None:
        """Load a specific domain by its ID.

        Args:
            domain_id: The domain ID to search for

        Returns:
            DomainDefinition if found, None otherwise
        """
        domains = self.load_domains()
        for domain in domains:
            if domain.domain.id == domain_id:
                return domain
        return None
