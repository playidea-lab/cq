"""Integration tests for Phase 4: Agent Routing"""

import tempfile
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import Task, ValidationConfig


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def agent_routing_daemon(temp_project):
    """Create daemon configured for agent routing testing"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("agent-routing-test", with_default_checkpoints=False)

    # Skip discovery phase to go directly to PLAN for testing
    daemon.state_machine.transition("skip_discovery")

    daemon._config.validation = ValidationConfig(
        commands={
            "lint": "echo 'ok'",
            "unit": "echo 'ok'",
        },
        required=["lint", "unit"],
    )
    daemon._save_config()

    return daemon


class TestAgentRoutingIntegration:
    """Integration tests for agent routing in c4_get_task()"""

    def test_get_task_returns_agent_routing_info(self, agent_routing_daemon):
        """Test that c4_get_task returns agent routing information"""
        daemon = agent_routing_daemon

        # Add a task
        task = Task(
            id="T-001",
            title="Create login form",
            dod="Form validates email and password",
            scope="auth",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        # Get task
        result = daemon.c4_get_task("worker-test-001")

        # Verify agent routing fields are present
        assert result is not None
        assert result.task_id == "T-001"
        assert result.recommended_agent is not None
        assert result.agent_chain is not None
        assert isinstance(result.agent_chain, list)
        assert len(result.agent_chain) >= 1

    def test_get_task_with_web_frontend_domain(self, agent_routing_daemon):
        """Test agent routing for web-frontend domain"""
        daemon = agent_routing_daemon

        # Add a task with web-frontend domain
        task = Task(
            id="T-002",
            title="Add React component",
            dod="Component renders correctly",
            scope="frontend",
            domain="web-frontend",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-002")

        assert result is not None
        assert result.domain == "web-frontend"
        assert result.recommended_agent == "frontend-developer"
        assert "frontend-developer" in result.agent_chain
        assert "test-automator" in result.agent_chain
        assert "code-reviewer" in result.agent_chain

    def test_get_task_with_ml_domain(self, agent_routing_daemon):
        """Test agent routing for ml-dl domain"""
        daemon = agent_routing_daemon

        # Add a task with ml-dl domain
        task = Task(
            id="T-003",
            title="Train baseline model",
            dod="Model achieves 85% accuracy",
            scope="ml",
            domain="ml-dl",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-003")

        assert result is not None
        assert result.domain == "ml-dl"
        assert result.recommended_agent == "ml-engineer"
        assert "ml-engineer" in result.agent_chain
        assert "python-pro" in result.agent_chain

    def test_get_task_with_project_config_domain(self, temp_project):
        """Test that project config domain is used when task has no domain"""
        daemon = C4Daemon(temp_project)
        daemon.initialize("config-domain-test", with_default_checkpoints=False)

        # Skip discovery phase
        daemon.state_machine.transition("skip_discovery")

        # Set project-level domain in config
        daemon._config.domain = "web-backend"
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo 'ok'", "unit": "echo 'ok'"},
            required=["lint", "unit"],
        )
        daemon._save_config()

        # Add task without domain (should use config domain)
        task = Task(
            id="T-004",
            title="Create API endpoint",
            dod="Endpoint returns JSON",
            scope="api",
            domain=None,  # No task-level domain
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-004")

        assert result is not None
        # Should use project config domain
        assert result.domain == "web-backend"
        assert result.recommended_agent == "backend-architect"

    def test_task_domain_overrides_config_domain(self, temp_project):
        """Test that task-level domain takes precedence over config domain"""
        daemon = C4Daemon(temp_project)
        daemon.initialize("override-domain-test", with_default_checkpoints=False)

        # Skip discovery phase
        daemon.state_machine.transition("skip_discovery")

        # Set project-level domain to web-backend
        daemon._config.domain = "web-backend"
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo 'ok'", "unit": "echo 'ok'"},
            required=["lint", "unit"],
        )
        daemon._save_config()

        # Add task with explicit domain (should override config)
        task = Task(
            id="T-005",
            title="Add UI component",
            dod="Component works",
            scope="ui",
            domain="web-frontend",  # Task-level domain
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-005")

        assert result is not None
        # Task domain should override config domain
        assert result.domain == "web-frontend"
        assert result.recommended_agent == "frontend-developer"

    def test_handoff_instructions_included(self, agent_routing_daemon):
        """Test that handoff instructions are included in response"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-006",
            title="Create API",
            dod="API works",
            scope="api",
            domain="web-backend",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-006")

        assert result is not None
        assert result.handoff_instructions is not None
        assert len(result.handoff_instructions) > 0
        # web-backend should mention API
        assert (
            "API" in result.handoff_instructions
            or "api" in result.handoff_instructions.lower()
        )

    def test_unknown_domain_uses_general_purpose(self, agent_routing_daemon):
        """Test that unknown domain falls back to general-purpose agent"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-007",
            title="Some task",
            dod="Task done",
            scope="misc",
            domain="some-unknown-domain",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-007")

        assert result is not None
        # Unknown domain should use general-purpose
        assert result.recommended_agent == "general-purpose"
        assert "general-purpose" in result.agent_chain

    def test_infra_domain_routing(self, agent_routing_daemon):
        """Test agent routing for infrastructure domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-008",
            title="Setup AWS infrastructure",
            dod="Terraform deployed",
            scope="infra",
            domain="infra",
        )
        daemon.add_task(task)

        # Start execution
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-test-008")

        assert result is not None
        assert result.domain == "infra"
        assert result.recommended_agent == "cloud-architect"
        assert "cloud-architect" in result.agent_chain
        assert "deployment-engineer" in result.agent_chain


class TestGraphRouterFallbackIntegration:
    """Integration tests for GraphRouter fallback to legacy AgentRouter."""

    def test_graph_router_fallback_matches_legacy(self):
        """GraphRouter without graph should match legacy AgentRouter exactly."""
        from c4.supervisor.agent_graph.router import GraphRouter
        from c4.supervisor.agent_router import AgentRouter

        legacy_router = AgentRouter()
        graph_router = GraphRouter()  # No graph - fallback mode

        # Test all known domains
        domains = ["web-frontend", "web-backend", "ml-dl", "infra", "library"]

        for domain in domains:
            legacy_config = legacy_router.get_recommended_agent(domain)
            graph_config = graph_router.get_recommended_agent(domain)

            assert graph_config.primary == legacy_config.primary, f"Primary mismatch for {domain}"
            assert graph_config.chain == legacy_config.chain, f"Chain mismatch for {domain}"

    def test_graph_router_task_type_fallback(self):
        """GraphRouter task type overrides should match legacy behavior."""
        from c4.supervisor.agent_graph.router import GraphRouter
        from c4.supervisor.agent_router import AgentRouter

        legacy_router = AgentRouter()
        graph_router = GraphRouter()

        # Test task type overrides
        task_types = ["debug", "security", "test", "deploy", "refactor"]

        for task_type in task_types:
            legacy_agent = legacy_router.get_agent_for_task_type(task_type)
            graph_agent = graph_router.get_agent_for_task_type(task_type)

            assert graph_agent == legacy_agent, f"Task type override mismatch for {task_type}"

    def test_graph_router_chain_fallback(self):
        """GraphRouter chain should match legacy for fallback domains."""
        from c4.supervisor.agent_graph.router import GraphRouter
        from c4.supervisor.agent_router import AgentRouter

        legacy_router = AgentRouter()
        graph_router = GraphRouter()

        domains = ["fullstack", "mobile-app", "firmware"]

        for domain in domains:
            legacy_chain = legacy_router.get_chain_for_domain(domain)
            graph_chain = graph_router.get_chain_for_domain(domain)

            assert graph_chain == legacy_chain, f"Chain mismatch for {domain}"

    def test_graph_router_all_domains_includes_legacy(self):
        """GraphRouter.get_all_domains() should include all legacy domains."""
        from c4.supervisor.agent_graph.router import GraphRouter
        from c4.supervisor.agent_router import AgentRouter

        legacy_router = AgentRouter()
        graph_router = GraphRouter()

        legacy_domains = set(legacy_router.get_all_domains())
        graph_domains = set(graph_router.get_all_domains())

        # All legacy domains should be in graph router
        assert legacy_domains.issubset(graph_domains), "Missing legacy domains"

    def test_graph_router_unknown_domain_fallback(self):
        """GraphRouter should handle unknown domain same as legacy."""
        from c4.supervisor.agent_graph.router import GraphRouter
        from c4.supervisor.agent_router import AgentRouter

        legacy_router = AgentRouter()
        graph_router = GraphRouter()

        legacy_config = legacy_router.get_recommended_agent("completely-unknown-domain")
        graph_config = graph_router.get_recommended_agent("completely-unknown-domain")

        assert graph_config.primary == legacy_config.primary
        assert graph_config.chain == legacy_config.chain


class TestNewDomainsIntegration:
    """Integration tests for newly added domains"""

    def test_fullstack_domain_routing(self, agent_routing_daemon):
        """Test agent routing for fullstack domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-101",
            title="Build full-stack feature",
            dod="API + React UI working",
            scope="feature",
            domain="fullstack",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-fullstack")

        assert result is not None
        assert result.domain == "fullstack"
        assert result.recommended_agent == "backend-architect"
        assert "backend-architect" in result.agent_chain
        assert "frontend-developer" in result.agent_chain

    def test_mobile_app_domain_routing(self, agent_routing_daemon):
        """Test agent routing for mobile-app domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-102",
            title="Create mobile login screen",
            dod="Login works on iOS and Android",
            scope="auth",
            domain="mobile-app",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-mobile")

        assert result is not None
        assert result.domain == "mobile-app"
        assert result.recommended_agent == "mobile-developer"
        assert "mobile-developer" in result.agent_chain

    def test_library_domain_routing(self, agent_routing_daemon):
        """Test agent routing for library domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-103",
            title="Create Python library",
            dod="Library published with docs",
            scope="lib",
            domain="library",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-library")

        assert result is not None
        assert result.domain == "library"
        assert result.recommended_agent == "python-pro"
        assert "api-documenter" in result.agent_chain

    def test_firmware_domain_routing(self, agent_routing_daemon):
        """Test agent routing for firmware domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-104",
            title="Write embedded driver",
            dod="Driver works on target board",
            scope="driver",
            domain="firmware",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-firmware")

        assert result is not None
        assert result.domain == "firmware"
        assert result.recommended_agent == "general-purpose"

    def test_data_science_domain_routing(self, agent_routing_daemon):
        """Test agent routing for data-science domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-105",
            title="Analyze sales data",
            dod="Analysis report generated",
            scope="analysis",
            domain="data-science",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-datascience")

        assert result is not None
        assert result.domain == "data-science"
        assert result.recommended_agent == "data-scientist"
        assert "data-scientist" in result.agent_chain
        assert "python-pro" in result.agent_chain

    def test_devops_domain_routing(self, agent_routing_daemon):
        """Test agent routing for devops domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-106",
            title="Setup CI/CD pipeline",
            dod="Pipeline deploys to staging",
            scope="cicd",
            domain="devops",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-devops")

        assert result is not None
        assert result.domain == "devops"
        assert result.recommended_agent == "deployment-engineer"
        assert "cloud-architect" in result.agent_chain
        assert "security-auditor" in result.agent_chain

    def test_api_domain_routing(self, agent_routing_daemon):
        """Test agent routing for api domain"""
        daemon = agent_routing_daemon

        task = Task(
            id="T-107",
            title="Design REST API",
            dod="API documented with OpenAPI",
            scope="api",
            domain="api",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-api")

        assert result is not None
        assert result.domain == "api"
        assert result.recommended_agent == "backend-architect"
        assert "api-documenter" in result.agent_chain


class TestCustomAgentConfigIntegration:
    """Integration tests for custom agent configuration from config.yaml"""

    def test_custom_domain_from_yaml_config(self, temp_project):
        """Test that custom domains from config.yaml are loaded"""
        from c4.models.config import AgentChainDef, AgentConfig

        daemon = C4Daemon(temp_project)
        daemon.initialize("custom-agents-test", with_default_checkpoints=False)
        daemon.state_machine.transition("skip_discovery")

        # Set custom agent configuration
        daemon._config.agents = AgentConfig(
            chains={
                "my-custom-domain": AgentChainDef(
                    primary="my-custom-agent",
                    chain=["my-custom-agent", "test-automator"],
                    handoff="Custom handoff instructions",
                )
            }
        )
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo 'ok'", "unit": "echo 'ok'"},
            required=["lint", "unit"],
        )
        daemon._save_config()

        # Reset agent router to pick up new config
        daemon._agent_router = None

        # Add task with custom domain
        task = Task(
            id="T-201",
            title="Custom domain task",
            dod="Task done",
            scope="custom",
            domain="my-custom-domain",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-custom")

        assert result is not None
        assert result.recommended_agent == "my-custom-agent"
        assert "my-custom-agent" in result.agent_chain

    def test_custom_domain_overrides_builtin(self, temp_project):
        """Test that custom config overrides built-in domains"""
        from c4.models.config import AgentChainDef, AgentConfig

        daemon = C4Daemon(temp_project)
        daemon.initialize("override-builtin-test", with_default_checkpoints=False)
        daemon.state_machine.transition("skip_discovery")

        # Override web-frontend with custom agent
        daemon._config.agents = AgentConfig(
            chains={
                "web-frontend": AgentChainDef(
                    primary="my-frontend-agent",
                    chain=["my-frontend-agent", "my-reviewer"],
                    handoff="My custom handoff",
                )
            }
        )
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo 'ok'", "unit": "echo 'ok'"},
            required=["lint", "unit"],
        )
        daemon._save_config()

        # Reset agent router to pick up new config
        daemon._agent_router = None

        # Add task with built-in domain
        task = Task(
            id="T-202",
            title="Override test",
            dod="Task done",
            scope="frontend",
            domain="web-frontend",
        )
        daemon.add_task(task)
        daemon.state_machine.transition("c4_run")

        result = daemon.c4_get_task("worker-override")

        assert result is not None
        # Should use custom agent, not built-in
        assert result.recommended_agent == "my-frontend-agent"
        assert "my-frontend-agent" in result.agent_chain

    def test_custom_task_override_from_config(self, temp_project):
        """Test that custom task overrides from config.yaml work"""
        from c4.models.config import AgentConfig

        daemon = C4Daemon(temp_project)
        daemon.initialize("task-override-test", with_default_checkpoints=False)
        daemon.state_machine.transition("skip_discovery")

        # Set custom task override
        daemon._config.agents = AgentConfig(
            task_overrides={
                "my-special-task": "my-special-agent",
            }
        )
        daemon._config.validation = ValidationConfig(
            commands={"lint": "echo 'ok'", "unit": "echo 'ok'"},
            required=["lint", "unit"],
        )
        daemon._save_config()

        # Reset agent router to pick up new config
        daemon._agent_router = None

        # Get agent for custom task type
        agent = daemon.agent_router.get_agent_for_task_type("my-special-task", "unknown")
        assert agent == "my-special-agent"


class TestMCPAgentRoutingTool:
    """Tests for c4_test_agent_routing MCP tool"""

    def test_tool_returns_all_domains_when_no_args(self, agent_routing_daemon):
        """Test that tool returns all domains when called without arguments"""
        daemon = agent_routing_daemon

        result = daemon.c4_test_agent_routing()

        assert "total_domains" in result
        assert result["total_domains"] >= 12  # 9 built-in + 3 new
        assert "domains" in result
        assert "web-frontend" in result["domains"]
        assert "data-science" in result["domains"]
        assert "task_type_overrides" in result
        assert "custom_config_loaded" in result

    def test_tool_returns_domain_details(self, agent_routing_daemon):
        """Test that tool returns details for specific domain"""
        daemon = agent_routing_daemon

        result = daemon.c4_test_agent_routing(domain="web-frontend")

        assert result["domain"] == "web-frontend"
        assert result["primary_agent"] == "frontend-developer"
        assert isinstance(result["agent_chain"], list)
        assert "frontend-developer" in result["agent_chain"]
        assert "handoff_instructions" in result

    def test_tool_returns_task_type_override(self, agent_routing_daemon):
        """Test that tool shows task type override"""
        daemon = agent_routing_daemon

        result = daemon.c4_test_agent_routing(domain="web-backend", task_type="debug")

        assert result["domain"] == "web-backend"
        assert result["task_type"] == "debug"
        assert result["overridden_agent"] == "debugger"
        assert result["is_override"] is True

    def test_tool_shows_no_override_when_task_matches_primary(self, agent_routing_daemon):
        """Test that is_override is False when no override applies"""
        daemon = agent_routing_daemon

        result = daemon.c4_test_agent_routing(
            domain="web-frontend", task_type="unknown-task"
        )

        assert result["domain"] == "web-frontend"
        assert result["overridden_agent"] == "frontend-developer"
        assert result["is_override"] is False

    def test_tool_new_domains(self, agent_routing_daemon):
        """Test that new domains are accessible via tool"""
        daemon = agent_routing_daemon

        # Test data-science
        result = daemon.c4_test_agent_routing(domain="data-science")
        assert result["primary_agent"] == "data-scientist"

        # Test devops
        result = daemon.c4_test_agent_routing(domain="devops")
        assert result["primary_agent"] == "deployment-engineer"

        # Test api
        result = daemon.c4_test_agent_routing(domain="api")
        assert result["primary_agent"] == "backend-architect"

    def test_tool_new_task_overrides(self, agent_routing_daemon):
        """Test that new task overrides work via tool"""
        daemon = agent_routing_daemon

        # Test api-design
        result = daemon.c4_test_agent_routing(domain="web-backend", task_type="api-design")
        assert result["overridden_agent"] == "backend-architect"

        # Test notebook
        result = daemon.c4_test_agent_routing(domain="ml-dl", task_type="notebook")
        assert result["overridden_agent"] == "data-scientist"

        # Test incident
        result = daemon.c4_test_agent_routing(domain="web-backend", task_type="incident")
        assert result["overridden_agent"] == "incident-responder"
