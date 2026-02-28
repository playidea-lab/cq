"""Tests for c4.knowledge.reindex — OllamaEmbeddings reindex support."""

from __future__ import annotations

import sqlite3
from pathlib import Path
from unittest.mock import patch

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_knowledge_db(path: Path, n_docs: int = 5) -> Path:
    """Create a minimal knowledge.db with n_docs rows."""
    path.mkdir(parents=True, exist_ok=True)
    db = path / "knowledge.db"
    conn = sqlite3.connect(str(db))
    conn.execute(
        "CREATE TABLE knowledge_docs "
        "(id TEXT PRIMARY KEY, title TEXT, body TEXT, tags TEXT, doc_type TEXT)"
    )
    for i in range(n_docs):
        conn.execute(
            "INSERT INTO knowledge_docs VALUES (?,?,?,?,?)",
            (f"doc-{i:03d}", f"Title {i}", f"Body content {i}", "tag1,tag2", "insight"),
        )
    conn.commit()
    conn.close()
    return db


def _make_legacy_db(path: Path, n_docs: int = 3) -> Path:
    """Create a minimal knowledge.db using legacy 'documents' table name."""
    path.mkdir(parents=True, exist_ok=True)
    db = path / "knowledge.db"
    conn = sqlite3.connect(str(db))
    conn.execute(
        "CREATE TABLE documents "
        "(id TEXT PRIMARY KEY, title TEXT, body TEXT, tags TEXT, doc_type TEXT)"
    )
    for i in range(n_docs):
        conn.execute(
            "INSERT INTO documents VALUES (?,?,?,?,?)",
            (f"leg-{i:03d}", f"Legacy {i}", f"Legacy body {i}", "tag1", "insight"),
        )
    conn.commit()
    conn.close()
    return db


# ---------------------------------------------------------------------------
# Mock embedder that records calls
# ---------------------------------------------------------------------------

class MockEmbedderForReindex:
    """Wraps OllamaEmbeddings-shaped API with a mock backend."""

    def __init__(self, dimension: int = 768) -> None:
        self._dimension = dimension
        self.indexed: list[str] = []

    def index_document(self, doc_id: str, document: dict) -> bool:
        self.indexed.append(doc_id)
        return True

    @property
    def count(self) -> int:
        return len(self.indexed)


# ---------------------------------------------------------------------------
# TestReindexWithOllamaMock
# ---------------------------------------------------------------------------

class TestReindexWithOllamaMock:
    """TestReindexWithOllamaMock: mock OllamaEmbeddings inject → 5 docs reindex success."""

    def test_reindex_five_docs_with_mock(self, tmp_path: Path) -> None:
        """Inject mock embedder; verify 5 docs are reindexed successfully."""
        _make_knowledge_db(tmp_path / "knowledge")

        mock_embedder = MockEmbedderForReindex(dimension=768)

        from c4.knowledge.reindex import reindex_all

        result = reindex_all(
            provider="ollama",
            base_path=tmp_path / "knowledge",
            _embedder=mock_embedder,
        )

        assert result["total"] == 5
        assert result["success"] == 5
        assert result["skipped"] == 0
        assert len(mock_embedder.indexed) == 5

    def test_reindex_no_db_returns_zeros(self, tmp_path: Path) -> None:
        """If DB does not exist, returns all-zero result gracefully."""
        from c4.knowledge.reindex import reindex_all

        result = reindex_all(provider="ollama", base_path=tmp_path / "nonexistent")

        assert result == {"success": 0, "skipped": 0, "total": 0}

    def test_reindex_legacy_documents_table_fallback(self, tmp_path: Path) -> None:
        """Legacy 'documents' table is used when 'knowledge_docs' does not exist."""
        _make_legacy_db(tmp_path / "knowledge", n_docs=3)

        mock_embedder = MockEmbedderForReindex(dimension=768)

        from c4.knowledge.reindex import reindex_all

        result = reindex_all(
            provider="ollama",
            base_path=tmp_path / "knowledge",
            _embedder=mock_embedder,
        )

        assert result["total"] == 3
        assert result["success"] == 3
        assert result["skipped"] == 0
        assert len(mock_embedder.indexed) == 3

    def test_reindex_partial_failure_counts_skipped(self, tmp_path: Path) -> None:
        """Documents that fail to index are counted as skipped."""
        _make_knowledge_db(tmp_path / "knowledge", n_docs=3)

        class FailingEmbedder:
            call_count = 0

            def index_document(self, doc_id: str, document: dict) -> bool:
                self.call_count += 1
                if self.call_count == 2:
                    raise RuntimeError("Ollama unavailable")
                return True

        embedder = FailingEmbedder()

        from c4.knowledge.reindex import reindex_all

        result = reindex_all(
            provider="ollama",
            base_path=tmp_path / "knowledge",
            _embedder=embedder,
        )

        assert result["total"] == 3
        assert result["success"] == 2
        assert result["skipped"] == 1


# ---------------------------------------------------------------------------
# TestKnowledgeReindexMCPTool_OllamaParam
# ---------------------------------------------------------------------------

class TestKnowledgeReindexMCPTool_OllamaParam:
    """TestKnowledgeReindexMCPTool_OllamaParam: provider='ollama' param passed correctly."""

    def test_ollama_provider_registered(self) -> None:
        """OllamaEmbeddings is accessible via get_embeddings_provider('ollama')."""
        from c4.knowledge.embeddings_provider import OllamaEmbeddings, get_embeddings_provider

        provider = get_embeddings_provider("ollama")
        assert isinstance(provider, OllamaEmbeddings)

    def test_ollama_provider_dimension(self) -> None:
        """OllamaEmbeddings with default model reports 768 dimensions."""
        from c4.knowledge.embeddings_provider import OllamaEmbeddings

        p = OllamaEmbeddings()
        assert p.dimension == 768

    def test_ollama_provider_custom_model_dimension(self) -> None:
        """OllamaEmbeddings with mxbai-embed-large reports 1024 dimensions."""
        from c4.knowledge.embeddings_provider import OllamaEmbeddings

        p = OllamaEmbeddings(model="mxbai-embed-large")
        assert p.dimension == 1024

    def test_reindex_all_passes_provider_to_embedder(self, tmp_path: Path) -> None:
        """reindex_all(provider='ollama') passes provider string to KnowledgeEmbedder."""
        _make_knowledge_db(tmp_path / "knowledge", n_docs=2)

        captured: dict = {}

        class CapturingEmbedder:
            def __init__(self, base_path, embedding_model):
                captured["embedding_model"] = embedding_model
                self.indexed: list[str] = []

            def index_document(self, doc_id, document):
                self.indexed.append(doc_id)
                return True

        with patch(
            "c4.knowledge.reindex.KnowledgeEmbedder",
            side_effect=lambda base_path, embedding_model: CapturingEmbedder(base_path, embedding_model),
        ):
            from c4.knowledge.reindex import reindex_all

            result = reindex_all(
                provider="ollama",
                base_path=tmp_path / "knowledge",
            )

        assert result["total"] == 2
        assert result["success"] == 2
        assert captured.get("embedding_model") == "ollama"
