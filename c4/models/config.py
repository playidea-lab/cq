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
