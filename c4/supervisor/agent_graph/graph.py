"""AgentGraph - NetworkX-based graph for agent routing.

This module provides a graph-based representation of the agent ecosystem:
- Nodes: Skills, Agents, Domains
- Edges: Relationships between nodes (has_skill, hands_off_to, prefers, etc.)

The graph is used for intelligent routing decisions based on:
- Agent capabilities (skills)
- Agent relationships (handoffs)
- Domain preferences (workflows)
"""

from __future__ import annotations

from enum import Enum
from typing import TYPE_CHECKING, Any

import networkx as nx

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.models import (
        AgentDefinition,
        DomainDefinition,
        SkillDefinition,
    )


class NodeType(str, Enum):
    """Types of nodes in the agent graph."""

    SKILL = "skill"
    AGENT = "agent"
    DOMAIN = "domain"


class EdgeType(str, Enum):
    """Types of edges (relationships) in the agent graph."""

    # Agent -> Skill: Agent possesses this skill
    HAS_SKILL = "has_skill"

    # Agent -> Agent: Agent hands off work to another agent
    HANDS_OFF_TO = "hands_off_to"

    # Domain -> Agent: Domain prefers this agent
    PREFERS = "prefers"

    # Skill -> Agent: Skill triggers this agent (via skill matching)
    TRIGGERS = "triggers"

    # Skill -> Skill: Skill requires another skill as prerequisite
    REQUIRES = "requires"

    # Skill -> Skill: Skill works well with another skill
    COMPLEMENTS = "complements"


class AgentGraph:
    """NetworkX-based graph for agent routing.

    The graph stores:
    - Skill nodes: Atomic capabilities
    - Agent nodes: Personas with skills and relationships
    - Domain nodes: Problem areas with workflows

    Edges represent relationships:
    - HAS_SKILL: Agent -> Skill (agent possesses skill)
    - HANDS_OFF_TO: Agent -> Agent (workflow handoff)
    - PREFERS: Domain -> Agent (domain preference)
    - TRIGGERS: Skill -> Agent (skill-based routing)
    - REQUIRES: Skill -> Skill (prerequisite)
    - COMPLEMENTS: Skill -> Skill (complementary)

    Example:
        >>> graph = AgentGraph()
        >>> graph.add_skill(skill_def)
        >>> graph.add_agent(agent_def)
        >>> graph.add_domain(domain_def)
        >>> graph.get_edges("backend-dev", EdgeType.HAS_SKILL)
        [{"to": "python-coding", "edge_type": EdgeType.HAS_SKILL, "is_primary": True}]
    """

    def __init__(self) -> None:
        """Initialize an empty agent graph."""
        self._graph: nx.DiGraph[str] = nx.DiGraph()

    def add_skill(self, skill: SkillDefinition) -> None:
        """Add a skill node to the graph.

        Creates the skill node and automatically creates:
        - REQUIRES edges to prerequisite skills (if they exist in graph)
        - COMPLEMENTS edges to complementary skills (if they exist in graph)

        Args:
            skill: SkillDefinition to add
        """
        skill_data = skill.skill
        skill_id = skill_data.id

        # Add the skill node
        self._graph.add_node(
            skill_id,
            type=NodeType.SKILL,
            name=skill_data.name,
            description=skill_data.description,
            definition=skill,
        )

        # Create REQUIRES edges for prerequisites
        if skill_data.prerequisites:
            for prereq_id in skill_data.prerequisites:
                if self._graph.has_node(prereq_id):
                    self.add_edge(skill_id, prereq_id, EdgeType.REQUIRES)

        # Create COMPLEMENTS edges for complementary skills
        if skill_data.complementary_skills:
            for comp_id in skill_data.complementary_skills:
                if self._graph.has_node(comp_id):
                    self.add_edge(skill_id, comp_id, EdgeType.COMPLEMENTS)

    def add_agent(self, agent: AgentDefinition) -> None:
        """Add an agent node to the graph.

        Creates the agent node and automatically creates:
        - HAS_SKILL edges to primary and secondary skills (if they exist in graph)
        - HANDS_OFF_TO edges to other agents (if they exist in graph)

        Args:
            agent: AgentDefinition to add
        """
        agent_data = agent.agent
        agent_id = agent_data.id

        # Add the agent node
        self._graph.add_node(
            agent_id,
            type=NodeType.AGENT,
            name=agent_data.name,
            persona=agent_data.persona,
            definition=agent,
        )

        # Create HAS_SKILL edges for primary skills
        for skill_id in agent_data.skills.primary:
            if self._graph.has_node(skill_id):
                self.add_edge(agent_id, skill_id, EdgeType.HAS_SKILL, is_primary=True)

        # Create HAS_SKILL edges for secondary skills
        if agent_data.skills.secondary:
            for skill_id in agent_data.skills.secondary:
                if self._graph.has_node(skill_id):
                    self.add_edge(agent_id, skill_id, EdgeType.HAS_SKILL, is_primary=False)

        # Create HANDS_OFF_TO edges
        if agent_data.relationships.hands_off_to:
            for handoff in agent_data.relationships.hands_off_to:
                target_agent_id = handoff.agent
                if self._graph.has_node(target_agent_id):
                    self.add_edge(
                        agent_id,
                        target_agent_id,
                        EdgeType.HANDS_OFF_TO,
                        when=handoff.when,
                        passes=handoff.passes,
                        weight=handoff.weight,
                    )

    def add_domain(self, domain: DomainDefinition) -> None:
        """Add a domain node to the graph.

        Creates the domain node and automatically creates:
        - PREFERS edges to agents referenced in workflow prefer_agent (if they exist)

        Args:
            domain: DomainDefinition to add
        """
        domain_data = domain.domain
        domain_id = domain_data.id

        # Add the domain node
        self._graph.add_node(
            domain_id,
            type=NodeType.DOMAIN,
            name=domain_data.name,
            description=domain_data.description,
            definition=domain,
        )

        # Create PREFERS edges from workflow steps
        for step in domain_data.workflow:
            if step.select.prefer_agent:
                agent_id = step.select.prefer_agent
                if self._graph.has_node(agent_id):
                    self.add_edge(
                        domain_id,
                        agent_id,
                        EdgeType.PREFERS,
                        step=step.step,
                        role=step.role,
                        purpose=step.purpose,
                    )

    def add_edge(
        self,
        from_id: str,
        to_id: str,
        edge_type: EdgeType,
        **attrs: Any,
    ) -> None:
        """Add an edge between two nodes.

        Args:
            from_id: Source node ID
            to_id: Target node ID
            edge_type: Type of the edge
            **attrs: Additional edge attributes
        """
        self._graph.add_edge(from_id, to_id, edge_type=edge_type, **attrs)

    def get_node(self, node_id: str) -> dict[str, Any] | None:
        """Get node attributes by ID.

        Args:
            node_id: The node ID to look up

        Returns:
            Dictionary of node attributes, or None if node doesn't exist
        """
        if not self._graph.has_node(node_id):
            return None
        return dict(self._graph.nodes[node_id])

    def get_edges(
        self,
        node_id: str,
        edge_type: EdgeType | None = None,
    ) -> list[dict[str, Any]]:
        """Get outgoing edges from a node.

        Args:
            node_id: The source node ID
            edge_type: Optional filter by edge type

        Returns:
            List of edge dictionaries with 'to' and 'edge_type' keys plus any attributes
        """
        if not self._graph.has_node(node_id):
            return []

        edges = []
        for _, to_id, data in self._graph.out_edges(node_id, data=True):
            if edge_type is None or data.get("edge_type") == edge_type:
                edge_info = {"to": to_id, **data}
                edges.append(edge_info)

        return edges

    def get_all_nodes(self, node_type: NodeType | None = None) -> list[str]:
        """Get all node IDs, optionally filtered by type.

        Args:
            node_type: Optional filter by node type

        Returns:
            List of node IDs
        """
        if node_type is None:
            return list(self._graph.nodes())

        return [
            node_id
            for node_id, data in self._graph.nodes(data=True)
            if data.get("type") == node_type
        ]

    @property
    def skills(self) -> list[str]:
        """Get all skill node IDs."""
        return self.get_all_nodes(NodeType.SKILL)

    @property
    def agents(self) -> list[str]:
        """Get all agent node IDs."""
        return self.get_all_nodes(NodeType.AGENT)

    @property
    def domains(self) -> list[str]:
        """Get all domain node IDs."""
        return self.get_all_nodes(NodeType.DOMAIN)

    # =========================================================================
    # Query Methods
    # =========================================================================

    def find_agents_with_skill(self, skill_id: str) -> list[str]:
        """Find all agents that have a specific skill.

        Traverses HAS_SKILL edges in reverse direction to find agents
        that possess the given skill.

        Args:
            skill_id: The skill ID to search for

        Returns:
            List of agent IDs that have the skill
        """
        if not self._graph.has_node(skill_id):
            return []

        agents = []
        # Look for incoming HAS_SKILL edges to this skill
        for from_id, _, data in self._graph.in_edges(skill_id, data=True):
            if data.get("edge_type") == EdgeType.HAS_SKILL:
                agents.append(from_id)

        return agents

    def find_skills_for_agent(self, agent_id: str) -> list[str]:
        """Find all skills that an agent possesses.

        Traverses HAS_SKILL edges in forward direction from the agent.

        Args:
            agent_id: The agent ID to search for

        Returns:
            List of skill IDs that the agent has
        """
        if not self._graph.has_node(agent_id):
            return []

        skills = []
        for _, to_id, data in self._graph.out_edges(agent_id, data=True):
            if data.get("edge_type") == EdgeType.HAS_SKILL:
                skills.append(to_id)

        return skills

    def find_handoff_targets(self, agent_id: str) -> list[tuple[str, float]]:
        """Find agents that this agent can hand off work to.

        Traverses HANDS_OFF_TO edges from the agent and returns targets
        sorted by weight (highest first).

        Args:
            agent_id: The agent ID to search from

        Returns:
            List of (agent_id, weight) tuples, sorted by weight descending
        """
        if not self._graph.has_node(agent_id):
            return []

        targets = []
        for _, to_id, data in self._graph.out_edges(agent_id, data=True):
            if data.get("edge_type") == EdgeType.HANDS_OFF_TO:
                weight = data.get("weight", 0.5)
                targets.append((to_id, weight))

        # Sort by weight descending
        targets.sort(key=lambda x: x[1], reverse=True)
        return targets

    def find_agents_for_domain(self, domain_id: str) -> dict[str, Any]:
        """Find agent information for a domain.

        Traverses PREFERS edges from the domain to find preferred agents.

        Args:
            domain_id: The domain ID to search for

        Returns:
            Dictionary with:
            - primary: First preferred agent (or None)
            - chain: List of all preferred agents in order
            - domain: The domain ID
        """
        result: dict[str, Any] = {
            "primary": None,
            "chain": [],
            "domain": domain_id,
        }

        if not self._graph.has_node(domain_id):
            return result

        # Get PREFERS edges and sort by step if available
        prefers_edges = []
        for _, to_id, data in self._graph.out_edges(domain_id, data=True):
            if data.get("edge_type") == EdgeType.PREFERS:
                step = data.get("step", 0)
                prefers_edges.append((step, to_id))

        # Sort by step number
        prefers_edges.sort(key=lambda x: x[0])
        chain = [agent_id for _, agent_id in prefers_edges]

        result["chain"] = chain
        result["primary"] = chain[0] if chain else None

        return result

    def get_path(self, from_agent: str, to_agent: str) -> list[str] | None:
        """Find the shortest handoff path between two agents.

        Uses only HANDS_OFF_TO edges to find a path from source to target.

        Args:
            from_agent: Source agent ID
            to_agent: Target agent ID

        Returns:
            List of agent IDs forming the path, or None if no path exists
        """
        if not self._graph.has_node(from_agent):
            return None
        if not self._graph.has_node(to_agent):
            return None

        # Same node case
        if from_agent == to_agent:
            return [from_agent]

        # Create a view of the graph with only HANDS_OFF_TO edges
        handoff_edges = [
            (u, v)
            for u, v, data in self._graph.edges(data=True)
            if data.get("edge_type") == EdgeType.HANDS_OFF_TO
        ]

        # Create a subgraph with only handoff edges
        handoff_graph: nx.DiGraph[str] = nx.DiGraph()
        handoff_graph.add_edges_from(handoff_edges)

        try:
            return nx.shortest_path(handoff_graph, from_agent, to_agent)
        except nx.NetworkXNoPath:
            return None
        except nx.NodeNotFound:
            return None

    def find_triggering_agents(self, skill_id: str) -> list[str]:
        """Find agents that a skill triggers.

        Traverses TRIGGERS edges from the skill to find agents.

        Args:
            skill_id: The skill ID to search from

        Returns:
            List of agent IDs that the skill triggers
        """
        if not self._graph.has_node(skill_id):
            return []

        agents = []
        for _, to_id, data in self._graph.out_edges(skill_id, data=True):
            if data.get("edge_type") == EdgeType.TRIGGERS:
                agents.append(to_id)

        return agents

    # =========================================================================
    # Chain Builder
    # =========================================================================

    def build_chain(
        self,
        primary_agent: str,
        max_chain_length: int = 10,
        min_weight: float = 0.0,
    ) -> list[str]:
        """Build an agent chain starting from the primary agent.

        Follows HANDS_OFF_TO edges in order of weight (highest first),
        preventing cycles and respecting the maximum chain length.

        Args:
            primary_agent: The starting agent ID
            max_chain_length: Maximum number of agents in the chain (default 10)
            min_weight: Minimum weight threshold for following edges (default 0.0)

        Returns:
            List of agent IDs forming the chain, or empty list if agent doesn't exist

        Example:
            >>> chain = graph.build_chain("architect", max_chain_length=5)
            ["architect", "backend-dev", "test-automator", "code-reviewer"]
        """
        if not self._graph.has_node(primary_agent):
            return []

        chain: list[str] = []
        visited: set[str] = set()
        current = primary_agent

        while len(chain) < max_chain_length:
            # Add current agent to chain
            chain.append(current)
            visited.add(current)

            # Find handoff targets sorted by weight
            targets = self.find_handoff_targets(current)

            # Filter by min_weight and already visited
            next_agent = None
            for target_id, weight in targets:
                if weight >= min_weight and target_id not in visited:
                    next_agent = target_id
                    break  # Take highest weight valid target

            if next_agent is None:
                break  # No more valid handoffs

            current = next_agent

        return chain
