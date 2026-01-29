"""Tests for C4 LSP Server."""

from __future__ import annotations

import pytest

from c4.docs.analyzer import Location, SymbolKind

# Check if pygls is available
try:
    from lsprotocol import types as lsp

    from c4.lsp.server import (
        C4LSPServer,
        _location_to_lsp_location,
        _location_to_lsp_range,
        _symbol_kind_to_completion_kind,
        _symbol_kind_to_lsp,
    )

    PYGLS_AVAILABLE = True
except ImportError:
    PYGLS_AVAILABLE = False


pytestmark = pytest.mark.skipif(
    not PYGLS_AVAILABLE,
    reason="pygls not installed",
)


class TestSymbolKindConversion:
    """Tests for SymbolKind conversion."""

    def test_function_to_lsp(self):
        """Function should map to LSP Function (12)."""
        assert _symbol_kind_to_lsp(SymbolKind.FUNCTION) == 12

    def test_class_to_lsp(self):
        """Class should map to LSP Class (5)."""
        assert _symbol_kind_to_lsp(SymbolKind.CLASS) == 5

    def test_method_to_lsp(self):
        """Method should map to LSP Method (6)."""
        assert _symbol_kind_to_lsp(SymbolKind.METHOD) == 6

    def test_variable_to_lsp(self):
        """Variable should map to LSP Variable (13)."""
        assert _symbol_kind_to_lsp(SymbolKind.VARIABLE) == 13

    def test_constant_to_lsp(self):
        """Constant should map to LSP Constant (14)."""
        assert _symbol_kind_to_lsp(SymbolKind.CONSTANT) == 14

    def test_interface_to_lsp(self):
        """Interface should map to LSP Interface (11)."""
        assert _symbol_kind_to_lsp(SymbolKind.INTERFACE) == 11

    def test_enum_to_lsp(self):
        """Enum should map to LSP Enum (10)."""
        assert _symbol_kind_to_lsp(SymbolKind.ENUM) == 10


class TestLocationConversion:
    """Tests for Location conversion."""

    def test_location_to_range(self):
        """Location should convert to LSP Range correctly."""
        loc = Location(
            file_path="/test/file.py",
            start_line=10,
            start_column=4,
            end_line=15,
            end_column=20,
        )

        range_ = _location_to_lsp_range(loc)

        # LSP uses 0-based lines
        assert range_.start.line == 9
        assert range_.start.character == 4
        assert range_.end.line == 14
        assert range_.end.character == 20

    def test_location_to_lsp_location(self):
        """Location should convert to LSP Location with URI."""
        loc = Location(
            file_path="/test/file.py",
            start_line=1,
            start_column=0,
            end_line=1,
            end_column=10,
        )

        lsp_loc = _location_to_lsp_location(loc)

        assert lsp_loc.uri == "file:///test/file.py"
        assert lsp_loc.range.start.line == 0


class TestC4LSPServer:
    """Tests for C4LSPServer."""

    def test_server_initialization(self):
        """Server should initialize with default values."""
        server = C4LSPServer()

        assert server.server.name == "c4-lsp"
        assert server.analyzer is not None

    def test_server_custom_name(self):
        """Server should accept custom name and version."""
        server = C4LSPServer(name="test-server", version="1.0.0")

        assert server.server.name == "test-server"
        assert server.server.version == "1.0.0"

    def test_get_word_at_position(self):
        """Should extract word at cursor position."""
        server = C4LSPServer()

        # Add a file with content
        test_content = "def hello_world():\n    return 42\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # Get word at "hello_world"
        position = lsp.Position(line=0, character=5)
        word = server._get_word_at_position("file:///test/file.py", position)

        assert word == "hello_world"

    def test_get_word_at_position_empty(self):
        """Should return None for non-word position."""
        server = C4LSPServer()

        test_content = "def foo():\n    pass\n"
        server.analyzer.add_file("/test/file.py", test_content)

        # Position at whitespace
        position = lsp.Position(line=1, character=0)
        word = server._get_word_at_position("file:///test/file.py", position)

        assert word is None

    def test_get_word_at_position_nonexistent_file(self):
        """Should return None for non-existent file."""
        server = C4LSPServer()

        position = lsp.Position(line=0, character=0)
        word = server._get_word_at_position("file:///nonexistent.py", position)

        assert word is None


class TestHoverHandler:
    """Tests for hover request handling."""

    def test_hover_on_function(self):
        """Hover on function should return function info."""
        server = C4LSPServer()

        test_content = '''def greet(name):
    """Say hello."""
    return f"Hello, {name}"
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.HoverParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=5),
        )

        result = server._handle_hover(params)

        assert result is not None
        assert "greet" in result.contents.value
        assert "Function" in result.contents.value

    def test_hover_on_unknown_symbol(self):
        """Hover on unknown symbol should return None."""
        server = C4LSPServer()

        test_content = "x = unknown_func()\n"
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.HoverParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=5),
        )

        result = server._handle_hover(params)

        # "unknown_func" is not defined in our file
        assert result is None


class TestDefinitionHandler:
    """Tests for definition request handling."""

    def test_definition_finds_function(self):
        """Definition should find function definition."""
        server = C4LSPServer()

        test_content = '''def my_func():
    pass

result = my_func()
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.DefinitionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=3, character=10),  # "my_func" in call
        )

        result = server._handle_definition(params)

        assert result is not None
        assert len(result) >= 1
        # Should point to the definition at line 1 (0-indexed: 0)
        assert result[0].range.start.line == 0

    def test_definition_unknown_symbol(self):
        """Definition on unknown symbol should return None."""
        server = C4LSPServer()

        test_content = "x = unknown()\n"
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.DefinitionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=5),
        )

        result = server._handle_definition(params)

        assert result is None


class TestReferencesHandler:
    """Tests for references request handling."""

    def test_references_finds_usages(self):
        """References should find all usages."""
        server = C4LSPServer()

        test_content = '''def helper():
    pass

helper()
result = helper()
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.ReferenceParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=0, character=5),  # "helper" in definition
            context=lsp.ReferenceContext(include_declaration=True),
        )

        result = server._handle_references(params)

        assert result is not None
        # Should find at least 3 references: definition + 2 calls
        assert len(result) >= 3


class TestDocumentSymbolHandler:
    """Tests for document symbol request handling."""

    def test_document_symbols(self):
        """Should return document outline."""
        server = C4LSPServer()

        test_content = '''class MyClass:
    def method(self):
        pass

def standalone():
    pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.DocumentSymbolParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
        )

        result = server._handle_document_symbol(params)

        assert result is not None
        # Should have 2 top-level symbols: MyClass and standalone
        top_level_names = [s.name for s in result]
        assert "MyClass" in top_level_names
        assert "standalone" in top_level_names


class TestWorkspaceSymbolHandler:
    """Tests for workspace symbol request handling."""

    def test_workspace_symbol_search(self):
        """Should search symbols across workspace."""
        server = C4LSPServer()

        # Add multiple files
        server.analyzer.add_file("/test/a.py", "def alpha(): pass\n")
        server.analyzer.add_file("/test/b.py", "def beta(): pass\n")
        server.analyzer.add_file("/test/c.py", "def alpha_two(): pass\n")

        params = lsp.WorkspaceSymbolParams(query="alpha")

        result = server._handle_workspace_symbol(params)

        assert result is not None
        names = [s.name for s in result]
        # Should find both alpha and alpha_two
        assert "alpha" in names
        assert "alpha_two" in names
        assert "beta" not in names

    def test_workspace_symbol_empty_query(self):
        """Empty query should return None."""
        server = C4LSPServer()

        params = lsp.WorkspaceSymbolParams(query="")

        result = server._handle_workspace_symbol(params)

        assert result is None


class TestCompletionKindConversion:
    """Tests for CompletionItemKind conversion."""

    def test_function_to_completion_kind(self):
        """Function should map to CompletionItemKind.Function (3)."""
        assert _symbol_kind_to_completion_kind(SymbolKind.FUNCTION) == 3

    def test_class_to_completion_kind(self):
        """Class should map to CompletionItemKind.Class (7)."""
        assert _symbol_kind_to_completion_kind(SymbolKind.CLASS) == 7

    def test_method_to_completion_kind(self):
        """Method should map to CompletionItemKind.Method (2)."""
        assert _symbol_kind_to_completion_kind(SymbolKind.METHOD) == 2

    def test_variable_to_completion_kind(self):
        """Variable should map to CompletionItemKind.Variable (6)."""
        assert _symbol_kind_to_completion_kind(SymbolKind.VARIABLE) == 6

    def test_constant_to_completion_kind(self):
        """Constant should map to CompletionItemKind.Constant (21)."""
        assert _symbol_kind_to_completion_kind(SymbolKind.CONSTANT) == 21


class TestCompletionHandler:
    """Tests for completion request handling."""

    def test_completion_basic(self):
        """Completion should return matching symbols."""
        server = C4LSPServer()

        test_content = '''def process_data():
    pass

def process_file():
    pass

def other_func():
    pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=9, character=4),  # After typing "proc"
        )

        # Simulate typing "proc"
        server.analyzer._file_contents["/test/file.py"] += "\nproc"

        result = server._handle_completion(params)

        assert result is not None
        assert isinstance(result, lsp.CompletionList)

        # Should find process_data and process_file
        names = [item.label for item in result.items]
        assert "process_data" in names
        assert "process_file" in names

    def test_completion_empty_prefix(self):
        """Completion with empty prefix should return all symbols."""
        server = C4LSPServer()

        test_content = '''def alpha(): pass
def beta(): pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=2, character=0),
        )

        result = server._handle_completion(params)

        assert result is not None
        names = [item.label for item in result.items]
        assert "alpha" in names
        assert "beta" in names

    def test_completion_case_insensitive(self):
        """Completion should be case-insensitive."""
        server = C4LSPServer()

        test_content = '''def MyFunction(): pass
def myfunction(): pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        # Simulate typing "my"
        server.analyzer._file_contents["/test/file.py"] += "\nmy"

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=2, character=2),
        )

        result = server._handle_completion(params)

        assert result is not None
        names = [item.label for item in result.items]
        # Both should match (case-insensitive)
        assert "MyFunction" in names
        assert "myfunction" in names

    def test_completion_with_detail(self):
        """Completion items should have detail info."""
        server = C4LSPServer()

        test_content = '''def greet(name):
    """Say hello."""
    pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=3, character=0),
        )

        result = server._handle_completion(params)

        assert result is not None
        greet_item = next((i for i in result.items if i.label == "greet"), None)
        assert greet_item is not None
        assert greet_item.detail is not None

    def test_completion_resolve(self):
        """Completion resolve should add documentation."""
        server = C4LSPServer()

        test_content = '''def greet(name):
    """Say hello to someone."""
    pass
'''
        server.analyzer.add_file("/test/file.py", test_content)

        # Create a completion item to resolve
        item = lsp.CompletionItem(
            label="greet",
            data={"name": "greet", "file_path": "/test/file.py", "line": 1},
        )

        resolved = server._handle_completion_resolve(item)

        assert resolved.documentation is not None
        assert "greet" in resolved.documentation.value or "Say hello" in resolved.documentation.value

    def test_completion_limit_results(self):
        """Completion should limit results to avoid performance issues."""
        server = C4LSPServer()

        # Create many symbols
        funcs = "\n".join([f"def func_{i}(): pass" for i in range(100)])
        server.analyzer.add_file("/test/file.py", funcs)

        params = lsp.CompletionParams(
            text_document=lsp.TextDocumentIdentifier(uri="file:///test/file.py"),
            position=lsp.Position(line=100, character=0),
        )

        result = server._handle_completion(params)

        assert result is not None
        # Should be limited to 50
        assert len(result.items) <= 50
        assert result.is_incomplete is True


class TestGetPrefixAtPosition:
    """Tests for _get_prefix_at_position helper."""

    def test_get_prefix_basic(self):
        """Should get prefix at cursor."""
        server = C4LSPServer()

        test_content = "hello_world"
        server.analyzer.add_file("/test/file.py", test_content)

        # Cursor after "hello"
        position = lsp.Position(line=0, character=5)
        prefix = server._get_prefix_at_position("file:///test/file.py", position)

        assert prefix == "hello"

    def test_get_prefix_empty(self):
        """Should return empty string at line start."""
        server = C4LSPServer()

        test_content = "hello"
        server.analyzer.add_file("/test/file.py", test_content)

        position = lsp.Position(line=0, character=0)
        prefix = server._get_prefix_at_position("file:///test/file.py", position)

        assert prefix == ""

    def test_get_prefix_nonexistent_file(self):
        """Should return empty string for non-existent file."""
        server = C4LSPServer()

        position = lsp.Position(line=0, character=5)
        prefix = server._get_prefix_at_position("file:///nonexistent.py", position)

        assert prefix == ""


class TestAsyncIndexing:
    """Tests for async workspace indexing."""

    def test_indexing_flags_initialization(self):
        """Server should initialize with indexing flags."""
        server = C4LSPServer()

        assert server._indexing_in_progress is False
        assert server._indexed_count == 0

    @pytest.mark.asyncio
    async def test_async_index_workspace_sets_flags(self, tmp_path):
        """Async indexing should set and clear flags correctly."""
        server = C4LSPServer()
        server._workspace_root = tmp_path

        # Create a test Python file
        (tmp_path / "test.py").write_text("def hello(): pass")

        await server._index_workspace_async()

        assert server._indexing_in_progress is False
        assert server._indexed_count == 1

    @pytest.mark.asyncio
    async def test_async_index_excludes_directories(self, tmp_path):
        """Async indexing should exclude common directories."""
        server = C4LSPServer()
        server._workspace_root = tmp_path

        # Create files in various directories
        (tmp_path / "main.py").write_text("def main(): pass")

        # These should be excluded
        (tmp_path / ".venv").mkdir()
        (tmp_path / ".venv" / "lib.py").write_text("def lib(): pass")

        (tmp_path / "node_modules").mkdir()
        (tmp_path / "node_modules" / "dep.py").write_text("def dep(): pass")

        (tmp_path / "__pycache__").mkdir()
        (tmp_path / "__pycache__" / "cache.py").write_text("def cache(): pass")

        await server._index_workspace_async()

        # Only main.py should be indexed
        assert server._indexed_count == 1

    @pytest.mark.asyncio
    async def test_async_index_skips_if_already_in_progress(self, tmp_path):
        """Should skip indexing if already in progress."""
        server = C4LSPServer()
        server._workspace_root = tmp_path
        server._indexing_in_progress = True

        (tmp_path / "test.py").write_text("def test(): pass")

        await server._index_workspace_async()

        # Should not have indexed (skipped)
        assert server._indexed_count == 0

    @pytest.mark.asyncio
    async def test_async_index_handles_empty_workspace(self, tmp_path):
        """Should handle workspace with no Python files."""
        server = C4LSPServer()
        server._workspace_root = tmp_path

        # No Python files
        (tmp_path / "readme.md").write_text("# Readme")

        await server._index_workspace_async()

        assert server._indexing_in_progress is False
        assert server._indexed_count == 0
