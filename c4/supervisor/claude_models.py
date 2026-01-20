"""Claude Model Presets and Auto-detection.

Provides optimized configurations for Claude models and automatic
API key detection from environment variables.

Supported models:
- claude-sonnet-4-20250514 (Claude Sonnet 4 - Latest, recommended)
- claude-opus-4-20250514 (Claude Opus 4 - Most capable)
- claude-3-5-sonnet-20241022 (Claude 3.5 Sonnet - Fast and capable)
- claude-3-5-haiku-20241022 (Claude 3.5 Haiku - Fastest, cheapest)
- claude-3-opus-20240229 (Claude 3 Opus - Legacy)
- claude-3-sonnet-20240229 (Claude 3 Sonnet - Legacy)
- claude-3-haiku-20240307 (Claude 3 Haiku - Legacy)
"""

import logging
import os
from dataclasses import dataclass
from enum import Enum

logger = logging.getLogger(__name__)


class ClaudeModelTier(str, Enum):
    """Claude model capability tiers."""

    OPUS = "opus"  # Most capable, highest cost
    SONNET = "sonnet"  # Balanced capability and cost
    HAIKU = "haiku"  # Fastest, lowest cost


@dataclass(frozen=True)
class ClaudeModelPreset:
    """Preset configuration for a Claude model."""

    model_id: str
    tier: ClaudeModelTier
    display_name: str
    max_output_tokens: int
    context_window: int
    input_cost_per_1m: float  # USD per 1M input tokens
    output_cost_per_1m: float  # USD per 1M output tokens
    supports_vision: bool = True
    supports_tool_use: bool = True
    is_latest: bool = False


# Claude model presets with pricing (as of 2025-01)
CLAUDE_MODELS: dict[str, ClaudeModelPreset] = {
    # Claude 4 models (Latest)
    "claude-sonnet-4-20250514": ClaudeModelPreset(
        model_id="claude-sonnet-4-20250514",
        tier=ClaudeModelTier.SONNET,
        display_name="Claude Sonnet 4",
        max_output_tokens=16384,
        context_window=200000,
        input_cost_per_1m=3.0,
        output_cost_per_1m=15.0,
        is_latest=True,
    ),
    "claude-opus-4-20250514": ClaudeModelPreset(
        model_id="claude-opus-4-20250514",
        tier=ClaudeModelTier.OPUS,
        display_name="Claude Opus 4",
        max_output_tokens=16384,
        context_window=200000,
        input_cost_per_1m=15.0,
        output_cost_per_1m=75.0,
        is_latest=True,
    ),
    # Claude 3.5 models
    "claude-3-5-sonnet-20241022": ClaudeModelPreset(
        model_id="claude-3-5-sonnet-20241022",
        tier=ClaudeModelTier.SONNET,
        display_name="Claude 3.5 Sonnet",
        max_output_tokens=8192,
        context_window=200000,
        input_cost_per_1m=3.0,
        output_cost_per_1m=15.0,
    ),
    "claude-3-5-haiku-20241022": ClaudeModelPreset(
        model_id="claude-3-5-haiku-20241022",
        tier=ClaudeModelTier.HAIKU,
        display_name="Claude 3.5 Haiku",
        max_output_tokens=8192,
        context_window=200000,
        input_cost_per_1m=0.80,
        output_cost_per_1m=4.0,
        is_latest=True,
    ),
    # Claude 3 models (Legacy)
    "claude-3-opus-20240229": ClaudeModelPreset(
        model_id="claude-3-opus-20240229",
        tier=ClaudeModelTier.OPUS,
        display_name="Claude 3 Opus",
        max_output_tokens=4096,
        context_window=200000,
        input_cost_per_1m=15.0,
        output_cost_per_1m=75.0,
    ),
    "claude-3-sonnet-20240229": ClaudeModelPreset(
        model_id="claude-3-sonnet-20240229",
        tier=ClaudeModelTier.SONNET,
        display_name="Claude 3 Sonnet",
        max_output_tokens=4096,
        context_window=200000,
        input_cost_per_1m=3.0,
        output_cost_per_1m=15.0,
    ),
    "claude-3-haiku-20240307": ClaudeModelPreset(
        model_id="claude-3-haiku-20240307",
        tier=ClaudeModelTier.HAIKU,
        display_name="Claude 3 Haiku",
        max_output_tokens=4096,
        context_window=200000,
        input_cost_per_1m=0.25,
        output_cost_per_1m=1.25,
    ),
}

# Aliases for convenience
MODEL_ALIASES: dict[str, str] = {
    # Latest versions
    "claude-sonnet-4": "claude-sonnet-4-20250514",
    "claude-opus-4": "claude-opus-4-20250514",
    "claude-4-sonnet": "claude-sonnet-4-20250514",
    "claude-4-opus": "claude-opus-4-20250514",
    # 3.5 aliases
    "claude-3.5-sonnet": "claude-3-5-sonnet-20241022",
    "claude-3.5-haiku": "claude-3-5-haiku-20241022",
    "claude-3-5-sonnet": "claude-3-5-sonnet-20241022",
    "claude-3-5-haiku": "claude-3-5-haiku-20241022",
    # 3.0 aliases
    "claude-3-opus": "claude-3-opus-20240229",
    "claude-3-sonnet": "claude-3-sonnet-20240229",
    "claude-3-haiku": "claude-3-haiku-20240307",
    # Tier aliases (resolve to latest in tier)
    "opus": "claude-opus-4-20250514",
    "sonnet": "claude-sonnet-4-20250514",
    "haiku": "claude-3-5-haiku-20241022",
}

# Default model for supervisor
DEFAULT_CLAUDE_MODEL = "claude-sonnet-4-20250514"

# Environment variable names for API keys
ANTHROPIC_API_KEY_ENV_VARS = [
    "ANTHROPIC_API_KEY",
    "CLAUDE_API_KEY",
]


def resolve_model_id(model: str) -> str:
    """Resolve model alias to full model ID.

    Args:
        model: Model ID or alias

    Returns:
        Full model ID

    Examples:
        >>> resolve_model_id("sonnet")
        'claude-sonnet-4-20250514'
        >>> resolve_model_id("claude-3-opus")
        'claude-3-opus-20240229'
    """
    # Check if it's an alias
    if model in MODEL_ALIASES:
        return MODEL_ALIASES[model]

    # Check if it's already a full model ID
    if model in CLAUDE_MODELS:
        return model

    # Return as-is (might be a custom/newer model)
    return model


def get_model_preset(model: str) -> ClaudeModelPreset | None:
    """Get preset configuration for a Claude model.

    Args:
        model: Model ID or alias

    Returns:
        Model preset if found, None otherwise
    """
    resolved = resolve_model_id(model)
    return CLAUDE_MODELS.get(resolved)


def is_claude_model(model: str) -> bool:
    """Check if a model ID is a Claude model.

    Args:
        model: Model ID to check

    Returns:
        True if it's a recognized Claude model
    """
    resolved = resolve_model_id(model)
    return resolved in CLAUDE_MODELS or resolved.startswith("claude-")


def validate_model_id(model: str) -> tuple[bool, str | None]:
    """Validate a Claude model ID.

    Args:
        model: Model ID to validate

    Returns:
        Tuple of (is_valid, error_message)
    """
    resolved = resolve_model_id(model)

    # Known model - valid
    if resolved in CLAUDE_MODELS:
        return True, None

    # Unknown but looks like Claude - warn but allow
    if resolved.startswith("claude-"):
        logger.warning(
            f"Model '{resolved}' is not a known Claude model. "
            f"Known models: {', '.join(CLAUDE_MODELS.keys())}"
        )
        return True, None

    # Not a Claude model
    return False, f"'{model}' is not a Claude model"


def detect_anthropic_api_key() -> str | None:
    """Auto-detect Anthropic API key from environment variables.

    Checks multiple environment variable names in order of priority.

    Returns:
        API key if found, None otherwise
    """
    for env_var in ANTHROPIC_API_KEY_ENV_VARS:
        api_key = os.environ.get(env_var)
        if api_key:
            # Validate key format (should start with sk-ant-)
            if api_key.startswith("sk-ant-"):
                logger.debug(f"Found Anthropic API key in {env_var}")
                return api_key
            else:
                logger.warning(
                    f"{env_var} doesn't look like a valid Anthropic API key "
                    f"(should start with 'sk-ant-')"
                )
    return None


def get_api_key(api_key_env: str | None = None) -> str | None:
    """Get Anthropic API key from specified env var or auto-detect.

    Args:
        api_key_env: Specific environment variable name to use

    Returns:
        API key if found, None otherwise
    """
    # If specific env var is provided, use it
    if api_key_env:
        api_key = os.environ.get(api_key_env)
        if api_key:
            return api_key
        logger.warning(f"Environment variable '{api_key_env}' not set")
        return None

    # Otherwise, auto-detect
    return detect_anthropic_api_key()


def estimate_cost(
    model: str,
    input_tokens: int,
    output_tokens: int,
) -> float | None:
    """Estimate cost for a Claude API call.

    Args:
        model: Model ID or alias
        input_tokens: Number of input tokens
        output_tokens: Number of output tokens

    Returns:
        Estimated cost in USD, or None if model not found
    """
    preset = get_model_preset(model)
    if not preset:
        return None

    input_cost = (input_tokens / 1_000_000) * preset.input_cost_per_1m
    output_cost = (output_tokens / 1_000_000) * preset.output_cost_per_1m

    return input_cost + output_cost


def get_recommended_model(task_complexity: str = "medium") -> str:
    """Get recommended Claude model based on task complexity.

    Args:
        task_complexity: "low", "medium", or "high"

    Returns:
        Recommended model ID
    """
    if task_complexity == "low":
        return "claude-3-5-haiku-20241022"  # Fastest, cheapest
    elif task_complexity == "high":
        return "claude-opus-4-20250514"  # Most capable
    else:
        return DEFAULT_CLAUDE_MODEL  # Balanced


def list_available_models() -> list[dict]:
    """List all available Claude models with their configurations.

    Returns:
        List of model info dictionaries
    """
    models = []
    for model_id, preset in CLAUDE_MODELS.items():
        models.append(
            {
                "model_id": model_id,
                "display_name": preset.display_name,
                "tier": preset.tier.value,
                "max_output_tokens": preset.max_output_tokens,
                "context_window": preset.context_window,
                "input_cost_per_1m": preset.input_cost_per_1m,
                "output_cost_per_1m": preset.output_cost_per_1m,
                "is_latest": preset.is_latest,
            }
        )
    return sorted(models, key=lambda x: (not x["is_latest"], x["tier"], x["model_id"]))
