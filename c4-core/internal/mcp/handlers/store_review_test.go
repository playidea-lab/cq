package handlers

import (
	"testing"
)

func TestNormalizeRequiredChanges(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "removes duplicates",
			input: []string{"fix A", "fix B", "fix A"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "trims whitespace",
			input: []string{"  fix A  ", "fix B"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "removes empty strings",
			input: []string{"fix A", "", "  ", "fix B"},
			want:  []string{"fix A", "fix B"},
		},
		{
			name:  "nil input",
			input: nil,
			want:  []string{},
		},
		{
			name:  "all empty",
			input: []string{"", "  "},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRequiredChanges(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got=%v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCompleteReviewTask(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert a T-task and corresponding R-task
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-001-0', 'Impl', 'done', 'test')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('R-001-0', 'Review', 'pending', 'test')`)

	// Completing T-001-0 should cascade to R-001-0
	reviewID := store.completeReviewTask("T-001-0")
	if reviewID != "R-001-0" {
		t.Errorf("cascaded reviewID = %q, want %q", reviewID, "R-001-0")
	}

	// Verify R-task is now done
	var status string
	db.QueryRow("SELECT status FROM c4_tasks WHERE task_id='R-001-0'").Scan(&status)
	if status != "done" {
		t.Errorf("R-001-0 status = %q, want %q", status, "done")
	}
}

func TestCompleteReviewTask_NonTPrefix(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// R- prefix should not cascade
	result := store.completeReviewTask("R-001-0")
	if result != "" {
		t.Errorf("expected empty for R- prefix, got %q", result)
	}

	// CP- prefix should not cascade
	result = store.completeReviewTask("CP-001")
	if result != "" {
		t.Errorf("expected empty for CP- prefix, got %q", result)
	}
}

func TestCompleteReviewTask_InvalidIDFailFast(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	// Non-conforming IDs are skipped (fail-fast: no cascade)
	for _, invalid := range []string{"", "T-bad!!!", "T-", "no-prefix"} {
		result := store.completeReviewTask(invalid)
		if result != "" {
			t.Errorf("completeReviewTask(%q) = %q, want \"\" (fail-fast)", invalid, result)
		}
	}
}

func TestCompleteReviewTask_NoMatchingReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod) VALUES ('T-002-0', 'Impl', 'done', 'test')`)

	// No R-002-0 exists
	result := store.completeReviewTask("T-002-0")
	if result != "" {
		t.Errorf("expected empty when no review exists, got %q", result)
	}
}

func TestResolveCheckpointTargets(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert checkpoint with JSON array deps
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('CP-001', 'Check', 'pending', 'review', '["T-001-0","R-001-0"]')`)

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "T-001-0" {
		t.Errorf("targetTaskID = %q, want %q", taskID, "T-001-0")
	}
	if reviewID != "R-001-0" {
		t.Errorf("targetReviewID = %q, want %q", reviewID, "R-001-0")
	}
}

func TestResolveCheckpointTargets_InfersTaskFromReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Only R- dep, should infer T- from it
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, dependencies) VALUES ('CP-002', 'Check', 'pending', 'review', '["R-005-0"]')`)

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "T-005-0" {
		t.Errorf("targetTaskID = %q, want %q", taskID, "T-005-0")
	}
	if reviewID != "R-005-0" {
		t.Errorf("targetReviewID = %q, want %q", reviewID, "R-005-0")
	}
}

func TestResolveCheckpointTargets_NotFound(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	taskID, reviewID, err := store.resolveCheckpointTargets("CP-NONEXIST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskID != "" || reviewID != "" {
		t.Errorf("expected empty results for non-existent checkpoint, got task=%q review=%q", taskID, reviewID)
	}
}
