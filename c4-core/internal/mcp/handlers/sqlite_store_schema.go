package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// initSchema creates all required tables if they don't exist.
// It also detects and migrates legacy Python c4_tasks schema.
func (s *SQLiteStore) initSchema() error {
	// Migrate legacy c4_tasks if needed (before CREATE TABLE IF NOT EXISTS)
	if err := s.migrateTasksIfNeeded(); err != nil {
		fmt.Fprintf(os.Stderr, "c4: tasks migration failed (non-fatal): %v\n", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS c4_state (
			project_id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_tasks (
			task_id      TEXT PRIMARY KEY,
			title        TEXT NOT NULL,
			scope        TEXT DEFAULT '',
			dod          TEXT DEFAULT '',
			status       TEXT DEFAULT 'pending',
			dependencies TEXT DEFAULT '[]',
			domain       TEXT DEFAULT '',
			priority     INTEGER DEFAULT 0,
			model        TEXT DEFAULT '',
			execution_mode TEXT DEFAULT 'worker',
			worker_id    TEXT DEFAULT '',
			branch       TEXT DEFAULT '',
			commit_sha   TEXT DEFAULT '',
			files_changed TEXT DEFAULT '',
			review_decision_evidence TEXT DEFAULT '',
			failure_signature TEXT DEFAULT '',
			blocked_attempts INTEGER DEFAULT 0,
			last_error TEXT DEFAULT '',
			superseded_by TEXT NOT NULL DEFAULT '',
			session_id   TEXT DEFAULT '',
			created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_checkpoints (
			checkpoint_id    TEXT PRIMARY KEY,
			decision         TEXT NOT NULL,
			notes            TEXT DEFAULT '',
			required_changes TEXT DEFAULT '[]',
			target_task_id   TEXT DEFAULT '',
			target_review_id TEXT DEFAULT '',
			created_at       TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS persona_stats (
			persona_id   TEXT NOT NULL,
			task_id      TEXT NOT NULL,
			outcome      TEXT NOT NULL,
			review_score REAL DEFAULT 0.0,
			feedback     TEXT DEFAULT '',
			created_at   TEXT NOT NULL,
			UNIQUE(persona_id, task_id)
		)`,
		`CREATE TABLE IF NOT EXISTS twin_growth (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			username    TEXT NOT NULL,
			metric      TEXT NOT NULL,
			value       REAL NOT NULL,
			period      TEXT NOT NULL,
			recorded_at TEXT DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(username, metric, period)
		)`,
		`CREATE TABLE IF NOT EXISTS c4_agent_traces (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			agent_id   TEXT DEFAULT '',
			task_id    TEXT DEFAULT '',
			detail     TEXT DEFAULT '',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS c4_lighthouses (
			name         TEXT PRIMARY KEY,
			description  TEXT NOT NULL DEFAULT '',
			input_schema TEXT NOT NULL DEFAULT '{}',
			spec         TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'stub',
			version      INTEGER NOT NULL DEFAULT 1,
			created_by   TEXT DEFAULT '',
			promoted_by  TEXT DEFAULT '',
			task_id      TEXT DEFAULT '',
			created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	// Best-effort migrations for existing tables
	migrations := []string{
		"ALTER TABLE c4_tasks ADD COLUMN handoff TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN execution_mode TEXT DEFAULT 'worker'",
		"ALTER TABLE c4_tasks ADD COLUMN review_decision_evidence TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN failure_signature TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN blocked_attempts INTEGER DEFAULT 0",
		"ALTER TABLE c4_tasks ADD COLUMN last_error TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN files_changed TEXT DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN superseded_by TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE c4_tasks ADD COLUMN session_id TEXT DEFAULT ''",
		"ALTER TABLE c4_lighthouses ADD COLUMN task_id TEXT DEFAULT ''",
		"ALTER TABLE c4_checkpoints ADD COLUMN target_task_id TEXT DEFAULT ''",
		"ALTER TABLE c4_checkpoints ADD COLUMN target_review_id TEXT DEFAULT ''",
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}

	// Best-effort: apply migrations (ignore only "duplicate column" errors)
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				fmt.Fprintf(os.Stderr, "c4: warning: migration failed: %v\n", err)
			}
		}
	}

	// Best-effort: create indexes (idempotent via IF NOT EXISTS)
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_tasks_superseded_by ON c4_tasks(superseded_by) WHERE superseded_by != ''",
		"CREATE INDEX IF NOT EXISTS idx_c4_tasks_session ON c4_tasks(session_id)",
	}
	for _, idx := range indexes {
		if _, err := s.db.Exec(idx); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: index creation failed: %v\n", err)
		}
	}

	return nil
}

// migrateTasksIfNeeded detects legacy Python c4_tasks schema and migrates data.
// Legacy schema: (project_id, task_id, task_json, status, assigned_to, updated_at)
// Go schema: (task_id, title, scope, dod, status, dependencies, domain, priority, model, ...)
func (s *SQLiteStore) migrateTasksIfNeeded() error {
	// Check if c4_tasks exists at all
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='c4_tasks'").Scan(&count)
	if err != nil || count == 0 {
		return nil // Table doesn't exist yet — initSchema will create it
	}

	// Check if it's legacy schema (has task_json column = legacy)
	var hasTaskJSON bool
	rows, err := s.db.Query("PRAGMA table_info(c4_tasks)")
	if err != nil {
		return fmt.Errorf("checking table info: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == "task_json" {
			hasTaskJSON = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading table_info: %w", err)
	}

	if !hasTaskJSON {
		return nil // Already Go schema — no migration needed
	}

	fmt.Fprintln(os.Stderr, "c4: detected legacy c4_tasks schema, migrating...")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Rename legacy table
	if _, err := tx.Exec("ALTER TABLE c4_tasks RENAME TO c4_tasks_legacy"); err != nil {
		return fmt.Errorf("rename legacy table: %w", err)
	}

	// 2. Create new table with Go schema
	if _, err := tx.Exec(`CREATE TABLE c4_tasks (
		task_id      TEXT PRIMARY KEY,
		title        TEXT NOT NULL,
		scope        TEXT DEFAULT '',
		dod          TEXT DEFAULT '',
		status       TEXT DEFAULT 'pending',
		dependencies TEXT DEFAULT '[]',
		domain       TEXT DEFAULT '',
		priority     INTEGER DEFAULT 0,
		model        TEXT DEFAULT '',
		worker_id    TEXT DEFAULT '',
		branch       TEXT DEFAULT '',
		commit_sha   TEXT DEFAULT '',
		review_decision_evidence TEXT DEFAULT '',
		failure_signature TEXT DEFAULT '',
		blocked_attempts INTEGER DEFAULT 0,
		last_error TEXT DEFAULT '',
		created_at   TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at   TEXT DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	// 3. Migrate data from legacy task_json
	legacyRows, err := tx.Query("SELECT task_id, task_json, status, updated_at FROM c4_tasks_legacy")
	if err != nil {
		return fmt.Errorf("read legacy rows: %w", err)
	}
	migrated := 0
	for legacyRows.Next() {
		var taskID, taskJSON, status string
		var updatedAt sql.NullString
		if err := legacyRows.Scan(&taskID, &taskJSON, &status, &updatedAt); err != nil {
			continue
		}

		var m map[string]any
		if err := json.Unmarshal([]byte(taskJSON), &m); err != nil {
			continue
		}

		title, _ := m["title"].(string)
		if title == "" {
			title = taskID
		}
		scope, _ := m["scope"].(string)
		dod, _ := m["dod"].(string)
		domain, _ := m["domain"].(string)
		model, _ := m["model"].(string)
		workerID, _ := m["assigned_to"].(string)
		branch, _ := m["branch"].(string)
		commitSHA, _ := m["commit_sha"].(string)

		priority := 0
		if p, ok := m["priority"].(float64); ok {
			priority = int(p)
		}

		deps := "[]"
		if d, ok := m["dependencies"]; ok {
			if dBytes, err := json.Marshal(d); err == nil {
				deps = string(dBytes)
			}
		}

		ts := time.Now().Format(time.RFC3339)
		if updatedAt.Valid && updatedAt.String != "" {
			ts = updatedAt.String
		}

		// Use status from task_json if richer, else from column
		if statusStr, ok := m["status"].(string); ok && statusStr != "" {
			status = statusStr
		}

		_, err := tx.Exec(`INSERT OR IGNORE INTO c4_tasks
			(task_id, title, scope, dod, status, dependencies, domain, priority, model, worker_id, branch, commit_sha, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskID, title, scope, dod, status, deps, domain, priority, model, workerID, branch, commitSHA, ts, ts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "c4: migrate task %s failed: %v\n", taskID, err)
			continue
		}
		migrated++
	}
	legacyRows.Close()
	if err := legacyRows.Err(); err != nil {
		return fmt.Errorf("iterating legacy rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	fmt.Fprintf(os.Stderr, "c4: migrated %d tasks from legacy schema\n", migrated)
	return nil
}
