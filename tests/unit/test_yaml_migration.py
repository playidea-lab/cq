"""Validation tests for YAML migration from AgentRouter.

These tests verify that:
1. All YAML files in examples/ directory are valid and loadable
2. The routing results are consistent with AgentRouter defaults
3. All domains, agents, and rules are correctly defined
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from c4.supervisor.agent_graph import EXAMPLES_DIR
from c4.supervisor.agent_router import (
    DOMAIN_AGENT_MAP,
    TASK_TYPE_AGENT_OVERRIDES,
    AgentRouter,
)

# ============================================================================
# YAML File Loading Tests
# ============================================================================


class TestYAMLFileValidity:
    """Tests that all YAML files are valid and loadable."""

    @pytest.fixture
    def examples_dir(self) -> Path:
        """Get the examples directory path."""
        return EXAMPLES_DIR

    def test_examples_directory_exists(self, examples_dir: Path) -> None:
        """Examples directory should exist."""
        assert examples_dir.exists(), f"Examples directory not found: {examples_dir}"

    def test_all_domain_files_valid(self, examples_dir: Path) -> None:
        """All domain YAML files should be valid."""
        domains_dir = examples_dir / "domains"
        assert domains_dir.exists(), f"Domains directory not found: {domains_dir}"

        yaml_files = list(domains_dir.glob("*.yaml"))
        assert len(yaml_files) >= 12, f"Expected 12+ domain files, found {len(yaml_files)}"

        for yaml_file in yaml_files:
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            assert "domain" in data, f"Missing 'domain' key in {yaml_file.name}"
            assert "id" in data["domain"], f"Missing domain.id in {yaml_file.name}"
            assert "name" in data["domain"], f"Missing domain.name in {yaml_file.name}"

    def test_all_persona_files_valid(self, examples_dir: Path) -> None:
        """All persona (agent) YAML files should be valid."""
        personas_dir = examples_dir / "personas"
        assert personas_dir.exists(), f"Personas directory not found: {personas_dir}"

        yaml_files = list(personas_dir.glob("*.yaml"))
        assert len(yaml_files) >= 22, f"Expected 22+ persona files, found {len(yaml_files)}"

        for yaml_file in yaml_files:
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            assert "agent" in data, f"Missing 'agent' key in {yaml_file.name}"
            assert "id" in data["agent"], f"Missing agent.id in {yaml_file.name}"
            assert "name" in data["agent"], f"Missing agent.name in {yaml_file.name}"

    def test_all_rule_files_valid(self, examples_dir: Path) -> None:
        """All rule YAML files should be valid."""
        rules_dir = examples_dir / "rules"
        assert rules_dir.exists(), f"Rules directory not found: {rules_dir}"

        yaml_files = list(rules_dir.glob("*.yaml"))
        assert len(yaml_files) >= 1, f"Expected 1+ rule files, found {len(yaml_files)}"

        for yaml_file in yaml_files:
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            assert "rules" in data, f"Missing 'rules' key in {yaml_file.name}"


# ============================================================================
# Domain Coverage Tests
# ============================================================================


class TestDomainCoverage:
    """Tests that all AgentRouter domains are covered in YAML."""

    @pytest.fixture
    def yaml_domains(self) -> set[str]:
        """Load domain IDs from YAML files."""
        domains_dir = EXAMPLES_DIR / "domains"
        domain_ids = set()
        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            if "domain" in data and "id" in data["domain"]:
                domain_ids.add(data["domain"]["id"])
        return domain_ids

    def test_all_legacy_domains_covered(self, yaml_domains: set[str]) -> None:
        """All domains from AgentRouter should have YAML files."""
        legacy_domains = set(DOMAIN_AGENT_MAP.keys())

        missing = legacy_domains - yaml_domains
        assert not missing, f"Missing YAML files for domains: {missing}"

    def test_domain_count_matches(self, yaml_domains: set[str]) -> None:
        """YAML domain count should match or exceed legacy count."""
        legacy_count = len(DOMAIN_AGENT_MAP)
        yaml_count = len(yaml_domains)
        assert yaml_count >= legacy_count, (
            f"Expected at least {legacy_count} domains, found {yaml_count}"
        )


# ============================================================================
# Agent Coverage Tests
# ============================================================================


class TestAgentCoverage:
    """Tests that all agents referenced in AgentRouter are covered."""

    @pytest.fixture
    def yaml_agents(self) -> set[str]:
        """Load agent IDs from YAML files."""
        personas_dir = EXAMPLES_DIR / "personas"
        agent_ids = set()
        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            if "agent" in data and "id" in data["agent"]:
                agent_ids.add(data["agent"]["id"])
        return agent_ids

    @pytest.fixture
    def legacy_agents(self) -> set[str]:
        """Get all agents referenced in AgentRouter."""
        agents = set()
        # Agents from domain chains
        for config in DOMAIN_AGENT_MAP.values():
            agents.add(config.primary)
            agents.update(config.chain)
        # Agents from task type overrides
        agents.update(TASK_TYPE_AGENT_OVERRIDES.values())
        return agents

    def test_all_legacy_agents_covered(
        self, yaml_agents: set[str], legacy_agents: set[str]
    ) -> None:
        """All agents from AgentRouter should have YAML files."""
        missing = legacy_agents - yaml_agents
        assert not missing, f"Missing YAML files for agents: {missing}"

    def test_agent_count_matches(
        self, yaml_agents: set[str], legacy_agents: set[str]
    ) -> None:
        """YAML agent count should match or exceed legacy count."""
        assert len(yaml_agents) >= len(legacy_agents), (
            f"Expected at least {len(legacy_agents)} agents, found {len(yaml_agents)}"
        )


# ============================================================================
# Task Type Override Coverage Tests
# ============================================================================


class TestTaskTypeOverrideCoverage:
    """Tests that all task type overrides are documented in rules."""

    @pytest.fixture
    def yaml_task_overrides(self) -> dict[str, str]:
        """Load task type overrides from YAML rules."""
        rules_dir = EXAMPLES_DIR / "rules"
        overrides = {}

        for yaml_file in rules_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            if "rules" not in data:
                continue

            rules = data["rules"]
            if "overrides" not in rules:
                continue

            for override in rules["overrides"]:
                if "condition" not in override or "action" not in override:
                    continue
                condition = override["condition"]
                if "task_type" not in condition:
                    continue

                task_types = condition["task_type"]
                if isinstance(task_types, str):
                    task_types = [task_types]

                agent = override["action"].get("set_primary")
                if agent:
                    for task_type in task_types:
                        overrides[task_type] = agent

        return overrides

    def test_all_legacy_overrides_documented(
        self, yaml_task_overrides: dict[str, str]
    ) -> None:
        """All task type overrides from AgentRouter should be in YAML."""
        legacy_overrides = set(TASK_TYPE_AGENT_OVERRIDES.keys())
        yaml_overrides = set(yaml_task_overrides.keys())

        missing = legacy_overrides - yaml_overrides
        assert not missing, f"Missing YAML rules for task types: {missing}"

    def test_override_agents_match(
        self, yaml_task_overrides: dict[str, str]
    ) -> None:
        """YAML override agents should match legacy agents."""
        for task_type, yaml_agent in yaml_task_overrides.items():
            if task_type in TASK_TYPE_AGENT_OVERRIDES:
                legacy_agent = TASK_TYPE_AGENT_OVERRIDES[task_type]
                assert yaml_agent == legacy_agent, (
                    f"Agent mismatch for {task_type}: "
                    f"YAML={yaml_agent}, legacy={legacy_agent}"
                )


# ============================================================================
# Routing Consistency Tests
# ============================================================================


class TestRoutingConsistency:
    """Tests that routing produces consistent results."""

    @pytest.fixture
    def router(self) -> AgentRouter:
        """Create AgentRouter instance."""
        return AgentRouter()

    def test_domain_primary_agents_consistent(self, router: AgentRouter) -> None:
        """Domain primary agents should be consistent with YAML definitions."""
        domains_dir = EXAMPLES_DIR / "domains"

        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            if "domain" not in data:
                continue

            domain_id = data["domain"]["id"]

            # Get YAML preferred agent
            yaml_primary = None
            if "workflow" in data["domain"]:
                for step in data["domain"]["workflow"]:
                    if step.get("role") == "primary":
                        select = step.get("select", {})
                        yaml_primary = select.get("prefer_agent")
                        break

            if yaml_primary is None:
                continue

            # Get legacy primary
            legacy_config = router.get_recommended_agent(domain_id)
            legacy_primary = legacy_config.primary

            assert yaml_primary == legacy_primary, (
                f"Primary agent mismatch for {domain_id}: "
                f"YAML={yaml_primary}, legacy={legacy_primary}"
            )

    def test_task_type_routing_consistent(self, router: AgentRouter) -> None:
        """Task type routing should produce expected agents."""
        for task_type, expected_agent in TASK_TYPE_AGENT_OVERRIDES.items():
            actual_agent = router.get_agent_for_task_type(task_type)
            assert actual_agent == expected_agent, (
                f"Task type {task_type}: expected {expected_agent}, got {actual_agent}"
            )


# ============================================================================
# YAML Structure Tests
# ============================================================================


class TestYAMLStructure:
    """Tests that YAML files follow expected structure."""

    def test_domain_has_required_fields(self) -> None:
        """Domain YAML files should have required fields."""
        domains_dir = EXAMPLES_DIR / "domains"
        required_fields = {"id", "name"}

        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            domain = data.get("domain", {})
            missing = required_fields - set(domain.keys())
            assert not missing, (
                f"Missing required fields in {yaml_file.name}: {missing}"
            )

    def test_agent_has_required_fields(self) -> None:
        """Agent YAML files should have required fields."""
        personas_dir = EXAMPLES_DIR / "personas"
        required_fields = {"id", "name", "persona", "skills"}

        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            agent = data.get("agent", {})
            missing = required_fields - set(agent.keys())
            assert not missing, (
                f"Missing required fields in {yaml_file.name}: {missing}"
            )

    def test_agent_has_primary_skills(self) -> None:
        """Agent skills should have primary list."""
        personas_dir = EXAMPLES_DIR / "personas"

        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            agent = data.get("agent", {})
            skills = agent.get("skills", {})
            assert "primary" in skills, (
                f"Missing primary skills in {yaml_file.name}"
            )
            assert isinstance(skills["primary"], list), (
                f"Primary skills should be a list in {yaml_file.name}"
            )
