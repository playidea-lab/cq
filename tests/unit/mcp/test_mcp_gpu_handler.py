"""Tests for MCP GPU, Knowledge, and Artifact handlers."""

from unittest.mock import MagicMock

# Import handlers to register them
import c4.mcp.handlers.artifacts  # noqa: F401
import c4.mcp.handlers.gpu  # noqa: F401
import c4.mcp.handlers.knowledge  # noqa: F401
from c4.mcp.registry import tool_registry


class TestToolRegistration:
    """Verify all 9 new tools are registered."""

    EXPECTED_TOOLS = [
        "c4_gpu_status",
        "c4_job_submit",
        "c4_job_status",
        "c4_experiment_search",
        "c4_experiment_record",
        "c4_pattern_suggest",
        "c4_artifact_list",
        "c4_artifact_save",
        "c4_artifact_get",
    ]

    def test_all_tools_registered(self):
        for tool_name in self.EXPECTED_TOOLS:
            assert tool_registry.is_registered(tool_name), f"{tool_name} not registered"


class TestGpuHandlers:
    def test_gpu_status(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_gpu_status", daemon, {})
        # Should return gpu_count (may be 0 on CI/test machines)
        assert "gpu_count" in result or "error" in result

    def test_job_submit_missing_command(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_job_submit", daemon, {})
        assert "error" in result
        assert "command" in result["error"]

    def test_job_status_no_jobs(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_job_status", daemon, {})
        # Should return job list (possibly empty) or error
        assert "job_count" in result or "error" in result


class TestKnowledgeHandlers:
    def test_experiment_search_missing_query(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_experiment_search", daemon, {})
        assert "error" in result
        assert "query" in result["error"]

    def test_experiment_record_missing_title(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_experiment_record", daemon, {})
        assert "error" in result
        assert "title" in result["error"]

    def test_experiment_record_success(self, tmp_path, monkeypatch):
        monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))
        daemon = MagicMock()
        result = tool_registry.dispatch(
            "c4_experiment_record",
            daemon,
            {
                "title": "Test experiment",
                "task_id": "T-001-0",
                "hypothesis": "Test hypothesis",
                "domain": "ml-dl",
            },
        )
        assert result.get("success") is True
        assert "experiment_id" in result

    def test_experiment_search_success(self, tmp_path, monkeypatch):
        monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))
        daemon = MagicMock()

        # First record an experiment
        tool_registry.dispatch(
            "c4_experiment_record",
            daemon,
            {"title": "RandomForest baseline", "domain": "ml-dl"},
        )

        # Then search
        result = tool_registry.dispatch(
            "c4_experiment_search",
            daemon,
            {"query": "RandomForest"},
        )
        assert result.get("count", 0) >= 1

    def test_pattern_suggest(self, tmp_path, monkeypatch):
        monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_pattern_suggest", daemon, {})
        assert "pattern_count" in result


class TestArtifactHandlers:
    def test_artifact_list_missing_task_id(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_artifact_list", daemon, {})
        assert "error" in result
        assert "task_id" in result["error"]

    def test_artifact_save_missing_fields(self):
        daemon = MagicMock()
        result = tool_registry.dispatch("c4_artifact_save", daemon, {})
        assert "error" in result

    def test_artifact_save_file_not_found(self, tmp_path, monkeypatch):
        monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))
        daemon = MagicMock()
        result = tool_registry.dispatch(
            "c4_artifact_save",
            daemon,
            {"task_id": "T-001-0", "path": "/nonexistent/file.txt"},
        )
        assert "error" in result
        assert "not found" in result["error"]

    def test_artifact_get_missing_fields(self):
        daemon = MagicMock()
        result = tool_registry.dispatch(
            "c4_artifact_get", daemon, {"task_id": "T-001-0"}
        )
        assert "error" in result
        assert "name" in result["error"]

    def test_artifact_save_and_list(self, tmp_path, monkeypatch):
        monkeypatch.setenv("C4_PROJECT_ROOT", str(tmp_path))
        daemon = MagicMock()

        # Create a test file
        test_file = tmp_path / "output.txt"
        test_file.write_text("test content")

        # Save artifact
        result = tool_registry.dispatch(
            "c4_artifact_save",
            daemon,
            {"task_id": "T-001-0", "path": str(test_file)},
        )
        assert result.get("success") is True

        # List artifacts
        result = tool_registry.dispatch(
            "c4_artifact_list",
            daemon,
            {"task_id": "T-001-0"},
        )
        assert result.get("count", 0) >= 1
