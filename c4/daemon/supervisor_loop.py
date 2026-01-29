"""C4D Supervisor Loop - Background processing of checkpoint and repair queues"""

from __future__ import annotations

import asyncio
import logging
import threading
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..mcp_server import C4Daemon

from ..constants import REPAIR_PREFIX, WORKER_STALE_TIMEOUT_SEC
from ..monitoring import tracing
from ..notification import NotificationManager
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
        stale_worker_check_interval: float = 60.0,  # Check every minute
    ):
        self.daemon = daemon
        self.poll_interval = poll_interval
        self.max_retries = max_retries
        self.supervisor_timeout = supervisor_timeout
        self.stale_worker_check_interval = stale_worker_check_interval
        self.running = False
        self._task: asyncio.Task | None = None
        self._last_stale_check: float = 0

    async def start(self) -> None:
        """Start the supervisor loop"""
        import time

        if self.running:
            logger.warning("Supervisor loop already running")
            return

        self.running = True
        self._last_stale_check = time.time()
        logger.info("Supervisor loop started")

        while self.running:
            try:
                # Process checkpoint queue first (higher priority)
                processed_cp = await self._process_checkpoint_queue()

                # Then process repair queue if no checkpoint was processed
                if not processed_cp:
                    await self._process_repair_queue()

                # Periodically check for stale workers
                await self._check_stale_workers()

            except Exception as e:
                logger.error(f"Supervisor loop error: {e}")

            await asyncio.sleep(self.poll_interval)

        logger.info("Supervisor loop stopped")

    async def _check_stale_workers(self) -> None:
        """Check for stale workers periodically.

        By default (auto_recover=False), only logs warnings for long-running workers.
        The user can manually intervene via c4_handle_long_running.

        If auto_recover=True in config, automatically recovers stale workers.
        """
        import time

        now = time.time()
        if now - self._last_stale_check < self.stale_worker_check_interval:
            return

        self._last_stale_check = now

        if self.daemon.state_machine is None:
            return

        # Check if auto_recover is enabled (default: False)
        config = getattr(self.daemon, "_config", None)
        auto_recover = False
        if config and hasattr(config, "long_running"):
            auto_recover = config.long_running.auto_recover

        if not auto_recover:
            # Default behavior: only log warnings, don't auto-recover
            # Long-running alerts are shown via c4_status
            # User can manually kill via c4_handle_long_running
            return

        # Auto-recover mode: proceed with stale worker recovery
        try:
            recoveries = self.daemon.worker_manager.recover_stale_workers(
                stale_timeout_seconds=WORKER_STALE_TIMEOUT_SEC,
                lock_store=self.daemon.lock_store,
            )

            if recoveries:
                for recovery in recoveries:
                    logger.warning(
                        f"Auto-recovered stale worker {recovery['worker_id']}: "
                        f"task={recovery.get('task_id')}, "
                        f"elapsed={recovery.get('elapsed_seconds', 0):.0f}s"
                    )
                # Send notification for stale worker recovery
                NotificationManager.notify(
                    title="C4 Worker Recovery",
                    message=f"Recovered {len(recoveries)} stale worker(s)",
                    urgency="critical",
                )
        except Exception as e:
            logger.error(f"Error checking stale workers: {e}")

    def stop(self) -> None:
        """Stop the supervisor loop"""
        self.running = False
        logger.info("Supervisor loop stop requested")

    def _get_verifications_config(self) -> list[dict] | None:
        """
        Get verifications configuration from project config.

        Returns:
            List of verification configs or None if not configured/disabled
        """
        if not hasattr(self.daemon, "config") or self.daemon.config is None:
            return None

        verifications_config = getattr(self.daemon.config, "verifications", None)
        if verifications_config is None:
            return None

        if not verifications_config.enabled:
            return None

        # Convert VerificationItem models to dicts for VerificationRunner
        result = []
        base_url = verifications_config.base_url

        for item in verifications_config.items:
            if not item.enabled:
                continue

            config = dict(item.config)
            if base_url:
                config["base_url"] = base_url

            result.append(
                {
                    "type": item.type,
                    "name": item.name,
                    "config": config,
                }
            )

        return result if result else None

    async def _process_checkpoint_queue(self) -> bool:
        """
        Process the next item in the checkpoint queue.
        ...
        Returns:
            True if an item was processed, False if queue is empty
        """
        tracer = tracing.get_tracer()
        with tracer.start_as_current_span("supervisor_process_checkpoint") as span:
            if self.daemon.state_machine is None:
                return False

            state = self.daemon.state_machine.state
            if not state.checkpoint_queue:
                return False

            # Get the first item (FIFO)
            item = state.checkpoint_queue[0]
            span.set_attribute("checkpoint_id", item.checkpoint_id)

            # Check if checkpoint_as_task is enabled
        config = getattr(self.daemon, "config", None)
        if config and getattr(config, "checkpoint_as_task", False):
            # Checkpoint is handled as a task (CP-XXX)
            # Just clear the queue item - the CP task is created by _check_and_create_checkpoint_task
            logger.info(
                f"Checkpoint {item.checkpoint_id} handled as task. Removing from checkpoint_queue."
            )
            state.checkpoint_queue.pop(0)
            self._safe_save_state(f"checkpoint {item.checkpoint_id} moved to task")
            return True

        # Legacy mode: process checkpoint directly
        logger.info(f"Processing checkpoint: {item.checkpoint_id}")

        try:
            # Create bundle
            bundle_dir = self.daemon.create_checkpoint_bundle(item.checkpoint_id)

            # Run supervisor
            supervisor = Supervisor(
                self.daemon.root,
                prompts_dir=self.daemon.root / "prompts",
                daemon=self.daemon,
            )

            # Get verifications from config
            verifications = self._get_verifications_config()

            # Use strict mode if verifications are enabled or always for stricter review
            response = await asyncio.to_thread(
                supervisor.run_supervisor_strict,
                bundle_dir,
                verifications=verifications,
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

            # Send notification for checkpoint completion
            NotificationManager.notify(
                title="C4 Checkpoint",
                message=f"{item.checkpoint_id}: {response.decision.value}",
                urgency="normal" if response.decision.value == "APPROVE" else "critical",
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
        tracer = tracing.get_tracer()
        with tracer.start_as_current_span("supervisor_process_repair") as span:
            if self.daemon.state_machine is None:
                return False

            state = self.daemon.state_machine.state
            if not state.repair_queue:
                return False

            # Get the first item (FIFO)
            item = state.repair_queue[0]
            span.set_attribute("task_id", item.task_id)
            logger.info(f"Processing repair for task: {item.task_id}")

        try:
            # Create repair prompt for supervisor guidance
            guidance = await self._get_repair_guidance(item)

            if guidance:
                # Create new task with supervisor guidance
                repair_task_id = f"{REPAIR_PREFIX}{item.task_id}"
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
    """Manager for starting/stopping supervisor loop in background thread"""

    def __init__(self, daemon: C4Daemon):
        self.daemon = daemon
        self._loop: SupervisorLoop | None = None
        self._thread: threading.Thread | None = None
        self._event_loop: asyncio.AbstractEventLoop | None = None

    def _run_in_thread(
        self,
        poll_interval: float,
        max_retries: int,
        supervisor_timeout: int,
    ) -> None:
        """Run supervisor loop in a dedicated thread with its own event loop"""
        # Create a new event loop for this thread
        self._event_loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self._event_loop)

        self._loop = SupervisorLoop(
            self.daemon,
            poll_interval=poll_interval,
            max_retries=max_retries,
            supervisor_timeout=supervisor_timeout,
        )

        try:
            self._event_loop.run_until_complete(self._loop.start())
        except Exception as e:
            logger.error(f"Supervisor loop thread error: {e}")
        finally:
            self._event_loop.close()
            self._event_loop = None

    def start(
        self,
        poll_interval: float = 1.0,
        max_retries: int = 3,
        supervisor_timeout: int = 300,
    ) -> None:
        """Start supervisor loop in background thread"""
        if self._thread is not None and self._thread.is_alive():
            logger.warning("Supervisor loop already running")
            return

        self._thread = threading.Thread(
            target=self._run_in_thread,
            args=(poll_interval, max_retries, supervisor_timeout),
            daemon=True,  # Thread will be killed when main process exits
            name="c4-supervisor-loop",
        )
        self._thread.start()
        logger.info("Supervisor loop started in background thread")

    def stop(self) -> None:
        """Stop the supervisor loop"""
        if self._loop is not None:
            self._loop.stop()

        # Wait for thread to finish (with timeout)
        if self._thread is not None and self._thread.is_alive():
            self._thread.join(timeout=5.0)
            if self._thread.is_alive():
                logger.warning("Supervisor loop thread did not stop gracefully")

        self._thread = None

    @property
    def is_running(self) -> bool:
        """Check if supervisor loop is running"""
        return (
            self._thread is not None
            and self._thread.is_alive()
            and self._loop is not None
            and self._loop.running
        )
