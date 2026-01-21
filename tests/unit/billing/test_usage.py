"""Tests for async UsageTracker with Stripe sync.

TDD RED Phase: Define test scenarios for async usage tracking.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from c4.billing.async_usage import AsyncUsageTracker, Usage


class TestUsageDataclass:
    """Test Usage dataclass."""

    def test_usage_defaults(self) -> None:
        """Test Usage default values."""
        usage = Usage(user_id="user-123")

        assert usage.user_id == "user-123"
        assert usage.workspace_minutes == 0.0
        assert usage.llm_input_tokens == 0
        assert usage.llm_output_tokens == 0
        assert usage.storage_mb == 0.0
        assert usage.timestamp is not None

    def test_usage_custom_values(self) -> None:
        """Test Usage with custom values."""
        usage = Usage(
            user_id="user-456",
            workspace_minutes=10.5,
            llm_input_tokens=1000,
            llm_output_tokens=500,
            storage_mb=100.0,
        )

        assert usage.workspace_minutes == 10.5
        assert usage.llm_input_tokens == 1000
        assert usage.llm_output_tokens == 500
        assert usage.storage_mb == 100.0


class TestAsyncUsageTrackerInit:
    """Test AsyncUsageTracker initialization."""

    def test_init_basic(self) -> None:
        """Test basic initialization."""
        tracker = AsyncUsageTracker(user_id="user-123")
        assert tracker.user_id == "user-123"

    def test_init_with_subscription_item(self) -> None:
        """Test initialization with subscription item ID."""
        tracker = AsyncUsageTracker(
            user_id="user-123",
            subscription_item_id="si_item123",
        )
        assert tracker._subscription_item_id == "si_item123"


class TestAsyncUsageTrackerTracking:
    """Test usage tracking methods."""

    @pytest.fixture
    def tracker(self) -> AsyncUsageTracker:
        """Create tracker instance."""
        return AsyncUsageTracker(user_id="user-123")

    @pytest.mark.asyncio
    async def test_track_workspace_usage(self, tracker: AsyncUsageTracker) -> None:
        """Test tracking workspace minutes."""
        await tracker.track_workspace_usage("user-123", minutes=5.5)

        usage = await tracker.get_current_usage("user-123")
        assert usage.workspace_minutes == 5.5

    @pytest.mark.asyncio
    async def test_track_workspace_usage_accumulates(self, tracker: AsyncUsageTracker) -> None:
        """Test workspace usage accumulates."""
        await tracker.track_workspace_usage("user-123", minutes=5.0)
        await tracker.track_workspace_usage("user-123", minutes=3.5)

        usage = await tracker.get_current_usage("user-123")
        assert usage.workspace_minutes == 8.5

    @pytest.mark.asyncio
    async def test_track_llm_usage(self, tracker: AsyncUsageTracker) -> None:
        """Test tracking LLM token usage."""
        await tracker.track_llm_usage("user-123", input_tokens=1000, output_tokens=500)

        usage = await tracker.get_current_usage("user-123")
        assert usage.llm_input_tokens == 1000
        assert usage.llm_output_tokens == 500

    @pytest.mark.asyncio
    async def test_track_llm_usage_accumulates(self, tracker: AsyncUsageTracker) -> None:
        """Test LLM usage accumulates."""
        await tracker.track_llm_usage("user-123", input_tokens=1000, output_tokens=500)
        await tracker.track_llm_usage("user-123", input_tokens=2000, output_tokens=800)

        usage = await tracker.get_current_usage("user-123")
        assert usage.llm_input_tokens == 3000
        assert usage.llm_output_tokens == 1300

    @pytest.mark.asyncio
    async def test_track_storage(self, tracker: AsyncUsageTracker) -> None:
        """Test tracking storage usage."""
        await tracker.track_storage("user-123", mb=100.5)

        usage = await tracker.get_current_usage("user-123")
        assert usage.storage_mb == 100.5

    @pytest.mark.asyncio
    async def test_track_storage_sets_not_accumulates(self, tracker: AsyncUsageTracker) -> None:
        """Test storage is set (not accumulated) - it's current state."""
        await tracker.track_storage("user-123", mb=100.0)
        await tracker.track_storage("user-123", mb=150.0)

        usage = await tracker.get_current_usage("user-123")
        assert usage.storage_mb == 150.0  # Last value, not accumulated


class TestAsyncUsageTrackerGetUsage:
    """Test getting current usage."""

    @pytest.fixture
    def tracker(self) -> AsyncUsageTracker:
        """Create tracker instance."""
        return AsyncUsageTracker(user_id="user-123")

    @pytest.mark.asyncio
    async def test_get_current_usage_empty(self, tracker: AsyncUsageTracker) -> None:
        """Test getting usage with no tracked data."""
        usage = await tracker.get_current_usage("user-123")

        assert usage.user_id == "user-123"
        assert usage.workspace_minutes == 0.0
        assert usage.llm_input_tokens == 0

    @pytest.mark.asyncio
    async def test_get_current_usage_different_user(self, tracker: AsyncUsageTracker) -> None:
        """Test getting usage for different user."""
        await tracker.track_workspace_usage("user-123", minutes=10.0)

        # Get usage for different user
        usage = await tracker.get_current_usage("user-456")

        assert usage.user_id == "user-456"
        assert usage.workspace_minutes == 0.0  # Different user, no data


class TestAsyncUsageTrackerStripeSync:
    """Test Stripe synchronization."""

    @pytest.fixture
    def tracker(self) -> AsyncUsageTracker:
        """Create tracker with subscription item."""
        return AsyncUsageTracker(
            user_id="user-123",
            subscription_item_id="si_item123",
            stripe_secret_key="sk_test_123",
        )

    @pytest.mark.asyncio
    async def test_sync_to_stripe(self, tracker: AsyncUsageTracker) -> None:
        """Test syncing usage to Stripe."""
        # Track some usage
        await tracker.track_workspace_usage("user-123", minutes=10.0)
        await tracker.track_llm_usage("user-123", input_tokens=5000, output_tokens=2000)

        with patch("c4.billing.stripe_integration.StripeBilling") as mock_billing_class:
            mock_billing = AsyncMock()
            mock_billing_class.return_value = mock_billing

            await tracker.sync_to_stripe("user-123")

            # Should have called record_usage
            mock_billing.record_usage.assert_called()

    @pytest.mark.asyncio
    async def test_sync_to_stripe_clears_usage(self, tracker: AsyncUsageTracker) -> None:
        """Test syncing clears local usage after successful sync."""
        await tracker.track_workspace_usage("user-123", minutes=10.0)

        with patch("c4.billing.stripe_integration.StripeBilling") as mock_billing_class:
            mock_billing = AsyncMock()
            mock_billing_class.return_value = mock_billing

            await tracker.sync_to_stripe("user-123")

            # Usage should be cleared after sync
            usage = await tracker.get_current_usage("user-123")
            assert usage.workspace_minutes == 0.0

    @pytest.mark.asyncio
    async def test_sync_to_stripe_no_subscription_item(self) -> None:
        """Test sync without subscription item ID raises error."""
        tracker = AsyncUsageTracker(user_id="user-123")  # No subscription_item_id

        await tracker.track_workspace_usage("user-123", minutes=10.0)

        with pytest.raises(ValueError, match="subscription_item_id"):
            await tracker.sync_to_stripe("user-123")

    @pytest.mark.asyncio
    async def test_sync_to_stripe_empty_usage(self, tracker: AsyncUsageTracker) -> None:
        """Test sync with no usage data does nothing."""
        with patch("c4.billing.stripe_integration.StripeBilling") as mock_billing_class:
            mock_billing = AsyncMock()
            mock_billing_class.return_value = mock_billing

            await tracker.sync_to_stripe("user-123")

            # Should not call record_usage for zero usage
            mock_billing.record_usage.assert_not_called()
