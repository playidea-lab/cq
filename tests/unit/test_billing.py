"""Tests for Billing module."""

from datetime import datetime, timedelta

import pytest

from c4.billing.plans import (
    PLANS,
    Plan,
    PlanLimits,
    PlanType,
    get_plan,
)
from c4.billing.stripe_client import (
    CheckoutSession,
    Customer,
    StripeClient,
    Subscription,
    SubscriptionStatus,
)
from c4.billing.usage import (
    UsageMetric,
    UsageRecord,
    UsageSummary,
    UsageTracker,
)


class TestPlanType:
    """Test PlanType enum."""

    def test_values(self) -> None:
        """Test enum values."""
        assert PlanType.FREE.value == "free"
        assert PlanType.PRO.value == "pro"
        assert PlanType.TEAM.value == "team"
        assert PlanType.ENTERPRISE.value == "enterprise"


class TestPlanLimits:
    """Test PlanLimits dataclass."""

    def test_frozen(self) -> None:
        """Test limits are immutable."""
        limits = PlanLimits(
            max_projects=5,
            max_workers=2,
            max_tasks_per_day=50,
            max_tokens_per_month=500000,
            max_execution_minutes_per_month=1000,
            max_team_members=3,
            cloud_execution=True,
            priority_support=False,
        )

        with pytest.raises(Exception):  # FrozenInstanceError
            limits.max_projects = 10  # type: ignore


class TestPlan:
    """Test Plan class."""

    def test_free_plan(self) -> None:
        """Test free plan defaults."""
        plan = Plan.free()

        assert plan.type == PlanType.FREE
        assert plan.price_monthly == 0
        assert plan.limits.max_projects == 1
        assert plan.limits.cloud_execution is False

    def test_pro_plan(self) -> None:
        """Test pro plan."""
        plan = Plan.pro()

        assert plan.type == PlanType.PRO
        assert plan.price_monthly == 2900
        assert plan.limits.max_projects == 10
        assert plan.limits.cloud_execution is True

    def test_team_plan(self) -> None:
        """Test team plan."""
        plan = Plan.team()

        assert plan.type == PlanType.TEAM
        assert plan.price_monthly == 9900
        assert plan.limits.max_team_members == 10

    def test_enterprise_plan(self) -> None:
        """Test enterprise plan (unlimited)."""
        plan = Plan.enterprise()

        assert plan.type == PlanType.ENTERPRISE
        assert plan.limits.max_projects == 0  # Unlimited

    def test_from_type(self) -> None:
        """Test creating plan from type."""
        for plan_type in PlanType:
            plan = Plan.from_type(plan_type)
            assert plan.type == plan_type

    def test_check_limit_within(self) -> None:
        """Test limit check when within."""
        plan = Plan.free()
        allowed, error = plan.check_limit("projects", 0)

        assert allowed is True
        assert error is None

    def test_check_limit_exceeded(self) -> None:
        """Test limit check when exceeded."""
        plan = Plan.free()
        allowed, error = plan.check_limit("projects", 1)

        assert allowed is False
        assert "Limit reached" in error

    def test_check_limit_unlimited(self) -> None:
        """Test limit check for unlimited."""
        plan = Plan.enterprise()
        allowed, error = plan.check_limit("projects", 9999)

        assert allowed is True  # 0 = unlimited

    def test_is_feature_enabled(self) -> None:
        """Test feature check."""
        free = Plan.free()
        pro = Plan.pro()

        assert free.is_feature_enabled("cloud_execution") is False
        assert pro.is_feature_enabled("cloud_execution") is True


class TestGetPlan:
    """Test get_plan function."""

    def test_get_by_type(self) -> None:
        """Test getting plan by type."""
        plan = get_plan(PlanType.PRO)
        assert plan.type == PlanType.PRO

    def test_get_by_string(self) -> None:
        """Test getting plan by string."""
        plan = get_plan("team")
        assert plan.type == PlanType.TEAM

    def test_plans_registry(self) -> None:
        """Test plans registry."""
        assert len(PLANS) == 4
        assert PlanType.FREE in PLANS


class TestUsageMetric:
    """Test UsageMetric enum."""

    def test_values(self) -> None:
        """Test metric values."""
        assert UsageMetric.TASKS_COMPLETED.value == "tasks_completed"
        assert UsageMetric.TOKENS_USED.value == "tokens_used"


class TestUsageRecord:
    """Test UsageRecord dataclass."""

    def test_basic_record(self) -> None:
        """Test creating a record."""
        record = UsageRecord(
            metric=UsageMetric.TASKS_COMPLETED,
            value=1,
            timestamp=datetime.now(),
        )

        assert record.metric == UsageMetric.TASKS_COMPLETED
        assert record.value == 1
        assert record.project_id is None


class TestUsageSummary:
    """Test UsageSummary dataclass."""

    def test_defaults(self) -> None:
        """Test default values."""
        summary = UsageSummary(
            period_start=datetime.now(),
            period_end=datetime.now(),
        )

        assert summary.tasks_completed == 0
        assert summary.tokens_used == 0


class TestUsageTracker:
    """Test UsageTracker class."""

    @pytest.fixture
    def tracker(self) -> UsageTracker:
        """Create a tracker instance."""
        return UsageTracker(user_id="user-123")

    def test_init(self, tracker: UsageTracker) -> None:
        """Test initialization."""
        assert tracker.user_id == "user-123"

    def test_record(self, tracker: UsageTracker) -> None:
        """Test recording usage."""
        record = tracker.record(UsageMetric.TASKS_COMPLETED, 1)

        assert record.metric == UsageMetric.TASKS_COMPLETED
        assert record.value == 1

    def test_record_task(self, tracker: UsageTracker) -> None:
        """Test recording a task."""
        record = tracker.record_task("proj-1", "T-001")

        assert record.metric == UsageMetric.TASKS_COMPLETED
        assert record.project_id == "proj-1"

    def test_record_tokens(self, tracker: UsageTracker) -> None:
        """Test recording tokens."""
        record = tracker.record_tokens(1500, model="claude-3")

        assert record.metric == UsageMetric.TOKENS_USED
        assert record.value == 1500

    def test_get_records_filtered(self, tracker: UsageTracker) -> None:
        """Test filtering records."""
        tracker.record(UsageMetric.TASKS_COMPLETED, 1)
        tracker.record(UsageMetric.TOKENS_USED, 100)
        tracker.record(UsageMetric.TASKS_COMPLETED, 1)

        records = tracker.get_records(metric=UsageMetric.TASKS_COMPLETED)

        assert len(records) == 2

    def test_get_total(self, tracker: UsageTracker) -> None:
        """Test getting totals."""
        tracker.record(UsageMetric.TOKENS_USED, 100)
        tracker.record(UsageMetric.TOKENS_USED, 200)
        tracker.record(UsageMetric.TOKENS_USED, 300)

        total = tracker.get_total(UsageMetric.TOKENS_USED)

        assert total == 600

    def test_get_daily_usage(self, tracker: UsageTracker) -> None:
        """Test daily usage summary."""
        tracker.record(UsageMetric.TASKS_COMPLETED, 5)
        tracker.record(UsageMetric.TOKENS_USED, 1000)

        summary = tracker.get_daily_usage()

        assert summary.tasks_completed == 5
        assert summary.tokens_used == 1000

    def test_get_monthly_usage(self, tracker: UsageTracker) -> None:
        """Test monthly usage summary."""
        tracker.record(UsageMetric.TASKS_COMPLETED, 10)
        tracker.record(UsageMetric.TOKENS_USED, 5000)

        summary = tracker.get_monthly_usage()

        assert summary.tasks_completed == 10
        assert summary.tokens_used == 5000

    def test_check_daily_task_limit(self, tracker: UsageTracker) -> None:
        """Test daily task limit check."""
        for _ in range(5):
            tracker.record(UsageMetric.TASKS_COMPLETED, 1)

        within, count = tracker.check_daily_task_limit(10)
        assert within is True
        assert count == 5

        within, count = tracker.check_daily_task_limit(5)
        assert within is False

    def test_check_monthly_token_limit(self, tracker: UsageTracker) -> None:
        """Test monthly token limit check."""
        tracker.record(UsageMetric.TOKENS_USED, 50000)

        within, count = tracker.check_monthly_token_limit(100000)
        assert within is True
        assert count == 50000

    def test_export_for_billing(self, tracker: UsageTracker) -> None:
        """Test billing export."""
        tracker.record(UsageMetric.TASKS_COMPLETED, 5)
        tracker.record(UsageMetric.TOKENS_USED, 1000)

        now = datetime.now()
        export = tracker.export_for_billing(
            since=now - timedelta(days=30),
            until=now,
        )

        assert export["user_id"] == "user-123"
        assert export["totals"]["tasks_completed"] == 5
        assert export["totals"]["tokens_used"] == 1000

    def test_clear(self, tracker: UsageTracker) -> None:
        """Test clearing records."""
        tracker.record(UsageMetric.TASKS_COMPLETED, 1)
        tracker.clear()

        assert len(tracker.get_records()) == 0


class TestSubscriptionStatus:
    """Test SubscriptionStatus enum."""

    def test_values(self) -> None:
        """Test status values."""
        assert SubscriptionStatus.ACTIVE.value == "active"
        assert SubscriptionStatus.CANCELED.value == "canceled"


class TestCustomer:
    """Test Customer dataclass."""

    def test_customer(self) -> None:
        """Test customer creation."""
        customer = Customer(
            id="cus_123",
            email="user@example.com",
            name="Test User",
            created_at=datetime.now(),
        )

        assert customer.id == "cus_123"
        assert customer.email == "user@example.com"


class TestSubscription:
    """Test Subscription dataclass."""

    def test_subscription(self) -> None:
        """Test subscription creation."""
        sub = Subscription(
            id="sub_123",
            customer_id="cus_123",
            plan_type=PlanType.PRO,
            status=SubscriptionStatus.ACTIVE,
            current_period_start=datetime.now(),
            current_period_end=datetime.now() + timedelta(days=30),
        )

        assert sub.status == SubscriptionStatus.ACTIVE
        assert sub.cancel_at_period_end is False


class TestCheckoutSession:
    """Test CheckoutSession dataclass."""

    def test_session(self) -> None:
        """Test checkout session."""
        session = CheckoutSession(
            id="cs_123",
            url="https://checkout.stripe.com/...",
            customer_id=None,
            plan_type=PlanType.PRO,
        )

        assert session.plan_type == PlanType.PRO


class TestStripeClient:
    """Test StripeClient class."""

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment."""
        monkeypatch.setenv("STRIPE_API_KEY", "sk_test_123")
        monkeypatch.setenv("STRIPE_WEBHOOK_SECRET", "whsec_123")

        client = StripeClient()

        assert client._api_key == "sk_test_123"
        assert client._webhook_secret == "whsec_123"

    def test_init_with_params(self) -> None:
        """Test initialization with params."""
        client = StripeClient(
            api_key="sk_test_456",
            webhook_secret="whsec_456",
        )

        assert client._api_key == "sk_test_456"

    def test_is_configured(self) -> None:
        """Test configuration check."""
        unconfigured = StripeClient()
        assert unconfigured.is_configured() is False

        configured = StripeClient(api_key="sk_test_123")
        assert configured.is_configured() is True

    def test_create_checkout_no_price_id(self) -> None:
        """Test checkout fails without price ID."""
        client = StripeClient(api_key="sk_test")

        with pytest.raises(ValueError, match="no Stripe price ID"):
            client.create_checkout_session(
                plan=Plan.free(),  # No price ID
                success_url="https://example.com/success",
                cancel_url="https://example.com/cancel",
            )
