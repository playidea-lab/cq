"""Tests for install.sh script."""

import subprocess
from pathlib import Path

import pytest


@pytest.fixture
def install_script() -> Path:
    """Get path to install.sh."""
    return Path(__file__).parent.parent.parent / "install.sh"


class TestInstallShellScript:
    """Test install.sh shell script."""

    def test_script_exists(self, install_script: Path) -> None:
        """Verify install.sh exists."""
        assert install_script.exists(), "install.sh not found"

    def test_script_syntax(self, install_script: Path) -> None:
        """Verify shell script has valid syntax."""
        result = subprocess.run(
            ["bash", "-n", str(install_script)],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0, f"Syntax error: {result.stderr}"

    def test_script_is_executable(self, install_script: Path) -> None:
        """Verify script has proper shebang."""
        content = install_script.read_text()
        assert content.startswith("#!/bin/bash"), "Missing bash shebang"

    def test_git_check_section_exists(self, install_script: Path) -> None:
        """Verify Git check section is present."""
        content = install_script.read_text()
        assert "Checking Git" in content, "Git check section missing"
        assert "command -v git" in content, "Git command check missing"

    def test_git_install_functions_exist(self, install_script: Path) -> None:
        """Verify OS-specific Git install logic exists."""
        content = install_script.read_text()
        # Check for OS-specific install commands
        assert "brew install git" in content, "macOS Git install missing"
        assert "apt-get" in content, "Debian Git install missing"
        assert "dnf install" in content or "yum install" in content, "RHEL Git install missing"
        assert "pacman" in content, "Arch Git install missing"

    def test_git_error_messages_exist(self, install_script: Path) -> None:
        """Verify error messages for Git installation failure."""
        content = install_script.read_text()
        assert "ERROR:" in content, "Error messages missing"
        assert "Git is required" in content, "Git requirement message missing"

    def test_all_steps_present(self, install_script: Path) -> None:
        """Verify all installation steps are present."""
        content = install_script.read_text()
        # Check for all 9 steps (0-8)
        assert "[0/8] Checking Git" in content, "Step 0 missing"
        assert "[1/8] Installing dependencies" in content, "Step 1 missing"
        assert "[2/8] Saving install path" in content, "Step 2 missing"
        assert "[3/8] Creating global 'c4' command" in content, "Step 3 missing"
        assert "[4/8] Installing Claude Code slash commands" in content, "Step 4 missing"
        assert "[5/8] Installing Cursor commands" in content, "Step 5 missing"
        assert "[6/8] Installing Gemini CLI slash commands" in content, "Step 6 missing"
        assert "[7/8] Installing Claude Code hooks" in content, "Step 7 missing"
        assert "[8/8] Registering hooks" in content, "Step 8 missing"


class TestGitDetection:
    """Test Git detection scenarios."""

    def test_git_available(self) -> None:
        """Test that git command is available on this system."""
        result = subprocess.run(
            ["git", "--version"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0, "Git not available"
        assert "git version" in result.stdout.lower()

    def test_git_check_command(self) -> None:
        """Test git availability check command."""
        result = subprocess.run(
            ["bash", "-c", "command -v git && echo 'found'"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "found" in result.stdout
