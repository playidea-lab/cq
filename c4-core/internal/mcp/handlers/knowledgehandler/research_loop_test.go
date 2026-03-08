package knowledgehandler

import (
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

func setupResearchLoopTest(t *testing.T) (*mcp.Registry, *KnowledgeNativeOpts) {
	t.Helper()
	dir := t.TempDir()
	store, err := knowledge.NewStore(filepath.Join(dir, "knowledge"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	opts := &KnowledgeNativeOpts{Store: store}
	reg := mcp.NewRegistry()
	RegisterResearchLoopHandlers(reg, opts)
	return reg, opts
}

func seedHypothesis(t *testing.T, store *knowledge.Store) string {
	t.Helper()
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "Test Hypothesis",
		"status": "pending",
	}, "## Hypothesis\nTest body.")
	if err != nil {
		t.Fatalf("seed hypothesis: %v", err)
	}
	return hypID
}

func TestResearchLoopStart_MissingHypothesisID(t *testing.T) {
	reg, _ := setupResearchLoopTest(t)
	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing hypothesis_id")
	}
}

func TestResearchLoopStart_HypothesisNotFound(t *testing.T) {
	reg, _ := setupResearchLoopTest(t)
	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": "hyp-nonexistent",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for non-existent hypothesis")
	}
}

func TestResearchLoopStart_WrongDocType(t *testing.T) {
	reg, opts := setupResearchLoopTest(t)
	// Create an experiment doc (not a hypothesis)
	expID, err := opts.Store.Create(knowledge.TypeExperiment, map[string]any{
		"title": "Not a Hypothesis",
	}, "body")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}
	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": expID,
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error when doc is not a hypothesis")
	}
}

func TestResearchLoopStart_HappyPath(t *testing.T) {
	reg, opts := setupResearchLoopTest(t)
	hypID := seedHypothesis(t, opts.Store)

	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": hypID,
	})

	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}

	loopID, ok := result["loop_id"].(string)
	if !ok || loopID == "" {
		t.Errorf("expected loop_id, got %v", result["loop_id"])
	}
	if result["hypothesis_id"] != hypID {
		t.Errorf("expected hypothesis_id=%s, got %v", hypID, result["hypothesis_id"])
	}
	if result["status"] != "running" {
		t.Errorf("expected status=running, got %v", result["status"])
	}
	if result["budget"] == nil {
		t.Error("expected budget field")
	}
}

func TestResearchLoopStart_WithBudget(t *testing.T) {
	reg, opts := setupResearchLoopTest(t)
	hypID := seedHypothesis(t, opts.Store)

	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id":  hypID,
		"max_iterations": float64(5),
		"max_cost_usd":   float64(2.5),
	})

	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}

	budget, ok := result["budget"].(map[string]any)
	if !ok {
		t.Fatalf("expected budget map, got %T", result["budget"])
	}
	if budget["max_iterations"] != 5 {
		t.Errorf("expected max_iterations=5, got %v", budget["max_iterations"])
	}
	if budget["max_cost_usd"] != 2.5 {
		t.Errorf("expected max_cost_usd=2.5, got %v", budget["max_cost_usd"])
	}
}

func TestResearchLoopStart_DuplicateLoop(t *testing.T) {
	reg, opts := setupResearchLoopTest(t)
	hypID := seedHypothesis(t, opts.Store)

	// Start loop once
	first := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": hypID,
	})
	if _, hasErr := first["error"]; hasErr {
		t.Fatalf("first loop start failed: %v", first["error"])
	}

	// Try to start again — should fail
	second := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": hypID,
	})
	if _, hasErr := second["error"]; !hasErr {
		t.Error("expected error for duplicate loop on same hypothesis")
	}
}

func TestResearchLoopStart_DefaultNullResultThreshold(t *testing.T) {
	reg, opts := setupResearchLoopTest(t)
	hypID := seedHypothesis(t, opts.Store)

	result := callHandler(t, reg, "c4_research_loop_start", map[string]any{
		"hypothesis_id": hypID,
	})

	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("unexpected error: %v", result["error"])
	}

	budget, ok := result["budget"].(map[string]any)
	if !ok {
		t.Fatalf("expected budget map, got %T", result["budget"])
	}
	if budget["null_result_threshold"] != 2 {
		t.Errorf("expected default null_result_threshold=2, got %v", budget["null_result_threshold"])
	}
}
