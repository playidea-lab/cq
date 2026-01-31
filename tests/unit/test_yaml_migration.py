"""Tests for T-161: Full agent YAML migration validation.

Validates that YAML-defined agents produce the same routing results
as the existing DOMAIN_AGENT_MAP and TASK_TYPE_AGENT_OVERRIDES.
"""

from pathlib import Path

from c4.supervisor._legacy.agent_router import (
    DOMAIN_AGENT_MAP,
    TASK_TYPE_AGENT_OVERRIDES,
)


class TestYAMLFilesExist:
    """Test that all required YAML files were generated."""

    AGENTS_DIR = Path(".c4/agents")

    def test_domains_directory_has_files(self):
        """Domains directory should have YAML files for each domain."""
        domains_dir = self.AGENTS_DIR / "domains"
        yaml_files = list(domains_dir.glob("*.yaml"))

        # Should have at least the main domains
        assert len(yaml_files) >= 10, f"Expected at least 10 domain files, got {len(yaml_files)}"

        # Check key domains exist
        domain_names = {f.stem for f in yaml_files}
        assert "web-frontend" in domain_names
        assert "web-backend" in domain_names
        assert "ml-dl" in domain_names
        assert "devops" in domain_names

    def test_personas_directory_has_files(self):
        """Personas directory should have YAML files for each agent."""
        personas_dir = self.AGENTS_DIR / "personas"
        yaml_files = list(personas_dir.glob("*.yaml"))

        # Should have at least 20 agents
        assert len(yaml_files) >= 20, f"Expected at least 20 agent files, got {len(yaml_files)}"

        # Check key agents exist
        agent_names = {f.stem for f in yaml_files}
        assert "frontend-developer" in agent_names
        assert "backend-architect" in agent_names
        assert "debugger" in agent_names
        assert "code-reviewer" in agent_names

    def test_rules_directory_has_overrides(self):
        """Rules directory should have task override rules."""
        rules_dir = self.AGENTS_DIR / "rules"
        yaml_files = list(rules_dir.glob("*.yaml"))

        assert len(yaml_files) >= 1, "Expected at least 1 rules file"
        assert any("override" in f.stem for f in yaml_files)


class TestYAMLFormatValidity:
    """Test that YAML files are valid and parseable."""

    AGENTS_DIR = Path(".c4/agents")

    def test_domain_yaml_is_valid(self):
        """All domain YAML files should be valid."""
        import yaml

        domains_dir = self.AGENTS_DIR / "domains"
        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            assert "domain" in data, f"{yaml_file} missing 'domain' key"
            assert "id" in data["domain"], f"{yaml_file} missing domain.id"
            assert "name" in data["domain"], f"{yaml_file} missing domain.name"
            assert "workflow" in data["domain"], f"{yaml_file} missing domain.workflow"

    def test_persona_yaml_is_valid(self):
        """All persona YAML files should be valid."""
        import yaml

        personas_dir = self.AGENTS_DIR / "personas"
        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            assert "agent" in data, f"{yaml_file} missing 'agent' key"
            assert "id" in data["agent"], f"{yaml_file} missing agent.id"
            assert "name" in data["agent"], f"{yaml_file} missing agent.name"
            assert "persona" in data["agent"], f"{yaml_file} missing agent.persona"

    def test_rules_yaml_is_valid(self):
        """All rules YAML files should be valid."""
        import yaml

        rules_dir = self.AGENTS_DIR / "rules"
        for yaml_file in rules_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            assert "rules" in data, f"{yaml_file} missing 'rules' key"


class TestRoutingCompatibility:
    """Test that YAML-based routing matches legacy routing."""

    def test_all_domains_covered(self):
        """All domains in DOMAIN_AGENT_MAP should have YAML files."""
        domains_dir = Path(".c4/agents/domains")
        yaml_domains = {f.stem for f in domains_dir.glob("*.yaml")}

        for domain in DOMAIN_AGENT_MAP.keys():
            assert domain in yaml_domains, f"Domain {domain} missing YAML file"

    def test_all_agents_covered(self):
        """All agents referenced in routing should have YAML files."""
        personas_dir = Path(".c4/agents/personas")
        yaml_agents = {f.stem for f in personas_dir.glob("*.yaml")}

        # Collect all agents from domain chains
        all_agents = set()
        for config in DOMAIN_AGENT_MAP.values():
            all_agents.update(config.chain)

        # Add agents from task overrides
        all_agents.update(TASK_TYPE_AGENT_OVERRIDES.values())

        for agent in all_agents:
            assert agent in yaml_agents, f"Agent {agent} missing YAML file"

    def test_yaml_domain_primary_matches_legacy(self):
        """Domain primary agents in YAML should match legacy config."""
        import yaml

        domains_dir = Path(".c4/agents/domains")

        for domain, legacy_config in DOMAIN_AGENT_MAP.items():
            yaml_file = domains_dir / f"{domain}.yaml"
            if not yaml_file.exists():
                continue

            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            # Get primary from workflow step 1
            workflow = data["domain"]["workflow"]
            primary_step = workflow[0]
            yaml_primary = primary_step["select"]["prefer_agent"]

            assert yaml_primary == legacy_config.primary, (
                f"Domain {domain}: YAML primary {yaml_primary} != legacy {legacy_config.primary}"
            )


class TestAgentGraphLoading:
    """Test that YAML files can be loaded into AgentGraph."""

    def test_load_personas_into_graph(self):
        """Agent personas should load into AgentGraph."""
        import yaml

        from c4.supervisor.agent_graph.graph import AgentGraph
        from c4.supervisor.agent_graph.models import AgentDefinition

        graph = AgentGraph()
        personas_dir = Path(".c4/agents/personas")

        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            agent_def = AgentDefinition.model_validate(data)
            graph.add_agent(agent_def)

        # Should have loaded all agents
        assert len(graph.agents) >= 20

    def test_load_domains_into_graph(self):
        """Domain definitions should load into AgentGraph."""
        import yaml

        from c4.supervisor.agent_graph.graph import AgentGraph
        from c4.supervisor.agent_graph.models import AgentDefinition, DomainDefinition

        graph = AgentGraph()

        # Load personas first (domains reference them)
        personas_dir = Path(".c4/agents/personas")
        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            agent_def = AgentDefinition.model_validate(data)
            graph.add_agent(agent_def)

        # Load domains
        domains_dir = Path(".c4/agents/domains")
        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)

            domain_def = DomainDefinition.model_validate(data)
            graph.add_domain(domain_def)

        # Should have loaded all domains
        assert len(graph.domains) >= 10

    def test_loaded_graph_finds_agents_for_domain(self):
        """Loaded graph should find correct agents for domains."""
        import yaml

        from c4.supervisor.agent_graph.graph import AgentGraph
        from c4.supervisor.agent_graph.models import AgentDefinition, DomainDefinition

        graph = AgentGraph()

        # Load personas first
        personas_dir = Path(".c4/agents/personas")
        for yaml_file in personas_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            agent_def = AgentDefinition.model_validate(data)
            graph.add_agent(agent_def)

        # Load domains
        domains_dir = Path(".c4/agents/domains")
        for yaml_file in domains_dir.glob("*.yaml"):
            with open(yaml_file) as f:
                data = yaml.safe_load(f)
            domain_def = DomainDefinition.model_validate(data)
            graph.add_domain(domain_def)

        # Test web-frontend domain
        domain_info = graph.find_agents_for_domain("web-frontend")
        assert domain_info["primary"] == "frontend-developer"

        # Test web-backend domain
        domain_info = graph.find_agents_for_domain("web-backend")
        assert domain_info["primary"] == "backend-architect"
