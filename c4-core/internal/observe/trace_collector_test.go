package observe_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/changmin/c4-core/internal/observe"
)

// openTestDB opens an in-memory (or temp file) SQLite DB and creates the
// traces/trace_steps tables via TraceStore.CreateTable.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "trace_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	db, err := sql.Open("sqlite", f.Name())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Use NewTraceStore to create the DDL tables.
	tstore, err := observe.NewTraceStore(f.Name())
	if err != nil {
		t.Fatalf("new trace store: %v", err)
	}
	if err := tstore.CreateTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Close the TraceStore's own connection; we'll use db directly.
	tstore.Close()
	db.Close()

	// Reopen for test use.
	db, err = sql.Open("sqlite", f.Name())
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTraceCollector_StartTrace(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)

	traceID := tc.StartTrace("session-1", "task-1")
	if traceID == "" {
		t.Fatal("expected non-empty traceID")
	}

	// Flush by closing.
	tc.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM traces WHERE id = ?`, traceID).Scan(&count); err != nil {
		t.Fatalf("query traces: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 trace row, got %d", count)
	}
}

func TestTraceCollector_AddStep(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)

	traceID := tc.StartTrace("session-2", "task-2")
	tc.AddStep(traceID, observe.TraceStep{
		StepType:  observe.StepTypeLLM,
		Provider:  "anthropic",
		Model:     "claude-3",
		InputTok:  100,
		OutputTok: 50,
		LatencyMs: 200,
		CostUSD:   0.001,
		Success:   true,
	})
	tc.AddStep(traceID, observe.TraceStep{
		StepType: observe.StepTypeTool,
		ToolName: "bash",
		Success:  true,
	})

	tc.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM trace_steps WHERE trace_id = ?`, traceID).Scan(&count); err != nil {
		t.Fatalf("query trace_steps: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 trace_step rows, got %d", count)
	}
}

func TestTraceCollector_SetOutcome(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)

	traceID := tc.StartTrace("session-3", "task-3")
	outcome := observe.TraceOutcome{
		TotalInputTok:  500,
		TotalOutputTok: 200,
		TotalCostUSD:   0.005,
		TotalLatencyMs: 1000,
		StepCount:      3,
		Success:        true,
	}
	tc.SetOutcome(traceID, outcome)
	tc.Close()

	var outcomeJSON sql.NullString
	if err := db.QueryRow(`SELECT outcome_json FROM traces WHERE id = ?`, traceID).Scan(&outcomeJSON); err != nil {
		t.Fatalf("query traces: %v", err)
	}
	if !outcomeJSON.Valid || outcomeJSON.String == "" {
		t.Errorf("expected non-empty outcome_json, got: %v", outcomeJSON)
	}
}

func TestTraceCollector_EndTrace(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)

	traceID := tc.StartTrace("session-4", "task-4")
	tc.EndTrace(traceID)
	tc.Close()

	var endedAt sql.NullString
	if err := db.QueryRow(`SELECT ended_at FROM traces WHERE id = ?`, traceID).Scan(&endedAt); err != nil {
		t.Fatalf("query traces: %v", err)
	}
	if !endedAt.Valid || endedAt.String == "" {
		t.Errorf("expected non-empty ended_at, got: %v", endedAt)
	}
}

func TestTraceCollector_NoSetDB(t *testing.T) {
	// Without SetDB, all operations should be no-ops (no panic).
	tc := observe.NewTraceCollector()
	traceID := tc.StartTrace("s", "t")
	if traceID == "" {
		t.Fatal("expected non-empty traceID even without DB")
	}
	tc.AddStep(traceID, observe.TraceStep{StepType: observe.StepTypeTool, ToolName: "x"})
	tc.SetOutcome(traceID, observe.TraceOutcome{})
	tc.EndTrace(traceID)
	tc.Close() // should not block
}

func TestTraceCollector_SetDBCalledTwice(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)
	// Second call should be a no-op (warns but does not panic).
	tc.SetDB(db)
	t.Cleanup(tc.Close)
}

func TestTraceCollector_StepTimestamp(t *testing.T) {
	db := openTestDB(t)

	tc := observe.NewTraceCollector()
	tc.SetDB(db)

	traceID := tc.StartTrace("s", "t")
	// Add a step with an explicit timestamp.
	explicit := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	tc.AddStep(traceID, observe.TraceStep{
		StepType:  observe.StepTypeTask,
		Timestamp: explicit,
		Success:   true,
	})
	tc.Close()

	var ts string
	if err := db.QueryRow(`SELECT ts FROM trace_steps WHERE trace_id = ?`, traceID).Scan(&ts); err != nil {
		t.Fatalf("query trace_steps: %v", err)
	}
	if ts == "" {
		t.Error("expected non-empty ts")
	}
}
