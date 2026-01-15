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

    # Supabase (cloud)
    from c4.store import SupabaseStateStore
    state_store = SupabaseStateStore()  # Uses SUPABASE_URL/KEY env vars
"""

from .exceptions import (
    ConcurrentModificationError,
    LockConflictError,
    StateNotFoundError,
    StoreError,
)
from .local_file import LocalFileLockStore, LocalFileStateStore
from .protocol import LockStore, StateStore
from .sqlite import SQLiteLockStore, SQLiteStateStore, SQLiteTaskStore
from .supabase import SupabaseStateStore, create_supabase_store

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
    # Implementations - Supabase
    "SupabaseStateStore",
    "create_supabase_store",
]
