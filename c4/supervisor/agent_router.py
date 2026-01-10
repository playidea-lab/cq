"""Agent Routing System - Domain-based agent selection and chaining.

This module provides automatic agent selection based on project domain,
with support for agent chaining (sequential execution of multiple agents).

Usage:
    # Get recommended agent chain for a domain
    config = get_recommended_agent("web-frontend")
    print(config.primary)  # "frontend-developer"
    print(config.chain)    # ["frontend-developer", "test-automator", "code-reviewer"]

    # Get agent for specific task type (future)
    agent = get_agent_for_task_type("api-design", "web-backend")
"""

from dataclasses import dataclass, field

from c4.discovery.models import Domain


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
}


# =============================================================================
# Public API Functions
# =============================================================================


def get_recommended_agent(domain: str | Domain | None) -> AgentChainConfig:
    """
    Get recommended agent chain configuration for a domain.

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
        lines.extend([
            "## Your Role",
            f"You are the **primary implementer** ({agent}) for this task.",
            "Implement the core functionality according to the DoD.",
            "",
        ])
    elif agent == "code-reviewer":
        lines.extend([
            "## Your Role",
            "You are the **code reviewer** for this task.",
            "Review the implementation for:",
            "- Code quality and best practices",
            "- Test coverage",
            "- Security concerns",
            "- Performance issues",
            "",
        ])
    elif agent == "test-automator":
        lines.extend([
            "## Your Role",
            "You are the **test engineer** for this task.",
            "Your responsibilities:",
            "- Write unit tests for new functionality",
            "- Add integration tests if applicable",
            "- Ensure edge cases are covered",
            "",
        ])
    else:
        lines.extend([
            "## Your Role",
            f"You are the **{agent}** in this chain.",
            "Continue building on previous work.",
            "",
        ])

    if handoff:
        lines.append(handoff.to_prompt())

    if handoff_instructions and not is_first:
        lines.extend([
            "## Handoff Guidelines",
            handoff_instructions,
            "",
        ])

    if not is_last:
        next_agent = agent_chain[agent_index + 1]
        lines.extend([
            "## Next Steps",
            f"After completing your work, the **{next_agent}** will continue.",
            "Prepare a clear handoff summary.",
            "",
        ])

    return "\n".join(lines)
