"""Tests for Tree-sitter incremental parser."""

import pytest

from c4.lsp.incremental_parser import (
    LANGUAGE_EXTENSIONS,
    ParsedSymbol,
    ParseResult,
    TreeSitterParser,
    get_tree_sitter_parser,
    reset_global_parser,
)


class TestLanguageExtensions:
    """Tests for language extension mapping."""

    def test_python_extensions(self) -> None:
        """Should map Python extensions."""
        assert LANGUAGE_EXTENSIONS[".py"] == "python"
        assert LANGUAGE_EXTENSIONS[".pyw"] == "python"

    def test_javascript_extensions(self) -> None:
        """Should map JavaScript extensions."""
        assert LANGUAGE_EXTENSIONS[".js"] == "javascript"
        assert LANGUAGE_EXTENSIONS[".jsx"] == "javascript"
        assert LANGUAGE_EXTENSIONS[".mjs"] == "javascript"

    def test_typescript_extensions(self) -> None:
        """Should map TypeScript extensions."""
        assert LANGUAGE_EXTENSIONS[".ts"] == "typescript"
        assert LANGUAGE_EXTENSIONS[".tsx"] == "typescript"


class TestParsedSymbol:
    """Tests for ParsedSymbol dataclass."""

    def test_to_dict_basic(self) -> None:
        """Should convert to dictionary."""
        symbol = ParsedSymbol(
            name="test_func",
            kind="function_definition",
            line=10,
            column=0,
            end_line=15,
            end_column=0,
        )
        result = symbol.to_dict()

        assert result["name"] == "test_func"
        assert result["kind"] == 12  # function kind
        assert result["location"]["line"] == 10
        assert result["location"]["end_line"] == 15

    def test_to_dict_with_children(self) -> None:
        """Should include children in dictionary."""
        child = ParsedSymbol(
            name="inner_func",
            kind="function_definition",
            line=11,
            column=4,
            end_line=13,
            end_column=4,
        )
        parent = ParsedSymbol(
            name="outer_class",
            kind="class_definition",
            line=10,
            column=0,
            end_line=15,
            end_column=0,
            children=[child],
        )
        result = parent.to_dict()

        assert "children" in result
        assert len(result["children"]) == 1
        assert result["children"][0]["name"] == "inner_func"


class TestParseResult:
    """Tests for ParseResult dataclass."""

    def test_has_errors_false(self) -> None:
        """Should return False when no errors."""
        result = ParseResult(language="python", symbols=[])
        assert result.has_errors is False

    def test_has_errors_true(self) -> None:
        """Should return True when errors exist."""
        result = ParseResult(
            language="python",
            symbols=[],
            errors=["Parse error at line 5"],
        )
        assert result.has_errors is True


class TestTreeSitterParser:
    """Tests for TreeSitterParser."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Create parser instance."""
        return TreeSitterParser()

    def test_detect_language_python(self, parser: TreeSitterParser) -> None:
        """Should detect Python from extension."""
        assert parser.detect_language("test.py") == "python"
        assert parser.detect_language("/path/to/module.pyw") == "python"

    def test_detect_language_javascript(self, parser: TreeSitterParser) -> None:
        """Should detect JavaScript from extension."""
        assert parser.detect_language("app.js") == "javascript"
        assert parser.detect_language("component.jsx") == "javascript"

    def test_detect_language_typescript(self, parser: TreeSitterParser) -> None:
        """Should detect TypeScript from extension."""
        assert parser.detect_language("service.ts") == "typescript"
        assert parser.detect_language("Component.tsx") == "typescript"

    def test_detect_language_unknown(self, parser: TreeSitterParser) -> None:
        """Should return None for unknown extensions."""
        assert parser.detect_language("file.rs") is None
        assert parser.detect_language("file.go") is None

    def test_get_status(self, parser: TreeSitterParser) -> None:
        """Should return status dictionary."""
        status = parser.get_status()
        assert "initialized" in status
        assert "available_languages" in status
        assert "cached_files" in status


@pytest.mark.skipif(
    not get_tree_sitter_parser().is_available,
    reason="Tree-sitter not available",
)
class TestTreeSitterParserWithTreeSitter:
    """Tests that require tree-sitter to be installed."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Create parser instance."""
        return TreeSitterParser()

    def test_parse_python_function(self, parser: TreeSitterParser) -> None:
        """Should parse Python function."""
        code = '''
def hello_world():
    print("Hello, World!")
'''
        result = parser.parse("test.py", code)

        assert result.language == "python"
        assert not result.has_errors
        assert len(result.symbols) >= 1

        func_names = [s.name for s in result.symbols]
        assert "hello_world" in func_names

    def test_parse_python_class(self, parser: TreeSitterParser) -> None:
        """Should parse Python class with methods."""
        code = '''
class MyClass:
    def __init__(self):
        self.value = 0

    def get_value(self):
        return self.value
'''
        result = parser.parse("test.py", code)

        assert not result.has_errors
        assert len(result.symbols) >= 1

        class_symbol = next((s for s in result.symbols if s.name == "MyClass"), None)
        assert class_symbol is not None
        assert class_symbol.kind == "class_definition"

    def test_parse_javascript_function(self, parser: TreeSitterParser) -> None:
        """Should parse JavaScript function."""
        if not parser.supports_language("javascript"):
            pytest.skip("JavaScript not supported")

        code = '''
function greet(name) {
    return `Hello, ${name}!`;
}
'''
        result = parser.parse("test.js", code)

        assert result.language == "javascript"
        assert not result.has_errors

    def test_parse_typescript_interface(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript interface."""
        if not parser.supports_language("typescript"):
            pytest.skip("TypeScript not supported")

        code = '''
interface User {
    name: string;
    age: number;
}

function getUser(): User {
    return { name: "John", age: 30 };
}
'''
        result = parser.parse("test.ts", code)

        assert result.language == "typescript"
        assert not result.has_errors

    def test_incremental_parse(self, parser: TreeSitterParser) -> None:
        """Should use incremental parsing on second call."""
        code_v1 = '''
def foo():
    pass
'''
        code_v2 = '''
def foo():
    return 42

def bar():
    pass
'''
        # First parse
        result1 = parser.parse("test.py", code_v1)
        assert not result1.has_errors

        # Second parse (incremental)
        result2 = parser.parse("test.py", code_v2, incremental=True)
        assert not result2.has_errors

        func_names = [s.name for s in result2.symbols]
        assert "foo" in func_names
        assert "bar" in func_names

    def test_invalidate_cache(self, parser: TreeSitterParser) -> None:
        """Should invalidate cached tree."""
        code = "def test(): pass"
        parser.parse("test.py", code)

        # Should have cached
        assert parser.get_status()["cached_files"] >= 1

        # Invalidate
        removed = parser.invalidate("test.py")
        assert removed is True

        # Invalidate non-existent
        removed = parser.invalidate("nonexistent.py")
        assert removed is False

    def test_clear_cache(self, parser: TreeSitterParser) -> None:
        """Should clear all cached trees."""
        parser.parse("test1.py", "def a(): pass")
        parser.parse("test2.py", "def b(): pass")

        count = parser.clear()
        assert count >= 2
        assert parser.get_status()["cached_files"] == 0


class TestGlobalParser:
    """Tests for global parser instance."""

    def test_get_tree_sitter_parser_returns_same_instance(self) -> None:
        """Should return the same instance."""
        reset_global_parser()
        parser1 = get_tree_sitter_parser()
        parser2 = get_tree_sitter_parser()
        assert parser1 is parser2

    def test_reset_global_parser(self) -> None:
        """Should reset the global instance."""
        parser1 = get_tree_sitter_parser()
        assert parser1 is not None
        reset_global_parser()
        parser2 = get_tree_sitter_parser()
        # After reset, should be a new instance
        assert parser2 is not None


class TestParseUnsupportedLanguage:
    """Tests for unsupported language handling."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Create parser instance."""
        return TreeSitterParser()

    def test_parse_unsupported_returns_error(self, parser: TreeSitterParser) -> None:
        """Should return error for unsupported language."""
        result = parser.parse("test.rs", "fn main() {}")

        assert result.language is None or result.language == "unknown"
        assert result.has_errors
        assert len(result.symbols) == 0
