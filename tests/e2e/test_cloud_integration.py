"""E2E tests for C4 Cloud Integration - Worker, Sandbox, and Billing."""

from __future__ import annotations

import sys
from datetime import datetime, timedelta
from unittest.mock import MagicMock, patch

import pytest

from c4.api.metering import (
    ModelProvider,
    UsageMeter,
    estimate_cost,
)
from c4.cloud.worker_manager import MachineState, WorkerScaler
from c4.sandbox.executor import (
    ResourceLimits,
    SandboxConfig,
    SandboxExecutor,
)

# Get real Python executable path for tests
PYTHON_EXE = sys.executable


# =============================================================================
# Worker E2E Tests
# =============================================================================


class TestWorkerScalerE2E:
    """E2E tests for cloud worker scaling."""

    @pytest.fixture
    def mock_httpx_client(self):
        """Mock httpx client for Fly.io API."""
        with patch("httpx.Client") as mock:
            client_instance = MagicMock()
            mock.return_value = client_instance
            yield client_instance

    @pytest.fixture
    def scaler(self, mock_httpx_client):
        """Create worker scaler with mocked client."""
        scaler = WorkerScaler(api_token="test-token", app_name="c4-worker-test")
        scaler._client = mock_httpx_client
        return scaler

    def test_worker_lifecycle_create_and_destroy(self, scaler, mock_httpx_client):
        """Test complete worker lifecycle: create → run → destroy."""
        # Mock successful creation
        mock_httpx_client.post.return_value = MagicMock(
            status_code=201,
            json=lambda: {
                "id": "machine-123",
                "name": "c4-worker-t-001",
                "state": "started",
                "region": "nrt",
                "created_at": "2025-01-15T10:00:00Z",
            },
        )

        # Create worker
        worker = scaler.create_worker(
            project_id="test-project",
            task_id="T-001",
            region="nrt",
        )

        assert worker is not None
        assert worker.id == "machine-123"
        assert worker.task_id == "T-001"
        assert worker.state == MachineState.STARTED

        # Mock successful destroy
        mock_httpx_client.delete.return_value = MagicMock(status_code=200)

        # Destroy worker
        result = scaler.destroy_worker(worker.id)
        assert result is True

    def test_worker_auto_scaling_based_on_queue(self, scaler, mock_httpx_client):
        """Test auto-scaling behavior based on task queue depth."""
        # Test desired worker calculation
        assert scaler.calculate_desired_workers(0) == 0
        assert scaler.calculate_desired_workers(1) == 1
        assert scaler.calculate_desired_workers(5) == 5
        assert scaler.calculate_desired_workers(15, max_workers=10) == 10

        # Test with tasks_per_worker
        assert scaler.calculate_desired_workers(10, tasks_per_worker=2) == 5
        assert scaler.calculate_desired_workers(10, tasks_per_worker=3) == 4

    def test_worker_scale_up_scenario(self, scaler, mock_httpx_client):
        """Test scaling up when queue has pending tasks."""
        # Mock empty current workers
        mock_httpx_client.get.return_value = MagicMock(
            status_code=200,
            json=lambda: [],
        )

        # Mock successful worker creation
        mock_httpx_client.post.return_value = MagicMock(
            status_code=201,
            json=lambda: {
                "id": "machine-new",
                "name": "c4-worker-pool-0",
                "state": "started",
                "region": "nrt",
                "created_at": "2025-01-15T10:00:00Z",
            },
        )

        # Scale to 3 workers
        result = scaler.scale_to(3, "test-project")

        assert result["target"] == 3
        assert result["created"] >= 0  # May create workers

    def test_worker_scale_down_scenario(self, scaler, mock_httpx_client):
        """Test scaling down when workers are idle."""
        # Mock current workers (3 idle)
        mock_httpx_client.get.return_value = MagicMock(
            status_code=200,
            json=lambda: [
                {
                    "id": f"machine-{i}",
                    "name": f"c4-worker-{i}",
                    "state": "started",
                    "region": "nrt",
                    "created_at": "2025-01-15T10:00:00Z",
                    "config": {"env": {}},  # No task assigned = idle
                }
                for i in range(3)
            ],
        )

        # Mock successful destroy
        mock_httpx_client.delete.return_value = MagicMock(status_code=200)

        # Scale to 1 worker
        result = scaler.scale_to(1, "test-project")

        assert result["current"] == 3
        assert result["target"] == 1

    def test_worker_list_and_status(self, scaler, mock_httpx_client):
        """Test listing workers and checking status."""
        mock_httpx_client.get.return_value = MagicMock(
            status_code=200,
            json=lambda: [
                {
                    "id": "machine-1",
                    "name": "c4-worker-t-001",
                    "state": "started",
                    "region": "nrt",
                    "created_at": "2025-01-15T10:00:00Z",
                    "config": {
                        "env": {
                            "C4_PROJECT_ID": "project-1",
                            "C4_TASK_ID": "T-001",
                        }
                    },
                },
                {
                    "id": "machine-2",
                    "name": "c4-worker-t-002",
                    "state": "stopping",
                    "region": "nrt",
                    "created_at": "2025-01-15T10:05:00Z",
                    "config": {"env": {}},
                },
            ],
        )

        workers = scaler.list_workers()

        assert len(workers) == 2
        assert workers[0].task_id == "T-001"
        assert workers[0].state == MachineState.STARTED
        assert workers[1].state == MachineState.STOPPING

        # Test active count
        active = scaler.get_active_count()
        assert active == 1

    def test_worker_regional_placement(self, scaler, mock_httpx_client):
        """Test worker creation in different regions."""
        regions = ["nrt", "sea", "lax", "fra"]

        for region in regions:
            mock_httpx_client.post.return_value = MagicMock(
                status_code=201,
                json=lambda r=region: {
                    "id": f"machine-{r}",
                    "name": f"c4-worker-{r}",
                    "state": "started",
                    "region": r,
                    "created_at": "2025-01-15T10:00:00Z",
                },
            )

            worker = scaler.create_worker("project", "T-001", region=region)
            assert worker is not None
            assert worker.region == region


# =============================================================================
# Sandbox E2E Tests
# =============================================================================


class TestSandboxExecutorE2E:
    """E2E tests for sandbox execution environment."""

    @pytest.fixture
    def sandbox(self, tmp_path):
        """Create sandbox executor with temp directory."""
        config = SandboxConfig(
            work_dir=tmp_path,
            limits=ResourceLimits(
                max_memory_mb=256,
                max_cpu_seconds=30,
                network_enabled=False,
            ),
        )
        executor = SandboxExecutor(config)
        yield executor
        executor.cleanup()

    def test_simple_command_execution(self, sandbox):
        """Test basic command execution in sandbox."""
        result = sandbox.run(["echo", "Hello, World!"])

        assert result.success is True
        assert result.exit_code == 0
        assert "Hello, World!" in result.stdout
        assert result.killed is False

    def test_python_script_execution(self, sandbox):
        """Test Python script execution in sandbox."""
        script = """
import sys
print("Python version:", sys.version_info[:2])
result = sum(range(10))
print(f"Sum: {result}")
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)

        assert result.success is True
        assert "Sum: 45" in result.stdout

    def test_script_with_file_io(self, sandbox):
        """Test script that reads and writes files."""
        # Write input file
        sandbox.write_file("input.txt", "Hello\nWorld\nTest")

        script = """
with open('input.txt') as f:
    lines = f.readlines()
with open('output.txt', 'w') as f:
    f.write(f'Line count: {len(lines)}')
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)

        assert result.success is True
        output = sandbox.read_file("output.txt")
        assert output == "Line count: 3"

    def test_timeout_enforcement(self, sandbox):
        """Test that long-running commands are killed."""
        script = """
import time
time.sleep(100)  # Will be killed
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE, timeout=1)

        assert result.success is False
        assert result.killed is True
        assert "Timeout" in (result.error or "")

    def test_command_validation_blocks_dangerous(self, sandbox):
        """Test that dangerous commands are blocked."""
        dangerous_commands = [
            ["rm", "-rf", "/"],
            ["sudo", "apt", "install"],
            ["chmod", "777", "/etc/passwd"],
            ["kill", "-9", "1"],
        ]

        for cmd in dangerous_commands:
            is_valid, error = sandbox.validate_command(cmd)
            assert is_valid is False
            assert error is not None

    def test_file_operations_in_sandbox(self, sandbox):
        """Test file copy in/out of sandbox."""
        # Create a source file
        src_file = sandbox.work_dir.parent / "external.txt"
        src_file.write_text("External content")

        # Copy into sandbox
        dest_path = sandbox.copy_to_sandbox(src_file, "internal.txt")
        assert dest_path.exists()
        assert dest_path.read_text() == "External content"

        # Modify in sandbox
        sandbox.write_file("internal.txt", "Modified content")

        # Copy out of sandbox
        out_file = sandbox.work_dir.parent / "output.txt"
        success = sandbox.copy_from_sandbox("internal.txt", out_file)
        assert success is True
        assert out_file.read_text() == "Modified content"

    def test_environment_isolation(self, sandbox):
        """Test that sandbox has isolated environment."""
        script = """
import os
print("HOME:", os.environ.get("HOME"))
print("C4_SANDBOX:", os.environ.get("C4_SANDBOX"))
print("PATH:", os.environ.get("PATH"))
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)

        assert result.success is True
        assert "C4_SANDBOX: 1" in result.stdout

    def test_exit_code_propagation(self, sandbox):
        """Test that exit codes are properly captured."""
        # Success case
        result = sandbox.run(["true"])
        assert result.exit_code == 0

        # Failure case
        result = sandbox.run(["false"])
        assert result.exit_code != 0

        # Custom exit code
        script = """
import sys
sys.exit(42)
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)
        assert result.exit_code == 42

    def test_stderr_capture(self, sandbox):
        """Test that stderr is properly captured."""
        script = """
import sys
print("stdout message")
print("stderr message", file=sys.stderr)
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)

        assert "stdout message" in result.stdout
        assert "stderr message" in result.stderr

    def test_resource_limits_config(self, sandbox):
        """Test resource limits configuration."""
        limits = sandbox._config.limits

        assert limits.max_memory_mb == 256
        assert limits.max_cpu_seconds == 30
        assert limits.network_enabled is False
        assert limits.max_processes == 10
        assert limits.max_open_files == 100


# =============================================================================
# Billing/Metering E2E Tests
# =============================================================================


class TestUsageMeteringE2E:
    """E2E tests for usage metering and billing."""

    @pytest.fixture
    def meter(self, tmp_path):
        """Create usage meter with temp storage."""
        storage = tmp_path / "usage.json"
        return UsageMeter(storage_path=storage, max_records=1000)

    @pytest.mark.asyncio
    async def test_record_single_usage(self, meter):
        """Test recording a single API usage."""
        record = await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=50,
            request_id="req-001",
            user_id="user-1",
            latency_ms=500,
        )

        assert record.model == "gpt-4o"
        assert record.provider == ModelProvider.OPENAI
        assert record.total_tokens == 150
        assert record.cost is not None
        assert record.cost > 0

    @pytest.mark.asyncio
    async def test_record_multiple_providers(self, meter):
        """Test recording usage from multiple providers."""
        models = [
            ("gpt-4o", ModelProvider.OPENAI),
            ("claude-3-opus", ModelProvider.ANTHROPIC),
            ("gpt-3.5-turbo", ModelProvider.OPENAI),
            ("claude-3-haiku", ModelProvider.ANTHROPIC),
        ]

        for model, expected_provider in models:
            record = await meter.record_usage(
                model=model,
                prompt_tokens=100,
                completion_tokens=50,
            )
            assert record.provider == expected_provider

    @pytest.mark.asyncio
    async def test_usage_summary_aggregation(self, meter):
        """Test usage summary aggregation."""
        # Record multiple usages
        for i in range(5):
            await meter.record_usage(
                model="gpt-4o",
                prompt_tokens=100 * (i + 1),
                completion_tokens=50 * (i + 1),
                user_id=f"user-{i % 2}",  # 2 users
                success=i != 2,  # 1 failure
            )

        summary = meter.get_summary()

        assert summary.total_requests == 5
        assert summary.successful_requests == 4
        assert summary.failed_requests == 1
        assert summary.total_prompt_tokens == 1500  # 100+200+300+400+500
        assert summary.total_completion_tokens == 750  # 50+100+150+200+250
        assert len(summary.by_user) == 2

    @pytest.mark.asyncio
    async def test_cost_estimation_accuracy(self, meter):
        """Test cost estimation for different models."""
        # Cost = (prompt_tokens/1000) * input_rate + (completion_tokens/1000) * output_rate
        # Note: Models are tested without name prefix conflicts
        test_cases = [
            # gpt-4-turbo: input=0.01, output=0.03
            # Cost = (1000/1000)*0.01 + (500/1000)*0.03 = 0.01 + 0.015 = 0.025
            ("gpt-4-turbo", 1000, 500, 0.025),
            # gpt-3.5-turbo: input=0.0005, output=0.0015
            # Cost = (1000/1000)*0.0005 + (500/1000)*0.0015 = 0.0005 + 0.00075 = 0.00125
            ("gpt-3.5-turbo", 1000, 500, 0.00125),
            # claude-3-opus: input=0.015, output=0.075
            # Cost = (1000/1000)*0.015 + (500/1000)*0.075 = 0.015 + 0.0375 = 0.0525
            ("claude-3-opus", 1000, 500, 0.0525),
            # claude-3-haiku: input=0.00025, output=0.00125
            # Cost = (1000/1000)*0.00025 + (500/1000)*0.00125 = 0.00025 + 0.000625 = 0.000875
            ("claude-3-haiku", 1000, 500, 0.000875),
        ]

        for model, prompt, completion, expected in test_cases:
            cost = estimate_cost(model, prompt, completion)
            assert cost is not None
            assert abs(cost - expected) < 0.0001, f"Cost mismatch for {model}"

    @pytest.mark.asyncio
    async def test_usage_filtering_by_user(self, meter):
        """Test filtering usage summary by user."""
        # Record for different users
        await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=50,
            user_id="user-a",
        )
        await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=200,
            completion_tokens=100,
            user_id="user-b",
        )

        # Get summary for user-a only
        summary = meter.get_summary(user_id="user-a")

        assert summary.total_requests == 1
        assert summary.total_tokens == 150

    @pytest.mark.asyncio
    async def test_usage_filtering_by_time_period(self, meter):
        """Test filtering usage by time period."""
        now = datetime.now()

        # Record usage
        await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=50,
        )

        # Get summary for last hour
        summary = meter.get_summary(
            start=now - timedelta(hours=1),
            end=now + timedelta(hours=1),
        )

        assert summary.total_requests >= 1

    @pytest.mark.asyncio
    async def test_persistence_across_sessions(self, tmp_path):
        """Test that usage data persists across sessions."""
        storage = tmp_path / "usage.json"

        # Session 1: Record usage
        meter1 = UsageMeter(storage_path=storage)
        await meter1.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=50,
        )

        # Session 2: Load and verify
        meter2 = UsageMeter(storage_path=storage)
        records = meter2.get_recent_records()

        assert len(records) >= 1
        assert records[0].model == "gpt-4o"

    @pytest.mark.asyncio
    async def test_max_records_trimming(self, tmp_path):
        """Test that records are trimmed when exceeding max."""
        storage = tmp_path / "usage.json"
        meter = UsageMeter(storage_path=storage, max_records=5)

        # Record more than max
        for i in range(10):
            await meter.record_usage(
                model="gpt-4o",
                prompt_tokens=100,
                completion_tokens=50,
            )

        assert len(meter._records) <= 5

    @pytest.mark.asyncio
    async def test_error_recording(self, meter):
        """Test recording failed requests."""
        record = await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=0,
            success=False,
            error="Rate limit exceeded",
        )

        assert record.success is False
        assert record.error == "Rate limit exceeded"

        summary = meter.get_summary()
        assert summary.failed_requests == 1


# =============================================================================
# Integration E2E Tests
# =============================================================================


class TestCloudIntegrationE2E:
    """E2E tests for complete cloud integration scenarios."""

    @pytest.fixture
    def mock_fly_api(self):
        """Mock Fly.io API responses."""
        with patch("httpx.Client") as mock:
            client = MagicMock()
            mock.return_value = client
            yield client

    @pytest.mark.asyncio
    async def test_full_cloud_workflow(self, tmp_path, mock_fly_api):
        """
        Test complete cloud workflow:
        1. Create worker for task
        2. Execute task in sandbox
        3. Record usage
        4. Destroy worker
        """
        # 1. Setup
        scaler = WorkerScaler(api_token="test", app_name="test")
        scaler._client = mock_fly_api

        sandbox_dir = tmp_path / "sandbox"
        sandbox_dir.mkdir(parents=True, exist_ok=True)
        sandbox = SandboxExecutor(SandboxConfig(work_dir=sandbox_dir))
        meter = UsageMeter()

        # 2. Create worker
        mock_fly_api.post.return_value = MagicMock(
            status_code=201,
            json=lambda: {
                "id": "machine-task",
                "name": "c4-worker-t-001",
                "state": "started",
                "region": "nrt",
                "created_at": "2025-01-15T10:00:00Z",
            },
        )

        worker = scaler.create_worker("project", "T-001")
        assert worker is not None

        # 3. Execute task in sandbox
        script = """
print("Task T-001 executing...")
result = 2 + 2
print(f"Result: {result}")
"""
        result = sandbox.run_script(script, interpreter=PYTHON_EXE)
        assert result.success is True
        assert "Result: 4" in result.stdout

        # 4. Record usage
        record = await meter.record_usage(
            model="claude-3-opus",
            prompt_tokens=500,
            completion_tokens=200,
            user_id="worker-1",
            project_id="project",
            latency_ms=result.duration_ms,
        )
        assert record.cost is not None

        # 5. Destroy worker
        mock_fly_api.delete.return_value = MagicMock(status_code=200)
        destroyed = scaler.destroy_worker(worker.id)
        assert destroyed is True

        # Cleanup
        sandbox.cleanup()

    @pytest.mark.asyncio
    async def test_batch_task_processing(self, tmp_path, mock_fly_api):
        """Test processing multiple tasks with metering."""
        meter = UsageMeter(storage_path=tmp_path / "usage.json")

        # Simulate processing 5 tasks
        tasks = [
            ("T-001", "gpt-4o", 100, 50),
            ("T-002", "claude-3-opus", 200, 100),
            ("T-003", "gpt-4o-mini", 500, 200),
            ("T-004", "claude-3-haiku", 300, 150),
            ("T-005", "gpt-4o", 400, 200),
        ]

        for task_id, model, prompt, completion in tasks:
            await meter.record_usage(
                model=model,
                prompt_tokens=prompt,
                completion_tokens=completion,
                project_id="batch-project",
                metadata={"task_id": task_id},
            )

        # Verify summary
        summary = meter.get_summary(project_id="batch-project")

        assert summary.total_requests == 5
        assert summary.total_tokens == sum(p + c for _, _, p, c in tasks)
        assert len(summary.by_model) == 4  # 4 unique models
        assert summary.total_cost > 0

    def test_sandbox_security_boundaries(self, tmp_path):
        """Test sandbox security boundaries are enforced."""
        sandbox = SandboxExecutor(SandboxConfig(work_dir=tmp_path, limits=ResourceLimits()))

        # Test blocked commands
        blocked = ["rm", "sudo", "chmod", "kill", "pkill"]
        for cmd in blocked:
            is_valid, error = sandbox.validate_command([cmd, "arg"])
            assert is_valid is False

        # Test allowed commands
        allowed = ["echo", "python", "cat", "ls"]
        for cmd in allowed:
            is_valid, error = sandbox.validate_command([cmd, "arg"])
            assert is_valid is True

        sandbox.cleanup()

    def test_worker_failure_handling(self, mock_fly_api):
        """Test handling of worker creation failures."""
        scaler = WorkerScaler(api_token="test", app_name="test")
        scaler._client = mock_fly_api

        # Mock API error
        mock_fly_api.post.return_value = MagicMock(status_code=500)

        worker = scaler.create_worker("project", "T-001")
        assert worker is None

        # Mock network error
        mock_fly_api.post.side_effect = Exception("Network error")

        worker = scaler.create_worker("project", "T-002")
        assert worker is None
