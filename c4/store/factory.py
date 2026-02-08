"""Store Factory - Pluggable backend selection

Supports two backends:
- sqlite (default): Local SQLite database
- local_file: File-based JSON storage

Configuration:
    Environment variable (in .env file or shell):
        C4_STORE_BACKEND=sqlite|local_file

Example:
    from c4.store.factory import create_state_store, create_lock_store

    # Uses C4_STORE_BACKEND env or defaults to sqlite
    state_store = create_state_store(c4_dir)
    lock_store = create_lock_store(c4_dir, state_store)
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import TYPE_CHECKING

from .protocol import LockStore, StateStore

if TYPE_CHECKING:
    from c4.models.config import StoreConfig


# Supported backends
BACKEND_SQLITE = "sqlite"
BACKEND_LOCAL_FILE = "local_file"

DEFAULT_BACKEND = BACKEND_SQLITE


def get_backend(config: "StoreConfig | None" = None) -> str:
    """Determine which backend to use.

    Priority:
    1. config.backend if provided
    2. C4_STORE_BACKEND environment variable
    3. Default: sqlite
    """
    if config and config.backend:
        return config.backend

    return os.environ.get("C4_STORE_BACKEND", DEFAULT_BACKEND)


def create_state_store(
    c4_dir: Path,
    config: "StoreConfig | None" = None,
) -> StateStore:
    """Create appropriate StateStore based on configuration."""
    backend = get_backend(config)

    if backend == BACKEND_SQLITE:
        from .sqlite import SQLiteStateStore

        return SQLiteStateStore(c4_dir / "c4.db")

    elif backend == BACKEND_LOCAL_FILE:
        from .local_file import LocalFileStateStore

        return LocalFileStateStore(c4_dir)

    else:
        raise ValueError(
            f"Unknown store backend: {backend}. "
            f"Supported: {BACKEND_SQLITE}, {BACKEND_LOCAL_FILE}"
        )


def create_lock_store(
    c4_dir: Path,
    state_store: StateStore | None = None,
    config: "StoreConfig | None" = None,
) -> LockStore:
    """Create appropriate LockStore based on configuration."""
    backend = get_backend(config)

    if backend == BACKEND_SQLITE:
        from .sqlite import SQLiteLockStore

        return SQLiteLockStore(c4_dir / "c4.db")

    elif backend == BACKEND_LOCAL_FILE:
        from .local_file import LocalFileLockStore, LocalFileStateStore

        if state_store is None:
            state_store = LocalFileStateStore(c4_dir)
        elif not isinstance(state_store, LocalFileStateStore):
            state_store = LocalFileStateStore(c4_dir)

        return LocalFileLockStore(state_store)

    else:
        raise ValueError(
            f"Unknown store backend: {backend}. "
            f"Supported: {BACKEND_SQLITE}, {BACKEND_LOCAL_FILE}"
        )


def create_task_store(
    c4_dir: Path,
    config: "StoreConfig | None" = None,
):
    """Create SQLiteTaskStore (only available for SQLite backend)."""
    backend = get_backend(config)

    if backend != BACKEND_SQLITE:
        raise ValueError(
            f"TaskStore is only available for SQLite backend, current backend: {backend}"
        )

    from .sqlite import SQLiteTaskStore

    return SQLiteTaskStore(c4_dir / "c4.db")
