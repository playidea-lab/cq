"""C4 UI Server - Local web server for C4 dashboard."""

from pathlib import Path
from typing import Any

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, HTMLResponse
from fastapi.staticfiles import StaticFiles


class UIServer:
    """Local UI server for C4 dashboard."""

    DEFAULT_PORT = 4000
    DEFAULT_HOST = "localhost"

    def __init__(
        self,
        port: int = DEFAULT_PORT,
        host: str = DEFAULT_HOST,
        static_dir: Path | None = None,
    ):
        """Initialize UI server.

        Args:
            port: Server port (default: 4000)
            host: Server host (default: localhost)
            static_dir: Directory for static files
        """
        self.port = port
        self.host = host
        self.static_dir = static_dir or (Path(__file__).parent / "static")
        self.app = self._create_app()

    def _create_app(self) -> FastAPI:
        """Create FastAPI application for UI."""
        app = FastAPI(
            title="C4 UI",
            docs_url=None,  # Disable docs for UI server
            redoc_url=None,
        )

        # CORS
        app.add_middleware(
            CORSMiddleware,
            allow_origins=["*"],
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )

        # Import and include chat API
        from ..api.chat import router as chat_router

        app.include_router(chat_router, prefix="/api/chat", tags=["chat"])

        # API status endpoint
        @app.get("/api/status")
        async def api_status() -> dict[str, Any]:
            """Get C4 status."""
            try:
                from ..mcp_server import C4Daemon

                daemon = C4Daemon()
                if daemon.is_initialized():
                    daemon.load()
                    return daemon.c4_status()
                return {"initialized": False}
            except Exception as e:
                return {"error": str(e)}

        # Serve static files if directory exists
        if self.static_dir.exists():
            app.mount(
                "/static",
                StaticFiles(directory=self.static_dir),
                name="static",
            )

        # Serve index.html for SPA routing
        @app.get("/")
        async def index() -> HTMLResponse:
            """Serve main page."""
            return HTMLResponse(content=self._get_index_html())

        @app.get("/{path:path}", response_model=None)
        async def catch_all(path: str) -> HTMLResponse | FileResponse:
            """Catch-all for SPA routing."""
            # Try to serve static file first
            static_path = self.static_dir / path
            if static_path.exists() and static_path.is_file():
                return FileResponse(static_path)
            # Otherwise serve index for SPA
            return HTMLResponse(content=self._get_index_html())

        return app

    def _get_index_html(self) -> str:
        """Get index.html content."""
        index_path = self.static_dir / "index.html"
        if index_path.exists():
            return index_path.read_text()

        # Return embedded minimal UI
        return self._get_embedded_ui()

    def _get_embedded_ui(self) -> str:
        """Get embedded minimal UI when no static files exist."""
        return '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>C4 Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f172a;
            color: #e2e8f0;
            min-height: 100vh;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            padding: 2rem;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 1rem 0;
            border-bottom: 1px solid #334155;
            margin-bottom: 2rem;
        }
        h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: #f1f5f9;
        }
        .status {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.5rem 1rem;
            background: #1e293b;
            border-radius: 0.5rem;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #22c55e;
        }
        .status-dot.loading { background: #eab308; }
        .status-dot.error { background: #ef4444; }
        .card {
            background: #1e293b;
            border-radius: 0.75rem;
            padding: 1.5rem;
            margin-bottom: 1rem;
        }
        .card h2 {
            font-size: 1rem;
            color: #94a3b8;
            margin-bottom: 1rem;
        }
        .chat-container {
            display: flex;
            flex-direction: column;
            height: 400px;
        }
        .messages {
            flex: 1;
            overflow-y: auto;
            padding: 1rem;
            background: #0f172a;
            border-radius: 0.5rem;
            margin-bottom: 1rem;
        }
        .message {
            padding: 0.75rem;
            margin-bottom: 0.5rem;
            border-radius: 0.5rem;
            max-width: 80%;
        }
        .message.user {
            background: #3b82f6;
            margin-left: auto;
        }
        .message.assistant {
            background: #334155;
        }
        .input-area {
            display: flex;
            gap: 0.5rem;
        }
        input[type="text"] {
            flex: 1;
            padding: 0.75rem 1rem;
            background: #0f172a;
            border: 1px solid #334155;
            border-radius: 0.5rem;
            color: #e2e8f0;
            font-size: 1rem;
        }
        input[type="text"]:focus {
            outline: none;
            border-color: #3b82f6;
        }
        button {
            padding: 0.75rem 1.5rem;
            background: #3b82f6;
            color: white;
            border: none;
            border-radius: 0.5rem;
            font-size: 1rem;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover { background: #2563eb; }
        button:disabled {
            background: #475569;
            cursor: not-allowed;
        }
        .info {
            color: #94a3b8;
            font-size: 0.875rem;
        }
        .info a {
            color: #60a5fa;
            text-decoration: none;
        }
        .info a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>C4 Dashboard</h1>
            <div class="status">
                <span class="status-dot loading" id="status-dot"></span>
                <span id="status-text">Loading...</span>
            </div>
        </header>

        <div class="card">
            <h2>Chat</h2>
            <div class="chat-container">
                <div class="messages" id="messages"></div>
                <div class="input-area">
                    <input type="text" id="input" placeholder="Type a message..." />
                    <button id="send" onclick="sendMessage()">Send</button>
                </div>
            </div>
        </div>

        <div class="card">
            <p class="info">
                This is the C4 local dashboard. Connect to the
                <a href="/api/docs" target="_blank">API documentation</a>
                for more options.
            </p>
        </div>
    </div>

    <script>
        const messagesEl = document.getElementById('messages');
        const inputEl = document.getElementById('input');
        const sendBtn = document.getElementById('send');
        const statusDot = document.getElementById('status-dot');
        const statusText = document.getElementById('status-text');
        let conversationId = null;

        // Check status
        async function checkStatus() {
            try {
                const res = await fetch('/api/status');
                const data = await res.json();
                if (data.initialized) {
                    statusDot.classList.remove('loading', 'error');
                    statusText.textContent = data.status || 'Ready';
                } else if (data.error) {
                    statusDot.classList.remove('loading');
                    statusDot.classList.add('error');
                    statusText.textContent = 'Error';
                } else {
                    statusDot.classList.remove('loading', 'error');
                    statusText.textContent = 'Not initialized';
                }
            } catch (e) {
                statusDot.classList.remove('loading');
                statusDot.classList.add('error');
                statusText.textContent = 'Disconnected';
            }
        }

        // Add message to UI
        function addMessage(role, content) {
            const div = document.createElement('div');
            div.className = 'message ' + role;
            div.textContent = content;
            messagesEl.appendChild(div);
            messagesEl.scrollTop = messagesEl.scrollHeight;
        }

        // Send message
        async function sendMessage() {
            const message = inputEl.value.trim();
            if (!message) return;

            inputEl.value = '';
            sendBtn.disabled = true;
            addMessage('user', message);

            try {
                const res = await fetch('/api/chat/message', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        message,
                        conversation_id: conversationId,
                        stream: false
                    })
                });
                const data = await res.json();
                conversationId = data.conversation_id;
                addMessage('assistant', data.message.content);
            } catch (e) {
                addMessage('assistant', 'Error: ' + e.message);
            }

            sendBtn.disabled = false;
            inputEl.focus();
        }

        // Enter key to send
        inputEl.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') sendMessage();
        });

        // Initial status check
        checkStatus();
        setInterval(checkStatus, 5000);
    </script>
</body>
</html>'''

    def run(self) -> None:
        """Run the UI server."""
        import uvicorn

        uvicorn.run(
            self.app,
            host=self.host,
            port=self.port,
            log_level="info",
        )


def run_ui_server(
    port: int = UIServer.DEFAULT_PORT,
    host: str = UIServer.DEFAULT_HOST,
    open_browser: bool = True,
) -> None:
    """Run UI server with optional browser opening.

    Args:
        port: Server port
        host: Server host
        open_browser: Whether to open browser automatically
    """
    import threading
    import time
    import webbrowser

    server = UIServer(port=port, host=host)

    if open_browser:
        # Open browser after short delay
        def open_delayed() -> None:
            time.sleep(1)
            webbrowser.open(f"http://{host}:{port}")

        threading.Thread(target=open_delayed, daemon=True).start()

    server.run()
