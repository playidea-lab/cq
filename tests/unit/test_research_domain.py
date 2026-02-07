"""Tests for Research Domain: YAML schema validation and routing."""

from pathlib import Path

import pytest
import yaml

REGISTRY_V1 = Path(__file__).parent.parent.parent / "c4" / "system" / "registry" / "v1"


# =============================================================================
# YAML Schema Validation
# =============================================================================


class TestResearchSkillsSchema:
    """Validate research skill YAML files."""

    REQUIRED_SKILL_FIELDS = {"id", "name", "description", "impact", "category"}
    SKILL_FILES = [
        "paper-reading.yaml",
        "paper-writing.yaml",
        "paper-reviewing.yaml",
        "knowledge-extraction.yaml",
    ]

    @pytest.mark.parametrize("filename", SKILL_FILES)
    def test_skill_file_exists(self, filename):
        path = REGISTRY_V1 / "skills" / filename
        assert path.exists(), f"Missing skill file: {filename}"

    @pytest.mark.parametrize("filename", SKILL_FILES)
    def test_skill_has_required_fields(self, filename):
        path = REGISTRY_V1 / "skills" / filename
        data = yaml.safe_load(path.read_text())
        assert "skill" in data, f"Missing 'skill' root key in {filename}"
        skill = data["skill"]
        for field in self.REQUIRED_SKILL_FIELDS:
            assert field in skill, f"Missing field '{field}' in {filename}"

    @pytest.mark.parametrize("filename", SKILL_FILES)
    def test_skill_has_triggers(self, filename):
        path = REGISTRY_V1 / "skills" / filename
        data = yaml.safe_load(path.read_text())
        skill = data["skill"]
        assert "triggers" in skill, f"Missing triggers in {filename}"
        assert "keywords" in skill["triggers"], f"Missing keywords trigger in {filename}"

    @pytest.mark.parametrize("filename", SKILL_FILES)
    def test_skill_domain_is_research(self, filename):
        path = REGISTRY_V1 / "skills" / filename
        data = yaml.safe_load(path.read_text())
        assert "research" in data["skill"]["domains"]

    def test_skill_ids_are_unique(self):
        ids = set()
        for filename in self.SKILL_FILES:
            path = REGISTRY_V1 / "skills" / filename
            data = yaml.safe_load(path.read_text())
            skill_id = data["skill"]["id"]
            assert skill_id not in ids, f"Duplicate skill ID: {skill_id}"
            ids.add(skill_id)


class TestResearchPersonasSchema:
    """Validate research persona YAML files."""

    REQUIRED_AGENT_FIELDS = {"id", "name", "persona", "skills", "instructions"}
    PERSONA_FILES = [
        "paper-reader.yaml",
        "paper-writer.yaml",
        "paper-reviewer.yaml",
        "knowledge-engineer.yaml",
    ]

    @pytest.mark.parametrize("filename", PERSONA_FILES)
    def test_persona_file_exists(self, filename):
        path = REGISTRY_V1 / "personas" / filename
        assert path.exists(), f"Missing persona file: {filename}"

    @pytest.mark.parametrize("filename", PERSONA_FILES)
    def test_persona_has_required_fields(self, filename):
        path = REGISTRY_V1 / "personas" / filename
        data = yaml.safe_load(path.read_text())
        assert "agent" in data, f"Missing 'agent' root key in {filename}"
        agent = data["agent"]
        for field in self.REQUIRED_AGENT_FIELDS:
            assert field in agent, f"Missing field '{field}' in {filename}"

    @pytest.mark.parametrize("filename", PERSONA_FILES)
    def test_persona_has_personality(self, filename):
        path = REGISTRY_V1 / "personas" / filename
        data = yaml.safe_load(path.read_text())
        persona = data["agent"]["persona"]
        assert "personality" in persona
        p = persona["personality"]
        assert "style" in p
        assert "communication" in p
        assert "approach" in p

    @pytest.mark.parametrize("filename", PERSONA_FILES)
    def test_persona_has_handoff(self, filename):
        path = REGISTRY_V1 / "personas" / filename
        data = yaml.safe_load(path.read_text())
        instructions = data["agent"]["instructions"]
        assert "on_receive" in instructions
        assert "on_handoff" in instructions

    def test_persona_ids_are_unique(self):
        ids = set()
        for filename in self.PERSONA_FILES:
            path = REGISTRY_V1 / "personas" / filename
            data = yaml.safe_load(path.read_text())
            agent_id = data["agent"]["id"]
            assert agent_id not in ids, f"Duplicate persona ID: {agent_id}"
            ids.add(agent_id)


class TestResearchDomainSchema:
    """Validate research domain YAML."""

    def test_domain_file_exists(self):
        path = REGISTRY_V1 / "domains" / "research.yaml"
        assert path.exists()

    def test_domain_has_required_fields(self):
        path = REGISTRY_V1 / "domains" / "research.yaml"
        data = yaml.safe_load(path.read_text())
        assert "domain" in data
        domain = data["domain"]
        assert domain["id"] == "research"
        assert "name" in domain
        assert "required_skills" in domain
        assert "workflow" in domain

    def test_domain_workflow_has_steps(self):
        path = REGISTRY_V1 / "domains" / "research.yaml"
        data = yaml.safe_load(path.read_text())
        workflow = data["domain"]["workflow"]
        assert len(workflow) >= 3  # primary, support, quality
        roles = [step["role"] for step in workflow]
        assert "primary" in roles
        assert "quality" in roles


class TestTaskTypeOverrides:
    """Validate research entries in task-type-overrides."""

    def test_overrides_file_exists(self):
        path = REGISTRY_V1 / "rules" / "task-type-overrides.yaml"
        assert path.exists()

    def test_has_research_overrides(self):
        path = REGISTRY_V1 / "rules" / "task-type-overrides.yaml"
        data = yaml.safe_load(path.read_text())
        override_names = [o["name"] for o in data["rules"]["overrides"]]
        assert "paper-review-tasks" in override_names
        assert "literature-review-tasks" in override_names
        assert "paper-writing-tasks" in override_names
        assert "knowledge-extraction-tasks" in override_names

    def test_paper_review_override_priority(self):
        path = REGISTRY_V1 / "rules" / "task-type-overrides.yaml"
        data = yaml.safe_load(path.read_text())
        for override in data["rules"]["overrides"]:
            if override["name"] == "paper-review-tasks":
                assert override["priority"] == 90
                assert override["action"]["set_primary"] == "paper-reviewer"
                break
        else:
            pytest.fail("paper-review-tasks override not found")

    def test_research_chain_extension(self):
        path = REGISTRY_V1 / "rules" / "task-type-overrides.yaml"
        data = yaml.safe_load(path.read_text())
        ext_names = [e["name"] for e in data["rules"]["chain_extensions"]]
        assert "research-needs-knowledge" in ext_names
