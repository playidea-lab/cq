package handlers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
)

// TestHandleAddTodo tests the handleAddTodo function
func TestHandleAddTodo(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		wantErr   bool
		errMsg    string
		wantTasks int
	}{
		{
			name: "normal add with all fields",
			args: `{
				"task_id": "T-NEW-001",
				"title": "New feature",
				"dod": "Implement the feature with tests",
				"scope": "src/feature/",
				"dependencies": ["T-000-0"],
				"priority": 5,
				"domain": "web-backend"
			}`,
			wantErr:   false,
			wantTasks: 2, // T-task + auto-generated R-task
		},
		{
			name:      "missing task_id",
			args:      `{"title": "Test", "dod": "Done"}`,
			wantErr:   true,
			errMsg:    "task_id is required",
			wantTasks: 0,
		},
		{
			name:      "missing title",
			args:      `{"task_id": "T-002", "dod": "Done"}`,
			wantErr:   true,
			errMsg:    "title is required",
			wantTasks: 0,
		},
		{
			name:      "missing dod",
			args:      `{"task_id": "T-003", "title": "Test"}`,
			wantErr:   true,
			errMsg:    "dod is required",
			wantTasks: 0,
		},
		{
			name:      "invalid task_id format",
			args:      `{"task_id": "INVALID", "title": "Bad", "dod": "Done"}`,
			wantErr:   true,
			errMsg:    "invalid task_id format",
			wantTasks: 0,
		},
		{
			name:      "invalid dependency format",
			args:      `{"task_id": "T-VALID-0", "title": "Task", "dod": "Done", "dependencies": ["bad dep"]}`,
			wantErr:   true,
			errMsg:    "invalid dependency",
			wantTasks: 0,
		},
		{
			name: "first add creates review task",
			args: `{
				"task_id": "T-DUP-001",
				"title": "First task",
				"dod": "Done"
			}`,
			wantErr:   false,
			wantTasks: 2,
		},
		{
			name: "no review task for non-T prefix",
			args: `{
				"task_id": "CP-001",
				"title": "Checkpoint",
				"dod": "Review checkpoint"
			}`,
			wantErr:   false,
			wantTasks: 1, // No auto-review for non-T prefix
		},
		{
			name: "review_required false",
			args: `{
				"task_id": "T-NO-REVIEW-001",
				"title": "No review",
				"dod": "Done",
				"review_required": false
			}`,
			wantErr:   false,
			wantTasks: 1, // No auto-review when explicitly disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()

			result, err := handleAddTodo(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}

			if m["success"] != true {
				t.Errorf("success = %v, want true", m["success"])
			}

			if len(store.addedTasks) != tt.wantTasks {
				t.Errorf("added %d tasks, want %d", len(store.addedTasks), tt.wantTasks)
			}

			// Verify review task auto-generation
			if tt.wantTasks == 2 {
				if _, hasReview := m["review_task_id"]; !hasReview {
					t.Error("expected review_task_id in result")
				}
			}
		})
	}
}

func TestHandleAddTodo_DeterministicReviewIDForHyphenBase(t *testing.T) {
	store := newMockStore()

	args := `{
		"task_id": "T-LH-my-cool-tool-0",
		"title": "Lighthouse implementation",
		"dod": "Implement and test"
	}`
	result, err := handleAddTodo(store, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	wantReviewID := "R-LH-my-cool-tool-0"
	if got := m["review_task_id"]; got != wantReviewID {
		t.Fatalf("review_task_id = %v, want %s", got, wantReviewID)
	}
	if len(store.addedTasks) != 2 {
		t.Fatalf("added tasks = %d, want 2", len(store.addedTasks))
	}
	if got := store.addedTasks[1].ID; got != wantReviewID {
		t.Fatalf("generated review task ID = %s, want %s", got, wantReviewID)
	}
}

func TestHandleAddTodo_ExecutionModeDefaultAndExplicit(t *testing.T) {
	t.Run("default execution_mode is worker", func(t *testing.T) {
		store := newMockStore()
		args := `{"task_id":"T-EX-MODE-001","title":"Task","dod":"Done"}`
		if _, err := handleAddTodo(store, json.RawMessage(args)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.addedTasks) == 0 {
			t.Fatal("expected added task")
		}
		if got := store.addedTasks[0].ExecutionMode; got != "worker" {
			t.Fatalf("execution_mode = %q, want worker", got)
		}
	})

	t.Run("explicit execution_mode is preserved", func(t *testing.T) {
		store := newMockStore()
		args := `{"task_id":"T-EX-MODE-002","title":"Task","dod":"Done","execution_mode":"direct"}`
		if _, err := handleAddTodo(store, json.RawMessage(args)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.addedTasks) == 0 {
			t.Fatal("expected added task")
		}
		if got := store.addedTasks[0].ExecutionMode; got != "direct" {
			t.Fatalf("execution_mode = %q, want direct", got)
		}
	})
}

func TestHandleAddTodo_InvalidExecutionMode(t *testing.T) {
	store := newMockStore()
	args := `{"task_id":"T-EX-MODE-003","title":"Task","dod":"Done","execution_mode":"invalid"}`
	_, err := handleAddTodo(store, json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for invalid execution_mode, got nil")
	}
	if !contains(err.Error(), "invalid execution_mode") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "invalid execution_mode")
	}
}

// TestHandleAddTodo_ReviewTaskFailureRollsBackMain verifies that when review_required=true
// and adding the review task fails, the main task is deleted (rollback).
func TestHandleAddTodo_ReviewTaskFailureRollsBackMain(t *testing.T) {
	store := newMockStore()
	store.addTaskFn = func(task *Task) error {
		if strings.HasPrefix(task.ID, "R-") {
			return fmt.Errorf("injected review add failure")
		}
		store.addedTasks = append(store.addedTasks, task)
		store.tasks[task.ID] = task
		return nil
	}
	args := `{"task_id":"T-ROLL-001","title":"Rollback test","dod":"Done"}`
	_, err := handleAddTodo(store, json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error when review task add fails, got nil")
	}
	if !strings.Contains(err.Error(), "review task") || !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error = %q, want mention of review task and rollback", err.Error())
	}
	if len(store.deleteTaskCalls) != 1 || store.deleteTaskCalls[0] != "T-ROLL-001" {
		t.Errorf("DeleteTask calls = %v, want [T-ROLL-001]", store.deleteTaskCalls)
	}
	if _, ok := store.tasks["T-ROLL-001"]; ok {
		t.Error("main task T-ROLL-001 should be removed after rollback")
	}
}

func TestHandleAddTodo_ExecutionModeAuto(t *testing.T) {
	store := newMockStore()
	args := `{"task_id":"T-AUTO-001","title":"Task","dod":"Done","execution_mode":"auto"}`
	if _, err := handleAddTodo(store, json.RawMessage(args)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.addedTasks) == 0 {
		t.Fatal("expected added task")
	}
	if got := store.addedTasks[0].ExecutionMode; got != "auto" {
		t.Fatalf("execution_mode = %q, want auto", got)
	}
}

func TestHandleAddTodo_TaskIDParsingBoundaries(t *testing.T) {
	t.Run("hyphen base accepted", func(t *testing.T) {
		store := newMockStore()
		args := `{"task_id":"T-DASH-001-0","title":"Task","dod":"Done"}`
		result, err := handleAddTodo(store, json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m, _ := result.(map[string]any)
		if m["task_id"] != "T-DASH-001-0" {
			t.Errorf("task_id = %v, want T-DASH-001-0", m["task_id"])
		}
	})
	t.Run("legacy no version accepted", func(t *testing.T) {
		store := newMockStore()
		args := `{"task_id":"T-LEGACY","title":"Task","dod":"Done"}`
		_, err := handleAddTodo(store, json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.addedTasks) == 0 {
			t.Fatal("expected added task")
		}
		if store.addedTasks[0].ID != "T-LEGACY" {
			t.Errorf("task_id = %q, want T-LEGACY", store.addedTasks[0].ID)
		}
	})
}

func TestHandleTaskList_UsesStoreListTasks(t *testing.T) {
	store := newMockStore()
	args := `{"status":"pending","domain":"backend","limit":10}`
	result, err := handleTaskList(store, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.listTasksCalls) != 1 {
		t.Fatalf("ListTasks calls = %d, want 1", len(store.listTasksCalls))
	}
	f := store.listTasksCalls[0]
	if f.Status != "pending" || f.Domain != "backend" || f.Limit != 10 {
		t.Errorf("filter = Status=%q Domain=%q Limit=%d; want pending, backend, 10", f.Status, f.Domain, f.Limit)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if _, hasTasks := m["tasks"]; !hasTasks {
		t.Error("result missing tasks key")
	}
	if _, hasTotal := m["total"]; !hasTotal {
		t.Error("result missing total key")
	}
}

// TestHandleGetTask tests the handleGetTask function
func TestHandleGetTask(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		assignment *TaskAssignment
		wantErr    bool
		errMsg     string
		wantEmpty  bool
	}{
		{
			name: "normal assignment",
			args: `{"worker_id": "worker-test123"}`,
			assignment: &TaskAssignment{
				TaskID:   "T-001-0",
				Title:    "Test task",
				DoD:      "Write tests",
				WorkerID: "worker-abc",
				Branch:   "c4/w-T-001-0",
			},
			wantErr:   false,
			wantEmpty: false,
		},
		{
			name:       "no tasks available",
			args:       `{"worker_id": "worker-test123"}`,
			assignment: nil,
			wantErr:    false,
			wantEmpty:  true,
		},
		{
			name:    "missing worker_id",
			args:    `{}`,
			wantErr: true,
			errMsg:  "worker_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.assignment = tt.assignment

			result, err := handleGetTask(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantEmpty {
				m, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("result type = %T, want map[string]any", result)
				}
				if len(m) != 0 {
					t.Errorf("expected empty map, got %v", m)
				}
				return
			}

			a, ok := result.(*TaskAssignment)
			if !ok {
				t.Fatalf("result type = %T, want *TaskAssignment", result)
			}
			if a.TaskID != tt.assignment.TaskID {
				t.Errorf("TaskID = %q, want %q", a.TaskID, tt.assignment.TaskID)
			}
		})
	}
}

// TestHandleSubmit tests the handleSubmit function
func TestHandleSubmit(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		submitErr error
		wantErr   bool
		errMsg    string
	}{
		{
			name: "normal submit",
			args: `{
				"task_id": "T-001-0",
				"commit_sha": "abc123",
				"validation_results": [
					{"name": "lint", "status": "pass"},
					{"name": "unit", "status": "pass"}
				],
				"worker_id": "worker-test"
			}`,
			submitErr: nil,
			wantErr:   false,
		},
		{
			name: "validation failure case",
			args: `{
				"task_id": "T-002-0",
				"commit_sha": "def456",
				"validation_results": [
					{"name": "lint", "status": "fail", "message": "syntax error"}
				],
				"worker_id": "worker-test"
			}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "cannot submit",
		},
		{
			name:      "missing task_id",
			args:      `{"commit_sha": "abc123", "validation_results": []}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "task_id is required",
		},
		{
			name:      "missing commit_sha",
			args:      `{"task_id": "T-001-0", "validation_results": []}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "commit_sha is required",
		},
		{
			name:      "missing worker_id",
			args:      `{"task_id": "T-001-0", "commit_sha": "abc123", "validation_results": []}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "worker_id is required",
		},
		{
			name: "store submit error",
			args: `{
				"task_id": "T-003-0",
				"commit_sha": "ghi789",
				"worker_id": "worker-test"
			}`,
			submitErr: fmt.Errorf("database error"),
			wantErr:   true,
			errMsg:    "submitting task",
		},
		{
			name: "invalid status enum unknown",
			args: `{
				"task_id": "T-004-0",
				"commit_sha": "abc123",
				"worker_id": "worker-test",
				"validation_results": [
					{"name": "unit", "status": "unknown"}
				]
			}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "must be \"pass\" or \"fail\"",
		},
		{
			name: "invalid status enum empty string",
			args: `{
				"task_id": "T-004-0",
				"commit_sha": "abc123",
				"worker_id": "worker-test",
				"validation_results": [
					{"name": "unit", "status": ""}
				]
			}`,
			submitErr: nil,
			wantErr:   true,
			errMsg:    "must be \"pass\" or \"fail\"",
		},
		{
			name:      "validation_results omitted is valid",
			args:      `{"task_id": "T-005-0", "commit_sha": "sha123", "worker_id": "worker-test"}`,
			submitErr: nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.submitErr = tt.submitErr

			result, err := handleSubmit(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			r, ok := result.(*SubmitResult)
			if !ok {
				t.Fatalf("result type = %T, want *SubmitResult", result)
			}
			if !r.Success {
				t.Error("expected success=true")
			}
		})
	}
}

// TestHandleSubmitWorkerIDRequired verifies that handleSubmit rejects requests
// with a missing worker_id at the handler level (schema enforcement).
func TestHandleSubmitWorkerIDRequired(t *testing.T) {
	store := newMockStore()
	_, err := handleSubmit(store, json.RawMessage(`{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"validation_results": []
	}`))
	if err == nil {
		t.Fatal("expected error for missing worker_id, got nil")
	}
	if !contains(err.Error(), "worker_id is required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "worker_id is required")
	}
}

// TestHandleSubmitOwnerMismatch verifies that handleSubmit rejects a submission
// when the provided worker_id does not match the task's assigned owner.
func TestHandleSubmitOwnerMismatch(t *testing.T) {
	store := newMockStore()
	// Configure mock store to return an ownership-rejection SubmitResult.
	store.submitRes = &SubmitResult{
		Success:    false,
		NextAction: "get_next_task",
		Message:    "Task T-001-0 is owned by worker worker-a (submitter: worker-b)",
	}
	result, err := handleSubmit(store, json.RawMessage(`{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-b"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := result.(*SubmitResult)
	if !ok {
		t.Fatalf("result type = %T, want *SubmitResult", result)
	}
	if r.Success {
		t.Error("expected success=false for owner mismatch")
	}
	if !contains(r.Message, "owned by worker") {
		t.Errorf("message = %q, want substring 'owned by worker'", r.Message)
	}
}

// TestHandleClaim tests the handleClaim function (direct mode)
func TestHandleClaim(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		claimErr error
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "normal claim",
			args:     `{"task_id": "T-DASH-001-0"}`,
			claimErr: nil,
			wantErr:  false,
		},
		{
			name:     "already claimed task",
			args:     `{"task_id": "T-DASH-002-0"}`,
			claimErr: fmt.Errorf("task already claimed"),
			wantErr:  true,
			errMsg:   "claiming task",
		},
		{
			name:     "missing task_id",
			args:     `{}`,
			claimErr: nil,
			wantErr:  true,
			errMsg:   "task_id is required",
		},
		{
			name:     "invalid task_id format",
			args:     `{"task_id": "INVALID"}`,
			claimErr: nil,
			wantErr:  true,
			errMsg:   "invalid task_id format",
		},
		{
			name:     "review task_id not claimable",
			args:     `{"task_id": "R-001-0"}`,
			claimErr: nil,
			wantErr:  true,
			errMsg:   "implementation task (T-)",
		},
		{
			name:     "checkpoint task_id not claimable",
			args:     `{"task_id": "CP-001"}`,
			claimErr: nil,
			wantErr:  true,
			errMsg:   "implementation task (T-)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.claimErr = tt.claimErr

			result, err := handleClaim(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}
			if m["success"] != true {
				t.Errorf("success = %v, want true", m["success"])
			}
			if m["status"] != "in_progress" {
				t.Errorf("status = %v, want in_progress", m["status"])
			}
		})
	}
}

// TestHandleReport tests the handleReport function (direct mode)
func TestHandleReport(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		reportErr error
		wantErr   bool
		errMsg    string
	}{
		{
			name: "normal report",
			args: `{
				"task_id": "T-001-0",
				"summary": "Implemented the feature with full test coverage",
				"files_changed": ["src/feature.go", "src/feature_test.go"]
			}`,
			reportErr: nil,
			wantErr:   false,
		},
		{
			name:      "non-existent task",
			args:      `{"task_id": "T-NONEXIST", "summary": "Done"}`,
			reportErr: fmt.Errorf("task not found"),
			wantErr:   true,
			errMsg:    "reporting task",
		},
		{
			name:      "missing task_id",
			args:      `{"summary": "Done"}`,
			reportErr: nil,
			wantErr:   true,
			errMsg:    "task_id is required",
		},
		{
			name:      "missing summary",
			args:      `{"task_id": "T-001-0"}`,
			reportErr: nil,
			wantErr:   true,
			errMsg:    "summary is required",
		},
		{
			name:      "invalid task_id format",
			args:      `{"task_id": "INVALID", "summary": "Done"}`,
			reportErr: nil,
			wantErr:   true,
			errMsg:    "invalid task_id format",
		},
		{
			name: "empty files_changed accepted",
			args: `{
				"task_id": "T-001-0",
				"summary": "Done with no files",
				"files_changed": []
			}`,
			reportErr: nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.reportErr = tt.reportErr

			result, err := handleReport(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}
			if m["success"] != true {
				t.Errorf("success = %v, want true", m["success"])
			}
		})
	}
}

// TestHandleMarkBlocked tests the handleMarkBlocked function
func TestHandleMarkBlocked(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		blockErr     error
		wantErr      bool
		errMsg       string
		wantAttempts int
	}{
		{
			name: "normal mark blocked",
			args: `{
				"task_id": "T-001-0",
				"worker_id": "worker-abc",
				"failure_signature": "lint: syntax error in main.go",
				"attempts": 3,
				"last_error": "unexpected token"
			}`,
			blockErr:     nil,
			wantErr:      false,
			wantAttempts: 3,
		},
		{
			name:     "missing task_id",
			args:     `{"worker_id": "w", "failure_signature": "err", "attempts": 1}`,
			blockErr: nil,
			wantErr:  true,
			errMsg:   "task_id is required",
		},
		{
			name:     "missing worker_id",
			args:     `{"task_id": "T-001", "failure_signature": "err", "attempts": 1}`,
			blockErr: nil,
			wantErr:  true,
			errMsg:   "worker_id is required",
		},
		{
			name:     "missing failure_signature",
			args:     `{"task_id": "T-001", "worker_id": "w", "attempts": 1}`,
			blockErr: nil,
			wantErr:  true,
			errMsg:   "failure_signature is required",
		},
		{
			name: "store returns not found error",
			args: `{
				"task_id": "T-GHOST-0",
				"worker_id": "worker-1",
				"failure_signature": "build_fail",
				"attempts": 1,
				"last_error": "exit 1"
			}`,
			blockErr: fmt.Errorf("task T-GHOST-0 not found"),
			wantErr:  true,
			errMsg:   "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.blockErr = tt.blockErr

			result, err := handleMarkBlocked(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map[string]any", result)
			}
			if m["success"] != true {
				t.Errorf("success = %v, want true", m["success"])
			}
			if m["status"] != "blocked" {
				t.Errorf("status = %v, want blocked", m["status"])
			}

			if len(store.blockedCalls) != 1 {
				t.Fatalf("expected 1 blocked call, got %d", len(store.blockedCalls))
			}
			call := store.blockedCalls[0]
			if call.attempts != tt.wantAttempts {
				t.Errorf("attempts = %d, want %d", call.attempts, tt.wantAttempts)
			}
		})
	}
}

// TestHandleRequestChanges tests the handleRequestChanges function
func TestHandleRequestChanges(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "normal request changes",
			args: `{
				"review_task_id": "R-001-0",
				"comments": "Needs more test coverage",
				"required_changes": ["Add unit tests", "Add integration tests"]
			}`,
			wantErr: false,
		},
		{
			name:    "missing review_task_id",
			args:    `{"comments": "bad", "required_changes": ["fix"]}`,
			wantErr: true,
			errMsg:  "review_task_id is required",
		},
		{
			name:    "missing comments",
			args:    `{"review_task_id": "R-001-0", "required_changes": ["fix"]}`,
			wantErr: true,
			errMsg:  "comments is required",
		},
		{
			name:    "empty required_changes",
			args:    `{"review_task_id": "R-001-0", "comments": "bad", "required_changes": []}`,
			wantErr: true,
			errMsg:  "required_changes must contain at least one non-empty item",
		},
		{
			name:    "required_changes normalize to empty",
			args:    `{"review_task_id": "R-001-0", "comments": "bad", "required_changes": [" ", "\n\t", ""]}`,
			wantErr: true,
			errMsg:  "required_changes must contain at least one non-empty item",
		},
		{
			name:    "invalid review_task_id format",
			args:    `{"review_task_id": "INVALID", "comments": "bad", "required_changes": ["fix"]}`,
			wantErr: true,
			errMsg:  "invalid task_id format",
		},
		{
			name:    "implementation task_id not allowed",
			args:    `{"review_task_id": "T-001-0", "comments": "bad", "required_changes": ["fix"]}`,
			wantErr: true,
			errMsg:  "review_task_id must be an R- task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()

			result, err := handleRequestChanges(store, json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			r, ok := result.(*RequestChangesResult)
			if !ok {
				t.Fatalf("result type = %T, want *RequestChangesResult", result)
			}
			if !r.Success {
				t.Error("expected success=true")
			}
		})
	}
}

func TestHandleRequestChanges_NormalizesRequiredChanges(t *testing.T) {
	store := newMockStore()

	args := `{
		"review_task_id": "R-002-0",
		"comments": "needs work",
		"required_changes": ["  fix A  ", "", "fix A", " fix B ", "\n\t"]
	}`
	_, err := handleRequestChanges(store, json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.requestChangesCalls) != 1 {
		t.Fatalf("requestChangesCalls = %d, want 1", len(store.requestChangesCalls))
	}
	got := store.requestChangesCalls[0].requiredChanges
	want := []string{"fix A", "fix B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized required_changes = %#v, want %#v", got, want)
	}
}

// TestTaskHandlersViaRegistry tests handlers through MCP registry
func TestTaskHandlersViaRegistry(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	t.Run("c4_add_todo via registry", func(t *testing.T) {
		args := `{"task_id": "T-REG-001", "title": "Registry test", "dod": "Done"}`
		result, err := reg.Call("c4_add_todo", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_get_task via registry", func(t *testing.T) {
		args := `{"worker_id": "worker-reg"}`
		result, err := reg.Call("c4_get_task", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		a := result.(*TaskAssignment)
		if a.WorkerID != "worker-reg" {
			t.Errorf("WorkerID = %q, want worker-reg", a.WorkerID)
		}
	})

	t.Run("c4_submit via registry", func(t *testing.T) {
		args := `{"task_id": "T-001", "commit_sha": "xyz", "worker_id": "worker-reg"}`
		result, err := reg.Call("c4_submit", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := result.(*SubmitResult)
		if !r.Success {
			t.Error("expected success=true")
		}
	})

	t.Run("c4_mark_blocked via registry", func(t *testing.T) {
		args := `{"task_id": "T-002", "worker_id": "w", "failure_signature": "err", "attempts": 2}`
		result, err := reg.Call("c4_mark_blocked", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := result.(map[string]any)
		if m["success"] != true {
			t.Errorf("success = %v, want true", m["success"])
		}
	})

	t.Run("c4_request_changes via registry", func(t *testing.T) {
		args := `{"review_task_id": "R-002-0", "comments": "needs work", "required_changes": ["fix x"]}`
		result, err := reg.Call("c4_request_changes", json.RawMessage(args))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := result.(*RequestChangesResult)
		if !r.Success {
			t.Error("expected success=true")
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
