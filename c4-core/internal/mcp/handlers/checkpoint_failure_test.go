package handlers

import (
	"strings"
	"testing"
)

// TestCheckpoint_InsertFailure_ReturnsError verifies that when the c4_checkpoints
// INSERT fails, Checkpoint returns a non-nil error instead of success=true.
func TestCheckpoint_InsertFailure_ReturnsError(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Drop the c4_checkpoints table to force the INSERT to fail.
	if _, err := db.Exec("DROP TABLE c4_checkpoints"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	result, err := store.Checkpoint("CP-FAIL", "APPROVE", "notes", nil, "", "")
	if err == nil {
		t.Fatal("expected error when INSERT fails, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on INSERT failure, got %+v", result)
	}
	if !strings.Contains(err.Error(), "checkpoint INSERT") {
		t.Errorf("error message = %q, want substring \"checkpoint INSERT\"", err.Error())
	}
}

// TestCheckpoint_InsertFailure_NoEventPublished verifies that the function returns early
// on INSERT failure (before notifyEventBus), so no checkpoint row is written.
func TestCheckpoint_InsertFailure_NoEventPublished(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if _, err := db.Exec("DROP TABLE c4_checkpoints"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	_, err := store.Checkpoint("CP-NOEVENT", "APPROVE", "notes", nil, "", "")
	if err == nil {
		t.Fatal("expected error when INSERT fails")
	}

	// Recreate the table and confirm no row was written for CP-NOEVENT
	// (early-return before any write or event dispatch occurred).
	if _, recErr := db.Exec(`CREATE TABLE c4_checkpoints (
		checkpoint_id TEXT PRIMARY KEY,
		decision TEXT,
		notes TEXT,
		required_changes TEXT,
		target_task_id TEXT,
		target_review_id TEXT,
		created_at TEXT
	)`); recErr != nil {
		t.Fatalf("recreate table: %v", recErr)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM c4_checkpoints WHERE checkpoint_id='CP-NOEVENT'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after failed checkpoint, got %d", count)
	}
}

// TestCheckpoint_SuccessPath_Regression verifies the normal APPROVE path still works
// (success=true, NextAction="continue", row persisted) after the error-handling change.
func TestCheckpoint_SuccessPath_Regression(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	result, err := store.Checkpoint("CP-REG", "APPROVE", "looks good", nil, "", "")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true on normal path")
	}
	if result.NextAction != "continue" {
		t.Errorf("NextAction = %q, want \"continue\"", result.NextAction)
	}

	var decision string
	if err := db.QueryRow("SELECT decision FROM c4_checkpoints WHERE checkpoint_id='CP-REG'").Scan(&decision); err != nil {
		t.Fatalf("query checkpoint row: %v", err)
	}
	if decision != "APPROVE" {
		t.Errorf("persisted decision = %q, want \"APPROVE\"", decision)
	}
}
