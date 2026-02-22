package handlers

import (
	"testing"

	"github.com/changmin/c4-core/internal/config"
	_ "modernc.org/sqlite"
)

func TestClassifyTaskRisk(t *testing.T) {
	cfg := config.RiskRoutingConfig{
		Enabled: true,
		Paths: config.RiskPathsConfig{
			High: []string{"infra/migrations/", "internal/auth/"},
			Low:  []string{"docs/", "*.md"},
		},
		Models: config.RiskModelsConfig{
			High: "opus",
			Low:  "haiku",
		},
	}

	tests := []struct {
		name  string
		scope string
		want  string
	}{
		{
			name:  "directory match high",
			scope: "infra/migrations/",
			want:  "high",
		},
		{
			name:  "directory match high with file",
			scope: "infra/migrations/0001.sql",
			want:  "high",
		},
		{
			name:  "directory match low",
			scope: "docs/guide.md",
			want:  "low",
		},
		{
			name:  "glob match low *.md",
			scope: "README.md",
			want:  "low",
		},
		{
			name:  "no match returns default",
			scope: "src/main.go",
			want:  "default",
		},
		{
			name:  "empty scope returns default",
			scope: "",
			want:  "default",
		},
		{
			name:  "multi-scope comma high wins",
			scope: "infra/migrations/001.sql, docs/guide.md",
			want:  "high",
		},
		{
			name:  "multi-scope comma high second",
			scope: "docs/guide.md, infra/migrations/0002.sql",
			want:  "high",
		},
		{
			name:  "multi-scope all low",
			scope: "docs/a.md, docs/b.md",
			want:  "low",
		},
		{
			name:  "multi-scope no match",
			scope: "src/a.go, src/b.go",
			want:  "default",
		},
		{
			name:  "substring match high",
			scope: "internal/auth/handler.go",
			want:  "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTaskRisk(tt.scope, cfg)
			if got != tt.want {
				t.Errorf("classifyTaskRisk(%q) = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}

func TestClassifyTaskRisk_EmptyPatterns(t *testing.T) {
	cfg := config.RiskRoutingConfig{
		Enabled: true,
		Paths: config.RiskPathsConfig{
			High: []string{},
			Low:  []string{},
		},
	}
	got := classifyTaskRisk("src/main.go", cfg)
	if got != "default" {
		t.Errorf("classifyTaskRisk with no patterns = %q, want default", got)
	}
}

func TestClassifyTaskRisk_EmptyPatternSkipped(t *testing.T) {
	cfg := config.RiskRoutingConfig{
		Enabled: true,
		Paths: config.RiskPathsConfig{
			High: []string{"", "infra/"},
			Low:  []string{""},
		},
	}
	// Empty pattern "" should be skipped, not cause false match.
	got := classifyTaskRisk("src/main.go", cfg)
	if got != "default" {
		t.Errorf("classifyTaskRisk with empty pattern = %q, want default", got)
	}
	// "infra/" should still match.
	got = classifyTaskRisk("infra/db.sql", cfg)
	if got != "high" {
		t.Errorf("classifyTaskRisk(infra/db.sql) = %q, want high", got)
	}
}

func TestAssignTask_RiskRouting_HighScope(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", true)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/", "*.md"})
	cfg.Set("risk_routing.models.high", "opus")
	cfg.Set("risk_routing.models.low", "haiku")
	store.config = cfg

	// Parent task
	if err := store.AddTask(&Task{
		ID: "T-RR1-0", Title: "Impl", DoD: "done", Status: "pending",
		ExecutionMode: "direct", Scope: "infra/migrations/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask("T-RR1-0"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.ReportTask("T-RR1-0", "done", []string{"infra/migrations/001.sql"}); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Review task with high-risk scope
	if err := store.AddTask(&Task{
		ID:           "R-RR1-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Scope:        "infra/migrations/",
		Dependencies: []string{"T-RR1-0"},
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-risk")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	if assignment.Model != "opus" {
		t.Errorf("model = %q, want opus (high risk)", assignment.Model)
	}
}

func TestAssignTask_RiskRouting_LowScope(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", true)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/", "*.md"})
	cfg.Set("risk_routing.models.high", "opus")
	cfg.Set("risk_routing.models.low", "haiku")
	store.config = cfg

	// Parent task
	if err := store.AddTask(&Task{
		ID: "T-RR2-0", Title: "Impl", DoD: "done", Status: "pending",
		ExecutionMode: "direct", Scope: "docs/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask("T-RR2-0"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.ReportTask("T-RR2-0", "done", []string{"docs/guide.md"}); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Review task with low-risk scope
	if err := store.AddTask(&Task{
		ID:           "R-RR2-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Scope:        "docs/guide.md",
		Dependencies: []string{"T-RR2-0"},
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-risk2")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	if assignment.Model != "haiku" {
		t.Errorf("model = %q, want haiku (low risk)", assignment.Model)
	}
}

func TestAssignTask_RiskRouting_Disabled(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", false)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/"})
	cfg.Set("risk_routing.models.high", "opus")
	cfg.Set("risk_routing.models.low", "haiku")
	store.config = cfg

	// Parent task
	if err := store.AddTask(&Task{
		ID: "T-RR3-0", Title: "Impl", DoD: "done", Status: "pending",
		ExecutionMode: "direct", Scope: "infra/migrations/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask("T-RR3-0"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.ReportTask("T-RR3-0", "done", []string{"infra/001.sql"}); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Review task — risk_routing disabled, so model should NOT be overridden
	if err := store.AddTask(&Task{
		ID:           "R-RR3-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Scope:        "infra/migrations/",
		Dependencies: []string{"T-RR3-0"},
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-risk3")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	// Model should be from GetModelForTask (economic mode), not risk routing.
	if assignment.Model == "opus" {
		t.Errorf("model = opus, but risk_routing is disabled — should not override")
	}
}

func TestAssignTask_RiskRouting_TaskRowModelPreserved(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", true)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/"})
	cfg.Set("risk_routing.models.high", "opus")
	cfg.Set("risk_routing.models.low", "haiku")
	store.config = cfg

	// Parent task
	if err := store.AddTask(&Task{
		ID: "T-RR4-0", Title: "Impl", DoD: "done", Status: "pending",
		ExecutionMode: "direct", Scope: "infra/migrations/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask("T-RR4-0"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.ReportTask("T-RR4-0", "done", []string{"infra/001.sql"}); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Review task with explicit model in task row — override forbidden
	if err := store.AddTask(&Task{
		ID:           "R-RR4-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Scope:        "infra/migrations/",
		Dependencies: []string{"T-RR4-0"},
	}); err != nil {
		t.Fatal(err)
	}
	// Set model directly in task row
	if _, err := db.Exec("UPDATE c4_tasks SET model='sonnet' WHERE task_id='R-RR4-0'"); err != nil {
		t.Fatalf("set model: %v", err)
	}

	assignment, err := store.AssignTask("worker-risk4")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	if assignment.Model != "sonnet" {
		t.Errorf("model = %q, want sonnet (task row model should not be overridden)", assignment.Model)
	}
}

func TestAssignTask_RiskRouting_TTaskNoOverride(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", true)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/"})
	cfg.Set("risk_routing.models.high", "opus")
	cfg.Set("risk_routing.models.low", "haiku")
	store.config = cfg

	// T- task with high-risk scope — should NOT be overridden
	if err := store.AddTask(&Task{
		ID:     "T-RR5-0",
		Title:  "Impl",
		DoD:    "done",
		Status: "pending",
		Scope:  "infra/migrations/",
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-risk5")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	// T- task should not get risk routing override
	if assignment.Model == "opus" {
		t.Errorf("model = opus, but T- task should not get risk routing override")
	}
}

func TestAssignTask_RiskRouting_EmptyModelsWarn(t *testing.T) {
	store, db := newTestSQLiteStore(t)
	defer db.Close()

	cfg, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Set("risk_routing.enabled", true)
	cfg.Set("risk_routing.paths.high", []string{"infra/migrations/"})
	cfg.Set("risk_routing.paths.low", []string{"docs/"})
	cfg.Set("risk_routing.models.high", "")
	cfg.Set("risk_routing.models.low", "")
	store.config = cfg

	// Parent task
	if err := store.AddTask(&Task{
		ID: "T-RR6-0", Title: "Impl", DoD: "done", Status: "pending",
		ExecutionMode: "direct", Scope: "infra/migrations/",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTask("T-RR6-0"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.ReportTask("T-RR6-0", "done", []string{"infra/001.sql"}); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Review task — Models.High="" should trigger slog.Warn and skip override
	if err := store.AddTask(&Task{
		ID:           "R-RR6-0",
		Title:        "Review",
		DoD:          "review",
		Status:       "pending",
		Scope:        "infra/migrations/",
		Dependencies: []string{"T-RR6-0"},
	}); err != nil {
		t.Fatal(err)
	}

	assignment, err := store.AssignTask("worker-risk6")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment, got nil")
	}
	// Should NOT override model when Models.High is empty.
	if assignment.Model == "opus" || assignment.Model == "haiku" {
		t.Errorf("model = %q, want no risk override when Models are empty", assignment.Model)
	}
}
