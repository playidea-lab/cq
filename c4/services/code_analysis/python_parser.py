"""Python AST parser for code symbol extraction."""

from __future__ import annotations

import ast
from pathlib import Path

from .models import (
    SymbolInfo,
    SymbolKind,
    SymbolLocation,
    SymbolTable,
)


class PythonParser:
    """Parse Python source code and extract symbols."""

    def __init__(self) -> None:
        """Initialize the parser."""
        self._current_file: str = ""
        self._current_parent: str | None = None

    def parse_file(self, file_path: str | Path) -> SymbolTable:
        """Parse a Python file and return its symbol table.

        Args:
            file_path: Path to the Python file.

        Returns:
            SymbolTable with all symbols found in the file.
        """
        file_path = Path(file_path)
        self._current_file = str(file_path)

        table = SymbolTable(
            file_path=str(file_path),
            language="python",
        )

        try:
            source = file_path.read_text(encoding="utf-8")
            tree = ast.parse(source, filename=str(file_path))
            self._extract_symbols(tree, table)
            self._extract_imports(tree, table)
        except SyntaxError as e:
            table.errors.append(f"Syntax error: {e}")
        except Exception as e:
            table.errors.append(f"Parse error: {e}")

        return table

    def parse_source(self, source: str, filename: str = "<string>") -> SymbolTable:
        """Parse Python source code string.

        Args:
            source: Python source code.
            filename: Virtual filename for error messages.

        Returns:
            SymbolTable with all symbols found.
        """
        self._current_file = filename

        table = SymbolTable(
            file_path=filename,
            language="python",
        )

        try:
            tree = ast.parse(source, filename=filename)
            self._extract_symbols(tree, table)
            self._extract_imports(tree, table)
        except SyntaxError as e:
            table.errors.append(f"Syntax error: {e}")
        except Exception as e:
            table.errors.append(f"Parse error: {e}")

        return table

    def _extract_symbols(
        self,
        tree: ast.AST,
        table: SymbolTable,
        parent: str | None = None,
    ) -> None:
        """Recursively extract symbols from AST."""
        for node in ast.iter_child_nodes(tree):
            if isinstance(node, ast.ClassDef):
                self._process_class(node, table, parent)
            elif isinstance(node, ast.FunctionDef | ast.AsyncFunctionDef):
                self._process_function(node, table, parent)
            elif isinstance(node, ast.Assign):
                self._process_assignment(node, table, parent)
            elif isinstance(node, ast.AnnAssign):
                self._process_annotated_assignment(node, table, parent)

    def _process_class(
        self,
        node: ast.ClassDef,
        table: SymbolTable,
        parent: str | None = None,
    ) -> None:
        """Process a class definition."""
        name_path = f"{parent}/{node.name}" if parent else node.name

        # Extract decorators
        decorators = [self._get_decorator_name(d) for d in node.decorator_list]

        # Extract docstring
        doc = ast.get_docstring(node)

        # Build signature (base classes)
        bases = [self._node_to_source(b) for b in node.bases]
        signature = f"class {node.name}({', '.join(bases)})" if bases else f"class {node.name}"

        symbol = SymbolInfo(
            name=node.name,
            kind=SymbolKind.CLASS,
            location=SymbolLocation(
                file_path=self._current_file,
                start_line=node.lineno,
                start_col=node.col_offset,
                end_line=node.end_lineno,
                end_col=node.end_col_offset,
            ),
            doc=doc,
            signature=signature,
            parent=parent,
            decorators=decorators,
            is_exported=not node.name.startswith("_"),
        )

        table.add_symbol(symbol)

        # Process class body
        child_names = []
        for child in node.body:
            if isinstance(child, ast.FunctionDef | ast.AsyncFunctionDef):
                self._process_function(child, table, name_path)
                child_names.append(child.name)
            elif isinstance(child, ast.Assign):
                for target in child.targets:
                    if isinstance(target, ast.Name):
                        child_names.append(target.id)
                self._process_assignment(child, table, name_path)
            elif isinstance(child, ast.AnnAssign):
                if isinstance(child.target, ast.Name):
                    child_names.append(child.target.id)
                self._process_annotated_assignment(child, table, name_path)
            elif isinstance(child, ast.ClassDef):
                self._process_class(child, table, name_path)
                child_names.append(child.name)

        symbol.children = child_names

    def _process_function(
        self,
        node: ast.FunctionDef | ast.AsyncFunctionDef,
        table: SymbolTable,
        parent: str | None = None,
    ) -> None:
        """Process a function or method definition."""
        is_method = parent is not None and "/" not in (parent or "")

        # Determine kind
        if is_method:
            if node.name.startswith("_") and not node.name.startswith("__"):
                kind = SymbolKind.METHOD
            elif any(
                self._get_decorator_name(d) == "property" for d in node.decorator_list
            ):
                kind = SymbolKind.PROPERTY
            else:
                kind = SymbolKind.METHOD
        else:
            kind = SymbolKind.FUNCTION

        # Extract decorators
        decorators = [self._get_decorator_name(d) for d in node.decorator_list]

        # Extract docstring
        doc = ast.get_docstring(node)

        # Build signature
        signature = self._build_function_signature(node)

        # Extract return type hint
        return_type = self._node_to_source(node.returns) if node.returns else None

        symbol = SymbolInfo(
            name=node.name,
            kind=kind,
            location=SymbolLocation(
                file_path=self._current_file,
                start_line=node.lineno,
                start_col=node.col_offset,
                end_line=node.end_lineno,
                end_col=node.end_col_offset,
            ),
            doc=doc,
            signature=signature,
            parent=parent,
            decorators=decorators,
            type_hint=return_type,
            is_async=isinstance(node, ast.AsyncFunctionDef),
            is_exported=not node.name.startswith("_"),
        )

        table.add_symbol(symbol)

    def _process_assignment(
        self,
        node: ast.Assign,
        table: SymbolTable,
        parent: str | None = None,
    ) -> None:
        """Process a variable assignment."""
        for target in node.targets:
            if isinstance(target, ast.Name):
                name = target.id

                # Determine if constant (ALL_CAPS)
                is_constant = name.isupper() or (
                    "_" in name and all(p.isupper() for p in name.split("_") if p)
                )

                symbol = SymbolInfo(
                    name=name,
                    kind=SymbolKind.CONSTANT if is_constant else SymbolKind.VARIABLE,
                    location=SymbolLocation(
                        file_path=self._current_file,
                        start_line=node.lineno,
                        start_col=node.col_offset,
                        end_line=node.end_lineno,
                        end_col=node.end_col_offset,
                    ),
                    parent=parent,
                    is_exported=not name.startswith("_"),
                )

                table.add_symbol(symbol)

    def _process_annotated_assignment(
        self,
        node: ast.AnnAssign,
        table: SymbolTable,
        parent: str | None = None,
    ) -> None:
        """Process an annotated assignment (type hint)."""
        if isinstance(node.target, ast.Name):
            name = node.target.id

            # Determine if constant
            is_constant = name.isupper() or (
                "_" in name and all(p.isupper() for p in name.split("_") if p)
            )

            type_hint = self._node_to_source(node.annotation)

            symbol = SymbolInfo(
                name=name,
                kind=SymbolKind.CONSTANT if is_constant else SymbolKind.VARIABLE,
                location=SymbolLocation(
                    file_path=self._current_file,
                    start_line=node.lineno,
                    start_col=node.col_offset,
                    end_line=node.end_lineno,
                    end_col=node.end_col_offset,
                ),
                parent=parent,
                type_hint=type_hint,
                is_exported=not name.startswith("_"),
            )

            table.add_symbol(symbol)

    def _extract_imports(self, tree: ast.AST, table: SymbolTable) -> None:
        """Extract import statements."""
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    table.imports.append(alias.name)
            elif isinstance(node, ast.ImportFrom):
                module = node.module or ""
                for alias in node.names:
                    if alias.name == "*":
                        table.imports.append(f"{module}.*")
                    else:
                        table.imports.append(f"{module}.{alias.name}")

    def _build_function_signature(
        self,
        node: ast.FunctionDef | ast.AsyncFunctionDef,
    ) -> str:
        """Build function signature string."""
        args = []

        # Positional only args (Python 3.8+)
        for arg in node.args.posonlyargs:
            args.append(self._format_arg(arg))
        if node.args.posonlyargs:
            args.append("/")

        # Regular args
        num_defaults = len(node.args.defaults)
        num_args = len(node.args.args)
        for i, arg in enumerate(node.args.args):
            default_idx = i - (num_args - num_defaults)
            if default_idx >= 0:
                default = self._node_to_source(node.args.defaults[default_idx])
                args.append(f"{self._format_arg(arg)}={default}")
            else:
                args.append(self._format_arg(arg))

        # *args
        if node.args.vararg:
            args.append(f"*{self._format_arg(node.args.vararg)}")
        elif node.args.kwonlyargs:
            args.append("*")

        # Keyword only args
        for i, arg in enumerate(node.args.kwonlyargs):
            default = node.args.kw_defaults[i]
            if default:
                args.append(f"{self._format_arg(arg)}={self._node_to_source(default)}")
            else:
                args.append(self._format_arg(arg))

        # **kwargs
        if node.args.kwarg:
            args.append(f"**{self._format_arg(node.args.kwarg)}")

        prefix = "async def" if isinstance(node, ast.AsyncFunctionDef) else "def"
        return_hint = f" -> {self._node_to_source(node.returns)}" if node.returns else ""

        return f"{prefix} {node.name}({', '.join(args)}){return_hint}"

    def _format_arg(self, arg: ast.arg) -> str:
        """Format a function argument."""
        if arg.annotation:
            return f"{arg.arg}: {self._node_to_source(arg.annotation)}"
        return arg.arg

    def _get_decorator_name(self, node: ast.expr) -> str:
        """Get decorator name as string."""
        if isinstance(node, ast.Name):
            return node.id
        elif isinstance(node, ast.Attribute):
            return f"{self._get_decorator_name(node.value)}.{node.attr}"
        elif isinstance(node, ast.Call):
            return self._get_decorator_name(node.func)
        return self._node_to_source(node)

    def _node_to_source(self, node: ast.AST | None) -> str:
        """Convert AST node to source code string."""
        if node is None:
            return ""
        try:
            return ast.unparse(node)
        except Exception:
            return "<unknown>"
