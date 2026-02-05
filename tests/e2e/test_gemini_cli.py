
from unittest.mock import patch

from typer.testing import CliRunner

from c4.cli import c4_app

runner = CliRunner()

def test_gemini_command_missing_executable():
    """Test 'c4 gemini' when 'gemini' executable is missing."""
    with patch("shutil.which", return_value=None):
        result = runner.invoke(c4_app, ["gemini"])

        assert result.exit_code != 0
        assert "Error: 'gemini' command not found" in result.stdout
        assert "npm install -g @google/gemini-cli" in result.stdout

@patch("c4.commands.gemini_installer.install_gemini_commands")
@patch("c4.cli.os.chdir")
@patch("c4.cli.os.system")
@patch("shutil.which", return_value="/usr/bin/gemini")
def test_gemini_command_success(mock_which, mock_system, mock_chdir, mock_install):
    """Test 'c4 gemini' happy path."""
    # Mock os.system to return 0 (success)
    mock_system.return_value = 0
    # Mock install results
    mock_install.return_value = {"test": (True, "Installed")}

    # We need to mock C4Daemon initialization to avoid real DB/File creation
    with patch("c4.cli.C4Daemon") as MockDaemon:
        # Mock daemon instance
        mock_daemon = MockDaemon.return_value
        mock_daemon.initialize.return_value = None

        # Also mock internal setup functions
        with patch("c4.cli._create_mcp_config"), \
             patch("c4.cli._create_project_settings"), \
             patch("c4.cli.install_all_hooks"), \
             patch("c4.cli.set_platform_config"):

            result = runner.invoke(c4_app, ["gemini", "--init"])

            assert result.exit_code == 0
            assert "Installed 1 C4 commands" in result.stdout
            assert "Starting Gemini..." in result.stdout
            mock_system.assert_called_with("gemini")

@patch("c4.cli.os.system")
@patch("shutil.which", return_value="/usr/bin/gemini")
def test_gemini_command_execution_failure(mock_which, mock_system):
    """Test 'c4 gemini' when gemini exits with error."""
    # Mock os.system to return error code
    mock_system.return_value = 1

    with patch("c4.cli.C4Daemon"), \
            patch("c4.commands.gemini_installer.install_gemini_commands", return_value={}):

        result = runner.invoke(c4_app, ["gemini", "--no-init"])

        assert result.exit_code != 0
        assert "Gemini exited with code 1" in result.stdout
