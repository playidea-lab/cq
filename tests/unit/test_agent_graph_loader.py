"""Unit tests for AgentGraphLoader.

Tests cover:
1. Loading skills, agents, domains, rules from example files
2. Schema validation failures
3. YAML parsing errors
4. File/directory not found errors
5. Pydantic model validation errors
6. Loading by ID
7. Empty directories
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from c4.supervisor.agent_graph import (
    EXAMPLES_DIR,
    SCHEMA_DIR,
    AgentDefinition,
    AgentGraphLoader,
    DomainDefinition,
    RuleDefinition,
    SkillDefinition,
)
from c4.supervisor.agent_graph.loader import (
    FileNotFoundError as LoaderFileNotFoundError,
)
from c4.supervisor.agent_graph.loader import (
    ModelValidationError,
    SchemaValidationError,
    YAMLParseError,
)


class TestAgentGraphLoaderInit:
    """Tests for AgentGraphLoader initialization."""

    def test_default_directories(self):
        """Test that loader uses default directories when none provided."""
        loader = AgentGraphLoader()
        assert loader.base_dir == EXAMPLES_DIR
        assert loader.schema_dir == SCHEMA_DIR

    def test_custom_directories(self, tmp_path: Path):
        """Test that loader uses custom directories when provided."""
        custom_schema = tmp_path / "custom_schema"
        custom_schema.mkdir()

        loader = AgentGraphLoader(base_dir=tmp_path, schema_dir=custom_schema)
        assert loader.base_dir == tmp_path
        assert loader.schema_dir == custom_schema


class TestLoadSkills:
    """Tests for loading skill definitions."""

    def test_load_skills_from_examples(self):
        """Test loading skills from the examples directory."""
        loader = AgentGraphLoader()
        skills = loader.load_skills()

        assert len(skills) >= 1
        assert all(isinstance(s, SkillDefinition) for s in skills)

        # Check that we have the debugging skill
        skill_ids = [s.skill.id for s in skills]
        assert "debugging" in skill_ids

    def test_load_skill_structure(self):
        """Test that loaded skills have correct structure."""
        loader = AgentGraphLoader()
        skills = loader.load_skills()

        debugging_skill = next((s for s in skills if s.skill.id == "debugging"), None)
        assert debugging_skill is not None

        # Check required fields
        assert debugging_skill.skill.name == "Debugging & Error Analysis"
        assert len(debugging_skill.skill.capabilities) >= 1
        assert debugging_skill.skill.triggers.keywords is not None
        assert "bug" in debugging_skill.skill.triggers.keywords

    def test_load_skill_by_id(self):
        """Test loading a specific skill by ID."""
        loader = AgentGraphLoader()
        skill = loader.load_skill_by_id("debugging")

        assert skill is not None
        assert skill.skill.id == "debugging"

    def test_load_skill_by_id_not_found(self):
        """Test loading a non-existent skill returns None."""
        loader = AgentGraphLoader()
        skill = loader.load_skill_by_id("non-existent-skill")

        assert skill is None

    def test_load_skills_empty_directory(self, tmp_path: Path):
        """Test loading from an empty skills directory returns empty list."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()
        # Create other required directories
        (tmp_path / "personas").mkdir()
        (tmp_path / "domains").mkdir()
        (tmp_path / "rules").mkdir()

        loader = AgentGraphLoader(base_dir=tmp_path)
        skills = loader.load_skills()

        assert skills == []

    def test_load_skills_directory_not_found(self, tmp_path: Path):
        """Test that missing skills directory raises FileNotFoundError."""
        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(LoaderFileNotFoundError) as exc_info:
            loader.load_skills()

        assert "skills" in str(exc_info.value.path)


class TestLoadAgents:
    """Tests for loading agent definitions."""

    def test_load_agents_from_examples(self):
        """Test loading agents from the examples directory."""
        loader = AgentGraphLoader()
        agents = loader.load_agents()

        assert len(agents) >= 1
        assert all(isinstance(a, AgentDefinition) for a in agents)

        # Check that we have the debugger agent
        agent_ids = [a.agent.id for a in agents]
        assert "debugger" in agent_ids

    def test_load_agent_structure(self):
        """Test that loaded agents have correct structure."""
        loader = AgentGraphLoader()
        agents = loader.load_agents()

        debugger = next((a for a in agents if a.agent.id == "debugger"), None)
        assert debugger is not None

        # Check required fields
        assert debugger.agent.name == "Debugger"
        assert debugger.agent.persona.role == "Senior Debug Engineer"
        assert "debugging" in debugger.agent.skills.primary

    def test_load_agent_by_id(self):
        """Test loading a specific agent by ID."""
        loader = AgentGraphLoader()
        agent = loader.load_agent_by_id("debugger")

        assert agent is not None
        assert agent.agent.id == "debugger"

    def test_load_agent_by_id_not_found(self):
        """Test loading a non-existent agent returns None."""
        loader = AgentGraphLoader()
        agent = loader.load_agent_by_id("non-existent-agent")

        assert agent is None


class TestLoadDomains:
    """Tests for loading domain definitions."""

    def test_load_domains_from_examples(self):
        """Test loading domains from the examples directory."""
        loader = AgentGraphLoader()
        domains = loader.load_domains()

        assert len(domains) >= 1
        assert all(isinstance(d, DomainDefinition) for d in domains)

        # Check that we have the web-backend domain
        domain_ids = [d.domain.id for d in domains]
        assert "web-backend" in domain_ids

    def test_load_domain_structure(self):
        """Test that loaded domains have correct structure."""
        loader = AgentGraphLoader()
        domains = loader.load_domains()

        web_backend = next((d for d in domains if d.domain.id == "web-backend"), None)
        assert web_backend is not None

        # Check required fields
        assert web_backend.domain.name == "Web Backend Development"
        assert len(web_backend.domain.required_skills.core) >= 1
        assert len(web_backend.domain.workflow) >= 1

    def test_load_domain_by_id(self):
        """Test loading a specific domain by ID."""
        loader = AgentGraphLoader()
        domain = loader.load_domain_by_id("web-backend")

        assert domain is not None
        assert domain.domain.id == "web-backend"

    def test_load_domain_by_id_not_found(self):
        """Test loading a non-existent domain returns None."""
        loader = AgentGraphLoader()
        domain = loader.load_domain_by_id("non-existent-domain")

        assert domain is None


class TestLoadRules:
    """Tests for loading rule definitions."""

    def test_load_rules_from_examples(self):
        """Test loading rules from the examples directory."""
        loader = AgentGraphLoader()
        rules = loader.load_rules()

        assert len(rules) >= 1
        assert all(isinstance(r, RuleDefinition) for r in rules)

    def test_load_rules_structure(self):
        """Test that loaded rules have correct structure."""
        loader = AgentGraphLoader()
        rules = loader.load_rules()

        # Should have at least one rule definition
        assert len(rules) >= 1

        # Check routing rules
        routing_rules = rules[0]
        assert routing_rules.rules.overrides is not None or \
               routing_rules.rules.chain_extensions is not None or \
               routing_rules.rules.selection is not None


class TestLoadAll:
    """Tests for loading all definitions at once."""

    def test_load_all_from_examples(self):
        """Test loading all definitions from examples directory."""
        loader = AgentGraphLoader()
        all_defs = loader.load_all()

        assert "skills" in all_defs
        assert "agents" in all_defs
        assert "domains" in all_defs
        assert "rules" in all_defs

        assert len(all_defs["skills"]) >= 1
        assert len(all_defs["agents"]) >= 1
        assert len(all_defs["domains"]) >= 1
        assert len(all_defs["rules"]) >= 1


class TestSchemaValidation:
    """Tests for JSON schema validation."""

    def test_invalid_skill_missing_required(self, tmp_path: Path):
        """Test that missing required fields are caught by schema validation."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        # Create invalid skill (missing capabilities)
        invalid_skill = {
            "skill": {
                "id": "test-skill",
                "name": "Test Skill",
                "description": "A test skill for validation",
                # Missing: capabilities, triggers
            }
        }
        skill_file = skills_dir / "invalid.yaml"
        with open(skill_file, "w") as f:
            yaml.dump(invalid_skill, f)

        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(SchemaValidationError) as exc_info:
            loader.load_skills()

        assert "invalid.yaml" in str(exc_info.value.file_path)
        assert len(exc_info.value.errors) >= 1

    def test_invalid_skill_wrong_id_pattern(self, tmp_path: Path):
        """Test that invalid ID pattern is caught by schema validation."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        # Create skill with invalid ID (should be kebab-case)
        invalid_skill = {
            "skill": {
                "id": "Test_Skill",  # Invalid: underscore and uppercase
                "name": "Test Skill",
                "description": "A test skill for validation",
                "capabilities": ["test-cap"],
                "triggers": {"keywords": ["test"]},
            }
        }
        skill_file = skills_dir / "invalid.yaml"
        with open(skill_file, "w") as f:
            yaml.dump(invalid_skill, f)

        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(SchemaValidationError) as exc_info:
            loader.load_skills()

        assert "pattern" in str(exc_info.value.errors).lower() or \
               "id" in str(exc_info.value.errors).lower()


class TestYAMLParseErrors:
    """Tests for YAML parsing error handling."""

    def test_invalid_yaml_syntax(self, tmp_path: Path):
        """Test that invalid YAML syntax raises YAMLParseError."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        # Create file with invalid YAML
        invalid_file = skills_dir / "invalid.yaml"
        with open(invalid_file, "w") as f:
            f.write("skill:\n  id: test\n  invalid:: yaml: here")

        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(YAMLParseError) as exc_info:
            loader.load_skills()

        assert "invalid.yaml" in str(exc_info.value.file_path)

    def test_empty_yaml_file(self, tmp_path: Path):
        """Test that empty YAML file raises YAMLParseError."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        empty_file = skills_dir / "empty.yaml"
        empty_file.touch()

        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(YAMLParseError) as exc_info:
            loader.load_skills()

        assert "Empty YAML file" in str(exc_info.value.message)


class TestFileNotFoundErrors:
    """Tests for file not found error handling."""

    def test_schema_file_not_found(self, tmp_path: Path):
        """Test that missing schema file raises FileNotFoundError."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        # Create a valid skill file
        valid_skill = {
            "skill": {
                "id": "test-skill",
                "name": "Test Skill",
                "description": "A test skill for validation",
                "capabilities": ["test-cap"],
                "triggers": {"keywords": ["test"]},
            }
        }
        skill_file = skills_dir / "valid.yaml"
        with open(skill_file, "w") as f:
            yaml.dump(valid_skill, f)

        # Use non-existent schema directory
        empty_schema_dir = tmp_path / "non_existent_schema"

        loader = AgentGraphLoader(base_dir=tmp_path, schema_dir=empty_schema_dir)

        with pytest.raises(LoaderFileNotFoundError) as exc_info:
            loader.load_skills()

        assert "schema" in str(exc_info.value.path).lower()

    def test_path_is_not_directory(self, tmp_path: Path):
        """Test that a file path instead of directory raises error."""
        # Create a file instead of directory
        file_path = tmp_path / "skills"
        file_path.touch()

        loader = AgentGraphLoader(base_dir=tmp_path)

        with pytest.raises(LoaderFileNotFoundError) as exc_info:
            loader.load_skills()

        assert "not a directory" in str(exc_info.value.message)


class TestModelValidation:
    """Tests for Pydantic model validation errors."""

    def test_pydantic_validation_error(self, tmp_path: Path):
        """Test that Pydantic validation errors are properly handled."""
        skills_dir = tmp_path / "skills"
        skills_dir.mkdir()

        # Create skill that passes schema but fails Pydantic validation
        # (e.g., description too short according to Pydantic model)
        invalid_skill = {
            "skill": {
                "id": "test-skill",
                "name": "Test",
                "description": "Short",  # Too short (min 10 chars)
                "capabilities": ["test-cap"],
                "triggers": {"keywords": ["test"]},
            }
        }
        skill_file = skills_dir / "invalid.yaml"
        with open(skill_file, "w") as f:
            yaml.dump(invalid_skill, f)

        loader = AgentGraphLoader(base_dir=tmp_path)

        # This should fail at schema validation (minLength) or model validation
        with pytest.raises((SchemaValidationError, ModelValidationError)):
            loader.load_skills()


class TestSchemaCache:
    """Tests for schema caching behavior."""

    def test_schema_is_cached(self):
        """Test that schemas are cached after first load."""
        loader = AgentGraphLoader()

        # Load skills twice
        loader.load_skills()
        loader.load_skills()

        # Schema should be in cache
        assert AgentGraphLoader.SKILL_SCHEMA in loader._schema_cache

    def test_different_loaders_have_separate_caches(self):
        """Test that different loader instances have separate caches."""
        loader1 = AgentGraphLoader()
        loader2 = AgentGraphLoader()

        loader1.load_skills()

        # loader2 should have empty cache
        assert AgentGraphLoader.SKILL_SCHEMA not in loader2._schema_cache
