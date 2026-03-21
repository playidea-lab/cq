package observe_test

import (
	"testing"

	"github.com/changmin/c4-core/internal/observe"
)

// seedSteps inserts trace + steps directly into the DB for analyzer tests.
func seedSteps(t *testing.T, db interface {
	Exec(query string, args ...any) (interface{ RowsAffected() (int64, error) }, error)
}) {
	t.Helper()
}

func TestTraceAnalyzer_StatsByTaskType(t *testing.T) {
	db := openTestDB(t)

	// Insert a trace.
	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at) VALUES ('tr1', 's1', 't1', '2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// Insert LLM steps: model-a 2 successes, model-b 1 success 1 failure.
	steps := []struct {
		id       string
		model    string
		taskType string
		success  int
		latency  int
		cost     float64
	}{
		{"tr1", "model-a", "code", 1, 100, 0.001},
		{"tr1", "model-a", "code", 1, 200, 0.002},
		{"tr1", "model-b", "code", 1, 150, 0.003},
		{"tr1", "model-b", "code", 0, 300, 0.004},
	}
	for _, s := range steps {
		_, err := db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model, task_type, success, latency_ms, cost_usd)
			 VALUES (?, 'llm', '2024-01-01T00:00:00Z', ?, ?, ?, ?, ?)`,
			s.id, s.model, s.taskType, s.success, s.latency, s.cost,
		)
		if err != nil {
			t.Fatalf("insert step: %v", err)
		}
	}

	analyzer := observe.NewTraceAnalyzer(db)
	result, err := analyzer.StatsByTaskType()
	if err != nil {
		t.Fatalf("StatsByTaskType: %v", err)
	}

	codeStats, ok := result["code"]
	if !ok {
		t.Fatal("expected 'code' task_type in result")
	}
	if len(codeStats) != 2 {
		t.Fatalf("expected 2 model entries for 'code', got %d", len(codeStats))
	}

	// model-a: 2/2 success = 1.0 success_rate, composite = 0.6
	// model-b: 1/2 success = 0.5 success_rate, composite = 0.3
	// Should be sorted: model-a first.
	if codeStats[0].Model != "model-a" {
		t.Errorf("expected model-a first (highest composite), got %s", codeStats[0].Model)
	}
	if codeStats[0].SuccessRate != 1.0 {
		t.Errorf("expected model-a success_rate=1.0, got %f", codeStats[0].SuccessRate)
	}
	if codeStats[1].Model != "model-b" {
		t.Errorf("expected model-b second, got %s", codeStats[1].Model)
	}
	// model-b: 0.5 success rate
	const eps = 1e-9
	if diff := codeStats[1].SuccessRate - 0.5; diff > eps || diff < -eps {
		t.Errorf("expected model-b success_rate=0.5, got %f", codeStats[1].SuccessRate)
	}
}

func TestTraceAnalyzer_BestModel(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at) VALUES ('tr2', 's2', 't2', '2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// model-x: 3 successes; model-y: 1 success
	for _, row := range []struct {
		model   string
		success int
	}{
		{"model-x", 1},
		{"model-x", 1},
		{"model-x", 1},
		{"model-y", 1},
	} {
		_, err := db.Exec(
			`INSERT INTO trace_steps (trace_id, step_type, ts, model, task_type, success)
			 VALUES ('tr2', 'llm', '2024-01-01T00:00:00Z', ?, 'review', ?)`,
			row.model, row.success,
		)
		if err != nil {
			t.Fatalf("insert step: %v", err)
		}
	}

	analyzer := observe.NewTraceAnalyzer(db)
	best, err := analyzer.BestModel("review")
	if err != nil {
		t.Fatalf("BestModel: %v", err)
	}
	if best != "model-x" {
		t.Errorf("expected best model to be model-x, got %s", best)
	}
}

func TestTraceAnalyzer_BestModel_NotFound(t *testing.T) {
	db := openTestDB(t)

	analyzer := observe.NewTraceAnalyzer(db)
	_, err := analyzer.BestModel("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task_type, got nil")
	}
}

func TestTraceAnalyzer_CompositeFormula(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO traces (id, session_id, task_id, created_at, outcome_json)
		VALUES ('tr3', 's3', 't3', '2024-01-01T00:00:00Z', '{"quality":0.8}')`)
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	// 1 success step with quality=0.8 in outcome
	_, err = db.Exec(
		`INSERT INTO trace_steps (trace_id, step_type, ts, model, task_type, success)
		 VALUES ('tr3', 'llm', '2024-01-01T00:00:00Z', 'model-z', 'gen', 1)`,
	)
	if err != nil {
		t.Fatalf("insert step: %v", err)
	}

	analyzer := observe.NewTraceAnalyzer(db)
	result, err := analyzer.StatsByTaskType()
	if err != nil {
		t.Fatalf("StatsByTaskType: %v", err)
	}

	genStats, ok := result["gen"]
	if !ok || len(genStats) == 0 {
		t.Fatal("expected gen stats")
	}

	ms := genStats[0]
	// success_rate=1.0, avg_quality=0.8, composite=0.6*1.0+0.4*0.8=0.92
	expectedComposite := 0.6*1.0 + 0.4*0.8
	const eps = 1e-9
	if diff := ms.Composite - expectedComposite; diff > eps || diff < -eps {
		t.Errorf("expected composite=%f, got %f", expectedComposite, ms.Composite)
	}
}
