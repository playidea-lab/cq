package handlers

import (
	"encoding/json"
	"testing"
)

func TestEnrichWithReviewContext_IncludesEvidence_WhenPresent(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Parent T task with evidence in handoff
	evidence := []HandoffEvidence{
		{Type: "test_result", ArtifactID: "art-001", Description: "test passed"},
	}
	handoffPayload, _ := json.Marshal(map[string]any{
		"summary":  "done",
		"evidence": evidence,
	})

	if err := store.AddTask(&Task{ID: "T-001-0", Title: "Impl", DoD: "done"}); err != nil {
		t.Fatalf("add T task: %v", err)
	}
	// AddTask always inserts as 'pending'; set done + handoff manually
	db.Exec(`UPDATE c4_tasks SET status='done', commit_sha='abc123', handoff=? WHERE task_id='T-001-0'`,
		string(handoffPayload))

	if err := store.AddTask(&Task{
		ID:           "R-001-0",
		Title:        "Review impl",
		DoD:          "review done",
		Dependencies: []string{"T-001-0"},
	}); err != nil {
		t.Fatalf("add R task: %v", err)
	}

	assignment, err := store.AssignTask("reviewer-1")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment.TaskID != "R-001-0" {
		t.Fatalf("expected R-001-0, got %s", assignment.TaskID)
	}

	rc := assignment.ReviewContext
	if rc == nil {
		t.Fatal("ReviewContext is nil")
	}
	if len(rc.Evidence) == 0 {
		t.Fatal("Evidence is empty, want 1 item")
	}
	if rc.Evidence[0].Type != "test_result" {
		t.Errorf("Evidence[0].Type = %q, want %q", rc.Evidence[0].Type, "test_result")
	}
	if rc.Evidence[0].ArtifactID != "art-001" {
		t.Errorf("Evidence[0].ArtifactID = %q, want %q", rc.Evidence[0].ArtifactID, "art-001")
	}
}

func TestEnrichWithReviewContext_EmptyEvidence_WhenAbsent(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-002-0", Title: "Impl 2", DoD: "done"}); err != nil {
		t.Fatalf("add T task: %v", err)
	}
	db.Exec(`UPDATE c4_tasks SET status='done', commit_sha='def456' WHERE task_id='T-002-0'`)

	if err := store.AddTask(&Task{
		ID:           "R-002-0",
		Title:        "Review impl 2",
		DoD:          "review done",
		Dependencies: []string{"T-002-0"},
	}); err != nil {
		t.Fatalf("add R task: %v", err)
	}

	assignment, err := store.AssignTask("reviewer-2")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment.TaskID != "R-002-0" {
		t.Fatalf("expected R-002-0, got %s", assignment.TaskID)
	}

	rc := assignment.ReviewContext
	if rc == nil {
		t.Fatal("ReviewContext is nil")
	}
	if len(rc.Evidence) != 0 {
		t.Errorf("Evidence should be empty, got %v", rc.Evidence)
	}
}
