
import os
import shutil
from pathlib import Path
from unittest.mock import patch, MagicMock
import pytest
from c4.commands.gemini_installer import install_gemini_commands, get_gemini_commands_dir

@pytest.fixture
def mock_home(tmp_path):
    """Mock Path.home() to return a temporary directory."""
    with patch("pathlib.Path.home", return_value=tmp_path):
        yield tmp_path

def test_get_gemini_commands_dir(mock_home):
    """Test getting the Gemini commands directory."""
    expected = mock_home / ".gemini" / "commands"
    assert get_gemini_commands_dir() == expected

def test_install_gemini_commands(mock_home):
    """Test installing Gemini commands."""
    # Run installation
    results = install_gemini_commands()
    
    # Check results
    assert len(results) > 0
    assert all(success for success, _ in results.values())
    
    # Verify files created
    cmd_dir = mock_home / ".gemini" / "commands"
    assert cmd_dir.exists()
    
    # Check a specific command file (e.g., c4-status.toml)
    status_file = cmd_dir / "c4-status.toml"
    assert status_file.exists()
    content = status_file.read_text()
    assert 'description = "Show the current C4 project status."' in content
    assert "prompt =" in content

def test_install_gemini_commands_no_overwrite(mock_home):
    """Test that existing files are not overwritten without force=True."""
    cmd_dir = mock_home / ".gemini" / "commands"
    cmd_dir.mkdir(parents=True)
    
    # Create a dummy file
    status_file = cmd_dir / "c4-status.toml"
    status_file.write_text('description = "Old version"')
    
    # Run installation without force
    results = install_gemini_commands(force=False)
    
    # Check that file was skipped
    assert results["c4-status"][1] == "Already up to date (skipped)"
    assert status_file.read_text() == 'description = "Old version"'

def test_install_gemini_commands_force_overwrite(mock_home):
    """Test that existing files are overwritten with force=True."""
    cmd_dir = mock_home / ".gemini" / "commands"
    cmd_dir.mkdir(parents=True)
    
    # Create a dummy file
    status_file = cmd_dir / "c4-status.toml"
    status_file.write_text('description = "Old version"')
    
    # Run installation with force
    results = install_gemini_commands(force=True)
    
    # Check that file was updated
    assert results["c4-status"][1] == "Installed"
    content = status_file.read_text()
    assert 'description = "Show the current C4 project status."' in content
