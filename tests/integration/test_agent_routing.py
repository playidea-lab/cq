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
