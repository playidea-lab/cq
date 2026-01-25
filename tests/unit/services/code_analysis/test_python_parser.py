"""Tests for Python AST parser."""

import pytest

from c4.services.code_analysis import PythonParser, SymbolKind


class TestPythonParser:
    """Test PythonParser class."""

    @pytest.fixture
    def parser(self) -> PythonParser:
        """Create a parser instance."""
        return PythonParser()

    def test_parse_simple_function(self, parser: PythonParser):
        """Test parsing a simple function."""
        source = '''
def hello(name: str) -> str:
    """Say hello."""
    return f"Hello, {name}!"
'''
        table = parser.parse_source(source)

        assert len(table.symbols) == 1
        symbol = table.symbols["hello"]
        assert symbol.name == "hello"
        assert symbol.kind == SymbolKind.FUNCTION
        assert symbol.doc == "Say hello."
        assert "name: str" in symbol.signature
        assert "-> str" in symbol.signature

    def test_parse_async_function(self, parser: PythonParser):
        """Test parsing an async function."""
        source = '''
async def fetch_data(url: str) -> dict:
    """Fetch data from URL."""
    pass
'''
        table = parser.parse_source(source)

        symbol = table.symbols["fetch_data"]
        assert symbol.is_async is True
        assert "async def" in symbol.signature

    def test_parse_class_with_methods(self, parser: PythonParser):
        """Test parsing a class with methods."""
        source = '''
class Calculator:
    """A simple calculator."""

    def __init__(self, value: int = 0):
        self.value = value

    def add(self, x: int) -> int:
        """Add x to value."""
        self.value += x
        return self.value

    @property
    def current(self) -> int:
        """Get current value."""
        return self.value
'''
        table = parser.parse_source(source)

        # Check class
        assert "Calculator" in table.symbols
        calc = table.symbols["Calculator"]
        assert calc.kind == SymbolKind.CLASS
        assert calc.doc == "A simple calculator."
        assert "class Calculator" in calc.signature

        # Check methods
        assert "Calculator/__init__" in table.symbols
        assert "Calculator/add" in table.symbols
        add_method = table.symbols["Calculator/add"]
        assert add_method.kind == SymbolKind.METHOD
        assert add_method.parent == "Calculator"

        # Check property
        assert "Calculator/current" in table.symbols
        current = table.symbols["Calculator/current"]
        assert current.kind == SymbolKind.PROPERTY

    def test_parse_decorated_function(self, parser: PythonParser):
        """Test parsing a decorated function."""
        source = '''
@app.route("/api/users")
@login_required
def get_users():
    pass
'''
        table = parser.parse_source(source)

        symbol = table.symbols["get_users"]
        assert "app.route" in symbol.decorators
        assert "login_required" in symbol.decorators

    def test_parse_constants_and_variables(self, parser: PythonParser):
        """Test parsing constants and variables."""
        source = '''
MAX_SIZE = 100
DEFAULT_NAME = "test"
_private_var = "hidden"
regular_var: str = "hello"
'''
        table = parser.parse_source(source)

        assert "MAX_SIZE" in table.symbols
        assert table.symbols["MAX_SIZE"].kind == SymbolKind.CONSTANT

        assert "DEFAULT_NAME" in table.symbols
        assert table.symbols["DEFAULT_NAME"].kind == SymbolKind.CONSTANT

        assert "_private_var" in table.symbols
        assert table.symbols["_private_var"].is_exported is False

        assert "regular_var" in table.symbols
        assert table.symbols["regular_var"].type_hint == "str"

    def test_parse_imports(self, parser: PythonParser):
        """Test parsing import statements."""
        source = '''
import os
import sys
from pathlib import Path
from typing import Optional, List
from .models import User
'''
        table = parser.parse_source(source)

        assert "os" in table.imports
        assert "sys" in table.imports
        assert "pathlib.Path" in table.imports
        assert "typing.Optional" in table.imports
        assert "typing.List" in table.imports
        assert "models.User" in table.imports  # Relative imports stored without leading dot

    def test_parse_nested_class(self, parser: PythonParser):
        """Test parsing nested classes."""
        source = '''
class Outer:
    """Outer class."""

    class Inner:
        """Inner class."""

        def method(self):
            pass
'''
        table = parser.parse_source(source)

        assert "Outer" in table.symbols
        assert "Outer/Inner" in table.symbols
        assert "Outer/Inner/method" in table.symbols

        inner = table.symbols["Outer/Inner"]
        assert inner.parent == "Outer"

    def test_parse_syntax_error(self, parser: PythonParser):
        """Test handling of syntax errors."""
        source = "def broken("  # Missing closing paren

        table = parser.parse_source(source)

        assert len(table.errors) > 0
        assert "Syntax error" in table.errors[0]

    def test_parse_name_path(self, parser: PythonParser):
        """Test name_path property."""
        source = '''
class MyClass:
    def my_method(self):
        pass
'''
        table = parser.parse_source(source)

        method = table.symbols["MyClass/my_method"]
        assert method.name_path == "MyClass/my_method"

    def test_parse_class_attributes(self, parser: PythonParser):
        """Test parsing class attributes."""
        source = '''
class Config:
    DEBUG: bool = True
    HOST: str = "localhost"
    PORT: int = 8000
'''
        table = parser.parse_source(source)

        assert "Config/DEBUG" in table.symbols
        debug = table.symbols["Config/DEBUG"]
        assert debug.type_hint == "bool"
        assert debug.parent == "Config"

    def test_find_by_kind(self, parser: PythonParser):
        """Test finding symbols by kind."""
        source = '''
class MyClass:
    pass

def my_function():
    pass

MY_CONSTANT = 42
'''
        table = parser.parse_source(source)

        classes = table.find_by_kind(SymbolKind.CLASS)
        assert len(classes) == 1
        assert classes[0].name == "MyClass"

        functions = table.find_by_kind(SymbolKind.FUNCTION)
        assert len(functions) == 1
        assert functions[0].name == "my_function"

        constants = table.find_by_kind(SymbolKind.CONSTANT)
        assert len(constants) == 1
        assert constants[0].name == "MY_CONSTANT"

    def test_find_by_name(self, parser: PythonParser):
        """Test finding symbols by name."""
        source = '''
class UserService:
    def get_user(self):
        pass

    def get_users(self):
        pass

def get_user_by_id():
    pass
'''
        table = parser.parse_source(source)

        # Exact match
        exact = table.find_by_name("get_user")
        assert len(exact) == 1

        # Substring match
        substring = table.find_by_name("get_user", substring=True)
        assert len(substring) == 3  # get_user, get_users, get_user_by_id
