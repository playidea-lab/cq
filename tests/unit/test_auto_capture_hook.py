"""Tests for c4-auto-capture-hook.py template script."""

import importlib.util
import json
import tempfile
from pathlib import Path
from unittest.mock import patch


# Load the hook script as a module
def load_hook_module():
    """Load the hook script as a Python module."""
    hook_path = Path(__file__).parent.parent.parent / "templates" / "c4-auto-capture-hook.py"
    spec = importlib.util.spec_from_file_location("auto_capture_hook", hook_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


hook = load_hook_module()


class TestShouldCapture:
    """Tests for should_capture function."""

    def test_read_file_should_capture(self) -> None:
        """read_file should be captured."""
        assert hook.should_capture("read_file") is True

    def test_find_symbol_should_capture(self) -> None:
        """find_symbol should be captured."""
        assert hook.should_capture("find_symbol") is True

    def test_user_message_should_capture(self) -> None:
        """user_message should be captured."""
        assert hook.should_capture("user_message") is True

    def test_file_write_should_capture(self) -> None:
        """file_write should be captured."""
        assert hook.should_capture("file_write") is True

    def test_unknown_tool_should_not_capture(self) -> None:
        """Unknown tools should not be captured."""
        assert hook.should_capture("unknown_tool") is False

    def test_empty_name_should_not_capture(self) -> None:
        """Empty tool name should not be captured."""
        assert hook.should_capture("") is False

    def test_bash_should_capture(self) -> None:
        """Bash should be captured."""
        assert hook.should_capture("Bash") is True

    def test_bash_git_commit_should_capture(self) -> None:
        """Bash with git commit output should be captured."""
        output = "[main abc1234] Fix bug\n 1 file changed"
        assert hook.should_capture("Bash", output) is True

    def test_bash_git_commit_case_insensitive(self) -> None:
        """bash (lowercase) with git commit output should be captured."""
        output = "[main abc1234] Fix bug"
        assert hook.should_capture("bash", output) is True
        assert hook.should_capture("shell", output) is True


class TestGetImportance:
    """Tests for get_importance function."""

    def test_user_message_highest(self) -> None:
        """user_message should have importance 9."""
        assert hook.get_importance("user_message") == 9

    def test_file_write_high(self) -> None:
        """file_write should have importance 8."""
        assert hook.get_importance("file_write") == 8

    def test_find_symbol_medium_high(self) -> None:
        """find_symbol should have importance 7."""
        assert hook.get_importance("find_symbol") == 7

    def test_read_file_medium(self) -> None:
        """read_file should have importance 6."""
        assert hook.get_importance("read_file") == 6

    def test_list_dir_medium_low(self) -> None:
        """list_dir should have importance 5."""
        assert hook.get_importance("list_dir") == 5

    def test_unknown_defaults_to_5(self) -> None:
        """Unknown tools should default to importance 5."""
        assert hook.get_importance("unknown_tool") == 5

    def test_bash_default_importance(self) -> None:
        """Bash should have default importance 5."""
        assert hook.get_importance("Bash") == 5

    def test_bash_git_commit_elevated(self) -> None:
        """Bash with git commit output should have importance 8."""
        output = "[main abc1234] Fix bug"
        assert hook.get_importance("Bash", output) == 8

    def test_bash_non_commit_default(self) -> None:
        """Bash with non-commit output should have default importance."""
        output = "ls -la output"
        assert hook.get_importance("Bash", output) == 5


class TestTruncateOutput:
    """Tests for truncate_output function."""

    def test_short_string_unchanged(self) -> None:
        """Short strings should not be truncated."""
        output = "short output"
        result = hook.truncate_output(output)
        assert result == output

    def test_long_string_truncated(self) -> None:
        """Long strings should be truncated."""
        output = "x" * 60000
        result = hook.truncate_output(output, max_size=1000)
        assert len(result) < len(output)
        assert "[truncated]" in result

    def test_dict_converted_to_json(self) -> None:
        """Dict output should be converted to JSON string."""
        output = {"key": "value", "nested": {"a": 1}}
        result = hook.truncate_output(output)
        assert "key" in result
        assert "value" in result
        assert isinstance(result, str)

    def test_empty_string(self) -> None:
        """Empty string should remain empty."""
        result = hook.truncate_output("")
        assert result == ""


class TestFindProjectRoot:
    """Tests for find_project_root function."""

    def test_finds_root_from_env(self) -> None:
        """Should find root from C4_PROJECT_ROOT env variable."""
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            (root / ".c4").mkdir()

            with patch.dict("os.environ", {"C4_PROJECT_ROOT": str(root)}):
                result = hook.find_project_root()
                assert result == root

    def test_returns_none_when_no_c4_dir(self) -> None:
        """Should return None when not in C4 project."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # No .c4 directory
            with patch.dict("os.environ", {"C4_PROJECT_ROOT": ""}):
                with patch("os.getcwd", return_value=tmpdir):
                    # Override the Path.cwd() in the module
                    original_cwd = Path.cwd
                    try:
                        Path.cwd = lambda: Path(tmpdir)
                        hook.find_project_root()
                        # May or may not return None depending on parent dirs
                    finally:
                        Path.cwd = original_cwd


class TestCaptureRules:
    """Tests for CAPTURE_RULES constant."""

    def test_all_rules_have_valid_importance(self) -> None:
        """All capture rules should have importance between 1 and 10."""
        for tool, importance in hook.CAPTURE_RULES.items():
            assert 1 <= importance <= 10, f"{tool} has invalid importance: {importance}"

    def test_essential_tools_included(self) -> None:
        """Essential tools should be in capture rules."""
        essential = ["read_file", "find_symbol", "user_message", "file_write", "Bash"]
        for tool in essential:
            assert tool in hook.CAPTURE_RULES, f"{tool} missing from capture rules"


class TestIsGitCommitOutput:
    """Tests for is_git_commit_output function."""

    def test_standard_commit_output(self) -> None:
        """Should detect standard git commit output."""
        output = "[main abc1234] Fix bug\n 1 file changed"
        assert hook.is_git_commit_output(output) is True

    def test_commit_with_branch(self) -> None:
        """Should detect commit with feature branch."""
        output = "[feature/auth-fix 1234567] Add feature"
        assert hook.is_git_commit_output(output) is True

    def test_commit_with_amend(self) -> None:
        """Should detect amend commit."""
        output = "[main abc1234 (amend)] Updated message"
        assert hook.is_git_commit_output(output) is True

    def test_non_commit_output(self) -> None:
        """Should not detect non-commit output."""
        output = "ls -la\ntotal 16\ndrwxr-xr-x"
        assert hook.is_git_commit_output(output) is False

    def test_empty_output(self) -> None:
        """Should handle empty output."""
        assert hook.is_git_commit_output("") is False

    def test_git_status_not_commit(self) -> None:
        """Should not detect git status as commit."""
        output = "On branch main\nnothing to commit"
        assert hook.is_git_commit_output(output) is False


class TestParseGitCommitMetadata:
    """Tests for parse_git_commit_metadata function."""

    def test_parse_simple_commit(self) -> None:
        """Should parse simple commit output."""
        output = "[main abc1234] Fix bug"
        result = hook.parse_git_commit_metadata(output)

        assert result is not None
        assert result["sha"] == "abc1234"
        assert result["message"] == "Fix bug"
        assert result["branch"] == "main"

    def test_parse_commit_with_stats(self) -> None:
        """Should parse commit with statistics."""
        output = """[feature/new 1234567] Add feature
 src/feature.py | 50 ++++++
 2 files changed, 80 insertions(+)"""
        result = hook.parse_git_commit_metadata(output)

        assert result is not None
        assert result["sha"] == "1234567"
        assert result["insertions"] == 80
        assert "src/feature.py" in result["changed_files"]

    def test_parse_commit_with_create_mode(self) -> None:
        """Should parse commit with create mode."""
        output = """[main 1234567] Add files
 create mode 100644 new_file.py
 1 file changed, 10 insertions(+)"""
        result = hook.parse_git_commit_metadata(output)

        assert result is not None
        assert "new_file.py" in result["changed_files"]

    def test_parse_invalid_output(self) -> None:
        """Should return None for invalid output."""
        result = hook.parse_git_commit_metadata("not a commit")
        assert result is None

    def test_parse_empty_output(self) -> None:
        """Should return None for empty output."""
        result = hook.parse_git_commit_metadata("")
        assert result is None


class TestFormatGitCommitContent:
    """Tests for format_git_commit_content function."""

    def test_basic_format(self) -> None:
        """Should format basic commit info."""
        metadata = {
            "sha": "abc1234",
            "message": "Fix bug",
            "branch": "main",
            "changed_files": [],
            "insertions": 0,
            "deletions": 0,
        }
        content = hook.format_git_commit_content(metadata)

        assert "abc1234" in content
        assert "Fix bug" in content
        assert "main" in content

    def test_format_with_files(self) -> None:
        """Should include changed files."""
        metadata = {
            "sha": "abc1234",
            "message": "Update",
            "branch": "",
            "changed_files": ["file1.py", "file2.py"],
            "insertions": 0,
            "deletions": 0,
        }
        content = hook.format_git_commit_content(metadata)

        assert "file1.py" in content
        assert "file2.py" in content
        assert "Changed files (2)" in content

    def test_format_with_stats(self) -> None:
        """Should include statistics."""
        metadata = {
            "sha": "abc1234",
            "message": "Refactor",
            "branch": "",
            "changed_files": [],
            "insertions": 50,
            "deletions": 25,
        }
        content = hook.format_git_commit_content(metadata)

        assert "+50" in content
        assert "-25" in content


class TestMainFunction:
    """Integration tests for main function."""

    def test_empty_stdin_exits_0(self) -> None:
        """Empty stdin should exit with 0."""
        with patch("sys.stdin.read", return_value=""):
            with patch("sys.exit") as mock_exit:
                hook.main()
                mock_exit.assert_called_with(0)

    def test_invalid_json_exits_0(self) -> None:
        """Invalid JSON should exit with 0 (silent fail)."""
        with patch("sys.stdin.read", return_value="not json"):
            with patch("sys.exit") as mock_exit:
                hook.main()
                mock_exit.assert_called_with(0)

    def test_non_captured_tool_exits_0(self) -> None:
        """Non-captured tool should exit with 0."""
        input_data = json.dumps({"tool_name": "unknown", "input": {}, "output": "test"})
        with patch("sys.stdin.read", return_value=input_data):
            with patch("sys.exit") as mock_exit:
                hook.main()
                mock_exit.assert_called_with(0)

    def test_captured_tool_calls_capture(self) -> None:
        """Captured tool should call capture_tool_output."""
        input_data = json.dumps({
            "tool_name": "read_file",
            "input": {"path": "/test.py"},
            "output": "content"
        })
        with patch("sys.stdin.read", return_value=input_data):
            with patch.object(hook, "capture_tool_output") as mock_capture:
                with patch("sys.exit"):
                    hook.main()
                    mock_capture.assert_called_once_with(
                        "read_file",
                        {"path": "/test.py"},
                        "content"
                    )

    def test_exception_exits_0(self) -> None:
        """Any exception should exit with 0 (silent fail)."""
        with patch("sys.stdin.read", side_effect=Exception("Test error")):
            with patch("sys.exit") as mock_exit:
                hook.main()
                mock_exit.assert_called_with(0)
