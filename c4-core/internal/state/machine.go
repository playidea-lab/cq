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
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// ErrStateChanged is returned by Transition when the DB state differs from
// the in-memory expected state at the moment the IMMEDIATE lock is acquired.
// Callers may retry after re-loading state via LoadState.
var ErrStateChanged = errors.New("state changed by concurrent writer")

// ErrInvalidTransition is returned by Transition when the current DB state
// does not allow the requested event. This is non-retryable.
var ErrInvalidTransition = errors.New("invalid transition from current state")

// ErrDatabase is returned by Transition on transient DB errors. Callers may retry.
var ErrDatabase = errors.New("database error")

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

// Transition executes a state transition using a BEGIN IMMEDIATE transaction.
//
// Concurrency: acquires a write-reservation lock (BEGIN IMMEDIATE) before
// re-reading the DB state. This prevents last-write-wins races when multiple
// Claude Code sessions call Transition concurrently on the same project.
//
// Error classification:
//   - ErrStateChanged:      DB state != in-memory expected state → caller should
//     call LoadState and retry
//   - ErrInvalidTransition: event not allowed from current DB state → non-retryable
//   - ErrDatabase:          transient DB error → retry
func (m *Machine) Transition(event string) (ProjectStatus, error) {
	if m.state == nil {
		return "", fmt.Errorf("no state loaded")
	}

	expectedStatus := m.state.Status
	projectID := m.state.ProjectID

	ctx := context.Background()
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: get connection: %v", ErrDatabase, err)
	}
	defer conn.Close()

	// BEGIN IMMEDIATE acquires a write-reservation lock immediately.
	// busy_timeout (if set on the DB) causes concurrent callers to wait
	// rather than fail immediately, serialising transitions cleanly.
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", fmt.Errorf("%w: begin immediate: %v", ErrDatabase, err)
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		}
	}()

	// Re-read state from DB inside the transaction to detect concurrent changes.
	var stateJSON string
	dbErr := conn.QueryRowContext(ctx,
		"SELECT state_json FROM c4_state WHERE project_id = ?",
		projectID,
	).Scan(&stateJSON)
	if dbErr != nil && dbErr != sql.ErrNoRows {
		return "", fmt.Errorf("%w: read state: %v", ErrDatabase, dbErr)
	}

	// Determine actual current DB status.
	var dbStatus ProjectStatus
	if dbErr == sql.ErrNoRows {
		dbStatus = StatusINIT
	} else {
		var dbState ProjectState
		if jsonErr := json.Unmarshal([]byte(stateJSON), &dbState); jsonErr != nil {
			return "", fmt.Errorf("%w: parse state: %v", ErrDatabase, jsonErr)
		}
		dbStatus = dbState.Status
	}

	// Guard: if DB state differs from in-memory expectation, a concurrent
	// writer has already advanced the state. Signal the caller to retry.
	if dbStatus != expectedStatus {
		return "", fmt.Errorf("%w: expected %s but DB has %s",
			ErrStateChanged, expectedStatus, dbStatus)
	}

	// Validate the transition against the actual DB state.
	key := transitionKey{From: dbStatus, Event: event}
	newStatus, ok := transitions[key]
	if !ok {
		return "", fmt.Errorf("%w: %s --[%s]--> ? (allowed: %v)",
			ErrInvalidTransition, dbStatus, event, AllowedEvents(dbStatus))
	}

	// Build new state.
	now := time.Now()
	newState := &ProjectState{
		ProjectID: projectID,
		Status:    newStatus,
		UpdatedAt: now,
	}
	if newStatus == StatusEXECUTE {
		newState.ExecutionMode = ModeRunning
	}
	// ExecutionMode stays zero-value ("") when leaving EXECUTE.

	newStateJSON, err := json.Marshal(newState)
	if err != nil {
		return "", fmt.Errorf("%w: marshal state: %v", ErrDatabase, err)
	}

	_, err = conn.ExecContext(ctx,
		`INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at)
		 VALUES (?, ?, ?)`,
		projectID, string(newStateJSON), now,
	)
	if err != nil {
		return "", fmt.Errorf("%w: save state: %v", ErrDatabase, err)
	}

	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", fmt.Errorf("%w: commit: %v", ErrDatabase, err)
	}
	committed = true

	// Update in-memory state only after a successful commit.
	m.state = newState

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

// TransitionTarget returns the target state for a (from, event) pair.
// Returns empty string if the transition is invalid.
func TransitionTarget(from ProjectStatus, event string) ProjectStatus {
	key := transitionKey{From: from, Event: event}
	target, ok := transitions[key]
	if !ok {
		return ""
	}
	return target
}

// saveState persists the current state to the database.
// Used by Initialize only (non-concurrent path, no IMMEDIATE needed).
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
