"""Tests for C4 LSP symbol editing MCP tools."""

from __future__ import annotations


class TestReplaceSymbolBody:
    """Tests for c4_replace_symbol_body MCP tool."""

    def test_replace_function_body(self, tmp_path):
        """Should replace a function body."""
        from c4.mcp_server import C4Daemon

        # Create a test file
        test_file = tmp_path / "test_module.py"
        test_file.write_text("""
def my_function():
    return 1

def other_function():
    return 2
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_replace_symbol_body(
            name_path="my_function",
            file_path=str(test_file),
            new_body="def my_function():\n    return 42\n",
        )

        assert result["success"] is True
        assert result["symbol"] == "my_function"

        # Verify the file was changed
        content = test_file.read_text()
        assert "return 42" in content
        assert "return 2" in content  # Other function unchanged

    def test_replace_method_body(self, tmp_path):
        """Should replace a class method body."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_class.py"
        test_file.write_text("""
class MyClass:
    def my_method(self):
        return "old"

    def other_method(self):
        return "keep"
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_replace_symbol_body(
            name_path="MyClass.my_method",
            file_path=str(test_file),
            new_body="    def my_method(self):\n        return \"new\"\n",
        )

        assert result["success"] is True

        content = test_file.read_text()
        assert '"new"' in content
        assert '"keep"' in content

    def test_replace_symbol_not_found(self, tmp_path):
        """Should return error when symbol not found."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "empty.py"
        test_file.write_text("x = 1\n")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_replace_symbol_body(
            name_path="NonExistent",
            file_path=str(test_file),
            new_body="def foo(): pass",
        )

        assert result["success"] is False
        assert "not found" in result["error"].lower()

    def test_replace_file_not_found(self, tmp_path):
        """Should return error when file not found."""
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_replace_symbol_body(
            name_path="some_func",
            file_path="nonexistent.py",
            new_body="def foo(): pass",
        )

        assert result["success"] is False
        assert "not found" in result["error"].lower()


class TestInsertBeforeSymbol:
    """Tests for c4_insert_before_symbol MCP tool."""

    def test_insert_before_function(self, tmp_path):
        """Should insert content before a function."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_module.py"
        test_file.write_text("""
def target_function():
    pass
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_insert_before_symbol(
            name_path="target_function",
            file_path=str(test_file),
            content="# This is a comment\n",
        )

        assert result["success"] is True
        assert result["lines_inserted"] >= 1

        content = test_file.read_text()
        assert "# This is a comment" in content
        # Comment should appear before the function
        comment_pos = content.find("# This is a comment")
        func_pos = content.find("def target_function")
        assert comment_pos < func_pos

    def test_insert_decorator_before_function(self, tmp_path):
        """Should insert a decorator before a function."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_decorator.py"
        test_file.write_text("""
def my_function():
    return 1
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_insert_before_symbol(
            name_path="my_function",
            file_path=str(test_file),
            content="@decorator\n",
        )

        assert result["success"] is True

        content = test_file.read_text()
        assert "@decorator" in content
        dec_pos = content.find("@decorator")
        func_pos = content.find("def my_function")
        assert dec_pos < func_pos


class TestInsertAfterSymbol:
    """Tests for c4_insert_after_symbol MCP tool."""

    def test_insert_after_function(self, tmp_path):
        """Should insert content after a function."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_module.py"
        test_file.write_text("""
def first_function():
    pass

def second_function():
    pass
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_insert_after_symbol(
            name_path="first_function",
            file_path=str(test_file),
            content="def inserted_function():\n    pass\n",
        )

        assert result["success"] is True

        content = test_file.read_text()
        assert "def inserted_function" in content
        # Inserted function should be between first and second
        first_pos = content.find("def first_function")
        inserted_pos = content.find("def inserted_function")
        second_pos = content.find("def second_function")
        assert first_pos < inserted_pos < second_pos

    def test_insert_after_class(self, tmp_path):
        """Should insert content after a class."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_class.py"
        test_file.write_text("""
class MyClass:
    def method(self):
        pass

# End of file
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_insert_after_symbol(
            name_path="MyClass",
            file_path=str(test_file),
            content="class NewClass:\n    pass\n",
        )

        assert result["success"] is True

        content = test_file.read_text()
        assert "class NewClass" in content


class TestRenameSymbol:
    """Tests for c4_rename_symbol MCP tool."""

    def test_rename_function(self, tmp_path):
        """Should rename a function across the file."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_rename.py"
        test_file.write_text("""
def old_name():
    return 1

result = old_name()
x = old_name
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_rename_symbol(
            name_path="old_name",
            file_path=str(test_file),
            new_name="new_name",
        )

        assert result["success"] is True
        assert result["old_name"] == "old_name"
        assert result["new_name"] == "new_name"
        assert result["total_replacements"] >= 3  # def, call, reference

        content = test_file.read_text()
        assert "old_name" not in content
        assert "new_name" in content

    def test_rename_class(self, tmp_path):
        """Should rename a class and its usages."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_rename_class.py"
        test_file.write_text("""
class OldClass:
    pass

obj = OldClass()
isinstance(obj, OldClass)
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_rename_symbol(
            name_path="OldClass",
            file_path=str(test_file),
            new_name="NewClass",
        )

        assert result["success"] is True

        content = test_file.read_text()
        assert "OldClass" not in content
        assert "NewClass" in content

    def test_rename_invalid_identifier(self, tmp_path):
        """Should reject invalid identifiers."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_invalid.py"
        test_file.write_text("def my_func(): pass\n")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_rename_symbol(
            name_path="my_func",
            file_path=str(test_file),
            new_name="123invalid",
        )

        assert result["success"] is False
        assert "invalid" in result["error"].lower()

    def test_rename_across_multiple_files(self, tmp_path):
        """Should rename symbol across multiple files."""
        from c4.mcp_server import C4Daemon

        # Create multiple files with shared symbol
        (tmp_path / "module_a.py").write_text("""
def shared_func():
    return 1
""")
        (tmp_path / "module_b.py").write_text("""
from module_a import shared_func

result = shared_func()
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_rename_symbol(
            name_path="shared_func",
            file_path=str(tmp_path / "module_a.py"),
            new_name="renamed_func",
        )

        assert result["success"] is True
        assert result["total_files"] >= 1

        # Check both files
        content_a = (tmp_path / "module_a.py").read_text()
        content_b = (tmp_path / "module_b.py").read_text()
        assert "renamed_func" in content_a
        assert "renamed_func" in content_b


class TestSymbolEditingEdgeCases:
    """Edge case tests for symbol editing tools."""

    def test_get_symbol_by_qualified_name(self, tmp_path):
        """Should find symbol by qualified name."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_qualified.py"
        test_file.write_text("""
class Parent:
    def child_method(self):
        pass

def child_method():
    pass
""")

        daemon = C4Daemon()
        daemon.root = tmp_path

        # Should find the class method, not the function
        result = daemon.c4_replace_symbol_body(
            name_path="Parent.child_method",
            file_path=str(test_file),
            new_body="    def child_method(self):\n        return 'modified'\n",
        )

        assert result["success"] is True

        content = test_file.read_text()
        # The standalone function should still exist
        lines = content.split("\n")
        # Count definitions
        def_count = sum(1 for line in lines if "def child_method" in line)
        assert def_count == 2

    def test_preserve_file_encoding(self, tmp_path):
        """Should preserve file encoding with unicode."""
        from c4.mcp_server import C4Daemon

        test_file = tmp_path / "test_unicode.py"
        test_file.write_text("""
def greeting():
    return "Hello, 世界!"
""", encoding="utf-8")

        daemon = C4Daemon()
        daemon.root = tmp_path

        result = daemon.c4_replace_symbol_body(
            name_path="greeting",
            file_path=str(test_file),
            new_body='def greeting():\n    return "你好, World!"\n',
        )

        assert result["success"] is True

        content = test_file.read_text(encoding="utf-8")
        assert "你好" in content
