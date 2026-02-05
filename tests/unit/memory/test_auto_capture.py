"""Tests for the auto capture module."""

import json
import sqlite3
import tempfile
from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock

from c4.memory.auto_capture import (
    TOOL_IMPORTANCE,
    AutoCaptureHandler,
    Observation,
    get_auto_capture_handler,
)


class TestObservation:
    """Tests for Observation dataclass."""

    def test_create_observation(self) -> None:
        """Should create observation with all fields."""
        obs = Observation(
            id="obs-001",
            project_id="test-project",
            source="read_file",
            content="Hello, world!",
            importance=7,
            tags=["test", "file"],
            metadata={"path": "/src/main.py"},
        )

        assert obs.id == "obs-001"
        assert obs.project_id == "test-project"
        assert obs.source == "read_file"
        assert obs.content == "Hello, world!"
        assert obs.importance == 7
        assert obs.tags == ["test", "file"]
        assert obs.metadata == {"path": "/src/main.py"}

    def test_observation_defaults(self) -> None:
        """Should use default values for optional fields."""
        obs = Observation(
            id="obs-001",
            project_id="test-project",
            source="user_message",
            content="Test content",
        )

        assert obs.importance == 5
        assert obs.tags == []
        assert obs.metadata == {}
        assert isinstance(obs.created_at, datetime)

    def test_to_dict(self) -> None:
        """Should convert observation to dictionary."""
        obs = Observation(
            id="obs-001",
            project_id="test-project",
            source="test",
            content="Content",
            tags=["tag1"],
            metadata={"key": "value"},
        )

        data = obs.to_dict()

        assert data["id"] == "obs-001"
        assert data["project_id"] == "test-project"
        assert json.loads(data["tags"]) == ["tag1"]
        assert json.loads(data["metadata"]) == {"key": "value"}

    def test_from_dict(self) -> None:
        """Should create observation from dictionary."""
        data = {
            "id": "obs-001",
            "project_id": "test-project",
            "source": "test",
            "content": "Content",
            "importance": 8,
            "tags": '["tag1", "tag2"]',
            "metadata": '{"key": "value"}',
            "created_at": "2024-01-01T12:00:00",
        }

        obs = Observation.from_dict(data)

        assert obs.id == "obs-001"
        assert obs.tags == ["tag1", "tag2"]
        assert obs.metadata == {"key": "value"}
        assert obs.created_at == datetime(2024, 1, 1, 12, 0, 0)

    def test_from_dict_with_parsed_json(self) -> None:
        """Should handle already-parsed JSON values."""
        data = {
            "id": "obs-001",
            "project_id": "test-project",
            "source": "test",
            "content": "Content",
            "tags": ["tag1"],  # Already a list
            "metadata": {"key": "value"},  # Already a dict
        }

        obs = Observation.from_dict(data)

        assert obs.tags == ["tag1"]
        assert obs.metadata == {"key": "value"}


class TestAutoCaptureHandler:
    """Tests for AutoCaptureHandler."""

    def test_capture_tool_output_basic(self) -> None:
        """Should capture tool output as observation."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="read_file",
                input_data={"path": "/src/main.py"},
                output="def main(): pass",
            )

            assert obs.source == "read_file"
            assert obs.content == "def main(): pass"
            assert obs.project_id == "test-project"
            assert obs.id.startswith("obs-")
            assert "tool:read_file" in obs.tags

    def test_capture_tool_output_with_importance(self) -> None:
        """Should use provided importance."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="read_file",
                input_data=None,
                output="content",
                importance=9,
            )

            assert obs.importance == 9

    def test_capture_tool_output_default_importance(self) -> None:
        """Should use tool-specific default importance."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # user_message has importance 9
            obs = handler.capture_tool_output(
                tool_name="user_message",
                input_data=None,
                output="Hello",
            )
            assert obs.importance == 9

            # read_file has importance 6
            obs2 = handler.capture_tool_output(
                tool_name="read_file",
                input_data=None,
                output="content",
            )
            assert obs2.importance == 6

    def test_capture_tool_output_with_tags(self) -> None:
        """Should include provided tags."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="test_tool",
                input_data=None,
                output="content",
                tags=["custom", "tags"],
            )

            assert "custom" in obs.tags
            assert "tags" in obs.tags
            assert "tool:test_tool" in obs.tags

    def test_capture_tool_output_dict_output(self) -> None:
        """Should JSON-encode dict output."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="test",
                input_data=None,
                output={"key": "value", "nested": {"a": 1}},
            )

            assert '"key": "value"' in obs.content
            assert '"nested"' in obs.content

    def test_capture_tool_output_stores_input_metadata(self) -> None:
        """Should store input in metadata."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="read_file",
                input_data={"path": "/src/main.py"},
                output="content",
            )

            assert obs.metadata["tool_name"] == "read_file"
            assert obs.metadata["input"]["path"] == "/src/main.py"

    def test_store_observation(self) -> None:
        """Should store observation in database."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = Observation(
                id="obs-test-001",
                project_id="test-project",
                source="test",
                content="Test content",
            )

            obs_id = handler.store_observation(obs)

            assert obs_id == "obs-test-001"

            # Verify stored
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT content FROM c4_observations WHERE id = ?",
                ("obs-test-001",),
            )
            row = cursor.fetchone()
            conn.close()

            assert row is not None
            assert row[0] == "Test content"

    def test_store_observation_returns_id(self) -> None:
        """store_observation should return the observation ID."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="test",
                input_data=None,
                output="content",
            )

            result_id = handler.store_observation(obs)

            assert result_id == obs.id

    def test_get_observation(self) -> None:
        """Should retrieve observation by ID."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # Store an observation
            obs = Observation(
                id="obs-get-001",
                project_id="test-project",
                source="test",
                content="Get test content",
                importance=7,
                tags=["test"],
            )
            handler.store_observation(obs)

            # Retrieve it
            retrieved = handler.get_observation("obs-get-001")

            assert retrieved is not None
            assert retrieved.id == "obs-get-001"
            assert retrieved.content == "Get test content"
            assert retrieved.importance == 7

    def test_get_observation_not_found(self) -> None:
        """Should return None for non-existent observation."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            result = handler.get_observation("nonexistent")

            assert result is None

    def test_get_observation_wrong_project(self) -> None:
        """Should not return observation from different project."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # Store with project A
            handler_a = AutoCaptureHandler("project-a", db_path)
            obs = Observation(
                id="obs-001",
                project_id="project-a",
                source="test",
                content="Content A",
            )
            handler_a.store_observation(obs)

            # Try to retrieve with project B
            handler_b = AutoCaptureHandler("project-b", db_path)
            result = handler_b.get_observation("obs-001")

            assert result is None

    def test_list_observations(self) -> None:
        """Should list observations for project."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # Store multiple observations
            for i in range(5):
                obs = handler.capture_tool_output(
                    tool_name="test",
                    input_data=None,
                    output=f"Content {i}",
                )
                handler.store_observation(obs)

            observations = handler.list_observations()

            assert len(observations) == 5

    def test_list_observations_filter_by_source(self) -> None:
        """Should filter observations by source."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # Store observations with different sources
            for tool in ["read_file", "read_file", "list_dir"]:
                obs = handler.capture_tool_output(
                    tool_name=tool,
                    input_data=None,
                    output="content",
                )
                handler.store_observation(obs)

            read_file_obs = handler.list_observations(source="read_file")
            list_dir_obs = handler.list_observations(source="list_dir")

            assert len(read_file_obs) == 2
            assert len(list_dir_obs) == 1

    def test_list_observations_filter_by_importance(self) -> None:
        """Should filter observations by minimum importance."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # Store observations with different importance
            for importance in [3, 5, 7, 9]:
                obs = Observation(
                    id=f"obs-imp-{importance}",
                    project_id="test-project",
                    source="test",
                    content="Content",
                    importance=importance,
                )
                handler.store_observation(obs)

            high_importance = handler.list_observations(min_importance=7)

            assert len(high_importance) == 2
            assert all(o.importance >= 7 for o in high_importance)

    def test_list_observations_limit(self) -> None:
        """Should respect limit parameter."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            # Store 10 observations
            for i in range(10):
                obs = handler.capture_tool_output(
                    tool_name="test",
                    input_data=None,
                    output=f"Content {i}",
                )
                handler.store_observation(obs)

            limited = handler.list_observations(limit=3)

            assert len(limited) == 3

    def test_create_embedding_without_provider(self) -> None:
        """Should log warning when no embedding provider."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            handler = AutoCaptureHandler("test-project", db_path)

            obs = handler.capture_tool_output(
                tool_name="test",
                input_data=None,
                output="content",
            )

            # Should not raise, just log warning
            handler.create_embedding(obs)

    def test_create_embedding_with_provider(self) -> None:
        """Should create embedding when provider configured."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # Mock embedding provider
            mock_provider = MagicMock()
            mock_provider.embed.return_value = [0.1, 0.2, 0.3, 0.4]

            # Mock vector store
            mock_vector_store = MagicMock()

            handler = AutoCaptureHandler(
                project_id="test-project",
                db_path=db_path,
                embedding_provider=mock_provider,
                vector_store=mock_vector_store,
            )

            obs = Observation(
                id="obs-emb-001",
                project_id="test-project",
                source="test",
                content="Content to embed",
            )
            handler.store_observation(obs)
            handler.create_embedding(obs)

            # Verify embedding was generated and stored
            mock_provider.embed.assert_called_once_with("Content to embed")
            mock_vector_store.add.assert_called_once()

            # Verify memory index entry was created
            conn = sqlite3.connect(db_path)
            cursor = conn.execute(
                "SELECT * FROM c4_memory_index WHERE observation_id = ?",
                ("obs-emb-001",),
            )
            row = cursor.fetchone()
            conn.close()

            assert row is not None


class TestToolImportance:
    """Tests for TOOL_IMPORTANCE mapping."""

    def test_high_importance_tools(self) -> None:
        """User actions and key operations should have high importance."""
        assert TOOL_IMPORTANCE["user_message"] >= 8
        assert TOOL_IMPORTANCE["file_write"] >= 8
        assert TOOL_IMPORTANCE["git_commit"] >= 8

    def test_medium_importance_tools(self) -> None:
        """Code analysis tools should have medium importance."""
        assert 5 <= TOOL_IMPORTANCE["find_symbol"] <= 8
        assert 5 <= TOOL_IMPORTANCE["read_file"] <= 8

    def test_default_importance(self) -> None:
        """Default importance should be 5."""
        assert TOOL_IMPORTANCE["default"] == 5


class TestGetAutoCaptureHandler:
    """Tests for get_auto_capture_handler factory function."""

    def test_create_handler(self) -> None:
        """Should create handler without embeddings."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            handler = get_auto_capture_handler("test-project", db_path)

            assert handler.project_id == "test-project"
            assert handler._embedding_provider is None
            assert handler._vector_store is None

    def test_create_handler_with_embeddings_no_api_key(self) -> None:
        """Should fallback gracefully when no API key available."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            # enable_embeddings=True but no API key should use mock
            handler = get_auto_capture_handler(
                "test-project",
                db_path,
                enable_embeddings=True,
            )

            assert handler.project_id == "test-project"
            # May or may not have provider depending on mock availability
