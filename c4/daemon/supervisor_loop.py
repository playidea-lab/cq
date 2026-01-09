"""C4D Supervisor Loop - Background processing of checkpoint and repair queues"""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..mcp_server import C4Daemon

from ..models import SupervisorDecision
from ..supervisor import Supervisor, SupervisorError

logger = logging.getLogger(__name__)


class SupervisorLoop:
    """Background loop that processes checkpoint queue and repair queue"""

    def __init__(
        self,
        daemon: C4Daemon,
        poll_interval: float = 1.0,
        max_retries: int = 3,
        supervisor_timeout: int = 300,
    ):
        self.daemon = daemon
        self.poll_interval = poll_interval
        self.max_retries = max_retries
        self.supervisor_timeout = supervisor_timeout
        self.running = False
        self._task: asyncio.Task | None = None

    async def start(self) -> None:
        """Start the supervisor loop"""
        if self.running:
            logger.warning("Supervisor loop already running")
            return

        self.running = True
        logger.info("Supervisor loop started")

        while self.running:
            try:
                # Process checkpoint queue first (higher priority)
                processed_cp = await self._process_checkpoint_queue()

                # Then process repair queue if no checkpoint was processed
                if not processed_cp:
                    await self._process_repair_queue()

            except Exception as e:
                logger.error(f"Supervisor loop error: {e}")

            await asyncio.sleep(self.poll_interval)

        logger.info("Supervisor loop stopped")

    def stop(self) -> None:
        """Stop the supervisor loop"""
        self.running = False
        logger.info("Supervisor loop stop requested")

    async def _process_checkpoint_queue(self) -> bool:
        """
        Process the next item in the checkpoint queue.

        Returns:
            True if an item was processed, False if queue is empty
        """
        if self.daemon.state_machine is None:
            return False

        state = self.daemon.state_machine.state
        if not state.checkpoint_queue:
            return False

        # Get the first item (FIFO)
        item = state.checkpoint_queue[0]
        logger.info(f"Processing checkpoint: {item.checkpoint_id}")

        try:
            # Create bundle
            bundle_dir = self.daemon.create_checkpoint_bundle(item.checkpoint_id)

            # Run supervisor
            supervisor = Supervisor(
                self.daemon.root,
                prompts_dir=self.daemon.root / "prompts",
            )

            response = await asyncio.to_thread(
                supervisor.run_supervisor,
                bundle_dir,
                timeout=self.supervisor_timeout,
                max_retries=self.max_retries,
            )

            # Apply decision via c4_checkpoint
            result = self.daemon.c4_checkpoint(
                checkpoint_id=response.checkpoint_id,
                decision=response.decision.value,
                notes=response.notes,
                required_changes=response.required_changes if response.required_changes else None,
            )

            logger.info(
                f"Checkpoint {item.checkpoint_id} processed: "
                f"decision={response.decision.value}, success={result.success}"
            )

            # Remove from queue on success
            if result.success:
                state.checkpoint_queue.pop(0)
                self._safe_save_state(f"checkpoint {item.checkpoint_id} success")

            return True

        except SupervisorError as e:
            logger.error(f"Supervisor failed for checkpoint {item.checkpoint_id}: {e}")
            self._handle_checkpoint_retry(state, item, "supervisor error")
            return True

        except Exception as e:
            logger.error(f"Unexpected error processing checkpoint {item.checkpoint_id}: {e}")
            self._handle_checkpoint_retry(state, item, "unexpected error")
            return True

    def _handle_checkpoint_retry(self, state, item, error_type: str) -> None:
        """
        Handle retry logic for checkpoint processing failures.
        Consolidated to avoid code duplication between SupervisorError and Exception handlers.
        """
        # Increment retry count (actual attempt number = retry_count + 1)
        item.retry_count += 1

        if item.retry_count >= item.max_retries:
            # Dead letter - remove from queue after max retries
            # Note: retry_count tracks retries after initial attempt
            total_attempts = item.retry_count + 1
            logger.error(
                f"Checkpoint {item.checkpoint_id} failed after {total_attempts} attempts "
                f"({item.retry_count} retries). Removing from queue. Manual intervention required."
            )
            state.checkpoint_queue.pop(0)
            self._safe_save_state(f"checkpoint {item.checkpoint_id} dead letter")
        else:
            # Save updated retry count
            self._safe_save_state(f"checkpoint {item.checkpoint_id} retry")
            logger.warning(
                f"Checkpoint {item.checkpoint_id} retry {item.retry_count}/{item.max_retries}"
            )

    def _safe_save_state(self, context: str) -> bool:
        """
        Safely save state with error handling.

        Args:
            context: Description of what operation triggered the save (for logging)

        Returns:
            True if save succeeded, False otherwise
        """
        try:
            self.daemon.state_machine.save_state()
            return True
        except Exception as e:
            logger.critical(
                f"CRITICAL: Failed to persist state after {context}: {e}. "
                f"State may be inconsistent. Manual intervention required."
            )
            return False

    async def _process_repair_queue(self) -> bool:
        """
        Process the next item in the repair queue.

        Returns:
            True if an item was processed, False if queue is empty
        """
        if self.daemon.state_machine is None:
            return False

        state = self.daemon.state_machine.state
        if not state.repair_queue:
            return False

        # Get the first item (FIFO)
        item = state.repair_queue[0]
        logger.info(f"Processing repair for task: {item.task_id}")

        try:
            # Create repair prompt for supervisor guidance
            guidance = await self._get_repair_guidance(item)

            if guidance:
                # Create new task with supervisor guidance
                repair_task_id = f"REPAIR-{item.task_id}"
                self.daemon.c4_add_todo(
                    task_id=repair_task_id,
                    title=f"Fix blocked task {item.task_id}",
                    scope=None,
                    dod=guidance,
                )

                logger.info(f"Created repair task {repair_task_id} for {item.task_id}")

            # Remove from repair queue
            state.repair_queue.pop(0)
            self._safe_save_state(f"repair queue item {item.task_id}")

            return True

        except Exception as e:
            logger.error(f"Error processing repair for task {item.task_id}: {e}")
            return True

    async def _get_repair_guidance(self, item) -> str | None:
        """
        Get supervisor guidance for a blocked task.

        Args:
            item: RepairQueueItem

        Returns:
            Guidance string or None if failed
        """
        # Create a repair prompt
        prompt = f"""# Repair Guidance Request

A task has been blocked after {item.attempts} failed attempts.

## Task ID
{item.task_id}

## Failure Signature
{item.failure_signature}

## Last Error
{item.last_error}

## Your Task

Provide specific guidance on how to fix this issue. Include:
1. Root cause analysis
2. Specific steps to resolve
3. Code changes if applicable

Respond with a clear, actionable guidance paragraph."""

        try:
            import subprocess

            result = await asyncio.to_thread(
                subprocess.run,
                ["claude", "-p", prompt],
                capture_output=True,
                text=True,
                timeout=60,
                cwd=self.daemon.root,
            )

            if result.returncode == 0:
                return result.stdout.strip()
            else:
                logger.error(f"Claude CLI error: {result.stderr}")
                return f"Fix the issue in task {item.task_id}: {item.failure_signature}"

        except subprocess.TimeoutExpired:
            logger.error("Repair guidance request timed out")
            return f"Fix the issue in task {item.task_id}: {item.failure_signature}"

        except FileNotFoundError:
            logger.warning("Claude CLI not found, using fallback guidance")
            return f"Fix the issue in task {item.task_id}: {item.failure_signature}"


class SupervisorLoopManager:
    """Manager for starting/stopping supervisor loop in background"""

    def __init__(self, daemon: C4Daemon):
        self.daemon = daemon
        self._loop: SupervisorLoop | None = None
        self._task: asyncio.Task | None = None

    def start(
        self,
        poll_interval: float = 1.0,
        max_retries: int = 3,
        supervisor_timeout: int = 300,
    ) -> None:
        """Start supervisor loop in background"""
        if self._loop is not None and self._loop.running:
            logger.warning("Supervisor loop already running")
            return

        self._loop = SupervisorLoop(
            self.daemon,
            poll_interval=poll_interval,
            max_retries=max_retries,
            supervisor_timeout=supervisor_timeout,
        )

        # Create task in current event loop
        try:
            loop = asyncio.get_running_loop()
            self._task = loop.create_task(self._loop.start())
            logger.info("Supervisor loop started in background")
        except RuntimeError:
            # No event loop running, will need to be started externally
            logger.warning("No event loop running, supervisor loop not started")

    def stop(self) -> None:
        """Stop the supervisor loop"""
        if self._loop is not None:
            self._loop.stop()

        if self._task is not None:
            self._task.cancel()
            self._task = None

    @property
    def is_running(self) -> bool:
        """Check if supervisor loop is running"""
        return self._loop is not None and self._loop.running
