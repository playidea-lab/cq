"""Tests for git commit capture rules."""


from c4.memory.capture_rules import (
    CAPTURE_RULES,
    GitCommitMetadata,
    extract_changed_files_from_diff,
    format_commit_content,
    get_capture_importance,
    is_git_commit_output,
    parse_git_commit,
)


class TestIsGitCommitOutput:
    """Tests for is_git_commit_output function."""

    def test_standard_commit_output(self) -> None:
        """Should detect standard git commit output."""
        output = "[main abc1234] Fix bug in authentication\n 1 file changed, 5 insertions(+)"
        assert is_git_commit_output("Bash", output) is True

    def test_commit_with_branch_name(self) -> None:
        """Should detect commit output with various branch names."""
        output = "[feature/auth-fix 1234567] Add new feature"
        assert is_git_commit_output("Bash", output) is True

    def test_commit_with_amend(self) -> None:
        """Should detect amend commit output."""
        output = "[main abc1234 (amend)] Updated commit message"
        assert is_git_commit_output("Bash", output) is True

    def test_commit_with_stats(self) -> None:
        """Should detect commit with file change statistics."""
        output = """[main def5678] Refactor module
         3 files changed, 50 insertions(+), 20 deletions(-)"""
        assert is_git_commit_output("Bash", output) is True

    def test_non_git_output(self) -> None:
        """Should not detect non-git output."""
        output = "total 16\ndrwxr-xr-x  4 user  staff  128 Jan  1 12:00 ."
        assert is_git_commit_output("Bash", output) is False

    def test_non_shell_tool(self) -> None:
        """Should not detect git output from non-shell tools."""
        output = "[main abc1234] Fix bug"
        assert is_git_commit_output("read_file", output) is False

    def test_empty_output(self) -> None:
        """Should handle empty output."""
        assert is_git_commit_output("Bash", "") is False

    def test_shell_tool_lowercase(self) -> None:
        """Should work with lowercase tool names."""
        output = "[main abc1234] Fix bug"
        assert is_git_commit_output("bash", output) is True
        assert is_git_commit_output("shell", output) is True

    def test_git_status_not_detected(self) -> None:
        """Should not detect git status output as commit."""
        output = """On branch main
Your branch is up to date with 'origin/main'.

nothing to commit, working tree clean"""
        assert is_git_commit_output("Bash", output) is False


class TestParseGitCommit:
    """Tests for parse_git_commit function."""

    def test_parse_simple_commit(self) -> None:
        """Should parse simple commit output."""
        output = "[main abc1234] Fix authentication bug"
        result = parse_git_commit(output)

        assert result is not None
        assert result.sha == "abc1234"
        assert result.message == "Fix authentication bug"
        assert result.branch == "main"

    def test_parse_commit_with_stats(self) -> None:
        """Should parse commit with file statistics."""
        output = """[feature/new 1234567] Add new feature
 src/feature.py | 50 ++++++++++++++++++++++++++++++++++
 tests/test_feature.py | 30 ++++++++++++++++++++
 2 files changed, 80 insertions(+)"""
        result = parse_git_commit(output)

        assert result is not None
        assert result.sha == "1234567"
        assert result.message == "Add new feature"
        assert result.insertions == 80
        assert result.deletions == 0
        assert "src/feature.py" in result.changed_files
        assert "tests/test_feature.py" in result.changed_files

    def test_parse_commit_with_deletions(self) -> None:
        """Should parse commit with insertions and deletions."""
        output = """[main abcdef0] Refactor code
 3 files changed, 25 insertions(+), 15 deletions(-)"""
        result = parse_git_commit(output)

        assert result is not None
        assert result.insertions == 25
        assert result.deletions == 15

    def test_parse_commit_with_create_mode(self) -> None:
        """Should parse commit with create mode lines."""
        output = """[main 1234567] Add new files
 create mode 100644 new_file.py
 create mode 100644 another_file.py
 2 files changed, 100 insertions(+)"""
        result = parse_git_commit(output)

        assert result is not None
        assert "new_file.py" in result.changed_files
        assert "another_file.py" in result.changed_files

    def test_parse_empty_output(self) -> None:
        """Should return None for empty output."""
        result = parse_git_commit("")
        assert result is None

    def test_parse_invalid_output(self) -> None:
        """Should return None for non-commit output."""
        result = parse_git_commit("This is not a git commit output")
        assert result is None

    def test_parse_full_sha(self) -> None:
        """Should parse full 40-character SHA."""
        output = "[main 1234567890abcdef1234567890abcdef12345678] Full SHA commit"
        result = parse_git_commit(output)

        assert result is not None
        assert result.sha == "1234567890abcdef1234567890abcdef12345678"

    def test_parse_commit_with_author(self) -> None:
        """Should extract author from input command."""
        output = "[main abc1234] Commit message"
        input_cmd = 'git commit --author="John Doe <john@example.com>" -m "msg"'
        result = parse_git_commit(output, input_cmd)

        assert result is not None
        assert result.author == "John Doe <john@example.com>"

    def test_parse_branch_with_slashes(self) -> None:
        """Should handle branch names with slashes."""
        output = "[feature/auth/oauth2 abc1234] OAuth2 implementation"
        result = parse_git_commit(output)

        assert result is not None
        assert result.branch == "feature/auth/oauth2"


class TestGitCommitMetadata:
    """Tests for GitCommitMetadata dataclass."""

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        metadata = GitCommitMetadata(
            sha="abc1234",
            message="Test commit",
            branch="main",
            changed_files=["file1.py", "file2.py"],
            insertions=10,
            deletions=5,
            author="Test Author",
        )

        result = metadata.to_dict()

        assert result["sha"] == "abc1234"
        assert result["message"] == "Test commit"
        assert result["branch"] == "main"
        assert result["changed_files"] == ["file1.py", "file2.py"]
        assert result["insertions"] == 10
        assert result["deletions"] == 5
        assert result["author"] == "Test Author"

    def test_default_values(self) -> None:
        """Should have sensible defaults."""
        metadata = GitCommitMetadata(sha="abc", message="msg")

        assert metadata.branch == ""
        assert metadata.changed_files == []
        assert metadata.insertions == 0
        assert metadata.deletions == 0
        assert metadata.author == ""
        assert metadata.raw_diff == ""


class TestExtractChangedFilesFromDiff:
    """Tests for extract_changed_files_from_diff function."""

    def test_single_file(self) -> None:
        """Should extract single file from diff."""
        diff = """diff --git a/foo.py b/foo.py
index 123..456 789
--- a/foo.py
+++ b/foo.py
@@ -1,3 +1,4 @@
+# New line
 old content"""
        files = extract_changed_files_from_diff(diff)
        assert files == ["foo.py"]

    def test_multiple_files(self) -> None:
        """Should extract multiple files from diff."""
        diff = """diff --git a/foo.py b/foo.py
+++ b/foo.py
diff --git a/bar.py b/bar.py
+++ b/bar.py
diff --git a/baz/qux.py b/baz/qux.py
+++ b/baz/qux.py"""
        files = extract_changed_files_from_diff(diff)
        assert len(files) == 3
        assert "foo.py" in files
        assert "bar.py" in files
        assert "baz/qux.py" in files

    def test_empty_diff(self) -> None:
        """Should return empty list for empty diff."""
        assert extract_changed_files_from_diff("") == []

    def test_no_duplicates(self) -> None:
        """Should not include duplicate files."""
        diff = """diff --git a/foo.py b/foo.py
+++ b/foo.py
diff --git a/foo.py b/foo.py
+++ b/foo.py"""
        files = extract_changed_files_from_diff(diff)
        assert files == ["foo.py"]


class TestFormatCommitContent:
    """Tests for format_commit_content function."""

    def test_basic_format(self) -> None:
        """Should format basic commit info."""
        metadata = GitCommitMetadata(
            sha="abc1234",
            message="Fix critical bug",
            branch="main",
        )
        content = format_commit_content(metadata)

        assert "abc1234" in content
        assert "Fix critical bug" in content
        assert "main" in content

    def test_format_with_files(self) -> None:
        """Should include changed files."""
        metadata = GitCommitMetadata(
            sha="abc1234",
            message="Update files",
            changed_files=["file1.py", "file2.py"],
        )
        content = format_commit_content(metadata)

        assert "file1.py" in content
        assert "file2.py" in content
        assert "Changed files (2)" in content

    def test_format_with_stats(self) -> None:
        """Should include statistics."""
        metadata = GitCommitMetadata(
            sha="abc1234",
            message="Refactor",
            insertions=50,
            deletions=25,
        )
        content = format_commit_content(metadata)

        assert "+50" in content
        assert "-25" in content

    def test_format_truncates_many_files(self) -> None:
        """Should truncate when many files changed."""
        metadata = GitCommitMetadata(
            sha="abc1234",
            message="Big change",
            changed_files=[f"file{i}.py" for i in range(30)],
        )
        content = format_commit_content(metadata)

        assert "... and 10 more" in content


class TestCaptureRules:
    """Tests for CAPTURE_RULES constant."""

    def test_git_commit_in_rules(self) -> None:
        """Should have git_commit in capture rules."""
        assert "git_commit" in CAPTURE_RULES
        assert CAPTURE_RULES["git_commit"] == 8

    def test_bash_in_rules(self) -> None:
        """Should have Bash in capture rules."""
        assert "Bash" in CAPTURE_RULES

    def test_all_rules_valid_importance(self) -> None:
        """All rules should have valid importance."""
        for tool, importance in CAPTURE_RULES.items():
            assert 1 <= importance <= 10, f"{tool} has invalid importance"


class TestGetCaptureImportance:
    """Tests for get_capture_importance function."""

    def test_known_tool(self) -> None:
        """Should return correct importance for known tools."""
        assert get_capture_importance("read_file") == 6
        assert get_capture_importance("file_write") == 8
        assert get_capture_importance("user_message") == 9

    def test_unknown_tool(self) -> None:
        """Should return default 5 for unknown tools."""
        assert get_capture_importance("unknown_tool") == 5

    def test_bash_git_commit_elevated(self) -> None:
        """Should elevate Bash importance for git commits."""
        output = "[main abc1234] Fix bug"
        importance = get_capture_importance("Bash", output)
        assert importance == 8

    def test_bash_non_commit_default(self) -> None:
        """Should use default importance for non-commit Bash output."""
        output = "ls -la output"
        importance = get_capture_importance("Bash", output)
        assert importance == 5

    def test_case_insensitive_bash(self) -> None:
        """Should handle bash tool name case-insensitively."""
        output = "[main abc1234] Fix bug"
        assert get_capture_importance("bash", output) == 8
        assert get_capture_importance("BASH", output) == 8  # Case-insensitive check


class TestIntegration:
    """Integration tests for git capture workflow."""

    def test_full_capture_workflow(self) -> None:
        """Test complete workflow of detecting and parsing git commit."""
        # Simulated git commit output
        output = """[c4/w-T-GIT-001-0 abc1234] Add git commit capture to PostToolUse hook

 c4/memory/capture_rules.py | 200 +++++++++++++++++++++++++++++++++++
 tests/unit/memory/test_git_capture.py | 150 ++++++++++++++++++++++++++
 2 files changed, 350 insertions(+)
 create mode 100644 c4/memory/capture_rules.py
 create mode 100644 tests/unit/memory/test_git_capture.py"""

        # Step 1: Detect git commit
        assert is_git_commit_output("Bash", output) is True

        # Step 2: Parse commit
        metadata = parse_git_commit(output)
        assert metadata is not None
        assert metadata.sha == "abc1234"
        assert metadata.branch == "c4/w-T-GIT-001-0"
        assert "Add git commit capture" in metadata.message

        # Step 3: Check files
        assert len(metadata.changed_files) >= 2
        assert any("capture_rules.py" in f for f in metadata.changed_files)

        # Step 4: Format content
        content = format_commit_content(metadata)
        assert "abc1234" in content
        assert "capture_rules.py" in content

        # Step 5: Get importance
        importance = get_capture_importance("Bash", output)
        assert importance == 8  # Git commits are high importance
