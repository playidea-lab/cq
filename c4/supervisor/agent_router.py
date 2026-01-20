"""Agent Routing System - Domain-based agent selection and chaining.

.. deprecated::
    This module is deprecated in favor of c4.supervisor.agent_graph.router.GraphRouter.
    Set C4_USE_GRAPH_ROUTER=true (default) to use the new GraphRouter with:
    - Skill-based agent selection
    - Rule engine with overrides and chain extensions
    - Dynamic chain building based on task keywords
    - Graph-based handoff relationships

This module provides automatic agent selection based on project domain,
with support for agent chaining (sequential execution of multiple agents).

Usage:
    # Get recommended agent chain for a domain
    config = get_recommended_agent("web-frontend")
    print(config.primary)  # "frontend-developer"
    print(config.chain)    # ["frontend-developer", "test-automator", "code-reviewer"]

    # Get agent for specific task type (future)
    agent = get_agent_for_task_type("api-design", "web-backend")

    # Using AgentRouter class with custom config
    from c4.models.config import AgentConfig
    router = AgentRouter(config=my_agent_config)
    agent = router.get_recommended_agent("my-custom-domain")

Note:
    Consider migrating to GraphRouter for advanced routing features.
"""

from __future__ import annotations

import warnings
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from c4.discovery.models import Domain

if TYPE_CHECKING:
    from c4.models.config import AgentConfig


@dataclass
class AgentChainConfig:
    """Configuration for agent chain execution.

    Attributes:
        primary: The main agent for this domain (first in chain)
        chain: Ordered list of agents to execute sequentially
        description: Human-readable description of this agent combination
        handoff_instructions: Instructions for context passing between agents
    """

    primary: str
    chain: list[str] = field(default_factory=list)
    description: str = ""
    handoff_instructions: str = ""

    def __post_init__(self):
        """Ensure primary is first in chain if chain is provided."""
        if self.chain and self.primary not in self.chain:
            self.chain = [self.primary] + self.chain
        elif not self.chain:
            self.chain = [self.primary]


# =============================================================================
# Domain → Agent Chain Mapping
# =============================================================================

DOMAIN_AGENT_MAP: dict[str, AgentChainConfig] = {
    "web-frontend": AgentChainConfig(
        primary="frontend-developer",
        chain=["frontend-developer", "test-automator", "code-reviewer"],
        description="React/Vue/CSS, component development, accessibility",
        handoff_instructions=(
            "Pass component specifications and test coverage requirements. "
            "Ensure accessibility compliance (WCAG) and responsive design."
        ),
    ),
    "web-backend": AgentChainConfig(
        primary="backend-architect",
        chain=["backend-architect", "python-pro", "test-automator", "code-reviewer"],
        description="API design, database schemas, Python/Node implementation",
        handoff_instructions=(
            "Pass API specifications and database schema changes. "
            "Include error handling patterns and validation requirements."
        ),
    ),
    "fullstack": AgentChainConfig(
        primary="backend-architect",
        chain=[
            "backend-architect",
            "frontend-developer",
            "test-automator",
            "code-reviewer",
        ],
        description="Full-stack: API + Frontend integration",
        handoff_instructions=(
            "Pass API contract to frontend developer. "
            "Ensure type consistency between backend and frontend."
        ),
    ),
    "ml-dl": AgentChainConfig(
        primary="ml-engineer",
        chain=["ml-engineer", "python-pro", "test-automator"],
        description="ML pipelines, model training, data processing, PyTorch/TensorFlow",
        handoff_instructions=(
            "Pass model specifications, training configurations, and evaluation metrics. "
            "Include data preprocessing requirements and reproducibility constraints."
        ),
    ),
    "mobile-app": AgentChainConfig(
        primary="mobile-developer",
        chain=["mobile-developer", "test-automator", "code-reviewer"],
        description="React Native, Flutter, iOS/Android native integrations",
        handoff_instructions=(
            "Pass platform-specific requirements and UI/UX guidelines. "
            "Consider offline capabilities and push notification handling."
        ),
    ),
    "infra": AgentChainConfig(
        primary="cloud-architect",
        chain=["cloud-architect", "deployment-engineer"],
        description="Terraform, AWS/GCP/Azure, infrastructure as code",
        handoff_instructions=(
            "Pass infrastructure requirements, cost constraints, and compliance needs. "
            "Ensure state management and drift detection are configured."
        ),
    ),
    "library": AgentChainConfig(
        primary="python-pro",
        chain=["python-pro", "api-documenter", "test-automator", "code-reviewer"],
        description="Library/package development, public API design, documentation",
        handoff_instructions=(
            "Pass public API design and backwards compatibility requirements. "
            "Ensure comprehensive docstrings and usage examples."
        ),
    ),
    "firmware": AgentChainConfig(
        primary="general-purpose",
        chain=["general-purpose", "test-automator"],
        description="Embedded systems, low-level code, hardware interfaces",
        handoff_instructions=(
            "Pass hardware constraints, timing requirements, and memory limitations. "
            "Consider real-time constraints and interrupt handling."
        ),
    ),
    "unknown": AgentChainConfig(
        primary="general-purpose",
        chain=["general-purpose", "code-reviewer"],
        description="General purpose for undetected or mixed domains",
        handoff_instructions=(
            "Analyze the task requirements and apply best practices. "
            "Request clarification if domain-specific knowledge is needed."
        ),
    ),
    # Additional domains for specialized workflows
    "data-science": AgentChainConfig(
        primary="data-scientist",
        chain=["data-scientist", "python-pro", "test-automator"],
        description="Data analysis, visualization, ML experiments, Jupyter notebooks",
        handoff_instructions=(
            "Pass dataset specifications, analysis requirements, and evaluation metrics. "
            "Include data preprocessing steps and reproducibility constraints."
        ),
    ),
    "devops": AgentChainConfig(
        primary="deployment-engineer",
        chain=["deployment-engineer", "cloud-architect", "security-auditor"],
        description="CI/CD, monitoring, infrastructure automation, Docker/K8s",
        handoff_instructions=(
            "Pass deployment environment, infrastructure requirements, and security checklist. "
            "Include rollback procedures and monitoring configurations."
        ),
    ),
    "api": AgentChainConfig(
        primary="backend-architect",
        chain=["backend-architect", "api-documenter", "test-automator"],
        description="REST/GraphQL API design, documentation, client SDKs",
        handoff_instructions=(
            "Pass API specifications, authentication methods, and versioning strategy. "
            "Include rate limiting and error handling patterns."
        ),
    ),
}


# =============================================================================
# Task Type → Agent Overrides (for specific task patterns)
# =============================================================================

TASK_TYPE_AGENT_OVERRIDES: dict[str, str] = {
    # Debugging tasks
    "debug": "debugger",
    "debugging": "debugger",
    "fix-bug": "debugger",
    # Performance tasks
    "performance": "performance-engineer",
    "optimization": "performance-engineer",
    "profiling": "performance-engineer",
    # Security tasks
    "security": "security-auditor",
    "vulnerability": "security-auditor",
    "audit": "security-auditor",
    # Database tasks
    "database": "database-optimizer",
    "query": "database-optimizer",
    "migration": "database-optimizer",
    # Documentation tasks
    "docs": "api-documenter",
    "documentation": "api-documenter",
    "readme": "api-documenter",
    # Code review tasks
    "review": "code-reviewer",
    "code-review": "code-reviewer",
    # Refactoring tasks
    "refactor": "code-refactorer",
    "cleanup": "code-refactorer",
    "restructure": "code-refactorer",
    # Testing tasks
    "test": "test-automator",
    "testing": "test-automator",
    "coverage": "test-automator",
    # DevOps tasks
    "deploy": "deployment-engineer",
    "ci-cd": "deployment-engineer",
    "pipeline": "deployment-engineer",
    # GraphQL tasks
    "graphql": "graphql-architect",
    "schema": "graphql-architect",
    # Payment tasks
    "payment": "payment-integration",
    "stripe": "payment-integration",
    "billing": "payment-integration",
    # Data tasks
    "data-pipeline": "data-engineer",
    "etl": "data-engineer",
    "analytics": "data-scientist",
    # Additional task type overrides
    "api-design": "backend-architect",
    "data-analysis": "data-scientist",
    "monitoring": "devops-troubleshooter",
    "incident": "incident-responder",
    "infra-setup": "cloud-architect",
    "notebook": "data-scientist",
}


# =============================================================================
# Public API Functions
# =============================================================================


def get_recommended_agent(domain: str | Domain | None) -> AgentChainConfig:
    """
    Get recommended agent chain configuration for a domain.

    .. deprecated::
        Use c4.supervisor.agent_graph.GraphRouter.get_recommended_agent() instead.
        Set C4_USE_GRAPH_ROUTER=true (default) for advanced routing features.

    Args:
        domain: Domain string or Domain enum, or None for unknown

    Returns:
        AgentChainConfig with primary agent and chain

    Example:
        >>> config = get_recommended_agent("web-frontend")
        >>> config.primary
        'frontend-developer'
        >>> config.chain
        ['frontend-developer', 'test-automator', 'code-reviewer']
    """
    warnings.warn(
        "get_recommended_agent is deprecated. Use GraphRouter.get_recommended_agent() "
        "with C4_USE_GRAPH_ROUTER=true (default) for advanced routing.",
        DeprecationWarning,
        stacklevel=2,
    )
    if domain is None:
        return DOMAIN_AGENT_MAP["unknown"]

    domain_str = domain.value if isinstance(domain, Domain) else domain
    domain_str = domain_str.lower().replace("_", "-")

    return DOMAIN_AGENT_MAP.get(domain_str, DOMAIN_AGENT_MAP["unknown"])


def get_agent_for_task_type(
    task_type: str | None,
    domain: str | Domain | None = None,
) -> str:
    """
    Get specific agent for a task type, with domain fallback.

    Task type overrides take precedence over domain defaults.
    This allows specialized agents for specific task patterns.

    Args:
        task_type: Type of task (e.g., "debug", "security", "refactor")
        domain: Fallback domain if no task type match

    Returns:
        Agent name string

    Example:
        >>> get_agent_for_task_type("debug", "web-backend")
        'debugger'
        >>> get_agent_for_task_type(None, "web-backend")
        'backend-architect'
    """
    # Check task type overrides first
    if task_type:
        task_type_lower = task_type.lower().replace("_", "-")
        if task_type_lower in TASK_TYPE_AGENT_OVERRIDES:
            return TASK_TYPE_AGENT_OVERRIDES[task_type_lower]

    # Fall back to domain primary agent
    config = get_recommended_agent(domain)
    return config.primary


def get_all_domains() -> list[str]:
    """Get list of all supported domains."""
    return list(DOMAIN_AGENT_MAP.keys())


def get_chain_for_domain(domain: str | Domain | None) -> list[str]:
    """
    Get just the agent chain list for a domain.

    Args:
        domain: Domain string or Domain enum

    Returns:
        List of agent names in execution order
    """
    config = get_recommended_agent(domain)
    return config.chain


def get_handoff_instructions(domain: str | Domain | None) -> str:
    """
    Get handoff instructions for a domain's agent chain.

    Args:
        domain: Domain string or Domain enum

    Returns:
        Instructions string for context passing between agents
    """
    config = get_recommended_agent(domain)
    return config.handoff_instructions


# =============================================================================
# Agent Chain Execution Helpers
# =============================================================================


@dataclass
class AgentHandoff:
    """Context to pass between agents in a chain.

    Attributes:
        from_agent: Agent that completed work
        to_agent: Agent receiving handoff
        summary: Brief summary of completed work
        files_modified: List of files changed
        next_steps: Suggested actions for next agent
        warnings: Any issues or concerns to note
    """

    from_agent: str
    to_agent: str
    summary: str
    files_modified: list[str] = field(default_factory=list)
    next_steps: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)

    def to_prompt(self) -> str:
        """Convert handoff to prompt text for next agent."""
        lines = [
            f"## Handoff from {self.from_agent}",
            "",
            f"**Summary:** {self.summary}",
            "",
        ]

        if self.files_modified:
            lines.append("**Files Modified:**")
            for f in self.files_modified:
                lines.append(f"- {f}")
            lines.append("")

        if self.next_steps:
            lines.append("**Your Tasks:**")
            for step in self.next_steps:
                lines.append(f"- {step}")
            lines.append("")

        if self.warnings:
            lines.append("**⚠️ Notes:**")
            for warning in self.warnings:
                lines.append(f"- {warning}")
            lines.append("")

        return "\n".join(lines)


def build_chain_prompt(
    task_title: str,
    task_dod: str,
    agent_index: int,
    agent_chain: list[str],
    handoff: AgentHandoff | None = None,
    handoff_instructions: str = "",
) -> str:
    """
    Build a prompt for an agent in the chain.

    Args:
        task_title: Title of the task
        task_dod: Definition of Done
        agent_index: Position in chain (0-based)
        agent_chain: Full chain list
        handoff: Handoff context from previous agent
        handoff_instructions: Domain-specific handoff guidance

    Returns:
        Formatted prompt string for the agent
    """
    agent = agent_chain[agent_index]
    is_first = agent_index == 0
    is_last = agent_index == len(agent_chain) - 1

    lines = [
        f"# Task: {task_title}",
        "",
        f"**Definition of Done:** {task_dod}",
        "",
    ]

    if is_first:
        lines.extend(
            [
                "## Your Role",
                f"You are the **primary implementer** ({agent}) for this task.",
                "Implement the core functionality according to the DoD.",
                "",
            ]
        )
    elif agent == "code-reviewer":
        lines.extend(
            [
                "## Your Role",
                "You are the **code reviewer** for this task.",
                "Review the implementation for:",
                "- Code quality and best practices",
                "- Test coverage",
                "- Security concerns",
                "- Performance issues",
                "",
            ]
        )
    elif agent == "test-automator":
        lines.extend(
            [
                "## Your Role",
                "You are the **test engineer** for this task.",
                "Your responsibilities:",
                "- Write unit tests for new functionality",
                "- Add integration tests if applicable",
                "- Ensure edge cases are covered",
                "",
            ]
        )
    else:
        lines.extend(
            [
                "## Your Role",
                f"You are the **{agent}** in this chain.",
                "Continue building on previous work.",
                "",
            ]
        )

    if handoff:
        lines.append(handoff.to_prompt())

    if handoff_instructions and not is_first:
        lines.extend(
            [
                "## Handoff Guidelines",
                handoff_instructions,
                "",
            ]
        )

    if not is_last:
        next_agent = agent_chain[agent_index + 1]
        lines.extend(
            [
                "## Next Steps",
                f"After completing your work, the **{next_agent}** will continue.",
                "Prepare a clear handoff summary.",
                "",
            ]
        )

    return "\n".join(lines)


# =============================================================================
# AgentRouter Class - Extensible Agent Configuration
# =============================================================================


class AgentRouter:
    """Extensible agent router with YAML-based configuration support.

    .. deprecated::
        Use c4.supervisor.agent_graph.GraphRouter instead for advanced features:
        - Skill-based agent selection
        - Rule engine with overrides and chain extensions
        - Dynamic chain building based on task keywords
        - Graph-based handoff relationships

        Set C4_USE_GRAPH_ROUTER=true (default) to use GraphRouter.

    Merges user-defined agent configurations (from config.yaml) with
    built-in defaults, allowing project-specific customization.

    Usage:
        # With custom config from config.yaml
        from c4.models.config import AgentConfig
        config = AgentConfig(
            chains={"my-domain": AgentChainDef(primary="my-agent", chain=["my-agent", "reviewer"])},
            task_overrides={"my-task": "my-agent"},
        )
        router = AgentRouter(config=config)
        agent = router.get_recommended_agent("my-domain")  # "my-agent"

        # Without config - uses defaults
        router = AgentRouter()
        agent = router.get_recommended_agent("web-frontend")  # "frontend-developer"
    """

    def __init__(self, config: AgentConfig | None = None):
        """Initialize router with optional custom configuration.

        .. deprecated::
            Consider using GraphRouter instead for advanced features.

        Args:
            config: AgentConfig from config.yaml. If None, uses built-in defaults.
        """
        warnings.warn(
            "AgentRouter is deprecated. Use GraphRouter with C4_USE_GRAPH_ROUTER=true (default) "
            "for skill-based matching, rule engine, and dynamic chain building.",
            DeprecationWarning,
            stacklevel=2,
        )
        self._config = config
        self._merged_chains: dict[str, AgentChainConfig] | None = None
        self._merged_overrides: dict[str, str] | None = None

    @property
    def config(self) -> AgentConfig | None:
        """Get the agent configuration."""
        return self._config

    @property
    def merged_chains(self) -> dict[str, AgentChainConfig]:
        """Get merged domain-agent chains (lazy loaded)."""
        if self._merged_chains is None:
            self._merged_chains = self._merge_chains()
        return self._merged_chains

    @property
    def merged_overrides(self) -> dict[str, str]:
        """Get merged task type overrides (lazy loaded)."""
        if self._merged_overrides is None:
            self._merged_overrides = self._merge_overrides()
        return self._merged_overrides

    def _merge_chains(self) -> dict[str, AgentChainConfig]:
        """Merge built-in defaults with user configuration.

        User configuration takes precedence over defaults.
        """
        # Start with built-in defaults
        result = dict(DOMAIN_AGENT_MAP)

        if self._config is None:
            return result

        # Import here to avoid circular dependency
        from c4.models.config import AgentChainDef

        # Override with user config
        for domain, chain_def in self._config.chains.items():
            if isinstance(chain_def, dict):
                chain_def = AgentChainDef(**chain_def)

            # Build chain list - ensure primary is first if not in chain
            chain = chain_def.chain if chain_def.chain else [chain_def.primary]
            if chain_def.primary not in chain:
                chain = [chain_def.primary] + chain

            result[domain] = AgentChainConfig(
                primary=chain_def.primary,
                chain=chain,
                description=f"Custom agent chain for {domain}",
                handoff_instructions=chain_def.handoff,
            )

        return result

    def _merge_overrides(self) -> dict[str, str]:
        """Merge built-in task type overrides with user configuration."""
        result = dict(TASK_TYPE_AGENT_OVERRIDES)

        if self._config is None:
            return result

        # User overrides take precedence
        result.update(self._config.task_overrides)
        return result

    def get_recommended_agent(self, domain: str | Domain | None) -> AgentChainConfig:
        """Get recommended agent chain configuration for a domain.

        Args:
            domain: Domain string or Domain enum, or None for unknown

        Returns:
            AgentChainConfig with primary agent and chain
        """
        if domain is None:
            fallback = self._get_fallback_domain()
            return self.merged_chains.get(fallback, DOMAIN_AGENT_MAP["unknown"])

        domain_str = domain.value if isinstance(domain, Domain) else domain
        domain_str = domain_str.lower().replace("_", "-")

        fallback = self._get_fallback_domain()
        return self.merged_chains.get(
            domain_str, self.merged_chains.get(fallback, DOMAIN_AGENT_MAP["unknown"])
        )

    def get_agent_for_task_type(
        self,
        task_type: str | None,
        domain: str | Domain | None = None,
    ) -> str:
        """Get specific agent for a task type, with domain fallback.

        Args:
            task_type: Type of task (e.g., "debug", "security", "refactor")
            domain: Fallback domain if no task type match

        Returns:
            Agent name string
        """
        # Check task type overrides first
        if task_type:
            task_type_lower = task_type.lower().replace("_", "-")
            if task_type_lower in self.merged_overrides:
                return self.merged_overrides[task_type_lower]

        # Fall back to domain primary agent
        config = self.get_recommended_agent(domain)
        return config.primary

    def get_chain_for_domain(self, domain: str | Domain | None) -> list[str]:
        """Get agent chain list for a domain."""
        config = self.get_recommended_agent(domain)
        return config.chain

    def get_handoff_instructions(self, domain: str | Domain | None) -> str:
        """Get handoff instructions for a domain's agent chain."""
        config = self.get_recommended_agent(domain)
        return config.handoff_instructions

    def get_all_domains(self) -> list[str]:
        """Get list of all supported domains (built-in + custom)."""
        return list(self.merged_chains.keys())

    def _get_fallback_domain(self) -> str:
        """Get fallback domain from config or default."""
        if self._config and self._config.defaults:
            return self._config.defaults.get("fallback_domain", "unknown")
        return "unknown"

    def _get_fallback_agent(self) -> str:
        """Get fallback agent from config or default."""
        if self._config and self._config.defaults:
            return self._config.defaults.get("fallback_agent", "general-purpose")
        return "general-purpose"


# =============================================================================
# Default Router Instance (for backward compatibility)
# =============================================================================

# Module-level default router (uses built-in defaults)
_default_router: AgentRouter | None = None


def get_default_router() -> AgentRouter:
    """Get or create the default agent router."""
    global _default_router
    if _default_router is None:
        _default_router = AgentRouter()
    return _default_router


def set_default_router(router: AgentRouter) -> None:
    """Set a custom router as the default (for testing or global config)."""
    global _default_router
    _default_router = router
