package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// SQLiteExperimentStore implements ExperimentStore backed by SQLite.
type SQLiteExperimentStore struct {
	db *sql.DB
}

// newRunID generates a random 16-byte hex string as a run identifier.
func newRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// NewSQLiteExperimentStore creates an experiment store and runs schema migrations.
func NewSQLiteExperimentStore(db *sql.DB) (*SQLiteExperimentStore, error) {
	s := &SQLiteExperimentStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("experiment store migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteExperimentStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS exp_runs (
	run_id           TEXT PRIMARY KEY,
	name             TEXT NOT NULL,
	config           TEXT,
	status           TEXT NOT NULL DEFAULT 'running',
	best_metric      REAL,
	checkpoint_path  TEXT,
	final_metric     REAL,
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS exp_checkpoints (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id           TEXT NOT NULL,
	metric           REAL NOT NULL,
	checkpoint_path  TEXT,
	recorded_at      TEXT NOT NULL
);`)
	return err
}

// StartRun creates a new experiment run and returns its run_id.
func (s *SQLiteExperimentStore) StartRun(ctx context.Context, name, config string) (string, error) {
	runID := newRunID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO exp_runs(run_id, name, config, status, created_at, updated_at) VALUES(?,?,?,?,?,?)`,
		runID, name, config, "running", now, now)
	if err != nil {
		return "", fmt.Errorf("insert exp_run: %w", err)
	}
	return runID, nil
}

// RecordCheckpoint records a checkpoint metric atomically and returns true if it's the best so far.
func (s *SQLiteExperimentStore) RecordCheckpoint(ctx context.Context, runID string, metric float64, path string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// CAS UPDATE: only update best_metric when metric improves (lower is better by convention).
	res, err := tx.ExecContext(ctx,
		`UPDATE exp_runs SET best_metric=?, checkpoint_path=?, updated_at=? WHERE run_id=? AND (best_metric IS NULL OR ?<best_metric)`,
		metric, path, time.Now().UTC().Format(time.RFC3339), runID, metric)
	if err != nil {
		return false, fmt.Errorf("cas update: %w", err)
	}
	rows, _ := res.RowsAffected()
	isBest := rows > 0

	// Insert checkpoint history record unconditionally.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO exp_checkpoints(run_id, metric, checkpoint_path, recorded_at) VALUES(?,?,?,?)`,
		runID, metric, path, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, fmt.Errorf("insert checkpoint: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return isBest, nil
}

// ShouldContinue returns true if the run exists and is still in 'running' status.
func (s *SQLiteExperimentStore) ShouldContinue(ctx context.Context, runID string) (bool, error) {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM exp_runs WHERE run_id=?`, runID).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query run status: %w", err)
	}
	return status == "running", nil
}

// CompleteRun marks the run as complete.
func (s *SQLiteExperimentStore) CompleteRun(ctx context.Context, runID, status string, finalMetric float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE exp_runs SET status=?, final_metric=?, updated_at=? WHERE run_id=?`,
		status, finalMetric, now, runID)
	if err != nil {
		return fmt.Errorf("complete run: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("complete run %s: %w", runID, errors.New("run_id not found"))
	}
	return nil
}
