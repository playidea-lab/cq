"""Tests for C4 API Proxy - Rate limiting, metering, and LLM proxy."""

import time
from datetime import datetime, timedelta

import pytest

from c4.api.metering import (
    MODEL_COSTS,
    ModelProvider,
    UsageMeter,
    UsageRecord,
    UsageSummary,
    estimate_cost,
)
from c4.api.rate_limit import (
    RateLimitConfig,
    RateLimiter,
    RateLimitStore,
    TokenBucket,
)

# =============================================================================
# Token Bucket Tests
# =============================================================================


class TestTokenBucket:
    """Test TokenBucket rate limiter."""

    def test_initial_capacity(self):
        """Test bucket starts at full capacity."""
        bucket = TokenBucket(capacity=10, fill_rate=1)
        assert bucket.tokens == 10

    def test_consume_success(self):
        """Test successful token consumption."""
        bucket = TokenBucket(capacity=10, fill_rate=1)
        assert bucket.consume(5) is True
        assert bucket.available == pytest.approx(5, abs=0.1)

    def test_consume_insufficient(self):
        """Test failed consumption when insufficient."""
        bucket = TokenBucket(capacity=10, fill_rate=1)
        bucket.consume(10)
        assert bucket.consume(1) is False

    def test_refill_over_time(self):
        """Test bucket refills over time."""
        bucket = TokenBucket(capacity=10, fill_rate=10)  # 10 tokens/sec
        bucket.consume(10)  # Empty bucket

        # Simulate time passing
        bucket.last_update -= 0.5  # 0.5 seconds ago
        bucket._refill()

        assert bucket.tokens == pytest.approx(5, abs=1)

    def test_refill_caps_at_capacity(self):
        """Test refill doesn't exceed capacity."""
        bucket = TokenBucket(capacity=10, fill_rate=100)
        bucket.last_update -= 1  # 1 second ago

        assert bucket.available <= 10


# =============================================================================
# Rate Limiter Tests
# =============================================================================


class TestRateLimiter:
    """Test RateLimiter with multiple buckets."""

    def test_default_config(self):
        """Test limiter with default config."""
        limiter = RateLimiter()
        assert limiter.minute_bucket is not None
        assert limiter.hour_bucket is not None

    def test_check_request_limit_allowed(self):
        """Test request is allowed within limits."""
        limiter = RateLimiter()
        allowed, reason = limiter.check_request_limit()
        assert allowed is True
        assert reason is None

    def test_check_request_limit_exceeded(self):
        """Test request denied when limit exceeded."""
        config = RateLimitConfig(requests_per_minute=1, burst_multiplier=1)
        limiter = RateLimiter(config=config)

        # First request allowed
        allowed1, _ = limiter.check_request_limit()
        assert allowed1 is True

        # Second request denied
        allowed2, reason = limiter.check_request_limit()
        assert allowed2 is False
        assert "minute" in reason.lower()

    def test_check_token_limit(self):
        """Test token limit checking."""
        config = RateLimitConfig(tokens_per_minute=100, burst_multiplier=1)
        limiter = RateLimiter(config=config)

        # Small request allowed
        allowed1, _ = limiter.check_token_limit(50)
        assert allowed1 is True

        # Large request denied
        allowed2, reason = limiter.check_token_limit(60)
        assert allowed2 is False

    def test_get_status(self):
        """Test getting rate limit status."""
        limiter = RateLimiter()
        status = limiter.get_status()

        assert "requests_per_minute_available" in status
        assert "requests_per_hour_available" in status
        assert "tokens_per_minute_available" in status
        assert "tokens_per_hour_available" in status


# =============================================================================
# Rate Limit Store Tests
# =============================================================================


class TestRateLimitStore:
    """Test RateLimitStore for per-user rate limiting."""

    @pytest.fixture
    def store(self):
        """Create rate limit store."""
        return RateLimitStore()

    @pytest.mark.asyncio
    async def test_get_limiter_creates_new(self, store):
        """Test getting limiter creates new one."""
        limiter = await store.get_limiter("user1")
        assert limiter is not None

    @pytest.mark.asyncio
    async def test_get_limiter_returns_same(self, store):
        """Test getting same key returns same limiter."""
        limiter1 = await store.get_limiter("user1")
        limiter2 = await store.get_limiter("user1")
        assert limiter1 is limiter2

    @pytest.mark.asyncio
    async def test_different_users_different_limiters(self, store):
        """Test different users get different limiters."""
        limiter1 = await store.get_limiter("user1")
        limiter2 = await store.get_limiter("user2")
        assert limiter1 is not limiter2

    def test_cleanup_expired(self, store):
        """Test cleanup removes old limiters."""
        # Add some limiters with old timestamps
        store._limiters["old_user"] = RateLimiter()
        store._limiters["old_user"].minute_bucket.last_update = (
            time.monotonic() - 7200
        )  # 2 hours ago

        removed = store.cleanup_expired(max_age_seconds=3600)
        assert removed == 1
        assert "old_user" not in store._limiters


# =============================================================================
# Usage Metering Tests
# =============================================================================


class TestEstimateCost:
    """Test cost estimation."""

    def test_known_model_cost(self):
        """Test cost estimation for known model."""
        cost = estimate_cost("gpt-4o", 1000, 500)
        assert cost is not None
        assert cost > 0

    def test_unknown_model_returns_none(self):
        """Test unknown model returns None."""
        cost = estimate_cost("unknown-model", 1000, 500)
        assert cost is None

    def test_case_insensitive(self):
        """Test model name is case insensitive."""
        cost1 = estimate_cost("GPT-4o", 1000, 500)
        cost2 = estimate_cost("gpt-4o", 1000, 500)
        assert cost1 == cost2


class TestUsageRecord:
    """Test UsageRecord dataclass."""

    def test_create_record(self):
        """Test creating usage record."""
        record = UsageRecord(
            timestamp=datetime.now(),
            model="gpt-4o",
            provider=ModelProvider.OPENAI,
            prompt_tokens=100,
            completion_tokens=50,
            total_tokens=150,
        )
        assert record.total_tokens == 150
        assert record.success is True

    def test_to_dict(self):
        """Test converting record to dict."""
        record = UsageRecord(
            timestamp=datetime.now(),
            model="gpt-4o",
            provider=ModelProvider.OPENAI,
            prompt_tokens=100,
            completion_tokens=50,
            total_tokens=150,
        )
        data = record.to_dict()
        assert data["model"] == "gpt-4o"
        assert data["provider"] == "openai"


class TestUsageMeter:
    """Test UsageMeter for tracking API usage."""

    @pytest.fixture
    def meter(self):
        """Create usage meter."""
        return UsageMeter()

    @pytest.mark.asyncio
    async def test_record_usage(self, meter):
        """Test recording usage."""
        record = await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=100,
            completion_tokens=50,
        )

        assert record.model == "gpt-4o"
        assert record.total_tokens == 150
        assert record.provider == ModelProvider.OPENAI

    @pytest.mark.asyncio
    async def test_provider_detection(self, meter):
        """Test automatic provider detection."""
        # OpenAI
        record1 = await meter.record_usage("gpt-4o", 10, 10)
        assert record1.provider == ModelProvider.OPENAI

        # Anthropic
        record2 = await meter.record_usage("claude-3-sonnet", 10, 10)
        assert record2.provider == ModelProvider.ANTHROPIC

        # Azure
        record3 = await meter.record_usage("azure/gpt-4", 10, 10)
        assert record3.provider == ModelProvider.AZURE

    @pytest.mark.asyncio
    async def test_get_summary(self, meter):
        """Test getting usage summary."""
        # Record some usage
        await meter.record_usage("gpt-4o", 100, 50)
        await meter.record_usage("gpt-4o", 200, 100)
        await meter.record_usage("claude-3-sonnet", 150, 75)

        summary = meter.get_summary()

        assert summary.total_requests == 3
        assert summary.successful_requests == 3
        assert summary.total_prompt_tokens == 450
        assert summary.total_completion_tokens == 225
        assert len(summary.by_model) == 2

    @pytest.mark.asyncio
    async def test_get_summary_filtered(self, meter):
        """Test getting filtered summary."""
        await meter.record_usage("gpt-4o", 100, 50, user_id="user1")
        await meter.record_usage("gpt-4o", 200, 100, user_id="user2")

        summary = meter.get_summary(user_id="user1")

        assert summary.total_requests == 1
        assert summary.total_prompt_tokens == 100

    @pytest.mark.asyncio
    async def test_get_recent_records(self, meter):
        """Test getting recent records."""
        await meter.record_usage("gpt-4o", 100, 50)
        await meter.record_usage("gpt-4o", 200, 100)

        records = meter.get_recent_records(limit=10)

        assert len(records) == 2
        # Most recent first
        assert records[0].prompt_tokens == 200

    @pytest.mark.asyncio
    async def test_record_with_error(self, meter):
        """Test recording failed request."""
        record = await meter.record_usage(
            model="gpt-4o",
            prompt_tokens=0,
            completion_tokens=0,
            success=False,
            error="API error",
        )

        assert record.success is False
        assert record.error == "API error"

        summary = meter.get_summary()
        assert summary.failed_requests == 1

    def test_clear(self, meter):
        """Test clearing records."""
        # Add some records synchronously for simplicity
        meter._records.append(
            UsageRecord(
                timestamp=datetime.now(),
                model="gpt-4o",
                provider=ModelProvider.OPENAI,
                prompt_tokens=100,
                completion_tokens=50,
                total_tokens=150,
            )
        )

        count = meter.clear()
        assert count == 1
        assert len(meter._records) == 0


class TestUsageSummary:
    """Test UsageSummary dataclass."""

    def test_to_dict(self):
        """Test converting summary to dict."""
        summary = UsageSummary(
            period_start=datetime.now() - timedelta(hours=1),
            period_end=datetime.now(),
            total_requests=10,
            total_tokens=5000,
        )
        data = summary.to_dict()
        assert data["total_requests"] == 10
        assert data["total_tokens"] == 5000


# =============================================================================
# Model Costs Tests
# =============================================================================


class TestModelCosts:
    """Test model cost definitions."""

    def test_known_models_have_costs(self):
        """Test known models have cost definitions."""
        expected_models = ["gpt-4o", "claude-3-sonnet", "gpt-3.5-turbo"]
        for model in expected_models:
            assert model in MODEL_COSTS

    def test_cost_structure(self):
        """Test cost structure is correct."""
        for model, costs in MODEL_COSTS.items():
            assert "input" in costs
            assert "output" in costs
            assert costs["input"] >= 0
            assert costs["output"] >= 0
