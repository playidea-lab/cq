"""C4D State Machine - State transitions and invariant enforcement"""

from datetime import datetime
from pathlib import Path

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
TRANSITIONS: dict[tuple[ProjectStatus, str], ProjectStatus] = {
    # INIT transitions
    (ProjectStatus.INIT, "c4_init"): ProjectStatus.PLAN,
    # PLAN transitions
    (ProjectStatus.PLAN, "c4_run"): ProjectStatus.EXECUTE,
    (ProjectStatus.PLAN, "c4_stop"): ProjectStatus.HALTED,
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
    (ProjectStatus.CHECKPOINT, "fatal_error"): ProjectStatus.ERROR,
    # HALTED transitions
    (ProjectStatus.HALTED, "c4_run"): ProjectStatus.EXECUTE,
    (ProjectStatus.HALTED, "c4_plan"): ProjectStatus.PLAN,
    # COMPLETE - no transitions out
    # ERROR - no transitions out (manual intervention required)
}

# Allowed CLI commands per state
ALLOWED_COMMANDS: dict[ProjectStatus, list[str]] = {
    ProjectStatus.INIT: ["init"],
    ProjectStatus.PLAN: ["plan", "run", "stop", "status"],
    ProjectStatus.EXECUTE: ["status", "worker join", "worker submit", "stop"],
    ProjectStatus.CHECKPOINT: ["status"],
    ProjectStatus.COMPLETE: ["status"],
    ProjectStatus.HALTED: ["run", "plan", "status"],
    ProjectStatus.ERROR: ["status"],
}


class StateMachine:
    """C4 State Machine with transition validation and invariant enforcement"""

    def __init__(self, c4_dir: Path):
        self.c4_dir = c4_dir
        self.state_file = c4_dir / "state.json"
        self.events_dir = c4_dir / "events"
        self._state: C4State | None = None
        self._event_counter: int = 0

    # =========================================================================
    # State Management
    # =========================================================================

    def load_state(self) -> C4State:
        """Load state from state.json"""
        if self.state_file.exists():
            import json

            data = json.loads(self.state_file.read_text())
            self._state = C4State.model_validate(data)
        else:
            raise FileNotFoundError(f"State file not found: {self.state_file}")
        return self._state

    def save_state(self) -> None:
        """Save state to state.json (flush immediately after every transition)"""
        if self._state is None:
            raise ValueError("No state to save")

        self._state.updated_at = datetime.now()
        self.state_file.write_text(self._state.model_dump_json(indent=2))

    def initialize_state(self, project_id: str) -> C4State:
        """Initialize new state for a project"""
        self._state = C4State(project_id=project_id)
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
        self._event_counter += 1
        event_id = f"{self._event_counter:06d}"

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

        # Update metrics
        self.state.metrics.events_emitted += 1

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
        """
        for cp_config in config.checkpoints:
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
