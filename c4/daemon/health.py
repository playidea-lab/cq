"""C4 Daemon Health Monitor - Periodic health checking for daemon services.

This module provides the HealthMonitor class for monitoring the health of
various C4 daemon components including MCP server, LSP server, and Socket server.
"""

from __future__ import annotations

import asyncio
import logging
import socket
import sys
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Any, Callable

logger = logging.getLogger(__name__)


class ServiceStatus(str, Enum):
    """Status of a monitored service."""

    HEALTHY = "healthy"
    UNHEALTHY = "unhealthy"
    UNKNOWN = "unknown"
    NOT_CONFIGURED = "not_configured"


@dataclass
class ServiceHealth:
    """Health information for a single service.

    Attributes:
        name: Service name (mcp, lsp, socket)
        status: Current health status
        last_check: Time of last health check
        error: Error message if unhealthy
        response_time_ms: Response time in milliseconds
        details: Additional service-specific details
    """

    name: str
    status: ServiceStatus
    last_check: datetime = field(default_factory=datetime.now)
    error: str | None = None
    response_time_ms: float | None = None
    details: dict[str, Any] = field(default_factory=dict)


@dataclass
class OverallHealth:
    """Overall health status of all services.

    Attributes:
        healthy: True if all services are healthy
        services: Individual service health statuses
        checked_at: Time of health check
    """

    healthy: bool
    services: dict[str, ServiceHealth] = field(default_factory=dict)
    checked_at: datetime = field(default_factory=datetime.now)


@dataclass
class HealthMonitorConfig:
    """Configuration for HealthMonitor.

    Attributes:
        check_interval_sec: Interval between health checks (default: 30s)
        mcp_host: MCP server host (default: 127.0.0.1)
        mcp_port: MCP server port (default: 4100)
        lsp_host: LSP server host (default: 127.0.0.1)
        lsp_port: LSP server port (default: 4200)
        socket_host: Socket/WebSocket server host (default: 127.0.0.1)
        socket_port: Socket/WebSocket server port (default: 4000)
        timeout_sec: Timeout for health check connections
        enable_mcp: Whether to monitor MCP server
        enable_lsp: Whether to monitor LSP server
        enable_socket: Whether to monitor Socket server
    """

    check_interval_sec: float = 30.0
    mcp_host: str = "127.0.0.1"
    mcp_port: int = 4100
    lsp_host: str = "127.0.0.1"
    lsp_port: int = 4200
    socket_host: str = "127.0.0.1"
    socket_port: int = 4000
    timeout_sec: float = 5.0
    enable_mcp: bool = True
    enable_lsp: bool = True
    enable_socket: bool = True


class HealthMonitor:
    """Monitors health of C4 daemon services.

    Periodically checks the health of configured services (MCP, LSP, Socket)
    and logs results. Can be used to detect and alert on service failures.

    Example:
        >>> config = HealthMonitorConfig(check_interval_sec=30)
        >>> monitor = HealthMonitor(c4_dir=Path(".c4"), config=config)
        >>> await monitor.start()
        >>> health = await monitor.check_health()
        >>> print(f"All services healthy: {health.healthy}")
        >>> await monitor.stop()
    """

    def __init__(
        self,
        c4_dir: Path,
        config: HealthMonitorConfig | None = None,
        on_unhealthy: Callable[[ServiceHealth], Any] | None = None,
    ) -> None:
        """Initialize HealthMonitor.

        Args:
            c4_dir: Path to .c4 directory
            config: Health monitor configuration
            on_unhealthy: Callback when a service becomes unhealthy
        """
        self._c4_dir = c4_dir
        self._config = config or HealthMonitorConfig()
        self._on_unhealthy = on_unhealthy
        self._running = False
        self._task: asyncio.Task | None = None
        self._last_health: OverallHealth | None = None

        # Setup logging to file
        self._setup_logging()

    @property
    def log_file(self) -> Path:
        """Path to daemon log file."""
        return self._c4_dir / "logs" / "daemon.log"

    @property
    def is_running(self) -> bool:
        """Check if monitor is running."""
        return self._running

    @property
    def last_health(self) -> OverallHealth | None:
        """Get last health check result."""
        return self._last_health

    def _setup_logging(self) -> None:
        """Setup logging to daemon.log file."""
        logs_dir = self._c4_dir / "logs"
        logs_dir.mkdir(parents=True, exist_ok=True)

        # Create file handler for daemon log
        file_handler = logging.FileHandler(self.log_file)
        file_handler.setLevel(logging.INFO)
        file_handler.setFormatter(
            logging.Formatter(
                "%(asctime)s [%(levelname)s] %(name)s: %(message)s",
                datefmt="%Y-%m-%d %H:%M:%S",
            )
        )

        # Add handler to this module's logger
        module_logger = logging.getLogger(__name__)
        module_logger.addHandler(file_handler)
        module_logger.setLevel(logging.INFO)

    # =========================================================================
    # Health Check Methods
    # =========================================================================

    async def check_health(self) -> OverallHealth:
        """Perform health check on all configured services.

        Returns:
            OverallHealth with status of all services
        """
        services: dict[str, ServiceHealth] = {}

        # Check MCP server
        if self._config.enable_mcp:
            services["mcp"] = await self._check_tcp_service(
                "mcp",
                self._config.mcp_host,
                self._config.mcp_port,
            )

        # Check LSP server
        if self._config.enable_lsp:
            services["lsp"] = await self._check_tcp_service(
                "lsp",
                self._config.lsp_host,
                self._config.lsp_port,
            )

        # Check Socket server
        if self._config.enable_socket:
            services["socket"] = await self._check_tcp_service(
                "socket",
                self._config.socket_host,
                self._config.socket_port,
            )

        # Determine overall health
        all_healthy = all(
            s.status == ServiceStatus.HEALTHY
            for s in services.values()
            if s.status != ServiceStatus.NOT_CONFIGURED
        )

        health = OverallHealth(
            healthy=all_healthy,
            services=services,
            checked_at=datetime.now(),
        )

        self._last_health = health
        return health

    async def _check_tcp_service(
        self,
        name: str,
        host: str,
        port: int,
    ) -> ServiceHealth:
        """Check if a TCP service is reachable.

        Args:
            name: Service name for logging
            host: Host address
            port: Port number

        Returns:
            ServiceHealth with check results
        """
        start_time = datetime.now()

        try:
            # Try to connect with timeout
            reader, writer = await asyncio.wait_for(
                asyncio.open_connection(host, port),
                timeout=self._config.timeout_sec,
            )

            # Connection successful
            writer.close()
            await writer.wait_closed()

            response_time = (datetime.now() - start_time).total_seconds() * 1000

            return ServiceHealth(
                name=name,
                status=ServiceStatus.HEALTHY,
                last_check=datetime.now(),
                response_time_ms=response_time,
                details={"host": host, "port": port},
            )

        except asyncio.TimeoutError:
            return ServiceHealth(
                name=name,
                status=ServiceStatus.UNHEALTHY,
                last_check=datetime.now(),
                error=f"Connection timeout after {self._config.timeout_sec}s",
                details={"host": host, "port": port},
            )

        except ConnectionRefusedError:
            return ServiceHealth(
                name=name,
                status=ServiceStatus.UNHEALTHY,
                last_check=datetime.now(),
                error="Connection refused - service may not be running",
                details={"host": host, "port": port},
            )

        except OSError as e:
            return ServiceHealth(
                name=name,
                status=ServiceStatus.UNHEALTHY,
                last_check=datetime.now(),
                error=str(e),
                details={"host": host, "port": port},
            )

    # =========================================================================
    # Monitor Loop
    # =========================================================================

    async def start(self) -> None:
        """Start the health monitor background task.

        The monitor will periodically check service health and log results.
        """
        if self._running:
            logger.warning("Health monitor already running")
            return

        self._running = True
        self._task = asyncio.create_task(self._monitor_loop())
        logger.info(
            f"Health monitor started (interval: {self._config.check_interval_sec}s)"
        )

    async def stop(self) -> None:
        """Stop the health monitor."""
        if not self._running:
            return

        self._running = False

        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None

        logger.info("Health monitor stopped")

    async def _monitor_loop(self) -> None:
        """Background loop for periodic health checks."""
        while self._running:
            try:
                health = await self.check_health()
                self._log_health(health)

                # Call unhealthy callback if any service is unhealthy
                if not health.healthy and self._on_unhealthy:
                    for service in health.services.values():
                        if service.status == ServiceStatus.UNHEALTHY:
                            self._on_unhealthy(service)

            except Exception as e:
                logger.error(f"Health check error: {e}")

            # Wait for next check interval
            try:
                await asyncio.sleep(self._config.check_interval_sec)
            except asyncio.CancelledError:
                break

    def _log_health(self, health: OverallHealth) -> None:
        """Log health check results.

        Args:
            health: Health check results to log
        """
        if health.healthy:
            logger.info(
                f"Health check passed - all services healthy "
                f"({len(health.services)} services checked)"
            )
        else:
            # Log to both file and stderr for unhealthy services
            unhealthy_services = [
                s.name
                for s in health.services.values()
                if s.status == ServiceStatus.UNHEALTHY
            ]
            message = (
                f"Health check WARNING - unhealthy services: "
                f"{', '.join(unhealthy_services)}"
            )
            logger.warning(message)

            # Also print to stderr for immediate visibility
            print(f"[C4 Health] {message}", file=sys.stderr)

            # Log detailed error for each unhealthy service
            for service in health.services.values():
                if service.status == ServiceStatus.UNHEALTHY:
                    error_msg = service.error or "Unknown error"
                    logger.warning(
                        f"  - {service.name}: {error_msg} "
                        f"(host={service.details.get('host')}, "
                        f"port={service.details.get('port')})"
                    )

    # =========================================================================
    # Utility Methods
    # =========================================================================

    def get_service_status(self, name: str) -> ServiceStatus:
        """Get status of a specific service from last check.

        Args:
            name: Service name (mcp, lsp, socket)

        Returns:
            ServiceStatus or UNKNOWN if no check performed
        """
        if self._last_health is None:
            return ServiceStatus.UNKNOWN

        service = self._last_health.services.get(name)
        if service is None:
            return ServiceStatus.NOT_CONFIGURED

        return service.status

    async def wait_for_healthy(
        self,
        timeout_sec: float = 60.0,
        check_interval_sec: float = 2.0,
    ) -> bool:
        """Wait for all services to become healthy.

        Useful for waiting after daemon startup.

        Args:
            timeout_sec: Maximum time to wait
            check_interval_sec: Interval between checks

        Returns:
            True if all services became healthy, False on timeout
        """
        deadline = asyncio.get_event_loop().time() + timeout_sec

        while asyncio.get_event_loop().time() < deadline:
            health = await self.check_health()
            if health.healthy:
                return True
            await asyncio.sleep(check_interval_sec)

        return False


def check_port_available(host: str, port: int) -> bool:
    """Check if a TCP port is available (not in use).

    Args:
        host: Host address
        port: Port number

    Returns:
        True if port is available, False if in use
    """
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.settimeout(1)
        result = sock.connect_ex((host, port))
        # If connect returns 0, something is listening (port in use)
        return result != 0
    except socket.error:
        return True
    finally:
        sock.close()


def check_service_reachable(host: str, port: int, timeout: float = 5.0) -> bool:
    """Synchronously check if a TCP service is reachable.

    Args:
        host: Host address
        port: Port number
        timeout: Connection timeout in seconds

    Returns:
        True if service is reachable
    """
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.settimeout(timeout)
        result = sock.connect_ex((host, port))
        return result == 0
    except socket.error:
        return False
    finally:
        sock.close()
