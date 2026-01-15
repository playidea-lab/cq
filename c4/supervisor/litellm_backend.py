"""LiteLLM Backend - Universal LLM provider support.

Supports 100+ providers through LiteLLM:
- OpenAI: gpt-4o, gpt-4o-mini, o1, o1-mini
- Anthropic: claude-3-opus, claude-3-sonnet
- Azure: azure/deployment-name
- Ollama: ollama/llama3, ollama/mistral
- Bedrock: bedrock/anthropic.claude-3-sonnet
- And many more...

Full list: https://docs.litellm.ai/docs/providers
"""

import logging
from pathlib import Path

from .backend import SupervisorBackend, SupervisorError, SupervisorResponse, TokenUsage
from .response_parser import ResponseParser

logger = logging.getLogger(__name__)


class LiteLLMBackend(SupervisorBackend):
    """
    Supervisor backend using LiteLLM for universal LLM access.

    Provides access to 100+ LLM providers through a unified interface.
    Includes built-in cost tracking and usage logging.

    Example:
        >>> backend = LiteLLMBackend(model="gpt-4o", api_key="sk-...")
        >>> response = backend.run_review(prompt, bundle_dir)
    """

    def __init__(
        self,
        model: str = "gpt-4o",
        api_key: str | None = None,
        max_retries: int = 3,
        timeout: int = 300,
        temperature: float = 0.0,
        max_tokens: int = 4096,
        api_base: str | None = None,
        drop_params: bool = True,
    ):
        """
        Initialize LiteLLM backend.

        Args:
            model: LiteLLM model identifier (e.g., "gpt-4o", "claude-3-opus")
            api_key: API key for the provider (optional if set in env)
            max_retries: Maximum retry attempts on failure
            timeout: Request timeout in seconds
            temperature: Sampling temperature (0.0 = deterministic)
            max_tokens: Maximum output tokens
            api_base: Custom API base URL (for Azure, Ollama, etc.)
            drop_params: Drop unsupported parameters for the model
        """
        self.model = model
        self.api_key = api_key
        self.max_retries = max_retries
        self.timeout = timeout
        self.temperature = temperature
        self.max_tokens = max_tokens
        self.api_base = api_base
        self.drop_params = drop_params

        self._last_usage: TokenUsage | None = None

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
    ) -> SupervisorResponse:
        """
        Run supervisor review using LiteLLM.

        Args:
            prompt: Rendered review prompt
            bundle_dir: Path to bundle directory for saving artifacts
            timeout: Timeout in seconds (overrides instance default)

        Returns:
            SupervisorResponse with decision

        Raises:
            SupervisorError: If review fails after retries
        """
        # Lazy import to avoid requiring litellm if not used
        try:
            import litellm
        except ImportError:
            raise SupervisorError(
                "litellm package not installed. Run: uv add litellm"
            )

        # Save prompt to bundle
        (bundle_dir / "prompt.md").write_text(prompt)

        last_error: SupervisorError | None = None
        effective_timeout = timeout or self.timeout

        for attempt in range(self.max_retries):
            try:
                # Build request parameters
                kwargs = {
                    "model": self.model,
                    "messages": [
                        {
                            "role": "system",
                            "content": (
                                "You are a code review supervisor. "
                                "Always respond with a JSON object containing: "
                                "decision (APPROVE/REQUEST_CHANGES/REPLAN), "
                                "checkpoint, notes, and required_changes array."
                            ),
                        },
                        {"role": "user", "content": prompt},
                    ],
                    "temperature": self.temperature,
                    "max_tokens": self.max_tokens,
                    "timeout": effective_timeout,
                    "drop_params": self.drop_params,
                }

                # Add optional parameters
                if self.api_key:
                    kwargs["api_key"] = self.api_key
                if self.api_base:
                    kwargs["api_base"] = self.api_base

                # Call LiteLLM
                response = litellm.completion(**kwargs)

                # Track usage (LiteLLM provides cost estimation)
                if response.usage:
                    self._last_usage = TokenUsage(
                        prompt_tokens=response.usage.prompt_tokens,
                        completion_tokens=response.usage.completion_tokens,
                        total_tokens=response.usage.total_tokens,
                        cost=getattr(response, "_hidden_params", {}).get(
                            "response_cost"
                        ),
                    )
                    self._log_usage()

                # Parse response
                output = response.choices[0].message.content or ""
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
                logger.warning(
                    f"Attempt {attempt + 1}/{self.max_retries} failed: {error_type}"
                )

        raise last_error or SupervisorError("LiteLLM failed after retries")

    def _log_usage(self) -> None:
        """Log token usage and estimated cost."""
        if self._last_usage:
            cost_str = ""
            if self._last_usage.cost is not None:
                cost_str = f", ~${self._last_usage.cost:.4f}"

            logger.info(
                f"LiteLLM [{self.model}]: "
                f"{self._last_usage.total_tokens} tokens"
                f"{cost_str}"
            )
