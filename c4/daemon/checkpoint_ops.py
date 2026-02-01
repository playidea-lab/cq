"""Checkpoint operations for C4 Daemon.

This module contains checkpoint operations extracted from C4Daemon:
- c4_checkpoint: Record supervisor checkpoint decision
- check_and_trigger_checkpoint: Check and trigger checkpoint conditions
- create_checkpoint_bundle: Create a bundle for supervisor review
- run_supervisor_review: Run supervisor review on a checkpoint bundle
- process_supervisor_decision: Process supervisor decision and update state

These operations are delegated from C4Daemon for modularity.
"""

import logging
from pathlib import Path
from typing import TYPE_CHECKING, Any

from ..models import (
    CheckpointResponse,
    EventType,
    ProjectStatus,
)
from ..notification import NotificationManager
from ..state_machine import StateTransitionError

if TYPE_CHECKING:
    from .c4_daemon import C4Daemon

logger = logging.getLogger(__name__)


class CheckpointOps:
    """Checkpoint operations handler for C4 Daemon.

    Provides checkpoint management operations including
    supervisor review and decision processing.
    """

    def __init__(self, daemon: "C4Daemon"):
        """Initialize CheckpointOps with parent daemon reference.

        Args:
            daemon: Parent C4Daemon instance for state and config access
        """
        self._daemon = daemon

    # =========================================================================
    # Checkpoint Decision
    # =========================================================================

    def checkpoint(
        self,
        checkpoint_id: str,
        decision: str,
        notes: str,
        required_changes: list[str] | None = None,
    ) -> CheckpointResponse:
        """Record supervisor checkpoint decision.

        Args:
            checkpoint_id: ID of the checkpoint
            decision: Decision (APPROVE, REQUEST_CHANGES, REPLAN)
            notes: Supervisor notes
            required_changes: List of required changes for REQUEST_CHANGES

        Returns:
            CheckpointResponse with success status
        """
        if self._daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self._daemon.state_machine.state

        # Validate we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return CheckpointResponse(
                success=False,
                message=f"Not in CHECKPOINT state (current: {state.status.value})",
            )

        # Emit decision event
        self._daemon.state_machine.emit_event(
            EventType.SUPERVISOR_DECISION,
            "supervisor",
            {
                "checkpoint_id": checkpoint_id,
                "decision": decision,
                "notes": notes,
                "required_changes": required_changes,
            },
        )

        # Process decision
        try:
            if decision == "APPROVE":
                # Add to passed checkpoints to prevent re-triggering
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Merge completed task branches into work branch
                # Branch strategy: task branches -> work branch on checkpoint APPROVE
                merge_results = self._daemon._merge_completed_task_branches(state)
                if merge_results:
                    logger.info(
                        f"Checkpoint {checkpoint_id}: merged {len(merge_results)} branches"
                    )

                # Check if this is the final checkpoint
                is_final = not state.queue.pending
                if is_final:
                    self._daemon.state_machine.transition("approve_final")
                    # Perform completion action (merge, pr, or manual)
                    completion_result = self._daemon._perform_completion_action()
                    if completion_result:
                        logger.info(f"Plan completed: {completion_result}")
                else:
                    self._daemon.state_machine.transition("approve")
                state.metrics.checkpoints_passed += 1

            elif decision == "REQUEST_CHANGES":
                # Mark checkpoint as passed to prevent re-triggering
                # (supervisor has reviewed; RC tasks are the follow-up)
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Add required changes as tasks
                if required_changes:
                    for i, change in enumerate(required_changes):
                        task_id = f"RC-{checkpoint_id}-{i+1:02d}"
                        self._daemon.c4_add_todo(
                            task_id=task_id,
                            title=change,
                            scope=None,
                            dod=change,
                        )
                self._daemon.state_machine.transition("request_changes")

            elif decision == "REPLAN":
                self._daemon.state_machine.transition("replan")

            else:
                return CheckpointResponse(
                    success=False,
                    message=f"Invalid decision: {decision}",
                )

            # Clear checkpoint state
            state.checkpoint.current = None
            state.checkpoint.state = "pending"

            # Send notification for checkpoint decision
            urgency = "normal" if decision == "APPROVE" else "critical"
            NotificationManager.notify(
                title="C4 Checkpoint Decision",
                message=f"{checkpoint_id}: {decision}",
                urgency=urgency,
            )

            return CheckpointResponse(success=True)

        except StateTransitionError as e:
            return CheckpointResponse(success=False, message=str(e))

    # =========================================================================
    # Checkpoint Triggering
    # =========================================================================

    def check_and_trigger_checkpoint(self) -> dict[str, Any] | None:
        """Check if checkpoint conditions are met and trigger if so.

        Returns:
            Checkpoint info if triggered, None otherwise
        """
        if self._daemon.state_machine is None:
            return None

        state = self._daemon.state_machine.state

        # Only check in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Check gate conditions
        cp_id = self._daemon.state_machine.check_gate_conditions(self._daemon.config)
        if cp_id:
            # Enter checkpoint state
            self._daemon.state_machine.enter_checkpoint(cp_id)

            return {
                "checkpoint_id": cp_id,
                "triggered": True,
                "message": f"Checkpoint {cp_id} conditions met, entering CHECKPOINT state",
            }

        return None

    # =========================================================================
    # Supervisor Integration
    # =========================================================================

    def create_checkpoint_bundle(self, checkpoint_id: str | None = None) -> Path:
        """Create a bundle for supervisor review.

        Args:
            checkpoint_id: Checkpoint ID (uses current if not specified)

        Returns:
            Path to the created bundle directory
        """
        from ..bundle import BundleCreator

        if self._daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self._daemon.state_machine.state

        # Use current checkpoint if not specified
        if checkpoint_id is None:
            checkpoint_id = state.checkpoint.current
        if checkpoint_id is None:
            raise ValueError("No checkpoint ID specified or active")

        # Get completed tasks
        tasks_completed = list(state.queue.done)

        # Get last validation results
        validation_results = []
        if state.last_validation:
            for name, status in state.last_validation.items():
                validation_results.append({"name": name, "status": status})

        # Create bundle
        bundle_creator = BundleCreator(self._daemon.root, self._daemon.c4_dir)
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id=checkpoint_id,
            tasks_completed=tasks_completed,
            validation_results=validation_results,
        )

        return bundle_dir

    def run_supervisor_review(
        self,
        bundle_dir: Path | None = None,
        use_mock: bool = False,
        mock_decision: str = "APPROVE",
        timeout: int = 300,
        max_retries: int = 3,
    ) -> dict[str, Any]:
        """Run supervisor review on a checkpoint bundle.

        Args:
            bundle_dir: Path to bundle (creates new if None)
            use_mock: Use mock supervisor instead of real Claude CLI
            mock_decision: Decision for mock supervisor
            timeout: Timeout for real supervisor
            max_retries: Max retries for real supervisor

        Returns:
            Dictionary with supervisor decision and processing result
        """
        from ..models import SupervisorDecision
        from ..supervisor import Supervisor
        from ..supervisor.backend import SupervisorError

        if self._daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self._daemon.state_machine.state

        # Ensure we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return {
                "success": False,
                "error": f"Not in CHECKPOINT state (current: {state.status.value})",
            }

        # Create bundle if not provided
        if bundle_dir is None:
            bundle_dir = self.create_checkpoint_bundle()

        # Run supervisor
        supervisor = Supervisor(self._daemon.root, prompts_dir=self._daemon.root / "prompts")

        try:
            if use_mock:
                decision_enum = SupervisorDecision(mock_decision)
                response = supervisor.run_supervisor_mock(
                    bundle_dir,
                    mock_decision=decision_enum,
                    mock_notes=f"Mock {mock_decision} decision",
                    mock_changes=["Mock change 1"] if mock_decision == "REQUEST_CHANGES" else None,
                )
            else:
                response = supervisor.run_supervisor(
                    bundle_dir,
                    timeout=timeout,
                    max_retries=max_retries,
                )

            # Process the decision
            return self.process_supervisor_decision(response)

        except SupervisorError as e:
            return {"success": False, "error": str(e)}

    def process_supervisor_decision(self, response: Any) -> dict[str, Any]:
        """Process supervisor decision and update state accordingly.

        Args:
            response: SupervisorResponse from supervisor

        Returns:
            Dictionary with processing result
        """
        if self._daemon.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Apply decision via checkpoint
        checkpoint_response = self.checkpoint(
            checkpoint_id=response.checkpoint_id,
            decision=response.decision.value,
            notes=response.notes,
            required_changes=response.required_changes if response.required_changes else None,
        )

        return {
            "success": checkpoint_response.success,
            "decision": response.decision.value,
            "checkpoint_id": response.checkpoint_id,
            "notes": response.notes,
            "required_changes": response.required_changes,
            "new_status": self._daemon.state_machine.state.status.value,
            "message": checkpoint_response.message,
        }
