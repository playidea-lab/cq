//go:build research

package main

import (
	"context"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

// =========================================================================
// mock implementations
// =========================================================================

type mockLoopHubClient struct {
	submitJobFunc func(ctx context.Context, req loopHubJobRequest) (string, error)
}

func (m *mockLoopHubClient) SubmitJob(ctx context.Context, req loopHubJobRequest) (string, error) {
	return m.submitJobFunc(ctx, req)
}

type mockLoopLineageBuilder struct {
	buildContextFunc func(ctx context.Context, hypothesisID string, limit int) (string, error)
}

func (m *mockLoopLineageBuilder) BuildContext(ctx context.Context, hypothesisID string, limit int) (string, error) {
	return m.buildContextFunc(ctx, hypothesisID, limit)
}

// newTestOrchestrator creates a LoopOrchestrator with mock internals.
func newTestOrchestrator(t *testing.T, llmResponses []string) (*LoopOrchestrator, *knowledge.Store) {
	t.Helper()
	kStore := mustNewHypothesisStore(t)
	mock := &mockDebateLLM{responses: llmResponses}
	store := &testDebateStore{s: kStore}
	hubCli := &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, _ loopHubJobRequest) (string, error) {
			return "job-new-001", nil
		},
	}
	lineage := &mockLoopLineageBuilder{
		buildContextFunc: func(_ context.Context, _ string, _ int) (string, error) {
			return "", nil
		},
	}
	o := &LoopOrchestrator{
		cfg:     LoopOrchestratorConfig{ExploreThreshold: 2},
		caller:  mock,
		store:   store,
		hubCli:  hubCli,
		lineage: lineage,
		kStore:  kStore,
	}
	return o, kStore
}

// mustCreateHyp creates a TypeHypothesis doc and returns its ID.
func mustCreateHyp(t *testing.T, kStore *knowledge.Store) string {
	t.Helper()
	id, err := kStore.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "test hypothesis",
		"status": "approved",
	}, "test body")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}
	return id
}

// =========================================================================
// tests
// =========================================================================

func TestOnJobDone_Approved(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	// Override hub to capture new job ID.
	submittedJobID := ""
	o.hubCli = &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, _ loopHubJobRequest) (string, error) {
			submittedJobID = "job-approved-001"
			return submittedJobID, nil
		},
	}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-old-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-old-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	if session.Round != 1 {
		t.Errorf("Round = %d, want 1", session.Round)
	}
	if session.JobID != "job-approved-001" {
		t.Errorf("JobID = %q, want job-approved-001", session.JobID)
	}
	if session.NullResultCount != 0 {
		t.Errorf("NullResultCount = %d, want 0", session.NullResultCount)
	}
	if submittedJobID == "" {
		t.Error("expected SubmitJob to be called")
	}
}

func TestOnJobDone_NullResult_ExploreFlag(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: pivot\nRATIONALE: low signal\nNEXT_HYPOTHESIS: try different approach",
		"CHALLENGE: flaw\nALTERNATIVE: restart\nVERDICT: null_result",
		`{"verdict":"null_result","next_hypothesis_draft":"try different approach"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)
	// Duplicate responses for two calls.
	o.caller = &mockDebateLLM{responses: llmResponses}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-001", Status: "completed"}

	// First null_result.
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone round 1: %v", err)
	}
	if session.NullResultCount != 1 {
		t.Errorf("NullResultCount after round 1 = %d, want 1", session.NullResultCount)
	}
	if session.ExploreFlag {
		t.Error("ExploreFlag should not be set after 1 null_result")
	}

	// Second null_result → ExploreFlag=true.
	o.caller = &mockDebateLLM{responses: llmResponses}
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone round 2: %v", err)
	}
	if session.NullResultCount != 2 {
		t.Errorf("NullResultCount after round 2 = %d, want 2", session.NullResultCount)
	}
	if !session.ExploreFlag {
		t.Error("ExploreFlag should be true after 2 consecutive null_results")
	}
}

func TestOnJobDone_Escalate(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: stop\nRATIONALE: no hope\nNEXT_HYPOTHESIS: abandon",
		"CHALLENGE: fatal\nALTERNATIVE: quit\nVERDICT: escalate",
		`{"verdict":"escalate","next_hypothesis_draft":""}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-esc-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-esc-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}
	if session.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", session.Status)
	}
}

func TestOnJobDone_BudgetGate(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-budget-001",
		Round:         4, // one below limit
		MaxIterations: 5,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-budget-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}
	// Round becomes 5 (== MaxIterations) → Status="completed".
	if session.Round != 5 {
		t.Errorf("Round = %d, want 5", session.Round)
	}
	if session.Status != "completed" {
		t.Errorf("Status = %q, want completed", session.Status)
	}
}

func TestOnJobDone_ExtractDraftFailure(t *testing.T) {
	// Optimizer output has no NEXT_HYPOTHESIS → draft is empty.
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more", // no NEXT_HYPOTHESIS
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":""}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-draft-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-draft-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}
	// draft extraction failure → null_result count incremented.
	if session.NullResultCount != 1 {
		t.Errorf("NullResultCount = %d, want 1 (extractDraft failure → null_result)", session.NullResultCount)
	}
	// Round should not advance.
	if session.Round != 0 {
		t.Errorf("Round = %d, want 0 (no advance on draft failure)", session.Round)
	}
}
