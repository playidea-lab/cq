"""Tests for post-commit hook functionality."""

import os
import stat
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.hooks import (
    FALLBACK_COMMANDS,
    HookClient,
    get_post_commit_status,
    get_post_commit_template,
    install_post_commit_hook,
    uninstall_post_commit_hook,
)

# =============================================================================
# Test Fixtures
# =============================================================================


@pytest.fixture
def git_repo(tmp_path: Path) -> Path:
    """Create a mock git repository."""
    git_dir = tmp_path / ".git"
    git_dir.mkdir()
    hooks_dir = git_dir / "hooks"
    hooks_dir.mkdir()
    return tmp_path


@pytest.fixture
def c4_project(git_repo: Path) -> Path:
    """Create a C4 project structure."""
    c4_dir = git_repo / ".c4"
    c4_dir.mkdir()
    (c4_dir / "state.json").write_text('{"status": "EXECUTE"}')
    return git_repo


# =============================================================================
# Post-Commit Template Tests
# =============================================================================


class TestPostCommitTemplate:
    """Tests for post-commit template content."""

    def test_template_has_c4_marker(self) -> None:
        """Template should have C4 marker for identification."""
        template = get_post_commit_template()
        assert "C4 Git Hook" in template or "C4:" in template

    def test_template_is_bash_script(self) -> None:
        """Template should be a valid bash script."""
        template = get_post_commit_template()
        assert template.startswith("#!/bin/bash")

    def test_template_skips_non_c4_projects(self) -> None:
        """Template should skip non-C4 projects."""
        template = get_post_commit_template()
        assert ".c4/state.json" in template or ".c4/config.yaml" in template

    def test_template_always_exits_zero(self) -> None:
        """Template should always exit 0 (commit is already done)."""
        template = get_post_commit_template()
        # The template should exit 0 regardless of test result
        assert "exit 0" in template

    def test_template_has_test_execution(self) -> None:
        """Template should execute tests."""
        template = get_post_commit_template()
        # Should have pytest or test command
        assert "pytest" in template or "test" in template.lower()

    def test_template_has_warning_message(self) -> None:
        """Template should show warning on test failure."""
        template = get_post_commit_template()
        assert "WARNING" in template or "warning" in template.lower()

    def test_template_has_success_message(self) -> None:
        """Template should show success on test pass."""
        template = get_post_commit_template()
        assert "SUCCESS" in template or "passed" in template.lower()


# =============================================================================
# Post-Commit Hook Installation Tests
# =============================================================================


class TestInstallPostCommitHook:
    """Tests for post-commit hook installation."""

    def test_install_creates_hook(self, git_repo: Path) -> None:
        """Installing hook should create the file."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = install_post_commit_hook()

            assert success is True
            assert "Installed" in message

            hook_path = git_repo / ".git" / "hooks" / "post-commit"
            assert hook_path.exists()

    def test_install_makes_executable(self, git_repo: Path) -> None:
        """Installed hook should be executable."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            install_post_commit_hook()

            hook_path = git_repo / ".git" / "hooks" / "post-commit"
            assert os.access(hook_path, os.X_OK)

    def test_install_idempotent_for_c4_hooks(self, git_repo: Path) -> None:
        """Installing same hook twice should report already installed."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            install_post_commit_hook()
            success, message = install_post_commit_hook()

            assert success is True
            assert "Already installed" in message

    def test_install_skips_existing_non_c4_hook(self, git_repo: Path) -> None:
        """Should not overwrite existing non-C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom hook'")

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = install_post_commit_hook()

            assert success is False
            assert "Existing hook" in message
            assert hook_path.read_text() == "#!/bin/bash\necho 'custom hook'"

    def test_install_force_overwrites(self, git_repo: Path) -> None:
        """Force should overwrite existing hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom hook'")

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = install_post_commit_hook(force=True)

            assert success is True
            assert "C4 Git Hook" in hook_path.read_text() or "C4:" in hook_path.read_text()

    def test_install_fails_outside_git_repo(self, tmp_path: Path) -> None:
        """Should fail when not in a git repository."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=None):
            success, message = install_post_commit_hook()

            assert success is False
            assert "git repository" in message.lower()


# =============================================================================
# Post-Commit Hook Uninstallation Tests
# =============================================================================


class TestUninstallPostCommitHook:
    """Tests for post-commit hook uninstallation."""

    def test_uninstall_removes_c4_hook(self, git_repo: Path) -> None:
        """Should remove C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\n# C4 Git Hook: post-commit\necho test")

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = uninstall_post_commit_hook()

            assert success is True
            assert "Uninstalled" in message
            assert not hook_path.exists()

    def test_uninstall_skips_non_c4_hook(self, git_repo: Path) -> None:
        """Should not remove non-C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\necho 'custom hook'")

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = uninstall_post_commit_hook()

            assert success is False
            assert "Not a C4 hook" in message
            assert hook_path.exists()

    def test_uninstall_succeeds_when_not_installed(self, git_repo: Path) -> None:
        """Should succeed if hook was never installed."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            success, message = uninstall_post_commit_hook()

            assert success is True
            assert "Not installed" in message


# =============================================================================
# Post-Commit Hook Status Tests
# =============================================================================


class TestGetPostCommitStatus:
    """Tests for post-commit hook status checking."""

    def test_status_not_installed(self, git_repo: Path) -> None:
        """Should report not installed for missing hooks."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            status = get_post_commit_status()

            assert status["installed"] is False
            assert status["is_c4"] is False

    def test_status_c4_hook_installed(self, git_repo: Path) -> None:
        """Should identify C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\n# C4 Git Hook: post-commit\necho test")
        hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR)

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            status = get_post_commit_status()

            assert status["installed"] is True
            assert status["is_c4"] is True
            assert status["executable"] is True

    def test_status_external_hook(self, git_repo: Path) -> None:
        """Should identify non-C4 hooks."""
        hook_path = git_repo / ".git" / "hooks" / "post-commit"
        hook_path.write_text("#!/bin/bash\necho 'external'")

        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            status = get_post_commit_status()

            assert status["installed"] is True
            assert status["is_c4"] is False


# =============================================================================
# HookClient Test Execution Tests
# =============================================================================


class TestHookClientTestValidation:
    """Tests for HookClient test/unit validation type."""

    def test_fallback_commands_include_test_types(self) -> None:
        """FALLBACK_COMMANDS should include test types."""
        assert "unit" in FALLBACK_COMMANDS
        assert "test" in FALLBACK_COMMANDS
        assert "pytest" in FALLBACK_COMMANDS["unit"]
        assert "pytest" in FALLBACK_COMMANDS["test"]

    def test_run_fallback_validation_with_unit_type(self, tmp_path: Path) -> None:
        """Should run pytest for unit validation type."""
        client = HookClient(project_root=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stderr="", stdout="")

            result = client.run_fallback_validation("unit")

            assert result.passed is True
            mock_run.assert_called_once()
            call_args = mock_run.call_args
            assert "pytest" in call_args[1]["command"] if "command" in call_args[1] else "pytest" in call_args[0][0]

    def test_run_fallback_validation_with_test_type(self, tmp_path: Path) -> None:
        """Should run pytest for test validation type."""
        client = HookClient(project_root=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stderr="", stdout="")

            result = client.run_fallback_validation("test")

            assert result.passed is True


# =============================================================================
# Background Execution Tests
# =============================================================================


class TestPostCommitBackgroundExecution:
    """Tests for post-commit background execution option."""

    def test_template_supports_background_mode(self) -> None:
        """Template should support background execution setting."""
        template = get_post_commit_template()
        # Should have background environment variable check
        assert "BACKGROUND" in template or "background" in template


# =============================================================================
# Integration-like Tests
# =============================================================================


class TestPostCommitHookIntegration:
    """Integration-like tests for post-commit hook."""

    def test_install_and_uninstall_cycle(self, git_repo: Path) -> None:
        """Full install and uninstall cycle should work."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            # Install
            success, _ = install_post_commit_hook()
            assert success is True

            # Check status
            status = get_post_commit_status()
            assert status["installed"] is True
            assert status["is_c4"] is True

            # Uninstall
            success, _ = uninstall_post_commit_hook()
            assert success is True

            # Check status again
            status = get_post_commit_status()
            assert status["installed"] is False

    def test_hook_content_matches_template(self, git_repo: Path) -> None:
        """Installed hook content should match template."""
        with patch("c4.hooks.get_git_hooks_dir", return_value=git_repo / ".git" / "hooks"):
            install_post_commit_hook()

            hook_path = git_repo / ".git" / "hooks" / "post-commit"
            installed_content = hook_path.read_text()
            template_content = get_post_commit_template()

            assert installed_content == template_content
