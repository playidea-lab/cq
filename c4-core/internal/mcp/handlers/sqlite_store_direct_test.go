package handlers

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestSQLiteStore(t *testing.T) (*SQLiteStore, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatalf("new store: %v", err)
	}

	return store, db
}

func TestSQLiteStoreSubmitTaskOwnerGuard(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:     "T-001-0",
		Title:  "Implement feature",
		DoD:    "Done when tests pass",
		Status: "pending",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	assignment, err := store.AssignTask("worker-a")
	if err != nil {
		t.Fatalf("assign task: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}

	result, err := store.SubmitTask("T-001-0", "worker-b", "abc123", "", []ValidationResult{
		{Name: "lint", Status: "pass"},
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if result.Success {
		t.Fatal("expected submit to fail for wrong owner")
	}
	if !strings.Contains(result.Message, "owned by worker worker-a") {
		t.Fatalf("unexpected message: %q", result.Message)
	}

	got, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got.Status)
	}
}

func TestSQLiteStoreSubmitTaskStateGuard(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:     "T-001-0",
		Title:  "Implement feature",
		DoD:    "Done when tests pass",
		Status: "pending",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	result, err := store.SubmitTask("T-001-0", "worker-a", "abc123", "", []ValidationResult{
		{Name: "lint", Status: "pass"},
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if result.Success {
		t.Fatal("expected submit to fail for pending task")
	}
	if !strings.Contains(result.Message, "expected in_progress") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}

func TestSQLiteStoreReportTaskRequiresDirectOwner(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:     "T-001-0",
		Title:  "Implement feature",
		DoD:    "Done when tests pass",
		Status: "pending",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if _, err := store.AssignTask("worker-a"); err != nil {
		t.Fatalf("assign task: %v", err)
	}

	err := store.ReportTask("T-001-0", "done", []string{"feature.go"})
	if err == nil {
		t.Fatal("expected report to fail for non-direct owner")
	}
	if !strings.Contains(err.Error(), "expected direct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteStoreReportTaskDirectSuccess(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:     "T-001-0",
		Title:  "Implement feature",
		DoD:    "Done when tests pass",
		Status: "pending",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if _, err := store.ClaimTask("T-001-0"); err != nil {
		t.Fatalf("claim task: %v", err)
	}

	if err := store.ReportTask("T-001-0", "done", []string{"feature.go"}); err != nil {
		t.Fatalf("report task: %v", err)
	}

	got, err := store.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != "done" {
		t.Fatalf("status = %q, want done", got.Status)
	}
}

func TestSubmitTask_CascadeReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Add T-task and its paired R-task
	if err := store.AddTask(&Task{ID: "T-010-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-010-0", Title: "Review", DoD: "done", Status: "pending", Dependencies: []string{"T-010-0"}}); err != nil {
		t.Fatal(err)
	}

	// Assign and submit T-task
	if _, err := store.AssignTask("worker-1"); err != nil {
		t.Fatal(err)
	}
	result, err := store.SubmitTask("T-010-0", "worker-1", "sha1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("submit failed: %s", result.Message)
	}
	if result.PendingReview != "R-010-0" {
		t.Fatalf("PendingReview = %q, want R-010-0", result.PendingReview)
	}

	// R-task should be auto-completed
	review, err := store.GetTask("R-010-0")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "done" {
		t.Fatalf("R-task status = %q, want done", review.Status)
	}
}

func TestReportTask_CascadeReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-011-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-011-0", Title: "Review", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}

	// Claim (direct mode) and report
	if _, err := store.ClaimTask("T-011-0"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReportTask("T-011-0", "done", []string{"f.go"}); err != nil {
		t.Fatal(err)
	}

	review, err := store.GetTask("R-011-0")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "done" {
		t.Fatalf("R-task status = %q, want done", review.Status)
	}
}

func TestCompleteReviewTask_NoReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// T-task without a paired R-task — should not panic or error
	if err := store.AddTask(&Task{ID: "T-012-0", Title: "Solo", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignTask("worker-1"); err != nil {
		t.Fatal(err)
	}
	result, err := store.SubmitTask("T-012-0", "worker-1", "sha2", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("submit failed: %s", result.Message)
	}
	if result.PendingReview != "" {
		t.Fatalf("PendingReview = %q, want empty", result.PendingReview)
	}
}

func TestCompleteReviewTask_AlreadyDone(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-013-0", Title: "Impl", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-013-0", Title: "Review", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	// Manually mark R-task as done (AddTask always inserts as "pending")
	if _, err := db.Exec(`UPDATE c4_tasks SET status='done' WHERE task_id='R-013-0'`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AssignTask("worker-1"); err != nil {
		t.Fatal(err)
	}
	result, err := store.SubmitTask("T-013-0", "worker-1", "sha3", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("submit failed: %s", result.Message)
	}
	// Already done R-task should not be re-cascaded
	if result.PendingReview != "" {
		t.Fatalf("PendingReview = %q, want empty (already done)", result.PendingReview)
	}
}

func TestGetStatus_OrphanCount(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Create T-task and R-task, then manually mark T-task as done (orphan scenario)
	if err := store.AddTask(&Task{ID: "T-014-0", Title: "Impl", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-014-0", Title: "Review", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	// Mark T-task done manually to simulate orphan (AddTask always inserts as "pending")
	if _, err := db.Exec(`UPDATE c4_tasks SET status='done' WHERE task_id='T-014-0'`); err != nil {
		t.Fatal(err)
	}

	// Non-orphan: T pending, R pending
	if err := store.AddTask(&Task{ID: "T-015-0", Title: "Impl2", DoD: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-015-0", Title: "Review2", DoD: "done"}); err != nil {
		t.Fatal(err)
	}

	status, err := store.GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.OrphanReviews != 1 {
		t.Fatalf("OrphanReviews = %d, want 1", status.OrphanReviews)
	}
}

func TestSQLiteStoreStatusReadyTasks(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{
		ID:     "T-001-0",
		Title:  "Root task",
		DoD:    "done",
		Status: "pending",
	}); err != nil {
		t.Fatalf("add task 1: %v", err)
	}

	if err := store.AddTask(&Task{
		ID:           "T-002-0",
		Title:        "Depends on T-001-0",
		DoD:          "done",
		Status:       "pending",
		Dependencies: []string{"T-001-0"},
	}); err != nil {
		t.Fatalf("add task 2: %v", err)
	}

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("get status: %v", err)
	}

	if status.PendingTasks != 2 {
		t.Fatalf("pending = %d, want 2", status.PendingTasks)
	}
	if status.ReadyTasks != 1 {
		t.Fatalf("ready = %d, want 1", status.ReadyTasks)
	}
	if status.BlockedByDeps != 1 {
		t.Fatalf("blocked_by_dependencies = %d, want 1", status.BlockedByDeps)
	}
	if len(status.ReadyTaskIDs) == 0 || status.ReadyTaskIDs[0] != "T-001-0" {
		t.Fatalf("ready_task_ids = %v, want first T-001-0", status.ReadyTaskIDs)
	}
}
