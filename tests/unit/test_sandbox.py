"""Tests for Sandbox Executor."""

from pathlib import Path

from c4.sandbox.executor import (
    ExecutionResult,
    ResourceLimits,
    SandboxConfig,
    SandboxExecutor,
)


class TestResourceLimits:
    """Test ResourceLimits dataclass."""

    def test_defaults(self) -> None:
        """Test default limits."""
        limits = ResourceLimits()

        assert limits.max_memory_mb == 512
        assert limits.max_cpu_seconds == 300
        assert limits.network_enabled is False

    def test_custom_limits(self) -> None:
        """Test custom limits."""
        limits = ResourceLimits(
            max_memory_mb=256,
            max_cpu_seconds=60,
            network_enabled=True,
        )

        assert limits.max_memory_mb == 256
        assert limits.max_cpu_seconds == 60
        assert limits.network_enabled is True


class TestSandboxConfig:
    """Test SandboxConfig dataclass."""

    def test_defaults(self) -> None:
        """Test default config."""
        config = SandboxConfig()

        assert config.work_dir is None
        assert config.allowed_paths == []
        assert config.env_vars == {}

    def test_with_work_dir(self, tmp_path: Path) -> None:
        """Test config with work directory."""
        config = SandboxConfig(work_dir=tmp_path)
        assert config.work_dir == tmp_path


class TestExecutionResult:
    """Test ExecutionResult dataclass."""

    def test_success_result(self) -> None:
        """Test successful execution result."""
        result = ExecutionResult(
            success=True,
            exit_code=0,
            stdout="Hello",
            stderr="",
            duration_ms=100,
        )

        assert result.success is True
        assert result.exit_code == 0
        assert result.killed is False

    def test_failed_result(self) -> None:
        """Test failed execution result."""
        result = ExecutionResult(
            success=False,
            exit_code=1,
            stdout="",
            stderr="Error",
            duration_ms=50,
            error="Command failed",
        )

        assert result.success is False
        assert result.error == "Command failed"

    def test_killed_result(self) -> None:
        """Test killed execution result."""
        result = ExecutionResult(
            success=False,
            exit_code=-1,
            stdout="",
            stderr="",
            duration_ms=5000,
            killed=True,
            error="Timeout",
        )

        assert result.killed is True


class TestSandboxExecutorInit:
    """Test SandboxExecutor initialization."""

    def test_default_init(self) -> None:
        """Test default initialization."""
        with SandboxExecutor() as executor:
            assert executor._config is not None
            assert executor.work_dir.exists()

    def test_with_config(self, tmp_path: Path) -> None:
        """Test initialization with config."""
        config = SandboxConfig(work_dir=tmp_path)

        with SandboxExecutor(config) as executor:
            assert executor.work_dir == tmp_path

    def test_cleanup(self) -> None:
        """Test cleanup removes temp directory."""
        executor = SandboxExecutor()
        work_dir = executor.work_dir

        assert work_dir.exists()

        executor.cleanup()

        assert not work_dir.exists()


class TestSandboxExecution:
    """Test command execution."""

    def test_run_simple_command(self) -> None:
        """Test running a simple command."""
        with SandboxExecutor() as executor:
            result = executor.run(["echo", "hello"])

            assert result.success is True
            assert "hello" in result.stdout

    def test_run_with_exit_code(self) -> None:
        """Test running command with non-zero exit."""
        with SandboxExecutor() as executor:
            result = executor.run(["bash", "-c", "exit 42"])

            assert result.success is False
            assert result.exit_code == 42

    def test_run_with_stderr(self) -> None:
        """Test running command that outputs to stderr."""
        with SandboxExecutor() as executor:
            result = executor.run(["bash", "-c", "echo error >&2"])

            assert "error" in result.stderr

    def test_run_with_timeout(self) -> None:
        """Test command timeout."""
        with SandboxExecutor() as executor:
            result = executor.run(["sleep", "10"], timeout=1)

            assert result.success is False
            assert result.killed is True
            assert "Timeout" in (result.error or "")

    def test_run_script(self) -> None:
        """Test running a Python script."""
        with SandboxExecutor() as executor:
            script = "print('Hello from script')"
            result = executor.run_script(script, interpreter="python3")

            assert result.success is True
            assert "Hello from script" in result.stdout


class TestFileOperations:
    """Test sandbox file operations."""

    def test_write_and_read_file(self) -> None:
        """Test writing and reading files."""
        with SandboxExecutor() as executor:
            executor.write_file("test.txt", "Hello World")

            content = executor.read_file("test.txt")

            assert content == "Hello World"

    def test_read_nonexistent_file(self) -> None:
        """Test reading non-existent file."""
        with SandboxExecutor() as executor:
            content = executor.read_file("nonexistent.txt")
            assert content is None

    def test_list_files(self) -> None:
        """Test listing files."""
        with SandboxExecutor() as executor:
            executor.write_file("file1.txt", "1")
            executor.write_file("file2.txt", "2")

            files = executor.list_files()

            assert "file1.txt" in files
            assert "file2.txt" in files

    def test_copy_to_sandbox(self, tmp_path: Path) -> None:
        """Test copying file into sandbox."""
        # Create source file
        src = tmp_path / "source.txt"
        src.write_text("Source content")

        with SandboxExecutor() as executor:
            dest = executor.copy_to_sandbox(src)

            assert dest.exists()
            assert dest.read_text() == "Source content"

    def test_copy_from_sandbox(self, tmp_path: Path) -> None:
        """Test copying file out of sandbox."""
        with SandboxExecutor() as executor:
            executor.write_file("output.txt", "Output content")

            dest = tmp_path / "output.txt"
            success = executor.copy_from_sandbox("output.txt", dest)

            assert success is True
            assert dest.read_text() == "Output content"


class TestCommandValidation:
    """Test command validation."""

    def test_valid_command(self) -> None:
        """Test valid command passes."""
        executor = SandboxExecutor()
        valid, error = executor.validate_command(["python", "script.py"])

        assert valid is True
        assert error is None

    def test_empty_command(self) -> None:
        """Test empty command fails."""
        executor = SandboxExecutor()
        valid, error = executor.validate_command([])

        assert valid is False
        assert "Empty command" in error

    def test_blocked_command(self) -> None:
        """Test blocked commands fail."""
        executor = SandboxExecutor()

        blocked = ["rm", "sudo", "chmod", "kill"]
        for cmd in blocked:
            valid, error = executor.validate_command([cmd])
            assert valid is False
            assert "blocked" in error.lower()

    def test_allowed_absolute_path(self) -> None:
        """Test allowed absolute paths pass."""
        executor = SandboxExecutor()
        valid, error = executor.validate_command(["/usr/bin/python"])

        assert valid is True

    def test_blocked_absolute_path(self) -> None:
        """Test blocked absolute paths fail."""
        executor = SandboxExecutor()
        valid, error = executor.validate_command(["/etc/passwd"])

        assert valid is False
        assert "not in allowed list" in error


class TestEnvironment:
    """Test environment setup."""

    def test_sandbox_env_marker(self) -> None:
        """Test C4_SANDBOX env is set."""
        with SandboxExecutor() as executor:
            result = executor.run(["bash", "-c", "echo $C4_SANDBOX"])

            assert "1" in result.stdout

    def test_custom_env_vars(self) -> None:
        """Test custom environment variables."""
        config = SandboxConfig(env_vars={"MY_VAR": "my_value"})

        with SandboxExecutor(config) as executor:
            result = executor.run(["bash", "-c", "echo $MY_VAR"])

            assert "my_value" in result.stdout

    def test_network_disabled_marker(self) -> None:
        """Test network disabled marker."""
        config = SandboxConfig(limits=ResourceLimits(network_enabled=False))

        with SandboxExecutor(config) as executor:
            result = executor.run(["bash", "-c", "echo $C4_NETWORK_DISABLED"])

            assert "1" in result.stdout
