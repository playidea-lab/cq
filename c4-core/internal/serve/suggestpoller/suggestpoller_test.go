package suggestpoller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// compile-time interface assertion (TestSuggestPoller_ImplementsComponent)
var _ serve.Component = (*KnowledgeSuggestPoller)(nil)

func TestSuggestPoller_ImplementsComponent(t *testing.T) {
	p := New(Config{
		Store: mustNewKnowledgeStore(t),
	})
	var _ serve.Component = p
}

func TestSuggestPoller_Watermark_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	wmPath := filepath.Join(dir, "watermark.json")
	p := &KnowledgeSuggestPoller{cfg: Config{WatermarkPath: wmPath}}

	now := time.Now().Truncate(time.Second)
	wm := Watermark{LastAnalyzedAt: now, LastCount: 7}
	if err := p.saveWatermark(wm); err != nil {
		t.Fatalf("saveWatermark: %v", err)
	}

	loaded := p.loadWatermark()
	if !loaded.LastAnalyzedAt.Equal(now) {
		t.Errorf("LastAnalyzedAt: got %v, want %v", loaded.LastAnalyzedAt, now)
	}
	if loaded.LastCount != 7 {
		t.Errorf("LastCount: got %d, want 7", loaded.LastCount)
	}
}

func TestSuggestPoller_Watermark_AtomicSave(t *testing.T) {
	dir := t.TempDir()
	wmPath := filepath.Join(dir, "watermark.json")
	p := &KnowledgeSuggestPoller{cfg: Config{WatermarkPath: wmPath}}

	wm := Watermark{LastAnalyzedAt: time.Now(), LastCount: 3}
	if err := p.saveWatermark(wm); err != nil {
		t.Fatalf("saveWatermark: %v", err)
	}

	// No tmp files should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != filepath.Base(wmPath) && filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}

	// Final file must be valid JSON.
	data, err := os.ReadFile(wmPath)
	if err != nil {
		t.Fatalf("read watermark: %v", err)
	}
	var out Watermark
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestSuggestPoller_CountThreshold_NoAnalysis(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	called := false
	p := New(Config{
		Store:                  store,
		LLMCaller:              &trackingLLMCaller{called: &called},
		NewExperimentThreshold: 5,
		WatermarkPath:          filepath.Join(t.TempDir(), "wm.json"),
	})

	// Add only 3 experiments — below threshold.
	for i := 0; i < 3; i++ {
		store.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body")
	}

	p.poll(context.Background())

	if called {
		t.Error("LLM was called even though count < threshold")
	}
}

func TestSuggestPoller_LLMFailure_NoRetry(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	callCount := 0
	p := New(Config{
		Store: store,
		LLMCaller: &errLLMCaller{
			err:       errLLMFailed,
			callCount: &callCount,
		},
		NewExperimentThreshold: 2,
		WatermarkPath:          filepath.Join(t.TempDir(), "wm.json"),
	})

	for i := 0; i < 3; i++ {
		store.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body")
	}

	p.poll(context.Background())

	if callCount != 1 {
		t.Errorf("expected exactly 1 LLM call (no retry), got %d", callCount)
	}
}

func TestSuggestPoller_Trigger_StoresHypothesis(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	for i := 0; i < 3; i++ {
		store.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body")
	}

	p := New(Config{
		Store: store,
		LLMCaller: &fixedLLMCaller{
			response: `{"insight": "good experiments", "yaml_draft": "run: python train.py"}`,
		},
		WatermarkPath: filepath.Join(t.TempDir(), "wm.json"),
	})

	result, err := p.Trigger(context.Background(), "mytag", 10)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if result.DocID == "" {
		t.Error("expected non-empty DocID")
	}
	if result.Insight != "good experiments" {
		t.Errorf("Insight: got %q", result.Insight)
	}
	if result.YAMLDraft != "run: python train.py" {
		t.Errorf("YAMLDraft: got %q", result.YAMLDraft)
	}

	// Verify the hypothesis document was actually stored.
	doc, err := store.Get(result.DocID)
	if err != nil || doc == nil {
		t.Fatalf("hypothesis document not found: %v", err)
	}
	if doc.Type != knowledge.TypeHypothesis {
		t.Errorf("doc.Type: got %q, want %q", doc.Type, knowledge.TypeHypothesis)
	}
}

func TestSuggestPoller_MalformedLLMResponse(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	for i := 0; i < 3; i++ {
		store.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body")
	}

	p := New(Config{
		Store:         store,
		LLMCaller:     &fixedLLMCaller{response: "not json at all"},
		WatermarkPath: filepath.Join(t.TempDir(), "wm.json"),
	})

	_, err := p.Trigger(context.Background(), "", 10)
	if err == nil {
		t.Error("expected error for malformed LLM response, got nil")
	}

	// No hypothesis should be stored.
	docs, _ := store.List(string(knowledge.TypeHypothesis), "", 10)
	if len(docs) != 0 {
		t.Errorf("expected 0 hypothesis docs, got %d", len(docs))
	}
}

func TestSuggestPoller_TriggerDuringPoll_NoDuplicate(t *testing.T) {
	store := mustNewKnowledgeStore(t)
	for i := 0; i < 3; i++ {
		store.Create(knowledge.TypeExperiment, map[string]any{"title": "exp"}, "body")
	}

	blocked := make(chan struct{})
	unblock := make(chan struct{})
	callCount := 0
	p := New(Config{
		Store: store,
		LLMCaller: &blockingLLMCaller{
			response: `{"insight": "ok", "yaml_draft": "run: x"}`,
			blocked:  blocked,
			unblock:  unblock,
			count:    &callCount,
		},
		WatermarkPath: filepath.Join(t.TempDir(), "wm.json"),
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Trigger(context.Background(), "", 10) //nolint:errcheck
	}()

	// Wait until first call is inside LLM.
	<-blocked

	// Second Trigger should fail immediately (analyzing=true).
	_, err := p.Trigger(context.Background(), "", 10)
	if err == nil {
		t.Error("expected error: analysis already in progress")
	}

	// Unblock the first call.
	close(unblock)
	wg.Wait()
}

// ─── helpers ───────────────────────────────────────────────────────────────

func mustNewKnowledgeStore(t *testing.T) *knowledge.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var errLLMFailed = &mockError{"LLM error"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

type trackingLLMCaller struct {
	called *bool
}

func (c *trackingLLMCaller) Call(_ context.Context, _ string) (string, error) {
	*c.called = true
	return `{"insight": "ok", "yaml_draft": "run: x"}`, nil
}

type fixedLLMCaller struct {
	response string
}

func (c *fixedLLMCaller) Call(_ context.Context, _ string) (string, error) {
	return c.response, nil
}

type errLLMCaller struct {
	err       error
	callCount *int
}

func (c *errLLMCaller) Call(_ context.Context, _ string) (string, error) {
	*c.callCount++
	return "", c.err
}

// blockingLLMCaller blocks until unblock is closed, used to simulate concurrent calls.
type blockingLLMCaller struct {
	response string
	blocked  chan struct{}
	unblock  chan struct{}
	count    *int
	once     sync.Once
}

func (c *blockingLLMCaller) Call(_ context.Context, _ string) (string, error) {
	*c.count++
	c.once.Do(func() { close(c.blocked) })
	<-c.unblock
	return c.response, nil
}
