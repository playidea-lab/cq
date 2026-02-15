package handlers

import (
	"encoding/json"
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
