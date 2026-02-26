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

	jobs, err := s.ListJobs("", "", 10, 0)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(jobs))
	}

	// Filter by status
	jobs, err = s.ListJobs("RUNNING", "", 10, 0)
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

	jobs, err := s.ListJobs("", "", 3, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3, got %d", len(jobs))
	}

	jobs2, err := s.ListJobs("", "", 3, 3)
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

	job, err := s.GetHighestPriorityQueuedJob(false, "")
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

	workers, err := s.ListWorkers("")
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

	lease, job, err := s.AcquireLease(w.ID, false, "")
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

	lease, job, err := s.AcquireLease(w.ID, false, "")
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

	_, job, err := s.AcquireLease(w.ID, false, "")
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

	lease, _, _ := s.AcquireLease(w.ID, false, "")

	// Set lease to expired and worker heartbeat to stale (UTC to match ExpireLeases)
	stale := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`, stale, lease.ID)
	s.db.Exec(`UPDATE workers SET last_heartbeat = ? WHERE id = ?`, stale, w.ID)

	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired, got %d", n)
	}

	// Job should be re-queued
	job, _ := s.GetHighestPriorityQueuedJob(false, "")
	if job == nil {
		t.Fatal("job should be re-queued after lease expiry")
	}
}

func TestExpireLeases_WorkerAlive_NoExpiry(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "alive"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	lease, _, _ := s.AcquireLease(w.ID, false, "")

	// Set lease to expired but keep worker heartbeat recent (< 2 min ago)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339), lease.ID)
	// Worker heartbeat is still fresh (set by RegisterWorker = now)

	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 expired (worker alive), got %d", n)
	}

	// Job should NOT be re-queued
	job, _ := s.GetHighestPriorityQueuedJob(false, "")
	if job != nil {
		t.Fatal("job should not be re-queued when worker is alive")
	}
}

func TestExpireLeases_WorkerStale_Expires(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "stale"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	lease, _, _ := s.AcquireLease(w.ID, false, "")

	stale := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`, stale, lease.ID)
	s.db.Exec(`UPDATE workers SET last_heartbeat = ? WHERE id = ?`, stale, w.ID)

	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired (worker stale), got %d", n)
	}

	job, _ := s.GetHighestPriorityQueuedJob(false, "")
	if job == nil {
		t.Fatal("job should be re-queued when worker is stale")
	}
}

func TestExpireLeases_Transaction_Atomicity(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "tx-test"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job1", Command: "echo"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job2", Command: "echo"})

	lease1, _, _ := s.AcquireLease(w.ID, false, "")
	lease2, _, _ := s.AcquireLease(w.ID, false, "")

	stale := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`, stale, lease1.ID)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`, stale, lease2.ID)
	s.db.Exec(`UPDATE workers SET last_heartbeat = ? WHERE id = ?`, stale, w.ID)

	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 expired, got %d", n)
	}

	// Both leases should be gone
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM leases`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 leases remaining, got %d", count)
	}
}

func TestExpireLeases_ErrorHandling(t *testing.T) {
	s := newTestStore(t)

	w, _ := s.RegisterWorker(&model.WorkerRegisterRequest{Hostname: "err-test"})
	s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	lease, _, _ := s.AcquireLease(w.ID, false, "")

	stale := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	s.db.Exec(`UPDATE leases SET expires_at = ? WHERE id = ?`, stale, lease.ID)
	s.db.Exec(`UPDATE workers SET last_heartbeat = ? WHERE id = ?`, stale, w.ID)

	// Normal expiry should succeed without error
	n, err := s.ExpireLeases()
	if err != nil {
		t.Fatalf("ExpireLeases returned unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired, got %d", n)
	}

	// Second call should return 0 (lease already deleted) without error
	n2, err2 := s.ExpireLeases()
	if err2 != nil {
		t.Fatalf("second ExpireLeases returned error: %v", err2)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 on second call, got %d", n2)
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

	entries, err := s.GetMetrics(job.ID, 0, 10)
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

func TestGetMetrics_MinStep(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	for step := 0; step < 5; step++ {
		s.InsertMetric(job.ID, &model.MetricsLogRequest{
			Step:    step,
			Metrics: map[string]any{"loss": float64(step)},
		})
	}

	// minStep=0 → all 5 rows
	all, err := s.GetMetrics(job.ID, 0, 0)
	if err != nil {
		t.Fatalf("get all metrics: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(all))
	}

	// minStep=2 → rows with step > 2 → steps 3, 4
	incremental, err := s.GetMetrics(job.ID, 2, 0)
	if err != nil {
		t.Fatalf("get incremental metrics: %v", err)
	}
	if len(incremental) != 2 {
		t.Fatalf("expected 2 entries (step>2), got %d", len(incremental))
	}
	if incremental[0].Step != 3 {
		t.Fatalf("expected first step=3, got %d", incremental[0].Step)
	}
	if incremental[1].Step != 4 {
		t.Fatalf("expected second step=4, got %d", incremental[1].Step)
	}

	// minStep=4 → no new rows
	empty, err := s.GetMetrics(job.ID, 4, 0)
	if err != nil {
		t.Fatalf("get empty metrics: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 entries (step>4), got %d", len(empty))
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

// =========================================================================
// DAGs
// =========================================================================

func TestCreateAndGetDAG(t *testing.T) {
	s := newTestStore(t)

	dag, err := s.CreateDAG(&model.DAGCreateRequest{
		Name:        "test-pipeline",
		Description: "A test pipeline",
		Tags:        []string{"test", "ci"},
	})
	if err != nil {
		t.Fatalf("create dag: %v", err)
	}
	if dag.ID == "" {
		t.Fatal("dag ID should not be empty")
	}
	if dag.Status != "pending" {
		t.Fatalf("expected pending, got %s", dag.Status)
	}

	got, err := s.GetDAG(dag.ID)
	if err != nil {
		t.Fatalf("get dag: %v", err)
	}
	if got.Name != "test-pipeline" {
		t.Fatalf("expected test-pipeline, got %s", got.Name)
	}
	if got.Description != "A test pipeline" {
		t.Fatalf("expected description, got %s", got.Description)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got.Tags))
	}
}

func TestGetDAGNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetDAG("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent dag")
	}
}

func TestListDAGs(t *testing.T) {
	s := newTestStore(t)

	s.CreateDAG(&model.DAGCreateRequest{Name: "dag1"})
	s.CreateDAG(&model.DAGCreateRequest{Name: "dag2"})
	s.CreateDAG(&model.DAGCreateRequest{Name: "dag3"})

	dags, err := s.ListDAGs("", 10)
	if err != nil {
		t.Fatalf("list dags: %v", err)
	}
	if len(dags) != 3 {
		t.Fatalf("expected 3 dags, got %d", len(dags))
	}

	// Filter by status
	dags, err = s.ListDAGs("running", 10)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(dags) != 0 {
		t.Fatalf("expected 0 running, got %d", len(dags))
	}
}

func TestAddDAGNodeAndDependency(t *testing.T) {
	s := newTestStore(t)

	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "pipeline"})

	node1, err := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{
		Name:    "preprocess",
		Command: "python preprocess.py",
	})
	if err != nil {
		t.Fatalf("add node 1: %v", err)
	}

	node2, err := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{
		Name:       "train",
		Command:    "python train.py",
		WorkingDir: "/workspace",
		GPUCount:   1,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("add node 2: %v", err)
	}

	// Add dependency: preprocess -> train
	err = s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{
		SourceID: node1.ID,
		TargetID: node2.ID,
		Type:     "sequential",
	})
	if err != nil {
		t.Fatalf("add dep: %v", err)
	}

	// Verify by getting DAG
	got, _ := s.GetDAG(dag.ID)
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got.Nodes))
	}
	if len(got.Dependencies) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(got.Dependencies))
	}
	if got.Dependencies[0].SourceID != node1.ID {
		t.Fatalf("expected source %s, got %s", node1.ID, got.Dependencies[0].SourceID)
	}
}

func TestTopologicalSort(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "topo"})

	// A -> B -> C
	a, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "A", Command: "echo A"})
	b, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "B", Command: "echo B"})
	c, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "C", Command: "echo C"})

	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{SourceID: a.ID, TargetID: b.ID})
	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{SourceID: b.ID, TargetID: c.ID})

	order, err := s.TopologicalSort(dag.ID)
	if err != nil {
		t.Fatalf("topo sort: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(order))
	}
	// A must come before B, B before C
	indexOf := func(id string) int {
		for i, v := range order {
			if v == id {
				return i
			}
		}
		return -1
	}
	if indexOf(a.ID) >= indexOf(b.ID) {
		t.Fatal("A should come before B")
	}
	if indexOf(b.ID) >= indexOf(c.ID) {
		t.Fatal("B should come before C")
	}
}

func TestTopologicalSortCycleDetection(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "cycle"})

	a, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "A", Command: "echo"})
	b, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "B", Command: "echo"})

	// A -> B -> A (cycle)
	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{SourceID: a.ID, TargetID: b.ID})
	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{SourceID: b.ID, TargetID: a.ID})

	_, err := s.TopologicalSort(dag.ID)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestGetReadyNodes(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "ready"})

	a, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "A", Command: "echo A"})
	b, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "B", Command: "echo B"})

	// A -> B
	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{SourceID: a.ID, TargetID: b.ID})

	// Initially only A should be ready (no deps)
	ready, err := s.GetReadyNodes(dag.ID)
	if err != nil {
		t.Fatalf("get ready: %v", err)
	}
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready node, got %d", len(ready))
	}
	if ready[0].ID != a.ID {
		t.Fatalf("expected node A, got %s", ready[0].Name)
	}

	// Mark A as succeeded -> B should become ready
	s.db.Exec(`UPDATE dag_nodes SET status = 'succeeded' WHERE id = ?`, a.ID)
	ready, _ = s.GetReadyNodes(dag.ID)
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready node after A succeeded, got %d", len(ready))
	}
	if ready[0].ID != b.ID {
		t.Fatalf("expected node B, got %s", ready[0].Name)
	}
}

func TestAdvanceDAG(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "advance"})

	a, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "A", Command: "echo A"})
	s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "B", Command: "echo B"})

	// A -> B
	s.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{
		SourceID: a.ID,
		TargetID: func() string {
			d, _ := s.GetDAG(dag.ID)
			for _, n := range d.Nodes {
				if n.Name == "B" {
					return n.ID
				}
			}
			return ""
		}(),
	})

	// Advance should queue A (root node)
	created, err := s.AdvanceDAG(dag.ID)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 job created, got %d", created)
	}

	// Check that node A is now running with a linked job
	got, _ := s.GetDAG(dag.ID)
	for _, n := range got.Nodes {
		if n.Name == "A" {
			if n.Status != "running" {
				t.Fatalf("expected A running, got %s", n.Status)
			}
			if n.JobID == "" {
				t.Fatal("A should have a linked job")
			}
		}
	}
}

func TestUpdateDAGNodeFromJob(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "update-node"})

	node, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "A", Command: "echo"})

	// Simulate: advance creates a job, then job completes
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "dag-job", Command: "echo"})
	s.db.Exec(`UPDATE dag_nodes SET status = 'running', job_id = ? WHERE id = ?`, job.ID, node.ID)

	dagID, err := s.UpdateDAGNodeFromJob(job.ID, model.StatusSucceeded, 0)
	if err != nil {
		t.Fatalf("update node from job: %v", err)
	}
	if dagID != dag.ID {
		t.Fatalf("expected dag ID %s, got %s", dag.ID, dagID)
	}

	// Node should be succeeded
	got, _ := s.GetDAG(dag.ID)
	if got.Nodes[0].Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s", got.Nodes[0].Status)
	}
}

func TestUpdateDAGNodeFromJobNonDAGJob(t *testing.T) {
	s := newTestStore(t)

	dagID, err := s.UpdateDAGNodeFromJob("nonexistent-job", model.StatusSucceeded, 0)
	if err != nil {
		t.Fatalf("expected no error for non-DAG job: %v", err)
	}
	if dagID != "" {
		t.Fatalf("expected empty dagID, got %s", dagID)
	}
}

func TestDAGCompletionStatus(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "completion"})
	s.UpdateDAGStatus(dag.ID, "running")

	node, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "only", Command: "echo"})

	// Mark node as succeeded
	s.db.Exec(`UPDATE dag_nodes SET status = 'succeeded' WHERE id = ?`, node.ID)

	// Advance should detect completion
	created, _ := s.AdvanceDAG(dag.ID)
	if created != 0 {
		t.Fatalf("expected 0 jobs, got %d", created)
	}

	got, _ := s.GetDAG(dag.ID)
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestDAGFailedStatus(t *testing.T) {
	s := newTestStore(t)
	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "fail"})
	s.UpdateDAGStatus(dag.ID, "running")

	node, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "only", Command: "echo"})

	// Mark node as failed
	s.db.Exec(`UPDATE dag_nodes SET status = 'failed' WHERE id = ?`, node.ID)

	s.AdvanceDAG(dag.ID)

	got, _ := s.GetDAG(dag.ID)
	if got.Status != "failed" {
		t.Fatalf("expected failed, got %s", got.Status)
	}
}

// =========================================================================
// Edges
// =========================================================================

func TestRegisterAndGetEdge(t *testing.T) {
	s := newTestStore(t)

	edge, err := s.RegisterEdge(&model.EdgeRegisterRequest{
		Name:    "jetson-1",
		Tags:    []string{"arm64", "onnx"},
		Arch:    "arm64",
		Runtime: "onnx",
		Storage: 32.0,
	})
	if err != nil {
		t.Fatalf("register edge: %v", err)
	}
	if edge.ID == "" {
		t.Fatal("edge ID should not be empty")
	}
	if edge.Status != "online" {
		t.Fatalf("expected online, got %s", edge.Status)
	}

	got, err := s.GetEdge(edge.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if got.Name != "jetson-1" {
		t.Fatalf("expected jetson-1, got %s", got.Name)
	}
	if got.Arch != "arm64" {
		t.Fatalf("expected arm64, got %s", got.Arch)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got.Tags))
	}
}

func TestListEdges(t *testing.T) {
	s := newTestStore(t)

	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "edge1"})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "edge2"})

	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("list edges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

func TestUpdateEdgeHeartbeat(t *testing.T) {
	s := newTestStore(t)

	edge, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "hb-test"})

	err := s.UpdateEdgeHeartbeat(&model.EdgeHeartbeatRequest{
		EdgeID: edge.ID,
		Status: "busy",
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	got, _ := s.GetEdge(edge.ID)
	if got.Status != "busy" {
		t.Fatalf("expected busy, got %s", got.Status)
	}
}

func TestUpdateEdgeHeartbeatNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateEdgeHeartbeat(&model.EdgeHeartbeatRequest{EdgeID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent edge")
	}
}

func TestRemoveEdge(t *testing.T) {
	s := newTestStore(t)

	edge, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "remove-me"})

	err := s.RemoveEdge(edge.ID)
	if err != nil {
		t.Fatalf("remove edge: %v", err)
	}

	_, err = s.GetEdge(edge.ID)
	if err == nil {
		t.Fatal("expected error after removal")
	}
}

func TestRemoveEdgeNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.RemoveEdge("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent edge")
	}
}

func TestMarkStaleEdges(t *testing.T) {
	s := newTestStore(t)

	edge, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "stale-edge"})

	// Set old last_seen
	s.db.Exec(`UPDATE edges SET last_seen = ? WHERE id = ?`,
		time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339), edge.ID)

	n, err := s.MarkStaleEdges(2 * time.Minute)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 stale, got %d", n)
	}

	got, _ := s.GetEdge(edge.ID)
	if got.Status != "offline" {
		t.Fatalf("expected offline, got %s", got.Status)
	}
}

func TestMatchEdgesTag(t *testing.T) {
	s := newTestStore(t)

	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1", Tags: []string{"onnx", "arm64"}})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e2", Tags: []string{"tensorrt"}})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e3", Tags: []string{"onnx"}})

	matched, err := s.MatchEdges("tag:onnx")
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 onnx edges, got %d", len(matched))
	}
}

func TestMatchEdgesName(t *testing.T) {
	s := newTestStore(t)

	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "jetson-factory-1"})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "jetson-factory-2"})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "rpi-lab-1"})

	matched, err := s.MatchEdges("name:jetson-*")
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 jetson edges, got %d", len(matched))
	}
}

func TestMatchEdgesAll(t *testing.T) {
	s := newTestStore(t)

	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e2"})

	matched, _ := s.MatchEdges("all")
	if len(matched) != 2 {
		t.Fatalf("expected 2, got %d", len(matched))
	}

	matched2, _ := s.MatchEdges("")
	if len(matched2) != 2 {
		t.Fatalf("expected 2 for empty filter, got %d", len(matched2))
	}
}

// =========================================================================
// Deploy Rules & Deployments
// =========================================================================

func TestCreateAndListDeployRules(t *testing.T) {
	s := newTestStore(t)

	rule, err := s.CreateDeployRule(&model.DeployRuleCreateRequest{
		Name:            "auto-deploy",
		Trigger:         "job_tag:production",
		EdgeFilter:      "tag:onnx",
		ArtifactPattern: "outputs/*.onnx",
		PostCommand:     "systemctl restart inference",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("rule ID should not be empty")
	}
	if !rule.Enabled {
		t.Fatal("rule should be enabled by default")
	}

	rules, err := s.ListDeployRules()
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Trigger != "job_tag:production" {
		t.Fatalf("expected trigger, got %s", rules[0].Trigger)
	}
}

func TestDeleteDeployRule(t *testing.T) {
	s := newTestStore(t)

	rule, _ := s.CreateDeployRule(&model.DeployRuleCreateRequest{
		Trigger:         "test",
		EdgeFilter:      "all",
		ArtifactPattern: "*",
	})

	err := s.DeleteDeployRule(rule.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	rules, _ := s.ListDeployRules()
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestDeleteDeployRuleNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteDeployRule("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}

func TestCreateAndGetDeployment(t *testing.T) {
	s := newTestStore(t)

	// Create edges
	e1, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "edge-1"})
	e2, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "edge-2"})

	// Create a job
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "model-train", Command: "train"})

	// Create deployment
	edges := []model.Edge{
		{ID: e1.ID, Name: e1.Name},
		{ID: e2.ID, Name: e2.Name},
	}
	dep, err := s.CreateDeployment(&model.DeployTriggerRequest{JobID: job.ID}, edges)
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if dep.ID == "" {
		t.Fatal("deployment ID should not be empty")
	}
	if dep.Status != "pending" {
		t.Fatalf("expected pending, got %s", dep.Status)
	}
	if len(dep.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(dep.Targets))
	}

	// Get deployment
	got, err := s.GetDeployment(dep.ID)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got.Targets))
	}
	if got.JobID != job.ID {
		t.Fatalf("expected job %s, got %s", job.ID, got.JobID)
	}
}

func TestUpdateDeployTargetComplete(t *testing.T) {
	s := newTestStore(t)

	e1, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	e2, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e2"})
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})

	edges := []model.Edge{{ID: e1.ID, Name: "e1"}, {ID: e2.ID, Name: "e2"}}
	dep, _ := s.CreateDeployment(&model.DeployTriggerRequest{JobID: job.ID}, edges)

	// Both succeed -> deployment completed
	s.UpdateDeployTarget(dep.ID, e1.ID, "succeeded", "")
	s.UpdateDeployTarget(dep.ID, e2.ID, "succeeded", "")

	got, _ := s.GetDeployment(dep.ID)
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestUpdateDeployTargetPartial(t *testing.T) {
	s := newTestStore(t)

	e1, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	e2, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e2"})
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})

	edges := []model.Edge{{ID: e1.ID, Name: "e1"}, {ID: e2.ID, Name: "e2"}}
	dep, _ := s.CreateDeployment(&model.DeployTriggerRequest{JobID: job.ID}, edges)

	// One succeeds, one fails -> partial
	s.UpdateDeployTarget(dep.ID, e1.ID, "succeeded", "")
	s.UpdateDeployTarget(dep.ID, e2.ID, "failed", "timeout")

	got, _ := s.GetDeployment(dep.ID)
	if got.Status != "partial" {
		t.Fatalf("expected partial, got %s", got.Status)
	}
}

func TestUpdateDeployTargetAllFailed(t *testing.T) {
	s := newTestStore(t)

	e1, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})

	edges := []model.Edge{{ID: e1.ID, Name: "e1"}}
	dep, _ := s.CreateDeployment(&model.DeployTriggerRequest{JobID: job.ID}, edges)

	s.UpdateDeployTarget(dep.ID, e1.ID, "failed", "disk full")

	got, _ := s.GetDeployment(dep.ID)
	if got.Status != "failed" {
		t.Fatalf("expected failed, got %s", got.Status)
	}
}

func TestListPendingAssignmentsForEdge(t *testing.T) {
	s := newTestStore(t)

	e1, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	e2, _ := s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e2"})
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})
	rule, _ := s.CreateDeployRule(&model.DeployRuleCreateRequest{
		Name: "r1", Trigger: "job_id:*", EdgeFilter: "all", ArtifactPattern: "*", PostCommand: "",
	})
	dep, _ := s.CreateDeployment(&model.DeployTriggerRequest{
		JobID: job.ID, RuleID: rule.ID, ArtifactPattern: "*", PostCommand: "",
	}, []model.Edge{{ID: e1.ID, Name: "e1"}, {ID: e2.ID, Name: "e2"}})
	_ = dep

	list, err := s.ListPendingAssignmentsForEdge(e1.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 assignment for e1, got %d", len(list))
	}
	if list[0].DeployID == "" || list[0].JobID != job.ID {
		t.Fatalf("assignment deploy_id or job_id wrong: %+v", list[0])
	}

	list2, _ := s.ListPendingAssignmentsForEdge(e2.ID)
	if len(list2) != 1 {
		t.Fatalf("expected 1 assignment for e2, got %d", len(list2))
	}

	s.UpdateDeployTarget(dep.ID, e1.ID, "succeeded", "")
	list3, _ := s.ListPendingAssignmentsForEdge(e1.ID)
	if len(list3) != 0 {
		t.Fatalf("e1 should have 0 pending after succeeded, got %d", len(list3))
	}
}

func TestEvaluateDeployRulesForDAG(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.RegisterEdge(&model.EdgeRegisterRequest{Name: "e1"})
	rule, _ := s.CreateDeployRule(&model.DeployRuleCreateRequest{
		Name: "dag-rule", Trigger: "dag_complete:pipeline-*", EdgeFilter: "all", ArtifactPattern: "*", PostCommand: "",
	})
	_ = rule

	dag, _ := s.CreateDAG(&model.DAGCreateRequest{Name: "pipeline-1", Description: "pipeline-1"})
	n1, _ := s.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{Name: "n1", Command: "echo a"})
	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "dag:j", Command: "echo"})
	now := time.Now().UTC().Format(time.RFC3339)
	s.db.Exec(`UPDATE dag_nodes SET status = 'succeeded', job_id = ?, started_at = ?, done_at = ? WHERE id = ?`, job.ID, now, now, n1.ID)

	n, err := s.EvaluateDeployRulesForDAG("pipeline-1", job.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deployment created, got %d", n)
	}

	// Non-matching DAG
	n2, _ := s.EvaluateDeployRulesForDAG("other-dag", job.ID)
	if n2 != 0 {
		t.Fatalf("expected 0 for non-matching dag, got %d", n2)
	}
}

// =========================================================================
// Artifacts
// =========================================================================

func TestCreateAndConfirmArtifact(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})

	art, err := s.CreateArtifact(job.ID, "outputs/model.onnx")
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if art.ID == "" {
		t.Fatal("artifact ID should not be empty")
	}
	if art.Confirmed {
		t.Fatal("artifact should not be confirmed yet")
	}

	resp, err := s.ConfirmArtifact(job.ID, &model.ArtifactConfirmRequest{
		Path:        "outputs/model.onnx",
		ContentHash: "sha256:abc123",
		SizeBytes:   1024000,
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !resp.Confirmed {
		t.Fatal("should be confirmed")
	}

	got, _ := s.GetArtifact(job.ID, "outputs/model.onnx")
	if !got.Confirmed {
		t.Fatal("artifact should be confirmed")
	}
	if got.ContentHash != "sha256:abc123" {
		t.Fatalf("expected hash, got %s", got.ContentHash)
	}
	if got.SizeBytes != 1024000 {
		t.Fatalf("expected 1024000, got %d", got.SizeBytes)
	}
}

func TestListArtifacts(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "j", Command: "echo"})

	s.CreateArtifact(job.ID, "model.onnx")
	s.CreateArtifact(job.ID, "model.pt")
	s.CreateArtifact(job.ID, "metrics.json")

	arts, err := s.ListArtifacts(job.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected 3, got %d", len(arts))
	}
}

func TestGetArtifactNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetArtifact("nonexistent", "none.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent artifact")
	}
}

func TestConfirmArtifactNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ConfirmArtifact("nonexistent", &model.ArtifactConfirmRequest{
		Path:        "none.txt",
		ContentHash: "hash",
		SizeBytes:   100,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent artifact")
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

// =========================================================================
// ArtifactRef / Job artifact columns
// =========================================================================

// TestJobRoundTrip_WithArtifacts verifies that InputArtifacts and OutputArtifacts
// survive a full submit → store → retrieve round-trip.
func TestJobRoundTrip_WithArtifacts(t *testing.T) {
	s := newTestStore(t)

	req := &model.JobSubmitRequest{
		Name:    "artifact-job",
		Command: "echo hi",
		Workdir: "/tmp",
		InputArtifacts: []model.ArtifactRef{
			{Path: "inputs/data.bin", LocalPath: "/local/data.bin", Required: true},
		},
		OutputArtifacts: []model.ArtifactRef{
			{Path: "outputs/result.bin"},
		},
	}

	job, err := s.CreateJob(req)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	got, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}

	if len(got.InputArtifacts) != 1 {
		t.Fatalf("expected 1 input artifact, got %d", len(got.InputArtifacts))
	}
	if got.InputArtifacts[0].Path != "inputs/data.bin" {
		t.Errorf("input path: got %q, want %q", got.InputArtifacts[0].Path, "inputs/data.bin")
	}
	if got.InputArtifacts[0].LocalPath != "/local/data.bin" {
		t.Errorf("local path: got %q", got.InputArtifacts[0].LocalPath)
	}
	if !got.InputArtifacts[0].Required {
		t.Error("expected Required=true")
	}

	if len(got.OutputArtifacts) != 1 {
		t.Fatalf("expected 1 output artifact, got %d", len(got.OutputArtifacts))
	}
	if got.OutputArtifacts[0].Path != "outputs/result.bin" {
		t.Errorf("output path: got %q", got.OutputArtifacts[0].Path)
	}
}

// TestMigration_Idempotent verifies that running the migration twice does not error.
func TestMigration_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "idem.db")

	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	// Second open runs migrate() again — duplicate column ALTERs must be silently ignored.
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second open (idempotent migration): %v", err)
	}
	s2.Close()
}

// TestStore_ExistingRows_NullSafe verifies that rows inserted without artifact columns
// (simulating a pre-migration database) return empty slices, not errors.
func TestStore_ExistingRows_NullSafe(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	// Insert a row the old-fashioned way without artifact columns (use DEFAULT values).
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		INSERT INTO jobs (id, name, status, priority, workdir, command,
			requires_gpu, env, tags, exp_id, memo, timeout_sec, project_id, created_at)
		VALUES (?, ?, 'QUEUED', 0, '.', 'echo', 0, '{}', '[]', '', '', 0, '', ?)`,
		"legacy-001", "legacy-job", now)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	got, err := s.GetJob("legacy-001")
	if err != nil {
		t.Fatalf("get legacy job: %v", err)
	}
	if got.InputArtifacts != nil {
		t.Errorf("expected nil InputArtifacts for legacy row, got %v", got.InputArtifacts)
	}
	if got.OutputArtifacts != nil {
		t.Errorf("expected nil OutputArtifacts for legacy row, got %v", got.OutputArtifacts)
	}
}

// =========================================================================
// Log rotation
// =========================================================================

func TestAppendLog_RotatesOld(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.CreateJob(&model.JobSubmitRequest{Name: "job", Command: "echo"})

	// Insert logs until AUTOINCREMENT id reaches a multiple of 1000.
	// We manually insert rows to control the id counter.
	// First, bulk-insert 50001 rows via direct SQL for speed.
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO job_logs (job_id, line, stream, created_at) VALUES (?, ?, 'stdout', '2025-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := 0; i < 50001; i++ {
		stmt.Exec(job.ID, "line")
	}
	stmt.Close()
	tx.Commit()

	// Verify we have 50001 rows
	var cnt int
	s.db.QueryRow(`SELECT COUNT(*) FROM job_logs`).Scan(&cnt)
	if cnt != 50001 {
		t.Fatalf("expected 50001, got %d", cnt)
	}

	// Now we need the next insert's AUTOINCREMENT id to be a multiple of 1000.
	// Current max id is 50001. We need to get to id=51000 (next multiple of 1000).
	// Insert 998 more rows (50001+998=50999 rows, last id=50999).
	for i := 0; i < 998; i++ {
		s.db.Exec(`INSERT INTO job_logs (job_id, line, stream, created_at) VALUES (?, 'pad', 'stdout', '2025-01-01T00:00:00Z')`, job.ID)
	}

	// The next AppendLog should get id=51000 (multiple of 1000) and trigger rotation.
	if err := s.AppendLog(job.ID, "trigger", "stdout"); err != nil {
		t.Fatalf("append log: %v", err)
	}

	// After rotation: should be at most 50000 rows.
	s.db.QueryRow(`SELECT COUNT(*) FROM job_logs`).Scan(&cnt)
	if cnt > 50000 {
		t.Fatalf("expected <= 50000 after rotation, got %d", cnt)
	}
}

func TestCleanupOldJobs(t *testing.T) {
	s := newTestStore(t)

	// Create two jobs
	oldJob, _ := s.CreateJob(&model.JobSubmitRequest{Name: "old-job", Command: "echo old"})
	newJob, _ := s.CreateJob(&model.JobSubmitRequest{Name: "new-job", Command: "echo new"})

	// Mark both as SUCCEEDED with different finished_at times
	oldFinished := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339) // 10 days ago
	newFinished := time.Now().UTC().Add(-1 * 24 * time.Hour).Format(time.RFC3339)  // 1 day ago

	s.db.Exec(`UPDATE jobs SET status='SUCCEEDED', finished_at=? WHERE id=?`, oldFinished, oldJob.ID)
	s.db.Exec(`UPDATE jobs SET status='SUCCEEDED', finished_at=? WHERE id=?`, newFinished, newJob.ID)

	// Add logs and metrics for both
	s.AppendLog(oldJob.ID, "old line 1", "stdout")
	s.AppendLog(oldJob.ID, "old line 2", "stdout")
	s.AppendLog(newJob.ID, "new line 1", "stdout")

	s.InsertMetric(oldJob.ID, &model.MetricsLogRequest{Step: 0, Metrics: map[string]any{"loss": 0.1}})
	s.InsertMetric(newJob.ID, &model.MetricsLogRequest{Step: 0, Metrics: map[string]any{"loss": 0.2}})

	// Cleanup with 7-day retention
	cleaned, err := s.CleanupOldJobs(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned != 3 { // 2 logs + 1 metric for old job
		t.Fatalf("expected 3 cleaned rows, got %d", cleaned)
	}

	// Verify old job's logs and metrics are gone
	var logCnt, metricCnt int
	s.db.QueryRow(`SELECT COUNT(*) FROM job_logs WHERE job_id=?`, oldJob.ID).Scan(&logCnt)
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE job_id=?`, oldJob.ID).Scan(&metricCnt)
	if logCnt != 0 {
		t.Fatalf("expected 0 logs for old job, got %d", logCnt)
	}
	if metricCnt != 0 {
		t.Fatalf("expected 0 metrics for old job, got %d", metricCnt)
	}

	// Verify new job's logs and metrics are untouched
	s.db.QueryRow(`SELECT COUNT(*) FROM job_logs WHERE job_id=?`, newJob.ID).Scan(&logCnt)
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE job_id=?`, newJob.ID).Scan(&metricCnt)
	if logCnt != 1 {
		t.Fatalf("expected 1 log for new job, got %d", logCnt)
	}
	if metricCnt != 1 {
		t.Fatalf("expected 1 metric for new job, got %d", metricCnt)
	}
}
