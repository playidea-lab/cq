"""DynamicChainBuilder - Task-aware agent chain construction.

This module provides dynamic chain building based on task requirements:
1. Keyword-to-role mapping: Detect required roles from task content
2. Role-aware chain building: Ensure required agents are included in the chain
3. Path optimization: Find efficient paths through the agent graph

Usage:
    >>> builder = DynamicChainBuilder(graph)
    >>> context = ChainBuildContext(
    ...     task=task,
    ...     required_roles=builder.detect_required_roles(task),
    ... )
    >>> chain = builder.build_chain("backend-architect", context)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.graph import AgentGraph
    from c4.supervisor.agent_graph.skill_matcher import TaskLike


# Default keyword-to-agent mapping for role detection
DEFAULT_ROLE_KEYWORDS: dict[str, list[str]] = {
    # Security-related keywords
    "security-auditor": [
        "security",
        "vulnerability",
        "auth",
        "authentication",
        "authorization",
        "xss",
        "csrf",
        "injection",
        "penetration",
        "pentest",
    ],
    # Payment/billing keywords
    "payment-integration": [
        "payment",
        "billing",
        "stripe",
        "paypal",
        "checkout",
        "transaction",
        "subscription",
    ],
    # Testing keywords
    "test-automator": [
        "test",
        "testing",
        "unittest",
        "pytest",
        "coverage",
        "tdd",
        "e2e",
        "integration test",
    ],
    # Review keywords
    "code-reviewer": [
        "review",
        "code review",
        "pr review",
        "refactor",
        "cleanup",
        "code quality",
    ],
    # Deployment keywords
    "deployment-engineer": [
        "deploy",
        "deployment",
        "production",
        "staging",
        "ci/cd",
        "pipeline",
        "kubernetes",
        "docker",
    ],
    # Performance keywords
    "performance-engineer": [
        "performance",
        "optimize",
        "optimization",
        "latency",
        "throughput",
        "benchmark",
        "profiling",
    ],
    # Documentation keywords
    "api-documenter": [
        "documentation",
        "docs",
        "api docs",
        "swagger",
        "openapi",
        "readme",
    ],
    # Debugging keywords
    "debugger": [
        "debug",
        "debugging",
        "bug",
        "fix bug",
        "error",
        "crash",
        "exception",
    ],
}


@dataclass
class ChainBuildContext:
    """Context for building an agent chain.

    Attributes:
        task: The task being processed (optional)
        required_roles: Agent IDs that must be included in the chain
        exclude_agents: Agent IDs to exclude from the chain
        max_length: Maximum chain length
    """

    task: TaskLike | None = None
    required_roles: set[str] = field(default_factory=set)
    exclude_agents: set[str] = field(default_factory=set)
    max_length: int = 10


class DynamicChainBuilder:
    """Builds agent chains dynamically based on task requirements.

    The builder analyzes task content to detect required roles (like security
    auditor for security-related tasks) and constructs chains that include
    these required agents while following the handoff graph.

    Example:
        >>> graph = AgentGraph()
        >>> # ... populate graph ...
        >>> builder = DynamicChainBuilder(graph)
        >>> task = TaskContext(title="Add payment feature with security review")
        >>> required = builder.detect_required_roles(task)
        >>> # required = {"payment-integration", "security-auditor"}
        >>> context = ChainBuildContext(task=task, required_roles=required)
        >>> chain = builder.build_chain("backend-architect", context)
        >>> # chain includes payment-integration and security-auditor
    """

    def __init__(
        self,
        graph: AgentGraph,
        role_keywords: dict[str, list[str]] | None = None,
    ) -> None:
        """Initialize the chain builder.

        Args:
            graph: AgentGraph for traversing agent relationships
            role_keywords: Optional custom keyword-to-agent mapping.
                          Defaults to DEFAULT_ROLE_KEYWORDS.
        """
        self._graph = graph
        self._role_keywords = DEFAULT_ROLE_KEYWORDS if role_keywords is None else role_keywords

    def detect_required_roles(self, task: TaskLike) -> set[str]:
        """Detect required roles from task content.

        Analyzes the task title, description, and DoD to identify
        keywords that indicate specific agent roles are needed.

        Args:
            task: Task to analyze

        Returns:
            Set of agent IDs that should be included in the chain
        """
        # Combine task text for searching
        text_parts = [
            getattr(task, "title", "") or "",
            getattr(task, "description", "") or "",
            getattr(task, "dod", "") or "",
        ]
        text = " ".join(text_parts).lower()

        required_roles: set[str] = set()

        for agent_id, keywords in self._role_keywords.items():
            for keyword in keywords:
                if keyword.lower() in text:
                    # Only add if agent exists in graph
                    if agent_id in self._graph.agents:
                        required_roles.add(agent_id)
                    break  # Found a match, no need to check more keywords

        return required_roles

    def build_chain(
        self,
        primary: str,
        context: ChainBuildContext | None = None,
    ) -> list[str]:
        """Build an agent chain starting from the primary agent.

        If required_roles are specified in the context, the chain will
        be constructed to include those agents. Otherwise, follows the
        standard handoff graph traversal.

        Args:
            primary: Starting agent ID
            context: Optional build context with requirements

        Returns:
            List of agent IDs forming the chain
        """
        if context is None:
            context = ChainBuildContext()

        # Detect required roles from task if not already specified
        if context.task and not context.required_roles:
            context.required_roles = self.detect_required_roles(context.task)

        # Build the chain
        chain: list[str] = [primary]
        visited: set[str] = {primary}
        remaining_required = context.required_roles - {primary}

        current = primary
        while len(chain) < context.max_length:
            # Get handoff targets
            targets = self._graph.find_handoff_targets(current)

            # Filter out excluded and visited agents
            candidates = [
                (agent_id, weight)
                for agent_id, weight in targets
                if agent_id not in visited and agent_id not in context.exclude_agents
            ]

            if not candidates:
                # No more candidates via handoff, try to add remaining required roles
                if remaining_required:
                    next_required = remaining_required.pop()
                    chain.append(next_required)
                    visited.add(next_required)
                    current = next_required
                    continue
                break

            # Prioritize required roles if they're in candidates
            next_agent = None
            for agent_id, _weight in candidates:
                if agent_id in remaining_required:
                    next_agent = agent_id
                    remaining_required.discard(agent_id)
                    break

            # If no required role found, take the highest-weight candidate
            if next_agent is None:
                next_agent = candidates[0][0]

            chain.append(next_agent)
            visited.add(next_agent)
            current = next_agent

        # Ensure any remaining required roles are added at the end
        for role in remaining_required:
            if len(chain) < context.max_length and role not in visited:
                chain.append(role)

        return chain

    def build_chain_with_path(
        self,
        primary: str,
        target: str,
        context: ChainBuildContext | None = None,
    ) -> list[str] | None:
        """Build a chain that reaches a specific target agent.

        Uses graph pathfinding to find the shortest path from primary
        to target agent through the handoff graph.

        Args:
            primary: Starting agent ID
            target: Target agent ID that must be in the chain
            context: Optional build context

        Returns:
            List of agent IDs forming the path, or None if no path exists
        """
        if context is None:
            context = ChainBuildContext()

        # First, find path to target
        path = self._graph.get_path(primary, target)

        if path is None:
            # No direct path, try building chain with target as required role
            ctx = ChainBuildContext(
                task=context.task,
                required_roles={target} | context.required_roles,
                exclude_agents=context.exclude_agents,
                max_length=context.max_length,
            )
            return self.build_chain(primary, ctx)

        return path

    def optimize_chain(
        self,
        chain: list[str],
        required_roles: set[str] | None = None,
        max_length: int = 10,
    ) -> list[str]:
        """Optimize an existing chain by removing unnecessary agents.

        Keeps the primary agent, any required roles, and agents that
        form critical handoff paths.

        Args:
            chain: Existing agent chain to optimize
            required_roles: Agent IDs that must be kept
            max_length: Maximum chain length

        Returns:
            Optimized chain
        """
        if len(chain) <= 1:
            return chain

        required = required_roles or set()
        optimized: list[str] = [chain[0]]  # Always keep primary

        for agent_id in chain[1:]:
            if len(optimized) >= max_length:
                break

            # Keep required roles
            if agent_id in required:
                optimized.append(agent_id)
                continue

            # Keep agents that have handoff from the previous agent
            prev_agent = optimized[-1]
            targets = self._graph.find_handoff_targets(prev_agent)
            target_ids = {t[0] for t in targets}

            if agent_id in target_ids:
                optimized.append(agent_id)

        return optimized

    @property
    def role_keywords(self) -> dict[str, list[str]]:
        """Get the role keyword mapping."""
        return self._role_keywords.copy()

    def add_role_keywords(self, agent_id: str, keywords: list[str]) -> None:
        """Add keywords for a role.

        Args:
            agent_id: Agent ID for the role
            keywords: Keywords that trigger this role
        """
        if agent_id in self._role_keywords:
            existing = set(self._role_keywords[agent_id])
            self._role_keywords[agent_id] = list(existing | set(keywords))
        else:
            self._role_keywords[agent_id] = keywords
