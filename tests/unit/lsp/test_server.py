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
