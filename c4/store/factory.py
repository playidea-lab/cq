"""Store Factory - Pluggable backend selection

Supports three backends:
- sqlite (default): Local SQLite database
- local_file: File-based JSON storage
- supabase: Cloud-based Supabase storage

Configuration:
    Environment variable (in .env file or shell):
        C4_STORE_BACKEND=sqlite|local_file|supabase
        SUPABASE_URL=https://xxx.supabase.co
        SUPABASE_KEY=your-anon-key

    Or in config.yaml:
        store:
          backend: supabase
          supabase_url: https://xxx.supabase.co
          supabase_key: your-anon-key

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

from dotenv import load_dotenv

from .protocol import LockStore, StateStore

# Load .env file from current directory or parent directories
# This allows environment variables to be set in .env file
load_dotenv()

if TYPE_CHECKING:
    from c4.models.config import StoreConfig


# Supported backends
BACKEND_SQLITE = "sqlite"
BACKEND_LOCAL_FILE = "local_file"
BACKEND_SUPABASE = "supabase"

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
    """Create appropriate StateStore based on configuration.

    Args:
        c4_dir: Path to .c4 directory
        config: Optional StoreConfig from config.yaml

    Returns:
        StateStore implementation

    Raises:
        ValueError: If backend is unknown or required config missing
        ImportError: If supabase package not installed for supabase backend
    """
    backend = get_backend(config)

    if backend == BACKEND_SQLITE:
        from .sqlite import SQLiteStateStore

        return SQLiteStateStore(c4_dir / "c4.db")

    elif backend == BACKEND_LOCAL_FILE:
        from .local_file import LocalFileStateStore

        return LocalFileStateStore(c4_dir)

    elif backend == BACKEND_SUPABASE:
        # Get Supabase credentials and team settings
        url = None
        key = None
        team_id = None
        access_token = None
        if config:
            url = config.supabase_url
            key = config.supabase_key
            team_id = config.team_id
            access_token = config.access_token

        try:
            from .supabase import create_supabase_store

            return create_supabase_store(
                url=url,
                key=key,
                team_id=team_id,
                access_token=access_token,
            )
        except ImportError as e:
            raise ImportError(
                "Supabase backend requires 'supabase' package. "
                "Install with: uv add 'c4[cloud]' or uv add supabase"
            ) from e

    else:
        raise ValueError(
            f"Unknown store backend: {backend}. "
            f"Supported: {BACKEND_SQLITE}, {BACKEND_LOCAL_FILE}, {BACKEND_SUPABASE}"
        )


def create_lock_store(
    c4_dir: Path,
    state_store: StateStore | None = None,
    config: "StoreConfig | None" = None,
) -> LockStore:
    """Create appropriate LockStore based on configuration.

    Args:
        c4_dir: Path to .c4 directory
        state_store: Optional StateStore (required for local_file backend)
        config: Optional StoreConfig from config.yaml

    Returns:
        LockStore implementation

    Raises:
        ValueError: If backend is unknown or required config missing
    """
    backend = get_backend(config)

    if backend == BACKEND_SQLITE:
        from .sqlite import SQLiteLockStore

        return SQLiteLockStore(c4_dir / "c4.db")

    elif backend == BACKEND_LOCAL_FILE:
        from .local_file import LocalFileLockStore, LocalFileStateStore

        if state_store is None:
            state_store = LocalFileStateStore(c4_dir)
        elif not isinstance(state_store, LocalFileStateStore):
            # Create a new LocalFileStateStore for lock storage
            state_store = LocalFileStateStore(c4_dir)

        return LocalFileLockStore(state_store)

    elif backend == BACKEND_SUPABASE:
        # SupabaseStateStore implements both StateStore and LockStore
        url = None
        key = None
        team_id = None
        access_token = None
        if config:
            url = config.supabase_url
            key = config.supabase_key
            team_id = config.team_id
            access_token = config.access_token

        try:
            from .supabase import create_supabase_store

            return create_supabase_store(
                url=url,
                key=key,
                team_id=team_id,
                access_token=access_token,
            )
        except ImportError as e:
            raise ImportError(
                "Supabase backend requires 'supabase' package. "
                "Install with: uv add 'c4[cloud]' or uv add supabase"
            ) from e

    else:
        raise ValueError(
            f"Unknown store backend: {backend}. "
            f"Supported: {BACKEND_SQLITE}, {BACKEND_LOCAL_FILE}, {BACKEND_SUPABASE}"
        )


def create_task_store(
    c4_dir: Path,
    config: "StoreConfig | None" = None,
):
    """Create SQLiteTaskStore (only available for SQLite backend).

    Args:
        c4_dir: Path to .c4 directory
        config: Optional StoreConfig from config.yaml

    Returns:
        SQLiteTaskStore instance

    Raises:
        ValueError: If backend is not sqlite
    """
    backend = get_backend(config)

    if backend != BACKEND_SQLITE:
        raise ValueError(
            f"TaskStore is only available for SQLite backend, "
            f"current backend: {backend}"
        )

    from .sqlite import SQLiteTaskStore

    return SQLiteTaskStore(c4_dir / "c4.db")
