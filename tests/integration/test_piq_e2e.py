"""E2E tests for PiQ absorption: Task -> @c4_track -> Artifacts -> Knowledge.

Tests the full pipeline from experiment execution through tracking, artifact
detection, and knowledge storage.
"""

from __future__ import annotations

import asyncio
import tempfile
from pathlib import Path
from unittest.mock import patch


class TestTrackerE2E:
    """Test @c4_track decorator captures metrics end-to-end."""

    def test_track_captures_stdout_metrics(self):
        """@c4_track should capture metrics printed to stdout."""
        from c4.tracker.decorator import c4_track

        @c4_track(task_id="T-E2E-001", capture_git=False)
        def train_dummy():
            for epoch in range(3):
                print(f"epoch: {epoch}, loss: {0.5 - epoch * 0.1:.1f}")
            return "done"

        result = train_dummy()
        assert result == "done"

        stats = train_dummy._last_stats
        assert stats is not None
        assert stats.run_time_sec >= 0  # May be very fast
        # Last epoch value should be captured
        assert "loss" in stats.metrics
        assert "epoch" in stats.metrics

    def test_track_captures_code_features(self):
        """@c4_track should analyze code features via AST."""
        from c4.tracker.decorator import c4_track

        @c4_track(task_id="T-E2E-002", capture_stdout=False, capture_git=False)
        def train_func():
            # Just a simple function whose source can be analyzed
            x = 42  # noqa: F841
            return "trained"

        train_func()

        code_features = train_func._code_features
        assert code_features is not None
        assert "imports" in code_features

    def test_track_profiles_data(self):
        """@c4_track should profile data arguments."""
        from c4.tracker.decorator import c4_track

        @c4_track(task_id="T-E2E-003", capture_stdout=False, capture_git=False)
        def process_data(data: list, labels: list):
            return len(data)

        result = process_data([1, 2, 3], [0, 1, 0])
        assert result == 3

        stats = process_data._last_stats
        assert stats is not None
        assert isinstance(stats.data_profile, dict)

    def test_track_env_var_task_id(self):
        """@c4_track should read C4_TASK_ID from environment."""
        from c4.tracker.decorator import c4_track

        @c4_track(capture_stdout=False, capture_git=False)
        def simple_func():
            return 42

        with patch.dict("os.environ", {"C4_TASK_ID": "T-ENV-001"}):
            simple_func()
            assert simple_func._last_task_id == "T-ENV-001"


class TestArtifactDetectionE2E:
    """Test artifact detection on task workspace."""

    def test_scan_outputs_directory(self):
        """Artifact detector should find .pt files at workspace root."""
        from c4.artifacts.detector import scan_outputs

        with tempfile.TemporaryDirectory() as tmpdir:
            workspace = Path(tmpdir)
            # Put .pt file at root level where *.pt pattern matches
            (workspace / "model.pt").write_bytes(b"fake model data" * 100)
            (workspace / "metrics.json").write_text('{"loss": 0.5}')

            detected = scan_outputs(str(workspace))
            names = {d["name"] for d in detected}
            assert len(detected) >= 1
            assert "model.pt" in names or "metrics.json" in names

    def test_scan_detects_pt_files_at_root(self):
        """Artifact detector should find .pt files at workspace root via *.pt pattern."""
        from c4.artifacts.detector import scan_outputs

        with tempfile.TemporaryDirectory() as tmpdir:
            workspace = Path(tmpdir)
            (workspace / "checkpoint.pt").write_bytes(b"checkpoint" * 50)
            (workspace / "model.pkl").write_bytes(b"pickle" * 50)

            detected = scan_outputs(str(workspace))
            assert len(detected) >= 2
            names = {d["name"] for d in detected}
            assert "checkpoint.pt" in names
            assert "model.pkl" in names

    def test_scan_empty_workspace(self):
        """Artifact detector should handle empty workspaces."""
        from c4.artifacts.detector import scan_outputs

        with tempfile.TemporaryDirectory() as tmpdir:
            detected = scan_outputs(tmpdir)
            assert detected == []


class TestArtifactStoreE2E:
    """Test local artifact store operations."""

    def test_save_and_retrieve(self):
        """Save an artifact and retrieve it by task_id."""
        from c4.artifacts.store import LocalArtifactStore

        with tempfile.TemporaryDirectory() as tmpdir:
            store = LocalArtifactStore(base_path=Path(tmpdir) / "artifacts")

            src = Path(tmpdir) / "model.pt"
            src.write_bytes(b"model weights" * 100)

            loop = asyncio.new_event_loop()
            try:
                ref = loop.run_until_complete(
                    store.save(task_id="T-E2E-010", local_path=src, artifact_type="output")
                )
                assert ref.content_hash is not None
                assert ref.size_bytes > 0

                artifacts = loop.run_until_complete(store.list(task_id="T-E2E-010"))
                assert len(artifacts) == 1

                retrieved = loop.run_until_complete(
                    store.get(task_id="T-E2E-010", name="model.pt")
                )
                assert retrieved is not None
            finally:
                loop.close()

    def test_content_addressable_dedup(self):
        """Same content should produce same hash."""
        from c4.artifacts.store import LocalArtifactStore

        with tempfile.TemporaryDirectory() as tmpdir:
            store = LocalArtifactStore(base_path=Path(tmpdir) / "artifacts")

            content = b"identical content" * 100
            src1 = Path(tmpdir) / "file1.bin"
            src2 = Path(tmpdir) / "file2.bin"
            src1.write_bytes(content)
            src2.write_bytes(content)

            loop = asyncio.new_event_loop()
            try:
                ref1 = loop.run_until_complete(
                    store.save(task_id="T-E2E-011", local_path=src1, artifact_type="output")
                )
                ref2 = loop.run_until_complete(
                    store.save(task_id="T-E2E-012", local_path=src2, artifact_type="output")
                )
                assert ref1.content_hash == ref2.content_hash
            finally:
                loop.close()


class TestKnowledgeStoreE2E:
    """Test knowledge store save and search (v2 DocumentStore)."""

    def test_save_and_search_experiment(self):
        """Save experiment and search by keyword."""
        from c4.knowledge.documents import DocumentStore

        with tempfile.TemporaryDirectory() as tmpdir:
            base = Path(tmpdir) / ".c4" / "knowledge"
            store = DocumentStore(base_path=base)

            doc_id = store.create("experiment", {
                "title": "Random Forest baseline for classification",
                "task_id": "T-E2E-020",
                "domain": "ml",
                "tags": ["sklearn", "random_forest"],
            }, body="# Random Forest\nAccuracy: 0.85, F1: 0.82")

            assert doc_id.startswith("exp-")

            results = store.search_fts("random forest")
            assert len(results) >= 1


class TestHookRegistryE2E:
    """Test hook registry with builtin hooks."""

    def test_register_and_execute_builtin_hooks(self):
        """Register builtin hooks and execute them."""
        from c4.hooks.base import HookContext, HookPhase
        from c4.hooks.builtin.artifact_hook import ArtifactHook
        from c4.hooks.builtin.knowledge_hook import KnowledgeHook
        from c4.hooks.registry import HookRegistry

        registry = HookRegistry()
        registry.register(KnowledgeHook())
        registry.register(ArtifactHook())

        assert registry.count == 2

        ctx = HookContext(
            task_id="T-E2E-030",
            phase=HookPhase.AFTER_COMPLETE,
            task_data={"title": "Simple task"},
        )
        results = registry.execute(HookPhase.AFTER_COMPLETE, ctx)
        assert len(results) == 2
        assert all(r["success"] for r in results)


class TestFullPipelineE2E:
    """Test the complete pipeline: track -> detect -> store knowledge."""

    def test_track_then_knowledge_save(self):
        """Run @c4_track, then save results to knowledge store (v2)."""
        from c4.knowledge.documents import DocumentStore
        from c4.tracker.decorator import c4_track

        @c4_track(task_id="T-PIPE-001", capture_git=False)
        def train_model():
            for i in range(5):
                print(f"epoch: {i}, loss: {1.0 - i * 0.15:.2f}, accuracy: {0.6 + i * 0.05:.2f}")
            return "model_v1"

        result = train_model()
        assert result == "model_v1"

        stats = train_model._last_stats
        assert stats is not None
        assert "loss" in stats.metrics
        assert "accuracy" in stats.metrics

        with tempfile.TemporaryDirectory() as tmpdir:
            base = Path(tmpdir) / ".c4" / "knowledge"
            store = DocumentStore(base_path=base)

            metrics_lines = [f"- {k}: {v}" for k, v in stats.metrics.items()]
            body = "# Pipeline test model training\n\n## Metrics\n" + "\n".join(metrics_lines)

            doc_id = store.create("experiment", {
                "title": "Pipeline test model training",
                "task_id": "T-PIPE-001",
                "domain": "ml",
                "tags": ["test"],
            }, body=body)

            assert doc_id.startswith("exp-")

            results = store.search_fts("pipeline model")
            assert len(results) >= 1

    def test_track_then_artifact_detect(self):
        """Run @c4_track, then detect artifacts in workspace."""
        from c4.artifacts.detector import scan_outputs
        from c4.tracker.decorator import c4_track

        with tempfile.TemporaryDirectory() as tmpdir:
            workspace = Path(tmpdir)
            outputs = workspace / "outputs"
            outputs.mkdir()

            @c4_track(task_id="T-PIPE-002", capture_git=False)
            def train_and_save():
                # Put .pt at workspace root to match *.pt pattern
                (workspace / "model.pt").write_bytes(b"trained model" * 50)
                (outputs / "metrics.json").write_text('{"loss": 0.3}')
                print("loss: 0.3, accuracy: 0.92")
                return "done"

            train_and_save()

            stats = train_and_save._last_stats
            assert stats is not None
            assert "loss" in stats.metrics

            detected = scan_outputs(str(workspace))
            # *.pt at root should be found
            assert len(detected) >= 1


class TestGpuToolsVisibility:
    """Test that GPU/Tracker tools are conditionally exposed."""

    def test_gpu_backend_detection(self):
        """GPU backend detection should work without GPU hardware."""
        from c4.gpu.monitor import detect_backend

        backend = detect_backend()
        # Should return a valid backend (mps, cuda, or cpu)
        assert backend in ("mps", "cuda", "cpu")

    def test_gpu_config_defaults(self):
        """GpuTaskConfig should have sensible defaults."""
        from c4.models.task import GpuTaskConfig

        config = GpuTaskConfig()
        assert config.gpu_count == 1
        assert config.min_vram_gb == 8
        assert config.parallelism == "single"

    def test_execution_stats_defaults(self):
        """ExecutionStats should have empty defaults."""
        from c4.models.task import ExecutionStats

        stats = ExecutionStats()
        assert stats.run_time_sec == 0
        assert stats.metrics == {}
        assert stats.code_features == {}
