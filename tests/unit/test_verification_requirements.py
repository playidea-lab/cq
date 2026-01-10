"""Tests for verification requirements in feature specs."""

from c4.discovery.models import Domain
from c4.discovery.specs import (
    FeatureSpec,
    VerificationRequirement,
)


class TestVerificationRequirement:
    """Tests for VerificationRequirement model."""

    def test_basic_creation(self):
        """Test basic verification requirement creation."""
        req = VerificationRequirement(
            type="http",
            name="API Health Check",
            reason="User requested health monitoring",
        )
        assert req.type == "http"
        assert req.name == "API Health Check"
        assert req.reason == "User requested health monitoring"
        assert req.priority == 2  # default
        assert req.enabled is True
        assert req.config == {}

    def test_with_config(self):
        """Test verification with configuration."""
        req = VerificationRequirement(
            type="metrics",
            name="Model Accuracy",
            reason="ML model needs accuracy check",
            config={
                "thresholds": {"accuracy": {"min": 0.95}},
            },
        )
        assert req.config["thresholds"]["accuracy"]["min"] == 0.95

    def test_from_user_request(self):
        """Test creation from user request."""
        req = VerificationRequirement.from_user_request(
            verification_type="browser",
            name="Login Flow E2E",
            reason="User wants login tested",
            url="http://localhost:3000",
            timeout=30000,
        )
        assert req.type == "browser"
        assert "User request:" in req.reason
        assert req.config["url"] == "http://localhost:3000"
        assert req.config["timeout"] == 30000

    def test_from_domain_default(self):
        """Test creation from domain default."""
        req = VerificationRequirement.from_domain_default(
            verification_type="http",
            name="API Health",
            domain="web-backend",
        )
        assert req.type == "http"
        assert "Domain default" in req.reason
        assert req.priority == 3  # lower priority for defaults


class TestFeatureSpecVerification:
    """Tests for verification requirements in FeatureSpec."""

    def test_add_verification(self):
        """Test adding verification to feature spec."""
        spec = FeatureSpec(
            feature="user-auth",
            domain=Domain.WEB_BACKEND,
        )

        verification = spec.add_verification(
            verification_type="http",
            name="Login API Response Time",
            reason="User requested performance verification",
            priority=1,
            url="/api/login",
            max_response_time=500,
        )

        assert len(spec.verification_requirements) == 1
        assert verification.type == "http"
        assert verification.name == "Login API Response Time"
        assert verification.priority == 1
        assert verification.config["url"] == "/api/login"
        assert verification.config["max_response_time"] == 500

    def test_add_multiple_verifications(self):
        """Test adding multiple verifications."""
        spec = FeatureSpec(
            feature="dashboard",
            domain=Domain.WEB_FRONTEND,
        )

        spec.add_verification(
            verification_type="browser",
            name="Dashboard Load",
            reason="User wants E2E test",
        )
        spec.add_verification(
            verification_type="visual",
            name="Dashboard Screenshot",
            reason="Visual regression check",
        )

        assert len(spec.verification_requirements) == 2

    def test_get_verifications_for_config(self):
        """Test exporting verifications in config format."""
        spec = FeatureSpec(
            feature="api",
            domain=Domain.WEB_BACKEND,
        )

        spec.add_verification(
            verification_type="http",
            name="Health Check",
            reason="Standard health check",
            url="/health",
            expected_status=200,
        )
        spec.add_verification(
            verification_type="cli",
            name="Server Start",
            reason="Verify server starts",
            command="python -m api --help",
        )

        config_list = spec.get_verifications_for_config()

        assert len(config_list) == 2
        assert config_list[0]["type"] == "http"
        assert config_list[0]["name"] == "Health Check"
        assert config_list[0]["config"]["url"] == "/health"
        assert config_list[1]["type"] == "cli"

    def test_disabled_verification_excluded_from_config(self):
        """Test that disabled verifications are excluded."""
        spec = FeatureSpec(
            feature="test",
            domain=Domain.WEB_BACKEND,
        )

        spec.add_verification(
            verification_type="http",
            name="Active Check",
            reason="Active",
        )

        # Manually disable one
        disabled_req = VerificationRequirement(
            type="http",
            name="Disabled Check",
            reason="Disabled",
            enabled=False,
        )
        spec.verification_requirements.append(disabled_req)

        config_list = spec.get_verifications_for_config()
        assert len(config_list) == 1
        assert config_list[0]["name"] == "Active Check"

    def test_yaml_serialization_with_verifications(self):
        """Test YAML serialization includes verifications."""
        spec = FeatureSpec(
            feature="ml-model",
            domain=Domain.ML_DL,
        )

        spec.add_verification(
            verification_type="metrics",
            name="Model Accuracy",
            reason="User requested accuracy validation",
            thresholds={"accuracy": {"min": 0.95}},
        )

        yaml_str = spec.to_yaml()
        assert "verification_requirements" in yaml_str
        assert "metrics" in yaml_str
        assert "Model Accuracy" in yaml_str

    def test_yaml_roundtrip(self):
        """Test YAML serialization and deserialization."""
        spec = FeatureSpec(
            feature="roundtrip-test",
            domain=Domain.WEB_FRONTEND,
        )
        spec.add_verification(
            verification_type="browser",
            name="E2E Test",
            reason="Testing roundtrip",
            url="http://localhost:3000",
        )

        yaml_str = spec.to_yaml()
        loaded = FeatureSpec.from_yaml(yaml_str)

        assert len(loaded.verification_requirements) == 1
        assert loaded.verification_requirements[0].type == "browser"
        assert loaded.verification_requirements[0].name == "E2E Test"
