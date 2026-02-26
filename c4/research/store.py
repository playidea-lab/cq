"""ResearchStore - SQLite-backed CRUD for research project tracking.

Stores projects and iterations in a local SQLite database.
Follows the same base_path pattern as DocumentStore (.c4/research/research.db).
"""

from __future__ import annotations

import json
import logging
import sqlite3
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from .models import Iteration, IterationStatus, ProjectStatus, ResearchProject

logger = logging.getLogger(__name__)

_SCHEMA = """
CREATE TABLE IF NOT EXISTS research_projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    paper_path TEXT,
    repo_path TEXT,
    target_score REAL DEFAULT 7.0,
    current_iteration INTEGER DEFAULT 0,
    status TEXT DEFAULT 'active',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS research_iterations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES research_projects(id),
    iteration_num INTEGER NOT NULL,
    review_score REAL,
    axis_scores_json TEXT,
    gaps_json TEXT,
    experiments_json TEXT,
    status TEXT DEFAULT 'reviewing',
    started_at TEXT DEFAULT (datetime('now')),
    completed_at TEXT,
    UNIQUE(project_id, iteration_num)
);
"""


class ResearchStore:
    """SQLite-backed store for research project tracking."""

    def __init__(self, base_path: Path):
        self._base_path = Path(base_path)
        self._base_path.mkdir(parents=True, exist_ok=True)
        self._db_path = self._base_path / "research.db"
        self._conn: sqlite3.Connection | None = None
        self._init_db()

    def __enter__(self) -> "ResearchStore":
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
        conn.executescript(_SCHEMA)
        conn.commit()

    def _get_conn(self) -> sqlite3.Connection:
        if self._conn is None:
            self._conn = sqlite3.connect(str(self._db_path))
            self._conn.row_factory = sqlite3.Row
            self._conn.execute("PRAGMA journal_mode=WAL")
            self._conn.execute("PRAGMA foreign_keys=ON")
            self._conn.execute("PRAGMA busy_timeout=5000")
        return self._conn

    # ------------------------------------------------------------------
    # Project CRUD
    # ------------------------------------------------------------------

    def create_project(
        self,
        name: str,
        paper_path: str | None = None,
        repo_path: str | None = None,
        target_score: float = 7.0,
    ) -> str:
        project_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()
        with self._get_conn() as conn:
            conn.execute(
                """INSERT INTO research_projects
                   (id, name, paper_path, repo_path, target_score, current_iteration, status, created_at, updated_at)
                   VALUES (?, ?, ?, ?, ?, 0, 'active', ?, ?)""",
                (project_id, name, paper_path, repo_path, target_score, now, now),
            )
        return project_id

    def get_project(self, project_id: str) -> ResearchProject | None:
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT * FROM research_projects WHERE id = ?", (project_id,)
            ).fetchone()
        if row is None:
            return None
        return self._row_to_project(row)

    def list_projects(self, status: str | None = None) -> list[ResearchProject]:
        with self._get_conn() as conn:
            if status:
                rows = conn.execute(
                    "SELECT * FROM research_projects WHERE status = ? ORDER BY created_at DESC",
                    (status,),
                ).fetchall()
            else:
                rows = conn.execute(
                    "SELECT * FROM research_projects ORDER BY created_at DESC"
                ).fetchall()
        return [self._row_to_project(r) for r in rows]

    def update_project(self, project_id: str, **kwargs: Any) -> None:
        allowed = {"name", "paper_path", "repo_path", "target_score", "current_iteration", "status"}
        updates = {k: v for k, v in kwargs.items() if k in allowed}
        if not updates:
            return
        updates["updated_at"] = datetime.now(timezone.utc).isoformat()
        set_clause = ", ".join(f"{k} = ?" for k in updates)
        values = list(updates.values()) + [project_id]
        with self._get_conn() as conn:
            conn.execute(
                f"UPDATE research_projects SET {set_clause} WHERE id = ?",
                values,
            )

    # ------------------------------------------------------------------
    # Iteration CRUD
    # ------------------------------------------------------------------

    def create_iteration(self, project_id: str) -> str:
        iteration_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()
        with self._get_conn() as conn:
            # Get next iteration number
            row = conn.execute(
                "SELECT COALESCE(MAX(iteration_num), 0) + 1 AS next_num "
                "FROM research_iterations WHERE project_id = ?",
                (project_id,),
            ).fetchone()
            next_num = row["next_num"]

            conn.execute(
                """INSERT INTO research_iterations
                   (id, project_id, iteration_num, status, started_at)
                   VALUES (?, ?, ?, 'reviewing', ?)""",
                (iteration_id, project_id, next_num, now),
            )
            conn.execute(
                "UPDATE research_projects SET current_iteration = ?, updated_at = ? WHERE id = ?",
                (next_num, now, project_id),
            )
        return iteration_id

    def get_iteration(self, iteration_id: str) -> Iteration | None:
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT * FROM research_iterations WHERE id = ?", (iteration_id,)
            ).fetchone()
        if row is None:
            return None
        return self._row_to_iteration(row)

    def get_current_iteration(self, project_id: str) -> Iteration | None:
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT * FROM research_iterations WHERE project_id = ? "
                "ORDER BY iteration_num DESC LIMIT 1",
                (project_id,),
            ).fetchone()
        if row is None:
            return None
        return self._row_to_iteration(row)

    def list_iterations(self, project_id: str) -> list[Iteration]:
        with self._get_conn() as conn:
            rows = conn.execute(
                "SELECT * FROM research_iterations WHERE project_id = ? ORDER BY iteration_num",
                (project_id,),
            ).fetchall()
        return [self._row_to_iteration(r) for r in rows]

    def update_iteration(self, iteration_id: str, **kwargs: Any) -> None:
        allowed = {"review_score", "axis_scores", "gaps", "experiments", "status", "completed_at"}
        updates: dict[str, Any] = {}
        for k, v in kwargs.items():
            if k not in allowed:
                continue
            if k in ("axis_scores", "gaps", "experiments"):
                updates[f"{k}_json"] = json.dumps(v) if v is not None else None
            else:
                updates[k] = v
        if not updates:
            return
        set_clause = ", ".join(f"{k} = ?" for k in updates)
        values = list(updates.values()) + [iteration_id]
        with self._get_conn() as conn:
            conn.execute(
                f"UPDATE research_iterations SET {set_clause} WHERE id = ?",
                values,
            )

    # ------------------------------------------------------------------
    # suggest_next
    # ------------------------------------------------------------------

    def suggest_next(self, project_id: str) -> dict[str, Any]:
        project = self.get_project(project_id)
        if project is None:
            return {"action": "none", "reason": "Project not found"}

        if project.status != ProjectStatus.ACTIVE:
            return {"action": "none", "reason": f"Project is {project.status.value}"}

        current = self.get_current_iteration(project_id)

        if current is None:
            return {"action": "review", "reason": "No iterations yet. Start with a review.", "iteration": 0}

        if current.status == IterationStatus.REVIEWING:
            return {"action": "review", "reason": "Review in progress. Record results when done.", "iteration": current.iteration_num}

        if current.review_score is not None and current.review_score >= project.target_score:
            no_exp_gaps = not current.gaps or all(
                g.get("type") != "experiment" for g in current.gaps
            )
            if no_exp_gaps:
                score = current.review_score
                target = project.target_score
                return {
                    "action": "complete",
                    "reason": f"Score {score} >= target {target}",
                    "iteration": current.iteration_num,
                }

        if current.status == IterationStatus.DONE:
            return {
                "action": "review",
                "reason": "Previous iteration complete. Review updated paper.",
                "iteration": current.iteration_num + 1,
            }

        if current.gaps and any(g.get("type") == "experiment" for g in current.gaps):
            pending = [
                g for g in current.gaps
                if g.get("type") == "experiment" and g.get("status") != "completed"
            ]
            if pending:
                return {
                    "action": "run_experiments",
                    "reason": (
                        f"{len(pending)} experiments remaining. "
                        "Suggestion: Use Gemini 3.0 (c4-research-scientist) for real-time benchmark grounding."
                    ),
                    "iteration": current.iteration_num
                }

        return {
            "action": "plan_experiments",
            "reason": (
                "Review done. Plan experiments for identified gaps. "
                "Suggestion: Consult c4-global-brain to verify paper-to-code alignment."
            ),
            "iteration": current.iteration_num,
        }

    # ------------------------------------------------------------------
    # Row -> Model helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _row_to_project(row: sqlite3.Row) -> ResearchProject:
        return ResearchProject(
            id=row["id"],
            name=row["name"],
            paper_path=row["paper_path"],
            repo_path=row["repo_path"],
            target_score=row["target_score"],
            current_iteration=row["current_iteration"],
            status=ProjectStatus(row["status"]),
            created_at=_parse_dt(row["created_at"]),
            updated_at=_parse_dt(row["updated_at"]),
        )

    @staticmethod
    def _row_to_iteration(row: sqlite3.Row) -> Iteration:
        return Iteration(
            id=row["id"],
            project_id=row["project_id"],
            iteration_num=row["iteration_num"],
            review_score=row["review_score"],
            axis_scores=_parse_json(row["axis_scores_json"]),
            gaps=_parse_json(row["gaps_json"]),
            experiments=_parse_json(row["experiments_json"]),
            status=IterationStatus(row["status"]),
            started_at=_parse_dt(row["started_at"]),
            completed_at=_parse_dt(row["completed_at"]),
        )


def _parse_json(val: str | None) -> Any:
    if val is None:
        return None
    try:
        return json.loads(val)
    except (json.JSONDecodeError, TypeError):
        return None


def _parse_dt(val: str | None) -> datetime | None:
    if val is None:
        return None
    try:
        return datetime.fromisoformat(val)
    except (ValueError, TypeError):
        return None
