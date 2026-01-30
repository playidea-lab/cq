"""C4 Configuration Management.

This module provides unified configuration management for C4 projects:
- Config models (Pydantic)
- Credentials management (API keys)
- Environment variable integration

Note:
    User-facing config models (UserConfig, UserValidationConfig) are defined here.
    System-internal config models (C4Config, ValidationConfig) are in c4.models.config.
"""

from .credentials import CredentialsManager
from .github_app import GitHubAppConfig, GitHubAppCredentials
from .manager import ConfigManager
from .models import (
    GitConfig,
    TeamConfig,
    UserConfig,
    UserValidationConfig,
    WorkerConfig,
)

# Backward compatibility aliases
C4Config = UserConfig
ValidationConfig = UserValidationConfig

__all__ = [
    # New names (preferred)
    "UserConfig",
    "UserValidationConfig",
    # Backward compatibility aliases
    "C4Config",
    "ValidationConfig",
    # Other exports
    "ConfigManager",
    "CredentialsManager",
    "GitConfig",
    "GitHubAppConfig",
    "GitHubAppCredentials",
    "TeamConfig",
    "WorkerConfig",
]
