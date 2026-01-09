"""SQLite Store - SQLite-based state, lock, and task storage"""

import json
import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING, Generator

from .exceptions import StateNotFoundError
from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.models import C4State
    from c4.models.task import Task


# Python 3.12+ requires explicit adapters/converters for datetime
def _adapt_datetime(dt: datetime) -> str:
    """Convert datetime to ISO format string for SQLite storage"""
    return dt.isoformat()


def _convert_datetime(val: bytes) -> datetime:
    """Convert ISO format string from SQLite to datetime"""
    return datetime.fromisoformat(val.decode())


# Register adapters and converters
sqlite3.register_adapter(datetime, _adapt_datetime)
sqlite3.register_converter("TIMESTAMP", _convert_datetime)


class SQLiteStateStore(StateStore):
    """
    SQLite-based state storage.

    Schema:
        c4_state (
            project_id TEXT PRIMARY KEY,
            state_json TEXT NOT NULL,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """

    def __init__(self, db_path: Path):
        """
        Initialize SQLite state store.

        Args:
            db_path: Path to SQLite database file
        """
        self.db_path = db_path
        self._init_db()

    def _init_db(self) -> None:
        """Create tables if they don't exist"""
        with self._get_connection() as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_state (
                    project_id TEXT PRIMARY KEY,
                    state_json TEXT NOT NULL,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                )
            """)
            conn.commit()

    @contextmanager
    def _get_connection(self) -> Generator[sqlite3.Connection, None, None]:
        """Get a database connection with context management"""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
            timeout=30.0,  # Wait up to 30s for lock
        )
        # Enable WAL mode for concurrent reads
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA busy_timeout=30000")  # 30s in ms
        try:
            yield conn
        finally:
            conn.close()

    def load(self, project_id: str) -> "C4State":
        """Load state from SQLite"""
        from c4.models import C4State

        with self._get_connection() as conn:
            if project_id:
                # Load specific project
                cursor = conn.execute(
                    "SELECT state_json FROM c4_state WHERE project_id = ?",
                    (project_id,),
                )
            else:
                # Load any available project (single-project case)
                cursor = conn.execute(
                    "SELECT state_json FROM c4_state LIMIT 1"
                )
            row = cursor.fetchone()

        if row is None:
            raise StateNotFoundError(f"State not found for project: {project_id or '(any)'}")

        import json

        data = json.loads(row[0])
        return C4State.model_validate(data)

    def save(self, state: "C4State") -> None:
        """Save state to SQLite"""
        state.updated_at = datetime.now()
        state_json = state.model_dump_json()

        with self._get_connection() as conn:
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at)
                VALUES (?, ?, ?)
            """,
                (state.project_id, state_json, state.updated_at),
            )
            conn.commit()

    def exists(self, project_id: str) -> bool:
        """Check if state exists in SQLite"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT 1 FROM c4_state WHERE project_id = ?",
                (project_id,),
            )
            return cursor.fetchone() is not None

    def delete(self, project_id: str) -> None:
        """Delete state from SQLite"""
        with self._get_connection() as conn:
            conn.execute(
                "DELETE FROM c4_state WHERE project_id = ?",
                (project_id,),
            )
            conn.commit()

    @contextmanager
    def atomic_modify(
        self, project_id: str
    ) -> Generator["C4State", None, None]:
        """
        Atomically load, modify, and save state.

        Uses EXCLUSIVE transaction to prevent race conditions when
        multiple workers modify state concurrently.

        Usage:
            with store.atomic_modify(project_id) as state:
                state.queue.done.append(task_id)
                del state.queue.in_progress[task_id]
            # State is automatically saved on context exit
        """
        from c4.models import C4State
        import json

        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
            timeout=30.0,
            isolation_level=None,  # Manual transaction control
        )
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA busy_timeout=30000")

        try:
            # Start EXCLUSIVE transaction - blocks all other connections
            conn.execute("BEGIN EXCLUSIVE")

            # Load current state
            cursor = conn.execute(
                "SELECT state_json FROM c4_state WHERE project_id = ?",
                (project_id,),
            )
            row = cursor.fetchone()
            if row is None:
                conn.execute("ROLLBACK")
                raise StateNotFoundError(f"State not found: {project_id}")

            state = C4State.model_validate(json.loads(row[0]))

            # Yield for modification
            yield state

            # Save modified state
            state.updated_at = datetime.now()
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at)
                VALUES (?, ?, ?)
                """,
                (state.project_id, state.model_dump_json(), state.updated_at),
            )
            conn.execute("COMMIT")
        except Exception:
            try:
                conn.execute("ROLLBACK")
            except Exception:
                pass
            raise
        finally:
            conn.close()


class SQLiteLockStore(LockStore):
    """
    SQLite-based lock storage.

    Schema:
        c4_locks (
            project_id TEXT,
            scope TEXT,
            owner TEXT NOT NULL,
            expires_at TIMESTAMP NOT NULL,
            PRIMARY KEY (project_id, scope)
        )

        c4_leader_lock (
            project_id TEXT PRIMARY KEY,
            owner TEXT NOT NULL,
            pid INTEGER NOT NULL,
            started_at TIMESTAMP NOT NULL
        )
    """

    def __init__(self, db_path: Path):
        """
        Initialize SQLite lock store.

        Args:
            db_path: Path to SQLite database file
        """
        self.db_path = db_path
        self._init_db()

    def _init_db(self) -> None:
        """Create tables if they don't exist"""
        with self._get_connection() as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_locks (
                    project_id TEXT,
                    scope TEXT,
                    owner TEXT NOT NULL,
                    expires_at TIMESTAMP NOT NULL,
                    PRIMARY KEY (project_id, scope)
                )
            """)
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_leader_lock (
                    project_id TEXT PRIMARY KEY,
                    owner TEXT NOT NULL,
                    pid INTEGER NOT NULL,
                    started_at TIMESTAMP NOT NULL
                )
            """)
            conn.commit()

    @contextmanager
    def _get_connection(self) -> Generator[sqlite3.Connection, None, None]:
        """Get a database connection with context management"""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
            timeout=30.0,  # Wait up to 30s for lock
        )
        # Enable WAL mode for concurrent reads
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA busy_timeout=30000")  # 30s in ms
        try:
            yield conn
        finally:
            conn.close()

    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Acquire scope lock with TTL"""
        now = datetime.now()
        expires_at = now + timedelta(seconds=ttl_seconds)

        with self._get_connection() as conn:
            # Check existing lock
            cursor = conn.execute(
                "SELECT owner, expires_at FROM c4_locks WHERE project_id = ? AND scope = ?",
                (project_id, scope),
            )
            row = cursor.fetchone()

            if row:
                existing_owner, existing_expires = row
                # Parse timestamp if needed
                if isinstance(existing_expires, str):
                    existing_expires = datetime.fromisoformat(existing_expires)

                if existing_expires > now:
                    if existing_owner == owner:
                        # Same owner - refresh TTL
                        conn.execute(
                            "UPDATE c4_locks SET expires_at = ? WHERE project_id = ? AND scope = ?",
                            (expires_at, project_id, scope),
                        )
                        conn.commit()
                        return True
                    else:
                        # Different owner - conflict
                        return False
                # Expired - delete and continue

            # Acquire or update lock
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_locks (project_id, scope, owner, expires_at)
                VALUES (?, ?, ?, ?)
            """,
                (project_id, scope, owner, expires_at),
            )
            conn.commit()
            return True

    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """Release scope lock"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "DELETE FROM c4_locks WHERE project_id = ? AND scope = ?",
                (project_id, scope),
            )
            conn.commit()
            return cursor.rowcount > 0

    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """Refresh lock TTL if owner matches"""
        expires_at = datetime.now() + timedelta(seconds=ttl_seconds)

        with self._get_connection() as conn:
            cursor = conn.execute(
                """
                UPDATE c4_locks
                SET expires_at = ?
                WHERE project_id = ? AND scope = ? AND owner = ?
            """,
                (expires_at, project_id, scope, owner),
            )
            conn.commit()
            return cursor.rowcount > 0

    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """Get current lock holder and expiry"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT owner, expires_at FROM c4_locks WHERE project_id = ? AND scope = ?",
                (project_id, scope),
            )
            row = cursor.fetchone()

        if row is None:
            return None

        owner, expires_at = row
        # Parse timestamp if needed
        if isinstance(expires_at, str):
            expires_at = datetime.fromisoformat(expires_at)
        return (owner, expires_at)

    def get_lock_owner(
        self,
        project_id: str,
        scope: str,
    ) -> str | None:
        """Get owner of a valid (non-expired) lock, or None if no valid lock exists"""
        now = datetime.now()
        with self._get_connection() as conn:
            cursor = conn.execute(
                """
                SELECT owner FROM c4_locks
                WHERE project_id = ? AND scope = ? AND expires_at > ?
                """,
                (project_id, scope, now),
            )
            row = cursor.fetchone()

        return row[0] if row else None

    def cleanup_expired(self, project_id: str) -> list[str]:
        """Remove expired locks"""
        now = datetime.now()

        with self._get_connection() as conn:
            # Get expired scopes first
            cursor = conn.execute(
                "SELECT scope FROM c4_locks WHERE project_id = ? AND expires_at < ?",
                (project_id, now),
            )
            expired = [row[0] for row in cursor.fetchall()]

            # Delete expired
            if expired:
                conn.execute(
                    "DELETE FROM c4_locks WHERE project_id = ? AND expires_at < ?",
                    (project_id, now),
                )
                conn.commit()

        return expired

    def acquire_leader_lock(
        self,
        project_id: str,
        owner: str,
        pid: int,
    ) -> bool:
        """Acquire leader lock"""
        with self._get_connection() as conn:
            # Check existing leader
            cursor = conn.execute(
                "SELECT owner FROM c4_leader_lock WHERE project_id = ?",
                (project_id,),
            )
            row = cursor.fetchone()

            if row:
                existing_owner = row[0]
                if existing_owner == owner:
                    return True
                return False

            # Acquire
            conn.execute(
                """
                INSERT INTO c4_leader_lock (project_id, owner, pid, started_at)
                VALUES (?, ?, ?, ?)
            """,
                (project_id, owner, pid, datetime.now()),
            )
            conn.commit()
            return True

    def release_leader_lock(self, project_id: str) -> bool:
        """Release leader lock"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "DELETE FROM c4_leader_lock WHERE project_id = ?",
                (project_id,),
            )
            conn.commit()
            return cursor.rowcount > 0


class SQLiteTaskStore:
    """
    SQLite-based task storage.

    Stores tasks in a dedicated table for atomic updates alongside state.
    This prevents race conditions when multiple workers update tasks concurrently.

    Schema:
        c4_tasks (
            project_id TEXT,
            task_id TEXT,
            task_json TEXT NOT NULL,
            status TEXT NOT NULL,
            assigned_to TEXT,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (project_id, task_id)
        )
    """

    def __init__(self, db_path: Path):
        """
        Initialize SQLite task store.

        Args:
            db_path: Path to SQLite database file
        """
        self.db_path = db_path
        self._init_db()

    def _init_db(self) -> None:
        """Create tables if they don't exist"""
        with self._get_connection() as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_tasks (
                    project_id TEXT,
                    task_id TEXT,
                    task_json TEXT NOT NULL,
                    status TEXT NOT NULL,
                    assigned_to TEXT,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (project_id, task_id)
                )
            """)
            # Index for faster queries by status
            conn.execute("""
                CREATE INDEX IF NOT EXISTS idx_c4_tasks_status
                ON c4_tasks (project_id, status)
            """)
            conn.commit()

    @contextmanager
    def _get_connection(self) -> Generator[sqlite3.Connection, None, None]:
        """Get a database connection with context management"""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
            timeout=30.0,
        )
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA busy_timeout=30000")
        try:
            yield conn
        finally:
            conn.close()

    def load_all(self, project_id: str) -> dict[str, "Task"]:
        """Load all tasks for a project"""
        from c4.models.task import Task

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT task_id, task_json FROM c4_tasks WHERE project_id = ?",
                (project_id,),
            )
            rows = cursor.fetchall()

        return {row[0]: Task.model_validate(json.loads(row[1])) for row in rows}

    def get(self, project_id: str, task_id: str) -> "Task | None":
        """Get a single task by ID"""
        from c4.models.task import Task

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            row = cursor.fetchone()

        if row is None:
            return None
        return Task.model_validate(json.loads(row[0]))

    def save(self, project_id: str, task: "Task") -> None:
        """Save a single task (insert or update)"""
        with self._get_connection() as conn:
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_tasks
                    (project_id, task_id, task_json, status, assigned_to, updated_at)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (
                    project_id,
                    task.id,
                    task.model_dump_json(),
                    task.status.value,
                    task.assigned_to,
                    datetime.now(),
                ),
            )
            conn.commit()

    def save_all(self, project_id: str, tasks: dict[str, "Task"]) -> None:
        """Save multiple tasks (bulk insert/update)"""
        with self._get_connection() as conn:
            for task in tasks.values():
                conn.execute(
                    """
                    INSERT OR REPLACE INTO c4_tasks
                        (project_id, task_id, task_json, status, assigned_to, updated_at)
                    VALUES (?, ?, ?, ?, ?, ?)
                    """,
                    (
                        project_id,
                        task.id,
                        task.model_dump_json(),
                        task.status.value,
                        task.assigned_to,
                        datetime.now(),
                    ),
                )
            conn.commit()

    def delete(self, project_id: str, task_id: str) -> bool:
        """Delete a single task"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "DELETE FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            conn.commit()
            return cursor.rowcount > 0

    def delete_all(self, project_id: str) -> int:
        """Delete all tasks for a project"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "DELETE FROM c4_tasks WHERE project_id = ?",
                (project_id,),
            )
            conn.commit()
            return cursor.rowcount

    def update_status(
        self,
        project_id: str,
        task_id: str,
        status: str,
        assigned_to: str | None = None,
        branch: str | None = None,
        commit_sha: str | None = None,
    ) -> bool:
        """
        Update task status and related fields atomically.

        This is the primary method for task state changes during execution.
        It loads the task, updates fields, and saves in one operation.
        """
        from c4.models.task import Task
        from c4.models.enums import TaskStatus

        with self._get_connection() as conn:
            # Load current task
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            row = cursor.fetchone()
            if row is None:
                return False

            # Update task
            task = Task.model_validate(json.loads(row[0]))
            task.status = TaskStatus(status)
            if assigned_to is not None:
                task.assigned_to = assigned_to
            if branch is not None:
                task.branch = branch
            if commit_sha is not None:
                task.commit_sha = commit_sha

            # Save updated task
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_tasks
                    (project_id, task_id, task_json, status, assigned_to, updated_at)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (
                    project_id,
                    task_id,
                    task.model_dump_json(),
                    task.status.value,
                    task.assigned_to,
                    datetime.now(),
                ),
            )
            conn.commit()
            return True

    def exists(self, project_id: str) -> bool:
        """Check if any tasks exist for a project"""
        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT 1 FROM c4_tasks WHERE project_id = ? LIMIT 1",
                (project_id,),
            )
            return cursor.fetchone() is not None

    def migrate_from_json(self, project_id: str, tasks_json_path: Path) -> int:
        """
        Migrate tasks from tasks.json file to SQLite.

        Args:
            project_id: Project ID to associate tasks with
            tasks_json_path: Path to the tasks.json file

        Returns:
            Number of tasks migrated
        """
        from c4.models.task import Task

        if not tasks_json_path.exists():
            return 0

        data = json.loads(tasks_json_path.read_text())
        tasks = {t["id"]: Task.model_validate(t) for t in data}

        if not tasks:
            return 0

        self.save_all(project_id, tasks)
        return len(tasks)
