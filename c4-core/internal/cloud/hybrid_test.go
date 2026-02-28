package cloud

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/store"
)

// Compile-time assertion: mockStore must fully implement store.Store.
var _ store.Store = (*mockStore)(nil)

// mockStore implements store.Store for testing.
type mockStore struct {
	statusFn          func() (*store.ProjectStatus, error)
	startFn           func() error
	clearFn           func(bool) error
	transitionStateFn func(string, string) error
	addTaskFn         func(*store.Task) error
	getTaskFn         func(string) (*store.Task, error)
	assignTaskFn      func(string) (*store.TaskAssignment, error)
	submitTaskFn      func(string, string, string, string, []store.ValidationResult) (*store.SubmitResult, error)
	markBlockedFn     func(string, string, string, int, string) error
	claimTaskFn       func(string) (*store.Task, error)
	reportTaskFn      func(string, string, []string) error
	checkpointFn      func(string, string, string, []string) (*store.CheckpointResult, error)
	requestChangesFn  func(string, string, []string) (*store.RequestChangesResult, error)
}

func (m *mockStore) GetStatus() (*store.ProjectStatus, error) {
	if m.statusFn != nil {
		return m.statusFn()
	}
	return &store.ProjectStatus{State: "EXECUTE"}, nil
}

func (m *mockStore) Start() error {
	if m.startFn != nil {
		return m.startFn()
	}
	return nil
}

func (m *mockStore) Clear(keepConfig bool) error {
	if m.clearFn != nil {
		return m.clearFn(keepConfig)
	}
	return nil
}

func (m *mockStore) TransitionState(from, to string) error {
	if m.transitionStateFn != nil {
		return m.transitionStateFn(from, to)
	}
	return nil
}

func (m *mockStore) AddTask(task *store.Task) error {
	if m.addTaskFn != nil {
		return m.addTaskFn(task)
	}
	return nil
}

func (m *mockStore) DeleteTask(taskID string) error { return nil }

func (m *mockStore) ListTasks(filter store.TaskFilter) ([]store.Task, int, error) {
	return nil, 0, nil
}

func (m *mockStore) GetTask(taskID string) (*store.Task, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(taskID)
	}
	return &store.Task{ID: taskID, Title: "test"}, nil
}

func (m *mockStore) AssignTask(workerID string) (*store.TaskAssignment, error) {
	if m.assignTaskFn != nil {
		return m.assignTaskFn(workerID)
	}
	return &store.TaskAssignment{TaskID: "T-001-0", WorkerID: workerID}, nil
}

func (m *mockStore) SubmitTask(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
	if m.submitTaskFn != nil {
		return m.submitTaskFn(taskID, workerID, commitSHA, handoff, results)
	}
	return &store.SubmitResult{Success: true}, nil
}

func (m *mockStore) MarkBlocked(taskID, workerID, failureSignature string, attempts int, lastError string) error {
	if m.markBlockedFn != nil {
		return m.markBlockedFn(taskID, workerID, failureSignature, attempts, lastError)
	}
	return nil
}

func (m *mockStore) ClaimTask(taskID string) (*store.Task, error) {
	if m.claimTaskFn != nil {
		return m.claimTaskFn(taskID)
	}
	return &store.Task{ID: taskID, Status: "in_progress"}, nil
}

func (m *mockStore) ReportTask(taskID, summary string, filesChanged []string) error {
	if m.reportTaskFn != nil {
		return m.reportTaskFn(taskID, summary, filesChanged)
	}
	return nil
}

func (m *mockStore) Checkpoint(checkpointID, decision, notes string, requiredChanges []string, targetTaskID, targetReviewID string) (*store.CheckpointResult, error) {
	if m.checkpointFn != nil {
		return m.checkpointFn(checkpointID, decision, notes, requiredChanges)
	}
	return &store.CheckpointResult{Success: true}, nil
}

func (m *mockStore) RequestChanges(reviewTaskID string, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
	if m.requestChangesFn != nil {
		return m.requestChangesFn(reviewTaskID, comments, requiredChanges)
	}
	return &store.RequestChangesResult{Success: true}, nil
}

func TestHybridStoreReadsFromLocal(t *testing.T) {
	local := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return &store.ProjectStatus{State: "EXECUTE", TotalTasks: 5}, nil
		},
	}
	remote := &mockStore{
		statusFn: func() (*store.ProjectStatus, error) {
			return &store.ProjectStatus{State: "PLAN", TotalTasks: 99}, nil
		},
	}

	hybrid := NewHybridStore(local, remote)

	status, err := hybrid.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if status.State != "EXECUTE" {
		t.Errorf("State = %q, want %q (should read from local)", status.State, "EXECUTE")
	}
	if status.TotalTasks != 5 {
		t.Errorf("TotalTasks = %d, want %d", status.TotalTasks, 5)
	}
}

func TestHybridStoreWritesLocalFirst(t *testing.T) {
	var localCalled, remoteCalled atomic.Int32

	local := &mockStore{
		addTaskFn: func(task *store.Task) error {
			localCalled.Add(1)
			return nil
		},
	}
	remote := &mockStore{
		addTaskFn: func(task *store.Task) error {
			remoteCalled.Add(1)
			return nil
		},
	}

	hybrid := NewHybridStore(local, remote)

	err := hybrid.AddTask(&store.Task{ID: "T-001-0", Title: "test"})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	// Local should be called immediately
	if localCalled.Load() != 1 {
		t.Errorf("local AddTask called %d times, want 1", localCalled.Load())
	}

	// Wait for async cloud push
	time.Sleep(50 * time.Millisecond)

	if remoteCalled.Load() != 1 {
		t.Errorf("remote AddTask called %d times, want 1", remoteCalled.Load())
	}
}

func TestHybridStoreCloudFailureNonFatal(t *testing.T) {
	local := &mockStore{}
	remote := &mockStore{
		addTaskFn: func(task *store.Task) error {
			return errors.New("cloud unavailable")
		},
	}

	hybrid := NewHybridStore(local, remote)

	// Should succeed despite cloud failure
	err := hybrid.AddTask(&store.Task{ID: "T-001-0", Title: "test"})
	if err != nil {
		t.Fatalf("AddTask() should not fail on cloud error: %v", err)
	}

	// Wait for async goroutine
	time.Sleep(50 * time.Millisecond)

	if hybrid.CloudFailures() != 1 {
		t.Errorf("CloudFailures() = %d, want 1", hybrid.CloudFailures())
	}
}

func TestHybridStoreLocalFailureBlocks(t *testing.T) {
	local := &mockStore{
		addTaskFn: func(task *store.Task) error {
			return errors.New("disk full")
		},
	}
	remote := &mockStore{}

	hybrid := NewHybridStore(local, remote)

	err := hybrid.AddTask(&store.Task{ID: "T-001-0", Title: "test"})
	if err == nil {
		t.Fatal("AddTask() should fail when local store fails")
	}
}

func TestHybridStoreAllWriteOperations(t *testing.T) {
	var count atomic.Int32

	recordOp := func(_ string) {
		count.Add(1)
	}

	local := &mockStore{
		startFn: func() error { recordOp("local_start"); return nil },
		clearFn: func(bool) error { recordOp("local_clear"); return nil },
	}
	remote := &mockStore{
		startFn: func() error { recordOp("remote_start"); return nil },
		clearFn: func(bool) error { recordOp("remote_clear"); return nil },
	}

	hybrid := NewHybridStore(local, remote)

	if err := hybrid.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if err := hybrid.Clear(false); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Wait for async
	time.Sleep(50 * time.Millisecond)

	// Should have called both local and remote for both operations
	if count.Load() < 4 {
		t.Errorf("Expected at least 4 operations, got %d", count.Load())
	}
}

func TestHybridStoreClaimReportCycle(t *testing.T) {
	var claimCount, reportCount atomic.Int32

	local := &mockStore{}
	remote := &mockStore{
		claimTaskFn: func(string) (*store.Task, error) {
			claimCount.Add(1)
			return &store.Task{}, nil
		},
		reportTaskFn: func(string, string, []string) error {
			reportCount.Add(1)
			return nil
		},
	}

	hybrid := NewHybridStore(local, remote)

	task, err := hybrid.ClaimTask("T-001-0")
	if err != nil {
		t.Fatalf("ClaimTask() error: %v", err)
	}
	if task == nil {
		t.Fatal("ClaimTask() returned nil")
	}

	err = hybrid.ReportTask("T-001-0", "done", []string{"file.go"})
	if err != nil {
		t.Fatalf("ReportTask() error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if claimCount.Load() != 1 {
		t.Errorf("remote ClaimTask called %d times, want 1", claimCount.Load())
	}
	if reportCount.Load() != 1 {
		t.Errorf("remote ReportTask called %d times, want 1", reportCount.Load())
	}
}

func TestHybridStoreLocal(t *testing.T) {
	local := &mockStore{}
	remote := &mockStore{}
	hybrid := NewHybridStore(local, remote)
	if hybrid.Local() != local {
		t.Error("Local() should return the underlying local store")
	}
}

func TestHybridStoreGetTask(t *testing.T) {
	local := &mockStore{
		getTaskFn: func(id string) (*store.Task, error) {
			return &store.Task{ID: id, Title: "found"}, nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	task, err := hybrid.GetTask("T-001-0")
	if err != nil {
		t.Fatalf("GetTask() error: %v", err)
	}
	if task.Title != "found" {
		t.Errorf("GetTask() title = %q, want %q", task.Title, "found")
	}
}

func TestHybridStoreListTasks(t *testing.T) {
	local := &mockStore{}
	hybrid := NewHybridStore(local, &mockStore{})
	tasks, total, err := hybrid.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks() error: %v", err)
	}
	if total != 0 || len(tasks) != 0 {
		t.Errorf("ListTasks() = %v, %d, want [], 0", tasks, total)
	}
}

func TestHybridStoreTransitionState(t *testing.T) {
	var called string
	local := &mockStore{
		transitionStateFn: func(from, to string) error {
			called = from + "->" + to
			return nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.TransitionState("PLAN", "EXECUTE"); err != nil {
		t.Fatalf("TransitionState() error: %v", err)
	}
	if called != "PLAN->EXECUTE" {
		t.Errorf("TransitionState called with %q, want PLAN->EXECUTE", called)
	}
}

func TestHybridStoreTransitionStateLocalFail(t *testing.T) {
	local := &mockStore{
		transitionStateFn: func(from, to string) error {
			return errors.New("lock error")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.TransitionState("PLAN", "EXECUTE"); err == nil {
		t.Fatal("TransitionState() should fail when local fails")
	}
}

func TestHybridStoreAssignTask(t *testing.T) {
	local := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			return &store.TaskAssignment{TaskID: "T-001-0", WorkerID: workerID}, nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	a, err := hybrid.AssignTask("w1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if a.TaskID != "T-001-0" {
		t.Errorf("AssignTask() TaskID = %q, want T-001-0", a.TaskID)
	}
}

func TestHybridStoreAssignTaskNilAssignment(t *testing.T) {
	// When local returns nil assignment (no pending tasks), cloud should not be called.
	local := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			return nil, nil
		},
	}
	var cloudCalled bool
	remote := &mockStore{
		assignTaskFn: func(workerID string) (*store.TaskAssignment, error) {
			cloudCalled = true
			return nil, nil
		},
	}
	hybrid := NewHybridStore(local, remote)
	a, err := hybrid.AssignTask("w1")
	if err != nil {
		t.Fatalf("AssignTask() error: %v", err)
	}
	if a != nil {
		t.Errorf("AssignTask() should return nil when local returns nil")
	}
	if cloudCalled {
		t.Error("cloud should not be called when assignment is nil")
	}
}

func TestHybridStoreSubmitTask(t *testing.T) {
	local := &mockStore{
		submitTaskFn: func(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
			return &store.SubmitResult{Success: true}, nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	result, err := hybrid.SubmitTask("T-001-0", "w1", "abc123", "done", nil)
	if err != nil {
		t.Fatalf("SubmitTask() error: %v", err)
	}
	if !result.Success {
		t.Error("SubmitTask() should return success")
	}
}

func TestHybridStoreSubmitTaskLocalFail(t *testing.T) {
	local := &mockStore{
		submitTaskFn: func(taskID, workerID, commitSHA, handoff string, results []store.ValidationResult) (*store.SubmitResult, error) {
			return nil, errors.New("db error")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	_, err := hybrid.SubmitTask("T-001-0", "w1", "abc123", "done", nil)
	if err == nil {
		t.Fatal("SubmitTask() should fail when local fails")
	}
}

func TestHybridStoreMarkBlocked(t *testing.T) {
	var called bool
	local := &mockStore{
		markBlockedFn: func(taskID, workerID, failureSignature string, attempts int, lastError string) error {
			called = true
			return nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.MarkBlocked("T-001-0", "w1", "sig", 3, "err"); err != nil {
		t.Fatalf("MarkBlocked() error: %v", err)
	}
	if !called {
		t.Error("local MarkBlocked should be called")
	}
}

func TestHybridStoreMarkBlockedLocalFail(t *testing.T) {
	local := &mockStore{
		markBlockedFn: func(taskID, workerID, failureSignature string, attempts int, lastError string) error {
			return errors.New("db locked")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.MarkBlocked("T-001-0", "w1", "sig", 3, "err"); err == nil {
		t.Fatal("MarkBlocked() should fail when local fails")
	}
}

func TestHybridStoreCheckpoint(t *testing.T) {
	local := &mockStore{
		checkpointFn: func(checkpointID, decision, notes string, requiredChanges []string) (*store.CheckpointResult, error) {
			return &store.CheckpointResult{Success: true}, nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	result, err := hybrid.Checkpoint("CP-001", "APPROVE", "ok", nil, "", "")
	if err != nil {
		t.Fatalf("Checkpoint() error: %v", err)
	}
	if !result.Success {
		t.Error("Checkpoint() should return success")
	}
}

func TestHybridStoreCheckpointLocalFail(t *testing.T) {
	local := &mockStore{
		checkpointFn: func(checkpointID, decision, notes string, requiredChanges []string) (*store.CheckpointResult, error) {
			return nil, errors.New("state conflict")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	_, err := hybrid.Checkpoint("CP-001", "APPROVE", "ok", nil, "", "")
	if err == nil {
		t.Fatal("Checkpoint() should fail when local fails")
	}
}

func TestHybridStoreRequestChanges(t *testing.T) {
	local := &mockStore{
		requestChangesFn: func(reviewTaskID, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
			return &store.RequestChangesResult{Success: true}, nil
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	result, err := hybrid.RequestChanges("R-001-0", "needs fix", []string{"fix A"})
	if err != nil {
		t.Fatalf("RequestChanges() error: %v", err)
	}
	if !result.Success {
		t.Error("RequestChanges() should return success")
	}
}

func TestHybridStoreRequestChangesLocalFail(t *testing.T) {
	local := &mockStore{
		requestChangesFn: func(reviewTaskID, comments string, requiredChanges []string) (*store.RequestChangesResult, error) {
			return nil, errors.New("not found")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	_, err := hybrid.RequestChanges("R-001-0", "needs fix", []string{"fix A"})
	if err == nil {
		t.Fatal("RequestChanges() should fail when local fails")
	}
}

func TestHybridStoreDeleteTask(t *testing.T) {
	var localCalled bool
	local := &mockStore{
		// DeleteTask is defined as simple no-op in mockStore, we need to intercept it
	}
	_ = local
	local2 := &mockStore{}
	// Override DeleteTask with a direct approach via an embedded struct is not possible,
	// but since mockStore.DeleteTask is always no-op and returns nil, we just verify no error.
	hybrid := NewHybridStore(local2, &mockStore{})
	if err := hybrid.DeleteTask("T-001-0"); err != nil {
		t.Fatalf("DeleteTask() error: %v", err)
	}
	_ = localCalled
}

func TestHybridStoreDeleteTaskLocalFail(t *testing.T) {
	// When local DeleteTask fails, the error should be returned immediately.
	local := &deleteFailLocalStore{}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.DeleteTask("T-001-0"); err == nil {
		t.Fatal("DeleteTask() should fail when local fails")
	}
}

func TestHybridStoreStartLocalFail(t *testing.T) {
	local := &mockStore{
		startFn: func() error {
			return errors.New("start failed")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.Start(); err == nil {
		t.Fatal("Start() should fail when local fails")
	}
}

func TestHybridStoreClearLocalFail(t *testing.T) {
	local := &mockStore{
		clearFn: func(bool) error {
			return errors.New("clear failed")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.Clear(false); err == nil {
		t.Fatal("Clear() should fail when local fails")
	}
}

func TestHybridStoreReportTaskLocalFail(t *testing.T) {
	local := &mockStore{
		reportTaskFn: func(string, string, []string) error {
			return errors.New("report failed")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	if err := hybrid.ReportTask("T-001-0", "done", nil); err == nil {
		t.Fatal("ReportTask() should fail when local fails")
	}
}

func TestHybridStoreAssignTaskLocalFail(t *testing.T) {
	local := &mockStore{
		assignTaskFn: func(string) (*store.TaskAssignment, error) {
			return nil, errors.New("assign failed")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	_, err := hybrid.AssignTask("w1")
	if err == nil {
		t.Fatal("AssignTask() should fail when local fails")
	}
}

func TestHybridStoreClaimTaskLocalFail(t *testing.T) {
	local := &mockStore{
		claimTaskFn: func(string) (*store.Task, error) {
			return nil, errors.New("claim failed")
		},
	}
	hybrid := NewHybridStore(local, &mockStore{})
	_, err := hybrid.ClaimTask("T-001-0")
	if err == nil {
		t.Fatal("ClaimTask() should fail when local fails")
	}
}

func TestHybridStoreFailureCountReset(t *testing.T) {
	callCount := atomic.Int32{}

	remote := &mockStore{
		addTaskFn: func(task *store.Task) error {
			n := callCount.Add(1)
			if n == 1 {
				return errors.New("temporary failure")
			}
			return nil // second call succeeds
		},
	}

	hybrid := NewHybridStore(&mockStore{}, remote)

	// First call — cloud fails
	hybrid.AddTask(&store.Task{ID: "T-001-0"})
	time.Sleep(50 * time.Millisecond)
	if hybrid.CloudFailures() != 1 {
		t.Errorf("CloudFailures() = %d, want 1", hybrid.CloudFailures())
	}

	// Second call — cloud succeeds, counter resets
	hybrid.AddTask(&store.Task{ID: "T-002-0"})
	time.Sleep(50 * time.Millisecond)
	if hybrid.CloudFailures() != 0 {
		t.Errorf("CloudFailures() = %d, want 0 (should reset on success)", hybrid.CloudFailures())
	}
}

// deleteFailLocalStore is a store.Store whose DeleteTask always fails.
type deleteFailLocalStore struct{ mockStore }

func (d *deleteFailLocalStore) DeleteTask(taskID string) error {
	return errors.New("local delete failed")
}
