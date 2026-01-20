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
# Browser Verifier - Uses Playwright (optional dependency)
# =============================================================================


@VerifierRegistry.register("browser")
class BrowserVerifier(Verifier):
    """Browser E2E test verifier using Playwright."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.BROWSER

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Run browser E2E test with Playwright.

        Config:
            url: str - Starting URL (required)
            steps: list - List of step dicts (optional)
            screenshot: bool - Take screenshot at end (default: True)
            screenshot_dir: str - Directory for screenshots (default: /tmp)
            timeout: int - Timeout in milliseconds (default: 30000)
            headless: bool - Run headless (default: True)

        Step format:
            - {"action": "goto", "url": "http://..."}
            - {"action": "click", "selector": "#button"}
            - {"action": "type", "selector": "#input", "value": "text"}
            - {"action": "wait", "selector": ".element", "state": "visible"}
            - {"action": "screenshot", "name": "step1.png"}
            - {"action": "assert_text", "selector": ".msg", "text": "Success"}
            - {"action": "assert_visible", "selector": ".element"}
        """
        import time

        try:
            from playwright.sync_api import TimeoutError as PlaywrightTimeout
            from playwright.sync_api import sync_playwright
        except ImportError:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Browser E2E"),
                status="skip",
                summary="playwright not installed. Run: uv add playwright && playwright install",
            )

        url = config.get("url", "")
        steps = config.get("steps", [])
        take_screenshot = config.get("screenshot", True)
        screenshot_dir = Path(config.get("screenshot_dir", "/tmp"))
        timeout = config.get("timeout", 30000)
        headless = config.get("headless", True)

        if not url and not steps:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Browser E2E"),
                status="error",
                summary="No URL or steps specified",
            )

        start_time = time.time()
        screenshot_path = None
        step_results = []

        try:
            with sync_playwright() as p:
                browser = p.chromium.launch(headless=headless)
                context = browser.new_context()
                page = context.new_page()
                page.set_default_timeout(timeout)

                # Navigate to initial URL if provided
                if url:
                    page.goto(url)
                    step_results.append({"action": "goto", "url": url, "status": "ok"})

                # Execute steps
                for i, step in enumerate(steps):
                    action = step.get("action", "")
                    step_result = {"action": action, "index": i}

                    try:
                        if action == "goto":
                            page.goto(step["url"])
                            step_result["status"] = "ok"

                        elif action == "click":
                            page.click(step["selector"])
                            step_result["status"] = "ok"

                        elif action == "type":
                            page.fill(step["selector"], step["value"])
                            step_result["status"] = "ok"

                        elif action == "wait":
                            state = step.get("state", "visible")
                            page.wait_for_selector(step["selector"], state=state)
                            step_result["status"] = "ok"

                        elif action == "screenshot":
                            name = step.get("name", f"step_{i}.png")
                            path = screenshot_dir / name
                            page.screenshot(path=str(path))
                            step_result["status"] = "ok"
                            step_result["path"] = str(path)

                        elif action == "assert_text":
                            element = page.locator(step["selector"])
                            actual_text = element.text_content()
                            expected_text = step["text"]
                            if expected_text in (actual_text or ""):
                                step_result["status"] = "ok"
                            else:
                                step_result["status"] = "fail"
                                step_result["expected"] = expected_text
                                step_result["actual"] = actual_text[:100]

                        elif action == "assert_visible":
                            element = page.locator(step["selector"])
                            if element.is_visible():
                                step_result["status"] = "ok"
                            else:
                                step_result["status"] = "fail"
                                step_result["message"] = "Element not visible"

                        elif action == "press":
                            page.keyboard.press(step["key"])
                            step_result["status"] = "ok"

                        elif action == "select":
                            page.select_option(step["selector"], step["value"])
                            step_result["status"] = "ok"

                        else:
                            step_result["status"] = "skip"
                            step_result["message"] = f"Unknown action: {action}"

                    except PlaywrightTimeout:
                        step_result["status"] = "fail"
                        step_result["message"] = "Timeout"
                    except Exception as e:
                        step_result["status"] = "fail"
                        step_result["message"] = str(e)[:100]

                    step_results.append(step_result)

                    # Stop on first failure
                    if step_result["status"] == "fail":
                        break

                # Take final screenshot if requested
                if take_screenshot:
                    screenshot_path = screenshot_dir / f"final_{int(time.time())}.png"
                    page.screenshot(path=str(screenshot_path))

                browser.close()

            duration_ms = int((time.time() - start_time) * 1000)

            # Check if any step failed
            failed_steps = [s for s in step_results if s.get("status") == "fail"]
            if failed_steps:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Browser E2E"),
                    status="fail",
                    summary=f"Step {failed_steps[0]['index']} failed: {failed_steps[0].get('message', 'assertion error')}",
                    details={"steps": step_results},
                    screenshot_path=screenshot_path,
                    duration_ms=duration_ms,
                )

            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Browser E2E"),
                status="pass",
                summary=f"All {len(step_results)} steps passed",
                details={"steps": step_results},
                screenshot_path=screenshot_path,
                duration_ms=duration_ms,
            )

        except PlaywrightTimeout:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Browser E2E"),
                status="fail",
                summary=f"Browser test timed out after {timeout}ms",
                details={"steps": step_results},
            )
        except Exception as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Browser E2E"),
                status="error",
                summary=f"Browser error: {e!s}",
                details={"steps": step_results},
            )


# =============================================================================
# Visual Verifier - Screenshot comparison (optional Pillow dependency)
# =============================================================================


@VerifierRegistry.register("visual")
class VisualVerifier(Verifier):
    """Visual comparison verifier using screenshot diff."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.VISUAL

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Compare two screenshots for visual differences.

        Config:
            baseline: str - Path to baseline screenshot (required)
            current: str - Path to current screenshot (required)
            threshold: float - Difference threshold 0-1 (default: 0.05 = 5%)
            output: str - Path to save diff image (optional)
        """
        try:
            from PIL import Image, ImageChops
        except ImportError:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Visual Diff"),
                status="skip",
                summary="Pillow not installed. Run: uv add pillow",
            )

        baseline_path = config.get("baseline", "")
        current_path = config.get("current", "")
        threshold = config.get("threshold", 0.05)
        output_path = config.get("output")

        if not baseline_path or not current_path:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Visual Diff"),
                status="error",
                summary="Both baseline and current paths required",
            )

        try:
            baseline = Image.open(baseline_path)
            current = Image.open(current_path)

            # Ensure same size
            if baseline.size != current.size:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Visual Diff"),
                    status="fail",
                    summary=f"Size mismatch: {baseline.size} vs {current.size}",
                    details={
                        "baseline_size": baseline.size,
                        "current_size": current.size,
                    },
                )

            # Calculate difference
            diff = ImageChops.difference(baseline, current)
            diff_data = list(diff.getdata())

            # Calculate difference percentage
            total_pixels = len(diff_data)
            diff_sum = sum(sum(pixel) if isinstance(pixel, tuple) else pixel for pixel in diff_data)
            max_diff = total_pixels * 255 * (3 if baseline.mode == "RGB" else 1)
            diff_percent = diff_sum / max_diff if max_diff > 0 else 0

            # Save diff image if requested
            if output_path:
                diff.save(output_path)

            if diff_percent > threshold:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Visual Diff"),
                    status="fail",
                    summary=f"Visual difference {diff_percent:.1%} exceeds threshold {threshold:.1%}",
                    details={
                        "diff_percent": round(diff_percent, 4),
                        "threshold": threshold,
                    },
                    screenshot_path=Path(output_path) if output_path else None,
                )

            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Visual Diff"),
                status="pass",
                summary=f"Visual difference {diff_percent:.1%} within threshold",
                details={"diff_percent": round(diff_percent, 4)},
            )

        except FileNotFoundError as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Visual Diff"),
                status="error",
                summary=f"File not found: {e!s}",
            )
        except Exception as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Visual Diff"),
                status="error",
                summary=f"Visual comparison error: {e!s}",
            )


# =============================================================================
# Metrics Verifier - ML/DL performance metrics
# =============================================================================


@VerifierRegistry.register("metrics")
class MetricsVerifier(Verifier):
    """Performance metrics verifier for ML/DL models."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.METRICS

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Verify performance metrics against thresholds.

        Config:
            metrics_file: str - Path to JSON file with metrics (optional)
            metrics: dict - Inline metrics dict (optional)
            thresholds: dict - Metric thresholds (required)
                - {metric_name: {"min": value} or {"max": value} or {"eq": value}}

        Example:
            thresholds:
                accuracy: {"min": 0.95}
                loss: {"max": 0.1}
                inference_time_ms: {"max": 100}
        """
        import json

        metrics_file = config.get("metrics_file")
        metrics = config.get("metrics", {})
        thresholds = config.get("thresholds", {})

        if not thresholds:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Metrics Check"),
                status="error",
                summary="No thresholds specified",
            )

        # Load metrics from file if specified
        if metrics_file:
            try:
                with open(metrics_file) as f:
                    metrics = json.load(f)
            except FileNotFoundError:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Metrics Check"),
                    status="error",
                    summary=f"Metrics file not found: {metrics_file}",
                )
            except json.JSONDecodeError as e:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Metrics Check"),
                    status="error",
                    summary=f"Invalid JSON in metrics file: {e!s}",
                )

        if not metrics:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Metrics Check"),
                status="error",
                summary="No metrics provided",
            )

        # Check each threshold
        failures = []
        passes = []

        for metric_name, threshold in thresholds.items():
            if metric_name not in metrics:
                failures.append(
                    {
                        "metric": metric_name,
                        "reason": "missing",
                    }
                )
                continue

            value = metrics[metric_name]

            if "min" in threshold and value < threshold["min"]:
                failures.append(
                    {
                        "metric": metric_name,
                        "value": value,
                        "threshold": f"min {threshold['min']}",
                        "reason": f"{value} < {threshold['min']}",
                    }
                )
            elif "max" in threshold and value > threshold["max"]:
                failures.append(
                    {
                        "metric": metric_name,
                        "value": value,
                        "threshold": f"max {threshold['max']}",
                        "reason": f"{value} > {threshold['max']}",
                    }
                )
            elif "eq" in threshold and value != threshold["eq"]:
                failures.append(
                    {
                        "metric": metric_name,
                        "value": value,
                        "threshold": f"eq {threshold['eq']}",
                        "reason": f"{value} != {threshold['eq']}",
                    }
                )
            else:
                passes.append(
                    {
                        "metric": metric_name,
                        "value": value,
                    }
                )

        if failures:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Metrics Check"),
                status="fail",
                summary=f"{len(failures)} metric(s) failed: {failures[0]['metric']}",
                details={
                    "failures": failures,
                    "passes": passes,
                    "metrics": metrics,
                },
            )

        return VerificationResult(
            type=self.verification_type.value,
            name=config.get("name", "Metrics Check"),
            status="pass",
            summary=f"All {len(passes)} metrics within thresholds",
            details={
                "passes": passes,
                "metrics": metrics,
            },
        )


# =============================================================================
# Dryrun Verifier - Dry-run commands (terraform plan, etc.)
# =============================================================================


@VerifierRegistry.register("dryrun")
class DryrunVerifier(Verifier):
    """Dry-run command verifier for infrastructure changes."""

    @property
    def verification_type(self) -> VerificationType:
        return VerificationType.DRYRUN

    def verify(self, config: dict[str, Any]) -> VerificationResult:
        """
        Run a dry-run command and verify output.

        Config:
            command: str - Dry-run command (required)
            success_patterns: list[str] - Patterns indicating success (optional)
            failure_patterns: list[str] - Patterns indicating failure (optional)
            timeout: int - Timeout in seconds (default: 120)
            cwd: str - Working directory (optional)

        Example:
            command: "terraform plan -no-color"
            success_patterns: ["No changes", "Plan:"]
            failure_patterns: ["Error:", "Failed"]
        """
        import re
        import subprocess
        import time

        command = config.get("command", "")
        success_patterns = config.get("success_patterns", [])
        failure_patterns = config.get("failure_patterns", ["Error:", "error:", "FAILED", "failed"])
        timeout = config.get("timeout", 120)
        cwd = config.get("cwd")

        if not command:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Dry-run Check"),
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

            # Check for failure patterns first
            for pattern in failure_patterns:
                if re.search(pattern, output):
                    return VerificationResult(
                        type=self.verification_type.value,
                        name=config.get("name", "Dry-run Check"),
                        status="fail",
                        summary=f"Found failure pattern: {pattern}",
                        details={
                            "exit_code": result.returncode,
                            "output": output[:2000],
                            "matched_pattern": pattern,
                        },
                        duration_ms=duration_ms,
                    )

            # Check for success patterns if specified
            if success_patterns:
                matched = [p for p in success_patterns if re.search(p, output)]
                if not matched:
                    return VerificationResult(
                        type=self.verification_type.value,
                        name=config.get("name", "Dry-run Check"),
                        status="fail",
                        summary="No success patterns matched",
                        details={
                            "exit_code": result.returncode,
                            "output": output[:2000],
                            "expected_patterns": success_patterns,
                        },
                        duration_ms=duration_ms,
                    )

            # Check exit code
            if result.returncode != 0:
                return VerificationResult(
                    type=self.verification_type.value,
                    name=config.get("name", "Dry-run Check"),
                    status="fail",
                    summary=f"Exit code {result.returncode}",
                    details={
                        "exit_code": result.returncode,
                        "output": output[:2000],
                    },
                    duration_ms=duration_ms,
                )

            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Dry-run Check"),
                status="pass",
                summary="Dry-run completed successfully",
                details={
                    "exit_code": result.returncode,
                    "output": output[:1000],
                },
                duration_ms=duration_ms,
            )

        except subprocess.TimeoutExpired:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Dry-run Check"),
                status="fail",
                summary=f"Command timed out after {timeout}s",
            )
        except Exception as e:
            return VerificationResult(
                type=self.verification_type.value,
                name=config.get("name", "Dry-run Check"),
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
