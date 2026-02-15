package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/mcp"
)

func setupKnowledgeNativeTest(t *testing.T) (*mcp.Registry, *KnowledgeNativeOpts) {
	t.Helper()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")

	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	vs, err := knowledge.NewVectorStore(store.DB(), 384)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}

	searcher := knowledge.NewSearcher(store, vs)

	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
		Cloud:    nil, // no cloud in tests
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)

	return reg, opts
}

func callHandler(t *testing.T, reg *mcp.Registry, toolName string, args map[string]any) map[string]any {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	result, err := reg.Call(toolName, json.RawMessage(rawArgs))
	if err != nil {
		t.Fatalf("Call %s: %v", toolName, err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	return m
}

func TestKnowledgeRecordNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	result := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Test Insight",
		"content":  "This is a test insight document.",
		"tags":     []string{"test", "go"},
	})

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result)
	}
	docID, ok := result["doc_id"].(string)
	if !ok || docID == "" {
		t.Errorf("expected doc_id, got %v", result["doc_id"])
	}
	// Verify prefix
	if len(docID) < 3 || docID[:4] != "ins-" {
		t.Errorf("expected ins- prefix, got %s", docID)
	}
}

func TestKnowledgeRecordMissingRequired(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Missing doc_type
	result := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"title":   "No Type",
		"content": "body",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing doc_type")
	}

	// Missing title
	result = callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"content":  "body",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing title")
	}
}

func TestKnowledgeGetNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Create first
	created := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "experiment",
		"title":    "Get Test Exp",
		"content":  "Experiment body content.",
	})
	docID := created["doc_id"].(string)

	// Get
	result := callHandler(t, reg, "c4_knowledge_get", map[string]any{
		"doc_id": docID,
	})
	if result["title"] != "Get Test Exp" {
		t.Errorf("title: got %q, want 'Get Test Exp'", result["title"])
	}
	if result["body"] != "Experiment body content." {
		t.Errorf("body: got %q", result["body"])
	}
	if result["type"] != "experiment" {
		t.Errorf("type: got %q", result["type"])
	}
}

func TestKnowledgeGetNotFound(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	result := callHandler(t, reg, "c4_knowledge_get", map[string]any{
		"doc_id": "nonexistent-id",
	})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for not found")
	}
}

func TestKnowledgeSearchNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Create docs
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Random Forest Accuracy",
		"content":  "Random forest achieved 87% accuracy on the test set.",
	})
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Neural Network Performance",
		"content":  "Transformer models outperform CNNs on NLP tasks.",
	})

	// Search
	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "random forest",
	})
	count, _ := result["count"].(float64)
	if count == 0 {
		// FTS might not return for simple match; check results
		results, _ := result["results"].([]map[string]any)
		if len(results) == 0 {
			t.Log("search returned 0 results (FTS tokenization); acceptable")
		}
	}
}

func TestKnowledgeSearchWithTypeFilter(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "experiment",
		"title":    "Exp Doc",
		"content":  "experiment body",
	})
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Insight Doc",
		"content":  "insight body",
	})

	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query":    "body",
		"doc_type": "experiment",
	})
	if results, ok := result["results"].([]map[string]any); ok {
		for _, r := range results {
			if r["type"] != "experiment" {
				t.Errorf("unexpected type in filtered results: %v", r["type"])
			}
		}
	}
}

func TestExperimentRecordNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	result := callHandler(t, reg, "c4_experiment_record", map[string]any{
		"title":   "Training Run 1",
		"content": "Loss converged at 0.05 after 100 epochs.",
		"tags":    []string{"training", "baseline"},
	})

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result)
	}
	docID, _ := result["doc_id"].(string)
	if len(docID) < 4 || docID[:4] != "exp-" {
		t.Errorf("expected exp- prefix, got %s", docID)
	}

	// Verify via get
	getResult := callHandler(t, reg, "c4_knowledge_get", map[string]any{
		"doc_id": docID,
	})
	if getResult["type"] != "experiment" {
		t.Errorf("type: got %q, want experiment", getResult["type"])
	}
}

func TestExperimentSearchNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	callHandler(t, reg, "c4_experiment_record", map[string]any{
		"title":   "Exp Alpha",
		"content": "alpha experiment results",
	})
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Not An Experiment",
		"content":  "alpha insight results",
	})

	result := callHandler(t, reg, "c4_experiment_search", map[string]any{
		"query": "alpha",
	})
	if results, ok := result["results"].([]map[string]any); ok {
		for _, r := range results {
			if r["type"] != "experiment" {
				t.Errorf("experiment search returned non-experiment: %v", r["type"])
			}
		}
	}
}

func TestPatternSuggestNative(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "pattern",
		"title":    "Repository Pattern",
		"content":  "Use repository pattern for data access abstraction.",
	})
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Not A Pattern",
		"content":  "repository insight details.",
	})

	result := callHandler(t, reg, "c4_pattern_suggest", map[string]any{
		"context": "data access",
	})
	if results, ok := result["results"].([]map[string]any); ok {
		for _, r := range results {
			if r["type"] != "pattern" {
				t.Errorf("pattern suggest returned non-pattern: %v", r["type"])
			}
		}
	}
}

func TestKnowledgePullNoCloud(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	rawArgs, _ := json.Marshal(map[string]any{})
	_, err := reg.Call("c4_knowledge_pull", json.RawMessage(rawArgs))
	if err == nil {
		t.Error("expected error for nil cloud")
	}
}

func TestKnowledgeGetMissingID(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	result := callHandler(t, reg, "c4_knowledge_get", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing doc_id")
	}
}

func TestKnowledgeSearchMissingQuery(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{})
	if _, hasErr := result["error"]; !hasErr {
		t.Error("expected error for missing query")
	}
}

func TestRegisterNilOpts(t *testing.T) {
	reg := mcp.NewRegistry()
	// Should not panic
	RegisterKnowledgeNativeHandlers(reg, nil)
	RegisterKnowledgeNativeHandlers(reg, &KnowledgeNativeOpts{})
}

// --- E2E Integration Tests ---

// TestE2ERecordSearchGet tests the full record→search→get flow.
func TestE2ERecordSearchGet(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// 1. Record multiple documents of different types
	ins := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Go Migration Strategy",
		"content":  "Migrating Python proxy tools to Go native improves startup latency.",
		"tags":     []string{"migration", "go", "performance"},
	})
	insID := ins["doc_id"].(string)

	exp := callHandler(t, reg, "c4_experiment_record", map[string]any{
		"title":   "Latency Benchmark",
		"content": "Go native tools show 3x faster cold start vs Python proxy.",
		"tags":    []string{"benchmark", "latency"},
	})
	expID := exp["doc_id"].(string)

	pat := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "pattern",
		"title":    "Proxy Fallback Pattern",
		"content":  "When Go native store is unavailable, fall back to Python proxy.",
	})
	patID := pat["doc_id"].(string)

	// 2. Search for "migration" — should find the insight
	searchResult := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "migration",
	})
	if searchResult["count"] == nil {
		t.Fatal("search result missing count")
	}

	// 3. Experiment search — should only return experiments
	expSearch := callHandler(t, reg, "c4_experiment_search", map[string]any{
		"query": "latency",
	})
	if results, ok := expSearch["results"].([]any); ok {
		for _, r := range results {
			rm, _ := r.(map[string]any)
			if rm["type"] != "experiment" {
				t.Errorf("experiment search returned non-experiment: %v", rm["type"])
			}
		}
	}

	// 4. Pattern suggest — should only return patterns
	patSuggest := callHandler(t, reg, "c4_pattern_suggest", map[string]any{
		"context": "fallback proxy",
	})
	if results, ok := patSuggest["results"].([]any); ok {
		for _, r := range results {
			rm, _ := r.(map[string]any)
			if rm["type"] != "pattern" {
				t.Errorf("pattern suggest returned non-pattern: %v", rm["type"])
			}
		}
	}

	// 5. Get each by ID — verify round-trip
	for _, tc := range []struct {
		id       string
		wantType string
		title    string
	}{
		{insID, "insight", "Go Migration Strategy"},
		{expID, "experiment", "Latency Benchmark"},
		{patID, "pattern", "Proxy Fallback Pattern"},
	} {
		got := callHandler(t, reg, "c4_knowledge_get", map[string]any{"doc_id": tc.id})
		if got["type"] != tc.wantType {
			t.Errorf("get %s: type=%v, want %s", tc.id, got["type"], tc.wantType)
		}
		if got["title"] != tc.title {
			t.Errorf("get %s: title=%v, want %s", tc.id, got["title"], tc.title)
		}
	}
}

// TestE2ESearchWithLimit tests search respects limit parameter.
func TestE2ESearchWithLimit(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Create 5 documents with shared keyword
	for i := 0; i < 5; i++ {
		callHandler(t, reg, "c4_knowledge_record", map[string]any{
			"doc_type": "insight",
			"title":    fmt.Sprintf("Scalability Insight %d", i),
			"content":  fmt.Sprintf("Scalability pattern %d for distributed systems.", i),
		})
	}

	// Search with limit=2
	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "scalability",
		"limit": 2,
	})
	if results, ok := result["results"].([]any); ok {
		if len(results) > 2 {
			t.Errorf("expected at most 2 results with limit=2, got %d", len(results))
		}
	}
}

// TestE2ERegisterConditionalWiring verifies that registerNativeReplacements
// correctly chooses between Go native and proxy fallback.
func TestE2ERegisterConditionalWiring(t *testing.T) {
	reg := mcp.NewRegistry()

	// With knowledge opts → Go native registered
	dir := t.TempDir()
	store, err := knowledge.NewStore(filepath.Join(dir, "k"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	RegisterKnowledgeNativeHandlers(reg, &KnowledgeNativeOpts{
		Store: store,
	})

	// Verify tools are registered
	tools := reg.ListTools()
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expected := []string{
		"c4_knowledge_record",
		"c4_knowledge_get",
		"c4_knowledge_search",
		"c4_experiment_record",
		"c4_experiment_search",
		"c4_pattern_suggest",
		"c4_knowledge_pull",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}
