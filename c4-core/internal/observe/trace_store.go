package observe

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // SQLite driver
)

// TraceStore manages persistence of traces and trace_steps in SQLite.
type TraceStore struct {
	db *sql.DB
}

// NewTraceStore opens (or creates) a SQLite database at dbPath and returns
// a TraceStore ready for use. Call CreateTable before recording any traces.
func NewTraceStore(dbPath string) (*TraceStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("observe: open sqlite %s: %w", dbPath, err)
	}
	// Single writer to avoid SQLITE_BUSY under concurrent access.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return &TraceStore{db: db}, nil
}

// Close releases the underlying database connection.
func (s *TraceStore) Close() error {
	return s.db.Close()
}

// CreateTable ensures the traces and trace_steps tables (and their indexes)
// exist. It is idempotent and safe to call on every startup.
func (s *TraceStore) CreateTable() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS traces (
    id          TEXT    PRIMARY KEY,
    session_id  TEXT    NOT NULL DEFAULT '',
    task_id     TEXT    NOT NULL DEFAULT '',
    task_type   TEXT    NOT NULL DEFAULT '',
    project_id  TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    ended_at    TEXT,
    outcome_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_traces_session_id  ON traces (session_id);
CREATE INDEX IF NOT EXISTS idx_traces_task_id     ON traces (task_id);

CREATE TABLE IF NOT EXISTS trace_steps (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id    TEXT    NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    step_type   TEXT    NOT NULL,
    ts          TEXT    NOT NULL,
    provider    TEXT    NOT NULL DEFAULT '',
    model       TEXT    NOT NULL DEFAULT '',
    task_type   TEXT    NOT NULL DEFAULT '',
    tool_name   TEXT    NOT NULL DEFAULT '',
    input_tok   INTEGER NOT NULL DEFAULT 0,
    output_tok  INTEGER NOT NULL DEFAULT 0,
    latency_ms  INTEGER NOT NULL DEFAULT 0,
    cost_usd    REAL    NOT NULL DEFAULT 0,
    success     INTEGER NOT NULL DEFAULT 1,
    error_msg   TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_trace_steps_trace_id        ON trace_steps (trace_id);
CREATE INDEX IF NOT EXISTS idx_trace_steps_model_task_type ON trace_steps (model, task_type);
`
	_, err := s.db.Exec(ddl)
	if err != nil {
		return fmt.Errorf("observe: create tables: %w", err)
	}
	// Migrate: add task_type column to traces if missing (pre-existing DBs).
	s.db.Exec(`ALTER TABLE traces ADD COLUMN task_type TEXT NOT NULL DEFAULT ''`)
	return nil
}
