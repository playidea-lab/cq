// Package store provides SQLite-backed persistence for C5 job queue.
//
// It follows the same patterns as c4-core/internal/daemon/store.go:
// MaxOpenConns(1) + WAL + busy_timeout to prevent deadlocks.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if _, err := db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		fmt.Fprintf(os.Stderr, "c5: PRAGMA busy_timeout=30000 failed: %v\n", err)
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
			project_id  TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			started_at  TEXT,
			finished_at TEXT,
			exit_code   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);
		CREATE INDEX IF NOT EXISTS idx_jobs_project ON jobs(project_id);

		CREATE TABLE IF NOT EXISTS workers (
			id             TEXT PRIMARY KEY,
			hostname       TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'online',
			gpu_count      INTEGER NOT NULL DEFAULT 0,
			gpu_model      TEXT NOT NULL DEFAULT '',
			total_vram     REAL NOT NULL DEFAULT 0,
			free_vram      REAL NOT NULL DEFAULT 0,
			tags           TEXT NOT NULL DEFAULT '[]',
			project_id     TEXT NOT NULL DEFAULT '',
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

		CREATE TABLE IF NOT EXISTS dags (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags        TEXT NOT NULL DEFAULT '[]',
			project_id  TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			created_at  TEXT NOT NULL,
			started_at  TEXT,
			finished_at TEXT
		);

		CREATE TABLE IF NOT EXISTS dag_nodes (
			id          TEXT PRIMARY KEY,
			dag_id      TEXT NOT NULL,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			command     TEXT NOT NULL,
			working_dir TEXT NOT NULL DEFAULT '.',
			env         TEXT NOT NULL DEFAULT '{}',
			gpu_count   INTEGER NOT NULL DEFAULT 0,
			max_retries INTEGER NOT NULL DEFAULT 3,
			status      TEXT NOT NULL DEFAULT 'pending',
			job_id      TEXT NOT NULL DEFAULT '',
			started_at  TEXT,
			finished_at TEXT,
			exit_code   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_dag_nodes_dag ON dag_nodes(dag_id);

		CREATE TABLE IF NOT EXISTS dag_dependencies (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			dag_id    TEXT NOT NULL,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			dep_type  TEXT NOT NULL DEFAULT 'sequential'
		);
		CREATE INDEX IF NOT EXISTS idx_dag_deps_dag ON dag_dependencies(dag_id);

		CREATE TABLE IF NOT EXISTS edges (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'online',
			tags       TEXT NOT NULL DEFAULT '[]',
			arch       TEXT NOT NULL DEFAULT '',
			runtime    TEXT NOT NULL DEFAULT '',
			storage    REAL NOT NULL DEFAULT 0,
			metadata   TEXT NOT NULL DEFAULT '{}',
			last_seen  TEXT NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS deploy_rules (
			id               TEXT PRIMARY KEY,
			name             TEXT NOT NULL DEFAULT '',
			project_id       TEXT NOT NULL DEFAULT '',
			trigger_expr     TEXT NOT NULL,
			edge_filter      TEXT NOT NULL,
			artifact_pattern TEXT NOT NULL,
			post_command     TEXT NOT NULL DEFAULT '',
			enabled          INTEGER NOT NULL DEFAULT 1,
			created_at       TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS deployments (
			id          TEXT PRIMARY KEY,
			rule_id     TEXT NOT NULL DEFAULT '',
			job_id      TEXT NOT NULL DEFAULT '',
			project_id  TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			created_at  TEXT NOT NULL,
			finished_at TEXT
		);

		CREATE TABLE IF NOT EXISTS deploy_targets (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			deploy_id  TEXT NOT NULL,
			edge_id    TEXT NOT NULL,
			edge_name  TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'pending',
			error      TEXT NOT NULL DEFAULT '',
			started_at TEXT,
			done_at    TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_deploy_targets_deploy ON deploy_targets(deploy_id);

		CREATE TABLE IF NOT EXISTS artifacts (
			id           TEXT PRIMARY KEY,
			job_id       TEXT NOT NULL,
			path         TEXT NOT NULL,
			content_hash TEXT NOT NULL DEFAULT '',
			size_bytes   INTEGER NOT NULL DEFAULT 0,
			confirmed    INTEGER NOT NULL DEFAULT 0,
			created_at   TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_artifacts_job ON artifacts(job_id);

		CREATE TABLE IF NOT EXISTS api_keys (
			key_hash    TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_project ON api_keys(project_id);

		CREATE TABLE IF NOT EXISTS capabilities (
			id          TEXT PRIMARY KEY,
			worker_id   TEXT NOT NULL,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			input_schema TEXT NOT NULL DEFAULT '{}',
			tags        TEXT NOT NULL DEFAULT '[]',
			version     TEXT NOT NULL DEFAULT '',
			command     TEXT NOT NULL DEFAULT '',
			project_id  TEXT NOT NULL DEFAULT '',
			updated_at  TEXT NOT NULL,
			UNIQUE(worker_id, name)
		);
		CREATE INDEX IF NOT EXISTS idx_capabilities_name ON capabilities(name);
		CREATE INDEX IF NOT EXISTS idx_capabilities_project ON capabilities(project_id);
		CREATE INDEX IF NOT EXISTS idx_capabilities_worker ON capabilities(worker_id);

		CREATE TABLE IF NOT EXISTS device_sessions (
			state          TEXT PRIMARY KEY,
			user_code      TEXT UNIQUE NOT NULL,
			csrf_token     TEXT NOT NULL DEFAULT '',
			code_challenge TEXT NOT NULL,
			supabase_url   TEXT NOT NULL,
			auth_code      TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'pending',
			poll_count     INTEGER NOT NULL DEFAULT 0,
			token_attempts INTEGER NOT NULL DEFAULT 0,
			expires_at     INTEGER NOT NULL,
			created_at     INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_device_sessions_user_code ON device_sessions(user_code);
		CREATE INDEX IF NOT EXISTS idx_device_sessions_expires_at ON device_sessions(expires_at);
	`)
	if err != nil {
		return err
	}

	// Migration: add project_id columns for existing databases.
	// ALTER TABLE ADD COLUMN is idempotent-safe: ignore "duplicate column" errors.
	for _, stmt := range []string{
		`ALTER TABLE jobs ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_project ON jobs(project_id)`,
		// Migration: add input/output artifact JSON columns.
		`ALTER TABLE jobs ADD COLUMN input_artifacts TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE jobs ADD COLUMN output_artifacts TEXT NOT NULL DEFAULT '[]'`,
		// Migration: add VRAM requirement for GPU job matching.
		`ALTER TABLE jobs ADD COLUMN vram_required_gb REAL NOT NULL DEFAULT 0`,
		// Migration: add project_id to edges, dags, deploy_rules, deployments.
		`ALTER TABLE edges ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dags ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE deploy_rules ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE deployments ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`,
		// Migration: capability-typed jobs.
		`ALTER TABLE jobs ADD COLUMN capability TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE jobs ADD COLUMN params TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE jobs ADD COLUMN result TEXT NOT NULL DEFAULT '{}'`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_capability ON jobs(capability)`,
		// Migration: submitted_by for audit trail (nullable; master key → empty).
		`ALTER TABLE jobs ADD COLUMN submitted_by TEXT DEFAULT NULL`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migration %q: %w", stmt, err)
			}
		}
	}

	return nil
}

// =========================================================================
// Jobs
// =========================================================================

// CreateJob inserts a new job with QUEUED status.
func (s *Store) CreateJob(req *model.JobSubmitRequest) (*model.Job, error) {
	id := generateID("j")
	now := time.Now().UTC()

	job := &model.Job{
		ID:              id,
		Name:            req.Name,
		Status:          model.StatusQueued,
		Priority:        req.Priority,
		Workdir:         req.Workdir,
		Command:         req.Command,
		RequiresGPU:     req.RequiresGPU,
		VRAMRequiredGB:  req.VRAMRequiredGB,
		Env:             req.Env,
		Tags:            req.Tags,
		ExpID:           req.ExpID,
		Memo:            req.Memo,
		TimeoutSec:      req.TimeoutSec,
		ProjectID:       req.ProjectID,
		SubmittedBy:     req.SubmittedBy,
		InputArtifacts:  req.InputArtifacts,
		OutputArtifacts: req.OutputArtifacts,
		Capability:      req.Capability,
		Params:          req.Params,
		CreatedAt:       now,
	}

	_, err := s.db.Exec(`
		INSERT INTO jobs (id, name, status, priority, workdir, command,
			requires_gpu, vram_required_gb, env, tags, exp_id, memo, timeout_sec, project_id,
			submitted_by, input_artifacts, output_artifacts, capability, params, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, string(job.Status), job.Priority, job.Workdir, job.Command,
		boolToInt(job.RequiresGPU), job.VRAMRequiredGB,
		marshalJSON(job.Env), marshalJSON(job.Tags),
		job.ExpID, job.Memo, job.TimeoutSec, job.ProjectID,
		nullableText(job.SubmittedBy),
		marshalArtifacts(job.InputArtifacts), marshalArtifacts(job.OutputArtifacts),
		job.Capability, marshalJSON(job.Params),
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

// ListJobs returns jobs filtered by status and/or project_id with limit/offset.
func (s *Store) ListJobs(status, projectID string, limit, offset int) ([]*model.Job, error) {
	query := jobSelectCols + " FROM jobs"
	args := []any{}
	conditions := []string{}

	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if projectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, projectID)
	}
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
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

// SetJobResult stores a structured result map for a completed capability job.
func (s *Store) SetJobResult(id string, result map[string]any) error {
	if len(result) == 0 {
		return nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	_, err = s.db.Exec(`UPDATE jobs SET result = ? WHERE id = ?`, string(b), id)
	return err
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
// If projectID is non-empty, only jobs from that project are considered.
func (s *Store) GetHighestPriorityQueuedJob(requiresGPU bool, projectID string) (*model.Job, error) {
	query := jobSelectCols + " FROM jobs WHERE status = 'QUEUED'"
	args := []any{}
	if requiresGPU {
		query += " AND requires_gpu = 1"
	}
	if projectID != "" {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY priority DESC, created_at ASC LIMIT 1"

	row := s.db.QueryRow(query, args...)
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
		ProjectID:     req.ProjectID,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	_, err := s.db.Exec(`
		INSERT INTO workers (id, hostname, status, gpu_count, gpu_model,
			total_vram, free_vram, tags, project_id, last_heartbeat, registered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Hostname, w.Status, w.GPUCount, w.GPUModel,
		w.TotalVRAM, w.FreeVRAM, marshalJSON(w.Tags), w.ProjectID,
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

// ListWorkers returns all workers, optionally filtered by project_id.
func (s *Store) ListWorkers(projectID string) ([]*model.Worker, error) {
	query := `SELECT id, hostname, status, gpu_count, gpu_model,
		total_vram, free_vram, tags, project_id, last_heartbeat, registered_at
		FROM workers`
	args := []any{}
	if projectID != "" {
		query += " WHERE project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY registered_at DESC"
	rows, err := s.db.Query(query, args...)
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
			total_vram, free_vram, tags, project_id, last_heartbeat, registered_at
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
// Uses UPDATE-first pattern: atomically claims the highest-priority queued
// job, then reads the claimed data. Prevents race conditions where two
// workers could SELECT the same job before either UPDATEs it.
func (s *Store) AcquireLease(workerID string, requiresGPU bool, projectID string, workerVRAM ...float64) (*model.Lease, *model.Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	expiresAt := now.Add(defaultLeaseDuration)
	nowStr := now.Format(time.RFC3339)

	// UPDATE-first: atomically claim the highest-priority queued job.
	// The subquery finds the target, UPDATE claims it — one atomic step.
	extraFilter := ""
	args := []any{nowStr, workerID}
	if requiresGPU {
		extraFilter += " AND requires_gpu = 1"
	}
	// VRAM filter: only match jobs whose vram_required_gb <= worker's available VRAM.
	if len(workerVRAM) > 0 && workerVRAM[0] > 0 {
		extraFilter += " AND vram_required_gb <= ?"
		args = append(args, workerVRAM[0])
	}
	if projectID != "" {
		extraFilter += " AND project_id = ?"
		args = append(args, projectID)
	}
	// Capability filter: only pick capability jobs if worker has that capability registered.
	// Non-capability jobs (capability='') are always eligible.
	extraFilter += " AND (capability = '' OR capability IN (SELECT name FROM capabilities WHERE worker_id = ?))"
	args = append(args, workerID)
	result, err := tx.Exec(`
		UPDATE jobs SET status = 'RUNNING', started_at = ?, worker_id = ?
		WHERE id = (
			SELECT id FROM jobs WHERE status = 'QUEUED'`+extraFilter+`
			ORDER BY priority DESC, created_at ASC LIMIT 1
		) AND status = 'QUEUED'`,
		args...)
	if err != nil {
		return nil, nil, fmt.Errorf("claim job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, nil, nil // no job available
	}

	// Read the claimed job
	row := tx.QueryRow(jobSelectCols+" FROM jobs WHERE worker_id = ? AND started_at = ?",
		workerID, nowStr)
	job, err := scanJob(row)
	if err != nil {
		return nil, nil, fmt.Errorf("read claimed job: %w", err)
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
		nowStr, expiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, nil, fmt.Errorf("insert lease: %w", err)
	}

	// Update worker status
	tx.Exec(`UPDATE workers SET status = 'busy' WHERE id = ?`, workerID)

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}

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
// Leases belonging to workers that sent a heartbeat within the last 2 minutes are skipped.
// Each expiry is wrapped in a transaction. Returns the number of expired leases processed.
func (s *Store) ExpireLeases() (int, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	heartbeatCutoff := now.Add(-2 * time.Minute).Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT l.id, l.job_id, l.worker_id FROM leases l
		JOIN workers w ON w.id = l.worker_id
		WHERE l.expires_at < ? AND w.last_heartbeat < ?`, nowStr, heartbeatCutoff)
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

	count := 0
	for _, l := range expired {
		tx, err := s.db.Begin()
		if err != nil {
			return count, fmt.Errorf("begin tx for lease %s: %w", l.id, err)
		}

		// Re-queue the job
		if _, err := tx.Exec(`UPDATE jobs SET status = 'QUEUED', started_at = NULL, worker_id = '' WHERE id = ? AND status = 'RUNNING'`, l.jobID); err != nil {
			tx.Rollback()
			return count, fmt.Errorf("re-queue job %s: %w", l.jobID, err)
		}
		// Remove the lease
		if _, err := tx.Exec(`DELETE FROM leases WHERE id = ?`, l.id); err != nil {
			tx.Rollback()
			return count, fmt.Errorf("delete lease %s: %w", l.id, err)
		}

		if err := tx.Commit(); err != nil {
			return count, fmt.Errorf("commit lease expiry %s: %w", l.id, err)
		}
		// Mark worker as online (available again) — best-effort, outside tx
		s.db.Exec(`UPDATE workers SET status = 'online' WHERE id = ?`, l.workerID)
		count++
	}

	return count, nil
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

// GetMetrics retrieves metrics for a job, optionally filtered by minStep and limited.
// If minStep > 0, only rows with step > minStep are returned (incremental fetch).
func (s *Store) GetMetrics(jobID string, minStep int, limit int) ([]model.MetricEntry, error) {
	var query string
	var args []any
	if minStep > 0 {
		query = `SELECT step, metrics, created_at FROM metrics WHERE job_id = ? AND step > ? ORDER BY step ASC`
		args = []any{jobID, minStep}
	} else {
		query = `SELECT step, metrics, created_at FROM metrics WHERE job_id = ? ORDER BY step ASC`
		args = []any{jobID}
	}
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
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
	res, err := s.db.Exec(`
		INSERT INTO job_logs (job_id, line, stream, created_at)
		VALUES (?, ?, ?, ?)`,
		jobID, line, stream, now)
	if err != nil {
		return err
	}

	// Row-count rotation (hot path): caps job_logs at 50k rows.
	// Works alongside time-based CleanupOldJobs (background, 7-day retention).
	// Both are needed: rotation prevents unbounded growth between cleanup cycles,
	// cleanup removes stale data that rotation alone would keep.
	if id, _ := res.LastInsertId(); id > 0 && id%1000 == 0 {
		const maxLogs = 50000
		var cnt int
		if err2 := s.db.QueryRow(`SELECT COUNT(*) FROM job_logs`).Scan(&cnt); err2 == nil && cnt > maxLogs {
			excess := cnt - maxLogs
			s.db.Exec(`DELETE FROM job_logs WHERE id IN (
				SELECT id FROM job_logs ORDER BY id ASC LIMIT ?)`, excess)
		}
	}
	return nil
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
// DAGs
// =========================================================================

// CreateDAG creates a new DAG.
func (s *Store) CreateDAG(projectID string, req *model.DAGCreateRequest) (*model.DAG, error) {
	id := generateID("dag")
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO dags (id, name, description, tags, project_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		id, req.Name, req.Description, marshalJSON(req.Tags), projectID, now)
	if err != nil {
		return nil, fmt.Errorf("create dag: %w", err)
	}

	return &model.DAG{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		ProjectID:   projectID,
		Status:      "pending",
		CreatedAt:   now,
	}, nil
}

// GetDAG retrieves a DAG with all its nodes and dependencies.
func (s *Store) GetDAG(id string) (*model.DAG, error) {
	var dag model.DAG
	var tagsJSON string
	var startedAt, finishedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, description, tags, status, created_at, started_at, finished_at
		FROM dags WHERE id = ?`, id).Scan(
		&dag.ID, &dag.Name, &dag.Description, &tagsJSON, &dag.Status,
		&dag.CreatedAt, &startedAt, &finishedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("dag not found: %s", id)
		}
		return nil, fmt.Errorf("get dag: %w", err)
	}
	json.Unmarshal([]byte(tagsJSON), &dag.Tags)
	if startedAt.Valid {
		dag.StartedAt = startedAt.String
	}
	if finishedAt.Valid {
		dag.FinishedAt = finishedAt.String
	}

	// Load nodes
	nodes, err := s.getDAGNodes(id)
	if err != nil {
		return nil, err
	}
	dag.Nodes = nodes

	// Load dependencies
	deps, err := s.getDAGDependencies(id)
	if err != nil {
		return nil, err
	}
	dag.Dependencies = deps

	return &dag, nil
}

// ListDAGs returns DAGs with optional status and project filters.
// Pass "" as projectID to list all DAGs (master key or internal callers).
func (s *Store) ListDAGs(projectID, status string, limit int) ([]model.DAG, error) {
	query := `SELECT id, name, description, project_id, tags, status, created_at, started_at, finished_at FROM dags`
	args := []any{}
	conditions := []string{}
	if projectID != "" {
		conditions = append(conditions, "(project_id = ? OR project_id = '')")
		args = append(args, projectID)
	}
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list dags: %w", err)
	}
	defer rows.Close()

	var dags []model.DAG
	for rows.Next() {
		var d model.DAG
		var tagsJSON string
		var startedAt, finishedAt sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.ProjectID, &tagsJSON, &d.Status,
			&d.CreatedAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &d.Tags)
		if startedAt.Valid {
			d.StartedAt = startedAt.String
		}
		if finishedAt.Valid {
			d.FinishedAt = finishedAt.String
		}
		dags = append(dags, d)
	}
	return dags, rows.Err()
}

// UpdateDAGStatus updates the status of a DAG.
func (s *Store) UpdateDAGStatus(dagID, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var err error
	switch status {
	case "running":
		_, err = s.db.Exec(`UPDATE dags SET status = ?, started_at = ? WHERE id = ?`, status, now, dagID)
	case "completed", "failed":
		_, err = s.db.Exec(`UPDATE dags SET status = ?, finished_at = ? WHERE id = ?`, status, now, dagID)
	default:
		_, err = s.db.Exec(`UPDATE dags SET status = ? WHERE id = ?`, status, dagID)
	}
	return err
}

// AddDAGNode adds a node to a DAG.
func (s *Store) AddDAGNode(dagID string, req *model.DAGAddNodeRequest) (*model.DAGNode, error) {
	id := generateID("dn")
	_, err := s.db.Exec(`
		INSERT INTO dag_nodes (id, dag_id, name, description, command, working_dir, env, gpu_count, max_retries, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		id, dagID, req.Name, req.Description, req.Command,
		req.WorkingDir, marshalJSON(req.Env), req.GPUCount, req.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("add dag node: %w", err)
	}
	return &model.DAGNode{
		ID:         id,
		Name:       req.Name,
		Command:    req.Command,
		WorkingDir: req.WorkingDir,
		Env:        req.Env,
		GPUCount:   req.GPUCount,
		MaxRetries: req.MaxRetries,
		Status:     "pending",
	}, nil
}

// AddDAGDependency adds a dependency between two nodes.
func (s *Store) AddDAGDependency(dagID string, req *model.DAGAddDependencyRequest) error {
	depType := req.Type
	if depType == "" {
		depType = "sequential"
	}
	_, err := s.db.Exec(`
		INSERT INTO dag_dependencies (dag_id, source_id, target_id, dep_type)
		VALUES (?, ?, ?, ?)`,
		dagID, req.SourceID, req.TargetID, depType)
	if err != nil {
		return fmt.Errorf("add dag dep: %w", err)
	}
	return nil
}

// TopologicalSort returns nodes in topological order. Returns error if cycle detected.
func (s *Store) TopologicalSort(dagID string) ([]string, error) {
	nodes, err := s.getDAGNodes(dagID)
	if err != nil {
		return nil, err
	}
	deps, err := s.getDAGDependencies(dagID)
	if err != nil {
		return nil, err
	}

	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for _, n := range nodes {
		inDegree[n.ID] = 0
	}
	for _, d := range deps {
		adj[d.SourceID] = append(adj[d.SourceID], d.TargetID)
		inDegree[d.TargetID]++
	}

	// Kahn's algorithm
	var queue []string
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in DAG %s", dagID)
	}
	return order, nil
}

// GetReadyNodes returns nodes that have no pending dependencies (all sources completed).
func (s *Store) GetReadyNodes(dagID string) ([]model.DAGNode, error) {
	rows, err := s.db.Query(`
		SELECT n.id, n.name, n.description, n.command, n.working_dir, n.env,
			n.gpu_count, n.max_retries, n.status, n.job_id, n.started_at, n.finished_at, n.exit_code
		FROM dag_nodes n
		WHERE n.dag_id = ? AND n.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM dag_dependencies d
			JOIN dag_nodes src ON d.source_id = src.id
			WHERE d.dag_id = ? AND d.target_id = n.id
			AND src.status != 'succeeded'
		)`, dagID, dagID)
	if err != nil {
		return nil, fmt.Errorf("get ready nodes: %w", err)
	}
	defer rows.Close()

	return scanDAGNodes(rows)
}

// AdvanceDAG checks if the DAG can advance after a job completes.
// It queues ready nodes as jobs and updates the DAG status.
// Returns the number of new jobs created.
func (s *Store) AdvanceDAG(dagID string) (int, error) {
	readyNodes, err := s.GetReadyNodes(dagID)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, node := range readyNodes {
		job, err := s.CreateJob(&model.JobSubmitRequest{
			Name:        fmt.Sprintf("dag:%s/%s", dagID, node.Name),
			Workdir:     node.WorkingDir,
			Command:     node.Command,
			Env:         node.Env,
			RequiresGPU: node.GPUCount > 0,
		})
		if err != nil {
			return created, fmt.Errorf("create job for node %s: %w", node.ID, err)
		}

		// Link node to job
		now := time.Now().UTC().Format(time.RFC3339)
		s.db.Exec(`UPDATE dag_nodes SET status = 'running', job_id = ?, started_at = ? WHERE id = ?`,
			job.ID, now, node.ID)
		created++
	}

	// Check if DAG is complete (all nodes succeeded) or failed
	if created == 0 {
		var pending, running, failed int
		s.db.QueryRow(`SELECT COUNT(*) FROM dag_nodes WHERE dag_id = ? AND status = 'pending'`, dagID).Scan(&pending)
		s.db.QueryRow(`SELECT COUNT(*) FROM dag_nodes WHERE dag_id = ? AND status = 'running'`, dagID).Scan(&running)
		s.db.QueryRow(`SELECT COUNT(*) FROM dag_nodes WHERE dag_id = ? AND status = 'failed'`, dagID).Scan(&failed)

		if pending == 0 && running == 0 {
			if failed > 0 {
				s.UpdateDAGStatus(dagID, "failed")
			} else {
				s.UpdateDAGStatus(dagID, "completed")
				var repJobID string
				if err := s.db.QueryRow(`SELECT job_id FROM dag_nodes WHERE dag_id = ? AND status = 'succeeded' LIMIT 1`, dagID).Scan(&repJobID); err == nil && repJobID != "" {
					_, _ = s.EvaluateDeployRulesForDAG(dagID, repJobID, "")
				}
			}
		}
	}

	return created, nil
}

// UpdateDAGNodeFromJob updates a DAG node's status based on its linked job's completion.
func (s *Store) UpdateDAGNodeFromJob(jobID string, status model.JobStatus, exitCode int) (string, error) {
	var nodeID, dagID string
	err := s.db.QueryRow(`SELECT id, dag_id FROM dag_nodes WHERE job_id = ?`, jobID).Scan(&nodeID, &dagID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // not a DAG node job
		}
		return "", fmt.Errorf("find dag node for job: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	nodeStatus := "succeeded"
	if status == model.StatusFailed {
		nodeStatus = "failed"
	}

	_, err = s.db.Exec(`UPDATE dag_nodes SET status = ?, finished_at = ?, exit_code = ? WHERE id = ?`,
		nodeStatus, now, exitCode, nodeID)
	if err != nil {
		return "", fmt.Errorf("update dag node: %w", err)
	}

	return dagID, nil
}

// getDAGNodes returns all nodes for a DAG.
func (s *Store) getDAGNodes(dagID string) ([]model.DAGNode, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, command, working_dir, env,
			gpu_count, max_retries, status, job_id, started_at, finished_at, exit_code
		FROM dag_nodes WHERE dag_id = ? ORDER BY id ASC`, dagID)
	if err != nil {
		return nil, fmt.Errorf("get dag nodes: %w", err)
	}
	defer rows.Close()
	return scanDAGNodes(rows)
}

// getDAGDependencies returns all dependencies for a DAG.
func (s *Store) getDAGDependencies(dagID string) ([]model.DAGDependency, error) {
	rows, err := s.db.Query(`
		SELECT source_id, target_id, dep_type
		FROM dag_dependencies WHERE dag_id = ?`, dagID)
	if err != nil {
		return nil, fmt.Errorf("get dag deps: %w", err)
	}
	defer rows.Close()

	var deps []model.DAGDependency
	for rows.Next() {
		var d model.DAGDependency
		if err := rows.Scan(&d.SourceID, &d.TargetID, &d.Type); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

func scanDAGNodes(rows *sql.Rows) ([]model.DAGNode, error) {
	var nodes []model.DAGNode
	for rows.Next() {
		var n model.DAGNode
		var envJSON string
		var startedAt, finishedAt sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(&n.ID, &n.Name, &n.Description, &n.Command, &n.WorkingDir, &envJSON,
			&n.GPUCount, &n.MaxRetries, &n.Status, &n.JobID, &startedAt, &finishedAt, &exitCode); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(envJSON), &n.Env)
		if startedAt.Valid {
			n.StartedAt = startedAt.String
		}
		if finishedAt.Valid {
			n.FinishedAt = finishedAt.String
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			n.ExitCode = &ec
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// =========================================================================
// Edges
// =========================================================================

// RegisterEdge inserts a new edge device.
func (s *Store) RegisterEdge(projectID string, req *model.EdgeRegisterRequest) (*model.Edge, error) {
	id := generateID("edge")
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO edges (id, name, project_id, status, tags, arch, runtime, storage, metadata, last_seen, created_at)
		VALUES (?, ?, ?, 'online', ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Name, projectID, marshalJSON(req.Tags), req.Arch, req.Runtime,
		req.Storage, marshalJSON(req.Meta), now, now)
	if err != nil {
		return nil, fmt.Errorf("register edge: %w", err)
	}
	return &model.Edge{
		ID:        id,
		Name:      req.Name,
		ProjectID: projectID,
		Status:    "online",
		Tags:      req.Tags,
		Arch:      req.Arch,
		Runtime:   req.Runtime,
		Storage:   req.Storage,
		Metadata:  req.Meta,
		LastSeen:  now,
	}, nil
}

// GetEdge retrieves an edge device by ID.
func (s *Store) GetEdge(id string) (*model.Edge, error) {
	var e model.Edge
	var tagsJSON, metaJSON string
	err := s.db.QueryRow(`
		SELECT id, name, project_id, status, tags, arch, runtime, storage, metadata, last_seen
		FROM edges WHERE id = ?`, id).Scan(
		&e.ID, &e.Name, &e.ProjectID, &e.Status, &tagsJSON, &e.Arch, &e.Runtime,
		&e.Storage, &metaJSON, &e.LastSeen)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("edge not found: %s", id)
		}
		return nil, fmt.Errorf("get edge: %w", err)
	}
	json.Unmarshal([]byte(tagsJSON), &e.Tags)
	json.Unmarshal([]byte(metaJSON), &e.Metadata)
	return &e, nil
}

// ListEdges returns registered edge devices, optionally scoped to a project.
// Pass "" to list all edges (master key or internal callers).
func (s *Store) ListEdges(projectID string) ([]model.Edge, error) {
	query := `SELECT id, name, project_id, status, tags, arch, runtime, storage, metadata, last_seen FROM edges`
	args := []any{}
	if projectID != "" {
		query += " WHERE project_id = ? OR project_id = ''"
		args = append(args, projectID)
	}
	query += " ORDER BY created_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list edges: %w", err)
	}
	defer rows.Close()

	var edges []model.Edge
	for rows.Next() {
		var e model.Edge
		var tagsJSON, metaJSON string
		if err := rows.Scan(&e.ID, &e.Name, &e.ProjectID, &e.Status, &tagsJSON, &e.Arch, &e.Runtime,
			&e.Storage, &metaJSON, &e.LastSeen); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &e.Tags)
		json.Unmarshal([]byte(metaJSON), &e.Metadata)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// UpdateEdgeHeartbeat updates an edge device's last_seen timestamp.
func (s *Store) UpdateEdgeHeartbeat(req *model.EdgeHeartbeatRequest) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `UPDATE edges SET last_seen = ?`
	args := []any{now}

	if req.Status != "" {
		query += ", status = ?"
		args = append(args, req.Status)
	}
	query += " WHERE id = ?"
	args = append(args, req.EdgeID)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update edge heartbeat: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("edge not found: %s", req.EdgeID)
	}
	return nil
}

// RemoveEdge deletes an edge device with optional project ownership check.
// Pass "" as projectID for master-key callers (no ownership restriction).
func (s *Store) RemoveEdge(id, projectID string) error {
	var result sql.Result
	var err error
	if projectID != "" {
		result, err = s.db.Exec(`DELETE FROM edges WHERE id = ? AND (project_id = ? OR project_id = '')`, id, projectID)
	} else {
		result, err = s.db.Exec(`DELETE FROM edges WHERE id = ?`, id)
	}
	if err != nil {
		return fmt.Errorf("remove edge: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("edge not found: %s", id)
	}
	return nil
}

// MarkStaleEdges marks edges with last_seen older than threshold as offline.
func (s *Store) MarkStaleEdges(threshold time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	result, err := s.db.Exec(`
		UPDATE edges SET status = 'offline'
		WHERE status = 'online' AND last_seen < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// MatchEdges returns edges matching a filter expression, scoped to a project.
// Supports: "tag:xxx" (tag match), "name:xxx" (glob match), "all" (all edges).
// Pass "" as projectID for master-key callers.
func (s *Store) MatchEdges(filter, projectID string) ([]model.Edge, error) {
	edges, err := s.ListEdges(projectID)
	if err != nil {
		return nil, err
	}
	if filter == "" || filter == "all" {
		return edges, nil
	}

	var matched []model.Edge
	for _, e := range edges {
		if matchEdgeFilter(e, filter) {
			matched = append(matched, e)
		}
	}
	return matched, nil
}

func matchEdgeFilter(e model.Edge, filter string) bool {
	// tag:xxx — match edges with this tag
	if len(filter) > 4 && filter[:4] == "tag:" {
		tag := filter[4:]
		for _, t := range e.Tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	// name:xxx — simple prefix/suffix match with *
	if len(filter) > 5 && filter[:5] == "name:" {
		pattern := filter[5:]
		if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
			prefix := pattern[:len(pattern)-1]
			return len(e.Name) >= len(prefix) && e.Name[:len(prefix)] == prefix
		}
		return e.Name == pattern
	}
	return false
}

// =========================================================================
// Deploy Rules & Deployments
// =========================================================================

// CreateDeployRule creates a new auto-deployment rule.
func (s *Store) CreateDeployRule(projectID string, req *model.DeployRuleCreateRequest) (*model.DeployRule, error) {
	id := generateID("dr")
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO deploy_rules (id, name, project_id, trigger_expr, edge_filter, artifact_pattern, post_command, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		id, req.Name, projectID, req.Trigger, req.EdgeFilter, req.ArtifactPattern, req.PostCommand, now)
	if err != nil {
		return nil, fmt.Errorf("create deploy rule: %w", err)
	}
	return &model.DeployRule{
		ID:              id,
		Name:            req.Name,
		ProjectID:       projectID,
		Trigger:         req.Trigger,
		EdgeFilter:      req.EdgeFilter,
		ArtifactPattern: req.ArtifactPattern,
		PostCommand:     req.PostCommand,
		Enabled:         true,
		CreatedAt:       now,
	}, nil
}

// ListDeployRules returns deploy rules, optionally scoped to a project.
// Pass "" to list all rules (master key or internal callers).
func (s *Store) ListDeployRules(projectID string) ([]model.DeployRule, error) {
	query := `SELECT id, name, project_id, trigger_expr, edge_filter, artifact_pattern, post_command, enabled, created_at FROM deploy_rules`
	args := []any{}
	if projectID != "" {
		query += " WHERE project_id = ? OR project_id = ''"
		args = append(args, projectID)
	}
	query += " ORDER BY created_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deploy rules: %w", err)
	}
	defer rows.Close()

	var rules []model.DeployRule
	for rows.Next() {
		var r model.DeployRule
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.ProjectID, &r.Trigger, &r.EdgeFilter,
			&r.ArtifactPattern, &r.PostCommand, &enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// DeleteDeployRule removes a deploy rule with optional project ownership check.
// Pass "" as projectID for master-key callers.
func (s *Store) DeleteDeployRule(id, projectID string) error {
	var result sql.Result
	var err error
	if projectID != "" {
		result, err = s.db.Exec(`DELETE FROM deploy_rules WHERE id = ? AND (project_id = ? OR project_id = '')`, id, projectID)
	} else {
		result, err = s.db.Exec(`DELETE FROM deploy_rules WHERE id = ?`, id)
	}
	if err != nil {
		return fmt.Errorf("delete deploy rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("deploy rule not found: %s", id)
	}
	return nil
}

// matchRuleTriggerForJob returns true if the rule trigger matches the job.
// Supports: job_tag:X (X in jobTags), job_id:J (exact or prefix match with jobID).
func matchRuleTriggerForJob(triggerExpr, jobID string, jobTags []string) bool {
	triggerExpr = strings.TrimSpace(triggerExpr)
	if triggerExpr == "" {
		return false
	}
	if strings.HasPrefix(triggerExpr, "job_tag:") {
		tag := strings.TrimSpace(triggerExpr[8:])
		for _, t := range jobTags {
			if t == tag {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(triggerExpr, "job_id:") {
		prefix := strings.TrimSpace(triggerExpr[7:])
		if prefix == "" {
			return false
		}
		return jobID == prefix || strings.HasPrefix(jobID, prefix)
	}
	return false
}

// EvaluateDeployRulesForJob evaluates enabled deploy rules for a completed job and creates deployments for matching rules.
// Pass "" as projectID for master-key callers (no project restriction).
func (s *Store) EvaluateDeployRulesForJob(jobID string, jobTags []string, projectID string) (int, error) {
	rules, err := s.ListDeployRules(projectID)
	if err != nil {
		return 0, err
	}
	var created int
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchRuleTriggerForJob(rule.Trigger, jobID, jobTags) {
			continue
		}
		edges, err := s.MatchEdges(rule.EdgeFilter, projectID)
		if err != nil {
			continue
		}
		if len(edges) == 0 {
			continue
		}
		req := &model.DeployTriggerRequest{
			JobID:           jobID,
			RuleID:          rule.ID,
			ArtifactPattern: rule.ArtifactPattern,
			PostCommand:     rule.PostCommand,
		}
		_, err = s.CreateDeployment(req, edges)
		if err != nil {
			continue
		}
		created++
	}
	return created, nil
}

// matchRuleTriggerForDAG returns true if the rule trigger matches the DAG (e.g. dag_complete:dag-*).
func matchRuleTriggerForDAG(triggerExpr, dagID string) bool {
	triggerExpr = strings.TrimSpace(triggerExpr)
	if triggerExpr == "" || dagID == "" {
		return false
	}
	if !strings.HasPrefix(triggerExpr, "dag_complete:") {
		return false
	}
	pattern := strings.TrimSpace(triggerExpr[13:])
	if pattern == "" {
		return false
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(dagID, prefix)
	}
	return dagID == pattern
}

// EvaluateDeployRulesForDAG evaluates enabled deploy rules for a completed DAG and creates deployments for matching rules.
// representativeJobID is used as the deployment job_id (e.g. one succeeded job from the DAG).
// Pass "" as projectID for master-key callers.
func (s *Store) EvaluateDeployRulesForDAG(dagID, representativeJobID, projectID string) (int, error) {
	if representativeJobID == "" {
		return 0, nil
	}
	rules, err := s.ListDeployRules(projectID)
	if err != nil {
		return 0, err
	}
	var created int
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchRuleTriggerForDAG(rule.Trigger, dagID) {
			continue
		}
		edges, err := s.MatchEdges(rule.EdgeFilter, projectID)
		if err != nil {
			continue
		}
		if len(edges) == 0 {
			continue
		}
		req := &model.DeployTriggerRequest{
			JobID:           representativeJobID,
			RuleID:          rule.ID,
			ArtifactPattern: rule.ArtifactPattern,
			PostCommand:     rule.PostCommand,
		}
		_, err = s.CreateDeployment(req, edges)
		if err != nil {
			continue
		}
		created++
	}
	return created, nil
}

// CreateDeployment creates a new deployment and its targets.
func (s *Store) CreateDeployment(req *model.DeployTriggerRequest, edges []model.Edge) (*model.Deployment, error) {
	id := generateID("dep")
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO deployments (id, rule_id, job_id, status, created_at)
		VALUES (?, ?, ?, 'pending', ?)`, id, req.RuleID, req.JobID, now)
	if err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	targets := make([]model.DeployTarget, 0, len(edges))
	for _, e := range edges {
		_, err = tx.Exec(`
			INSERT INTO deploy_targets (deploy_id, edge_id, edge_name, status)
			VALUES (?, ?, ?, 'pending')`, id, e.ID, e.Name)
		if err != nil {
			return nil, fmt.Errorf("create deploy target: %w", err)
		}
		targets = append(targets, model.DeployTarget{
			EdgeID:   e.ID,
			EdgeName: e.Name,
			Status:   "pending",
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &model.Deployment{
		ID:        id,
		JobID:     req.JobID,
		Status:    "pending",
		Targets:   targets,
		CreatedAt: now,
	}, nil
}

// GetDeployment retrieves a deployment with all its targets.
func (s *Store) GetDeployment(id string) (*model.Deployment, error) {
	var d model.Deployment
	var finishedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT id, rule_id, job_id, status, created_at, finished_at
		FROM deployments WHERE id = ?`, id).Scan(
		&d.ID, &d.RuleID, &d.JobID, &d.Status, &d.CreatedAt, &finishedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("deployment not found: %s", id)
		}
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if finishedAt.Valid {
		d.FinishedAt = finishedAt.String
	}

	// Load targets
	rows, err := s.db.Query(`
		SELECT edge_id, edge_name, status, error, started_at, done_at
		FROM deploy_targets WHERE deploy_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get deploy targets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t model.DeployTarget
		var startedAt, doneAt sql.NullString
		if err := rows.Scan(&t.EdgeID, &t.EdgeName, &t.Status, &t.Error, &startedAt, &doneAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			t.StartedAt = startedAt.String
		}
		if doneAt.Valid {
			t.DoneAt = doneAt.String
		}
		d.Targets = append(d.Targets, t)
	}

	return &d, rows.Err()
}

// ListDeployments returns deployments with pagination (no targets loaded).
func (s *Store) ListDeployments(limit, offset int) ([]model.Deployment, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, rule_id, job_id, status, created_at, finished_at
		FROM deployments ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()

	var result []model.Deployment
	for rows.Next() {
		var d model.Deployment
		var finishedAt sql.NullString
		if err := rows.Scan(&d.ID, &d.RuleID, &d.JobID, &d.Status, &d.CreatedAt, &finishedAt); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			d.FinishedAt = finishedAt.String
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// ListPendingAssignmentsForEdge returns pending deployment assignments for the given edge (for GET /v1/deploy/assignments/{edge_id}).
func (s *Store) ListPendingAssignmentsForEdge(edgeID string) ([]model.PendingAssignment, error) {
	rows, err := s.db.Query(`
		SELECT dt.deploy_id, d.job_id, COALESCE(r.artifact_pattern,''), COALESCE(r.post_command,'')
		FROM deploy_targets dt
		JOIN deployments d ON dt.deploy_id = d.id
		LEFT JOIN deploy_rules r ON d.rule_id = r.id
		WHERE dt.edge_id = ? AND dt.status = 'pending' AND d.status = 'pending'
		ORDER BY d.created_at ASC`, edgeID)
	if err != nil {
		return nil, fmt.Errorf("list pending assignments: %w", err)
	}
	defer rows.Close()

	var out []model.PendingAssignment
	for rows.Next() {
		var a model.PendingAssignment
		if err := rows.Scan(&a.DeployID, &a.JobID, &a.ArtifactPattern, &a.PostCommand); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateDeployTarget updates a single target's status within a deployment.
func (s *Store) UpdateDeployTarget(deployID, edgeID, status, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	switch status {
	case "downloading", "deploying":
		_, err := s.db.Exec(`
			UPDATE deploy_targets SET status = ?, started_at = ?
			WHERE deploy_id = ? AND edge_id = ?`,
			status, now, deployID, edgeID)
		return err
	case "succeeded", "failed":
		_, err := s.db.Exec(`
			UPDATE deploy_targets SET status = ?, error = ?, done_at = ?
			WHERE deploy_id = ? AND edge_id = ?`,
			status, errMsg, now, deployID, edgeID)
		if err != nil {
			return err
		}
		// Check if all targets are done
		return s.checkDeploymentComplete(deployID)
	default:
		_, err := s.db.Exec(`
			UPDATE deploy_targets SET status = ?
			WHERE deploy_id = ? AND edge_id = ?`,
			status, deployID, edgeID)
		return err
	}
}

// checkDeploymentComplete checks if all targets are done and updates deployment status.
func (s *Store) checkDeploymentComplete(deployID string) error {
	var pending, failed int
	s.db.QueryRow(`SELECT COUNT(*) FROM deploy_targets WHERE deploy_id = ? AND status NOT IN ('succeeded', 'failed')`, deployID).Scan(&pending)

	if pending > 0 {
		return nil // still in progress
	}

	s.db.QueryRow(`SELECT COUNT(*) FROM deploy_targets WHERE deploy_id = ? AND status = 'failed'`, deployID).Scan(&failed)

	now := time.Now().UTC().Format(time.RFC3339)
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM deploy_targets WHERE deploy_id = ?`, deployID).Scan(&total)

	status := "completed"
	if failed > 0 {
		if failed == total {
			status = "failed"
		} else {
			status = "partial"
		}
	}

	_, err := s.db.Exec(`UPDATE deployments SET status = ?, finished_at = ? WHERE id = ?`,
		status, now, deployID)
	return err
}

// =========================================================================
// Artifacts
// =========================================================================

// CreateArtifact creates a new artifact record (before upload).
func (s *Store) CreateArtifact(jobID, path string) (*model.Artifact, error) {
	id := generateID("art")
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO artifacts (id, job_id, path, confirmed, created_at)
		VALUES (?, ?, ?, 0, ?)`, id, jobID, path, now)
	if err != nil {
		return nil, fmt.Errorf("create artifact: %w", err)
	}
	return &model.Artifact{
		ID:        id,
		JobID:     jobID,
		Path:      path,
		Confirmed: false,
		CreatedAt: now,
	}, nil
}

// ConfirmArtifact marks an artifact as confirmed with hash and size.
func (s *Store) ConfirmArtifact(jobID string, req *model.ArtifactConfirmRequest) (*model.ArtifactConfirmResponse, error) {
	result, err := s.db.Exec(`
		UPDATE artifacts SET confirmed = 1, content_hash = ?, size_bytes = ?
		WHERE job_id = ? AND path = ?`,
		req.ContentHash, req.SizeBytes, jobID, req.Path)
	if err != nil {
		return nil, fmt.Errorf("confirm artifact: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("artifact not found: job=%s path=%s", jobID, req.Path)
	}

	var id string
	s.db.QueryRow(`SELECT id FROM artifacts WHERE job_id = ? AND path = ?`, jobID, req.Path).Scan(&id)

	return &model.ArtifactConfirmResponse{
		ArtifactID: id,
		Confirmed:  true,
	}, nil
}

// GetArtifact retrieves a specific artifact by job ID and path.
func (s *Store) GetArtifact(jobID, path string) (*model.Artifact, error) {
	var a model.Artifact
	var confirmed int
	err := s.db.QueryRow(`
		SELECT id, job_id, path, content_hash, size_bytes, confirmed, created_at
		FROM artifacts WHERE job_id = ? AND path = ?`, jobID, path).Scan(
		&a.ID, &a.JobID, &a.Path, &a.ContentHash, &a.SizeBytes, &confirmed, &a.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("artifact not found: job=%s path=%s", jobID, path)
		}
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	a.Confirmed = confirmed != 0
	return &a, nil
}

// ListArtifacts returns all artifacts for a job.
func (s *Store) ListArtifacts(jobID string) ([]model.Artifact, error) {
	rows, err := s.db.Query(`
		SELECT id, job_id, path, content_hash, size_bytes, confirmed, created_at
		FROM artifacts WHERE job_id = ? ORDER BY created_at ASC`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []model.Artifact
	for rows.Next() {
		var a model.Artifact
		var confirmed int
		if err := rows.Scan(&a.ID, &a.JobID, &a.Path, &a.ContentHash,
			&a.SizeBytes, &confirmed, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Confirmed = confirmed != 0
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// =========================================================================
// Helpers
// =========================================================================

const jobSelectCols = `SELECT id, name, status, priority, workdir, command,
	requires_gpu, vram_required_gb, env, tags, exp_id, memo, timeout_sec, worker_id,
	created_at, started_at, finished_at, exit_code, project_id, submitted_by,
	input_artifacts, output_artifacts, capability, params, result`

type scanner interface {
	Scan(dest ...any) error
}

func populateJob(sc scanner) (*model.Job, error) {
	var (
		j                   model.Job
		status              string
		requiresGPU         int
		envJSON             string
		tagsJSON            string
		createdAt           string
		startedAt           sql.NullString
		finishedAt          sql.NullString
		exitCode            sql.NullInt64
		submittedBy         sql.NullString
		inputArtifactsJSON  string
		outputArtifactsJSON string
		paramsJSON          string
		resultJSON          string
	)
	err := sc.Scan(
		&j.ID, &j.Name, &status, &j.Priority, &j.Workdir, &j.Command,
		&requiresGPU, &j.VRAMRequiredGB, &envJSON, &tagsJSON,
		&j.ExpID, &j.Memo, &j.TimeoutSec, &j.WorkerID,
		&createdAt, &startedAt, &finishedAt, &exitCode,
		&j.ProjectID, &submittedBy,
		&inputArtifactsJSON, &outputArtifactsJSON,
		&j.Capability, &paramsJSON, &resultJSON,
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
	if submittedBy.Valid {
		j.SubmittedBy = submittedBy.String
	}
	json.Unmarshal([]byte(envJSON), &j.Env)
	json.Unmarshal([]byte(tagsJSON), &j.Tags)
	j.InputArtifacts = unmarshalArtifacts(inputArtifactsJSON)
	j.OutputArtifacts = unmarshalArtifacts(outputArtifactsJSON)
	if paramsJSON != "" && paramsJSON != "{}" {
		json.Unmarshal([]byte(paramsJSON), &j.Params)
	}
	if resultJSON != "" && resultJSON != "{}" {
		json.Unmarshal([]byte(resultJSON), &j.Result)
	}
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
		&w.TotalVRAM, &w.FreeVRAM, &tagsJSON, &w.ProjectID, &lastHB, &regAt,
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
		&w.TotalVRAM, &w.FreeVRAM, &tagsJSON, &w.ProjectID, &lastHB, &regAt,
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

// nullableText converts a string to sql.NullString so that empty strings
// are stored as NULL (preserving the nullable semantics of submitted_by).
func nullableText(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
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

// marshalArtifacts serialises a []ArtifactRef to JSON, always returning "[]" for nil/empty.
func marshalArtifacts(refs []model.ArtifactRef) string {
	if len(refs) == 0 {
		return "[]"
	}
	data, err := json.Marshal(refs)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// unmarshalArtifacts deserialises a JSON TEXT column to []ArtifactRef.
// Empty or invalid JSON returns nil (which JSON marshals to omitempty).
func unmarshalArtifacts(s string) []model.ArtifactRef {
	if s == "" || s == "[]" || s == "null" {
		return nil
	}
	var refs []model.ArtifactRef
	if err := json.Unmarshal([]byte(s), &refs); err != nil {
		return nil
	}
	return refs
}

// =========================================================================
// API Keys
// =========================================================================

// CreateAPIKey generates a new per-project API key and stores its SHA256 hash.
// Returns the raw key (c5pk_<32 hex chars>) — only shown once.
func (s *Store) CreateAPIKey(projectID, description string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	key := fmt.Sprintf("c5pk_%x", raw)
	hash := model.SHA256Hex(key)

	_, err := s.db.Exec(
		`INSERT INTO api_keys (key_hash, project_id, description, created_at)
		 VALUES (?, ?, ?, datetime('now'))`,
		hash, projectID, description)
	if err != nil {
		return "", fmt.Errorf("store api key: %w", err)
	}
	return key, nil
}

// LookupAPIKey looks up a key hash and returns the associated project ID.
func (s *Store) LookupAPIKey(keyHash string) (string, error) {
	var projectID string
	err := s.db.QueryRow("SELECT project_id FROM api_keys WHERE key_hash = ?", keyHash).Scan(&projectID)
	if err != nil {
		return "", fmt.Errorf("api key not found")
	}
	return projectID, nil
}

// DeleteAPIKey removes an API key by its hash.
func (s *Store) DeleteAPIKey(keyHash string) error {
	res, err := s.db.Exec("DELETE FROM api_keys WHERE key_hash = ?", keyHash)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

// CleanupOldJobs deletes logs and metrics for completed/failed/cancelled jobs
// older than the given retention period.
func (s *Store) CleanupOldJobs(retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)

	var total int64

	res, err := s.db.Exec(`DELETE FROM job_logs WHERE job_id IN (
		SELECT id FROM jobs WHERE status IN ('SUCCEEDED','FAILED','CANCELLED')
		AND finished_at < ?)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup job_logs: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		total += n
	}

	res, err = s.db.Exec(`DELETE FROM metrics WHERE job_id IN (
		SELECT id FROM jobs WHERE status IN ('SUCCEEDED','FAILED','CANCELLED')
		AND finished_at < ?)`, cutoff)
	if err != nil {
		return total, fmt.Errorf("cleanup metrics: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		total += n
	}

	return total, nil
}

// ListAPIKeys returns all API keys (without the raw key).
func (s *Store) ListAPIKeys() ([]model.APIKeyInfo, error) {
	rows, err := s.db.Query("SELECT key_hash, project_id, description, created_at FROM api_keys ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKeyInfo
	for rows.Next() {
		var k model.APIKeyInfo
		if err := rows.Scan(&k.KeyHash, &k.ProjectID, &k.Description, &k.CreatedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// =========================================================================
// Capabilities
// =========================================================================

// UpsertCapabilities inserts or replaces a worker's capability set.
func (s *Store) UpsertCapabilities(workerID, projectID string, caps []model.Capability) error {
	if len(caps) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range caps {
		if c.Name == "" {
			continue // skip unnamed capabilities to avoid routing non-capability jobs
		}
		id := generateID("cap")
		tagsJSON := marshalJSON(c.Tags)
		schemaJSON := marshalJSON(c.InputSchema)
		if schemaJSON == "null" {
			schemaJSON = "{}"
		}
		_, err := s.db.Exec(`
			INSERT INTO capabilities (id, worker_id, name, description, input_schema, tags, version, command, project_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(worker_id, name) DO UPDATE SET
				description=excluded.description,
				input_schema=excluded.input_schema,
				tags=excluded.tags,
				version=excluded.version,
				command=excluded.command,
				project_id=excluded.project_id,
				updated_at=excluded.updated_at`,
			id, workerID, c.Name, c.Description, schemaJSON, tagsJSON, c.Version, c.Command, projectID, now)
		if err != nil {
			return fmt.Errorf("upsert capability %q: %w", c.Name, err)
		}
	}
	return nil
}

// DeleteWorkerCapabilities removes all capabilities for a worker.
func (s *Store) DeleteWorkerCapabilities(workerID string) error {
	_, err := s.db.Exec(`DELETE FROM capabilities WHERE worker_id = ?`, workerID)
	return err
}

// ListCapabilities returns capabilities scoped to a project (or all if projectID is empty).
// Results are grouped by capability name with worker summaries.
func (s *Store) ListCapabilities(projectID string) ([]model.CapabilityGroup, error) {
	query := `
		SELECT c.name, c.description, c.input_schema, c.tags, c.version,
		       w.id, w.hostname, w.status, w.gpu_model
		FROM capabilities c
		JOIN workers w ON w.id = c.worker_id
		WHERE w.status != 'offline'`
	args := []any{}
	if projectID != "" {
		query += " AND c.project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY c.name, w.id"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list capabilities: %w", err)
	}
	defer rows.Close()

	// Group by capability name
	grouped := map[string]*model.CapabilityGroup{}
	var order []string
	for rows.Next() {
		var (
			name, description, schemaJSON, tagsJSON, version string
			workerID, hostname, status, gpuModel             string
		)
		if err := rows.Scan(&name, &description, &schemaJSON, &tagsJSON, &version,
			&workerID, &hostname, &status, &gpuModel); err != nil {
			continue
		}
		g, ok := grouped[name]
		if !ok {
			g = &model.CapabilityGroup{
				Capability: model.Capability{
					Name:        name,
					Description: description,
					Version:     version,
				},
			}
			json.Unmarshal([]byte(schemaJSON), &g.InputSchema)
			json.Unmarshal([]byte(tagsJSON), &g.Tags)
			grouped[name] = g
			order = append(order, name)
		}
		g.Workers = append(g.Workers, model.CapabilityWorker{
			WorkerID: workerID,
			Hostname: hostname,
			Status:   status,
			GPUModel: gpuModel,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]model.CapabilityGroup, 0, len(order))
	for _, name := range order {
		result = append(result, *grouped[name])
	}
	return result, nil
}

// FindCapability returns capability registrations matching the given name and project.
func (s *Store) FindCapability(name, projectID string) ([]model.CapabilityRegistration, error) {
	query := `SELECT c.worker_id, c.name, c.description, c.input_schema, c.tags, c.version, c.command, c.project_id, c.updated_at
	          FROM capabilities c
	          JOIN workers w ON w.id = c.worker_id
	          WHERE c.name = ? AND w.status != 'offline'`
	args := []any{name}
	if projectID != "" {
		query += " AND c.project_id = ?"
		args = append(args, projectID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("find capability: %w", err)
	}
	defer rows.Close()

	var regs []model.CapabilityRegistration
	for rows.Next() {
		var r model.CapabilityRegistration
		var schemaJSON, tagsJSON string
		if err := rows.Scan(&r.WorkerID, &r.Name, &r.Description, &schemaJSON, &tagsJSON,
			&r.Version, &r.Command, &r.ProjectID, &r.UpdatedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(schemaJSON), &r.InputSchema)
		json.Unmarshal([]byte(tagsJSON), &r.Tags)
		regs = append(regs, r)
	}
	return regs, rows.Err()
}

// =========================================================================
// Device Sessions (OAuth 2.0 Device Authorization Grant)
// =========================================================================

// generateUserCode generates a random 8-character base32 user code (e.g. "ABCD1234").
func generateUserCode() (string, error) {
	b := make([]byte, 5) // 5 bytes → 8 base32 chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// scanDeviceSession scans a device_sessions row into a DeviceSession.
func scanDeviceSession(sc interface {
	Scan(dest ...any) error
}) (*model.DeviceSession, error) {
	var ds model.DeviceSession
	var expiresAtUnix, createdAtUnix int64
	if err := sc.Scan(
		&ds.State, &ds.UserCode, &ds.CSRFToken, &ds.CodeChallenge,
		&ds.SupabaseURL, &ds.AuthCode, &ds.Status, &ds.PollCount,
		&ds.TokenAttempts, &expiresAtUnix, &createdAtUnix,
	); err != nil {
		return nil, err
	}
	ds.ExpiresAt = time.Unix(expiresAtUnix, 0).UTC()
	ds.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
	return &ds, nil
}

const deviceSessionCols = `state, user_code, csrf_token, code_challenge, supabase_url, auth_code, status, poll_count, token_attempts, expires_at, created_at`

// CreateDeviceSession inserts a new device session with a unique user_code.
// Expired sessions are deleted first. Retries up to 3 times on user_code conflict.
func (s *Store) CreateDeviceSession(state, userCode, codeChallenge, supabaseURL string, expiresAt time.Time) error {
	now := time.Now().Unix()
	exp := expiresAt.Unix()

	for attempt := 0; attempt < 3; attempt++ {
		code := userCode
		if attempt > 0 {
			var err error
			code, err = generateUserCode()
			if err != nil {
				return fmt.Errorf("generate user code: %w", err)
			}
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		// Delete expired sessions first.
		if _, err := tx.Exec(`DELETE FROM device_sessions WHERE expires_at < ?`, now); err != nil {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("cleanup expired: %w", err)
		}

		_, err = tx.Exec(
			`INSERT INTO device_sessions (state, user_code, code_challenge, supabase_url, expires_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			state, code, codeChallenge, supabaseURL, exp, now,
		)
		if err != nil {
			tx.Rollback() //nolint:errcheck
			if strings.Contains(err.Error(), "UNIQUE constraint") && strings.Contains(err.Error(), "user_code") {
				continue // retry with new user_code
			}
			return fmt.Errorf("insert device session: %w", err)
		}
		return tx.Commit()
	}
	return fmt.Errorf("failed to generate unique user_code after 3 attempts")
}

// PeekDeviceSession retrieves a device session by state WITHOUT incrementing poll_count.
// Use this for operations that should not count as a poll (e.g. token exchange, idempotent writes).
// Returns sql.ErrNoRows if not found or already expired.
func (s *Store) PeekDeviceSession(state string) (*model.DeviceSession, error) {
	now := time.Now().Unix()
	row := s.db.QueryRow(
		`SELECT `+deviceSessionCols+` FROM device_sessions WHERE state = ? AND expires_at >= ? AND status != 'expired'`,
		state, now,
	)
	ds, err := scanDeviceSession(row)
	if err != nil {
		return nil, sql.ErrNoRows
	}
	return ds, nil
}

// GetDeviceSession retrieves a device session by state, incrementing poll_count atomically.
// Returns sql.ErrNoRows if not found or already expired.
func (s *Store) GetDeviceSession(state string) (*model.DeviceSession, error) {
	now := time.Now().Unix()

	// Atomically increment poll_count and expire if over limit.
	// Only expire 'pending' sessions: once status='ready', poll_count must not override it.
	_, err := s.db.Exec(`
		UPDATE device_sessions
		SET poll_count = poll_count + 1,
		    status = CASE WHEN poll_count + 1 > 60 AND status = 'pending' THEN 'expired' ELSE status END
		WHERE state = ? AND expires_at >= ? AND status != 'expired'`,
		state, now,
	)
	if err != nil {
		return nil, fmt.Errorf("update poll_count: %w", err)
	}

	row := s.db.QueryRow(
		`SELECT `+deviceSessionCols+` FROM device_sessions WHERE state = ? AND expires_at >= ? AND status != 'expired'`,
		state, now,
	)
	ds, err := scanDeviceSession(row)
	if err != nil {
		return nil, sql.ErrNoRows
	}
	return ds, nil
}

// GetDeviceSessionByUserCode retrieves a non-expired device session by user_code.
// Only returns sessions that have not expired, consistent with GetDeviceSession/PeekDeviceSession.
func (s *Store) GetDeviceSessionByUserCode(userCode string) (*model.DeviceSession, error) {
	now := time.Now().Unix()
	row := s.db.QueryRow(
		`SELECT `+deviceSessionCols+` FROM device_sessions WHERE user_code = ? AND expires_at >= ? AND status != 'expired'`,
		userCode, now,
	)
	ds, err := scanDeviceSession(row)
	if err != nil {
		return nil, sql.ErrNoRows
	}
	return ds, nil
}

// SetDeviceSessionAuthCode sets the auth_code and transitions status from pending to ready.
// Idempotent: if already ready, no error is returned.
func (s *Store) SetDeviceSessionAuthCode(state, authCode string) error {
	res, err := s.db.Exec(
		`UPDATE device_sessions SET auth_code = ?, status = 'ready'
		 WHERE state = ? AND status IN ('pending', 'ready')`,
		authCode, state,
	)
	if err != nil {
		return fmt.Errorf("set device session auth code: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Already ready or not found — check if session exists (no poll_count side-effect).
		ds, err := s.PeekDeviceSession(state)
		if err != nil {
			return fmt.Errorf("device session not found or expired")
		}
		if ds.Status == "ready" {
			return nil // idempotent
		}
		return fmt.Errorf("device session not found or expired")
	}
	return nil
}

// SetDeviceSessionCSRF sets the csrf_token for a device session.
func (s *Store) SetDeviceSessionCSRF(state, csrfToken string) error {
	_, err := s.db.Exec(
		`UPDATE device_sessions SET csrf_token = ? WHERE state = ?`,
		csrfToken, state,
	)
	return err
}

// IncrementTokenAttempts atomically increments token_attempts and marks the session
// expired when the limit is reached. Returns the new attempt count.
// Uses a transaction to ensure the increment and read are atomic.
func (s *Store) IncrementTokenAttempts(state string, limit int) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`UPDATE device_sessions SET token_attempts = token_attempts + 1 WHERE state = ?`, state,
	); err != nil {
		return 0, fmt.Errorf("increment token attempts: %w", err)
	}

	var n int
	if err := tx.QueryRow(`SELECT token_attempts FROM device_sessions WHERE state = ?`, state).Scan(&n); err != nil {
		return 0, fmt.Errorf("read token attempts: %w", err)
	}

	if n >= limit {
		if _, err := tx.Exec(
			`UPDATE device_sessions SET status = 'expired' WHERE state = ? AND status != 'expired'`, state,
		); err != nil {
			return n, fmt.Errorf("expire device session: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit token attempts: %w", err)
	}
	return n, nil
}

// DeleteExpiredDeviceSessions removes sessions whose expiry time has passed plus the given grace period.
// Uses expires_at (not created_at) to be consistent with GetDeviceSession/PeekDeviceSession filters.
func (s *Store) DeleteExpiredDeviceSessions(gracePeriod time.Duration) (int64, error) {
	cutoff := time.Now().Unix() - int64(gracePeriod.Seconds())
	res, err := s.db.Exec(`DELETE FROM device_sessions WHERE expires_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired device sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// StartBackgroundCleanup starts a goroutine that deletes expired device_sessions every minute.
func (s *Store) StartBackgroundCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().Unix()
				s.db.Exec(`DELETE FROM device_sessions WHERE expires_at < ?`, now) //nolint:errcheck
			}
		}
	}()
}
