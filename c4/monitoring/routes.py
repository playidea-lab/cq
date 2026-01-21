"""Prometheus metrics API routes.

Provides /metrics endpoint for Prometheus scraping.
"""

from fastapi import APIRouter
from fastapi.responses import Response

from .prometheus import CONTENT_TYPE_LATEST, get_metrics

router = APIRouter(tags=["Metrics"])


@router.get("/metrics")
async def metrics() -> Response:
    """Prometheus metrics endpoint.

    Returns metrics in Prometheus exposition format for scraping.

    Returns:
        Response with Prometheus-formatted metrics
    """
    return Response(
        content=get_metrics(),
        media_type=CONTENT_TYPE_LATEST,
    )
