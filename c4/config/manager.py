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
    UserConfig,
    parse_config_value,
)

# Environment variable to config key mapping
# Format: config_key -> (env_var_name, type)
# type: "str", "bool", "int"
ENV_VAR_MAPPING: dict[str, tuple[str, str]] = {
    # Platform
    "platform": ("C4_PLATFORM", "str"),
    # Git settings
    "git.user": ("C4_GIT_USER", "str"),
    "git.email": ("C4_GIT_EMAIL", "str"),
    "git.auto_commit": ("C4_GIT_AUTO_COMMIT", "bool"),
    "git.auto_pr": ("C4_GIT_AUTO_PR", "bool"),
    # Validation settings
    "validation.lint": ("C4_VALIDATION_LINT", "str"),
    "validation.test": ("C4_VALIDATION_TEST", "str"),
    "validation.typecheck": ("C4_VALIDATION_TYPECHECK", "str"),
    # Worker settings
    "worker.max_retries": ("C4_WORKER_MAX_RETRIES", "int"),
    "worker.timeout": ("C4_WORKER_TIMEOUT", "int"),
    # Team settings
    "team.id": ("C4_TEAM_ID", "str"),
    "team.name": ("C4_TEAM_NAME", "str"),
}


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

    def load(self) -> UserConfig:
        """Load merged configuration (global + project + env).

        Priority (highest to lowest):
        1. Environment variables
        2. Project config (.c4/config.yaml)
        3. Global config (~/.c4/config.yaml)
        4. Default values

        Returns:
            Merged UserConfig
        """
        # Start with defaults
        config = UserConfig()

        # Load global config
        if self.global_config_path.exists():
            global_config = self._load_yaml(self.global_config_path)
            if global_config:
                config = config.merge_with(UserConfig(**global_config))

        # Load project config
        if self.project_config_path.exists():
            project_config = self._load_yaml(self.project_config_path)
            if project_config:
                config = config.merge_with(UserConfig(**project_config))

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
        global_flat = UserConfig(**global_config).to_flat_dict() if global_config else {}
        project_flat = UserConfig(**project_config).to_flat_dict() if project_config else {}

        for key in VALID_CONFIG_KEYS:
            result["values"][key] = flat.get(key)

            # Determine source
            if self._is_from_env(key):
                result["sources"][key] = "env"
            elif key in project_flat and project_flat[key] != UserConfig().to_flat_dict().get(key):
                result["sources"][key] = "project"
            elif key in global_flat and global_flat[key] != UserConfig().to_flat_dict().get(key):
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
        mapping = ENV_VAR_MAPPING.get(key)
        if mapping is None:
            return False
        env_var, _ = mapping
        return os.environ.get(env_var) is not None

    def _apply_env_overrides(self, config: UserConfig) -> UserConfig:
        """Apply environment variable overrides to config.

        Supports all keys defined in ENV_VAR_MAPPING with proper type conversion.
        """
        data = config.model_dump()

        for config_key, (env_var, value_type) in ENV_VAR_MAPPING.items():
            env_value = os.environ.get(env_var)
            if env_value is None:
                continue

            # Parse value based on type
            parsed_value = self._parse_env_value(env_value, value_type)

            # Set nested value
            parts = config_key.split(".")
            if len(parts) == 1:
                data[parts[0]] = parsed_value
            else:
                section, field = parts[0], parts[1]
                if section not in data:
                    data[section] = {}
                data[section][field] = parsed_value

        return UserConfig(**data)

    def _parse_env_value(self, value: str, value_type: str) -> str | bool | int:
        """Parse environment variable value to appropriate type.

        Args:
            value: Raw string value from environment
            value_type: Expected type ("str", "bool", "int")

        Returns:
            Parsed value
        """
        if value_type == "bool":
            return value.lower() in ("true", "1", "yes", "on")
        elif value_type == "int":
            try:
                return int(value)
            except ValueError:
                return 0  # Default to 0 for invalid int
        else:
            return value


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
