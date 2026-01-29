"""C4 LSP Server CLI entry point

Run the LSP server in stdio or TCP mode:
    # stdio mode (for IDE integration)
    python -m c4.lsp --stdio

    # TCP mode
    python -m c4.lsp --tcp --host 127.0.0.1 --port 8765
"""

import argparse
import asyncio
import logging
from pathlib import Path

from .server import LSPServer


def main() -> None:
    """Main entry point for LSP server."""
    parser = argparse.ArgumentParser(description="C4 LSP Server")
    parser.add_argument(
        "--c4-dir",
        type=Path,
        default=Path.cwd() / ".c4",
        help="Path to .c4 directory (default: ./.c4)",
    )
    parser.add_argument(
        "--stdio",
        action="store_true",
        help="Run in stdio mode (default)",
    )
    parser.add_argument(
        "--tcp",
        action="store_true",
        help="Run in TCP mode",
    )
    parser.add_argument(
        "--host",
        type=str,
        default="127.0.0.1",
        help="TCP host (default: 127.0.0.1)",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8765,
        help="TCP port (default: 8765)",
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging level (default: INFO)",
    )
    parser.add_argument(
        "--log-file",
        type=Path,
        default=None,
        help="Log file path (default: stderr)",
    )

    args = parser.parse_args()

    # Setup logging
    log_handlers: list[logging.Handler] = []
    if args.log_file:
        log_handlers.append(logging.FileHandler(args.log_file))
    else:
        log_handlers.append(logging.StreamHandler())

    logging.basicConfig(
        level=getattr(logging, args.log_level),
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
        handlers=log_handlers,
    )

    # Create server
    server = LSPServer(args.c4_dir)

    # Run in appropriate mode
    if args.tcp:
        asyncio.run(server.run_tcp(args.host, args.port))
    else:
        # Default to stdio
        server.run_stdio()


if __name__ == "__main__":
    main()
