"""C4 Billing - Usage tracking, subscription management, and Stripe integration.

This package provides:
- StripeBilling: Async Stripe API integration
- AsyncUsageTracker: Usage tracking with Stripe sync
- WebhookHandler: Stripe webhook event handling
- Plans and limits management

Environment Variables:
    STRIPE_SECRET_KEY: Stripe secret API key
    STRIPE_WEBHOOK_SECRET: Webhook signing secret

Example:
    from c4.billing import StripeBilling, AsyncUsageTracker

    # Create Stripe customer
    billing = StripeBilling()
    customer_id = await billing.create_customer(
        user_id="user-123",
        email="user@example.com",
    )

    # Track usage
    tracker = AsyncUsageTracker(
        user_id="user-123",
        subscription_item_id="si_xxx",
    )
    await tracker.track_llm_usage("user-123", input_tokens=1000, output_tokens=500)
    await tracker.sync_to_stripe("user-123")
"""

# Models
# Async usage tracking
from .async_usage import (
    AsyncUsageTracker,
    Usage,
)
from .models import (
    CheckoutSession,
    Customer,
    Invoice,
    StripeUsageRecord,
    Subscription,
)

# Plans
from .plans import (
    PLANS,
    Plan,
    PlanLimits,
    PlanType,
    get_plan,
)

# Legacy compatibility (existing sync API)
from .stripe_client import (
    StripeClient,
    SubscriptionStatus,
)

# Stripe integration (new async API)
from .stripe_integration import (
    StripeBilling,
    StripeError,
)
from .usage import (
    UsageMetric,
    UsageRecord,
    UsageSummary,
    UsageTracker,
)

# Webhook handling
from .webhook import (
    WebhookEvent,
    WebhookHandler,
    handle_webhook,
    verify_signature,
)

__all__ = [
    # Models
    "CheckoutSession",
    "Customer",
    "Invoice",
    "StripeUsageRecord",
    "Subscription",
    # Plans
    "PLANS",
    "Plan",
    "PlanLimits",
    "PlanType",
    "get_plan",
    # Stripe integration (async)
    "StripeBilling",
    "StripeError",
    # Async usage tracking
    "AsyncUsageTracker",
    "Usage",
    # Webhook handling
    "WebhookEvent",
    "WebhookHandler",
    "handle_webhook",
    "verify_signature",
    # Legacy (sync)
    "StripeClient",
    "SubscriptionStatus",
    "UsageMetric",
    "UsageRecord",
    "UsageSummary",
    "UsageTracker",
]
