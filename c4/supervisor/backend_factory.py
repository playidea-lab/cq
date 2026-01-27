"""Backend Factory - Creates supervisor backends from configuration."""

from __future__ import annotations

import os
from pathlib import Path
from typing import TYPE_CHECKING

from .backend import SupervisorBackend, SupervisorError

if TYPE_CHECKING:
    from ..models.config import LLMConfig


def create_backend(
    config: LLMConfig,
    working_dir: Path | None = None,
) -> SupervisorBackend:
    """
    Create a supervisor backend from LLM configuration.

    Args:
        config: LLM configuration from config.yaml
        working_dir: Working directory (required for ClaudeCliBackend)

    Returns:
        Configured SupervisorBackend instance

    Raises:
        SupervisorError: If API key is missing or provider not supported

    Example:
        >>> from c4.models.config import LLMConfig
        >>> config = LLMConfig(model="gpt-4o", api_key_env="OPENAI_API_KEY")
        >>> backend = create_backend(config)
    """
    # Legacy Claude CLI support (default)
    if config.is_claude_cli():
        from .claude_backend import ClaudeCliBackend

        return ClaudeCliBackend(
            working_dir=working_dir,
            max_retries=config.max_retries,
            model=None,  # Uses default from CLI
        )

    # LiteLLM for all other providers
    api_key = None
    target_env = config.api_key_env

    # Auto-detect API key env if not specified
    if not target_env:
        model_lower = config.model.lower()
        if "gemini" in model_lower:
            target_env = "GOOGLE_API_KEY"
        elif "gpt" in model_lower or "o1-" in model_lower or "openai" in model_lower:
            target_env = "OPENAI_API_KEY"
        elif "mistral" in model_lower:
            target_env = "MISTRAL_API_KEY"
        elif "command" in model_lower:
            target_env = "COHERE_API_KEY"

    if target_env:
        # 1. Try environment variable directly
        api_key = os.environ.get(target_env)
        
        # 2. Try CredentialsManager if not in env
        if not api_key:
            from ..config.credentials import ENV_VAR_MAPPING, CredentialsManager
            
            # Find provider name from env var name (e.g., GOOGLE_API_KEY -> gemini)
            provider = next(
                (p for p, env in ENV_VAR_MAPPING.items() if env == target_env),
                None
            )
            
            if provider:
                api_key = CredentialsManager().get_api_key(provider)

        if not api_key and config.api_key_env:
            raise SupervisorError(
                f"API key not found in environment variable: {config.api_key_env}"
            )

    from .litellm_backend import LiteLLMBackend

    return LiteLLMBackend(
        model=config.model,
        api_key=api_key,
        max_retries=config.max_retries,
        timeout=config.timeout,
        temperature=config.temperature,
        max_tokens=config.max_tokens,
        api_base=config.api_base,
        drop_params=config.drop_params,
    )


def create_backend_from_config_file(
    c4_dir: Path,
    working_dir: Path | None = None,
) -> SupervisorBackend:
    """
    Create backend by loading config from .c4/config.yaml.

    Falls back to ClaudeCliBackend if no config or llm section exists.

    Args:
        c4_dir: Path to .c4 directory
        working_dir: Working directory for CLI backends

    Returns:
        Configured SupervisorBackend instance
    """
    import yaml

    from ..models.config import C4Config, LLMConfig

    config_file = c4_dir / "config.yaml"

    if config_file.exists():
        try:
            data = yaml.safe_load(config_file.read_text())
            c4_config = C4Config.model_validate(data)
            return create_backend(c4_config.llm, working_dir)
        except Exception:
            # Fall back to default on any config error
            pass

    # No config or error, use default (Claude CLI)
    return create_backend(LLMConfig(), working_dir)
