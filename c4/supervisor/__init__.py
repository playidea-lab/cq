"""C4 Supervisor - Pluggable checkpoint review system

Usage:
    from c4.supervisor import Supervisor, ClaudeCliBackend, MockBackend

    # Default (Claude CLI)
    supervisor = Supervisor(project_root)
    response = supervisor.run_supervisor(bundle_dir)

    # With specific backend
    backend = ClaudeCliBackend(model="claude-3-opus")
    supervisor = Supervisor(project_root, backend=backend)

    # With LLM config (OpenAI, Anthropic, etc.)
    from c4.models import LLMConfig
    config = LLMConfig(model="gpt-4o", api_key_env="OPENAI_API_KEY")
    supervisor = Supervisor(project_root, llm_config=config)

    # For testing
    backend = MockBackend(decision=SupervisorDecision.APPROVE)
    supervisor = Supervisor(project_root, backend=backend)
"""

from .agent_router import (
    DOMAIN_AGENT_MAP,
    AgentChainConfig,
    AgentHandoff,
    build_chain_prompt,
    get_agent_for_task_type,
    get_chain_for_domain,
    get_handoff_instructions,
    get_recommended_agent,
)
from .backend import SupervisorBackend, SupervisorError, SupervisorResponse
from .backend_factory import create_backend, create_backend_from_config_file
from .claude_backend import ClaudeCliBackend
from .cloud_supervisor import (
    CloudSupervisor,
    ReviewRequest,
    ReviewResult,
    ReviewStatus,
    ReviewType,
)
from .litellm_backend import LiteLLMBackend
from .mock_backend import MockBackend
from .prompt import PromptRenderer
from .response_parser import ResponseParser
from .supervisor import Supervisor
from .verifier import (
    CliVerifier,
    HttpVerifier,
    VerificationResult,
    VerificationRunner,
    VerificationType,
    Verifier,
    VerifierRegistry,
)

__all__ = [
    # Main class
    "Supervisor",
    # Cloud Supervisor
    "CloudSupervisor",
    "ReviewRequest",
    "ReviewResult",
    "ReviewStatus",
    "ReviewType",
    # Backends
    "SupervisorBackend",
    "ClaudeCliBackend",
    "LiteLLMBackend",
    "MockBackend",
    # Backend factory
    "create_backend",
    "create_backend_from_config_file",
    # Supporting classes
    "SupervisorResponse",
    "SupervisorError",
    "PromptRenderer",
    "ResponseParser",
    # Verification
    "Verifier",
    "VerifierRegistry",
    "VerificationType",
    "VerificationResult",
    "VerificationRunner",
    "HttpVerifier",
    "CliVerifier",
    # Agent routing
    "AgentChainConfig",
    "AgentHandoff",
    "get_recommended_agent",
    "get_agent_for_task_type",
    "get_chain_for_domain",
    "get_handoff_instructions",
    "build_chain_prompt",
    "DOMAIN_AGENT_MAP",
]
