"""Tests for c4.daemon.code_ops module.

Covers:
- _LocationProxy construction
- _SymbolProxy multilspy and Jedi formats
- replace_symbol_body (success, indentation, symbol-not-found)
- insert_before_symbol / insert_after_symbol (success, symbol-not-found)
- rename_symbol (success, invalid identifier)
- find_symbol (no relative_path error, mocked provider)
- get_symbols_overview (mocked provider)
- find_referencing_symbols (tree-sitter fallback)
- File operation delegates (read_file, create_text_file, replace_content)
"""

from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace
from typing import Any
from unittest.mock import MagicMock, patch

import pytest

from c4.daemon.code_ops import CodeOps, _LocationProxy, _SymbolProxy

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture()
def mock_daemon(tmp_path: Path) -> SimpleNamespace:
    """Create a minimal daemon mock with a root attribute."""
    return SimpleNamespace(root=tmp_path)


@pytest.fixture()
def code_ops(mock_daemon: SimpleNamespace) -> CodeOps:
    """Create a CodeOps instance backed by the mock daemon."""
    return CodeOps(mock_daemon)


def _make_symbol_proxy(
    file_path: str,
    start_line: int,
    end_line: int,
    name: str = "my_func",
) -> _SymbolProxy:
    """Helper: build a _SymbolProxy from Jedi-flat format (1-indexed)."""
    return _SymbolProxy({
        "name": name,
        "module_path": file_path,
        "line": start_line,
        "end_line": end_line,
        "column": 0,
        "end_column": 0,
    })


# ---------------------------------------------------------------------------
# 1. _LocationProxy construction
# ---------------------------------------------------------------------------


class TestLocationProxy:
    def test_construction(self) -> None:
        loc = _LocationProxy(
            file_path="/tmp/a.py",
            start_line=10,
            start_column=4,
            end_line=20,
            end_column=0,
        )
        assert loc.file_path == "/tmp/a.py"
        assert loc.start_line == 10
        assert loc.start_column == 4
        assert loc.end_line == 20
        assert loc.end_column == 0


# ---------------------------------------------------------------------------
# 2-3. _SymbolProxy formats
# ---------------------------------------------------------------------------


class TestSymbolProxy:
    def test_multilspy_format(self) -> None:
        """multilspy provides nested location dict with 0-indexed lines."""
        d: dict[str, Any] = {
            "name": "greet",
            "parent_name": "Greeter",
            "qualified_name": "Greeter.greet",
            "location": {
                "file_path": "/src/greeter.py",
                "line": 5,         # 0-indexed
                "end_line": 10,    # 0-indexed
                "column": 4,
                "end_column": 0,
            },
        }
        proxy = _SymbolProxy(d)

        assert proxy.name == "greet"
        assert proxy.parent == "Greeter"
        assert proxy.qualified_name == "Greeter.greet"
        # Lines should be converted to 1-indexed
        assert proxy.location.start_line == 6   # 5 + 1
        assert proxy.location.end_line == 11    # 10 + 1
        assert proxy.location.file_path == "/src/greeter.py"
        assert proxy.location.start_column == 4

    def test_jedi_flat_format(self) -> None:
        """Jedi provides flat dict with 1-indexed lines already."""
        d: dict[str, Any] = {
            "name": "helper",
            "module_path": "/src/utils.py",
            "line": 15,       # already 1-indexed
            "end_line": 25,
            "column": 0,
            "end_column": 0,
        }
        proxy = _SymbolProxy(d)

        assert proxy.name == "helper"
        assert proxy.parent is None  # no parent_name key
        assert proxy.qualified_name == "helper"  # falls back to name
        assert proxy.location.start_line == 15
        assert proxy.location.end_line == 25
        assert proxy.location.file_path == "/src/utils.py"


# ---------------------------------------------------------------------------
# 4-6. replace_symbol_body
# ---------------------------------------------------------------------------


class TestReplaceSymbolBody:
    def test_success(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """Replace a function body and verify file contents."""
        src = tmp_path / "mod.py"
        src.write_text(
            "def hello():\n"
            "    return 'old'\n"
            "\n"
            "x = 1\n",
            encoding="utf-8",
        )

        symbol = _make_symbol_proxy(str(src), start_line=1, end_line=2, name="hello")

        with patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src), None)):
            result = code_ops.replace_symbol_body("hello", str(src), "def hello():\n    return 'new'\n")

        assert result["success"] is True
        assert result["lines_replaced"] == 2
        content = src.read_text(encoding="utf-8")
        assert "return 'new'" in content
        assert "return 'old'" not in content
        # The trailing content should still be there
        assert "x = 1" in content

    def test_indentation_preservation(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """Non-first lines of the replacement get the original indent applied.

        The implementation detects the indent of the original first line and
        applies it to lines 2+ of the new body (lstrip + prepend indent).
        The first line of new_body is inserted as-is, so callers should
        provide it with the correct indent.
        """
        src = tmp_path / "cls.py"
        src.write_text(
            "class Foo:\n"
            "    def bar(self):\n"
            "        return 1\n"
            "\n"
            "    def baz(self):\n"
            "        return 2\n",
            encoding="utf-8",
        )

        # Symbol for 'bar' at lines 2-3 (1-indexed)
        symbol = _make_symbol_proxy(str(src), start_line=2, end_line=3, name="bar")

        # Provide the first line with correct indent (matching original "    def bar")
        new_body = "    def bar(self):\n        return 42\n"
        with patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src), None)):
            result = code_ops.replace_symbol_body("Foo.bar", str(src), new_body)

        assert result["success"] is True
        content = src.read_text(encoding="utf-8")
        # First line kept as-is, second line gets original indent ("    ") + lstripped content
        assert "    def bar(self):\n" in content
        assert "    return 42\n" in content
        # baz should be untouched
        assert "    def baz(self):" in content

    def test_symbol_not_found(self, code_ops: CodeOps) -> None:
        """When the symbol is not found, return error dict."""
        with patch.object(
            code_ops,
            "_get_symbol_by_name_path",
            return_value=(None, None, "Symbol not found: missing_func"),
        ):
            result = code_ops.replace_symbol_body("missing_func", None, "pass")

        assert result["success"] is False
        assert "Symbol not found" in result["error"]


# ---------------------------------------------------------------------------
# 7-9. insert_before_symbol / insert_after_symbol
# ---------------------------------------------------------------------------


class TestInsertBeforeSymbol:
    def test_success(self, code_ops: CodeOps, tmp_path: Path) -> None:
        src = tmp_path / "before.py"
        src.write_text(
            "import os\n"
            "\n"
            "def target():\n"
            "    pass\n",
            encoding="utf-8",
        )

        symbol = _make_symbol_proxy(str(src), start_line=3, end_line=4, name="target")

        with patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src), None)):
            result = code_ops.insert_before_symbol("target", str(src), "# inserted comment\n")

        assert result["success"] is True
        assert result["inserted_at_line"] == 3
        content = src.read_text(encoding="utf-8")
        lines = content.splitlines()
        # The comment should appear right before def target()
        idx = lines.index("def target():")
        assert lines[idx - 1] == "# inserted comment"

    def test_symbol_not_found(self, code_ops: CodeOps) -> None:
        with patch.object(
            code_ops,
            "_get_symbol_by_name_path",
            return_value=(None, None, "Symbol not found: nope"),
        ):
            result = code_ops.insert_before_symbol("nope", None, "# content")

        assert result["success"] is False
        assert "not found" in result["error"].lower()


class TestInsertAfterSymbol:
    def test_success(self, code_ops: CodeOps, tmp_path: Path) -> None:
        src = tmp_path / "after.py"
        src.write_text(
            "def first():\n"
            "    return 1\n"
            "\n"
            "def second():\n"
            "    return 2\n",
            encoding="utf-8",
        )

        symbol = _make_symbol_proxy(str(src), start_line=1, end_line=2, name="first")

        with patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src), None)):
            result = code_ops.insert_after_symbol("first", str(src), "# after first\n")

        assert result["success"] is True
        content = src.read_text(encoding="utf-8")
        lines = content.splitlines()
        # After "return 1" there should be the comment
        ret_idx = lines.index("    return 1")
        # The inserted content is placed after end_line (line 2), so after "    return 1"
        # insert_after_symbol prepends "\n" if missing, so we expect an empty line then the comment
        assert "# after first" in lines
        # The comment should appear after the first function
        comment_idx = lines.index("# after first")
        assert comment_idx > ret_idx


# ---------------------------------------------------------------------------
# 10-11. rename_symbol
# ---------------------------------------------------------------------------


class TestRenameSymbol:
    def test_success(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """Rename a symbol across files using regex word boundary."""
        src_a = tmp_path / "a.py"
        src_a.write_text(
            "def old_name():\n"
            "    return 'hello'\n",
            encoding="utf-8",
        )
        src_b = tmp_path / "b.py"
        src_b.write_text(
            "from a import old_name\n"
            "result = old_name()\n",
            encoding="utf-8",
        )

        symbol = _make_symbol_proxy(str(src_a), start_line=1, end_line=2, name="old_name")

        # Mock _get_symbol_by_name_path to return the symbol
        # Mock _find_references_via_lsp to return None (no LSP)
        # Mock _find_references_via_treesitter to return refs in both files
        ts_refs = {
            "success": True,
            "references": [
                {"file_path": str(src_a), "line": 1, "column": 4, "end_line": 1, "end_column": 12, "context": "", "ref_kind": "definition"},
                {"file_path": str(src_b), "line": 1, "column": 17, "end_line": 1, "end_column": 25, "context": "", "ref_kind": "import"},
                {"file_path": str(src_b), "line": 2, "column": 9, "end_line": 2, "end_column": 17, "context": "", "ref_kind": "call"},
            ],
            "total": 3,
            "symbol": "old_name",
        }

        with (
            patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src_a), None)),
            patch.object(code_ops, "_find_references_via_lsp", return_value=None),
            patch.object(code_ops, "_find_references_via_treesitter", return_value=ts_refs),
        ):
            result = code_ops.rename_symbol("old_name", str(src_a), "new_name")

        assert result["success"] is True
        assert result["old_name"] == "old_name"
        assert result["new_name"] == "new_name"
        assert result["total_replacements"] > 0

        # Verify file contents
        assert "new_name" in src_a.read_text()
        assert "old_name" not in src_a.read_text()
        assert "new_name" in src_b.read_text()
        assert "old_name" not in src_b.read_text()

    def test_invalid_identifier(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """Renaming to an invalid identifier should fail."""
        src = tmp_path / "inv.py"
        src.write_text("def valid():\n    pass\n", encoding="utf-8")

        symbol = _make_symbol_proxy(str(src), start_line=1, end_line=2, name="valid")

        with patch.object(code_ops, "_get_symbol_by_name_path", return_value=(symbol, str(src), None)):
            result = code_ops.rename_symbol("valid", str(src), "123-bad!")

        assert result["success"] is False
        assert "Invalid identifier" in result["error"]


# ---------------------------------------------------------------------------
# 12-13. find_symbol
# ---------------------------------------------------------------------------


class TestFindSymbol:
    def test_no_relative_path_returns_error(self, code_ops: CodeOps) -> None:
        """find_symbol without relative_path should return an error."""
        result = code_ops.find_symbol("SomeClass")
        assert result["success"] is False
        assert "relative_path is required" in result["error"]

    def test_with_mocked_provider(self, code_ops: CodeOps) -> None:
        """find_symbol delegates to find_symbol_unified."""
        mock_symbols = [
            {"name": "MyClass", "kind": "class", "name_path": "MyClass", "location": {"file_path": "mod.py", "line": 0, "end_line": 10}},
        ]

        with patch("c4.daemon.code_ops.find_symbol_unified", create=True):
            # Patch at the import location inside the method
            with patch.dict("sys.modules", {"c4.lsp.unified_provider": MagicMock(find_symbol_unified=MagicMock(return_value=mock_symbols))}):
                # Re-import to pick up the mock
                import importlib

                import c4.daemon.code_ops as code_ops_mod
                importlib.reload(code_ops_mod)
                ops = code_ops_mod.CodeOps(code_ops._daemon)
                result = ops.find_symbol("MyClass", relative_path="mod.py")

        assert result["success"] is True
        assert result["count"] == 1
        assert result["symbols"][0]["name"] == "MyClass"


# ---------------------------------------------------------------------------
# 14. get_symbols_overview
# ---------------------------------------------------------------------------


class TestGetSymbolsOverview:
    def test_with_mocked_provider(self, code_ops: CodeOps) -> None:
        """get_symbols_overview delegates to get_symbols_overview_unified."""
        mock_result = {
            "file": "mod.py",
            "symbols": [{"name": "Foo", "kind": "class"}],
        }

        mock_module = MagicMock()
        mock_module.get_symbols_overview_unified = MagicMock(return_value=mock_result)

        with patch.dict("sys.modules", {"c4.lsp.unified_provider": mock_module}):
            import importlib

            import c4.daemon.code_ops as code_ops_mod
            importlib.reload(code_ops_mod)
            ops = code_ops_mod.CodeOps(code_ops._daemon)
            result = ops.get_symbols_overview("mod.py", depth=1)

        assert result["success"] is True
        assert result["symbols"][0]["name"] == "Foo"


# ---------------------------------------------------------------------------
# 15. find_referencing_symbols (tree-sitter fallback)
# ---------------------------------------------------------------------------


class TestFindReferencingSymbols:
    def test_treesitter_fallback(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """When LSP returns None, tree-sitter fallback should be used."""
        ts_result = {
            "success": True,
            "references": [
                {
                    "file_path": str(tmp_path / "example.py"),
                    "line": 5,
                    "column": 0,
                    "end_line": 5,
                    "end_column": 8,
                    "context": "my_func()",
                    "ref_kind": "call",
                },
            ],
            "total": 1,
            "symbol": "my_func",
        }

        with (
            patch.object(code_ops, "_find_references_via_lsp", return_value=None),
            patch.object(code_ops, "_find_references_via_treesitter", return_value=ts_result),
        ):
            result = code_ops.find_referencing_symbols("my_func", str(tmp_path / "example.py"))

        assert result["success"] is True
        assert result["total"] == 1
        assert result["references"][0]["ref_kind"] == "call"


# ---------------------------------------------------------------------------
# 16-18. File operation delegates
# ---------------------------------------------------------------------------


class TestFileOperations:
    def test_read_file(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """read_file delegates to FileTools.read_file."""
        f = tmp_path / "readme.txt"
        f.write_text("line1\nline2\nline3\n", encoding="utf-8")

        result = code_ops.read_file("readme.txt")
        assert "content" in result
        assert "line1" in result["content"]
        assert result["total_lines"] == 3

    def test_create_text_file(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """create_text_file creates a new file via FileTools."""
        result = code_ops.create_text_file("sub/new.txt", "hello world")
        assert result["success"] is True

        created = tmp_path / "sub" / "new.txt"
        assert created.exists()
        assert created.read_text(encoding="utf-8") == "hello world"

    def test_replace_content(self, code_ops: CodeOps, tmp_path: Path) -> None:
        """replace_content delegates to FileTools.replace_content."""
        f = tmp_path / "data.txt"
        f.write_text("foo bar baz\n", encoding="utf-8")

        result = code_ops.replace_content("data.txt", "bar", "qux")
        assert result["success"] is True
        assert result["replacements_made"] == 1
        assert f.read_text(encoding="utf-8") == "foo qux baz\n"
