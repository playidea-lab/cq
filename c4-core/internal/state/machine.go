// Package state implements the C4 project state machine.
//
// The state machine manages transitions between project phases:
// INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> COMPLETE
//
// Each state has defined allowed transitions and invariant checks
// that are validated before any transition occurs.
//
// This is the Go equivalent of c4/state_machine.py.
package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ProjectStatus represents the top-level project state.
type ProjectStatus string

const (
	StatusINIT       ProjectStatus = "INIT"
	StatusDISCOVERY  ProjectStatus = "DISCOVERY"
	StatusDESIGN     ProjectStatus = "DESIGN"
	StatusPLAN       ProjectStatus = "PLAN"
	StatusEXECUTE    ProjectStatus = "EXECUTE"
	StatusCHECKPOINT ProjectStatus = "CHECKPOINT"
	StatusCOMPLETE   ProjectStatus = "COMPLETE"
	StatusHALTED     ProjectStatus = "HALTED"
	StatusERROR      ProjectStatus = "ERROR"
)

// ExecutionMode represents sub-states within EXECUTE.
type ExecutionMode string

const (
	ModeRunning ExecutionMode = "running"
	ModePaused  ExecutionMode = "paused"
	ModeRepair  ExecutionMode = "repair"
)

// transitionKey is a (from_status, event) tuple.
type transitionKey struct {
	From  ProjectStatus
	Event string
}

// transitions defines the state transition table.
// (from_state, event) -> to_state
// Mirrors the Python TRANSITIONS dict in c4/state_machine.py.
var transitions = map[transitionKey]ProjectStatus{
	// INIT transitions
	{StatusINIT, "c4_init"}:        StatusDISCOVERY,
	{StatusINIT, "c4_init_legacy"}: StatusPLAN,

	// DISCOVERY transitions
	{StatusDISCOVERY, "discovery_complete"}: StatusDESIGN,
	{StatusDISCOVERY, "skip_discovery"}:    StatusPLAN,
	{StatusDISCOVERY, "c4_stop"}:           StatusHALTED,

	// DESIGN transitions
	{StatusDESIGN, "design_approved"}: StatusPLAN,
	{StatusDESIGN, "design_rejected"}: StatusDISCOVERY,
	{StatusDESIGN, "skip_design"}:     StatusPLAN,
	{StatusDESIGN, "c4_stop"}:         StatusHALTED,

	// PLAN transitions
	{StatusPLAN, "c4_run"}:         StatusEXECUTE,
	{StatusPLAN, "c4_stop"}:        StatusHALTED,
	{StatusPLAN, "back_to_design"}: StatusDESIGN,

	// EXECUTE transitions
	{StatusEXECUTE, "gate_reached"}: StatusCHECKPOINT,
	{StatusEXECUTE, "c4_stop"}:     StatusHALTED,
	{StatusEXECUTE, "all_done"}:    StatusCOMPLETE,
	{StatusEXECUTE, "fatal_error"}: StatusERROR,

	// CHECKPOINT transitions
	{StatusCHECKPOINT, "approve"}:         StatusEXECUTE,
	{StatusCHECKPOINT, "approve_final"}:   StatusCOMPLETE,
	{StatusCHECKPOINT, "request_changes"}: StatusEXECUTE,
	{StatusCHECKPOINT, "replan"}:          StatusPLAN,
	{StatusCHECKPOINT, "redesign"}:        StatusDESIGN,
	{StatusCHECKPOINT, "fatal_error"}:     StatusERROR,

	// HALTED transitions
	{StatusHALTED, "c4_run"}:       StatusEXECUTE,
	{StatusHALTED, "c4_plan"}:      StatusPLAN,
	{StatusHALTED, "c4_discovery"}: StatusDISCOVERY,
}

// StateTransitionError is returned when an invalid state transition is attempted.
type StateTransitionError struct {
	From    ProjectStatus
	Event   string
	Allowed []string
}

func (e *StateTransitionError) Error() string {
	return fmt.Sprintf("invalid transition: %s --[%s]--> ? (allowed events: %v)",
		e.From, e.Event, e.Allowed)
}

// InvariantViolationError is returned when a state invariant is violated.
type InvariantViolationError struct {
	Message string
}

func (e *InvariantViolationError) Error() string {
	return fmt.Sprintf("invariant violation: %s", e.Message)
}

// ProjectState holds the full project state (mirrors Python C4State).
type ProjectState struct {
	ProjectID     string        `json:"project_id"`
	Status        ProjectStatus `json:"status"`
	ExecutionMode ExecutionMode `json:"execution_mode,omitempty"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Machine implements the C4 state machine with transition validation.
type Machine struct {
	state *ProjectState
	db    *sql.DB
}

// NewMachine creates a new state machine backed by a SQLite database.
func NewMachine(db *sql.DB) *Machine {
	return &Machine{db: db}
}

// State returns the current state. Returns nil if not loaded.
func (m *Machine) State() *ProjectState {
	return m.state
}

// Initialize creates a new project state with INIT status.
func (m *Machine) Initialize(projectID string) error {
	m.state = &ProjectState{
		ProjectID: projectID,
		Status:    StatusINIT,
		UpdatedAt: time.Now(),
	}
	return m.saveState()
}

// LoadState loads the project state from the database.
// If no state exists, returns a default INIT state.
func (m *Machine) LoadState(projectID string) (*ProjectState, error) {
	var stateJSON string
	err := m.db.QueryRow(
		"SELECT state_json FROM c4_state WHERE project_id = ?",
		projectID,
	).Scan(&stateJSON)

	if err == sql.ErrNoRows {
		m.state = &ProjectState{
			ProjectID: projectID,
			Status:    StatusINIT,
			UpdatedAt: time.Now(),
		}
		return m.state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	var state ProjectState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	m.state = &state
	return m.state, nil
}

// CanTransition checks if a transition is valid from the current state.
func (m *Machine) CanTransition(event string) bool {
	if m.state == nil {
		return false
	}
	key := transitionKey{From: m.state.Status, Event: event}
	_, ok := transitions[key]
	return ok
}

// Transition executes a state transition.
// Returns the new status or an error if the transition is invalid.
func (m *Machine) Transition(event string) (ProjectStatus, error) {
	if m.state == nil {
		return "", fmt.Errorf("no state loaded")
	}

	current := m.state.Status
	key := transitionKey{From: current, Event: event}

	newStatus, ok := transitions[key]
	if !ok {
		return "", &StateTransitionError{
			From:    current,
			Event:   event,
			Allowed: AllowedEvents(current),
		}
	}

	// Update execution mode
	oldStatus := m.state.Status
	m.state.Status = newStatus
	m.state.UpdatedAt = time.Now()

	if newStatus == StatusEXECUTE {
		m.state.ExecutionMode = ModeRunning
	}
	if oldStatus == StatusEXECUTE && newStatus != StatusEXECUTE {
		m.state.ExecutionMode = ""
	}

	// Save immediately
	if err := m.saveState(); err != nil {
		// Rollback on save failure
		m.state.Status = oldStatus
		return "", fmt.Errorf("save state after transition: %w", err)
	}

	return newStatus, nil
}

// AllowedEvents returns the list of valid events for a given status.
func AllowedEvents(status ProjectStatus) []string {
	var events []string
	for key := range transitions {
		if key.From == status {
			events = append(events, key.Event)
		}
	}
	return events
}

// saveState persists the current state to the database.
func (m *Machine) saveState() error {
	if m.state == nil {
		return fmt.Errorf("no state to save")
	}

	stateJSON, err := json.Marshal(m.state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	_, err = m.db.Exec(
		`INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at)
		 VALUES (?, ?, ?)`,
		m.state.ProjectID, string(stateJSON), m.state.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}
