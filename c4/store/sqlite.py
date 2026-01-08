"""SQLite Store - SQLite-based state and lock storage"""

import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING, Generator

from .exceptions import StateNotFoundError
from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.models import C4State


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
        )
        try:
            yield conn
        finally:
            conn.close()

    def load(self, project_id: str) -> "C4State":
        """Load state from SQLite"""
        from c4.models import C4State

        with self._get_connection() as conn:
            cursor = conn.execute(
                "SELECT state_json FROM c4_state WHERE project_id = ?",
                (project_id,) if project_id else ("",),
            )
            row = cursor.fetchone()

        if row is None:
            # Try loading with empty project_id for backward compatibility
            if project_id:
                return self.load("")
            raise StateNotFoundError(f"State not found for project: {project_id}")

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
        )
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
