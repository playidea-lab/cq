// Package store provides SQLite-backed persistence for C5 job queue.
//
// It follows the same patterns as c4-core/internal/daemon/store.go:
// MaxOpenConns(1) + WAL + busy_timeout to prevent deadlocks.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/piqsol/c4/c5/internal/model"

	_ "modernc.org/sqlite"
)

var idCounter atomic.Int64

// Store provides job queue persistence backed by SQLite.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the C5 database at dbPath.
func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		fmt.Fprintf(os.Stderr, "c5: PRAGMA journal_mode=WAL failed: %v\n", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		fmt.Fprintf(os.Stderr, "c5: PRAGMA busy_timeout=5000 failed: %v\n", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'QUEUED',
			priority    INTEGER NOT NULL DEFAULT 0,
			workdir     TEXT NOT NULL DEFAULT '.',
			command     TEXT NOT NULL,
			requires_gpu INTEGER NOT NULL DEFAULT 0,
			env         TEXT NOT NULL DEFAULT '{}',
			tags        TEXT NOT NULL DEFAULT '[]',
			exp_id      TEXT NOT NULL DEFAULT '',
			memo        TEXT NOT NULL DEFAULT '',
			timeout_sec INTEGER NOT NULL DEFAULT 0,
			worker_id   TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			started_at  TEXT,
			finished_at TEXT,
			exit_code   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);

		CREATE TABLE IF NOT EXISTS workers (
			id             TEXT PRIMARY KEY,
			hostname       TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'online',
			gpu_count      INTEGER NOT NULL DEFAULT 0,
			gpu_model      TEXT NOT NULL DEFAULT '',
			total_vram     REAL NOT NULL DEFAULT 0,
			free_vram      REAL NOT NULL DEFAULT 0,
			tags           TEXT NOT NULL DEFAULT '[]',
			last_heartbeat TEXT NOT NULL,
			registered_at  TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS leases (
			id         TEXT PRIMARY KEY,
			job_id     TEXT NOT NULL,
			worker_id  TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_leases_worker ON leases(worker_id);
		CREATE INDEX IF NOT EXISTS idx_leases_expires ON leases(expires_at);

		CREATE TABLE IF NOT EXISTS metrics (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id     TEXT NOT NULL,
			step       INTEGER NOT NULL,
			metrics    TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_metrics_job ON metrics(job_id, step);

		CREATE TABLE IF NOT EXISTS job_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id     TEXT NOT NULL,
			line       TEXT NOT NULL,
			stream     TEXT NOT NULL DEFAULT 'stdout',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_job_logs_job ON job_logs(job_id);

		CREATE TABLE IF NOT EXISTS job_durations (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			command_hash TEXT NOT NULL,
			duration_sec REAL NOT NULL,
			created_at   TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_job_durations_hash ON job_durations(command_hash);
	`)
	return err
}

// =========================================================================
// Jobs
// =========================================================================

// CreateJob inserts a new job with QUEUED status.
func (s *Store) CreateJob(req *model.JobSubmitRequest) (*model.Job, error) {
	id := generateID("j")
	now := time.Now().UTC()

	job := &model.Job{
		ID:          id,
		Name:        req.Name,
		Status:      model.StatusQueued,
		Priority:    req.Priority,
		Workdir:     req.Workdir,
		Command:     req.Command,
		RequiresGPU: req.RequiresGPU,
		Env:         req.Env,
		Tags:        req.Tags,
		ExpID:       req.ExpID,
		Memo:        req.Memo,
		TimeoutSec:  req.TimeoutSec,
		CreatedAt:   now,
	}

	_, err := s.db.Exec(`
		INSERT INTO jobs (id, name, status, priority, workdir, command,
			requires_gpu, env, tags, exp_id, memo, timeout_sec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, string(job.Status), job.Priority, job.Workdir, job.Command,
		boolToInt(job.RequiresGPU),
		marshalJSON(job.Env), marshalJSON(job.Tags),
		job.ExpID, job.Memo, job.TimeoutSec,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}
	return job, nil
}

// GetJob retrieves a single job by ID.
func (s *Store) GetJob(id string) (*model.Job, error) {
	row := s.db.QueryRow(jobSelectCols+" FROM jobs WHERE id = ?", id)
	j, err := scanJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job not found: %s", id)
		}
		return nil, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

// ListJobs returns jobs filtered by status with limit/offset.
func (s *Store) ListJobs(status string, limit, offset int) ([]*model.Job, error) {
	query := jobSelectCols + " FROM jobs"
	args := []any{}

	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*model.Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus transitions a job to a new status.
func (s *Store) UpdateJobStatus(id string, status model.JobStatus, workerID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var result sql.Result
	var err error

	switch status {
	case model.StatusRunning:
		result, err = s.db.Exec(`
			UPDATE jobs SET status = ?, started_at = ?, worker_id = ?
			WHERE id = ? AND status = 'QUEUED'`,
			string(status), now, workerID, id)
	case model.StatusSucceeded, model.StatusFailed:
		result, err = s.db.Exec(`
			UPDATE jobs SET status = ?, finished_at = ?
			WHERE id = ? AND status = 'RUNNING'`,
			string(status), now, id)
	case model.StatusCancelled:
		result, err = s.db.Exec(`
			UPDATE jobs SET status = ?, finished_at = ?
			WHERE id = ? AND status IN ('QUEUED', 'RUNNING')`,
			string(status), now, id)
	default:
		return fmt.Errorf("invalid target status: %s", status)
	}

	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job %s not found or invalid transition to %s", id, status)
	}
	return nil
}

// CompleteJob marks a job as finished and records duration.
func (s *Store) CompleteJob(id string, status model.JobStatus, exitCode int) error {
	now := time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var command string
	var startedAt sql.NullString
	err = tx.QueryRow(`SELECT command, started_at FROM jobs WHERE id = ?`, id).Scan(&command, &startedAt)
	if err != nil {
		return fmt.Errorf("get job for complete: %w", err)
	}

	result, err := tx.Exec(`
		UPDATE jobs SET status = ?, finished_at = ?, exit_code = ?
		WHERE id = ? AND status = 'RUNNING'`,
		string(status), now.Format(time.RFC3339), exitCode, id)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job %s not found or not RUNNING", id)
	}

	// Record duration for estimation
	if startedAt.Valid && status == model.StatusSucceeded {
		start, err := time.Parse(time.RFC3339, startedAt.String)
		if err == nil {
			duration := now.Sub(start).Seconds()
			hash := model.NormalizeCommandHash(command)
			tx.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
				hash, duration, now.Format(time.RFC3339))
		}
	}

	return tx.Commit()
}

// GetQueueStats returns aggregate counts by status.
func (s *Store) GetQueueStats() (*model.QueueStats, error) {
	stats := &model.QueueStats{}
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch model.JobStatus(status) {
		case model.StatusQueued:
			stats.Queued = count
		case model.StatusRunning:
			stats.Running = count
		case model.StatusSucceeded:
			stats.Succeeded = count
		case model.StatusFailed:
			stats.Failed = count
		case model.StatusCancelled:
			stats.Cancelled = count
		}
	}
	return stats, rows.Err()
}

// CountByStatus returns the count of jobs with a given status.
func (s *Store) CountByStatus(status model.JobStatus) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status = ?`, string(status)).Scan(&count)
	return count, err
}

// GetHighestPriorityQueuedJob returns the next job to assign (highest priority, oldest).
func (s *Store) GetHighestPriorityQueuedJob(requiresGPU bool) (*model.Job, error) {
	query := jobSelectCols + " FROM jobs WHERE status = 'QUEUED'"
	if requiresGPU {
		query += " AND requires_gpu = 1"
	}
	query += " ORDER BY priority DESC, created_at ASC LIMIT 1"

	row := s.db.QueryRow(query)
	j, err := scanJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // no job available
		}
		return nil, err
	}
	return j, nil
}

// =========================================================================
// Workers
// =========================================================================

// RegisterWorker inserts a new worker.
func (s *Store) RegisterWorker(req *model.WorkerRegisterRequest) (*model.Worker, error) {
	id := generateID("w")
	now := time.Now().UTC()

	w := &model.Worker{
		ID:            id,
		Hostname:      req.Hostname,
		Status:        "online",
		GPUCount:      req.GPUCount,
		GPUModel:      req.GPUModel,
		TotalVRAM:     req.TotalVRAM,
		FreeVRAM:      req.FreeVRAM,
		Tags:          req.Tags,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	_, err := s.db.Exec(`
		INSERT INTO workers (id, hostname, status, gpu_count, gpu_model,
			total_vram, free_vram, tags, last_heartbeat, registered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Hostname, w.Status, w.GPUCount, w.GPUModel,
		w.TotalVRAM, w.FreeVRAM, marshalJSON(w.Tags),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("insert worker: %w", err)
	}
	return w, nil
}

// UpdateHeartbeat updates a worker's heartbeat timestamp and optional fields.
func (s *Store) UpdateHeartbeat(req *model.HeartbeatRequest) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `UPDATE workers SET last_heartbeat = ?`
	args := []any{now}

	if req.Status != "" {
		query += ", status = ?"
		args = append(args, req.Status)
	}
	if req.FreeVRAM > 0 {
		query += ", free_vram = ?"
		args = append(args, req.FreeVRAM)
	}
	if req.GPUCount > 0 {
		query += ", gpu_count = ?"
		args = append(args, req.GPUCount)
	}

	query += " WHERE id = ?"
	args = append(args, req.WorkerID)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("worker not found: %s", req.WorkerID)
	}
	return nil
}

// ListWorkers returns all workers.
func (s *Store) ListWorkers() ([]*model.Worker, error) {
	rows, err := s.db.Query(`
		SELECT id, hostname, status, gpu_count, gpu_model,
			total_vram, free_vram, tags, last_heartbeat, registered_at
		FROM workers ORDER BY registered_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()

	var workers []*model.Worker
	for rows.Next() {
		w, err := scanWorkerRow(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

// GetWorker retrieves a single worker by ID.
func (s *Store) GetWorker(id string) (*model.Worker, error) {
	row := s.db.QueryRow(`
		SELECT id, hostname, status, gpu_count, gpu_model,
			total_vram, free_vram, tags, last_heartbeat, registered_at
		FROM workers WHERE id = ?`, id)
	return scanWorkerSingle(row)
}

// MarkStaleWorkers marks workers with heartbeat older than threshold as offline.
func (s *Store) MarkStaleWorkers(threshold time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	result, err := s.db.Exec(`
		UPDATE workers SET status = 'offline'
		WHERE status = 'online' AND last_heartbeat < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// =========================================================================
// Leases
// =========================================================================

const defaultLeaseDuration = 5 * time.Minute

// AcquireLease assigns a queued job to a worker and creates a lease.
func (s *Store) AcquireLease(workerID string, requiresGPU bool) (*model.Lease, *model.Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Find highest-priority queued job
	query := jobSelectCols + " FROM jobs WHERE status = 'QUEUED'"
	if requiresGPU {
		query += " AND requires_gpu = 1"
	}
	query += " ORDER BY priority DESC, created_at ASC LIMIT 1"

	row := tx.QueryRow(query)
	job, err := scanJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil // no job available
		}
		return nil, nil, fmt.Errorf("find queued job: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(defaultLeaseDuration)

	// Update job status to RUNNING
	_, err = tx.Exec(`
		UPDATE jobs SET status = 'RUNNING', started_at = ?, worker_id = ?
		WHERE id = ? AND status = 'QUEUED'`,
		now.Format(time.RFC3339), workerID, job.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("start job: %w", err)
	}

	// Create lease
	leaseID := generateID("l")
	lease := &model.Lease{
		ID:        leaseID,
		JobID:     job.ID,
		WorkerID:  workerID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	_, err = tx.Exec(`
		INSERT INTO leases (id, job_id, worker_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		lease.ID, lease.JobID, lease.WorkerID,
		now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, nil, fmt.Errorf("insert lease: %w", err)
	}

	// Update worker status
	tx.Exec(`UPDATE workers SET status = 'busy' WHERE id = ?`, workerID)

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}

	job.Status = model.StatusRunning
	job.WorkerID = workerID
	job.StartedAt = &now

	return lease, job, nil
}

// RenewLease extends a lease's expiry.
func (s *Store) RenewLease(leaseID, workerID string) (*time.Time, error) {
	newExpiry := time.Now().UTC().Add(defaultLeaseDuration)

	result, err := s.db.Exec(`
		UPDATE leases SET expires_at = ?
		WHERE id = ? AND worker_id = ?`,
		newExpiry.Format(time.RFC3339), leaseID, workerID)
	if err != nil {
		return nil, fmt.Errorf("renew lease: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("lease not found: %s", leaseID)
	}
	return &newExpiry, nil
}

// ExpireLeases finds and handles expired leases.
// Returns the number of expired leases processed.
func (s *Store) ExpireLeases() (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT id, job_id, worker_id FROM leases
		WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var expired []struct{ id, jobID, workerID string }
	for rows.Next() {
		var l struct{ id, jobID, workerID string }
		if err := rows.Scan(&l.id, &l.jobID, &l.workerID); err != nil {
			return 0, err
		}
		expired = append(expired, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, l := range expired {
		// Re-queue the job
		s.db.Exec(`UPDATE jobs SET status = 'QUEUED', started_at = NULL, worker_id = '' WHERE id = ? AND status = 'RUNNING'`, l.jobID)
		// Remove the lease
		s.db.Exec(`DELETE FROM leases WHERE id = ?`, l.id)
		// Mark worker as online (available again)
		s.db.Exec(`UPDATE workers SET status = 'online' WHERE id = ?`, l.workerID)
	}

	return len(expired), nil
}

// DeleteLease removes a lease (called after job completion).
func (s *Store) DeleteLease(jobID string) error {
	_, err := s.db.Exec(`DELETE FROM leases WHERE job_id = ?`, jobID)
	return err
}

// =========================================================================
// Metrics
// =========================================================================

// InsertMetric records a metric entry for a job.
func (s *Store) InsertMetric(jobID string, entry *model.MetricsLogRequest) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO metrics (job_id, step, metrics, created_at)
		VALUES (?, ?, ?, ?)`,
		jobID, entry.Step, marshalJSON(entry.Metrics), now)
	if err != nil {
		return fmt.Errorf("insert metric: %w", err)
	}
	return nil
}

// GetMetrics retrieves metrics for a job, optionally limited.
func (s *Store) GetMetrics(jobID string, limit int) ([]model.MetricEntry, error) {
	query := `SELECT step, metrics, created_at FROM metrics WHERE job_id = ? ORDER BY step ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, jobID)
	if err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}
	defer rows.Close()

	var entries []model.MetricEntry
	for rows.Next() {
		var e model.MetricEntry
		var metricsJSON string
		var createdAt string
		if err := rows.Scan(&e.Step, &metricsJSON, &createdAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(metricsJSON), &e.Metrics)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// =========================================================================
// Logs
// =========================================================================

// AppendLog appends a log line for a job.
func (s *Store) AppendLog(jobID, line, stream string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO job_logs (job_id, line, stream, created_at)
		VALUES (?, ?, ?, ?)`,
		jobID, line, stream, now)
	return err
}

// GetLogs retrieves log lines for a job with offset/limit.
func (s *Store) GetLogs(jobID string, offset, limit int) (lines []string, total int, hasMore bool, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM job_logs WHERE job_id = ?`, jobID).Scan(&total)
	if err != nil {
		return nil, 0, false, fmt.Errorf("count logs: %w", err)
	}

	if limit == 0 {
		limit = 200
	}

	rows, err := s.db.Query(`
		SELECT line FROM job_logs WHERE job_id = ?
		ORDER BY id ASC LIMIT ? OFFSET ?`,
		jobID, limit, offset)
	if err != nil {
		return nil, 0, false, fmt.Errorf("get logs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, 0, false, err
		}
		lines = append(lines, line)
	}

	hasMore = offset+len(lines) < total
	return lines, total, hasMore, rows.Err()
}

// =========================================================================
// Duration estimation
// =========================================================================

// GetDurations returns historical durations for a command hash.
func (s *Store) GetDurations(commandHash string, limit int) ([]float64, error) {
	rows, err := s.db.Query(
		`SELECT duration_sec FROM job_durations WHERE command_hash = ? ORDER BY created_at DESC LIMIT ?`,
		commandHash, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

// GetGlobalDurations returns recent durations across all commands.
func (s *Store) GetGlobalDurations(limit int) ([]float64, error) {
	rows, err := s.db.Query(
		`SELECT duration_sec FROM job_durations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

// =========================================================================
// Helpers
// =========================================================================

const jobSelectCols = `SELECT id, name, status, priority, workdir, command,
	requires_gpu, env, tags, exp_id, memo, timeout_sec, worker_id,
	created_at, started_at, finished_at, exit_code`

type scanner interface {
	Scan(dest ...any) error
}

func populateJob(sc scanner) (*model.Job, error) {
	var (
		j           model.Job
		status      string
		requiresGPU int
		envJSON     string
		tagsJSON    string
		createdAt   string
		startedAt   sql.NullString
		finishedAt  sql.NullString
		exitCode    sql.NullInt64
	)
	err := sc.Scan(
		&j.ID, &j.Name, &status, &j.Priority, &j.Workdir, &j.Command,
		&requiresGPU, &envJSON, &tagsJSON,
		&j.ExpID, &j.Memo, &j.TimeoutSec, &j.WorkerID,
		&createdAt, &startedAt, &finishedAt, &exitCode,
	)
	if err != nil {
		return nil, err
	}
	j.Status = model.JobStatus(status)
	j.RequiresGPU = requiresGPU != 0
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		j.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		j.FinishedAt = &t
	}
	if exitCode.Valid {
		ec := int(exitCode.Int64)
		j.ExitCode = &ec
	}
	json.Unmarshal([]byte(envJSON), &j.Env)
	json.Unmarshal([]byte(tagsJSON), &j.Tags)
	return &j, nil
}

func scanJob(row *sql.Row) (*model.Job, error) {
	j, err := populateJob(row)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func scanJobRow(rows *sql.Rows) (*model.Job, error) {
	return populateJob(rows)
}

func scanWorkerRow(rows *sql.Rows) (*model.Worker, error) {
	var w model.Worker
	var tagsJSON string
	var lastHB, regAt string

	err := rows.Scan(
		&w.ID, &w.Hostname, &w.Status, &w.GPUCount, &w.GPUModel,
		&w.TotalVRAM, &w.FreeVRAM, &tagsJSON, &lastHB, &regAt,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(tagsJSON), &w.Tags)
	w.LastHeartbeat, _ = time.Parse(time.RFC3339, lastHB)
	w.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
	return &w, nil
}

func scanWorkerSingle(row *sql.Row) (*model.Worker, error) {
	var w model.Worker
	var tagsJSON string
	var lastHB, regAt string

	err := row.Scan(
		&w.ID, &w.Hostname, &w.Status, &w.GPUCount, &w.GPUModel,
		&w.TotalVRAM, &w.FreeVRAM, &tagsJSON, &lastHB, &regAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("worker not found")
		}
		return nil, err
	}
	json.Unmarshal([]byte(tagsJSON), &w.Tags)
	w.LastHeartbeat, _ = time.Parse(time.RFC3339, lastHB)
	w.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
	return &w, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func generateID(prefix string) string {
	seq := idCounter.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano()/1e6, seq)
}

func marshalJSON(v any) string {
	if v == nil {
		return "null"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(data)
}
