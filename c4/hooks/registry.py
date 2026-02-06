"""Hook Registry - manages task lifecycle hooks.

Provides registration, execution, and management of hooks that respond
to task lifecycle events (BEFORE_SUBMIT, AFTER_COMPLETE, ON_FAILURE).
"""

from __future__ import annotations

import logging
from collections import defaultdict
from typing import Any

from .base import BaseHook, HookContext, HookPhase

logger = logging.getLogger(__name__)


class HookRegistry:
    """Central registry for task lifecycle hooks.

    Usage:
        registry = HookRegistry()
        registry.register(my_hook)
        registry.execute(HookPhase.AFTER_COMPLETE, context)
    """

    def __init__(self) -> None:
        self._hooks: dict[HookPhase, list[BaseHook]] = defaultdict(list)

    def register(self, hook: BaseHook) -> None:
        """Register a hook for its declared phase.

        Args:
            hook: Hook instance to register.

        Raises:
            ValueError: If a hook with the same name is already registered for the phase.
        """
        existing_names = {h.name for h in self._hooks[hook.phase]}
        if hook.name in existing_names:
            raise ValueError(
                f"Hook '{hook.name}' already registered for phase {hook.phase.value}"
            )
        self._hooks[hook.phase].append(hook)
        logger.debug("Registered hook '%s' for phase %s", hook.name, hook.phase.value)

    def unregister(self, name: str, phase: HookPhase | None = None) -> bool:
        """Remove a hook by name.

        Args:
            name: Hook name to remove.
            phase: If given, only remove from that phase. Otherwise remove from all.

        Returns:
            True if any hook was removed.
        """
        removed = False
        phases = [phase] if phase else list(HookPhase)
        for p in phases:
            before = len(self._hooks[p])
            self._hooks[p] = [h for h in self._hooks[p] if h.name != name]
            if len(self._hooks[p]) < before:
                removed = True
        return removed

    def execute(
        self,
        phase: HookPhase,
        context: HookContext,
    ) -> list[dict[str, Any]]:
        """Execute all hooks registered for a phase.

        Args:
            phase: The lifecycle phase to trigger.
            context: Execution context with task info.

        Returns:
            List of result dicts: [{"hook": name, "success": bool, "error": str|None}]
        """
        results: list[dict[str, Any]] = []
        for hook in self._hooks.get(phase, []):
            if not hook.enabled:
                logger.debug("Hook '%s' is disabled, skipping", hook.name)
                results.append({"hook": hook.name, "success": True, "error": "disabled"})
                continue

            try:
                success = hook.execute(context)
                results.append({"hook": hook.name, "success": success, "error": None})
                if success:
                    logger.debug("Hook '%s' executed successfully", hook.name)
                else:
                    logger.warning("Hook '%s' returned failure", hook.name)
            except Exception as e:
                logger.warning("Hook '%s' raised exception: %s", hook.name, e)
                results.append({"hook": hook.name, "success": False, "error": str(e)})

        return results

    def list_hooks(self, phase: HookPhase | None = None) -> list[dict[str, Any]]:
        """List registered hooks.

        Args:
            phase: If given, only list hooks for that phase.

        Returns:
            List of hook info dicts.
        """
        result = []
        phases = [phase] if phase else list(HookPhase)
        for p in phases:
            for hook in self._hooks.get(p, []):
                result.append({
                    "name": hook.name,
                    "phase": p.value,
                    "enabled": hook.enabled,
                })
        return result

    @property
    def count(self) -> int:
        """Total number of registered hooks."""
        return sum(len(hooks) for hooks in self._hooks.values())


# Module-level default registry
_default_registry: HookRegistry | None = None


def get_default_registry() -> HookRegistry:
    """Get or create the default hook registry."""
    global _default_registry
    if _default_registry is None:
        _default_registry = HookRegistry()
    return _default_registry
