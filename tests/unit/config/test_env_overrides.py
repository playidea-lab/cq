"""Tests for environment variable overrides in ConfigManager."""

import os
from pathlib import Path
from unittest.mock import patch

import pytest

from c4.config.manager import ENV_VAR_MAPPING, ConfigManager


class TestEnvVarMapping:
    """Tests for ENV_VAR_MAPPING constant."""

    def test_platform_mapping_exists(self) -> None:
        """Platform should have env var mapping."""
        assert "platform" in ENV_VAR_MAPPING
        assert ENV_VAR_MAPPING["platform"] == ("C4_PLATFORM", "str")

    def test_git_mappings_exist(self) -> None:
        """Git settings should have env var mappings."""
        assert "git.user" in ENV_VAR_MAPPING
        assert "git.email" in ENV_VAR_MAPPING
        assert "git.auto_commit" in ENV_VAR_MAPPING
        assert "git.auto_pr" in ENV_VAR_MAPPING

        assert ENV_VAR_MAPPING["git.auto_commit"][1] == "bool"
        assert ENV_VAR_MAPPING["git.auto_pr"][1] == "bool"

    def test_validation_mappings_exist(self) -> None:
        """Validation settings should have env var mappings."""
        assert "validation.lint" in ENV_VAR_MAPPING
        assert "validation.test" in ENV_VAR_MAPPING
        assert "validation.typecheck" in ENV_VAR_MAPPING

    def test_worker_mappings_exist(self) -> None:
        """Worker settings should have env var mappings."""
        assert "worker.max_retries" in ENV_VAR_MAPPING
        assert "worker.timeout" in ENV_VAR_MAPPING

        assert ENV_VAR_MAPPING["worker.max_retries"][1] == "int"
        assert ENV_VAR_MAPPING["worker.timeout"][1] == "int"

    def test_team_mappings_exist(self) -> None:
        """Team settings should have env var mappings."""
        assert "team.id" in ENV_VAR_MAPPING
        assert "team.name" in ENV_VAR_MAPPING


class TestIsFromEnv:
    """Tests for ConfigManager._is_from_env()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> ConfigManager:
        """Create ConfigManager instance."""
        return ConfigManager(tmp_path)

    def test_returns_false_when_env_not_set(self, manager: ConfigManager) -> None:
        """Should return False when env var is not set."""
        with patch.dict(os.environ, {}, clear=True):
            assert manager._is_from_env("platform") is False

    def test_returns_true_when_env_set(self, manager: ConfigManager) -> None:
        """Should return True when env var is set."""
        with patch.dict(os.environ, {"C4_PLATFORM": "github"}):
            assert manager._is_from_env("platform") is True

    def test_returns_false_for_unknown_key(self, manager: ConfigManager) -> None:
        """Should return False for keys without env mapping."""
        assert manager._is_from_env("unknown.key") is False


class TestParseEnvValue:
    """Tests for ConfigManager._parse_env_value()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> ConfigManager:
        """Create ConfigManager instance."""
        return ConfigManager(tmp_path)

    def test_parse_string(self, manager: ConfigManager) -> None:
        """Should return string as-is."""
        assert manager._parse_env_value("hello", "str") == "hello"
        assert manager._parse_env_value("", "str") == ""

    def test_parse_bool_true_values(self, manager: ConfigManager) -> None:
        """Should parse various true values."""
        for value in ["true", "True", "TRUE", "1", "yes", "YES", "on", "ON"]:
            assert manager._parse_env_value(value, "bool") is True

    def test_parse_bool_false_values(self, manager: ConfigManager) -> None:
        """Should parse various false values."""
        for value in ["false", "False", "0", "no", "off", "", "anything"]:
            assert manager._parse_env_value(value, "bool") is False

    def test_parse_int_valid(self, manager: ConfigManager) -> None:
        """Should parse valid integers."""
        assert manager._parse_env_value("42", "int") == 42
        assert manager._parse_env_value("0", "int") == 0
        assert manager._parse_env_value("-5", "int") == -5

    def test_parse_int_invalid(self, manager: ConfigManager) -> None:
        """Should return 0 for invalid integers."""
        assert manager._parse_env_value("not-a-number", "int") == 0
        assert manager._parse_env_value("", "int") == 0


class TestApplyEnvOverrides:
    """Tests for ConfigManager._apply_env_overrides()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> ConfigManager:
        """Create ConfigManager instance."""
        return ConfigManager(tmp_path)

    def test_override_platform(self, manager: ConfigManager) -> None:
        """Should override platform from env."""
        # platform is a Literal type: "claude", "cursor", "codex", "gemini", "opencode"
        with patch.dict(os.environ, {"C4_PLATFORM": "cursor"}):
            config = manager.load()
            assert config.platform == "cursor"

    def test_override_git_user(self, manager: ConfigManager) -> None:
        """Should override git.user from env."""
        with patch.dict(os.environ, {"C4_GIT_USER": "envuser"}):
            config = manager.load()
            assert config.git.user == "envuser"

    def test_override_git_email(self, manager: ConfigManager) -> None:
        """Should override git.email from env."""
        with patch.dict(os.environ, {"C4_GIT_EMAIL": "env@example.com"}):
            config = manager.load()
            assert config.git.email == "env@example.com"

    def test_override_git_auto_commit(self, manager: ConfigManager) -> None:
        """Should override git.auto_commit from env."""
        with patch.dict(os.environ, {"C4_GIT_AUTO_COMMIT": "true"}):
            config = manager.load()
            assert config.git.auto_commit is True

        with patch.dict(os.environ, {"C4_GIT_AUTO_COMMIT": "false"}):
            config = manager.load()
            assert config.git.auto_commit is False

    def test_override_git_auto_pr(self, manager: ConfigManager) -> None:
        """Should override git.auto_pr from env."""
        with patch.dict(os.environ, {"C4_GIT_AUTO_PR": "1"}):
            config = manager.load()
            assert config.git.auto_pr is True

    def test_override_worker_max_retries(self, manager: ConfigManager) -> None:
        """Should override worker.max_retries from env."""
        with patch.dict(os.environ, {"C4_WORKER_MAX_RETRIES": "5"}):
            config = manager.load()
            assert config.worker.max_retries == 5

    def test_override_worker_timeout(self, manager: ConfigManager) -> None:
        """Should override worker.timeout from env."""
        with patch.dict(os.environ, {"C4_WORKER_TIMEOUT": "120"}):
            config = manager.load()
            assert config.worker.timeout == 120

    def test_override_team_id(self, manager: ConfigManager) -> None:
        """Should override team.id from env."""
        with patch.dict(os.environ, {"C4_TEAM_ID": "team-123"}):
            config = manager.load()
            assert config.team.id == "team-123"

    def test_override_team_name(self, manager: ConfigManager) -> None:
        """Should override team.name from env."""
        with patch.dict(os.environ, {"C4_TEAM_NAME": "My Team"}):
            config = manager.load()
            assert config.team.name == "My Team"

    def test_multiple_overrides(self, manager: ConfigManager) -> None:
        """Should apply multiple env overrides."""
        env = {
            "C4_PLATFORM": "codex",  # Valid platform value
            "C4_GIT_USER": "testuser",
            "C4_GIT_AUTO_COMMIT": "true",
            "C4_WORKER_MAX_RETRIES": "10",
        }
        with patch.dict(os.environ, env):
            config = manager.load()
            assert config.platform == "codex"
            assert config.git.user == "testuser"
            assert config.git.auto_commit is True
            assert config.worker.max_retries == 10


class TestEnvOverridePriority:
    """Tests for env override priority over file configs."""

    @pytest.fixture
    def manager_with_config(self, tmp_path: Path) -> ConfigManager:
        """Create ConfigManager with project config file."""
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()
        config_file = c4_dir / "config.yaml"
        # Use valid platform value (Literal type)
        config_file.write_text(
            """
platform: claude
git:
  user: fileuser
  email: file@example.com
  auto_commit: false
worker:
  max_retries: 3
"""
        )
        return ConfigManager(tmp_path)

    def test_env_takes_priority_over_file(self, manager_with_config: ConfigManager) -> None:
        """Env vars should override file config."""
        # Without env var
        config = manager_with_config.load()
        assert config.platform == "claude"
        assert config.git.user == "fileuser"
        assert config.worker.max_retries == 3

        # With env var (use valid platform value)
        with patch.dict(os.environ, {"C4_PLATFORM": "cursor", "C4_GIT_USER": "envuser"}):
            config = manager_with_config.load()
            assert config.platform == "cursor"
            assert config.git.user == "envuser"
            # Non-overridden values from file
            assert config.git.email == "file@example.com"
            assert config.worker.max_retries == 3

    def test_show_reports_env_source(self, manager_with_config: ConfigManager) -> None:
        """show() should report 'env' as source for overridden values."""
        with patch.dict(os.environ, {"C4_PLATFORM": "cursor"}):
            result = manager_with_config.show()
            assert result["sources"]["platform"] == "env"
            assert result["sources"]["git.user"] == "project"
