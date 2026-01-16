"""C4 OAuth - Browser-based OAuth flow for CLI authentication."""

import http.server
import secrets
import socketserver
import threading
import urllib.parse
import webbrowser
from dataclasses import dataclass
from typing import Callable


@dataclass
class OAuthConfig:
    """OAuth configuration for Supabase."""

    supabase_url: str
    redirect_port: int = 8765
    redirect_path: str = "/auth/callback"
    scopes: list[str] | None = None

    @property
    def redirect_uri(self) -> str:
        """Get the redirect URI for OAuth callback."""
        return f"http://localhost:{self.redirect_port}{self.redirect_path}"


@dataclass
class OAuthResult:
    """Result of OAuth flow."""

    success: bool
    access_token: str | None = None
    refresh_token: str | None = None
    error: str | None = None
    raw_params: dict[str, str] | None = None


class OAuthCallbackHandler(http.server.BaseHTTPRequestHandler):
    """HTTP handler for OAuth callback."""

    result: OAuthResult | None = None
    expected_state: str | None = None
    callback_received: threading.Event | None = None

    def log_message(self, format: str, *args) -> None:  # noqa: A002
        """Suppress HTTP server logs."""
        pass

    def do_GET(self) -> None:
        """Handle GET request (OAuth callback)."""
        parsed = urllib.parse.urlparse(self.path)
        params = dict(urllib.parse.parse_qsl(parsed.query))

        # Handle Supabase callback
        if parsed.path == "/auth/callback":
            self._handle_callback(params)
        else:
            self.send_error(404)

    def _handle_callback(self, params: dict[str, str]) -> None:
        """Process OAuth callback parameters."""
        # Check for error
        if "error" in params:
            OAuthCallbackHandler.result = OAuthResult(
                success=False,
                error=params.get("error_description", params["error"]),
                raw_params=params,
            )
            self._send_response("Authentication failed. You can close this window.")
            return

        # Verify state if expected
        if OAuthCallbackHandler.expected_state:
            state = params.get("state")
            if state != OAuthCallbackHandler.expected_state:
                OAuthCallbackHandler.result = OAuthResult(
                    success=False,
                    error="State mismatch - possible CSRF attack",
                    raw_params=params,
                )
                self._send_response("Authentication failed. Invalid state.")
                return

        # Extract tokens
        # Supabase returns tokens in URL fragment, but for PKCE flow
        # they come as query parameters
        access_token = params.get("access_token")
        refresh_token = params.get("refresh_token")

        if access_token:
            OAuthCallbackHandler.result = OAuthResult(
                success=True,
                access_token=access_token,
                refresh_token=refresh_token,
                raw_params=params,
            )
            self._send_response(
                "Authentication successful! You can close this window."
            )
        else:
            # Tokens might be in fragment, need client-side handling
            OAuthCallbackHandler.result = OAuthResult(
                success=False,
                error="No access token received",
                raw_params=params,
            )
            self._send_response(
                "Authentication incomplete. Please check the console."
            )

        # Signal that callback was received
        if OAuthCallbackHandler.callback_received:
            OAuthCallbackHandler.callback_received.set()

    def _send_response(self, message: str) -> None:
        """Send HTML response to browser."""
        self.send_response(200)
        self.send_header("Content-type", "text/html")
        self.end_headers()

        html = f"""<!DOCTYPE html>
<html>
<head>
    <title>C4 Authentication</title>
    <style>
        body {{
            font-family: -apple-system, BlinkMacSystemFont, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: #f5f5f5;
        }}
        .container {{
            text-align: center;
            padding: 40px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }}
        h1 {{ color: #333; margin-bottom: 10px; }}
        p {{ color: #666; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>C4</h1>
        <p>{message}</p>
    </div>
</body>
</html>"""
        self.wfile.write(html.encode())


class OAuthFlow:
    """Manages the OAuth authentication flow."""

    def __init__(self, config: OAuthConfig):
        """Initialize OAuth flow.

        Args:
            config: OAuth configuration
        """
        self.config = config
        self.state = secrets.token_urlsafe(32)

    def get_authorization_url(self, provider: str = "github") -> str:
        """Get the authorization URL to redirect user to.

        Args:
            provider: OAuth provider (github, google)

        Returns:
            Authorization URL
        """
        params = {
            "provider": provider,
            "redirect_to": self.config.redirect_uri,
        }

        if self.config.scopes:
            params["scopes"] = " ".join(self.config.scopes)

        query = urllib.parse.urlencode(params)
        return f"{self.config.supabase_url}/auth/v1/authorize?{query}"

    def start_callback_server(
        self,
        timeout: int = 120,
    ) -> OAuthResult:
        """Start local server to receive OAuth callback.

        Args:
            timeout: Timeout in seconds

        Returns:
            OAuthResult with tokens or error
        """
        # Reset handler state
        OAuthCallbackHandler.result = None
        OAuthCallbackHandler.expected_state = self.state
        OAuthCallbackHandler.callback_received = threading.Event()

        # Start server
        with socketserver.TCPServer(
            ("localhost", self.config.redirect_port),
            OAuthCallbackHandler,
        ) as httpd:
            # Set timeout
            httpd.timeout = timeout

            # Wait for callback
            server_thread = threading.Thread(target=httpd.handle_request)
            server_thread.start()

            # Wait for callback or timeout
            callback_received = OAuthCallbackHandler.callback_received.wait(timeout)
            server_thread.join(timeout=1)

            if not callback_received:
                return OAuthResult(
                    success=False,
                    error="Authentication timed out",
                )

            return OAuthCallbackHandler.result or OAuthResult(
                success=False,
                error="No result received",
            )

    def run(
        self,
        provider: str = "github",
        open_browser: bool = True,
        on_waiting: Callable[[], None] | None = None,
    ) -> OAuthResult:
        """Run the complete OAuth flow.

        Args:
            provider: OAuth provider to use
            open_browser: Whether to automatically open browser
            on_waiting: Callback when waiting for user

        Returns:
            OAuthResult with tokens or error
        """
        auth_url = self.get_authorization_url(provider)

        if open_browser:
            webbrowser.open(auth_url)

        if on_waiting:
            on_waiting()

        return self.start_callback_server()
