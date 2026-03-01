package handlers

import (
	"context"
	"time"
)

// TryAcquireScopeLock attempts to acquire an advisory scope lock for the given worker.
// Returns true if the lock was acquired, false if another worker holds it.
// scope="" is a no-op (always returns true).
// TTL defines how long the lock is valid before being considered stale.
func (s *SQLiteStore) TryAcquireScopeLock(scope, workerID string, ttl time.Duration) (bool, error) {
	if scope == "" {
		return true, nil
	}

	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		}
	}()

	// Evict stale locks (TTL expired).
	// Use strftime to normalize both sides to the same format for comparison.
	if _, err = conn.ExecContext(ctx,
		"DELETE FROM c4_scope_locks WHERE scope = ? AND strftime('%Y-%m-%d %H:%M:%S', expires_at) < strftime('%Y-%m-%d %H:%M:%S', 'now')",
		scope,
	); err != nil {
		return false, err
	}

	// Try to insert our lock (INSERT OR IGNORE skips if another worker holds it).
	// Store timestamps in SQLite datetime format for consistent comparison.
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	fmtSQLite := "2006-01-02 15:04:05"
	res, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO c4_scope_locks (scope, worker_id, acquired_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		scope, workerID, now.Format(fmtSQLite), expiresAt.Format(fmtSQLite),
	)
	if err != nil {
		return false, err
	}

	var acquired bool
	if n, err2 := res.RowsAffected(); err2 == nil && n > 0 {
		acquired = true
	} else {
		// Another worker holds the lock — check if it's us (idempotent re-acquire).
		var holder string
		if err3 := conn.QueryRowContext(ctx,
			"SELECT worker_id FROM c4_scope_locks WHERE scope = ?", scope,
		).Scan(&holder); err3 == nil && holder == workerID {
			acquired = true
		}
	}

	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return false, err
	}
	committed = true

	return acquired, nil
}

// ReleaseScopeLock releases the advisory scope lock held by workerID for scope.
// scope="" is a no-op.
func (s *SQLiteStore) ReleaseScopeLock(scope, workerID string) error {
	if scope == "" {
		return nil
	}
	_, err := s.db.Exec(
		"DELETE FROM c4_scope_locks WHERE scope = ? AND worker_id = ?",
		scope, workerID,
	)
	return err
}
