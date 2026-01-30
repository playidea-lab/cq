"""Tests for C4 slash commands installation."""

from pathlib import Path
from unittest.mock import patch

from c4 import commands as c4_commands


class TestGetCommandsDir:
    """Tests for get_commands_dir function."""

    def test_returns_path(self):
        """Should return a Path object."""
        result = c4_commands.get_commands_dir()
        assert isinstance(result, Path)

    def test_returns_commands_directory(self):
        """Should return the commands package directory."""
        result = c4_commands.get_commands_dir()
        assert result.name == "commands"
        assert result.parent.name == "c4"


class TestGetTargetDir:
    """Tests for get_target_dir function."""

    def test_returns_claude_commands_dir(self):
        """Should return ~/.claude/commands/."""
        result = c4_commands.get_target_dir()
        assert result == Path.home() / ".claude" / "commands"


class TestGetCommandHash:
    """Tests for get_command_hash function."""

    def test_returns_12_char_hash(self):
        """Should return a 12-character hash."""
        result = c4_commands.get_command_hash("test content")
        assert len(result) == 12

    def test_same_content_same_hash(self):
        """Same content should produce same hash."""
        hash1 = c4_commands.get_command_hash("test content")
        hash2 = c4_commands.get_command_hash("test content")
        assert hash1 == hash2

    def test_different_content_different_hash(self):
        """Different content should produce different hash."""
        hash1 = c4_commands.get_command_hash("content 1")
        hash2 = c4_commands.get_command_hash("content 2")
        assert hash1 != hash2


class TestShouldUpdate:
    """Tests for should_update function."""

    def test_returns_true_if_target_missing(self, tmp_path):
        """Should return True if target file doesn't exist."""
        target = tmp_path / "nonexistent.md"
        result = c4_commands.should_update("content", target)
        assert result is True

    def test_returns_false_if_same_content(self, tmp_path):
        """Should return False if content is identical."""
        target = tmp_path / "test.md"
        content = "# Test Command\nSome content"
        target.write_text(content)

        result = c4_commands.should_update(content, target)
        assert result is False

    def test_returns_true_if_different_content(self, tmp_path):
        """Should return True if content differs."""
        target = tmp_path / "test.md"
        target.write_text("old content")

        result = c4_commands.should_update("new content", target)
        assert result is True


class TestInstallCommand:
    """Tests for install_command function."""

    def test_installs_to_target_dir(self, tmp_path):
        """Should install command to target directory."""
        # Create mock source command
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "test-cmd.md").write_text("# Test Command")

        target_dir = tmp_path / "target"

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                success, msg = c4_commands.install_command("test-cmd.md")

        assert success is True
        assert "Installed:" in msg
        assert (target_dir / "test-cmd.md").exists()
        assert (target_dir / "test-cmd.md").read_text() == "# Test Command"

    def test_creates_target_dir_if_missing(self, tmp_path):
        """Should create target directory if it doesn't exist."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "test-cmd.md").write_text("# Test")

        target_dir = tmp_path / "nested" / "target"

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                success, _ = c4_commands.install_command("test-cmd.md")

        assert success is True
        assert target_dir.exists()

    def test_skips_if_already_up_to_date(self, tmp_path):
        """Should skip if file is already up to date."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "test-cmd.md").write_text("# Same Content")

        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "test-cmd.md").write_text("# Same Content")

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                success, msg = c4_commands.install_command("test-cmd.md")

        assert success is True
        assert "Already up to date" in msg

    def test_overwrites_with_force(self, tmp_path):
        """Should overwrite existing file with force=True."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "test-cmd.md").write_text("# New Content")

        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "test-cmd.md").write_text("# Same Content")

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                success, msg = c4_commands.install_command("test-cmd.md", force=True)

        assert success is True
        assert "Installed:" in msg
        assert (target_dir / "test-cmd.md").read_text() == "# New Content"

    def test_returns_error_for_missing_source(self, tmp_path):
        """Should return error if source file doesn't exist."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            success, msg = c4_commands.install_command("nonexistent.md")

        assert success is False
        assert "not found" in msg


class TestInstallAllCommands:
    """Tests for install_all_commands function."""

    def test_installs_all_core_commands(self, tmp_path):
        """Should install all core commands."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        # Create all core command files
        for cmd in c4_commands.CORE_COMMANDS:
            (source_dir / cmd).write_text(f"# {cmd}")

        target_dir = tmp_path / "target"

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                results = c4_commands.install_all_commands()

        assert len(results) == len(c4_commands.CORE_COMMANDS)
        assert all(success for success, _ in results.values())

    def test_can_specify_custom_commands(self, tmp_path):
        """Should allow specifying custom command list."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "custom-cmd.md").write_text("# Custom")

        target_dir = tmp_path / "target"

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                results = c4_commands.install_all_commands(commands=["custom-cmd.md"])

        assert len(results) == 1
        assert "custom-cmd.md" in results


class TestUninstallCommand:
    """Tests for uninstall_command function."""

    def test_removes_c4_command(self, tmp_path):
        """Should remove C4 command file."""
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "c4-test.md").write_text("# C4 Test Command")

        with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
            success, msg = c4_commands.uninstall_command("c4-test.md")

        assert success is True
        assert "Uninstalled:" in msg
        assert not (target_dir / "c4-test.md").exists()

    def test_skips_non_c4_command(self, tmp_path):
        """Should not remove files that don't look like C4 commands."""
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "other.md").write_text("# Not a related command")

        with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
            success, msg = c4_commands.uninstall_command("other.md")

        assert success is False
        assert "Not a C4 command" in msg
        assert (target_dir / "other.md").exists()

    def test_succeeds_if_not_installed(self, tmp_path):
        """Should succeed if file doesn't exist."""
        target_dir = tmp_path / "target"
        target_dir.mkdir()

        with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
            success, msg = c4_commands.uninstall_command("nonexistent.md")

        assert success is True
        assert "Not installed" in msg


class TestGetCommandStatus:
    """Tests for get_command_status function."""

    def test_returns_status_for_all_commands(self, tmp_path):
        """Should return status for all core commands."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        for cmd in c4_commands.CORE_COMMANDS:
            (source_dir / cmd).write_text(f"# {cmd}")

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            status = c4_commands.get_command_status()

        assert len(status) == len(c4_commands.CORE_COMMANDS)

    def test_shows_installed_status(self, tmp_path):
        """Should correctly identify installed commands."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "c4-plan.md").write_text("# Plan")

        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "c4-plan.md").write_text("# Plan")

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                with patch.object(c4_commands, "CORE_COMMANDS", ["c4-plan.md"]):
                    status = c4_commands.get_command_status()

        assert status["c4-plan.md"]["installed"] is True
        assert status["c4-plan.md"]["up_to_date"] is True

    def test_shows_outdated_status(self, tmp_path):
        """Should correctly identify outdated commands."""
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "c4-plan.md").write_text("# New Content")

        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / "c4-plan.md").write_text("# Old Content")

        with patch.object(c4_commands, "get_commands_dir", return_value=source_dir):
            with patch.object(c4_commands, "get_target_dir", return_value=target_dir):
                with patch.object(c4_commands, "CORE_COMMANDS", ["c4-plan.md"]):
                    status = c4_commands.get_command_status()

        assert status["c4-plan.md"]["installed"] is True
        assert status["c4-plan.md"]["up_to_date"] is False


class TestCoreCommands:
    """Tests for CORE_COMMANDS constant."""

    def test_includes_essential_commands(self):
        """Should include all essential C4 commands."""
        essential = [
            "c4-init.md",
            "c4-plan.md",
            "c4-run.md",
            "c4-status.md",
            "c4-swarm.md",
        ]
        for cmd in essential:
            assert cmd in c4_commands.CORE_COMMANDS

    def test_all_commands_exist_in_package(self):
        """All listed commands should exist in the package."""
        commands_dir = c4_commands.get_commands_dir()
        for cmd in c4_commands.CORE_COMMANDS:
            assert (commands_dir / cmd).exists(), f"Missing command file: {cmd}"
