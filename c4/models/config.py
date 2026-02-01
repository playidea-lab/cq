"""C4 System Configuration Models.

Comprehensive configuration schemas for internal C4 operations:
- Agent configuration (chains, task overrides)
- Validation and verification settings
- Store backend configuration (SQLite, Supabase)
- LLM configuration (Claude CLI, LiteLLM)
- GitHub integration
- Worktree configuration

Note:
    For user-facing configuration models (git, worker settings, etc.),
    see c4.config.models. This module is for system-internal configuration.
"""

from typing import Any

from pydantic import BaseModel, Field, ValidationInfo, field_validator

from .checkpoint import CheckpointConfig

# =============================================================================
# Agent Configuration Models
# =============================================================================


class AgentChainDef(BaseModel):
    """Agent chain definition for YAML config.

    Example:
        web-frontend:
          primary: frontend-developer
          chain: [frontend-developer, test-automator, code-reviewer]
          handoff: "Pass component specs and test requirements"
    """

    primary: str
    chain: list[str] = Field(default_factory=list)
    handoff: str = ""


class AgentConfig(BaseModel):
    """Agent configuration section for config.yaml.

    Example:
        agents:
          chains:
            web-frontend:
              primary: frontend-developer
              chain: [frontend-developer, test-automator]
              handoff: "Pass specs"
            my-custom-domain:
              primary: custom-agent
              chain: [custom-agent, reviewer]
          task_overrides:
            test: test-automator
            review: code-reviewer
          defaults:
            fallback_domain: unknown
            fallback_agent: general-purpose
    """

    chains: dict[str, AgentChainDef] = Field(default_factory=dict)
    task_overrides: dict[str, str] = Field(default_factory=dict)
    defaults: dict[str, str] = Field(
        default_factory=lambda: {
            "fallback_domain": "unknown",
            "fallback_agent": "general-purpose",
        }
    )


# =============================================================================
# Validation & Verification Configuration
# =============================================================================


class ValidationConfig(BaseModel):
    """Validation command configuration (static analysis)."""

    commands: dict[str, str] = Field(
        default_factory=lambda: {
            "lint": "npm run lint",
            "unit": "npm test",
            "e2e": "npm run e2e",
        }
    )
    required: list[str] = Field(default_factory=lambda: ["lint", "unit"])


class VerificationItem(BaseModel):
    """Single verification configuration.

    Example:
        - type: http
          name: API Health Check
          config:
            url: http://localhost:8000/health
            method: GET
            expected_status: 200
    """

    type: str  # http, cli, browser, etc.
    name: str
    config: dict[str, Any] = Field(default_factory=dict)
    enabled: bool = True


class VerificationConfig(BaseModel):
    """Verification configuration (runtime verification).

    These are dynamic checks that run before supervisor review:
    - HTTP: API calls
    - CLI: Command execution
    - Browser: E2E tests with Playwright

    Example in config.yaml:
        verifications:
          items:
            - type: http
              name: API Health
              config:
                url: /api/health
                expected_status: 200
          base_url: http://localhost:8000
          enabled: true
    """

    items: list[VerificationItem] = Field(default_factory=list)
    base_url: str | None = None  # For HTTP verifications
    enabled: bool = True  # Global enable/disable


class BudgetConfig(BaseModel):
    """Budget limits"""

    max_iterations_per_task: int = 7
    max_failures_same_signature: int = 3


class LongRunningConfig(BaseModel):
    """Long-running task handling configuration.

    When a worker has been unresponsive (no heartbeat) for longer than
    warning_timeout_sec, a warning is shown in c4_status. The user can
    then decide to continue, extend the timeout, or kill the worker.

    By default, the system only warns and waits for user decision.
    Set auto_recover=True to enable automatic stale recovery after stale_timeout_sec.
    """

    warning_timeout_sec: int = Field(
        default=2400,  # 40 minutes
        ge=60,
        description="Seconds before showing warning in c4_status",
    )
    stale_timeout_sec: int = Field(
        default=3600,  # 60 minutes
        ge=120,
        description="Seconds threshold for stale detection (only used if auto_recover=True)",
    )
    auto_extend: bool = Field(
        default=False,
        description="If true, auto-extend timeout on warning instead of requiring user action",
    )
    auto_recover: bool = Field(
        default=False,
        description="If true, automatically recover stale workers. If false (default), only warn and wait for user decision.",
    )

    @field_validator("stale_timeout_sec")
    @classmethod
    def stale_must_exceed_warning(cls, v: int, info: ValidationInfo) -> int:
        """Ensure stale timeout is greater than warning timeout."""
        warning = info.data.get("warning_timeout_sec", 2400)
        if v <= warning:
            raise ValueError("stale_timeout_sec must be greater than warning_timeout_sec")
        return v


# =============================================================================
# LLM Configuration
# =============================================================================


class StoreConfig(BaseModel):
    """Store backend configuration.

    Supports three backends:
    - sqlite (default): Local SQLite database in .c4/c4.db
    - local_file: JSON file in .c4/state.json
    - supabase: Cloud-based Supabase storage

    Example in config.yaml:
        # Default SQLite
        store:
          backend: sqlite

        # Supabase for team collaboration
        store:
          backend: supabase
          supabase_url: https://xxx.supabase.co
          supabase_key: your-anon-key

        # Or use environment variables
        # C4_STORE_BACKEND=supabase
        # SUPABASE_URL=https://xxx.supabase.co
        # SUPABASE_KEY=your-anon-key
    """

    backend: str | None = Field(
        default=None,
        pattern="^(sqlite|local_file|supabase)$",
        description="Store backend: sqlite, local_file, or supabase",
    )
    supabase_url: str | None = Field(
        default=None,
        description="Supabase project URL (or use SUPABASE_URL env)",
    )
    supabase_key: str | None = Field(
        default=None,
        description="Supabase anon key (or use SUPABASE_KEY env)",
    )
    team_id: str | None = Field(
        default=None,
        description="Team ID for RLS isolation (or use C4_TEAM_ID env)",
    )
    access_token: str | None = Field(
        default=None,
        description="Supabase Auth JWT token for RLS (or use SUPABASE_ACCESS_TOKEN env)",
    )


class LLMConfig(BaseModel):
    """LLM configuration for supervisor backend.

    Supports two modes:
    1. claude-cli (default): Uses `claude -p` CLI with user's Claude subscription
    2. LiteLLM: Direct API calls to 100+ providers (OpenAI, Anthropic, etc.)

    Example in config.yaml:
        # Default - uses Claude Code (your subscription)
        llm:
          model: claude-cli

        # OpenAI
        llm:
          model: gpt-4o
          api_key_env: OPENAI_API_KEY

        # Anthropic API (separate from Claude Code)
        llm:
          model: claude-3-opus-20240229
          api_key_env: ANTHROPIC_API_KEY

        # Ollama (local)
        llm:
          model: ollama/llama3
          api_base: http://localhost:11434

        # Azure OpenAI
        llm:
          model: azure/gpt-4o-deployment
          api_base: https://my-resource.openai.azure.com
          api_key_env: AZURE_OPENAI_API_KEY
    """

    model: str = Field(
        default="claude-cli",
        description="LiteLLM model identifier or 'claude-cli' for CLI backend",
    )
    api_key_env: str | None = Field(
        default=None,
        description="Environment variable name containing API key",
    )
    timeout: int = Field(
        default=300,
        ge=30,
        le=600,
        description="Request timeout in seconds",
    )
    max_retries: int = Field(
        default=3,
        ge=1,
        le=10,
        description="Maximum retry attempts",
    )
    temperature: float = Field(
        default=0.0,
        ge=0.0,
        le=2.0,
        description="Sampling temperature",
    )
    max_tokens: int = Field(
        default=4096,
        ge=256,
        description="Maximum output tokens",
    )
    api_base: str | None = Field(
        default=None,
        description="Custom API base URL (for Azure, Ollama, etc.)",
    )
    drop_params: bool = Field(
        default=True,
        description="Drop unsupported parameters for the model",
    )

    def is_claude_cli(self) -> bool:
        """Check if using Claude CLI backend (user's subscription)."""
        return self.model == "claude-cli"


class GitHubConfig(BaseModel):
    """GitHub integration configuration for auto commit/PR.

    Example in config.yaml:
        # Default - disabled
        github:
          enabled: false

        # Full automation
        github:
          enabled: true
          auto_commit: true
          auto_pr: true
          base_branch: main
          reviewers: ["user1", "user2"]

        # Or use environment variable for token
        # GITHUB_TOKEN=ghp_xxx
    """

    enabled: bool = Field(
        default=False,
        description="Enable GitHub integration",
    )
    auto_commit: bool = Field(
        default=True,
        description="Auto-commit when task completes with validation pass",
    )
    auto_pr: bool = Field(
        default=True,
        description="Auto-create PR when all tasks complete",
    )
    base_branch: str = Field(
        default="main",
        description="Base branch for PRs",
    )
    reviewers: list[str] = Field(
        default_factory=list,
        description="GitHub usernames to request review from",
    )
    labels: list[str] = Field(
        default_factory=lambda: ["c4-generated"],
        description="Labels to add to PRs",
    )
    draft: bool = Field(
        default=False,
        description="Create PRs as drafts",
    )
    commit_prefix: str = Field(
        default="[C4]",
        description="Prefix for auto-commit messages",
    )



class TaskSystemConfig(BaseModel):
    """Unified task system configuration.

    Controls behavior of checkpoint and repair tasks in the single queue model.
    """

    # Checkpoint settings
    checkpoint_required_completions: int = Field(
        default=2,
        ge=1,
        le=5,
        description="Number of completions required for CP tasks (default: 2)",
    )
    checkpoint_require_different_workers: bool = Field(
        default=False,
        description=(
            "If true, each completion must be by a different worker. "
            "If false (default), same worker OK after context clear. "
            "Automatically false if only 1 worker available."
        ),
    )

    # Repair settings
    repair_failure_threshold: int = Field(
        default=3,
        ge=1,
        le=10,
        description="Number of failures before auto-creating RPR task",
    )
    repair_auto_create: bool = Field(
        default=True,
        description="Auto-create RPR tasks on failure threshold. If false, manual only.",
    )


class HooksConfig(BaseModel):
    """Git hooks configuration for C4 workflow."""

    enabled: bool = Field(
        default=True,
        description="Enable C4 Git hooks integration",
    )
    install_on_init: bool = Field(
        default=False,
        description="Auto-install hooks when running 'c4 init'",
    )

    # Pre-commit hook settings
    pre_commit_enabled: bool = Field(
        default=True,
        description="Enable pre-commit hook (runs lint)",
    )
    pre_commit_validations: list[str] = Field(
        default_factory=lambda: ["lint"],
        description="Validations to run in pre-commit hook",
    )

    # Commit-msg hook settings
    commit_msg_enabled: bool = Field(
        default=True,
        description="Enable commit-msg hook (validates Task ID)",
    )
    commit_msg_mode: str = Field(
        default="warn",
        pattern="^(warn|strict)$",
        description="Mode for Task ID validation: 'warn' or 'strict'",
    )
    commit_msg_pattern: str = Field(
        default=r"\[T-\d+-\d+\]|\[R-\d+-\d+\]|\[CP-\d+\]",
        description="Regex pattern for valid Task IDs in commit messages",
    )

    # Post-commit hook settings
    post_commit_enabled: bool = Field(
        default=True,
        description="Enable post-commit hook (state sync)",
    )
    post_commit_sync_state: bool = Field(
        default=False,
        description="Sync C4 state after commit (experimental)",
    )


class PlanSyncConfig(BaseModel):
    """Plan file synchronization configuration.

    Enables bidirectional sync between Claude plan files (~/.claude/plans/)
    and C4 task queue.

    Example in config.yaml:
        plan_sync:
          enabled: true
          auto_generate: true
          auto_update_status: true
    """

    enabled: bool = Field(
        default=True,
        description="Enable plan file synchronization with Claude",
    )
    auto_generate: bool = Field(
        default=True,
        description="Auto-generate plan file when tasks are added",
    )
    auto_update_status: bool = Field(
        default=True,
        description="Auto-update plan file checkbox when task completes",
    )
    plan_dir: str | None = Field(
        default=None,
        description="Custom plan directory (default: ~/.claude/plans)",
    )


class WorktreeConfig(BaseModel):
    """Worktree configuration for isolated parallel execution.

    When enabled, each worker operates in an isolated Git worktree,
    preventing file conflicts during parallel execution.

    Example in config.yaml:
        worktree:
          enabled: true
          base_branch: work
          work_dir: .c4/worktrees
          auto_cleanup: true
          completion_action: pr
    """

    enabled: bool = Field(
        default=False,
        description="Enable worktree-based isolation for parallel workers",
    )
    base_branch: str = Field(
        default="work",
        description="Base branch name for worktrees (relative to project)",
    )
    work_dir: str | None = Field(
        default=None,
        description="Directory for worktrees. Defaults to '.c4/worktrees' if not specified.",
    )
    auto_cleanup: bool = Field(
        default=True,
        description="Automatically clean up worktrees after task completion",
    )
    completion_action: str = Field(
        default="pr",
        pattern="^(merge|pr)$",
        description="Action when task completes: 'merge' = merge to base, 'pr' = create PR",
    )

    def get_work_dir(self) -> str:
        """Get the effective worktree directory path."""
        return self.work_dir or ".c4/worktrees"


# =============================================================================
# Enforce Mode Configuration (AI Agent Hints)
# =============================================================================


class EnforceModeHints(BaseModel):
    """Hints configuration for enforce_mode."""

    message: str = Field(
        default="ENFORCE MODE active",
        description="Message shown to AI agents in MCP responses",
    )


class EnforceModeDocs(BaseModel):
    """Document patterns to block in enforce_mode."""

    blocked_patterns: list[str] = Field(
        default_factory=lambda: ["PLAN.md", "TODO.md", "PHASES.md", "DONE.md"],
        description="File patterns that AI agents should not create",
    )
    redirect_message: str = Field(
        default="Use docs/ROADMAP.md for roadmap, .c4/tasks.db for tasks",
        description="Message explaining where to store information instead",
    )


class EnforceModeTools(BaseModel):
    """Tool preference hints for enforce_mode."""

    prefer_c4_tools: bool = Field(
        default=True,
        description="Whether to prefer C4 tools over generic Read/Write",
    )
    redirect_message: str = Field(
        default="Prefer c4_* tools for task management",
        description="Message explaining tool preferences",
    )


class EnforceModeConfig(BaseModel):
    """Configuration for AI agent enforcement hints.

    When enabled, MCP responses include hints that guide AI agents
    to use C4 tools and avoid creating duplicate planning documents.
    """

    enabled: bool = Field(
        default=False,
        description="Enable enforce mode hints in MCP responses",
    )
    hints: EnforceModeHints = Field(default_factory=EnforceModeHints)
    docs: EnforceModeDocs = Field(default_factory=EnforceModeDocs)
    tools: EnforceModeTools = Field(default_factory=EnforceModeTools)


class C4Config(BaseModel):
    """config.yaml schema"""

    project_id: str
    default_branch: str = "main"
    work_branch_prefix: str = "c4/w-"
    poll_interval_ms: int = 1000
    max_idle_minutes: int = 0  # 0 = unlimited
    scope_lock_ttl_sec: int = 3600  # 60 minutes, synchronized with WORKER_STALE_TIMEOUT
    validation: ValidationConfig = Field(default_factory=ValidationConfig)
    verifications: VerificationConfig = Field(default_factory=VerificationConfig)
    checkpoints: list[CheckpointConfig] = Field(default_factory=list)
    budgets: BudgetConfig = Field(default_factory=BudgetConfig)
    domain: str | None = None  # Project domain for default verifications
    agents: AgentConfig | None = None  # Custom agent configuration
    llm: LLMConfig = Field(default_factory=LLMConfig)  # LLM provider configuration
    store: StoreConfig = Field(default_factory=StoreConfig)  # Store backend config
    github: GitHubConfig = Field(default_factory=GitHubConfig)  # GitHub integration
    hooks: HooksConfig = Field(default_factory=HooksConfig)  # Git hooks configuration
    worktree: WorktreeConfig = Field(
        default_factory=WorktreeConfig,
        description="Worktree configuration for isolated parallel execution",
    )
    long_running: LongRunningConfig = Field(
        default_factory=LongRunningConfig,
        description="Long-running task timeout and recovery settings",
    )
    task_system: TaskSystemConfig = Field(
        default_factory=TaskSystemConfig,
        description="Unified task system settings (checkpoint/repair)",
    )
    plan_sync: PlanSyncConfig = Field(
        default_factory=PlanSyncConfig,
        description="Claude plan file synchronization settings",
    )

    # Review-as-Task configuration
    review_as_task: bool = Field(
        default=True,
        description="Auto-generate review tasks (R-XXX-N) when implementation tasks complete",
    )
    max_revision: int = Field(
        default=3,
        ge=1,
        le=10,
        description="Maximum revision count before task is marked BLOCKED",
    )
    review_priority_offset: int = Field(
        default=10,
        ge=0,
        description="Priority reduction for review tasks (lower priority = later in queue)",
    )

    # Checkpoint-as-Task configuration
    checkpoint_as_task: bool = Field(
        default=True,
        description="Auto-generate checkpoint tasks (CP-XXX) when all phase reviews complete",
    )
    checkpoint_priority_offset: int = Field(
        default=20,
        ge=0,
        description="Priority reduction for checkpoint tasks (lower than review tasks)",
    )

    # Branch strategy configuration
    work_branch: str | None = Field(
        default=None,
        description=(
            "Main working branch for C4 tasks. "
            "Defaults to 'c4/{project_id}' if not specified. "
            "All task branches (c4/w-T-XXX) are created from this branch."
        ),
    )
    completion_action: str = Field(
        default="merge",
        pattern="^(merge|pr|manual)$",
        description=(
            "Action when all tasks complete: "
            "'merge' = auto squash-merge to default_branch, "
            "'pr' = create pull request, "
            "'manual' = do nothing (user handles)"
        ),
    )

    # Enforce Mode - AI Agent Hints
    enforce_mode: EnforceModeConfig = Field(
        default_factory=EnforceModeConfig,
        description="AI agent enforcement hints for tool usage and document creation",
    )

    def get_work_branch(self) -> str:
        """Get the effective work branch name."""
        if self.work_branch:
            return self.work_branch
        return f"c4/{self.project_id}"
