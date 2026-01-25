"""Tests for Documentation API Routes."""

from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def mock_daemon():
    """Create a mock C4Daemon."""
    daemon = MagicMock()
    daemon.is_initialized.return_value = True
    daemon._state_manager = MagicMock()
    daemon._state_manager._c4_dir = "/test/project/.c4"
    return daemon


@pytest.fixture
def client(mock_daemon):
    """Create test client with mocked daemon."""
    with patch("c4.api.deps.get_daemon_singleton", return_value=mock_daemon):
        from c4.api.server import create_app

        app = create_app()
        yield TestClient(app)


@pytest.fixture
def mock_doc_generator():
    """Create a mock DocGenerator."""
    from c4.mcp.docs_server import DocSnapshot, SnapshotDiff

    generator = MagicMock()

    # Mock list_snapshots
    generator.list_snapshots.return_value = [
        DocSnapshot(
            version="v1.0.0",
            created_at="2024-01-01T00:00:00Z",
            commit_hash="abc1234",
            description="Initial release",
            files_count=10,
            symbols_count=50,
            content_hash="sha256:abcdef",
        ),
        DocSnapshot(
            version="v1.1.0",
            created_at="2024-02-01T00:00:00Z",
            commit_hash="def5678",
            description="Feature update",
            files_count=12,
            symbols_count=60,
            content_hash="sha256:123456",
        ),
    ]

    # Mock create_snapshot
    generator.create_snapshot.return_value = DocSnapshot(
        version="v2.0.0",
        created_at="2024-03-01T00:00:00Z",
        commit_hash="ghi9012",
        description="Major release",
        files_count=15,
        symbols_count=75,
        content_hash="sha256:789abc",
    )

    # Mock get_snapshot
    generator.get_snapshot.return_value = {"docs": "content"}

    # Mock compare_snapshots
    generator.compare_snapshots.return_value = SnapshotDiff(
        from_version="v1.0.0",
        to_version="v1.1.0",
        added_symbols=["new_func"],
        removed_symbols=[],
        modified_symbols=["old_func"],
        added_files=["new.py"],
        removed_files=[],
        summary="1 symbol added, 1 modified",
    ).to_dict()

    # Mock delete_snapshot
    generator.delete_snapshot.return_value = True

    return generator


class TestDocsListSnapshots:
    """Tests for GET /api/docs/snapshots."""

    def test_list_snapshots_success(self, client, mock_daemon, mock_doc_generator):
        """Test listing snapshots returns all snapshots."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots")

        assert response.status_code == 200
        data = response.json()
        assert "snapshots" in data
        assert len(data["snapshots"]) == 2
        assert data["snapshots"][0]["version"] == "v1.0.0"
        assert data["snapshots"][1]["version"] == "v1.1.0"

    def test_list_snapshots_empty(self, client, mock_daemon, mock_doc_generator):
        """Test listing snapshots when none exist."""
        mock_doc_generator.list_snapshots.return_value = []

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots")

        assert response.status_code == 200
        data = response.json()
        assert data["snapshots"] == []


class TestDocsCreateSnapshot:
    """Tests for POST /api/docs/snapshots."""

    def test_create_snapshot_success(self, client, mock_daemon, mock_doc_generator):
        """Test creating a snapshot."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.post(
                "/api/docs/snapshots",
                json={"version": "v2.0.0", "description": "Major release"},
            )

        assert response.status_code == 200
        data = response.json()
        assert data["version"] == "v2.0.0"
        assert data["description"] == "Major release"
        mock_doc_generator.create_snapshot.assert_called_once_with(
            version="v2.0.0", description="Major release"
        )

    def test_create_snapshot_no_description(self, client, mock_daemon, mock_doc_generator):
        """Test creating a snapshot without description."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.post(
                "/api/docs/snapshots",
                json={"version": "v2.0.0"},
            )

        assert response.status_code == 200
        mock_doc_generator.create_snapshot.assert_called_once_with(
            version="v2.0.0", description=None
        )

    def test_create_snapshot_duplicate_version(
        self, client, mock_daemon, mock_doc_generator
    ):
        """Test creating a snapshot with existing version."""
        mock_doc_generator.create_snapshot.side_effect = ValueError(
            "Snapshot version 'v1.0.0' already exists"
        )

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.post(
                "/api/docs/snapshots",
                json={"version": "v1.0.0"},
            )

        assert response.status_code == 400
        assert "already exists" in response.json()["detail"]


class TestDocsGetSnapshot:
    """Tests for GET /api/docs/snapshots/{version}."""

    def test_get_snapshot_json(self, client, mock_daemon, mock_doc_generator):
        """Test getting a snapshot in JSON format."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots/v1.0.0?format=json")

        assert response.status_code == 200
        mock_doc_generator.get_snapshot.assert_called_once()

    def test_get_snapshot_markdown(self, client, mock_daemon, mock_doc_generator):
        """Test getting a snapshot in markdown format."""
        mock_doc_generator.get_snapshot.return_value = "# API Documentation"

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots/v1.0.0?format=markdown")

        assert response.status_code == 200

    def test_get_snapshot_not_found(self, client, mock_daemon, mock_doc_generator):
        """Test getting a non-existent snapshot."""
        mock_doc_generator.get_snapshot.return_value = {
            "error": "Snapshot 'v999' not found"
        }

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots/v999")

        assert response.status_code == 404


class TestDocsCompareSnapshots:
    """Tests for GET /api/docs/snapshots/{version}/compare/{other_version}."""

    def test_compare_snapshots_success(self, client, mock_daemon, mock_doc_generator):
        """Test comparing two snapshots."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots/v1.0.0/compare/v1.1.0")

        assert response.status_code == 200
        data = response.json()
        assert data["from_version"] == "v1.0.0"
        assert data["to_version"] == "v1.1.0"
        assert "added_symbols" in data
        assert "removed_symbols" in data

    def test_compare_snapshots_not_found(self, client, mock_daemon, mock_doc_generator):
        """Test comparing with non-existent snapshot."""
        mock_doc_generator.compare_snapshots.return_value = {
            "error": "Snapshot 'v999' not found"
        }

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.get("/api/docs/snapshots/v1.0.0/compare/v999")

        assert response.status_code == 404


class TestDocsDeleteSnapshot:
    """Tests for DELETE /api/docs/snapshots/{version}."""

    def test_delete_snapshot_success(self, client, mock_daemon, mock_doc_generator):
        """Test deleting a snapshot."""
        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.delete("/api/docs/snapshots/v1.0.0")

        assert response.status_code == 200
        data = response.json()
        assert "deleted successfully" in data["message"]

    def test_delete_snapshot_not_found(self, client, mock_daemon, mock_doc_generator):
        """Test deleting non-existent snapshot."""
        mock_doc_generator.delete_snapshot.return_value = False

        with patch(
            "c4.api.routes.docs._get_doc_generator", return_value=mock_doc_generator
        ):
            response = client.delete("/api/docs/snapshots/v999")

        assert response.status_code == 404
        assert "not found" in response.json()["detail"]
