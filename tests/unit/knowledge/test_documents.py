"""Tests for DocumentStore - Obsidian-style knowledge document CRUD."""


import pytest

from c4.knowledge.documents import (
    DocumentStore,
    _doc_to_markdown,
    _generate_id,
    _parse_frontmatter,
)
from c4.knowledge.models import DocumentType, KnowledgeDocument


@pytest.fixture
def store(tmp_path):
    """Create a DocumentStore in a temp directory."""
    return DocumentStore(base_path=tmp_path / "knowledge")


class TestGenerateId:
    def test_experiment_prefix(self):
        doc_id = _generate_id("experiment")
        assert doc_id.startswith("exp-")
        assert len(doc_id) == 12  # "exp-" + 8 hex chars

    def test_pattern_prefix(self):
        doc_id = _generate_id("pattern")
        assert doc_id.startswith("pat-")

    def test_insight_prefix(self):
        doc_id = _generate_id("insight")
        assert doc_id.startswith("ins-")

    def test_hypothesis_prefix(self):
        doc_id = _generate_id("hypothesis")
        assert doc_id.startswith("hyp-")

    def test_unknown_type_fallback(self):
        doc_id = _generate_id("unknown")
        assert doc_id.startswith("doc-")


class TestParseFrontmatter:
    def test_valid_frontmatter(self):
        text = "---\ntitle: Test\ntype: experiment\n---\n\n# Body"
        fm, body = _parse_frontmatter(text)
        assert fm["title"] == "Test"
        assert fm["type"] == "experiment"
        assert body == "# Body"

    def test_no_frontmatter(self):
        text = "# Just a body"
        fm, body = _parse_frontmatter(text)
        assert fm == {}
        assert body == "# Just a body"

    def test_empty_frontmatter(self):
        text = "---\n---\n\nBody here"
        fm, body = _parse_frontmatter(text)
        assert fm == {}
        assert body == "Body here"

    def test_complex_frontmatter(self):
        text = "---\ntags:\n  - ml\n  - rf\nconfidence: 0.95\n---\n\nContent"
        fm, body = _parse_frontmatter(text)
        assert fm["tags"] == ["ml", "rf"]
        assert fm["confidence"] == 0.95


class TestDocToMarkdown:
    def test_experiment_roundtrip(self):
        doc = KnowledgeDocument(
            id="exp-a1b2c3d4",
            type=DocumentType.EXPERIMENT,
            title="RF Baseline",
            domain="ml",
            tags=["sklearn"],
            hypothesis="RF achieves 85%+",
            hypothesis_status="supported",
            created_at="2026-01-01T00:00:00Z",
            updated_at="2026-01-01T00:00:00Z",
            body="# RF Baseline\n\n## Result\n- accuracy: 0.87",
        )
        md = _doc_to_markdown(doc)
        assert "---" in md
        assert "RF Baseline" in md
        assert "hypothesis_status: supported" in md
        assert "# RF Baseline" in md

    def test_pattern_document(self):
        doc = KnowledgeDocument(
            id="pat-b2c3d4e5",
            type=DocumentType.PATTERN,
            title="High LR Pattern",
            confidence=0.9,
            evidence_count=5,
            evidence_ids=["exp-001", "exp-002"],
            created_at="2026-01-01T00:00:00Z",
            updated_at="2026-01-01T00:00:00Z",
            body="Pattern description",
        )
        md = _doc_to_markdown(doc)
        assert "confidence: 0.9" in md
        assert "evidence_count: 5" in md


class TestDocumentStoreCRUD:
    def test_create_and_get(self, store):
        doc_id = store.create("experiment", {
            "title": "RF Baseline",
            "domain": "ml",
            "hypothesis": "RF achieves 85%+",
            "hypothesis_status": "supported",
            "tags": ["sklearn", "classification"],
        }, body="# RF Baseline\n\n## Result\n- accuracy: 0.87")

        assert doc_id.startswith("exp-")

        doc = store.get(doc_id)
        assert doc is not None
        assert doc.title == "RF Baseline"
        assert doc.domain == "ml"
        assert doc.hypothesis == "RF achieves 85%+"
        assert doc.hypothesis_status == "supported"
        assert doc.tags == ["sklearn", "classification"]
        assert "accuracy: 0.87" in doc.body

    def test_create_with_custom_id(self, store):
        doc_id = store.create("experiment", {
            "id": "exp-custom01",
            "title": "Custom ID Test",
        })
        assert doc_id == "exp-custom01"

    def test_get_nonexistent(self, store):
        assert store.get("exp-nonexist") is None

    def test_update(self, store):
        doc_id = store.create("experiment", {
            "title": "Original",
            "domain": "ml",
        }, body="Original body")

        success = store.update(doc_id, metadata={"title": "Updated"}, body="New body")
        assert success is True

        doc = store.get(doc_id)
        assert doc.title == "Updated"
        assert doc.body == "New body"
        assert doc.version == 2

    def test_update_nonexistent(self, store):
        assert store.update("exp-nonexist", {"title": "X"}) is False

    def test_delete(self, store):
        doc_id = store.create("experiment", {"title": "To Delete"})
        assert store.get(doc_id) is not None

        success = store.delete(doc_id)
        assert success is True
        assert store.get(doc_id) is None

    def test_delete_nonexistent(self, store):
        assert store.delete("exp-nonexist") is False

    def test_create_all_types(self, store):
        exp_id = store.create("experiment", {"title": "Exp"})
        assert exp_id.startswith("exp-")

        pat_id = store.create("pattern", {
            "title": "Pattern",
            "confidence": 0.9,
            "evidence_count": 3,
        })
        assert pat_id.startswith("pat-")

        ins_id = store.create("insight", {
            "title": "Insight",
            "insight_type": "best-practice",
        })
        assert ins_id.startswith("ins-")

        hyp_id = store.create("hypothesis", {
            "title": "Hypothesis",
            "status": "proposed",
            "confidence": 0.5,
        })
        assert hyp_id.startswith("hyp-")


class TestDocumentStoreList:
    def test_list_all(self, store):
        store.create("experiment", {"title": "Exp 1"})
        store.create("pattern", {"title": "Pat 1"})
        store.create("insight", {"title": "Ins 1"})

        docs = store.list_documents()
        assert len(docs) == 3

    def test_list_by_type(self, store):
        store.create("experiment", {"title": "Exp 1"})
        store.create("experiment", {"title": "Exp 2"})
        store.create("pattern", {"title": "Pat 1"})

        docs = store.list_documents(doc_type="experiment")
        assert len(docs) == 2
        assert all(d["type"] == "experiment" for d in docs)

    def test_list_by_domain(self, store):
        store.create("experiment", {"title": "ML Exp", "domain": "ml"})
        store.create("experiment", {"title": "Web Exp", "domain": "web"})

        docs = store.list_documents(domain="ml")
        assert len(docs) == 1
        assert docs[0]["domain"] == "ml"


class TestDocumentStoreFTS:
    def test_search_by_title(self, store):
        store.create("experiment", {"title": "RandomForest Baseline", "domain": "ml"})
        store.create("experiment", {"title": "XGBoost Comparison", "domain": "ml"})

        results = store.search_fts("RandomForest")
        assert len(results) >= 1
        assert results[0]["title"] == "RandomForest Baseline"

    def test_search_by_body(self, store):
        store.create("experiment", {"title": "Test"}, body="accuracy is 0.95 on CIFAR-10")
        store.create("experiment", {"title": "Other"}, body="loss converged at epoch 50")

        results = store.search_fts("CIFAR")
        assert len(results) >= 1

    def test_search_no_results(self, store):
        store.create("experiment", {"title": "Something"})
        results = store.search_fts("nonexistent_query_xyz")
        assert len(results) == 0

    def test_search_by_domain(self, store):
        store.create("experiment", {"title": "Test", "domain": "machine-learning"})
        results = store.search_fts("machine-learning")
        assert len(results) >= 1


class TestDocumentStoreBacklinks:
    def test_find_backlinks(self, store):
        store.create("experiment", {
            "id": "exp-aaaaaaaa",
            "title": "Exp A",
        }, body="First experiment")

        store.create("experiment", {
            "id": "exp-bbbbbbbb",
            "title": "Exp B",
        }, body="Builds on [[exp-aaaaaaaa]] results")

        store.create("experiment", {
            "id": "exp-cccccccc",
            "title": "Exp C",
        }, body="Also references [[exp-aaaaaaaa|Exp A]]")

        backlinks = store.get_backlinks("exp-aaaaaaaa")
        assert set(backlinks) == {"exp-bbbbbbbb", "exp-cccccccc"}

    def test_no_backlinks(self, store):
        store.create("experiment", {
            "id": "exp-aaaaaaaa",
            "title": "Lonely",
        })
        assert store.get_backlinks("exp-aaaaaaaa") == []


class TestDocumentStoreRebuild:
    def test_rebuild_index(self, store):
        store.create("experiment", {"title": "Exp 1", "domain": "ml"})
        store.create("pattern", {"title": "Pat 1", "domain": "ml"})

        count = store.rebuild_index()
        assert count == 2

        # Verify FTS still works after rebuild
        results = store.search_fts("Exp")
        assert len(results) >= 1


class TestTransactionProtection:
    """B1-fix: File + index transaction protection."""

    def test_create_rolls_back_file_on_index_failure(self, store, monkeypatch):
        """If indexing fails during create, the Markdown file should be removed."""

        def failing_index(self, doc, file_path, md_content):
            raise RuntimeError("Simulated index failure")

        monkeypatch.setattr(DocumentStore, "_index_document", failing_index)

        with pytest.raises(RuntimeError, match="Simulated index failure"):
            store.create("experiment", {"title": "Should Not Persist"})

        # No orphan Markdown files should exist
        md_files = list(store.docs_dir.glob("*.md"))
        assert len(md_files) == 0

    def test_update_rolls_back_file_on_index_failure(self, store, monkeypatch):
        """If indexing fails during update, the file should revert to previous content."""
        doc_id = store.create("experiment", {"title": "Original"}, body="Original body")

        def failing_index(self, doc, file_path, md_content):
            raise RuntimeError("Simulated index failure")

        monkeypatch.setattr(DocumentStore, "_index_document", failing_index)

        with pytest.raises(RuntimeError, match="Simulated index failure"):
            store.update(doc_id, metadata={"title": "Updated"}, body="New body")

        # File should still have original content
        doc = store.get(doc_id)
        assert doc.title == "Original"
        assert doc.body == "Original body"


class TestFTSFallbackWarning:
    """B3-fix: FTS fallback should log a warning."""

    def test_fts_bad_syntax_falls_back_to_like(self, store, caplog):
        """Bad FTS5 syntax should fall back to LIKE and log a warning."""
        import logging

        store.create("experiment", {"title": "Test Doc", "domain": "ml"})

        with caplog.at_level(logging.WARNING, logger="c4.knowledge.documents"):
            # Syntax that's invalid for FTS5 (unbalanced quotes)
            store.search_fts('"unclosed')

        # Should have logged a warning about fallback
        assert any("FTS5 query failed" in msg for msg in caplog.messages)


class TestMarkdownFileSSoT:
    def test_markdown_file_is_created(self, store):
        doc_id = store.create("experiment", {"title": "File Test"})
        md_path = store.docs_dir / f"{doc_id}.md"
        assert md_path.exists()

        content = md_path.read_text()
        assert "---" in content
        assert "File Test" in content

    def test_external_edit_reflected_after_get(self, store):
        """Markdown file is SSOT - direct edits are reflected in get()."""
        doc_id = store.create("experiment", {"title": "Original"}, body="Original body")

        # Manually edit the file (simulating external edit)
        md_path = store.docs_dir / f"{doc_id}.md"
        content = md_path.read_text()
        content = content.replace("Original body", "Externally modified body")
        md_path.write_text(content)

        # get() reads from file, should reflect change
        doc = store.get(doc_id)
        assert "Externally modified body" in doc.body
