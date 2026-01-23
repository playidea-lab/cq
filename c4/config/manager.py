"""C4 Configuration Manager.

Unified configuration management with:
- Global config (~/.c4/config.yaml)
- Project config (.c4/config.yaml)
- Environment variable overrides
- Credentials management (separate file)
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any

import yaml

from .credentials import CredentialsManager
from .models import (
    VALID_CONFIG_KEYS,
    C4Config,
    parse_config_value,
)


class ConfigManager:
    """Unified configuration manager for C4.

    Handles:
    - Loading/saving config from YAML files
    - Merging global + project configs
    - Environment variable overrides
    - Credentials management (via CredentialsManager)
    """

    def __init__(self, project_path: Path | None = None):
        """Initialize config manager.

        Args:
            project_path: Project directory path. Defaults to cwd.
        """
        self.project_path = project_path or Path.cwd()
        self.global_config_path = Path.home() / ".c4" / "config.yaml"
        self.project_config_path = self.project_path / ".c4" / "config.yaml"
        self.credentials = CredentialsManager(self.project_path)

    def load(self) -> C4Config:
        """Load merged configuration (global + project + env).

        Priority (highest to lowest):
        1. Environment variables
        2. Project config (.c4/config.yaml)
        3. Global config (~/.c4/config.yaml)
        4. Default values

        Returns:
            Merged C4Config
        """
        # Start with defaults
        config = C4Config()

        # Load global config
        if self.global_config_path.exists():
            global_config = self._load_yaml(self.global_config_path)
            if global_config:
                config = config.merge_with(C4Config(**global_config))

        # Load project config
        if self.project_config_path.exists():
            project_config = self._load_yaml(self.project_config_path)
            if project_config:
                config = config.merge_with(C4Config(**project_config))

        # Apply environment variable overrides
        config = self._apply_env_overrides(config)

        return config

    def get(self, key: str) -> Any:
        """Get a specific config value.

        Args:
            key: Config key (e.g., "platform", "git.user")

        Returns:
            Config value

        Raises:
            KeyError: If key is not valid
        """
        if key == "api-key":
            return self.credentials.get_masked_api_key()

        if key not in VALID_CONFIG_KEYS:
            raise KeyError(f"Unknown config key: {key}. Valid keys: {VALID_CONFIG_KEYS}")

        config = self.load()
        flat = config.to_flat_dict()
        return flat.get(key)

    def set(self, key: str, value: str, *, is_global: bool = True) -> Path:
        """Set a config value.

        Args:
            key: Config key (e.g., "platform", "git.user")
            value: Value to set
            is_global: If True, set in global config; else project config

        Returns:
            Path to the config file

        Raises:
            KeyError: If key is not valid
            ValueError: If value is invalid for the key type
        """
        # Handle api-key specially
        if key == "api-key":
            return self.credentials.set_api_key("anthropic", value, is_global=is_global)

        if key not in VALID_CONFIG_KEYS:
            raise KeyError(f"Unknown config key: {key}. Valid keys: {VALID_CONFIG_KEYS}")

        # Parse value to appropriate type
        parsed_value = parse_config_value(key, value)

        # Load existing config
        config_path = self.global_config_path if is_global else self.project_config_path
        existing: dict[str, Any] = {}
        if config_path.exists():
            existing = self._load_yaml(config_path) or {}

        # Update config
        self._set_nested_value(existing, key, parsed_value)

        # Save config
        config_path.parent.mkdir(parents=True, exist_ok=True)
        config_path.write_text(yaml.dump(existing, default_flow_style=False, allow_unicode=True))

        return config_path

    def show(self) -> dict[str, Any]:
        """Get all config values with their sources.

        Returns:
            Dict with config values and metadata
        """
        result: dict[str, Any] = {"values": {}, "sources": {}}

        config = self.load()
        flat = config.to_flat_dict()

        # Get sources
        global_config = self._load_yaml(self.global_config_path) if self.global_config_path.exists() else {}
        project_config = self._load_yaml(self.project_config_path) if self.project_config_path.exists() else {}
        global_flat = C4Config(**global_config).to_flat_dict() if global_config else {}
        project_flat = C4Config(**project_config).to_flat_dict() if project_config else {}

        for key in VALID_CONFIG_KEYS:
            result["values"][key] = flat.get(key)

            # Determine source
            if self._is_from_env(key):
                result["sources"][key] = "env"
            elif key in project_flat and project_flat[key] != C4Config().to_flat_dict().get(key):
                result["sources"][key] = "project"
            elif key in global_flat and global_flat[key] != C4Config().to_flat_dict().get(key):
                result["sources"][key] = "global"
            else:
                result["sources"][key] = "default"

        # Add api-key info
        providers = self.credentials.list_configured_providers()
        result["credentials"] = {}
        for provider, source in providers.items():
            masked = self.credentials.get_masked_api_key(provider)
            result["credentials"][provider] = {"masked": masked, "source": source}

        return result

    def reset(self, *, is_global: bool = True) -> bool:
        """Reset config to defaults.

        Args:
            is_global: If True, reset global config; else project config

        Returns:
            True if reset successful
        """
        config_path = self.global_config_path if is_global else self.project_config_path

        if config_path.exists():
            config_path.unlink()
            return True
        return False

    def _load_yaml(self, path: Path) -> dict[str, Any] | None:
        """Load YAML file safely."""
        try:
            content = yaml.safe_load(path.read_text())
            return content if isinstance(content, dict) else None
        except (yaml.YAMLError, OSError):
            return None

    def _set_nested_value(self, data: dict[str, Any], key: str, value: Any) -> None:
        """Set a nested value using dot notation key."""
        parts = key.split(".")
        if len(parts) == 1:
            data[key] = value
        else:
            # Handle nested keys like "git.user"
            section = parts[0]
            # Convert kebab-case to snake_case for field names
            field = parts[1].replace("-", "_")

            if section not in data:
                data[section] = {}
            data[section][field] = value

    def _is_from_env(self, key: str) -> bool:
        """Check if a config key is overridden by environment variable."""
        env_mapping = {
            "platform": "C4_PLATFORM",
            "git.user": "C4_GIT_USER",
            "git.email": "C4_GIT_EMAIL",
        }
        env_var = env_mapping.get(key)
        return env_var is not None and os.environ.get(env_var) is not None

    def _apply_env_overrides(self, config: C4Config) -> C4Config:
        """Apply environment variable overrides to config."""
        data = config.model_dump()

        # Platform
        if platform := os.environ.get("C4_PLATFORM"):
            data["platform"] = platform

        # Git
        if git_user := os.environ.get("C4_GIT_USER"):
            data["git"]["user"] = git_user
        if git_email := os.environ.get("C4_GIT_EMAIL"):
            data["git"]["email"] = git_email

        return C4Config(**data)


def ensure_credentials_gitignore(project_path: Path) -> bool:
    """Ensure credentials.yaml is in .gitignore.

    Args:
        project_path: Project directory path

    Returns:
        True if .gitignore was updated
    """
    gitignore_path = project_path / ".gitignore"
    pattern = ".c4/credentials.yaml"

    if gitignore_path.exists():
        content = gitignore_path.read_text()
        if pattern in content:
            return False  # Already present

        # Append to existing .gitignore
        with gitignore_path.open("a") as f:
            f.write(f"\n# C4 credentials (auto-added)\n{pattern}\n")
        return True
    else:
        # Create new .gitignore
        gitignore_path.write_text(f"# C4 credentials (auto-added)\n{pattern}\n")
        return True
