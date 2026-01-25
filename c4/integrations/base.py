"""Integration Provider Base Classes.

Defines the abstract interface for all integration providers (GitHub, Discord, Dooray, etc.).
This enables a unified approach to managing external service connections.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class IntegrationCategory(str, Enum):
    """Categories of integration providers."""

    SOURCE_CONTROL = "source_control"  # GitHub, GitLab, Bitbucket
    MESSAGING = "messaging"  # Discord, Slack
    COLLABORATION = "collaboration"  # Dooray, Notion, Jira
    CI_CD = "ci_cd"  # Jenkins, CircleCI, GitHub Actions


class IntegrationCapability(str, Enum):
    """Capabilities that an integration provider can support."""

    PR_REVIEW = "pr_review"  # Can review pull requests
    NOTIFICATIONS = "notifications"  # Can send notifications
    COMMANDS = "commands"  # Can receive slash commands
    WEBHOOKS = "webhooks"  # Can receive webhooks
    OAUTH = "oauth"  # Supports OAuth flow
    BOT = "bot"  # Has bot functionality


@dataclass
class ConnectionResult:
    """Result of connecting an integration."""

    success: bool
    message: str
    external_id: str | None = None
    external_name: str | None = None
    credentials: dict[str, Any] | None = None
    error_code: str | None = None


@dataclass
class NotificationResult:
    """Result of sending a notification."""

    success: bool
    message: str
    message_id: str | None = None


@dataclass
class WebhookEvent:
    """Parsed webhook event from a provider."""

    event_type: str
    external_id: str  # installation_id, guild_id, etc.
    action: str | None = None
    data: dict[str, Any] = field(default_factory=dict)
    raw_payload: dict[str, Any] = field(default_factory=dict)


@dataclass
class MergeRequestInfo:
    """Provider-agnostic merge/pull request information.

    This unified data class abstracts GitHub PRs and GitLab MRs,
    allowing services to work with both providers.
    """

    provider: str  # "github" | "gitlab"
    owner: str  # Organization/user (GitHub) or namespace (GitLab)
    repo: str  # Repository name
    number: int  # PR number (GitHub) or MR IID (GitLab)
    title: str
    head_sha: str  # Latest commit SHA
    base_branch: str  # Target branch
    head_branch: str  # Source branch
    author: str  # Author username
    diff_url: str  # URL to get diff
    project_id: int | str  # installation_id (GitHub) or project_id (GitLab)


@dataclass
class IntegrationInfo:
    """Information about an integration provider."""

    id: str
    name: str
    category: IntegrationCategory
    capabilities: list[IntegrationCapability]
    description: str
    icon_url: str | None = None
    docs_url: str | None = None
    oauth_url: str | None = None
    webhook_path: str | None = None


class IntegrationProvider(ABC):
    """Abstract base class for all integration providers.

    Each provider (GitHub, Discord, Dooray, etc.) must implement this interface
    to be usable in the C4 integration system.

    Example:
        @IntegrationRegistry.register
        class GitHubProvider(IntegrationProvider):
            @property
            def id(self) -> str:
                return "github"

            @property
            def name(self) -> str:
                return "GitHub"

            # ... implement other methods
    """

    # =========================================================================
    # Provider Identity
    # =========================================================================

    @property
    @abstractmethod
    def id(self) -> str:
        """Unique provider identifier (e.g., 'github', 'discord').

        This ID is used for routing webhooks and storing integration data.
        """
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """Human-readable provider name (e.g., 'GitHub', 'Discord')."""
        pass

    @property
    @abstractmethod
    def category(self) -> IntegrationCategory:
        """Category of this provider."""
        pass

    @property
    @abstractmethod
    def capabilities(self) -> list[IntegrationCapability]:
        """List of capabilities this provider supports."""
        pass

    @property
    def description(self) -> str:
        """Description of what this provider does."""
        return ""

    @property
    def icon_url(self) -> str | None:
        """URL to provider icon."""
        return None

    @property
    def docs_url(self) -> str | None:
        """URL to provider documentation."""
        return None

    def get_info(self) -> IntegrationInfo:
        """Get complete information about this provider."""
        return IntegrationInfo(
            id=self.id,
            name=self.name,
            category=self.category,
            capabilities=self.capabilities,
            description=self.description,
            icon_url=self.icon_url,
            docs_url=self.docs_url,
            oauth_url=self.get_oauth_url(""),
            webhook_path=f"/webhooks/{self.id}",
        )

    # =========================================================================
    # OAuth Flow
    # =========================================================================

    @abstractmethod
    def get_oauth_url(self, state: str) -> str:
        """Get OAuth authorization URL.

        Args:
            state: State parameter for CSRF protection.
                   Should contain encoded workspace_id and user_id.

        Returns:
            Full OAuth authorization URL to redirect user to.
        """
        pass

    @abstractmethod
    async def exchange_code(self, code: str, state: str) -> ConnectionResult:
        """Exchange OAuth authorization code for tokens.

        Args:
            code: Authorization code from OAuth callback
            state: State parameter for verification

        Returns:
            ConnectionResult with credentials if successful
        """
        pass

    # =========================================================================
    # Connection Management
    # =========================================================================

    @abstractmethod
    async def connect(
        self,
        workspace_id: str,
        credentials: dict[str, Any],
    ) -> ConnectionResult:
        """Connect an integration to a workspace.

        Args:
            workspace_id: C4 workspace ID
            credentials: Credentials from OAuth or manual setup

        Returns:
            ConnectionResult with connection details
        """
        pass

    @abstractmethod
    async def disconnect(
        self,
        workspace_id: str,
        external_id: str,
    ) -> bool:
        """Disconnect an integration from a workspace.

        Args:
            workspace_id: C4 workspace ID
            external_id: External service ID (installation_id, guild_id, etc.)

        Returns:
            True if disconnected successfully
        """
        pass

    @abstractmethod
    async def validate_connection(
        self,
        credentials: dict[str, Any],
    ) -> bool:
        """Validate that credentials are still valid.

        Args:
            credentials: Stored credentials to validate

        Returns:
            True if credentials are valid
        """
        pass

    # =========================================================================
    # Notifications
    # =========================================================================

    @abstractmethod
    async def send_notification(
        self,
        credentials: dict[str, Any],
        message: str,
        *,
        channel_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> NotificationResult:
        """Send a notification through this provider.

        Args:
            credentials: Provider credentials
            message: Message content (may include markdown)
            channel_id: Target channel/room ID (provider-specific)
            metadata: Additional provider-specific options

        Returns:
            NotificationResult with status
        """
        pass

    # =========================================================================
    # Webhooks
    # =========================================================================

    @abstractmethod
    async def verify_webhook(
        self,
        payload: bytes,
        headers: dict[str, str],
        secret: str,
    ) -> bool:
        """Verify webhook signature.

        Args:
            payload: Raw request body
            headers: Request headers
            secret: Webhook secret for verification

        Returns:
            True if signature is valid
        """
        pass

    @abstractmethod
    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
    ) -> WebhookEvent | None:
        """Parse webhook payload into a WebhookEvent.

        Args:
            payload: Parsed JSON payload
            headers: Request headers

        Returns:
            WebhookEvent if parseable, None otherwise
        """
        pass

    # =========================================================================
    # Optional: Provider-Specific Features
    # =========================================================================

    async def refresh_token(
        self,
        credentials: dict[str, Any],
    ) -> dict[str, Any] | None:
        """Refresh OAuth tokens if supported.

        Args:
            credentials: Current credentials with refresh_token

        Returns:
            Updated credentials or None if refresh not supported/failed
        """
        return None

    async def get_channels(
        self,
        credentials: dict[str, Any],
    ) -> list[dict[str, str]]:
        """Get available channels/rooms for notifications.

        Args:
            credentials: Provider credentials

        Returns:
            List of {id, name} dicts
        """
        return []


class SourceControlProvider(IntegrationProvider):
    """Base class for source control providers (GitHub, GitLab, etc.).

    Adds PR review specific methods.
    """

    @property
    def category(self) -> IntegrationCategory:
        return IntegrationCategory.SOURCE_CONTROL

    @abstractmethod
    async def get_pr_diff(
        self,
        credentials: dict[str, Any],
        owner: str,
        repo: str,
        pr_number: int,
    ) -> str | None:
        """Get PR diff content.

        Args:
            credentials: Provider credentials
            owner: Repository owner
            repo: Repository name
            pr_number: PR number

        Returns:
            Diff content as string
        """
        pass

    @abstractmethod
    async def create_review(
        self,
        credentials: dict[str, Any],
        owner: str,
        repo: str,
        pr_number: int,
        body: str,
        event: str = "COMMENT",
        comments: list[dict[str, Any]] | None = None,
    ) -> bool:
        """Create a PR review.

        Args:
            credentials: Provider credentials
            owner: Repository owner
            repo: Repository name
            pr_number: PR number
            body: Review body
            event: Review event (COMMENT, APPROVE, REQUEST_CHANGES)
            comments: Line comments

        Returns:
            True if successful
        """
        pass


class MessagingProvider(IntegrationProvider):
    """Base class for messaging providers (Discord, Slack, etc.).

    Adds messaging-specific methods.
    """

    @property
    def category(self) -> IntegrationCategory:
        return IntegrationCategory.MESSAGING

    @abstractmethod
    async def send_embed(
        self,
        credentials: dict[str, Any],
        channel_id: str,
        title: str,
        description: str,
        *,
        color: int | None = None,
        fields: list[dict[str, str]] | None = None,
        footer: str | None = None,
    ) -> NotificationResult:
        """Send a rich embed message.

        Args:
            credentials: Provider credentials
            channel_id: Target channel ID
            title: Embed title
            description: Embed description
            color: Embed color (integer)
            fields: List of {name, value, inline} dicts
            footer: Footer text

        Returns:
            NotificationResult with status
        """
        pass

    @abstractmethod
    async def add_reaction(
        self,
        credentials: dict[str, Any],
        channel_id: str,
        message_id: str,
        emoji: str,
    ) -> bool:
        """Add a reaction to a message.

        Args:
            credentials: Provider credentials
            channel_id: Channel ID
            message_id: Message ID
            emoji: Emoji to add

        Returns:
            True if successful
        """
        pass
