"""MCP Code Analysis Tools.

Provides advanced code analysis tools for MCP integration:
- Semantic search (TF-IDF based natural language search)
- Call graph analysis (callers, callees, paths)
- Symbol analysis (find definitions, references)
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

from mcp.types import Tool

from c4.docs.analyzer import CodeAnalyzer, SymbolKind
from c4.docs.call_graph import CallGraphAnalyzer, MCP_CALL_GRAPH_TOOLS
from c4.docs.semantic_search import (
    SearchScope,
    SemanticSearcher,
    MCP_SEMANTIC_TOOLS,
)


class CodeToolsHandler:
    """Handler for code analysis MCP tools.

    Provides:
    - Semantic search over codebase
    - Call graph analysis
    - Symbol lookup and navigation

    Example:
        handler = CodeToolsHandler(project_root="/path/to/project")
        handler.initialize()

        # Handle tool call
        result = handler.handle("semantic_search", {"query": "authentication"})
    """

    def __init__(self, project_root: str | Path) -> None:
        """Initialize the handler.

        Args:
            project_root: Root directory of the project
        """
        self.project_root = Path(project_root).resolve()
        self._analyzer: CodeAnalyzer | None = None
        self._searcher: SemanticSearcher | None = None
        self._call_graph: CallGraphAnalyzer | None = None
        self._initialized = False

    def initialize(self) -> int:
        """Initialize all analyzers.

        Returns:
            Number of files indexed
        """
        # Create shared analyzer
        self._analyzer = CodeAnalyzer()

        # Initialize searcher
        self._searcher = SemanticSearcher(
            project_root=self.project_root,
            analyzer=self._analyzer,
        )
        count = self._searcher.index()

        # Initialize call graph
        self._call_graph = CallGraphAnalyzer(
            project_root=self.project_root,
            code_analyzer=self._analyzer,
        )
        self._call_graph.build()

        self._initialized = True
        return count

    def _ensure_initialized(self) -> None:
        """Ensure analyzers are initialized."""
        if not self._initialized:
            self.initialize()

    @property
    def tools(self) -> list[Tool]:
        """Get all available tools.

        Returns:
            List of MCP Tool definitions
        """
        tools = []

        # Semantic search tools
        for tool_def in MCP_SEMANTIC_TOOLS:
            tools.append(Tool(
                name=f"c4_{tool_def['name']}",
                description=tool_def["description"],
                inputSchema=tool_def["inputSchema"],
            ))

        # Call graph tools
        for tool_def in MCP_CALL_GRAPH_TOOLS:
            tools.append(Tool(
                name=f"c4_{tool_def['name']}",
                description=tool_def["description"],
                inputSchema=tool_def["inputSchema"],
            ))

        # Additional code analysis tools
        tools.extend([
            Tool(
                name="c4_find_definition",
                description="Find the definition of a symbol (function, class, variable, etc.)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "symbol_name": {
                            "type": "string",
                            "description": "Name of the symbol to find",
                        },
                        "kind": {
                            "type": "string",
                            "description": "Optional symbol kind filter",
                            "enum": ["class", "function", "method", "variable", "constant", "interface"],
                        },
                        "format": {
                            "type": "string",
                            "description": "Output format (markdown or json)",
                            "enum": ["markdown", "json"],
                            "default": "markdown",
                        },
                    },
                    "required": ["symbol_name"],
                },
            ),
            Tool(
                name="c4_find_references",
                description="Find all references to a symbol in the codebase",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "symbol_name": {
                            "type": "string",
                            "description": "Name of the symbol to find references for",
                        },
                        "limit": {
                            "type": "integer",
                            "description": "Maximum number of references to return (default: 20)",
                            "default": 20,
                        },
                        "format": {
                            "type": "string",
                            "description": "Output format (markdown or json)",
                            "enum": ["markdown", "json"],
                            "default": "markdown",
                        },
                    },
                    "required": ["symbol_name"],
                },
            ),
            Tool(
                name="c4_analyze_file",
                description="Analyze a file and list all its symbols (classes, functions, etc.)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "file_path": {
                            "type": "string",
                            "description": "Path to the file (relative to project root)",
                        },
                        "include_private": {
                            "type": "boolean",
                            "description": "Include private symbols (starting with _)",
                            "default": False,
                        },
                        "format": {
                            "type": "string",
                            "description": "Output format (markdown or json)",
                            "enum": ["markdown", "json"],
                            "default": "markdown",
                        },
                    },
                    "required": ["file_path"],
                },
            ),
            Tool(
                name="c4_get_dependencies",
                description="Get import dependencies for a file or module",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "file_path": {
                            "type": "string",
                            "description": "Path to the file (relative to project root)",
                        },
                        "include_stdlib": {
                            "type": "boolean",
                            "description": "Include standard library imports",
                            "default": False,
                        },
                        "format": {
                            "type": "string",
                            "description": "Output format (markdown or json)",
                            "enum": ["markdown", "json"],
                            "default": "markdown",
                        },
                    },
                    "required": ["file_path"],
                },
            ),
        ])

        return tools

    def handle(
        self,
        tool_name: str,
        arguments: dict[str, Any],
    ) -> str | dict[str, Any]:
        """Handle a tool call.

        Args:
            tool_name: Name of the tool (with or without c4_ prefix)
            arguments: Tool arguments

        Returns:
            Tool result
        """
        self._ensure_initialized()

        # Remove c4_ prefix if present
        if tool_name.startswith("c4_"):
            tool_name = tool_name[3:]

        format_str = arguments.get("format", "markdown")

        # Semantic search tools
        if tool_name == "semantic_search":
            scope_str = arguments.get("scope", "all")
            scope = SearchScope(scope_str)

            result = self._searcher.search(
                query=arguments["query"],
                scope=scope,
                limit=arguments.get("limit", 20),
                expand_synonyms=arguments.get("expand_synonyms", True),
            )

            if format_str == "json":
                return result.to_dict()
            return result.to_markdown()

        elif tool_name == "find_related_symbols":
            hits = self._searcher.find_related(
                symbol_name=arguments["symbol_name"],
                limit=arguments.get("limit", 10),
            )

            if format_str == "json":
                return {
                    "symbol": arguments["symbol_name"],
                    "related": [h.to_dict() for h in hits],
                }

            lines = [f"# Related Symbols: `{arguments['symbol_name']}`", ""]
            for hit in hits:
                lines.append(f"- `{hit.symbol_name}` ({hit.file_path}:{hit.line_number}) - score: {hit.score:.3f}")
            return "\n".join(lines)

        elif tool_name == "search_by_type":
            kind_map = {
                "class": SymbolKind.CLASS,
                "function": SymbolKind.FUNCTION,
                "method": SymbolKind.METHOD,
                "interface": SymbolKind.INTERFACE,
                "type_alias": SymbolKind.TYPE_ALIAS,
                "enum": SymbolKind.ENUM,
                "constant": SymbolKind.CONSTANT,
                "variable": SymbolKind.VARIABLE,
            }
            kind = kind_map.get(arguments["kind"], SymbolKind.FUNCTION)

            hits = self._searcher.search_by_type(
                kind=kind,
                query=arguments.get("query"),
                limit=arguments.get("limit", 20),
            )

            if format_str == "json":
                return {
                    "kind": arguments["kind"],
                    "results": [h.to_dict() for h in hits],
                }

            lines = [f"# Symbols of type: `{arguments['kind']}`", ""]
            for hit in hits:
                lines.append(f"- `{hit.symbol_name}` ({hit.file_path}:{hit.line_number})")
            return "\n".join(lines)

        # Call graph tools
        elif tool_name == "get_callers":
            callers = self._call_graph.get_callers(
                symbol_name=arguments["symbol_name"],
                max_depth=arguments.get("max_depth", 1),
            )

            if format_str == "json":
                return {
                    "symbol": arguments["symbol_name"],
                    "callers": [c.to_dict() for c in callers],
                }

            lines = [f"# Callers of `{arguments['symbol_name']}`", ""]
            for c in callers:
                lines.append(f"- `{c.qualified_name}` ({c.file_path}:{c.line_number})")
            return "\n".join(lines)

        elif tool_name == "get_callees":
            callees = self._call_graph.get_callees(
                symbol_name=arguments["symbol_name"],
                max_depth=arguments.get("max_depth", 1),
            )

            if format_str == "json":
                return {
                    "symbol": arguments["symbol_name"],
                    "callees": [c.to_dict() for c in callees],
                }

            lines = [f"# Callees of `{arguments['symbol_name']}`", ""]
            for c in callees:
                lines.append(f"- `{c.qualified_name}` ({c.file_path}:{c.line_number})")
            return "\n".join(lines)

        elif tool_name == "find_call_paths":
            paths = self._call_graph.find_paths(
                from_symbol=arguments["from_symbol"],
                to_symbol=arguments["to_symbol"],
                max_depth=arguments.get("max_depth", 5),
            )

            if format_str == "json":
                return {
                    "from": arguments["from_symbol"],
                    "to": arguments["to_symbol"],
                    "paths": [p.to_dict() for p in paths],
                }

            lines = [f"# Call Paths: `{arguments['from_symbol']}` → `{arguments['to_symbol']}`", ""]
            for i, p in enumerate(paths, 1):
                lines.append(f"### Path {i}")
                lines.append(" → ".join(p.nodes))
                lines.append("")
            return "\n".join(lines)

        elif tool_name == "call_graph_stats":
            stats = self._call_graph.get_stats()

            if format_str == "json":
                return stats.to_dict()
            return stats.to_markdown()

        elif tool_name == "call_graph_diagram":
            diagram = self._call_graph.to_mermaid(
                symbol_name=arguments.get("symbol_name"),
                depth=arguments.get("depth", 2),
            )
            return diagram

        # Code analysis tools
        elif tool_name == "find_definition":
            kind_map = {
                "class": SymbolKind.CLASS,
                "function": SymbolKind.FUNCTION,
                "method": SymbolKind.METHOD,
                "variable": SymbolKind.VARIABLE,
                "constant": SymbolKind.CONSTANT,
                "interface": SymbolKind.INTERFACE,
            }
            kind = kind_map.get(arguments.get("kind"))

            symbols = self._analyzer.find_symbol(
                name=arguments["symbol_name"],
                kind=kind,
                exact_match=True,
            )

            if not symbols:
                symbols = self._analyzer.find_symbol(
                    name=arguments["symbol_name"],
                    kind=kind,
                    exact_match=False,
                )

            if format_str == "json":
                return {
                    "symbol": arguments["symbol_name"],
                    "definitions": [
                        {
                            "name": s.name,
                            "kind": s.kind.value,
                            "file_path": s.location.file_path,
                            "line_number": s.location.start_line,
                            "signature": s.signature,
                            "docstring": s.docstring,
                        }
                        for s in symbols
                    ],
                }

            lines = [f"# Definition: `{arguments['symbol_name']}`", ""]
            for s in symbols:
                lines.append(f"### {s.kind.value}: `{s.qualified_name}`")
                lines.append(f"**Location:** `{s.location.file_path}:{s.location.start_line}`")
                if s.signature:
                    lines.append(f"```python\n{s.signature}\n```")
                if s.docstring:
                    lines.append(f"\n{s.docstring}")
                lines.append("")
            return "\n".join(lines)

        elif tool_name == "find_references":
            refs = self._analyzer.find_references(
                symbol_name=arguments["symbol_name"],
            )
            refs = refs[:arguments.get("limit", 20)]

            if format_str == "json":
                return {
                    "symbol": arguments["symbol_name"],
                    "references": [
                        {
                            "file_path": r.location.file_path,
                            "line_number": r.location.start_line,
                            "context": r.context,
                            "kind": r.ref_kind,
                        }
                        for r in refs
                    ],
                }

            lines = [f"# References: `{arguments['symbol_name']}`", ""]
            lines.append(f"Found **{len(refs)}** references", "")
            for r in refs:
                lines.append(f"- `{r.location.file_path}:{r.location.start_line}`: {r.context}")
            return "\n".join(lines)

        elif tool_name == "analyze_file":
            file_path = arguments["file_path"]
            abs_path = self.project_root / file_path

            # Add file if not already indexed
            if str(abs_path) not in self._analyzer._symbols:
                self._analyzer.add_file(abs_path)

            symbols = self._analyzer.get_file_symbols(str(abs_path))

            # Filter private if requested
            if not arguments.get("include_private", False):
                symbols = [s for s in symbols if not s.name.startswith("_")]

            if format_str == "json":
                return {
                    "file_path": file_path,
                    "symbols": [
                        {
                            "name": s.name,
                            "kind": s.kind.value,
                            "line_number": s.location.start_line,
                            "signature": s.signature,
                            "parent": s.parent,
                        }
                        for s in symbols
                    ],
                }

            lines = [f"# File Analysis: `{file_path}`", ""]

            # Group by kind
            classes = [s for s in symbols if s.kind == SymbolKind.CLASS]
            functions = [s for s in symbols if s.kind == SymbolKind.FUNCTION]
            methods = [s for s in symbols if s.kind == SymbolKind.METHOD]

            if classes:
                lines.append("## Classes")
                for s in classes:
                    lines.append(f"- `{s.name}` (line {s.location.start_line})")
                lines.append("")

            if functions:
                lines.append("## Functions")
                for s in functions:
                    sig = s.signature or s.name
                    lines.append(f"- `{sig}` (line {s.location.start_line})")
                lines.append("")

            if methods:
                lines.append("## Methods")
                for s in methods:
                    lines.append(f"- `{s.parent}.{s.name}` (line {s.location.start_line})")
                lines.append("")

            return "\n".join(lines)

        elif tool_name == "get_dependencies":
            file_path = arguments["file_path"]
            abs_path = self.project_root / file_path

            # Add file if not already indexed
            if str(abs_path) not in self._analyzer._dependencies:
                self._analyzer.add_file(abs_path)

            deps = self._analyzer.get_dependencies(str(abs_path))

            # Filter stdlib if requested
            stdlib_modules = {
                "os", "sys", "re", "json", "logging", "typing", "pathlib",
                "collections", "datetime", "time", "math", "random",
                "functools", "itertools", "dataclasses", "enum", "abc",
            }

            if not arguments.get("include_stdlib", False):
                deps = [d for d in deps if d.target.split(".")[0] not in stdlib_modules]

            if format_str == "json":
                return {
                    "file_path": file_path,
                    "dependencies": [
                        {
                            "target": d.target,
                            "import_name": d.import_name,
                            "is_relative": d.is_relative,
                        }
                        for d in deps
                    ],
                }

            lines = [f"# Dependencies: `{file_path}`", ""]

            local_deps = [d for d in deps if d.is_relative]
            external_deps = [d for d in deps if not d.is_relative]

            if local_deps:
                lines.append("## Local Dependencies")
                for d in local_deps:
                    lines.append(f"- `{d.target}` (imports: {d.import_name})")
                lines.append("")

            if external_deps:
                lines.append("## External Dependencies")
                for d in external_deps:
                    lines.append(f"- `{d.target}` (imports: {d.import_name})")
                lines.append("")

            return "\n".join(lines)

        else:
            return {"error": f"Unknown tool: {tool_name}"}


# Tool definitions for registration
def get_code_tools() -> list[dict[str, Any]]:
    """Get all code analysis tool definitions.

    Returns:
        List of tool definition dictionaries
    """
    tools = []

    # Semantic search tools
    for tool_def in MCP_SEMANTIC_TOOLS:
        tools.append({
            "name": f"c4_{tool_def['name']}",
            "description": tool_def["description"],
            "inputSchema": tool_def["inputSchema"],
        })

    # Call graph tools
    for tool_def in MCP_CALL_GRAPH_TOOLS:
        tools.append({
            "name": f"c4_{tool_def['name']}",
            "description": tool_def["description"],
            "inputSchema": tool_def["inputSchema"],
        })

    # Additional tools
    tools.extend([
        {
            "name": "c4_find_definition",
            "description": "Find the definition of a symbol (function, class, variable, etc.)",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "symbol_name": {"type": "string"},
                    "kind": {"type": "string", "enum": ["class", "function", "method", "variable", "constant", "interface"]},
                    "format": {"type": "string", "enum": ["markdown", "json"], "default": "markdown"},
                },
                "required": ["symbol_name"],
            },
        },
        {
            "name": "c4_find_references",
            "description": "Find all references to a symbol in the codebase",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "symbol_name": {"type": "string"},
                    "limit": {"type": "integer", "default": 20},
                    "format": {"type": "string", "enum": ["markdown", "json"], "default": "markdown"},
                },
                "required": ["symbol_name"],
            },
        },
        {
            "name": "c4_analyze_file",
            "description": "Analyze a file and list all its symbols",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "file_path": {"type": "string"},
                    "include_private": {"type": "boolean", "default": False},
                    "format": {"type": "string", "enum": ["markdown", "json"], "default": "markdown"},
                },
                "required": ["file_path"],
            },
        },
        {
            "name": "c4_get_dependencies",
            "description": "Get import dependencies for a file",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "file_path": {"type": "string"},
                    "include_stdlib": {"type": "boolean", "default": False},
                    "format": {"type": "string", "enum": ["markdown", "json"], "default": "markdown"},
                },
                "required": ["file_path"],
            },
        },
    ])

    return tools
