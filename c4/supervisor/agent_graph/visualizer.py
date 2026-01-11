"""GraphVisualizer - Mermaid diagram generation for AgentGraph.

This module provides visualization capabilities for the agent graph,
generating Mermaid diagrams that can be rendered in markdown viewers.

Features:
- Full graph visualization with subgraphs for each node type
- Filtered visualization (skills only, agents only, domains only)
- Path highlighting between nodes
- Edge styling based on edge type
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Literal

from c4.supervisor.agent_graph.graph import EdgeType, NodeType

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.graph import AgentGraph


# Mermaid styles for different node types
NODE_STYLES = {
    NodeType.SKILL: ":::skill",
    NodeType.AGENT: ":::agent",
    NodeType.DOMAIN: ":::domain",
}

# Mermaid arrow styles for different edge types
EDGE_STYLES = {
    EdgeType.HAS_SKILL: "-->",  # Solid arrow
    EdgeType.HANDS_OFF_TO: "-.->",  # Dotted arrow (handoff)
    EdgeType.PREFERS: "==>",  # Thick arrow (preference)
    EdgeType.TRIGGERS: "-->",  # Solid arrow
    EdgeType.REQUIRES: "-->",  # Solid arrow
    EdgeType.COMPLEMENTS: "<-->",  # Bidirectional
}

# Edge labels
EDGE_LABELS = {
    EdgeType.HAS_SKILL: "has",
    EdgeType.HANDS_OFF_TO: "hands off",
    EdgeType.PREFERS: "prefers",
    EdgeType.TRIGGERS: "triggers",
    EdgeType.REQUIRES: "requires",
    EdgeType.COMPLEMENTS: "complements",
}


FilterType = Literal["all", "skills", "agents", "domains"]


class GraphVisualizer:
    """Visualizer for AgentGraph that generates Mermaid diagrams.

    Example:
        >>> visualizer = GraphVisualizer(graph)
        >>> print(visualizer.to_mermaid())
        graph TD
            subgraph Skills
                python-coding[Python Coding]
                testing[Testing]
            end
            ...
    """

    def __init__(self, graph: AgentGraph) -> None:
        """Initialize the visualizer with an AgentGraph.

        Args:
            graph: The AgentGraph to visualize
        """
        self._graph = graph

    def to_mermaid(
        self,
        filter_type: FilterType = "all",
        include_labels: bool = True,
        direction: Literal["TD", "LR", "BT", "RL"] = "TD",
    ) -> str:
        """Generate Mermaid diagram code for the graph.

        Args:
            filter_type: Type of nodes to include:
                - "all": Include all nodes
                - "skills": Include only skill nodes
                - "agents": Include only agent nodes
                - "domains": Include only domain nodes
            include_labels: Whether to include edge labels
            direction: Graph direction (TD=top-down, LR=left-right, etc.)

        Returns:
            Mermaid diagram code as a string
        """
        lines: list[str] = []
        lines.append(f"graph {direction}")

        # Add style definitions
        lines.extend(self._generate_styles())

        # Get nodes and edges based on filter
        if filter_type == "all":
            lines.extend(self._generate_full_graph(include_labels))
        elif filter_type == "skills":
            lines.extend(self._generate_filtered_graph(NodeType.SKILL, include_labels))
        elif filter_type == "agents":
            lines.extend(self._generate_filtered_graph(NodeType.AGENT, include_labels))
        elif filter_type == "domains":
            lines.extend(
                self._generate_filtered_graph(NodeType.DOMAIN, include_labels)
            )

        return "\n".join(lines)

    def highlight_path(
        self,
        from_node: str,
        to_node: str,
        direction: Literal["TD", "LR", "BT", "RL"] = "TD",
    ) -> str:
        """Generate Mermaid diagram with a highlighted path.

        The path between the two nodes is highlighted using special styling.

        Args:
            from_node: Starting node ID
            to_node: Ending node ID
            direction: Graph direction

        Returns:
            Mermaid diagram code with highlighted path
        """
        # Get the path
        path = self._graph.get_path(from_node, to_node)

        lines: list[str] = []
        lines.append(f"graph {direction}")

        # Add style definitions including highlight style
        lines.extend(self._generate_styles())
        lines.append("    classDef highlight fill:#ff9,stroke:#f66,stroke-width:3px")

        # Generate the full graph
        lines.extend(self._generate_full_graph(include_labels=True))

        # Add highlight class to path nodes
        if path:
            path_nodes = " & ".join(path)
            lines.append(f"    class {path_nodes} highlight")

        return "\n".join(lines)

    def _generate_styles(self) -> list[str]:
        """Generate Mermaid class definitions for styling."""
        return [
            "    classDef skill fill:#e1f5fe,stroke:#0288d1,color:#01579b",
            "    classDef agent fill:#fff3e0,stroke:#ef6c00,color:#e65100",
            "    classDef domain fill:#f3e5f5,stroke:#7b1fa2,color:#4a148c",
        ]

    def _generate_full_graph(self, include_labels: bool) -> list[str]:
        """Generate Mermaid code for the full graph with subgraphs."""
        lines: list[str] = []

        # Skills subgraph
        skills = self._graph.skills
        if skills:
            lines.append("    subgraph Skills")
            for skill_id in skills:
                node = self._graph.get_node(skill_id)
                name = node.get("name", skill_id) if node else skill_id
                lines.append(f"        {skill_id}[{name}]:::skill")
            lines.append("    end")

        # Agents subgraph
        agents = self._graph.agents
        if agents:
            lines.append("    subgraph Agents")
            for agent_id in agents:
                node = self._graph.get_node(agent_id)
                name = node.get("name", agent_id) if node else agent_id
                lines.append(f"        {agent_id}[{name}]:::agent")
            lines.append("    end")

        # Domains subgraph
        domains = self._graph.domains
        if domains:
            lines.append("    subgraph Domains")
            for domain_id in domains:
                node = self._graph.get_node(domain_id)
                name = node.get("name", domain_id) if node else domain_id
                lines.append(f"        {domain_id}[{name}]:::domain")
            lines.append("    end")

        # Add edges
        lines.extend(self._generate_edges(include_labels))

        return lines

    def _generate_filtered_graph(
        self,
        node_type: NodeType,
        include_labels: bool,
    ) -> list[str]:
        """Generate Mermaid code for a filtered graph with only one node type."""
        lines: list[str] = []

        # Get nodes of the specified type
        if node_type == NodeType.SKILL:
            nodes = self._graph.skills
            style_class = "skill"
        elif node_type == NodeType.AGENT:
            nodes = self._graph.agents
            style_class = "agent"
        else:
            nodes = self._graph.domains
            style_class = "domain"

        # Add nodes
        for node_id in nodes:
            node = self._graph.get_node(node_id)
            name = node.get("name", node_id) if node else node_id
            lines.append(f"    {node_id}[{name}]:::{style_class}")

        # Add edges only between nodes of the same type
        for node_id in nodes:
            edges = self._graph.get_edges(node_id)
            for edge in edges:
                to_id = edge["to"]
                if to_id in nodes:
                    edge_type = edge.get("edge_type", EdgeType.HAS_SKILL)
                    arrow = EDGE_STYLES.get(edge_type, "-->")

                    if include_labels:
                        label = EDGE_LABELS.get(edge_type, "")
                        if label:
                            lines.append(f"    {node_id} {arrow}|{label}| {to_id}")
                        else:
                            lines.append(f"    {node_id} {arrow} {to_id}")
                    else:
                        lines.append(f"    {node_id} {arrow} {to_id}")

        return lines

    def _generate_edges(self, include_labels: bool) -> list[str]:
        """Generate Mermaid edge definitions."""
        lines: list[str] = []
        all_nodes = self._graph.get_all_nodes()

        for node_id in all_nodes:
            edges = self._graph.get_edges(node_id)
            for edge in edges:
                to_id = edge["to"]
                edge_type = edge.get("edge_type", EdgeType.HAS_SKILL)
                arrow = EDGE_STYLES.get(edge_type, "-->")

                if include_labels:
                    label = EDGE_LABELS.get(edge_type, "")
                    if label:
                        lines.append(f"    {node_id} {arrow}|{label}| {to_id}")
                    else:
                        lines.append(f"    {node_id} {arrow} {to_id}")
                else:
                    lines.append(f"    {node_id} {arrow} {to_id}")

        return lines


def to_mermaid(
    graph: AgentGraph,
    filter_type: FilterType = "all",
    include_labels: bool = True,
    direction: Literal["TD", "LR", "BT", "RL"] = "TD",
) -> str:
    """Generate Mermaid diagram code for an AgentGraph.

    This is a convenience function that creates a GraphVisualizer internally.

    Args:
        graph: The AgentGraph to visualize
        filter_type: Type of nodes to include
        include_labels: Whether to include edge labels
        direction: Graph direction

    Returns:
        Mermaid diagram code as a string

    Example:
        >>> mermaid_code = to_mermaid(graph)
        >>> print(mermaid_code)
        graph TD
            subgraph Skills
                python-coding[Python Coding]
            end
            ...
    """
    visualizer = GraphVisualizer(graph)
    return visualizer.to_mermaid(filter_type, include_labels, direction)


def highlight_path(
    graph: AgentGraph,
    from_node: str,
    to_node: str,
    direction: Literal["TD", "LR", "BT", "RL"] = "TD",
) -> str:
    """Generate Mermaid diagram with a highlighted path.

    This is a convenience function that creates a GraphVisualizer internally.

    Args:
        graph: The AgentGraph to visualize
        from_node: Starting node ID
        to_node: Ending node ID
        direction: Graph direction

    Returns:
        Mermaid diagram code with highlighted path
    """
    visualizer = GraphVisualizer(graph)
    return visualizer.highlight_path(from_node, to_node, direction)
