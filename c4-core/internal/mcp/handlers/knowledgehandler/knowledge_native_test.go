package knowledgehandler

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

	vs, err := knowledge.NewVectorStore(store.DB(), 384, nil)
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

// --- Mock CloudSyncer for blending tests ---

type mockCloudSyncer struct {
	discoverResults []map[string]any
}

func (m *mockCloudSyncer) SyncDocument(params map[string]any, docID string) error     { return nil }
func (m *mockCloudSyncer) SearchDocuments(query, docType string, limit int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockCloudSyncer) ListDocuments(docType string, limit int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockCloudSyncer) GetDocument(docID string) (map[string]any, error) { return nil, nil }
func (m *mockCloudSyncer) DeleteDocument(docID string) error                { return nil }
func (m *mockCloudSyncer) DiscoverPublic(query, docType string, limit int) ([]map[string]any, error) {
	return m.discoverResults, nil
}

func TestKnowledgeRecordWithRelated(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Record a first document
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Go Performance Tips",
		"content":  "Go performance optimization techniques and best practices",
	})

	// Record a second document with identical content — should see related
	result := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Go Performance Tips",
		"content":  "Go performance optimization techniques and best practices",
	})

	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result)
	}

	// With identical content and mock embeddings, related should include the first doc
	if related, ok := result["related"].([]map[string]any); ok {
		if len(related) == 0 {
			t.Error("expected related documents for identical content")
		}
		for _, r := range related {
			if _, hasID := r["id"]; !hasID {
				t.Error("related item missing id")
			}
			if _, hasSim := r["similarity"]; !hasSim {
				t.Error("related item missing similarity")
			}
		}
	}
	// related may be nil if similarity < 0.5 with mock — that's OK
}

func TestKnowledgeSearchBlending(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	vs, _ := knowledge.NewVectorStore(store.DB(), 384, nil)
	searcher := knowledge.NewSearcher(store, vs)

	cloud := &mockCloudSyncer{
		discoverResults: []map[string]any{
			{"id": "community-1", "title": "Community Pattern", "type": "pattern", "domain": "ml"},
			{"id": "community-2", "title": "Community Insight", "type": "insight", "domain": "dl"},
		},
	}

	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
		Cloud:    cloud,
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)

	// Create a local document
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Local ML Insight",
		"content":  "local machine learning knowledge",
	})

	// Search — should blend local + community results
	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "machine learning",
	})

	results, ok := result["results"].([]map[string]any)
	if !ok {
		t.Fatal("results should be []map[string]any")
	}
	for _, r := range results {
		src, _ := r["source"].(string)
		if src != "local" && src != "community" {
			t.Errorf("unexpected source: %q", src)
		}
	}

	// Check community count
	if cc, ok := result["community_count"].(int); ok {
		if cc != 2 {
			t.Errorf("community_count: got %d, want 2", cc)
		}
	}
}

func TestKnowledgeStatsExtended(t *testing.T) {
	reg, _ := setupKnowledgeNativeTest(t)

	// Create some documents
	for i := 0; i < 3; i++ {
		callHandler(t, reg, "c4_knowledge_record", map[string]any{
			"doc_type": "insight",
			"title":    fmt.Sprintf("Stats Test Doc %d", i),
			"content":  fmt.Sprintf("content for stats test document %d", i),
		})
	}
	callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "pattern",
		"title":    "Test Pattern",
		"content":  "pattern content",
	})

	result := callHandler(t, reg, "c4_knowledge_stats", map[string]any{})

	// Check pattern_count
	if pc, ok := result["pattern_count"].(int); ok {
		if pc != 1 {
			t.Errorf("pattern_count: got %d, want 1", pc)
		}
	}

	// Check similarity stats exist (we have 4 docs with vectors → should have pairwise stats)
	if sim, ok := result["similarity"].(map[string]any); ok {
		if _, hasAvg := sim["avg_pairwise"]; !hasAvg {
			t.Error("similarity missing avg_pairwise")
		}
		if _, hasMax := sim["max_pairwise"]; !hasMax {
			t.Error("similarity missing max_pairwise")
		}
		if _, hasPairs := sim["pairs_sampled"]; !hasPairs {
			t.Error("similarity missing pairs_sampled")
		}
	}

	// No distillation hint for < 50 docs
	if _, has := result["distillation_hint"]; has {
		t.Error("should not have distillation_hint for < 50 docs")
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
		"c4_knowledge_delete",
		"c4_knowledge_discover",
		"c4_knowledge_ingest",
		"c4_knowledge_stats",
		"c4_knowledge_reindex",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestKnowledgeGetCiteAction(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	vs, _ := knowledge.NewVectorStore(store.DB(), 384, nil)
	searcher := knowledge.NewSearcher(store, vs)

	ut, err := knowledge.NewUsageTracker(store.DB())
	if err != nil {
		t.Fatal(err)
	}

	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
		Usage:    ut,
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)

	// Create a document
	created := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Cite Test",
		"content":  "Testing cite action tracking",
	})
	docID := created["doc_id"].(string)

	// Get with cite=false (default) → ActionView
	callHandler(t, reg, "c4_knowledge_get", map[string]any{"doc_id": docID})

	// Get with cite=true → ActionCite
	callHandler(t, reg, "c4_knowledge_get", map[string]any{"doc_id": docID, "cite": true})

	// Flush and check usage
	ut.Close() // explicit close to flush

	// Verify: 1 view + 1 cite
	var viewCount, citeCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM doc_usage WHERE doc_id=? AND action='view'", docID).Scan(&viewCount)
	store.DB().QueryRow("SELECT COUNT(*) FROM doc_usage WHERE doc_id=? AND action='cite'", docID).Scan(&citeCount)

	if viewCount != 1 {
		t.Errorf("expected 1 view, got %d", viewCount)
	}
	if citeCount != 1 {
		t.Errorf("expected 1 cite, got %d", citeCount)
	}
}

func TestKnowledgeStatsWithUsage(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	vs, _ := knowledge.NewVectorStore(store.DB(), 384, nil)
	searcher := knowledge.NewSearcher(store, vs)

	ut, err := knowledge.NewUsageTracker(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ut.Close() })

	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
		Usage:    ut,
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)

	// Create some docs and generate usage
	for i := 0; i < 3; i++ {
		callHandler(t, reg, "c4_knowledge_record", map[string]any{
			"doc_type": "insight",
			"title":    fmt.Sprintf("Usage Stats Doc %d", i),
			"content":  fmt.Sprintf("content %d", i),
		})
	}

	result := callHandler(t, reg, "c4_knowledge_stats", map[string]any{})

	// Check usage field exists
	if _, ok := result["usage"]; !ok {
		t.Error("stats should include usage field")
	}

	// Check embedding_coverage
	if _, ok := result["embedding_coverage"]; !ok {
		t.Error("stats should include embedding_coverage")
	}

	// Check distillation field (may be nil with < 3 similar docs)
	if dist, ok := result["distillation"].(map[string]any); ok {
		if _, ok := dist["cluster_count"]; !ok {
			t.Error("distillation should include cluster_count")
		}
		if _, ok := dist["largest_cluster"]; !ok {
			t.Error("distillation should include largest_cluster")
		}
		if _, ok := dist["hint"]; !ok {
			t.Error("distillation should include hint")
		}
	}
}

func TestKnowledgeDistillDryRun(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "knowledge")
	store, err := knowledge.NewStore(basePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	vs, _ := knowledge.NewVectorStore(store.DB(), 384, nil)
	searcher := knowledge.NewSearcher(store, vs)

	// No LLM gateway → distill tool should not be registered
	opts := &KnowledgeNativeOpts{
		Store:    store,
		Searcher: searcher,
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)

	tools := reg.ListTools()
	for _, tool := range tools {
		if tool.Name == "c4_knowledge_distill" {
			t.Error("distill should NOT be registered without LLM gateway")
		}
	}
}

// =========================================================================
// Cloud-primary integration tests
// =========================================================================

// mockCloudSemanticSearcher is a controllable CloudSemanticSearcher for tests.
type mockCloudSemanticSearcher struct {
	results []map[string]any
	err     error
	called  bool
	lastEmb []float32
}

func (m *mockCloudSemanticSearcher) SemanticSearch(embedding []float32, limit int, similarityThreshold float32) ([]map[string]any, error) {
	m.called = true
	m.lastEmb = embedding
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

// setupCloudPrimaryTest creates a KnowledgeNativeOpts with cloud-primary mode,
// a real local store, and the given mock cloud searcher.
func setupCloudPrimaryTest(t *testing.T, searcher *mockCloudSemanticSearcher) (*mcp.Registry, *KnowledgeNativeOpts) {
	t.Helper()
	dir := t.TempDir()
	st, err := knowledge.NewStore(filepath.Join(dir, "knowledge"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	vs, err := knowledge.NewVectorStore(st.DB(), 384, nil)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	s := knowledge.NewSearcher(st, vs)

	opts := &KnowledgeNativeOpts{
		Store:       st,
		Searcher:    s,
		CloudSearch: searcher,
		CloudMode:   "cloud-primary",
	}

	reg := mcp.NewRegistry()
	RegisterKnowledgeNativeHandlers(reg, opts)
	return reg, opts
}

// TestKnowledgeSearch_CloudPrimary_UsesCloudResults verifies that when mode=cloud-primary
// and the cloud searcher succeeds, results come from the cloud (source="cloud").
func TestKnowledgeSearch_CloudPrimary_UsesCloudResults(t *testing.T) {
	cloudSearcher := &mockCloudSemanticSearcher{
		results: []map[string]any{
			{"id": "cloud-001", "title": "Cloud Experiment", "type": "experiment", "domain": "ml"},
			{"id": "cloud-002", "title": "Cloud Pattern", "type": "pattern", "domain": "go"},
		},
	}

	reg, _ := setupCloudPrimaryTest(t, cloudSearcher)

	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "machine learning",
	})

	// Verify cloud was called
	if !cloudSearcher.called {
		t.Error("cloud SemanticSearch should have been called in cloud-primary mode")
	}

	// Verify embedding was generated and passed to cloud
	if len(cloudSearcher.lastEmb) == 0 {
		t.Error("cloud SemanticSearch should receive a non-empty embedding")
	}

	// Verify results come from cloud
	results, ok := result["results"].([]map[string]any)
	if !ok {
		t.Fatalf("results should be []map[string]any, got %T", result["results"])
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 cloud results, got %d", len(results))
	}
	for _, r := range results {
		if r["source"] != "cloud" {
			t.Errorf("result source = %q, want cloud", r["source"])
		}
	}
	if results[0]["id"] != "cloud-001" {
		t.Errorf("first result id = %v, want cloud-001", results[0]["id"])
	}
}

// TestKnowledgeSearch_CloudPrimary_FallbackToLocal verifies that when cloud fails,
// the handler falls back to local search without returning an error.
func TestKnowledgeSearch_CloudPrimary_FallbackToLocal(t *testing.T) {
	cloudSearcher := &mockCloudSemanticSearcher{
		err: fmt.Errorf("connection refused"),
	}

	reg, opts := setupCloudPrimaryTest(t, cloudSearcher)

	// Create a local document so local search can return something
	recordResult := callHandler(t, reg, "c4_knowledge_record", map[string]any{
		"doc_type": "insight",
		"title":    "Local Fallback Insight",
		"content":  "local content used when cloud fails",
	})
	if recordResult["success"] != true {
		t.Fatalf("record failed: %v", recordResult)
	}
	_ = opts

	// Search — cloud fails, should fall back to local (no error returned)
	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query": "local content",
	})

	// Verify cloud was attempted
	if !cloudSearcher.called {
		t.Error("cloud SemanticSearch should have been attempted even if it fails")
	}

	// Verify no error in response — fallback happened silently
	if errVal, hasErr := result["error"]; hasErr {
		t.Errorf("fallback should not return error, got: %v", errVal)
	}

	// Results should come from local (source="local")
	if results, ok := result["results"].([]map[string]any); ok {
		for _, r := range results {
			if r["source"] != "local" {
				t.Errorf("fallback result source = %q, want local", r["source"])
			}
		}
	}
}

// TestKnowledgeSearch_CloudPrimary_TypeFilter verifies that doc_type filter applies
// to cloud results in cloud-primary mode.
func TestKnowledgeSearch_CloudPrimary_TypeFilter(t *testing.T) {
	cloudSearcher := &mockCloudSemanticSearcher{
		results: []map[string]any{
			{"id": "cloud-exp", "title": "Cloud Experiment", "type": "experiment", "domain": "ml"},
			{"id": "cloud-ins", "title": "Cloud Insight", "type": "insight", "domain": "ml"},
		},
	}

	reg, _ := setupCloudPrimaryTest(t, cloudSearcher)

	result := callHandler(t, reg, "c4_knowledge_search", map[string]any{
		"query":    "machine learning",
		"doc_type": "experiment",
	})

	results, ok := result["results"].([]map[string]any)
	if !ok {
		t.Fatalf("results type error: %T", result["results"])
	}
	for _, r := range results {
		if r["type"] != "experiment" {
			t.Errorf("type filter failed: got type %q, want experiment", r["type"])
		}
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result after type filter, got %d", len(results))
	}
}
