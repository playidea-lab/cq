"""Execution Verification System - Domain-specific runtime verification.

This module provides a pluggable verification system that runs actual
checks (HTTP calls, browser tests, CLI commands) before supervisor review.

Usage:
    # Get verifiers for a domain
    verifiers = VerifierRegistry.get_for_domain("web-backend")

    # Run verifications
    results = []
    for verifier_type, verifier in verifiers:
        result = verifier.verify(config)
        results.append(result)
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any


class VerificationType(Enum):
    """Types of verification that can be performed."""

    BROWSER = "browser"  # Playwright, Claude in Chrome
    HTTP = "http"  # API calls with httpx
    CLI = "cli"  # Command line execution
    VISUAL = "visual"  # Screenshot comparison
    METRICS = "metrics"  # Performance/ML metrics
    DRYRUN = "dryrun"  # Dry-run commands (terraform plan, etc.)


@dataclass
class VerificationResult:
    """Result of a verification run."""

    type: str  # VerificationType value
    name: str  # User-friendly name
    status: str  # "pass", "fail", "skip", "error"
    summary: str  # Brief description of result
    details: dict[str, Any] = field(default_factory=dict)
    screenshot_path: Path | None = None
    duration_ms: int | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        result = {
            "type": self.type,
            "name": self.name,
            "status": self.status,
            "summary": self.summary,
        }
        if self.details:
            result["details"] = self.details
        if self.screenshot_path:
            result["screenshot_path"] = str(self.screenshot_path)
        if self.duration_ms is not None:
            result["duration_ms"] = self.duration_ms
        return result


class Verifier(ABC):
    """Abstract base class for verification implementations."""

    @property
    @abstractmethod
    def verification_type(self) -> VerificationType:
        """Return the type of verification this verifier performs."""
        pass

    @abstractmethod
    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Run verification with the given configuration.

        Args:
            config: Configuration dict specific to this verifier type

        Returns:
            VerificationResult with status and details
        """
        pass

    @property
    def name(self) -> str:
        """Return human-readable name for this verifier."""
        return self.__class__.__name__


class VerifierRegistry:
    """Registry for automatic verifier registration and lookup."""

    _verifiers: dict[str, type[Verifier]] = {}

    @classmethod
    def register(cls, verifier_type: str):
        """
        Decorator to register a verifier class.

        Usage:
            @VerifierRegistry.register("http")
            class HttpVerifier(Verifier):
                pass
        """

        def decorator(verifier_class: type[Verifier]):
            cls._verifiers[verifier_type] = verifier_class
            return verifier_class

        return decorator

    @classmethod
    def get(cls, verifier_type: str) -> Verifier | None:
        """Get an instance of the verifier for the given type."""
        verifier_class = cls._verifiers.get(verifier_type)
        if verifier_class:
            return verifier_class()
        return None

    @classmethod
    def list_types(cls) -> list[str]:
        """List all registered verifier types."""
        return list(cls._verifiers.keys())

    @classmethod
    def get_for_domain(cls, domain: str) -> list[tuple[str, Verifier]]:
        """
        Get all default verifiers for a domain.

        Returns list of (type, verifier) tuples.
        """
        default_types = DOMAIN_DEFAULT_VERIFICATIONS.get(domain, [])
        result = []
        for item in default_types:
            verifier_type = item["type"]
            verifier = cls.get(verifier_type)
            if verifier:
                result.append((verifier_type, verifier))
        return result


# Domain -> Default verification types mapping
DOMAIN_DEFAULT_VERIFICATIONS: dict[str, list[dict[str, str]]] = {
    "web-frontend": [
        {"type": "browser", "name": "E2E Flow"},
        {"type": "visual", "name": "Screenshot Diff"},
    ],
    "web-backend": [
        {"type": "http", "name": "API Health"},
        {"type": "cli", "name": "Server Start"},
    ],
    "ml-dl": [
        {"type": "cli", "name": "Model Inference"},
        {"type": "metrics", "name": "Performance Check"},
    ],
    "infra": [
        {"type": "cli", "name": "Terraform Plan"},
        {"type": "dryrun", "name": "Apply Dry-Run"},
    ],
    "mobile": [
        {"type": "cli", "name": "Build Check"},
    ],
    "library": [
        {"type": "cli", "name": "Import Test"},
        {"type": "cli", "name": "Example Run"},
    ],
}


# =============================================================================
# HTTP Verifier - Uses httpx (optional dependency)
# =============================================================================


@VerifierRegistry.register("http")
class HttpVerifier(Verifier):
    """HTTP API call verifier using httpx."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.HTTP

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Make an HTTP request and verify the response.

        Config:
            url: str - URL to request (required)
            method: str - HTTP method (default: GET)
            headers: dict - Request headers (optional)
            body: dict - Request body for POST/PUT (optional)
            expected_status: int - Expected status code (default: 200)
            expected_body: str - Expected substring in response body (optional)
            timeout: int - Timeout in seconds (default: 30)
            base_url: str - Base URL to prepend to url (optional)
        """
        import time

        try:
            import httpx
        except ImportError:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="skip",
                summary="httpx not installed. Run: uv add httpx",
            )

        url = config.get("url", "")
        base_url = config.get("base_url", "")
        if base_url and not url.startswith(("http://", "https://")):
            url = f"{base_url.rstrip('/')}/{url.lstrip('/')}"

        method = config.get("method", "GET").upper()
        headers = config.get("headers", {})
        body = config.get("body")
        expected_status = config.get("expected_status", 200)
        expected_body = config.get("expected_body")
        timeout = config.get("timeout", 30)

        if not url:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="error",
                summary="No URL specified",
            )

        start_time = time.time()
        try:
            with httpx.Client(timeout=timeout) as client:
                if method == "GET":
                    response = client.get(url, headers=headers)
                elif method == "POST":
                    response = client.post(url, headers=headers, json=body)
                elif method == "PUT":
                    response = client.put(url, headers=headers, json=body)
                elif method == "DELETE":
                    response = client.delete(url, headers=headers)
                elif method == "PATCH":
                    response = client.patch(url, headers=headers, json=body)
                else:
                    return VerificationResult(
                        type=self.verification_type.value,
                        name=config.get("name", "HTTP Check"),
                        status="error",
                        summary=f"Unsupported HTTP method: {method}",
                    )

            duration_ms = int((time.time() - start_time) * 1000)
            response_text = response.text[:1000]

            # Check status code
            if response.status_code != expected_status:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "HTTP Check"),
                    status="fail",
                    summary=f"Status {response.status_code} (expected {expected_status})",
                    details={
                        "status_code": response.status_code,
                        "response_body": response_text,
                        "url": url,
                    },
                    duration_ms=duration_ms,
                )

            # Check expected body if specified
            if expected_body and expected_body not in response.text:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "HTTP Check"),
                    status="fail",
                    summary=f"Expected body not found: {expected_body[:50]}",
                    details={
                        "status_code": response.status_code,
                        "response_body": response_text,
                    },
                    duration_ms=duration_ms,
                )

            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="pass",
                summary=f"{method} {url} -> {response.status_code} OK",
                details={
                    "status_code": response.status_code,
                    "response_size": len(response.content),
                },
                duration_ms=duration_ms,
            )

        except httpx.TimeoutException:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="fail",
                summary=f"Request timed out after {timeout}s",
                details={"url": url},
            )
        except httpx.ConnectError as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="fail",
                summary=f"Connection failed: {e!s}",
                details={"url": url},
            )
        except Exception as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "HTTP Check"),
                status="error",
                summary=f"Request error: {e!s}",
                details={"url": url},
            )


# =============================================================================
# CLI Verifier - Always available (uses subprocess)
# =============================================================================


@VerifierRegistry.register("cli")
class CliVerifier(Verifier):
    """CLI command execution verifier."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.CLI

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Execute a CLI command and verify output.

        Config:
            command: str - Command to execute
            expected_exit_code: int - Expected exit code (default: 0)
            expected_output: str - Expected substring in output (optional)
            timeout: int - Timeout in seconds (default: 30)
            cwd: str - Working directory (optional)
        """
        import subprocess
        import time

        command = config.get("command", "")
        expected_exit_code = config.get("expected_exit_code", 0)
        expected_output = config.get("expected_output")
        timeout = config.get("timeout", 30)
        cwd = config.get("cwd")

        if not command:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "CLI Check"),
                status="error",
                summary="No command specified",
            )

        start_time = time.time()
        try:
            result = subprocess.run(
                command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=timeout,
                cwd=cwd,
            )
            duration_ms = int((time.time() - start_time) * 1000)

            output = result.stdout + result.stderr

            # Check exit code
            if result.returncode != expected_exit_code:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "CLI Check"),
                    status="fail",
                    summary=f"Exit code {result.returncode} (expected {expected_exit_code})",
                    details={
                        "exit_code": result.returncode,
                        "stdout": result.stdout[:500],
                        "stderr": result.stderr[:500],
                    },
                    duration_ms=duration_ms,
                )

            # Check expected output if specified
            if expected_output and expected_output not in output:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "CLI Check"),
                    status="fail",
                    summary=f"Expected output not found: {expected_output[:50]}",
                    details={"output": output[:500]},
                    duration_ms=duration_ms,
                )

            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "CLI Check"),
                status="pass",
                summary=f"Command completed successfully (exit code {result.returncode})",
                details={"output": output[:500]},
                duration_ms=duration_ms,
            )

        except subprocess.TimeoutExpired:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "CLI Check"),
                status="fail",
                summary=f"Command timed out after {timeout}s",
            )
        except Exception as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "CLI Check"),
                status="error",
                summary=f"Execution error: {e!s}",
            )


# =============================================================================
# Verification Runner
# =============================================================================


class VerificationRunner:
    """Runs multiple verifications and collects results."""

    def __init__(self, verifications: list[dict[str, Any]] | None = None):
        """
        Initialize runner with verification configs.

        Args:
            verifications: List of verification configs from config.yaml
        """
        self.verifications = verifications or []

    def run_all(self) -> list[VerificationResult]:
        """
        Run all configured verifications.

        Returns:
            List of VerificationResult objects
        """
        results = []

        for verification in self.verifications:
            verifier_type = verification.get("type", "")
            verifier = VerifierRegistry.get(verifier_type)

            if verifier is None:
                results.append(
                    VerificationResult(
                        type=verifier_type,
                        name=verification.get("name", "Unknown"),
                        status="skip",
                        summary=f"No verifier registered for type: {verifier_type}",
                    )
                )
                continue

            # Merge verification config
            config = verification.get("config", {})
            config["name"] = verification.get("name", verifier.name)

            try:
                result = verifier.verify(config)
                results.append(result)
            except Exception as e:
                results.append(
                    VerificationResult(
                        type=verifier_type,
                        name=verification.get("name", "Unknown"),
                        status="error",
                        summary=f"Verifier error: {e!s}",
                    )
                )

        return results

    def run_for_domain(self, domain: str) -> list[VerificationResult]:
        """
        Run default verifications for a domain.

        Args:
            domain: Domain name (e.g., "web-backend")

        Returns:
            List of VerificationResult objects
        """
        verifiers = VerifierRegistry.get_for_domain(domain)
        results = []

        for verifier_type, verifier in verifiers:
            try:
                # Default config for domain verifiers
                config = {"name": f"{domain} - {verifier.name}"}
                result = verifier.verify(config)
                results.append(result)
            except Exception as e:
                results.append(
                    VerificationResult(
                        type=verifier_type,
                        name=verifier.name,
                        status="error",
                        summary=f"Verifier error: {e!s}",
                    )
                )

        return results
