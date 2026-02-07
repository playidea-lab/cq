"""Tests for LSP → MCP reference finding integration.

Tests cover:
1. MultilspyProvider.find_references() + end_line extraction
2. UnifiedSymbolProvider.find_references() with fallback chain
3. CodeOps LSP-first integration (_get_symbol_by_name_path, find_referencing_symbols)
"""

from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

# ============================================================================
# Phase 1: MultilspyProvider tests
# ============================================================================


class TestMultilspyEndLine:
    """Test end_line/end_column extraction in _convert_workspace_symbol."""

    def _make_provider(self):
        """Create a MultilspyProvider with mocked multilspy."""
        with patch("c4.lsp.multilspy_provider.MULTILSPY_AVAILABLE", True):
            with patch("c4.lsp.multilspy_provider.MultilspyProvider.__init__", lambda self, **kw: None):
                from c4.lsp.multilspy_provider import MultilspyProvider

                provider = MultilspyProvider.__new__(MultilspyProvider)
                provider.project_path = Path("/fake")
                provider.timeout = 30
                provider.idle_timeout = 300
                provider._servers = {}
                provider._global_lock = __import__("threading").Lock()
                return provider

    def test_extract_end_line(self):
        provider = self._make_provider()
        location = SimpleNamespace(
            range=SimpleNamespace(
                start=SimpleNamespace(line=10, character=4),
                end=SimpleNamespace(line=20, character=0),
            ),
            uri="file:///fake/test.py",
        )
        assert provider._extract_end_line(location) == 20
        assert provider._extract_end_column(location) == 0

    def test_extract_end_line_none_location(self):
        provider = self._make_provider()
        assert provider._extract_end_line(None) == 0
        assert provider._extract_end_column(None) == 0

    def test_extract_end_line_no_range(self):
        provider = self._make_provider()
        location = SimpleNamespace()  # no range attr
        assert provider._extract_end_line(location) == 0

    def test_convert_workspace_symbol_includes_end_line(self):
        provider = self._make_provider()

        sym = SimpleNamespace(
            name="my_func",
            kind=SimpleNamespace(value=12),  # function
            container_name="MyClass",
            location=SimpleNamespace(
                uri="file:///fake/test.py",
                range=SimpleNamespace(
                    start=SimpleNamespace(line=5, character=4),
                    end=SimpleNamespace(line=15, character=0),
                ),
            ),
        )

        result = provider._convert_workspace_symbol(sym)
        assert result is not None
        assert result["location"]["end_line"] == 15
        assert result["location"]["end_column"] == 0
        assert result["location"]["line"] == 5
        assert result["location"]["column"] == 4


class TestMultilspyFindReferences:
    """Test MultilspyProvider.find_references()."""

    def _make_provider(self, mock_server=None):
        with patch("c4.lsp.multilspy_provider.MULTILSPY_AVAILABLE", True):
            with patch("c4.lsp.multilspy_provider.MultilspyProvider.__init__", lambda self, **kw: None):
                from c4.lsp.multilspy_provider import MultilspyProvider

                provider = MultilspyProvider.__new__(MultilspyProvider)
                provider.project_path = Path("/fake/project")
                provider.timeout = 30
                provider.idle_timeout = 300
                provider._servers = {}
                provider._global_lock = __import__("threading").Lock()
                return provider

    def test_find_references_returns_locations(self):
        provider = self._make_provider()

        mock_server = MagicMock()
        mock_server.start_server.return_value.__enter__ = MagicMock()
        mock_server.start_server.return_value.__exit__ = MagicMock(return_value=False)
        mock_server.request_references.return_value = [
            {
                "uri": "file:///fake/project/test.py",
                "absolutePath": "/fake/project/test.py",
                "relativePath": "test.py",
                "range": {
                    "start": {"line": 10, "character": 4},
                    "end": {"line": 10, "character": 12},
                },
            },
            {
                "uri": "file:///fake/project/other.py",
                "absolutePath": "/fake/project/other.py",
                "relativePath": "other.py",
                "range": {
                    "start": {"line": 5, "character": 0},
                    "end": {"line": 5, "character": 8},
                },
            },
        ]

        with patch.object(provider, "_get_server", return_value=mock_server):
            with patch.object(provider, "_detect_language", return_value="python"):
                results = provider.find_references(
                    file_path="/fake/project/test.py",
                    line=10,
                    column=4,
                )

        assert len(results) == 2
        assert results[0]["file_path"] == "/fake/project/test.py"
        assert results[0]["line"] == 10
        assert results[0]["column"] == 4
        assert results[0]["end_line"] == 10
        assert results[0]["end_column"] == 12
        assert results[1]["file_path"] == "/fake/project/other.py"

    def test_find_references_empty_result(self):
        provider = self._make_provider()

        mock_server = MagicMock()
        mock_server.start_server.return_value.__enter__ = MagicMock()
        mock_server.start_server.return_value.__exit__ = MagicMock(return_value=False)
        mock_server.request_references.return_value = []

        with patch.object(provider, "_get_server", return_value=mock_server):
            with patch.object(provider, "_detect_language", return_value="python"):
                results = provider.find_references("/fake/project/test.py", 0, 0)

        assert results == []

    def test_find_references_unsupported_language(self):
        provider = self._make_provider()

        with patch.object(provider, "_detect_language", return_value=None):
            results = provider.find_references("/fake/project/test.txt", 0, 0)

        assert results == []

    def test_find_references_server_error(self):
        provider = self._make_provider()

        with patch.object(provider, "_detect_language", return_value="python"):
            with patch.object(provider, "_get_server", side_effect=RuntimeError("Server not available")):
                results = provider.find_references("/fake/project/test.py", 0, 0)

        assert results == []


# ============================================================================
# Phase 2: UnifiedSymbolProvider tests
# ============================================================================


class TestUnifiedFindReferences:
    """Test UnifiedSymbolProvider.find_references() with fallback chain."""

    def _make_provider(self, multilspy_available=True, jedi_available=True):
        with patch("c4.lsp.unified_provider.MULTILSPY_AVAILABLE", multilspy_available):
            with patch("c4.lsp.unified_provider.JEDI_AVAILABLE", jedi_available):
                with patch("c4.lsp.unified_provider.MultilspyProvider"):
                    from c4.lsp.unified_provider import UnifiedSymbolProvider

                    provider = UnifiedSymbolProvider.__new__(UnifiedSymbolProvider)
                    provider.project_path = Path("/fake/project")
                    provider.timeout = 30
                    provider.prefer_multilspy = True
                    provider.use_isolated_jedi = False

                    if multilspy_available:
                        provider._multilspy = MagicMock()
                    else:
                        provider._multilspy = None
                    provider._jedi = MagicMock() if jedi_available else None

                    return provider

    def test_multilspy_primary(self):
        provider = self._make_provider()
        provider._multilspy.find_references.return_value = [
            {"file_path": "/fake/project/a.py", "line": 5, "column": 0, "end_line": 5, "end_column": 8},
        ]

        results = provider.find_references("/fake/project/test.py", 10, 4)

        assert len(results) == 1
        assert results[0]["line"] == 5
        provider._multilspy.find_references.assert_called_once()

    def test_jedi_fallback_when_multilspy_fails(self):
        provider = self._make_provider()
        provider._multilspy.find_references.side_effect = RuntimeError("LSP error")

        with patch.object(provider, "_find_references_jedi", return_value=[
            {"file_path": "/fake/project/a.py", "line": 3, "column": 0, "end_line": 3, "end_column": 5},
        ]) as mock_jedi:
            with patch.object(provider, "_detect_language", return_value="python"):
                results = provider.find_references("/fake/project/test.py", 10, 4)

        assert len(results) == 1
        assert results[0]["line"] == 3
        mock_jedi.assert_called_once()

    def test_jedi_fallback_when_multilspy_empty(self):
        provider = self._make_provider()
        provider._multilspy.find_references.return_value = []

        with patch.object(provider, "_find_references_jedi", return_value=[
            {"file_path": "/fake/project/a.py", "line": 7, "column": 0, "end_line": 7, "end_column": 5},
        ]) as mock_jedi:
            with patch.object(provider, "_detect_language", return_value="python"):
                results = provider.find_references("/fake/project/test.py", 10, 4)

        assert len(results) == 1
        mock_jedi.assert_called_once()

    def test_jedi_only_when_multilspy_unavailable(self):
        provider = self._make_provider(multilspy_available=False)

        with patch.object(provider, "_find_references_jedi", return_value=[
            {"file_path": "/fake/project/a.py", "line": 1, "column": 0, "end_line": 1, "end_column": 3},
        ]):
            with patch.object(provider, "_detect_language", return_value="python"):
                results = provider.find_references("/fake/project/test.py", 0, 0)

        assert len(results) == 1

    def test_no_results_for_non_python_without_multilspy(self):
        provider = self._make_provider(multilspy_available=False)

        with patch.object(provider, "_detect_language", return_value="typescript"):
            results = provider.find_references("/fake/project/test.ts", 0, 0)

        assert results == []


class TestFindReferencesJedi:
    """Test _find_references_jedi with real Jedi."""

    def test_find_references_in_source(self, tmp_path):
        from c4.lsp.unified_provider import UnifiedSymbolProvider

        # Create a test file
        test_file = tmp_path / "test.py"
        test_file.write_text("def foo(): pass\nfoo()\nprint(foo)\n")

        with patch("c4.lsp.unified_provider.MULTILSPY_AVAILABLE", False):
            provider = UnifiedSymbolProvider(project_path=str(tmp_path))

        # Line 0, col 4 = "foo" in "def foo(): pass"
        results = provider._find_references_jedi(test_file, line=0, column=4)

        assert len(results) >= 2  # definition + at least 1 usage
        # All results should reference "foo"
        for ref in results:
            assert ref["file_path"] == str(test_file)
            assert isinstance(ref["line"], int)
            assert isinstance(ref["end_line"], int)


# ============================================================================
# Phase 3: CodeOps LSP integration tests
# ============================================================================


class TestSymbolProxy:
    """Test _SymbolProxy line indexing conversion."""

    def test_lsp_0indexed_to_1indexed(self):
        from c4.daemon.code_ops import _SymbolProxy

        d = {
            "name": "my_func",
            "name_path": "MyClass/my_func",
            "location": {
                "file_path": "/fake/test.py",
                "line": 10,        # 0-indexed
                "column": 4,
                "end_line": 20,    # 0-indexed
                "end_column": 0,
            },
        }
        proxy = _SymbolProxy(d)

        assert proxy.name == "my_func"
        assert proxy.location.start_line == 11  # 1-indexed
        assert proxy.location.end_line == 21    # 1-indexed
        assert proxy.location.start_column == 4
        assert proxy.location.end_column == 0
        assert proxy.location.file_path == "/fake/test.py"

    def test_qualified_name_from_name_path(self):
        from c4.daemon.code_ops import _SymbolProxy

        d = {
            "name": "method",
            "name_path": "ClassName/method",
            "location": {
                "file_path": "/fake/test.py",
                "line": 0,
                "column": 0,
                "end_line": 5,
                "end_column": 0,
            },
        }
        proxy = _SymbolProxy(d)
        assert proxy.qualified_name == "ClassName/method"

    def test_parent_name(self):
        from c4.daemon.code_ops import _SymbolProxy

        d = {
            "name": "method",
            "parent_name": "ClassName",
            "location": {
                "file_path": "/fake/test.py",
                "line": 0,
                "column": 0,
                "end_line": 5,
                "end_column": 0,
            },
        }
        proxy = _SymbolProxy(d)
        assert proxy.parent == "ClassName"


class TestSymbolProxyJediFormat:
    """Test _SymbolProxy with Jedi worker flat dict format."""

    def test_jedi_flat_dict_1indexed(self):
        from c4.daemon.code_ops import _SymbolProxy

        d = {
            "name": "Calculator",
            "type": "class",
            "line": 1,           # Jedi: 1-indexed
            "column": 0,
            "end_line": 10,      # Jedi: 1-indexed
            "end_column": 0,
            "module_path": "/fake/test.py",
            "parent_name": None,
        }
        proxy = _SymbolProxy(d)

        assert proxy.name == "Calculator"
        assert proxy.location.start_line == 1   # stays 1-indexed
        assert proxy.location.end_line == 10    # stays 1-indexed
        assert proxy.location.file_path == "/fake/test.py"

    def test_jedi_flat_dict_no_end_line_uses_line(self):
        from c4.daemon.code_ops import _SymbolProxy

        d = {
            "name": "x",
            "type": "variable",
            "line": 5,
            "column": 0,
            # no end_line
            "module_path": "/fake/test.py",
        }
        proxy = _SymbolProxy(d)

        assert proxy.location.start_line == 5
        assert proxy.location.end_line == 5  # falls back to line


class TestCodeOpsLSPIntegration:
    """Test CodeOps methods with LSP-first, Tree-sitter fallback."""

    def _make_code_ops(self, root: Path | None = None):
        from c4.daemon.code_ops import CodeOps

        daemon = MagicMock()
        daemon.root = root or Path("/fake/project")
        return CodeOps(daemon)

    def test_get_symbol_by_name_path_lsp_first(self):
        ops = self._make_code_ops()

        mock_symbols = [{
            "name": "my_func",
            "name_path": "my_func",
            "location": {
                "file_path": "/fake/project/test.py",
                "line": 10,
                "column": 0,
                "end_line": 20,
                "end_column": 0,
            },
        }]

        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=mock_symbols):
            symbol, file_path, error = ops._get_symbol_by_name_path("my_func", "test.py")

        assert error is None
        assert symbol is not None
        assert symbol.name == "my_func"
        assert symbol.location.start_line == 11  # 0-indexed → 1-indexed
        assert symbol.location.end_line == 21
        assert file_path == "/fake/project/test.py"

    def test_get_symbol_by_name_path_fallback_to_treesitter(self):
        ops = self._make_code_ops()

        # LSP returns no results (or no end_line)
        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=[]):
            # Tree-sitter should be called
            with patch.object(ops, "_get_symbol_via_treesitter", return_value=(
                MagicMock(name="my_func"), "/fake/test.py", None
            )) as mock_ts:
                symbol, file_path, error = ops._get_symbol_by_name_path("my_func")

        mock_ts.assert_called_once()

    def test_get_symbol_lsp_no_end_line_falls_back(self):
        """LSP result without end_line should fallback to Tree-sitter."""
        ops = self._make_code_ops()

        # LSP returns result but without end_line
        mock_symbols = [{
            "name": "my_func",
            "name_path": "my_func",
            "location": {
                "file_path": "/fake/project/test.py",
                "line": 10,
                "column": 0,
                # No end_line
            },
        }]

        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=mock_symbols):
            with patch.object(ops, "_get_symbol_via_treesitter", return_value=(
                MagicMock(), "/fake/test.py", None
            )) as mock_ts:
                ops._get_symbol_by_name_path("my_func", "test.py")

        mock_ts.assert_called_once()

    def test_get_symbol_by_name_path_jedi_flat_dict(self):
        """Jedi flat dict with end_line should be usable via LSP path."""
        ops = self._make_code_ops()

        # Jedi returns flat dict (no "location" key)
        mock_symbols = [{
            "name": "Calculator",
            "type": "class",
            "line": 1,
            "column": 0,
            "end_line": 10,
            "end_column": 0,
            "module_path": "/fake/project/test.py",
            "parent_name": None,
        }]

        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=mock_symbols):
            symbol, file_path, error = ops._get_symbol_by_name_path("Calculator", "test.py")

        assert error is None
        assert symbol is not None
        assert symbol.name == "Calculator"
        assert symbol.location.start_line == 1   # Jedi: already 1-indexed
        assert symbol.location.end_line == 10
        assert file_path == "/fake/project/test.py"

    def test_find_referencing_symbols_lsp_first(self):
        ops = self._make_code_ops()

        mock_symbols = [{
            "name": "my_func",
            "name_path": "my_func",
            "location": {
                "file_path": "/fake/project/test.py",
                "line": 10,
                "column": 0,
            },
        }]

        mock_refs = [
            {"file_path": "/fake/project/a.py", "line": 5, "column": 0, "end_line": 5, "end_column": 7},
            {"file_path": "/fake/project/b.py", "line": 3, "column": 8, "end_line": 3, "end_column": 15},
        ]

        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=mock_symbols):
            with patch("c4.lsp.unified_provider.find_references_unified", return_value=mock_refs):
                result = ops.find_referencing_symbols("my_func")

        assert result["success"] is True
        assert result["total"] == 2
        assert result["references"] == mock_refs

    def test_find_referencing_symbols_fallback_to_treesitter(self):
        ops = self._make_code_ops()

        # LSP returns no symbols → fallback
        with patch("c4.lsp.unified_provider.find_symbol_unified", return_value=[]):
            with patch.object(ops, "_find_references_via_treesitter", return_value={
                "success": True, "references": [], "total": 0, "symbol": "my_func",
            }) as mock_ts:
                ops.find_referencing_symbols("my_func")

        mock_ts.assert_called_once()

    def test_find_referencing_symbols_lsp_import_error(self):
        ops = self._make_code_ops()

        with patch("c4.lsp.unified_provider.find_symbol_unified", side_effect=ImportError):
            with patch.object(ops, "_find_references_via_treesitter", return_value={
                "success": True, "references": [], "total": 0, "symbol": "my_func",
            }) as mock_ts:
                ops.find_referencing_symbols("my_func")

        mock_ts.assert_called_once()


# ============================================================================
# Convenience function tests
# ============================================================================


class TestConvenienceFunctions:
    """Test module-level convenience functions."""

    def test_find_references_multilspy_not_available(self):
        with patch("c4.lsp.multilspy_provider.MULTILSPY_AVAILABLE", False):
            from c4.lsp.multilspy_provider import find_references_multilspy

            result = find_references_multilspy("/fake/test.py", 0, 0)
            assert result == []

    def test_find_references_unified_provider_error(self):
        with patch("c4.lsp.unified_provider.get_provider", side_effect=Exception("fail")):
            from c4.lsp.unified_provider import find_references_unified

            result = find_references_unified("/fake/test.py", 0, 0)
            assert result == []
