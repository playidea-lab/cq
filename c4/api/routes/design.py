"""Design Phase API Routes.

Provides endpoints for the Design phase:
- POST /save-design - Save design specification
- GET /designs - List all designs
- GET /designs/{feature} - Get specific design
- POST /complete - Mark design complete
"""

import logging
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import require_initialized_project
from ..models import (
    ArchitectureOption,
    ComponentDesign,
    DesignDecision,
    DesignResponse,
    ErrorResponse,
    ListDesignsResponse,
    SaveDesignRequest,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/design", tags=["Design Phase"])


@router.post(
    "/save-design",
    responses={400: {"model": ErrorResponse}},
    summary="Save Design Specification",
    description="Save a design specification with architecture options and decisions.",
)
async def save_design(
    request: SaveDesignRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Save a design specification."""
    try:
        # Convert to dict format
        options = [
            {
                "id": o.id,
                "name": o.name,
                "description": o.description,
                "pros": o.pros,
                "cons": o.cons,
                "complexity": o.complexity,
                "recommended": o.recommended,
            }
            for o in request.options
        ]

        components = [
            {
                "name": c.name,
                "type": c.type,
                "description": c.description,
                "responsibilities": c.responsibilities,
                "interfaces": c.interfaces,
                "dependencies": c.dependencies,
            }
            for c in request.components
        ]

        decisions = [
            {
                "id": d.id,
                "question": d.question,
                "decision": d.decision,
                "rationale": d.rationale,
                "alternatives_considered": d.alternatives_considered,
            }
            for d in request.decisions
        ]

        result = daemon.c4_save_design(
            feature=request.feature,
            domain=request.domain,
            description=request.description,
            options=options,
            selected_option=request.selected_option,
            components=components,
            decisions=decisions,
            constraints=request.constraints,
            nfr=request.nfr,
            mermaid_diagram=request.mermaid_diagram,
        )
        return result
    except Exception as e:
        logger.error(f"Failed to save design: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/designs",
    response_model=ListDesignsResponse,
    responses={400: {"model": ErrorResponse}},
    summary="List Designs",
    description="List all saved design specifications.",
)
async def list_designs(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> ListDesignsResponse:
    """List all saved designs."""
    try:
        result = daemon.c4_list_designs()
        return ListDesignsResponse(designs=result.get("designs", []))
    except Exception as e:
        logger.error(f"Failed to list designs: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/designs/{feature}",
    response_model=DesignResponse,
    responses={404: {"model": ErrorResponse}},
    summary="Get Design",
    description="Get a specific design specification by feature name.",
)
async def get_design(
    feature: str,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> DesignResponse:
    """Get a specific design specification."""
    try:
        result = daemon.c4_get_design(feature)

        if not result or "error" in result:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Design '{feature}' not found",
            )

        # Convert options
        options = [
            ArchitectureOption(
                id=o.get("id", ""),
                name=o.get("name", ""),
                description=o.get("description", ""),
                pros=o.get("pros", []),
                cons=o.get("cons", []),
                complexity=o.get("complexity"),
                recommended=o.get("recommended", False),
            )
            for o in result.get("options", [])
        ]

        # Convert components
        components = [
            ComponentDesign(
                name=c.get("name", ""),
                type=c.get("type", ""),
                description=c.get("description", ""),
                responsibilities=c.get("responsibilities", []),
                interfaces=c.get("interfaces", []),
                dependencies=c.get("dependencies", []),
            )
            for c in result.get("components", [])
        ]

        # Convert decisions
        decisions = [
            DesignDecision(
                id=d.get("id", ""),
                question=d.get("question", ""),
                decision=d.get("decision", ""),
                rationale=d.get("rationale", ""),
                alternatives_considered=d.get("alternatives_considered", []),
            )
            for d in result.get("decisions", [])
        ]

        return DesignResponse(
            feature=result.get("feature", feature),
            domain=result.get("domain", "unknown"),
            description=result.get("description"),
            options=options,
            selected_option=result.get("selected_option"),
            components=components,
            decisions=decisions,
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to get design: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/complete",
    responses={400: {"model": ErrorResponse}},
    summary="Complete Design Phase",
    description="Mark design phase as complete and transition to PLAN state.",
)
async def complete_design(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Mark design phase as complete."""
    try:
        result = daemon.c4_design_complete()
        return result
    except Exception as e:
        logger.error(f"Failed to complete design: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
