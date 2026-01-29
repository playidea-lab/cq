"""Unit tests for c4/git_hooks.py"""

import stat

from c4 import git_hooks


class TestHookTemplates:
    """Test hook template content."""

    def test_pre_commit_hook_has_c4_marker(self):
        """Pre-commit hook should have C4 marker."""
        assert "C4 Git Hook" in git_hooks.PRE_COMMIT_HOOK
        assert "pre-commit" in git_hooks.PRE_COMMIT_HOOK

    def test_commit_msg_hook_has_c4_marker(self):
        """Commit-msg hook should have C4 marker."""
        assert "C4 Git Hook" in git_hooks.COMMIT_MSG_HOOK
        assert "commit-msg" in git_hooks.COMMIT_MSG_HOOK

    def test_post_commit_hook_has_c4_marker(self):
        """Post-commit hook should have C4 marker."""
        assert "C4 Git Hook" in git_hooks.POST_COMMIT_HOOK
        assert "post-commit" in git_hooks.POST_COMMIT_HOOK

    def test_commit_msg_hook_has_task_pattern(self):
        """Commit-msg hook should check for Task ID pattern."""
        assert "T-" in git_hooks.COMMIT_MSG_HOOK
        assert "R-" in git_hooks.COMMIT_MSG_HOOK
        assert "CP-" in git_hooks.COMMIT_MSG_HOOK

    def test_post_commit_hook_updates_sqlite(self):
        """Post-commit hook should update SQLite database."""
        assert "sqlite3" in git_hooks.POST_COMMIT_HOOK
        assert "tasks.db" in git_hooks.POST_COMMIT_HOOK
        assert "c4_tasks" in git_hooks.POST_COMMIT_HOOK
        assert "json_set" in git_hooks.POST_COMMIT_HOOK


class TestGetGitHooksDir:
    """Test get_git_hooks_dir function."""

    def test_returns_none_for_non_git_repo(self, tmp_path):
        """Should return None for non-git directory."""
        result = git_hooks.get_git_hooks_dir(tmp_path)
        assert result is None

    def test_returns_hooks_dir_for_git_repo(self, tmp_path):
        """Should return .git/hooks for git repo."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        result = git_hooks.get_git_hooks_dir(tmp_path)
        assert result == tmp_path / ".git" / "hooks"


class TestInstallHook:
    """Test install_hook function."""

    def test_fails_for_non_git_repo(self, tmp_path):
        """Should fail for non-git directory."""
        success, msg = git_hooks.install_hook("pre-commit", "content", tmp_path)
        assert not success
        assert "Not a Git repository" in msg

    def test_installs_hook_successfully(self, tmp_path):
        """Should install hook to .git/hooks/."""
        # Create git repo
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        success, msg = git_hooks.install_hook(
            "pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path
        )

        assert success
        assert "Installed" in msg

        hook_path = tmp_path / ".git" / "hooks" / "pre-commit"
        assert hook_path.exists()
        assert "C4 Git Hook" in hook_path.read_text()

    def test_makes_hook_executable(self, tmp_path):
        """Installed hook should be executable."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        git_hooks.install_hook("pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path)

        hook_path = tmp_path / ".git" / "hooks" / "pre-commit"
        mode = hook_path.stat().st_mode
        assert mode & stat.S_IXUSR  # User executable

    def test_does_not_overwrite_non_c4_hook_without_force(self, tmp_path):
        """Should not overwrite existing non-C4 hook without force."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Create existing non-C4 hook
        existing_hook = hooks_dir / "pre-commit"
        existing_hook.write_text("#!/bin/bash\necho 'My custom hook'")

        success, msg = git_hooks.install_hook(
            "pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path
        )

        assert not success
        assert "force=True" in msg

        # Original hook unchanged
        assert "My custom hook" in existing_hook.read_text()

    def test_overwrites_non_c4_hook_with_force(self, tmp_path):
        """Should overwrite existing non-C4 hook with force=True."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Create existing non-C4 hook
        existing_hook = hooks_dir / "pre-commit"
        existing_hook.write_text("#!/bin/bash\necho 'My custom hook'")

        success, msg = git_hooks.install_hook(
            "pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path, force=True
        )

        assert success
        assert "C4 Git Hook" in existing_hook.read_text()

    def test_overwrites_c4_hook_without_force(self, tmp_path):
        """Should overwrite existing C4 hook without needing force."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Create existing C4 hook
        existing_hook = hooks_dir / "pre-commit"
        existing_hook.write_text("#!/bin/bash\n# C4 Git Hook: old version")

        success, msg = git_hooks.install_hook(
            "pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path
        )

        assert success
        # Updated with new content
        assert "pre-commit validation" in existing_hook.read_text().lower()


class TestInstallAllHooks:
    """Test install_all_hooks function."""

    def test_installs_all_hooks(self, tmp_path):
        """Should install all three hooks."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        results = git_hooks.install_all_hooks(tmp_path)

        assert len(results) == 3
        assert all(success for success, _ in results.values())

        for hook_name in ["pre-commit", "commit-msg", "post-commit"]:
            hook_path = tmp_path / ".git" / "hooks" / hook_name
            assert hook_path.exists()


class TestUninstallHook:
    """Test uninstall_hook function."""

    def test_uninstalls_c4_hook(self, tmp_path):
        """Should uninstall C4 hook."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        # Install first
        git_hooks.install_hook("pre-commit", git_hooks.PRE_COMMIT_HOOK, tmp_path)

        # Uninstall
        success, msg = git_hooks.uninstall_hook("pre-commit", tmp_path)

        assert success
        assert "Uninstalled" in msg

        hook_path = tmp_path / ".git" / "hooks" / "pre-commit"
        assert not hook_path.exists()

    def test_does_not_uninstall_non_c4_hook(self, tmp_path):
        """Should not uninstall non-C4 hook."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Create non-C4 hook
        hook_path = hooks_dir / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'My custom hook'")

        success, msg = git_hooks.uninstall_hook("pre-commit", tmp_path)

        assert not success
        assert "not a C4 hook" in msg

        # Hook still exists
        assert hook_path.exists()

    def test_succeeds_for_missing_hook(self, tmp_path):
        """Should succeed if hook doesn't exist."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        success, msg = git_hooks.uninstall_hook("pre-commit", tmp_path)

        assert success
        assert "not found" in msg


class TestUninstallAllHooks:
    """Test uninstall_all_hooks function."""

    def test_uninstalls_all_hooks(self, tmp_path):
        """Should uninstall all C4 hooks."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        # Install first
        git_hooks.install_all_hooks(tmp_path)

        # Uninstall
        results = git_hooks.uninstall_all_hooks(tmp_path)

        assert len(results) == 3
        assert all(success for success, _ in results.values())

        for hook_name in ["pre-commit", "commit-msg", "post-commit"]:
            hook_path = tmp_path / ".git" / "hooks" / hook_name
            assert not hook_path.exists()


class TestCheckHooksInstalled:
    """Test check_hooks_installed function."""

    def test_returns_false_for_non_git_repo(self, tmp_path):
        """Should return all False for non-git directory."""
        results = git_hooks.check_hooks_installed(tmp_path)

        assert all(not installed for installed in results.values())

    def test_detects_installed_c4_hooks(self, tmp_path):
        """Should detect installed C4 hooks."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()

        # Install hooks
        git_hooks.install_all_hooks(tmp_path)

        results = git_hooks.check_hooks_installed(tmp_path)

        assert results["pre-commit"] is True
        assert results["commit-msg"] is True
        assert results["post-commit"] is True

    def test_does_not_detect_non_c4_hooks(self, tmp_path):
        """Should not detect non-C4 hooks as installed."""
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        hooks_dir = git_dir / "hooks"
        hooks_dir.mkdir()

        # Create non-C4 hook
        hook_path = hooks_dir / "pre-commit"
        hook_path.write_text("#!/bin/bash\necho 'My custom hook'")

        results = git_hooks.check_hooks_installed(tmp_path)

        assert results["pre-commit"] is False
        assert results["commit-msg"] is False
        assert results["post-commit"] is False
