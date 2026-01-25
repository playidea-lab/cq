"""Documentation API Routes.

Provides public API endpoints for documentation:
- GET /docs/snapshots - List all documentation snapshots
- POST /docs/snapshots - Create a new documentation snapshot
- GET /docs/snapshots/{version} - Get documentation for a specific version
- GET /docs/snapshots/{version}/compare/{other_version} - Compare two versions
- DELETE /docs/snapshots/{version} - Delete a snapshot
"""

import logging
from enum import Enum
from pathlib import Path
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Query, status
from pydantic import BaseModel, Field

from c4.mcp_server import C4Daemon

from ..deps import require_initialized_project
from ..models import ErrorResponse

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/docs", tags=["Documentation"])


class DocFormatParam(str, Enum):
    """Documentation output format."""

    markdown = "markdown"
    json = "json"
    html = "html"


class SnapshotMetadata(BaseModel):
    """Snapshot metadata."""

    version: str
    created_at: str
    commit_hash: str | None
    description: str | None
    files_count: int
    symbols_count: int
    content_hash: str


class CreateSnapshotRequest(BaseModel):
    """Request to create a documentation snapshot."""

    version: str = Field(..., description="Version identifier (e.g., 'v1.0.0')")
    description: str | None = Field(None, description="Optional description")


class ListSnapshotsResponse(BaseModel):
    """Response for listing snapshots."""

    snapshots: list[SnapshotMetadata]


class SnapshotDiffResponse(BaseModel):
    """Response for snapshot comparison."""

    from_version: str
    to_version: str
    added_symbols: list[str]
    removed_symbols: list[str]
    modified_symbols: list[str]
    added_files: list[str]
    removed_files: list[str]
    summary: str


def _get_doc_generator(daemon: C4Daemon):
    """Get or create DocGenerator for the project."""
    from c4.mcp.docs_server import DocGenerator

    # Use the project root from daemon's config
    project_root = Path(daemon._state_manager._c4_dir).parent

    # Create doc generator (lazily indexed)
    return DocGenerator(project_root)


@router.get(
    "/snapshots",
    response_model=ListSnapshotsResponse,
    responses={500: {"model": ErrorResponse}},
    summary="List Documentation Snapshots",
    description="List all available documentation snapshots.",
)
async def list_snapshots(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> ListSnapshotsResponse:
    """List all documentation snapshots."""
    try:
        doc_gen = _get_doc_generator(daemon)
        snapshots = doc_gen.list_snapshots()

        return ListSnapshotsResponse(
            snapshots=[
                SnapshotMetadata(
                    version=s.version,
                    created_at=s.created_at,
                    commit_hash=s.commit_hash,
                    description=s.description,
                    files_count=s.files_count,
                    symbols_count=s.symbols_count,
                    content_hash=s.content_hash,
                )
                for s in snapshots
            ]
        )
    except Exception as e:
        logger.error(f"Failed to list snapshots: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/snapshots",
    response_model=SnapshotMetadata,
    responses={400: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
    summary="Create Documentation Snapshot",
    description="Create a versioned documentation snapshot from current codebase.",
)
async def create_snapshot(
    request: CreateSnapshotRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> SnapshotMetadata:
    """Create a new documentation snapshot."""
    try:
        doc_gen = _get_doc_generator(daemon)
        snapshot = doc_gen.create_snapshot(
            version=request.version,
            description=request.description,
        )

        return SnapshotMetadata(
            version=snapshot.version,
            created_at=snapshot.created_at,
            commit_hash=snapshot.commit_hash,
            description=snapshot.description,
            files_count=snapshot.files_count,
            symbols_count=snapshot.symbols_count,
            content_hash=snapshot.content_hash,
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        ) from e
    except Exception as e:
        logger.error(f"Failed to create snapshot: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/snapshots/{version}",
    responses={404: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
    summary="Get Documentation Snapshot",
    description="Get documentation for a specific snapshot version.",
)
async def get_snapshot(
    version: str,
    format: DocFormatParam = Query(
        DocFormatParam.markdown,
        description="Output format (markdown, json, or html)",
    ),
    daemon: C4Daemon = Depends(require_initialized_project),
) -> Any:
    """Get documentation for a specific version."""
    try:
        from c4.mcp.docs_server import DocFormat

        doc_gen = _get_doc_generator(daemon)

        # Map format parameter to DocFormat enum
        format_map = {
            DocFormatParam.markdown: DocFormat.MARKDOWN,
            DocFormatParam.json: DocFormat.JSON,
            DocFormatParam.html: DocFormat.HTML,
        }

        result = doc_gen.get_snapshot(
            version=version,
            format=format_map[format],
        )

        # Handle error responses
        if isinstance(result, dict) and "error" in result:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=result["error"],
            )

        return result
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to get snapshot: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/snapshots/{version}/compare/{other_version}",
    responses={404: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
    summary="Compare Documentation Snapshots",
    description="Compare two documentation snapshots and show differences.",
)
async def compare_snapshots(
    version: str,
    other_version: str,
    format: DocFormatParam = Query(
        DocFormatParam.markdown,
        description="Output format (markdown or json)",
    ),
    daemon: C4Daemon = Depends(require_initialized_project),
) -> Any:
    """Compare two documentation snapshots."""
    try:
        from c4.mcp.docs_server import DocFormat

        doc_gen = _get_doc_generator(daemon)

        # Map format parameter to DocFormat enum
        format_map = {
            DocFormatParam.markdown: DocFormat.MARKDOWN,
            DocFormatParam.json: DocFormat.JSON,
            DocFormatParam.html: DocFormat.MARKDOWN,  # HTML not supported for diff
        }

        result = doc_gen.compare_snapshots(
            from_version=version,
            to_version=other_version,
            format=format_map[format],
        )

        # Handle error responses
        if isinstance(result, dict) and "error" in result:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=result["error"],
            )

        return result
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to compare snapshots: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.delete(
    "/snapshots/{version}",
    responses={404: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
    summary="Delete Documentation Snapshot",
    description="Delete a documentation snapshot.",
)
async def delete_snapshot(
    version: str,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, str]:
    """Delete a documentation snapshot."""
    try:
        doc_gen = _get_doc_generator(daemon)
        success = doc_gen.delete_snapshot(version)

        if not success:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Snapshot '{version}' not found",
            )

        return {"message": f"Snapshot '{version}' deleted successfully"}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to delete snapshot: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
