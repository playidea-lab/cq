"""C4 User Configuration Models.

Pydantic models for user-facing C4 configuration with support for:
- Global config (~/.c4/config.yaml)
- Project config (.c4/config.yaml)
- Environment variable overrides

Note:
    This module defines the user-configuration layer (git, validation commands,
    worker settings, team settings). For system-internal configuration models
    (LLM, store, github integration, worktree, etc.), see c4.models.config.

    Class naming:
    - UserConfig: User-facing configuration (this module)
    - C4Config: System-internal configuration (c4.models.config)
"""

from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


class GitConfig(BaseModel):
    """Git configuration for auto-commit and PR."""

    user: str | None = Field(default=None, description="Git commit author name")
    email: str | None = Field(default=None, description="Git commit author email")
    auto_commit: bool = Field(default=True, description="Auto-commit on task completion")
    auto_pr: bool = Field(default=True, description="Auto-create PR on project completion")


class UserValidationConfig(BaseModel):
    """Validation settings for lint, test, typecheck (user-facing).

    Note: This is distinct from c4.models.config.ValidationConfig which
    defines system-internal validation command configuration.
    """

    lint: bool = Field(default=True, description="Run linting")
    test: bool = Field(default=True, description="Run tests")
    typecheck: bool = Field(default=True, description="Run type checking")


class WorkerConfig(BaseModel):
    """Worker execution settings."""

    max_retries: int = Field(default=3, ge=0, le=10, description="Max retry attempts on failure")
    timeout: int = Field(default=300, ge=30, le=3600, description="Task timeout in seconds")


class TeamConfig(BaseModel):
    """Team settings for C4 Cloud."""

    id: str | None = Field(default=None, description="Team ID")
    name: str | None = Field(default=None, description="Team name")


class UserConfig(BaseModel):
    """Main user-facing C4 configuration model.

    Supports hierarchical config:
    - Environment variables (highest priority)
    - Project config (.c4/config.yaml)
    - Global config (~/.c4/config.yaml)
    - Default values (lowest priority)

    Note: This is distinct from c4.models.config.C4Config which defines
    system-internal configuration (LLM, store, agents, etc.).
    """

    platform: Literal["claude", "cursor", "codex", "gemini", "opencode"] = Field(
        default="claude",
        description="Default platform",
    )
    git: GitConfig = Field(default_factory=GitConfig)
    validation: UserValidationConfig = Field(default_factory=UserValidationConfig)
    worker: WorkerConfig = Field(default_factory=WorkerConfig)
    team: TeamConfig = Field(default_factory=TeamConfig)

    def merge_with(self, other: "UserConfig") -> "UserConfig":
        """Merge with another config (other takes precedence for non-None values).

        Args:
            other: Config to merge with (higher priority)

        Returns:
            Merged config
        """
        merged_data: dict[str, Any] = {}

        # Platform
        merged_data["platform"] = other.platform if other.platform != "claude" else self.platform

        # Git
        merged_data["git"] = {
            "user": other.git.user or self.git.user,
            "email": other.git.email or self.git.email,
            "auto_commit": other.git.auto_commit,
            "auto_pr": other.git.auto_pr,
        }

        # Validation
        merged_data["validation"] = {
            "lint": other.validation.lint,
            "test": other.validation.test,
            "typecheck": other.validation.typecheck,
        }

        # Worker
        merged_data["worker"] = {
            "max_retries": other.worker.max_retries,
            "timeout": other.worker.timeout,
        }

        # Team
        merged_data["team"] = {
            "id": other.team.id or self.team.id,
            "name": other.team.name or self.team.name,
        }

        return UserConfig(**merged_data)

    def to_flat_dict(self) -> dict[str, Any]:
        """Convert to flat dictionary with dot notation keys.

        Returns:
            Flat dictionary like {"git.user": "value", "git.email": "value"}
        """
        flat: dict[str, Any] = {"platform": self.platform}

        # Git
        flat["git.user"] = self.git.user
        flat["git.email"] = self.git.email
        flat["git.auto-commit"] = self.git.auto_commit
        flat["git.auto-pr"] = self.git.auto_pr

        # Validation
        flat["validation.lint"] = self.validation.lint
        flat["validation.test"] = self.validation.test
        flat["validation.typecheck"] = self.validation.typecheck

        # Worker
        flat["worker.max-retries"] = self.worker.max_retries
        flat["worker.timeout"] = self.worker.timeout

        # Team
        flat["team.id"] = self.team.id
        flat["team.name"] = self.team.name

        return flat

    @classmethod
    def from_flat_dict(cls, flat: dict[str, Any]) -> "UserConfig":
        """Create from flat dictionary with dot notation keys.

        Args:
            flat: Flat dictionary like {"git.user": "value"}

        Returns:
            UserConfig instance
        """
        data: dict[str, Any] = {}

        # Platform
        if "platform" in flat:
            data["platform"] = flat["platform"]

        # Git
        git_data: dict[str, Any] = {}
        if "git.user" in flat:
            git_data["user"] = flat["git.user"]
        if "git.email" in flat:
            git_data["email"] = flat["git.email"]
        if "git.auto-commit" in flat:
            git_data["auto_commit"] = flat["git.auto-commit"]
        if "git.auto-pr" in flat:
            git_data["auto_pr"] = flat["git.auto-pr"]
        if git_data:
            data["git"] = git_data

        # Validation
        validation_data: dict[str, Any] = {}
        if "validation.lint" in flat:
            validation_data["lint"] = flat["validation.lint"]
        if "validation.test" in flat:
            validation_data["test"] = flat["validation.test"]
        if "validation.typecheck" in flat:
            validation_data["typecheck"] = flat["validation.typecheck"]
        if validation_data:
            data["validation"] = validation_data

        # Worker
        worker_data: dict[str, Any] = {}
        if "worker.max-retries" in flat:
            worker_data["max_retries"] = flat["worker.max-retries"]
        if "worker.timeout" in flat:
            worker_data["timeout"] = flat["worker.timeout"]
        if worker_data:
            data["worker"] = worker_data

        # Team
        team_data: dict[str, Any] = {}
        if "team.id" in flat:
            team_data["id"] = flat["team.id"]
        if "team.name" in flat:
            team_data["name"] = flat["team.name"]
        if team_data:
            data["team"] = team_data

        return cls(**data)


# Valid config keys for CLI
VALID_CONFIG_KEYS = [
    "platform",
    "git.user",
    "git.email",
    "git.auto-commit",
    "git.auto-pr",
    "validation.lint",
    "validation.test",
    "validation.typecheck",
    "worker.max-retries",
    "worker.timeout",
    "team.id",
    "team.name",
]

# Keys that accept boolean values
BOOLEAN_KEYS = {
    "git.auto-commit",
    "git.auto-pr",
    "validation.lint",
    "validation.test",
    "validation.typecheck",
}

# Keys that accept integer values
INTEGER_KEYS = {
    "worker.max-retries",
    "worker.timeout",
}


def parse_config_value(key: str, value: str) -> str | bool | int:
    """Parse a config value from string to appropriate type.

    Args:
        key: Config key
        value: String value to parse

    Returns:
        Parsed value (str, bool, or int)

    Raises:
        ValueError: If value is invalid for the key type
    """
    if key in BOOLEAN_KEYS:
        lower = value.lower()
        if lower in ("true", "1", "yes", "on"):
            return True
        elif lower in ("false", "0", "no", "off"):
            return False
        else:
            raise ValueError(f"Invalid boolean value for {key}: {value}")

    if key in INTEGER_KEYS:
        try:
            return int(value)
        except ValueError:
            raise ValueError(f"Invalid integer value for {key}: {value}") from None

    return value
