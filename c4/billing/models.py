"""Billing Models - Data classes for Stripe integration.

This module contains the data models used for Stripe billing integration.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone


@dataclass
class Customer:
    """Stripe customer information.

    Attributes:
        id: Stripe customer ID (e.g., 'cus_xxx')
        user_id: Internal user ID
        email: Customer email
        name: Customer name (optional)
        created_at: Customer creation timestamp
    """

    id: str
    user_id: str
    email: str
    name: str | None = None
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


@dataclass
class Subscription:
    """Stripe subscription information.

    Attributes:
        id: Subscription ID (e.g., 'sub_xxx')
        customer_id: Stripe customer ID
        status: Subscription status (active, canceled, past_due, etc.)
        current_period_end: End of current billing period
        items: List of subscription item IDs
    """

    id: str
    customer_id: str
    status: str  # active, canceled, past_due, trialing, unpaid
    current_period_end: datetime
    items: list[str] = field(default_factory=list)

    @property
    def is_active(self) -> bool:
        """Check if subscription is active."""
        return self.status in ("active", "trialing")

    @property
    def is_canceled(self) -> bool:
        """Check if subscription is canceled."""
        return self.status == "canceled"


@dataclass
class StripeUsageRecord:
    """Usage record from Stripe metered billing.

    Attributes:
        id: Usage record ID (e.g., 'mbur_xxx')
        subscription_item_id: Subscription item ID
        quantity: Usage quantity
        timestamp: When usage was recorded
    """

    id: str
    subscription_item_id: str
    quantity: int
    timestamp: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


@dataclass
class CheckoutSession:
    """Stripe checkout session.

    Attributes:
        id: Session ID
        url: Checkout URL
        customer_id: Customer ID (if existing customer)
        price_id: Price ID being purchased
    """

    id: str
    url: str
    customer_id: str | None
    price_id: str


@dataclass
class Invoice:
    """Stripe invoice information.

    Attributes:
        id: Invoice ID
        customer_id: Customer ID
        subscription_id: Subscription ID
        amount_due: Amount due in cents
        amount_paid: Amount paid in cents
        status: Invoice status
        paid: Whether invoice is paid
    """

    id: str
    customer_id: str
    subscription_id: str | None
    amount_due: int
    amount_paid: int
    status: str  # draft, open, paid, uncollectible, void
    paid: bool = False
