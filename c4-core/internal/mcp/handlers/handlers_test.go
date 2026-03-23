package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/store"
)

// mockStore implements Store for testing.
type mockStore struct {
	status     *ProjectStatus
	tasks      map[string]*Task
	assignment *TaskAssignment
	submitRes  *SubmitResult
	cpResult   *CheckpointResult

	// Track calls
	startCalled         bool
	clearCalled         bool
	clearKeepConfig     bool
	addedTasks          []*Task
	blockedCalls        []markBlockedCall
	claimedTasks        []string
	reportedTasks       []reportCall
	checkpointCalls     []checkpointCall
	requestChangesCalls []requestChangesCall

	// Error injection
	statusErr     error
	startErr      error
	clearErr      error
	addErr        error
	assignErr     error
	submitErr     error
	blockErr      error
	claimErr      error
	reportErr     error
	checkpointErr error
	addTaskFn       func(*Task) error  // if set, AddTask calls this
	deleteTaskCalls []string          // records DeleteTask(taskID)
	listTasksCalls  []store.TaskFilter // records ListTasks(filter)
}

type markBlockedCall struct {
	taskID, workerID, failureSig string
	attempts                     int
	lastError                    string
}

type reportCall struct {
	taskID, summary string
	filesChanged    []string
}

type checkpointCall struct {
	id, decision, notes string
	requiredChanges     []string
}

type requestChangesCall struct {
	reviewTaskID    string
	comments        string
	requiredChanges []string
}

func newMockStore() *mockStore {
	return &mockStore{
		status: &ProjectStatus{
			State:        "EXECUTE",
			ProjectName:  "test-project",
			TotalTasks:   10,
			PendingTasks: 5,
			InProgress:   2,
			DoneTasks:    3,
			BlockedTasks: 0,
		},
		tasks: make(map[string]*Task),
		assignment: &TaskAssignment{
			TaskID:   "T-001-0",
			Title:    "Test task",
			DoD:      "Write tests",
			WorkerID: "worker-abc",
			Branch:   "c4/w-T-001-0",
		},
		submitRes: &SubmitResult{
			Success:    true,
			NextAction: "get_next_task",
			Message:    "Task completed",
		},
		cpResult: &CheckpointResult{
			Success:    true,
			NextAction: "continue",
			Message:    "Checkpoint approved",
		},
	}
}

func (m *mockStore) GetStatus() (*ProjectStatus, error) {
	if m.statusErr != nil {
		return nil, m.statusErr
	}
	return m.status, nil
}

func (m *mockStore) Start() error {
	m.startCalled = true
	return m.startErr
}

func (m *mockStore) Clear(keepConfig bool) error {
	m.clearCalled = true
	m.clearKeepConfig = keepConfig
	return m.clearErr
}

func (m *mockStore) AddTask(task *Task) error {
	if m.addTaskFn != nil {
		return m.addTaskFn(task)
	}
	if m.addErr != nil {
		return m.addErr
	}
	m.addedTasks = append(m.addedTasks, task)
	m.tasks[task.ID] = task
	return nil
}

func (m *mockStore) DeleteTask(taskID string) error {
	m.deleteTaskCalls = append(m.deleteTaskCalls, taskID)
	delete(m.tasks, taskID)
	return nil
}

func (m *mockStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	m.listTasksCalls = append(m.listTasksCalls, filter)
	return nil, 0, nil
}

func (m *mockStore) GetTask(taskID string) (*Task, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

func (m *mockStore) AssignTask(workerID string) (*TaskAssignment, error) {
	if m.assignErr != nil {
		return nil, m.assignErr
	}
	if m.assignment == nil {
		return nil, nil
	}
	// Set worker ID from request
	a := *m.assignment
	a.WorkerID = workerID
	return &a, nil
}

func (m *mockStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []ValidationResult) (*SubmitResult, error) {
	if m.submitErr != nil {
		return nil, m.submitErr
	}
	return m.submitRes, nil
}

func (m *mockStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	if m.blockErr != nil {
		return m.blockErr
	}
	m.blockedCalls = append(m.blockedCalls, markBlockedCall{
		taskID: taskID, workerID: workerID, failureSig: failureSignature,
		attempts: attempts, lastError: lastError,
	})
	return nil
}

func (m *mockStore) ClaimTask(taskID string) (*Task, error) {
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	m.claimedTasks = append(m.claimedTasks, taskID)
	return &Task{
		ID:     taskID,
		Title:  "Claimed task",
		Scope:  "src/",
		DoD:    "Do the work",
		Status: "in_progress",
	}, nil
}

func (m *mockStore) ReportTask(taskID, summary string, filesChanged []string) error {
	if m.reportErr != nil {
		return m.reportErr
	}
	m.reportedTasks = append(m.reportedTasks, reportCall{
		taskID: taskID, summary: summary, filesChanged: filesChanged,
	})
	return nil
}

func (m *mockStore) TransitionState(from, to string) error {
	return nil
}

func (m *mockStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*CheckpointResult, error) {
	if m.checkpointErr != nil {
		return nil, m.checkpointErr
	}
	m.checkpointCalls = append(m.checkpointCalls, checkpointCall{
		id: checkpointID, decision: decision, notes: notes, requiredChanges: requiredChanges,
	})
	return m.cpResult, nil
}

func (m *mockStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*RequestChangesResult, error) {
	m.requestChangesCalls = append(m.requestChangesCalls, requestChangesCall{
		reviewTaskID:    reviewTaskID,
		comments:        comments,
		requiredChanges: requiredChanges,
	})
	return &RequestChangesResult{
		Success:      true,
		NextTaskID:   "T-001-1",
		NextReviewID: "R-001-1",
		Version:      1,
		Message:      "Created T-001-1 + R-001-1 (v1)",
	}, nil
}

// --- c4_status tests ---

func TestStatusSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	result, err := reg.Call("c4_status", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status, ok := result.(*ProjectStatus)
	if !ok {
		t.Fatalf("result type = %T, want *ProjectStatus", result)
	}
	if status.State != "EXECUTE" {
		t.Errorf("State = %q, want %q", status.State, "EXECUTE")
	}
	if status.TotalTasks != 10 {
		t.Errorf("TotalTasks = %d, want %d", status.TotalTasks, 10)
	}
}

func TestStatusError(t *testing.T) {
	store := newMockStore()
	store.statusErr = fmt.Errorf("database error")
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	_, err := reg.Call("c4_status", json.RawMessage("{}"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- c4_start tests ---

func TestStartSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	result, err := reg.Call("c4_start", json.RawMessage("{}"))
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
	if m["status"] != "EXECUTE" {
		t.Errorf("status = %v, want EXECUTE", m["status"])
	}
	if !store.startCalled {
		t.Error("store.Start() was not called")
	}
}

func TestStartError(t *testing.T) {
	store := newMockStore()
	store.startErr = fmt.Errorf("already in EXECUTE state")
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	result, err := reg.Call("c4_start", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != false {
		t.Errorf("success = %v, want false", m["success"])
	}
}

// --- c4_clear tests ---

func TestClearSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	args := `{"confirm": true, "keep_config": true}`
	result, err := reg.Call("c4_clear", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if !store.clearCalled {
		t.Error("store.Clear() was not called")
	}
	if !store.clearKeepConfig {
		t.Error("keep_config was not passed through")
	}
}

func TestClearWithoutConfirm(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterStateHandlers(reg, store)

	args := `{"confirm": false}`
	result, err := reg.Call("c4_clear", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if _, hasError := m["error"]; !hasError {
		t.Error("expected error when confirm=false")
	}
	if store.clearCalled {
		t.Error("store.Clear() should not be called without confirm")
	}
}

// --- c4_get_task tests ---

func TestGetTaskSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{"worker_id": "worker-test123"}`
	result, err := reg.Call("c4_get_task", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a, ok := result.(*TaskAssignment)
	if !ok {
		t.Fatalf("result type = %T, want *TaskAssignment", result)
	}
	if a.TaskID != "T-001-0" {
		t.Errorf("TaskID = %q, want %q", a.TaskID, "T-001-0")
	}
	if a.WorkerID != "worker-test123" {
		t.Errorf("WorkerID = %q, want %q", a.WorkerID, "worker-test123")
	}
}

func TestGetTaskNoTaskAvailable(t *testing.T) {
	store := newMockStore()
	store.assignment = nil // No tasks
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{"worker_id": "worker-test123"}`
	result, err := reg.Call("c4_get_task", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestGetTaskMissingWorkerID(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{}`
	_, err := reg.Call("c4_get_task", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing worker_id")
	}
}

// --- c4_submit tests ---

func TestSubmitSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"validation_results": [
			{"name": "lint", "status": "pass"},
			{"name": "unit", "status": "pass"}
		],
		"worker_id": "worker-test"
	}`
	result, err := reg.Call("c4_submit", json.RawMessage(args))
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
	if r.NextAction != "get_next_task" {
		t.Errorf("NextAction = %q, want %q", r.NextAction, "get_next_task")
	}
}

func TestSubmitMissingTaskID(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{"commit_sha": "abc123", "validation_results": []}`
	_, err := reg.Call("c4_submit", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

func TestSubmitInvalidStatus(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-test",
		"validation_results": [{"name": "lint", "status": "unknown"}]
	}`
	_, err := reg.Call("c4_submit", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for status=unknown")
	}
}

func TestSubmitEmptyValidationResults(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-test",
		"validation_results": []
	}`
	_, err := reg.Call("c4_submit", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for empty validation_results")
	}
}

func TestSubmitAbsentValidationResults(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	// validation_results field absent entirely — should succeed
	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-test"
	}`
	result, err := reg.Call("c4_submit", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error for absent validation_results: %v", err)
	}
	r, ok := result.(*SubmitResult)
	if !ok {
		t.Fatalf("result type = %T, want *SubmitResult", result)
	}
	if !r.Success {
		t.Errorf("expected success=true for absent validation_results, got message: %s", r.Message)
	}
}

func TestSubmitSchemaValidationResultsNotRequired(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	// Verify the schema: validation_results must NOT be in required list
	toolSchema, ok := reg.GetToolSchema("c4_submit")
	if !ok {
		t.Fatal("c4_submit tool not registered")
	}
	required, ok := toolSchema.InputSchema["required"].([]string)
	if !ok {
		t.Fatal("required not a []string")
	}
	for _, field := range required {
		if field == "validation_results" {
			t.Error("validation_results must NOT be in required list (it is optional)")
		}
	}
}

func TestSubmitFailedValidationRejected(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-test",
		"validation_results": [{"name": "unit", "status": "fail", "message": "3 tests failed"}]
	}`
	_, err := reg.Call("c4_submit", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for failed validation, got nil")
	}
	if !strings.Contains(err.Error(), "cannot submit") {
		t.Errorf("error should mention 'cannot submit', got: %s", err.Error())
	}
}

func TestSubmitMixedValidationRejected(t *testing.T) {
	s := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, s)

	args := `{
		"task_id": "T-001-0",
		"commit_sha": "abc123",
		"worker_id": "worker-test",
		"validation_results": [
			{"name": "lint", "status": "pass"},
			{"name": "unit", "status": "fail", "message": "1 test failed"}
		]
	}`
	_, err := reg.Call("c4_submit", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error when any validation fails, got nil")
	}
	if !strings.Contains(err.Error(), `"unit"`) {
		t.Errorf("error should name the failing validation, got: %s", err.Error())
	}
}

// --- c4_add_todo tests ---

func TestAddTodoSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{
		"task_id": "T-NEW-001",
		"title": "New feature",
		"dod": "Implement the feature with tests",
		"scope": "src/feature/",
		"dependencies": ["T-000-0"],
		"priority": 5,
		"domain": "web-backend"
	}`
	result, err := reg.Call("c4_add_todo", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["task_id"] != "T-NEW-001" {
		t.Errorf("task_id = %v, want T-NEW-001", m["task_id"])
	}

	// Check stored tasks: T + R (auto-generated review)
	if len(store.addedTasks) != 2 {
		t.Fatalf("expected 2 added tasks (T + R), got %d", len(store.addedTasks))
	}
	implTask := store.addedTasks[0]
	if implTask.Title != "New feature" {
		t.Errorf("Title = %q, want %q", implTask.Title, "New feature")
	}
	if implTask.Priority != 5 {
		t.Errorf("Priority = %d, want %d", implTask.Priority, 5)
	}
	if implTask.Status != "pending" {
		t.Errorf("Status = %q, want %q", implTask.Status, "pending")
	}

	// Check auto-generated review task
	// ParseTaskID("T-NEW-001") → baseID="NEW", version=1 → ReviewID("NEW", 1) = "R-NEW-1"
	reviewTask := store.addedTasks[1]
	if reviewTask.ID != "R-NEW-1" {
		t.Errorf("Review task ID = %q, want %q", reviewTask.ID, "R-NEW-1")
	}
	if reviewTask.Dependencies[0] != "T-NEW-001" {
		t.Errorf("Review task dependency = %q, want %q", reviewTask.Dependencies[0], "T-NEW-001")
	}

	// Check result includes review_task_id
	if m["review_task_id"] != "R-NEW-1" {
		t.Errorf("review_task_id = %v, want R-NEW-1", m["review_task_id"])
	}
}

func TestAddTodoMissingRequired(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	// Missing dod
	args := `{"task_id": "T-001", "title": "Test"}`
	_, err := reg.Call("c4_add_todo", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing dod")
	}
}

// --- c4_mark_blocked tests ---

func TestMarkBlockedSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	args := `{
		"task_id": "T-001-0",
		"worker_id": "worker-abc",
		"failure_signature": "lint: syntax error in main.go",
		"attempts": 3,
		"last_error": "unexpected token"
	}`
	result, err := reg.Call("c4_mark_blocked", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
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
	if call.taskID != "T-001-0" {
		t.Errorf("taskID = %q, want %q", call.taskID, "T-001-0")
	}
	if call.attempts != 3 {
		t.Errorf("attempts = %d, want %d", call.attempts, 3)
	}
}

func TestMarkBlockedMissingFields(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTaskHandlers(reg, store)

	// Missing failure_signature
	args := `{"task_id": "T-001", "worker_id": "w", "attempts": 1}`
	_, err := reg.Call("c4_mark_blocked", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing failure_signature")
	}
}

// --- c4_claim tests ---

func TestClaimSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"task_id": "T-DASH-001-0"}`
	result, err := reg.Call("c4_claim", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}
	if m["task_id"] != "T-DASH-001-0" {
		t.Errorf("task_id = %v, want T-DASH-001-0", m["task_id"])
	}
	if m["status"] != "in_progress" {
		t.Errorf("status = %v, want in_progress", m["status"])
	}

	if len(store.claimedTasks) != 1 || store.claimedTasks[0] != "T-DASH-001-0" {
		t.Errorf("claimedTasks = %v, want [T-DASH-001-0]", store.claimedTasks)
	}
}

func TestClaimError(t *testing.T) {
	store := newMockStore()
	store.claimErr = fmt.Errorf("task not found")
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"task_id": "T-NONEXIST"}`
	_, err := reg.Call("c4_claim", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for claim failure")
	}
}

// --- c4_report tests ---

func TestReportSuccess(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{
		"task_id": "T-001-0",
		"summary": "Implemented the feature with full test coverage",
		"files_changed": ["src/feature.go", "src/feature_test.go"]
	}`
	result, err := reg.Call("c4_report", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["success"] != true {
		t.Errorf("success = %v, want true", m["success"])
	}

	if len(store.reportedTasks) != 1 {
		t.Fatalf("expected 1 report call, got %d", len(store.reportedTasks))
	}
	call := store.reportedTasks[0]
	if call.taskID != "T-001-0" {
		t.Errorf("taskID = %q, want %q", call.taskID, "T-001-0")
	}
	if len(call.filesChanged) != 2 {
		t.Errorf("filesChanged len = %d, want 2", len(call.filesChanged))
	}
}

func TestReportMissingSummary(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{"task_id": "T-001-0"}`
	_, err := reg.Call("c4_report", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing summary")
	}
}

// --- c4_checkpoint tests ---

func TestCheckpointApprove(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{
		"checkpoint_id": "CP-001",
		"decision": "APPROVE",
		"notes": "All tasks look good"
	}`
	result, err := reg.Call("c4_checkpoint", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if success, _ := r["success"].(bool); !success {
		t.Error("expected success=true")
	}

	if len(store.checkpointCalls) != 1 {
		t.Fatalf("expected 1 checkpoint call, got %d", len(store.checkpointCalls))
	}
	call := store.checkpointCalls[0]
	if call.decision != "APPROVE" {
		t.Errorf("decision = %q, want %q", call.decision, "APPROVE")
	}
}

func TestCheckpointApproveFinal(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{
		"checkpoint_id": "CP-002",
		"decision": "APPROVE_FINAL",
		"notes": "Final approval, project complete"
	}`
	result, err := reg.Call("c4_checkpoint", json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error for APPROVE_FINAL: %v", err)
	}

	r, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if success, _ := r["success"].(bool); !success {
		t.Error("expected success=true for APPROVE_FINAL")
	}

	if len(store.checkpointCalls) != 1 {
		t.Fatalf("expected 1 checkpoint call, got %d", len(store.checkpointCalls))
	}
	call := store.checkpointCalls[0]
	if call.decision != "APPROVE_FINAL" {
		t.Errorf("decision = %q, want %q", call.decision, "APPROVE_FINAL")
	}
}

func TestCheckpointAllValidDecisions(t *testing.T) {
	validDecisions := []string{"APPROVE", "APPROVE_FINAL", "REQUEST_CHANGES", "REPLAN"}

	for _, decision := range validDecisions {
		t.Run(decision, func(t *testing.T) {
			store := newMockStore()
			reg := mcp.NewRegistry()
			RegisterTrackingHandlers(reg, store)

			args := fmt.Sprintf(`{
				"checkpoint_id": "CP-001",
				"decision": %q,
				"notes": "test"
			}`, decision)
			_, err := reg.Call("c4_checkpoint", json.RawMessage(args))
			if err != nil {
				t.Fatalf("unexpected error for decision %q: %v", decision, err)
			}
		})
	}
}

func TestCheckpointInvalidDecision(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterTrackingHandlers(reg, store)

	args := `{
		"checkpoint_id": "CP-001",
		"decision": "INVALID",
		"notes": "bad"
	}`
	_, err := reg.Call("c4_checkpoint", json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for invalid decision")
	}
}

// --- RegisterAll integration test ---

func TestRegisterAllToolCount(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterAll(reg, store)

	tools := reg.ListTools()
	if len(tools) != 14 {
		names := make([]string, 0, len(tools))
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
		t.Errorf("registered %d tools, want 14: %v", len(tools), names)
	}

	// Verify all 14 expected tools are present
	expectedTools := []string{
		"c4_status", "c4_start", "c4_clear",
		"c4_get_task", "c4_submit", "c4_add_todo", "c4_request_changes", "c4_mark_blocked",
		"c4_task_list", "c4_worker_heartbeat", "c4_record_gate", "c4_claim", "c4_report", "c4_checkpoint",
	}
	for _, name := range expectedTools {
		if !reg.HasTool(name) {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestUnknownTool(t *testing.T) {
	store := newMockStore()
	reg := mcp.NewRegistry()
	RegisterAll(reg, store)

	_, err := reg.Call("c4_nonexistent", json.RawMessage("{}"))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
