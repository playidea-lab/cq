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

    Shell:
        POST /api/shell/run                - Run shell command
        POST /api/shell/run-validation     - Run validations

    Health:
        GET  /health                       - Health check
        GET  /                             - API info
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .deps import clear_daemon_cache
from .routes import c4, design, discovery, git, shell, validation

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
) -> FastAPI:
    """Create and configure the FastAPI application.

    Args:
        title: API title for docs
        description: API description for docs
        version: API version
        cors_origins: Allowed CORS origins (None = allow all in dev)

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

    # Include routers
    app.include_router(c4.router, prefix="/api")
    app.include_router(discovery.router, prefix="/api")
    app.include_router(design.router, prefix="/api")
    app.include_router(validation.router, prefix="/api")
    app.include_router(git.router, prefix="/api")
    app.include_router(shell.router, prefix="/api")

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
                "shell": "/api/shell",
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
