"""Validation API Routes.

Provides endpoints for running validations:
- POST /run - Run validations (lint, test, etc.)
- GET /config - Get validation configuration
"""

import logging
import time
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status

from c4.mcp_server import C4Daemon

from ..deps import require_initialized_project
from ..models import (
    ErrorResponse,
    RunValidationRequest,
    RunValidationResponse,
    ValidationResult,
    ValidationStatus,
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/validation", tags=["Validation"])


@router.post(
    "/run",
    response_model=RunValidationResponse,
    responses={400: {"model": ErrorResponse}},
    summary="Run Validations",
    description="Run specified validations (lint, test, etc.) and return results.",
)
async def run_validations(
    request: RunValidationRequest,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> RunValidationResponse:
    """Run validations and return results."""
    try:
        start_time = time.time()

        result = daemon.c4_run_validation(
            names=request.names,
            fail_fast=request.fail_fast,
            timeout=request.timeout,
        )

        duration = time.time() - start_time

        # Convert results to response format
        validation_results = []
        all_passed = True

        for r in result.get("results", []):
            is_pass = r.get("status") == "pass"
            status_val = ValidationStatus.PASS if is_pass else ValidationStatus.FAIL
            if status_val == ValidationStatus.FAIL:
                all_passed = False

            validation_results.append(
                ValidationResult(
                    name=r.get("name", "unknown"),
                    status=status_val,
                    message=r.get("message"),
                )
            )

        return RunValidationResponse(
            results=validation_results,
            all_passed=all_passed,
            duration_seconds=duration,
        )
    except Exception as e:
        logger.error(f"Failed to run validations: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.get(
    "/config",
    responses={400: {"model": ErrorResponse}},
    summary="Get Validation Config",
    description="Get the current validation configuration.",
)
async def get_validation_config(
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Get validation configuration."""
    try:
        config = daemon.config
        verifications = config.get("verifications", {})
        return {
            "verifications": verifications,
            "available": list(verifications.keys()) if verifications else [],
        }
    except Exception as e:
        logger.error(f"Failed to get validation config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e


@router.post(
    "/add",
    responses={400: {"model": ErrorResponse}},
    summary="Add Verification",
    description="Add a new verification command.",
)
async def add_verification(
    name: str,
    command: str,
    timeout: int = 300,
    daemon: C4Daemon = Depends(require_initialized_project),
) -> dict[str, Any]:
    """Add a new verification command."""
    try:
        result = daemon.c4_add_verification(
            name=name,
            command=command,
            timeout=timeout,
        )
        return result
    except Exception as e:
        logger.error(f"Failed to add verification: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        ) from e
