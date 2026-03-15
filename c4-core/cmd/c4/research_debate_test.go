//go:build research

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve/orchestrator"
)

// mockDebateLLM is a test double for debateCaller / orchestrator.DebateCaller.
type mockDebateLLM struct {
	responses []string
	idx       int
}

func (m *mockDebateLLM) call(_ context.Context, _, _ string) (string, error) {
	r := m.responses[m.idx%len(m.responses)]
	m.idx++
	return r, nil
}

// Call implements orchestrator.DebateCaller.
func (m *mockDebateLLM) Call(ctx context.Context, system, user string) (string, error) {
	return m.call(ctx, system, user)
}

// testDebateStore wraps *knowledge.Store to implement debateStore and orchestrator.DebateStore.
type testDebateStore struct{ s *knowledge.Store }

func (t *testDebateStore) get(id string) (*knowledge.Document, error) { return t.s.Get(id) }
func (t *testDebateStore) create(dt knowledge.DocumentType, meta map[string]any, body string) (string, error) {
	return t.s.Create(dt, meta, body)
}

// Get implements orchestrator.DebateStore.
func (t *testDebateStore) Get(id string) (*knowledge.Document, error) { return t.s.Get(id) }

// Create implements orchestrator.DebateStore.
func (t *testDebateStore) Create(dt knowledge.DocumentType, meta map[string]any, body string) (string, error) {
	return t.s.Create(dt, meta, body)
}

func TestResearchDebate_HappyPath(t *testing.T) {
	store := mustNewHypothesisStore(t)
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "test hyp",
		"status": "approved",
	}, "test body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mock := &mockDebateLLM{responses: []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: sample bias\nALTERNATIVE: stratified sampling\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples","experiment_spec_draft":"run 100 trials"}`,
	}}

	result, err := orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, hypID, "manual", "", "")
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	m := result.(map[string]any)
	if m["debate_doc_id"] == "" {
		t.Error("expected debate_doc_id")
	}
	if m["verdict"] == "" {
		t.Error("expected verdict")
	}
}

func TestResearchDebate_TypeDebateDoc(t *testing.T) {
	store := mustNewHypothesisStore(t)
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "test",
		"status": "approved",
	}, "body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mock := &mockDebateLLM{responses: []string{
		"opt out",
		"VERDICT: null_result",
		`{"verdict":"null_result","next_hypothesis_draft":"try again"}`,
	}}

	result, err := orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, hypID, "dod_null", "", "")
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	m := result.(map[string]any)

	docID := m["debate_doc_id"].(string)
	if !strings.HasPrefix(docID, "deb-") {
		t.Errorf("debate doc should have deb- prefix, got %s", docID)
	}

	doc, err := store.Get(docID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if doc == nil {
		t.Fatal("debate doc not found")
	}
	if doc.Type != knowledge.TypeDebate {
		t.Errorf("doc type = %s, want debate", doc.Type)
	}
}

func TestResearchDebate_InvalidHypothesis(t *testing.T) {
	store := mustNewHypothesisStore(t)
	mock := &mockDebateLLM{responses: []string{"any"}}

	_, err := orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, "hyp-nonexistent", "manual", "", "")
	if err == nil {
		t.Error("expected error for unknown hypothesis")
	}
}

func TestResearchDebate_NullResult(t *testing.T) {
	store := mustNewHypothesisStore(t)
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "null result hyp",
		"status": "approved",
	}, "null result body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mock := &mockDebateLLM{responses: []string{
		"DIRECTION: pivot\nRATIONALE: low signal\nNEXT_HYPOTHESIS: try different approach",
		"CHALLENGE: fundamental flaw\nALTERNATIVE: start over\nVERDICT: null_result",
		`{"verdict":"null_result","next_hypothesis_draft":"try different approach","experiment_spec_draft":"redesign experiment"}`,
	}}

	result, err := orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, hypID, "dod_null", "", "")
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	m := result.(map[string]any)
	if m["verdict"] != "null_result" {
		t.Errorf("verdict = %q, want null_result", m["verdict"])
	}
	if m["next_hypothesis_draft"] == "" {
		t.Error("expected next_hypothesis_draft to be non-empty for null_result")
	}
}

// capturingDebateLLM records the user messages passed to Call().
type capturingDebateLLM struct {
	responses []string
	idx       int
	userMsgs  []string
}

func (c *capturingDebateLLM) Call(_ context.Context, _, user string) (string, error) {
	c.userMsgs = append(c.userMsgs, user)
	r := c.responses[c.idx%len(c.responses)]
	c.idx++
	return r, nil
}

func TestRunDebate_EmptyLineageContext_BackwardCompat(t *testing.T) {
	store := mustNewHypothesisStore(t)
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "compat hyp",
		"status": "approved",
	}, "compat body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mock := &capturingDebateLLM{responses: []string{
		"DIRECTION: d\nRATIONALE: r\nNEXT_HYPOTHESIS: n",
		"VERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"n"}`,
	}}

	_, err = orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, hypID, "manual", "ctx1", "")
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	optimizerMsg := mock.userMsgs[0]
	if strings.Contains(optimizerMsg, "\n\n\n") {
		t.Error("empty lineageContext should not inject extra blank lines")
	}
	if !strings.Contains(optimizerMsg, "Context: ctx1") {
		t.Error("userMsg should contain the extraContext")
	}
}

func TestRunDebate_LineageContextInjected(t *testing.T) {
	store := mustNewHypothesisStore(t)
	hypID, err := store.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "lineage hyp",
		"status": "approved",
	}, "lineage body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mock := &capturingDebateLLM{responses: []string{
		"DIRECTION: d\nRATIONALE: r\nNEXT_HYPOTHESIS: n",
		"VERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"n"}`,
	}}

	lineage := "## Lineage\nPrev failure: X"
	_, err = orchestrator.RunDebate(context.Background(), mock, &testDebateStore{store}, hypID, "manual", "", lineage)
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	for i, msg := range mock.userMsgs[:2] {
		if !strings.Contains(msg, lineage) {
			t.Errorf("userMsgs[%d] does not contain lineageContext", i)
		}
	}
}
