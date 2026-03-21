package observe

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const dbChanCap = 1000

// dbOp is a single database operation sent to the async writer goroutine.
type dbOp struct {
	kind string // "trace_insert", "step_insert", "trace_outcome", "trace_end"

	// trace_insert fields
	traceID   string
	sessionID string
	taskID    string
	createdAt time.Time

	// step_insert fields
	step TraceStep

	// trace_outcome fields
	outcomeJSON string

	// trace_end fields
	endedAt time.Time
}

// TraceCollector manages the lifecycle of traces and persists them to SQLite.
// It follows the CostTracker chan+goroutine pattern for async writes.
type TraceCollector struct {
	// async SQLite persistence (nil if SetDB not called)
	ch chan dbOp
	wg sync.WaitGroup
}

// NewTraceCollector creates a new TraceCollector ready for use.
// Call SetDB to enable SQLite persistence.
func NewTraceCollector() *TraceCollector {
	return &TraceCollector{}
}

// SetDB wires an open *sql.DB for async persistence.
// It starts a background writer goroutine. Must be called at most once.
// Safe to skip (operations become no-ops if SetDB is not called).
func (tc *TraceCollector) SetDB(db *sql.DB) {
	if db == nil {
		return
	}
	if tc.ch != nil {
		slog.Warn("observe: TraceCollector.SetDB called more than once, ignoring")
		return
	}

	// Ensure trace tables exist (idempotent).
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS traces (
    id          TEXT    PRIMARY KEY,
    session_id  TEXT    NOT NULL DEFAULT '',
    task_id     TEXT    NOT NULL DEFAULT '',
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
`); err != nil {
		slog.Warn("observe: failed to create trace tables", "err", err)
		return
	}

	tc.ch = make(chan dbOp, dbChanCap)
	tc.wg.Add(1)
	go tc.writer(db)
}

// writer is the background goroutine that drains tc.ch and executes DB operations.
func (tc *TraceCollector) writer(db *sql.DB) {
	defer tc.wg.Done()
	for op := range tc.ch {
		switch op.kind {
		case "trace_insert":
			if _, err := db.Exec(
				`INSERT INTO traces (id, session_id, task_id, created_at) VALUES (?, ?, ?, ?)`,
				op.traceID, op.sessionID, op.taskID,
				op.createdAt.UTC().Format(time.RFC3339Nano),
			); err != nil {
				slog.Warn("observe: failed to insert trace", "trace_id", op.traceID, "err", err)
			}

		case "step_insert":
			s := op.step
			if _, err := db.Exec(
				`INSERT INTO trace_steps
					(trace_id, step_type, ts, provider, model, task_type, tool_name,
					 input_tok, output_tok, latency_ms, cost_usd, success, error_msg)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.TraceID,
				string(s.StepType),
				s.Timestamp.UTC().Format(time.RFC3339Nano),
				s.Provider, s.Model, s.TaskType, s.ToolName,
				s.InputTok, s.OutputTok, s.LatencyMs, s.CostUSD,
				boolToInt(s.Success), s.ErrorMsg,
			); err != nil {
				slog.Warn("observe: failed to insert trace_step", "trace_id", s.TraceID, "err", err)
			}

		case "trace_outcome":
			if _, err := db.Exec(
				`UPDATE traces SET outcome_json = ? WHERE id = ?`,
				op.outcomeJSON, op.traceID,
			); err != nil {
				slog.Warn("observe: failed to update trace outcome", "trace_id", op.traceID, "err", err)
			}

		case "trace_ensure":
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO traces (id, session_id, created_at) VALUES (?, ?, ?)`,
				op.traceID, op.sessionID,
				op.createdAt.UTC().Format(time.RFC3339Nano),
			); err != nil {
				slog.Warn("observe: failed to ensure trace", "trace_id", op.traceID, "err", err)
			}

		case "trace_end":
			if _, err := db.Exec(
				`UPDATE traces SET ended_at = ? WHERE id = ?`,
				op.endedAt.UTC().Format(time.RFC3339Nano), op.traceID,
			); err != nil {
				slog.Warn("observe: failed to update trace ended_at", "trace_id", op.traceID, "err", err)
			}
		}
	}
}

// send enqueues an operation. Drops with a warning if the channel is full.
func (tc *TraceCollector) send(op dbOp) {
	if tc.ch == nil {
		return
	}
	select {
	case tc.ch <- op:
	default:
		slog.Warn("observe: trace collector buffer full, dropping operation", "kind", op.kind, "trace_id", op.traceID)
	}
}

// StartTrace creates a new trace, inserts it into SQLite, and returns the trace ID.
func (tc *TraceCollector) StartTrace(sessionID, taskID string) string {
	traceID := uuid.New().String()
	tc.send(dbOp{
		kind:      "trace_insert",
		traceID:   traceID,
		sessionID: sessionID,
		taskID:    taskID,
		createdAt: time.Now(),
	})
	return traceID
}

// AddStep appends a step to the given trace (async write).
func (tc *TraceCollector) AddStep(traceID string, step TraceStep) {
	step.TraceID = traceID
	if step.Timestamp.IsZero() {
		step.Timestamp = time.Now()
	}
	tc.send(dbOp{
		kind: "step_insert",
		step: step,
	})
}

// SetOutcome updates the outcome_json column of the given trace (async write).
func (tc *TraceCollector) SetOutcome(traceID string, outcome TraceOutcome) {
	data, err := json.Marshal(outcome)
	if err != nil {
		slog.Warn("observe: failed to marshal TraceOutcome", "trace_id", traceID, "err", err)
		return
	}
	tc.send(dbOp{
		kind:        "trace_outcome",
		traceID:     traceID,
		outcomeJSON: string(data),
	})
}

// EndTrace sets ended_at on the given trace (async write).
func (tc *TraceCollector) EndTrace(traceID string) {
	tc.send(dbOp{
		kind:    "trace_end",
		traceID: traceID,
		endedAt: time.Now(),
	})
}

// Close drains the channel and waits for the writer goroutine to finish.
// No-op if SetDB was never called.
func (tc *TraceCollector) Close() {
	if tc.ch == nil {
		return
	}
	close(tc.ch)
	tc.wg.Wait()
}

// EnsureTrace creates a parent trace row if it doesn't already exist.
// Uses INSERT OR IGNORE so concurrent calls for the same traceID are safe.
func (tc *TraceCollector) EnsureTrace(traceID string) {
	tc.send(dbOp{
		kind:      "trace_ensure",
		traceID:   traceID,
		sessionID: traceID,
		createdAt: time.Now(),
	})
}

// boolToInt converts a bool to SQLite-compatible 0/1.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
