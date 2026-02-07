"""Tests for c4.knowledge.migration - experiments.db → Markdown migration."""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path

import pytest


@pytest.fixture()
def legacy_db(tmp_path: Path) -> Path:
    """Create a legacy experiments.db with sample data."""
    knowledge_dir = tmp_path / "knowledge"
    knowledge_dir.mkdir(parents=True)
    db_path = knowledge_dir / "experiments.db"

    conn = sqlite3.connect(str(db_path))
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS experiments (
            id TEXT PRIMARY KEY,
            task_id TEXT NOT NULL,
            title TEXT NOT NULL DEFAULT '',
            hypothesis TEXT DEFAULT '',
            hypothesis_status TEXT DEFAULT 'proposed',
            config_json TEXT DEFAULT '{}',
            result_json TEXT DEFAULT '{}',
            observations_json TEXT DEFAULT '[]',
            lessons_json TEXT DEFAULT '[]',
            tags_json TEXT DEFAULT '[]',
            domain TEXT DEFAULT '',
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS patterns (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT DEFAULT '',
            domain TEXT DEFAULT '',
            confidence REAL DEFAULT 0.0,
            evidence_count INTEGER DEFAULT 0,
            evidence_ids_json TEXT DEFAULT '[]',
            config_pattern_json TEXT DEFAULT '{}',
            created_at TEXT NOT NULL
        );
    """)

    # Insert sample experiments
    conn.execute(
        "INSERT INTO experiments VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (
            "exp-11111111",
            "T-001-0",
            "RF Baseline",
            "RF achieves 85%+",
            "supported",
            json.dumps({"n_estimators": 100, "algorithm": "RandomForest"}),
            json.dumps({"success": True, "metrics": {"accuracy": 0.87}}),
            json.dumps([]),
            json.dumps(["Default params work well"]),
            json.dumps(["sklearn", "classification"]),
            "ml",
            "2026-01-15T10:00:00Z",
        ),
    )
    conn.execute(
        "INSERT INTO experiments VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (
            "exp-22222222",
            "T-002-0",
            "XGBoost Attempt",
            "XGBoost outperforms RF",
            "refuted",
            json.dumps({"algorithm": "XGBoost"}),
            json.dumps({"success": False, "metrics": {"accuracy": 0.80}}),
            json.dumps([]),
            json.dumps(["Needs more hyperparameter tuning"]),
            json.dumps(["xgboost"]),
            "ml",
            "2026-01-16T10:00:00Z",
        ),
    )

    # Insert sample pattern
    conn.execute(
        "INSERT INTO patterns VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (
            "pat-33333333",
            "success_config:algorithm",
            "algorithm=RandomForest in 1/1 successful",
            "ml",
            0.8,
            1,
            json.dumps(["exp-11111111"]),
            json.dumps({"algorithm": "RandomForest"}),
            "2026-01-17T10:00:00Z",
        ),
    )
    conn.commit()
    conn.close()

    return knowledge_dir


class TestMigration:
    """Test experiments.db → Markdown migration."""

    def test_migrate_creates_markdown_files(self, legacy_db: Path) -> None:
        from c4.knowledge.migration import migrate_experiments_db

        result = migrate_experiments_db(base_path=legacy_db)

        assert result["migrated_count"] == 3  # 2 experiments + 1 pattern
        assert result["errors"] == []

        # Check experiment files exist
        docs_dir = legacy_db / "docs"
        assert (docs_dir / "exp-11111111.md").exists()
        assert (docs_dir / "exp-22222222.md").exists()
        assert (docs_dir / "pat-33333333.md").exists()

    def test_migrate_preserves_experiment_content(self, legacy_db: Path) -> None:
        from c4.knowledge.documents import DocumentStore
        from c4.knowledge.migration import migrate_experiments_db

        migrate_experiments_db(base_path=legacy_db)

        store = DocumentStore(base_path=legacy_db)
        doc = store.get("exp-11111111")

        assert doc is not None
        assert doc.title == "RF Baseline"
        assert doc.domain == "ml"
        assert doc.task_id == "T-001-0"
        assert doc.hypothesis == "RF achieves 85%+"
        assert doc.hypothesis_status == "supported"
        assert "sklearn" in doc.tags

    def test_migrate_preserves_pattern_content(self, legacy_db: Path) -> None:
        from c4.knowledge.documents import DocumentStore
        from c4.knowledge.migration import migrate_experiments_db

        migrate_experiments_db(base_path=legacy_db)

        store = DocumentStore(base_path=legacy_db)
        doc = store.get("pat-33333333")

        assert doc is not None
        assert doc.title == "success_config:algorithm"
        assert doc.domain == "ml"
        assert doc.confidence == 0.8

    def test_migrate_body_contains_experiment_data(self, legacy_db: Path) -> None:
        from c4.knowledge.documents import DocumentStore
        from c4.knowledge.migration import migrate_experiments_db

        migrate_experiments_db(base_path=legacy_db)

        store = DocumentStore(base_path=legacy_db)
        doc = store.get("exp-11111111")

        assert doc is not None
        assert "RF Baseline" in doc.body
        assert "RF achieves 85%+" in doc.body
        assert "n_estimators" in doc.body
        assert "Default params work well" in doc.body

    def test_migrate_pattern_body_has_backlinks(self, legacy_db: Path) -> None:
        from c4.knowledge.documents import DocumentStore
        from c4.knowledge.migration import migrate_experiments_db

        migrate_experiments_db(base_path=legacy_db)

        store = DocumentStore(base_path=legacy_db)
        doc = store.get("pat-33333333")

        assert doc is not None
        assert "[[exp-11111111]]" in doc.body

    def test_migrate_creates_backup(self, legacy_db: Path) -> None:
        from c4.knowledge.migration import migrate_experiments_db

        result = migrate_experiments_db(base_path=legacy_db)

        assert "backup" in result
        backup_path = Path(result["backup"])
        assert backup_path.exists()
        assert backup_path.suffix == ".bak"

    def test_migrate_skips_already_migrated(self, legacy_db: Path) -> None:
        from c4.knowledge.migration import migrate_experiments_db

        # First migration
        result1 = migrate_experiments_db(base_path=legacy_db)
        assert result1["migrated_count"] == 3

        # Second migration should skip all
        result2 = migrate_experiments_db(base_path=legacy_db)
        assert result2["migrated_count"] == 0
        assert result2["skipped_count"] == 3

    def test_migrate_dry_run(self, legacy_db: Path) -> None:
        from c4.knowledge.migration import migrate_experiments_db

        result = migrate_experiments_db(base_path=legacy_db, dry_run=True)

        assert result["dry_run"] is True
        assert result["experiments_found"] == 2
        assert result["patterns_found"] == 1
        # No files should be created
        docs_dir = legacy_db / "docs"
        assert not docs_dir.exists() or len(list(docs_dir.glob("*.md"))) == 0

    def test_migrate_no_db(self, tmp_path: Path) -> None:
        from c4.knowledge.migration import migrate_experiments_db

        result = migrate_experiments_db(base_path=tmp_path / "nonexistent")
        assert result["migrated_count"] == 0
        assert "No experiments.db found" in result.get("message", "")

    def test_migrated_docs_searchable_via_fts(self, legacy_db: Path) -> None:
        from c4.knowledge.documents import DocumentStore
        from c4.knowledge.migration import migrate_experiments_db

        migrate_experiments_db(base_path=legacy_db)

        store = DocumentStore(base_path=legacy_db)
        results = store.search_fts("Baseline", top_k=5)

        assert len(results) >= 1
        assert any(r["id"] == "exp-11111111" for r in results)
