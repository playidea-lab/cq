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

func TestRequestChanges_RecordsRejected(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Create T-001-0 (parent) and R-001-0 (review)
	if err := store.AddTask(&Task{ID: "T-001-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	// Assign and submit T-001-0 so it's done
	store.db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-1' WHERE task_id='T-001-0'")
	store.db.Exec("UPDATE c4_tasks SET status='done' WHERE task_id='T-001-0'")

	if err := store.AddTask(&Task{ID: "R-001-0", Title: "Review", DoD: "review", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}

	// RequestChanges should record rejected for parent's worker
	_, err := store.RequestChanges("R-001-0", "needs fixes", []string{"fix X"})
	if err != nil {
		t.Fatalf("request changes: %v", err)
	}

	// Check persona_stats has rejected
	var outcome string
	err = db.QueryRow("SELECT outcome FROM persona_stats WHERE persona_id='worker-1' AND task_id='T-001-0'").Scan(&outcome)
	if err != nil {
		t.Fatalf("query persona_stats: %v", err)
	}
	if outcome != "rejected" {
		t.Fatalf("outcome = %q, want rejected", outcome)
	}
}

func TestCheckpoint_RequestChanges_RecordsRejected(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Create target implementation task with known worker
	if err := store.AddTask(&Task{ID: "T-002-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	store.db.Exec("UPDATE c4_tasks SET worker_id='worker-2', updated_at=CURRENT_TIMESTAMP WHERE task_id='T-002-0'")

	// Link CP task to review task, then to target T task.
	if err := store.AddTask(&Task{
		ID:           "R-002-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Dependencies: []string{"T-002-0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{
		ID:           "CP-001",
		Title:        "Checkpoint",
		DoD:          "checkpoint",
		Status:       "pending",
		Dependencies: []string{"R-002-0"},
	}); err != nil {
		t.Fatal(err)
	}

	// Checkpoint with REQUEST_CHANGES
	result, err := store.Checkpoint("CP-001", "REQUEST_CHANGES", "not good", []string{"fix Y"})
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if result.NextAction != "apply_changes" {
		t.Fatalf("next_action = %q, want apply_changes", result.NextAction)
	}

	// Check persona_stats
	var outcome string
	err = db.QueryRow("SELECT outcome FROM persona_stats WHERE persona_id='worker-2' AND task_id='T-002-0'").Scan(&outcome)
	if err != nil {
		t.Fatalf("query persona_stats: %v", err)
	}
	if outcome != "rejected" {
		t.Fatalf("outcome = %q, want rejected", outcome)
	}
}

func TestCheckpoint_PersistsTargetLinkage(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-100-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{
		ID:           "R-100-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Dependencies: []string{"T-100-0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{
		ID:           "CP-100",
		Title:        "Checkpoint",
		DoD:          "checkpoint",
		Status:       "pending",
		Dependencies: []string{"R-100-0"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Checkpoint("CP-100", "APPROVE", "looks good", nil); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	var targetTaskID, targetReviewID string
	if err := db.QueryRow("SELECT target_task_id, target_review_id FROM c4_checkpoints WHERE checkpoint_id='CP-100'").Scan(&targetTaskID, &targetReviewID); err != nil {
		t.Fatalf("query checkpoint linkage: %v", err)
	}
	if targetTaskID != "T-100-0" {
		t.Fatalf("target_task_id = %q, want T-100-0", targetTaskID)
	}
	if targetReviewID != "R-100-0" {
		t.Fatalf("target_review_id = %q, want R-100-0", targetReviewID)
	}
}

func TestSubmitTask_ValidationFail_RecordsValidationFailed(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-003-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	// Assign task
	store.db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-3' WHERE task_id='T-003-0'")

	// Submit with failing validation
	result, err := store.SubmitTask("T-003-0", "worker-3", "abc", "", []ValidationResult{
		{Name: "lint", Status: "fail", Message: "syntax error"},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure")
	}

	// Check persona_stats
	var outcome string
	err = db.QueryRow("SELECT outcome FROM persona_stats WHERE persona_id='worker-3' AND task_id='T-003-0'").Scan(&outcome)
	if err != nil {
		t.Fatalf("query persona_stats: %v", err)
	}
	if outcome != "validation_failed" {
		t.Fatalf("outcome = %q, want validation_failed", outcome)
	}
}

func TestGetStatus_PersonaDigest(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Insert persona_stats directly
	db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES ('w1','T-1','approved',datetime('now'))")
	db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES ('w1','T-2','rejected',datetime('now'))")
	db.Exec("INSERT INTO persona_stats (persona_id, task_id, outcome, created_at) VALUES ('w1','T-3','approved',datetime('now'))")

	// Need at least one task for GetStatus
	store.AddTask(&Task{ID: "T-010-0", Title: "Test", DoD: "done", Status: "pending"})

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status.PersonaDigest == nil {
		t.Fatal("expected persona_digest, got nil")
	}
	if status.PersonaDigest.TotalTasks != 3 {
		t.Fatalf("total = %d, want 3", status.PersonaDigest.TotalTasks)
	}
	// 2/3 approved ≈ 0.667
	if status.PersonaDigest.ApprovalRate < 0.66 || status.PersonaDigest.ApprovalRate > 0.67 {
		t.Fatalf("approval_rate = %.3f, want ~0.667", status.PersonaDigest.ApprovalRate)
	}
}

func TestGetStatus_PersonaDigest_Empty(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	store.AddTask(&Task{ID: "T-011-0", Title: "Test", DoD: "done", Status: "pending"})

	status, err := store.GetStatus()
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status.PersonaDigest != nil {
		t.Fatalf("expected nil persona_digest, got %+v", status.PersonaDigest)
	}
}
