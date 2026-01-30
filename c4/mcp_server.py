"""C4D MCP Server - Main server implementation with MCP tools"""

import json
import logging
from pathlib import Path

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from .daemon import C4Daemon

# Import handlers to register them (side effect)
from .mcp import handlers as _handlers  # noqa: F401

# Import tool registry and handlers
from .mcp.registry import tool_registry

logger = logging.getLogger(__name__)


def _use_graph_router() -> bool:
    """Check if GraphRouter should be used (feature flag).

    The C4_USE_GRAPH_ROUTER environment variable controls which routing system to use:
    - True (default): Use GraphRouter with skill matching and rule engine
    - False: Use legacy AgentRouter with static domain mapping

    Returns:
        True if GraphRouter should be used, False for legacy AgentRouter.
    """
    import os
    flag = os.environ.get("C4_USE_GRAPH_ROUTER", "true").lower()
    return flag in ("true", "1", "yes", "on")


def _get_workflow_guide(status: str) -> dict[str, str]:
    """Get workflow guide for current project status.

    Provides hints for next actions, useful for all MCP clients
    (Claude Code, Codex CLI, Gemini CLI, etc.)

    Args:
        status: Current project status (e.g., "INIT", "EXECUTE")

    Returns:
        Dict with phase, next action, and hint for the LLM
    """
    guides: dict[str, dict[str, str]] = {
        "INIT": {
            "phase": "init",
            "next": "discovery",
            "hint": (
                "Start planning: scan docs/*.md for requirements, "
                "detect project domain, collect EARS requirements, "
                "call c4_save_spec() for each feature, "
                "then c4_discovery_complete() when done"
            ),
        },
        "DISCOVERY": {
            "phase": "discovery",
            "next": "design",
            "hint": (
                "Continue collecting requirements using EARS patterns. "
                "Call c4_save_spec() for each feature, "
                "then c4_discovery_complete() to proceed to design"
            ),
        },
        "DESIGN": {
            "phase": "design",
            "next": "plan",
            "hint": (
                "Define architecture options for each feature. "
                "Call c4_save_design() with components and decisions, "
                "then c4_design_complete() to proceed to planning"
            ),
        },
        "PLAN": {
            "phase": "plan",
            "next": "execute",
            "hint": (
                "Tasks are ready. Call c4_start() to begin execution, "
                "then use c4_get_task(worker_id) in a loop to process tasks"
            ),
        },
        "EXECUTE": {
            "phase": "execute",
            "next": "worker_loop",
            "hint": (
                "Worker loop: call c4_get_task(worker_id) to get a task, "
                "implement it, run validations with c4_run_validation(), "
                "then c4_submit(task_id, commit_sha, validation_results)"
            ),
        },
        "CHECKPOINT": {
            "phase": "checkpoint",
            "next": "review",
            "hint": (
                "Supervisor review in progress. "
                "Wait for c4_ensure_supervisor() to complete the review, "
                "or call c4_checkpoint() to manually process"
            ),
        },
        "HALTED": {
            "phase": "halted",
            "next": "resume",
            "hint": (
                "Execution is paused. Call c4_start() to resume, "
                "or review repair_queue for blocked tasks"
            ),
        },
        "COMPLETE": {
            "phase": "complete",
            "next": "done",
            "hint": "Project is complete. All tasks have been processed.",
        },
    }
    return guides.get(status, {"phase": "unknown", "next": "unknown", "hint": "Unknown status"})

def create_server(project_root: Path | None = None) -> Server:
    """Create the MCP server with all tools registered"""
    import os

    server = Server("c4d")

    # Cache of daemons per project root
    _daemon_cache: dict[str, C4Daemon] = {}

    def get_daemon() -> C4Daemon:
        """Get or create a daemon for the current project root"""
        # Determine project root: env var > param > cwd
        if os.environ.get("C4_PROJECT_ROOT"):
            root = Path(os.environ["C4_PROJECT_ROOT"])
        elif project_root:
            root = project_root
        else:
            root = Path.cwd()

        root_str = str(root.resolve())

        if root_str not in _daemon_cache:
            daemon = C4Daemon(root)
            if daemon.is_initialized():
                daemon.load()
                # Auto-restart supervisor loop if in EXECUTE/CHECKPOINT state
                daemon._auto_restart_supervisor_if_needed()
            _daemon_cache[root_str] = daemon

        return _daemon_cache[root_str]

    def clear_daemon_cache(project_root_str: str | None = None) -> bool:
        """Clear daemon cache for a specific project or all projects"""
        if project_root_str:
            if project_root_str in _daemon_cache:
                del _daemon_cache[project_root_str]
                return True
            return False
        else:
            _daemon_cache.clear()
            return True

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        from .mcp.tool_schemas import get_tool_definitions
        return get_tool_definitions()

    @server.call_tool()
    async def call_tool(name: str, arguments: dict) -> list[TextContent]:
        try:
            # Get daemon dynamically for the current project
            daemon = get_daemon()

            # Dispatch to registered handler
            result = tool_registry.dispatch(name, daemon, arguments)

            return [TextContent(type="text", text=json.dumps(result, indent=2, default=str))]

        except Exception as e:
            return [TextContent(type="text", text=json.dumps({"error": str(e)}))]

    return server


async def main():
    """Run the MCP server"""
    from mcp.server import InitializationOptions
    from mcp.types import ServerCapabilities, ToolsCapability

    server = create_server()
    init_options = InitializationOptions(
        server_name="c4d",
        server_version="0.1.0",
        capabilities=ServerCapabilities(
            tools=ToolsCapability(listChanged=False),
        ),
    )
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, init_options)


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
