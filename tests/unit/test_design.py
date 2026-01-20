"""Tests for C4 Design System - models, storage, and MCP tools."""

import tempfile
from pathlib import Path

import pytest

from c4.discovery.design import (
    ArchitectureOption,
    ComponentDesign,
    DesignSpec,
    DesignStore,
)
from c4.discovery.models import Domain
from c4.mcp_server import C4Daemon
from c4.models import ProjectStatus


@pytest.fixture
def temp_project():
    """Create a temporary project directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def design_store(temp_project):
    """Create a design store for testing."""
    specs_dir = temp_project / ".c4" / "specs"
    specs_dir.mkdir(parents=True, exist_ok=True)
    return DesignStore(specs_dir)


@pytest.fixture
def daemon_in_design(temp_project):
    """Create a daemon in DESIGN state."""
    daemon = C4Daemon(temp_project)
    daemon.initialize()  # INIT → DISCOVERY

    # Save a spec to allow discovery completion
    daemon.c4_save_spec(
        feature="test-feature",
        requirements=[{"id": "REQ-001", "text": "Test requirement"}],
        domain="web-frontend",
    )

    # Complete discovery to enter DESIGN
    daemon.c4_discovery_complete()  # DISCOVERY → DESIGN

    assert daemon.state_machine.state.status == ProjectStatus.DESIGN
    return daemon


class TestDesignSpec:
    """Test DesignSpec model."""

    def test_create_design_spec(self):
        """Test creating a basic design spec."""
        spec = DesignSpec(
            feature="user-auth",
            domain=Domain.WEB_FRONTEND,
            description="User authentication feature",
        )

        assert spec.feature == "user-auth"
        assert spec.domain == Domain.WEB_FRONTEND
        assert spec.description == "User authentication feature"
        assert spec.architecture_options == []
        assert spec.components == []
        assert spec.decisions == []

    def test_add_architecture_option(self):
        """Test adding architecture options."""
        spec = DesignSpec(feature="auth", domain=Domain.WEB_BACKEND)

        option = ArchitectureOption(
            id="option-a",
            name="Session-based Auth",
            description="Traditional session cookies",
            complexity="low",
            pros=["Simple", "Stateful"],
            cons=["Not scalable", "Server-side sessions"],
            recommended=True,
        )
        spec.add_option(option)

        assert len(spec.architecture_options) == 1
        assert spec.architecture_options[0].id == "option-a"
        assert spec.architecture_options[0].recommended is True

    def test_select_option(self):
        """Test selecting an architecture option."""
        spec = DesignSpec(feature="auth", domain=Domain.WEB_BACKEND)

        spec.add_option(
            ArchitectureOption(id="option-a", name="Option A", description="First option")
        )
        spec.add_option(
            ArchitectureOption(id="option-b", name="Option B", description="Second option")
        )

        # Select valid option
        assert spec.select_option("option-b") is True
        assert spec.selected_option == "option-b"

        # Try to select invalid option
        assert spec.select_option("option-c") is False
        assert spec.selected_option == "option-b"  # Unchanged

    def test_add_component(self):
        """Test adding components."""
        spec = DesignSpec(feature="dashboard", domain=Domain.WEB_FRONTEND)

        comp = ComponentDesign(
            name="DashboardWidget",
            type="frontend",
            description="Reusable dashboard widget",
            responsibilities=["Display data", "Handle refresh"],
            dependencies=["React", "Chart.js"],
        )
        spec.add_component(comp)

        assert len(spec.components) == 1
        assert spec.components[0].name == "DashboardWidget"

    def test_add_decision(self):
        """Test adding design decisions."""
        spec = DesignSpec(feature="api", domain=Domain.WEB_BACKEND)

        dec = spec.add_decision(
            id="DEC-001",
            question="Which database to use?",
            decision="PostgreSQL",
            rationale="Better JSON support, ACID compliance",
            alternatives=["MongoDB", "MySQL"],
        )

        assert len(spec.decisions) == 1
        assert dec.id == "DEC-001"
        assert dec.decision == "PostgreSQL"
        assert "MongoDB" in dec.alternatives_considered

    def test_to_yaml(self):
        """Test YAML export."""
        spec = DesignSpec(
            feature="test",
            domain=Domain.WEB_FRONTEND,
            description="Test feature",
        )
        spec.add_option(ArchitectureOption(id="opt-1", name="Option 1", description="First"))

        yaml_str = spec.to_yaml()

        assert "feature: test" in yaml_str
        assert "domain: web-frontend" in yaml_str
        assert "opt-1" in yaml_str

    def test_to_markdown(self):
        """Test Markdown export."""
        spec = DesignSpec(
            feature="user-auth",
            domain=Domain.WEB_BACKEND,
            description="Authentication feature",
        )
        spec.add_option(
            ArchitectureOption(
                id="opt-1",
                name="JWT Auth",
                description="Token-based",
                recommended=True,
                pros=["Stateless"],
                cons=["Token size"],
            )
        )
        spec.select_option("opt-1")
        spec.mermaid_diagram = "sequenceDiagram\n  User->>API: Login"

        md = spec.to_markdown()

        assert "# Design: user-auth" in md
        assert "**Domain**: web-backend" in md
        assert "JWT Auth" in md
        assert "(Selected)" in md
        assert "(Recommended)" in md
        assert "```mermaid" in md

    def test_from_yaml(self):
        """Test loading from YAML."""
        spec = DesignSpec(
            feature="roundtrip",
            domain=Domain.ML_DL,
            description="Test roundtrip",
        )
        spec.add_option(ArchitectureOption(id="opt-1", name="Option 1", description="First"))
        spec.select_option("opt-1")

        yaml_str = spec.to_yaml()
        loaded = DesignSpec.from_yaml(yaml_str)

        assert loaded.feature == "roundtrip"
        assert loaded.domain == Domain.ML_DL
        assert loaded.selected_option == "opt-1"
        assert len(loaded.architecture_options) == 1


class TestDesignStore:
    """Test DesignStore storage."""

    def test_save_and_load(self, design_store):
        """Test saving and loading design spec."""
        spec = DesignSpec(
            feature="test-feature",
            domain=Domain.WEB_FRONTEND,
            description="Test",
        )
        spec.add_option(ArchitectureOption(id="opt-1", name="Option 1", description="First"))

        yaml_path, md_path = design_store.save(spec)

        assert yaml_path.exists()
        assert md_path.exists()
        assert yaml_path.name == "design.yaml"
        assert md_path.name == "design.md"

        loaded = design_store.load("test-feature")
        assert loaded is not None
        assert loaded.feature == "test-feature"
        assert len(loaded.architecture_options) == 1

    def test_load_nonexistent(self, design_store):
        """Test loading non-existent design."""
        result = design_store.load("nonexistent")
        assert result is None

    def test_exists(self, design_store):
        """Test checking if design exists."""
        assert design_store.exists("test") is False

        spec = DesignSpec(feature="test", domain=Domain.WEB_FRONTEND)
        design_store.save(spec)

        assert design_store.exists("test") is True

    def test_list_features_with_design(self, design_store):
        """Test listing features with designs."""
        assert design_store.list_features_with_design() == []

        design_store.save(DesignSpec(feature="feature-a", domain=Domain.WEB_FRONTEND))
        design_store.save(DesignSpec(feature="feature-b", domain=Domain.WEB_BACKEND))

        features = design_store.list_features_with_design()
        assert len(features) == 2
        assert "feature-a" in features
        assert "feature-b" in features

    def test_path_traversal_protection(self, design_store):
        """Test path traversal is blocked via normalization and resolve check."""
        # Empty names should fail
        with pytest.raises(ValueError, match="cannot be empty"):
            design_store.get_feature_dir("")

        with pytest.raises(ValueError, match="cannot be empty"):
            design_store.get_feature_dir("   ")

        # Path traversal attempts are normalized safely
        # "../etc/passwd" becomes "--etc-passwd" which is safe
        path = design_store.get_feature_dir("../etc/passwd")
        assert "--etc-passwd" in str(path)
        assert str(path).startswith(str(design_store.specs_dir.resolve()))

        # Absolute paths are normalized (/ becomes -)
        path = design_store.get_feature_dir("/absolute/path")
        assert "-absolute-path" in str(path)

        # Dots are normalized to dashes
        path = design_store.get_feature_dir(".")
        assert str(path).endswith("-")

        path = design_store.get_feature_dir("..")
        assert str(path).endswith("--")

        # Valid names should work
        path = design_store.get_feature_dir("valid-feature")
        assert "valid-feature" in str(path)


class TestC4SaveDesign:
    """Test c4_save_design MCP tool."""

    def test_save_design_success(self, daemon_in_design):
        """Test saving a design specification."""
        result = daemon_in_design.c4_save_design(
            feature="user-auth",
            domain="web-frontend",
            description="User authentication",
            options=[
                {
                    "id": "option-a",
                    "name": "Session Auth",
                    "description": "Cookie-based sessions",
                    "complexity": "low",
                    "recommended": True,
                },
                {
                    "id": "option-b",
                    "name": "JWT Auth",
                    "description": "Token-based auth",
                    "complexity": "medium",
                },
            ],
            selected_option="option-a",
            components=[
                {
                    "name": "LoginForm",
                    "type": "frontend",
                    "description": "Login UI component",
                    "responsibilities": ["Collect credentials", "Submit form"],
                },
            ],
            decisions=[
                {
                    "id": "DEC-001",
                    "question": "Which auth method?",
                    "decision": "Session-based",
                    "rationale": "Simpler for our use case",
                },
            ],
        )

        assert result["success"] is True
        assert result["feature"] == "user-auth"
        assert result["options_count"] == 2
        assert result["components_count"] == 1
        assert result["decisions_count"] == 1
        assert "yaml_path" in result
        assert "md_path" in result

    def test_save_design_minimal(self, daemon_in_design):
        """Test saving minimal design."""
        result = daemon_in_design.c4_save_design(
            feature="simple-feature",
            domain="web-backend",
        )

        assert result["success"] is True
        assert result["options_count"] == 0
        assert result["components_count"] == 0

    def test_save_design_with_diagram(self, daemon_in_design):
        """Test saving design with mermaid diagram."""
        result = daemon_in_design.c4_save_design(
            feature="api-flow",
            domain="web-backend",
            mermaid_diagram="""sequenceDiagram
    User->>API: POST /login
    API->>DB: Check credentials
    DB-->>API: User data
    API-->>User: JWT token""",
        )

        assert result["success"] is True

        # Verify diagram was saved
        spec = daemon_in_design.design_store.load("api-flow")
        assert spec is not None
        assert "sequenceDiagram" in spec.mermaid_diagram

    def test_save_design_invalid_domain(self, daemon_in_design):
        """Test saving with invalid domain."""
        result = daemon_in_design.c4_save_design(
            feature="test",
            domain="invalid-domain",
        )

        assert result["success"] is False
        assert "Invalid domain" in result["error"]


class TestC4GetDesign:
    """Test c4_get_design MCP tool."""

    def test_get_design_success(self, daemon_in_design):
        """Test getting a saved design."""
        # Save first
        daemon_in_design.c4_save_design(
            feature="get-test",
            domain="web-frontend",
            description="Test feature",
            options=[{"id": "opt-1", "name": "Option 1", "description": "First"}],
            selected_option="opt-1",
        )

        # Get
        result = daemon_in_design.c4_get_design("get-test")

        assert result["success"] is True
        assert result["feature"] == "get-test"
        assert result["domain"] == "web-frontend"
        assert result["selected_option"] == "opt-1"
        assert len(result["options"]) == 1

    def test_get_design_not_found(self, daemon_in_design):
        """Test getting non-existent design."""
        result = daemon_in_design.c4_get_design("nonexistent")

        assert result["success"] is False
        assert "not found" in result["error"]


class TestC4ListDesigns:
    """Test c4_list_designs MCP tool."""

    def test_list_designs_empty(self, daemon_in_design):
        """Test listing when no designs exist."""
        result = daemon_in_design.c4_list_designs()

        assert result["success"] is True
        assert result["count"] == 0
        assert result["designs"] == []

    def test_list_designs_with_data(self, daemon_in_design):
        """Test listing multiple designs."""
        daemon_in_design.c4_save_design(
            feature="feature-a",
            domain="web-frontend",
            options=[{"id": "opt-1", "name": "Opt 1", "description": "First"}],
            selected_option="opt-1",
        )
        daemon_in_design.c4_save_design(
            feature="feature-b",
            domain="web-backend",
        )

        result = daemon_in_design.c4_list_designs()

        assert result["success"] is True
        assert result["count"] == 2

        features = [d["feature"] for d in result["designs"]]
        assert "feature-a" in features
        assert "feature-b" in features


class TestC4DesignComplete:
    """Test c4_design_complete MCP tool."""

    def test_design_complete_success(self, daemon_in_design):
        """Test completing design phase."""
        # Save a design with selected option
        daemon_in_design.c4_save_design(
            feature="complete-test",
            domain="web-frontend",
            options=[{"id": "opt-1", "name": "Option 1", "description": "First"}],
            selected_option="opt-1",
        )

        result = daemon_in_design.c4_design_complete()

        assert result["success"] is True
        assert result["previous_status"] == "DESIGN"
        assert result["new_status"] == "PLAN"
        assert result["designs_count"] == 1

    def test_design_complete_no_designs(self, daemon_in_design):
        """Test completing without any designs."""
        result = daemon_in_design.c4_design_complete()

        assert result["success"] is False
        assert "No design specifications found" in result["error"]

    def test_design_complete_missing_selection(self, daemon_in_design):
        """Test completing with unselected options."""
        # Save design with options but no selection
        daemon_in_design.c4_save_design(
            feature="incomplete-design",
            domain="web-frontend",
            options=[{"id": "opt-1", "name": "Option 1", "description": "First"}],
            # No selected_option!
        )

        result = daemon_in_design.c4_design_complete()

        assert result["success"] is False
        assert "without selected option" in result["error"]

    def test_design_complete_not_in_design_state(self, temp_project):
        """Test completing when not in DESIGN state."""
        daemon = C4Daemon(temp_project)
        daemon.initialize()  # DISCOVERY state

        result = daemon.c4_design_complete()

        assert result["success"] is False
        assert "Not in DESIGN state" in result["error"]


class TestDesignStoreProperty:
    """Test design_store property."""

    def test_design_store_lazy_init(self, daemon_in_design):
        """Test that design_store is lazily initialized."""
        store = daemon_in_design.design_store

        assert store is not None
        assert store.specs_dir == daemon_in_design.c4_dir / "specs"

    def test_design_store_singleton(self, daemon_in_design):
        """Test that design_store returns same instance."""
        store1 = daemon_in_design.design_store
        store2 = daemon_in_design.design_store

        assert store1 is store2
