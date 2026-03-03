package pop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// --- mock implementations ---

type mockMessageSource struct {
	msgs []Message
	err  error
}

func (m *mockMessageSource) RecentMessages(_ context.Context, _ time.Time, _ int) ([]Message, error) {
	return m.msgs, m.err
}

type mockKnowledgeStore struct {
	recorded []Proposal
	err      error
}

func (m *mockKnowledgeStore) RecordProposal(_ context.Context, p Proposal) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.recorded = append(m.recorded, p)
	return "id-" + p.Title, nil
}

type mockSoulWriter struct {
	insights []string
	err      error
}

func (m *mockSoulWriter) AppendInsight(_ context.Context, _, insight string) error {
	if m.err != nil {
		return m.err
	}
	m.insights = append(m.insights, insight)
	return nil
}

type mockNotifier struct {
	notified []Proposal
	err      error
}

func (m *mockNotifier) Notify(_ context.Context, p Proposal) error {
	if m.err != nil {
		return m.err
	}
	m.notified = append(m.notified, p)
	return nil
}

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

// --- helpers ---

func newTestEngine(t *testing.T, msgs *mockMessageSource, ks *mockKnowledgeStore, sw *mockSoulWriter, n *mockNotifier, llm *mockLLMClient) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	gaugeFile := filepath.Join(dir, "gauge.json")
	e := NewEngine(msgs, ks, sw, n, llm, stateFile, gaugeFile)
	return e, stateFile, gaugeFile
}

// --- tests ---

func TestRunOnce_NoMessages(t *testing.T) {
	msgs := &mockMessageSource{msgs: nil}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{response: "[]"}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with no messages: %v", err)
	}

	// No proposals should have been recorded or notified.
	if len(ks.recorded) != 0 {
		t.Fatalf("expected 0 recorded proposals, got %d", len(ks.recorded))
	}
	if len(n.notified) != 0 {
		t.Fatalf("expected 0 notified proposals, got %d", len(n.notified))
	}
}

func TestRunOnce_WithMessages_ProposalsExtracted(t *testing.T) {
	msgs := &mockMessageSource{msgs: []Message{
		{ID: "1", Content: "We should always validate inputs.", CreatedAt: time.Now().Add(-10 * time.Minute)},
	}}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{
		response: `[{"title":"Input Validation","content":"Always validate inputs.","item_type":"rule","confidence":0.9,"visibility":"team"}]`,
	}

	e, stateFile, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(ks.recorded) != 1 {
		t.Fatalf("expected 1 recorded proposal, got %d", len(ks.recorded))
	}
	if ks.recorded[0].Title != "Input Validation" {
		t.Fatalf("unexpected title: %q", ks.recorded[0].Title)
	}

	if len(n.notified) != 1 {
		t.Fatalf("expected 1 notified proposal, got %d", len(n.notified))
	}

	if len(sw.insights) != 1 {
		t.Fatalf("expected 1 soul insight, got %d", len(sw.insights))
	}

	// State should have been updated.
	state, err := Load(stateFile)
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	if state.LastExtractedAt.IsZero() {
		t.Fatal("expected LastExtractedAt to be updated")
	}
	if state.LastCrystallizedAt.IsZero() {
		t.Fatal("expected LastCrystallizedAt to be updated")
	}
}

func TestRunOnce_StateLastExtractedAt_UsedAsSince(t *testing.T) {
	var capturedAfter time.Time
	captureSrc := &capturingMessageSource{}

	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{response: "[]"}

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	gaugeFile := filepath.Join(dir, "gauge.json")

	// Pre-populate state with a known LastExtractedAt.
	known := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	s := &PopState{LastExtractedAt: known}
	if err := s.Save(stateFile); err != nil {
		t.Fatal(err)
	}

	e := NewEngine(captureSrc, ks, sw, n, llm, stateFile, gaugeFile)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	capturedAfter = captureSrc.capturedAfter
	if !capturedAfter.Equal(known) {
		t.Fatalf("expected RecentMessages called with after=%v, got %v", known, capturedAfter)
	}
}

// capturingMessageSource records the `after` argument passed to RecentMessages.
type capturingMessageSource struct {
	capturedAfter time.Time
}

func (c *capturingMessageSource) RecentMessages(_ context.Context, after time.Time, _ int) ([]Message, error) {
	c.capturedAfter = after
	return nil, nil
}

func TestRunOnce_MessageSourceError(t *testing.T) {
	msgs := &mockMessageSource{err: errors.New("db down")}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	err := e.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, msgs.err) {
		t.Fatalf("expected wrapped db error, got: %v", err)
	}
}

func TestRunOnce_LLMError(t *testing.T) {
	msgs := &mockMessageSource{msgs: []Message{{ID: "1", Content: "hello", CreatedAt: time.Now()}}}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{err: errors.New("llm unavailable")}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	err := e.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, llm.err) {
		t.Fatalf("expected wrapped llm error, got: %v", err)
	}
}

func TestRunOnce_KnowledgeStoreError(t *testing.T) {
	msgs := &mockMessageSource{msgs: []Message{{ID: "1", Content: "hello", CreatedAt: time.Now()}}}
	ks := &mockKnowledgeStore{err: errors.New("store error")}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{
		response: `[{"title":"X","content":"Y","item_type":"fact","confidence":0.8,"visibility":"private"}]`,
	}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	err := e.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ks.err) {
		t.Fatalf("expected wrapped store error, got: %v", err)
	}
}

func TestRunOnce_NotifierError_NonFatal(t *testing.T) {
	msgs := &mockMessageSource{msgs: []Message{{ID: "1", Content: "hello", CreatedAt: time.Now()}}}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{err: errors.New("notify failed")}
	llm := &mockLLMClient{
		response: `[{"title":"X","content":"Y","item_type":"fact","confidence":0.8,"visibility":"private"}]`,
	}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	// Notifier error should not abort RunOnce.
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("expected RunOnce to succeed despite notifier error, got: %v", err)
	}

	// Knowledge store should still have recorded the proposal.
	if len(ks.recorded) != 1 {
		t.Fatalf("expected 1 recorded proposal despite notifier failure, got %d", len(ks.recorded))
	}
}

func TestRunOnce_SoulWriterError_NonFatal(t *testing.T) {
	msgs := &mockMessageSource{msgs: []Message{{ID: "1", Content: "hello", CreatedAt: time.Now()}}}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{err: errors.New("soul write failed")}
	n := &mockNotifier{}
	llm := &mockLLMClient{
		response: `[{"title":"X","content":"Y","item_type":"fact","confidence":0.8,"visibility":"private"}]`,
	}

	e, _, _ := newTestEngine(t, msgs, ks, sw, n, llm)

	// SoulWriter error should not abort RunOnce.
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("expected RunOnce to succeed despite soul write error, got: %v", err)
	}
}

func TestRunOnce_GaugeThresholdExceeded(t *testing.T) {
	msgs := &mockMessageSource{msgs: nil}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{response: "[]"}

	e, _, gaugeFile := newTestEngine(t, msgs, ks, sw, n, llm)

	// Pre-write gauge data that exceeds a threshold.
	gt := NewGaugeTracker(gaugeFile)
	if err := gt.Load(); err != nil {
		t.Fatal(err)
	}
	gt.Set("contradictions", 99.0) // well above ThresholdContradictions=10
	if err := gt.Save(); err != nil {
		t.Fatal(err)
	}

	err := e.RunOnce(context.Background())
	if !errors.Is(err, ErrGaugeThresholdExceeded) {
		t.Fatalf("expected ErrGaugeThresholdExceeded, got: %v", err)
	}
}

func TestRunOnce_GaugeThresholdNotExceeded(t *testing.T) {
	msgs := &mockMessageSource{msgs: nil}
	ks := &mockKnowledgeStore{}
	sw := &mockSoulWriter{}
	n := &mockNotifier{}
	llm := &mockLLMClient{response: "[]"}

	e, _, gaugeFile := newTestEngine(t, msgs, ks, sw, n, llm)

	// Write gauge data below all thresholds.
	gt := NewGaugeTracker(gaugeFile)
	if err := gt.Load(); err != nil {
		t.Fatal(err)
	}
	gt.Set("contradictions", 1.0)
	if err := gt.Save(); err != nil {
		t.Fatal(err)
	}

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestParseProposals_ValidJSON(t *testing.T) {
	raw := `Some preamble. [{"title":"T1","content":"C1","item_type":"insight","confidence":0.95,"visibility":"team"}] trailing text`
	props := parseProposals(raw)
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
	if props[0].Title != "T1" {
		t.Errorf("title: got %q, want %q", props[0].Title, "T1")
	}
	if props[0].Confidence != 0.95 {
		t.Errorf("confidence: got %v, want 0.95", props[0].Confidence)
	}
}

func TestParseProposals_InvalidJSON(t *testing.T) {
	props := parseProposals("not json at all")
	if props != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", props)
	}
}

func TestParseProposals_EmptyArray(t *testing.T) {
	props := parseProposals("[]")
	if len(props) != 0 {
		t.Fatalf("expected empty slice, got %v", props)
	}
}
