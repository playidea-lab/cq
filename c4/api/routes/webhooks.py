"""Webhook API Routes.

Provides endpoints for external webhooks:
- POST /github - Handle GitHub App webhooks (PR events)
"""

import logging
from typing import Any

from fastapi import APIRouter, BackgroundTasks, Header, HTTPException, Request, status
from pydantic import BaseModel, Field

from c4.config.github_app import GitHubAppConfig, GitHubAppCredentials
from c4.integrations.github_app import GitHubAppClient
from c4.services.pr_review import PRReviewService

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/webhooks", tags=["Webhooks"])


# =============================================================================
# Response Models
# =============================================================================


class WebhookResponse(BaseModel):
    """Response for webhook processing."""

    success: bool = Field(..., description="Whether webhook was accepted")
    message: str = Field(..., description="Status message")
    action: str | None = Field(None, description="Action taken")


class WebhookErrorResponse(BaseModel):
    """Error response for webhook failures."""

    error: str = Field(..., description="Error type")
    message: str = Field(..., description="Error message")


# =============================================================================
# Dependencies
# =============================================================================


def get_github_app_client() -> GitHubAppClient | None:
    """Get GitHub App client from configuration.

    Returns client if properly configured, None otherwise.
    """
    config = GitHubAppConfig(enabled=True)
    credentials = GitHubAppCredentials.from_config(config)

    if not credentials:
        return None

    return GitHubAppClient(
        app_id=credentials.app_id,
        private_key=credentials.private_key,
        webhook_secret=credentials.webhook_secret,
    )


def get_pr_review_service(client: GitHubAppClient) -> PRReviewService:
    """Get PR review service.

    Args:
        client: GitHub App client

    Returns:
        Configured PRReviewService
    """
    config = GitHubAppConfig(enabled=True)
    return PRReviewService(
        github_client=client,
        model=config.review_model,
        max_diff_size=config.max_diff_size,
    )


# =============================================================================
# Background Tasks
# =============================================================================


async def process_pr_review(
    client: GitHubAppClient,
    payload: dict[str, Any],
) -> None:
    """Process PR review in background.

    Args:
        client: GitHub App client
        payload: Webhook payload
    """
    pr_info = client.parse_pr_webhook(payload)
    if not pr_info:
        logger.warning("Failed to parse PR info from payload")
        return

    # Create check run to show processing status
    client.create_check_run(
        pr_info=pr_info,
        name="C4 Code Review",
        status="in_progress",
    )

    try:
        # Run review
        service = get_pr_review_service(client)
        result = await service.review_pr(pr_info)

        # Update check run with result
        conclusion = "success" if result.success else "failure"
        client.create_check_run(
            pr_info=pr_info,
            name="C4 Code Review",
            status="completed",
            conclusion=conclusion,
            output={
                "title": "Code Review Complete",
                "summary": result.message,
            },
        )

        logger.info(f"PR review completed: {result.message}")

    except Exception as e:
        logger.error(f"PR review failed: {e}")
        client.create_check_run(
            pr_info=pr_info,
            name="C4 Code Review",
            status="completed",
            conclusion="failure",
            output={
                "title": "Code Review Failed",
                "summary": str(e),
            },
        )


# =============================================================================
# Routes
# =============================================================================


@router.post(
    "/github",
    response_model=WebhookResponse,
    responses={
        400: {"model": WebhookErrorResponse},
        401: {"model": WebhookErrorResponse},
        503: {"model": WebhookErrorResponse},
    },
    summary="GitHub Webhook",
    description="Handle GitHub App webhook events (PR opened, synchronized, reopened).",
)
async def handle_github_webhook(
    request: Request,
    background_tasks: BackgroundTasks,
    x_github_event: str = Header(..., alias="X-GitHub-Event"),
    x_hub_signature_256: str = Header(..., alias="X-Hub-Signature-256"),
    x_github_delivery: str = Header(..., alias="X-GitHub-Delivery"),
) -> WebhookResponse:
    """Handle GitHub webhook events.

    This endpoint receives webhooks from GitHub App installations.
    It verifies the signature and processes PR events asynchronously.

    Security:
    - Verifies webhook signature using HMAC-SHA256
    - Rejects requests with invalid signatures
    - Processes in background to respond quickly
    """
    # Get GitHub App client
    client = get_github_app_client()
    if not client:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="GitHub App not configured. Set GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, and GITHUB_WEBHOOK_SECRET environment variables.",
        )

    # Read raw body for signature verification
    body = await request.body()

    # Verify signature (CRITICAL security check)
    if not client.verify_webhook_signature(body, x_hub_signature_256):
        logger.warning(f"Invalid webhook signature for delivery {x_github_delivery}")
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid webhook signature",
        )

    # Parse payload
    try:
        payload = await request.json()
    except Exception as e:
        logger.error(f"Failed to parse webhook payload: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid JSON payload",
        )

    # Log event
    logger.info(f"Received GitHub webhook: {x_github_event} (delivery: {x_github_delivery})")

    # Handle different event types
    if x_github_event == "pull_request":
        action = payload.get("action", "")

        # Only process specific actions
        if action not in ("opened", "synchronize", "reopened"):
            return WebhookResponse(
                success=True,
                message=f"Ignored PR action: {action}",
                action=None,
            )

        # Process in background
        background_tasks.add_task(process_pr_review, client, payload)

        return WebhookResponse(
            success=True,
            message="PR review queued",
            action="review_queued",
        )

    elif x_github_event == "ping":
        # GitHub sends ping on app installation
        return WebhookResponse(
            success=True,
            message="Pong! Webhook configured successfully.",
            action="ping",
        )

    elif x_github_event == "installation":
        # App installed/uninstalled
        action = payload.get("action", "")
        installation_id = payload.get("installation", {}).get("id")
        logger.info(f"GitHub App installation {action}: {installation_id}")

        return WebhookResponse(
            success=True,
            message=f"Installation {action}",
            action=f"installation_{action}",
        )

    else:
        # Acknowledge other events without processing
        return WebhookResponse(
            success=True,
            message=f"Event type not handled: {x_github_event}",
            action=None,
        )


@router.get(
    "/github/status",
    summary="GitHub Webhook Status",
    description="Check if GitHub App webhook handler is configured.",
)
async def github_webhook_status() -> dict[str, Any]:
    """Check GitHub App configuration status."""
    config = GitHubAppConfig(enabled=True)

    return {
        "configured": config.is_configured(),
        "app_id_set": bool(config.get_app_id()),
        "private_key_set": bool(config.get_private_key()),
        "webhook_secret_set": bool(config.get_webhook_secret()),
        "review_enabled": config.review_enabled,
        "review_model": config.review_model,
        "max_diff_size": config.max_diff_size,
    }
