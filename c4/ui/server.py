"""C4 UI Server - FastAPI web server for local UI."""

import asyncio
import signal
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import FileResponse, HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

# Default port for UI server
DEFAULT_PORT = 4000
DEFAULT_HOST = "127.0.0.1"


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup/shutdown."""
    # Startup
    yield
    # Shutdown


def create_ui_app(
    project_root: Path | None = None,
    static_dir: Path | None = None,
    title: str = "C4 Dashboard",
) -> FastAPI:
    """Create the FastAPI application for UI server.

    Args:
        project_root: Project root directory for status API
        static_dir: Directory to serve static files from
        title: Application title

    Returns:
        Configured FastAPI application
    """
    app = FastAPI(
        title=title,
        description="C4 Local UI Server",
        version="0.1.0",
        lifespan=lifespan,
    )

    # Store project root in app state
    app.state.project_root = project_root or Path.cwd()

    # Mount static files if directory exists
    if static_dir and static_dir.exists():
        app.mount(
            "/static",
            StaticFiles(directory=str(static_dir)),
            name="static",
        )

    # Register routes
    _register_routes(app)

    return app


def _register_routes(app: FastAPI) -> None:
    """Register all routes for the UI application."""

    @app.get("/", response_class=HTMLResponse)
    async def root() -> HTMLResponse:
        """Serve the main UI page."""
        html_content = _get_default_html()
        return HTMLResponse(content=html_content)

    @app.get("/health")
    async def health() -> dict[str, str]:
        """Health check endpoint."""
        return {"status": "healthy", "service": "c4-ui"}

    @app.get("/api/status")
    async def api_status(request: Request) -> JSONResponse:
        """Get C4 project status."""
        try:
            from c4.mcp_server import C4Daemon

            project_root = request.app.state.project_root

            # Set project root for daemon
            import os
            old_root = os.environ.get("C4_PROJECT_ROOT")
            os.environ["C4_PROJECT_ROOT"] = str(project_root)

            try:
                daemon = C4Daemon()
                if not daemon.is_initialized():
                    return JSONResponse(
                        {"error": "C4 not initialized"},
                        status_code=404,
                    )

                daemon.load()
                status = daemon.c4_status()
                return JSONResponse(status)
            finally:
                if old_root is not None:
                    os.environ["C4_PROJECT_ROOT"] = old_root
                elif "C4_PROJECT_ROOT" in os.environ:
                    del os.environ["C4_PROJECT_ROOT"]

        except Exception as e:
            return JSONResponse(
                {"error": str(e)},
                status_code=500,
            )

    @app.get("/api/tasks")
    async def api_tasks(request: Request) -> JSONResponse:
        """Get task queue information."""
        try:
            from c4.mcp_server import C4Daemon

            project_root = request.app.state.project_root

            import os
            old_root = os.environ.get("C4_PROJECT_ROOT")
            os.environ["C4_PROJECT_ROOT"] = str(project_root)

            try:
                daemon = C4Daemon()
                if not daemon.is_initialized():
                    return JSONResponse(
                        {"error": "C4 not initialized"},
                        status_code=404,
                    )

                daemon.load()
                queue = daemon.task_queue

                tasks = {
                    "pending": [t.model_dump() for t in queue.pending],
                    "in_progress": [
                        {"task_id": tid, "worker_id": wid}
                        for tid, wid in queue.in_progress.items()
                    ],
                    "done": [t.model_dump() for t in queue.done],
                }
                return JSONResponse(tasks)
            finally:
                if old_root is not None:
                    os.environ["C4_PROJECT_ROOT"] = old_root
                elif "C4_PROJECT_ROOT" in os.environ:
                    del os.environ["C4_PROJECT_ROOT"]

        except Exception as e:
            return JSONResponse(
                {"error": str(e)},
                status_code=500,
            )

    @app.get("/api/workers")
    async def api_workers(request: Request) -> JSONResponse:
        """Get worker information."""
        try:
            from c4.mcp_server import C4Daemon

            project_root = request.app.state.project_root

            import os
            old_root = os.environ.get("C4_PROJECT_ROOT")
            os.environ["C4_PROJECT_ROOT"] = str(project_root)

            try:
                daemon = C4Daemon()
                if not daemon.is_initialized():
                    return JSONResponse(
                        {"error": "C4 not initialized"},
                        status_code=404,
                    )

                daemon.load()
                workers = daemon.worker_manager.get_all_workers()
                return JSONResponse({"workers": workers})
            finally:
                if old_root is not None:
                    os.environ["C4_PROJECT_ROOT"] = old_root
                elif "C4_PROJECT_ROOT" in os.environ:
                    del os.environ["C4_PROJECT_ROOT"]

        except Exception as e:
            return JSONResponse(
                {"error": str(e)},
                status_code=500,
            )


def _get_default_html() -> str:
    """Generate default HTML page for UI."""
    return """<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>C4 Dashboard</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0d1117;
            color: #c9d1d9;
            min-height: 100vh;
            padding: 2rem;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 {
            color: #58a6ff;
            margin-bottom: 1.5rem;
            font-size: 2rem;
        }
        .status-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 1.5rem;
            margin-bottom: 1.5rem;
        }
        .status-card h2 {
            color: #8b949e;
            font-size: 0.875rem;
            text-transform: uppercase;
            margin-bottom: 1rem;
        }
        .status-value {
            font-size: 1.5rem;
            font-weight: 600;
        }
        .status-value.execute { color: #3fb950; }
        .status-value.plan { color: #58a6ff; }
        .status-value.halted { color: #f85149; }
        .status-value.checkpoint { color: #d29922; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1.5rem; }
        .task-list {
            list-style: none;
            max-height: 300px;
            overflow-y: auto;
        }
        .task-item {
            padding: 0.75rem;
            border-bottom: 1px solid #30363d;
            font-family: monospace;
        }
        .task-item:last-child { border-bottom: none; }
        .badge {
            display: inline-block;
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
        }
        .badge.pending { background: #388bfd33; color: #58a6ff; }
        .badge.progress { background: #d2992233; color: #d29922; }
        .badge.done { background: #3fb95033; color: #3fb950; }
        .refresh-btn {
            background: #21262d;
            border: 1px solid #30363d;
            color: #c9d1d9;
            padding: 0.5rem 1rem;
            border-radius: 6px;
            cursor: pointer;
            float: right;
        }
        .refresh-btn:hover { background: #30363d; }
        .error { color: #f85149; }
        .loading { color: #8b949e; }
    </style>
</head>
<body>
    <div class="container">
        <h1>C4 Dashboard <button class="refresh-btn" onclick="loadData()">Refresh</button></h1>

        <div class="grid">
            <div class="status-card">
                <h2>Project Status</h2>
                <div id="status" class="loading">Loading...</div>
            </div>

            <div class="status-card">
                <h2>Task Queue</h2>
                <div id="queue" class="loading">Loading...</div>
            </div>
        </div>

        <div class="status-card">
            <h2>Pending Tasks</h2>
            <ul id="pending-tasks" class="task-list">
                <li class="loading">Loading...</li>
            </ul>
        </div>

        <div class="status-card">
            <h2>In Progress</h2>
            <ul id="progress-tasks" class="task-list">
                <li class="loading">Loading...</li>
            </ul>
        </div>

        <div class="status-card">
            <h2>Completed Tasks</h2>
            <ul id="done-tasks" class="task-list">
                <li class="loading">Loading...</li>
            </ul>
        </div>
    </div>

    <script>
        async function loadData() {
            try {
                // Load status
                const statusRes = await fetch('/api/status');
                const status = await statusRes.json();

                if (status.error) {
                    document.getElementById('status').innerHTML =
                        '<span class="error">' + status.error + '</span>';
                } else {
                    const statusClass = status.status.toLowerCase();
                    document.getElementById('status').innerHTML =
                        '<div class="status-value ' + statusClass + '">' + status.status + '</div>' +
                        '<div style="margin-top: 0.5rem; color: #8b949e;">' +
                        'Project: ' + status.project_id + '</div>';

                    document.getElementById('queue').innerHTML =
                        '<div>Pending: <strong>' + status.queue.pending + '</strong></div>' +
                        '<div>In Progress: <strong>' + status.queue.in_progress + '</strong></div>' +
                        '<div>Done: <strong>' + status.queue.done + '</strong></div>';
                }

                // Load tasks
                const tasksRes = await fetch('/api/tasks');
                const tasks = await tasksRes.json();

                if (tasks.error) {
                    ['pending-tasks', 'progress-tasks', 'done-tasks'].forEach(id => {
                        document.getElementById(id).innerHTML =
                            '<li class="error">' + tasks.error + '</li>';
                    });
                } else {
                    // Pending
                    const pendingHtml = tasks.pending.length ? tasks.pending.map(t =>
                        '<li class="task-item"><span class="badge pending">PENDING</span> ' +
                        t.id + ' - ' + t.title + '</li>'
                    ).join('') : '<li class="task-item" style="color: #8b949e;">No pending tasks</li>';
                    document.getElementById('pending-tasks').innerHTML = pendingHtml;

                    // In Progress
                    const progressHtml = tasks.in_progress.length ? tasks.in_progress.map(t =>
                        '<li class="task-item"><span class="badge progress">IN PROGRESS</span> ' +
                        t.task_id + ' (Worker: ' + t.worker_id + ')</li>'
                    ).join('') : '<li class="task-item" style="color: #8b949e;">No tasks in progress</li>';
                    document.getElementById('progress-tasks').innerHTML = progressHtml;

                    // Done
                    const doneHtml = tasks.done.length ? tasks.done.slice(-10).reverse().map(t =>
                        '<li class="task-item"><span class="badge done">DONE</span> ' +
                        t.id + ' - ' + t.title + '</li>'
                    ).join('') : '<li class="task-item" style="color: #8b949e;">No completed tasks</li>';
                    document.getElementById('done-tasks').innerHTML = doneHtml;
                }
            } catch (err) {
                document.getElementById('status').innerHTML =
                    '<span class="error">Failed to load: ' + err.message + '</span>';
            }
        }

        // Initial load
        loadData();

        // Auto-refresh every 5 seconds
        setInterval(loadData, 5000);
    </script>
</body>
</html>"""


def run_ui_server(
    host: str = DEFAULT_HOST,
    port: int = DEFAULT_PORT,
    project_root: Path | None = None,
    static_dir: Path | None = None,
    reload: bool = False,
) -> None:
    """Run the UI server.

    Args:
        host: Host to bind to
        port: Port to listen on
        project_root: Project root directory
        static_dir: Static files directory
        reload: Enable auto-reload for development
    """
    app = create_ui_app(
        project_root=project_root,
        static_dir=static_dir,
    )

    config = uvicorn.Config(
        app,
        host=host,
        port=port,
        reload=reload,
        log_level="info",
    )

    server = uvicorn.Server(config)
    server.run()


async def run_ui_server_async(
    host: str = DEFAULT_HOST,
    port: int = DEFAULT_PORT,
    project_root: Path | None = None,
    static_dir: Path | None = None,
) -> None:
    """Run the UI server asynchronously.

    Args:
        host: Host to bind to
        port: Port to listen on
        project_root: Project root directory
        static_dir: Static files directory
    """
    app = create_ui_app(
        project_root=project_root,
        static_dir=static_dir,
    )

    config = uvicorn.Config(
        app,
        host=host,
        port=port,
        log_level="info",
    )

    server = uvicorn.Server(config)
    await server.serve()
