"""LiteLLM Backend - Universal LLM provider support.

Supports 100+ providers through LiteLLM:
- OpenAI: gpt-4o, gpt-4o-mini, o1, o1-mini
- Anthropic: claude-sonnet-4, claude-opus-4, claude-3-5-sonnet, etc.
- Azure: azure/deployment-name
- Ollama: ollama/llama3, ollama/mistral
- Bedrock: bedrock/anthropic.claude-3-sonnet
- And many more...

Full list: https://docs.litellm.ai/docs/providers
"""

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from .backend import SupervisorBackend, SupervisorError, SupervisorResponse, TokenUsage
from .claude_models import (
    estimate_cost,
    get_api_key,
    get_model_preset,
    is_claude_model,
    resolve_model_id,
)
from .context_loader import ContextLoader
from .response_parser import ResponseParser
from .strategies import get_strategy_for_model

if TYPE_CHECKING:
    from c4.supervisor.agent_graph.models import AgentDefinition

logger = logging.getLogger(__name__)


class LiteLLMBackend(SupervisorBackend):
    """
    Supervisor backend using LiteLLM for universal LLM access.

    Provides access to 100+ LLM providers through a unified interface.
    Includes built-in cost tracking and usage logging.

    For Claude models, automatically detects API key from ANTHROPIC_API_KEY
    and applies model-specific optimizations.

    Example:
        >>> # Claude with auto-detection
        >>> backend = LiteLLMBackend(model="claude-sonnet-4")
        >>> response = backend.run_review(prompt, bundle_dir)

        >>> # OpenAI with explicit key
        >>> backend = LiteLLMBackend(model="gpt-4o", api_key="sk-...")
    """

    def __init__(
        self,
        model: str = "gpt-4o",
        api_key: str | None = None,
        api_key_env: str | None = None,
        max_retries: int = 3,
        timeout: int = 300,
        temperature: float = 0.0,
        max_tokens: int | None = None,
        api_base: str | None = None,
        drop_params: bool = True,
        daemon: "C4Daemon | None" = None,
    ):
        """
        Initialize LiteLLM backend.

        Args:
            model: LiteLLM model identifier (e.g., "gpt-4o", "claude-sonnet-4")
                   Supports Claude aliases: "sonnet", "opus", "haiku"
            api_key: API key for the provider (optional if set in env)
            api_key_env: Environment variable name for API key
            max_retries: Maximum retry attempts on failure
            timeout: Request timeout in seconds
            temperature: Sampling temperature (0.0 = deterministic)
            max_tokens: Maximum output tokens (auto-detected for Claude models)
            api_base: Custom API base URL (for Azure, Ollama, etc.)
            drop_params: Drop unsupported parameters for the model
            daemon: Optional C4Daemon instance for metrics tracking
        """
        # Resolve model alias to full ID
        self.model = resolve_model_id(model)
        self._original_model = model
        self.strategy = get_strategy_for_model(self.model)
        self.daemon = daemon

        # Auto-detect API key for Claude models
        if api_key:
            self.api_key = api_key
        elif is_claude_model(self.model):
            self.api_key = get_api_key(api_key_env)
            if not self.api_key:
                logger.warning(
                    "No Anthropic API key found. Set ANTHROPIC_API_KEY environment variable "
                    "or provide api_key parameter."
                )
        else:
            self.api_key = None

        self.max_retries = max_retries
        self.timeout = timeout
        self.temperature = temperature
        self.api_base = api_base
        self.drop_params = drop_params

        # Auto-detect max_tokens for Claude models
        preset = get_model_preset(self.model)
        if max_tokens is not None:
            self.max_tokens = max_tokens
        elif preset:
            self.max_tokens = preset.max_output_tokens
            logger.debug(f"Using max_tokens={self.max_tokens} from {preset.display_name} preset")
        else:
            self.max_tokens = 4096

        self._last_usage: TokenUsage | None = None
        self._is_claude = is_claude_model(self.model)

        # Log model resolution
        if model != self.model:
            logger.info(f"Resolved model alias '{model}' -> '{self.model}'")

    @property
    def name(self) -> str:
        """Backend name for logging."""
        return f"litellm-{self.model}"

    @property
    def last_usage(self) -> TokenUsage | None:
        """Get token usage from last request."""
        return self._last_usage

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
        agent: "AgentDefinition | None" = None,
    ) -> SupervisorResponse:
        """
        Run supervisor review using LiteLLM.

        Args:
            prompt: Rendered review prompt
            bundle_dir: Path to bundle directory for saving artifacts
            timeout: Timeout in seconds (overrides instance default)
            agent: Optional agent definition for persona injection

        Returns:
            SupervisorResponse with decision

        Raises:
            SupervisorError: If review fails after retries
        """
        # Lazy import to avoid requiring litellm if not used
        try:
            import litellm
        except ImportError:
            raise SupervisorError("litellm package not installed. Run: uv add litellm")

        # Save prompt to bundle
        (bundle_dir / "prompt.md").write_text(prompt)

        last_error: SupervisorError | None = None
        effective_timeout = timeout or self.timeout

        for attempt in range(self.max_retries):
            try:
                # Build request parameters
                kwargs = self._build_request_kwargs(prompt, effective_timeout, agent)

                # Call LiteLLM
                response = litellm.completion(**kwargs)

                # Track usage
                self._track_usage(response)

                # Parse response
                output = self.strategy.parse_response(response)
                result = ResponseParser.parse(output)

                # Save artifacts
                (bundle_dir / "raw_response.txt").write_text(output)
                self.save_response(bundle_dir, result)

                return result

            except ImportError as e:
                raise SupervisorError(str(e))
            except ValueError as e:
                last_error = SupervisorError(f"Failed to parse response: {e}")
                logger.warning(f"Attempt {attempt + 1}/{self.max_retries} failed: {e}")
            except Exception as e:
                error_type = type(e).__name__
                last_error = SupervisorError(f"{error_type}: {str(e)[:200]}")
                logger.warning(f"Attempt {attempt + 1}/{self.max_retries} failed: {error_type}")

        raise last_error or SupervisorError("LiteLLM failed after retries")

    def _build_request_kwargs(
        self,
        prompt: str,
        timeout: int,
        agent: "AgentDefinition | None" = None,
    ) -> dict:
        """Build request parameters for LiteLLM with Prompt Caching optimization.

        Rearranges system message components to maximize cache hits:
        [Standards/Rules] -> [Output Format] -> [Agent Persona] -> [Dynamic Prompt]
        """
        # Load dynamic context (standards/rules) - Fixed block
        dynamic_context = ContextLoader.load_standards()

        # Build system message parts in deterministic order for caching
        system_parts = []

        # 1. Standards/Rules (Largest fixed block, should be at the top)
        if dynamic_context:
            system_parts.append(dynamic_context)

        # 2. Output Format Instruction (Fixed instruction)
        system_parts.append(
            "# OUTPUT FORMAT\n"
            "Always respond with a JSON object containing:\n"
            "- decision: APPROVE, REQUEST_CHANGES, or REPLAN\n"
            "- checkpoint: Current checkpoint ID\n"
            "- notes: Detailed explanation of your decision\n"
            "- required_changes: List of specific changes needed if not approved"
        )

        # 3. Agent Persona (Fixed per agent/worker)
        if agent:
            system_parts.append("# AGENT PERSONA\n" + self._format_agent_persona(agent))
        else:
            system_parts.append("# ROLE\nYou are a code review supervisor.")

        system_message = "\n\n".join(system_parts)

        kwargs = self.strategy.get_request_params(
            model=self.model,
            system_message=system_message,
            user_message=prompt,
            temperature=self.temperature,
            max_tokens=self.max_tokens,
            timeout=timeout,
            drop_params=self.drop_params,
            api_key=self.api_key,
            api_base=self.api_base,
        )

        # Add Prompt Caching headers for Claude if applicable
        if self._is_claude:
            if "extra_headers" not in kwargs:
                kwargs["extra_headers"] = {}
            # Standard Anthropic Prompt Caching header
            kwargs["extra_headers"]["anthropic-beta"] = "prompt-caching-2024-07-31"

        return kwargs

    def _format_agent_persona(self, agent: "AgentDefinition") -> str:
        """Format AgentDefinition into a text persona description.

        Args:
            agent: Agent definition model

        Returns:
            Formatted string describing the agent's role and behavior
        """
        a = agent.agent
        persona = a.persona

        lines = [
            f"You are {a.name} ({a.id}).",
            f"Role: {persona.role}",
            f"Expertise: {persona.expertise}",
        ]

        if persona.personality:
            p = persona.personality
            traits = []
            if p.style: traits.append(f"Style: {p.style}")
            if p.communication: traits.append(f"Communication: {p.communication}")
            if p.approach: traits.append(f"Approach: {p.approach}")
            if traits:
                lines.append("Personality: " + ", ".join(traits))

        if a.instructions and a.instructions.on_receive:
            lines.append("\nInstructions:")
            lines.append(a.instructions.on_receive)

        return "\n".join(lines)

    def _track_usage(self, response) -> None:
        """Track token usage from response.

        Args:
            response: LiteLLM response object
        """
        if not response.usage:
            return

        # Get cost from LiteLLM or calculate for Claude
        cost = getattr(response, "_hidden_params", {}).get("response_cost")

        if cost is None and self._is_claude:
            cost = estimate_cost(
                self.model,
                response.usage.prompt_tokens,
                response.usage.completion_tokens,
            )

        self._last_usage = TokenUsage(
            prompt_tokens=response.usage.prompt_tokens,
            completion_tokens=response.usage.completion_tokens,
            total_tokens=response.usage.total_tokens,
            cost=cost,
        )

        # Update daemon metrics if available
        if self.daemon:
            try:
                self.daemon.update_metrics(
                    prompt_tokens=response.usage.prompt_tokens,
                    completion_tokens=response.usage.completion_tokens,
                    cost_usd=cost or 0.0,
                )
            except Exception as e:
                logger.warning(f"Failed to update daemon metrics: {e}")

        self._log_usage()

    def _log_usage(self) -> None:
        """Log token usage and estimated cost."""
        if self._last_usage:
            cost_str = ""
            if self._last_usage.cost is not None:
                cost_str = f", ~${self._last_usage.cost:.4f}"

            logger.info(f"LiteLLM [{self.model}]: {self._last_usage.total_tokens} tokens{cost_str}")


def create_claude_backend(
    model: str = "sonnet",
    api_key_env: str | None = None,
    **kwargs,
) -> LiteLLMBackend:
    """Create a LiteLLM backend configured for Claude.

    Convenience function for creating Claude-specific backends with
    automatic API key detection and model preset application.

    Args:
        model: Claude model tier or ID ("sonnet", "opus", "haiku" or full ID)
        api_key_env: Environment variable name for API key
        **kwargs: Additional arguments passed to LiteLLMBackend

    Returns:
        Configured LiteLLMBackend

    Example:
        >>> backend = create_claude_backend("sonnet")
        >>> backend = create_claude_backend("claude-3-5-haiku-20241022")
    """
    return LiteLLMBackend(
        model=model,
        api_key_env=api_key_env,
        **kwargs,
    )
