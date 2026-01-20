"""Unit tests for Cost Optimizer."""

from c4.supervisor.claude_models import ClaudeModelTier
from c4.supervisor.cost_optimizer import (
    CostAlert,
    CostAlertInfo,
    CostEstimate,
    CostOptimizer,
    ModelSelection,
    OptimizedPrompt,
    TaskComplexity,
    create_cost_optimizer,
)


class TestTaskComplexity:
    """Tests for TaskComplexity enum."""

    def test_complexity_values(self):
        """All expected complexity values exist."""
        assert TaskComplexity.LOW == "low"
        assert TaskComplexity.MEDIUM == "medium"
        assert TaskComplexity.HIGH == "high"


class TestCostAlert:
    """Tests for CostAlert enum."""

    def test_alert_types(self):
        """All expected alert types exist."""
        assert CostAlert.BUDGET_WARNING == "budget_warning"
        assert CostAlert.BUDGET_EXCEEDED == "budget_exceeded"
        assert CostAlert.MODEL_UNAVAILABLE == "model_unavailable"
        assert CostAlert.RATE_LIMITED == "rate_limited"


class TestCostAlertInfo:
    """Tests for CostAlertInfo dataclass."""

    def test_create_alert_info(self):
        """Test creating an alert info."""
        alert = CostAlertInfo(
            alert_type=CostAlert.BUDGET_EXCEEDED,
            message="Budget exceeded",
            details={"remaining": 0.0, "estimated_cost": 0.5},
        )
        assert alert.alert_type == CostAlert.BUDGET_EXCEEDED
        assert alert.message == "Budget exceeded"
        assert alert.details["remaining"] == 0.0


class TestModelSelection:
    """Tests for ModelSelection dataclass."""

    def test_create_selection(self):
        """Test creating a model selection."""
        selection = ModelSelection(
            model_id="claude-sonnet-4-20250514",
            tier=ClaudeModelTier.SONNET,
            reason="Selected for medium complexity",
            estimated_cost=0.05,
        )
        assert selection.model_id == "claude-sonnet-4-20250514"
        assert selection.tier == ClaudeModelTier.SONNET
        assert selection.estimated_cost == 0.05


class TestOptimizedPrompt:
    """Tests for OptimizedPrompt dataclass."""

    def test_create_optimized_prompt(self):
        """Test creating an optimized prompt."""
        optimized = OptimizedPrompt(
            content="optimized content",
            original_length=1000,
            optimized_length=800,
            tokens_saved_estimate=50,
            cache_hints=["cache_key:abc123"],
            truncated=False,
        )
        assert optimized.optimized_length == 800
        assert optimized.tokens_saved_estimate == 50
        assert not optimized.truncated


class TestCostEstimate:
    """Tests for CostEstimate dataclass."""

    def test_create_estimate(self):
        """Test creating a cost estimate."""
        estimate = CostEstimate(
            model="claude-sonnet-4-20250514",
            input_tokens=1000,
            output_tokens_estimate=500,
            total_tokens=1500,
            estimated_cost=0.0105,
            budget_percentage=10.5,
        )
        assert estimate.input_tokens == 1000
        assert estimate.estimated_cost == 0.0105


class TestCostOptimizer:
    """Tests for CostOptimizer class."""

    def test_default_initialization(self):
        """Test default optimizer initialization."""
        optimizer = CostOptimizer()
        assert optimizer.budget is None
        assert optimizer.prefer_cost_savings is False
        assert optimizer.cache_prompts is True

    def test_custom_initialization(self):
        """Test custom optimizer initialization."""
        optimizer = CostOptimizer(
            budget=10.0,
            prefer_cost_savings=True,
            cache_prompts=False,
        )
        assert optimizer.budget == 10.0
        assert optimizer.prefer_cost_savings is True
        assert optimizer.cache_prompts is False


class TestModelSelectionLogic:
    """Tests for model selection logic."""

    def test_select_model_low_complexity(self):
        """Select Haiku for low complexity."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Fix the typo",
            complexity=TaskComplexity.LOW,
        )
        assert selection.tier == ClaudeModelTier.HAIKU

    def test_select_model_medium_complexity(self):
        """Select Sonnet for medium complexity."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Review this code",
            complexity=TaskComplexity.MEDIUM,
        )
        assert selection.tier == ClaudeModelTier.SONNET

    def test_select_model_high_complexity(self):
        """Select Sonnet for high complexity (default)."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Architect the system",
            complexity=TaskComplexity.HIGH,
        )
        # High complexity uses Sonnet by default (Opus for critical only)
        assert selection.tier == ClaudeModelTier.SONNET

    def test_select_model_default_complexity(self):
        """Default complexity is MEDIUM."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model("Some task")
        # Default is MEDIUM -> Sonnet
        assert selection.tier == ClaudeModelTier.SONNET
        assert "Complexity: medium" in selection.reason

    def test_select_model_string_complexity(self):
        """Accept string complexity value."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model("Review code", complexity="medium")
        assert selection.tier == ClaudeModelTier.SONNET

    def test_select_model_with_min_tier(self):
        """Enforce minimum tier constraint."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Simple fix",
            complexity=TaskComplexity.LOW,
            min_tier=ClaudeModelTier.SONNET,
        )
        # Should upgrade from Haiku to Sonnet
        assert selection.tier == ClaudeModelTier.SONNET
        assert "Upgraded" in selection.reason

    def test_select_model_with_max_tier(self):
        """Enforce maximum tier constraint."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Complex architecture",
            complexity=TaskComplexity.HIGH,
            max_tier=ClaudeModelTier.HAIKU,
        )
        # Should cap at Haiku
        assert selection.tier == ClaudeModelTier.HAIKU
        assert "Capped" in selection.reason

    def test_select_model_cost_savings(self):
        """Prefer cheaper model when cost savings enabled."""
        optimizer = CostOptimizer(prefer_cost_savings=True)
        selection = optimizer.select_model(
            "Review code",
            complexity=TaskComplexity.MEDIUM,
        )
        # With cost savings, medium tasks use Haiku
        assert selection.tier == ClaudeModelTier.HAIKU
        assert "Cost savings" in selection.reason

    def test_select_model_includes_estimate(self):
        """Model selection includes cost estimate."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model("Review this code", complexity=TaskComplexity.MEDIUM)
        assert selection.estimated_cost is not None
        assert selection.estimated_cost >= 0


class TestPromptOptimization:
    """Tests for prompt optimization."""

    def test_optimize_removes_whitespace(self):
        """Remove excessive whitespace."""
        optimizer = CostOptimizer()
        prompt = "Line 1\n\n\n\n\nLine 2   with   spaces"

        result = optimizer.optimize_prompt(prompt)
        assert "\n\n\n" not in result.content
        assert "   " not in result.content

    def test_optimize_truncates_long_prompt(self):
        """Truncate very long prompts."""
        optimizer = CostOptimizer()
        long_prompt = "x" * 100000  # Very long prompt

        result = optimizer.optimize_prompt(long_prompt, max_tokens=1000)
        assert result.truncated is True
        assert len(result.content) < len(long_prompt)
        assert "TRUNCATED" in result.content

    def test_optimize_preserves_short_prompt(self):
        """Don't truncate short prompts."""
        optimizer = CostOptimizer()
        short_prompt = "Review this code"

        result = optimizer.optimize_prompt(short_prompt)
        assert result.truncated is False
        assert result.content == short_prompt

    def test_optimize_tracks_savings(self):
        """Track token savings estimate."""
        optimizer = CostOptimizer()
        prompt = "Line 1\n\n\n\n\nLine 2"  # Has excessive newlines

        result = optimizer.optimize_prompt(prompt)
        assert result.original_length >= result.optimized_length
        assert result.tokens_saved_estimate >= 0

    def test_optimize_generates_cache_hints(self):
        """Generate cache hints when enabled."""
        optimizer = CostOptimizer(cache_prompts=True)
        prompt = "You are a helpful assistant. Review this code."

        result = optimizer.optimize_prompt(prompt)
        assert len(result.cache_hints) > 0
        assert any("cache_key" in hint for hint in result.cache_hints)

    def test_optimize_no_cache_hints_when_disabled(self):
        """No cache hints when disabled."""
        optimizer = CostOptimizer(cache_prompts=False)
        prompt = "Review this code"

        result = optimizer.optimize_prompt(prompt)
        assert len(result.cache_hints) == 0

    def test_optimize_with_use_case(self):
        """Apply use-case specific limits."""
        optimizer = CostOptimizer()

        # Summary has lower token limit
        result = optimizer.optimize_prompt(
            "x" * 50000,
            use_case="summary",
        )
        # Should truncate based on summary limit
        assert result.truncated is True


class TestCostEstimation:
    """Tests for cost estimation."""

    def test_estimate_cost(self):
        """Estimate cost for a request."""
        optimizer = CostOptimizer()
        estimate = optimizer.estimate_cost(
            prompt="Review this code" * 100,
            model="claude-sonnet-4-20250514",
        )

        assert estimate.input_tokens > 0
        assert estimate.output_tokens_estimate > 0
        assert estimate.estimated_cost > 0

    def test_estimate_cost_uses_default_model(self):
        """Use default model when not specified."""
        optimizer = CostOptimizer(default_model="claude-3-5-haiku-20241022")
        estimate = optimizer.estimate_cost("Test prompt")
        assert estimate.model == "claude-3-5-haiku-20241022"

    def test_estimate_cost_with_expected_output(self):
        """Use specified expected output tokens."""
        optimizer = CostOptimizer()
        estimate = optimizer.estimate_cost(
            "Test prompt",
            expected_output_tokens=5000,
        )
        assert estimate.output_tokens_estimate == 5000

    def test_estimate_cost_budget_percentage(self):
        """Include budget percentage when budget set."""
        optimizer = CostOptimizer(budget=1.0)
        estimate = optimizer.estimate_cost("Test prompt")
        assert estimate.budget_percentage is not None


class TestBudgetManagement:
    """Tests for budget management."""

    def test_record_usage(self):
        """Record usage updates session cost."""
        optimizer = CostOptimizer()
        optimizer.record_usage(0.05)
        optimizer.record_usage(0.03)
        assert optimizer._session_cost == 0.08

    def test_check_budget_within(self):
        """Check budget returns True when within."""
        optimizer = CostOptimizer(budget=1.0)
        assert optimizer.check_budget(0.5) is True

    def test_check_budget_exceeded(self):
        """Check budget returns False when exceeded."""
        optimizer = CostOptimizer(budget=0.1)
        optimizer.record_usage(0.08)
        assert optimizer.check_budget(0.05) is False

    def test_check_budget_no_budget(self):
        """Check budget returns True when no budget set."""
        optimizer = CostOptimizer()  # No budget
        assert optimizer.check_budget(100.0) is True

    def test_get_budget_status_no_budget(self):
        """Get status when no budget set."""
        optimizer = CostOptimizer()
        optimizer.record_usage(0.05)

        status = optimizer.get_budget_status()
        assert status["budget"] is None
        assert status["used"] == 0.05
        assert status["remaining"] is None

    def test_get_budget_status_with_budget(self):
        """Get status with budget set."""
        optimizer = CostOptimizer(budget=1.0)
        optimizer.record_usage(0.30)

        status = optimizer.get_budget_status()
        assert status["budget"] == 1.0
        assert status["used"] == 0.30
        assert status["remaining"] == 0.70
        assert status["percentage_used"] == 30.0
        assert status["is_exceeded"] is False

    def test_get_budget_status_exceeded(self):
        """Get status when budget exceeded."""
        optimizer = CostOptimizer(budget=0.1)
        optimizer.record_usage(0.15)

        status = optimizer.get_budget_status()
        assert status["is_exceeded"] is True
        assert status["remaining"] < 0


class TestBudgetAlerts:
    """Tests for budget alert creation."""

    def test_no_alert_without_budget(self):
        """No alert when no budget set."""
        optimizer = CostOptimizer()  # No budget
        alert = optimizer.create_budget_alert(0.5)
        assert alert is None

    def test_no_alert_within_budget(self):
        """No alert when within budget threshold."""
        optimizer = CostOptimizer(budget=1.0)
        alert = optimizer.create_budget_alert(0.1)
        assert alert is None

    def test_budget_warning_alert(self):
        """Create warning alert when approaching threshold."""
        optimizer = CostOptimizer(budget=1.0)
        optimizer.record_usage(0.85)  # 85% used

        alert = optimizer.create_budget_alert(0.05)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_WARNING
        assert "warning" in alert.message.lower()
        assert alert.details["percentage_used"] >= 80.0

    def test_budget_exceeded_alert(self):
        """Create exceeded alert when over budget."""
        optimizer = CostOptimizer(budget=0.1)
        optimizer.record_usage(0.08)

        # Estimated cost exceeds remaining budget
        alert = optimizer.create_budget_alert(0.05)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_EXCEEDED
        assert "exceeded" in alert.message.lower()
        assert alert.details["estimated_cost"] == 0.05

    def test_custom_threshold(self):
        """Use custom threshold for warning."""
        optimizer = CostOptimizer(budget=1.0)
        optimizer.record_usage(0.55)  # 55% used

        # Default threshold (80%) - no alert
        alert = optimizer.create_budget_alert(0.1)
        assert alert is None

        # Custom threshold (50%) - alert
        alert = optimizer.create_budget_alert(0.1, threshold_percentage=50.0)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_WARNING


class TestModelUnavailableAlert:
    """Tests for model unavailable alerts."""

    def test_create_model_unavailable_alert(self):
        """Create alert for unavailable model."""
        optimizer = CostOptimizer()
        alert = optimizer.create_model_unavailable_alert(
            model="claude-opus-4-20250514",
            reason="Quota exceeded",
        )

        assert alert.alert_type == CostAlert.MODEL_UNAVAILABLE
        assert "claude-opus-4-20250514" in alert.message
        assert "Quota exceeded" in alert.message
        assert alert.details["model"] == "claude-opus-4-20250514"
        assert alert.details["reason"] == "Quota exceeded"


class TestRateLimitAlert:
    """Tests for rate limit alerts."""

    def test_create_rate_limit_alert(self):
        """Create alert for rate limiting."""
        optimizer = CostOptimizer()
        alert = optimizer.create_rate_limit_alert(
            model="claude-sonnet-4-20250514",
            retry_after=30.0,
        )

        assert alert.alert_type == CostAlert.RATE_LIMITED
        assert "claude-sonnet-4-20250514" in alert.message
        assert "30.0" in alert.message
        assert alert.details["model"] == "claude-sonnet-4-20250514"
        assert alert.details["retry_after"] == 30.0

    def test_rate_limit_alert_without_retry_after(self):
        """Create alert without retry_after."""
        optimizer = CostOptimizer()
        alert = optimizer.create_rate_limit_alert(
            model="claude-sonnet-4-20250514",
        )

        assert alert.alert_type == CostAlert.RATE_LIMITED
        assert "Retry after" not in alert.message
        assert alert.details["retry_after"] is None


class TestCreateCostOptimizer:
    """Tests for create_cost_optimizer factory."""

    def test_create_basic(self):
        """Create basic optimizer."""
        optimizer = create_cost_optimizer()
        assert optimizer.budget is None
        assert optimizer.prefer_cost_savings is False

    def test_create_with_budget(self):
        """Create optimizer with budget."""
        optimizer = create_cost_optimizer(budget=10.0)
        assert optimizer.budget == 10.0

    def test_create_cost_savings(self):
        """Create optimizer with cost savings preference."""
        optimizer = create_cost_optimizer(prefer_cost_savings=True)
        assert optimizer.prefer_cost_savings is True


class TestIntegration:
    """Integration tests for cost optimizer."""

    def test_full_workflow(self):
        """Test complete cost optimization workflow."""
        optimizer = CostOptimizer(budget=1.0)

        # 1. Select model with explicit complexity
        selection = optimizer.select_model(
            "Review this pull request for security issues",
            complexity=TaskComplexity.MEDIUM,
        )
        assert selection.model_id is not None

        # 2. Optimize prompt
        long_prompt = "Review this: " + "code " * 1000
        optimized = optimizer.optimize_prompt(long_prompt, max_tokens=2000)
        assert optimized.tokens_saved_estimate >= 0

        # 3. Estimate cost
        estimate = optimizer.estimate_cost(
            optimized.content,
            model=selection.model_id,
        )
        assert estimate.estimated_cost > 0

        # 4. Check budget alert
        alert = optimizer.create_budget_alert(estimate.estimated_cost)
        assert alert is None  # Should be within budget

        # 5. Check budget
        assert optimizer.check_budget(estimate.estimated_cost) is True

        # 6. Record usage
        optimizer.record_usage(estimate.estimated_cost)

        # 7. Check status
        status = optimizer.get_budget_status()
        assert status["used"] > 0
        assert status["is_exceeded"] is False

    def test_budget_exceeded_workflow(self):
        """Test workflow when budget is exceeded."""
        optimizer = CostOptimizer(budget=0.01)  # Very small budget

        # Try to make a request that exceeds budget
        estimate = optimizer.estimate_cost(
            "A long prompt " * 100,
            model="claude-sonnet-4-20250514",
        )

        # Should get budget exceeded alert
        alert = optimizer.create_budget_alert(estimate.estimated_cost)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_EXCEEDED

        # Should fail budget check
        assert optimizer.check_budget(estimate.estimated_cost) is False
