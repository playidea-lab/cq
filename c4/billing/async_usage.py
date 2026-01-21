"""Async Usage Tracking - Monitor usage and sync to Stripe.

This module provides async usage tracking with Stripe synchronization
for metered billing.

Example:
    tracker = AsyncUsageTracker(
        user_id="user-123",
        subscription_item_id="si_xxx",
        stripe_secret_key="sk_test_xxx",
    )

    # Track usage
    await tracker.track_workspace_usage("user-123", minutes=5.0)
    await tracker.track_llm_usage("user-123", input_tokens=1000, output_tokens=500)

    # Sync to Stripe periodically
    await tracker.sync_to_stripe("user-123")
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


@dataclass
class Usage:
    """Usage data for a user.

    Attributes:
        user_id: User identifier
        workspace_minutes: Container/workspace execution time
        llm_input_tokens: LLM input tokens used
        llm_output_tokens: LLM output tokens used
        storage_mb: Storage used in MB
        timestamp: When usage was recorded
    """

    user_id: str
    workspace_minutes: float = 0.0
    llm_input_tokens: int = 0
    llm_output_tokens: int = 0
    storage_mb: float = 0.0
    timestamp: datetime = field(default_factory=datetime.utcnow)

    @property
    def total_llm_tokens(self) -> int:
        """Get total LLM tokens (input + output)."""
        return self.llm_input_tokens + self.llm_output_tokens

    def is_empty(self) -> bool:
        """Check if usage is empty (nothing tracked)."""
        return (
            self.workspace_minutes == 0.0
            and self.llm_input_tokens == 0
            and self.llm_output_tokens == 0
        )


class AsyncUsageTracker:
    """
    Async usage tracker with Stripe synchronization.

    Tracks various usage metrics and syncs them to Stripe
    for metered billing.

    Features:
    - Track workspace/container execution time
    - Track LLM token usage
    - Track storage usage
    - Sync accumulated usage to Stripe

    Attributes:
        user_id: User to track usage for
        _subscription_item_id: Stripe subscription item ID
        _stripe_secret_key: Stripe API key
    """

    def __init__(
        self,
        user_id: str,
        subscription_item_id: str | None = None,
        stripe_secret_key: str | None = None,
    ):
        """Initialize async usage tracker.

        Args:
            user_id: User identifier
            subscription_item_id: Stripe subscription item ID (for sync)
            stripe_secret_key: Stripe API key (for sync)
        """
        self._user_id = user_id
        self._subscription_item_id = subscription_item_id
        self._stripe_secret_key = stripe_secret_key
        self._usage: dict[str, Usage] = {}

    @property
    def user_id(self) -> str:
        """Get user ID."""
        return self._user_id

    # =========================================================================
    # Usage Tracking
    # =========================================================================

    async def track_workspace_usage(self, user_id: str, minutes: float) -> None:
        """Track workspace/container execution time.

        Args:
            user_id: User identifier
            minutes: Execution time in minutes
        """
        usage = self._get_or_create_usage(user_id)
        usage.workspace_minutes += minutes
        usage.timestamp = datetime.now(timezone.utc)

    async def track_llm_usage(
        self,
        user_id: str,
        input_tokens: int,
        output_tokens: int,
    ) -> None:
        """Track LLM token usage.

        Args:
            user_id: User identifier
            input_tokens: Number of input tokens
            output_tokens: Number of output tokens
        """
        usage = self._get_or_create_usage(user_id)
        usage.llm_input_tokens += input_tokens
        usage.llm_output_tokens += output_tokens
        usage.timestamp = datetime.now(timezone.utc)

    async def track_storage(self, user_id: str, mb: float) -> None:
        """Track storage usage.

        Note: Storage is set to current value (not accumulated),
        as it represents current state, not delta.

        Args:
            user_id: User identifier
            mb: Storage in megabytes
        """
        usage = self._get_or_create_usage(user_id)
        usage.storage_mb = mb  # Set, not accumulate
        usage.timestamp = datetime.now(timezone.utc)

    # =========================================================================
    # Usage Retrieval
    # =========================================================================

    async def get_current_usage(self, user_id: str) -> Usage:
        """Get current usage for a user.

        Args:
            user_id: User identifier

        Returns:
            Usage data (empty if no data tracked)
        """
        return self._usage.get(user_id, Usage(user_id=user_id))

    def _get_or_create_usage(self, user_id: str) -> Usage:
        """Get or create usage record for user.

        Args:
            user_id: User identifier

        Returns:
            Usage record
        """
        if user_id not in self._usage:
            self._usage[user_id] = Usage(user_id=user_id)
        return self._usage[user_id]

    # =========================================================================
    # Stripe Synchronization
    # =========================================================================

    async def sync_to_stripe(self, user_id: str) -> None:
        """Sync usage to Stripe metered billing.

        Args:
            user_id: User identifier

        Raises:
            ValueError: If subscription_item_id not configured
        """
        if not self._subscription_item_id:
            raise ValueError("subscription_item_id required for Stripe sync")

        usage = await self.get_current_usage(user_id)

        if usage.is_empty():
            return  # Nothing to sync

        # Import here to avoid circular imports
        from .stripe_integration import StripeBilling

        billing = StripeBilling(secret_key=self._stripe_secret_key)

        # Convert usage to billable units
        # Example: 1 unit = 1 minute of workspace time + 1000 tokens
        total_units = int(usage.workspace_minutes) + (usage.total_llm_tokens // 1000)

        if total_units > 0:
            await billing.record_usage(
                subscription_item_id=self._subscription_item_id,
                quantity=total_units,
            )

        # Clear usage after successful sync
        self._clear_usage(user_id)

    def _clear_usage(self, user_id: str) -> None:
        """Clear usage data for user after sync.

        Args:
            user_id: User identifier
        """
        if user_id in self._usage:
            self._usage[user_id] = Usage(user_id=user_id)

    # =========================================================================
    # Batch Operations
    # =========================================================================

    async def get_all_users_usage(self) -> dict[str, Usage]:
        """Get usage for all tracked users.

        Returns:
            Dictionary of user_id -> Usage
        """
        return dict(self._usage)

    async def sync_all_to_stripe(self) -> int:
        """Sync all users' usage to Stripe.

        Returns:
            Number of users synced
        """
        synced = 0
        for user_id in list(self._usage.keys()):
            usage = self._usage.get(user_id)
            if usage and not usage.is_empty():
                await self.sync_to_stripe(user_id)
                synced += 1
        return synced

    def clear_all(self) -> None:
        """Clear all usage data."""
        self._usage.clear()
