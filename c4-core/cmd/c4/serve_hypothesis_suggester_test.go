package main

import (
	"context"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/serve"
)

// compile-time: hypothesisSuggester implements serve.Component
var _ serve.Component = (*hypothesisSuggester)(nil)

// stubLLMProvider is a minimal llm.Provider for testing.
type stubLLMProvider struct {
	response string
}

func (s *stubLLMProvider) Name() string                    { return "stub" }
func (s *stubLLMProvider) IsAvailable() bool               { return true }
func (s *stubLLMProvider) Models() []llm.ModelInfo         { return nil }
func (s *stubLLMProvider) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: s.response}, nil
}

func newTestGateway(response string) *llm.Gateway {
	gw := llm.NewGateway(llm.RoutingTable{})
	gw.Register(&stubLLMProvider{response: response})
	return gw
}

func newTestKnowledgeStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return ks
}

func newTestSuggesterCfg(threshold int, ttl time.Duration) config.ServeHypothesisSuggesterConfig {
	return config.ServeHypothesisSuggesterConfig{
		Enabled:   true,
		Threshold: threshold,
		Interval:  "30s",
		TTL:       ttl,
	}
}

// TestHypothesisSuggester_TriggerOnThreshold: 5 experiments → 1 TypeHypothesis created.
func TestHypothesisSuggester_TriggerOnThreshold(t *testing.T) {
	ks := newTestKnowledgeStore(t)
	gw := newTestGateway("hypothesis: experiments suggest X")

	for i := 0; i < 5; i++ {
		if _, err := ks.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body"); err != nil {
			t.Fatalf("Create experiment: %v", err)
		}
	}

	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), gw, ks)
	h.poll(context.Background())

	hyps, err := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if err != nil {
		t.Fatalf("List hypotheses: %v", err)
	}
	if len(hyps) != 1 {
		t.Errorf("expected 1 TypeHypothesis, got %d", len(hyps))
	}
}

// TestHypothesisSuggester_NoTriggerBelowThreshold: 4 experiments → no hypothesis.
func TestHypothesisSuggester_NoTriggerBelowThreshold(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	for i := 0; i < 4; i++ {
		ks.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body") //nolint:errcheck
	}

	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), nil, ks)
	h.poll(context.Background())

	hyps, err := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(hyps) != 0 {
		t.Errorf("expected 0 TypeHypothesis, got %d", len(hyps))
	}
}

// TestHypothesisSuggester_TTLCleanup: expired hypothesis → marked expired.
func TestHypothesisSuggester_TTLCleanup(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	// Store expires_at in frontmatter metadata (same format poll() uses)
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)

	_, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
		"hypothesis_status": "pending",
		"expires_at":        past,
	}, "old hypothesis")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}

	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), nil, ks)
	h.cleanup()

	docs, err := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected 1 doc")
	}
	status, _ := docs[0]["hypothesis_status"].(string)
	if status != "expired" {
		t.Errorf("expected hypothesis_status=expired, got %q", status)
	}
}

// TestHypothesisSuggester_LLMFailure: empty gateway (no providers) → no panic, poll completes.
func TestHypothesisSuggester_LLMFailure(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	for i := 0; i < 5; i++ {
		ks.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body") //nolint:errcheck
	}

	// Gateway with no providers → Chat returns error
	gw := llm.NewGateway(llm.RoutingTable{})
	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), gw, ks)

	// Should not panic; hypothesis is created with empty insight (graceful degradation)
	h.poll(context.Background())

	hyps, err := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Hypothesis is created even when LLM fails (empty insight)
	if len(hyps) != 1 {
		t.Errorf("expected 1 TypeHypothesis (with empty insight), got %d", len(hyps))
	}
}

// TestHypothesisSuggester_InvalidTTL: TTL=0 → Start() returns error.
func TestHypothesisSuggester_InvalidTTL(t *testing.T) {
	ks := newTestKnowledgeStore(t)
	// Bypass newHypothesisSuggester default TTL by constructing directly with TTL=0
	h := &hypothesisSuggester{
		cfg: config.ServeHypothesisSuggesterConfig{
			Enabled:   true,
			Threshold: 5,
			Interval:  "30s",
			TTL:       0,
		},
		kStore:   ks,
		interval: 30 * time.Second,
		status:   "ok",
	}
	err := h.Start(context.Background())
	if err == nil {
		h.Stop(context.Background()) //nolint:errcheck
		t.Error("expected error for TTL=0, got nil")
	}
}

// TestHypothesisSuggester_CrossComponent: poll() creates hypothesis with unified status fields
// so that runSuggestList (checks doc.Status) and cleanup() (checks hypothesis_status) both work.
func TestHypothesisSuggester_CrossComponent(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	// Trigger poll() to create a hypothesis
	for i := 0; i < 5; i++ {
		ks.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body") //nolint:errcheck
	}
	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), nil, ks)
	h.poll(context.Background())

	// Hypothesis should be visible to CLI (doc.Status == "pending")
	hyps, err := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(hyps) == 0 {
		t.Fatal("expected 1 hypothesis after poll")
	}
	id, _ := hyps[0]["id"].(string)
	doc, err := ks.Get(id)
	if err != nil || doc == nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	if doc.Status != "pending" {
		t.Errorf("doc.Status = %q, want pending (CLI must find this hypothesis)", doc.Status)
	}
	if doc.HypothesisStatus != "pending" {
		t.Errorf("doc.HypothesisStatus = %q, want pending (cleanup must find this hypothesis)", doc.HypothesisStatus)
	}
}

// TestHypothesisSuggester_CleanupPreservesBody: cleanup() must not overwrite hypothesis body.
func TestHypothesisSuggester_CleanupPreservesBody(t *testing.T) {
	ks := newTestKnowledgeStore(t)

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	wantBody := "original insight text"

	_, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
		"hypothesis_status": "pending",
		"status":            "pending",
		"expires_at":        past,
	}, wantBody)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), nil, ks)
	h.cleanup()

	docs, _ := ks.List(string(knowledge.TypeHypothesis), "", 10)
	if len(docs) == 0 {
		t.Fatal("expected 1 doc")
	}
	id, _ := docs[0]["id"].(string)
	doc, err := ks.Get(id)
	if err != nil || doc == nil {
		t.Fatalf("Get: %v", err)
	}
	if doc.Body != wantBody {
		t.Errorf("body was overwritten: got %q, want %q", doc.Body, wantBody)
	}
}

// TestHypothesisSuggester_ComponentInterface: implements serve.Component with correct Name/Health.
func TestHypothesisSuggester_ComponentInterface(t *testing.T) {
	ks := newTestKnowledgeStore(t)
	h := newHypothesisSuggester(newTestSuggesterCfg(5, 24*time.Hour), nil, ks)

	if h.Name() != "hypothesis-suggester" {
		t.Errorf("Name() = %q, want hypothesis-suggester", h.Name())
	}
	health := h.Health()
	if health.Status != "ok" {
		t.Errorf("Health().Status = %q, want ok", health.Status)
	}
}
