"""C4 Configuration Management.

This module provides unified configuration management for C4 projects:
- Config models (Pydantic)
- Credentials management (API keys)
- Environment variable integration
"""

from .credentials import CredentialsManager
from .github_app import GitHubAppConfig, GitHubAppCredentials
from .manager import ConfigManager
from .models import (
    C4Config,
    GitConfig,
    TeamConfig,
    ValidationConfig,
    WorkerConfig,
)

__all__ = [
    "C4Config",
    "ConfigManager",
    "CredentialsManager",
    "GitConfig",
    "GitHubAppConfig",
    "GitHubAppCredentials",
    "TeamConfig",
    "ValidationConfig",
    "WorkerConfig",
]
