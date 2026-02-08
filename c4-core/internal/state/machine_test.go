package state

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary SQLite database with the c4_state table.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS c4_state (
			project_id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// TestStateTransitionINITtoDISCOVERY verifies INIT -> DISCOVERY via c4_init.
func TestStateTransitionINITtoDISCOVERY(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)

	if err := m.Initialize("test-project"); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if m.State().Status != StatusINIT {
		t.Fatalf("expected INIT, got %s", m.State().Status)
	}

	newStatus, err := m.Transition("c4_init")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusDISCOVERY {
		t.Errorf("expected DISCOVERY, got %s", newStatus)
	}
}

// TestStateTransitionINITtoPLAN verifies INIT -> PLAN via c4_init_legacy.
func TestStateTransitionINITtoPLAN(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")

	newStatus, err := m.Transition("c4_init_legacy")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusPLAN {
		t.Errorf("expected PLAN, got %s", newStatus)
	}
}

// TestStateTransitionFullWorkflow tests the full INIT->DISCOVERY->DESIGN->PLAN->EXECUTE->CHECKPOINT->COMPLETE path.
func TestStateTransitionFullWorkflow(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")

	steps := []struct {
		event    string
		expected ProjectStatus
	}{
		{"c4_init", StatusDISCOVERY},
		{"discovery_complete", StatusDESIGN},
		{"design_approved", StatusPLAN},
		{"c4_run", StatusEXECUTE},
		{"gate_reached", StatusCHECKPOINT},
		{"approve_final", StatusCOMPLETE},
	}

	for _, step := range steps {
		newStatus, err := m.Transition(step.event)
		if err != nil {
			t.Fatalf("transition %q: %v", step.event, err)
		}
		if newStatus != step.expected {
			t.Errorf("after %q: expected %s, got %s", step.event, step.expected, newStatus)
		}
	}
}

// TestStateTransitionPLANtoEXECUTE verifies PLAN -> EXECUTE via c4_run.
func TestStateTransitionPLANtoEXECUTE(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy") // INIT -> PLAN

	newStatus, err := m.Transition("c4_run")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusEXECUTE {
		t.Errorf("expected EXECUTE, got %s", newStatus)
	}
	if m.State().ExecutionMode != ModeRunning {
		t.Errorf("expected execution_mode 'running', got %q", m.State().ExecutionMode)
	}
}

// TestStateTransitionEXECUTEtoCHECKPOINT verifies EXECUTE -> CHECKPOINT.
func TestStateTransitionEXECUTEtoCHECKPOINT(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")

	newStatus, err := m.Transition("gate_reached")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusCHECKPOINT {
		t.Errorf("expected CHECKPOINT, got %s", newStatus)
	}
	// ExecutionMode should be cleared when leaving EXECUTE
	if m.State().ExecutionMode != "" {
		t.Errorf("expected empty execution_mode, got %q", m.State().ExecutionMode)
	}
}

// TestStateTransitionCHECKPOINTtoCOMPLETE verifies CHECKPOINT -> COMPLETE via approve_final.
func TestStateTransitionCHECKPOINTtoCOMPLETE(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("gate_reached")

	newStatus, err := m.Transition("approve_final")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusCOMPLETE {
		t.Errorf("expected COMPLETE, got %s", newStatus)
	}
}

// TestStateTransitionCHECKPOINTtoEXECUTE verifies CHECKPOINT -> EXECUTE via approve.
func TestStateTransitionCHECKPOINTtoEXECUTE(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("gate_reached")

	newStatus, err := m.Transition("approve")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusEXECUTE {
		t.Errorf("expected EXECUTE, got %s", newStatus)
	}
	if m.State().ExecutionMode != ModeRunning {
		t.Errorf("expected execution_mode 'running', got %q", m.State().ExecutionMode)
	}
}

// TestStateTransitionEXECUTEtoHALTED verifies EXECUTE -> HALTED via c4_stop.
func TestStateTransitionEXECUTEtoHALTED(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")

	newStatus, err := m.Transition("c4_stop")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusHALTED {
		t.Errorf("expected HALTED, got %s", newStatus)
	}
}

// TestStateTransitionHALTEDtoEXECUTE verifies HALTED -> EXECUTE via c4_run.
func TestStateTransitionHALTEDtoEXECUTE(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("c4_stop") // HALTED

	newStatus, err := m.Transition("c4_run")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusEXECUTE {
		t.Errorf("expected EXECUTE, got %s", newStatus)
	}
}

// TestStateTransitionEXECUTEtoCOMPLETE verifies EXECUTE -> COMPLETE via all_done.
func TestStateTransitionEXECUTEtoCOMPLETE(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")

	newStatus, err := m.Transition("all_done")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusCOMPLETE {
		t.Errorf("expected COMPLETE, got %s", newStatus)
	}
}

// TestStateTransitionInvalidEvent verifies that an invalid event returns an error.
func TestStateTransitionInvalidEvent(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")

	_, err := m.Transition("nonexistent_event")
	if err == nil {
		t.Fatal("expected error for invalid event")
	}

	sterr, ok := err.(*StateTransitionError)
	if !ok {
		t.Fatalf("expected StateTransitionError, got %T", err)
	}
	if sterr.From != StatusINIT {
		t.Errorf("expected from INIT, got %s", sterr.From)
	}
	if sterr.Event != "nonexistent_event" {
		t.Errorf("expected event nonexistent_event, got %s", sterr.Event)
	}
}

// TestStateTransitionCOMPLETEHasNoTransitions verifies COMPLETE is terminal.
func TestStateTransitionCOMPLETEHasNoTransitions(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("all_done") // COMPLETE

	events := AllowedEvents(StatusCOMPLETE)
	if len(events) != 0 {
		t.Errorf("COMPLETE should have no transitions, got %v", events)
	}

	// Verify any transition attempt fails
	_, err := m.Transition("c4_run")
	if err == nil {
		t.Fatal("expected error for transition from COMPLETE")
	}
}

// TestStateTransitionERRORHasNoTransitions verifies ERROR is terminal.
func TestStateTransitionERRORHasNoTransitions(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("fatal_error") // ERROR

	events := AllowedEvents(StatusERROR)
	if len(events) != 0 {
		t.Errorf("ERROR should have no transitions, got %v", events)
	}
}

// TestCanTransition verifies the CanTransition helper.
func TestCanTransition(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")

	if !m.CanTransition("c4_init") {
		t.Error("should be able to transition INIT -> DISCOVERY")
	}
	if m.CanTransition("c4_run") {
		t.Error("should not be able to transition INIT -> EXECUTE")
	}
}

// TestLoadStatePersistence verifies that state persists across Machine instances.
func TestLoadStatePersistence(t *testing.T) {
	db := setupTestDB(t)

	// Create and initialize with Machine 1
	m1 := NewMachine(db)
	m1.Initialize("test-project")
	m1.Transition("c4_init_legacy") // PLAN
	m1.Transition("c4_run")         // EXECUTE

	// Load with Machine 2
	m2 := NewMachine(db)
	state, err := m2.LoadState("test-project")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Status != StatusEXECUTE {
		t.Errorf("expected EXECUTE, got %s", state.Status)
	}
	if state.ProjectID != "test-project" {
		t.Errorf("expected project_id test-project, got %s", state.ProjectID)
	}
}

// TestLoadStateNoRows verifies default INIT state when no rows exist.
func TestLoadStateNoRows(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)

	state, err := m.LoadState("nonexistent")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Status != StatusINIT {
		t.Errorf("expected INIT for missing project, got %s", state.Status)
	}
}

// TestStatePythonCompatibility verifies that Go can read Python-formatted state JSON.
func TestStatePythonCompatibility(t *testing.T) {
	db := setupTestDB(t)

	// Insert state in the same format Python writes it
	pythonJSON := `{
		"project_id": "my-project",
		"status": "EXECUTE",
		"execution_mode": "running",
		"updated_at": "2026-01-15T10:30:00Z"
	}`
	_, err := db.Exec(
		"INSERT INTO c4_state (project_id, state_json) VALUES (?, ?)",
		"my-project", pythonJSON,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := NewMachine(db)
	state, err := m.LoadState("my-project")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if state.ProjectID != "my-project" {
		t.Errorf("project_id = %q, want my-project", state.ProjectID)
	}
	if state.Status != StatusEXECUTE {
		t.Errorf("status = %q, want EXECUTE", state.Status)
	}
	if state.ExecutionMode != ModeRunning {
		t.Errorf("execution_mode = %q, want running", state.ExecutionMode)
	}
}

// TestAllowedEvents verifies AllowedEvents for various states.
func TestAllowedEvents(t *testing.T) {
	initEvents := AllowedEvents(StatusINIT)
	if len(initEvents) < 1 {
		t.Errorf("INIT should have at least 1 event, got %d", len(initEvents))
	}

	planEvents := AllowedEvents(StatusPLAN)
	found := false
	for _, e := range planEvents {
		if e == "c4_run" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PLAN should allow c4_run, got %v", planEvents)
	}
}

// TestStateTransitionReplan verifies CHECKPOINT -> PLAN via replan.
func TestStateTransitionReplan(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("gate_reached")

	newStatus, err := m.Transition("replan")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusPLAN {
		t.Errorf("expected PLAN, got %s", newStatus)
	}
}

// TestStateTransitionRequestChanges verifies CHECKPOINT -> EXECUTE via request_changes.
func TestStateTransitionRequestChanges(t *testing.T) {
	db := setupTestDB(t)
	m := NewMachine(db)
	m.Initialize("test-project")
	m.Transition("c4_init_legacy")
	m.Transition("c4_run")
	m.Transition("gate_reached")

	newStatus, err := m.Transition("request_changes")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if newStatus != StatusEXECUTE {
		t.Errorf("expected EXECUTE, got %s", newStatus)
	}
}

// TestStateDbReadWrite verifies that Go can read/write .c4/c4.db format.
func TestStateDbReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dbFile := filepath.Join(c4Dir, "c4.db")
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Enable WAL mode like Python does
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=30000")

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS c4_state (
			project_id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	m := NewMachine(db)
	if err := m.Initialize("rw-test"); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Transition through states
	m.Transition("c4_init")
	m.Transition("discovery_complete")
	m.Transition("design_approved")
	m.Transition("c4_run")

	// Re-open and verify
	m2 := NewMachine(db)
	state, err := m2.LoadState("rw-test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if state.Status != StatusEXECUTE {
		t.Errorf("expected EXECUTE after full workflow, got %s", state.Status)
	}
}
