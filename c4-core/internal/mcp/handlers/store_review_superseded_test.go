package handlers

import (
	"testing"
)

// TestRequestChanges_SetsSupersededBy verifies that after RequestChanges(),
// the old R-task has superseded_by set to the new R-task ID.
func TestRequestChanges_SetsSupersededBy(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Create T-010-0 (parent, done) and R-010-0 (review, in_progress)
	if err := store.AddTask(&Task{ID: "T-010-0", Title: "Impl", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE c4_tasks SET status='done' WHERE task_id='T-010-0'")
	if err := store.AddTask(&Task{ID: "R-010-0", Title: "Review", DoD: "review"}); err != nil {
		t.Fatal(err)
	}

	result, err := store.RequestChanges("R-010-0", "needs fixes", []string{"fix X"})
	if err != nil {
		t.Fatalf("request changes: %v", err)
	}

	// Old R-task must have superseded_by = new R-task ID
	r, err := store.GetTask("R-010-0")
	if err != nil {
		t.Fatalf("GetTask R-010-0: %v", err)
	}
	if r.SupersededBy != result.NextReviewID {
		t.Errorf("R-010-0 superseded_by = %q, want %q", r.SupersededBy, result.NextReviewID)
	}

	// New R-task must NOT be superseded
	newR, err := store.GetTask(result.NextReviewID)
	if err != nil {
		t.Fatalf("GetTask %s: %v", result.NextReviewID, err)
	}
	if newR.SupersededBy != "" {
		t.Errorf("new %s superseded_by = %q, want empty", result.NextReviewID, newR.SupersededBy)
	}
}

// TestRequestChanges_ScopeInherited verifies that the new T and R tasks created
// by RequestChanges() inherit the scope from the parent T-task.
func TestRequestChanges_ScopeInherited(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	const wantScope = "c4-core/internal/mcp/"

	// Create T-020-0 with a scope, mark done; create R-020-0
	if err := store.AddTask(&Task{ID: "T-020-0", Title: "Impl", DoD: "done", Scope: wantScope}); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE c4_tasks SET status='done' WHERE task_id='T-020-0'")
	if err := store.AddTask(&Task{ID: "R-020-0", Title: "Review", DoD: "review"}); err != nil {
		t.Fatal(err)
	}

	result, err := store.RequestChanges("R-020-0", "needs fixes", []string{"fix Y"})
	if err != nil {
		t.Fatalf("request changes: %v", err)
	}

	// New fix T-task must inherit scope
	newT, err := store.GetTask(result.NextTaskID)
	if err != nil {
		t.Fatalf("GetTask %s: %v", result.NextTaskID, err)
	}
	if newT.Scope != wantScope {
		t.Errorf("new T-task scope = %q, want %q", newT.Scope, wantScope)
	}

	// New R-task must inherit scope
	newR, err := store.GetTask(result.NextReviewID)
	if err != nil {
		t.Fatalf("GetTask %s: %v", result.NextReviewID, err)
	}
	if newR.Scope != wantScope {
		t.Errorf("new R-task scope = %q, want %q", newR.Scope, wantScope)
	}
}

// TestAssignTask_SkipsSupersededReviews verifies that a superseded R-task is
// not returned by AssignTask even when its dependencies are met.
func TestAssignTask_SkipsSupersededReviews(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Create T-030-0 (done), R-030-0 (done, superseded), T-030-1 (done), R-030-1 (pending)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, superseded_by, dependencies) VALUES ('T-030-0', 'Impl', 'done', 'dod', '', '[]')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, superseded_by, dependencies) VALUES ('R-030-0', 'Review old', 'done', 'dod', 'R-030-1', '[]')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, superseded_by, dependencies) VALUES ('T-030-1', 'Fix', 'done', 'dod', '', '[]')`)
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, superseded_by, dependencies) VALUES ('R-030-1', 'Review new', 'pending', 'dod', '', '["T-030-1"]')`)

	assignment, err := store.AssignTask("worker-test-x")
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if assignment == nil {
		t.Fatal("AssignTask returned nil, want a task")
	}
	if assignment.TaskID != "R-030-1" {
		t.Errorf("AssignTask returned %q, want %q", assignment.TaskID, "R-030-1")
	}
}

// TestAssignTask_SkipsSupersededPendingReviews verifies that a pending R-task
// with superseded_by set is NOT returned by AssignTask.
func TestAssignTask_SkipsSupersededPendingReviews(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Only superseded pending task available — should return nil
	db.Exec(`INSERT INTO c4_tasks (task_id, title, status, dod, superseded_by, dependencies) VALUES ('R-040-0', 'Stale review', 'pending', 'dod', 'R-040-1', '[]')`)

	assignment, err := store.AssignTask("worker-test-y")
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if assignment != nil {
		t.Errorf("AssignTask returned %q, want nil (superseded tasks must be skipped)", assignment.TaskID)
	}
}
