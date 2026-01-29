"""Tests for C4 Git hooks management."""

import os
import stat
from unittest.mock import patch

import pytest

from c4.git_hooks import (
    COMMIT_MSG_HOOK,
    HOOKS,
    POST_COMMIT_HOOK,
    PRE_COMMIT_HOOK,
    get_all_hook_status,
    get_git_hooks_dir,
    get_hook_status,
    install_all_hooks,
    install_hook,
    uninstall_all_hooks,
    uninstall_hook,
)


class TestHookTemplates:
    """Tests for hook template content."""

    def test_pre_commit_hook_has_c4_marker(self):
        """Pre-commit hook should have C4 marker for identification."""
        assert "C4 Git Hook" in PRE_COMMIT_HOOK
        assert "pre-commit" in PRE_COMMIT_HOOK

    def test_commit_msg_hook_has_c4_marker(self):
        """Commit-msg hook should have C4 marker for identification."""
        assert "C4 Git Hook" in COMMIT_MSG_HOOK
        assert "commit-msg" in COMMIT_MSG_HOOK

    def test_commit_msg_hook_has_task_pattern(self):
        """Commit-msg hook should check for Task ID pattern."""
        assert "T-[0-9]+" in COMMIT_MSG_HOOK
        assert "R-[0-9]+" in COMMIT_MSG_HOOK

    def test_post_commit_hook_has_c4_marker(self):
        """Post-commit hook should have C4 marker."""
        assert "C4 Git Hook" in POST_COMMIT_HOOK

    def test_all_hooks_are_bash_scripts(self):
        """All hooks should start with bash shebang."""
        for hook_name, content in HOOKS.items():
            assert content.startswith("#!/bin/bash"), f"{hook_name} missing shebang"


class TestGetGitHooksDir:
    """Tests for get_git_hooks_dir function."""

    def test_returns_none_outside_git_repo(self, tmp_path):
        """Should return None when not in a git repo."""
        with patch("c4.git_hooks.Path.cwd", return_value=tmp_path):
            result = get_git_hooks_dir()
            assert result is None

    def test_returns_hooks_dir_in_git_repo(self, tmp_path):
        """Should return .git/hooks path when in a git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        with patch("c4.git_hooks.Path.cwd", return_value=tmp_path):
            result = get_git_hooks_dir()
            assert result == git_dir / "hooks"

    def test_finds_git_dir_in_parent(self, tmp_path):
        """Should find .git in parent directory."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        subdir = tmp_path / "src" / "module"
        subdir.mkdir(parents=True)

        with patch("c4.git_hooks.Path.cwd", return_value=subdir):
            result = get_git_hooks_dir()
            assert result == git_dir / "hooks"


class TestInstallHook:
    """Tests for install_hook function."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a mock git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()
        return tmp_path

    def test_install_hook_creates_file(self, git_repo):
        """Should create hook file with correct content."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            success, message = install_hook("pre-commit", PRE_COMMIT_HOOK)

            assert success is True
            assert "Installed" in message

            hook_path = git_repo / ".git" / "hooks" / "pre-commit"
            assert hook_path.exists()
            assert hook_path.read_text() == PRE_COMMIT_HOOK

    def test_install_hook_makes_executable(self, git_repo):
        """Should make hook file executable."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_hook("pre-commit", PRE_COMMIT_HOOK)

            hook_path = git_repo / ".git" / "hooks" / "pre-commit"
            assert os.access(hook_path, os.X_OK)

    def test_install_hook_skips_existing_non_c4(self, git_repo):
        """Should not overwrite existing non-C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom hook'")

        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            success, message = install_hook("pre-commit", PRE_COMMIT_HOOK)

            assert success is False
            assert "Existing hook" in message
            assert hook_path.read_text() == "#!/bin/bash\necho 'custom hook'"

    def test_install_hook_force_overwrites(self, git_repo):
        """Should overwrite existing hooks with --force."""
        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom hook'")

        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            success, message = install_hook("pre-commit", PRE_COMMIT_HOOK, force=True)

            assert success is True
            assert hook_path.read_text() == PRE_COMMIT_HOOK

    def test_install_hook_idempotent_for_c4_hooks(self, git_repo):
        """Should report success for already installed C4 hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_hook("pre-commit", PRE_COMMIT_HOOK)
            success, message = install_hook("pre-commit", PRE_COMMIT_HOOK)

            assert success is True
            assert "Already installed" in message

    def test_install_hook_fails_outside_git(self, tmp_path):
        """Should fail when not in a git repo."""
        with patch("c4.git_hooks.Path.cwd", return_value=tmp_path):
            success, message = install_hook("pre-commit", PRE_COMMIT_HOOK)

            assert success is False
            assert "Not in a git repository" in message


class TestUninstallHook:
    """Tests for uninstall_hook function."""

    @pytest.fixture
    def git_repo_with_hooks(self, tmp_path):
        """Create a mock git repo with C4 hooks installed."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Install a C4 hook
        (hooks_dir / "pre-commit").write_text(PRE_COMMIT_HOOK)
        return tmp_path

    def test_uninstall_removes_c4_hook(self, git_repo_with_hooks):
        """Should remove C4 hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo_with_hooks):
            success, message = uninstall_hook("pre-commit")

            assert success is True
            assert "Uninstalled" in message
            assert not (git_repo_with_hooks / ".git" / "hooks" / "pre-commit").exists()

    def test_uninstall_skips_non_c4_hook(self, git_repo_with_hooks):
        """Should not remove non-C4 hooks."""
        hook_path = git_repo_with_hooks / ".git" / "hooks" / "commit-msg"
        hook_path.write_text("#!/bin/bash\necho 'custom'")

        with patch("c4.git_hooks.Path.cwd", return_value=git_repo_with_hooks):
            success, message = uninstall_hook("commit-msg")

            assert success is False
            assert "Not a C4 hook" in message
            assert hook_path.exists()

    def test_uninstall_succeeds_when_not_installed(self, git_repo_with_hooks):
        """Should succeed if hook was never installed."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo_with_hooks):
            success, message = uninstall_hook("post-commit")

            assert success is True
            assert "Not installed" in message


class TestGetHookStatus:
    """Tests for get_hook_status function."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a mock git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()
        return tmp_path

    def test_status_not_installed(self, git_repo):
        """Should report not installed for missing hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            status = get_hook_status("pre-commit")

            assert status["installed"] is False
            assert status["is_c4"] is False

    def test_status_c4_hook_installed(self, git_repo):
        """Should identify C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        hook_path.write_text(PRE_COMMIT_HOOK)
        hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR)

        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            status = get_hook_status("pre-commit")

            assert status["installed"] is True
            assert status["is_c4"] is True
            assert status["executable"] is True

    def test_status_external_hook(self, git_repo):
        """Should identify non-C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'external'")

        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            status = get_hook_status("pre-commit")

            assert status["installed"] is True
            assert status["is_c4"] is False


class TestInstallAllHooks:
    """Tests for install_all_hooks function."""

    @pytest.fixture
    def git_repo(self, tmp_path):
        """Create a mock git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()
        return tmp_path

    def test_install_all_hooks(self, git_repo):
        """Should install all hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            results = install_all_hooks()

            assert len(results) == len(HOOKS)
            for hook_name, (success, message) in results.items():
                assert success is True, f"{hook_name}: {message}"

    def test_uninstall_all_hooks(self, git_repo):
        """Should uninstall all hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_all_hooks()
            results = uninstall_all_hooks()

            assert len(results) == len(HOOKS)
            for hook_name, (success, message) in results.items():
                assert success is True, f"{hook_name}: {message}"

    def test_get_all_hook_status(self, git_repo):
        """Should return status for all hooks."""
        with patch("c4.git_hooks.Path.cwd", return_value=git_repo):
            install_all_hooks()
            results = get_all_hook_status()

            assert len(results) == len(HOOKS)
            for hook_name, status in results.items():
                assert status["installed"] is True
                assert status["is_c4"] is True



class TestHooksConfig:
    """Tests for HooksConfig model."""

    def test_default_config(self):
        """Should have sensible defaults."""
        from c4.models.config import HooksConfig

        config = HooksConfig()

        assert config.enabled is True
        assert config.install_on_init is False
        assert config.pre_commit_enabled is True
        assert config.commit_msg_enabled is True
        assert config.commit_msg_mode == "warn"
        assert config.post_commit_enabled is True

    def test_strict_mode(self):
        """Should support strict commit-msg mode."""
        from c4.models.config import HooksConfig

        config = HooksConfig(commit_msg_mode="strict")
        assert config.commit_msg_mode == "strict"

    def test_invalid_mode_rejected(self):
        """Should reject invalid commit-msg modes."""
        from pydantic import ValidationError

        from c4.models.config import HooksConfig

        with pytest.raises(ValidationError):
            HooksConfig(commit_msg_mode="invalid")

    def test_custom_validations(self):
        """Should support custom pre-commit validations."""
        from c4.models.config import HooksConfig

        config = HooksConfig(pre_commit_validations=["lint", "unit"])
        assert config.pre_commit_validations == ["lint", "unit"]

    def test_c4_config_includes_hooks(self):
        """C4Config should include hooks field."""
        from c4.models.config import C4Config

        config = C4Config(project_id="test")
        assert hasattr(config, "hooks")
        assert config.hooks.enabled is True
