"""Unit tests for Claude model presets and auto-detection."""

import pytest

from c4.supervisor.claude_models import (
    CLAUDE_MODELS,
    DEFAULT_CLAUDE_MODEL,
    MODEL_ALIASES,
    ClaudeModelPreset,
    ClaudeModelTier,
    detect_anthropic_api_key,
    estimate_cost,
    get_api_key,
    get_model_preset,
    get_recommended_model,
    is_claude_model,
    list_available_models,
    resolve_model_id,
    validate_model_id,
)


class TestClaudeModelTier:
    """Tests for ClaudeModelTier enum."""

    def test_tier_values(self):
        """Test tier enum values."""
        assert ClaudeModelTier.OPUS.value == "opus"
        assert ClaudeModelTier.SONNET.value == "sonnet"
        assert ClaudeModelTier.HAIKU.value == "haiku"

    def test_tier_members(self):
        """Test all tier members exist."""
        assert len(ClaudeModelTier) == 3


class TestClaudeModelPreset:
    """Tests for ClaudeModelPreset dataclass."""

    def test_preset_creation(self):
        """Test creating a preset."""
        preset = ClaudeModelPreset(
            model_id="test-model",
            tier=ClaudeModelTier.SONNET,
            display_name="Test Model",
            max_output_tokens=4096,
            context_window=100000,
            input_cost_per_1m=1.0,
            output_cost_per_1m=5.0,
        )
        assert preset.model_id == "test-model"
        assert preset.tier == ClaudeModelTier.SONNET
        assert preset.max_output_tokens == 4096

    def test_preset_immutable(self):
        """Test that preset is immutable."""
        preset = CLAUDE_MODELS["claude-sonnet-4-20250514"]
        with pytest.raises(Exception):  # FrozenInstanceError
            preset.model_id = "modified"  # type: ignore


class TestModelRegistry:
    """Tests for CLAUDE_MODELS registry."""

    def test_claude_4_models_exist(self):
        """Test Claude 4 models are registered."""
        assert "claude-sonnet-4-20250514" in CLAUDE_MODELS
        assert "claude-opus-4-20250514" in CLAUDE_MODELS

    def test_claude_3_5_models_exist(self):
        """Test Claude 3.5 models are registered."""
        assert "claude-3-5-sonnet-20241022" in CLAUDE_MODELS
        assert "claude-3-5-haiku-20241022" in CLAUDE_MODELS

    def test_claude_3_models_exist(self):
        """Test Claude 3 legacy models are registered."""
        assert "claude-3-opus-20240229" in CLAUDE_MODELS
        assert "claude-3-sonnet-20240229" in CLAUDE_MODELS
        assert "claude-3-haiku-20240307" in CLAUDE_MODELS

    def test_all_models_have_required_fields(self):
        """Test all models have required fields."""
        for model_id, preset in CLAUDE_MODELS.items():
            assert preset.model_id == model_id
            assert preset.tier in ClaudeModelTier
            assert preset.display_name
            assert preset.max_output_tokens > 0
            assert preset.context_window > 0
            assert preset.input_cost_per_1m >= 0
            assert preset.output_cost_per_1m >= 0

    def test_latest_models_marked(self):
        """Test that latest models are marked."""
        latest_models = [m for m in CLAUDE_MODELS.values() if m.is_latest]
        assert len(latest_models) >= 2  # At least Opus 4 and Sonnet 4


class TestModelAliases:
    """Tests for MODEL_ALIASES."""

    def test_tier_aliases(self):
        """Test tier aliases resolve correctly."""
        assert MODEL_ALIASES["opus"] == "claude-opus-4-20250514"
        assert MODEL_ALIASES["sonnet"] == "claude-sonnet-4-20250514"
        assert MODEL_ALIASES["haiku"] == "claude-3-5-haiku-20241022"

    def test_short_aliases(self):
        """Test short aliases resolve correctly."""
        assert MODEL_ALIASES["claude-3-opus"] == "claude-3-opus-20240229"
        assert MODEL_ALIASES["claude-3.5-sonnet"] == "claude-3-5-sonnet-20241022"

    def test_all_aliases_point_to_valid_models(self):
        """Test all aliases point to valid models."""
        for alias, model_id in MODEL_ALIASES.items():
            assert model_id in CLAUDE_MODELS, f"Alias '{alias}' points to unknown model"


class TestResolveModelId:
    """Tests for resolve_model_id function."""

    def test_resolve_alias(self):
        """Test resolving aliases."""
        assert resolve_model_id("sonnet") == "claude-sonnet-4-20250514"
        assert resolve_model_id("opus") == "claude-opus-4-20250514"
        assert resolve_model_id("haiku") == "claude-3-5-haiku-20241022"

    def test_resolve_full_id(self):
        """Test full IDs are returned as-is."""
        assert resolve_model_id("claude-sonnet-4-20250514") == "claude-sonnet-4-20250514"

    def test_resolve_unknown(self):
        """Test unknown models are returned as-is."""
        assert resolve_model_id("unknown-model") == "unknown-model"


class TestGetModelPreset:
    """Tests for get_model_preset function."""

    def test_get_by_full_id(self):
        """Test getting preset by full ID."""
        preset = get_model_preset("claude-sonnet-4-20250514")
        assert preset is not None
        assert preset.model_id == "claude-sonnet-4-20250514"

    def test_get_by_alias(self):
        """Test getting preset by alias."""
        preset = get_model_preset("sonnet")
        assert preset is not None
        assert preset.model_id == "claude-sonnet-4-20250514"

    def test_get_unknown_returns_none(self):
        """Test getting unknown model returns None."""
        assert get_model_preset("unknown-model") is None


class TestIsClaudeModel:
    """Tests for is_claude_model function."""

    def test_known_models(self):
        """Test known Claude models are recognized."""
        assert is_claude_model("claude-sonnet-4-20250514")
        assert is_claude_model("claude-3-opus-20240229")

    def test_aliases(self):
        """Test aliases are recognized."""
        assert is_claude_model("sonnet")
        assert is_claude_model("opus")

    def test_unknown_claude_prefix(self):
        """Test unknown claude-* models are recognized."""
        assert is_claude_model("claude-future-model")

    def test_non_claude_models(self):
        """Test non-Claude models are not recognized."""
        assert not is_claude_model("gpt-4o")
        assert not is_claude_model("llama-3")


class TestValidateModelId:
    """Tests for validate_model_id function."""

    def test_known_model_valid(self):
        """Test known models are valid."""
        is_valid, error = validate_model_id("claude-sonnet-4-20250514")
        assert is_valid
        assert error is None

    def test_alias_valid(self):
        """Test aliases are valid."""
        is_valid, error = validate_model_id("sonnet")
        assert is_valid
        assert error is None

    def test_unknown_claude_valid_with_warning(self):
        """Test unknown claude-* models are valid (with warning)."""
        is_valid, error = validate_model_id("claude-future-model")
        assert is_valid
        assert error is None

    def test_non_claude_invalid(self):
        """Test non-Claude models are invalid."""
        is_valid, error = validate_model_id("gpt-4o")
        assert not is_valid
        assert error is not None


class TestDetectAnthropicApiKey:
    """Tests for detect_anthropic_api_key function."""

    def test_detect_from_anthropic_api_key(self, monkeypatch):
        """Test detecting from ANTHROPIC_API_KEY."""
        monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-test123")
        assert detect_anthropic_api_key() == "sk-ant-test123"

    def test_detect_from_claude_api_key(self, monkeypatch):
        """Test detecting from CLAUDE_API_KEY."""
        monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
        monkeypatch.setenv("CLAUDE_API_KEY", "sk-ant-test456")
        assert detect_anthropic_api_key() == "sk-ant-test456"

    def test_no_key_returns_none(self, monkeypatch):
        """Test no key returns None."""
        monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
        monkeypatch.delenv("CLAUDE_API_KEY", raising=False)
        assert detect_anthropic_api_key() is None

    def test_invalid_format_returns_none(self, monkeypatch):
        """Test invalid format returns None."""
        monkeypatch.setenv("ANTHROPIC_API_KEY", "invalid-key")
        assert detect_anthropic_api_key() is None


class TestGetApiKey:
    """Tests for get_api_key function."""

    def test_specific_env_var(self, monkeypatch):
        """Test getting from specific env var."""
        monkeypatch.setenv("CUSTOM_API_KEY", "sk-custom")
        assert get_api_key("CUSTOM_API_KEY") == "sk-custom"

    def test_auto_detect(self, monkeypatch):
        """Test auto-detection."""
        monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-auto")
        assert get_api_key() == "sk-ant-auto"

    def test_missing_specific_returns_none(self, monkeypatch):
        """Test missing specific env var returns None."""
        monkeypatch.delenv("MISSING_KEY", raising=False)
        assert get_api_key("MISSING_KEY") is None


class TestEstimateCost:
    """Tests for estimate_cost function."""

    def test_sonnet_cost(self):
        """Test cost estimation for Sonnet."""
        cost = estimate_cost("claude-sonnet-4-20250514", 1000, 500)
        assert cost is not None
        # $3/1M input + $15/1M output
        expected = (1000 / 1_000_000) * 3.0 + (500 / 1_000_000) * 15.0
        assert abs(cost - expected) < 0.0001

    def test_haiku_cost(self):
        """Test cost estimation for Haiku (cheapest)."""
        cost = estimate_cost("claude-3-5-haiku-20241022", 1000, 500)
        assert cost is not None
        # $0.80/1M input + $4/1M output
        expected = (1000 / 1_000_000) * 0.80 + (500 / 1_000_000) * 4.0
        assert abs(cost - expected) < 0.0001

    def test_opus_cost(self):
        """Test cost estimation for Opus (most expensive)."""
        cost = estimate_cost("claude-opus-4-20250514", 1000, 500)
        assert cost is not None
        # $15/1M input + $75/1M output
        expected = (1000 / 1_000_000) * 15.0 + (500 / 1_000_000) * 75.0
        assert abs(cost - expected) < 0.0001

    def test_unknown_model_returns_none(self):
        """Test unknown model returns None."""
        assert estimate_cost("unknown-model", 1000, 500) is None


class TestGetRecommendedModel:
    """Tests for get_recommended_model function."""

    def test_low_complexity(self):
        """Test low complexity recommends Haiku."""
        model = get_recommended_model("low")
        assert "haiku" in model

    def test_medium_complexity(self):
        """Test medium complexity recommends default (Sonnet)."""
        model = get_recommended_model("medium")
        assert model == DEFAULT_CLAUDE_MODEL

    def test_high_complexity(self):
        """Test high complexity recommends Opus."""
        model = get_recommended_model("high")
        assert "opus" in model


class TestListAvailableModels:
    """Tests for list_available_models function."""

    def test_returns_list(self):
        """Test returns a list."""
        models = list_available_models()
        assert isinstance(models, list)
        assert len(models) == len(CLAUDE_MODELS)

    def test_model_info_structure(self):
        """Test model info has required fields."""
        models = list_available_models()
        for model in models:
            assert "model_id" in model
            assert "display_name" in model
            assert "tier" in model
            assert "max_output_tokens" in model
            assert "input_cost_per_1m" in model
            assert "output_cost_per_1m" in model
            assert "is_latest" in model

    def test_sorted_by_latest_first(self):
        """Test latest models come first."""
        models = list_available_models()
        # First models should be latest
        assert models[0]["is_latest"] or models[1]["is_latest"]


class TestDefaultClaudeModel:
    """Tests for DEFAULT_CLAUDE_MODEL."""

    def test_default_is_valid(self):
        """Test default model is valid."""
        assert DEFAULT_CLAUDE_MODEL in CLAUDE_MODELS

    def test_default_is_sonnet(self):
        """Test default is Sonnet tier (balanced)."""
        preset = CLAUDE_MODELS[DEFAULT_CLAUDE_MODEL]
        assert preset.tier == ClaudeModelTier.SONNET
