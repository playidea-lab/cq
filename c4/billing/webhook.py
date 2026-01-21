"""Stripe Webhook Handler - Process Stripe webhook events.

This module handles Stripe webhook events for subscription lifecycle,
payment status, and other billing events.

Supported Events:
    - checkout.session.completed: Customer completed checkout
    - invoice.paid: Invoice was paid successfully
    - invoice.payment_failed: Payment failed
    - customer.subscription.deleted: Subscription was canceled

Example:
    # FastAPI endpoint
    @app.post("/webhook/stripe")
    async def stripe_webhook(request: Request):
        payload = await request.body()
        signature = request.headers.get("Stripe-Signature")
        result = await handle_webhook(payload, signature, webhook_secret)
        return result
"""

from __future__ import annotations

import asyncio
import hashlib
import hmac
import json
import os
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable

# Callback type: sync or async function that takes WebhookEvent
WebhookCallback = Callable[["WebhookEvent"], None | Awaitable[None]]


@dataclass
class WebhookEvent:
    """Parsed Stripe webhook event.

    Attributes:
        id: Event ID (e.g., 'evt_xxx')
        type: Event type (e.g., 'checkout.session.completed')
        data: Event data object
    """

    id: str
    type: str
    data: dict[str, Any] = field(default_factory=dict)


def verify_signature(
    payload: bytes,
    signature: str,
    secret: str,
    tolerance: int = 300,
) -> bool:
    """Verify Stripe webhook signature.

    Args:
        payload: Raw request body
        signature: Stripe-Signature header
        secret: Webhook signing secret
        tolerance: Maximum age in seconds (default 5 minutes)

    Returns:
        True if signature is valid
    """
    if not signature:
        return False

    try:
        # Parse signature header
        elements = dict(item.split("=", 1) for item in signature.split(","))
        timestamp = int(elements.get("t", "0"))
        expected_sig = elements.get("v1", "")

        # Check timestamp tolerance
        if abs(time.time() - timestamp) > tolerance:
            return False

        # Compute expected signature
        signed_payload = f"{timestamp}.{payload.decode('utf-8')}"
        computed_sig = hmac.new(
            secret.encode("utf-8"),
            signed_payload.encode("utf-8"),
            hashlib.sha256,
        ).hexdigest()

        return hmac.compare_digest(computed_sig, expected_sig)
    except (ValueError, KeyError):
        return False


async def handle_webhook(
    payload: bytes,
    signature: str,
    webhook_secret: str | None = None,
) -> dict[str, Any]:
    """Handle Stripe webhook event.

    Args:
        payload: Raw request body
        signature: Stripe-Signature header
        webhook_secret: Webhook signing secret (or from env)

    Returns:
        Result dictionary with status and event info

    Raises:
        ValueError: If signature is invalid
    """
    secret = webhook_secret or os.environ.get("STRIPE_WEBHOOK_SECRET", "")

    # Verify signature
    if not verify_signature(payload, signature, secret):
        raise ValueError("Invalid webhook signature")

    # Parse event
    try:
        event_data = json.loads(payload.decode("utf-8"))
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid JSON payload: {e}")

    event_type = event_data.get("type", "unknown")
    event_id = event_data.get("id", "")
    data = event_data.get("data", {}).get("object", {})

    # Handle known event types
    handlers = {
        "checkout.session.completed": _handle_checkout_completed,
        "invoice.paid": _handle_invoice_paid,
        "invoice.payment_failed": _handle_invoice_payment_failed,
        "customer.subscription.deleted": _handle_subscription_deleted,
    }

    handler = handlers.get(event_type)
    if handler:
        await handler(data)
        return {
            "status": "success",
            "event_type": event_type,
            "event_id": event_id,
        }
    else:
        # Unknown event type - ignore but acknowledge
        return {
            "status": "ignored",
            "event_type": event_type,
            "event_id": event_id,
        }


# =========================================================================
# Event Handlers
# =========================================================================


async def _handle_checkout_completed(data: dict[str, Any]) -> None:
    """Handle checkout.session.completed event.

    Called when a customer completes a checkout session.

    Args:
        data: Event data object
    """
    # Extract event data for processing
    _customer_id = data.get("customer")
    _subscription_id = data.get("subscription")
    _metadata = data.get("metadata", {})
    _user_id = _metadata.get("user_id")

    # TODO: Update user subscription status in database
    # This would typically:
    # 1. Find user by user_id or customer_id
    # 2. Update their subscription status to active
    # 3. Grant access to paid features
    pass


async def _handle_invoice_paid(data: dict[str, Any]) -> None:
    """Handle invoice.paid event.

    Called when an invoice is paid successfully.

    Args:
        data: Event data object
    """
    # Extract event data for processing
    _customer_id = data.get("customer")
    _subscription_id = data.get("subscription")
    _amount_paid = data.get("amount_paid", 0)

    # TODO: Record payment in database
    # This would typically:
    # 1. Record payment transaction
    # 2. Update billing history
    # 3. Send receipt email
    pass


async def _handle_invoice_payment_failed(data: dict[str, Any]) -> None:
    """Handle invoice.payment_failed event.

    Called when a payment attempt fails.

    Args:
        data: Event data object
    """
    # Extract event data for processing
    _customer_id = data.get("customer")
    _subscription_id = data.get("subscription")
    _attempt_count = data.get("attempt_count", 0)

    # TODO: Handle payment failure
    # This would typically:
    # 1. Notify user of failed payment
    # 2. Update subscription status if needed
    # 3. Schedule retry or grace period
    pass


async def _handle_subscription_deleted(data: dict[str, Any]) -> None:
    """Handle customer.subscription.deleted event.

    Called when a subscription is canceled.

    Args:
        data: Event data object
    """
    # Extract event data for processing
    _subscription_id = data.get("id")
    _customer_id = data.get("customer")
    _status = data.get("status")

    # TODO: Handle subscription cancellation
    # This would typically:
    # 1. Update user subscription status
    # 2. Revoke paid features
    # 3. Send cancellation confirmation
    pass


# =========================================================================
# Webhook Handler Class (for custom callbacks)
# =========================================================================


class WebhookHandler:
    """
    Webhook handler with custom callback support.

    Allows registering custom callbacks for specific event types.

    Example:
        handler = WebhookHandler(webhook_secret="whsec_xxx")

        @handler.on("checkout.session.completed")
        async def on_checkout(event: WebhookEvent):
            print(f"Checkout completed: {event.data}")

        await handler.process(payload, signature)
    """

    def __init__(self, webhook_secret: str | None = None):
        """Initialize webhook handler.

        Args:
            webhook_secret: Webhook signing secret (or from env)
        """
        self._webhook_secret = webhook_secret or os.environ.get("STRIPE_WEBHOOK_SECRET", "")
        self._callbacks: dict[str, list[WebhookCallback]] = {}

    def on(self, event_type: str, callback: WebhookCallback) -> None:
        """Register callback for event type.

        Args:
            event_type: Event type to handle
            callback: Callback function (sync or async)
        """
        if event_type not in self._callbacks:
            self._callbacks[event_type] = []
        self._callbacks[event_type].append(callback)

    async def process(self, payload: bytes, signature: str) -> dict[str, Any]:
        """Process webhook payload.

        Args:
            payload: Raw request body
            signature: Stripe-Signature header

        Returns:
            Result dictionary

        Raises:
            ValueError: If signature is invalid
        """
        if not verify_signature(payload, signature, self._webhook_secret):
            raise ValueError("Invalid webhook signature")

        event_data = json.loads(payload.decode("utf-8"))
        event = WebhookEvent(
            id=event_data.get("id", ""),
            type=event_data.get("type", ""),
            data=event_data.get("data", {}).get("object", {}),
        )

        await self._dispatch_event(event)

        return {
            "status": "success",
            "event_type": event.type,
            "event_id": event.id,
        }

    async def _dispatch_event(self, event: WebhookEvent) -> None:
        """Dispatch event to registered callbacks.

        Args:
            event: Webhook event
        """
        callbacks = self._callbacks.get(event.type, [])
        for callback in callbacks:
            result = callback(event)
            if asyncio.iscoroutine(result):
                await result
