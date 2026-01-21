"""Tests for webhook handler.

TDD RED Phase: Define test scenarios for Stripe webhook handling.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import time
from typing import Any
from unittest.mock import MagicMock, patch

import pytest

from c4.billing.webhook import (
    WebhookEvent,
    WebhookHandler,
    handle_webhook,
)


def create_webhook_signature(payload: bytes, secret: str, timestamp: int | None = None) -> str:
    """Create a valid Stripe webhook signature for testing."""
    timestamp = timestamp or int(time.time())
    signed_payload = f"{timestamp}.{payload.decode('utf-8')}"
    signature = hmac.new(
        secret.encode("utf-8"),
        signed_payload.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()
    return f"t={timestamp},v1={signature}"


class TestWebhookEvent:
    """Test WebhookEvent dataclass."""

    def test_event_creation(self) -> None:
        """Test creating webhook event."""
        event = WebhookEvent(
            id="evt_test123",
            type="checkout.session.completed",
            data={"customer": "cus_123"},
        )

        assert event.id == "evt_test123"
        assert event.type == "checkout.session.completed"
        assert event.data["customer"] == "cus_123"


class TestWebhookHandler:
    """Test WebhookHandler class."""

    @pytest.fixture
    def handler(self) -> WebhookHandler:
        """Create handler instance."""
        return WebhookHandler(webhook_secret="whsec_test123")

    def test_init(self, handler: WebhookHandler) -> None:
        """Test handler initialization."""
        assert handler._webhook_secret == "whsec_test123"

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("STRIPE_WEBHOOK_SECRET", "whsec_env")
        handler = WebhookHandler()
        assert handler._webhook_secret == "whsec_env"


class TestHandleWebhookFunction:
    """Test handle_webhook function."""

    @pytest.fixture
    def webhook_secret(self) -> str:
        """Webhook secret for tests."""
        return "whsec_test123"

    def make_event_payload(self, event_type: str, data: dict[str, Any]) -> bytes:
        """Create event payload."""
        return json.dumps({
            "id": f"evt_{event_type.replace('.', '_')}",
            "type": event_type,
            "data": {"object": data},
        }).encode("utf-8")

    @pytest.mark.asyncio
    async def test_checkout_session_completed(self, webhook_secret: str) -> None:
        """Test handling checkout.session.completed event."""
        payload = self.make_event_payload(
            "checkout.session.completed",
            {
                "id": "cs_test123",
                "customer": "cus_test123",
                "subscription": "sub_test123",
                "metadata": {"user_id": "user-123"},
            },
        )
        signature = create_webhook_signature(payload, webhook_secret)

        with patch("c4.billing.webhook.verify_signature", return_value=True):
            result = await handle_webhook(payload, signature, webhook_secret)

        assert result["status"] == "success"
        assert result["event_type"] == "checkout.session.completed"

    @pytest.mark.asyncio
    async def test_invoice_paid(self, webhook_secret: str) -> None:
        """Test handling invoice.paid event."""
        payload = self.make_event_payload(
            "invoice.paid",
            {
                "id": "in_test123",
                "customer": "cus_test123",
                "subscription": "sub_test123",
                "amount_paid": 2900,
            },
        )
        signature = create_webhook_signature(payload, webhook_secret)

        with patch("c4.billing.webhook.verify_signature", return_value=True):
            result = await handle_webhook(payload, signature, webhook_secret)

        assert result["status"] == "success"
        assert result["event_type"] == "invoice.paid"

    @pytest.mark.asyncio
    async def test_invoice_payment_failed(self, webhook_secret: str) -> None:
        """Test handling invoice.payment_failed event."""
        payload = self.make_event_payload(
            "invoice.payment_failed",
            {
                "id": "in_test123",
                "customer": "cus_test123",
                "subscription": "sub_test123",
                "attempt_count": 1,
            },
        )
        signature = create_webhook_signature(payload, webhook_secret)

        with patch("c4.billing.webhook.verify_signature", return_value=True):
            result = await handle_webhook(payload, signature, webhook_secret)

        assert result["status"] == "success"
        assert result["event_type"] == "invoice.payment_failed"

    @pytest.mark.asyncio
    async def test_subscription_deleted(self, webhook_secret: str) -> None:
        """Test handling customer.subscription.deleted event."""
        payload = self.make_event_payload(
            "customer.subscription.deleted",
            {
                "id": "sub_test123",
                "customer": "cus_test123",
                "status": "canceled",
            },
        )
        signature = create_webhook_signature(payload, webhook_secret)

        with patch("c4.billing.webhook.verify_signature", return_value=True):
            result = await handle_webhook(payload, signature, webhook_secret)

        assert result["status"] == "success"
        assert result["event_type"] == "customer.subscription.deleted"

    @pytest.mark.asyncio
    async def test_invalid_signature(self, webhook_secret: str) -> None:
        """Test handling invalid signature."""
        payload = self.make_event_payload("checkout.session.completed", {})
        invalid_signature = "t=123,v1=invalid"

        with patch("c4.billing.webhook.verify_signature", return_value=False):
            with pytest.raises(ValueError, match="Invalid webhook signature"):
                await handle_webhook(payload, invalid_signature, webhook_secret)

    @pytest.mark.asyncio
    async def test_unknown_event_type(self, webhook_secret: str) -> None:
        """Test handling unknown event type."""
        payload = self.make_event_payload(
            "unknown.event.type",
            {"id": "test123"},
        )
        signature = create_webhook_signature(payload, webhook_secret)

        with patch("c4.billing.webhook.verify_signature", return_value=True):
            result = await handle_webhook(payload, signature, webhook_secret)

        assert result["status"] == "ignored"
        assert result["event_type"] == "unknown.event.type"


class TestWebhookCallbacks:
    """Test webhook callback registration."""

    @pytest.fixture
    def handler(self) -> WebhookHandler:
        """Create handler instance."""
        return WebhookHandler(webhook_secret="whsec_test123")

    def test_register_callback(self, handler: WebhookHandler) -> None:
        """Test registering event callback."""
        callback = MagicMock()
        handler.on("checkout.session.completed", callback)

        assert "checkout.session.completed" in handler._callbacks

    @pytest.mark.asyncio
    async def test_callback_invoked(self, handler: WebhookHandler) -> None:
        """Test callback is invoked on event."""
        callback = MagicMock()
        handler.on("checkout.session.completed", callback)

        event = WebhookEvent(
            id="evt_test123",
            type="checkout.session.completed",
            data={"customer": "cus_123"},
        )

        await handler._dispatch_event(event)

        callback.assert_called_once_with(event)

    @pytest.mark.asyncio
    async def test_async_callback_invoked(self, handler: WebhookHandler) -> None:
        """Test async callback is invoked."""
        from unittest.mock import AsyncMock

        callback = AsyncMock()
        handler.on("checkout.session.completed", callback)

        event = WebhookEvent(
            id="evt_test123",
            type="checkout.session.completed",
            data={"customer": "cus_123"},
        )

        await handler._dispatch_event(event)

        callback.assert_awaited_once_with(event)
