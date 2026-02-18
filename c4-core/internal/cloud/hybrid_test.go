package cloud

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/store"
)

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
