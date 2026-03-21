package observe_test

import (
	"database/sql"
	"testing"

	"github.com/changmin/c4-core/internal/observe"
)

// insertStep is a helper used by policy tests to seed trace_steps rows.
func insertStep(t *testing.T, db *sql.DB, traceID, model, taskType string, success int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO trace_steps (trace_id, step_type, ts, model, task_type, success)
		 VALUES (?, 'llm', '2024-01-01T00:00:00Z', ?, ?, ?)`,
		traceID, model, taskType, success,
	)
	if err != nil {
		t.Fatalf("insertStep: %v", err)
	}
}

func TestPolicy_SuggestRoutes_MinSamples(t *testing.T) {
	db := openTestDB(t)

	// Insert a trace.
	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at)
		VALUES ('tp1', 's1', 't1', '2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// Insert 6 steps for "code" task type → should exceed minSamples=5.
	for i := 0; i < 6; i++ {
		insertStep(t, db, "tp1", "model-a", "code", 1)
	}
	// Insert only 3 steps for "review" task type → below minSamples=5.
	for i := 0; i < 3; i++ {
		insertStep(t, db, "tp1", "model-b", "review", 1)
	}

	analyzer := observe.NewTraceAnalyzer(db)
	policy := observe.NewTraceDrivenPolicy(analyzer, 5)

	routes, err := policy.SuggestRoutes()
	if err != nil {
		t.Fatalf("SuggestRoutes: %v", err)
	}

	// "code" should be in routes.
	ref, ok := routes["code"]
	if !ok {
		t.Fatal("expected 'code' in suggested routes")
	}
	if ref.Model != "model-a" {
		t.Errorf("expected model-a for 'code', got %s", ref.Model)
	}

	// "review" should NOT be in routes (only 3 samples < 5).
	if _, ok := routes["review"]; ok {
		t.Error("expected 'review' to be excluded (below minSamples)")
	}
}

func TestPolicy_SuggestRoutes_BestComposite(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at)
		VALUES ('tp2', 's2', 't2', '2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// model-good: 5 successes; model-bad: 5 failures → model-good has higher composite.
	for i := 0; i < 5; i++ {
		insertStep(t, db, "tp2", "model-good", "gen", 1)
		insertStep(t, db, "tp2", "model-bad", "gen", 0)
	}

	analyzer := observe.NewTraceAnalyzer(db)
	policy := observe.NewTraceDrivenPolicy(analyzer, 5)

	routes, err := policy.SuggestRoutes()
	if err != nil {
		t.Fatalf("SuggestRoutes: %v", err)
	}

	ref, ok := routes["gen"]
	if !ok {
		t.Fatal("expected 'gen' in suggested routes")
	}
	if ref.Model != "model-good" {
		t.Errorf("expected model-good (higher composite), got %s", ref.Model)
	}
}

func TestPolicy_SuggestRoutes_EmptyDB(t *testing.T) {
	db := openTestDB(t)

	analyzer := observe.NewTraceAnalyzer(db)
	policy := observe.NewTraceDrivenPolicy(analyzer, 5)

	routes, err := policy.SuggestRoutes()
	if err != nil {
		t.Fatalf("SuggestRoutes on empty db: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("expected empty routes on empty db, got %d entries", len(routes))
	}
}

func TestPolicy_DefaultMinSamples(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at)
		VALUES ('tp3', 's3', 't3', '2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// 4 steps < default minSamples(5).
	for i := 0; i < 4; i++ {
		insertStep(t, db, "tp3", "model-x", "analyze", 1)
	}

	analyzer := observe.NewTraceAnalyzer(db)
	// Pass 0 → should use default of 5.
	policy := observe.NewTraceDrivenPolicy(analyzer, 0)

	routes, err := policy.SuggestRoutes()
	if err != nil {
		t.Fatalf("SuggestRoutes: %v", err)
	}
	if _, ok := routes["analyze"]; ok {
		t.Error("expected 'analyze' excluded with default minSamples=5 and only 4 samples")
	}
}
