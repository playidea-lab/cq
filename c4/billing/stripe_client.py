"""Stripe Client - Payment and subscription management."""

from __future__ import annotations

import os
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import Any

from .plans import Plan, PlanType


class SubscriptionStatus(str, Enum):
    """Stripe subscription status."""

    ACTIVE = "active"
    PAST_DUE = "past_due"
    CANCELED = "canceled"
    UNPAID = "unpaid"
    TRIALING = "trialing"
    INCOMPLETE = "incomplete"


@dataclass
class Customer:
    """Stripe customer information.

    Attributes:
        id: Stripe customer ID
        email: Customer email
        name: Customer name
        created_at: Creation timestamp
    """

    id: str
    email: str
    name: str | None
    created_at: datetime


@dataclass
class Subscription:
    """Stripe subscription information.

    Attributes:
        id: Subscription ID
        customer_id: Customer ID
        plan_type: C4 plan type
        status: Subscription status
        current_period_start: Billing period start
        current_period_end: Billing period end
        cancel_at_period_end: Will cancel at period end
    """

    id: str
    customer_id: str
    plan_type: PlanType
    status: SubscriptionStatus
    current_period_start: datetime
    current_period_end: datetime
    cancel_at_period_end: bool = False


@dataclass
class CheckoutSession:
    """Stripe checkout session.

    Attributes:
        id: Session ID
        url: Checkout URL
        customer_id: Customer ID
        plan_type: Plan being purchased
    """

    id: str
    url: str
    customer_id: str | None
    plan_type: PlanType


class StripeClient:
    """
    Stripe integration for billing and subscriptions.

    Features:
    - Customer management
    - Subscription lifecycle
    - Checkout sessions
    - Usage-based billing

    Environment Variables:
        STRIPE_API_KEY: Stripe secret key
        STRIPE_WEBHOOK_SECRET: Webhook signing secret

    Example:
        client = StripeClient()
        session = client.create_checkout_session(
            customer_email="user@example.com",
            plan=Plan.pro(),
            success_url="https://app.c4.dev/success",
            cancel_url="https://app.c4.dev/cancel",
        )
        print(f"Checkout: {session.url}")
    """

    def __init__(
        self,
        api_key: str | None = None,
        webhook_secret: str | None = None,
    ):
        """Initialize Stripe client.

        Args:
            api_key: Stripe API key (or STRIPE_API_KEY env)
            webhook_secret: Webhook secret (or STRIPE_WEBHOOK_SECRET env)
        """
        self._api_key = api_key or os.environ.get("STRIPE_API_KEY", "")
        self._webhook_secret = webhook_secret or os.environ.get(
            "STRIPE_WEBHOOK_SECRET", ""
        )
        self._stripe: Any = None

    @property
    def stripe(self) -> Any:
        """Get Stripe module (lazy import)."""
        if self._stripe is None:
            try:
                import stripe

                stripe.api_key = self._api_key
                self._stripe = stripe
            except ImportError:
                raise ImportError("stripe package not installed")
        return self._stripe

    def is_configured(self) -> bool:
        """Check if Stripe is configured."""
        return bool(self._api_key)

    # =========================================================================
    # Customers
    # =========================================================================

    def create_customer(
        self,
        email: str,
        name: str | None = None,
        metadata: dict[str, str] | None = None,
    ) -> Customer:
        """Create a Stripe customer.

        Args:
            email: Customer email
            name: Customer name
            metadata: Additional metadata

        Returns:
            Created customer
        """
        customer = self.stripe.Customer.create(
            email=email,
            name=name,
            metadata=metadata or {},
        )

        return Customer(
            id=customer.id,
            email=customer.email,
            name=customer.name,
            created_at=datetime.fromtimestamp(customer.created),
        )

    def get_customer(self, customer_id: str) -> Customer | None:
        """Get a customer by ID.

        Args:
            customer_id: Stripe customer ID

        Returns:
            Customer if found
        """
        try:
            customer = self.stripe.Customer.retrieve(customer_id)
            return Customer(
                id=customer.id,
                email=customer.email,
                name=customer.name,
                created_at=datetime.fromtimestamp(customer.created),
            )
        except self.stripe.error.InvalidRequestError:
            return None

    def get_customer_by_email(self, email: str) -> Customer | None:
        """Get a customer by email.

        Args:
            email: Customer email

        Returns:
            Customer if found
        """
        customers = self.stripe.Customer.list(email=email, limit=1)
        if customers.data:
            c = customers.data[0]
            return Customer(
                id=c.id,
                email=c.email,
                name=c.name,
                created_at=datetime.fromtimestamp(c.created),
            )
        return None

    # =========================================================================
    # Subscriptions
    # =========================================================================

    def create_subscription(
        self,
        customer_id: str,
        plan: Plan,
    ) -> Subscription:
        """Create a subscription.

        Args:
            customer_id: Customer ID
            plan: Plan to subscribe to

        Returns:
            Created subscription
        """
        if not plan.stripe_price_id:
            raise ValueError(f"Plan {plan.type} has no Stripe price ID")

        sub = self.stripe.Subscription.create(
            customer=customer_id,
            items=[{"price": plan.stripe_price_id}],
            metadata={"plan_type": plan.type.value},
        )

        return self._parse_subscription(sub)

    def get_subscription(self, subscription_id: str) -> Subscription | None:
        """Get a subscription by ID.

        Args:
            subscription_id: Subscription ID

        Returns:
            Subscription if found
        """
        try:
            sub = self.stripe.Subscription.retrieve(subscription_id)
            return self._parse_subscription(sub)
        except self.stripe.error.InvalidRequestError:
            return None

    def get_customer_subscription(self, customer_id: str) -> Subscription | None:
        """Get active subscription for a customer.

        Args:
            customer_id: Customer ID

        Returns:
            Active subscription if exists
        """
        subs = self.stripe.Subscription.list(
            customer=customer_id,
            status="active",
            limit=1,
        )
        if subs.data:
            return self._parse_subscription(subs.data[0])
        return None

    def cancel_subscription(
        self,
        subscription_id: str,
        at_period_end: bool = True,
    ) -> Subscription:
        """Cancel a subscription.

        Args:
            subscription_id: Subscription ID
            at_period_end: Cancel at end of period (vs immediately)

        Returns:
            Updated subscription
        """
        if at_period_end:
            sub = self.stripe.Subscription.modify(
                subscription_id,
                cancel_at_period_end=True,
            )
        else:
            sub = self.stripe.Subscription.delete(subscription_id)

        return self._parse_subscription(sub)

    def update_subscription(
        self,
        subscription_id: str,
        new_plan: Plan,
    ) -> Subscription:
        """Update subscription to a new plan.

        Args:
            subscription_id: Subscription ID
            new_plan: New plan to switch to

        Returns:
            Updated subscription
        """
        if not new_plan.stripe_price_id:
            raise ValueError(f"Plan {new_plan.type} has no Stripe price ID")

        sub = self.stripe.Subscription.retrieve(subscription_id)

        self.stripe.Subscription.modify(
            subscription_id,
            items=[
                {
                    "id": sub["items"]["data"][0].id,
                    "price": new_plan.stripe_price_id,
                }
            ],
            metadata={"plan_type": new_plan.type.value},
        )

        return self.get_subscription(subscription_id)  # type: ignore

    def _parse_subscription(self, sub: Any) -> Subscription:
        """Parse Stripe subscription object."""
        plan_type_str = sub.metadata.get("plan_type", "free")
        try:
            plan_type = PlanType(plan_type_str)
        except ValueError:
            plan_type = PlanType.FREE

        return Subscription(
            id=sub.id,
            customer_id=sub.customer,
            plan_type=plan_type,
            status=SubscriptionStatus(sub.status),
            current_period_start=datetime.fromtimestamp(sub.current_period_start),
            current_period_end=datetime.fromtimestamp(sub.current_period_end),
            cancel_at_period_end=sub.cancel_at_period_end,
        )

    # =========================================================================
    # Checkout
    # =========================================================================

    def create_checkout_session(
        self,
        plan: Plan,
        success_url: str,
        cancel_url: str,
        customer_email: str | None = None,
        customer_id: str | None = None,
    ) -> CheckoutSession:
        """Create a checkout session for subscription.

        Args:
            plan: Plan to purchase
            success_url: URL after successful payment
            cancel_url: URL if payment cancelled
            customer_email: Email for new customer
            customer_id: Existing customer ID

        Returns:
            Checkout session with URL
        """
        if not plan.stripe_price_id:
            raise ValueError(f"Plan {plan.type} has no Stripe price ID")

        params: dict[str, Any] = {
            "mode": "subscription",
            "line_items": [{"price": plan.stripe_price_id, "quantity": 1}],
            "success_url": success_url,
            "cancel_url": cancel_url,
            "metadata": {"plan_type": plan.type.value},
        }

        if customer_id:
            params["customer"] = customer_id
        elif customer_email:
            params["customer_email"] = customer_email

        session = self.stripe.checkout.Session.create(**params)

        return CheckoutSession(
            id=session.id,
            url=session.url,
            customer_id=session.customer,
            plan_type=plan.type,
        )

    def create_portal_session(
        self,
        customer_id: str,
        return_url: str,
    ) -> str:
        """Create a billing portal session.

        Args:
            customer_id: Customer ID
            return_url: URL to return to

        Returns:
            Portal URL
        """
        session = self.stripe.billing_portal.Session.create(
            customer=customer_id,
            return_url=return_url,
        )
        return session.url

    # =========================================================================
    # Usage-Based Billing
    # =========================================================================

    def report_usage(
        self,
        subscription_item_id: str,
        quantity: int,
        timestamp: datetime | None = None,
    ) -> dict[str, Any]:
        """Report usage for metered billing.

        Args:
            subscription_item_id: Subscription item ID
            quantity: Usage amount
            timestamp: When usage occurred

        Returns:
            Usage record
        """
        params: dict[str, Any] = {
            "quantity": quantity,
            "action": "increment",
        }

        if timestamp:
            params["timestamp"] = int(timestamp.timestamp())

        record = self.stripe.SubscriptionItem.create_usage_record(
            subscription_item_id,
            **params,
        )

        return {
            "id": record.id,
            "quantity": record.quantity,
            "timestamp": record.timestamp,
        }

    # =========================================================================
    # Webhooks
    # =========================================================================

    def verify_webhook(
        self,
        payload: bytes,
        signature: str,
    ) -> dict[str, Any]:
        """Verify and parse webhook payload.

        Args:
            payload: Raw request body
            signature: Stripe-Signature header

        Returns:
            Parsed event data

        Raises:
            ValueError: If signature is invalid
        """
        try:
            event = self.stripe.Webhook.construct_event(
                payload,
                signature,
                self._webhook_secret,
            )
            return {
                "id": event.id,
                "type": event.type,
                "data": event.data.object,
            }
        except self.stripe.error.SignatureVerificationError as e:
            raise ValueError(f"Invalid webhook signature: {e}")
