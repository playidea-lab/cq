"""Unit tests for Cost Optimizer."""

from c4.supervisor.claude_models import ClaudeModelTier
from c4.supervisor.cost_optimizer import (
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
        assert TaskComplexity.AUTO == "auto"


class TestModelSelection:
    """Tests for ModelSelection dataclass."""

    def test_create_selection(self):
        """Test creating a model selection."""
        selection = ModelSelection(
            model_id="claude-sonnet-4-20250514",
            tier=ClaudeModelTier.SONNET,
            reason="Selected for medium complexity",
            estimated_cost=0.05,
            fallback_model="claude-3-5-haiku-20241022",
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


class TestComplexityDetection:
    """Tests for complexity detection."""

    def test_detect_low_complexity(self):
        """Detect low complexity tasks."""
        optimizer = CostOptimizer()

        prompts = [
            "Fix the typo in this file",
            "Format this code",
            "Simple rename of variable",
            "Quick lint fix",
        ]

        for prompt in prompts:
            complexity = optimizer.detect_complexity(prompt)
            assert complexity == TaskComplexity.LOW, f"Failed for: {prompt}"

    def test_detect_medium_complexity(self):
        """Detect medium complexity tasks."""
        optimizer = CostOptimizer()

        prompts = [
            "Review this pull request",
            "Implement the login feature",
            "Fix the bug in the payment module",
            "Add unit tests for the service",
        ]

        for prompt in prompts:
            complexity = optimizer.detect_complexity(prompt)
            assert complexity == TaskComplexity.MEDIUM, f"Failed for: {prompt}"

    def test_detect_high_complexity(self):
        """Detect high complexity tasks."""
        optimizer = CostOptimizer()

        prompts = [
            "Architect the new microservice system",
            "Design the distributed cache system",
            "Refactor the entire authentication module",
            "Deep analysis of the performance bottlenecks",
            "Security audit of the payment flow",
        ]

        for prompt in prompts:
            complexity = optimizer.detect_complexity(prompt)
            assert complexity == TaskComplexity.HIGH, f"Failed for: {prompt}"

    def test_detect_complexity_with_context(self):
        """Detect complexity using context hints."""
        optimizer = CostOptimizer()

        # Large file count suggests high complexity
        complexity = optimizer.detect_complexity(
            "Review these changes",
            context={"file_count": 15},
        )
        assert complexity == TaskComplexity.HIGH

        # Many lines of code suggests high complexity
        complexity = optimizer.detect_complexity(
            "Review these changes",
            context={"code_lines": 2000},
        )
        assert complexity == TaskComplexity.HIGH

        # Architecture flag
        complexity = optimizer.detect_complexity(
            "Review these changes",
            context={"architecture": True},
        )
        assert complexity == TaskComplexity.HIGH


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

    def test_select_model_auto_detection(self):
        """Auto-detect complexity and select model."""
        optimizer = CostOptimizer()

        selection = optimizer.select_model("Format this file")
        assert selection.tier == ClaudeModelTier.HAIKU
        assert "Auto-detected" in selection.reason

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
        selection = optimizer.select_model("Review this code")
        assert selection.estimated_cost is not None
        assert selection.estimated_cost >= 0

    def test_select_model_includes_fallback(self):
        """Model selection includes fallback suggestion."""
        optimizer = CostOptimizer()
        selection = optimizer.select_model(
            "Review code",
            complexity=TaskComplexity.MEDIUM,
        )
        # Sonnet should have Haiku as fallback
        assert selection.fallback_model is not None
        assert "haiku" in selection.fallback_model.lower()


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


class TestModelDowngrade:
    """Tests for model downgrade suggestions."""

    def test_downgrade_opus_to_sonnet(self):
        """Suggest Sonnet when Opus fails."""
        optimizer = CostOptimizer()
        suggestion = optimizer.suggest_model_downgrade(
            "claude-opus-4-20250514",
            error_reason="Rate limited",
        )
        assert suggestion is not None
        assert "sonnet" in suggestion.lower()

    def test_downgrade_sonnet_to_haiku(self):
        """Suggest Haiku when Sonnet fails."""
        optimizer = CostOptimizer()
        suggestion = optimizer.suggest_model_downgrade(
            "claude-sonnet-4-20250514",
            error_reason="Budget exceeded",
        )
        assert suggestion is not None
        assert "haiku" in suggestion.lower()

    def test_no_downgrade_from_haiku(self):
        """No downgrade suggestion from Haiku."""
        optimizer = CostOptimizer()
        suggestion = optimizer.suggest_model_downgrade(
            "claude-3-5-haiku-20241022",
            error_reason="Rate limited",
        )
        assert suggestion is None

    def test_no_downgrade_unknown_model(self):
        """No downgrade for unknown model."""
        optimizer = CostOptimizer()
        suggestion = optimizer.suggest_model_downgrade(
            "unknown-model",
            error_reason="Error",
        )
        assert suggestion is None


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

        # 1. Select model
        selection = optimizer.select_model(
            "Review this pull request for security issues"
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

        # 4. Check budget
        assert optimizer.check_budget(estimate.estimated_cost) is True

        # 5. Record usage
        optimizer.record_usage(estimate.estimated_cost)

        # 6. Check status
        status = optimizer.get_budget_status()
        assert status["used"] > 0
        assert status["is_exceeded"] is False
