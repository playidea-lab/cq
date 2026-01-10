"""Shared test fixtures for C4 tests"""

import tempfile
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import CheckpointConfig, Task


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon(temp_project):
    """Create an initialized daemon (without default checkpoints for test isolation)"""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    return d


@pytest.fixture
def multi_worker_daemon(temp_project):
    """Create a daemon configured for multi-worker testing"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("test-project", with_default_checkpoints=False)
    # Set short lock TTL for testing
    daemon._config.scope_lock_ttl_sec = 60
    return daemon


@pytest.fixture
def daemon_with_checkpoints(temp_project):
    """Create a daemon with checkpoint configuration"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("test-project", with_default_checkpoints=False)

    # Add checkpoint configuration
    daemon._config.checkpoints = [
        CheckpointConfig(
            id="CP0",
            required_tasks=["T-001", "T-002"],
            required_validations=["lint", "unit"],
        )
    ]

    # Add validation commands
    daemon._config.validation.commands = {
        "lint": "echo 'lint passed'",
        "unit": "echo 'unit passed'",
    }
    daemon._config.validation.required = ["lint", "unit"]

    return daemon


@pytest.fixture
def daemon_in_execute(daemon):
    """Create a daemon in EXECUTE state with a task"""
    task = Task(id="T-001", title="Test Task", dod="Test definition of done")
    daemon.add_task(task)
    daemon.state_machine.transition("c4_run")
    return daemon
