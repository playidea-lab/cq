"""Unit tests for C4LSPServer MCP tool methods."""

import tempfile
from pathlib import Path

import pytest

from c4.lsp.jedi_provider import JEDI_AVAILABLE

# Skip all tests if jedi is not available
pytestmark = pytest.mark.skipif(not JEDI_AVAILABLE, reason="jedi not installed")


class TestServerFindSymbol:
    """Tests for C4LSPServer.find_symbol MCP tool method."""

    @pytest.fixture
    def sample_code(self) -> str:
        """Sample Python code for testing."""
        return '''
class TestClass:
    """A test class."""

    def test_method(self) -> None:
        pass


def test_function() -> int:
    return 42
'''

    @pytest.fixture
    def temp_workspace(self, sample_code: str) -> Path:
        """Create a temporary workspace."""
        with tempfile.TemporaryDirectory() as tmpdir:
            workspace = Path(tmpdir)
            (workspace / "test_module.py").write_text(sample_code)
            yield workspace

    @pytest.fixture
    def mock_server(self, temp_workspace: Path):
        """Create a mock server with jedi provider."""
        # Import here to avoid import errors if pygls not available
        try:
            from c4.lsp.server import C4LSPServer

            server = C4LSPServer()
            server._workspace_root = temp_workspace

            # Initialize jedi provider
            from c4.lsp.jedi_provider import JediSymbolProvider

            server._jedi_provider = JediSymbolProvider(project_path=temp_workspace)
            return server
        except ImportError:
            pytest.skip("pygls not available")

    def test_find_symbol_returns_list(self, mock_server) -> None:
        """Test that find_symbol returns a list."""
        result = mock_server.find_symbol("TestClass")
        assert isinstance(result, list)

    def test_find_symbol_with_relative_path(
        self, mock_server, sample_code: str
    ) -> None:
        """Test find_symbol with relative path."""
        result = mock_server.find_symbol("TestClass", relative_path="test_module.py")

        assert len(result) >= 1
        assert any(s["name"] == "TestClass" for s in result)

    def test_find_symbol_method_pattern(self, mock_server) -> None:
        """Test find_symbol with class/method pattern."""
        result = mock_server.find_symbol(
            "TestClass/test_method", relative_path="test_module.py"
        )

        assert len(result) >= 1
        assert any(s["name"] == "test_method" for s in result)

    def test_find_symbol_not_found(self, mock_server) -> None:
        """Test find_symbol with non-existent symbol."""
        result = mock_server.find_symbol(
            "NonExistentSymbol", relative_path="test_module.py"
        )
        assert len(result) == 0

    def test_find_symbol_no_jedi_provider(self, mock_server) -> None:
        """Test find_symbol when jedi provider is not available."""
        mock_server._jedi_provider = None
        result = mock_server.find_symbol("TestClass")
        assert result == []


class TestServerGetSymbolsOverview:
    """Tests for C4LSPServer.get_symbols_overview MCP tool method."""

    @pytest.fixture
    def sample_code(self) -> str:
        """Sample Python code for testing."""
        return '''
class OverviewClass:
    """A class for overview testing."""

    def method_one(self) -> None:
        pass

    def method_two(self) -> int:
        return 42


def overview_function() -> str:
    return "hello"


CONSTANT_VALUE = 100
'''

    @pytest.fixture
    def temp_workspace(self, sample_code: str) -> Path:
        """Create a temporary workspace."""
        with tempfile.TemporaryDirectory() as tmpdir:
            workspace = Path(tmpdir)
            (workspace / "overview.py").write_text(sample_code)
            yield workspace

    @pytest.fixture
    def mock_server(self, temp_workspace: Path):
        """Create a mock server with jedi provider."""
        try:
            from c4.lsp.server import C4LSPServer

            server = C4LSPServer()
            server._workspace_root = temp_workspace

            from c4.lsp.jedi_provider import JediSymbolProvider

            server._jedi_provider = JediSymbolProvider(project_path=temp_workspace)
            return server
        except ImportError:
            pytest.skip("pygls not available")

    def test_get_symbols_overview_returns_dict(self, mock_server) -> None:
        """Test that get_symbols_overview returns a dict."""
        result = mock_server.get_symbols_overview("overview.py")
        assert isinstance(result, dict)

    def test_get_symbols_overview_has_file_key(self, mock_server) -> None:
        """Test that result has file key."""
        result = mock_server.get_symbols_overview("overview.py")
        assert "file" in result
        assert result["file"] == "overview.py"

    def test_get_symbols_overview_has_symbols_by_kind(self, mock_server) -> None:
        """Test that result has symbols_by_kind."""
        result = mock_server.get_symbols_overview("overview.py")
        assert "symbols_by_kind" in result
        assert isinstance(result["symbols_by_kind"], dict)

    def test_get_symbols_overview_has_total_count(self, mock_server) -> None:
        """Test that result has total_count."""
        result = mock_server.get_symbols_overview("overview.py")
        assert "total_count" in result
        assert isinstance(result["total_count"], int)

    def test_get_symbols_overview_finds_class(self, mock_server) -> None:
        """Test that overview finds the class."""
        result = mock_server.get_symbols_overview("overview.py")

        symbols_by_kind = result["symbols_by_kind"]
        assert "class" in symbols_by_kind
        class_names = [s["name"] for s in symbols_by_kind["class"]]
        assert "OverviewClass" in class_names

    def test_get_symbols_overview_finds_function(self, mock_server) -> None:
        """Test that overview finds the function."""
        result = mock_server.get_symbols_overview("overview.py")

        symbols_by_kind = result["symbols_by_kind"]
        assert "function" in symbols_by_kind
        func_names = [s["name"] for s in symbols_by_kind["function"]]
        assert "overview_function" in func_names

    def test_get_symbols_overview_file_not_found(self, mock_server) -> None:
        """Test get_symbols_overview with non-existent file."""
        result = mock_server.get_symbols_overview("nonexistent.py")
        assert "error" in result

    def test_get_symbols_overview_no_workspace(self, mock_server) -> None:
        """Test get_symbols_overview without workspace root."""
        mock_server._workspace_root = None
        result = mock_server.get_symbols_overview("overview.py")
        assert "error" in result

    def test_get_symbols_overview_no_jedi_provider(self, mock_server) -> None:
        """Test get_symbols_overview when jedi provider is not available."""
        mock_server._jedi_provider = None
        result = mock_server.get_symbols_overview("overview.py")
        assert "error" in result
