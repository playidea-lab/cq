"""DocumentStore - Obsidian-style Markdown document CRUD with FTS5 index.

Markdown 파일이 SSOT (Single Source of Truth).
index.db는 메타데이터 인덱스 + FTS5 검색용 파생물.

Usage:
    from c4.knowledge.documents import DocumentStore

    store = DocumentStore(base_path=".c4/knowledge")
    doc_id = store.create("experiment", {
        "title": "RF Baseline",
        "domain": "ml",
        "hypothesis": "RF achieves 85%+",
    }, body="# RF Baseline\\n## Result\\n- accuracy: 0.87")

    doc = store.get(doc_id)
    results = store.search_fts("random forest")
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import sqlite3
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from c4.interfaces import KnowledgeStore

from .models import BACKLINK_RE, DOC_TYPE_PREFIXES, DocumentType, KnowledgeDocument

logger = logging.getLogger(__name__)

_INDEX_SCHEMA = """
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    domain TEXT DEFAULT '',
    tags_json TEXT DEFAULT '[]',
    hypothesis_status TEXT DEFAULT '',
    confidence REAL DEFAULT 0.0,
    task_id TEXT DEFAULT '',
    metadata_json TEXT DEFAULT '{}',
    file_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_doc_type ON documents(type);
CREATE INDEX IF NOT EXISTS idx_doc_domain ON documents(domain);
CREATE INDEX IF NOT EXISTS idx_doc_task ON documents(task_id);
CREATE INDEX IF NOT EXISTS idx_doc_hypothesis ON documents(hypothesis_status);
"""

_FTS_SCHEMA = """
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    id, title, domain, tags_text, body_text
);
"""

# Frontmatter fields to exclude from metadata_json (stored in dedicated columns)
_DEDICATED_COLUMNS = {
    "id", "type", "title", "domain", "tags",
    "hypothesis_status", "confidence", "task_id",
    "created_at", "updated_at", "version",
}


def _generate_id(doc_type: str) -> str:
    """Generate a document ID with type prefix."""
    prefix_map = {v.value: k for k, v in DOC_TYPE_PREFIXES.items()}
    prefix = prefix_map.get(doc_type, "doc")
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


def _content_hash(content: str) -> str:
    """SHA256 hash of content for change detection."""
    return hashlib.sha256(content.encode()).hexdigest()[:16]


def _parse_frontmatter(text: str) -> tuple[dict[str, Any], str]:
    """Parse YAML frontmatter and body from Markdown text.

    Returns:
        (frontmatter_dict, body_text)
    """
    match = re.match(r"^---\n(.*?)\n---\n?(.*)", text, re.DOTALL)
    if not match:
        # Try empty frontmatter: ---\n---
        match2 = re.match(r"^---\n---\n?(.*)", text, re.DOTALL)
        if match2:
            return {}, match2.group(1).strip()
        return {}, text

    try:
        fm = yaml.safe_load(match.group(1)) or {}
    except yaml.YAMLError:
        fm = {}

    return fm, match.group(2).strip()


def _render_frontmatter(meta: dict[str, Any]) -> str:
    """Render frontmatter dict to YAML string."""
    return yaml.dump(meta, default_flow_style=False, allow_unicode=True, sort_keys=False).strip()


def _doc_to_markdown(doc: KnowledgeDocument) -> str:
    """Convert KnowledgeDocument to Markdown with YAML frontmatter."""
    fm: dict[str, Any] = {
        "id": doc.id,
        "type": doc.type.value,
        "title": doc.title,
    }
    if doc.domain:
        fm["domain"] = doc.domain
    if doc.task_id:
        fm["task_id"] = doc.task_id
    if doc.tags:
        fm["tags"] = doc.tags

    # Type-specific fields
    if doc.type == DocumentType.EXPERIMENT:
        if doc.hypothesis:
            fm["hypothesis"] = doc.hypothesis
        if doc.hypothesis_status:
            fm["hypothesis_status"] = doc.hypothesis_status
        if doc.parent_experiment:
            fm["parent_experiment"] = doc.parent_experiment
        if doc.compared_to:
            fm["compared_to"] = doc.compared_to
        if doc.builds_on:
            fm["builds_on"] = doc.builds_on

    elif doc.type == DocumentType.PATTERN:
        fm["confidence"] = doc.confidence
        fm["evidence_count"] = doc.evidence_count
        if doc.evidence_ids:
            fm["evidence_ids"] = doc.evidence_ids

    elif doc.type == DocumentType.INSIGHT:
        if doc.insight_type:
            fm["insight_type"] = doc.insight_type
        if doc.source_count:
            fm["source_count"] = doc.source_count

    elif doc.type == DocumentType.HYPOTHESIS:
        if doc.status:
            fm["status"] = doc.status
        fm["confidence"] = doc.confidence
        if doc.evidence_for:
            fm["evidence_for"] = doc.evidence_for
        if doc.evidence_against:
            fm["evidence_against"] = doc.evidence_against

    fm["created_at"] = doc.created_at
    fm["updated_at"] = doc.updated_at
    fm["version"] = doc.version

    return f"---\n{_render_frontmatter(fm)}\n---\n\n{doc.body}"


class DocumentStore(KnowledgeStore):
    """Obsidian-style knowledge document store.

    - CRUD over Markdown files in docs/
    - Maintains index.db with metadata + FTS5
    - Markdown files are SSOT; index.db is derived
    """

    def __init__(self, base_path: str | Path = ".c4/knowledge") -> None:
        self.base_path = Path(base_path)
        self.docs_dir = self.base_path / "docs"
        self.docs_dir.mkdir(parents=True, exist_ok=True)
        self._db_path = self.base_path / "index.db"
        self._conn: sqlite3.Connection | None = None
        self._init_db()

    def __enter__(self) -> "DocumentStore":
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    def close(self) -> None:
        """Close the database connection."""
        if self._conn is not None:
            self._conn.close()
            self._conn = None

    def _init_db(self) -> None:
        conn = self._get_conn()
        conn.executescript(_INDEX_SCHEMA)
        # FTS table needs special handling - check if exists
        cursor = conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' AND name='documents_fts'"
        )
        if cursor.fetchone() is None:
            conn.executescript(_FTS_SCHEMA)
        conn.commit()

    def _get_conn(self) -> sqlite3.Connection:
        if self._conn is None:
            self._conn = sqlite3.connect(str(self._db_path))
            self._conn.row_factory = sqlite3.Row
            self._conn.execute("PRAGMA busy_timeout=5000")
        return self._conn

    def create(
        self,
        doc_type: str,
        metadata: dict[str, Any],
        body: str = "",
    ) -> str:
        """Create a new knowledge document.

        Args:
            doc_type: "experiment", "pattern", "insight", "hypothesis"
            metadata: Frontmatter fields (title, domain, tags, etc.)
            body: Markdown body content

        Returns:
            Generated document ID
        """
        doc_id = metadata.get("id") or _generate_id(doc_type)
        now = datetime.now(tz=timezone.utc).isoformat()

        doc = KnowledgeDocument(
            id=doc_id,
            type=DocumentType(doc_type),
            title=metadata.get("title", ""),
            domain=metadata.get("domain", ""),
            tags=metadata.get("tags", []),
            task_id=metadata.get("task_id", ""),
            hypothesis=metadata.get("hypothesis", ""),
            hypothesis_status=metadata.get("hypothesis_status", ""),
            parent_experiment=metadata.get("parent_experiment"),
            compared_to=metadata.get("compared_to", []),
            builds_on=metadata.get("builds_on", []),
            confidence=metadata.get("confidence", 0.0),
            evidence_count=metadata.get("evidence_count", 0),
            evidence_ids=metadata.get("evidence_ids", []),
            insight_type=metadata.get("insight_type", ""),
            source_count=metadata.get("source_count", 0),
            status=metadata.get("status", ""),
            evidence_for=metadata.get("evidence_for", []),
            evidence_against=metadata.get("evidence_against", []),
            created_at=metadata.get("created_at", now),
            updated_at=now,
            version=1,
            body=body,
        )

        # Write Markdown file, then index. Rollback file on index failure.
        md_content = _doc_to_markdown(doc)
        file_path = self.docs_dir / f"{doc_id}.md"
        file_path.write_text(md_content, encoding="utf-8")

        try:
            self._index_document(doc, file_path, md_content)
        except Exception:
            # Rollback: remove file if indexing failed
            file_path.unlink(missing_ok=True)
            raise

        logger.debug("Created document: %s (%s)", doc_id, doc_type)
        return doc_id

    def get(self, doc_id: str) -> KnowledgeDocument | None:
        """Get a document by ID, reading from Markdown file.

        Returns:
            KnowledgeDocument or None if not found
        """
        file_path = self.docs_dir / f"{doc_id}.md"
        if not file_path.exists():
            return None

        text = file_path.read_text(encoding="utf-8")
        fm, body = _parse_frontmatter(text)

        return KnowledgeDocument(
            id=fm.get("id", doc_id),
            type=DocumentType(fm.get("type", "experiment")),
            title=fm.get("title", ""),
            domain=fm.get("domain", ""),
            tags=fm.get("tags", []),
            task_id=fm.get("task_id", ""),
            hypothesis=fm.get("hypothesis", ""),
            hypothesis_status=fm.get("hypothesis_status", ""),
            parent_experiment=fm.get("parent_experiment"),
            compared_to=fm.get("compared_to", []),
            builds_on=fm.get("builds_on", []),
            confidence=fm.get("confidence", 0.0),
            evidence_count=fm.get("evidence_count", 0),
            evidence_ids=fm.get("evidence_ids", []),
            insight_type=fm.get("insight_type", ""),
            source_count=fm.get("source_count", 0),
            status=fm.get("status", ""),
            evidence_for=fm.get("evidence_for", []),
            evidence_against=fm.get("evidence_against", []),
            created_at=fm.get("created_at", ""),
            updated_at=fm.get("updated_at", ""),
            version=fm.get("version", 1),
            body=body,
        )

    def update(
        self,
        doc_id: str,
        metadata: dict[str, Any] | None = None,
        body: str | None = None,
    ) -> bool:
        """Update an existing document.

        Args:
            doc_id: Document ID to update
            metadata: Fields to update in frontmatter (merged, not replaced)
            body: New body content (if None, keeps existing)

        Returns:
            True if updated, False if not found
        """
        doc = self.get(doc_id)
        if doc is None:
            return False

        now = datetime.now(tz=timezone.utc).isoformat()

        if metadata:
            for key, value in metadata.items():
                if hasattr(doc, key):
                    setattr(doc, key, value)

        if body is not None:
            doc.body = body

        doc.updated_at = now
        doc.version += 1

        # Write updated Markdown, rollback on index failure
        md_content = _doc_to_markdown(doc)
        file_path = self.docs_dir / f"{doc_id}.md"
        old_content = file_path.read_text(encoding="utf-8") if file_path.exists() else None
        file_path.write_text(md_content, encoding="utf-8")

        try:
            self._index_document(doc, file_path, md_content)
        except Exception:
            # Rollback: restore previous file content
            if old_content is not None:
                file_path.write_text(old_content, encoding="utf-8")
            else:
                file_path.unlink(missing_ok=True)
            raise

        return True

    def delete(self, doc_id: str) -> bool:
        """Delete a document.

        Returns:
            True if deleted, False if not found
        """
        file_path = self.docs_dir / f"{doc_id}.md"
        if not file_path.exists():
            return False

        file_path.unlink()

        with self._get_conn() as conn:
            # Delete from FTS
            conn.execute(
                "DELETE FROM documents_fts WHERE id = ?", (doc_id,)
            )
            # Delete from main table
            conn.execute("DELETE FROM documents WHERE id = ?", (doc_id,))

        return True

    def list_documents(
        self,
        doc_type: str | None = None,
        domain: str | None = None,
        limit: int = 50,
    ) -> list[dict[str, Any]]:
        """List documents with optional filters.

        Returns:
            List of document metadata dicts (no body)
        """
        conditions = []
        params: list[Any] = []

        if doc_type:
            conditions.append("type = ?")
            params.append(doc_type)
        if domain:
            conditions.append("domain = ?")
            params.append(domain)

        where = f" WHERE {' AND '.join(conditions)}" if conditions else ""
        query = f"SELECT * FROM documents{where} ORDER BY updated_at DESC LIMIT ?"
        params.append(limit)

        with self._get_conn() as conn:
            rows = conn.execute(query, params).fetchall()

        return [self._row_to_summary(r) for r in rows]

    def search_fts(self, query: str, top_k: int = 10) -> list[dict[str, Any]]:
        """Full-text search using FTS5.

        Args:
            query: Search query (FTS5 syntax supported)
            top_k: Maximum results

        Returns:
            List of {id, title, type, score, snippet}
        """
        with self._get_conn() as conn:
            try:
                rows = conn.execute(
                    """
                    SELECT d.id, d.title, d.type, d.domain,
                           rank AS score
                    FROM documents_fts f
                    JOIN documents d ON d.id = f.id
                    WHERE documents_fts MATCH ?
                    ORDER BY rank
                    LIMIT ?
                    """,
                    (query, top_k),
                ).fetchall()
            except sqlite3.OperationalError as fts_err:
                # Bad FTS query syntax - fall back to simple LIKE
                logger.warning("FTS5 query failed, falling back to LIKE: %s", fts_err)
                like_q = f"%{query}%"
                rows = conn.execute(
                    """
                    SELECT id, title, type, domain, 0.0 AS score
                    FROM documents
                    WHERE title LIKE ? OR domain LIKE ?
                    ORDER BY updated_at DESC
                    LIMIT ?
                    """,
                    (like_q, like_q, top_k),
                ).fetchall()

        return [
            {
                "id": r["id"],
                "title": r["title"],
                "type": r["type"],
                "domain": r["domain"],
                "score": abs(r["score"]),  # FTS5 rank is negative
            }
            for r in rows
        ]

    def get_backlinks(self, doc_id: str) -> list[str]:
        """Find all documents that reference doc_id via [[backlink]].

        Returns:
            List of document IDs that contain [[doc_id]]
        """
        backlinks = []
        for md_file in self.docs_dir.glob("*.md"):
            if md_file.stem == doc_id:
                continue
            text = md_file.read_text(encoding="utf-8")
            refs = BACKLINK_RE.findall(text)
            if doc_id in refs:
                backlinks.append(md_file.stem)
        return backlinks

    def rebuild_index(self) -> int:
        """Rebuild index.db from Markdown files.

        Returns:
            Number of documents indexed
        """
        with self._get_conn() as conn:
            conn.execute("DELETE FROM documents_fts")
            conn.execute("DELETE FROM documents")

        count = 0
        for md_file in sorted(self.docs_dir.glob("*.md")):
            text = md_file.read_text(encoding="utf-8")
            fm, body = _parse_frontmatter(text)

            doc_id = fm.get("id", md_file.stem)
            doc_type = fm.get("type", "experiment")

            doc = KnowledgeDocument(
                id=doc_id,
                type=DocumentType(doc_type),
                title=fm.get("title", ""),
                domain=fm.get("domain", ""),
                tags=fm.get("tags", []),
                task_id=fm.get("task_id", ""),
                hypothesis_status=fm.get("hypothesis_status", ""),
                confidence=fm.get("confidence", 0.0),
                created_at=fm.get("created_at", ""),
                updated_at=fm.get("updated_at", ""),
                version=fm.get("version", 1),
                body=body,
            )

            self._index_document(doc, md_file, text)
            count += 1

        logger.info("Rebuilt index: %d documents", count)
        return count

    def _index_document(
        self,
        doc: KnowledgeDocument,
        file_path: Path,
        md_content: str,
    ) -> None:
        """Insert or replace document in index.db + FTS."""
        # Build metadata_json from non-dedicated fields
        full_meta = doc.model_dump(exclude={"body"})
        extra_meta = {k: v for k, v in full_meta.items() if k not in _DEDICATED_COLUMNS}

        tags_text = " ".join(doc.tags)
        ch = _content_hash(md_content)

        with self._get_conn() as conn:
            conn.execute(
                """INSERT OR REPLACE INTO documents
                   (id, type, title, domain, tags_json, hypothesis_status,
                    confidence, task_id, metadata_json, file_path,
                    content_hash, created_at, updated_at, version)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
                (
                    doc.id,
                    doc.type.value,
                    doc.title,
                    doc.domain,
                    json.dumps(doc.tags),
                    doc.hypothesis_status,
                    doc.confidence,
                    doc.task_id,
                    json.dumps(extra_meta, default=str),
                    str(file_path),
                    ch,
                    doc.created_at,
                    doc.updated_at,
                    doc.version,
                ),
            )

            # Update FTS (delete + insert)
            conn.execute("DELETE FROM documents_fts WHERE id = ?", (doc.id,))
            conn.execute(
                """INSERT INTO documents_fts (id, title, domain, tags_text, body_text)
                   VALUES (?, ?, ?, ?, ?)""",
                (doc.id, doc.title, doc.domain, tags_text, doc.body),
            )

    @staticmethod
    def _row_to_summary(row: sqlite3.Row) -> dict[str, Any]:
        """Convert index row to summary dict (no body)."""
        return {
            "id": row["id"],
            "type": row["type"],
            "title": row["title"],
            "domain": row["domain"],
            "tags": json.loads(row["tags_json"]),
            "hypothesis_status": row["hypothesis_status"],
            "confidence": row["confidence"],
            "task_id": row["task_id"],
            "created_at": row["created_at"],
            "updated_at": row["updated_at"],
            "version": row["version"],
        }
