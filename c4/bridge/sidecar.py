"""C4 Bridge Sidecar -- entry point for the Python sidecar process.

Starts the BridgeServer and prints the port to stdout so the Go MCP server
can discover it. Handles SIGINT/SIGTERM for graceful shutdown.

Usage::

    # Via pyproject.toml script entry:
    c4-bridge

    # Or directly:
    uv run python -m c4.bridge.sidecar
"""

from __future__ import annotations

import asyncio
import logging
import os
import signal
import sys
from pathlib import Path

from c4.bridge.rpc_server import BridgeServer

logger = logging.getLogger(__name__)


async def _monitor_parent(stop_event: asyncio.Event, interval: float = 5.0) -> None:
    """Self-terminate when the Go parent process dies.

    The Go MCP server passes its PID via C4_PARENT_PID. Since the sidecar
    runs under a uv wrapper (Go → uv → python), os.getppid() returns uv's
    PID -- not the Go process. We must monitor the Go PID directly.

    This handles the SIGKILL case where the Go process is killed without
    a chance to run cleanup, leaving the sidecar as an orphan.
    """
    raw = os.environ.get("C4_PARENT_PID", "")
    if not raw:
        return
    parent_pid = int(raw)
    if parent_pid <= 0:
        return

    while not stop_event.is_set():
        await asyncio.sleep(interval)
        try:
            os.kill(parent_pid, 0)  # Check if process is alive
        except ProcessLookupError:
            logger.info("Go parent (PID %d) died, shutting down", parent_pid)
            stop_event.set()
            return
        except PermissionError:
            pass  # Process exists (different user — shouldn't happen)


async def _run_server(port: int | None = None) -> None:
    """Start the bridge server and wait until signaled to stop."""
    project_root = Path(os.environ.get("C4_PROJECT_ROOT", ".")).resolve()
    server = BridgeServer(port=port, project_root=project_root)

    port = await server.start()

    # Print port to stdout so the Go parent process can read it.
    # Use a structured format that's easy to parse.
    print(f"C4_BRIDGE_PORT={port}", flush=True)

    # Wait for shutdown signal
    stop_event = asyncio.Event()

    loop = asyncio.get_running_loop()

    def _signal_handler() -> None:
        logger.info("Shutdown signal received")
        stop_event.set()

    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, _signal_handler)

    # Start parent-death monitor (handles SIGKILL on Go process)
    monitor_task = asyncio.create_task(_monitor_parent(stop_event))

    await stop_event.wait()
    monitor_task.cancel()
    await server.stop()


def main() -> None:
    """Entry point for the c4-bridge console script."""
    import argparse

    parser = argparse.ArgumentParser(description="C4 Bridge Sidecar")
    parser.add_argument("--port", type=int, default=None, help="Port to listen on (0=auto, default=50051)")
    args = parser.parse_args()

    logging.basicConfig(
        level=os.environ.get("C4_LOG_LEVEL", "INFO").upper(),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        stream=sys.stderr,  # Logs go to stderr; port goes to stdout
    )

    try:
        asyncio.run(_run_server(port=args.port))
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
