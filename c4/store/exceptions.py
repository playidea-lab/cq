"""Store Exceptions"""


class StoreError(Exception):
    """Base exception for store operations."""

    pass


class StateNotFoundError(StoreError):
    """Raised when state doesn't exist for a project."""

    pass


class LockConflictError(StoreError):
    """Raised when lock acquisition fails due to conflict."""

    pass


class ConcurrentModificationError(StoreError):
    """Raised when state was modified by another process."""

    pass


class MigrationError(StoreError):
    """Raised when migration operation fails."""

    pass


class TransactionError(StoreError):
    """Raised when a database transaction fails (commit/rollback).

    This indicates a serious database error that may require attention.
    """

    pass
