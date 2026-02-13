package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetJob(t *testing.T) {
	s := tempStore(t)

	req := &JobSubmitRequest{
		Name:    "train-model",
		Workdir: "/workspace",
		Command: "python train.py --lr 0.001",
		Env:     map[string]string{"CUDA_VISIBLE_DEVICES": "0"},
		Tags:    []string{"ml", "train"},
		ExpID:   "exp-001",
		Memo:    "baseline run",
	}

	job, err := s.CreateJob(req)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if job.Status != StatusQueued {
		t.Errorf("status = %s, want QUEUED", job.Status)
	}
	if job.Name != "train-model" {
		t.Errorf("name = %s, want train-model", job.Name)
	}

	got, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Name != "train-model" {
		t.Errorf("got.Name = %s", got.Name)
	}
	if got.Workdir != "/workspace" {
		t.Errorf("got.Workdir = %s", got.Workdir)
	}
	if got.Command != "python train.py --lr 0.001" {
		t.Errorf("got.Command = %s", got.Command)
	}
	if got.Env["CUDA_VISIBLE_DEVICES"] != "0" {
		t.Errorf("got.Env = %v", got.Env)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "ml" {
		t.Errorf("got.Tags = %v", got.Tags)
	}
	if got.ExpID != "exp-001" {
		t.Errorf("got.ExpID = %s", got.ExpID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	s := tempStore(t)
	_, err := s.GetJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestListJobs_FilterByStatus(t *testing.T) {
	s := tempStore(t)

	// Create 3 jobs
	for i := 0; i < 3; i++ {
		s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	}

	// All jobs should be QUEUED
	all, err := s.ListJobs("", 0, 0)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}

	// Filter by QUEUED
	queued, err := s.ListJobs("QUEUED", 0, 0)
	if err != nil {
		t.Fatalf("ListJobs QUEUED: %v", err)
	}
	if len(queued) != 3 {
		t.Errorf("len(queued) = %d, want 3", len(queued))
	}

	// Filter by RUNNING (should be empty)
	running, err := s.ListJobs("RUNNING", 0, 0)
	if err != nil {
		t.Fatalf("ListJobs RUNNING: %v", err)
	}
	if len(running) != 0 {
		t.Errorf("len(running) = %d, want 0", len(running))
	}
}

func TestListJobs_LimitOffset(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 5; i++ {
		s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	}

	// Limit to 2
	limited, err := s.ListJobs("", 2, 0)
	if err != nil {
		t.Fatalf("ListJobs limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("len(limited) = %d, want 2", len(limited))
	}

	// Offset 3, limit 10
	offset, err := s.ListJobs("", 10, 3)
	if err != nil {
		t.Fatalf("ListJobs offset: %v", err)
	}
	if len(offset) != 2 {
		t.Errorf("len(offset) = %d, want 2", len(offset))
	}
}

func TestStartJob(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})

	err := s.StartJob(job.ID, 12345, []int{0, 1})
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != StatusRunning {
		t.Errorf("status = %s, want RUNNING", got.Status)
	}
	if got.PID != 12345 {
		t.Errorf("pid = %d, want 12345", got.PID)
	}
	if got.StartedAt == nil {
		t.Error("started_at should be set")
	}
	if len(got.GPUIndices) != 2 {
		t.Errorf("gpu_indices = %v, want [0, 1]", got.GPUIndices)
	}
}

func TestStartJob_NotQueued(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	s.StartJob(job.ID, 1, nil) // → RUNNING

	// Try to start again → should fail
	err := s.StartJob(job.ID, 2, nil)
	if err == nil {
		t.Fatal("expected error starting already-running job")
	}
}

func TestCompleteJob(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	s.StartJob(job.ID, 1, nil)

	err := s.CompleteJob(job.ID, StatusSucceeded, 0)
	if err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != StatusSucceeded {
		t.Errorf("status = %s, want SUCCEEDED", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit_code = %v, want 0", got.ExitCode)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at should be set")
	}
	if got.PID != 0 {
		t.Errorf("pid = %d, want 0 (cleared)", got.PID)
	}
}

func TestCompleteJob_RecordsDuration(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	s.StartJob(job.ID, 1, nil)
	s.CompleteJob(job.ID, StatusSucceeded, 0)

	durations, err := s.GetDurations(job.CommandHash(), 10)
	if err != nil {
		t.Fatalf("GetDurations: %v", err)
	}
	if len(durations) != 1 {
		t.Errorf("len(durations) = %d, want 1", len(durations))
	}
}

func TestCompleteJob_FailedNoDuration(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo fail", Workdir: "."})
	s.StartJob(job.ID, 1, nil)
	s.CompleteJob(job.ID, StatusFailed, 1)

	durations, _ := s.GetDurations(job.CommandHash(), 10)
	if len(durations) != 0 {
		t.Errorf("failed job should not record duration, got %d", len(durations))
	}
}

func TestCancelJob_Queued(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})

	err := s.CancelJob(job.ID)
	if err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != StatusCancelled {
		t.Errorf("status = %s, want CANCELLED", got.Status)
	}
}

func TestCancelJob_Running(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	s.StartJob(job.ID, 1, nil)

	err := s.CancelJob(job.ID)
	if err != nil {
		t.Fatalf("CancelJob running: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != StatusCancelled {
		t.Errorf("status = %s, want CANCELLED", got.Status)
	}
}

func TestCancelJob_AlreadyTerminal(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{Name: "job", Command: "echo hi", Workdir: "."})
	s.StartJob(job.ID, 1, nil)
	s.CompleteJob(job.ID, StatusSucceeded, 0)

	err := s.CancelJob(job.ID)
	if err == nil {
		t.Fatal("expected error cancelling completed job")
	}
}

func TestGetQueuedJobs_PriorityOrder(t *testing.T) {
	s := tempStore(t)

	// Create jobs with different priorities
	s.CreateJob(&JobSubmitRequest{Name: "low", Command: "echo low", Workdir: ".", Priority: 1})
	s.CreateJob(&JobSubmitRequest{Name: "high", Command: "echo high", Workdir: ".", Priority: 10})
	s.CreateJob(&JobSubmitRequest{Name: "mid", Command: "echo mid", Workdir: ".", Priority: 5})

	queued, err := s.GetQueuedJobs()
	if err != nil {
		t.Fatalf("GetQueuedJobs: %v", err)
	}
	if len(queued) != 3 {
		t.Fatalf("len = %d, want 3", len(queued))
	}
	if queued[0].Name != "high" {
		t.Errorf("first = %s, want high (priority 10)", queued[0].Name)
	}
	if queued[1].Name != "mid" {
		t.Errorf("second = %s, want mid (priority 5)", queued[1].Name)
	}
	if queued[2].Name != "low" {
		t.Errorf("third = %s, want low (priority 1)", queued[2].Name)
	}
}

func TestGetRunningJobs(t *testing.T) {
	s := tempStore(t)

	job1, _ := s.CreateJob(&JobSubmitRequest{Name: "r1", Command: "echo 1", Workdir: "."})
	job2, _ := s.CreateJob(&JobSubmitRequest{Name: "r2", Command: "echo 2", Workdir: "."})
	s.CreateJob(&JobSubmitRequest{Name: "q1", Command: "echo 3", Workdir: "."})

	s.StartJob(job1.ID, 100, nil)
	s.StartJob(job2.ID, 200, nil)

	running, err := s.GetRunningJobs()
	if err != nil {
		t.Fatalf("GetRunningJobs: %v", err)
	}
	if len(running) != 2 {
		t.Errorf("len = %d, want 2", len(running))
	}
}

func TestGetQueueStats(t *testing.T) {
	s := tempStore(t)

	// 2 queued
	s.CreateJob(&JobSubmitRequest{Name: "q1", Command: "echo 1", Workdir: "."})
	s.CreateJob(&JobSubmitRequest{Name: "q2", Command: "echo 2", Workdir: "."})

	// 1 running
	j3, _ := s.CreateJob(&JobSubmitRequest{Name: "r1", Command: "echo 3", Workdir: "."})
	s.StartJob(j3.ID, 1, nil)

	// 1 succeeded
	j4, _ := s.CreateJob(&JobSubmitRequest{Name: "s1", Command: "echo 4", Workdir: "."})
	s.StartJob(j4.ID, 2, nil)
	s.CompleteJob(j4.ID, StatusSucceeded, 0)

	// 1 failed
	j5, _ := s.CreateJob(&JobSubmitRequest{Name: "f1", Command: "echo 5", Workdir: "."})
	s.StartJob(j5.ID, 3, nil)
	s.CompleteJob(j5.ID, StatusFailed, 1)

	stats, err := s.GetQueueStats()
	if err != nil {
		t.Fatalf("GetQueueStats: %v", err)
	}
	if stats.Queued != 2 {
		t.Errorf("Queued = %d, want 2", stats.Queued)
	}
	if stats.Running != 1 {
		t.Errorf("Running = %d, want 1", stats.Running)
	}
	if stats.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", stats.Succeeded)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
}

func TestNewStore_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "dir", "daemon.db")

	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s.Close()

	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

func TestCountByStatus(t *testing.T) {
	s := tempStore(t)

	s.CreateJob(&JobSubmitRequest{Name: "q1", Command: "echo 1", Workdir: "."})
	s.CreateJob(&JobSubmitRequest{Name: "q2", Command: "echo 2", Workdir: "."})

	count, err := s.CountByStatus(StatusQueued)
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestJob_DurationSec(t *testing.T) {
	s := tempStore(t)
	job, _ := s.CreateJob(&JobSubmitRequest{Name: "dur", Command: "sleep 1", Workdir: "."})

	// No duration before start
	if job.DurationSec() != nil {
		t.Error("expected nil duration before start")
	}

	s.StartJob(job.ID, 1, nil)
	s.CompleteJob(job.ID, StatusSucceeded, 0)

	got, _ := s.GetJob(job.ID)
	dur := got.DurationSec()
	if dur == nil {
		t.Fatal("expected non-nil duration after completion")
	}
	// Duration should be very small (near 0) since start → complete is instant
	if *dur < 0 {
		t.Errorf("duration = %f, want >= 0", *dur)
	}
}

func TestJob_CommandHash(t *testing.T) {
	j := &Job{Command: "python train.py --lr 0.001"}
	hash := j.CommandHash()
	if len(hash) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("hash length = %d, want 16", len(hash))
	}

	// Same command → same hash
	j2 := &Job{Command: "python train.py --lr 0.001"}
	if j.CommandHash() != j2.CommandHash() {
		t.Error("same command should produce same hash")
	}

	// Different command → different hash
	j3 := &Job{Command: "python eval.py"}
	if j.CommandHash() == j3.CommandHash() {
		t.Error("different commands should produce different hashes")
	}
}

func TestJobStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status JobStatus
		want   bool
	}{
		{StatusQueued, false},
		{StatusRunning, false},
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusCancelled, true},
	}
	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.want {
			t.Errorf("IsTerminal(%s) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestGPUJob(t *testing.T) {
	s := tempStore(t)

	job, _ := s.CreateJob(&JobSubmitRequest{
		Name:        "gpu-train",
		Command:     "python train.py",
		Workdir:     "/workspace",
		RequiresGPU: true,
		GPUCount:    2,
	})

	got, _ := s.GetJob(job.ID)
	if !got.RequiresGPU {
		t.Error("requires_gpu should be true")
	}
	if got.GPUCount != 2 {
		t.Errorf("gpu_count = %d, want 2", got.GPUCount)
	}
}
