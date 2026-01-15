"""C4 Config Models - Configuration schemas"""

from typing import Any

from pydantic import BaseModel, Field

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


# =============================================================================
# LLM Configuration
# =============================================================================


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


class C4Config(BaseModel):
    """config.yaml schema"""

    project_id: str
    default_branch: str = "main"
    work_branch_prefix: str = "c4/w-"
    poll_interval_ms: int = 1000
    max_idle_minutes: int = 0  # 0 = unlimited
    scope_lock_ttl_sec: int = 1800  # 30 minutes, synchronized with WORKER_STALE_TIMEOUT
    validation: ValidationConfig = Field(default_factory=ValidationConfig)
    verifications: VerificationConfig = Field(default_factory=VerificationConfig)
    checkpoints: list[CheckpointConfig] = Field(default_factory=list)
    budgets: BudgetConfig = Field(default_factory=BudgetConfig)
    domain: str | None = None  # Project domain for default verifications
    agents: AgentConfig | None = None  # Custom agent configuration
    llm: LLMConfig = Field(default_factory=LLMConfig)  # LLM provider configuration
