package fileindex

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
)

// OpenDB opens (or creates) a SQLite database for the file index.
func OpenDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

// FileEntry represents a file in the index.
type FileEntry struct {
	Path       string // relative to project root
	Name       string // basename
	Size       int64
	ModifiedAt int64  // Unix timestamp
	DeviceID   string // hostname
}

// skipDirNames are directories whose contents should not be indexed.
var skipDirNames = map[string]bool{
	".git":        true,
	".c4":         true,
	"node_modules": true,
	"__pycache__": true,
	".venv":       true,
	"venv":        true,
}

// CreateTables creates the file_index table if it does not exist.
// Safe to call multiple times (idempotent).
func CreateTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_index (
		path        TEXT    NOT NULL,
		device_id   TEXT    NOT NULL,
		name        TEXT    NOT NULL,
		size        INTEGER NOT NULL,
		modified_at INTEGER NOT NULL,
		indexed_at  INTEGER NOT NULL,
		PRIMARY KEY (path, device_id)
	)`)
	if err != nil {
		return fmt.Errorf("fileindex: create table: %w", err)
	}
	return nil
}

// Index scans dir recursively and upserts entries into file_index.
// Only entries whose mtime has changed are written.
// After the walk, stale entries for this deviceID are removed.
// Returns total files visited (indexed) and count of rows written/updated.
func Index(db *sql.DB, dir string, deviceID string) (indexed int, updated int, err error) {
	now := time.Now().Unix()

	// Track paths seen during this walk to detect deletions.
	seen := make(map[string]struct{})

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// skip unreadable entries
			return nil
		}
		if d.IsDir() {
			if skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		mtime := info.ModTime().Unix()
		seen[path] = struct{}{}
		indexed++

		// Check existing entry.
		var existingMtime int64
		queryErr := db.QueryRow(
			`SELECT modified_at FROM file_index WHERE path=? AND device_id=?`,
			path, deviceID,
		).Scan(&existingMtime)

		if queryErr == nil && existingMtime == mtime {
			// Unchanged — skip.
			return nil
		}

		// New or changed — upsert.
		_, execErr := db.Exec(
			`INSERT OR REPLACE INTO file_index (path, device_id, name, size, modified_at, indexed_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			path, deviceID, d.Name(), info.Size(), mtime, now,
		)
		if execErr != nil {
			return fmt.Errorf("fileindex: upsert %s: %w", path, execErr)
		}
		updated++
		return nil
	})
	if walkErr != nil {
		return indexed, updated, fmt.Errorf("fileindex: walk %s: %w", dir, walkErr)
	}

	// Remove stale entries: rows for this device whose paths no longer exist.
	rows, queryErr := db.Query(
		`SELECT path FROM file_index WHERE device_id=?`, deviceID,
	)
	if queryErr != nil {
		return indexed, updated, fmt.Errorf("fileindex: query stale: %w", queryErr)
	}
	defer rows.Close()

	var stale []string
	for rows.Next() {
		var p string
		if scanErr := rows.Scan(&p); scanErr != nil {
			continue
		}
		if _, ok := seen[p]; !ok {
			stale = append(stale, p)
		}
	}
	if err := rows.Err(); err != nil {
		return indexed, updated, fmt.Errorf("fileindex: iterate stale: %w", err)
	}

	for _, p := range stale {
		if _, delErr := db.Exec(
			`DELETE FROM file_index WHERE path=? AND device_id=?`, p, deviceID,
		); delErr != nil {
			return indexed, updated, fmt.Errorf("fileindex: delete stale %s: %w", p, delErr)
		}
	}

	return indexed, updated, nil
}
