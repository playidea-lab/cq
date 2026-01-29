"""Unit tests for JediSymbolProvider."""

import tempfile
from pathlib import Path

import pytest

from c4.lsp.jedi_provider import (
    JEDI_AVAILABLE,
    JediSymbolProvider,
    LSPSymbolKind,
    SymbolInfo,
    SymbolLocation,
    SymbolType,
    _jedi_type_to_symbol_type,
    _symbol_type_to_lsp_kind,
)

# Skip all tests if jedi is not available
pytestmark = pytest.mark.skipif(not JEDI_AVAILABLE, reason="jedi not installed")


class TestSymbolTypes:
    """Tests for symbol type conversions."""

    def test_jedi_type_class(self) -> None:
        """Test class type conversion."""
        assert _jedi_type_to_symbol_type("class") == SymbolType.CLASS

    def test_jedi_type_function(self) -> None:
        """Test function type conversion."""
        assert _jedi_type_to_symbol_type("function") == SymbolType.FUNCTION

    def test_jedi_type_function_in_class(self) -> None:
        """Test function inside class becomes method."""
        assert _jedi_type_to_symbol_type("function", "class") == SymbolType.METHOD

    def test_jedi_type_property(self) -> None:
        """Test property type conversion."""
        assert _jedi_type_to_symbol_type("property") == SymbolType.PROPERTY

    def test_jedi_type_unknown(self) -> None:
        """Test unknown type conversion."""
        assert _jedi_type_to_symbol_type("unknown_type") == SymbolType.UNKNOWN

    def test_symbol_type_to_lsp_kind_class(self) -> None:
        """Test symbol type to LSP kind conversion."""
        assert _symbol_type_to_lsp_kind(SymbolType.CLASS) == LSPSymbolKind.CLASS

    def test_symbol_type_to_lsp_kind_method(self) -> None:
        """Test method type to LSP kind conversion."""
        assert _symbol_type_to_lsp_kind(SymbolType.METHOD) == LSPSymbolKind.METHOD


class TestSymbolInfo:
    """Tests for SymbolInfo dataclass."""

    def test_symbol_info_creation(self) -> None:
        """Test SymbolInfo creation."""
        loc = SymbolLocation(file_path="/test.py", line=10, column=0)
        symbol = SymbolInfo(
            name="test_func",
            kind=SymbolType.FUNCTION,
            location=loc,
            qualified_name="module.test_func",
        )

        assert symbol.name == "test_func"
        assert symbol.kind == SymbolType.FUNCTION
        assert symbol.location.file_path == "/test.py"
        assert symbol.qualified_name == "module.test_func"

    def test_symbol_info_to_dict(self) -> None:
        """Test SymbolInfo to_dict method."""
        loc = SymbolLocation(file_path="/test.py", line=10, column=0)
        symbol = SymbolInfo(
            name="test_func",
            kind=SymbolType.FUNCTION,
            location=loc,
        )

        result = symbol.to_dict()
        assert result["name"] == "test_func"
        assert result["kind"] == "function"
        assert result["location"]["file_path"] == "/test.py"


class TestSymbolLocation:
    """Tests for SymbolLocation dataclass."""

    def test_symbol_location_creation(self) -> None:
        """Test SymbolLocation creation."""
        loc = SymbolLocation(
            file_path="/test.py",
            line=10,
            column=4,
            end_line=15,
            end_column=0,
        )

        assert loc.file_path == "/test.py"
        assert loc.line == 10
        assert loc.column == 4
        assert loc.end_line == 15
        assert loc.end_column == 0

    def test_symbol_location_to_dict(self) -> None:
        """Test SymbolLocation to_dict method."""
        loc = SymbolLocation(file_path="/test.py", line=10, column=4)

        result = loc.to_dict()
        assert result["file_path"] == "/test.py"
        assert result["line"] == 10
        assert result["column"] == 4


class TestJediSymbolProvider:
    """Tests for JediSymbolProvider."""

    @pytest.fixture
    def sample_code(self) -> str:
        """Sample Python code for testing."""
        return '''
class MyClass:
    """A sample class."""

    def __init__(self, value: int) -> None:
        self.value = value

    def get_value(self) -> int:
        """Get the value."""
        return self.value

    @property
    def doubled(self) -> int:
        """Get doubled value."""
        return self.value * 2


def standalone_function(x: int) -> int:
    """A standalone function."""
    return x + 1


CONSTANT = 42
'''

    @pytest.fixture
    def temp_file(self, sample_code: str) -> Path:
        """Create a temporary Python file."""
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".py", delete=False
        ) as f:
            f.write(sample_code)
            return Path(f.name)

    @pytest.fixture
    def provider(self) -> JediSymbolProvider:
        """Create a JediSymbolProvider instance."""
        return JediSymbolProvider()

    def test_find_symbol_class(
        self, provider: JediSymbolProvider, sample_code: str, temp_file: Path
    ) -> None:
        """Test finding a class symbol."""
        symbols = provider.find_symbol(
            "MyClass",
            source=sample_code,
            file_path=str(temp_file),
        )

        assert len(symbols) >= 1
        class_symbol = next((s for s in symbols if s.kind == SymbolType.CLASS), None)
        assert class_symbol is not None
        assert class_symbol.name == "MyClass"

    def test_find_symbol_function(
        self, provider: JediSymbolProvider, sample_code: str, temp_file: Path
    ) -> None:
        """Test finding a function symbol."""
        symbols = provider.find_symbol(
            "standalone_function",
            source=sample_code,
            file_path=str(temp_file),
        )

        assert len(symbols) >= 1
        func_symbol = next((s for s in symbols if s.kind == SymbolType.FUNCTION), None)
        assert func_symbol is not None
        assert func_symbol.name == "standalone_function"

    def test_find_symbol_method(
        self, provider: JediSymbolProvider, sample_code: str, temp_file: Path
    ) -> None:
        """Test finding a method symbol with class path."""
        symbols = provider.find_symbol(
            "MyClass/get_value",
            source=sample_code,
            file_path=str(temp_file),
        )

        assert len(symbols) >= 1
        method_symbol = next(
            (s for s in symbols if s.kind == SymbolType.METHOD), None
        )
        assert method_symbol is not None
        assert method_symbol.name == "get_value"

    def test_find_symbol_not_found(
        self, provider: JediSymbolProvider, sample_code: str, temp_file: Path
    ) -> None:
        """Test finding a non-existent symbol."""
        symbols = provider.find_symbol(
            "nonexistent_symbol",
            source=sample_code,
            file_path=str(temp_file),
        )

        assert len(symbols) == 0

    def test_get_symbols_overview(
        self, provider: JediSymbolProvider, sample_code: str, temp_file: Path
    ) -> None:
        """Test getting symbols overview."""
        symbols = provider.get_symbols_overview(
            str(temp_file),
            source=sample_code,
            depth=0,
        )

        # Should have class, function, and constant
        assert len(symbols) >= 2

        names = [s.name for s in symbols]
        assert "MyClass" in names
        assert "standalone_function" in names

    def test_get_symbols_overview_nonexistent_file(
        self, provider: JediSymbolProvider
    ) -> None:
        """Test getting overview for non-existent file."""
        symbols = provider.get_symbols_overview("/nonexistent/file.py")
        assert len(symbols) == 0


class TestJediSymbolProviderWithProject:
    """Tests for JediSymbolProvider with project context."""

    @pytest.fixture
    def temp_project(self) -> Path:
        """Create a temporary project directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            project_dir = Path(tmpdir)

            # Create a simple Python file
            (project_dir / "module.py").write_text('''
class ProjectClass:
    """A class in the project."""

    def project_method(self) -> str:
        return "hello"


def project_function() -> int:
    return 42
''')

            yield project_dir

    def test_provider_with_project_path(self, temp_project: Path) -> None:
        """Test provider initialization with project path."""
        provider = JediSymbolProvider(project_path=temp_project)
        assert provider._project is not None

    def test_workspace_symbols(self, temp_project: Path) -> None:
        """Test workspace symbol search."""
        provider = JediSymbolProvider(project_path=temp_project)
        symbols = provider.workspace_symbols("project", max_results=10)

        # Should find ProjectClass and project_function and project_method
        names = [s.name for s in symbols]
        assert any("project" in name.lower() for name in names)
