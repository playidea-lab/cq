"""Migration: experiments.db → Obsidian-style Markdown documents.

Reads legacy experiments.db (v1) and creates Markdown documents
in the v2 DocumentStore format. Original DB is backed up as .bak.
"""

from __future__ import annotations

import json
import logging
import shutil
import sqlite3
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


def migrate_experiments_db(
    base_path: str | Path = ".c4/knowledge",
    dry_run: bool = False,
) -> dict[str, Any]:
    """Migrate experiments.db rows to Obsidian-style Markdown documents.

    Args:
        base_path: Knowledge store base directory.
        dry_run: If True, report what would be migrated without writing.

    Returns:
        Dict with migrated_count, skipped_count, errors.
    """
    from .documents import DocumentStore

    base = Path(base_path)
    db_path = base / "experiments.db"

    if not db_path.exists():
        return {"migrated_count": 0, "skipped_count": 0, "errors": [], "message": "No experiments.db found"}

    # Read legacy data
    experiments = _read_legacy_experiments(db_path)
    patterns = _read_legacy_patterns(db_path)

    if dry_run:
        return {
            "migrated_count": 0,
            "skipped_count": 0,
            "experiments_found": len(experiments),
            "patterns_found": len(patterns),
            "dry_run": True,
        }

    store = DocumentStore(base_path=base)
    migrated = 0
    skipped = 0
    errors: list[str] = []

    # Migrate experiments
    for exp in experiments:
        try:
            doc_id = _migrate_experiment(store, exp)
            if doc_id:
                migrated += 1
            else:
                skipped += 1
        except Exception as e:
            errors.append(f"experiment {exp.get('id', '?')}: {e}")

    # Migrate patterns
    for pat in patterns:
        try:
            doc_id = _migrate_pattern(store, pat)
            if doc_id:
                migrated += 1
            else:
                skipped += 1
        except Exception as e:
            errors.append(f"pattern {pat.get('id', '?')}: {e}")

    # Backup original DB
    backup_path = db_path.with_suffix(".db.bak")
    shutil.copy2(db_path, backup_path)
    logger.info("Backed up %s → %s", db_path, backup_path)

    return {
        "migrated_count": migrated,
        "skipped_count": skipped,
        "errors": errors,
        "backup": str(backup_path),
    }


def _read_legacy_experiments(db_path: Path) -> list[dict[str, Any]]:
    """Read all experiments from legacy SQLite DB."""
    try:
        conn = sqlite3.connect(str(db_path))
        conn.row_factory = sqlite3.Row
        rows = conn.execute("SELECT * FROM experiments ORDER BY created_at").fetchall()
        conn.close()
    except sqlite3.OperationalError:
        return []

    results = []
    for row in rows:
        results.append({
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
        })
    return results


def _read_legacy_patterns(db_path: Path) -> list[dict[str, Any]]:
    """Read all patterns from legacy SQLite DB."""
    try:
        conn = sqlite3.connect(str(db_path))
        conn.row_factory = sqlite3.Row
        rows = conn.execute("SELECT * FROM patterns ORDER BY created_at").fetchall()
        conn.close()
    except sqlite3.OperationalError:
        return []

    results = []
    for row in rows:
        results.append({
            "id": row["id"],
            "name": row["name"],
            "description": row["description"],
            "domain": row["domain"],
            "confidence": row["confidence"],
            "evidence_count": row["evidence_count"],
            "evidence_ids": json.loads(row["evidence_ids_json"]),
            "config_pattern": json.loads(row["config_pattern_json"]),
            "created_at": row["created_at"],
        })
    return results


def _migrate_experiment(store: Any, exp: dict[str, Any]) -> str | None:
    """Migrate a single experiment to a Markdown document."""
    # Check if already migrated (same ID exists)
    existing = store.get(exp["id"])
    if existing is not None:
        return None  # Skip already-migrated

    # Build body
    parts = [f"# {exp.get('title', 'Experiment')}"]

    if exp.get("hypothesis"):
        parts.append(f"\n## Hypothesis\n{exp['hypothesis']}")

    config = exp.get("config", {})
    if config:
        config_lines = [f"- {k}: {v}" for k, v in config.items()]
        parts.append("\n## Config\n" + "\n".join(config_lines))

    result = exp.get("result", {})
    if result:
        metrics = result.get("metrics", {})
        if metrics:
            metric_lines = [f"- {k}: {v}" for k, v in metrics.items()]
            parts.append("\n## Result\n" + "\n".join(metric_lines))
        parts.append(f"\n- success: {result.get('success', True)}")

    observations = exp.get("observations", [])
    if observations:
        obs_lines = []
        for o in observations:
            if isinstance(o, dict):
                obs_lines.append(f"- {o.get('timestamp', '')} [{o.get('source', '')}]: {o.get('content', '')}")
            else:
                obs_lines.append(f"- {o}")
        parts.append("\n## Observations\n" + "\n".join(obs_lines))

    lessons = exp.get("lessons_learned", [])
    if lessons:
        lesson_lines = [f"- {lesson}" for lesson in lessons]
        parts.append("\n## Lessons Learned\n" + "\n".join(lesson_lines))

    body = "\n".join(parts)

    metadata = {
        "title": exp.get("title", ""),
        "task_id": exp.get("task_id", ""),
        "domain": exp.get("domain", ""),
        "tags": exp.get("tags", []),
        "hypothesis": exp.get("hypothesis", ""),
        "hypothesis_status": exp.get("hypothesis_status", ""),
    }

    metadata["id"] = exp["id"]
    return store.create("experiment", metadata, body=body)


def _migrate_pattern(store: Any, pat: dict[str, Any]) -> str | None:
    """Migrate a single pattern to a Markdown document."""
    existing = store.get(pat["id"])
    if existing is not None:
        return None

    parts = [f"# {pat.get('name', 'Pattern')}"]

    if pat.get("description"):
        parts.append(f"\n{pat['description']}")

    config_pattern = pat.get("config_pattern", {})
    if config_pattern:
        lines = [f"- {k}: {v}" for k, v in config_pattern.items()]
        parts.append("\n## Config Pattern\n" + "\n".join(lines))

    evidence_ids = pat.get("evidence_ids", [])
    if evidence_ids:
        links = [f"- [[{eid}]]" for eid in evidence_ids]
        parts.append("\n## Evidence\n" + "\n".join(links))

    body = "\n".join(parts)

    metadata = {
        "title": pat.get("name", ""),
        "domain": pat.get("domain", ""),
        "confidence": pat.get("confidence", 0.0),
        "evidence_count": pat.get("evidence_count", 0),
        "evidence_ids": evidence_ids,
    }

    metadata["id"] = pat["id"]
    return store.create("pattern", metadata, body=body)
