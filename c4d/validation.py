"""C4D Validation Runner - Execute and capture validation results"""

import json
import subprocess
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Literal

from .models import C4Config, ValidationResult


@dataclass
class ValidationRun:
    """Result of a validation run"""

    name: str
    command: str
    exit_code: int
    duration_ms: int
    stdout: str
    stderr: str
    timestamp: datetime

    @property
    def passed(self) -> bool:
        return self.exit_code == 0

    def to_result(self) -> ValidationResult:
        """Convert to ValidationResult model"""
        return ValidationResult(
            name=self.name,
            status="pass" if self.passed else "fail",
            duration_ms=self.duration_ms,
            message=self.stderr[:500] if not self.passed else None,
        )


class ValidationRunner:
    """Execute validation commands and capture results"""

    def __init__(self, project_root: Path, config: C4Config):
        self.root = project_root
        self.config = config
        self.runs_dir = project_root / ".c4" / "runs"
        self.tests_dir = self.runs_dir / "tests"
        self.logs_dir = self.runs_dir / "logs"

    def run_validation(self, name: str, timeout: int = 300) -> ValidationRun:
        """
        Run a single validation command.

        Args:
            name: Validation name (e.g., "lint", "unit")
            timeout: Timeout in seconds

        Returns:
            ValidationRun with results
        """
        commands = self.config.validation.commands
        if name not in commands:
            raise ValueError(f"Unknown validation: {name}. Available: {list(commands.keys())}")

        command = commands[name]
        start = time.time()

        try:
            result = subprocess.run(
                command,
                shell=True,
                cwd=self.root,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
            exit_code = result.returncode
            stdout = result.stdout
            stderr = result.stderr

        except subprocess.TimeoutExpired:
            exit_code = -1
            stdout = ""
            stderr = f"Timeout after {timeout}s"

        except Exception as e:
            exit_code = -2
            stdout = ""
            stderr = str(e)

        duration_ms = int((time.time() - start) * 1000)

        run = ValidationRun(
            name=name,
            command=command,
            exit_code=exit_code,
            duration_ms=duration_ms,
            stdout=stdout,
            stderr=stderr,
            timestamp=datetime.now(),
        )

        # Save run to logs
        self._save_run(run)

        return run

    def run_all_required(self, timeout_per_validation: int = 300) -> list[ValidationRun]:
        """
        Run all required validations.

        Returns:
            List of ValidationRun results
        """
        required = self.config.validation.required
        results = []

        for name in required:
            run = self.run_validation(name, timeout=timeout_per_validation)
            results.append(run)

            # Stop on first failure (fail-fast)
            if not run.passed:
                break

        return results

    def run_validations(
        self, names: list[str], fail_fast: bool = True, timeout: int = 300
    ) -> list[ValidationRun]:
        """
        Run specific validations.

        Args:
            names: List of validation names to run
            fail_fast: Stop on first failure
            timeout: Timeout per validation in seconds

        Returns:
            List of ValidationRun results
        """
        results = []

        for name in names:
            run = self.run_validation(name, timeout=timeout)
            results.append(run)

            if fail_fast and not run.passed:
                break

        return results

    def _save_run(self, run: ValidationRun) -> Path:
        """Save validation run to file"""
        self.tests_dir.mkdir(parents=True, exist_ok=True)

        ts = run.timestamp.strftime("%Y%m%d_%H%M%S")
        filename = f"{ts}_{run.name}.json"
        filepath = self.tests_dir / filename

        data = {
            "name": run.name,
            "command": run.command,
            "exit_code": run.exit_code,
            "duration_ms": run.duration_ms,
            "passed": run.passed,
            "timestamp": run.timestamp.isoformat(),
            "stdout_lines": run.stdout.count("\n"),
            "stderr_lines": run.stderr.count("\n"),
        }

        # Save summary
        filepath.write_text(json.dumps(data, indent=2))

        # Save full output to logs
        log_file = self.logs_dir / f"{ts}_{run.name}.log"
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        log_file.write_text(f"=== STDOUT ===\n{run.stdout}\n\n=== STDERR ===\n{run.stderr}")

        return filepath

    def get_last_results(self) -> dict[str, str]:
        """Get the last validation results as name -> status mapping"""
        results = {}

        for name in self.config.validation.commands:
            # Find most recent file for this validation
            files = sorted(self.tests_dir.glob(f"*_{name}.json"), reverse=True)
            if files:
                data = json.loads(files[0].read_text())
                results[name] = "pass" if data["passed"] else "fail"

        return results


def parse_test_output(stdout: str, test_framework: str = "pytest") -> dict:
    """
    Parse test output to extract summary information.

    Args:
        stdout: Test command stdout
        test_framework: "pytest", "jest", "mocha", etc.

    Returns:
        Dictionary with parsed info (passed, failed, coverage, etc.)
    """
    info = {
        "passed": 0,
        "failed": 0,
        "skipped": 0,
        "coverage": None,
    }

    if test_framework == "pytest":
        # Parse pytest output
        # Example: "5 passed, 2 failed, 1 skipped in 1.23s"
        import re

        match = re.search(r"(\d+) passed", stdout)
        if match:
            info["passed"] = int(match.group(1))

        match = re.search(r"(\d+) failed", stdout)
        if match:
            info["failed"] = int(match.group(1))

        match = re.search(r"(\d+) skipped", stdout)
        if match:
            info["skipped"] = int(match.group(1))

        # Coverage
        match = re.search(r"TOTAL\s+\d+\s+\d+\s+(\d+)%", stdout)
        if match:
            info["coverage"] = int(match.group(1))

    elif test_framework == "jest":
        # Parse Jest output
        import re

        match = re.search(r"Tests:\s+(\d+) passed", stdout)
        if match:
            info["passed"] = int(match.group(1))

        match = re.search(r"Tests:\s+\d+ passed,\s+(\d+) failed", stdout)
        if match:
            info["failed"] = int(match.group(1))

    return info


def extract_failure_signature(stderr: str, stdout: str) -> str | None:
    """
    Extract a signature from test failures for deduplication.

    This helps detect repeated failures with the same root cause.
    """
    import re

    # Look for common error patterns
    patterns = [
        r"(TypeError|ValueError|AttributeError|KeyError):\s*(.+)",
        r"(AssertionError):\s*(.+)",
        r"(Error|Exception):\s*(.+)",
        r"FAILED\s+(\S+)",
    ]

    combined = stdout + "\n" + stderr

    for pattern in patterns:
        match = re.search(pattern, combined)
        if match:
            return match.group(0)[:200]  # Limit signature length

    return None
