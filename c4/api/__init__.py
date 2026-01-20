"""C4 API Server - FastAPI wrapper for MCP tools.

Provides HTTP endpoints for C4 orchestration:
- /api/c4/* - Core orchestration (status, tasks, submit)
- /api/discovery/* - Discovery phase (specs, requirements)
- /api/design/* - Design phase (architecture, decisions)
- /api/validation/* - Validation execution
- /api/git/* - Git operations (commit, status)

Usage:
    # Start server
    uvicorn c4.api.server:app --reload

    # Or programmatically
    from c4.api import create_app
    app = create_app()
"""

from .server import app, create_app

__all__ = ["app", "create_app"]
