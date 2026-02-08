"""C4 Store - Pluggable state and lock storage backends

Usage:
    from c4.store import LocalFileStateStore, LocalFileLockStore

    # File-based
    state_store = LocalFileStateStore(c4_dir)
    lock_store = LocalFileLockStore(state_store)

    # SQLite (default)
    from c4.store import SQLiteStateStore, SQLiteLockStore
    state_store = SQLiteStateStore(db_path)
    lock_store = SQLiteLockStore(db_path)
"""

from .exceptions import (
    ConcurrentModificationError,
    LockConflictError,
    StateNotFoundError,
    StoreError,
)
from .factory import (
    BACKEND_LOCAL_FILE,
    BACKEND_SQLITE,
    create_lock_store,
    create_state_store,
    create_task_store,
    get_backend,
)
from .local_file import LocalFileLockStore, LocalFileStateStore
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
    # Implementations - Local File
    "LocalFileStateStore",
    "LocalFileLockStore",
    # Implementations - SQLite
    "SQLiteStateStore",
    "SQLiteLockStore",
    "SQLiteTaskStore",
    # Factory functions
    "create_state_store",
    "create_lock_store",
    "create_task_store",
    "get_backend",
    # Backend constants
    "BACKEND_SQLITE",
    "BACKEND_LOCAL_FILE",
]
