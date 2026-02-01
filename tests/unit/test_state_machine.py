"""Tests for C4D State Machine"""

import tempfile
from pathlib import Path

import pytest

from c4.models import ExecutionMode, ProjectStatus
from c4.state_machine import (
    ALLOWED_COMMANDS,
    TRANSITIONS,
    StateMachine,
    StateTransitionError,
)


@pytest.fixture
def temp_c4_dir():
    """Create a temporary .c4 directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        c4_dir = Path(tmpdir) / ".c4"
        c4_dir.mkdir()
        (c4_dir / "events").mkdir()
        yield c4_dir


@pytest.fixture
def state_machine_init(temp_c4_dir):
    """Create a state machine in INIT state"""
    sm = StateMachine(temp_c4_dir)
    sm.initialize_state("test-project")
    return sm


@pytest.fixture
def state_machine_discovery(temp_c4_dir):
    """Create a state machine in DISCOVERY state"""
    sm = StateMachine(temp_c4_dir)
    sm.initialize_state("test-project")
    sm.transition("c4_init")  # INIT → DISCOVERY
    return sm


@pytest.fixture
def state_machine(temp_c4_dir):
    """Create a state machine in PLAN state (most common starting point)"""
    sm = StateMachine(temp_c4_dir)
    sm.initialize_state("test-project")
    sm.transition("c4_init")  # INIT → DISCOVERY
    sm.transition("skip_discovery")  # DISCOVERY → PLAN (skip for tests)
    return sm


class TestStateTransitions:
    """Test state transition logic"""

    def test_init_to_discovery(self, state_machine_init):
        """Test INIT → DISCOVERY transition"""
        assert state_machine_init.state.status == ProjectStatus.INIT
        state_machine_init.transition("c4_init")
        assert state_machine_init.state.status == ProjectStatus.DISCOVERY

    def test_discovery_to_plan(self, state_machine_discovery):
        """Test DISCOVERY → PLAN transition (skip_discovery)"""
        assert state_machine_discovery.state.status == ProjectStatus.DISCOVERY
        state_machine_discovery.transition("skip_discovery")
        assert state_machine_discovery.state.status == ProjectStatus.PLAN

    def test_plan_to_execute(self, state_machine):
        """Test PLAN → EXECUTE transition"""
        state_machine.transition("c4_run")
        assert state_machine.state.status == ProjectStatus.EXECUTE
        assert state_machine.state.execution_mode == ExecutionMode.RUNNING

    def test_execute_to_halted(self, state_machine):
        """Test EXECUTE → HALTED transition"""
        state_machine.transition("c4_run")  # PLAN → EXECUTE
        state_machine.transition("c4_stop")  # EXECUTE → HALTED
        assert state_machine.state.status == ProjectStatus.HALTED

    def test_halted_to_execute(self, state_machine):
        """Test HALTED → EXECUTE transition"""
        state_machine.transition("c4_run")  # PLAN → EXECUTE
        state_machine.transition("c4_stop")  # EXECUTE → HALTED
        state_machine.transition("c4_run")  # HALTED → EXECUTE
        assert state_machine.state.status == ProjectStatus.EXECUTE

    def test_invalid_transition_raises(self, state_machine):
        """Test that invalid transitions raise StateTransitionError"""
        # Try to run from PLAN to COMPLETE (invalid)
        with pytest.raises(StateTransitionError):
            state_machine.transition("approve")  # Not valid from PLAN

    def test_transition_persistence(self, state_machine, temp_c4_dir):
        """Test that transitions are persisted to state.json"""
        state_machine.transition("c4_run")

        # Load fresh state machine and check (must provide project_id on first load)
        sm2 = StateMachine(temp_c4_dir)
        sm2.load_state("test-project")
        assert sm2.state.status == ProjectStatus.EXECUTE


class TestCommandValidation:
    """Test command validation per state"""

    def test_init_allowed_commands(self):
        """Test allowed commands in INIT state"""
        assert ALLOWED_COMMANDS[ProjectStatus.INIT] == ["init"]

    def test_plan_allowed_commands(self):
        """Test allowed commands in PLAN state"""
        assert "run" in ALLOWED_COMMANDS[ProjectStatus.PLAN]
        assert "stop" in ALLOWED_COMMANDS[ProjectStatus.PLAN]
        assert "status" in ALLOWED_COMMANDS[ProjectStatus.PLAN]

    def test_execute_allowed_commands(self):
        """Test allowed commands in EXECUTE state"""
        assert "worker join" in ALLOWED_COMMANDS[ProjectStatus.EXECUTE]
        assert "stop" in ALLOWED_COMMANDS[ProjectStatus.EXECUTE]
        assert "init" not in ALLOWED_COMMANDS[ProjectStatus.EXECUTE]

    def test_is_command_allowed(self, state_machine):
        """Test is_command_allowed method"""
        # In PLAN state
        assert state_machine.is_command_allowed("run")
        assert state_machine.is_command_allowed("status")
        assert not state_machine.is_command_allowed("worker join")


class TestEventLogging:
    """Test event logging"""

    def test_transition_emits_event(self, state_machine, temp_c4_dir):
        """Test that transitions emit STATE_CHANGED events"""
        events_dir = temp_c4_dir / "events"

        # Count initial events
        initial_count = len(list(events_dir.glob("*.json")))

        # Make a transition
        state_machine.transition("c4_run")

        # Check event was created
        new_count = len(list(events_dir.glob("*.json")))
        assert new_count > initial_count

    def test_event_file_format(self, state_machine, temp_c4_dir):
        """Test event file naming format"""
        state_machine.transition("c4_run")

        events_dir = temp_c4_dir / "events"
        event_files = list(events_dir.glob("*STATE_CHANGED*.json"))

        assert len(event_files) > 0
        # Check format: NNNNNN-YYYYMMDDTHHMMSS-TYPE.json
        filename = event_files[0].name
        assert filename.startswith("0")  # Sequential ID
        assert "STATE_CHANGED" in filename


class TestInvariants:
    """Test invariant checking"""

    def test_check_invariants_empty_on_valid_state(self, state_machine):
        """Test that check_invariants returns empty list for valid state"""
        violations = state_machine.check_invariants()
        assert violations == []


class TestTransitionTable:
    """Test transition table completeness"""

    def test_all_states_have_transitions(self):
        """Test that all states (except COMPLETE/ERROR) have outgoing transitions"""
        states_with_transitions = set(s for s, _ in TRANSITIONS.keys())

        for status in ProjectStatus:
            if status in [ProjectStatus.COMPLETE, ProjectStatus.ERROR]:
                continue  # These are terminal states
            assert status in states_with_transitions, f"{status} has no transitions"

    def test_all_states_have_allowed_commands(self):
        """Test that all states have defined allowed commands"""
        for status in ProjectStatus:
            assert status in ALLOWED_COMMANDS, f"{status} missing from ALLOWED_COMMANDS"


class TestLoadState:
    """Test load_state validation"""

    def test_load_state_empty_project_id_raises_error(self, temp_c4_dir):
        """Test that load_state with empty project_id raises ValueError"""
        sm = StateMachine(temp_c4_dir)
        with pytest.raises(ValueError, match="project_id must be provided"):
            sm.load_state("")

    def test_load_state_whitespace_project_id_raises_error(self, temp_c4_dir):
        """Test that load_state with whitespace-only project_id raises ValueError"""
        sm = StateMachine(temp_c4_dir)
        with pytest.raises(ValueError, match="project_id must be provided"):
            sm.load_state("   ")

    def test_load_state_none_without_cache_raises_error(self, temp_c4_dir):
        """Test that load_state(None) raises error when no cached project_id"""
        sm = StateMachine(temp_c4_dir)
        with pytest.raises(ValueError, match="project_id must be provided"):
            sm.load_state(None)

    def test_load_state_uses_cached_project_id(self, temp_c4_dir):
        """Test that load_state(None) uses cached project_id after first call"""
        sm = StateMachine(temp_c4_dir)
        sm.initialize_state("test-project")
        sm.save_state()

        # Create new state machine and load
        sm2 = StateMachine(temp_c4_dir)
        sm2.load_state("test-project")

        # Now load_state(None) should use cached project_id
        sm2.load_state(None)  # Should not raise
        assert sm2.state.project_id == "test-project"

    def test_load_state_with_explicit_project_id(self, temp_c4_dir):
        """Test that load_state with explicit project_id works"""
        sm = StateMachine(temp_c4_dir)
        sm.initialize_state("my-project")
        sm.save_state()

        sm2 = StateMachine(temp_c4_dir)
        sm2.load_state("my-project")
        assert sm2.state.project_id == "my-project"
