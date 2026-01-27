"""Backend Factory - Creates supervisor backends from configuration."""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from .backend import SupervisorBackend

if TYPE_CHECKING:
    from ..mcp_server import C4Daemon
    from ..models.config import LLMConfig


def create_backend(
    config: LLMConfig,
    working_dir: Path | None = None,
    daemon: "C4Daemon | None" = None,
) -> SupervisorBackend:
    """
    Create a supervisor backend from LLM configuration.

    Args:
        config: LLM configuration from config.yaml
        working_dir: Working directory (required for ClaudeCliBackend)
        daemon: Optional C4Daemon instance for metrics tracking

    Returns:
        Configured SupervisorBackend instance
    """
    # ... (기존 로직) ...
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
        daemon=daemon,
    )


def create_backend_from_config_file(
    c4_dir: Path,
    working_dir: Path | None = None,
    daemon: "C4Daemon | None" = None,
) -> SupervisorBackend:
    """
    Create backend by loading config from .c4/config.yaml.

    Args:
        c4_dir: Path to .c4 directory
        working_dir: Working directory for CLI backends
        daemon: Optional C4Daemon instance for metrics tracking
    """
    import yaml

    from ..models.config import C4Config, LLMConfig

    config_file = c4_dir / "config.yaml"

    if config_file.exists():
        try:
            data = yaml.safe_load(config_file.read_text())
            c4_config = C4Config.model_validate(data)
            return create_backend(c4_config.llm, working_dir, daemon=daemon)
        except Exception:
            pass

    # No config or error, use default (Claude CLI)
    return create_backend(LLMConfig(), working_dir, daemon=daemon)
