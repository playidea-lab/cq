"""Tests for c4.web_worker.client module."""

import json
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from c4.web_worker.client import (
    C4APIClient,
    C4APIError,
    FileEntry,
    SearchResult,
    ShellResult,
)


class TestShellResult:
    """Test ShellResult dataclass."""

    def test_creation(self):
        """Should create ShellResult with all fields."""
        result = ShellResult(stdout="output", stderr="error", exit_code=0)
        assert result.stdout == "output"
        assert result.stderr == "error"
        assert result.exit_code == 0

    def test_to_dict(self):
        """Should convert to dictionary."""
        result = ShellResult(stdout="out", stderr="err", exit_code=1)
        d = result.to_dict()
        assert d == {"stdout": "out", "stderr": "err", "exit_code": 1}


class TestFileEntry:
    """Test FileEntry dataclass."""

    def test_creation(self):
        """Should create FileEntry with all fields."""
        entry = FileEntry(name="test.py", path="/src/test.py", is_directory=False, size=100)
        assert entry.name == "test.py"
        assert entry.path == "/src/test.py"
        assert entry.is_directory is False
        assert entry.size == 100

    def test_to_dict(self):
        """Should convert to dictionary."""
        entry = FileEntry(name="dir", path="/dir", is_directory=True)
        d = entry.to_dict()
        assert d["name"] == "dir"
        assert d["path"] == "/dir"
        assert d["is_directory"] is True
        assert d["size"] is None


class TestSearchResult:
    """Test SearchResult dataclass."""

    def test_creation(self):
        """Should create SearchResult with all fields."""
        result = SearchResult(path="/test.py", line_number=10, line_content="def test():", match="test")
        assert result.path == "/test.py"
        assert result.line_number == 10
        assert result.line_content == "def test():"
        assert result.match == "test"

    def test_to_dict(self):
        """Should convert to dictionary."""
        result = SearchResult(path="/a.py")
        d = result.to_dict()
        assert d["path"] == "/a.py"
        assert d["line_number"] is None


class TestC4APIError:
    """Test C4APIError exception."""

    def test_creation_with_message(self):
        """Should create error with message."""
        error = C4APIError("Not found")
        assert str(error) == "Not found"
        assert error.status_code is None

    def test_creation_with_status_code(self):
        """Should create error with status code."""
        error = C4APIError("Server error", status_code=500)
        assert str(error) == "Server error"
        assert error.status_code == 500


class TestC4APIClientInit:
    """Test C4APIClient initialization."""

    def test_init_with_base_url(self):
        """Should initialize with base URL."""
        client = C4APIClient("http://localhost:8000")
        assert client.base_url == "http://localhost:8000"
        assert client.auth_token is None
        assert client.timeout == C4APIClient.DEFAULT_TIMEOUT

    def test_init_strips_trailing_slash(self):
        """Should strip trailing slash from base URL."""
        client = C4APIClient("http://localhost:8000/")
        assert client.base_url == "http://localhost:8000"

    def test_init_with_auth_token(self):
        """Should initialize with auth token."""
        client = C4APIClient("http://localhost:8000", auth_token="test-token")
        assert client.auth_token == "test-token"

    def test_init_with_custom_timeout(self):
        """Should initialize with custom timeout."""
        client = C4APIClient("http://localhost:8000", timeout=30.0)
        assert client.timeout == 30.0


class TestC4APIClientBuildUrl:
    """Test URL building."""

    def test_build_workspace_url(self):
        """Should build correct workspace URL."""
        client = C4APIClient("http://localhost:8000")
        url = client._build_workspace_url("ws-123", "files/read")
        assert url == "/api/workspaces/ws-123/files/read"


@pytest.mark.asyncio
class TestC4APIClientMethods:
    """Test C4APIClient async methods."""

    async def test_read_file_success(self):
        """Should read file content."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"content": "file content"}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            content = await client.read_file("ws-123", "test.txt")
            assert content == "file content"
            mock_http.post.assert_called_once()

    async def test_read_file_not_found(self):
        """Should raise error when file not found."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 404
        mock_response.json.return_value = {"detail": "File not found"}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            with pytest.raises(C4APIError) as exc_info:
                await client.read_file("ws-123", "nonexistent.txt")
            assert exc_info.value.status_code == 404

    async def test_write_file_success(self):
        """Should write file content."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"success": True}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            result = await client.write_file("ws-123", "test.txt", "content")
            assert result["success"] is True

    async def test_run_shell_success(self):
        """Should run shell command."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"stdout": "output", "stderr": "", "exit_code": 0}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            result = await client.run_shell("ws-123", "ls -la")
            assert isinstance(result, ShellResult)
            assert result.stdout == "output"
            assert result.exit_code == 0

    async def test_run_shell_timeout_clamped(self):
        """Should clamp timeout to max value."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"stdout": "", "stderr": "", "exit_code": 0}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            # Request 600s timeout, should be clamped to 300
            await client.run_shell("ws-123", "sleep 1", timeout=600)

            call_args = mock_http.post.call_args
            assert call_args[1]["json"]["timeout"] == 300

    async def test_search_files_glob(self):
        """Should search files with glob pattern."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "results": [
                {"path": "/src/test.py", "line_number": None, "line_content": None, "match": None},
                {"path": "/src/test2.py", "line_number": None, "line_content": None, "match": None},
            ]
        }

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            results = await client.search_files("ws-123", "*.py", "glob")
            assert len(results) == 2
            assert all(isinstance(r, SearchResult) for r in results)
            assert results[0].path == "/src/test.py"

    async def test_search_files_grep(self):
        """Should search files with grep pattern."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "results": [
                {"path": "/src/test.py", "line_number": 10, "line_content": "def test():", "match": "test"},
            ]
        }

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            results = await client.search_files("ws-123", "def test", "grep", "/src")
            assert len(results) == 1
            assert results[0].line_number == 10

    async def test_search_files_invalid_type(self):
        """Should raise error for invalid search type."""
        client = C4APIClient("http://localhost:8000")

        with pytest.raises(C4APIError) as exc_info:
            await client.search_files("ws-123", "pattern", "invalid")
        assert "Invalid search_type" in str(exc_info.value)

    async def test_list_directory_success(self):
        """Should list directory contents."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "entries": [
                {"name": "src", "path": "/src", "is_directory": True, "size": None},
                {"name": "test.py", "path": "/test.py", "is_directory": False, "size": 100},
            ]
        }

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            entries = await client.list_directory("ws-123")
            assert len(entries) == 2
            assert all(isinstance(e, FileEntry) for e in entries)
            assert entries[0].is_directory is True
            assert entries[1].size == 100

    async def test_list_directory_recursive(self):
        """Should pass recursive flag."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"entries": []}

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.post = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            await client.list_directory("ws-123", "/src", recursive=True)

            call_args = mock_http.post.call_args
            assert call_args[1]["json"]["recursive"] is True

    async def test_health_check_healthy(self):
        """Should return True when API is healthy."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.get = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            result = await client.health_check()
            assert result is True

    async def test_health_check_unhealthy(self):
        """Should return False when API is unhealthy."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 503

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.get = AsyncMock(return_value=mock_response)
            mock_ensure.return_value = mock_http

            result = await client.health_check()
            assert result is False

    async def test_health_check_error(self):
        """Should return False on connection error."""
        client = C4APIClient("http://localhost:8000")

        with patch.object(client, "_ensure_client") as mock_ensure:
            mock_http = AsyncMock()
            mock_http.get = AsyncMock(side_effect=httpx.ConnectError("Connection refused"))
            mock_ensure.return_value = mock_http

            result = await client.health_check()
            assert result is False


@pytest.mark.asyncio
class TestC4APIClientContext:
    """Test C4APIClient context manager."""

    async def test_context_manager(self):
        """Should work as async context manager."""
        async with C4APIClient("http://localhost:8000") as client:
            assert client._client is not None

    async def test_close(self):
        """Should close client properly."""
        client = C4APIClient("http://localhost:8000")
        await client._ensure_client()
        assert client._client is not None

        await client.close()
        assert client._client is None


@pytest.mark.asyncio
class TestC4APIClientHandleResponse:
    """Test response handling."""

    async def test_handle_response_success_json(self):
        """Should parse JSON response."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"data": "value"}

        result = await client._handle_response(mock_response)
        assert result == {"data": "value"}

    async def test_handle_response_success_text(self):
        """Should handle non-JSON response."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.side_effect = json.JSONDecodeError("", "", 0)
        mock_response.text = "plain text"

        result = await client._handle_response(mock_response)
        assert result == {"content": "plain text"}

    async def test_handle_response_error_json(self):
        """Should extract error detail from JSON."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 400
        mock_response.json.return_value = {"detail": "Bad request"}

        with pytest.raises(C4APIError) as exc_info:
            await client._handle_response(mock_response)
        assert "Bad request" in str(exc_info.value)
        assert exc_info.value.status_code == 400

    async def test_handle_response_error_text(self):
        """Should use text for non-JSON errors."""
        client = C4APIClient("http://localhost:8000")

        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_response.json.side_effect = json.JSONDecodeError("", "", 0)
        mock_response.text = "Internal server error"

        with pytest.raises(C4APIError) as exc_info:
            await client._handle_response(mock_response)
        assert "Internal server error" in str(exc_info.value)
