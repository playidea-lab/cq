"""Base hook definitions for C4 task lifecycle hooks.

Provides BaseHook ABC and HookPhase enum for the hook registry system.
These are *task* lifecycle hooks, distinct from Git hooks in __init__.py.
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from enum import Enum
from typing import Any

logger = logging.getLogger(__name__)


class HookPhase(str, Enum):
    """Task lifecycle phases where hooks can be triggered."""

    BEFORE_SUBMIT = "before_submit"
    AFTER_COMPLETE = "after_complete"
    ON_FAILURE = "on_failure"


class HookContext:
    """Context passed to hooks during execution.

    Attributes:
        task_id: The task ID being processed.
        phase: The lifecycle phase triggering this hook.
        task_data: Full task data dict (fields vary by phase).
        extras: Additional context (commit_sha, validation_results, etc.).
    """

    def __init__(
        self,
        task_id: str,
        phase: HookPhase,
        task_data: dict[str, Any] | None = None,
        **extras: Any,
    ) -> None:
        self.task_id = task_id
        self.phase = phase
        self.task_data = task_data or {}
        self.extras = extras

    def get(self, key: str, default: Any = None) -> Any:
        """Get a value from extras or task_data."""
        if key in self.extras:
            return self.extras[key]
        return self.task_data.get(key, default)


class BaseHook(ABC):
    """Abstract base class for task lifecycle hooks.

    Subclasses must implement:
        - name: unique identifier for the hook
        - phase: which lifecycle phase to trigger on
        - execute: the hook logic
    """

    @property
    @abstractmethod
    def name(self) -> str:
        """Unique hook identifier."""
        ...

    @property
    @abstractmethod
    def phase(self) -> HookPhase:
        """Lifecycle phase this hook runs on."""
        ...

    @abstractmethod
    def execute(self, context: HookContext) -> bool:
        """Execute the hook.

        Args:
            context: Hook execution context with task info.

        Returns:
            True if hook executed successfully, False otherwise.
        """
        ...

    @property
    def enabled(self) -> bool:
        """Whether this hook is currently enabled. Override to add conditions."""
        return True
