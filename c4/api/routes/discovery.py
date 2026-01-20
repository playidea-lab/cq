"""Discovery Phase API Routes.

Provides endpoints for the Discovery phase:
- POST /save-spec - Save feature specification
- GET /specs - List all specifications
- GET /specs/{feature} - Get specific specification
- POST /complete - Mark discovery complete
"""

import logging
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import require_initialized_project
from ..models import (
    ErrorResponse,
    ListSpecsResponse,
    SaveSpecRequest,
    SpecResponse,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/discovery", tags=["Discovery Phase"])


@router.post(
    "/save-spec",
    responses={400: {"model": ErrorResponse}},
    summary="Save Feature Specification",
    description="Save a feature specification with EARS requirements.",
)
async def save_spec(
    request: SaveSpecRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Save a feature specification."""
    try:
        # Convert requirements to dict format
        requirements = [
            {
                "id": r.id,
                "text": r.text,
                "pattern": r.pattern.value if r.pattern else None,
            }
            for r in request.requirements
        ]

        result = daemon.c4_save_spec(
            feature=request.feature,
            requirements=requirements,
            domain=request.domain,
            description=request.description,
        )
        return result
    except Exception as e:
        logger.error(f"Failed to save spec: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/specs",
    response_model=ListSpecsResponse,
    responses={400: {"model": ErrorResponse}},
    summary="List Specifications",
    description="List all saved feature specifications.",
)
async def list_specs(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> ListSpecsResponse:
    """List all saved specifications."""
    try:
        result = daemon.c4_list_specs()
        return ListSpecsResponse(specs=result.get("specs", []))
    except Exception as e:
        logger.error(f"Failed to list specs: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/specs/{feature}",
    response_model=SpecResponse,
    responses={404: {"model": ErrorResponse}},
    summary="Get Specification",
    description="Get a specific feature specification by name.",
)
async def get_spec(
    feature: str,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> SpecResponse:
    """Get a specific feature specification."""
    try:
        result = daemon.c4_get_spec(feature)

        if not result or "error" in result:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Specification '{feature}' not found",
            )

        # Convert requirements
        requirements = []
        for r in result.get("requirements", []):
            from ..models import EarsPattern, Requirement

            pattern = None
            if r.get("pattern"):
                try:
                    pattern = EarsPattern(r["pattern"])
                except ValueError:
                    pass

            requirements.append(
                Requirement(
                    id=r.get("id", ""),
                    text=r.get("text", ""),
                    pattern=pattern,
                )
            )

        return SpecResponse(
            feature=result.get("feature", feature),
            requirements=requirements,
            domain=result.get("domain", "unknown"),
            description=result.get("description"),
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to get spec: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/complete",
    responses={400: {"model": ErrorResponse}},
    summary="Complete Discovery Phase",
    description="Mark discovery phase as complete and transition to DESIGN state.",
)
async def complete_discovery(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Mark discovery phase as complete."""
    try:
        result = daemon.c4_discovery_complete()
        return result
    except Exception as e:
        logger.error(f"Failed to complete discovery: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
