"""Discord Integration Provider.

Implements the IntegrationProvider interface for Discord Bot integration.
Provides notifications, slash commands, and interactive approvals.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import logging
import os
from typing import Any
from urllib.parse import urlencode
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

from .base import (
    ConnectionResult,
    IntegrationCapability,
    MessagingProvider,
    NotificationResult,
    WebhookEvent,
)
from .registry import IntegrationRegistry

logger = logging.getLogger(__name__)


@IntegrationRegistry.register
class DiscordProvider(MessagingProvider):
    """Discord Bot integration provider.

    Supports:
    - OAuth2 Bot authorization flow
    - Channel notifications (embeds, messages)
    - Slash commands
    - Interactive components (buttons, selects)

    Environment Variables:
        DISCORD_CLIENT_ID: Discord application client ID
        DISCORD_CLIENT_SECRET: Discord application client secret
        DISCORD_BOT_TOKEN: Bot token for API calls
        DISCORD_PUBLIC_KEY: Public key for interaction verification
    """

    OAUTH_AUTHORIZE_URL = "https://discord.com/api/oauth2/authorize"
    OAUTH_TOKEN_URL = "https://discord.com/api/oauth2/token"
    API_BASE = "https://discord.com/api/v10"

    # Bot permissions for C4 functionality
    # 2048 = Send Messages
    # 16384 = Embed Links
    # 32768 = Attach Files
    # 262144 = Add Reactions
    # 2147483648 = Use Slash Commands
    BOT_PERMISSIONS = 2048 | 16384 | 32768 | 262144 | 2147483648

    def __init__(self) -> None:
        """Initialize Discord provider."""
        self._bot_token: str | None = None

    # =========================================================================
    # Provider Identity
    # =========================================================================

    @property
    def id(self) -> str:
        return "discord"

    @property
    def name(self) -> str:
        return "Discord"

    @property
    def capabilities(self) -> list[IntegrationCapability]:
        return [
            IntegrationCapability.NOTIFICATIONS,
            IntegrationCapability.COMMANDS,
            IntegrationCapability.WEBHOOKS,
            IntegrationCapability.OAUTH,
            IntegrationCapability.BOT,
        ]

    @property
    def description(self) -> str:
        return "Discord bot for notifications, slash commands, and checkpoint approvals"

    @property
    def icon_url(self) -> str | None:
        return "https://assets-global.website-files.com/6257adef93867e50d84d30e2/636e0a6a49cf127bf92de1e2_icon_clyde_blurple_RGB.png"

    @property
    def docs_url(self) -> str | None:
        return "https://discord.com/developers/docs"

    # =========================================================================
    # Configuration
    # =========================================================================

    def _get_client_id(self) -> str | None:
        """Get Discord client ID from environment."""
        return os.environ.get("DISCORD_CLIENT_ID")

    def _get_client_secret(self) -> str | None:
        """Get Discord client secret from environment."""
        return os.environ.get("DISCORD_CLIENT_SECRET")

    def _get_bot_token(self) -> str | None:
        """Get Discord bot token from environment."""
        if self._bot_token is None:
            self._bot_token = os.environ.get("DISCORD_BOT_TOKEN")
        return self._bot_token

    def _get_public_key(self) -> str | None:
        """Get Discord public key for interaction verification."""
        return os.environ.get("DISCORD_PUBLIC_KEY")

    # =========================================================================
    # OAuth Flow
    # =========================================================================

    def get_oauth_url(self, state: str) -> str:
        """Get Discord OAuth2 authorization URL.

        Args:
            state: State parameter (contains team_id, user_id)

        Returns:
            OAuth authorization URL
        """
        client_id = self._get_client_id()
        redirect_uri = os.environ.get(
            "DISCORD_REDIRECT_URI",
            "http://localhost:8000/api/integrations/discord/oauth/callback"
        )

        params = {
            "client_id": client_id,
            "permissions": str(self.BOT_PERMISSIONS),
            "scope": "bot applications.commands",
            "response_type": "code",
            "redirect_uri": redirect_uri,
            "state": state,
        }

        return f"{self.OAUTH_AUTHORIZE_URL}?{urlencode(params)}"

    async def exchange_code(self, code: str, state: str) -> ConnectionResult:
        """Exchange OAuth code for bot token and guild info.

        For Discord bots, the authorization adds the bot to a guild.
        The code exchange gives us information about the guild.

        Args:
            code: Authorization code from callback
            state: State parameter for verification

        Returns:
            ConnectionResult with guild details
        """
        client_id = self._get_client_id()
        client_secret = self._get_client_secret()
        redirect_uri = os.environ.get(
            "DISCORD_REDIRECT_URI",
            "http://localhost:8000/api/integrations/discord/oauth/callback"
        )

        if not all([client_id, client_secret]):
            return ConnectionResult(
                success=False,
                message="Discord OAuth not configured",
                error_code="not_configured",
            )

        # Exchange code for token (to get guild info)
        try:
            data = {
                "client_id": client_id,
                "client_secret": client_secret,
                "grant_type": "authorization_code",
                "code": code,
                "redirect_uri": redirect_uri,
            }

            request = Request(
                self.OAUTH_TOKEN_URL,
                data=urlencode(data).encode(),
                headers={"Content-Type": "application/x-www-form-urlencoded"},
                method="POST",
            )

            with urlopen(request, timeout=30) as response:
                token_data = json.loads(response.read())

            # Extract guild info
            guild = token_data.get("guild", {})
            guild_id = guild.get("id")
            guild_name = guild.get("name")

            if not guild_id:
                return ConnectionResult(
                    success=False,
                    message="No guild information in response",
                    error_code="no_guild",
                )

            return ConnectionResult(
                success=True,
                message="Discord bot added to server",
                external_id=guild_id,
                external_name=guild_name,
                credentials={
                    "guild_id": guild_id,
                    "guild_name": guild_name,
                    # We use bot token from env, not OAuth token
                },
            )

        except HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            logger.error(f"Discord OAuth exchange failed: {e.code} {error_body}")
            return ConnectionResult(
                success=False,
                message=f"OAuth exchange failed: {error_body}",
                error_code="exchange_failed",
            )

        except Exception as e:
            logger.error(f"Discord OAuth exchange error: {e}")
            return ConnectionResult(
                success=False,
                message=str(e),
                error_code="exchange_error",
            )

    # =========================================================================
    # Connection Management
    # =========================================================================

    async def connect(
        self,
        workspace_id: str,
        credentials: dict[str, Any],
    ) -> ConnectionResult:
        """Connect Discord guild to workspace.

        Args:
            workspace_id: C4 workspace/team ID
            credentials: Guild credentials

        Returns:
            ConnectionResult
        """
        guild_id = credentials.get("guild_id")
        if not guild_id:
            return ConnectionResult(
                success=False,
                message="Missing guild_id in credentials",
                error_code="missing_guild_id",
            )

        return ConnectionResult(
            success=True,
            message="Discord connected",
            external_id=guild_id,
            external_name=credentials.get("guild_name", guild_id),
            credentials=credentials,
        )

    async def disconnect(
        self,
        workspace_id: str,
        external_id: str,
    ) -> bool:
        """Disconnect Discord integration.

        Note: This doesn't remove the bot from the server,
        just removes the association in C4.

        Args:
            workspace_id: C4 workspace/team ID
            external_id: Guild ID

        Returns:
            True (always succeeds on our side)
        """
        logger.info(f"Disconnecting Discord guild {external_id} from workspace {workspace_id}")
        return True

    async def validate_connection(
        self,
        credentials: dict[str, Any],
    ) -> bool:
        """Validate Discord connection is still active.

        Args:
            credentials: Stored credentials

        Returns:
            True if bot is still in guild
        """
        guild_id = credentials.get("guild_id")
        if not guild_id:
            return False

        bot_token = self._get_bot_token()
        if not bot_token:
            return False

        try:
            # Try to get guild info
            request = Request(
                f"{self.API_BASE}/guilds/{guild_id}",
                headers={
                    "Authorization": f"Bot {bot_token}",
                },
            )

            with urlopen(request, timeout=10) as response:
                return response.status == 200

        except Exception as e:
            logger.warning(f"Discord connection validation failed: {e}")
            return False

    # =========================================================================
    # Notifications
    # =========================================================================

    async def send_notification(
        self,
        credentials: dict[str, Any],
        message: str,
        *,
        channel_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> NotificationResult:
        """Send a notification to a Discord channel.

        Args:
            credentials: Guild credentials
            message: Message content (markdown supported)
            channel_id: Discord channel ID
            metadata: Additional options (embed, components)

        Returns:
            NotificationResult
        """
        if not channel_id:
            # Try to get default channel from settings
            channel_id = credentials.get("default_channel_id")
            if not channel_id:
                return NotificationResult(
                    success=False,
                    message="No channel_id specified",
                )

        bot_token = self._get_bot_token()
        if not bot_token:
            return NotificationResult(
                success=False,
                message="Discord bot not configured",
            )

        # Build message payload
        payload: dict[str, Any] = {"content": message}

        # Add embed if provided
        if metadata and metadata.get("embed"):
            payload["embeds"] = [metadata["embed"]]

        # Add components (buttons) if provided
        if metadata and metadata.get("components"):
            payload["components"] = metadata["components"]

        try:
            request = Request(
                f"{self.API_BASE}/channels/{channel_id}/messages",
                data=json.dumps(payload).encode(),
                headers={
                    "Authorization": f"Bot {bot_token}",
                    "Content-Type": "application/json",
                },
                method="POST",
            )

            with urlopen(request, timeout=30) as response:
                data = json.loads(response.read())

                return NotificationResult(
                    success=True,
                    message="Message sent",
                    message_id=data.get("id"),
                )

        except HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            logger.error(f"Discord send message failed: {e.code} {error_body}")
            return NotificationResult(
                success=False,
                message=f"Failed to send message: {error_body}",
            )

        except Exception as e:
            logger.error(f"Discord send error: {e}")
            return NotificationResult(
                success=False,
                message=str(e),
            )

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
            credentials: Guild credentials
            channel_id: Target channel ID
            title: Embed title
            description: Embed description
            color: Embed color (integer)
            fields: List of {name, value, inline} dicts
            footer: Footer text

        Returns:
            NotificationResult
        """
        embed: dict[str, Any] = {
            "title": title,
            "description": description,
        }

        if color is not None:
            embed["color"] = color

        if fields:
            embed["fields"] = [
                {
                    "name": f["name"],
                    "value": f["value"],
                    "inline": f.get("inline", False),
                }
                for f in fields
            ]

        if footer:
            embed["footer"] = {"text": footer}

        return await self.send_notification(
            credentials,
            "",  # No text content, just embed
            channel_id=channel_id,
            metadata={"embed": embed},
        )

    async def add_reaction(
        self,
        credentials: dict[str, Any],
        channel_id: str,
        message_id: str,
        emoji: str,
    ) -> bool:
        """Add a reaction to a message.

        Args:
            credentials: Guild credentials
            channel_id: Channel ID
            message_id: Message ID
            emoji: Emoji to add (URL encoded for custom)

        Returns:
            True if successful
        """
        bot_token = self._get_bot_token()
        if not bot_token:
            return False

        try:
            # URL encode emoji for custom emojis
            encoded_emoji = emoji.replace(":", "%3A") if ":" in emoji else emoji

            request = Request(
                f"{self.API_BASE}/channels/{channel_id}/messages/{message_id}/reactions/{encoded_emoji}/@me",
                headers={
                    "Authorization": f"Bot {bot_token}",
                },
                method="PUT",
            )

            with urlopen(request, timeout=10) as response:
                return response.status == 204

        except Exception as e:
            logger.error(f"Discord add reaction failed: {e}")
            return False

    # =========================================================================
    # Channel Management
    # =========================================================================

    async def get_channels(
        self,
        credentials: dict[str, Any],
    ) -> list[dict[str, str]]:
        """Get available text channels in the guild.

        Args:
            credentials: Guild credentials

        Returns:
            List of {id, name} dicts
        """
        guild_id = credentials.get("guild_id")
        bot_token = self._get_bot_token()

        if not guild_id or not bot_token:
            return []

        try:
            request = Request(
                f"{self.API_BASE}/guilds/{guild_id}/channels",
                headers={
                    "Authorization": f"Bot {bot_token}",
                },
            )

            with urlopen(request, timeout=30) as response:
                channels = json.loads(response.read())

                # Filter to text channels only (type 0)
                return [
                    {"id": ch["id"], "name": ch["name"]}
                    for ch in channels
                    if ch.get("type") == 0
                ]

        except Exception as e:
            logger.error(f"Discord get channels failed: {e}")
            return []

    # =========================================================================
    # Webhooks
    # =========================================================================

    async def verify_webhook(
        self,
        payload: bytes,
        headers: dict[str, str],
        secret: str,
    ) -> bool:
        """Verify Discord interaction signature.

        Discord uses Ed25519 signatures for interactions.

        Args:
            payload: Raw request body
            headers: Request headers
            secret: Public key (not secret, but consistent API)

        Returns:
            True if signature is valid
        """
        signature = headers.get("x-signature-ed25519", headers.get("X-Signature-Ed25519", ""))
        timestamp = headers.get("x-signature-timestamp", headers.get("X-Signature-Timestamp", ""))

        if not signature or not timestamp:
            return False

        public_key = self._get_public_key() or secret

        try:
            from nacl.signing import VerifyKey
            from nacl.exceptions import BadSignature

            verify_key = VerifyKey(bytes.fromhex(public_key))
            message = timestamp.encode() + payload

            verify_key.verify(message, bytes.fromhex(signature))
            return True

        except ImportError:
            logger.warning("PyNaCl not installed, cannot verify Discord signatures")
            return False

        except BadSignature:
            return False

        except Exception as e:
            logger.error(f"Discord signature verification error: {e}")
            return False

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
    ) -> WebhookEvent | None:
        """Parse Discord interaction into WebhookEvent.

        Args:
            payload: Parsed JSON payload
            headers: Request headers

        Returns:
            WebhookEvent or None
        """
        interaction_type = payload.get("type")
        guild_id = payload.get("guild_id")

        if not guild_id:
            return None

        # Map interaction types
        event_type_map = {
            1: "ping",
            2: "application_command",
            3: "message_component",
            4: "autocomplete",
            5: "modal_submit",
        }

        event_type = event_type_map.get(interaction_type, f"unknown_{interaction_type}")

        # Extract action from command name or component custom_id
        action = None
        if interaction_type == 2:  # Application command
            action = payload.get("data", {}).get("name")
        elif interaction_type == 3:  # Message component
            action = payload.get("data", {}).get("custom_id")

        return WebhookEvent(
            event_type=event_type,
            external_id=guild_id,
            action=action,
            data=payload,
            raw_payload=payload,
        )
