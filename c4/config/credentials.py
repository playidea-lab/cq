"""C4 Credentials Management.

Secure storage for API keys with:
- Separate credentials.yaml file (not in config.yaml)
- Automatic .gitignore addition
- File permission restrictions (600)
- Priority: env var > project > global
"""

from __future__ import annotations

import os
import stat
from pathlib import Path
from typing import TYPE_CHECKING

import yaml

if TYPE_CHECKING:
    pass

# Supported API key providers
SUPPORTED_PROVIDERS = ["anthropic", "openai", "gemini", "mistral", "cohere"]

# Environment variable mapping
ENV_VAR_MAPPING = {
    "anthropic": "ANTHROPIC_API_KEY",
    "openai": "OPENAI_API_KEY",
    "gemini": "GOOGLE_API_KEY",
    "mistral": "MISTRAL_API_KEY",
    "cohere": "COHERE_API_KEY",
}


class CredentialsManager:
    """Manage API key storage securely.

    API keys are stored in a separate credentials.yaml file:
    - Global: ~/.c4/credentials.yaml
    - Project: .c4/credentials.yaml

    Priority (highest to lowest):
    1. Environment variables (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)
    2. Project credentials (.c4/credentials.yaml)
    3. Global credentials (~/.c4/credentials.yaml)
    """

    def __init__(self, project_path: Path | None = None):
        """Initialize credentials manager.

        Args:
            project_path: Project directory path. Defaults to cwd.
        """
        self.global_path = Path.home() / ".c4" / "credentials.yaml"
        self.project_path = (project_path or Path.cwd()) / ".c4" / "credentials.yaml"

    def set_api_key(
        self,
        provider: str,
        api_key: str,
        *,
        is_global: bool = True,
    ) -> Path:
        """Store an API key.

        Args:
            provider: Provider name (anthropic, openai, etc.)
            api_key: API key value
            is_global: If True, store in global config; else project config

        Returns:
            Path to the credentials file

        Raises:
            ValueError: If provider is not supported
        """
        provider = provider.lower()
        if provider not in SUPPORTED_PROVIDERS and provider != "api-key":
            # Allow "api-key" as alias for "anthropic" (default)
            raise ValueError(
                f"Unknown provider: {provider}. Supported: {SUPPORTED_PROVIDERS}"
            )

        # Normalize "api-key" to "anthropic"
        if provider == "api-key":
            provider = "anthropic"

        creds_path = self.global_path if is_global else self.project_path

        # Load existing credentials
        creds: dict[str, str] = {}
        if creds_path.exists():
            try:
                content = yaml.safe_load(creds_path.read_text())
                if content:
                    creds = content
            except yaml.YAMLError:
                pass

        # Update credentials
        creds[provider] = api_key

        # Ensure directory exists
        creds_path.parent.mkdir(parents=True, exist_ok=True)

        # Write credentials
        creds_path.write_text(yaml.dump(creds, default_flow_style=False))

        # Set restrictive permissions (owner read/write only)
        try:
            creds_path.chmod(stat.S_IRUSR | stat.S_IWUSR)  # 600
        except OSError:
            pass  # May fail on Windows

        return creds_path

    def get_api_key(self, provider: str = "anthropic") -> str | None:
        """Get an API key with priority: env > project > global.

        Args:
            provider: Provider name (anthropic, openai, etc.)

        Returns:
            API key or None if not found
        """
        provider = provider.lower()

        # Normalize "api-key" to "anthropic"
        if provider == "api-key":
            provider = "anthropic"

        # 1. Check environment variable
        env_var = ENV_VAR_MAPPING.get(provider)
        if env_var and (env_value := os.environ.get(env_var)):
            return env_value

        # 2. Check project credentials
        if self.project_path.exists():
            try:
                content = yaml.safe_load(self.project_path.read_text())
                if content and provider in content:
                    return content[provider]
            except yaml.YAMLError:
                pass

        # 3. Check global credentials
        if self.global_path.exists():
            try:
                content = yaml.safe_load(self.global_path.read_text())
                if content and provider in content:
                    return content[provider]
            except yaml.YAMLError:
                pass

        return None

    def delete_api_key(self, provider: str, *, is_global: bool = True) -> bool:
        """Delete an API key.

        Args:
            provider: Provider name
            is_global: If True, delete from global config; else project config

        Returns:
            True if deleted, False if not found
        """
        provider = provider.lower()
        if provider == "api-key":
            provider = "anthropic"

        creds_path = self.global_path if is_global else self.project_path

        if not creds_path.exists():
            return False

        try:
            content = yaml.safe_load(creds_path.read_text())
            if not content or provider not in content:
                return False

            del content[provider]
            creds_path.write_text(yaml.dump(content, default_flow_style=False))
            return True
        except yaml.YAMLError:
            return False

    def list_configured_providers(self) -> dict[str, str]:
        """List all configured providers with their source.

        Returns:
            Dict of provider -> source ("env", "project", "global")
        """
        result: dict[str, str] = {}

        # Check environment variables
        for provider, env_var in ENV_VAR_MAPPING.items():
            if os.environ.get(env_var):
                result[provider] = "env"

        # Check project credentials
        if self.project_path.exists():
            try:
                content = yaml.safe_load(self.project_path.read_text())
                if content:
                    for provider in content:
                        if provider not in result:
                            result[provider] = "project"
            except yaml.YAMLError:
                pass

        # Check global credentials
        if self.global_path.exists():
            try:
                content = yaml.safe_load(self.global_path.read_text())
                if content:
                    for provider in content:
                        if provider not in result:
                            result[provider] = "global"
            except yaml.YAMLError:
                pass

        return result

    def mask_api_key(self, api_key: str, visible_chars: int = 4) -> str:
        """Mask an API key for display.

        Args:
            api_key: API key to mask
            visible_chars: Number of characters to show at start

        Returns:
            Masked key like "sk-ant-****"
        """
        if len(api_key) <= visible_chars:
            return "*" * len(api_key)
        return api_key[:visible_chars] + "*" * (len(api_key) - visible_chars)

    def get_masked_api_key(self, provider: str = "anthropic") -> str | None:
        """Get a masked API key for display.

        Args:
            provider: Provider name

        Returns:
            Masked API key or None if not found
        """
        api_key = self.get_api_key(provider)
        if api_key:
            return self.mask_api_key(api_key)
        return None
