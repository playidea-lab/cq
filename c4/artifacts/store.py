"""Local artifact store - content-addressable file storage.

Implements ArtifactStore ABC with SHA256-based deduplication.
Storage layout: .c4/artifacts/{hash[:2]}/{hash}
Index: .c4/artifacts/index.db (SQLite)
"""

from __future__ import annotations

import hashlib
import logging
import shutil
import sqlite3
from pathlib import Path

from c4.interfaces import ArtifactStore
from c4.models.task import ArtifactRef

from .models import ArtifactVersion

logger = logging.getLogger(__name__)

_SCHEMA = """
CREATE TABLE IF NOT EXISTS artifacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'output',
    content_hash TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0,
    version INTEGER DEFAULT 1,
    local_path TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_artifacts_task ON artifacts(task_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_name ON artifacts(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_task_name_ver ON artifacts(task_id, name, version);
"""


class LocalArtifactStore(ArtifactStore):
    """Content-addressable local artifact store."""

    def __init__(self, base_path: str | Path = ".c4/artifacts") -> None:
        self.base_path = Path(base_path)
        self.base_path.mkdir(parents=True, exist_ok=True)
        self._db_path = self.base_path / "index.db"
        self._init_db()

    def _init_db(self) -> None:
        with sqlite3.connect(str(self._db_path)) as conn:
            conn.executescript(_SCHEMA)

    def _get_conn(self) -> sqlite3.Connection:
        conn = sqlite3.connect(str(self._db_path))
        conn.row_factory = sqlite3.Row
        return conn

    async def save(
        self,
        task_id: str,
        local_path: Path,
        artifact_type: str = "output",
    ) -> ArtifactRef:
        """Save artifact with content-addressable storage."""
        local_path = Path(local_path)
        if not local_path.exists():
            raise FileNotFoundError(f"Artifact source not found: {local_path}")

        content_hash = _compute_hash(local_path)
        size_bytes = local_path.stat().st_size
        name = local_path.name

        # Content-addressable path
        store_dir = self.base_path / content_hash[:2]
        store_dir.mkdir(parents=True, exist_ok=True)
        store_path = store_dir / content_hash

        # Copy only if not already stored (dedup)
        if not store_path.exists():
            shutil.copy2(local_path, store_path)

        # Get next version
        version = self._next_version(task_id, name)

        # Record in index
        rel_path = str(store_path.relative_to(self.base_path))
        with self._get_conn() as conn:
            conn.execute(
                "INSERT INTO artifacts (task_id, name, type, content_hash, size_bytes, version, local_path) "
                "VALUES (?, ?, ?, ?, ?, ?, ?)",
                (task_id, name, artifact_type, content_hash, size_bytes, version, rel_path),
            )

        return ArtifactRef(
            name=name,
            type=artifact_type,
            content_hash=content_hash,
            size_bytes=size_bytes,
            version=version,
            local_path=rel_path,
        )

    async def get(
        self,
        task_id: str,
        name: str,
        version: int | None = None,
    ) -> Path:
        """Get artifact file path."""
        with self._get_conn() as conn:
            if version is not None:
                row = conn.execute(
                    "SELECT local_path FROM artifacts WHERE task_id=? AND name=? AND version=?",
                    (task_id, name, version),
                ).fetchone()
            else:
                row = conn.execute(
                    "SELECT local_path FROM artifacts WHERE task_id=? AND name=? ORDER BY version DESC LIMIT 1",
                    (task_id, name),
                ).fetchone()

        if not row:
            raise FileNotFoundError(f"Artifact not found: {task_id}/{name}")

        return self.base_path / row["local_path"]

    async def list(self, task_id: str) -> list[ArtifactRef]:
        """List artifacts for a task."""
        with self._get_conn() as conn:
            rows = conn.execute(
                "SELECT name, type, content_hash, size_bytes, version, local_path "
                "FROM artifacts WHERE task_id=? ORDER BY name, version DESC",
                (task_id,),
            ).fetchall()

        return [
            ArtifactRef(
                name=r["name"],
                type=r["type"],
                content_hash=r["content_hash"],
                size_bytes=r["size_bytes"],
                version=r["version"],
                local_path=r["local_path"],
            )
            for r in rows
        ]

    async def delete(self, task_id: str, name: str) -> bool:
        """Delete all versions of an artifact."""
        with self._get_conn() as conn:
            cursor = conn.execute(
                "DELETE FROM artifacts WHERE task_id=? AND name=?",
                (task_id, name),
            )
        return cursor.rowcount > 0

    async def versions(self, task_id: str, name: str) -> list[ArtifactVersion]:
        """Get version history for an artifact."""
        with self._get_conn() as conn:
            rows = conn.execute(
                "SELECT version, content_hash, size_bytes, created_at "
                "FROM artifacts WHERE task_id=? AND name=? ORDER BY version",
                (task_id, name),
            ).fetchall()

        return [
            ArtifactVersion(
                version=r["version"],
                content_hash=r["content_hash"],
                size_bytes=r["size_bytes"],
                created_at=r["created_at"],
            )
            for r in rows
        ]

    def _next_version(self, task_id: str, name: str) -> int:
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT MAX(version) as max_ver FROM artifacts WHERE task_id=? AND name=?",
                (task_id, name),
            ).fetchone()
        current = row["max_ver"] if row and row["max_ver"] is not None else 0
        return current + 1


def _compute_hash(path: Path) -> str:
    """Compute SHA256 hash of a file."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while chunk := f.read(8192):
            h.update(chunk)
    return h.hexdigest()
