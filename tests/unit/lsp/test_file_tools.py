"""Tests for C4 MCP file operation tools."""

from pathlib import Path

import pytest

from c4.lsp.file_tools import FileTools


@pytest.fixture
def project_dir(tmp_path: Path) -> Path:
    """Create a temporary project directory with sample files."""
    # Create sample files
    (tmp_path / "README.md").write_text("# Test Project\n\nThis is a test.")
    (tmp_path / "src").mkdir()
    (tmp_path / "src" / "main.py").write_text(
        "def hello():\n    print('Hello')\n\ndef world():\n    pass\n"
    )
    (tmp_path / "src" / "utils.py").write_text("def util_func():\n    return 42\n")
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_main.py").write_text(
        "def test_hello():\n    assert True\n"
    )
    return tmp_path


@pytest.fixture
def file_tools(project_dir: Path) -> FileTools:
    """Create FileTools instance for the project."""
    return FileTools(project_dir)


class TestReadFile:
    """Tests for read_file method."""

    def test_read_entire_file(self, file_tools: FileTools) -> None:
        """Should read entire file content."""
        result = file_tools.read_file("README.md")
        assert "content" in result
        assert "# Test Project" in result["content"]
        assert result["total_lines"] == 3

    def test_read_with_line_range(self, file_tools: FileTools) -> None:
        """Should read specific line range."""
        result = file_tools.read_file("src/main.py", start_line=0, end_line=1)
        assert result["start_line"] == 0
        assert result["end_line"] == 1
        assert "def hello():" in result["content"]
        assert "def world():" not in result["content"]

    def test_read_nonexistent_file(self, file_tools: FileTools) -> None:
        """Should return error for missing file."""
        result = file_tools.read_file("nonexistent.txt")
        assert "error" in result
        assert "not found" in result["error"].lower()

    def test_read_directory_returns_error(self, file_tools: FileTools) -> None:
        """Should return error when path is a directory."""
        result = file_tools.read_file("src")
        assert "error" in result
        assert "not a file" in result["error"].lower()

    def test_path_traversal_blocked(self, file_tools: FileTools) -> None:
        """Should block paths outside project root."""
        result = file_tools.read_file("../../../etc/passwd")
        assert "error" in result
        assert "outside project root" in result["error"].lower()


class TestCreateTextFile:
    """Tests for create_text_file method."""

    def test_create_new_file(self, file_tools: FileTools, project_dir: Path) -> None:
        """Should create a new file."""
        result = file_tools.create_text_file("new_file.txt", "Hello, World!")
        assert result["success"] is True

        content = (project_dir / "new_file.txt").read_text()
        assert content == "Hello, World!"

    def test_create_file_in_new_directory(
        self, file_tools: FileTools, project_dir: Path
    ) -> None:
        """Should create parent directories if needed."""
        result = file_tools.create_text_file("deep/nested/file.txt", "content")
        assert result["success"] is True
        assert (project_dir / "deep" / "nested" / "file.txt").exists()

    def test_overwrite_existing_file(
        self, file_tools: FileTools, project_dir: Path
    ) -> None:
        """Should overwrite existing file."""
        result = file_tools.create_text_file("README.md", "New content")
        assert result["success"] is True

        content = (project_dir / "README.md").read_text()
        assert content == "New content"


class TestListDir:
    """Tests for list_dir method."""

    def test_list_root_directory(self, file_tools: FileTools) -> None:
        """Should list files and directories in root."""
        result = file_tools.list_dir(".")
        assert "directories" in result
        assert "files" in result
        assert "src/" in result["directories"]
        assert "tests/" in result["directories"]
        assert "README.md" in result["files"]

    def test_list_subdirectory(self, file_tools: FileTools) -> None:
        """Should list files in subdirectory."""
        result = file_tools.list_dir("src")
        assert "main.py" in result["files"]
        assert "utils.py" in result["files"]

    def test_list_recursive(self, file_tools: FileTools) -> None:
        """Should list files recursively."""
        result = file_tools.list_dir(".", recursive=True)
        # Should include nested files
        assert any("main.py" in f for f in result["files"])
        assert any("test_main.py" in f for f in result["files"])

    def test_list_nonexistent_directory(self, file_tools: FileTools) -> None:
        """Should return error for missing directory."""
        result = file_tools.list_dir("nonexistent")
        assert "error" in result


class TestFindFile:
    """Tests for find_file method."""

    def test_find_by_extension(self, file_tools: FileTools) -> None:
        """Should find files by extension pattern."""
        result = file_tools.find_file("*.py")
        assert result["count"] >= 3
        assert any("main.py" in m for m in result["matches"])
        assert any("utils.py" in m for m in result["matches"])

    def test_find_by_prefix(self, file_tools: FileTools) -> None:
        """Should find files by name prefix."""
        result = file_tools.find_file("test_*.py")
        assert result["count"] >= 1
        assert any("test_main.py" in m for m in result["matches"])

    def test_find_in_subdirectory(self, file_tools: FileTools) -> None:
        """Should find files in specific directory."""
        result = file_tools.find_file("*.py", "src")
        assert result["count"] == 2
        assert all("src/" in m for m in result["matches"])


class TestSearchForPattern:
    """Tests for search_for_pattern method."""

    def test_search_simple_pattern(self, file_tools: FileTools) -> None:
        """Should find matches for simple pattern."""
        result = file_tools.search_for_pattern(r"def \w+\(\)")
        assert result["count"] >= 3
        assert any("main.py" in m["file"] for m in result["matches"])

    def test_search_with_glob_filter(self, file_tools: FileTools) -> None:
        """Should filter files by glob pattern."""
        result = file_tools.search_for_pattern(r"def", glob_pattern="**/test_*.py")
        assert all("test_" in m["file"] for m in result["matches"])

    def test_search_with_context(self, file_tools: FileTools) -> None:
        """Should include context lines."""
        result = file_tools.search_for_pattern(
            r"def hello", context_lines=1
        )
        match = next((m for m in result["matches"] if "main.py" in m["file"]), None)
        assert match is not None
        assert match["context"] is not None
        assert len(match["context"]) >= 1

    def test_search_no_matches(self, file_tools: FileTools) -> None:
        """Should return empty results for no matches."""
        result = file_tools.search_for_pattern(r"xyz_nonexistent_pattern_123")
        assert result["count"] == 0
        assert result["matches"] == []

    def test_search_invalid_regex(self, file_tools: FileTools) -> None:
        """Should return error for invalid regex."""
        result = file_tools.search_for_pattern(r"[invalid")
        assert "error" in result


class TestReplaceContent:
    """Tests for replace_content method."""

    def test_replace_literal(
        self, file_tools: FileTools, project_dir: Path
    ) -> None:
        """Should replace literal string."""
        result = file_tools.replace_content(
            "src/main.py",
            needle="def hello():",
            replacement="def greet():",
            mode="literal",
        )
        assert result["success"] is True
        assert result["replacements_made"] == 1

        content = (project_dir / "src" / "main.py").read_text()
        assert "def greet():" in content
        assert "def hello():" not in content

    def test_replace_regex(
        self, file_tools: FileTools, project_dir: Path
    ) -> None:
        """Should replace using regex pattern."""
        result = file_tools.replace_content(
            "src/main.py",
            needle=r"print\('(\w+)'\)",
            replacement=r"print('\1 World')",
            mode="regex",
        )
        assert result["success"] is True

        content = (project_dir / "src" / "main.py").read_text()
        assert "Hello World" in content

    def test_replace_multiple_not_allowed(self, file_tools: FileTools) -> None:
        """Should fail when multiple matches and allow_multiple=False."""
        result = file_tools.replace_content(
            "src/main.py",
            needle="def",
            replacement="DEF",
            mode="literal",
            allow_multiple=False,
        )
        assert result["success"] is False
        assert "multiple" in result["error"].lower()

    def test_replace_multiple_allowed(
        self, file_tools: FileTools, project_dir: Path
    ) -> None:
        """Should replace all matches when allow_multiple=True."""
        result = file_tools.replace_content(
            "src/main.py",
            needle="def",
            replacement="DEF",
            mode="literal",
            allow_multiple=True,
        )
        assert result["success"] is True
        assert result["replacements_made"] == 2

        content = (project_dir / "src" / "main.py").read_text()
        assert content.count("DEF") == 2

    def test_replace_pattern_not_found(self, file_tools: FileTools) -> None:
        """Should return error when pattern not found."""
        result = file_tools.replace_content(
            "README.md",
            needle="nonexistent_pattern",
            replacement="replacement",
        )
        assert result["success"] is False
        assert "not found" in result["error"].lower()
