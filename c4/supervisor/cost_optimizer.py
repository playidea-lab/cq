"""Cost Optimizer - Intelligent model selection and token management.

Provides cost optimization strategies including:
- Complexity-based model selection (haiku/sonnet/opus)
- Token limits and budget constraints
- Prompt caching hints for repeated content

NOTE: Complexity analysis should be done during Discovery phase using LLM,
not pattern matching. The complexity value should be stored in task spec
and passed to select_model() directly.

Example:
    >>> optimizer = CostOptimizer()
    >>> # Complexity comes from Discovery phase (LLM-analyzed)
    >>> model = optimizer.select_model(prompt, complexity="medium")
    >>> optimized = optimizer.optimize_prompt(prompt, max_tokens=4000)
"""

import hashlib
import logging
import re
from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from .claude_models import (
    CLAUDE_MODELS,
    ClaudeModelTier,
    estimate_cost,
    get_model_preset,
)

logger = logging.getLogger(__name__)


class TaskComplexity(str, Enum):
    """Task complexity levels for model selection.

    These values should be determined during Discovery phase
    by LLM analysis, not runtime pattern matching.
    """

    LOW = "low"  # Simple tasks: formatting, basic Q&A
    MEDIUM = "medium"  # Standard tasks: code review, summaries
    HIGH = "high"  # Complex tasks: architecture, deep analysis


class CostAlert(str, Enum):
    """Types of cost-related alerts for user notification."""

    BUDGET_WARNING = "budget_warning"  # Approaching budget limit
    BUDGET_EXCEEDED = "budget_exceeded"  # Budget limit reached
    MODEL_UNAVAILABLE = "model_unavailable"  # Requested model not available
    RATE_LIMITED = "rate_limited"  # Hit rate limit


@dataclass
class CostAlertInfo:
    """Information about a cost alert."""

    alert_type: CostAlert
    message: str
    details: dict = field(default_factory=dict)


@dataclass
class ModelSelection:
    """Result of model selection."""

    model_id: str
    tier: ClaudeModelTier
    reason: str
    estimated_cost: float | None = None


@dataclass
class OptimizedPrompt:
    """Result of prompt optimization."""

    content: str
    original_length: int
    optimized_length: int
    tokens_saved_estimate: int
    cache_hints: list[str] = field(default_factory=list)
    truncated: bool = False


@dataclass
class CostEstimate:
    """Estimated cost for a request."""

    model: str
    input_tokens: int
    output_tokens_estimate: int
    total_tokens: int
    estimated_cost: float
    budget_percentage: float | None = None


# Model selection by complexity
COMPLEXITY_TO_MODEL = {
    TaskComplexity.LOW: "claude-3-5-haiku-20241022",  # Fast, cheap
    TaskComplexity.MEDIUM: "claude-sonnet-4-20250514",  # Balanced
    TaskComplexity.HIGH: "claude-sonnet-4-20250514",  # Smart (opus for critical)
}

# Default token limits by use case
DEFAULT_TOKEN_LIMITS = {
    "review": 8000,
    "summary": 4000,
    "analysis": 12000,
    "implementation": 16000,
    "default": 8000,
}


class CostOptimizer:
    """Optimizer for Claude API costs.

    Provides intelligent model selection, token management,
    and prompt optimization strategies.

    Attributes:
        budget: Optional session budget in USD
        prefer_cost_savings: Prioritize cost over capability
        cache_prompts: Enable prompt caching hints

    Example:
        >>> optimizer = CostOptimizer(budget=10.0)
        >>> model = optimizer.select_model(prompt)
        >>> print(f"Selected: {model.model_id} ({model.reason})")
    """

    def __init__(
        self,
        budget: float | None = None,
        prefer_cost_savings: bool = False,
        cache_prompts: bool = True,
        default_model: str = "claude-sonnet-4-20250514",
    ):
        """Initialize cost optimizer.

        Args:
            budget: Optional session budget in USD
            prefer_cost_savings: If True, prefer cheaper models
            cache_prompts: Enable prompt caching optimization
            default_model: Default model when complexity is uncertain
        """
        self.budget = budget
        self.prefer_cost_savings = prefer_cost_savings
        self.cache_prompts = cache_prompts
        self.default_model = default_model

        self._prompt_cache: dict[str, str] = {}
        self._session_cost: float = 0.0

    def select_model(
        self,
        prompt: str,
        complexity: TaskComplexity | str = TaskComplexity.MEDIUM,
        min_tier: ClaudeModelTier | None = None,
        max_tier: ClaudeModelTier | None = None,
    ) -> ModelSelection:
        """Select optimal model based on task complexity.

        NOTE: Complexity should be determined during Discovery phase using LLM,
        not at runtime. Pass the pre-analyzed complexity value here.

        Args:
            prompt: The prompt text (used for token estimation)
            complexity: Task complexity (from Discovery phase LLM analysis)
            min_tier: Minimum model tier to use
            max_tier: Maximum model tier to use

        Returns:
            ModelSelection with chosen model and reasoning
        """
        # Convert string to enum if needed
        if isinstance(complexity, str):
            complexity = TaskComplexity(complexity)

        reason_prefix = f"Complexity: {complexity.value}. "

        # Get base model for complexity
        model_id = COMPLEXITY_TO_MODEL.get(complexity, self.default_model)

        # Apply tier constraints
        preset = get_model_preset(model_id)
        if preset:
            tier = preset.tier

            # Upgrade if below minimum
            if min_tier and self._tier_order(tier) < self._tier_order(min_tier):
                model_id = self._get_model_for_tier(min_tier)
                reason_prefix += f"Upgraded to {min_tier.value} (minimum tier). "

            # Downgrade if above maximum
            if max_tier and self._tier_order(tier) > self._tier_order(max_tier):
                model_id = self._get_model_for_tier(max_tier)
                reason_prefix += f"Capped at {max_tier.value} (maximum tier). "

        # Cost savings preference
        if self.prefer_cost_savings and complexity == TaskComplexity.MEDIUM:
            # Try haiku for medium tasks
            model_id = COMPLEXITY_TO_MODEL[TaskComplexity.LOW]
            reason_prefix += "Cost savings: using cheaper model. "

        # Get final preset and estimate
        final_preset = get_model_preset(model_id)
        tier = final_preset.tier if final_preset else ClaudeModelTier.SONNET

        # Estimate tokens and cost
        input_tokens = self._estimate_tokens(prompt)
        output_tokens = self._estimate_output_tokens(complexity)
        estimated_cost = estimate_cost(model_id, input_tokens, output_tokens)

        display_name = final_preset.display_name if final_preset else model_id
        reason = f"{reason_prefix}Selected {display_name}."

        return ModelSelection(
            model_id=model_id,
            tier=tier,
            reason=reason,
            estimated_cost=estimated_cost,
        )

    def _tier_order(self, tier: ClaudeModelTier) -> int:
        """Get ordering value for tier comparison."""
        return {
            ClaudeModelTier.HAIKU: 1,
            ClaudeModelTier.SONNET: 2,
            ClaudeModelTier.OPUS: 3,
        }.get(tier, 2)

    def _get_model_for_tier(self, tier: ClaudeModelTier) -> str:
        """Get default model ID for a tier."""
        for model_id, preset in CLAUDE_MODELS.items():
            if preset.tier == tier and preset.is_latest:
                return model_id
        return self.default_model

    def _estimate_tokens(self, text: str) -> int:
        """Rough token estimate (4 chars per token)."""
        return len(text) // 4

    def _estimate_output_tokens(self, complexity: TaskComplexity) -> int:
        """Estimate output tokens based on complexity."""
        return {
            TaskComplexity.LOW: 500,
            TaskComplexity.MEDIUM: 2000,
            TaskComplexity.HIGH: 4000,
        }.get(complexity, 2000)

    def optimize_prompt(
        self,
        prompt: str,
        max_tokens: int | None = None,
        use_case: str = "default",
        preserve_structure: bool = True,
    ) -> OptimizedPrompt:
        """Optimize prompt for cost efficiency.

        Args:
            prompt: Original prompt text
            max_tokens: Maximum token limit
            use_case: Use case for default limits
            preserve_structure: Keep important formatting

        Returns:
            OptimizedPrompt with optimized content
        """
        original_length = len(prompt)
        if max_tokens is None:
            max_tokens = DEFAULT_TOKEN_LIMITS.get(use_case, DEFAULT_TOKEN_LIMITS["default"])
        max_chars = max_tokens * 4  # Rough conversion

        cache_hints = []
        truncated = False
        optimized = prompt

        # Remove excessive whitespace
        optimized = re.sub(r"\n{3,}", "\n\n", optimized)
        optimized = re.sub(r" {2,}", " ", optimized)

        # Remove common verbose patterns
        if not preserve_structure:
            # Remove excessive comments
            optimized = re.sub(r"#.*?(\n|$)", "\n", optimized)
            # Remove docstrings (simple pattern)
            optimized = re.sub(r'"""[\s\S]*?"""', "", optimized)
            optimized = re.sub(r"'''[\s\S]*?'''", "", optimized)

        # Truncate if still too long
        if len(optimized) > max_chars:
            truncated = True
            # Keep beginning and end, truncate middle
            keep_start = int(max_chars * 0.6)
            keep_end = int(max_chars * 0.3)
            truncate_msg = f"\n\n... [TRUNCATED {len(optimized) - max_chars} chars] ...\n\n"

            optimized = optimized[:keep_start] + truncate_msg + optimized[-keep_end:]

        # Generate cache hints for repeated content
        if self.cache_prompts:
            cache_hints = self._generate_cache_hints(optimized)

        optimized_length = len(optimized)
        tokens_saved = (original_length - optimized_length) // 4

        return OptimizedPrompt(
            content=optimized,
            original_length=original_length,
            optimized_length=optimized_length,
            tokens_saved_estimate=tokens_saved,
            cache_hints=cache_hints,
            truncated=truncated,
        )

    def _generate_cache_hints(self, content: str) -> list[str]:
        """Generate cache hints for repeated sections."""
        hints = []

        # Hash content sections for caching
        content_hash = hashlib.md5(content[:1000].encode()).hexdigest()[:8]
        hints.append(f"cache_key:{content_hash}")

        # Detect system prompt patterns
        if "you are" in content.lower()[:500]:
            hints.append("system_prompt:cacheable")

        # Detect code blocks
        code_blocks = re.findall(r"```[\s\S]*?```", content)
        if code_blocks:
            hints.append(f"code_blocks:{len(code_blocks)}")

        return hints

    def estimate_cost(
        self,
        prompt: str,
        model: str | None = None,
        expected_output_tokens: int | None = None,
    ) -> CostEstimate:
        """Estimate cost for a request.

        Args:
            prompt: The prompt text
            model: Model ID (uses default if None)
            expected_output_tokens: Expected output tokens

        Returns:
            CostEstimate with breakdown
        """
        model = model or self.default_model
        input_tokens = self._estimate_tokens(prompt)
        output_tokens = expected_output_tokens or 2000

        cost = estimate_cost(model, input_tokens, output_tokens) or 0.0

        budget_pct = None
        if self.budget:
            remaining = self.budget - self._session_cost
            if remaining > 0:
                budget_pct = (cost / remaining) * 100

        return CostEstimate(
            model=model,
            input_tokens=input_tokens,
            output_tokens_estimate=output_tokens,
            total_tokens=input_tokens + output_tokens,
            estimated_cost=cost,
            budget_percentage=budget_pct,
        )

    def record_usage(self, cost: float) -> None:
        """Record usage cost for session tracking.

        Args:
            cost: Cost in USD
        """
        self._session_cost += cost
        logger.debug(f"Session cost updated: ${self._session_cost:.4f}")

    def check_budget(self, estimated_cost: float) -> bool:
        """Check if estimated cost fits within budget.

        Args:
            estimated_cost: Estimated cost for next request

        Returns:
            True if within budget, False otherwise
        """
        if self.budget is None:
            return True

        remaining = self.budget - self._session_cost
        return estimated_cost <= remaining

    def get_budget_status(self) -> dict[str, Any]:
        """Get current budget status.

        Returns:
            Dictionary with budget info
        """
        if self.budget is None:
            return {"budget": None, "used": self._session_cost, "remaining": None}

        remaining = self.budget - self._session_cost
        percentage = (self._session_cost / self.budget) * 100 if self.budget > 0 else 0

        return {
            "budget": self.budget,
            "used": self._session_cost,
            "remaining": remaining,
            "percentage_used": percentage,
            "is_exceeded": remaining < 0,
        }

    def create_budget_alert(
        self,
        estimated_cost: float,
        threshold_percentage: float = 80.0,
    ) -> CostAlertInfo | None:
        """Create alert if budget threshold exceeded.

        Args:
            estimated_cost: Estimated cost for next request
            threshold_percentage: Warning threshold (default 80%)

        Returns:
            CostAlertInfo if alert needed, None otherwise
        """
        if self.budget is None:
            return None

        status = self.get_budget_status()
        remaining = status["remaining"]

        # Budget exceeded
        if remaining < estimated_cost:
            return CostAlertInfo(
                alert_type=CostAlert.BUDGET_EXCEEDED,
                message=f"Budget exceeded. Remaining: ${remaining:.4f}, "
                f"Estimated cost: ${estimated_cost:.4f}",
                details={
                    "budget": self.budget,
                    "used": status["used"],
                    "remaining": remaining,
                    "estimated_cost": estimated_cost,
                },
            )

        # Budget warning (approaching threshold)
        if status["percentage_used"] >= threshold_percentage:
            return CostAlertInfo(
                alert_type=CostAlert.BUDGET_WARNING,
                message=f"Budget warning: {status['percentage_used']:.1f}% used. "
                f"Remaining: ${remaining:.4f}",
                details={
                    "budget": self.budget,
                    "used": status["used"],
                    "remaining": remaining,
                    "percentage_used": status["percentage_used"],
                },
            )

        return None

    def create_model_unavailable_alert(
        self,
        model: str,
        reason: str,
    ) -> CostAlertInfo:
        """Create alert when requested model is unavailable.

        Args:
            model: The unavailable model ID
            reason: Reason for unavailability

        Returns:
            CostAlertInfo for user notification
        """
        return CostAlertInfo(
            alert_type=CostAlert.MODEL_UNAVAILABLE,
            message=f"Model '{model}' is unavailable: {reason}",
            details={
                "model": model,
                "reason": reason,
            },
        )

    def create_rate_limit_alert(
        self,
        model: str,
        retry_after: float | None = None,
    ) -> CostAlertInfo:
        """Create alert when rate limited.

        Args:
            model: The rate-limited model ID
            retry_after: Suggested retry delay in seconds

        Returns:
            CostAlertInfo for user notification
        """
        msg = f"Rate limited on model '{model}'."
        if retry_after:
            msg += f" Retry after {retry_after:.1f}s."

        return CostAlertInfo(
            alert_type=CostAlert.RATE_LIMITED,
            message=msg,
            details={
                "model": model,
                "retry_after": retry_after,
            },
        )


def create_cost_optimizer(
    budget: float | None = None,
    prefer_cost_savings: bool = False,
) -> CostOptimizer:
    """Create a configured cost optimizer.

    Args:
        budget: Optional session budget in USD
        prefer_cost_savings: Prioritize cost over capability

    Returns:
        Configured CostOptimizer
    """
    return CostOptimizer(
        budget=budget,
        prefer_cost_savings=prefer_cost_savings,
    )
