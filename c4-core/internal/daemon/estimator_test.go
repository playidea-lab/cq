package daemon

import (
	"path/filepath"
	"testing"
)

func TestNormalizeCommandHash_SameBase(t *testing.T) {
	// Same base command with different seeds → same hash
	h1 := NormalizeCommandHash("python train.py --seed 42 --epochs 100")
	h2 := NormalizeCommandHash("python train.py --seed 123 --epochs 100")
	if h1 != h2 {
		t.Errorf("different seeds should produce same hash: %s != %s", h1, h2)
	}
}

func TestNormalizeCommandHash_DifferentCommand(t *testing.T) {
	h1 := NormalizeCommandHash("python train.py --epochs 100")
	h2 := NormalizeCommandHash("python eval.py --epochs 100")
	if h1 == h2 {
		t.Error("different commands should produce different hashes")
	}
}

func TestNormalizeCommandHash_Timestamps(t *testing.T) {
	h1 := NormalizeCommandHash("python train.py --run-dir /results/2026-02-13T14:30")
	h2 := NormalizeCommandHash("python train.py --run-dir /results/2026-02-12T10:00")
	if h1 != h2 {
		t.Errorf("different timestamps should produce same hash: %s != %s", h1, h2)
	}
}

func TestNormalizeCommandHash_TmpPaths(t *testing.T) {
	h1 := NormalizeCommandHash("python train.py --output /tmp/abc123")
	h2 := NormalizeCommandHash("python train.py --output /tmp/xyz789")
	if h1 != h2 {
		t.Errorf("different tmp paths should produce same hash: %s != %s", h1, h2)
	}
}

func TestNormalizeCommandHash_RunIDs(t *testing.T) {
	h1 := NormalizeCommandHash("python train.py --exp exp_abc123def")
	h2 := NormalizeCommandHash("python train.py --exp exp_xyz789ghi")
	if h1 != h2 {
		t.Errorf("different run IDs should produce same hash: %s != %s", h1, h2)
	}
}

func TestEstimator_Historical(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	// Create 3+ completed jobs with same command hash
	cmd := "python train.py --epochs 100"
	for i := 0; i < 4; i++ {
		job, _ := store.CreateJob(&JobSubmitRequest{Name: "j", Command: cmd, Workdir: "."})
		store.StartJob(job.ID, 1, nil)
		store.CompleteJob(job.ID, StatusSucceeded, 0)
	}

	testJob := &Job{Command: cmd, Status: StatusQueued}
	result := est.Estimate(testJob)

	if result.Method != "historical" {
		t.Errorf("method = %s, want historical", result.Method)
	}
	if result.Confidence != 0.8 {
		t.Errorf("confidence = %f, want 0.8", result.Confidence)
	}
}

func TestEstimator_SimilarJobs(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	// Create 1-2 completed jobs (not enough for historical)
	cmd := "python eval.py"
	job, _ := store.CreateJob(&JobSubmitRequest{Name: "j", Command: cmd, Workdir: "."})
	store.StartJob(job.ID, 1, nil)
	store.CompleteJob(job.ID, StatusSucceeded, 0)

	testJob := &Job{Command: cmd, Status: StatusQueued}
	result := est.Estimate(testJob)

	if result.Method != "similar_jobs" {
		t.Errorf("method = %s, want similar_jobs", result.Method)
	}
	if result.Confidence != 0.5 {
		t.Errorf("confidence = %f, want 0.5", result.Confidence)
	}
}

func TestEstimator_GlobalAvg(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	// Create jobs with a different command
	otherCmd := "python other.py"
	job, _ := store.CreateJob(&JobSubmitRequest{Name: "j", Command: otherCmd, Workdir: "."})
	store.StartJob(job.ID, 1, nil)
	store.CompleteJob(job.ID, StatusSucceeded, 0)

	// Estimate for a new command (no matching hash)
	testJob := &Job{Command: "python brand_new.py", Status: StatusQueued}
	result := est.Estimate(testJob)

	if result.Method != "global_avg" {
		t.Errorf("method = %s, want global_avg", result.Method)
	}
	if result.Confidence != 0.2 {
		t.Errorf("confidence = %f, want 0.2", result.Confidence)
	}
}

func TestEstimator_Default(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	// Empty store → default
	testJob := &Job{Command: "python train.py", Status: StatusQueued}
	result := est.Estimate(testJob)

	if result.Method != "default" {
		t.Errorf("method = %s, want default", result.Method)
	}
	if result.EstimatedDurationSec != 300 {
		t.Errorf("duration = %f, want 300", result.EstimatedDurationSec)
	}
	if result.Confidence != 0.1 {
		t.Errorf("confidence = %f, want 0.1", result.Confidence)
	}
}

func TestEstimator_WithQueue(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	// Create 2 queued jobs
	store.CreateJob(&JobSubmitRequest{Name: "q1", Command: "echo 1", Workdir: "."})
	store.CreateJob(&JobSubmitRequest{Name: "q2", Command: "echo 2", Workdir: "."})

	testJob := &Job{Command: "echo new", Status: StatusQueued}
	result := est.EstimateWithQueue(testJob)

	if result.QueueWaitSec <= 0 {
		t.Errorf("QueueWaitSec = %f, want > 0", result.QueueWaitSec)
	}
}

func TestEstimator_RunningJobNoQueueWait(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "test.db"))
	defer store.Close()
	est := NewEstimator(store)

	testJob := &Job{Command: "echo running", Status: StatusRunning}
	result := est.EstimateWithQueue(testJob)

	if result.QueueWaitSec != 0 {
		t.Errorf("QueueWaitSec = %f, want 0 for running job", result.QueueWaitSec)
	}
}
