"""Knowledge Reindex — re-embed all knowledge documents with a given provider.

Usage (CLI):
    uv run python -m c4.knowledge.reindex --provider ollama

Usage (as library):
    from c4.knowledge.reindex import reindex_all
    reindex_all(provider="ollama")
"""

from __future__ import annotations

import argparse
import logging
from pathlib import Path
from typing import Any

from c4.knowledge.embeddings import KnowledgeEmbedder

logger = logging.getLogger(__name__)


def reindex_all(
    provider: str = "ollama",
    base_path: str | Path = ".c4/knowledge",
    store_path: str | Path | None = None,
    *,
    _embedder: Any = None,
) -> dict[str, int]:
    """Re-embed all knowledge documents using the specified provider.

    Args:
        provider: Embedding provider name ("ollama", "openai", "local", "mock").
        base_path: Base path for the knowledge store (contains knowledge.db).
        store_path: Explicit path to the knowledge SQLite DB. If None, derived from base_path.
        _embedder: Injectable embedder (for testing). If provided, overrides provider.

    Returns:
        Dict with keys "success", "skipped", "total".
    """
    import sqlite3

    base_path = Path(base_path)
    db_path = Path(store_path) if store_path else base_path / "knowledge.db"

    if not db_path.exists():
        logger.warning("Knowledge DB not found at %s — nothing to reindex", db_path)
        return {"success": 0, "skipped": 0, "total": 0}

    # Build embedder
    if _embedder is None:
        embedder = KnowledgeEmbedder(base_path=base_path, embedding_model=provider)
    else:
        embedder = _embedder

    conn = sqlite3.connect(str(db_path))
    conn.row_factory = sqlite3.Row
    try:
        rows = conn.execute(
            "SELECT id, title, body, tags, doc_type FROM knowledge_docs"
        ).fetchall()
    except sqlite3.OperationalError:
        # Table may be named differently in older schemas
        try:
            rows = conn.execute(
                "SELECT id, title, body, tags, doc_type FROM documents"
            ).fetchall()
        except sqlite3.OperationalError:
            logger.warning("No knowledge table found in %s", db_path)
            conn.close()
            return {"success": 0, "skipped": 0, "total": 0}
    finally:
        conn.close()

    success = 0
    skipped = 0
    total = len(rows)

    for row in rows:
        doc_id = row["id"]
        doc = {
            "title": row["title"] or "",
            "body": row["body"] or "",
            "tags": (row["tags"] or "").split(",") if row["tags"] else [],
            "doc_type": row["doc_type"] or "",
        }
        try:
            ok = embedder.index_document(doc_id, doc)
            if ok:
                success += 1
            else:
                skipped += 1
        except Exception:
            logger.exception("Failed to reindex document %s", doc_id)
            skipped += 1

    logger.info("Reindex complete: %d/%d success, %d skipped", success, total, skipped)
    return {"success": success, "skipped": skipped, "total": total}


def main() -> None:
    parser = argparse.ArgumentParser(description="Reindex knowledge documents")
    parser.add_argument(
        "--provider",
        default="ollama",
        help="Embedding provider: ollama, openai, local, mock (default: ollama)",
    )
    parser.add_argument(
        "--base-path",
        default=".c4/knowledge",
        help="Base path for the knowledge store (default: .c4/knowledge)",
    )
    args = parser.parse_args()

    logging.basicConfig(level=logging.INFO)
    result = reindex_all(provider=args.provider, base_path=args.base_path)
    print(f"Reindex done: {result}")


if __name__ == "__main__":
    main()
