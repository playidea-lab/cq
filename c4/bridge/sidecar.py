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


async def _run_server() -> None:
    """Start the bridge server and wait until signaled to stop."""
    project_root = Path(os.environ.get("C4_PROJECT_ROOT", ".")).resolve()
    server = BridgeServer(project_root=project_root)

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

    await stop_event.wait()
    await server.stop()


def main() -> None:
    """Entry point for the c4-bridge console script."""
    logging.basicConfig(
        level=os.environ.get("C4_LOG_LEVEL", "INFO").upper(),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        stream=sys.stderr,  # Logs go to stderr; port goes to stdout
    )

    try:
        asyncio.run(_run_server())
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
