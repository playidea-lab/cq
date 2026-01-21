"""C4 Workspace Resource Limits - Configurable resource constraints.

This module provides ResourceLimits, a dataclass for configuring
workspace resource constraints including CPU, memory, disk, and timeouts.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


def _validate_positive(value: int | float, name: str) -> None:
    """Validate that a value is positive.

    Args:
        value: Value to validate
        name: Name of the field for error message

    Raises:
        ValueError: If value is not positive
    """
    if value <= 0:
        raise ValueError(f"{name} must be positive, got {value}")


@dataclass
class ResourceLimits:
    """Resource limits for workspace containers.

    Defines CPU, memory, disk, and timeout constraints for workspace
    environments. Provides preset configurations for common use cases
    and conversion to Docker container configuration.

    Attributes:
        cpu_cores: Number of CPU cores (default: 1.0)
        memory_mb: Memory limit in megabytes (default: 2048)
        disk_mb: Disk limit in megabytes (default: 10240)
        timeout_minutes: Maximum execution time in minutes (default: 30)
        idle_timeout_minutes: Inactivity timeout in minutes (default: 60)

    Example:
        # Default limits
        limits = ResourceLimits()

        # Custom limits
        limits = ResourceLimits(cpu_cores=2.0, memory_mb=4096)

        # Use presets
        limits = ResourceLimits.small()  # Lightweight workspaces
        limits = ResourceLimits.large()  # Resource-intensive workspaces

        # Convert to Docker config
        docker_config = limits.to_docker_config()
    """

    cpu_cores: float = field(default=1.0)
    memory_mb: int = field(default=2048)
    disk_mb: int = field(default=10240)
    timeout_minutes: int = field(default=30)
    idle_timeout_minutes: int = field(default=60)

    def __post_init__(self) -> None:
        """Validate all field values after initialization."""
        _validate_positive(self.cpu_cores, "cpu_cores")
        _validate_positive(self.memory_mb, "memory_mb")
        _validate_positive(self.disk_mb, "disk_mb")
        _validate_positive(self.timeout_minutes, "timeout_minutes")
        _validate_positive(self.idle_timeout_minutes, "idle_timeout_minutes")

    @classmethod
    def default(cls) -> ResourceLimits:
        """Create default resource limits.

        Returns:
            ResourceLimits with default values:
            - 1 CPU core
            - 2GB memory
            - 10GB disk
            - 30 min execution timeout
            - 60 min idle timeout
        """
        return cls()

    @classmethod
    def small(cls) -> ResourceLimits:
        """Create small resource limits for lightweight workspaces.

        Suitable for simple tasks, testing, or resource-constrained
        environments.

        Returns:
            ResourceLimits with reduced resources:
            - 0.5 CPU cores
            - 1GB memory
            - 5GB disk
        """
        return cls(
            cpu_cores=0.5,
            memory_mb=1024,
            disk_mb=5120,
        )

    @classmethod
    def large(cls) -> ResourceLimits:
        """Create large resource limits for resource-intensive workspaces.

        Suitable for builds, ML training, or heavy computation.

        Returns:
            ResourceLimits with increased resources:
            - 2 CPU cores
            - 4GB memory
            - 20GB disk
        """
        return cls(
            cpu_cores=2.0,
            memory_mb=4096,
            disk_mb=20480,
        )

    @property
    def idle_timeout_seconds(self) -> int:
        """Get idle timeout in seconds.

        Returns:
            Idle timeout converted to seconds
        """
        return self.idle_timeout_minutes * 60

    @property
    def timeout_seconds(self) -> int:
        """Get execution timeout in seconds.

        Returns:
            Execution timeout converted to seconds
        """
        return self.timeout_minutes * 60

    def to_docker_config(self) -> dict[str, Any]:
        """Convert to Docker container resource configuration.

        Creates a dictionary suitable for passing to Docker container
        creation APIs. Uses nano_cpus for CPU limiting and mem_limit
        for memory constraints.

        Returns:
            Dictionary with Docker resource configuration:
            - nano_cpus: CPU limit in nanoseconds
            - mem_limit: Memory limit as string (e.g., "2048m")

        Note:
            Disk limits are not directly supported by Docker container
            config and require storage driver configuration.
        """
        return {
            "nano_cpus": int(self.cpu_cores * 1_000_000_000),
            "mem_limit": f"{self.memory_mb}m",
        }

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for serialization.

        Returns:
            Dictionary with all resource limit fields
        """
        return {
            "cpu_cores": self.cpu_cores,
            "memory_mb": self.memory_mb,
            "disk_mb": self.disk_mb,
            "timeout_minutes": self.timeout_minutes,
            "idle_timeout_minutes": self.idle_timeout_minutes,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ResourceLimits:
        """Create ResourceLimits from dictionary.

        Args:
            data: Dictionary with resource limit fields.
                  Missing fields use default values.

        Returns:
            ResourceLimits instance
        """
        return cls(
            cpu_cores=data.get("cpu_cores", 1.0),
            memory_mb=data.get("memory_mb", 2048),
            disk_mb=data.get("disk_mb", 10240),
            timeout_minutes=data.get("timeout_minutes", 30),
            idle_timeout_minutes=data.get("idle_timeout_minutes", 60),
        )
