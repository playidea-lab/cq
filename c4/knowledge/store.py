"""Local Knowledge Store - SQLite-based experiment knowledge repository.

Implements KnowledgeStore ABC with synchronous SQLite backend.
Storage: .c4/knowledge/experiments.db
"""

from __future__ import annotations

import json
import logging
import sqlite3
import uuid
from pathlib import Path
from typing import Any

from c4.interfaces import KnowledgeStore

from .models import ExperimentKnowledge, Pattern

logger = logging.getLogger(__name__)

_SCHEMA = """
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
CREATE INDEX IF NOT EXISTS idx_exp_task ON experiments(task_id);
CREATE INDEX IF NOT EXISTS idx_exp_domain ON experiments(domain);

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
CREATE INDEX IF NOT EXISTS idx_pat_domain ON patterns(domain);
"""


class LocalKnowledgeStore(KnowledgeStore):
    """Local SQLite-based knowledge store."""

    def __init__(self, base_path: str | Path = ".c4/knowledge") -> None:
        self.base_path = Path(base_path)
        self.base_path.mkdir(parents=True, exist_ok=True)
        self._db_path = self.base_path / "experiments.db"
        self._init_db()

    def _init_db(self) -> None:
        with sqlite3.connect(str(self._db_path)) as conn:
            conn.executescript(_SCHEMA)

    def _get_conn(self) -> sqlite3.Connection:
        conn = sqlite3.connect(str(self._db_path))
        conn.row_factory = sqlite3.Row
        return conn

    async def save_experiment(self, experiment: dict[str, Any]) -> str:
        """Save experiment knowledge."""
        exp = ExperimentKnowledge(**experiment) if isinstance(experiment, dict) else experiment
        if not exp.id:
            exp.id = f"exp-{uuid.uuid4().hex[:8]}"

        with self._get_conn() as conn:
            conn.execute(
                "INSERT OR REPLACE INTO experiments "
                "(id, task_id, title, hypothesis, hypothesis_status, "
                "config_json, result_json, observations_json, lessons_json, "
                "tags_json, domain, created_at) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    exp.id,
                    exp.task_id,
                    exp.title,
                    exp.hypothesis,
                    exp.hypothesis_status.value,
                    json.dumps(exp.config),
                    exp.result.model_dump_json(),
                    json.dumps([o.model_dump() for o in exp.observations]),
                    json.dumps(exp.lessons_learned),
                    json.dumps(exp.tags),
                    exp.domain,
                    exp.created_at,
                ),
            )

        logger.debug("Saved experiment: %s", exp.id)
        return exp.id

    async def search(self, query: str, top_k: int = 5) -> list[dict[str, Any]]:
        """Search experiments by keyword matching.

        Matches against title, hypothesis, lessons, and tags.
        """
        query_lower = query.lower()
        keywords = query_lower.split()

        with self._get_conn() as conn:
            rows = conn.execute(
                "SELECT * FROM experiments ORDER BY created_at DESC"
            ).fetchall()

        results = []
        for row in rows:
            score = _compute_relevance(row, keywords)
            if score > 0:
                results.append((score, _row_to_dict(row)))

        results.sort(key=lambda x: x[0], reverse=True)
        return [r[1] for r in results[:top_k]]

    async def get_patterns(self, domain: str | None = None) -> list[dict[str, Any]]:
        """Get patterns, optionally filtered by domain."""
        with self._get_conn() as conn:
            if domain:
                rows = conn.execute(
                    "SELECT * FROM patterns WHERE domain=? ORDER BY confidence DESC",
                    (domain,),
                ).fetchall()
            else:
                rows = conn.execute(
                    "SELECT * FROM patterns ORDER BY confidence DESC"
                ).fetchall()

        return [_pattern_row_to_dict(r) for r in rows]

    async def save_pattern(self, pattern: Pattern) -> str:
        """Save or update a pattern."""
        if not pattern.id:
            pattern.id = f"pat-{uuid.uuid4().hex[:8]}"

        with self._get_conn() as conn:
            conn.execute(
                "INSERT OR REPLACE INTO patterns "
                "(id, name, description, domain, confidence, "
                "evidence_count, evidence_ids_json, config_pattern_json, created_at) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    pattern.id,
                    pattern.name,
                    pattern.description,
                    pattern.domain,
                    pattern.confidence,
                    pattern.evidence_count,
                    json.dumps(pattern.evidence_ids),
                    json.dumps(pattern.config_pattern),
                    pattern.created_at,
                ),
            )
        return pattern.id

    async def get_experiment(self, exp_id: str) -> dict[str, Any] | None:
        """Get a specific experiment by ID."""
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT * FROM experiments WHERE id=?", (exp_id,)
            ).fetchone()

        if not row:
            return None
        return _row_to_dict(row)

    async def list_experiments(
        self, task_id: str | None = None, domain: str | None = None, limit: int = 50
    ) -> list[dict[str, Any]]:
        """List experiments with optional filters."""
        conditions = []
        params: list[Any] = []

        if task_id:
            conditions.append("task_id=?")
            params.append(task_id)
        if domain:
            conditions.append("domain=?")
            params.append(domain)

        where = f" WHERE {' AND '.join(conditions)}" if conditions else ""
        query = f"SELECT * FROM experiments{where} ORDER BY created_at DESC LIMIT ?"
        params.append(limit)

        with self._get_conn() as conn:
            rows = conn.execute(query, params).fetchall()

        return [_row_to_dict(r) for r in rows]


def _compute_relevance(row: sqlite3.Row, keywords: list[str]) -> float:
    """Compute keyword relevance score for an experiment row."""
    text = " ".join([
        row["title"].lower(),
        row["hypothesis"].lower(),
        row["lessons_json"].lower(),
        row["tags_json"].lower(),
        row["domain"].lower(),
    ])

    score = 0.0
    for kw in keywords:
        if kw in text:
            score += 1.0
            # Boost for title match
            if kw in row["title"].lower():
                score += 0.5
    return score


def _row_to_dict(row: sqlite3.Row) -> dict[str, Any]:
    """Convert experiment row to dict."""
    return {
        "id": row["id"],
        "task_id": row["task_id"],
        "title": row["title"],
        "hypothesis": row["hypothesis"],
        "hypothesis_status": row["hypothesis_status"],
        "config": json.loads(row["config_json"]),
        "result": json.loads(row["result_json"]),
        "observations": json.loads(row["observations_json"]),
        "lessons_learned": json.loads(row["lessons_json"]),
        "tags": json.loads(row["tags_json"]),
        "domain": row["domain"],
        "created_at": row["created_at"],
    }


def _pattern_row_to_dict(row: sqlite3.Row) -> dict[str, Any]:
    """Convert pattern row to dict."""
    return {
        "id": row["id"],
        "name": row["name"],
        "description": row["description"],
        "domain": row["domain"],
        "confidence": row["confidence"],
        "evidence_count": row["evidence_count"],
        "evidence_ids": json.loads(row["evidence_ids_json"]),
        "config_pattern": json.loads(row["config_pattern_json"]),
        "created_at": row["created_at"],
    }
