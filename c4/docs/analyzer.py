"""Code Symbol Analyzer using tree-sitter.

Provides AST-based code analysis for Python and TypeScript:
- Symbol extraction (functions, classes, methods, variables)
- Reference finding
- Dependency graph generation
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any

# Try to import tree-sitter, but provide fallback
try:
    import tree_sitter_python as tspython
    import tree_sitter_typescript as tstypescript
    from tree_sitter import Language, Node, Parser

    TREE_SITTER_AVAILABLE = True
except ImportError:
    TREE_SITTER_AVAILABLE = False
    Language = None
    Parser = None
    Node = None


class SymbolKind(Enum):
    """Kind of code symbol."""

    FUNCTION = "function"
    CLASS = "class"
    METHOD = "method"
    VARIABLE = "variable"
    CONSTANT = "constant"
    IMPORT = "import"
    MODULE = "module"
    INTERFACE = "interface"
    TYPE_ALIAS = "type_alias"
    ENUM = "enum"
    PROPERTY = "property"
    PARAMETER = "parameter"


@dataclass
class Location:
    """Source code location."""

    file_path: str
    start_line: int
    start_column: int
    end_line: int
    end_column: int

    def __str__(self) -> str:
        return f"{self.file_path}:{self.start_line}:{self.start_column}"


@dataclass
class Symbol:
    """A code symbol (function, class, method, etc.)."""

    name: str
    kind: SymbolKind
    location: Location
    parent: str | None = None
    docstring: str | None = None
    signature: str | None = None
    children: list[Symbol] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def qualified_name(self) -> str:
        """Get fully qualified name (parent.name)."""
        if self.parent:
            return f"{self.parent}.{self.name}"
        return self.name


@dataclass
class Reference:
    """A reference to a symbol."""

    symbol_name: str
    location: Location
    context: str  # Line of code containing the reference
    ref_kind: str = "usage"  # "usage", "import", "definition"


@dataclass
class Dependency:
    """A dependency relationship between files/modules."""

    source: str  # File that imports
    target: str  # File/module being imported
    import_name: str  # What is imported
    is_relative: bool = False


class CodeAnalyzer:
    """Tree-sitter based code analyzer.

    Provides:
    - find_symbol(): Search for symbols by name
    - find_references(): Find all references to a symbol
    - get_dependencies(): Build dependency graph
    - get_file_symbols(): List all symbols in a file

    Example:
        analyzer = CodeAnalyzer()
        analyzer.add_file("src/main.py")

        # Find all classes named "User"
        symbols = analyzer.find_symbol("User", kind=SymbolKind.CLASS)

        # Find all references to "User"
        refs = analyzer.find_references("User")

        # Get import dependencies
        deps = analyzer.get_dependencies("src/main.py")
    """

    # Language file extensions
    PYTHON_EXTENSIONS = {".py", ".pyi"}
    TYPESCRIPT_EXTENSIONS = {".ts", ".tsx", ".js", ".jsx"}

    def __init__(self) -> None:
        """Initialize the code analyzer."""
        self._symbols: dict[str, list[Symbol]] = {}  # file_path -> symbols
        self._references: dict[str, list[Reference]] = {}  # file_path -> references
        self._dependencies: dict[str, list[Dependency]] = {}  # file_path -> deps
        self._file_contents: dict[str, str] = {}  # file_path -> content
        self._parsers: dict[str, Parser] = {}

        if TREE_SITTER_AVAILABLE:
            self._init_parsers()

    def _init_parsers(self) -> None:
        """Initialize tree-sitter parsers."""
        # Python parser
        py_parser = Parser(Language(tspython.language()))
        self._parsers["python"] = py_parser

        # TypeScript parser
        ts_parser = Parser(Language(tstypescript.language_typescript()))
        self._parsers["typescript"] = ts_parser

        # TSX parser
        tsx_parser = Parser(Language(tstypescript.language_tsx()))
        self._parsers["tsx"] = tsx_parser

    def _get_parser(self, file_path: str) -> Parser | None:
        """Get appropriate parser for file type."""
        ext = Path(file_path).suffix.lower()

        if ext in self.PYTHON_EXTENSIONS:
            return self._parsers.get("python")
        elif ext == ".tsx":
            return self._parsers.get("tsx")
        elif ext in self.TYPESCRIPT_EXTENSIONS:
            return self._parsers.get("typescript")

        return None

    def _get_language(self, file_path: str) -> str:
        """Get language name for file."""
        ext = Path(file_path).suffix.lower()

        if ext in self.PYTHON_EXTENSIONS:
            return "python"
        elif ext in self.TYPESCRIPT_EXTENSIONS:
            return "typescript"

        return "unknown"

    def add_file(self, file_path: str | Path, content: str | None = None) -> None:
        """Add a file to the analyzer.

        Args:
            file_path: Path to the file
            content: Optional file content (reads from disk if not provided)
        """
        file_path = str(file_path)

        if content is None:
            path = Path(file_path)
            if not path.exists():
                raise FileNotFoundError(f"File not found: {file_path}")
            content = path.read_text(encoding="utf-8")

        self._file_contents[file_path] = content
        self._parse_file(file_path, content)

    def add_directory(
        self,
        directory: str | Path,
        recursive: bool = True,
        exclude_patterns: list[str] | None = None,
    ) -> int:
        """Add all supported files from a directory.

        Args:
            directory: Directory path
            recursive: Whether to scan subdirectories
            exclude_patterns: Glob patterns to exclude

        Returns:
            Number of files added
        """
        directory = Path(directory)
        exclude_patterns = exclude_patterns or [
            "**/node_modules/**",
            "**/__pycache__/**",
            "**/.git/**",
            "**/venv/**",
            "**/.venv/**",
        ]

        count = 0
        extensions = self.PYTHON_EXTENSIONS | self.TYPESCRIPT_EXTENSIONS

        pattern = "**/*" if recursive else "*"

        for path in directory.glob(pattern):
            if not path.is_file():
                continue

            if path.suffix.lower() not in extensions:
                continue

            # Check exclusions
            path_str = str(path)
            excluded = False
            for pattern in exclude_patterns:
                if Path(path_str).match(pattern):
                    excluded = True
                    break

            if excluded:
                continue

            try:
                self.add_file(path)
                count += 1
            except Exception:
                # Skip files that can't be parsed
                pass

        return count

    def _parse_file(self, file_path: str, content: str) -> None:
        """Parse a file and extract symbols."""
        if not TREE_SITTER_AVAILABLE:
            # Fallback to regex-based parsing
            self._parse_file_regex(file_path, content)
            return

        parser = self._get_parser(file_path)
        if parser is None:
            return

        language = self._get_language(file_path)
        tree = parser.parse(content.encode("utf-8"))

        symbols = []
        references = []
        dependencies = []

        if language == "python":
            self._extract_python_symbols(tree.root_node, file_path, content, symbols)
            self._extract_python_references(
                tree.root_node, file_path, content, references
            )
            self._extract_python_dependencies(
                tree.root_node, file_path, content, dependencies
            )
        elif language == "typescript":
            self._extract_typescript_symbols(
                tree.root_node, file_path, content, symbols
            )
            self._extract_typescript_references(
                tree.root_node, file_path, content, references
            )
            self._extract_typescript_dependencies(
                tree.root_node, file_path, content, dependencies
            )

        self._symbols[file_path] = symbols
        self._references[file_path] = references
        self._dependencies[file_path] = dependencies

    def _parse_file_regex(self, file_path: str, content: str) -> None:
        """Fallback regex-based parsing when tree-sitter is not available."""
        symbols = []
        references = []
        dependencies = []
        lines = content.split("\n")
        language = self._get_language(file_path)

        if language == "python":
            # Python patterns
            class_pattern = re.compile(r"^class\s+(\w+)")
            func_pattern = re.compile(r"^(\s*)def\s+(\w+)")
            import_pattern = re.compile(r"^(?:from\s+(\S+)\s+)?import\s+(.+)")
            var_pattern = re.compile(r"^(\w+)\s*=")

            current_class = None

            for i, line in enumerate(lines, 1):
                # Class
                match = class_pattern.match(line)
                if match:
                    current_class = match.group(1)
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.CLASS,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Function/Method
                match = func_pattern.match(line)
                if match:
                    indent = len(match.group(1))
                    name = match.group(2)

                    if indent > 0 and current_class:
                        # Method
                        symbols.append(
                            Symbol(
                                name=name,
                                kind=SymbolKind.METHOD,
                                location=Location(file_path, i, indent, i, len(line)),
                                parent=current_class,
                            )
                        )
                    else:
                        # Function
                        current_class = None
                        symbols.append(
                            Symbol(
                                name=name,
                                kind=SymbolKind.FUNCTION,
                                location=Location(file_path, i, 0, i, len(line)),
                            )
                        )
                    continue

                # Import
                match = import_pattern.match(line)
                if match:
                    module = match.group(1) or ""
                    imports = match.group(2)
                    for imp in imports.split(","):
                        imp = imp.strip().split(" as ")[0].strip()
                        if imp:
                            dependencies.append(
                                Dependency(
                                    source=file_path,
                                    target=module or imp,
                                    import_name=imp,
                                    is_relative=module.startswith("."),
                                )
                            )
                    continue

                # Variable at module level
                if not line.startswith(" ") and not line.startswith("\t"):
                    match = var_pattern.match(line)
                    if match:
                        name = match.group(1)
                        if name.isupper():
                            kind = SymbolKind.CONSTANT
                        else:
                            kind = SymbolKind.VARIABLE
                        symbols.append(
                            Symbol(
                                name=name,
                                kind=kind,
                                location=Location(file_path, i, 0, i, len(line)),
                            )
                        )

        elif language == "typescript":
            # TypeScript patterns
            class_pattern = re.compile(r"^\s*(?:export\s+)?class\s+(\w+)")
            func_pattern = re.compile(
                r"^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)"
            )
            arrow_func_pattern = re.compile(
                r"^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\("
            )
            interface_pattern = re.compile(r"^\s*(?:export\s+)?interface\s+(\w+)")
            type_pattern = re.compile(r"^\s*(?:export\s+)?type\s+(\w+)")
            import_pattern = re.compile(r"^\s*import\s+.*\s+from\s+['\"]([^'\"]+)['\"]")

            for i, line in enumerate(lines, 1):
                # Class
                match = class_pattern.match(line)
                if match:
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.CLASS,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Function
                match = func_pattern.match(line)
                if match:
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.FUNCTION,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Arrow function
                match = arrow_func_pattern.match(line)
                if match:
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.FUNCTION,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Interface
                match = interface_pattern.match(line)
                if match:
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.INTERFACE,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Type alias
                match = type_pattern.match(line)
                if match:
                    symbols.append(
                        Symbol(
                            name=match.group(1),
                            kind=SymbolKind.TYPE_ALIAS,
                            location=Location(file_path, i, 0, i, len(line)),
                        )
                    )
                    continue

                # Import
                match = import_pattern.match(line)
                if match:
                    module = match.group(1)
                    dependencies.append(
                        Dependency(
                            source=file_path,
                            target=module,
                            import_name=module,
                            is_relative=module.startswith("."),
                        )
                    )

        self._symbols[file_path] = symbols
        self._references[file_path] = references
        self._dependencies[file_path] = dependencies

    def _extract_python_symbols(
        self,
        node: Node,
        file_path: str,
        content: str,
        symbols: list[Symbol],
        parent: str | None = None,
    ) -> None:
        """Extract symbols from Python AST."""
        if node.type == "function_definition":
            name_node = node.child_by_field_name("name")
            if name_node:
                name = content[name_node.start_byte : name_node.end_byte]
                kind = SymbolKind.METHOD if parent else SymbolKind.FUNCTION

                # Get docstring
                docstring = None
                body = node.child_by_field_name("body")
                if body and body.child_count > 0:
                    first_stmt = body.children[0]
                    if first_stmt.type == "expression_statement":
                        expr = first_stmt.children[0] if first_stmt.children else None
                        if expr and expr.type == "string":
                            docstring = content[expr.start_byte : expr.end_byte].strip(
                                '"\''
                            )

                # Get signature
                params = node.child_by_field_name("parameters")
                signature = (
                    content[params.start_byte : params.end_byte] if params else "()"
                )

                symbols.append(
                    Symbol(
                        name=name,
                        kind=kind,
                        location=Location(
                            file_path,
                            node.start_point[0] + 1,
                            node.start_point[1],
                            node.end_point[0] + 1,
                            node.end_point[1],
                        ),
                        parent=parent,
                        docstring=docstring,
                        signature=f"def {name}{signature}",
                    )
                )

        elif node.type == "class_definition":
            name_node = node.child_by_field_name("name")
            if name_node:
                class_name = content[name_node.start_byte : name_node.end_byte]

                # Get docstring
                docstring = None
                body = node.child_by_field_name("body")
                if body and body.child_count > 0:
                    first_stmt = body.children[0]
                    if first_stmt.type == "expression_statement":
                        expr = first_stmt.children[0] if first_stmt.children else None
                        if expr and expr.type == "string":
                            docstring = content[expr.start_byte : expr.end_byte].strip(
                                '"\''
                            )

                symbol = Symbol(
                    name=class_name,
                    kind=SymbolKind.CLASS,
                    location=Location(
                        file_path,
                        node.start_point[0] + 1,
                        node.start_point[1],
                        node.end_point[0] + 1,
                        node.end_point[1],
                    ),
                    docstring=docstring,
                )
                symbols.append(symbol)

                # Extract methods
                if body:
                    for child in body.children:
                        self._extract_python_symbols(
                            child, file_path, content, symbol.children, class_name
                        )

                # Also add to main list with parent reference
                for child_symbol in symbol.children:
                    child_symbol_copy = Symbol(
                        name=child_symbol.name,
                        kind=child_symbol.kind,
                        location=child_symbol.location,
                        parent=class_name,
                        docstring=child_symbol.docstring,
                        signature=child_symbol.signature,
                    )
                    symbols.append(child_symbol_copy)

                return  # Don't recurse further

        elif node.type == "assignment":
            # Module-level variables/constants
            if parent is None:
                left = node.children[0] if node.children else None
                if left and left.type == "identifier":
                    name = content[left.start_byte : left.end_byte]
                    kind = SymbolKind.CONSTANT if name.isupper() else SymbolKind.VARIABLE
                    symbols.append(
                        Symbol(
                            name=name,
                            kind=kind,
                            location=Location(
                                file_path,
                                node.start_point[0] + 1,
                                node.start_point[1],
                                node.end_point[0] + 1,
                                node.end_point[1],
                            ),
                        )
                    )

        # Recurse
        for child in node.children:
            self._extract_python_symbols(child, file_path, content, symbols, parent)

    def _extract_python_references(
        self,
        node: Node,
        file_path: str,
        content: str,
        references: list[Reference],
    ) -> None:
        """Extract references from Python AST."""
        if node.type == "identifier":
            name = content[node.start_byte : node.end_byte]
            line_start = content.rfind("\n", 0, node.start_byte) + 1
            line_end = content.find("\n", node.end_byte)
            if line_end == -1:
                line_end = len(content)
            context = content[line_start:line_end].strip()

            references.append(
                Reference(
                    symbol_name=name,
                    location=Location(
                        file_path,
                        node.start_point[0] + 1,
                        node.start_point[1],
                        node.end_point[0] + 1,
                        node.end_point[1],
                    ),
                    context=context,
                )
            )

        for child in node.children:
            self._extract_python_references(child, file_path, content, references)

    def _extract_python_dependencies(
        self,
        node: Node,
        file_path: str,
        content: str,
        dependencies: list[Dependency],
    ) -> None:
        """Extract dependencies from Python AST."""
        if node.type == "import_statement":
            # import x, y, z
            for child in node.children:
                if child.type == "dotted_name":
                    module = content[child.start_byte : child.end_byte]
                    dependencies.append(
                        Dependency(
                            source=file_path,
                            target=module,
                            import_name=module,
                            is_relative=False,
                        )
                    )

        elif node.type == "import_from_statement":
            # from x import y
            module_node = node.child_by_field_name("module_name")
            module = ""
            is_relative = False

            if module_node:
                module = content[module_node.start_byte : module_node.end_byte]

            # Check for relative import dots
            for child in node.children:
                if child.type == "relative_import":
                    is_relative = True
                    break
                elif child.type == ".":
                    is_relative = True

            # Get imported names
            for child in node.children:
                if child.type in ("dotted_name", "aliased_import"):
                    if child.type == "aliased_import":
                        name_node = child.child_by_field_name("name")
                        if name_node:
                            import_name = content[
                                name_node.start_byte : name_node.end_byte
                            ]
                        else:
                            import_name = content[child.start_byte : child.end_byte]
                    else:
                        import_name = content[child.start_byte : child.end_byte]

                    dependencies.append(
                        Dependency(
                            source=file_path,
                            target=module or import_name,
                            import_name=import_name,
                            is_relative=is_relative,
                        )
                    )

        for child in node.children:
            self._extract_python_dependencies(child, file_path, content, dependencies)

    def _extract_typescript_symbols(
        self,
        node: Node,
        file_path: str,
        content: str,
        symbols: list[Symbol],
        parent: str | None = None,
    ) -> None:
        """Extract symbols from TypeScript AST."""
        if node.type == "function_declaration":
            name_node = node.child_by_field_name("name")
            if name_node:
                name = content[name_node.start_byte : name_node.end_byte]
                symbols.append(
                    Symbol(
                        name=name,
                        kind=SymbolKind.FUNCTION,
                        location=Location(
                            file_path,
                            node.start_point[0] + 1,
                            node.start_point[1],
                            node.end_point[0] + 1,
                            node.end_point[1],
                        ),
                    )
                )

        elif node.type == "class_declaration":
            name_node = node.child_by_field_name("name")
            if name_node:
                class_name = content[name_node.start_byte : name_node.end_byte]
                symbol = Symbol(
                    name=class_name,
                    kind=SymbolKind.CLASS,
                    location=Location(
                        file_path,
                        node.start_point[0] + 1,
                        node.start_point[1],
                        node.end_point[0] + 1,
                        node.end_point[1],
                    ),
                )
                symbols.append(symbol)

                # Extract methods
                body = node.child_by_field_name("body")
                if body:
                    for child in body.children:
                        self._extract_typescript_symbols(
                            child, file_path, content, symbol.children, class_name
                        )
                        if child.type == "method_definition":
                            method_name_node = child.child_by_field_name("name")
                            if method_name_node:
                                method_name = content[
                                    method_name_node.start_byte : method_name_node.end_byte
                                ]
                                symbols.append(
                                    Symbol(
                                        name=method_name,
                                        kind=SymbolKind.METHOD,
                                        location=Location(
                                            file_path,
                                            child.start_point[0] + 1,
                                            child.start_point[1],
                                            child.end_point[0] + 1,
                                            child.end_point[1],
                                        ),
                                        parent=class_name,
                                    )
                                )
                return

        elif node.type == "interface_declaration":
            name_node = node.child_by_field_name("name")
            if name_node:
                name = content[name_node.start_byte : name_node.end_byte]
                symbols.append(
                    Symbol(
                        name=name,
                        kind=SymbolKind.INTERFACE,
                        location=Location(
                            file_path,
                            node.start_point[0] + 1,
                            node.start_point[1],
                            node.end_point[0] + 1,
                            node.end_point[1],
                        ),
                    )
                )

        elif node.type == "type_alias_declaration":
            name_node = node.child_by_field_name("name")
            if name_node:
                name = content[name_node.start_byte : name_node.end_byte]
                symbols.append(
                    Symbol(
                        name=name,
                        kind=SymbolKind.TYPE_ALIAS,
                        location=Location(
                            file_path,
                            node.start_point[0] + 1,
                            node.start_point[1],
                            node.end_point[0] + 1,
                            node.end_point[1],
                        ),
                    )
                )

        elif node.type == "lexical_declaration":
            # const/let/var declarations
            for child in node.children:
                if child.type == "variable_declarator":
                    name_node = child.child_by_field_name("name")
                    if name_node and name_node.type == "identifier":
                        name = content[name_node.start_byte : name_node.end_byte]
                        # Check if it's an arrow function
                        value = child.child_by_field_name("value")
                        if value and value.type == "arrow_function":
                            kind = SymbolKind.FUNCTION
                        elif name.isupper():
                            kind = SymbolKind.CONSTANT
                        else:
                            kind = SymbolKind.VARIABLE

                        symbols.append(
                            Symbol(
                                name=name,
                                kind=kind,
                                location=Location(
                                    file_path,
                                    node.start_point[0] + 1,
                                    node.start_point[1],
                                    node.end_point[0] + 1,
                                    node.end_point[1],
                                ),
                            )
                        )

        elif node.type == "enum_declaration":
            name_node = node.child_by_field_name("name")
            if name_node:
                name = content[name_node.start_byte : name_node.end_byte]
                symbols.append(
                    Symbol(
                        name=name,
                        kind=SymbolKind.ENUM,
                        location=Location(
                            file_path,
                            node.start_point[0] + 1,
                            node.start_point[1],
                            node.end_point[0] + 1,
                            node.end_point[1],
                        ),
                    )
                )

        for child in node.children:
            self._extract_typescript_symbols(child, file_path, content, symbols, parent)

    def _extract_typescript_references(
        self,
        node: Node,
        file_path: str,
        content: str,
        references: list[Reference],
    ) -> None:
        """Extract references from TypeScript AST."""
        if node.type == "identifier":
            name = content[node.start_byte : node.end_byte]
            line_start = content.rfind("\n", 0, node.start_byte) + 1
            line_end = content.find("\n", node.end_byte)
            if line_end == -1:
                line_end = len(content)
            context = content[line_start:line_end].strip()

            references.append(
                Reference(
                    symbol_name=name,
                    location=Location(
                        file_path,
                        node.start_point[0] + 1,
                        node.start_point[1],
                        node.end_point[0] + 1,
                        node.end_point[1],
                    ),
                    context=context,
                )
            )

        for child in node.children:
            self._extract_typescript_references(child, file_path, content, references)

    def _extract_typescript_dependencies(
        self,
        node: Node,
        file_path: str,
        content: str,
        dependencies: list[Dependency],
    ) -> None:
        """Extract dependencies from TypeScript AST."""
        if node.type == "import_statement":
            # Find the source string
            source_node = node.child_by_field_name("source")
            if source_node:
                module = content[source_node.start_byte : source_node.end_byte].strip(
                    "'\""
                )
                is_relative = module.startswith(".")

                # Get imported names
                for child in node.children:
                    if child.type == "import_clause":
                        for import_child in child.children:
                            if import_child.type == "identifier":
                                import_name = content[
                                    import_child.start_byte : import_child.end_byte
                                ]
                                dependencies.append(
                                    Dependency(
                                        source=file_path,
                                        target=module,
                                        import_name=import_name,
                                        is_relative=is_relative,
                                    )
                                )
                            elif import_child.type == "named_imports":
                                for named in import_child.children:
                                    if named.type == "import_specifier":
                                        name_node = named.child_by_field_name("name")
                                        if name_node:
                                            import_name = content[
                                                name_node.start_byte : name_node.end_byte
                                            ]
                                            dependencies.append(
                                                Dependency(
                                                    source=file_path,
                                                    target=module,
                                                    import_name=import_name,
                                                    is_relative=is_relative,
                                                )
                                            )

        for child in node.children:
            self._extract_typescript_dependencies(
                child, file_path, content, dependencies
            )

    # Public API

    def find_symbol(
        self,
        name: str,
        kind: SymbolKind | None = None,
        file_path: str | None = None,
        exact_match: bool = False,
    ) -> list[Symbol]:
        """Find symbols by name.

        Args:
            name: Symbol name to search for
            kind: Optional filter by symbol kind
            file_path: Optional filter by file
            exact_match: If True, require exact name match

        Returns:
            List of matching symbols
        """
        results = []

        files_to_search = (
            [file_path] if file_path else list(self._symbols.keys())
        )

        for fp in files_to_search:
            if fp not in self._symbols:
                continue

            for symbol in self._symbols[fp]:
                # Name match
                if exact_match:
                    if symbol.name != name:
                        continue
                else:
                    if name.lower() not in symbol.name.lower():
                        continue

                # Kind filter
                if kind and symbol.kind != kind:
                    continue

                results.append(symbol)

        return results

    def find_references(
        self,
        symbol_name: str,
        file_path: str | None = None,
    ) -> list[Reference]:
        """Find all references to a symbol.

        Args:
            symbol_name: Name of the symbol
            file_path: Optional filter by file

        Returns:
            List of references
        """
        results = []

        files_to_search = (
            [file_path] if file_path else list(self._references.keys())
        )

        for fp in files_to_search:
            if fp not in self._references:
                continue

            for ref in self._references[fp]:
                if ref.symbol_name == symbol_name:
                    results.append(ref)

        return results

    def get_dependencies(self, file_path: str) -> list[Dependency]:
        """Get import dependencies for a file.

        Args:
            file_path: Path to the file

        Returns:
            List of dependencies
        """
        return self._dependencies.get(file_path, [])

    def get_file_symbols(self, file_path: str) -> list[Symbol]:
        """Get all symbols in a file.

        Args:
            file_path: Path to the file

        Returns:
            List of symbols
        """
        return self._symbols.get(file_path, [])

    def get_all_symbols(self) -> list[Symbol]:
        """Get all symbols across all files.

        Returns:
            List of all symbols
        """
        all_symbols = []
        for symbols in self._symbols.values():
            all_symbols.extend(symbols)
        return all_symbols

    def get_dependency_graph(self) -> dict[str, list[str]]:
        """Build dependency graph for all files.

        Returns:
            Dict mapping file paths to their dependencies
        """
        graph = {}

        for file_path, deps in self._dependencies.items():
            targets = list({d.target for d in deps})
            graph[file_path] = targets

        return graph

    def clear(self) -> None:
        """Clear all parsed data."""
        self._symbols.clear()
        self._references.clear()
        self._dependencies.clear()
        self._file_contents.clear()
