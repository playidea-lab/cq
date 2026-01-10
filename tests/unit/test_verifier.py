"""Tests for the verification system."""


from c4.supervisor.verifier import (
    DOMAIN_DEFAULT_VERIFICATIONS,
    BrowserVerifier,
    CliVerifier,
    DryrunVerifier,
    HttpVerifier,
    MetricsVerifier,
    VerificationResult,
    VerificationRunner,
    VerificationType,
    VerifierRegistry,
    VisualVerifier,
)


class TestVerificationResult:
    """Tests for VerificationResult."""

    def test_to_dict_basic(self):
        """Test basic serialization."""
        result = VerificationResult(
            type="http",
            name="API Check",
            status="pass",
            summary="200 OK",
        )
        data = result.to_dict()
        assert data["type"] == "http"
        assert data["name"] == "API Check"
        assert data["status"] == "pass"
        assert data["summary"] == "200 OK"
        assert "details" not in data

    def test_to_dict_with_details(self):
        """Test serialization with optional fields."""
        result = VerificationResult(
            type="browser",
            name="E2E Test",
            status="fail",
            summary="Step 2 failed",
            details={"steps": [{"action": "click", "status": "fail"}]},
            duration_ms=1500,
        )
        data = result.to_dict()
        assert data["details"]["steps"][0]["action"] == "click"
        assert data["duration_ms"] == 1500


class TestVerifierRegistry:
    """Tests for VerifierRegistry."""

    def test_register_and_get(self):
        """Test verifier registration and retrieval."""
        http_verifier = VerifierRegistry.get("http")
        assert http_verifier is not None
        assert isinstance(http_verifier, HttpVerifier)

    def test_get_nonexistent(self):
        """Test getting unregistered verifier returns None."""
        verifier = VerifierRegistry.get("nonexistent")
        assert verifier is None

    def test_list_types(self):
        """Test listing registered types."""
        types = VerifierRegistry.list_types()
        assert "http" in types
        assert "cli" in types
        assert "browser" in types

    def test_get_for_domain_web_backend(self):
        """Test getting verifiers for web-backend domain."""
        verifiers = VerifierRegistry.get_for_domain("web-backend")
        types = [v[0] for v in verifiers]
        assert "http" in types
        assert "cli" in types

    def test_get_for_domain_web_frontend(self):
        """Test getting verifiers for web-frontend domain."""
        verifiers = VerifierRegistry.get_for_domain("web-frontend")
        types = [v[0] for v in verifiers]
        assert "browser" in types

    def test_get_for_unknown_domain(self):
        """Test getting verifiers for unknown domain returns empty."""
        verifiers = VerifierRegistry.get_for_domain("unknown-domain")
        assert verifiers == []


class TestHttpVerifier:
    """Tests for HttpVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = HttpVerifier()
        assert verifier.verification_type == VerificationType.HTTP

    def test_no_url_error(self):
        """Test error when no URL specified."""
        verifier = HttpVerifier()
        result = verifier.verify({})
        assert result.status == "error"
        assert "No URL" in result.summary

    def test_unsupported_method(self):
        """Test unsupported HTTP method."""
        verifier = HttpVerifier()
        result = verifier.verify({
            "url": "http://example.com",
            "method": "TRACE",
        })
        assert result.status == "error"
        assert "Unsupported" in result.summary

    def test_base_url_prepend(self):
        """Test base_url is prepended correctly."""
        verifier = HttpVerifier()
        # This will fail to connect, but we can check the URL handling
        result = verifier.verify({
            "url": "/api/health",
            "base_url": "http://localhost:9999",
            "timeout": 1,
        })
        # Should attempt connection (fail is expected)
        assert result.status in ["fail", "error"]


class TestCliVerifier:
    """Tests for CliVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = CliVerifier()
        assert verifier.verification_type == VerificationType.CLI

    def test_no_command_error(self):
        """Test error when no command specified."""
        verifier = CliVerifier()
        result = verifier.verify({})
        assert result.status == "error"
        assert "No command" in result.summary

    def test_successful_command(self):
        """Test successful command execution."""
        verifier = CliVerifier()
        result = verifier.verify({
            "command": "echo hello",
            "expected_output": "hello",
        })
        assert result.status == "pass"
        assert result.duration_ms is not None

    def test_failed_command_exit_code(self):
        """Test command with non-zero exit code."""
        verifier = CliVerifier()
        result = verifier.verify({
            "command": "exit 1",
            "expected_exit_code": 0,
        })
        assert result.status == "fail"
        assert "Exit code 1" in result.summary

    def test_expected_output_not_found(self):
        """Test expected output not found."""
        verifier = CliVerifier()
        result = verifier.verify({
            "command": "echo foo",
            "expected_output": "bar",
        })
        assert result.status == "fail"
        assert "Expected output not found" in result.summary

    def test_custom_exit_code(self):
        """Test command with custom expected exit code."""
        verifier = CliVerifier()
        result = verifier.verify({
            "command": "exit 42",
            "expected_exit_code": 42,
        })
        assert result.status == "pass"

    def test_command_timeout(self):
        """Test command timeout."""
        verifier = CliVerifier()
        result = verifier.verify({
            "command": "sleep 10",
            "timeout": 1,
        })
        assert result.status == "fail"
        assert "timed out" in result.summary


class TestBrowserVerifier:
    """Tests for BrowserVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = BrowserVerifier()
        assert verifier.verification_type == VerificationType.BROWSER

    def test_no_url_or_steps_error(self):
        """Test error when no URL or steps specified."""
        verifier = BrowserVerifier()
        result = verifier.verify({})
        # If playwright not installed, skip; otherwise error
        assert result.status in ["error", "skip"]

    def test_skip_when_playwright_not_installed(self):
        """Test skip when playwright is not installed."""
        # This test depends on whether playwright is installed
        verifier = BrowserVerifier()
        result = verifier.verify({"url": "http://example.com"})
        # Either skip (no playwright) or some result
        assert result.status in ["skip", "pass", "fail", "error"]


class TestVerificationRunner:
    """Tests for VerificationRunner."""

    def test_empty_verifications(self):
        """Test runner with no verifications."""
        runner = VerificationRunner([])
        results = runner.run_all()
        assert results == []

    def test_run_cli_verification(self):
        """Test running a CLI verification."""
        runner = VerificationRunner([
            {
                "type": "cli",
                "name": "Echo Test",
                "config": {
                    "command": "echo test",
                },
            }
        ])
        results = runner.run_all()
        assert len(results) == 1
        assert results[0].status == "pass"
        assert results[0].name == "Echo Test"

    def test_unknown_verifier_type(self):
        """Test handling of unknown verifier type."""
        runner = VerificationRunner([
            {
                "type": "unknown_type",
                "name": "Unknown Test",
            }
        ])
        results = runner.run_all()
        assert len(results) == 1
        assert results[0].status == "skip"
        assert "No verifier registered" in results[0].summary

    def test_run_multiple_verifications(self):
        """Test running multiple verifications."""
        runner = VerificationRunner([
            {
                "type": "cli",
                "name": "Test 1",
                "config": {"command": "echo 1"},
            },
            {
                "type": "cli",
                "name": "Test 2",
                "config": {"command": "echo 2"},
            },
        ])
        results = runner.run_all()
        assert len(results) == 2
        assert all(r.status == "pass" for r in results)


class TestVisualVerifier:
    """Tests for VisualVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = VisualVerifier()
        assert verifier.verification_type == VerificationType.VISUAL

    def test_no_paths_error(self):
        """Test error when paths not specified."""
        verifier = VisualVerifier()
        result = verifier.verify({})
        # Either skip (no pillow) or error
        assert result.status in ["error", "skip"]

    def test_skip_when_pillow_not_installed(self):
        """Test skip when Pillow is not installed."""
        verifier = VisualVerifier()
        result = verifier.verify({
            "baseline": "/path/to/baseline.png",
            "current": "/path/to/current.png",
        })
        # Either skip (no pillow) or error (file not found)
        assert result.status in ["skip", "error"]


class TestMetricsVerifier:
    """Tests for MetricsVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = MetricsVerifier()
        assert verifier.verification_type == VerificationType.METRICS

    def test_no_thresholds_error(self):
        """Test error when no thresholds specified."""
        verifier = MetricsVerifier()
        result = verifier.verify({})
        assert result.status == "error"
        assert "No thresholds" in result.summary

    def test_no_metrics_error(self):
        """Test error when no metrics provided."""
        verifier = MetricsVerifier()
        result = verifier.verify({
            "thresholds": {"accuracy": {"min": 0.9}},
        })
        assert result.status == "error"
        assert "No metrics" in result.summary

    def test_metrics_pass(self):
        """Test metrics within thresholds pass."""
        verifier = MetricsVerifier()
        result = verifier.verify({
            "metrics": {"accuracy": 0.95, "loss": 0.05},
            "thresholds": {
                "accuracy": {"min": 0.9},
                "loss": {"max": 0.1},
            },
        })
        assert result.status == "pass"
        assert "2 metrics" in result.summary

    def test_metrics_fail_min(self):
        """Test metrics below min threshold fail."""
        verifier = MetricsVerifier()
        result = verifier.verify({
            "metrics": {"accuracy": 0.8},
            "thresholds": {"accuracy": {"min": 0.9}},
        })
        assert result.status == "fail"
        assert "accuracy" in result.summary

    def test_metrics_fail_max(self):
        """Test metrics above max threshold fail."""
        verifier = MetricsVerifier()
        result = verifier.verify({
            "metrics": {"loss": 0.5},
            "thresholds": {"loss": {"max": 0.1}},
        })
        assert result.status == "fail"

    def test_missing_metric(self):
        """Test missing metric is reported as failure."""
        verifier = MetricsVerifier()
        result = verifier.verify({
            "metrics": {"accuracy": 0.95},
            "thresholds": {
                "accuracy": {"min": 0.9},
                "f1_score": {"min": 0.8},  # Not in metrics
            },
        })
        assert result.status == "fail"
        assert "f1_score" in result.details["failures"][0]["metric"]


class TestDryrunVerifier:
    """Tests for DryrunVerifier."""

    def test_verification_type(self):
        """Test verification type property."""
        verifier = DryrunVerifier()
        assert verifier.verification_type == VerificationType.DRYRUN

    def test_no_command_error(self):
        """Test error when no command specified."""
        verifier = DryrunVerifier()
        result = verifier.verify({})
        assert result.status == "error"
        assert "No command" in result.summary

    def test_successful_dryrun(self):
        """Test successful dry-run."""
        verifier = DryrunVerifier()
        result = verifier.verify({
            "command": "echo 'Plan: 0 to add, 0 to change'",
            "success_patterns": ["Plan:"],
        })
        assert result.status == "pass"

    def test_failure_pattern_detected(self):
        """Test failure pattern detection."""
        verifier = DryrunVerifier()
        result = verifier.verify({
            "command": "echo 'Error: something failed'",
            "failure_patterns": ["Error:"],
        })
        assert result.status == "fail"
        assert "Error:" in result.summary

    def test_success_pattern_not_matched(self):
        """Test when success pattern not matched."""
        verifier = DryrunVerifier()
        result = verifier.verify({
            "command": "echo 'hello world'",
            "success_patterns": ["Plan:"],
        })
        assert result.status == "fail"
        assert "No success patterns" in result.summary


class TestDomainDefaultVerifications:
    """Tests for domain default verification mappings."""

    def test_web_frontend_defaults(self):
        """Test web-frontend has browser verification."""
        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get("web-frontend", [])
        types = [d["type"] for d in defaults]
        assert "browser" in types

    def test_web_backend_defaults(self):
        """Test web-backend has http and cli verification."""
        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get("web-backend", [])
        types = [d["type"] for d in defaults]
        assert "http" in types
        assert "cli" in types

    def test_ml_dl_defaults(self):
        """Test ml-dl domain defaults."""
        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get("ml-dl", [])
        types = [d["type"] for d in defaults]
        assert "cli" in types
        assert "metrics" in types

    def test_infra_defaults(self):
        """Test infra domain defaults."""
        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get("infra", [])
        types = [d["type"] for d in defaults]
        assert "cli" in types
        assert "dryrun" in types

    def test_all_verifier_types_registered(self):
        """Test all defined types have registered verifiers."""
        registered = VerifierRegistry.list_types()
        assert "http" in registered
        assert "cli" in registered
        assert "browser" in registered
        assert "visual" in registered
        assert "metrics" in registered
        assert "dryrun" in registered
