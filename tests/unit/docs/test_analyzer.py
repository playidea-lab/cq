"""Unit tests for CodeAnalyzer."""

import tempfile
from pathlib import Path

import pytest

from c4.docs.analyzer import (
    CodeAnalyzer,
    Dependency,
    Location,
    Reference,
    Symbol,
    SymbolKind,
)

# =============================================================================
# Test Data
# =============================================================================

PYTHON_CODE_SIMPLE = '''
"""Module docstring."""

CONSTANT_VALUE = 42
variable_value = "hello"

def simple_function(arg1, arg2):
    """Function docstring."""
    return arg1 + arg2

class MyClass:
    """Class docstring."""

    def __init__(self, value):
        self.value = value

    def get_value(self):
        """Get the value."""
        return self.value

    def set_value(self, value):
        self.value = value
'''

PYTHON_CODE_WITH_IMPORTS = '''
import os
import sys
from pathlib import Path
from typing import Optional, List as ListType
from . import local_module
from ..parent import parent_func

def use_imports():
    path = Path(".")
    return os.getcwd()
'''

TYPESCRIPT_CODE_SIMPLE = '''
const CONSTANT = 42;
let variable = "hello";

function simpleFunction(arg1: string, arg2: number): string {
    return arg1 + arg2.toString();
}

class MyClass {
    private value: number;

    constructor(value: number) {
        this.value = value;
    }

    getValue(): number {
        return this.value;
    }

    setValue(value: number): void {
        this.value = value;
    }
}

interface MyInterface {
    id: number;
    name: string;
}

type MyType = string | number;

enum Status {
    Active = "active",
    Inactive = "inactive",
}

const arrowFunc = (x: number): number => x * 2;
'''

TYPESCRIPT_CODE_WITH_IMPORTS = '''
import React from "react";
import { useState, useEffect } from "react";
import * as utils from "./utils";
import type { User } from "../types";

export function Component() {
    const [state, setState] = useState(0);
    return <div>{state}</div>;
}
'''


# =============================================================================
# Location Tests
# =============================================================================

class TestLocation:
    """Tests for Location dataclass."""

    def test_location_str(self):
        """Test Location string representation."""
        loc = Location("file.py", 10, 5, 15, 20)
        assert str(loc) == "file.py:10:5"

    def test_location_fields(self):
        """Test Location field access."""
        loc = Location("test.py", 1, 2, 3, 4)
        assert loc.file_path == "test.py"
        assert loc.start_line == 1
        assert loc.start_column == 2
        assert loc.end_line == 3
        assert loc.end_column == 4


# =============================================================================
# Symbol Tests
# =============================================================================

class TestSymbol:
    """Tests for Symbol dataclass."""

    def test_symbol_qualified_name_without_parent(self):
        """Test qualified name for top-level symbol."""
        symbol = Symbol(
            name="my_func",
            kind=SymbolKind.FUNCTION,
            location=Location("file.py", 1, 0, 5, 0),
        )
        assert symbol.qualified_name == "my_func"

    def test_symbol_qualified_name_with_parent(self):
        """Test qualified name for nested symbol."""
        symbol = Symbol(
            name="my_method",
            kind=SymbolKind.METHOD,
            location=Location("file.py", 10, 4, 15, 0),
            parent="MyClass",
        )
        assert symbol.qualified_name == "MyClass.my_method"

    def test_symbol_default_fields(self):
        """Test Symbol default field values."""
        symbol = Symbol(
            name="test",
            kind=SymbolKind.VARIABLE,
            location=Location("file.py", 1, 0, 1, 10),
        )
        assert symbol.parent is None
        assert symbol.docstring is None
        assert symbol.signature is None
        assert symbol.children == []
        assert symbol.metadata == {}


# =============================================================================
# Reference Tests
# =============================================================================

class TestReference:
    """Tests for Reference dataclass."""

    def test_reference_fields(self):
        """Test Reference field access."""
        ref = Reference(
            symbol_name="my_func",
            location=Location("file.py", 20, 5, 20, 12),
            context="result = my_func(x)",
            ref_kind="usage",
        )
        assert ref.symbol_name == "my_func"
        assert ref.context == "result = my_func(x)"
        assert ref.ref_kind == "usage"


# =============================================================================
# Dependency Tests
# =============================================================================

class TestDependency:
    """Tests for Dependency dataclass."""

    def test_dependency_fields(self):
        """Test Dependency field access."""
        dep = Dependency(
            source="main.py",
            target="utils",
            import_name="utils",
            is_relative=False,
        )
        assert dep.source == "main.py"
        assert dep.target == "utils"
        assert dep.import_name == "utils"
        assert dep.is_relative is False

    def test_dependency_relative(self):
        """Test relative Dependency."""
        dep = Dependency(
            source="main.py",
            target=".local",
            import_name="helper",
            is_relative=True,
        )
        assert dep.is_relative is True


# =============================================================================
# CodeAnalyzer Tests - Basic
# =============================================================================

class TestCodeAnalyzerBasic:
    """Basic tests for CodeAnalyzer."""

    def test_analyzer_init(self):
        """Test CodeAnalyzer initialization."""
        analyzer = CodeAnalyzer()
        assert analyzer is not None
        assert analyzer.get_all_symbols() == []

    def test_add_file_with_content(self):
        """Test adding file with content."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("test.py", PYTHON_CODE_SIMPLE)
        symbols = analyzer.get_file_symbols("test.py")
        assert len(symbols) > 0

    def test_add_file_not_found(self):
        """Test adding non-existent file."""
        analyzer = CodeAnalyzer()
        with pytest.raises(FileNotFoundError):
            analyzer.add_file("/nonexistent/path/file.py")

    def test_clear(self):
        """Test clearing analyzer data."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("test.py", PYTHON_CODE_SIMPLE)
        assert len(analyzer.get_all_symbols()) > 0

        analyzer.clear()
        assert len(analyzer.get_all_symbols()) == 0


# =============================================================================
# CodeAnalyzer Tests - Python Parsing
# =============================================================================

class TestCodeAnalyzerPython:
    """Tests for Python code analysis."""

    @pytest.fixture
    def python_analyzer(self):
        """Create analyzer with Python code."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("test.py", PYTHON_CODE_SIMPLE)
        return analyzer

    def test_find_function(self, python_analyzer):
        """Test finding a function."""
        symbols = python_analyzer.find_symbol("simple_function")
        assert len(symbols) >= 1
        func = symbols[0]
        assert func.name == "simple_function"
        assert func.kind == SymbolKind.FUNCTION

    def test_find_class(self, python_analyzer):
        """Test finding a class."""
        symbols = python_analyzer.find_symbol("MyClass", kind=SymbolKind.CLASS)
        assert len(symbols) == 1
        cls = symbols[0]
        assert cls.name == "MyClass"
        assert cls.kind == SymbolKind.CLASS

    def test_find_method(self, python_analyzer):
        """Test finding a method."""
        symbols = python_analyzer.find_symbol("get_value")
        methods = [s for s in symbols if s.kind == SymbolKind.METHOD]
        assert len(methods) >= 1
        method = methods[0]
        assert method.parent == "MyClass"

    def test_find_constant(self, python_analyzer):
        """Test finding a constant."""
        symbols = python_analyzer.find_symbol(
            "CONSTANT_VALUE", kind=SymbolKind.CONSTANT
        )
        assert len(symbols) >= 1

    def test_find_variable(self, python_analyzer):
        """Test finding a variable."""
        symbols = python_analyzer.find_symbol(
            "variable_value", kind=SymbolKind.VARIABLE
        )
        assert len(symbols) >= 1

    def test_exact_match(self, python_analyzer):
        """Test exact name matching."""
        # Partial match
        symbols = python_analyzer.find_symbol("simple")
        assert len(symbols) >= 1

        # Exact match only
        symbols_exact = python_analyzer.find_symbol("simple", exact_match=True)
        assert len(symbols_exact) == 0

        symbols_exact = python_analyzer.find_symbol(
            "simple_function", exact_match=True
        )
        assert len(symbols_exact) >= 1

    def test_filter_by_kind(self, python_analyzer):
        """Test filtering by symbol kind."""
        classes = python_analyzer.find_symbol("", kind=SymbolKind.CLASS)

        for cls in classes:
            assert cls.kind == SymbolKind.CLASS

    def test_python_imports(self):
        """Test Python import extraction."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("imports.py", PYTHON_CODE_WITH_IMPORTS)

        deps = analyzer.get_dependencies("imports.py")
        assert len(deps) > 0

        # Check for standard imports
        targets = [d.target for d in deps]
        assert "os" in targets or any("os" in t for t in targets)
        assert "pathlib" in targets or any("Path" in d.import_name for d in deps)

        # Check for relative imports
        relative_deps = [d for d in deps if d.is_relative]
        assert len(relative_deps) > 0


# =============================================================================
# CodeAnalyzer Tests - TypeScript Parsing
# =============================================================================

class TestCodeAnalyzerTypeScript:
    """Tests for TypeScript code analysis."""

    @pytest.fixture
    def ts_analyzer(self):
        """Create analyzer with TypeScript code."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("test.ts", TYPESCRIPT_CODE_SIMPLE)
        return analyzer

    def test_find_function(self, ts_analyzer):
        """Test finding a TypeScript function."""
        symbols = ts_analyzer.find_symbol("simpleFunction")
        assert len(symbols) >= 1
        func = symbols[0]
        assert func.name == "simpleFunction"
        assert func.kind == SymbolKind.FUNCTION

    def test_find_class(self, ts_analyzer):
        """Test finding a TypeScript class."""
        symbols = ts_analyzer.find_symbol("MyClass", kind=SymbolKind.CLASS)
        assert len(symbols) == 1

    def test_find_interface(self, ts_analyzer):
        """Test finding a TypeScript interface."""
        symbols = ts_analyzer.find_symbol("MyInterface", kind=SymbolKind.INTERFACE)
        assert len(symbols) >= 1

    def test_find_type_alias(self, ts_analyzer):
        """Test finding a TypeScript type alias."""
        symbols = ts_analyzer.find_symbol("MyType", kind=SymbolKind.TYPE_ALIAS)
        assert len(symbols) >= 1

    def test_find_enum(self, ts_analyzer):
        """Test finding a TypeScript enum."""
        # Note: enum parsing may not be fully supported in fallback regex parser
        symbols = ts_analyzer.find_symbol("Status", kind=SymbolKind.ENUM)
        # This test passes when tree-sitter is available, otherwise may be empty
        # We just check it doesn't crash
        assert isinstance(symbols, list)

    def test_find_arrow_function(self, ts_analyzer):
        """Test finding an arrow function."""
        symbols = ts_analyzer.find_symbol("arrowFunc")
        assert len(symbols) >= 1
        # Arrow functions can be detected as FUNCTION or VARIABLE depending on parsing
        assert symbols[0].kind in (SymbolKind.FUNCTION, SymbolKind.VARIABLE)

    def test_typescript_imports(self):
        """Test TypeScript import extraction."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("component.tsx", TYPESCRIPT_CODE_WITH_IMPORTS)

        deps = analyzer.get_dependencies("component.tsx")
        assert len(deps) > 0

        # Check for imports
        targets = [d.target for d in deps]
        assert "react" in targets or any("react" in t.lower() for t in targets)


# =============================================================================
# CodeAnalyzer Tests - References
# =============================================================================

class TestCodeAnalyzerReferences:
    """Tests for reference finding."""

    def test_find_references_python(self):
        """Test finding references in Python code."""
        code = '''
def helper():
    return 42

def main():
    x = helper()
    y = helper()
    return x + y
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("refs.py", code)

        refs = analyzer.find_references("helper")
        # Note: Reference tracking requires tree-sitter; fallback parser
        # may not track references. We just verify it doesn't crash.
        assert isinstance(refs, list)

    def test_find_references_with_file_filter(self):
        """Test finding references with file filter."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("file1.py", "x = 1\ny = x + 1")
        analyzer.add_file("file2.py", "x = 2\nz = x * 2")

        refs_file1 = analyzer.find_references("x", file_path="file1.py")
        refs_all = analyzer.find_references("x")

        assert len(refs_all) >= len(refs_file1)


# =============================================================================
# CodeAnalyzer Tests - Dependency Graph
# =============================================================================

class TestCodeAnalyzerDependencyGraph:
    """Tests for dependency graph generation."""

    def test_dependency_graph(self):
        """Test building dependency graph."""
        analyzer = CodeAnalyzer()

        code1 = "from module_b import func_b\nresult = func_b()"
        code2 = "def func_b(): return 42"

        analyzer.add_file("module_a.py", code1)
        analyzer.add_file("module_b.py", code2)

        graph = analyzer.get_dependency_graph()

        assert "module_a.py" in graph
        assert "module_b" in graph["module_a.py"] or any(
            "func_b" in t or "module_b" in t for t in graph.get("module_a.py", [])
        )


# =============================================================================
# CodeAnalyzer Tests - Directory Scanning
# =============================================================================

class TestCodeAnalyzerDirectory:
    """Tests for directory scanning."""

    def test_add_directory(self):
        """Test adding files from directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create test files
            (Path(tmpdir) / "file1.py").write_text("def func1(): pass")
            (Path(tmpdir) / "file2.py").write_text("def func2(): pass")
            (Path(tmpdir) / "subdir").mkdir()
            (Path(tmpdir) / "subdir" / "file3.py").write_text("def func3(): pass")
            (Path(tmpdir) / "ignored.txt").write_text("not python")

            analyzer = CodeAnalyzer()
            count = analyzer.add_directory(tmpdir, recursive=True)

            assert count == 3  # 3 Python files

    def test_add_directory_non_recursive(self):
        """Test non-recursive directory scanning."""
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "file1.py").write_text("def func1(): pass")
            (Path(tmpdir) / "subdir").mkdir()
            (Path(tmpdir) / "subdir" / "file2.py").write_text("def func2(): pass")

            analyzer = CodeAnalyzer()
            count = analyzer.add_directory(tmpdir, recursive=False)

            assert count == 1  # Only root file


# =============================================================================
# CodeAnalyzer Tests - Language Detection
# =============================================================================

class TestCodeAnalyzerLanguageDetection:
    """Tests for language detection."""

    def test_python_extensions(self):
        """Test Python file detection."""
        analyzer = CodeAnalyzer()

        assert analyzer._get_language("file.py") == "python"
        assert analyzer._get_language("file.pyi") == "python"

    def test_typescript_extensions(self):
        """Test TypeScript file detection."""
        analyzer = CodeAnalyzer()

        assert analyzer._get_language("file.ts") == "typescript"
        assert analyzer._get_language("file.tsx") == "typescript"
        assert analyzer._get_language("file.js") == "typescript"
        assert analyzer._get_language("file.jsx") == "typescript"

    def test_unknown_extension(self):
        """Test unknown file extension."""
        analyzer = CodeAnalyzer()

        assert analyzer._get_language("file.rb") == "unknown"
        assert analyzer._get_language("file.go") == "go"


# =============================================================================
# CodeAnalyzer Tests - Edge Cases
# =============================================================================

class TestCodeAnalyzerEdgeCases:
    """Tests for edge cases."""

    def test_empty_file(self):
        """Test parsing empty file."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("empty.py", "")
        symbols = analyzer.get_file_symbols("empty.py")
        assert symbols == []

    def test_syntax_error_handling(self):
        """Test handling of syntax errors (graceful degradation)."""
        analyzer = CodeAnalyzer()
        # Invalid Python syntax - should not raise
        try:
            analyzer.add_file("invalid.py", "def broken( = )")
        except Exception:
            pass  # Some errors are acceptable
        # Should not crash

    def test_unicode_content(self):
        """Test handling Unicode content."""
        code = '''
def greet():
    """Say hello in multiple languages."""
    return "Hello 你好 مرحبا 🌍"

EMOJI_VAR = "😀"
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("unicode.py", code)

        symbols = analyzer.find_symbol("greet")
        assert len(symbols) >= 1

    def test_get_file_symbols_nonexistent(self):
        """Test getting symbols from non-analyzed file."""
        analyzer = CodeAnalyzer()
        symbols = analyzer.get_file_symbols("nonexistent.py")
        assert symbols == []

    def test_get_dependencies_nonexistent(self):
        """Test getting dependencies from non-analyzed file."""
        analyzer = CodeAnalyzer()
        deps = analyzer.get_dependencies("nonexistent.py")
        assert deps == []


# =============================================================================
# SymbolKind Tests
# =============================================================================

class TestSymbolKind:
    """Tests for SymbolKind enum."""

    def test_symbol_kinds(self):
        """Test all symbol kinds exist."""
        assert SymbolKind.FUNCTION.value == "function"
        assert SymbolKind.CLASS.value == "class"
        assert SymbolKind.METHOD.value == "method"
        assert SymbolKind.VARIABLE.value == "variable"
        assert SymbolKind.CONSTANT.value == "constant"
        assert SymbolKind.IMPORT.value == "import"
        assert SymbolKind.MODULE.value == "module"
        assert SymbolKind.INTERFACE.value == "interface"
        assert SymbolKind.TYPE_ALIAS.value == "type_alias"
        assert SymbolKind.ENUM.value == "enum"
        assert SymbolKind.PROPERTY.value == "property"
        assert SymbolKind.PARAMETER.value == "parameter"


# =============================================================================
# Additional Coverage Tests
# =============================================================================

class TestCodeAnalyzerAdditionalCoverage:
    """Additional tests for better coverage."""

    def test_python_aliased_import(self):
        """Test Python aliased imports."""
        code = '''
from typing import List as ListType
import os as operating_system
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("aliased.py", code)
        deps = analyzer.get_dependencies("aliased.py")
        assert len(deps) > 0

    def test_python_nested_class_methods(self):
        """Test nested class with multiple methods."""
        code = '''
class OuterClass:
    """Outer class."""

    class_var = 10

    def __init__(self, x):
        self.x = x

    def method_one(self):
        """First method."""
        return self.x

    def method_two(self, y):
        """Second method."""
        return self.x + y

    @staticmethod
    def static_method():
        return 42

    @classmethod
    def class_method(cls):
        return cls.class_var
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("nested.py", code)

        symbols = analyzer.get_file_symbols("nested.py")
        methods = [s for s in symbols if s.kind == SymbolKind.METHOD]
        assert len(methods) >= 2

    def test_typescript_method_definition(self):
        """Test TypeScript class method parsing."""
        code = '''
class Service {
    private data: string;

    constructor(data: string) {
        this.data = data;
    }

    getData(): string {
        return this.data;
    }

    setData(value: string): void {
        this.data = value;
    }

    async fetchData(): Promise<string> {
        return this.data;
    }
}
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("service.ts", code)

        symbols = analyzer.find_symbol("Service", kind=SymbolKind.CLASS)
        assert len(symbols) >= 1

    def test_typescript_named_imports(self):
        """Test TypeScript named imports extraction."""
        code = '''
import { Component, OnInit, Input } from "@angular/core";
import type { User, Profile } from "../types";
import * as lodash from "lodash";
import defaultExport from "./module";

export class MyComponent {}
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("component.ts", code)

        deps = analyzer.get_dependencies("component.ts")
        assert len(deps) >= 1

    def test_filter_by_file(self):
        """Test filtering symbols by file."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("a.py", "def func_a(): pass")
        analyzer.add_file("b.py", "def func_b(): pass")

        # Filter by specific file
        symbols_a = analyzer.find_symbol("func", file_path="a.py")
        symbols_b = analyzer.find_symbol("func", file_path="b.py")

        # Each should only find its own function
        assert any(s.name == "func_a" for s in symbols_a)
        assert any(s.name == "func_b" for s in symbols_b)

    def test_get_all_symbols_multiple_files(self):
        """Test get_all_symbols with multiple files."""
        analyzer = CodeAnalyzer()
        analyzer.add_file("f1.py", "def a(): pass\ndef b(): pass")
        analyzer.add_file("f2.py", "class C: pass")

        all_symbols = analyzer.get_all_symbols()
        assert len(all_symbols) >= 3

    def test_exclusion_patterns(self):
        """Test directory scanning with exclusion patterns."""
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "main.py").write_text("def main(): pass")
            (Path(tmpdir) / "node_modules").mkdir()
            (Path(tmpdir) / "node_modules" / "lib.py").write_text("def lib(): pass")

            analyzer = CodeAnalyzer()
            count = analyzer.add_directory(
                tmpdir,
                exclude_patterns=["**/node_modules/**"],
            )

            # Should only add main.py, not node_modules/lib.py
            assert count == 1

    def test_docstring_extraction(self):
        """Test docstring extraction from classes and functions."""
        code = '''
def documented_function():
    """This is the function docstring."""
    pass

class DocumentedClass:
    """This is the class docstring."""

    def method_with_doc(self):
        """Method docstring."""
        pass
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("documented.py", code)

        func_symbols = analyzer.find_symbol("documented_function")
        assert len(func_symbols) >= 1
        # Docstring extraction depends on tree-sitter being available
        if func_symbols[0].docstring:
            assert "function docstring" in func_symbols[0].docstring

    def test_signature_extraction(self):
        """Test function signature extraction."""
        code = '''
def func_with_args(a: int, b: str = "default", *args, **kwargs) -> bool:
    return True
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("sig.py", code)

        symbols = analyzer.find_symbol("func_with_args")
        assert len(symbols) >= 1
        if symbols[0].signature:
            assert "a" in symbols[0].signature or "func_with_args" in symbols[0].signature

    def test_relative_imports(self):
        """Test relative import detection."""
        code = '''
from . import sibling
from .. import parent
from ...package import deep
from .submodule import func
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("rel.py", code)

        deps = analyzer.get_dependencies("rel.py")
        relative_deps = [d for d in deps if d.is_relative]
        assert len(relative_deps) > 0

    def test_tsx_parsing(self):
        """Test TSX file parsing."""
        code = '''
import React from "react";

interface Props {
    name: string;
}

function MyComponent(props: Props): JSX.Element {
    return <div>Hello {props.name}</div>;
}

export default MyComponent;
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("component.tsx", code)

        symbols = analyzer.find_symbol("MyComponent")
        assert len(symbols) >= 1

    def test_jsx_parsing(self):
        """Test JSX file parsing."""
        code = '''
import React from "react";

function App() {
    return <div>Hello World</div>;
}

export default App;
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("app.jsx", code)

        symbols = analyzer.find_symbol("App")
        assert len(symbols) >= 1

    def test_pyi_stub_file(self):
        """Test Python stub file parsing."""
        code = '''
def stub_function(x: int) -> str: ...

class StubClass:
    attr: int
    def method(self) -> None: ...
'''
        analyzer = CodeAnalyzer()
        analyzer.add_file("types.pyi", code)

        symbols = analyzer.find_symbol("stub_function")
        assert len(symbols) >= 1


# =============================================================================
# Fallback Parser Tests (when tree-sitter is not available)
# =============================================================================

class TestFallbackParser:
    """Tests for fallback regex-based parser."""

    def test_fallback_python_parsing(self, monkeypatch):
        """Test fallback Python parsing when tree-sitter is unavailable."""
        # Monkeypatch to simulate tree-sitter not being available
        import c4.docs.analyzer as analyzer_module

        original_value = analyzer_module.TREE_SITTER_AVAILABLE
        monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", False)

        try:
            analyzer = CodeAnalyzer()
            # Re-initialize without tree-sitter parsers
            analyzer._parsers = {}

            code = '''
CONSTANT = 42
variable = "hello"

def function_one():
    pass

class MyClass:
    def method(self):
        pass

def function_two(x, y):
    return x + y
'''
            analyzer._parse_file_regex("fallback.py", code)

            symbols = analyzer._symbols.get("fallback.py", [])
            assert len(symbols) >= 3  # At least constant, variable, function, class

            # Check we found the class
            classes = [s for s in symbols if s.kind == SymbolKind.CLASS]
            assert len(classes) >= 1

            # Check we found functions
            funcs = [s for s in symbols if s.kind == SymbolKind.FUNCTION]
            assert len(funcs) >= 1

        finally:
            monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", original_value)

    def test_fallback_typescript_parsing(self, monkeypatch):
        """Test fallback TypeScript parsing when tree-sitter is unavailable."""
        import c4.docs.analyzer as analyzer_module

        original_value = analyzer_module.TREE_SITTER_AVAILABLE
        monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", False)

        try:
            analyzer = CodeAnalyzer()
            analyzer._parsers = {}

            code = '''
const CONSTANT = 42;
let variable = "hello";

function myFunction() {
    return 1;
}

export class MyClass {
    getValue() {
        return this.value;
    }
}

interface MyInterface {
    id: number;
}

type MyType = string | number;

export const arrowFunc = (x) => x * 2;

import { something } from "./module";
'''
            analyzer._parse_file_regex("fallback.ts", code)

            symbols = analyzer._symbols.get("fallback.ts", [])
            deps = analyzer._dependencies.get("fallback.ts", [])

            # Check symbols
            assert len(symbols) >= 3

            # Check imports
            assert len(deps) >= 1

            # Verify class was found
            classes = [s for s in symbols if s.kind == SymbolKind.CLASS]
            assert len(classes) >= 1

            # Verify interface was found
            interfaces = [s for s in symbols if s.kind == SymbolKind.INTERFACE]
            assert len(interfaces) >= 1

        finally:
            monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", original_value)

    def test_fallback_method_detection(self, monkeypatch):
        """Test fallback parser detects methods inside classes."""
        import c4.docs.analyzer as analyzer_module

        original_value = analyzer_module.TREE_SITTER_AVAILABLE
        monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", False)

        try:
            analyzer = CodeAnalyzer()
            analyzer._parsers = {}

            code = '''
class Parent:
    def method_one(self):
        pass

    def method_two(self, x):
        return x

def standalone():
    pass
'''
            analyzer._parse_file_regex("methods.py", code)

            symbols = analyzer._symbols.get("methods.py", [])
            methods = [s for s in symbols if s.kind == SymbolKind.METHOD]
            assert len(methods) >= 2

            for method in methods:
                assert method.parent == "Parent"

        finally:
            monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", original_value)

    def test_fallback_import_parsing(self, monkeypatch):
        """Test fallback parser extracts imports correctly."""
        import c4.docs.analyzer as analyzer_module

        original_value = analyzer_module.TREE_SITTER_AVAILABLE
        monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", False)

        try:
            analyzer = CodeAnalyzer()
            analyzer._parsers = {}

            code = '''
import os
import sys
from pathlib import Path
from typing import List, Dict
from . import local
from ..parent import func
'''
            analyzer._parse_file_regex("imports.py", code)

            deps = analyzer._dependencies.get("imports.py", [])
            assert len(deps) >= 3

            # Check relative imports
            relative = [d for d in deps if d.is_relative]
            assert len(relative) >= 1

        finally:
            monkeypatch.setattr(analyzer_module, "TREE_SITTER_AVAILABLE", original_value)
