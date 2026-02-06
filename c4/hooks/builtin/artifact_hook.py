"""Artifact hook - auto-detect artifacts on task completion.

Triggers on AFTER_COMPLETE to scan for output artifacts
and register them in the local artifact store.
"""

from __future__ import annotations

import logging

from c4.hooks.base import BaseHook, HookContext, HookPhase

logger = logging.getLogger(__name__)


class ArtifactHook(BaseHook):
    """Auto-detect and register artifacts after task completion.

    Scans configured directories (outputs/, checkpoints/, etc.) for
    new files created during task execution and registers them in
    the local artifact store.
    """

    @property
    def name(self) -> str:
        return "artifact_auto_detect"

    @property
    def phase(self) -> HookPhase:
        return HookPhase.AFTER_COMPLETE

    def execute(self, context: HookContext) -> bool:
        """Scan for and register task artifacts.

        Expects context to optionally contain:
            - workspace: Path to scan (defaults to cwd)
            - artifact_spec: Explicit artifact spec from task
        """
        workspace = context.get("workspace")

        if not workspace:
            logger.debug("No workspace for task %s, skipping artifact detection", context.task_id)
            return True

        try:
            from c4.artifacts.detector import scan_outputs

            detected = scan_outputs(workspace)
            logger.info(
                "Detected %d artifacts for task %s",
                len(detected),
                context.task_id,
            )
            return True
        except ImportError:
            logger.debug("Artifact modules not available, skipping auto-detect")
            return True
        except Exception as e:
            logger.warning("Artifact detection failed for task %s: %s", context.task_id, e)
            return False
