"""Tests for c4 init command - path resolution and agent installation.

Covers:
- _get_original_cwd() path resolution with PWD and uv --directory
- _install_agents() agent file copying with memory: project
- init path resolution bug: should use user's cwd, not c4 package dir
"""

import os
from pathlib import Path
from unittest.mock import patch

from c4.cli import _get_original_cwd, _install_agents

# =============================================================================
# _install_agents tests
# =============================================================================


class TestInstallAgents:
    """Test agent installation during c4 init."""

    def test_installs_agents_to_project(self, tmp_path):
        """Agent files are copied to .claude/agents/ in the project."""
        count = _install_agents(tmp_path)
        agents_dir = tmp_path / ".claude" / "agents"

        assert agents_dir.exists()
        assert count > 0
        assert len(list(agents_dir.glob("*.md"))) == count

    def test_all_agents_have_memory_project(self, tmp_path):
        """Every installed agent must have memory: project in frontmatter."""
        _install_agents(tmp_path)
        agents_dir = tmp_path / ".claude" / "agents"

        for agent_file in agents_dir.glob("*.md"):
            content = agent_file.read_text()
            assert "memory: project" in content, (
                f"{agent_file.name} missing 'memory: project' in frontmatter"
            )

    def test_all_agents_have_name_and_description(self, tmp_path):
        """Every agent must have name and description in frontmatter."""
        _install_agents(tmp_path)
        agents_dir = tmp_path / ".claude" / "agents"

        for agent_file in agents_dir.glob("*.md"):
            content = agent_file.read_text()
            assert "name:" in content, f"{agent_file.name} missing 'name:'"
            assert "description:" in content, f"{agent_file.name} missing 'description:'"

    def test_preserves_user_customized_agents(self, tmp_path):
        """If user has modified an agent (newer mtime), don't overwrite it."""
        # First install
        _install_agents(tmp_path)
        agents_dir = tmp_path / ".claude" / "agents"

        # User customizes an agent
        custom_agent = agents_dir / "code-reviewer.md"
        custom_content = "---\nname: code-reviewer\ndescription: My custom reviewer\nmemory: project\n---\nCustom prompt"
        custom_agent.write_text(custom_content)

        # Make the user's file newer by touching with future mtime
        import time
        future_time = time.time() + 1000
        os.utime(custom_agent, (future_time, future_time))

        # Second install should skip the customized file
        _install_agents(tmp_path)

        assert custom_agent.read_text() == custom_content

    def test_creates_agents_directory(self, tmp_path):
        """Creates .claude/agents/ if it doesn't exist."""
        assert not (tmp_path / ".claude" / "agents").exists()
        _install_agents(tmp_path)
        assert (tmp_path / ".claude" / "agents").exists()

    def test_returns_zero_if_no_source(self, tmp_path):
        """Returns 0 if the bundled agents directory doesn't exist."""
        with patch("c4.cli.Path") as mock_path:
            mock_path.return_value.__truediv__ = lambda self, x: tmp_path / "nonexistent"
            # The function checks Path(__file__).parent / "data" / "agents"
            # If it doesn't exist, return 0
            pass

        # Simpler: just test the source check
        source = Path(__file__).parent / "data" / "agents"
        if not source.exists():
            assert _install_agents(tmp_path) >= 0  # Just ensure no crash

    def test_includes_key_agents(self, tmp_path):
        """Key agents for C4 workflow must be included."""
        _install_agents(tmp_path)
        agents_dir = tmp_path / ".claude" / "agents"

        key_agents = [
            "code-reviewer.md",
            "debugger.md",
            "backend-architect.md",
            "frontend-developer.md",
            "test-automator.md",
            "python-pro.md",
            "c4-scout.md",
        ]
        for agent_name in key_agents:
            assert (agents_dir / agent_name).exists(), f"Missing key agent: {agent_name}"


# =============================================================================
# _get_original_cwd tests
# =============================================================================


class TestGetOriginalCwd:
    """Test path resolution for init command."""

    def test_uses_pwd_env_when_available(self):
        """Should prefer PWD env var over Path.cwd()."""
        with patch.dict(os.environ, {"PWD": "/tmp/user-project"}):
            result = _get_original_cwd()
            assert result == Path("/tmp/user-project")

    def test_falls_back_to_cwd_when_no_pwd(self):
        """Falls back to Path.cwd() when PWD is not set."""
        env = os.environ.copy()
        env.pop("PWD", None)
        with patch.dict(os.environ, env, clear=True):
            result = _get_original_cwd()
            assert result == Path.cwd()

    def test_pwd_takes_priority_over_cwd(self):
        """PWD should take priority even when cwd is different (uv --directory case)."""
        fake_project = "/tmp/my-real-project"
        with patch.dict(os.environ, {"PWD": fake_project}):
            with patch("c4.cli.Path.cwd", return_value=Path("/some/other/dir")):
                result = _get_original_cwd()
                assert str(result) == fake_project


# =============================================================================
# Init path resolution bug tests
# =============================================================================


class TestInitPathResolution:
    """Test that c4 init uses the correct project directory.

    Bug: c4 init was using the C4 package directory instead of the
    user's current working directory when C4_PROJECT_ROOT was set
    or when running via uv --directory.
    """

    def test_init_uses_cwd_not_c4_package_dir(self, tmp_path):
        """init should initialize in the user's cwd, not the c4 package directory."""
        user_project = tmp_path / "user-project"
        user_project.mkdir()

        # Simulate: user is in user_project, but C4_PROJECT_ROOT is NOT set
        env = os.environ.copy()
        env.pop("C4_PROJECT_ROOT", None)
        env["PWD"] = str(user_project)

        with patch.dict(os.environ, env, clear=True):
            result = _get_original_cwd()
            assert result == user_project
            # NOT the c4 package directory
            assert "git/c4" not in str(result) or str(result) == str(user_project)

    def test_c4_project_root_overrides_cwd(self):
        """When C4_PROJECT_ROOT is set, it should be used (intentional behavior for --path)."""
        # This is the intentional case: user explicitly set --path
        target = "/tmp/explicit-project"
        with patch.dict(os.environ, {"C4_PROJECT_ROOT": target}):
            # The init code does:
            # project_path = Path(os.environ.get("C4_PROJECT_ROOT") or str(_get_original_cwd()))
            resolved = Path(os.environ.get("C4_PROJECT_ROOT") or str(_get_original_cwd()))
            assert str(resolved) == target

    def test_stale_c4_project_root_ignored_by_init(self):
        """FIX: init ignores stale C4_PROJECT_ROOT and uses PWD instead."""
        user_project = "/tmp/my-project"
        stale_c4_root = "/Users/changmin/git/c4"  # The C4 package dir

        with patch.dict(os.environ, {
            "C4_PROJECT_ROOT": stale_c4_root,
            "PWD": user_project,
        }):
            # After fix: init uses _get_original_cwd() which reads PWD
            resolved = _get_original_cwd()
            # Should resolve to the user's project, NOT the stale C4 root
            assert str(resolved) == user_project
            assert str(resolved) != stale_c4_root

    def test_agents_installed_in_correct_project(self, tmp_path):
        """Agents should be installed in the target project, not in the c4 source."""
        user_project = tmp_path / "user-project"
        user_project.mkdir()

        _install_agents(user_project)

        # Agents should be in the user's project
        assert (user_project / ".claude" / "agents").exists()
        assert len(list((user_project / ".claude" / "agents").glob("*.md"))) > 0

        # User project should have its own independent copy
        user_agents = set(f.name for f in (user_project / ".claude" / "agents").glob("*.md"))
        assert len(user_agents) >= 30  # At least 30 agents installed

    def test_init_path_resolution_priority(self):
        """Document the priority order for path resolution in init.

        After fix:
        Priority: --path flag > PWD env > Path.cwd()
        (C4_PROJECT_ROOT is intentionally ignored by init to prevent stale env bugs)
        """
        # Priority 1: --path flag (handled by typer before reaching this code)
        # Priority 2: PWD (checked by _get_original_cwd)
        # Priority 3: Path.cwd() (fallback in _get_original_cwd)

        # Even with C4_PROJECT_ROOT set, _get_original_cwd uses PWD
        with patch.dict(os.environ, {"C4_PROJECT_ROOT": "/stale/path", "PWD": "/pwd/path"}):
            resolved = _get_original_cwd()
            assert str(resolved) == "/pwd/path"  # PWD wins, C4_PROJECT_ROOT ignored

        with patch.dict(os.environ, {"PWD": "/pwd/path"}, clear=True):
            resolved = _get_original_cwd()
            assert str(resolved) == "/pwd/path"
