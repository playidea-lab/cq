"""Tests for c4.platforms module - Multi-platform support."""

import os
from pathlib import Path

import pytest
import yaml

from c4.platforms import (
    COMPLEX_COMMANDS,
    PLATFORM_COMMANDS,
    REQUIRED_COMMANDS,
    generate_command_template,
    get_config_info,
    get_default_platform,
    get_platform_command,
    list_platforms,
    set_platform_config,
    setup_platform,
    validate_platform_commands,
)


class TestPlatformConfiguration:
    """Test platform configuration constants."""

    def test_platform_commands_structure(self):
        """Platform commands should be lists of strings."""
        assert isinstance(PLATFORM_COMMANDS, dict)
        for name, cmd in PLATFORM_COMMANDS.items():
            assert isinstance(name, str)
            assert isinstance(cmd, list)
            assert all(isinstance(c, str) for c in cmd)

    def test_required_platforms_exist(self):
        """Required platforms should be defined."""
        assert "claude" in PLATFORM_COMMANDS
        assert "cursor" in PLATFORM_COMMANDS
        assert "codex" in PLATFORM_COMMANDS
        assert "gemini" in PLATFORM_COMMANDS

    def test_required_commands_list(self):
        """Required commands should include core C4 commands."""
        assert "c4-status" in REQUIRED_COMMANDS
        assert "c4-run" in REQUIRED_COMMANDS
        assert "c4-plan" in REQUIRED_COMMANDS
        assert "c4-checkpoint" in REQUIRED_COMMANDS

    def test_complex_commands_subset(self):
        """Complex commands should be subset of required commands."""
        for cmd in COMPLEX_COMMANDS:
            assert cmd in REQUIRED_COMMANDS


class TestGetDefaultPlatform:
    """Test get_default_platform function."""

    def test_returns_claude_by_default(self, tmp_path: Path):
        """Should return 'claude' when no config exists."""
        # Clear environment
        old_env = os.environ.pop("C4_PLATFORM", None)
        try:
            result = get_default_platform(tmp_path)
            assert result == "claude"
        finally:
            if old_env:
                os.environ["C4_PLATFORM"] = old_env

    def test_respects_environment_variable(self, tmp_path: Path, monkeypatch):
        """Should use C4_PLATFORM environment variable."""
        # Use fake home to avoid real global config
        fake_home = tmp_path / "home"
        fake_home.mkdir()
        monkeypatch.setenv("HOME", str(fake_home))
        monkeypatch.setenv("C4_PLATFORM", "cursor")

        result = get_default_platform(tmp_path)
        assert result == "cursor"

    def test_respects_project_config(self, tmp_path: Path):
        """Should use project config over environment."""
        old_env = os.environ.get("C4_PLATFORM")
        try:
            os.environ["C4_PLATFORM"] = "cursor"

            # Create project config
            config_dir = tmp_path / ".c4"
            config_dir.mkdir()
            config_file = config_dir / "config.yaml"
            config_file.write_text("platform: codex\n")

            result = get_default_platform(tmp_path)
            assert result == "codex"
        finally:
            if old_env:
                os.environ["C4_PLATFORM"] = old_env
            else:
                os.environ.pop("C4_PLATFORM", None)

    def test_respects_global_config(self, tmp_path: Path, monkeypatch):
        """Should use global config when project config doesn't exist."""
        old_env = os.environ.pop("C4_PLATFORM", None)

        # Create fake home with global config
        fake_home = tmp_path / "home"
        fake_home.mkdir()
        monkeypatch.setenv("HOME", str(fake_home))

        global_config_dir = fake_home / ".c4"
        global_config_dir.mkdir()
        global_config = global_config_dir / "config.yaml"
        global_config.write_text("platform: gemini\n")

        # Use a different path for project (no config there)
        project_path = tmp_path / "project"
        project_path.mkdir()

        try:
            result = get_default_platform(project_path)
            assert result == "gemini"
        finally:
            if old_env:
                os.environ["C4_PLATFORM"] = old_env


class TestGetPlatformCommand:
    """Test get_platform_command function."""

    def test_returns_command_for_known_platform(self):
        """Should return command list for known platforms."""
        assert get_platform_command("claude") == ["claude"]
        assert get_platform_command("cursor") == ["cursor", "."]
        assert get_platform_command("codex") == ["codex"]

    def test_returns_none_for_unknown_platform(self):
        """Should return None for unknown platforms."""
        assert get_platform_command("unknown") is None
        assert get_platform_command("") is None


class TestListPlatforms:
    """Test list_platforms function."""

    def test_returns_all_platforms(self):
        """Should return list of all platform names."""
        platforms = list_platforms()
        assert isinstance(platforms, list)
        assert "claude" in platforms
        assert "cursor" in platforms
        assert "codex" in platforms


class TestSetPlatformConfig:
    """Test set_platform_config function."""

    def test_creates_project_config(self, tmp_path: Path):
        """Should create project config file."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()

        result = set_platform_config("cursor", global_config=False, project_path=tmp_path)

        assert result == c4_dir / "config.yaml"
        assert result.exists()

        config = yaml.safe_load(result.read_text())
        assert config["platform"] == "cursor"

    def test_creates_global_config(self, tmp_path: Path, monkeypatch):
        """Should create global config file."""
        fake_home = tmp_path / "home"
        fake_home.mkdir()
        monkeypatch.setenv("HOME", str(fake_home))

        result = set_platform_config("codex", global_config=True)

        expected = fake_home / ".c4" / "config.yaml"
        assert result == expected
        assert result.exists()

        config = yaml.safe_load(result.read_text())
        assert config["platform"] == "codex"

    def test_rejects_unknown_platform(self, tmp_path: Path):
        """Should raise ValueError for unknown platform."""
        with pytest.raises(ValueError, match="Unknown platform"):
            set_platform_config("unknown", project_path=tmp_path)

    def test_preserves_other_config(self, tmp_path: Path):
        """Should preserve other config values when updating."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        config_file = c4_dir / "config.yaml"
        config_file.write_text("other_setting: value\nplatform: old\n")

        set_platform_config("cursor", global_config=False, project_path=tmp_path)

        config = yaml.safe_load(config_file.read_text())
        assert config["platform"] == "cursor"
        assert config["other_setting"] == "value"


class TestGetConfigInfo:
    """Test get_config_info function."""

    def test_returns_default_info(self, tmp_path: Path, monkeypatch):
        """Should return default info when no config exists."""
        # Use fake home to avoid real global config
        fake_home = tmp_path / "home"
        fake_home.mkdir()
        monkeypatch.setenv("HOME", str(fake_home))
        monkeypatch.delenv("C4_PLATFORM", raising=False)

        info = get_config_info(tmp_path)

        assert info["global_platform"] is None
        assert info["project_platform"] is None
        assert info["env_platform"] is None
        assert info["effective_platform"] == "claude"
        assert info["source"] == "default"

    def test_detects_project_config(self, tmp_path: Path):
        """Should detect project config as source."""
        old_env = os.environ.pop("C4_PLATFORM", None)
        try:
            c4_dir = tmp_path / ".c4"
            c4_dir.mkdir()
            (c4_dir / "config.yaml").write_text("platform: cursor\n")

            info = get_config_info(tmp_path)

            assert info["project_platform"] == "cursor"
            assert info["effective_platform"] == "cursor"
            assert info["source"] == "project"
        finally:
            if old_env:
                os.environ["C4_PLATFORM"] = old_env


class TestValidatePlatformCommands:
    """Test validate_platform_commands function."""

    def test_reports_missing_commands(self, tmp_path: Path):
        """Should report all commands as missing when dir doesn't exist."""
        result = validate_platform_commands(tmp_path, "cursor")

        assert len(result["missing"]) == len(REQUIRED_COMMANDS)
        assert len(result["found"]) == 0

    def test_finds_existing_commands(self, tmp_path: Path):
        """Should find existing command files."""
        cmd_dir = tmp_path / ".cursor" / "commands"
        cmd_dir.mkdir(parents=True)
        (cmd_dir / "c4-status.md").write_text("# Status")
        (cmd_dir / "c4-run.md").write_text("# Run")

        result = validate_platform_commands(tmp_path, "cursor")

        assert "c4-status" in result["found"]
        assert "c4-run" in result["found"]
        assert "c4-plan" in result["missing"]


class TestGenerateCommandTemplate:
    """Test generate_command_template function."""

    def test_generates_simple_template(self, tmp_path: Path):
        """Should generate template for simple commands."""
        result = generate_command_template(tmp_path, "cursor", "c4-status")

        assert result is not None
        assert result.exists()
        assert "c4-status" in str(result)

        content = result.read_text()
        assert "C4 Project Status" in content
        assert "c4_status()" in content

    def test_copies_complex_command_from_reference(self, tmp_path: Path):
        """Should copy complex commands from reference platform."""
        # Create reference command
        ref_dir = tmp_path / ".claude" / "commands"
        ref_dir.mkdir(parents=True)
        ref_content = "# Complex Plan Command\n\nLots of instructions..."
        (ref_dir / "c4-plan.md").write_text(ref_content)

        result = generate_command_template(tmp_path, "cursor", "c4-plan")

        assert result is not None
        assert result.exists()

        content = result.read_text()
        assert "TODO: Customize" in content
        assert "Complex Plan Command" in content

    def test_returns_none_when_reference_missing(self, tmp_path: Path):
        """Should return None for complex commands without reference."""
        result = generate_command_template(tmp_path, "cursor", "c4-plan")
        assert result is None


class TestSetupPlatform:
    """Test setup_platform function."""

    def test_sets_up_platform_with_templates(self, tmp_path: Path):
        """Should create command directory and generate templates."""
        # Create reference commands
        ref_dir = tmp_path / ".claude" / "commands"
        ref_dir.mkdir(parents=True)
        (ref_dir / "c4-plan.md").write_text("# Plan")
        (ref_dir / "c4-run.md").write_text("# Run")

        result = setup_platform(tmp_path, "cursor", generate_templates=True)

        assert result["platform"] == "cursor"
        assert ".cursor/commands" in result["command_dir"]
        assert len(result["generated"]) > 0

    def test_skips_template_generation_when_disabled(self, tmp_path: Path):
        """Should not generate templates when disabled."""
        result = setup_platform(tmp_path, "cursor", generate_templates=False)

        assert result["generated"] == []
        assert result["skipped"] == []
