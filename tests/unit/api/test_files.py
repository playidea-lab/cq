"""Tests for File Operations API Routes."""

import tempfile
from pathlib import Path
from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient

from c4.api.routes.files import (
    PathSecurityError,
    WorkspaceManager,
    set_workspace_manager,
    validate_path,
)


@pytest.fixture
def temp_workspace():
    """Create a temporary workspace directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def workspace_manager(temp_workspace):
    """Create a workspace manager using temp directory."""
    manager = WorkspaceManager(base_path=temp_workspace)
    return manager


@pytest.fixture
def mock_daemon():
    """Create a mock C4Daemon."""
    from unittest.mock import MagicMock

    daemon = MagicMock()
    daemon.is_initialized.return_value = True
    daemon.c4_status.return_value = {
        "state": "EXECUTE",
        "queue": {},
        "workers": {},
        "project_root": "/test/project",
    }
    daemon.config = {"verifications": {}}
    return daemon


@pytest.fixture
def client(mock_daemon, workspace_manager):
    """Create test client with mocked daemon and workspace manager."""
    with patch("c4.api.deps.get_daemon_singleton", return_value=mock_daemon):
        # Set the workspace manager
        set_workspace_manager(workspace_manager)

        from c4.api.server import create_app

        app = create_app()
        yield TestClient(app)


class TestPathSecurity:
    """Tests for path security validation."""

    def test_validate_path_normal(self, temp_workspace):
        """Test normal path validation."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        # Normal path should work
        result = validate_path("file.txt", workspace_root)
        # Use resolve() to handle symlinks (e.g., /var -> /private/var on macOS)
        assert result == (workspace_root / "file.txt").resolve()

    def test_validate_path_nested(self, temp_workspace):
        """Test nested path validation."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        result = validate_path("dir/subdir/file.txt", workspace_root)
        # Use resolve() to handle symlinks (e.g., /var -> /private/var on macOS)
        assert result == (workspace_root / "dir" / "subdir" / "file.txt").resolve()

    def test_validate_path_traversal_blocked(self, temp_workspace):
        """Test that path traversal is blocked."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        with pytest.raises(PathSecurityError) as exc_info:
            validate_path("../secret.txt", workspace_root)
        assert "path traversal" in str(exc_info.value).lower()

    def test_validate_path_double_traversal_blocked(self, temp_workspace):
        """Test that double path traversal is blocked."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        with pytest.raises(PathSecurityError) as exc_info:
            validate_path("dir/../../secret.txt", workspace_root)
        assert "path traversal" in str(exc_info.value).lower()

    def test_validate_path_absolute_blocked(self, temp_workspace):
        """Test that absolute paths are blocked."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        with pytest.raises(PathSecurityError) as exc_info:
            validate_path("/etc/passwd", workspace_root)
        assert "absolute" in str(exc_info.value).lower()

    def test_validate_path_empty_blocked(self, temp_workspace):
        """Test that empty paths are blocked."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        with pytest.raises(PathSecurityError) as exc_info:
            validate_path("", workspace_root)
        assert "empty" in str(exc_info.value).lower()

    def test_validate_path_hidden_dotdot(self, temp_workspace):
        """Test that hidden .. patterns are blocked."""
        workspace_root = temp_workspace / "test-workspace"
        workspace_root.mkdir()

        # Various attempts to hide path traversal
        dangerous_paths = [
            "foo/..bar/../secret.txt",  # Contains ..
            "foo/../bar",  # Classic traversal
        ]

        for path in dangerous_paths:
            with pytest.raises(PathSecurityError):
                validate_path(path, workspace_root)


class TestWorkspaceManager:
    """Tests for WorkspaceManager."""

    def test_get_workspace_root_creates_dir(self, temp_workspace):
        """Test that get_workspace_root creates workspace directory."""
        manager = WorkspaceManager(base_path=temp_workspace)
        root = manager.get_workspace_root("my-workspace")

        assert root.exists()
        assert root.is_dir()
        assert root == temp_workspace / "my-workspace"

    def test_get_workspace_root_invalid_id(self, temp_workspace):
        """Test that invalid workspace_id is rejected."""
        manager = WorkspaceManager(base_path=temp_workspace)

        with pytest.raises(ValueError):
            manager.get_workspace_root("../escape")

        with pytest.raises(ValueError):
            manager.get_workspace_root("path/with/slash")

        with pytest.raises(ValueError):
            manager.get_workspace_root("")


class TestReadFileEndpoint:
    """Tests for POST /api/files/read endpoint."""

    def test_read_file_success(self, client, workspace_manager):
        """Test successful file read."""
        # Setup: create a file in workspace
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        test_file = workspace_root / "test.txt"
        test_file.write_text("Hello, World!")

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "test.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["content"] == "Hello, World!"
        assert data["size"] == 13
        assert data["path"] == "test.txt"

    def test_read_file_not_found(self, client):
        """Test reading non-existent file."""
        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "nonexistent.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not found" in data["error"].lower()

    def test_read_file_is_directory(self, client, workspace_manager):
        """Test reading a directory fails."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "mydir").mkdir()

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "mydir"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not a file" in data["error"].lower()

    def test_read_file_path_traversal_blocked(self, client):
        """Test that path traversal is blocked."""
        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "../secret.txt"},
        )

        assert response.status_code == 400
        assert "traversal" in response.json()["detail"].lower()

    def test_read_file_nested_path(self, client, workspace_manager):
        """Test reading file in nested directory."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        nested_dir = workspace_root / "dir1" / "dir2"
        nested_dir.mkdir(parents=True)
        test_file = nested_dir / "nested.txt"
        test_file.write_text("Nested content")

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "dir1/dir2/nested.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["content"] == "Nested content"


class TestWriteFileEndpoint:
    """Tests for POST /api/files/write endpoint."""

    def test_write_file_success(self, client, workspace_manager):
        """Test successful file write."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "new_file.txt",
                "content": "New content",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["size"] == 11

        # Verify file was written
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        assert (workspace_root / "new_file.txt").read_text() == "New content"

    def test_write_file_creates_dirs(self, client, workspace_manager):
        """Test that parent directories are created."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "a/b/c/deep.txt",
                "content": "Deep content",
                "create_dirs": True,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        workspace_root = workspace_manager.get_workspace_root("test-ws")
        assert (workspace_root / "a" / "b" / "c" / "deep.txt").exists()

    def test_write_file_no_create_dirs_fails(self, client):
        """Test that write fails when create_dirs is False and dir doesn't exist."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "nonexistent_dir/file.txt",
                "content": "Content",
                "create_dirs": False,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "parent directory" in data["error"].lower()

    def test_write_file_overwrite(self, client, workspace_manager):
        """Test overwriting existing file."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        test_file = workspace_root / "existing.txt"
        test_file.write_text("Original")

        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "existing.txt",
                "content": "Updated",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert test_file.read_text() == "Updated"

    def test_write_file_path_traversal_blocked(self, client):
        """Test that path traversal is blocked on write."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "../escape.txt",
                "content": "Malicious",
            },
        )

        assert response.status_code == 400
        assert "traversal" in response.json()["detail"].lower()


class TestListDirectoryEndpoint:
    """Tests for POST /api/files/list endpoint."""

    def test_list_directory_success(self, client, workspace_manager):
        """Test successful directory listing."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "file1.txt").write_text("content1")
        (workspace_root / "file2.txt").write_text("content2")
        (workspace_root / "subdir").mkdir()

        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "."},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["entries"]) == 3

        # Directories should come first
        names = [e["name"] for e in data["entries"]]
        assert "subdir" in names
        assert "file1.txt" in names
        assert "file2.txt" in names

    def test_list_directory_recursive(self, client, workspace_manager):
        """Test recursive directory listing."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "file1.txt").write_text("content1")
        subdir = workspace_root / "subdir"
        subdir.mkdir()
        (subdir / "nested.txt").write_text("nested")

        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": ".", "recursive": True},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        paths = [e["path"] for e in data["entries"]]
        assert any("nested.txt" in p for p in paths)

    def test_list_directory_hidden_files(self, client, workspace_manager):
        """Test hidden files are excluded by default."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "visible.txt").write_text("visible")
        (workspace_root / ".hidden").write_text("hidden")

        # Without include_hidden
        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "."},
        )

        data = response.json()
        names = [e["name"] for e in data["entries"]]
        assert "visible.txt" in names
        assert ".hidden" not in names

        # With include_hidden
        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": ".", "include_hidden": True},
        )

        data = response.json()
        names = [e["name"] for e in data["entries"]]
        assert "visible.txt" in names
        assert ".hidden" in names

    def test_list_directory_not_found(self, client):
        """Test listing non-existent directory."""
        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "nonexistent"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not found" in data["error"].lower()

    def test_list_directory_is_file(self, client, workspace_manager):
        """Test listing a file fails."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "file.txt").write_text("content")

        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "file.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not a directory" in data["error"].lower()


class TestSearchFilesEndpoint:
    """Tests for POST /api/files/search endpoint."""

    def test_search_glob_success(self, client, workspace_manager):
        """Test successful glob search."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "file1.txt").write_text("content1")
        (workspace_root / "file2.txt").write_text("content2")
        (workspace_root / "data.json").write_text("{}")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "*.txt",
                "path": ".",
                "search_type": "glob",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 2

        paths = [m["path"] for m in data["matches"]]
        assert "file1.txt" in paths
        assert "file2.txt" in paths

    def test_search_grep_success(self, client, workspace_manager):
        """Test successful grep search."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "file1.txt").write_text("Hello World")
        (workspace_root / "file2.txt").write_text("Goodbye World")
        (workspace_root / "file3.txt").write_text("Nothing here")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "World",
                "path": ".",
                "search_type": "grep",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 2

        for match in data["matches"]:
            assert match["line_number"] == 1
            assert "World" in match["line_content"]

    def test_search_grep_regex(self, client, workspace_manager):
        """Test grep search with regex pattern."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "code.py").write_text("def hello():\n    pass\ndef world():\n    pass")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": r"def \w+\(\):",
                "path": ".",
                "search_type": "grep",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 2

    def test_search_grep_invalid_regex(self, client, workspace_manager):
        """Test grep search with invalid regex."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "[invalid",
                "path": ".",
                "search_type": "grep",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "invalid regex" in data["error"].lower()

    def test_search_max_results(self, client, workspace_manager):
        """Test that search respects max_results."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        for i in range(10):
            (workspace_root / f"file{i}.txt").write_text(f"content{i}")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "*.txt",
                "path": ".",
                "search_type": "glob",
                "max_results": 5,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 5
        assert data["truncated"] is True

    def test_search_invalid_type(self, client, workspace_manager):
        """Test search with invalid search_type."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "test",
                "path": ".",
                "search_type": "invalid",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "invalid search_type" in data["error"].lower()

    def test_search_path_not_found(self, client):
        """Test search in non-existent path."""
        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "*.txt",
                "path": "nonexistent",
                "search_type": "glob",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not found" in data["error"].lower()


class TestDeleteFileEndpoint:
    """Tests for DELETE /api/files/delete endpoint."""

    def test_delete_file_success(self, client, workspace_manager):
        """Test successful file deletion."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        test_file = workspace_root / "to_delete.txt"
        test_file.write_text("delete me")
        assert test_file.exists()

        response = client.request(
            "DELETE",
            "/api/files/delete",
            json={"workspace_id": "test-ws", "path": "to_delete.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert not test_file.exists()

    def test_delete_file_not_found(self, client):
        """Test deleting non-existent file."""
        response = client.request(
            "DELETE",
            "/api/files/delete",
            json={"workspace_id": "test-ws", "path": "nonexistent.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not found" in data["error"].lower()

    def test_delete_directory_fails(self, client, workspace_manager):
        """Test that deleting a directory fails."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "mydir").mkdir()

        response = client.request(
            "DELETE",
            "/api/files/delete",
            json={"workspace_id": "test-ws", "path": "mydir"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "not a file" in data["error"].lower()

    def test_delete_path_traversal_blocked(self, client):
        """Test that path traversal is blocked on delete."""
        response = client.request(
            "DELETE",
            "/api/files/delete",
            json={"workspace_id": "test-ws", "path": "../escape.txt"},
        )

        assert response.status_code == 400
        assert "traversal" in response.json()["detail"].lower()


class TestInvalidWorkspaceId:
    """Tests for invalid workspace_id handling."""

    def test_read_invalid_workspace_id(self, client):
        """Test read with invalid workspace_id."""
        response = client.post(
            "/api/files/read",
            json={"workspace_id": "../escape", "path": "file.txt"},
        )

        assert response.status_code == 400

    def test_write_invalid_workspace_id(self, client):
        """Test write with invalid workspace_id."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "invalid/id",
                "path": "file.txt",
                "content": "test",
            },
        )

        assert response.status_code == 400

    def test_list_invalid_workspace_id(self, client):
        """Test list with invalid workspace_id."""
        response = client.post(
            "/api/files/list",
            json={"workspace_id": "", "path": "."},
        )

        assert response.status_code == 400


class TestEdgeCases:
    """Tests for edge cases and special scenarios."""

    def test_read_empty_file(self, client, workspace_manager):
        """Test reading empty file."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "empty.txt").write_text("")

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "empty.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["content"] == ""
        assert data["size"] == 0

    def test_write_empty_content(self, client, workspace_manager):
        """Test writing empty content."""
        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "empty.txt",
                "content": "",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        workspace_root = workspace_manager.get_workspace_root("test-ws")
        assert (workspace_root / "empty.txt").read_text() == ""

    def test_read_unicode_content(self, client, workspace_manager):
        """Test reading unicode content."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        unicode_content = "Hello 世界 Привет مرحبا"
        (workspace_root / "unicode.txt").write_text(unicode_content)

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "unicode.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["content"] == unicode_content

    def test_write_unicode_content(self, client, workspace_manager):
        """Test writing unicode content."""
        unicode_content = "Hello 世界 Привет مرحبا"

        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "unicode.txt",
                "content": unicode_content,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        workspace_root = workspace_manager.get_workspace_root("test-ws")
        assert (workspace_root / "unicode.txt").read_text() == unicode_content

    def test_list_empty_directory(self, client, workspace_manager):
        """Test listing empty directory."""
        workspace_manager.get_workspace_root("test-ws")  # Just create workspace

        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "."},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["entries"] == []

    def test_search_empty_directory(self, client, workspace_manager):
        """Test searching empty directory."""
        workspace_manager.get_workspace_root("test-ws")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "*.txt",
                "path": ".",
                "search_type": "glob",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["matches"] == []

    def test_special_characters_in_filename(self, client, workspace_manager):
        """Test file operations with special characters in filename."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        special_name = "file with spaces & special.txt"
        (workspace_root / special_name).write_text("content")

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": special_name},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["content"] == "content"

    def test_read_binary_file_fails(self, client, workspace_manager):
        """Test reading binary file fails gracefully."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        binary_file = workspace_root / "binary.bin"
        binary_file.write_bytes(b"\x00\x01\x02\x03\xff\xfe\xfd")

        response = client.post(
            "/api/files/read",
            json={"workspace_id": "test-ws", "path": "binary.bin"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is False
        assert "utf-8" in data["error"].lower()

    def test_search_grep_skips_binary_files(self, client, workspace_manager):
        """Test grep search skips binary files with invalid UTF-8."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "text.txt").write_text("findme")
        binary_file = workspace_root / "binary.bin"
        # Use bytes that are definitely invalid UTF-8 (invalid continuation bytes)
        binary_file.write_bytes(b"\x80\x81\x82findme\xff\xfe")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "findme",
                "path": ".",
                "search_type": "grep",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        # Should only find the text file, not the binary
        assert len(data["matches"]) == 1
        assert data["matches"][0]["path"] == "text.txt"

    def test_search_glob_nested(self, client, workspace_manager):
        """Test glob search in nested directories."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        nested = workspace_root / "a" / "b" / "c"
        nested.mkdir(parents=True)
        (nested / "deep.txt").write_text("deep content")
        (workspace_root / "shallow.txt").write_text("shallow content")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "*.txt",
                "path": ".",
                "search_type": "glob",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 2
        paths = [m["path"] for m in data["matches"]]
        assert any("deep.txt" in p for p in paths)
        assert any("shallow.txt" in p for p in paths)

    def test_list_directory_with_file_sizes(self, client, workspace_manager):
        """Test that file sizes are returned correctly."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "small.txt").write_text("hi")
        (workspace_root / "larger.txt").write_text("a" * 1000)
        (workspace_root / "subdir").mkdir()

        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": "."},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        entries_by_name = {e["name"]: e for e in data["entries"]}
        assert entries_by_name["small.txt"]["size"] == 2
        assert entries_by_name["larger.txt"]["size"] == 1000
        assert entries_by_name["subdir"]["size"] is None  # Directories have no size

    def test_search_grep_multiple_matches_per_file(self, client, workspace_manager):
        """Test grep search with multiple matches in same file."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        (workspace_root / "multi.txt").write_text("match1\nno\nmatch2\nno\nmatch3")

        response = client.post(
            "/api/files/search",
            json={
                "workspace_id": "test-ws",
                "pattern": "match",
                "path": ".",
                "search_type": "grep",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["matches"]) == 3
        line_numbers = [m["line_number"] for m in data["matches"]]
        assert line_numbers == [1, 3, 5]

    def test_delete_file_in_nested_dir(self, client, workspace_manager):
        """Test deleting file in nested directory."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        nested = workspace_root / "a" / "b"
        nested.mkdir(parents=True)
        test_file = nested / "delete_me.txt"
        test_file.write_text("content")
        assert test_file.exists()

        response = client.request(
            "DELETE",
            "/api/files/delete",
            json={"workspace_id": "test-ws", "path": "a/b/delete_me.txt"},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert not test_file.exists()

    def test_write_large_content(self, client, workspace_manager):
        """Test writing large content."""
        large_content = "x" * 100000  # 100KB

        response = client.post(
            "/api/files/write",
            json={
                "workspace_id": "test-ws",
                "path": "large.txt",
                "content": large_content,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["size"] == 100000

    def test_list_hidden_recursive(self, client, workspace_manager):
        """Test recursive listing respects hidden files in subdirs."""
        workspace_root = workspace_manager.get_workspace_root("test-ws")
        hidden_dir = workspace_root / ".hidden_dir"
        hidden_dir.mkdir()
        (hidden_dir / "file.txt").write_text("hidden")
        (workspace_root / "visible").mkdir()
        (workspace_root / "visible" / "file.txt").write_text("visible")

        # Without include_hidden
        response = client.post(
            "/api/files/list",
            json={"workspace_id": "test-ws", "path": ".", "recursive": True},
        )

        data = response.json()
        paths = [e["path"] for e in data["entries"]]
        assert not any(".hidden_dir" in p for p in paths)
        assert any("visible" in p for p in paths)

        # With include_hidden
        response = client.post(
            "/api/files/list",
            json={
                "workspace_id": "test-ws",
                "path": ".",
                "recursive": True,
                "include_hidden": True,
            },
        )

        data = response.json()
        paths = [e["path"] for e in data["entries"]]
        assert any(".hidden_dir" in p for p in paths)
