"""Stripe Integration - Async billing API for Stripe.

This module provides async methods for Stripe payment integration,
including customer management, subscriptions, and usage-based billing.

Environment Variables:
    STRIPE_SECRET_KEY: Stripe secret API key
    STRIPE_WEBHOOK_SECRET: Webhook signing secret

Example:
    billing = StripeBilling(secret_key="sk_test_xxx")

    # Create customer
    customer_id = await billing.create_customer(
        user_id="user-123",
        email="user@example.com",
    )

    # Create subscription
    sub_id = await billing.create_subscription(
        customer_id=customer_id,
        price_id="price_xxx",
    )
"""

from __future__ import annotations

import hashlib
import os
from datetime import datetime
from typing import TYPE_CHECKING, Any

from .models import Customer, StripeUsageRecord, Subscription

if TYPE_CHECKING:
    pass


class StripeError(Exception):
    """Stripe API error."""

    def __init__(self, message: str, code: str | None = None):
        super().__init__(message)
        self.code = code


class StripeBilling:
    """
    Async Stripe billing integration.

    Features:
    - Customer management with user_id metadata
    - Subscription lifecycle management
    - Usage-based billing (metered)
    - Checkout session creation
    - Idempotent operations

    Attributes:
        _secret_key: Stripe secret API key
        _webhook_secret: Webhook signing secret
    """

    def __init__(
        self,
        secret_key: str | None = None,
        webhook_secret: str | None = None,
    ):
        """Initialize Stripe billing.

        Args:
            secret_key: Stripe secret key (or STRIPE_SECRET_KEY env)
            webhook_secret: Webhook secret (or STRIPE_WEBHOOK_SECRET env)
        """
        self._secret_key = secret_key or os.environ.get("STRIPE_SECRET_KEY", "")
        self._webhook_secret = webhook_secret or os.environ.get("STRIPE_WEBHOOK_SECRET", "")
        self._stripe: Any = None

    def _get_stripe(self) -> Any:
        """Get Stripe module (lazy import)."""
        if self._stripe is None:
            try:
                import stripe

                stripe.api_key = self._secret_key
                self._stripe = stripe
            except ImportError:
                raise ImportError("stripe package not installed. Run: uv add stripe")
        return self._stripe

    def is_configured(self) -> bool:
        """Check if Stripe is configured with an API key."""
        return bool(self._secret_key)

    # =========================================================================
    # Customer Management
    # =========================================================================

    async def create_customer(
        self,
        user_id: str,
        email: str,
        name: str | None = None,
    ) -> str:
        """Create a Stripe customer.

        Args:
            user_id: Internal user ID (stored in metadata)
            email: Customer email
            name: Customer name (optional)

        Returns:
            Stripe customer ID (e.g., 'cus_xxx')

        Raises:
            StripeError: If customer creation fails
        """
        stripe = self._get_stripe()

        # Create idempotency key from user_id + email
        idempotency_key = hashlib.sha256(f"{user_id}:{email}".encode()).hexdigest()[:32]

        try:
            customer = stripe.Customer.create(
                email=email,
                name=name,
                metadata={"user_id": user_id},
                idempotency_key=idempotency_key,
            )
            return customer.id
        except Exception as e:
            raise StripeError(str(e))

    async def get_customer(self, customer_id: str) -> Customer | None:
        """Get customer by Stripe customer ID.

        Args:
            customer_id: Stripe customer ID

        Returns:
            Customer if found, None otherwise
        """
        stripe = self._get_stripe()

        try:
            customer = stripe.Customer.retrieve(customer_id)
            return Customer(
                id=customer.id,
                user_id=customer.metadata.get("user_id", ""),
                email=customer.email,
                name=customer.name,
                created_at=datetime.fromtimestamp(customer.created),
            )
        except Exception:
            return None

    async def get_customer_by_user_id(self, user_id: str) -> Customer | None:
        """Get customer by internal user ID.

        Args:
            user_id: Internal user ID

        Returns:
            Customer if found, None otherwise
        """
        stripe = self._get_stripe()

        try:
            # Search customers by metadata
            customers = stripe.Customer.search(
                query=f"metadata['user_id']:'{user_id}'",
                limit=1,
            )
            if customers.data:
                c = customers.data[0]
                return Customer(
                    id=c.id,
                    user_id=c.metadata.get("user_id", ""),
                    email=c.email,
                    name=c.name,
                    created_at=datetime.fromtimestamp(c.created),
                )
            return None
        except Exception:
            return None

    # =========================================================================
    # Subscription Management
    # =========================================================================

    async def create_subscription(
        self,
        customer_id: str,
        price_id: str,
    ) -> str:
        """Create a subscription for a customer.

        Args:
            customer_id: Stripe customer ID
            price_id: Stripe price ID

        Returns:
            Subscription ID (e.g., 'sub_xxx')

        Raises:
            StripeError: If subscription creation fails
        """
        stripe = self._get_stripe()

        try:
            subscription = stripe.Subscription.create(
                customer=customer_id,
                items=[{"price": price_id}],
            )
            return subscription.id
        except Exception as e:
            raise StripeError(str(e))

    async def get_subscription(self, subscription_id: str) -> Subscription | None:
        """Get subscription by ID.

        Args:
            subscription_id: Stripe subscription ID

        Returns:
            Subscription if found, None otherwise
        """
        stripe = self._get_stripe()

        try:
            sub = stripe.Subscription.retrieve(subscription_id)
            return Subscription(
                id=sub.id,
                customer_id=sub.customer,
                status=sub.status,
                current_period_end=datetime.fromtimestamp(sub.current_period_end),
                items=[item.id for item in sub["items"]["data"]],
            )
        except Exception:
            return None

    async def cancel_subscription(self, subscription_id: str) -> bool:
        """Cancel a subscription.

        Args:
            subscription_id: Stripe subscription ID

        Returns:
            True if canceled successfully, False otherwise
        """
        stripe = self._get_stripe()

        try:
            stripe.Subscription.delete(subscription_id)
            return True
        except Exception:
            return False

    async def get_customer_subscription(self, customer_id: str) -> Subscription | None:
        """Get active subscription for a customer.

        Args:
            customer_id: Stripe customer ID

        Returns:
            Active subscription if found, None otherwise
        """
        stripe = self._get_stripe()

        try:
            subscriptions = stripe.Subscription.list(
                customer=customer_id,
                status="active",
                limit=1,
            )
            if subscriptions.data:
                sub = subscriptions.data[0]
                return Subscription(
                    id=sub.id,
                    customer_id=sub.customer,
                    status=sub.status,
                    current_period_end=datetime.fromtimestamp(sub.current_period_end),
                    items=[item.id for item in sub["items"]["data"]],
                )
            return None
        except Exception:
            return None

    # =========================================================================
    # Usage-Based Billing
    # =========================================================================

    async def record_usage(
        self,
        subscription_item_id: str,
        quantity: int,
        timestamp: int | None = None,
    ) -> StripeUsageRecord:
        """Record metered usage.

        Args:
            subscription_item_id: Subscription item ID
            quantity: Usage amount
            timestamp: Optional timestamp (defaults to now)

        Returns:
            Created usage record

        Raises:
            StripeError: If recording fails
        """
        stripe = self._get_stripe()

        params: dict[str, Any] = {
            "quantity": quantity,
            "action": "increment",
        }

        if timestamp:
            params["timestamp"] = timestamp

        try:
            record = stripe.SubscriptionItem.create_usage_record(
                subscription_item_id,
                **params,
            )
            return StripeUsageRecord(
                id=record.id,
                subscription_item_id=subscription_item_id,
                quantity=record.quantity,
                timestamp=datetime.fromtimestamp(record.timestamp),
            )
        except Exception as e:
            raise StripeError(str(e))

    async def get_usage(
        self,
        subscription_item_id: str,
        start_time: int,
        end_time: int,
    ) -> list[StripeUsageRecord]:
        """Get usage records for a subscription item.

        Args:
            subscription_item_id: Subscription item ID
            start_time: Start timestamp
            end_time: End timestamp

        Returns:
            List of usage records
        """
        stripe = self._get_stripe()

        try:
            summaries = stripe.SubscriptionItem.list_usage_record_summaries(
                subscription_item_id,
            )
            records = []
            for summary in summaries.data:
                records.append(
                    StripeUsageRecord(
                        id=summary.id,
                        subscription_item_id=subscription_item_id,
                        quantity=summary.total_usage,
                        timestamp=datetime.fromtimestamp(summary.period.start),
                    )
                )
            return records
        except Exception:
            return []

    # =========================================================================
    # Checkout
    # =========================================================================

    async def create_checkout_session(
        self,
        customer_id: str,
        price_id: str,
        success_url: str,
        cancel_url: str,
    ) -> str:
        """Create a checkout session.

        Args:
            customer_id: Stripe customer ID
            price_id: Price ID for subscription
            success_url: URL after successful payment
            cancel_url: URL if payment cancelled

        Returns:
            Checkout session URL

        Raises:
            StripeError: If session creation fails
        """
        stripe = self._get_stripe()

        try:
            session = stripe.checkout.Session.create(
                customer=customer_id,
                mode="subscription",
                line_items=[{"price": price_id, "quantity": 1}],
                success_url=success_url,
                cancel_url=cancel_url,
            )
            return session.url
        except Exception as e:
            raise StripeError(str(e))

    async def create_portal_session(
        self,
        customer_id: str,
        return_url: str,
    ) -> str:
        """Create a billing portal session.

        Args:
            customer_id: Stripe customer ID
            return_url: URL to return to after portal

        Returns:
            Portal session URL

        Raises:
            StripeError: If session creation fails
        """
        stripe = self._get_stripe()

        try:
            session = stripe.billing_portal.Session.create(
                customer=customer_id,
                return_url=return_url,
            )
            return session.url
        except Exception as e:
            raise StripeError(str(e))
