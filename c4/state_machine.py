"""C4D State Machine - State transitions and invariant enforcement"""

from pathlib import Path
from typing import TYPE_CHECKING

from .models import (
    C4Config,
    C4State,
    CheckpointConfig,
    CheckpointState,
    Event,
    EventType,
    ExecutionMode,
    ProjectStatus,
)

if TYPE_CHECKING:
    from .store import StateStore


class StateTransitionError(Exception):
    """Raised when an invalid state transition is attempted"""

    pass


class InvariantViolationError(Exception):
    """Raised when an invariant is violated"""

    pass


# =============================================================================
# State Transition Table
# =============================================================================

# (from_state, event) → to_state
# Extended workflow: INIT → DISCOVERY → DESIGN → PLAN → EXECUTE → CHECKPOINT → COMPLETE
TRANSITIONS: dict[tuple[ProjectStatus, str], ProjectStatus] = {
    # INIT transitions
    (ProjectStatus.INIT, "c4_init"): ProjectStatus.DISCOVERY,  # Changed: now goes to DISCOVERY
    (ProjectStatus.INIT, "c4_init_legacy"): ProjectStatus.PLAN,  # Legacy: direct to PLAN
    # DISCOVERY transitions
    (ProjectStatus.DISCOVERY, "discovery_complete"): ProjectStatus.DESIGN,
    (ProjectStatus.DISCOVERY, "skip_discovery"): ProjectStatus.PLAN,  # Skip to PLAN (legacy mode)
    (ProjectStatus.DISCOVERY, "c4_stop"): ProjectStatus.HALTED,
    # DESIGN transitions
    (ProjectStatus.DESIGN, "design_approved"): ProjectStatus.PLAN,
    (ProjectStatus.DESIGN, "design_rejected"): ProjectStatus.DISCOVERY,  # Back to interview
    (ProjectStatus.DESIGN, "skip_design"): ProjectStatus.PLAN,  # Skip design phase
    (ProjectStatus.DESIGN, "c4_stop"): ProjectStatus.HALTED,
    # PLAN transitions
    (ProjectStatus.PLAN, "c4_run"): ProjectStatus.EXECUTE,
    (ProjectStatus.PLAN, "c4_stop"): ProjectStatus.HALTED,
    (ProjectStatus.PLAN, "back_to_design"): ProjectStatus.DESIGN,  # Revise design
    # EXECUTE transitions
    (ProjectStatus.EXECUTE, "gate_reached"): ProjectStatus.CHECKPOINT,
    (ProjectStatus.EXECUTE, "c4_stop"): ProjectStatus.HALTED,
    (ProjectStatus.EXECUTE, "all_done"): ProjectStatus.COMPLETE,
    (ProjectStatus.EXECUTE, "fatal_error"): ProjectStatus.ERROR,
    # CHECKPOINT transitions
    (ProjectStatus.CHECKPOINT, "approve"): ProjectStatus.EXECUTE,
    (ProjectStatus.CHECKPOINT, "approve_final"): ProjectStatus.COMPLETE,
    (ProjectStatus.CHECKPOINT, "request_changes"): ProjectStatus.EXECUTE,
    (ProjectStatus.CHECKPOINT, "replan"): ProjectStatus.PLAN,
    (ProjectStatus.CHECKPOINT, "redesign"): ProjectStatus.DESIGN,  # New: back to design
    (ProjectStatus.CHECKPOINT, "fatal_error"): ProjectStatus.ERROR,
    # HALTED transitions
    (ProjectStatus.HALTED, "c4_run"): ProjectStatus.EXECUTE,
    (ProjectStatus.HALTED, "c4_plan"): ProjectStatus.PLAN,
    (ProjectStatus.HALTED, "c4_discovery"): ProjectStatus.DISCOVERY,  # Resume discovery
    # COMPLETE - no transitions out
    # ERROR - no transitions out (manual intervention required)
}

# Allowed CLI commands per state
ALLOWED_COMMANDS: dict[ProjectStatus, list[str]] = {
    ProjectStatus.INIT: ["init"],
    ProjectStatus.DISCOVERY: ["plan", "status", "stop"],  # New state
    ProjectStatus.DESIGN: ["plan", "status", "stop"],  # New state
    ProjectStatus.PLAN: ["plan", "run", "stop", "status"],
    ProjectStatus.EXECUTE: ["status", "worker join", "worker submit", "stop"],
    ProjectStatus.CHECKPOINT: ["status"],
    ProjectStatus.COMPLETE: ["status"],
    ProjectStatus.HALTED: ["run", "plan", "status"],
    ProjectStatus.ERROR: ["status"],
}


class StateMachine:
    """C4 State Machine with transition validation and invariant enforcement"""

    def __init__(self, c4_dir: Path, store: "StateStore | None" = None):
        self.c4_dir = c4_dir
        self.events_dir = c4_dir / "events"
        self._state: C4State | None = None
        self._project_id: str | None = None  # Cached for atomic operations

        # Initialize store (default to LocalFileStateStore)
        if store is None:
            from .store import LocalFileStateStore

            self._store = LocalFileStateStore(c4_dir)
        else:
            self._store = store

    def _get_next_event_id(self) -> str:
        """
        Get the next event ID atomically.

        Uses store.atomic_modify to prevent race conditions.
        Falls back to file-based counter when project_id is not available.
        """
        project_id = self._project_id or (self._state.project_id if self._state else None)

        # Use atomic_modify for all stores that support it
        if project_id and self._store:
            try:
                with self._store.atomic_modify(project_id) as state:
                    next_id = state.metrics.events_emitted + 1
                    state.metrics.events_emitted = next_id
                    # Update cached state
                    if self._state:
                        self._state.metrics.events_emitted = next_id
                    return f"{next_id:06d}"
            except Exception:
                # Fallback if atomic_modify fails
                pass

        # Fallback: scan event files (for initial state or error cases)
        return self._get_next_event_id_from_files()

    def _get_next_event_id_from_files(self) -> str:
        """Fallback: get next event ID by scanning event files."""
        if not self.events_dir.exists():
            return "000001"

        max_num = 0
        for event_file in self.events_dir.glob("*.json"):
            try:
                # Event files are named: {number}-{timestamp}-{type}.json
                num_str = event_file.name.split("-")[0]
                num = int(num_str)
                if num > max_num:
                    max_num = num
            except (ValueError, IndexError):
                continue

        return f"{max_num + 1:06d}"

    # =========================================================================
    # State Management
    # =========================================================================

    def load_state(self) -> C4State:
        """Load state from store"""
        from .store import StateNotFoundError

        try:
            self._state = self._store.load("")  # project_id from state
            self._project_id = self._state.project_id  # Cache for atomic operations
        except StateNotFoundError:
            raise FileNotFoundError(f"State file not found: {self.c4_dir / 'state.json'}")
        return self._state

    def save_state(self) -> None:
        """Save state to store (flush immediately after every transition)"""
        if self._state is None:
            raise ValueError("No state to save")

        self._store.save(self._state)  # Store handles updated_at

    def initialize_state(self, project_id: str) -> C4State:
        """Initialize new state for a project"""
        self._state = C4State(project_id=project_id)
        self._project_id = project_id  # Cache for atomic operations
        self.c4_dir.mkdir(parents=True, exist_ok=True)
        self.events_dir.mkdir(exist_ok=True)
        self.save_state()
        return self._state

    @property
    def state(self) -> C4State:
        """Get current state, loading if necessary"""
        if self._state is None:
            self.load_state()
        return self._state  # type: ignore

    @property
    def store(self) -> "StateStore":
        """Get the underlying state store"""
        return self._store

    # =========================================================================
    # State Transitions
    # =========================================================================

    def can_transition(self, event: str) -> bool:
        """Check if a transition is valid from current state"""
        key = (self.state.status, event)
        return key in TRANSITIONS

    def transition(self, event: str, actor: str = "c4d", data: dict | None = None) -> ProjectStatus:
        """
        Execute a state transition.
        Raises StateTransitionError if transition is invalid.
        """
        current = self.state.status
        key = (current, event)

        if key not in TRANSITIONS:
            raise StateTransitionError(
                f"Invalid transition: {current.value} --[{event}]--> ? "
                f"(allowed events: {self._get_allowed_events(current)})"
            )

        new_status = TRANSITIONS[key]

        # Check invariants before transition
        self._check_invariants_before(event, new_status)

        # Execute transition
        old_status = self.state.status
        self.state.status = new_status

        # Update execution mode if entering EXECUTE
        if new_status == ProjectStatus.EXECUTE:
            self.state.execution_mode = ExecutionMode.RUNNING

        # Clear execution mode if leaving EXECUTE
        if old_status == ProjectStatus.EXECUTE and new_status != ProjectStatus.EXECUTE:
            self.state.execution_mode = None

        # Emit event
        self._emit_event(
            EventType.STATE_CHANGED,
            actor,
            {
                "from": old_status.value,
                "to": new_status.value,
                "trigger": event,
                **(data or {}),
            },
        )

        # Save state immediately (invariant: flush after every transition)
        self.save_state()

        return new_status

    def _get_allowed_events(self, status: ProjectStatus) -> list[str]:
        """Get list of allowed events for a status"""
        return [event for (s, event), _ in TRANSITIONS.items() if s == status]

    # =========================================================================
    # Command Validation
    # =========================================================================

    def is_command_allowed(self, command: str) -> bool:
        """Check if a CLI command is allowed in current state"""
        allowed = ALLOWED_COMMANDS.get(self.state.status, [])
        return command in allowed

    def get_allowed_commands(self) -> list[str]:
        """Get list of allowed commands for current state"""
        return ALLOWED_COMMANDS.get(self.state.status, [])

    # =========================================================================
    # Invariant Checking
    # =========================================================================

    def _check_invariants_before(self, event: str, new_status: ProjectStatus) -> None:
        """Check invariants before a transition"""
        # Invariant 1: CHECKPOINT 중에는 워커 실행 금지
        if self.state.status == ProjectStatus.CHECKPOINT:
            if self.state.queue.in_progress:
                raise InvariantViolationError(
                    "Cannot have workers running during CHECKPOINT. "
                    f"In progress: {self.state.queue.in_progress}"
                )

    def check_invariants(self) -> list[str]:
        """
        Check all invariants and return list of violations.
        Returns empty list if all invariants are satisfied.
        """
        violations = []

        # Invariant 1: EXECUTE 중에는 docs/PLAN.md 수정 금지
        # (This is enforced at runtime, not checkable here)

        # Invariant 2: CHECKPOINT 중에는 워커 실행 금지
        if self.state.status == ProjectStatus.CHECKPOINT:
            if self.state.queue.in_progress:
                violations.append(
                    f"Workers running during CHECKPOINT: {self.state.queue.in_progress}"
                )

        # Invariant 3: leader.lock은 EXECUTE/CHECKPOINT에서만 유지
        if self.state.locks.leader is not None:
            if self.state.status not in [ProjectStatus.EXECUTE, ProjectStatus.CHECKPOINT]:
                violations.append(
                    f"Leader lock held in state {self.state.status.value} "
                    "(should only be held in EXECUTE/CHECKPOINT)"
                )

        return violations

    # =========================================================================
    # Event Logging
    # =========================================================================

    def _emit_event(self, event_type: EventType, actor: str, data: dict) -> Event:
        """Emit an event to the event log"""
        # Get next event ID atomically (prevents race conditions)
        event_id = self._get_next_event_id()

        event = Event(
            id=event_id,
            type=event_type,
            actor=actor,
            data=data,
        )

        # Write event to file
        ts_str = event.ts.strftime("%Y%m%dT%H%M%S")
        filename = f"{event_id}-{ts_str}-{event_type.value}.json"
        event_file = self.events_dir / filename
        event_file.write_text(event.model_dump_json(indent=2))

        # Note: metrics.events_emitted is already updated in _get_next_event_id()
        # via atomic_modify for all stores.

        return event

    def emit_event(self, event_type: EventType, actor: str, data: dict) -> Event:
        """Public method to emit an event"""
        return self._emit_event(event_type, actor, data)

    # =========================================================================
    # Checkpoint Management
    # =========================================================================

    def check_gate_conditions(self, config: C4Config) -> str | None:
        """
        Check if any checkpoint gate conditions are met.
        Returns checkpoint ID if conditions met, None otherwise.
        Skips checkpoints that have already been passed.
        """
        for cp_config in config.checkpoints:
            # Skip already passed checkpoints
            if cp_config.id in self.state.passed_checkpoints:
                continue
            if self._is_checkpoint_satisfied(cp_config):
                return cp_config.id
        return None

    def _is_checkpoint_satisfied(self, cp_config: CheckpointConfig) -> bool:
        """Check if a specific checkpoint's conditions are satisfied"""
        # Check required tasks
        for task_id in cp_config.required_tasks:
            if task_id not in self.state.queue.done:
                return False

        # Check required validations
        if self.state.last_validation is None:
            return False

        for validation in cp_config.required_validations:
            if self.state.last_validation.get(validation) != "pass":
                return False

        return True

    def enter_checkpoint(self, checkpoint_id: str) -> None:
        """Enter checkpoint state"""
        self.state.checkpoint = CheckpointState(
            current=checkpoint_id,
            state="in_progress",
        )
        self.transition("gate_reached", data={"checkpoint": checkpoint_id})

    # =========================================================================
    # Execution Mode Management
    # =========================================================================

    def set_execution_mode(self, mode: ExecutionMode) -> None:
        """Set execution mode (only valid in EXECUTE state)"""
        if self.state.status != ProjectStatus.EXECUTE:
            raise StateTransitionError(
                f"Cannot set execution mode in state {self.state.status.value}"
            )
        self.state.execution_mode = mode
        self.save_state()
