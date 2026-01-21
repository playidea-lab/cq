"""Tests for C4 Workspace Resource Limits.

TDD: RED phase - These tests define the expected behavior.
"""

from __future__ import annotations

import pytest


class TestResourceLimits:
    """Tests for ResourceLimits dataclass."""

    def test_default_values(self):
        """Test default resource limits."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits()

        assert limits.cpu_cores == 1.0
        assert limits.memory_mb == 2048
        assert limits.disk_mb == 10240
        assert limits.timeout_minutes == 30
        assert limits.idle_timeout_minutes == 60

    def test_custom_values(self):
        """Test custom resource limits."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits(
            cpu_cores=2.0,
            memory_mb=4096,
            disk_mb=20480,
            timeout_minutes=60,
            idle_timeout_minutes=120,
        )

        assert limits.cpu_cores == 2.0
        assert limits.memory_mb == 4096
        assert limits.disk_mb == 20480
        assert limits.timeout_minutes == 60
        assert limits.idle_timeout_minutes == 120

    def test_default_preset(self):
        """Test default preset factory method."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits.default()

        assert limits.cpu_cores == 1.0
        assert limits.memory_mb == 2048
        assert limits.disk_mb == 10240
        assert limits.timeout_minutes == 30
        assert limits.idle_timeout_minutes == 60

    def test_small_preset(self):
        """Test small preset for lightweight workspaces."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits.small()

        assert limits.cpu_cores == 0.5
        assert limits.memory_mb == 1024
        assert limits.disk_mb == 5120

    def test_large_preset(self):
        """Test large preset for resource-intensive workspaces."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits.large()

        assert limits.cpu_cores == 2.0
        assert limits.memory_mb == 4096
        assert limits.disk_mb == 20480


class TestResourceLimitsToDockerConfig:
    """Tests for Docker configuration conversion."""

    def test_to_docker_config_default(self):
        """Test Docker config conversion with defaults."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits()
        config = limits.to_docker_config()

        # 1 CPU = 1_000_000_000 nanoseconds
        assert config["nano_cpus"] == 1_000_000_000
        assert config["mem_limit"] == "2048m"

    def test_to_docker_config_small(self):
        """Test Docker config for small preset."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits.small()
        config = limits.to_docker_config()

        # 0.5 CPU = 500_000_000 nanoseconds
        assert config["nano_cpus"] == 500_000_000
        assert config["mem_limit"] == "1024m"

    def test_to_docker_config_large(self):
        """Test Docker config for large preset."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits.large()
        config = limits.to_docker_config()

        # 2 CPU = 2_000_000_000 nanoseconds
        assert config["nano_cpus"] == 2_000_000_000
        assert config["mem_limit"] == "4096m"

    def test_to_docker_config_fractional_cpu(self):
        """Test Docker config with fractional CPU."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits(cpu_cores=0.25)
        config = limits.to_docker_config()

        # 0.25 CPU = 250_000_000 nanoseconds
        assert config["nano_cpus"] == 250_000_000


class TestResourceLimitsIdleTimeout:
    """Tests for idle timeout calculations."""

    def test_idle_timeout_seconds(self):
        """Test conversion to idle timeout in seconds."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits(idle_timeout_minutes=60)

        assert limits.idle_timeout_seconds == 3600

    def test_timeout_seconds(self):
        """Test conversion to execution timeout in seconds."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits(timeout_minutes=30)

        assert limits.timeout_seconds == 1800


class TestResourceLimitsValidation:
    """Tests for resource limits validation."""

    def test_negative_cpu_cores_raises(self):
        """Test that negative CPU cores raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="cpu_cores must be positive"):
            ResourceLimits(cpu_cores=-1.0)

    def test_zero_cpu_cores_raises(self):
        """Test that zero CPU cores raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="cpu_cores must be positive"):
            ResourceLimits(cpu_cores=0)

    def test_negative_memory_raises(self):
        """Test that negative memory raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="memory_mb must be positive"):
            ResourceLimits(memory_mb=-1)

    def test_zero_memory_raises(self):
        """Test that zero memory raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="memory_mb must be positive"):
            ResourceLimits(memory_mb=0)

    def test_negative_disk_raises(self):
        """Test that negative disk raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="disk_mb must be positive"):
            ResourceLimits(disk_mb=-1)

    def test_zero_disk_raises(self):
        """Test that zero disk raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="disk_mb must be positive"):
            ResourceLimits(disk_mb=0)

    def test_negative_timeout_raises(self):
        """Test that negative timeout raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="timeout_minutes must be positive"):
            ResourceLimits(timeout_minutes=-1)

    def test_negative_idle_timeout_raises(self):
        """Test that negative idle timeout raises ValueError."""
        from c4.workspace.limits import ResourceLimits

        with pytest.raises(ValueError, match="idle_timeout_minutes must be positive"):
            ResourceLimits(idle_timeout_minutes=-1)


class TestResourceLimitsPydanticCompat:
    """Tests for Pydantic compatibility."""

    def test_to_dict(self):
        """Test conversion to dictionary."""
        from c4.workspace.limits import ResourceLimits

        limits = ResourceLimits()
        data = limits.to_dict()

        assert data == {
            "cpu_cores": 1.0,
            "memory_mb": 2048,
            "disk_mb": 10240,
            "timeout_minutes": 30,
            "idle_timeout_minutes": 60,
        }

    def test_from_dict(self):
        """Test creation from dictionary."""
        from c4.workspace.limits import ResourceLimits

        data = {
            "cpu_cores": 2.0,
            "memory_mb": 4096,
            "disk_mb": 20480,
            "timeout_minutes": 60,
            "idle_timeout_minutes": 120,
        }
        limits = ResourceLimits.from_dict(data)

        assert limits.cpu_cores == 2.0
        assert limits.memory_mb == 4096
        assert limits.disk_mb == 20480
        assert limits.timeout_minutes == 60
        assert limits.idle_timeout_minutes == 120

    def test_from_dict_partial(self):
        """Test creation from partial dictionary uses defaults."""
        from c4.workspace.limits import ResourceLimits

        data = {"cpu_cores": 2.0}
        limits = ResourceLimits.from_dict(data)

        assert limits.cpu_cores == 2.0
        assert limits.memory_mb == 2048  # default
        assert limits.disk_mb == 10240  # default
