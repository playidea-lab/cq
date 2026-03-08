package knowledgehandler

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

func setupSuggestTest(t *testing.T, mock *llm.MockProvider) (*mcp.Registry, *KnowledgeNativeOpts) {
	t.Helper()
	dir := t.TempDir()
	store, err := knowledge.NewStore(filepath.Join(dir, "knowledge"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	var gw *llm.Gateway
	if mock != nil {
		gw = llm.NewGateway(llm.RoutingTable{Default: mock.Name()})
		gw.Register(mock)
	}

	opts := &KnowledgeNativeOpts{
		Store: store,
		LLM:   gw,
	}
	reg := mcp.NewRegistry()
	reg.Register(mcp.ToolSchema{Name: "c4_research_suggest"}, researchSuggestNativeHandler(opts))
	return reg, opts
}

// seedExperiments inserts n experiment docs into the store.
func seedExperiments(t *testing.T, store *knowledge.Store, n int, tag string) {
	t.Helper()
	for i := 0; i < n; i++ {
		tags := []string{}
		if tag != "" {
			tags = []string{tag}
		}
		meta := map[string]any{"title": "exp title", "tags": tags}
		if _, err := store.Create(knowledge.TypeExperiment, meta, "body"); err != nil {
			t.Fatalf("seed exp %d: %v", i, err)
		}
	}
}

func TestResearchSuggest_HappyPath(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	mock.Response = &llm.ChatResponse{
		Content:      `{"insight":"Use lower learning rate for better convergence","yaml_draft":"run: python train.py\n"}`,
		Model:        "mock-model",
		FinishReason: "stop",
	}

	reg, opts := setupSuggestTest(t, mock)
	seedExperiments(t, opts.Store, 10, "")

	result := callHandler(t, reg, "c4_research_suggest", map[string]any{})

	if _, ok := result["error"]; ok {
		t.Fatalf("unexpected error: %v", result)
	}
	hypID, ok := result["hypothesis_id"].(string)
	if !ok || hypID == "" {
		t.Errorf("expected hypothesis_id, got %v", result["hypothesis_id"])
	}
	if len(hypID) < 4 || hypID[:4] != "hyp-" {
		t.Errorf("expected hyp- prefix, got %s", hypID)
	}
	if result["insight"] == "" {
		t.Errorf("expected non-empty insight")
	}
	count := result["experiment_count"]
	if count != 10 {
		t.Errorf("expected experiment_count=10, got %v", count)
	}
}

func TestResearchSuggest_TagFilter(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	mock.Response = &llm.ChatResponse{
		Content:      `{"insight":"mnist experiments show batch size matters","yaml_draft":""}`,
		Model:        "mock-model",
		FinishReason: "stop",
	}

	reg, opts := setupSuggestTest(t, mock)
	seedExperiments(t, opts.Store, 5, "mnist")
	seedExperiments(t, opts.Store, 5, "other")

	result := callHandler(t, reg, "c4_research_suggest", map[string]any{"tag": "mnist"})

	if _, ok := result["error"]; ok {
		t.Fatalf("unexpected error: %v", result)
	}
	count := result["experiment_count"]
	if count != 5 {
		t.Errorf("expected 5 mnist experiments, got %v", count)
	}
	tags := toStringSliceAny(result["tags"])
	if len(tags) == 0 || tags[0] != "mnist" {
		t.Errorf("expected tags=[mnist], got %v", result["tags"])
	}
}

func TestResearchSuggest_NoExperiments(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	reg, _ := setupSuggestTest(t, mock)

	result := callHandler(t, reg, "c4_research_suggest", map[string]any{})

	if result["error"] != "no_experiments" {
		t.Errorf("expected error=no_experiments, got %v", result)
	}
}

func TestResearchSuggest_LLMError(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	mock.Err = errors.New("api key invalid")

	reg, opts := setupSuggestTest(t, mock)
	seedExperiments(t, opts.Store, 3, "")

	result := callHandler(t, reg, "c4_research_suggest", map[string]any{})

	if result["error"] != "llm_error" {
		t.Errorf("expected error=llm_error, got %v", result)
	}
	if _, ok := result["message"]; !ok {
		t.Errorf("expected message field in llm_error response")
	}
}
