package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// idCounter ensures unique job IDs even within the same millisecond.
var idCounter atomic.Int64

// Store provides job persistence backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the daemon database at dbPath.
// It uses MaxOpenConns(1) + WAL + busy_timeout to prevent deadlocks,
// following the same pattern as the main c4 MCP server.
func NewStore(dbPath string) (*Store, error) {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: PRAGMA journal_mode=WAL failed: %v\n", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: PRAGMA busy_timeout=5000 failed: %v\n", err)
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

// migrate creates tables if they don't exist.
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
			gpu_count   INTEGER NOT NULL DEFAULT 0,
			env         TEXT NOT NULL DEFAULT '{}',
			tags        TEXT NOT NULL DEFAULT '[]',
			exp_id      TEXT NOT NULL DEFAULT '',
			memo        TEXT NOT NULL DEFAULT '',
			timeout_sec INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL,
			started_at  TEXT,
			finished_at TEXT,
			exit_code   INTEGER,
			pid         INTEGER NOT NULL DEFAULT 0,
			gpu_indices TEXT NOT NULL DEFAULT '[]'
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);

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

// CreateJob inserts a new job with QUEUED status and returns it.
func (s *Store) CreateJob(req *JobSubmitRequest) (*Job, error) {
	id := generateID()
	now := time.Now().UTC()

	job := &Job{
		ID:          id,
		Name:        req.Name,
		Status:      StatusQueued,
		Priority:    req.Priority,
		Workdir:     req.Workdir,
		Command:     req.Command,
		RequiresGPU: req.RequiresGPU,
		GPUCount:    req.GPUCount,
		Env:         req.Env,
		Tags:        req.Tags,
		ExpID:       req.ExpID,
		Memo:        req.Memo,
		TimeoutSec:  req.TimeoutSec,
		CreatedAt:   now,
	}

	_, err := s.db.Exec(`
		INSERT INTO jobs (id, name, status, priority, workdir, command,
			requires_gpu, gpu_count, env, tags, exp_id, memo, timeout_sec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, string(job.Status), job.Priority, job.Workdir, job.Command,
		boolToInt(job.RequiresGPU), job.GPUCount,
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
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT id, name, status, priority, workdir, command,
		requires_gpu, gpu_count, env, tags, exp_id, memo, timeout_sec,
		created_at, started_at, finished_at, exit_code, pid, gpu_indices
		FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

// ListJobs returns jobs filtered by status (empty = all) with limit/offset.
func (s *Store) ListJobs(status string, limit, offset int) ([]*Job, error) {
	query := `SELECT id, name, status, priority, workdir, command,
		requires_gpu, gpu_count, env, tags, exp_id, memo, timeout_sec,
		created_at, started_at, finished_at, exit_code, pid, gpu_indices
		FROM jobs`
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

	var jobs []*Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// GetQueuedJobs returns QUEUED jobs ordered by priority DESC, created_at ASC.
// This is the scheduler's pick order.
func (s *Store) GetQueuedJobs() ([]*Job, error) {
	rows, err := s.db.Query(`SELECT id, name, status, priority, workdir, command,
		requires_gpu, gpu_count, env, tags, exp_id, memo, timeout_sec,
		created_at, started_at, finished_at, exit_code, pid, gpu_indices
		FROM jobs WHERE status = 'QUEUED'
		ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("get queued jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// GetRunningJobs returns all RUNNING jobs.
func (s *Store) GetRunningJobs() ([]*Job, error) {
	rows, err := s.db.Query(`SELECT id, name, status, priority, workdir, command,
		requires_gpu, gpu_count, env, tags, exp_id, memo, timeout_sec,
		created_at, started_at, finished_at, exit_code, pid, gpu_indices
		FROM jobs WHERE status = 'RUNNING'
		ORDER BY started_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("get running jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// StartJob marks a job as RUNNING with a PID and optional GPU indices.
func (s *Store) StartJob(id string, pid int, gpuIndices []int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`
		UPDATE jobs SET status = 'RUNNING', started_at = ?, pid = ?, gpu_indices = ?
		WHERE id = ? AND status = 'QUEUED'`,
		now, pid, marshalJSON(gpuIndices), id)
	if err != nil {
		return fmt.Errorf("start job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job %s not found or not QUEUED", id)
	}
	return nil
}

// CompleteJob marks a job as finished (SUCCEEDED or FAILED) with exit code.
// It also records the duration in job_durations for estimation.
func (s *Store) CompleteJob(id string, status JobStatus, exitCode int) error {
	now := time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get the job for duration recording
	var command string
	var startedAt sql.NullString
	err = tx.QueryRow(`SELECT command, started_at FROM jobs WHERE id = ?`, id).Scan(&command, &startedAt)
	if err != nil {
		return fmt.Errorf("get job for complete: %w", err)
	}

	// Update job status
	result, err := tx.Exec(`
		UPDATE jobs SET status = ?, finished_at = ?, exit_code = ?, pid = 0
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
	if startedAt.Valid && status == StatusSucceeded {
		start, err := time.Parse(time.RFC3339, startedAt.String)
		if err == nil {
			duration := now.Sub(start).Seconds()
			hash := (&Job{Command: command}).CommandHash()
			tx.Exec(`INSERT INTO job_durations (command_hash, duration_sec, created_at) VALUES (?, ?, ?)`,
				hash, duration, now.Format(time.RFC3339))
		}
	}

	return tx.Commit()
}

// CancelJob marks a QUEUED or RUNNING job as CANCELLED.
func (s *Store) CancelJob(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`
		UPDATE jobs SET status = 'CANCELLED', finished_at = ?, pid = 0
		WHERE id = ? AND status IN ('QUEUED', 'RUNNING')`,
		now, id)
	if err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job %s not found or already terminal", id)
	}
	return nil
}

// GetQueueStats returns aggregate counts by status.
func (s *Store) GetQueueStats() (*QueueStats, error) {
	stats := &QueueStats{}
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
		switch JobStatus(status) {
		case StatusQueued:
			stats.Queued = count
		case StatusRunning:
			stats.Running = count
		case StatusSucceeded:
			stats.Succeeded = count
		case StatusFailed:
			stats.Failed = count
		case StatusCancelled:
			stats.Cancelled = count
		}
	}
	return stats, rows.Err()
}

// GetDurations returns historical durations for a command hash (most recent first).
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

// GetGlobalDurations returns recent durations across all commands (for global avg fallback).
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

// CountByStatus returns the count of jobs with a given status.
func (s *Store) CountByStatus(status JobStatus) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status = ?`, string(status)).Scan(&count)
	return count, err
}

// =========================================================================
// Internal helpers
// =========================================================================

// scanner abstracts the Scan method shared by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// populateJob scans columns from a scanner into a Job struct.
func populateJob(s scanner) (*Job, error) {
	var (
		j           Job
		status      string
		requiresGPU int
		envJSON     string
		tagsJSON    string
		createdAt   string
		startedAt   sql.NullString
		finishedAt  sql.NullString
		exitCode    sql.NullInt64
		gpuJSON     string
	)
	err := s.Scan(
		&j.ID, &j.Name, &status, &j.Priority, &j.Workdir, &j.Command,
		&requiresGPU, &j.GPUCount, &envJSON, &tagsJSON,
		&j.ExpID, &j.Memo, &j.TimeoutSec,
		&createdAt, &startedAt, &finishedAt, &exitCode, &j.PID, &gpuJSON,
	)
	if err != nil {
		return nil, err
	}
	j.Status = JobStatus(status)
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
	if err := json.Unmarshal([]byte(envJSON), &j.Env); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: unmarshal env for job %s: %v\n", j.ID, err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &j.Tags); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: unmarshal tags for job %s: %v\n", j.ID, err)
	}
	if err := json.Unmarshal([]byte(gpuJSON), &j.GPUIndices); err != nil {
		fmt.Fprintf(os.Stderr, "c4: daemon: unmarshal gpu_indices for job %s: %v\n", j.ID, err)
	}
	return &j, nil
}

// scanJob scans a single *sql.Row into a Job.
func scanJob(row *sql.Row) (*Job, error) {
	j, err := populateJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job not found")
		}
		return nil, fmt.Errorf("scan job: %w", err)
	}
	return j, nil
}

// scanJobRow scans a *sql.Rows row into a Job.
func scanJobRow(rows *sql.Rows) (*Job, error) {
	j, err := populateJob(rows)
	if err != nil {
		return nil, fmt.Errorf("scan job row: %w", err)
	}
	return j, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// generateID creates a short unique job ID (timestamp + counter).
func generateID() string {
	seq := idCounter.Add(1)
	return fmt.Sprintf("j-%d-%d", time.Now().UnixNano()/1e6, seq)
}
