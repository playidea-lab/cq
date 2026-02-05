"""Tests for Extended Thinking and Economic Mode configuration."""

import pytest

from c4.models.config import (
    PRESET_CONFIGS,
    ContextCompressionConfig,
    EconomicModeConfig,
    ExtendedThinkingConfig,
    ModelRoutingConfig,
)


class TestModelRoutingConfig:
    """Tests for ModelRoutingConfig."""

    def test_default_values(self) -> None:
        """Test default model routing values."""
        config = ModelRoutingConfig()

        assert config.implementation == "sonnet"
        assert config.review == "sonnet"
        assert config.checkpoint == "sonnet"
        assert config.scout == "haiku"
        assert config.debug == "haiku"
        assert config.planning == "sonnet"

    def test_custom_values(self) -> None:
        """Test custom model routing values."""
        config = ModelRoutingConfig(
            implementation="opus",
            review="opus",
            checkpoint="opus",
            scout="sonnet",
            debug="sonnet",
            planning="opus",
        )

        assert config.implementation == "opus"
        assert config.review == "opus"
        assert config.scout == "sonnet"


class TestExtendedThinkingConfig:
    """Tests for ExtendedThinkingConfig."""

    def test_default_values(self) -> None:
        """Test default extended thinking values."""
        config = ExtendedThinkingConfig()

        assert config.enabled is False
        assert config.budget_tokens == 10000
        assert config.task_types == ["review", "checkpoint", "planning"]

    def test_enabled_config(self) -> None:
        """Test enabled extended thinking config."""
        config = ExtendedThinkingConfig(
            enabled=True,
            budget_tokens=20000,
            task_types=["review", "checkpoint"],
        )

        assert config.enabled is True
        assert config.budget_tokens == 20000
        assert "review" in config.task_types
        assert "planning" not in config.task_types

    def test_budget_tokens_validation(self) -> None:
        """Test budget_tokens range validation."""
        # Valid minimum
        config = ExtendedThinkingConfig(budget_tokens=1000)
        assert config.budget_tokens == 1000

        # Valid maximum
        config = ExtendedThinkingConfig(budget_tokens=100000)
        assert config.budget_tokens == 100000

        # Below minimum should fail
        with pytest.raises(ValueError):
            ExtendedThinkingConfig(budget_tokens=500)

        # Above maximum should fail
        with pytest.raises(ValueError):
            ExtendedThinkingConfig(budget_tokens=200000)


class TestContextCompressionConfig:
    """Tests for ContextCompressionConfig."""

    def test_default_values(self) -> None:
        """Test default context compression values."""
        config = ContextCompressionConfig()

        assert config.enabled is False
        assert config.max_context_tokens == 50000
        assert config.scout_budget == 5000
        assert config.compression_ratio == 0.3


class TestPresetConfigs:
    """Tests for PRESET_CONFIGS dictionary."""

    def test_all_presets_exist(self) -> None:
        """Test that all expected presets exist."""
        expected_presets = [
            "standard",
            "economic",
            "ultra-economic",
            "quality",
            "economic-thinking",
        ]

        for preset in expected_presets:
            assert preset in PRESET_CONFIGS, f"Missing preset: {preset}"

    def test_standard_preset(self) -> None:
        """Test standard preset configuration."""
        preset = PRESET_CONFIGS["standard"]

        assert preset["implementation"] == "sonnet"
        assert preset["review"] == "opus"
        assert preset["checkpoint"] == "opus"
        assert preset["scout"] == "haiku"

    def test_economic_preset(self) -> None:
        """Test economic preset configuration."""
        preset = PRESET_CONFIGS["economic"]

        assert preset["implementation"] == "sonnet"
        assert preset["review"] == "sonnet"
        assert preset["checkpoint"] == "sonnet"
        assert preset["scout"] == "haiku"

    def test_ultra_economic_preset(self) -> None:
        """Test ultra-economic preset configuration."""
        preset = PRESET_CONFIGS["ultra-economic"]

        assert preset["implementation"] == "haiku"
        assert preset["review"] == "sonnet"
        assert preset["checkpoint"] == "sonnet"

    def test_quality_preset(self) -> None:
        """Test quality preset configuration."""
        preset = PRESET_CONFIGS["quality"]

        assert preset["implementation"] == "opus"
        assert preset["review"] == "opus"
        assert preset["checkpoint"] == "opus"
        assert preset["scout"] == "sonnet"

    def test_economic_thinking_preset(self) -> None:
        """Test economic-thinking preset configuration."""
        preset = PRESET_CONFIGS["economic-thinking"]

        assert preset["implementation"] == "haiku"
        assert preset["review"] == "sonnet"
        assert preset["checkpoint"] == "sonnet"
        assert preset["planning"] == "sonnet"


class TestEconomicModeConfig:
    """Tests for EconomicModeConfig."""

    def test_default_values(self) -> None:
        """Test default economic mode values."""
        config = EconomicModeConfig()

        assert config.enabled is False
        assert config.preset == "economic"
        assert isinstance(config.model_routing, ModelRoutingConfig)
        assert isinstance(config.extended_thinking, ExtendedThinkingConfig)
        assert isinstance(config.context_compression, ContextCompressionConfig)

    def test_enabled_config(self) -> None:
        """Test enabled economic mode config."""
        config = EconomicModeConfig(
            enabled=True,
            preset="economic-thinking",
        )

        assert config.enabled is True
        assert config.preset == "economic-thinking"

    def test_get_model_for_task_type(self) -> None:
        """Test get_model_for_task_type method."""
        config = EconomicModeConfig(
            model_routing=ModelRoutingConfig(
                implementation="haiku",
                review="sonnet",
                checkpoint="opus",
            )
        )

        assert config.get_model_for_task_type("implementation") == "haiku"
        assert config.get_model_for_task_type("review") == "sonnet"
        assert config.get_model_for_task_type("checkpoint") == "opus"
        # Unknown type should return sonnet (default)
        assert config.get_model_for_task_type("unknown") == "sonnet"

    def test_should_use_extended_thinking_disabled(self) -> None:
        """Test should_use_extended_thinking when disabled."""
        config = EconomicModeConfig(
            extended_thinking=ExtendedThinkingConfig(enabled=False)
        )

        assert config.should_use_extended_thinking("review") is False
        assert config.should_use_extended_thinking("checkpoint") is False

    def test_should_use_extended_thinking_enabled(self) -> None:
        """Test should_use_extended_thinking when enabled."""
        config = EconomicModeConfig(
            extended_thinking=ExtendedThinkingConfig(
                enabled=True,
                task_types=["review", "checkpoint"],
            )
        )

        assert config.should_use_extended_thinking("review") is True
        assert config.should_use_extended_thinking("checkpoint") is True
        assert config.should_use_extended_thinking("implementation") is False
        assert config.should_use_extended_thinking("planning") is False

    def test_get_thinking_budget(self) -> None:
        """Test get_thinking_budget method."""
        config = EconomicModeConfig(
            extended_thinking=ExtendedThinkingConfig(budget_tokens=15000)
        )

        assert config.get_thinking_budget() == 15000

    def test_from_preset_economic(self) -> None:
        """Test from_preset factory method with economic preset."""
        config = EconomicModeConfig.from_preset("economic")

        assert config.enabled is True
        assert config.preset == "economic"
        assert config.model_routing.implementation == "sonnet"
        assert config.model_routing.review == "sonnet"
        assert config.extended_thinking.enabled is False

    def test_from_preset_economic_thinking(self) -> None:
        """Test from_preset factory method with economic-thinking preset."""
        config = EconomicModeConfig.from_preset("economic-thinking")

        assert config.enabled is True
        assert config.preset == "economic-thinking"
        assert config.model_routing.implementation == "haiku"
        assert config.model_routing.review == "sonnet"
        assert config.extended_thinking.enabled is True

    def test_from_preset_invalid(self) -> None:
        """Test from_preset with invalid preset name."""
        with pytest.raises(ValueError, match="Unknown preset"):
            EconomicModeConfig.from_preset("invalid-preset")

    def test_preset_pattern_validation(self) -> None:
        """Test preset pattern validation."""
        # Valid presets
        for preset in ["standard", "economic", "ultra-economic", "quality", "economic-thinking"]:
            config = EconomicModeConfig(preset=preset)
            assert config.preset == preset

        # Invalid preset should fail
        with pytest.raises(ValueError):
            EconomicModeConfig(preset="invalid")


class TestEconomicModeIntegration:
    """Integration tests for economic mode with C4Config."""

    def test_economic_mode_in_c4config(self) -> None:
        """Test that economic_mode is properly integrated in C4Config."""
        from c4.models.config import C4Config

        config = C4Config(
            project_id="test",
            economic_mode=EconomicModeConfig(
                enabled=True,
                preset="economic-thinking",
                extended_thinking=ExtendedThinkingConfig(
                    enabled=True,
                    budget_tokens=20000,
                ),
            ),
        )

        assert config.economic_mode.enabled is True
        assert config.economic_mode.preset == "economic-thinking"
        assert config.economic_mode.extended_thinking.enabled is True
        assert config.economic_mode.extended_thinking.budget_tokens == 20000

    def test_economic_mode_yaml_parsing(self) -> None:
        """Test parsing economic mode from YAML-like dict."""
        from c4.models.config import C4Config

        yaml_data = {
            "project_id": "test",
            "economic_mode": {
                "enabled": True,
                "preset": "economic",
                "model_routing": {
                    "implementation": "sonnet",
                    "review": "sonnet",
                    "checkpoint": "sonnet",
                    "scout": "haiku",
                    "debug": "haiku",
                },
                "extended_thinking": {
                    "enabled": False,
                    "budget_tokens": 10000,
                    "task_types": ["review", "checkpoint"],
                },
            },
        }

        config = C4Config(**yaml_data)

        assert config.economic_mode.enabled is True
        assert config.economic_mode.preset == "economic"
        assert config.economic_mode.model_routing.implementation == "sonnet"
        assert config.economic_mode.extended_thinking.enabled is False
