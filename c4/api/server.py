"""C4 API Server - FastAPI application.

A RESTful HTTP API that wraps C4 MCP tools for web-based orchestration.

Usage:
    # Development server
    uvicorn c4.api.server:app --reload --port 8000

    # Production server
    uvicorn c4.api.server:app --host 0.0.0.0 --port 8000 --workers 4

    # With custom project root
    C4_PROJECT_ROOT=/path/to/project uvicorn c4.api.server:app

Endpoints:
    Core:
        GET  /api/c4/status          - Get C4 status
        POST /api/c4/get-task        - Get task assignment
        POST /api/c4/submit          - Submit completed task
        POST /api/c4/add-task        - Add new task
        POST /api/c4/start           - Start execution
        POST /api/c4/checkpoint      - Record checkpoint

    Discovery:
        POST /api/discovery/save-spec      - Save specification
        GET  /api/discovery/specs          - List specifications
        GET  /api/discovery/specs/{name}   - Get specification
        POST /api/discovery/complete       - Complete discovery

    Design:
        POST /api/design/save-design       - Save design
        GET  /api/design/designs           - List designs
        GET  /api/design/designs/{name}    - Get design
        POST /api/design/complete          - Complete design

    Validation:
        POST /api/validation/run           - Run validations
        GET  /api/validation/config        - Get config

    Git:
        GET  /api/git/status               - Git status
        POST /api/git/commit               - Create commit
        GET  /api/git/log                  - Get log
        GET  /api/git/diff                 - Get diff

    Files:
        POST /api/files/read               - Read file
        POST /api/files/write              - Write file
        POST /api/files/list               - List directory
        POST /api/files/search             - Search files (glob/grep)
        DELETE /api/files/delete           - Delete file

    Shell:
        POST /api/shell/run                - Run shell command
        POST /api/shell/run-validation     - Run validations

    Workspace:
        POST /api/workspace/create         - Create workspace from git repo
        GET  /api/workspace/list           - List user's workspaces
        GET  /api/workspace/{id}           - Get workspace details
        DELETE /api/workspace/{id}         - Delete workspace
        GET  /api/workspace/{id}/status    - Get workspace status/resources
        POST /api/workspace/{id}/exec      - Execute command in workspace

    Chat:
        POST /api/chat/message             - Send chat message (SSE streaming)
        GET  /api/chat/history/{id}        - Get conversation history
        DELETE /api/chat/history/{id}      - Clear conversation
        POST /api/chat/workspace/bind      - Bind workspace to conversation

    Health:
        GET  /health                       - Health check
        GET  /                             - API info
"""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .chat import router as chat_router
from .deps import clear_daemon_cache
from .middleware import BrandingMiddleware
from .routes import branding, c4, design, discovery, docs, files, git, integrations, reports, shell, sso, teams, validation, workspace

if TYPE_CHECKING:
    from c4.services.branding import BrandingService

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    # Startup
    logger.info("C4 API Server starting...")
    yield
    # Shutdown
    logger.info("C4 API Server shutting down...")
    clear_daemon_cache()


def create_app(
    title: str = "C4 API",
    description: str = "HTTP API for C4 AI Orchestration System",
    version: str = "0.1.0",
    cors_origins: list[str] | None = None,
    branding_service: BrandingService | None = None,
    branding_cache_ttl: float = 60.0,
) -> FastAPI:
    """Create and configure the FastAPI application.

    Args:
        title: API title for docs
        description: API description for docs
        version: API version
        cors_origins: Allowed CORS origins (None = allow all in dev)
        branding_service: Optional branding service for white-label support
        branding_cache_ttl: TTL for branding cache in seconds (default 60)

    Returns:
        Configured FastAPI application
    """
    app = FastAPI(
        title=title,
        description=description,
        version=version,
        lifespan=lifespan,
        docs_url="/docs",
        redoc_url="/redoc",
        openapi_url="/openapi.json",
    )

    # Configure CORS
    if cors_origins is None:
        # Development: allow all origins
        cors_origins = ["*"]

    app.add_middleware(
        CORSMiddleware,
        allow_origins=cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # Add branding middleware if service is provided
    if branding_service is not None:
        app.add_middleware(
            BrandingMiddleware,
            branding_service=branding_service,
            cache_ttl=branding_cache_ttl,
        )
        logger.info("Branding middleware enabled with TTL=%s seconds", branding_cache_ttl)

    # Include routers
    app.include_router(c4.router, prefix="/api")
    app.include_router(discovery.router, prefix="/api")
    app.include_router(design.router, prefix="/api")
    app.include_router(validation.router, prefix="/api")
    app.include_router(git.router, prefix="/api")
    app.include_router(files.router, prefix="/api")
    app.include_router(shell.router, prefix="/api")
    app.include_router(workspace.router, prefix="/api")
    app.include_router(chat_router, prefix="/api/chat", tags=["Chat"])
    app.include_router(teams.router, prefix="/api")
    app.include_router(teams.invite_router, prefix="/api")
    app.include_router(branding.router, prefix="/api")
    app.include_router(branding.public_router, prefix="/api")
    app.include_router(integrations.router, prefix="/api")
    app.include_router(reports.router, prefix="/api")
    app.include_router(sso.router, prefix="/api")
    app.include_router(docs.router, prefix="/api")

    # Health check endpoint
    @app.get("/health", tags=["Health"])
    async def health_check():
        """Health check endpoint."""
        return {"status": "healthy", "service": "c4-api"}

    # Root endpoint
    @app.get("/", tags=["Info"])
    async def root():
        """API information endpoint."""
        return {
            "name": "C4 API",
            "version": version,
            "docs": "/docs",
            "openapi": "/openapi.json",
            "endpoints": {
                "c4": "/api/c4",
                "discovery": "/api/discovery",
                "design": "/api/design",
                "validation": "/api/validation",
                "git": "/api/git",
                "files": "/api/files",
                "shell": "/api/shell",
                "workspace": "/api/workspace",
                "chat": "/api/chat",
                "teams": "/api/teams",
                "invites": "/api/invites",
                "branding": "/api/teams/{team_id}/branding",
                "integrations": "/api/integrations",
                "reports": "/api/reports",
                "sso": "/api/sso",
                "docs": "/api/docs",
            },
        }

    return app


# Default application instance
app = create_app()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "c4.api.server:app",
        host="0.0.0.0",
        port=8000,
        reload=True,
        log_level="info",
    )
