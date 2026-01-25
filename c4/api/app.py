"""C4 Cloud API - FastAPI application factory."""

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .artifact import router as artifact_router
from .chat import router as chat_router
from .proxy import router as proxy_router
from .rate_limit import RateLimitConfig, RateLimitMiddleware, RateLimitStore
from .routes.integrations import router as integrations_router
from .routes.reports import router as reports_router
from .routes.sso import router as sso_router
from .routes.teams import invite_router, router as teams_router
from .routes.webhooks import router as webhooks_router


def create_app(
    title: str = "C4 Cloud API",
    version: str = "0.1.0",
    cors_origins: list[str] | None = None,
    enable_rate_limit: bool = True,
    rate_limit_config: RateLimitConfig | None = None,
) -> FastAPI:
    """Create and configure FastAPI application.

    Args:
        title: API title
        version: API version
        cors_origins: Allowed CORS origins (default: localhost)
        enable_rate_limit: Enable rate limiting middleware
        rate_limit_config: Rate limit configuration

    Returns:
        Configured FastAPI app
    """
    app = FastAPI(
        title=title,
        version=version,
        description="C4 AI Project Orchestration Cloud API",
        docs_url="/api/docs",
        redoc_url="/api/redoc",
        openapi_url="/api/openapi.json",
    )

    # CORS middleware
    if cors_origins is None:
        cors_origins = [
            "http://localhost:3000",
            "http://localhost:8000",
            "http://127.0.0.1:3000",
            "http://127.0.0.1:8000",
        ]

    app.add_middleware(
        CORSMiddleware,
        allow_origins=cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # Rate limiting middleware
    if enable_rate_limit:
        rate_store = RateLimitStore(config=rate_limit_config)
        app.add_middleware(
            RateLimitMiddleware,
            store=rate_store,
            exclude_paths=["/api/health", "/api/docs", "/api/redoc", "/api/openapi.json"],
        )

    # Include routers
    app.include_router(chat_router, prefix="/api/chat", tags=["chat"])
    app.include_router(proxy_router, prefix="/api/llm", tags=["llm-proxy"])
    app.include_router(artifact_router, prefix="/api/artifacts", tags=["artifacts"])
    app.include_router(webhooks_router, prefix="/api", tags=["webhooks"])
    app.include_router(reports_router, prefix="/api", tags=["reports"])

    # Team management routers
    app.include_router(teams_router, prefix="/api", tags=["teams"])
    app.include_router(invite_router, prefix="/api", tags=["invites"])

    # Integration and SSO routers
    app.include_router(integrations_router, prefix="/api", tags=["integrations"])
    app.include_router(sso_router, prefix="/api", tags=["sso"])

    # Health check endpoint
    @app.get("/api/health")
    async def health_check() -> dict[str, str]:
        """Health check endpoint."""
        return {"status": "healthy", "version": version}

    return app


# Default app instance for uvicorn
app = create_app()
