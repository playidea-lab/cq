"""Tests for StripeBilling class - Stripe integration with async API.

TDD RED Phase: Define test scenarios for StripeBilling functionality.
"""

from __future__ import annotations

from datetime import datetime
from unittest.mock import MagicMock, patch

import pytest

from c4.billing.stripe_integration import StripeBilling, StripeError


class TestStripeBillingInit:
    """Test StripeBilling initialization."""

    def test_init_with_key(self) -> None:
        """Test initialization with explicit key."""
        billing = StripeBilling(secret_key="sk_test_123")
        assert billing._secret_key == "sk_test_123"

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("STRIPE_SECRET_KEY", "sk_test_env")
        billing = StripeBilling()
        assert billing._secret_key == "sk_test_env"

    def test_init_webhook_secret(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test webhook secret from environment."""
        monkeypatch.setenv("STRIPE_WEBHOOK_SECRET", "whsec_123")
        billing = StripeBilling(secret_key="sk_test")
        assert billing._webhook_secret == "whsec_123"


class TestStripeBillingCustomer:
    """Test customer management."""

    @pytest.fixture
    def billing(self) -> StripeBilling:
        """Create billing instance."""
        return StripeBilling(secret_key="sk_test_123")

    @pytest.fixture
    def mock_stripe(self) -> MagicMock:
        """Create mock Stripe module."""
        mock = MagicMock()
        return mock

    @pytest.mark.asyncio
    async def test_create_customer(self, billing: StripeBilling) -> None:
        """Test creating a Stripe customer."""
        mock_customer = MagicMock()
        mock_customer.id = "cus_test123"
        mock_customer.email = "user@example.com"
        mock_customer.name = "Test User"
        mock_customer.created = int(datetime.now().timestamp())

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.create = MagicMock(return_value=mock_customer)

            customer_id = await billing.create_customer(
                user_id="user-123",
                email="user@example.com",
                name="Test User",
            )

            assert customer_id == "cus_test123"
            mock_stripe.return_value.Customer.create.assert_called_once()

    @pytest.mark.asyncio
    async def test_create_customer_with_metadata(self, billing: StripeBilling) -> None:
        """Test customer creation stores user_id in metadata."""
        mock_customer = MagicMock()
        mock_customer.id = "cus_test456"
        mock_customer.email = "user2@example.com"
        mock_customer.name = None
        mock_customer.created = int(datetime.now().timestamp())

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.create = MagicMock(return_value=mock_customer)

            await billing.create_customer(user_id="user-456", email="user2@example.com")

            call_kwargs = mock_stripe.return_value.Customer.create.call_args[1]
            assert call_kwargs["metadata"]["user_id"] == "user-456"

    @pytest.mark.asyncio
    async def test_get_customer(self, billing: StripeBilling) -> None:
        """Test getting customer by ID."""
        mock_customer = MagicMock()
        mock_customer.id = "cus_test123"
        mock_customer.email = "user@example.com"
        mock_customer.name = "Test User"
        mock_customer.created = int(datetime.now().timestamp())
        mock_customer.metadata = {"user_id": "user-123"}

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.retrieve = MagicMock(return_value=mock_customer)

            customer = await billing.get_customer("cus_test123")

            assert customer is not None
            assert customer.id == "cus_test123"
            assert customer.email == "user@example.com"
            assert customer.user_id == "user-123"

    @pytest.mark.asyncio
    async def test_get_customer_not_found(self, billing: StripeBilling) -> None:
        """Test getting non-existent customer."""
        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.retrieve.side_effect = Exception("No such customer")

            customer = await billing.get_customer("cus_invalid")

            assert customer is None


class TestStripeBillingSubscription:
    """Test subscription management."""

    @pytest.fixture
    def billing(self) -> StripeBilling:
        """Create billing instance."""
        return StripeBilling(secret_key="sk_test_123")

    @pytest.mark.asyncio
    async def test_create_subscription(self, billing: StripeBilling) -> None:
        """Test creating a subscription."""
        mock_sub = MagicMock()
        mock_sub.id = "sub_test123"
        mock_sub.customer = "cus_test123"
        mock_sub.status = "active"
        mock_sub.current_period_end = int(datetime.now().timestamp()) + 86400 * 30
        mock_sub.items.data = [MagicMock(id="si_item123")]

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Subscription.create = MagicMock(return_value=mock_sub)

            subscription_id = await billing.create_subscription(
                customer_id="cus_test123",
                price_id="price_test123",
            )

            assert subscription_id == "sub_test123"

    @pytest.mark.asyncio
    async def test_cancel_subscription(self, billing: StripeBilling) -> None:
        """Test canceling a subscription."""
        mock_sub = MagicMock()
        mock_sub.id = "sub_test123"
        mock_sub.status = "canceled"

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Subscription.delete = MagicMock(return_value=mock_sub)

            result = await billing.cancel_subscription("sub_test123")

            assert result is True

    @pytest.mark.asyncio
    async def test_cancel_subscription_not_found(self, billing: StripeBilling) -> None:
        """Test canceling non-existent subscription."""
        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Subscription.delete.side_effect = Exception("No such subscription")

            result = await billing.cancel_subscription("sub_invalid")

            assert result is False


class TestStripeBillingUsage:
    """Test usage-based billing."""

    @pytest.fixture
    def billing(self) -> StripeBilling:
        """Create billing instance."""
        return StripeBilling(secret_key="sk_test_123")

    @pytest.mark.asyncio
    async def test_record_usage(self, billing: StripeBilling) -> None:
        """Test recording metered usage."""
        mock_record = MagicMock()
        mock_record.id = "mbur_test123"
        mock_record.subscription_item = "si_item123"
        mock_record.quantity = 100
        mock_record.timestamp = int(datetime.now().timestamp())

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.SubscriptionItem.create_usage_record = MagicMock(return_value=mock_record)

            record = await billing.record_usage(
                subscription_item_id="si_item123",
                quantity=100,
            )

            assert record.id == "mbur_test123"
            assert record.quantity == 100

    @pytest.mark.asyncio
    async def test_record_usage_with_timestamp(self, billing: StripeBilling) -> None:
        """Test recording usage with specific timestamp."""
        timestamp = int(datetime.now().timestamp())
        mock_record = MagicMock()
        mock_record.id = "mbur_test456"
        mock_record.subscription_item = "si_item123"
        mock_record.quantity = 50
        mock_record.timestamp = timestamp

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.SubscriptionItem.create_usage_record = MagicMock(return_value=mock_record)

            await billing.record_usage(
                subscription_item_id="si_item123",
                quantity=50,
                timestamp=timestamp,
            )

            call_kwargs = mock_stripe.return_value.SubscriptionItem.create_usage_record.call_args[1]
            assert call_kwargs.get("timestamp") == timestamp

    @pytest.mark.asyncio
    async def test_get_usage(self, billing: StripeBilling) -> None:
        """Test getting usage records."""
        # Mock period object
        mock_period1 = MagicMock()
        mock_period1.start = 1000

        mock_period2 = MagicMock()
        mock_period2.start = 2000

        mock_records = MagicMock()
        mock_records.data = [
            MagicMock(id="mbur_1", total_usage=100, period=mock_period1),
            MagicMock(id="mbur_2", total_usage=200, period=mock_period2),
        ]

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.SubscriptionItem.list_usage_record_summaries = MagicMock(return_value=mock_records)

            records = await billing.get_usage(
                subscription_item_id="si_item123",
                start_time=0,
                end_time=3000,
            )

            assert len(records) == 2
            assert records[0].quantity == 100
            assert records[1].quantity == 200


class TestStripeBillingCheckout:
    """Test checkout session creation."""

    @pytest.fixture
    def billing(self) -> StripeBilling:
        """Create billing instance."""
        return StripeBilling(secret_key="sk_test_123")

    @pytest.mark.asyncio
    async def test_create_checkout_session(self, billing: StripeBilling) -> None:
        """Test creating checkout session."""
        mock_session = MagicMock()
        mock_session.id = "cs_test123"
        mock_session.url = "https://checkout.stripe.com/c/pay/cs_test123"

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.checkout.Session.create = MagicMock(return_value=mock_session)

            url = await billing.create_checkout_session(
                customer_id="cus_test123",
                price_id="price_test123",
                success_url="https://example.com/success",
                cancel_url="https://example.com/cancel",
            )

            assert url == "https://checkout.stripe.com/c/pay/cs_test123"

    @pytest.mark.asyncio
    async def test_create_checkout_session_params(self, billing: StripeBilling) -> None:
        """Test checkout session has correct parameters."""
        mock_session = MagicMock()
        mock_session.id = "cs_test456"
        mock_session.url = "https://checkout.stripe.com/c/pay/cs_test456"

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.checkout.Session.create = MagicMock(return_value=mock_session)

            await billing.create_checkout_session(
                customer_id="cus_test123",
                price_id="price_test123",
                success_url="https://example.com/success",
                cancel_url="https://example.com/cancel",
            )

            call_kwargs = mock_stripe.return_value.checkout.Session.create.call_args[1]
            assert call_kwargs["customer"] == "cus_test123"
            assert call_kwargs["success_url"] == "https://example.com/success"
            assert call_kwargs["cancel_url"] == "https://example.com/cancel"
            assert call_kwargs["mode"] == "subscription"


class TestStripeBillingErrorHandling:
    """Test error handling."""

    @pytest.fixture
    def billing(self) -> StripeBilling:
        """Create billing instance."""
        return StripeBilling(secret_key="sk_test_123")

    @pytest.mark.asyncio
    async def test_stripe_error_handling(self, billing: StripeBilling) -> None:
        """Test handling of Stripe errors."""
        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.create.side_effect = Exception("Card was declined")

            with pytest.raises(StripeError) as exc_info:
                await billing.create_customer(user_id="user-123", email="test@example.com")

            assert "Card was declined" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_idempotency_key(self, billing: StripeBilling) -> None:
        """Test idempotency key is used for create operations."""
        mock_customer = MagicMock()
        mock_customer.id = "cus_test123"
        mock_customer.email = "user@example.com"
        mock_customer.name = None
        mock_customer.created = int(datetime.now().timestamp())

        with patch.object(billing, "_get_stripe") as mock_stripe:
            mock_stripe.return_value.Customer.create = MagicMock(return_value=mock_customer)

            await billing.create_customer(
                user_id="user-123",
                email="user@example.com",
            )

            call_kwargs = mock_stripe.return_value.Customer.create.call_args[1]
            # Idempotency key should be set based on user_id + email
            assert "idempotency_key" in call_kwargs
