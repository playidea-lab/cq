package handlers

import (
	"encoding/json"
	"fmt"
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
			wantErr:   false,
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
			name: "store submit error",
			args: `{
				"task_id": "T-003-0",
				"commit_sha": "ghi789",
				"validation_results": []
			}`,
			submitErr: fmt.Errorf("database error"),
			wantErr:   true,
			errMsg:    "submitting task",
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
		name      string
		args      string
		blockErr  error
		wantErr   bool
		errMsg    string
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
			errMsg:  "required_changes must not be empty",
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
		args := `{"task_id": "T-001", "commit_sha": "xyz", "validation_results": []}`
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
