package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestSubmitTask_WithValidEvidence verifies that SubmitTask accepts handoff with valid evidence types.
func TestSubmitTask_WithValidEvidence(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec("INSERT INTO c4_tasks (task_id, title, status, worker_id, execution_mode) VALUES ('T-ev1-0','Evidence test','in_progress','worker-ev','worker')")

	handoff, _ := json.Marshal(map[string]any{
		"summary":       "completed with evidence",
		"files_changed": []string{"main.go"},
		"evidence": []map[string]string{
			{"type": "screenshot", "artifact_id": "art-001", "description": "before state"},
			{"type": "log", "artifact_id": "art-002", "description": "test output"},
			{"type": "test_result", "artifact_id": "art-003", "description": "all pass"},
		},
	})

	result, err := store.SubmitTask("T-ev1-0", "worker-ev", "abc123", string(handoff), nil)
	if err != nil {
		t.Fatalf("SubmitTask with valid evidence: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got message: %s", result.Message)
	}
}

// TestSubmitTask_WithInvalidEvidenceType verifies that SubmitTask rejects unknown evidence types.
func TestSubmitTask_WithInvalidEvidenceType(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec("INSERT INTO c4_tasks (task_id, title, status, worker_id, execution_mode) VALUES ('T-ev2-0','Evidence invalid','in_progress','worker-ev2','worker')")

	handoff, _ := json.Marshal(map[string]any{
		"summary": "completed",
		"evidence": []map[string]string{
			{"type": "video", "artifact_id": "art-004", "description": "screen recording"},
		},
	})

	_, err := store.SubmitTask("T-ev2-0", "worker-ev2", "abc456", string(handoff), nil)
	if err == nil {
		t.Fatal("expected error for invalid evidence type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid evidence type") {
		t.Errorf("error = %q, want substring \"invalid evidence type\"", err.Error())
	}
}

// TestSubmitTask_WithNoEvidence verifies backward compatibility when evidence field is absent.
func TestSubmitTask_WithNoEvidence(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec("INSERT INTO c4_tasks (task_id, title, status, worker_id, execution_mode) VALUES ('T-ev3-0','No evidence','in_progress','worker-ev3','worker')")

	handoff, _ := json.Marshal(map[string]any{
		"summary":       "done without evidence",
		"files_changed": []string{"foo.go"},
	})

	result, err := store.SubmitTask("T-ev3-0", "worker-ev3", "abc789", string(handoff), nil)
	if err != nil {
		t.Fatalf("SubmitTask without evidence: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got message: %s", result.Message)
	}
}

// TestSubmitTask_WithEmptyEvidenceArray verifies that an empty evidence array is accepted.
func TestSubmitTask_WithEmptyEvidenceArray(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec("INSERT INTO c4_tasks (task_id, title, status, worker_id, execution_mode) VALUES ('T-ev4-0','Empty evidence','in_progress','worker-ev4','worker')")

	handoff, _ := json.Marshal(map[string]any{
		"summary":  "done with empty evidence slice",
		"evidence": []map[string]string{},
	})

	result, err := store.SubmitTask("T-ev4-0", "worker-ev4", "abcdef", string(handoff), nil)
	if err != nil {
		t.Fatalf("SubmitTask with empty evidence: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got message: %s", result.Message)
	}
}
