// Package harness provides utilities for watching Claude Code journal files
// and pushing messages to c1_channels.
package harness

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// PositionStore tracks byte offsets for JSONL files that have been partially
// processed, so that cold restarts resume from where they left off.
// The database is stored at ~/.c4/harness_positions.db.
type PositionStore struct {
	db *sql.DB
}

// NewPositionStore opens (or creates) the position store at dbPath.
func NewPositionStore(dbPath string) (*PositionStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("positionstore: create dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("positionstore: open: %w", err)
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("positionstore: set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("positionstore: set busy_timeout: %w", err)
	}

	s := &PositionStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("positionstore: migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *PositionStore) Close() error {
	return s.db.Close()
}

// GetOffset returns the last processed byte offset for filePath. Returns 0 if
// no offset has been recorded yet.
func (s *PositionStore) GetOffset(filePath string) int64 {
	var offset int64
	row := s.db.QueryRow(`SELECT offset FROM file_positions WHERE file_path = ?`, filePath)
	_ = row.Scan(&offset) // zero on no-row
	return offset
}

// SetOffset upserts the byte offset for filePath.
func (s *PositionStore) SetOffset(filePath string, offset int64) error {
	_, err := s.db.Exec(
		`INSERT INTO file_positions (file_path, offset) VALUES (?, ?)
		 ON CONFLICT(file_path) DO UPDATE SET offset = excluded.offset`,
		filePath, offset,
	)
	return err
}

func (s *PositionStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS file_positions (
  file_path TEXT PRIMARY KEY,
  offset    INTEGER NOT NULL DEFAULT 0
)`)
	return err
}
