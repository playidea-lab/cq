"""Tests for WorktreeConfig model."""

import pytest
from pydantic import ValidationError

from c4.models.config import C4Config, WorktreeConfig


class TestWorktreeConfig:
    """Tests for WorktreeConfig model."""

    def test_default_values(self) -> None:
        """Test WorktreeConfig has correct default values."""
        config = WorktreeConfig()

        assert config.enabled is False
        assert config.base_branch == "work"
        assert config.work_dir is None
        assert config.auto_cleanup is True
        assert config.completion_action == "pr"

    def test_custom_values(self) -> None:
        """Test WorktreeConfig with custom values."""
        config = WorktreeConfig(
            enabled=True,
            base_branch="develop",
            work_dir="/custom/worktrees",
            auto_cleanup=False,
            completion_action="merge",
        )

        assert config.enabled is True
        assert config.base_branch == "develop"
        assert config.work_dir == "/custom/worktrees"
        assert config.auto_cleanup is False
        assert config.completion_action == "merge"

    def test_get_work_dir_default(self) -> None:
        """Test get_work_dir returns default when work_dir is None."""
        config = WorktreeConfig()

        assert config.get_work_dir() == ".c4/worktrees"

    def test_get_work_dir_custom(self) -> None:
        """Test get_work_dir returns custom value when set."""
        config = WorktreeConfig(work_dir="/my/worktrees")

        assert config.get_work_dir() == "/my/worktrees"

    def test_completion_action_validation(self) -> None:
        """Test completion_action only accepts 'merge' or 'pr'."""
        # Valid values
        WorktreeConfig(completion_action="merge")
        WorktreeConfig(completion_action="pr")

        # Invalid value
        with pytest.raises(ValidationError) as exc_info:
            WorktreeConfig(completion_action="invalid")

        assert "completion_action" in str(exc_info.value)


class TestC4ConfigWithWorktree:
    """Tests for C4Config with worktree field."""

    def test_c4config_has_worktree_field(self) -> None:
        """Test C4Config has worktree field with default WorktreeConfig."""
        config = C4Config(project_id="test-project")

        assert hasattr(config, "worktree")
        assert isinstance(config.worktree, WorktreeConfig)
        assert config.worktree.enabled is False

    def test_c4config_custom_worktree(self) -> None:
        """Test C4Config with custom worktree configuration."""
        config = C4Config(
            project_id="test-project",
            worktree=WorktreeConfig(
                enabled=True,
                base_branch="main",
                completion_action="merge",
            ),
        )

        assert config.worktree.enabled is True
        assert config.worktree.base_branch == "main"
        assert config.worktree.completion_action == "merge"

    def test_c4config_worktree_from_dict(self) -> None:
        """Test C4Config can parse worktree from dict (YAML style)."""
        config = C4Config(
            project_id="test-project",
            worktree={
                "enabled": True,
                "base_branch": "develop",
                "work_dir": ".c4/custom",
                "auto_cleanup": False,
                "completion_action": "pr",
            },
        )

        assert config.worktree.enabled is True
        assert config.worktree.base_branch == "develop"
        assert config.worktree.work_dir == ".c4/custom"
        assert config.worktree.auto_cleanup is False
        assert config.worktree.completion_action == "pr"
