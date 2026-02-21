package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/config"
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

// TestSQLiteStoreSubmitTaskOwnerGuardEmptyWorker verifies that SubmitTask rejects
// a submission even when the task has an empty worker_id (i.e., not yet assigned).
// Before this fix the condition was `task.WorkerID != "" && task.WorkerID != workerID`,
// which allowed bypass when task.WorkerID was empty.
func TestSQLiteStoreSubmitTaskOwnerGuardEmptyWorker(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:     "T-001-0",
		Title:  "Implement feature",
		DoD:    "Done when tests pass",
		Status: "in_progress", // manually set in_progress with empty worker_id
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	// Force status to in_progress with empty worker_id (simulates tampered/corrupt state)
	if _, err := store.db.Exec(`UPDATE c4_tasks SET status = 'in_progress', worker_id = '' WHERE task_id = 'T-001-0'`); err != nil {
		t.Fatalf("force in_progress: %v", err)
	}

	result, err := store.SubmitTask("T-001-0", "worker-x", "abc123", "", []ValidationResult{
		{Name: "lint", Status: "pass"},
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if result.Success {
		t.Fatal("expected submit to fail: task has no assigned owner, worker-x should be rejected")
	}
	if !strings.Contains(result.Message, "owned by worker") {
		t.Fatalf("unexpected message: %q", result.Message)
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

func TestSQLiteStoreClaimTaskExecutionModeGuard(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:            "T-001-0",
		Title:         "Implement feature",
		DoD:           "Done when tests pass",
		Status:        "pending",
		ExecutionMode: "worker",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	_, err := store.ClaimTask("T-001-0")
	if err == nil {
		t.Fatal("expected claim to fail for worker execution_mode")
	}
	if !strings.Contains(err.Error(), "expected direct or auto") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteStoreSubmitTaskExecutionModeGuard(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:            "T-001-0",
		Title:         "Implement feature",
		DoD:           "Done when tests pass",
		Status:        "pending",
		ExecutionMode: "direct",
	}
	if err := store.AddTask(task); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if _, err := db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-a' WHERE task_id='T-001-0'"); err != nil {
		t.Fatalf("seed in_progress task: %v", err)
	}

	result, err := store.SubmitTask("T-001-0", "worker-a", "abc123", "", []ValidationResult{{Name: "lint", Status: "pass"}})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if result.Success {
		t.Fatal("expected submit to fail for direct execution_mode")
	}
	if !strings.Contains(result.Message, "worker submit allowed") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}

func TestSQLiteStoreReportTaskDirectSuccess(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	task := &Task{
		ID:            "T-001-0",
		Title:         "Implement feature",
		DoD:           "Done when tests pass",
		Status:        "pending",
		ExecutionMode: "direct",
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

	var commitSHA, branch, handoff string
	if err := db.QueryRow("SELECT commit_sha, branch, handoff FROM c4_tasks WHERE task_id='T-001-0'").Scan(&commitSHA, &branch, &handoff); err != nil {
		t.Fatalf("query direct report persistence: %v", err)
	}
	if commitSHA != "" {
		t.Fatalf("commit_sha = %q, want empty for direct report", commitSHA)
	}
	if branch != "" {
		t.Fatalf("branch = %q, want empty for direct report", branch)
	}

	var payload struct {
		Type         string   `json:"type"`
		Summary      string   `json:"summary"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(handoff), &payload); err != nil {
		t.Fatalf("handoff should be JSON payload: %v", err)
	}
	if payload.Type != "direct_report" {
		t.Fatalf("handoff.type = %q, want direct_report", payload.Type)
	}
	if payload.Summary != "done" {
		t.Fatalf("handoff.summary = %q, want done", payload.Summary)
	}
	if len(payload.FilesChanged) != 1 || payload.FilesChanged[0] != "feature.go" {
		t.Fatalf("handoff.files_changed = %v, want [feature.go]", payload.FilesChanged)
	}
}

func TestAssignTask_ReviewContextFromDirectReportHandoff(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-301-0", Title: "Impl", DoD: "done", Status: "pending", ExecutionMode: "direct"}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ClaimTask("T-301-0"); err != nil {
		t.Fatalf("claim task: %v", err)
	}
	if err := store.ReportTask("T-301-0", "direct summary", []string{"a.go", "b.go"}); err != nil {
		t.Fatalf("report task: %v", err)
	}

	// Add review task after report to avoid auto-cascade completion on the paired R task.
	if err := store.AddTask(&Task{
		ID:           "R-301-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Dependencies: []string{"T-301-0"},
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-review")
	if err != nil {
		t.Fatalf("assign review task: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected review assignment, got nil")
	}
	if assignment.TaskID != "R-301-0" {
		t.Fatalf("task_id = %q, want R-301-0", assignment.TaskID)
	}
	if assignment.ReviewContext == nil {
		t.Fatal("expected review_context, got nil")
	}
	if assignment.ReviewContext.ParentTaskID != "T-301-0" {
		t.Fatalf("parent_task_id = %q, want T-301-0", assignment.ReviewContext.ParentTaskID)
	}
	if assignment.ReviewContext.CommitSHA != "" {
		t.Fatalf("commit_sha = %q, want empty for direct report", assignment.ReviewContext.CommitSHA)
	}
	if assignment.ReviewContext.FilesChanged != "a.go,b.go" {
		t.Fatalf("files_changed = %q, want a.go,b.go", assignment.ReviewContext.FilesChanged)
	}
}

func TestSubmitTask_CascadeReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// Add T-task and its paired R-task (review_required=true scenario)
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

	// R-task should stay pending for a real review Worker (no auto-cascade)
	review, err := store.GetTask("R-010-0")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "pending" {
		t.Fatalf("R-task status = %q, want pending (review Worker must process it)", review.Status)
	}
}

func TestReportTask_CascadeReview(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-011-0", Title: "Impl", DoD: "done", Status: "pending", ExecutionMode: "direct"}); err != nil {
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

	// R-task should stay pending for a real review Worker (no auto-cascade)
	review, err := store.GetTask("R-011-0")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "pending" {
		t.Fatalf("R-task status = %q, want pending (review Worker must process it)", review.Status)
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

// TestSubmitTask_ReviewRequiredTrue verifies that when a T-task has a paired R-task
// (review_required=true), the R-task stays pending after SubmitTask so a real review
// Worker can be assigned.
func TestSubmitTask_ReviewRequiredTrue(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-100-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	// Paired R-task simulates review_required=true
	if err := store.AddTask(&Task{ID: "R-100-0", Title: "Review", DoD: "done", Status: "pending", Dependencies: []string{"T-100-0"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AssignTask("worker-1"); err != nil {
		t.Fatal(err)
	}
	result, err := store.SubmitTask("T-100-0", "worker-1", "sha-rev-true", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("submit failed: %s", result.Message)
	}
	// PendingReview must be set so caller knows R-task needs a Worker
	if result.PendingReview != "R-100-0" {
		t.Fatalf("PendingReview = %q, want R-100-0", result.PendingReview)
	}
	// R-task must remain pending — not auto-cascaded to done
	review, err := store.GetTask("R-100-0")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "pending" {
		t.Fatalf("R-task status = %q, want pending (review Worker must process it)", review.Status)
	}
}

// TestSubmitTask_ReviewRequiredFalse verifies that when a T-task has no paired R-task
// (review_required=false), SubmitTask succeeds with empty PendingReview.
func TestSubmitTask_ReviewRequiredFalse(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// No R-task created — simulates review_required=false
	if err := store.AddTask(&Task{ID: "T-101-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AssignTask("worker-1"); err != nil {
		t.Fatal(err)
	}
	result, err := store.SubmitTask("T-101-0", "worker-1", "sha-rev-false", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("submit failed: %s", result.Message)
	}
	// No R-task exists — PendingReview must be empty
	if result.PendingReview != "" {
		t.Fatalf("PendingReview = %q, want empty (no review task)", result.PendingReview)
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

	// REQUEST_CHANGES reason must be in review_decision_evidence, not commit_sha
	r, gerr := store.GetTask("R-001-0")
	if gerr != nil {
		t.Fatalf("GetTask R-001-0: %v", gerr)
	}
	if r.ReviewDecisionEvidence != "needs fixes" {
		t.Errorf("review_decision_evidence = %q, want %q", r.ReviewDecisionEvidence, "needs fixes")
	}
	if r.CommitSHA != "" {
		t.Errorf("commit_sha = %q (should be empty for REQUEST_CHANGES)", r.CommitSHA)
	}
}

func TestRequestChanges_MaxRevisionBoundary(t *testing.T) {
	tests := []struct {
		name          string
		maxRevision   int
		reviewVersion int
		wantErr       bool
	}{
		{
			name:          "allow when next version equals max_revision",
			maxRevision:   3,
			reviewVersion: 2,
			wantErr:       false,
		},
		{
			name:          "block when next version exceeds max_revision",
			maxRevision:   3,
			reviewVersion: 3,
			wantErr:       true,
		},
		{
			name:          "allow first change at max_revision one",
			maxRevision:   1,
			reviewVersion: 0,
			wantErr:       false,
		},
		{
			name:          "block second change at max_revision one",
			maxRevision:   1,
			reviewVersion: 1,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, db := newTestSQLiteStore(t)
			defer db.Close()

			cfg, err := config.New(t.TempDir())
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			cfg.Set("max_revision", tt.maxRevision)
			store.config = cfg

			parentTaskID := fmt.Sprintf("T-101-%d", tt.reviewVersion)
			reviewTaskID := fmt.Sprintf("R-101-%d", tt.reviewVersion)
			if err := store.AddTask(&Task{
				ID:     parentTaskID,
				Title:  "Impl",
				DoD:    "done",
				Status: "pending",
			}); err != nil {
				t.Fatalf("add parent task: %v", err)
			}
			if err := store.AddTask(&Task{
				ID:           reviewTaskID,
				Title:        "Review",
				DoD:          "review",
				Status:       "pending",
				Dependencies: []string{parentTaskID},
			}); err != nil {
				t.Fatalf("add review task: %v", err)
			}

			_, err = store.RequestChanges(reviewTaskID, "needs fixes", []string{"fix X"})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected max_revision error")
				}
				if !strings.Contains(err.Error(), "max revision") {
					t.Fatalf("error = %q, want max revision boundary message", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("request changes: %v", err)
			}
		})
	}
}

func TestRequestChanges_NormalizesRequiredChangesForDoD(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-201-0", Title: "Impl", DoD: "original dod", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{
		ID:           "R-201-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Dependencies: []string{"T-201-0"},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := store.RequestChanges("R-201-0", "needs fixes", []string{
		"  fix A  ",
		"",
		"fix A",
		" fix B ",
		"\n\t",
	})
	if err != nil {
		t.Fatalf("request changes: %v", err)
	}

	nextTask, err := store.GetTask("T-201-1")
	if err != nil {
		t.Fatalf("get next task: %v", err)
	}
	if strings.Count(nextTask.DoD, "- fix A") != 1 {
		t.Fatalf("DoD should contain deduped fix A once, got: %q", nextTask.DoD)
	}
	if strings.Count(nextTask.DoD, "- fix B") != 1 {
		t.Fatalf("DoD should contain trimmed fix B once, got: %q", nextTask.DoD)
	}
	if strings.Contains(nextTask.DoD, "- \n") {
		t.Fatalf("DoD should not contain empty bullet entries: %q", nextTask.DoD)
	}
}

func TestRequestChanges_RejectsNormalizedEmptyRequiredChanges(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-202-0", Title: "Impl", DoD: "original dod", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{
		ID:           "R-202-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Dependencies: []string{"T-202-0"},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := store.RequestChanges("R-202-0", "needs fixes", []string{" ", "\n\t", ""})
	if err == nil {
		t.Fatal("expected normalization-empty required_changes error")
	}
	if !strings.Contains(err.Error(), "required_changes must contain at least one non-empty item") {
		t.Fatalf("error = %q, want normalized-empty message", err.Error())
	}
	if _, getErr := store.GetTask("T-202-1"); getErr == nil {
		t.Fatal("unexpected next task created on invalid required_changes")
	}
}

func TestMarkBlocked_PersistsDiagnostics(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	if err := store.AddTask(&Task{ID: "T-BLK-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkBlocked("T-BLK-0", "worker-1", "validation_fail", 5, "go test failed: timeout"); err != nil {
		t.Fatalf("MarkBlocked: %v", err)
	}
	task, err := store.GetTask("T-BLK-0")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != "blocked" {
		t.Errorf("status = %q, want blocked", task.Status)
	}
	if task.FailureSignature != "validation_fail" {
		t.Errorf("failure_signature = %q, want validation_fail", task.FailureSignature)
	}
	if task.Attempts != 5 {
		t.Errorf("attempts = %d, want 5", task.Attempts)
	}
	if task.LastError != "go test failed: timeout" {
		t.Errorf("last_error = %q, want go test failed: timeout", task.LastError)
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
	result, err := store.Checkpoint("CP-001", "REQUEST_CHANGES", "not good", []string{"fix Y"}, "", "")
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

	if _, err := store.Checkpoint("CP-100", "APPROVE", "looks good", nil, "", ""); err != nil {
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

func TestCheckpoint_ExplicitLinkage_NoCPTask(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	// No CP-999 task in c4_tasks; explicit target_task_id/target_review_id provided
	if err := store.AddTask(&Task{ID: "T-999-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	store.db.Exec("UPDATE c4_tasks SET worker_id='worker-explicit' WHERE task_id='T-999-0'")

	_, err := store.Checkpoint("CP-999", "REQUEST_CHANGES", "needs work", []string{"fix A"}, "T-999-0", "R-999-0")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	var outcome string
	err = db.QueryRow("SELECT outcome FROM persona_stats WHERE persona_id='worker-explicit' AND task_id='T-999-0'").Scan(&outcome)
	if err != nil {
		t.Fatalf("query persona_stats: %v", err)
	}
	if outcome != "rejected" {
		t.Errorf("outcome = %q, want rejected (explicit linkage)", outcome)
	}
	var targetTaskID, targetReviewID string
	if err := db.QueryRow("SELECT target_task_id, target_review_id FROM c4_checkpoints WHERE checkpoint_id='CP-999'").Scan(&targetTaskID, &targetReviewID); err != nil {
		t.Fatalf("query checkpoint: %v", err)
	}
	if targetTaskID != "T-999-0" || targetReviewID != "R-999-0" {
		t.Errorf("linkage = %q, %q; want T-999-0, R-999-0", targetTaskID, targetReviewID)
	}
}

// TestMarkBlocked_NotFound_DoesNotCreateTask: MarkBlocked on non-existent task_id does not create a row.
func TestMarkBlocked_NotFound_DoesNotCreateTask(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	err := store.MarkBlocked("T-NONE-0", "worker-1", "sig", 1, "err")
	if err != nil {
		t.Fatalf("MarkBlocked (no row) should not error: %v", err)
	}
	_, getErr := store.GetTask("T-NONE-0")
	if getErr == nil {
		t.Fatal("GetTask(T-NONE-0) should fail (task not created by MarkBlocked)")
	}
}

// TestMarkBlocked_NotFound_OtherTasksUnchanged: MarkBlocked on non-existent task leaves other tasks unchanged.
func TestMarkBlocked_NotFound_OtherTasksUnchanged(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	if err := store.AddTask(&Task{ID: "T-OK-0", Title: "Ok", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	_ = store.MarkBlocked("T-NONE-0", "w", "sig", 1, "err")
	task, err := store.GetTask("T-OK-0")
	if err != nil {
		t.Fatalf("GetTask T-OK-0: %v", err)
	}
	if task.Status != "pending" {
		t.Errorf("T-OK-0 status = %q, want pending (unchanged)", task.Status)
	}
}

// TestCheckpoint_NotFoundCPTask_StillPersists: Checkpoint with non-existent CP task id still inserts c4_checkpoints row (no target resolution).
func TestCheckpoint_NotFoundCPTask_StillPersists(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()
	result, err := store.Checkpoint("CP-NONE", "APPROVE", "notes", nil, "", "")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	var checkpointID, decision, targetTaskID, targetReviewID string
	err = db.QueryRow("SELECT checkpoint_id, decision, target_task_id, target_review_id FROM c4_checkpoints WHERE checkpoint_id='CP-NONE'").Scan(&checkpointID, &decision, &targetTaskID, &targetReviewID)
	if err != nil {
		t.Fatalf("query c4_checkpoints: %v", err)
	}
	if checkpointID != "CP-NONE" || decision != "APPROVE" {
		t.Errorf("checkpoint = %q, decision = %q", checkpointID, decision)
	}
	if targetTaskID != "" || targetReviewID != "" {
		t.Errorf("targets = %q, %q (want empty when CP task not in DB)", targetTaskID, targetReviewID)
	}
}

// TestCheckpoint_StateConsistency_AfterApprove: After Checkpoint APPROVE, checkpoint row and target linkage are consistent.
func TestCheckpoint_StateConsistency_AfterApprove(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()
	if err := store.AddTask(&Task{ID: "T-SC-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-SC-0", Title: "Review", DoD: "r", Status: "pending", Dependencies: []string{"T-SC-0"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "CP-SC", Title: "CP", DoD: "cp", Status: "pending", Dependencies: []string{"R-SC-0"}}); err != nil {
		t.Fatal(err)
	}
	result, err := store.Checkpoint("CP-SC", "APPROVE", "ok", nil, "", "")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if result.NextAction != "continue" {
		t.Errorf("next_action = %q, want continue", result.NextAction)
	}
	var targetTaskID, targetReviewID string
	if err := db.QueryRow("SELECT target_task_id, target_review_id FROM c4_checkpoints WHERE checkpoint_id='CP-SC'").Scan(&targetTaskID, &targetReviewID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if targetTaskID != "T-SC-0" || targetReviewID != "R-SC-0" {
		t.Errorf("attribution = %q, %q; want T-SC-0, R-SC-0", targetTaskID, targetReviewID)
	}
}

// TestCheckpoint_ApproveFinal_MatchesApprove verifies that APPROVE_FINAL produces
// NextAction="continue" and emits checkpoint.approved event, identical to APPROVE behavior.
func TestCheckpoint_ApproveFinal_MatchesApprove(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()
	if err := store.AddTask(&Task{ID: "T-AF-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "R-AF-0", Title: "Review", DoD: "r", Status: "pending", Dependencies: []string{"T-AF-0"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask(&Task{ID: "CP-AF", Title: "CP", DoD: "cp", Status: "pending", Dependencies: []string{"R-AF-0"}}); err != nil {
		t.Fatal(err)
	}
	result, err := store.Checkpoint("CP-AF", "APPROVE_FINAL", "final approval", nil, "", "")
	if err != nil {
		t.Fatalf("Checkpoint with APPROVE_FINAL: %v", err)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.NextAction != "continue" {
		t.Errorf("NextAction = %q, want \"continue\"", result.NextAction)
	}
	// Verify the checkpoint row was persisted with the correct decision
	var persistedDecision string
	if err := db.QueryRow("SELECT decision FROM c4_checkpoints WHERE checkpoint_id='CP-AF'").Scan(&persistedDecision); err != nil {
		t.Fatalf("query checkpoint row: %v", err)
	}
	if persistedDecision != "APPROVE_FINAL" {
		t.Errorf("persisted decision = %q, want \"APPROVE_FINAL\"", persistedDecision)
	}
}

// TestMarkBlocked_StateConsistency: After MarkBlocked, task state and diagnostics are consistent in DB.
func TestMarkBlocked_StateConsistency(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	if err := store.AddTask(&Task{ID: "T-CS-0", Title: "Impl", DoD: "done", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkBlocked("T-CS-0", "worker-1", "build_fail", 2, "exit code 1"); err != nil {
		t.Fatalf("MarkBlocked: %v", err)
	}
	task, err := store.GetTask("T-CS-0")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != "blocked" {
		t.Errorf("status = %q, want blocked", task.Status)
	}
	if task.FailureSignature != "build_fail" || task.Attempts != 2 || task.LastError != "exit code 1" {
		t.Errorf("diagnostics = sig=%q attempts=%d last=%q", task.FailureSignature, task.Attempts, task.LastError)
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

// TestSubmitTask_ValidationResultsOptional verifies that omitting validation_results
// (nil slice) is accepted and sets ValidationSkipped=true in the result.
func TestSubmitTask_ValidationResultsOptional(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-vr-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-vr' WHERE task_id='T-vr-0'")

	// Submit without validation_results (nil)
	result, err := store.SubmitTask("T-vr-0", "worker-vr", "sha-vr", "", nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Message)
	}
	if !result.ValidationSkipped {
		t.Error("expected ValidationSkipped=true when no validation_results provided")
	}
}

// TestSubmitTask_ValidationSkipped_False verifies that providing results sets ValidationSkipped=false.
func TestSubmitTask_ValidationSkipped_False(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-vs-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-vs' WHERE task_id='T-vs-0'")

	result, err := store.SubmitTask("T-vs-0", "worker-vs", "sha-vs", "", []ValidationResult{{Name: "unit", Status: "pass"}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Message)
	}
	if result.ValidationSkipped {
		t.Error("expected ValidationSkipped=false when validation_results provided")
	}
}

// TestHandleSubmit_StatusEnumRejected verifies that handleSubmit rejects status values
// outside the "pass"|"fail" enum at the handler level.
func TestHandleSubmit_StatusEnumRejected(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	if err := store.AddTask(&Task{ID: "T-en-0", Title: "Impl", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id='worker-en' WHERE task_id='T-en-0'")

	args := json.RawMessage(`{
		"task_id": "T-en-0",
		"commit_sha": "sha-en",
		"worker_id": "worker-en",
		"validation_results": [{"name": "unit", "status": "unknown"}]
	}`)

	_, err := handleSubmit(store, args)
	if err == nil {
		t.Fatal("expected error for status=unknown, got nil")
	}
	if !strings.Contains(err.Error(), "must be") {
		t.Errorf("error = %q, want substring \"must be\"", err.Error())
	}
}

// newTestSQLiteStoreWithGitRepo creates a SQLiteStore backed by a real git repo and config.
// Used to test worktree auto-cleanup that requires actual git operations.
func newTestSQLiteStoreWithGitRepo(t *testing.T) (*SQLiteStore, *sql.DB, string) {
	t.Helper()

	repoDir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("git setup %v: %s: %v", args, string(out), runErr)
		}
	}
	os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main"), 0644)
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.CombinedOutput()
	}
	os.MkdirAll(filepath.Join(repoDir, ".c4"), 0755)

	cfg, cfgErr := config.New(repoDir)
	if cfgErr != nil {
		t.Fatalf("config.New: %v", cfgErr)
	}

	db, dbErr := sql.Open("sqlite", ":memory:")
	if dbErr != nil {
		t.Fatalf("open sqlite: %v", dbErr)
	}

	store, storeErr := NewSQLiteStore(db, WithProjectRoot(repoDir), WithConfig(cfg))
	if storeErr != nil {
		db.Close()
		t.Fatalf("new store: %v", storeErr)
	}

	return store, db, repoDir
}

func TestSubmitTask_WorktreeAutoCleanup_DeletesBranch(t *testing.T) {
	store, db, repoDir := newTestSQLiteStoreWithGitRepo(t)
	defer db.Close()

	workerID := "worker-cleanup-test"
	taskID := "T-cleanup-0"

	if err := store.AddTask(&Task{ID: taskID, Title: "cleanup test", DoD: "done", Status: "pending"}); err != nil {
		t.Fatal(err)
	}

	cfg := store.config.GetConfig()
	branchName := cfg.WorkBranchPrefix + taskID // e.g. "c4/w-T-cleanup-0"
	wtPath := filepath.Join(repoDir, ".c4", "worktrees", workerID)

	// Create the worktree+branch as enrichWithConfig does during AssignTask.
	if out, wtErr := exec.Command("git", "-C", repoDir, "worktree", "add", wtPath, "-b", branchName).CombinedOutput(); wtErr != nil {
		t.Fatalf("create worktree: %s: %v", string(out), wtErr)
	}

	// Seed task as in_progress owned by our worker.
	if _, dbErr := db.Exec("UPDATE c4_tasks SET status='in_progress', worker_id=? WHERE task_id=?", workerID, taskID); dbErr != nil {
		t.Fatalf("seed in_progress: %v", dbErr)
	}

	// Verify preconditions.
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree path should exist before submit: %v", statErr)
	}
	branchOut, _ := exec.Command("git", "-C", repoDir, "branch", "--list", branchName).CombinedOutput()
	if !strings.Contains(string(branchOut), branchName) {
		t.Fatalf("branch %q should exist before submit", branchName)
	}

	// Submit — triggers auto-cleanup.
	result, submitErr := store.SubmitTask(taskID, workerID, "abc123", "", nil)
	if submitErr != nil {
		t.Fatalf("submit: %v", submitErr)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}

	// Worktree directory must be gone.
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree path %q should have been removed after submit", wtPath)
	}

	// Branch must also be gone.
	branchOut2, _ := exec.Command("git", "-C", repoDir, "branch", "--list", branchName).CombinedOutput()
	if strings.Contains(string(branchOut2), branchName) {
		t.Errorf("branch %q should have been deleted after submit, still present: %q", branchName, string(branchOut2))
	}
}
