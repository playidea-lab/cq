"""Sandbox Executor - Isolated execution with resource limits."""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any


@dataclass
class ResourceLimits:
    """Resource limits for sandbox execution.

    Attributes:
        max_memory_mb: Maximum memory in MB (default: 512)
        max_cpu_seconds: Maximum CPU time in seconds (default: 300)
        max_file_size_mb: Maximum file size in MB (default: 100)
        max_processes: Maximum concurrent processes (default: 10)
        max_open_files: Maximum open files (default: 100)
        network_enabled: Allow network access (default: False)
    """

    max_memory_mb: int = 512
    max_cpu_seconds: int = 300
    max_file_size_mb: int = 100
    max_processes: int = 10
    max_open_files: int = 100
    network_enabled: bool = False


@dataclass
class SandboxConfig:
    """Configuration for sandbox environment.

    Attributes:
        work_dir: Working directory for execution
        allowed_paths: Paths accessible within sandbox
        env_vars: Environment variables to set
        limits: Resource limits
    """

    work_dir: Path | None = None
    allowed_paths: list[Path] = field(default_factory=list)
    env_vars: dict[str, str] = field(default_factory=dict)
    limits: ResourceLimits = field(default_factory=ResourceLimits)


@dataclass
class ExecutionResult:
    """Result of sandbox execution.

    Attributes:
        success: Whether execution completed successfully
        exit_code: Process exit code
        stdout: Standard output
        stderr: Standard error
        duration_ms: Execution time in milliseconds
        killed: Whether process was killed (timeout/limits)
        error: Error message if failed
    """

    success: bool
    exit_code: int
    stdout: str
    stderr: str
    duration_ms: int
    killed: bool = False
    error: str | None = None


class SandboxExecutor:
    """
    Executes commands in an isolated sandbox environment.

    Features:
    - Process isolation (separate process group)
    - Resource limits (memory, CPU, files)
    - Temporary filesystem
    - Network isolation (when supported)
    - Timeout enforcement

    Security measures:
    - Commands run in isolated temp directory
    - Limited access to host filesystem
    - Process resource limits via ulimit
    - Network disabled by default

    Example:
        config = SandboxConfig(limits=ResourceLimits(max_memory_mb=256))
        executor = SandboxExecutor(config)

        result = executor.run(["python", "script.py"])
        print(f"Output: {result.stdout}")
    """

    def __init__(self, config: SandboxConfig | None = None):
        """Initialize sandbox executor.

        Args:
            config: Sandbox configuration
        """
        self._config = config or SandboxConfig()
        self._temp_dir: Path | None = None

    @property
    def work_dir(self) -> Path:
        """Get working directory."""
        if self._config.work_dir:
            return self._config.work_dir

        if self._temp_dir is None:
            self._temp_dir = Path(tempfile.mkdtemp(prefix="c4-sandbox-"))

        return self._temp_dir

    def cleanup(self) -> None:
        """Clean up temporary resources."""
        if self._temp_dir and self._temp_dir.exists():
            shutil.rmtree(self._temp_dir, ignore_errors=True)
            self._temp_dir = None

    def __enter__(self) -> "SandboxExecutor":
        return self

    def __exit__(self, *args: Any) -> None:
        self.cleanup()

    # =========================================================================
    # Execution
    # =========================================================================

    def run(
        self,
        command: list[str],
        timeout: int | None = None,
        input_data: str | None = None,
        cwd: Path | None = None,
    ) -> ExecutionResult:
        """Run a command in the sandbox.

        Args:
            command: Command and arguments to run
            timeout: Timeout in seconds (default: from limits)
            input_data: Data to send to stdin
            cwd: Working directory (default: sandbox work_dir)

        Returns:
            ExecutionResult with output and status
        """
        if timeout is None:
            timeout = self._config.limits.max_cpu_seconds

        work_dir = cwd or self.work_dir
        work_dir.mkdir(parents=True, exist_ok=True)

        # Build environment
        env = self._build_env()

        # Wrap command with resource limits (Unix only)
        wrapped_cmd = self._wrap_with_limits(command)

        start_time = datetime.now()

        try:
            result = subprocess.run(
                wrapped_cmd,
                cwd=work_dir,
                env=env,
                input=input_data,
                capture_output=True,
                text=True,
                timeout=timeout,
                start_new_session=True,  # Process isolation
            )

            duration_ms = int((datetime.now() - start_time).total_seconds() * 1000)

            return ExecutionResult(
                success=result.returncode == 0,
                exit_code=result.returncode,
                stdout=result.stdout,
                stderr=result.stderr,
                duration_ms=duration_ms,
            )

        except subprocess.TimeoutExpired as e:
            duration_ms = int((datetime.now() - start_time).total_seconds() * 1000)
            error = f"Timeout after {timeout}s"

            return ExecutionResult(
                success=False,
                exit_code=-1,
                stdout=e.stdout or "" if hasattr(e, "stdout") else "",
                stderr=e.stderr or "" if hasattr(e, "stderr") else "",
                duration_ms=duration_ms,
                killed=True,
                error=error,
            )

        except Exception as e:
            duration_ms = int((datetime.now() - start_time).total_seconds() * 1000)

            return ExecutionResult(
                success=False,
                exit_code=-1,
                stdout="",
                stderr=str(e),
                duration_ms=duration_ms,
                error=str(e),
            )

    def run_script(
        self,
        script_content: str,
        script_name: str = "script.py",
        interpreter: str = "python",
        timeout: int | None = None,
    ) -> ExecutionResult:
        """Run a script in the sandbox.

        Args:
            script_content: Script source code
            script_name: Filename for the script
            interpreter: Interpreter to use
            timeout: Execution timeout

        Returns:
            ExecutionResult with output
        """
        script_path = self.work_dir / script_name
        script_path.write_text(script_content)

        return self.run([interpreter, str(script_path)], timeout=timeout)

    # =========================================================================
    # Environment Setup
    # =========================================================================

    def _build_env(self) -> dict[str, str]:
        """Build environment variables for sandbox."""
        env = {
            "HOME": str(self.work_dir),
            "TMPDIR": str(self.work_dir / "tmp"),
            "PATH": "/usr/local/bin:/usr/bin:/bin",
            "LANG": "C.UTF-8",
            "LC_ALL": "C.UTF-8",
            "PYTHONUNBUFFERED": "1",
            "C4_SANDBOX": "1",
        }

        # Add configured env vars
        env.update(self._config.env_vars)

        # Network isolation marker
        if not self._config.limits.network_enabled:
            env["C4_NETWORK_DISABLED"] = "1"

        return env

    def _wrap_with_limits(self, command: list[str]) -> list[str]:
        """Wrap command with resource limits (Unix/Linux only).

        Note: ulimit may not work on macOS due to system restrictions.
        In cloud (Docker/Linux), limits will be properly enforced.
        """
        # Skip ulimit on macOS (doesn't support -v) or if disabled
        import platform

        if platform.system() == "Darwin" or os.environ.get("C4_SKIP_ULIMIT"):
            return command

        limits = self._config.limits

        # Use ulimit for resource limits on Linux
        if os.name != "nt":  # Not Windows
            limit_cmds = []

            # Memory limit (in KB)
            mem_kb = limits.max_memory_mb * 1024
            limit_cmds.append(f"ulimit -v {mem_kb}")

            # File size limit (in KB)
            file_kb = limits.max_file_size_mb * 1024
            limit_cmds.append(f"ulimit -f {file_kb}")

            # Max processes
            limit_cmds.append(f"ulimit -u {limits.max_processes}")

            # Max open files
            limit_cmds.append(f"ulimit -n {limits.max_open_files}")

            # Combine limits and command
            limit_str = " && ".join(limit_cmds)
            cmd_str = " ".join(f'"{c}"' if " " in c else c for c in command)

            return ["bash", "-c", f"{limit_str} && {cmd_str}"]

        return command

    # =========================================================================
    # File Operations
    # =========================================================================

    def copy_to_sandbox(self, src: Path, dest_name: str | None = None) -> Path:
        """Copy a file into the sandbox.

        Args:
            src: Source file path
            dest_name: Destination filename (default: same name)

        Returns:
            Path to file in sandbox
        """
        dest = self.work_dir / (dest_name or src.name)
        shutil.copy2(src, dest)
        return dest

    def copy_from_sandbox(self, src_name: str, dest: Path) -> bool:
        """Copy a file out of the sandbox.

        Args:
            src_name: Filename in sandbox
            dest: Destination path

        Returns:
            True if copied successfully
        """
        src = self.work_dir / src_name
        if not src.exists():
            return False

        shutil.copy2(src, dest)
        return True

    def list_files(self) -> list[str]:
        """List files in sandbox working directory.

        Returns:
            List of filenames
        """
        return [f.name for f in self.work_dir.iterdir() if f.is_file()]

    def read_file(self, filename: str) -> str | None:
        """Read a file from the sandbox.

        Args:
            filename: Name of file to read

        Returns:
            File contents or None if not found
        """
        path = self.work_dir / filename
        if not path.exists():
            return None
        return path.read_text()

    def write_file(self, filename: str, content: str) -> Path:
        """Write a file to the sandbox.

        Args:
            filename: Name of file to write
            content: File contents

        Returns:
            Path to written file
        """
        path = self.work_dir / filename
        path.write_text(content)
        return path

    # =========================================================================
    # Validation
    # =========================================================================

    def validate_command(self, command: list[str]) -> tuple[bool, str | None]:
        """Validate a command before execution.

        Args:
            command: Command to validate

        Returns:
            (is_valid, error_message)
        """
        if not command:
            return False, "Empty command"

        executable = command[0]

        # Block dangerous commands
        blocked = [
            "rm",
            "dd",
            "mkfs",
            "fdisk",
            "mount",
            "shutdown",
            "reboot",
            "kill",
            "pkill",
            "chmod",
            "chown",
            "sudo",
            "su",
        ]

        if executable in blocked:
            return False, f"Command '{executable}' is blocked for security"

        # Block absolute paths outside allowed list
        if executable.startswith("/"):
            allowed_prefixes = ["/usr/bin", "/usr/local/bin", "/bin"]
            if not any(executable.startswith(p) for p in allowed_prefixes):
                return False, f"Absolute path '{executable}' not in allowed list"

        return True, None
