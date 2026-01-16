"""C4 Store - Pluggable state and lock storage backends

Usage:
    from c4.store import LocalFileStateStore, LocalFileLockStore

    # File-based (default)
    state_store = LocalFileStateStore(c4_dir)
    lock_store = LocalFileLockStore(state_store)

    # SQLite
    from c4.store import SQLiteStateStore, SQLiteLockStore
    state_store = SQLiteStateStore(db_path)
    lock_store = SQLiteLockStore(db_path)
"""

from .exceptions import (
    ConcurrentModificationError,
    LockConflictError,
    MigrationError,
    StateNotFoundError,
    StoreError,
)
from .local_file import LocalFileLockStore, LocalFileStateStore
from .migration import (
    ExportData,
    MigrationManager,
    MigrationSnapshot,
    migrate_local_to_team,
    migrate_team_to_local,
)
from .protocol import LockStore, StateStore
from .sqlite import SQLiteLockStore, SQLiteStateStore, SQLiteTaskStore

__all__ = [
    # Protocols
    "StateStore",
    "LockStore",
    # Exceptions
    "StoreError",
    "StateNotFoundError",
    "LockConflictError",
    "ConcurrentModificationError",
    "MigrationError",
    # Implementations - Local File
    "LocalFileStateStore",
    "LocalFileLockStore",
    # Implementations - SQLite
    "SQLiteStateStore",
    "SQLiteLockStore",
    "SQLiteTaskStore",
    # Migration
    "MigrationManager",
    "MigrationSnapshot",
    "ExportData",
    "migrate_local_to_team",
    "migrate_team_to_local",
]
