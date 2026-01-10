"""Tests for C4 MCP Discovery & Specification Tools."""

import tempfile
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon_in_discovery(temp_project):
    """Create a daemon in DISCOVERY state."""
    daemon = C4Daemon(temp_project)
    daemon.initialize(with_default_checkpoints=False)
    # Transition to DISCOVERY
    daemon.state_machine.transition("c4_init")
    return daemon


@pytest.fixture
def daemon_initialized(temp_project):
    """Create an initialized daemon in DISCOVERY state."""
    daemon = C4Daemon(temp_project)
    daemon.initialize()  # Goes to DISCOVERY state after c4_init
    return daemon


@pytest.fixture
def daemon_in_plan(temp_project):
    """Create a daemon in PLAN state (skipped discovery)."""
    daemon = C4Daemon(temp_project)
    daemon.initialize()  # Goes to DISCOVERY
    daemon.state_machine.transition("skip_discovery")  # DISCOVERY → PLAN
    return daemon


class TestC4SaveSpec:
    """Test c4_save_spec MCP tool."""

    def test_save_spec_success(self, daemon_initialized):
        """Test saving a feature specification."""
        result = daemon_initialized.c4_save_spec(
            feature="user-auth",
            requirements=[
                {
                    "id": "REQ-001",
                    "pattern": "event-driven",
                    "text": "When user submits login form, the system shall validate credentials",
                },
                {
                    "id": "REQ-002",
                    "pattern": "unwanted",
                    "text": "If credentials are invalid, the system shall display error message",
                },
            ],
            domain="web-frontend",
            description="User authentication feature",
        )

        assert result["success"] is True
        assert result["feature"] == "user-auth"
        assert result["domain"] == "web-frontend"
        assert result["requirements_count"] == 2
        assert "file_path" in result

        # Verify file was created
        file_path = Path(result["file_path"])
        assert file_path.exists()

    def test_save_spec_with_default_pattern(self, daemon_initialized):
        """Test saving spec with default ubiquitous pattern."""
        result = daemon_initialized.c4_save_spec(
            feature="dashboard",
            requirements=[
                {
                    "id": "REQ-001",
                    "text": "The system shall display user data",
                },
            ],
            domain="web-frontend",
        )

        assert result["success"] is True
        assert result["requirements_count"] == 1

    def test_save_spec_invalid_domain(self, daemon_initialized):
        """Test saving spec with invalid domain."""
        result = daemon_initialized.c4_save_spec(
            feature="test-feature",
            requirements=[{"id": "REQ-001", "text": "Test requirement"}],
            domain="invalid-domain",
        )

        assert result["success"] is False
        assert "Invalid domain" in result["error"]

    def test_save_spec_ml_domain(self, daemon_initialized):
        """Test saving spec with ML/DL domain."""
        result = daemon_initialized.c4_save_spec(
            feature="model-training",
            requirements=[
                {
                    "id": "REQ-001",
                    "pattern": "ubiquitous",
                    "text": "The model shall achieve accuracy >= 0.9 on test set",
                },
            ],
            domain="ml-dl",
            description="Model training requirements",
        )

        assert result["success"] is True
        assert result["domain"] == "ml-dl"


class TestC4ListSpecs:
    """Test c4_list_specs MCP tool."""

    def test_list_specs_empty(self, daemon_initialized):
        """Test listing specs when none exist."""
        result = daemon_initialized.c4_list_specs()

        assert result["success"] is True
        assert result["count"] == 0
        assert result["features"] == []

    def test_list_specs_with_features(self, daemon_initialized):
        """Test listing multiple specs."""
        # Save some specs
        daemon_initialized.c4_save_spec(
            feature="feature-a",
            requirements=[{"id": "REQ-001", "text": "Feature A requirement"}],
            domain="web-frontend",
            description="Feature A",
        )
        daemon_initialized.c4_save_spec(
            feature="feature-b",
            requirements=[{"id": "REQ-001", "text": "Feature B requirement"}],
            domain="web-backend",
            description="Feature B",
        )

        result = daemon_initialized.c4_list_specs()

        assert result["success"] is True
        assert result["count"] == 2
        assert len(result["features"]) == 2

        # Check feature names are present
        feature_names = [f["feature"] for f in result["features"]]
        assert "feature-a" in feature_names
        assert "feature-b" in feature_names


class TestC4GetSpec:
    """Test c4_get_spec MCP tool."""

    def test_get_spec_success(self, daemon_initialized):
        """Test getting a specific feature spec."""
        # Save a spec first
        daemon_initialized.c4_save_spec(
            feature="user-auth",
            requirements=[
                {
                    "id": "REQ-001",
                    "pattern": "event-driven",
                    "text": "When user submits login form, the system shall authenticate",
                },
            ],
            domain="web-frontend",
            description="Authentication",
        )

        result = daemon_initialized.c4_get_spec("user-auth")

        assert result["success"] is True
        assert result["feature"] == "user-auth"
        assert result["domain"] == "web-frontend"
        assert result["description"] == "Authentication"
        assert len(result["requirements"]) == 1
        assert result["requirements"][0]["id"] == "REQ-001"
        assert result["requirements"][0]["pattern"] == "event-driven"

    def test_get_spec_not_found(self, daemon_initialized):
        """Test getting a non-existent feature spec."""
        result = daemon_initialized.c4_get_spec("non-existent")

        assert result["success"] is False
        assert "not found" in result["error"]


class TestC4DiscoveryComplete:
    """Test c4_discovery_complete MCP tool."""

    def test_discovery_complete_success(self, daemon_initialized):
        """Test completing discovery with specs saved."""
        # daemon_initialized is in DISCOVERY state after c4_init

        # Save a spec first
        daemon_initialized.c4_save_spec(
            feature="test-feature",
            requirements=[{"id": "REQ-001", "text": "Test requirement"}],
            domain="web-frontend",
        )

        # Complete discovery
        result = daemon_initialized.c4_discovery_complete()

        assert result["success"] is True
        assert result["previous_status"] == "DISCOVERY"  # Uppercase from enum
        assert result["new_status"] == "DESIGN"
        assert result["specs_count"] == 1

    def test_discovery_complete_not_in_discovery_state(self, daemon_in_plan):
        """Test completing discovery when not in DISCOVERY state."""
        # daemon_in_plan is in PLAN state (skipped discovery)
        result = daemon_in_plan.c4_discovery_complete()

        assert result["success"] is False
        assert "Not in DISCOVERY state" in result["error"]

    def test_discovery_complete_no_specs(self, daemon_initialized):
        """Test completing discovery without any specs."""
        # daemon_initialized is in DISCOVERY state, but no specs saved

        # Try to complete without specs
        result = daemon_initialized.c4_discovery_complete()

        assert result["success"] is False
        assert "No specifications found" in result["error"]


class TestSpecStoreProperty:
    """Test spec_store property."""

    def test_spec_store_lazy_init(self, daemon_initialized):
        """Test that spec_store is lazily initialized."""
        # Access spec_store
        store = daemon_initialized.spec_store

        assert store is not None
        assert store.c4_dir == daemon_initialized.c4_dir
        assert store.specs_dir == daemon_initialized.c4_dir / "specs"

    def test_spec_store_singleton(self, daemon_initialized):
        """Test that spec_store returns same instance."""
        store1 = daemon_initialized.spec_store
        store2 = daemon_initialized.spec_store

        assert store1 is store2


class TestSpecStoreIntegration:
    """Integration tests for spec storage."""

    def test_spec_roundtrip(self, daemon_initialized):
        """Test saving and loading a spec through MCP tools."""
        # Save
        save_result = daemon_initialized.c4_save_spec(
            feature="roundtrip-test",
            requirements=[
                {
                    "id": "REQ-001",
                    "pattern": "event-driven",
                    "text": "When event occurs, the system shall respond",
                },
                {
                    "id": "REQ-002",
                    "pattern": "state-driven",
                    "text": "While loading, the system shall show spinner",
                },
            ],
            domain="web-frontend",
            description="Roundtrip test feature",
        )
        assert save_result["success"] is True

        # Load
        get_result = daemon_initialized.c4_get_spec("roundtrip-test")
        assert get_result["success"] is True
        assert len(get_result["requirements"]) == 2
        assert get_result["requirements"][0]["pattern"] == "event-driven"
        assert get_result["requirements"][1]["pattern"] == "state-driven"

        # List
        list_result = daemon_initialized.c4_list_specs()
        assert list_result["success"] is True
        assert list_result["count"] >= 1
        assert any(f["feature"] == "roundtrip-test" for f in list_result["features"])
