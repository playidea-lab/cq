"""Call Graph Analyzer for Code Navigation.

Provides call relationship analysis:
- Function/method call graph
- Class inheritance hierarchy
- Module dependency graph
- Bidirectional navigation (callers/callees)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any

from c4.docs.analyzer import CodeAnalyzer, Symbol, SymbolKind


class RelationType(Enum):
    """Type of relationship between symbols."""

    CALLS = "calls"  # Function A calls function B
    CALLED_BY = "called_by"  # Function A is called by function B
    INHERITS = "inherits"  # Class A inherits from class B
    INHERITED_BY = "inherited_by"  # Class A is inherited by class B
    IMPORTS = "imports"  # File A imports from file B
    IMPORTED_BY = "imported_by"  # File A is imported by file B
    USES = "uses"  # Symbol A uses symbol B
    USED_BY = "used_by"  # Symbol A is used by symbol B
    IMPLEMENTS = "implements"  # Class implements interface
    IMPLEMENTED_BY = "implemented_by"  # Interface is implemented by class


@dataclass
class CallEdge:
    """An edge in the call graph."""

    source: str  # Caller symbol qualified name
    target: str  # Callee symbol qualified name
    source_file: str
    target_file: str | None
    source_line: int
    relation: RelationType
    count: int = 1  # Number of times this call occurs
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "source": self.source,
            "target": self.target,
            "source_file": self.source_file,
            "target_file": self.target_file,
            "source_line": self.source_line,
            "relation": self.relation.value,
            "count": self.count,
            "metadata": self.metadata,
        }


@dataclass
class CallNode:
    """A node in the call graph."""

    name: str
    qualified_name: str
    file_path: str
    line_number: int
    kind: SymbolKind
    outgoing: list[CallEdge] = field(default_factory=list)  # Calls made
    incoming: list[CallEdge] = field(default_factory=list)  # Callers
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "qualified_name": self.qualified_name,
            "file_path": self.file_path,
            "line_number": self.line_number,
            "kind": self.kind.value,
            "outgoing_count": len(self.outgoing),
            "incoming_count": len(self.incoming),
            "outgoing": [e.target for e in self.outgoing],
            "incoming": [e.source for e in self.incoming],
            "metadata": self.metadata,
        }


@dataclass
class CallPath:
    """A path through the call graph."""

    nodes: list[str]  # List of qualified names
    edges: list[CallEdge]
    total_weight: float = 0.0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "nodes": self.nodes,
            "edges": [e.to_dict() for e in self.edges],
            "total_weight": self.total_weight,
        }


@dataclass
class CallGraphStats:
    """Statistics about the call graph."""

    total_nodes: int
    total_edges: int
    max_depth: int
    most_called: list[tuple[str, int]]  # Top called functions
    most_callers: list[tuple[str, int]]  # Functions with most dependencies
    isolated: list[str]  # Functions with no calls
    entry_points: list[str]  # Functions with no callers

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "total_nodes": self.total_nodes,
            "total_edges": self.total_edges,
            "max_depth": self.max_depth,
            "most_called": [{"name": n, "count": c} for n, c in self.most_called],
            "most_callers": [{"name": n, "count": c} for n, c in self.most_callers],
            "isolated_count": len(self.isolated),
            "entry_points": self.entry_points[:10],
        }

    def to_markdown(self) -> str:
        """Convert to Markdown format."""
        lines = []
        lines.append("# Call Graph Statistics")
        lines.append("")
        lines.append(f"**Total Nodes:** {self.total_nodes}")
        lines.append(f"**Total Edges:** {self.total_edges}")
        lines.append(f"**Max Depth:** {self.max_depth}")
        lines.append(f"**Isolated Functions:** {len(self.isolated)}")
        lines.append("")

        if self.most_called:
            lines.append("## Most Called Functions")
            for name, count in self.most_called[:10]:
                lines.append(f"- `{name}`: {count} calls")
            lines.append("")

        if self.most_callers:
            lines.append("## Functions with Most Dependencies")
            for name, count in self.most_callers[:10]:
                lines.append(f"- `{name}`: {count} outgoing calls")
            lines.append("")

        if self.entry_points:
            lines.append("## Entry Points (No Callers)")
            for name in self.entry_points[:10]:
                lines.append(f"- `{name}`")
            lines.append("")

        return "\n".join(lines)


class CallGraphAnalyzer:
    """Analyzes call relationships in code.

    Features:
    - Build function call graph
    - Find callers/callees
    - Find call paths between functions
    - Identify hotspots and entry points

    Example:
        analyzer = CallGraphAnalyzer(project_root=".")
        analyzer.build()

        # Get callers of a function
        callers = analyzer.get_callers("process_request")

        # Get what a function calls
        callees = analyzer.get_callees("process_request")

        # Find path between functions
        paths = analyzer.find_paths("main", "save_to_db")
    """

    def __init__(
        self,
        project_root: str | Path = ".",
        code_analyzer: CodeAnalyzer | None = None,
    ) -> None:
        """Initialize the call graph analyzer.

        Args:
            project_root: Root directory of the project
            code_analyzer: Optional CodeAnalyzer instance to reuse
        """
        self.project_root = Path(project_root).resolve()
        self._analyzer = code_analyzer or CodeAnalyzer()
        self._built = False

        # Graph storage
        self._nodes: dict[str, CallNode] = {}  # qualified_name -> node
        self._edges: list[CallEdge] = []
        self._symbol_map: dict[str, Symbol] = {}  # name -> symbol

    def build(
        self,
        exclude_patterns: list[str] | None = None,
    ) -> int:
        """Build the call graph.

        Args:
            exclude_patterns: Glob patterns to exclude

        Returns:
            Number of nodes in the graph
        """
        exclude_patterns = exclude_patterns or [
            "**/node_modules/**",
            "**/__pycache__/**",
            "**/.git/**",
            "**/venv/**",
            "**/.venv/**",
        ]

        # Reset graph
        self._nodes.clear()
        self._edges.clear()
        self._symbol_map.clear()

        # Index code
        self._analyzer.add_directory(
            self.project_root,
            recursive=True,
            exclude_patterns=exclude_patterns,
        )

        # Build symbol map
        for symbol in self._analyzer.get_all_symbols():
            if symbol.kind in (SymbolKind.FUNCTION, SymbolKind.METHOD, SymbolKind.CLASS):
                key = symbol.qualified_name
                self._symbol_map[key] = symbol
                self._symbol_map[symbol.name] = symbol

                # Create node
                self._nodes[key] = CallNode(
                    name=symbol.name,
                    qualified_name=key,
                    file_path=symbol.location.file_path,
                    line_number=symbol.location.start_line,
                    kind=symbol.kind,
                )

        # Analyze calls
        self._analyze_calls()

        self._built = True
        return len(self._nodes)

    def _analyze_calls(self) -> None:
        """Analyze call relationships from references."""
        # For each symbol, find its references
        for qualified_name, symbol in self._symbol_map.items():
            if "." in qualified_name:  # Skip duplicates
                continue

            refs = self._analyzer.find_references(symbol.name)

            for ref in refs:
                # Find which function contains this reference
                caller = self._find_containing_function(
                    ref.location.file_path,
                    ref.location.start_line,
                )

                if caller and caller.qualified_name != qualified_name:
                    # Create edge
                    edge = CallEdge(
                        source=caller.qualified_name,
                        target=qualified_name,
                        source_file=ref.location.file_path,
                        target_file=symbol.location.file_path,
                        source_line=ref.location.start_line,
                        relation=RelationType.CALLS,
                    )
                    self._edges.append(edge)

                    # Update nodes
                    if caller.qualified_name in self._nodes:
                        self._nodes[caller.qualified_name].outgoing.append(edge)

                    if qualified_name in self._nodes:
                        reverse_edge = CallEdge(
                            source=caller.qualified_name,
                            target=qualified_name,
                            source_file=ref.location.file_path,
                            target_file=symbol.location.file_path,
                            source_line=ref.location.start_line,
                            relation=RelationType.CALLED_BY,
                        )
                        self._nodes[qualified_name].incoming.append(reverse_edge)

    def _find_containing_function(
        self,
        file_path: str,
        line_number: int,
    ) -> Symbol | None:
        """Find the function containing a given line.

        Args:
            file_path: Path to the file
            line_number: Line number

        Returns:
            Symbol of the containing function, or None
        """
        symbols = self._analyzer.get_file_symbols(file_path)

        # Find the innermost function containing this line
        best_match = None
        best_size = float("inf")

        for symbol in symbols:
            if symbol.kind not in (SymbolKind.FUNCTION, SymbolKind.METHOD):
                continue

            start = symbol.location.start_line
            end = symbol.location.end_line

            if start <= line_number <= end:
                size = end - start
                if size < best_size:
                    best_match = symbol
                    best_size = size

        return best_match

    def get_callers(
        self,
        symbol_name: str,
        max_depth: int = 1,
    ) -> list[CallNode]:
        """Get all functions that call a given symbol.

        Args:
            symbol_name: Name of the symbol
            max_depth: How many levels of callers to return

        Returns:
            List of caller nodes
        """
        if not self._built:
            self.build()

        # Find the symbol
        target_key = None
        for key in self._nodes:
            if key == symbol_name or key.endswith(f".{symbol_name}"):
                target_key = key
                break

        if not target_key or target_key not in self._nodes:
            return []

        callers = []
        visited = set()
        queue = [(target_key, 0)]

        while queue:
            current, depth = queue.pop(0)

            if current in visited or depth > max_depth:
                continue

            visited.add(current)

            node = self._nodes.get(current)
            if node and depth > 0:  # Don't include the target itself
                callers.append(node)

            # Add callers to queue
            if node and depth < max_depth:
                for edge in node.incoming:
                    if edge.source not in visited:
                        queue.append((edge.source, depth + 1))

        return callers

    def get_callees(
        self,
        symbol_name: str,
        max_depth: int = 1,
    ) -> list[CallNode]:
        """Get all functions called by a given symbol.

        Args:
            symbol_name: Name of the symbol
            max_depth: How many levels of callees to return

        Returns:
            List of callee nodes
        """
        if not self._built:
            self.build()

        # Find the symbol
        source_key = None
        for key in self._nodes:
            if key == symbol_name or key.endswith(f".{symbol_name}"):
                source_key = key
                break

        if not source_key or source_key not in self._nodes:
            return []

        callees = []
        visited = set()
        queue = [(source_key, 0)]

        while queue:
            current, depth = queue.pop(0)

            if current in visited or depth > max_depth:
                continue

            visited.add(current)

            node = self._nodes.get(current)
            if node and depth > 0:  # Don't include the source itself
                callees.append(node)

            # Add callees to queue
            if node and depth < max_depth:
                for edge in node.outgoing:
                    if edge.target not in visited:
                        queue.append((edge.target, depth + 1))

        return callees

    def find_paths(
        self,
        from_symbol: str,
        to_symbol: str,
        max_depth: int = 5,
    ) -> list[CallPath]:
        """Find call paths between two symbols.

        Args:
            from_symbol: Starting symbol name
            to_symbol: Target symbol name
            max_depth: Maximum path length

        Returns:
            List of paths from source to target
        """
        if not self._built:
            self.build()

        # Find symbol keys
        from_key = None
        to_key = None

        for key in self._nodes:
            if key == from_symbol or key.endswith(f".{from_symbol}"):
                from_key = key
            if key == to_symbol or key.endswith(f".{to_symbol}"):
                to_key = key

        if not from_key or not to_key:
            return []

        # BFS to find all paths
        paths = []
        queue = [([from_key], [])]  # (node path, edge path)

        while queue:
            node_path, edge_path = queue.pop(0)
            current = node_path[-1]

            if len(node_path) > max_depth:
                continue

            if current == to_key:
                paths.append(CallPath(
                    nodes=node_path,
                    edges=edge_path,
                    total_weight=len(edge_path),
                ))
                continue

            node = self._nodes.get(current)
            if not node:
                continue

            for edge in node.outgoing:
                if edge.target not in node_path:  # Avoid cycles
                    queue.append((
                        node_path + [edge.target],
                        edge_path + [edge],
                    ))

        return paths

    def get_stats(self) -> CallGraphStats:
        """Get statistics about the call graph.

        Returns:
            CallGraphStats with graph metrics
        """
        if not self._built:
            self.build()

        # Count incoming/outgoing
        incoming_counts: dict[str, int] = {}
        outgoing_counts: dict[str, int] = {}

        for node in self._nodes.values():
            incoming_counts[node.qualified_name] = len(node.incoming)
            outgoing_counts[node.qualified_name] = len(node.outgoing)

        # Most called
        most_called = sorted(
            incoming_counts.items(),
            key=lambda x: x[1],
            reverse=True,
        )[:10]

        # Most callers
        most_callers = sorted(
            outgoing_counts.items(),
            key=lambda x: x[1],
            reverse=True,
        )[:10]

        # Isolated functions
        isolated = [
            name for name, count in incoming_counts.items()
            if count == 0 and outgoing_counts.get(name, 0) == 0
        ]

        # Entry points (no callers but have callees)
        entry_points = [
            name for name, count in incoming_counts.items()
            if count == 0 and outgoing_counts.get(name, 0) > 0
        ]

        # Calculate max depth (longest path from entry point)
        max_depth = self._calculate_max_depth()

        return CallGraphStats(
            total_nodes=len(self._nodes),
            total_edges=len(self._edges),
            max_depth=max_depth,
            most_called=most_called,
            most_callers=most_callers,
            isolated=isolated,
            entry_points=entry_points,
        )

    def _calculate_max_depth(self) -> int:
        """Calculate the maximum depth of the call graph."""
        max_depth = 0

        for node in self._nodes.values():
            if not node.incoming:  # Entry point
                depth = self._dfs_depth(node.qualified_name, set())
                max_depth = max(max_depth, depth)

        return max_depth

    def _dfs_depth(self, node_name: str, visited: set) -> int:
        """DFS to find max depth from a node."""
        if node_name in visited:
            return 0

        visited.add(node_name)
        node = self._nodes.get(node_name)

        if not node or not node.outgoing:
            return 0

        max_child_depth = 0
        for edge in node.outgoing:
            child_depth = self._dfs_depth(edge.target, visited)
            max_child_depth = max(max_child_depth, child_depth)

        return max_child_depth + 1

    def to_mermaid(self, symbol_name: str | None = None, depth: int = 2) -> str:
        """Generate Mermaid diagram of call graph.

        Args:
            symbol_name: Optional symbol to center the diagram on
            depth: Depth of calls to include

        Returns:
            Mermaid diagram code
        """
        if not self._built:
            self.build()

        lines = ["graph TD"]

        if symbol_name:
            # Center on a specific symbol
            nodes_to_include = set()

            # Find symbol
            center_key = None
            for key in self._nodes:
                if key == symbol_name or key.endswith(f".{symbol_name}"):
                    center_key = key
                    break

            if center_key:
                # Get callers and callees
                callers = self.get_callers(symbol_name, depth)
                callees = self.get_callees(symbol_name, depth)

                nodes_to_include.add(center_key)
                for c in callers:
                    nodes_to_include.add(c.qualified_name)
                for c in callees:
                    nodes_to_include.add(c.qualified_name)

                # Add edges
                for node_name in nodes_to_include:
                    node = self._nodes.get(node_name)
                    if not node:
                        continue

                    for edge in node.outgoing:
                        if edge.target in nodes_to_include:
                            source_id = self._sanitize_id(edge.source)
                            target_id = self._sanitize_id(edge.target)
                            source_label = edge.source.split(".")[-1]
                            target_label = edge.target.split(".")[-1]
                            lines.append(f"    {source_id}[{source_label}] --> {target_id}[{target_label}]")
        else:
            # Full graph (limited)
            for edge in self._edges[:100]:
                source_id = self._sanitize_id(edge.source)
                target_id = self._sanitize_id(edge.target)
                source_label = edge.source.split(".")[-1]
                target_label = edge.target.split(".")[-1]
                lines.append(f"    {source_id}[{source_label}] --> {target_id}[{target_label}]")

        return "\n".join(lines)

    def _sanitize_id(self, name: str) -> str:
        """Sanitize a name for use as Mermaid ID."""
        return name.replace(".", "_").replace("-", "_")


# MCP Tool Definitions
MCP_CALL_GRAPH_TOOLS = [
    {
        "name": "get_callers",
        "description": "Find all functions/methods that call a given symbol. Useful for understanding impact of changes.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the function/method to find callers for",
                },
                "max_depth": {
                    "type": "integer",
                    "description": "How many levels of callers to return (default: 1)",
                    "default": 1,
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
    },
    {
        "name": "get_callees",
        "description": "Find all functions/methods called by a given symbol. Useful for understanding dependencies.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Name of the function/method to find callees for",
                },
                "max_depth": {
                    "type": "integer",
                    "description": "How many levels of callees to return (default: 1)",
                    "default": 1,
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
    },
    {
        "name": "find_call_paths",
        "description": "Find all call paths between two functions. Useful for tracing execution flow.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "from_symbol": {
                    "type": "string",
                    "description": "Starting function name",
                },
                "to_symbol": {
                    "type": "string",
                    "description": "Target function name",
                },
                "max_depth": {
                    "type": "integer",
                    "description": "Maximum path length (default: 5)",
                    "default": 5,
                },
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
            "required": ["from_symbol", "to_symbol"],
        },
    },
    {
        "name": "call_graph_stats",
        "description": "Get statistics about the codebase call graph (most called functions, entry points, etc.)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "format": {
                    "type": "string",
                    "description": "Output format (markdown or json)",
                    "enum": ["markdown", "json"],
                    "default": "markdown",
                },
            },
        },
    },
    {
        "name": "call_graph_diagram",
        "description": "Generate a Mermaid diagram of the call graph, optionally centered on a symbol.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol_name": {
                    "type": "string",
                    "description": "Optional symbol to center the diagram on",
                },
                "depth": {
                    "type": "integer",
                    "description": "Depth of calls to include (default: 2)",
                    "default": 2,
                },
            },
        },
    },
]
