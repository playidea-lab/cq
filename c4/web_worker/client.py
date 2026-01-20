"""HTTP Client for C4 Workspace API.

Provides async methods for file operations and shell commands
executed within a workspace sandbox.
"""

import json
from dataclasses import dataclass
from typing import Any

import httpx


class C4APIError(Exception):
    """Error from C4 API."""

    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.status_code = status_code


@dataclass
class ShellResult:
    """Result from shell command execution."""

    stdout: str
    stderr: str
    exit_code: int

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "stdout": self.stdout,
            "stderr": self.stderr,
            "exit_code": self.exit_code,
        }


@dataclass
class FileEntry:
    """File or directory entry."""

    name: str
    path: str
    is_directory: bool
    size: int | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "path": self.path,
            "is_directory": self.is_directory,
            "size": self.size,
        }


@dataclass
class SearchResult:
    """Search result entry."""

    path: str
    line_number: int | None = None
    line_content: str | None = None
    match: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "path": self.path,
            "line_number": self.line_number,
            "line_content": self.line_content,
            "match": self.match,
        }


class C4APIClient:
    """HTTP client for C4 Workspace API.

    Provides async methods to interact with workspace files and
    execute shell commands through the C4 API.
    """

    DEFAULT_TIMEOUT = 60.0
    MAX_SHELL_TIMEOUT = 300

    def __init__(
        self,
        base_url: str,
        auth_token: str | None = None,
        timeout: float = DEFAULT_TIMEOUT,
    ):
        """Initialize API client.

        Args:
            base_url: Base URL of C4 API (e.g., "http://localhost:8000")
            auth_token: Optional authentication token
            timeout: Default request timeout in seconds
        """
        self.base_url = base_url.rstrip("/")
        self.auth_token = auth_token
        self.timeout = timeout
        self._client: httpx.AsyncClient | None = None

    async def __aenter__(self) -> "C4APIClient":
        """Enter async context."""
        await self._ensure_client()
        return self

    async def __aexit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        """Exit async context."""
        await self.close()

    async def _ensure_client(self) -> httpx.AsyncClient:
        """Ensure HTTP client is initialized."""
        if self._client is None:
            headers = {}
            if self.auth_token:
                headers["Authorization"] = f"Bearer {self.auth_token}"
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                headers=headers,
                timeout=httpx.Timeout(self.timeout),
            )
        return self._client

    async def close(self) -> None:
        """Close the HTTP client."""
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    def _build_workspace_url(self, workspace_id: str, endpoint: str) -> str:
        """Build URL for workspace endpoint.

        Args:
            workspace_id: Workspace identifier
            endpoint: API endpoint path

        Returns:
            Full URL path
        """
        return f"/api/workspaces/{workspace_id}/{endpoint}"

    async def _handle_response(self, response: httpx.Response) -> dict[str, Any]:
        """Handle API response.

        Args:
            response: HTTP response

        Returns:
            Response JSON data

        Raises:
            C4APIError: If request failed
        """
        if response.status_code >= 400:
            try:
                error_data = response.json()
                message = error_data.get("detail", error_data.get("error", str(error_data)))
            except json.JSONDecodeError:
                message = response.text or f"HTTP {response.status_code}"
            raise C4APIError(message, response.status_code)

        try:
            return response.json()
        except json.JSONDecodeError:
            return {"content": response.text}

    async def read_file(self, workspace_id: str, path: str) -> str:
        """Read file contents from workspace.

        Args:
            workspace_id: Workspace identifier
            path: File path relative to workspace root

        Returns:
            File contents as string

        Raises:
            C4APIError: If file cannot be read
        """
        client = await self._ensure_client()
        url = self._build_workspace_url(workspace_id, "files/read")
        response = await client.post(url, json={"path": path})
        data = await self._handle_response(response)
        return data.get("content", "")

    async def write_file(self, workspace_id: str, path: str, content: str) -> dict[str, Any]:
        """Write content to file in workspace.

        Args:
            workspace_id: Workspace identifier
            path: File path relative to workspace root
            content: Content to write

        Returns:
            Response with status

        Raises:
            C4APIError: If file cannot be written
        """
        client = await self._ensure_client()
        url = self._build_workspace_url(workspace_id, "files/write")
        response = await client.post(url, json={"path": path, "content": content})
        return await self._handle_response(response)

    async def run_shell(
        self,
        workspace_id: str,
        command: str,
        timeout: int = 60,
    ) -> ShellResult:
        """Run shell command in workspace.

        Args:
            workspace_id: Workspace identifier
            command: Shell command to execute
            timeout: Timeout in seconds (max 300)

        Returns:
            ShellResult with stdout, stderr, exit_code

        Raises:
            C4APIError: If command execution fails
        """
        # Clamp timeout to max
        timeout = min(timeout, self.MAX_SHELL_TIMEOUT)

        client = await self._ensure_client()
        url = self._build_workspace_url(workspace_id, "shell/run")

        # Use longer timeout for shell commands
        response = await client.post(
            url,
            json={"command": command, "timeout": timeout},
            timeout=httpx.Timeout(timeout + 10),  # Extra buffer for network
        )
        data = await self._handle_response(response)

        return ShellResult(
            stdout=data.get("stdout", ""),
            stderr=data.get("stderr", ""),
            exit_code=data.get("exit_code", 0),
        )

    async def search_files(
        self,
        workspace_id: str,
        pattern: str,
        search_type: str,
        path: str = ".",
    ) -> list[SearchResult]:
        """Search for files or content in workspace.

        Args:
            workspace_id: Workspace identifier
            pattern: Search pattern (glob or regex)
            search_type: "glob" for file names, "grep" for content
            path: Directory to search in

        Returns:
            List of SearchResult entries

        Raises:
            C4APIError: If search fails
        """
        if search_type not in ("glob", "grep"):
            raise C4APIError(f"Invalid search_type: {search_type}")

        client = await self._ensure_client()
        url = self._build_workspace_url(workspace_id, "files/search")
        response = await client.post(
            url,
            json={
                "pattern": pattern,
                "search_type": search_type,
                "path": path,
            },
        )
        data = await self._handle_response(response)

        results = []
        for item in data.get("results", []):
            results.append(
                SearchResult(
                    path=item.get("path", ""),
                    line_number=item.get("line_number"),
                    line_content=item.get("line_content"),
                    match=item.get("match"),
                )
            )
        return results

    async def list_directory(
        self,
        workspace_id: str,
        path: str = ".",
        recursive: bool = False,
    ) -> list[FileEntry]:
        """List files and directories in workspace path.

        Args:
            workspace_id: Workspace identifier
            path: Directory path relative to workspace root
            recursive: Whether to list recursively

        Returns:
            List of FileEntry objects

        Raises:
            C4APIError: If directory cannot be listed
        """
        client = await self._ensure_client()
        url = self._build_workspace_url(workspace_id, "files/list")
        response = await client.post(
            url,
            json={"path": path, "recursive": recursive},
        )
        data = await self._handle_response(response)

        entries = []
        for item in data.get("entries", []):
            entries.append(
                FileEntry(
                    name=item.get("name", ""),
                    path=item.get("path", ""),
                    is_directory=item.get("is_directory", False),
                    size=item.get("size"),
                )
            )
        return entries

    async def health_check(self) -> bool:
        """Check if API is healthy.

        Returns:
            True if API is healthy
        """
        try:
            client = await self._ensure_client()
            response = await client.get("/health")
            return response.status_code == 200
        except Exception:
            return False
