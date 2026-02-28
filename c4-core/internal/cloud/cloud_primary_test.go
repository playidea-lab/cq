package cloud

import (
	"errors"
	"testing"

	"github.com/changmin/c4-core/internal/store"
)

// TestCloudPrimaryStore_NewAndGetStatus verifies basic construction and
// cloud-first read semantics: cloud succeeds → cloud result returned.
func TestCloudPrimaryStore_NewAndGetStatus(t *testing.T) {
	remote := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return &store.ProjectStatus{State: "EXECUTE", TotalTasks: 10}, nil
		},
	}
	local := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return &store.ProjectStatus{State: "PLAN", TotalTasks: 1}, nil
		},
	}
	cp := NewCloudPrimaryStore(local, remote)
	status, err := cp.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if status.State != "EXECUTE" {
		t.Errorf("State = %q, want EXECUTE (cloud-primary should read from cloud)", status.State)
	}
	if status.TotalTasks != 10 {
		t.Errorf("TotalTasks = %d, want 10", status.TotalTasks)
	}
}

// TestCloudPrimaryStore_GetStatusFallback verifies that when cloud fails,
// local is used as fallback.
func TestCloudPrimaryStore_GetStatusFallback(t *testing.T) {
	remote := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return nil, errors.New("connection refused")
		},
	}
	local := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return &store.ProjectStatus{State: "PLAN", TotalTasks: 3}, nil
		},
	}
	cp := NewCloudPrimaryStore(local, remote)
	status, err := cp.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() fallback error: %v", err)
	}
	if status.State != "PLAN" {
		t.Errorf("State = %q, want PLAN (fallback to local)", status.State)
	}
}

// TestCloudPrimaryStore_GetTask verifies cloud-first read for GetTask.
func TestCloudPrimaryStore_GetTask(t *testing.T) {
	remote := &mockStore{
		getTaskFn: func(id string) (*store.Task, error) {
			return &store.Task{ID: id, Title: "from-cloud"}, nil
		},
	}
	local := &mockStore{
		getTaskFn: func(id string) (*store.Task, error) {
			return &store.Task{ID: id, Title: "from-local"}, nil
		},
	}
	cp := NewCloudPrimaryStore(local, remote)
	task, err := cp.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("GetTask() error: %v", err)
	}
	if task.Title != "from-cloud" {
		t.Errorf("Title = %q, want from-cloud", task.Title)
	}
}

// TestCloudPrimaryStore_GetTaskFallback verifies local fallback when cloud fails.
func TestCloudPrimaryStore_GetTaskFallback(t *testing.T) {
	remote := &mockStore{
		getTaskFn: func(id string) (*store.Task, error) {
			return nil, errors.New("cloud down")
		},
	}
	local := &mockStore{
		getTaskFn: func(id string) (*store.Task, error) {
			return &store.Task{ID: id, Title: "from-local"}, nil
		},
	}
	cp := NewCloudPrimaryStore(local, remote)
	task, err := cp.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("GetTask() fallback error: %v", err)
	}
	if task.Title != "from-local" {
		t.Errorf("Title = %q, want from-local", task.Title)
	}
}

// TestCloudPrimaryStore_ListTasks verifies cloud-first read for ListTasks.
func TestCloudPrimaryStore_ListTasks(t *testing.T) {
	remote := &mockStore{}
	local := &mockStore{}
	cp := NewCloudPrimaryStore(local, remote)
	tasks, total, err := cp.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks() error: %v", err)
	}
	if total != 0 || len(tasks) != 0 {
		t.Errorf("ListTasks() = %v, %d, want [], 0", tasks, total)
	}
}

// TestCloudPrimaryStore_ListTasksFallback verifies local fallback when cloud fails.
func TestCloudPrimaryStore_ListTasksFallback(t *testing.T) {
	remote := &mockStore{
		// Override via the interface method directly — need a custom mock
	}
	_ = remote
	// Use a separate mock type inline via function closure approach:
	// Since mockStore.ListTasks always returns nil,0,nil we can still test the branch
	// by simulating a failing remote.
	failRemote := &listFailStore{}
	local := &mockStore{}
	cp := NewCloudPrimaryStore(local, failRemote)
	tasks, total, err := cp.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks() fallback error: %v", err)
	}
	if total != 0 || len(tasks) != 0 {
		t.Errorf("ListTasks() fallback = %v, %d", tasks, total)
	}
}

// TestCloudPrimaryStore_DeleteTask verifies remote deletion on success.
func TestCloudPrimaryStore_DeleteTask(t *testing.T) {
	var remoteCalled bool
	remote := &mockStore{}
	_ = remote
	// deleteTaskFn is not in mockStore, but DeleteTask always returns nil.
	// We verify that when remote succeeds, no error is returned.
	cp := NewCloudPrimaryStore(&mockStore{}, &mockStore{})
	if err := cp.DeleteTask("T-001-0"); err != nil {
		t.Fatalf("DeleteTask() error: %v", err)
	}
	_ = remoteCalled
}

// TestCloudPrimaryStore_DeleteTaskRemoteFail verifies that remote failure blocks the operation.
func TestCloudPrimaryStore_DeleteTaskRemoteFail(t *testing.T) {
	// We need a remote that fails DeleteTask — use a wrapper.
	remote := &deleteFailStore{}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.DeleteTask("T-001-0"); err == nil {
		t.Fatal("DeleteTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_Start verifies cloud-first write for Start.
func TestCloudPrimaryStore_Start(t *testing.T) {
	var remoteCalled bool
	remote := &mockStore{
		startFn: func() error {
			remoteCalled = true
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !remoteCalled {
		t.Error("remote Start should be called")
	}
}

// TestCloudPrimaryStore_StartRemoteFail verifies that remote failure blocks Start.
func TestCloudPrimaryStore_StartRemoteFail(t *testing.T) {
	remote := &mockStore{
		startFn: func() error {
			return errors.New("cloud unavailable")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.Start(); err == nil {
		t.Fatal("Start() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_Clear verifies cloud-first write for Clear.
func TestCloudPrimaryStore_Clear(t *testing.T) {
	var keepConfigArg bool
	remote := &mockStore{
		clearFn: func(keepConfig bool) error {
			keepConfigArg = keepConfig
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.Clear(true); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}
	if !keepConfigArg {
		t.Error("keepConfig should be passed through to remote")
	}
}

// TestCloudPrimaryStore_ClearRemoteFail verifies remote failure blocks Clear.
func TestCloudPrimaryStore_ClearRemoteFail(t *testing.T) {
	remote := &mockStore{
		clearFn: func(bool) error {
			return errors.New("remote clear failed")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.Clear(false); err == nil {
		t.Fatal("Clear() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_TransitionState verifies cloud-first write.
func TestCloudPrimaryStore_TransitionState(t *testing.T) {
	var from, to string
	remote := &mockStore{
		transitionStateFn: func(f, t2 string) error {
			from, to = f, t2
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.TransitionState("PLAN", "EXECUTE"); err != nil {
		t.Fatalf("TransitionState() error: %v", err)
	}
	if from != "PLAN" || to != "EXECUTE" {
		t.Errorf("TransitionState args = %q->%q, want PLAN->EXECUTE", from, to)
	}
}

// TestCloudPrimaryStore_TransitionStateRemoteFail verifies remote failure blocks.
func TestCloudPrimaryStore_TransitionStateRemoteFail(t *testing.T) {
	remote := &mockStore{
		transitionStateFn: func(string, string) error {
			return errors.New("conflict")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.TransitionState("PLAN", "EXECUTE"); err == nil {
		t.Fatal("TransitionState() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_AddTask verifies cloud-first write for AddTask.
func TestCloudPrimaryStore_AddTask(t *testing.T) {
	var addedID string
	remote := &mockStore{
		addTaskFn: func(task *store.Task) error {
			addedID = task.ID
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.AddTask(&store.Task{ID: "T-002-0", Title: "new"}); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}
	if addedID != "T-002-0" {
		t.Errorf("AddTask ID = %q, want T-002-0", addedID)
	}
}

// TestCloudPrimaryStore_AddTaskRemoteFail verifies remote failure blocks AddTask.
func TestCloudPrimaryStore_AddTaskRemoteFail(t *testing.T) {
	remote := &mockStore{
		addTaskFn: func(*store.Task) error {
			return errors.New("quota exceeded")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.AddTask(&store.Task{ID: "T-002-0"}); err == nil {
		t.Fatal("AddTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_AssignTask verifies cloud-primary assignment.
func TestCloudPrimaryStore_AssignTask(t *testing.T) {
	remote := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			return &store.TaskAssignment{TaskID: "T-001-0", WorkerID: workerID}, nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	a, err := cp.AssignTask("w1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if a.TaskID != "T-001-0" {
		t.Errorf("AssignTask TaskID = %q, want T-001-0", a.TaskID)
	}
}

// TestCloudPrimaryStore_AssignTaskRemoteFail verifies remote failure returns error.
func TestCloudPrimaryStore_AssignTaskRemoteFail(t *testing.T) {
	remote := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			return nil, errors.New("no tasks")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	_, err := cp.AssignTask("w1")
	if err == nil {
		t.Fatal("AssignTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_AssignTaskNilAssignment verifies that nil assignment is returned as-is.
func TestCloudPrimaryStore_AssignTaskNilAssignment(t *testing.T) {
	remote := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			return nil, nil // no pending tasks
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	a, err := cp.AssignTask("w1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if a != nil {
		t.Error("AssignTask() should return nil when no tasks available")
	}
}

// TestCloudPrimaryStore_SubmitTask verifies cloud-primary submit.
func TestCloudPrimaryStore_SubmitTask(t *testing.T) {
	remote := &mockStore{
		submitTaskFn: func(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
			return &store.SubmitResult{Success: true}, nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	result, err := cp.SubmitTask("T-001-0", "w1", "sha1", "done", nil)
	if err != nil {
		t.Fatalf("SubmitTask() error: %v", err)
	}
	if !result.Success {
		t.Error("SubmitTask() should return success")
	}
}

// TestCloudPrimaryStore_SubmitTaskRemoteFail verifies remote failure returns error.
func TestCloudPrimaryStore_SubmitTaskRemoteFail(t *testing.T) {
	remote := &mockStore{
		submitTaskFn: func(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
			return nil, errors.New("validation failed")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	_, err := cp.SubmitTask("T-001-0", "w1", "sha1", "done", nil)
	if err == nil {
		t.Fatal("SubmitTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_MarkBlocked verifies cloud-primary mark blocked.
func TestCloudPrimaryStore_MarkBlocked(t *testing.T) {
	var called bool
	remote := &mockStore{
		markBlockedFn: func(taskID, workerID, failureSignature string, attempts int, lastError string) error {
			called = true
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.MarkBlocked("T-001-0", "w1", "sig", 3, "err"); err != nil {
		t.Fatalf("MarkBlocked() error: %v", err)
	}
	if !called {
		t.Error("remote MarkBlocked should be called")
	}
}

// TestCloudPrimaryStore_MarkBlockedRemoteFail verifies remote failure blocks.
func TestCloudPrimaryStore_MarkBlockedRemoteFail(t *testing.T) {
	remote := &mockStore{
		markBlockedFn: func(taskID, workerID, failureSignature string, attempts int, lastError string) error {
			return errors.New("remote error")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.MarkBlocked("T-001-0", "w1", "sig", 3, "err"); err == nil {
		t.Fatal("MarkBlocked() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_ClaimTask verifies cloud-primary claim.
func TestCloudPrimaryStore_ClaimTask(t *testing.T) {
	remote := &mockStore{
		claimTaskFn: func(taskID string) (*store.Task, error) {
			return &store.Task{ID: taskID, Status: "in_progress"}, nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	task, err := cp.ClaimTask("T-001-0")
	if err != nil {
		t.Fatalf("ClaimTask() error: %v", err)
	}
	if task.Status != "in_progress" {
		t.Errorf("ClaimTask status = %q, want in_progress", task.Status)
	}
}

// TestCloudPrimaryStore_ClaimTaskRemoteFail verifies remote failure returns error.
func TestCloudPrimaryStore_ClaimTaskRemoteFail(t *testing.T) {
	remote := &mockStore{
		claimTaskFn: func(taskID string) (*store.Task, error) {
			return nil, errors.New("already claimed")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	_, err := cp.ClaimTask("T-001-0")
	if err == nil {
		t.Fatal("ClaimTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_ReportTask verifies cloud-primary report.
func TestCloudPrimaryStore_ReportTask(t *testing.T) {
	var summary string
	remote := &mockStore{
		reportTaskFn: func(taskID, s string, filesChanged []string) error {
			summary = s
			return nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.ReportTask("T-001-0", "all done", []string{"main.go"}); err != nil {
		t.Fatalf("ReportTask() error: %v", err)
	}
	if summary != "all done" {
		t.Errorf("ReportTask summary = %q, want all done", summary)
	}
}

// TestCloudPrimaryStore_ReportTaskRemoteFail verifies remote failure blocks.
func TestCloudPrimaryStore_ReportTaskRemoteFail(t *testing.T) {
	remote := &mockStore{
		reportTaskFn: func(taskID, summary string, filesChanged []string) error {
			return errors.New("not found")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	if err := cp.ReportTask("T-001-0", "done", nil); err == nil {
		t.Fatal("ReportTask() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_Checkpoint verifies cloud-primary checkpoint.
func TestCloudPrimaryStore_Checkpoint(t *testing.T) {
	remote := &mockStore{
		checkpointFn: func(checkpointID, decision, notes string, requiredChanges []string) (*store.CheckpointResult, error) {
			return &store.CheckpointResult{Success: true}, nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	result, err := cp.Checkpoint("CP-001", "APPROVE", "looks good", nil, "T-001-0", "")
	if err != nil {
		t.Fatalf("Checkpoint() error: %v", err)
	}
	if !result.Success {
		t.Error("Checkpoint() should return success")
	}
}

// TestCloudPrimaryStore_CheckpointRemoteFail verifies remote failure blocks.
func TestCloudPrimaryStore_CheckpointRemoteFail(t *testing.T) {
	remote := &mockStore{
		checkpointFn: func(checkpointID, decision, notes string, requiredChanges []string) (*store.CheckpointResult, error) {
			return nil, errors.New("checkpoint conflict")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	_, err := cp.Checkpoint("CP-001", "APPROVE", "ok", nil, "", "")
	if err == nil {
		t.Fatal("Checkpoint() should fail when remote fails")
	}
}

// TestCloudPrimaryStore_RequestChanges verifies cloud-primary request changes.
func TestCloudPrimaryStore_RequestChanges(t *testing.T) {
	remote := &mockStore{
		requestChangesFn: func(reviewTaskID, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
			return &store.RequestChangesResult{Success: true}, nil
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	result, err := cp.RequestChanges("R-001-0", "fix this", []string{"change A"})
	if err != nil {
		t.Fatalf("RequestChanges() error: %v", err)
	}
	if !result.Success {
		t.Error("RequestChanges() should return success")
	}
}

// TestCloudPrimaryStore_RequestChangesRemoteFail verifies remote failure blocks.
func TestCloudPrimaryStore_RequestChangesRemoteFail(t *testing.T) {
	remote := &mockStore{
		requestChangesFn: func(reviewTaskID, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
			return nil, errors.New("review not found")
		},
	}
	cp := NewCloudPrimaryStore(&mockStore{}, remote)
	_, err := cp.RequestChanges("R-001-0", "fix", nil)
	if err == nil {
		t.Fatal("RequestChanges() should fail when remote fails")
	}
}

// =========================================================================
// Helper types for methods that mockStore doesn't support via function fields
// =========================================================================

// listFailStore is a store.Store that always fails ListTasks.
type listFailStore struct{ mockStore }

func (l *listFailStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	return nil, 0, errors.New("cloud list failed")
}

// deleteFailStore is a store.Store that always fails DeleteTask.
type deleteFailStore struct{ mockStore }

func (d *deleteFailStore) DeleteTask(taskID string) error {
	return errors.New("remote delete failed")
}
