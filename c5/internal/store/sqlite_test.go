package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// =========================================================================
// Jobs
// =========================================================================

func TestCreateAndGetJob(t *testing.T) {
	s := newTestStore(t)

	job, err := s.CreateJob(&model.JobSubmitRequest{
		Name:    "test-job",
		Command: "echo hello",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.ID == "" {
		t.Fatal("job ID should not be empty")
	}
	if job.Status != model.StatusQueued {
		t.Fatalf("expected QUEUED, got %s", job.Status)
	}

	got, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Name != "test-job" {
		t.Fatalf("expected test-job, got %s", got.Name)
	}
	if got.Command != "echo hello" {
		t.Fatalf("expected echo hello, got %s", got.Command)
	}
}

func TestGetJobNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestListJobs(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		s.CreateJob(&model.JobSubmitRequest{
			Name:    "job",
			Command: "echo",
		})
	}

	jobs, err := s.ListJobs("", 10, 0)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(jobs))
	}

	// Filter by status
	jobs, err = s.ListJobs("RUNNING", 10, 0)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 running, got %d", len(jobs))
	}
}

func TestListJobsPagination(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 10; i++ {
		s.CreateJob(&model.JobSubmitRequest{
			Name:    "job",
			Command: "echo",
		})
	}

	jobs, err := s.ListJobs("", 3, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3, got %d", len(jobs))
	}

	jobs2, err := s.ListJobs("", 3, 3)
	if err != nil {
		t.Fatalf("list offset: %v", err)
	}
	if len(jobs2) != 3 {
		t.Fatalf("expected 3 offset, got %d", len(jobs2))
	}
	if jobs[0].ID == jobs2[0].ID {
		t.Fatal("pagination should return different jobs")
	}
}

func TestUpdateJobStatus(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{
		Name:    "test",
		Command: "echo",
	})

	// QUEUED -> RUNNING
	err := s.UpdateJobStatus(job.ID, model.StatusRunning, "w-1")
	if err != nil {
		t.Fatalf("update to running: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != model.StatusRunning {
		t.Fatalf("expected RUNNING, got %s", got.Status)
	}
	if got.WorkerID != "w-1" {
		t.Fatalf("expected worker w-1, got %s", got.WorkerID)
	}
	if got.StartedAt == nil {
		t.Fatal("started_at should be set")
	}

	// RUNNING -> CANCELLED
	err = s.UpdateJobStatus(job.ID, model.StatusCancelled, "")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	got, _ = s.GetJob(job.ID)
	if got.Status != model.StatusCancelled {
		t.Fatalf("expected CANCELLED, got %s", got.Status)
	}
}

func TestCompleteJob(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{
		Name:    "test",
		Command: "echo hello",
	})

	// Must be running first
	s.UpdateJobStatus(job.ID, model.StatusRunning, "w-1")

	err := s.CompleteJob(job.ID, model.StatusSucceeded, 0)
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != model.StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatal("exit code should be 0")
	}
	if got.FinishedAt == nil {
		t.Fatal("finished_at should be set")
	}
}

func TestCompleteJobRecordsDuration(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{
		Name:    "test",
		Command: "python train.py",
	})
	s.UpdateJobStatus(job.ID, model.StatusRunning, "w-1")
	s.CompleteJob(job.ID, model.StatusSucceeded, 0)

	hash := model.NormalizeCommandHash("python train.py")
	durations, err := s.GetDurations(hash, 10)
	if err != nil {
		t.Fatalf("get durations: %v", err)
	}
	if len(durations) != 1 {
		t.Fatalf("expected 1 duration, got %d", len(durations))
	}
}

func TestQueueStats(t *testing.T) {
	s := newTestStore(t)

	s.CreateJob(&model.JobSubmitRequest{Name: "j1", Command: "echo"})
	s.CreateJob(&model.JobSubmitRequest{Name: "j2", Command: "echo"})

	stats, err := s.GetQueueStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Queued != 2 {
		t.Fatalf("expected 2 queued, got %d", stats.Queued)
	}
}

func TestGetHighestPriorityQueuedJob(t *testing.T) {
	s := newTestStore(t)

	s.CreateJob(&model.JobSubmitRequest{Name: "low", Command: "echo", Priority: 1})
	s.CreateJob(&model.JobSubmitRequest{Name: "high", Command: "echo", Priority: 10})
	s.CreateJob(&model.JobSubmitRequest{Name: "mid", Command: "echo", Priority: 5})

	job, err := s.GetHighestPriorityQueuedJob(false)
	if err != nil {
		t.Fatalf("get highest priority: %v", err)
	}
	if job.Name != "high" {
		t.Fatalf("expected high priority job, got %s", job.Name)
	}
}

func TestJobWithEnvAndTags(t *testing.T) {
	s := newTestStore(t)

	job, err := s.CreateJob(&model.JobSubmitRequest{
		Name:    "tagged",
		Command: "echo",
		Env:     map[string]string{"FOO": "bar"},
		Tags:    []string{"gpu", "prod"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Env["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got %v", got.Env)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "gpu" {
		t.Fatalf("expected tags [gpu prod], got %v", got.Tags)
	}
}

// =========================================================================
// Workers
// =========================================================================

func TestRegisterAndListWorkers(t *testing.T) {
	s := newTestStore(t)

	w, err := s.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname:  "gpu-server-1",
		GPUCount:  2,
		GPUModel:  "A100",
		TotalVRAM: 80,
		FreeVRAM:  80,
	})
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}
	if w.ID == "" {
		t.Fatal("worker ID should not be empty")
	}
	if w.Status != "online" {
		t.Fatalf("expected online, got %s", w.Status)
	}

	workers, err := s.ListWorkers()
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].Hostname != "gpu-server-1" {
		t.Fatalf("expected gpu-server-1, got %s", workers[0].Hostname)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname: "test",
	})

	err := s.UpdateHeartbeat(&model.HeartbeatRequest{
		WorkerID: w.ID,
		FreeVRAM: 40,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	got, _ := s.GetWorker(w.ID)
	if got.FreeVRAM != 40 {
		t.Fatalf("expected 40 VRAM, got %f", got.FreeVRAM)
	}
}

func TestHeartbeatNonexistentWorker(t *testing.T) {
	s := newTestStore(t)

	err := s.UpdateHeartbeat(&model.HeartbeatRequest{
		WorkerID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent worker")
	}
}

func TestMarkStaleWorkers(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{
		Hostname: "stale-worker",
	})

	// Manually set old heartbeat (UTC to match MarkStaleWorkers)
	s.db.Exec(`UPDATE workers SET last_heartbeat = ? WHERE id = ?`,
		time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339), w.ID)

	n, err := s.MarkStaleWorkers(2 * time.Minute)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 stale, got %d", n)
	}

	got, _ := s.GetWorker(w.ID)
	if got.Status != "offline" {
		t.Fatalf("expected offline, got %s", got.Status)
	}
}

// =========================================================================
// Leases
// =========================================================================

func TestAcquireAndRenewLease(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "test"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job1", Command: "echo"})

	lease, job, err := s.AcquireLease(w.ID, false)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if lease == nil {
		t.Fatal("expected a lease")
	}
	if job == nil {
		t.Fatal("expected a job")
	}
	if job.Status != model.StatusRunning {
		t.Fatalf("expected RUNNING, got %s", job.Status)
	}

	// Renew
	newExpiry, err := s.RenewLease(lease.ID, w.ID)
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if newExpiry == nil {
		t.Fatal("expected new expiry time")
	}
}

func TestAcquireLeaseNoJobs(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "test"})

	lease, job, err := s.AcquireLease(w.ID, false)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lease != nil || job != nil {
		t.Fatal("expected nil when no jobs")
	}
}

func TestAcquireLeasePriority(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "test"})
	s.CreateJob(&model.JobSubmitRequest{Name: "low", Command: "echo", Priority: 1})
	s.CreateJob(&model.JobSubmitRequest{Name: "high", Command: "echo", Priority: 10})

	_, job, err := s.AcquireLease(w.ID, false)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if job.Name != "high" {
		t.Fatalf("expected high priority job, got %s", job.Name)
	}
}

func TestExpireLeases(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "test"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	lease, _, _ := s.AcquireLease(w.ID, false)

	// Set lease to expired (UTC to match ExpireLeases)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339), lease.ID)

	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired, got %d", n)
	}

	// Job should be re-queued
	job, _ := s.GetHighestPriorityQueuedJob(false)
	if job == nil {
		t.Fatal("job should be re-queued after lease expiry")
	}
}

// =========================================================================
// Metrics
// =========================================================================

func TestInsertAndGetMetrics(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	err := s.InsertMetric(job.ID, &model.MetricsLogRequest{
		Step:    0,
		Metrics: map[string]any{"loss": 0.5, "acc": 0.8},
	})
	if err != nil {
		t.Fatalf("insert metric: %v", err)
	}

	s.InsertMetric(job.ID, &model.MetricsLogRequest{
		Step:    1,
		Metrics: map[string]any{"loss": 0.3, "acc": 0.9},
	})

	entries, err := s.GetMetrics(job.ID, 10)
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Step != 0 {
		t.Fatalf("expected step 0 first, got %d", entries[0].Step)
	}
}

// =========================================================================
// Logs
// =========================================================================

func TestAppendAndGetLogs(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	for i := 0; i < 10; i++ {
		s.AppendLog(job.ID, "line "+string(rune('0'+i)), "stdout")
	}

	lines, total, hasMore, err := s.GetLogs(job.ID, 0, 5)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if total != 10 {
		t.Fatalf("expected 10 total, got %d", total)
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if !hasMore {
		t.Fatal("expected hasMore=true")
	}
}

// =========================================================================
// Duration estimation
// =========================================================================

func TestGetDurations(t *testing.T) {
	s := newTestStore(t)

	// Record some durations
	now := time.Now().UTC().Format(time.RFC3339)
	hash := model.NormalizeCommandHash("python train.py")
	s.db.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
		hash, 120.0, now)
	s.db.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
		hash, 130.0, now)

	durations, err := s.GetDurations(hash, 10)
	if err != nil {
		t.Fatalf("get durations: %v", err)
	}
	if len(durations) != 2 {
		t.Fatalf("expected 2 durations, got %d", len(durations))
	}
}

func TestGetGlobalDurations(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC().Format(time.RFC3339)
	s.db.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
		"hash1", 100.0, now)
	s.db.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
		"hash2", 200.0, now)

	durations, err := s.GetGlobalDurations(10)
	if err != nil {
		t.Fatalf("get global: %v", err)
	}
	if len(durations) != 2 {
		t.Fatalf("expected 2, got %d", len(durations))
	}
}

// =========================================================================
// Misc
// =========================================================================

func TestNewStoreCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "test.db")

	s, err := New(path)
	if err != nil {
		t.Fatalf("new store with nested dir: %v", err)
	}
	s.Close()

	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Fatal("directory should have been created")
	}
}
