"""SQLite Store - SQLite-based state, lock, and task storage"""

import json
import logging
import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING, Any, Generator

from .exceptions import StateNotFoundError, TransactionError
from .protocol import LockStore, StateStore

logger = logging.getLogger(__name__)

if TYPE_CHECKING:
    from c4.models import C4State, TaskQueue
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
        """Load state from SQLite.

        Args:
            project_id: The project ID to load. Must be non-empty.

        Raises:
            ValueError: If project_id is empty or whitespace-only.
            StateNotFoundError: If no state exists for the project.
        """
        from c4.models import C4State

        # Validate project_id to prevent loading wrong state
        if not project_id or not project_id.strip():
            raise ValueError(
                f"project_id must be non-empty, got: '{project_id}'. "
                "Check config.yaml has a valid project_id."
            )

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT state_json FROM c4_state WHERE project_id = ?",
                (project_id,),
            )
            row = cursor.fetchone()

        if row is None:
            raise StateNotFoundError(f"State not found for project: {project_id or '(any)'}")

        import json

        data = json.loads(row[0])
        return C4State.model_validate(data)

    def save(self, state: "C4State") -> None:
        """Save state to SQLite.

        Args:
            state: The C4State to save.

        Raises:
            ValueError: If state.project_id is empty or whitespace-only.
        """
        # Validate project_id to prevent creating invalid state rows
        if not state.project_id or not state.project_id.strip():
            raise ValueError(
                f"Cannot save state with empty project_id: '{state.project_id}'. "
                "Set project_id in config.yaml or during initialization."
            )

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
    def atomic_modify(self, project_id: str) -> Generator["C4State", None, None]:
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
        import json

        from c4.models import C4State

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
            # Start IMMEDIATE transaction - acquires write lock immediately
            # but allows concurrent readers (unlike EXCLUSIVE which blocks all)
            # This reduces lock contention while maintaining atomicity
            conn.execute("BEGIN IMMEDIATE")

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
        except sqlite3.Error as e:
            try:
                conn.execute("ROLLBACK")
            except sqlite3.Error as rollback_err:
                logger.error(
                    "Failed to rollback transaction for project %s: %s",
                    self.project_id,
                    rollback_err,
                )
                raise TransactionError(
                    f"Transaction failed and rollback also failed: {rollback_err}"
                ) from e
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
        """Acquire scope lock with TTL using atomic operations.

        Uses a single atomic transaction to prevent race conditions:
        1. Delete expired locks
        2. Try INSERT (fails if lock exists)
        3. If INSERT fails, check if we own the lock and refresh it
        """
        now = datetime.now()
        expires_at = now + timedelta(seconds=ttl_seconds)

        with self._get_connection() as conn:
            # Use IMMEDIATE transaction for write lock from the start
            conn.execute("BEGIN IMMEDIATE")
            try:
                # Step 1: Delete expired locks atomically
                conn.execute(
                    "DELETE FROM c4_locks WHERE project_id = ? AND scope = ? AND expires_at <= ?",
                    (project_id, scope, now),
                )

                # Step 2: Try to INSERT (will fail if lock exists due to UNIQUE constraint)
                try:
                    conn.execute(
                        "INSERT INTO c4_locks (project_id, scope, owner, expires_at) VALUES (?, ?, ?, ?)",
                        (project_id, scope, owner, expires_at),
                    )
                    conn.commit()
                    return True
                except sqlite3.IntegrityError:
                    # Lock exists - check if we own it
                    cursor = conn.execute(
                        "SELECT owner FROM c4_locks WHERE project_id = ? AND scope = ?",
                        (project_id, scope),
                    )
                    row = cursor.fetchone()

                    if row and row[0] == owner:
                        # We own the lock - refresh TTL
                        conn.execute(
                            "UPDATE c4_locks SET expires_at = ? WHERE project_id = ? AND scope = ?",
                            (expires_at, project_id, scope),
                        )
                        conn.commit()
                        return True
                    else:
                        # Different owner holds the lock
                        conn.rollback()
                        return False
            except sqlite3.Error:
                conn.rollback()
                raise

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

    IMPORTANT: Task status is DERIVED from c4_state.queue, not stored directly.
    This ensures single source of truth and prevents inconsistency between
    c4_state and c4_tasks tables.

    Schema:
        c4_tasks (
            project_id TEXT,
            task_id TEXT,
            task_json TEXT NOT NULL,
            status TEXT NOT NULL,  -- Cached only, derived from c4_state.queue on read
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
            # Task history for peer review support
            # Records which workers have previously attempted each task
            conn.execute("""
                CREATE TABLE IF NOT EXISTS c4_task_history (
                    project_id TEXT,
                    task_id TEXT,
                    worker_id TEXT,
                    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (project_id, task_id, worker_id)
                )
            """)
            # Index for faster queries by task
            conn.execute("""
                CREATE INDEX IF NOT EXISTS idx_c4_task_history_task
                ON c4_task_history (project_id, task_id)
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

    def _load_queue(self, project_id: str) -> "TaskQueue | None":
        """Load queue from c4_state table (single source of truth for task status)"""
        from c4.models import TaskQueue

        try:
            with self._get_connection() as conn:
                cursor = conn.execute(
                    "SELECT state_json FROM c4_state WHERE project_id = ?",
                    (project_id,),
                )
                row = cursor.fetchone()

            if row is None:
                # Also try loading without project_id (single-project case)
                with self._get_connection() as conn:
                    cursor = conn.execute("SELECT state_json FROM c4_state LIMIT 1")
                    row = cursor.fetchone()

            if row is None:
                return None

            state_data = json.loads(row[0])
            queue_data = state_data.get("queue", {})
            return TaskQueue.model_validate(queue_data)
        except sqlite3.OperationalError:
            # c4_state table doesn't exist yet (e.g., in isolated tests)
            return None

    def _derive_status(self, task_id: str, queue: "TaskQueue") -> str:
        """
        Derive task status from c4_state.queue (single source of truth).

        This prevents inconsistency between c4_state and c4_tasks tables
        by always computing status from the authoritative queue state.
        """
        if task_id in queue.done:
            return "done"
        elif task_id in queue.in_progress:
            return "in_progress"
        elif task_id in queue.pending:
            return "pending"
        else:
            return "unknown"

    def load_all(self, project_id: str) -> dict[str, "Task"]:
        """Load all tasks for a project with status derived from c4_state.queue"""
        from c4.models.enums import TaskStatus
        from c4.models.task import Task

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT task_id, task_json FROM c4_tasks WHERE project_id = ?",
                (project_id,),
            )
            rows = cursor.fetchall()

        # Load queue to derive status
        queue = self._load_queue(project_id)

        tasks = {}
        for row in rows:
            task_id, task_json_str = row
            task = Task.model_validate(json.loads(task_json_str))

            # Derive status from queue (single source of truth)
            if queue is not None:
                derived_status = self._derive_status(task_id, queue)
                if derived_status != "unknown":
                    task.status = TaskStatus(derived_status)

            tasks[task_id] = task

        return tasks

    def get(self, project_id: str, task_id: str) -> "Task | None":
        """Get a single task by ID with status derived from c4_state.queue"""
        from c4.models.enums import TaskStatus
        from c4.models.task import Task

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            row = cursor.fetchone()

        if row is None:
            return None

        task = Task.model_validate(json.loads(row[0]))

        # Derive status from queue (single source of truth)
        queue = self._load_queue(project_id)
        if queue is not None:
            derived_status = self._derive_status(task_id, queue)
            if derived_status != "unknown":
                task.status = TaskStatus(derived_status)

        return task

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

    def get_queue_stats(self, project_id: str) -> dict[str, Any]:
        """Get queue statistics directly from c4_tasks table.

        This provides accurate counts that don't rely on c4_state.queue,
        which can get out of sync. Use this for c4_status() to ensure
        consistent reporting.

        Returns:
            Dict with pending_count, in_progress_count, done_count,
            pending_ids (first 5), and in_progress_map.
        """
        with self._get_connection() as conn:
            # Get counts by status
            cursor = conn.execute(
                """
                SELECT status, COUNT(*) as count
                FROM c4_tasks
                WHERE project_id = ?
                GROUP BY status
                """,
                (project_id,),
            )
            counts = {row[0]: row[1] for row in cursor.fetchall()}

            # Get pending task IDs (first 5)
            cursor = conn.execute(
                """
                SELECT task_id FROM c4_tasks
                WHERE project_id = ? AND status = 'pending'
                ORDER BY task_id
                LIMIT 5
                """,
                (project_id,),
            )
            pending_ids = [row[0] for row in cursor.fetchall()]

            # Get in_progress map (task_id -> worker_id)
            cursor = conn.execute(
                """
                SELECT task_id, assigned_to FROM c4_tasks
                WHERE project_id = ? AND status = 'in_progress'
                """,
                (project_id,),
            )
            in_progress_map = {
                row[0]: row[1] or "unknown" for row in cursor.fetchall()
            }

        return {
            "pending_count": counts.get("pending", 0),
            "in_progress_count": counts.get("in_progress", 0),
            "done_count": counts.get("done", 0),
            "pending_ids": pending_ids,
            "in_progress_map": in_progress_map,
        }

    def sync_state_queue(self, project_id: str) -> bool:
        """Sync c4_state.queue with actual task statuses from c4_tasks.

        This fixes inconsistencies where c4_state.queue and c4_tasks.status
        have diverged. Call this when state appears stale.

        Returns:
            True if sync was performed, False if nothing to sync.
        """
        with self._get_connection() as conn:
            # Get all task IDs by status
            cursor = conn.execute(
                """
                SELECT task_id, status FROM c4_tasks
                WHERE project_id = ?
                """,
                (project_id,),
            )
            tasks = cursor.fetchall()

            if not tasks:
                return False

            pending = []
            in_progress = {}
            done = []

            for task_id, status in tasks:
                if status == "pending":
                    pending.append(task_id)
                elif status == "in_progress":
                    # Get assigned_to for in_progress tasks
                    cursor2 = conn.execute(
                        "SELECT assigned_to FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                        (project_id, task_id),
                    )
                    row = cursor2.fetchone()
                    worker_id = row[0] if row and row[0] else "unknown"
                    in_progress[task_id] = worker_id
                elif status == "done":
                    done.append(task_id)

            # Build new queue JSON
            queue_json = json.dumps({
                "pending": pending,
                "in_progress": in_progress,
                "done": done,
            })

            # Update state
            conn.execute(
                """
                UPDATE c4_state
                SET state_json = json_set(state_json, '$.queue', json(?))
                WHERE project_id = ?
                """,
                (queue_json, project_id),
            )
            conn.commit()

        return True

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
        from c4.models.enums import TaskStatus
        from c4.models.task import Task

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

    def update_commit_info(
        self,
        project_id: str,
        task_id: str,
        commit_sha: str,
        branch: str | None = None,
    ) -> bool:
        """
        Update commit info for a task WITHOUT changing status.

        Status is derived from c4_state.queue (single source of truth),
        so we only update commit_sha and branch here.

        This is the recommended method for c4_submit() to use.
        """
        from c4.models.task import Task

        with self._get_connection() as conn:
            # Load current task
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            row = cursor.fetchone()
            if row is None:
                return False

            # Update task (commit_sha and branch only, NOT status)
            task = Task.model_validate(json.loads(row[0]))
            task.commit_sha = commit_sha
            if branch is not None:
                task.branch = branch

            # Save updated task - status column value is just a cache
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
                    task.status.value,  # Cached, real status comes from queue
                    task.assigned_to,
                    datetime.now(),
                ),
            )
            conn.commit()
            return True

    def update_review_decision(
        self,
        project_id: str,
        task_id: str,
        review_decision: str,
    ) -> bool:
        """
        Update review_decision field for a review/checkpoint task.

        Args:
            project_id: Project ID
            task_id: Task ID (R-XXX-N or CP-XXX)
            review_decision: APPROVE, REQUEST_CHANGES, or REPLAN

        Returns:
            True if updated successfully, False if task not found
        """
        from c4.models.task import Task

        with self._get_connection() as conn:
            # Load current task
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ? AND task_id = ?",
                (project_id, task_id),
            )
            row = cursor.fetchone()
            if row is None:
                return False

            # Update review_decision
            task = Task.model_validate(json.loads(row[0]))
            task.review_decision = review_decision

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

    # =========================================================================
    # Task History (for peer review support)
    # =========================================================================

    def record_assignment(
        self, project_id: str, task_id: str, worker_id: str
    ) -> None:
        """Record that a worker was assigned to a task.

        Used for peer review: repair tasks should go to different workers.

        Args:
            project_id: Project identifier
            task_id: Task identifier
            worker_id: Worker who was assigned
        """
        with self._get_connection() as conn:
            conn.execute(
                """
                INSERT OR IGNORE INTO c4_task_history
                    (project_id, task_id, worker_id, assigned_at)
                VALUES (?, ?, ?, ?)
                """,
                (project_id, task_id, worker_id, datetime.now()),
            )
            conn.commit()

    def get_task_history(self, project_id: str, task_id: str) -> list[str]:
        """Get list of workers who have previously been assigned a task.

        Args:
            project_id: Project identifier
            task_id: Task identifier

        Returns:
            List of worker IDs in order of assignment
        """
        with self._get_connection() as conn:
            cursor = conn.execute(
                """
                SELECT worker_id FROM c4_task_history
                WHERE project_id = ? AND task_id = ?
                ORDER BY assigned_at ASC
                """,
                (project_id, task_id),
            )
            return [row[0] for row in cursor.fetchall()]

    def clear_task_history(self, project_id: str, task_id: str | None = None) -> int:
        """Clear task history for a project or specific task.

        Args:
            project_id: Project identifier
            task_id: Optional task ID (if None, clears all history for project)

        Returns:
            Number of records deleted
        """
        with self._get_connection() as conn:
            if task_id:
                cursor = conn.execute(
                    "DELETE FROM c4_task_history WHERE project_id = ? AND task_id = ?",
                    (project_id, task_id),
                )
            else:
                cursor = conn.execute(
                    "DELETE FROM c4_task_history WHERE project_id = ?",
                    (project_id,),
                )
            conn.commit()
            return cursor.rowcount
