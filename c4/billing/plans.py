"""Billing Plans - Subscription tiers and limits."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum


class PlanType(str, Enum):
    """Available subscription plans."""

    FREE = "free"
    PRO = "pro"
    TEAM = "team"
    ENTERPRISE = "enterprise"


@dataclass(frozen=True)
class PlanLimits:
    """Resource limits for a plan.

    Attributes:
        max_projects: Maximum concurrent projects
        max_workers: Maximum cloud workers
        max_tasks_per_day: Maximum tasks per day
        max_tokens_per_month: Maximum LLM tokens per month
        max_team_members: Maximum team members (0 = unlimited)
        cloud_execution: Allow cloud execution
        priority_support: Has priority support
    """

    max_projects: int
    max_workers: int
    max_tasks_per_day: int
    max_tokens_per_month: int
    max_team_members: int
    cloud_execution: bool
    priority_support: bool


@dataclass
class Plan:
    """Subscription plan definition.

    Attributes:
        type: Plan type
        name: Display name
        price_monthly: Monthly price in cents (USD)
        limits: Resource limits
        stripe_price_id: Stripe price ID for checkout
    """

    type: PlanType
    name: str
    price_monthly: int
    limits: PlanLimits
    stripe_price_id: str | None = None

    @classmethod
    def free(cls) -> "Plan":
        """Get the free plan."""
        return cls(
            type=PlanType.FREE,
            name="Free",
            price_monthly=0,
            limits=PlanLimits(
                max_projects=1,
                max_workers=0,
                max_tasks_per_day=10,
                max_tokens_per_month=100_000,
                max_team_members=1,
                cloud_execution=False,
                priority_support=False,
            ),
        )

    @classmethod
    def pro(cls, stripe_price_id: str | None = None) -> "Plan":
        """Get the pro plan."""
        return cls(
            type=PlanType.PRO,
            name="Pro",
            price_monthly=2900,  # $29/month
            limits=PlanLimits(
                max_projects=10,
                max_workers=5,
                max_tasks_per_day=100,
                max_tokens_per_month=1_000_000,
                max_team_members=1,
                cloud_execution=True,
                priority_support=False,
            ),
            stripe_price_id=stripe_price_id,
        )

    @classmethod
    def team(cls, stripe_price_id: str | None = None) -> "Plan":
        """Get the team plan."""
        return cls(
            type=PlanType.TEAM,
            name="Team",
            price_monthly=9900,  # $99/month
            limits=PlanLimits(
                max_projects=50,
                max_workers=20,
                max_tasks_per_day=500,
                max_tokens_per_month=10_000_000,
                max_team_members=10,
                cloud_execution=True,
                priority_support=True,
            ),
            stripe_price_id=stripe_price_id,
        )

    @classmethod
    def enterprise(cls) -> "Plan":
        """Get the enterprise plan (custom pricing)."""
        return cls(
            type=PlanType.ENTERPRISE,
            name="Enterprise",
            price_monthly=0,  # Custom pricing
            limits=PlanLimits(
                max_projects=0,  # Unlimited
                max_workers=0,  # Unlimited
                max_tasks_per_day=0,  # Unlimited
                max_tokens_per_month=0,  # Unlimited
                max_team_members=0,  # Unlimited
                cloud_execution=True,
                priority_support=True,
            ),
        )

    @classmethod
    def from_type(cls, plan_type: PlanType) -> "Plan":
        """Get plan by type."""
        plans = {
            PlanType.FREE: cls.free,
            PlanType.PRO: cls.pro,
            PlanType.TEAM: cls.team,
            PlanType.ENTERPRISE: cls.enterprise,
        }
        return plans[plan_type]()

    def check_limit(self, metric: str, current: int) -> tuple[bool, str | None]:
        """Check if a metric is within limits.

        Args:
            metric: Name of the limit to check
            current: Current usage value

        Returns:
            (is_allowed, error_message)
        """
        limit_map = {
            "projects": self.limits.max_projects,
            "workers": self.limits.max_workers,
            "tasks_per_day": self.limits.max_tasks_per_day,
            "tokens_per_month": self.limits.max_tokens_per_month,
            "team_members": self.limits.max_team_members,
        }

        if metric not in limit_map:
            return True, None

        limit = limit_map[metric]

        # 0 means unlimited
        if limit == 0:
            return True, None

        if current >= limit:
            return False, f"Limit reached: {metric} ({current}/{limit})"

        return True, None

    def is_feature_enabled(self, feature: str) -> bool:
        """Check if a feature is enabled for this plan."""
        feature_map = {
            "cloud_execution": self.limits.cloud_execution,
            "priority_support": self.limits.priority_support,
        }
        return feature_map.get(feature, False)


# Plan registry for easy lookup
PLANS: dict[PlanType, Plan] = {
    PlanType.FREE: Plan.free(),
    PlanType.PRO: Plan.pro(),
    PlanType.TEAM: Plan.team(),
    PlanType.ENTERPRISE: Plan.enterprise(),
}


def get_plan(plan_type: PlanType | str) -> Plan:
    """Get a plan by type.

    Args:
        plan_type: Plan type or string name

    Returns:
        Plan instance
    """
    if isinstance(plan_type, str):
        plan_type = PlanType(plan_type)
    return PLANS.get(plan_type, Plan.free())
