// Package worker provides shared infrastructure for C4/C5 integrated workers.
package worker

import (
	"database/sql"
	"fmt"
	"time"
)

// ShutdownStore manages shutdown signals for workers via SQLite.
// Workers poll this store to check if a graceful shutdown has been requested.
type ShutdownStore struct {
	db *sql.DB
}

// NewShutdownStore creates a ShutdownStore, initializing the table if needed.
func NewShutdownStore(db *sql.DB) (*ShutdownStore, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS worker_shutdown_signals (
		worker_id  TEXT PRIMARY KEY,
		reason     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return nil, fmt.Errorf("create shutdown table: %w", err)
	}
	return &ShutdownStore{db: db}, nil
}

// StoreSignal records a shutdown request for the given worker.
func (s *ShutdownStore) StoreSignal(workerID, reason string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO worker_shutdown_signals (worker_id, reason, created_at) VALUES (?, ?, ?)`,
		workerID, reason, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ConsumeSignal atomically checks and removes a shutdown signal for the worker.
// Returns the reason if a signal existed, or empty string if none.
func (s *ShutdownStore) ConsumeSignal(workerID string) (string, bool) {
	var reason string
	err := s.db.QueryRow(
		`DELETE FROM worker_shutdown_signals WHERE worker_id = ? RETURNING reason`,
		workerID,
	).Scan(&reason)
	if err != nil {
		return "", false
	}
	return reason, true
}
