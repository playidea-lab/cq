"""GitHub App Configuration.

Configuration for GitHub App integration including:
- App authentication (App ID, Private Key)
- Webhook verification (Webhook Secret)
- Review settings
"""

import os
from dataclasses import dataclass
from pathlib import Path

from pydantic import BaseModel, Field


class GitHubAppConfig(BaseModel):
    """GitHub App configuration for webhook handling and PR review.

    Environment variables (recommended for secrets):
    - GITHUB_APP_ID: App ID from GitHub App settings
    - GITHUB_APP_PRIVATE_KEY: Private key content (PEM format)
    - GITHUB_APP_PRIVATE_KEY_PATH: Path to private key file
    - GITHUB_WEBHOOK_SECRET: Webhook secret for signature verification

    Example in config.yaml:
        github_app:
          enabled: true
          app_id: "123456"
          webhook_secret_env: GITHUB_WEBHOOK_SECRET
          review:
            enabled: true
            max_diff_size: 50000
            auto_label: true
    """

    enabled: bool = Field(
        default=False,
        description="Enable GitHub App integration",
    )
    app_id: str | None = Field(
        default=None,
        description="GitHub App ID (or use GITHUB_APP_ID env)",
    )
    private_key: str | None = Field(
        default=None,
        description="Private key content in PEM format (or use GITHUB_APP_PRIVATE_KEY env)",
    )
    private_key_path: str | None = Field(
        default=None,
        description="Path to private key file (or use GITHUB_APP_PRIVATE_KEY_PATH env)",
    )
    webhook_secret: str | None = Field(
        default=None,
        description="Webhook secret for signature verification (or use GITHUB_WEBHOOK_SECRET env)",
    )

    # Review settings
    review_enabled: bool = Field(
        default=True,
        description="Enable automatic PR review",
    )
    max_diff_size: int = Field(
        default=50000,
        ge=1000,
        le=500000,
        description="Maximum diff size in bytes to review",
    )
    auto_label: bool = Field(
        default=True,
        description="Automatically add labels to PRs",
    )
    review_model: str = Field(
        default="claude-sonnet-4-20250514",
        description="LLM model for PR review",
    )

    def get_app_id(self) -> str | None:
        """Get App ID from config or environment."""
        return self.app_id or os.environ.get("GITHUB_APP_ID")

    def get_private_key(self) -> str | None:
        """Get private key from config, file, or environment.

        Priority:
        1. private_key field (direct content)
        2. private_key_path field (file path)
        3. GITHUB_APP_PRIVATE_KEY env (direct content)
        4. GITHUB_APP_PRIVATE_KEY_PATH env (file path)
        """
        # Direct content from config
        if self.private_key:
            return self.private_key

        # File path from config
        if self.private_key_path:
            key_path = Path(self.private_key_path).expanduser()
            if key_path.exists():
                return key_path.read_text()

        # Direct content from env
        env_key = os.environ.get("GITHUB_APP_PRIVATE_KEY")
        if env_key:
            return env_key

        # File path from env
        env_path = os.environ.get("GITHUB_APP_PRIVATE_KEY_PATH")
        if env_path:
            key_path = Path(env_path).expanduser()
            if key_path.exists():
                return key_path.read_text()

        return None

    def get_webhook_secret(self) -> str | None:
        """Get webhook secret from config or environment."""
        return self.webhook_secret or os.environ.get("GITHUB_WEBHOOK_SECRET")

    def is_configured(self) -> bool:
        """Check if all required settings are configured."""
        return bool(self.enabled and self.get_app_id() and self.get_private_key() and self.get_webhook_secret())


@dataclass
class GitHubAppCredentials:
    """Runtime credentials for GitHub App operations.

    This is a convenience class for passing validated credentials
    to the GitHub App client.
    """

    app_id: str
    private_key: str
    webhook_secret: str

    @classmethod
    def from_config(cls, config: GitHubAppConfig) -> "GitHubAppCredentials | None":
        """Create credentials from config, returning None if incomplete."""
        app_id = config.get_app_id()
        private_key = config.get_private_key()
        webhook_secret = config.get_webhook_secret()

        if not all([app_id, private_key, webhook_secret]):
            return None

        return cls(
            app_id=app_id,  # type: ignore
            private_key=private_key,  # type: ignore
            webhook_secret=webhook_secret,  # type: ignore
        )

    @classmethod
    def from_env(cls) -> "GitHubAppCredentials | None":
        """Create credentials from environment variables only."""
        config = GitHubAppConfig(enabled=True)
        return cls.from_config(config)
