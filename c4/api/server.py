"""C4 API Server - FastAPI application setup."""

from contextlib import asynccontextmanager
from datetime import datetime
from typing import Any

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .chat import router as chat_router
from .models import HealthResponse


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    yield


def create_app(
    title: str = "C4 API",
    description: str = "Chat API for C4 Development Platform",
    version: str = "0.1.0",
    cors_origins: list[str] | None = None,
) -> FastAPI:
    """Create and configure FastAPI application."""
    app = FastAPI(
        title=title,
        description=description,
        version=version,
        lifespan=lifespan,
        docs_url="/api/docs",
        redoc_url="/api/redoc",
        openapi_url="/api/openapi.json",
    )

    if cors_origins is None:
        cors_origins = [
            "http://localhost:3000",
            "http://localhost:5173",
            "http://localhost:8000",
        ]

    app.add_middleware(
        CORSMiddleware,
        allow_origins=cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    app.include_router(chat_router)

    @app.get("/health", response_model=HealthResponse)
    async def health_check() -> HealthResponse:
        """Health check endpoint."""
        return HealthResponse(
            status="ok",
            version=version,
            timestamp=datetime.now(),
        )

    @app.get("/")
    async def root() -> dict[str, Any]:
        """Root endpoint with API info."""
        return {
            "name": title,
            "version": version,
            "docs": "/api/docs",
            "health": "/health",
        }

    return app


app = create_app()
