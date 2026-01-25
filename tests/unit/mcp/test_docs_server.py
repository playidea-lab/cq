"""Unit tests for MCP Documentation Tools."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.docs.analyzer import Location, Reference, Symbol, SymbolKind
from c4.mcp.docs_server import (
    ChangelogEntry,
    DocFormat,
    DocGenerator,
    DocSnapshot,
    DocumentationEntry,
    ExampleEntry,
    MCP_TOOLS,
    QueryResult,
    SnapshotDiff,
    handle_mcp_tool_call,
)

# ruff: noqa: I001


# =============================================================================
# Fixtures
# =============================================================================


@pytest.fixture
def sample_symbol() -> Symbol:
    """Create a sample Symbol for testing."""
    return Symbol(
        name="TestClass",
        kind=SymbolKind.CLASS,
        location=Location(
            file_path="src/test.py",
            start_line=10,
            end_line=50,
            start_column=0,
            end_column=0,
        ),
        signature="class TestClass:",
        docstring="A test class for demonstration.",
        parent=None,
        children=[],
        metadata={},
    )


@pytest.fixture
def sample_method_symbol() -> Symbol:
    """Create a sample method Symbol."""
    return Symbol(
        name="test_method",
        kind=SymbolKind.METHOD,
        location=Location(
            file_path="src/test.py",
            start_line=15,
            end_line=25,
            start_column=4,
            end_column=0,
        ),
        signature="def test_method(self, arg: str) -> bool:",
        docstring="A test method.",
        parent="TestClass",
        children=[],
        metadata={},
    )


@pytest.fixture
def sample_reference() -> Reference:
    """Create a sample Reference for testing."""
    return Reference(
        symbol_name="TestClass",
        location=Location(
            file_path="src/other.py",
            start_line=5,
            end_line=5,
            start_column=10,
            end_column=19,
        ),
        context="obj = TestClass()",
        ref_kind="usage",
    )


@pytest.fixture
def doc_entry() -> DocumentationEntry:
    """Create a sample DocumentationEntry."""
    return DocumentationEntry(
        name="TestClass",
        kind=SymbolKind.CLASS,
        qualified_name="TestClass",
        file_path="src/test.py",
        line_number=10,
        signature="class TestClass:",
        docstring="A test class for demonstration.",
        parent=None,
        children=["method1", "method2"],
        references_count=5,
        metadata={"decorators": ["@dataclass"]},
    )


@pytest.fixture
def example_entry() -> ExampleEntry:
    """Create a sample ExampleEntry."""
    return ExampleEntry(
        symbol_name="TestClass",
        file_path="src/usage.py",
        line_number=10,
        context="obj = TestClass()",
        usage_type="usage",
        surrounding_lines=[
            "    8: def create_instance():",
            "    9:     # Create a new instance",
            ">>> 10: obj = TestClass()",
            "   11:     return obj",
            "   12: ",
        ],
    )


@pytest.fixture
def changelog_entry() -> ChangelogEntry:
    """Create a sample ChangelogEntry."""
    return ChangelogEntry(
        commit_hash="abc123def456",
        author="Test Author",
        date="2025-01-25",
        message="feat: add new feature",
        file_path="src/test.py",
        additions=10,
        deletions=3,
        diff_snippet="+def new_function():\n+    pass",
    )


@pytest.fixture
def mock_analyzer() -> MagicMock:
    """Create a mock CodeAnalyzer."""
    analyzer = MagicMock()
    analyzer.find_symbol.return_value = []
    analyzer.find_references.return_value = []
    analyzer.get_all_symbols.return_value = []
    analyzer.add_directory.return_value = 5
    return analyzer


# =============================================================================
# DocumentationEntry Tests
# =============================================================================


class TestDocumentationEntry:
    """Tests for DocumentationEntry dataclass."""

    def test_to_dict_returns_all_fields(self, doc_entry: DocumentationEntry) -> None:
        """to_dict should return all fields as a dictionary."""
        result = doc_entry.to_dict()

        assert result["name"] == "TestClass"
        assert result["kind"] == "class"
        assert result["qualified_name"] == "TestClass"
        assert result["file_path"] == "src/test.py"
        assert result["line_number"] == 10
        assert result["signature"] == "class TestClass:"
        assert result["docstring"] == "A test class for demonstration."
        assert result["parent"] is None
        assert result["children"] == ["method1", "method2"]
        assert result["references_count"] == 5
        assert result["metadata"] == {"decorators": ["@dataclass"]}

    def test_to_markdown_includes_header(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should include emoji and qualified name header."""
        result = doc_entry.to_markdown()

        assert "## 📦 TestClass" in result

    def test_to_markdown_includes_location(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should include file location."""
        result = doc_entry.to_markdown()

        assert "**Location:** `src/test.py:10`" in result

    def test_to_markdown_includes_signature(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should include signature in code block."""
        result = doc_entry.to_markdown()

        assert "```python" in result
        assert "class TestClass:" in result

    def test_to_markdown_includes_docstring(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should include description section."""
        result = doc_entry.to_markdown()

        assert "### Description" in result
        assert "A test class for demonstration." in result

    def test_to_markdown_includes_children(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should list member children."""
        result = doc_entry.to_markdown()

        assert "### Members" in result
        assert "- `method1`" in result
        assert "- `method2`" in result

    def test_to_markdown_includes_references(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should show references count."""
        result = doc_entry.to_markdown()

        assert "**References:** 5 usages found" in result

    def test_to_markdown_function_kind_emoji(self) -> None:
        """to_markdown should use correct emoji for functions."""
        entry = DocumentationEntry(
            name="test_func",
            kind=SymbolKind.FUNCTION,
            qualified_name="test_func",
            file_path="test.py",
            line_number=1,
            signature="def test_func():",
            docstring=None,
        )
        result = entry.to_markdown()

        assert "## ⚙️ test_func" in result


# =============================================================================
# ExampleEntry Tests
# =============================================================================


class TestExampleEntry:
    """Tests for ExampleEntry dataclass."""

    def test_to_dict_returns_all_fields(self, example_entry: ExampleEntry) -> None:
        """to_dict should return all fields."""
        result = example_entry.to_dict()

        assert result["symbol_name"] == "TestClass"
        assert result["file_path"] == "src/usage.py"
        assert result["line_number"] == 10
        assert result["context"] == "obj = TestClass()"
        assert result["usage_type"] == "usage"
        assert len(result["surrounding_lines"]) == 5

    def test_to_markdown_includes_location_header(
        self, example_entry: ExampleEntry
    ) -> None:
        """to_markdown should include file:line header."""
        result = example_entry.to_markdown()

        assert "### src/usage.py:10" in result

    def test_to_markdown_includes_code_block(
        self, example_entry: ExampleEntry
    ) -> None:
        """to_markdown should include code in python block."""
        result = example_entry.to_markdown()

        assert "```python" in result
        assert ">>> 10: obj = TestClass()" in result

    def test_to_markdown_uses_context_when_no_surrounding(self) -> None:
        """to_markdown should use context when no surrounding lines."""
        entry = ExampleEntry(
            symbol_name="func",
            file_path="test.py",
            line_number=1,
            context="func()",
            surrounding_lines=[],
        )
        result = entry.to_markdown()

        assert "func()" in result


# =============================================================================
# ChangelogEntry Tests
# =============================================================================


class TestChangelogEntry:
    """Tests for ChangelogEntry dataclass."""

    def test_to_dict_returns_all_fields(self, changelog_entry: ChangelogEntry) -> None:
        """to_dict should return all fields."""
        result = changelog_entry.to_dict()

        assert result["commit_hash"] == "abc123def456"
        assert result["author"] == "Test Author"
        assert result["date"] == "2025-01-25"
        assert result["message"] == "feat: add new feature"
        assert result["file_path"] == "src/test.py"
        assert result["additions"] == 10
        assert result["deletions"] == 3

    def test_to_markdown_includes_commit_and_date(
        self, changelog_entry: ChangelogEntry
    ) -> None:
        """to_markdown should include short hash and date."""
        result = changelog_entry.to_markdown()

        assert "### abc123d - 2025-01-25" in result

    def test_to_markdown_includes_author(
        self, changelog_entry: ChangelogEntry
    ) -> None:
        """to_markdown should include author."""
        result = changelog_entry.to_markdown()

        assert "**Author:** Test Author" in result

    def test_to_markdown_includes_message(
        self, changelog_entry: ChangelogEntry
    ) -> None:
        """to_markdown should include commit message."""
        result = changelog_entry.to_markdown()

        assert "**Message:** feat: add new feature" in result

    def test_to_markdown_includes_changes(
        self, changelog_entry: ChangelogEntry
    ) -> None:
        """to_markdown should include additions/deletions."""
        result = changelog_entry.to_markdown()

        assert "**Changes:** +10 -3" in result

    def test_to_markdown_includes_diff(
        self, changelog_entry: ChangelogEntry
    ) -> None:
        """to_markdown should include diff snippet."""
        result = changelog_entry.to_markdown()

        assert "```diff" in result
        assert "+def new_function():" in result


# =============================================================================
# QueryResult Tests
# =============================================================================


class TestQueryResult:
    """Tests for QueryResult dataclass."""

    def test_to_dict_returns_all_fields(self, doc_entry: DocumentationEntry) -> None:
        """to_dict should return all fields including entries."""
        result = QueryResult(
            query="TestClass",
            total_results=1,
            entries=[doc_entry],
            query_time_ms=5.5,
        )
        data = result.to_dict()

        assert data["query"] == "TestClass"
        assert data["total_results"] == 1
        assert len(data["entries"]) == 1
        assert data["query_time_ms"] == 5.5

    def test_to_markdown_includes_header(self, doc_entry: DocumentationEntry) -> None:
        """to_markdown should include search query header."""
        result = QueryResult(
            query="TestClass",
            total_results=1,
            entries=[doc_entry],
        )
        md = result.to_markdown()

        assert "# Documentation Search: `TestClass`" in md
        assert "Found **1** results" in md


# =============================================================================
# DocGenerator Tests
# =============================================================================


class TestDocGenerator:
    """Tests for DocGenerator class."""

    def test_init_sets_defaults(self, tmp_path: Path) -> None:
        """__init__ should set default patterns."""
        generator = DocGenerator(project_root=tmp_path)

        assert generator.project_root == tmp_path.resolve()
        assert "**/*.py" in generator.include_patterns
        assert "**/node_modules/**" in generator.exclude_patterns

    def test_init_accepts_custom_patterns(self, tmp_path: Path) -> None:
        """__init__ should accept custom include/exclude patterns."""
        generator = DocGenerator(
            project_root=tmp_path,
            include_patterns=["**/*.rs"],
            exclude_patterns=["**/target/**"],
        )

        assert generator.include_patterns == ["**/*.rs"]
        assert generator.exclude_patterns == ["**/target/**"]

    def test_index_codebase_calls_analyzer(self, tmp_path: Path) -> None:
        """index_codebase should call analyzer.add_directory."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator._analyzer, "add_directory", return_value=5):
            with patch.object(generator._analyzer, "get_all_symbols", return_value=[]):
                count = generator.index_codebase()

        assert count == 5
        assert generator._indexed is True

    def test_index_file_sets_indexed(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """index_file should set _indexed to True."""
        generator = DocGenerator(project_root=tmp_path)

        # Create a test file
        test_file = tmp_path / "test.py"
        test_file.write_text("class Foo: pass")

        with patch.object(generator._analyzer, "add_file"):
            with patch.object(
                generator._analyzer, "get_all_symbols", return_value=[sample_symbol]
            ):
                with patch.object(generator._analyzer, "find_references", return_value=[]):
                    generator.index_file(test_file)

        assert generator._indexed is True

    def test_query_docs_returns_markdown_by_default(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """query_docs should return markdown by default."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = generator.query_docs("TestClass")

        assert isinstance(result, str)
        assert "# Documentation Search:" in result

    def test_query_docs_returns_dict_for_json(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """query_docs should return dict for JSON format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = generator.query_docs("TestClass", format=DocFormat.JSON)

        assert isinstance(result, dict)
        assert result["query"] == "TestClass"

    def test_query_docs_filters_by_kind(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """query_docs should pass kind filter to analyzer."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ) as mock_find:
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                generator.query_docs("TestClass", kind=SymbolKind.CLASS)

        mock_find.assert_called_once_with(
            "TestClass", kind=SymbolKind.CLASS, exact_match=False
        )

    def test_query_docs_respects_limit(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """query_docs should respect limit parameter."""
        generator = DocGenerator(project_root=tmp_path)
        symbols = [sample_symbol] * 10

        with patch.object(generator._analyzer, "find_symbol", return_value=symbols):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = generator.query_docs("Test", limit=3, format=DocFormat.JSON)

        assert result["total_results"] == 3

    def test_get_api_reference_symbol_found(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """get_api_reference should return docs when symbol found."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = generator.get_api_reference("TestClass")

        assert isinstance(result, str)
        assert "TestClass" in result

    def test_get_api_reference_symbol_not_found(self, tmp_path: Path) -> None:
        """get_api_reference should return error when symbol not found."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator._analyzer, "find_symbol", return_value=[]):
            result = generator.get_api_reference("NonExistent")

        assert "not found" in result

    def test_get_api_reference_json_format(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """get_api_reference should return dict for JSON format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = generator.get_api_reference(
                    "TestClass", format=DocFormat.JSON
                )

        assert isinstance(result, dict)
        assert result["name"] == "TestClass"

    def test_get_api_reference_includes_references(
        self, tmp_path: Path, sample_symbol: Symbol, sample_reference: Reference
    ) -> None:
        """get_api_reference should include references when requested."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(
                generator._analyzer, "find_references", return_value=[sample_reference]
            ):
                result = generator.get_api_reference(
                    "TestClass", include_references=True, format=DocFormat.JSON
                )

        assert "references" in result
        assert len(result["references"]) == 1

    def test_search_examples_returns_usage_refs(
        self, tmp_path: Path, sample_reference: Reference
    ) -> None:
        """search_examples should return usage examples."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_references", return_value=[sample_reference]
        ):
            with patch.object(generator, "_get_surrounding_lines", return_value=[]):
                result = generator.search_examples("TestClass", format=DocFormat.JSON)

        assert result["symbol"] == "TestClass"
        assert result["total_examples"] == 1

    def test_search_examples_filters_definitions(
        self, tmp_path: Path, sample_reference: Reference
    ) -> None:
        """search_examples should filter out definition refs."""
        generator = DocGenerator(project_root=tmp_path)

        # Create a definition reference (should be filtered)
        def_ref = Reference(
            symbol_name="TestClass",
            location=Location(
                file_path="test.py",
                start_line=1,
                end_line=1,
                start_column=0,
                end_column=10,
            ),
            context="class TestClass:",
            ref_kind="definition",
        )

        with patch.object(
            generator._analyzer,
            "find_references",
            return_value=[def_ref, sample_reference],
        ):
            with patch.object(generator, "_get_surrounding_lines", return_value=[]):
                result = generator.search_examples("TestClass", format=DocFormat.JSON)

        # Should only include usage refs
        assert result["total_examples"] == 1

    def test_get_surrounding_lines_returns_context(self, tmp_path: Path) -> None:
        """_get_surrounding_lines should return lines with context."""
        generator = DocGenerator(project_root=tmp_path)

        # Create test file
        test_file = tmp_path / "test.py"
        test_file.write_text("line1\nline2\nline3\nline4\nline5\n")

        lines = generator._get_surrounding_lines(str(test_file), 3, 1)

        assert len(lines) == 3
        assert "line2" in lines[0]
        assert ">>> 3: line3" in lines[1]
        assert "line4" in lines[2]

    def test_get_surrounding_lines_handles_missing_file(self, tmp_path: Path) -> None:
        """_get_surrounding_lines should return empty for missing file."""
        generator = DocGenerator(project_root=tmp_path)

        lines = generator._get_surrounding_lines("nonexistent.py", 1, 1)

        assert lines == []

    def test_get_changelog_with_file_path(self, tmp_path: Path) -> None:
        """get_changelog should work with file path."""
        generator = DocGenerator(project_root=tmp_path)

        changelog_entry = ChangelogEntry(
            commit_hash="abc123",
            author="Test",
            date="2025-01-25",
            message="test commit",
            file_path="test.py",
        )

        with patch.object(
            generator, "_get_git_log", return_value=[changelog_entry]
        ):
            result = generator.get_changelog(file_path="test.py")

        assert "abc123" in result

    def test_get_changelog_with_symbol_name(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """get_changelog should find file from symbol name."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator, "_get_git_log", return_value=[]):
                result = generator.get_changelog(symbol_name="TestClass")

        assert "Changelog" in result

    def test_get_changelog_no_file_or_symbol(self, tmp_path: Path) -> None:
        """get_changelog should return error with no file or symbol."""
        generator = DocGenerator(project_root=tmp_path)

        result = generator.get_changelog()

        assert "No file or symbol specified" in result

    def test_get_changelog_json_format(self, tmp_path: Path) -> None:
        """get_changelog should return dict for JSON format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_git_log", return_value=[]):
            result = generator.get_changelog(
                file_path="test.py", format=DocFormat.JSON
            )

        assert isinstance(result, dict)
        assert result["file_path"] == "test.py"

    def test_get_git_log_parses_output(self, tmp_path: Path) -> None:
        """_get_git_log should parse git log output."""
        generator = DocGenerator(project_root=tmp_path)

        mock_output = "abc123|Author|2025-01-25 10:00:00 +0000|test message\n5\t3\ttest.py"

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(
                returncode=0, stdout=mock_output
            )
            entries = generator._get_git_log("test.py", 10)

        assert len(entries) == 1
        assert entries[0].commit_hash == "abc123"
        assert entries[0].author == "Author"
        assert entries[0].additions == 5
        assert entries[0].deletions == 3

    def test_get_git_log_handles_error(self, tmp_path: Path) -> None:
        """_get_git_log should return empty list on error."""
        generator = DocGenerator(project_root=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=1, stdout="")
            entries = generator._get_git_log("test.py", 10)

        assert entries == []

    def test_generate_full_docs_indexes_if_needed(self, tmp_path: Path) -> None:
        """generate_full_docs should index codebase if not indexed."""
        generator = DocGenerator(project_root=tmp_path)
        assert generator._indexed is False

        with patch.object(generator._analyzer, "add_directory", return_value=0):
            with patch.object(generator._analyzer, "get_all_symbols", return_value=[]):
                generator.generate_full_docs()

        assert generator._indexed is True

    def test_generate_full_docs_returns_markdown(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """generate_full_docs should return markdown content."""
        generator = DocGenerator(project_root=tmp_path)
        generator._indexed = True

        with patch.object(
            generator._analyzer, "get_all_symbols", return_value=[sample_symbol]
        ):
            result = generator.generate_full_docs()

        assert "# API Documentation" in result
        assert "src/test.py" in result

    def test_generate_full_docs_writes_to_file(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """generate_full_docs should write to output directory."""
        generator = DocGenerator(project_root=tmp_path)
        generator._indexed = True
        output_dir = tmp_path / "docs"

        with patch.object(
            generator._analyzer, "get_all_symbols", return_value=[sample_symbol]
        ):
            result = generator.generate_full_docs(output_dir=output_dir)

        assert (output_dir / "API.md").exists()
        assert result == str(output_dir / "API.md")

    def test_generate_full_docs_json_format(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """generate_full_docs should write JSON when format specified."""
        generator = DocGenerator(project_root=tmp_path)
        generator._indexed = True
        output_dir = tmp_path / "docs"

        with patch.object(
            generator._analyzer, "get_all_symbols", return_value=[sample_symbol]
        ):
            output_path = generator.generate_full_docs(
                output_dir=output_dir, format=DocFormat.JSON
            )

        assert output_path == str(output_dir / "API.json")
        assert (output_dir / "API.json").exists()
        # Verify valid JSON
        content = (output_dir / "API.json").read_text()
        data = json.loads(content)
        assert "generated_at" in data
        assert "files" in data


# =============================================================================
# MCP Tool Handler Tests
# =============================================================================


class TestMCPTools:
    """Tests for MCP tool definitions and handler."""

    def test_mcp_tools_has_required_tools(self) -> None:
        """MCP_TOOLS should include all required tools."""
        tool_names = [t["name"] for t in MCP_TOOLS]

        assert "query_docs" in tool_names
        assert "get_api_reference" in tool_names
        assert "search_examples" in tool_names
        assert "get_changelog" in tool_names

    def test_mcp_tools_have_input_schemas(self) -> None:
        """All MCP tools should have input schemas."""
        for tool in MCP_TOOLS:
            assert "inputSchema" in tool
            assert "type" in tool["inputSchema"]
            assert tool["inputSchema"]["type"] == "object"

    def test_handle_mcp_tool_call_query_docs(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """handle_mcp_tool_call should handle query_docs."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = handle_mcp_tool_call(
                    "query_docs",
                    {"query": "Test", "format": "json"},
                    generator,
                )

        assert isinstance(result, dict)
        assert result["query"] == "Test"

    def test_handle_mcp_tool_call_get_api_reference(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """handle_mcp_tool_call should handle get_api_reference."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = handle_mcp_tool_call(
                    "get_api_reference",
                    {"symbol_name": "TestClass", "format": "json"},
                    generator,
                )

        assert isinstance(result, dict)
        assert result["name"] == "TestClass"

    def test_handle_mcp_tool_call_search_examples(
        self, tmp_path: Path, sample_reference: Reference
    ) -> None:
        """handle_mcp_tool_call should handle search_examples."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_references", return_value=[sample_reference]
        ):
            with patch.object(generator, "_get_surrounding_lines", return_value=[]):
                result = handle_mcp_tool_call(
                    "search_examples",
                    {"symbol_name": "TestClass", "format": "json"},
                    generator,
                )

        assert isinstance(result, dict)
        assert result["symbol"] == "TestClass"

    def test_handle_mcp_tool_call_get_changelog(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should handle get_changelog."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_git_log", return_value=[]):
            result = handle_mcp_tool_call(
                "get_changelog",
                {"file_path": "test.py", "format": "json"},
                generator,
            )

        assert isinstance(result, dict)
        assert result["file_path"] == "test.py"

    def test_handle_mcp_tool_call_unknown_tool(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should return error for unknown tool."""
        generator = DocGenerator(project_root=tmp_path)

        result = handle_mcp_tool_call("unknown_tool", {}, generator)

        assert result == {"error": "Unknown tool: unknown_tool"}

    def test_handle_mcp_tool_call_kind_mapping(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """handle_mcp_tool_call should map kind string to enum."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ) as mock_find:
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                handle_mcp_tool_call(
                    "query_docs",
                    {"query": "Test", "kind": "class"},
                    generator,
                )

        mock_find.assert_called_once()
        call_args = mock_find.call_args
        assert call_args[1]["kind"] == SymbolKind.CLASS

    def test_handle_mcp_tool_call_default_format(
        self, tmp_path: Path, sample_symbol: Symbol
    ) -> None:
        """handle_mcp_tool_call should use markdown as default format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(
            generator._analyzer, "find_symbol", return_value=[sample_symbol]
        ):
            with patch.object(generator._analyzer, "find_references", return_value=[]):
                result = handle_mcp_tool_call(
                    "query_docs",
                    {"query": "Test"},  # No format specified
                    generator,
                )

        assert isinstance(result, str)
        assert "# Documentation Search:" in result


class TestDocSnapshot:
    """Tests for DocSnapshot dataclass."""

    def test_to_dict(self) -> None:
        """DocSnapshot.to_dict should return correct dictionary."""
        snapshot = DocSnapshot(
            version="v1.0.0",
            created_at="2025-01-25T10:00:00",
            commit_hash="abc1234",
            description="Initial release",
            files_count=10,
            symbols_count=50,
            content_hash="sha256:xyz789",
        )

        result = snapshot.to_dict()

        assert result["version"] == "v1.0.0"
        assert result["created_at"] == "2025-01-25T10:00:00"
        assert result["commit_hash"] == "abc1234"
        assert result["description"] == "Initial release"
        assert result["files_count"] == 10
        assert result["symbols_count"] == 50
        assert result["content_hash"] == "sha256:xyz789"

    def test_to_dict_no_optional_fields(self) -> None:
        """DocSnapshot.to_dict should handle None values."""
        snapshot = DocSnapshot(
            version="v1.0.0",
            created_at="2025-01-25T10:00:00",
            commit_hash=None,
            description=None,
            files_count=5,
            symbols_count=20,
            content_hash="sha256:abc123",
        )

        result = snapshot.to_dict()

        assert result["version"] == "v1.0.0"
        assert result["commit_hash"] is None
        assert result["description"] is None

    def test_to_markdown(self) -> None:
        """DocSnapshot.to_markdown should return formatted markdown."""
        snapshot = DocSnapshot(
            version="v1.0.0",
            created_at="2025-01-25T10:00:00",
            commit_hash="abc1234",
            description="Initial release",
            files_count=10,
            symbols_count=50,
            content_hash="sha256:xyz789",
        )

        result = snapshot.to_markdown()

        assert "## 📸 Snapshot: v1.0.0" in result
        assert "2025-01-25T10:00:00" in result
        assert "abc1234" in result
        assert "Initial release" in result
        assert "10" in result
        assert "50" in result


class TestSnapshotDiff:
    """Tests for SnapshotDiff dataclass."""

    def test_to_dict(self) -> None:
        """SnapshotDiff.to_dict should return correct dictionary."""
        diff = SnapshotDiff(
            from_version="v1.0.0",
            to_version="v2.0.0",
            added_symbols=["new_func", "NewClass"],
            removed_symbols=["old_func"],
            modified_symbols=["changed_func"],
            added_files=["new_file.py"],
            removed_files=["old_file.py"],
            summary="Major update with new features",
        )

        result = diff.to_dict()

        assert result["from_version"] == "v1.0.0"
        assert result["to_version"] == "v2.0.0"
        assert result["added_symbols"] == ["new_func", "NewClass"]
        assert result["removed_symbols"] == ["old_func"]
        assert result["modified_symbols"] == ["changed_func"]
        assert result["added_files"] == ["new_file.py"]
        assert result["removed_files"] == ["old_file.py"]
        assert result["summary"] == "Major update with new features"

    def test_to_dict_empty_lists(self) -> None:
        """SnapshotDiff.to_dict should handle empty lists."""
        diff = SnapshotDiff(
            from_version="v1.0.0",
            to_version="v1.0.1",
            added_symbols=[],
            removed_symbols=[],
            modified_symbols=[],
            added_files=[],
            removed_files=[],
            summary="No changes",
        )

        result = diff.to_dict()

        assert result["added_symbols"] == []
        assert result["removed_symbols"] == []

    def test_to_markdown(self) -> None:
        """SnapshotDiff.to_markdown should return formatted markdown."""
        diff = SnapshotDiff(
            from_version="v1.0.0",
            to_version="v2.0.0",
            added_symbols=["new_func"],
            removed_symbols=["old_func"],
            modified_symbols=["changed_func"],
            added_files=["new_file.py"],
            removed_files=["old_file.py"],
            summary="Major update",
        )

        result = diff.to_markdown()

        assert "# Documentation Diff: v1.0.0 → v2.0.0" in result
        assert "new_func" in result
        assert "old_func" in result
        assert "changed_func" in result

    def test_to_markdown_no_changes(self) -> None:
        """SnapshotDiff.to_markdown should handle no changes."""
        diff = SnapshotDiff(
            from_version="v1.0.0",
            to_version="v1.0.1",
            added_symbols=[],
            removed_symbols=[],
            modified_symbols=[],
            added_files=[],
            removed_files=[],
            summary="No changes",
        )

        result = diff.to_markdown()

        assert "v1.0.0 → v1.0.1" in result


class TestDocGeneratorSnapshots:
    """Tests for DocGenerator snapshot methods."""

    def test_create_snapshot(self, tmp_path: Path) -> None:
        """DocGenerator.create_snapshot should create a snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        # Create a test file
        test_file = tmp_path / "test.py"
        test_file.write_text("def hello(): pass")

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            snapshot = generator.create_snapshot(
                version="v1.0.0",
                description="Test snapshot",
            )

        assert snapshot.version == "v1.0.0"
        assert snapshot.description == "Test snapshot"
        assert snapshot.commit_hash == "abc1234"
        assert snapshot.content_hash is not None

    def test_create_snapshot_duplicate_version(self, tmp_path: Path) -> None:
        """DocGenerator.create_snapshot should raise error for duplicate version."""
        generator = DocGenerator(project_root=tmp_path)

        # Create first snapshot
        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        # Try to create duplicate
        with pytest.raises(ValueError, match="already exists"):
            generator.create_snapshot(version="v1.0.0")

    def test_list_snapshots_empty(self, tmp_path: Path) -> None:
        """DocGenerator.list_snapshots should return empty list if no snapshots."""
        generator = DocGenerator(project_root=tmp_path)

        result = generator.list_snapshots()

        assert result == []

    def test_list_snapshots(self, tmp_path: Path) -> None:
        """DocGenerator.list_snapshots should return all snapshots."""
        generator = DocGenerator(project_root=tmp_path)

        # Create snapshots
        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")
            generator.create_snapshot(version="v2.0.0")

        result = generator.list_snapshots()

        assert len(result) == 2
        versions = [s.version for s in result]
        assert "v1.0.0" in versions
        assert "v2.0.0" in versions

    def test_get_snapshot_exists(self, tmp_path: Path) -> None:
        """DocGenerator.get_snapshot should return snapshot data."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0", description="Test")

        result = generator.get_snapshot(version="v1.0.0", format=DocFormat.JSON)

        assert isinstance(result, dict)
        assert result.get("version") == "v1.0.0"

    def test_get_snapshot_not_found(self, tmp_path: Path) -> None:
        """DocGenerator.get_snapshot should return error for missing snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        result = generator.get_snapshot(version="v9.9.9", format=DocFormat.JSON)

        assert isinstance(result, dict)
        assert "error" in result

    def test_get_snapshot_markdown_format(self, tmp_path: Path) -> None:
        """DocGenerator.get_snapshot should support markdown format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        result = generator.get_snapshot(version="v1.0.0", format=DocFormat.MARKDOWN)

        assert isinstance(result, str)
        # API.md contains API documentation header, not version info
        assert "# API Documentation" in result

    def test_compare_snapshots(self, tmp_path: Path) -> None:
        """DocGenerator.compare_snapshots should return diff."""
        generator = DocGenerator(project_root=tmp_path)

        # Create first snapshot
        test_file = tmp_path / "test.py"
        test_file.write_text("def old_func(): pass")

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        # Modify and create second snapshot
        test_file.write_text("def new_func(): pass")

        with patch.object(generator, "_get_current_commit", return_value="def5678"):
            generator.create_snapshot(version="v2.0.0")

        result = generator.compare_snapshots(
            from_version="v1.0.0",
            to_version="v2.0.0",
            format=DocFormat.JSON,
        )

        assert isinstance(result, dict)
        assert result.get("from_version") == "v1.0.0"
        assert result.get("to_version") == "v2.0.0"

    def test_compare_snapshots_not_found(self, tmp_path: Path) -> None:
        """DocGenerator.compare_snapshots should return error if snapshot not found."""
        generator = DocGenerator(project_root=tmp_path)

        result = generator.compare_snapshots(
            from_version="v1.0.0",
            to_version="v2.0.0",
            format=DocFormat.JSON,
        )

        assert isinstance(result, dict)
        assert "error" in result

    def test_delete_snapshot(self, tmp_path: Path) -> None:
        """DocGenerator.delete_snapshot should remove snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        # Verify snapshot exists
        assert len(generator.list_snapshots()) == 1

        # Delete
        result = generator.delete_snapshot(version="v1.0.0")

        assert result is True
        assert len(generator.list_snapshots()) == 0

    def test_delete_snapshot_not_found(self, tmp_path: Path) -> None:
        """DocGenerator.delete_snapshot should return False for missing snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        result = generator.delete_snapshot(version="v9.9.9")

        assert result is False


class TestMCPToolCallSnapshots:
    """Tests for MCP tool call handlers for snapshot tools."""

    def test_handle_mcp_tool_call_create_snapshot(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should handle create_snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            result = handle_mcp_tool_call(
                "create_snapshot",
                {"version": "v1.0.0", "description": "Test", "format": "json"},
                generator,
            )

        assert isinstance(result, dict)
        assert result["version"] == "v1.0.0"

    def test_handle_mcp_tool_call_create_snapshot_markdown(
        self, tmp_path: Path
    ) -> None:
        """handle_mcp_tool_call create_snapshot should support markdown format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            result = handle_mcp_tool_call(
                "create_snapshot",
                {"version": "v1.0.0", "format": "markdown"},
                generator,
            )

        assert isinstance(result, str)
        assert "v1.0.0" in result

    def test_handle_mcp_tool_call_create_snapshot_error(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call create_snapshot should handle errors."""
        generator = DocGenerator(project_root=tmp_path)

        # Create first snapshot
        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        # Try duplicate
        result = handle_mcp_tool_call(
            "create_snapshot",
            {"version": "v1.0.0"},
            generator,
        )

        assert isinstance(result, dict)
        assert "error" in result

    def test_handle_mcp_tool_call_list_snapshots(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should handle list_snapshots."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")
            generator.create_snapshot(version="v2.0.0")

        result = handle_mcp_tool_call(
            "list_snapshots",
            {"format": "json"},
            generator,
        )

        assert isinstance(result, dict)
        assert "snapshots" in result
        assert len(result["snapshots"]) == 2

    def test_handle_mcp_tool_call_list_snapshots_markdown(
        self, tmp_path: Path
    ) -> None:
        """handle_mcp_tool_call list_snapshots should support markdown format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        result = handle_mcp_tool_call(
            "list_snapshots",
            {"format": "markdown"},
            generator,
        )

        assert isinstance(result, str)
        assert "v1.0.0" in result

    def test_handle_mcp_tool_call_list_snapshots_empty(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call list_snapshots should handle empty list."""
        generator = DocGenerator(project_root=tmp_path)

        result = handle_mcp_tool_call(
            "list_snapshots",
            {"format": "json"},
            generator,
        )

        assert isinstance(result, dict)
        assert result["snapshots"] == []

    def test_handle_mcp_tool_call_get_snapshot(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should handle get_snapshot."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        result = handle_mcp_tool_call(
            "get_snapshot",
            {"version": "v1.0.0", "format": "json"},
            generator,
        )

        assert isinstance(result, dict)
        assert result.get("version") == "v1.0.0"

    def test_handle_mcp_tool_call_get_snapshot_markdown(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call get_snapshot should support markdown format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")

        result = handle_mcp_tool_call(
            "get_snapshot",
            {"version": "v1.0.0", "format": "markdown"},
            generator,
        )

        assert isinstance(result, str)
        # API.md contains API documentation header, not version info
        assert "# API Documentation" in result

    def test_handle_mcp_tool_call_compare_snapshots(self, tmp_path: Path) -> None:
        """handle_mcp_tool_call should handle compare_snapshots."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")
            generator.create_snapshot(version="v2.0.0")

        result = handle_mcp_tool_call(
            "compare_snapshots",
            {"from_version": "v1.0.0", "to_version": "v2.0.0", "format": "json"},
            generator,
        )

        assert isinstance(result, dict)
        assert result.get("from_version") == "v1.0.0"
        assert result.get("to_version") == "v2.0.0"

    def test_handle_mcp_tool_call_compare_snapshots_markdown(
        self, tmp_path: Path
    ) -> None:
        """handle_mcp_tool_call compare_snapshots should support markdown format."""
        generator = DocGenerator(project_root=tmp_path)

        with patch.object(generator, "_get_current_commit", return_value="abc1234"):
            generator.create_snapshot(version="v1.0.0")
            generator.create_snapshot(version="v2.0.0")

        result = handle_mcp_tool_call(
            "compare_snapshots",
            {"from_version": "v1.0.0", "to_version": "v2.0.0", "format": "markdown"},
            generator,
        )

        assert isinstance(result, str)
        assert "v1.0.0" in result
        assert "v2.0.0" in result
