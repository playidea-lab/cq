//go:build research

package orchestrator

import (
	"context"
	"testing"
)

func TestOnJobDone_Approved(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	// Override hub to capture new job ID.
	submittedJobID := ""
	o.HubCli = &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, _ LoopHubJobRequest) (string, error) {
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
	if submittedJobID == "" {
		t.Error("expected SubmitJob to be called")
	}
	// Copy-on-write: retrieve the updated session from the map (stored under newHypID).
	var got *LoopSession
	o.Sessions.Range(func(_, v any) bool {
		got = v.(*LoopSession)
		return false
	})
	if got == nil {
		t.Fatal("no session found in map after approved advance")
	}
	if got.Round != 1 {
		t.Errorf("Round = %d, want 1", got.Round)
	}
	if got.JobID != "job-approved-001" {
		t.Errorf("JobID = %q, want job-approved-001", got.JobID)
	}
	if got.NullResultCount != 0 {
		t.Errorf("NullResultCount = %d, want 0", got.NullResultCount)
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
	o.Caller = &mockDebateLLM{responses: llmResponses}

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
	// Copy-on-write: retrieve updated session from map.
	got1 := o.GetLoop(hypID)
	if got1 == nil {
		t.Fatal("session not found after round 1")
	}
	if got1.NullResultCount != 1 {
		t.Errorf("NullResultCount after round 1 = %d, want 1", got1.NullResultCount)
	}
	if got1.ExploreFlag {
		t.Error("ExploreFlag should not be set after 1 null_result")
	}

	// Second null_result → ExploreFlag=true; pass updated session pointer.
	o.Caller = &mockDebateLLM{responses: llmResponses}
	if err := o.onJobDone(context.Background(), got1, jobStatus); err != nil {
		t.Fatalf("onJobDone round 2: %v", err)
	}
	got2 := o.GetLoop(hypID)
	if got2 == nil {
		t.Fatal("session not found after round 2")
	}
	if got2.NullResultCount != 2 {
		t.Errorf("NullResultCount after round 2 = %d, want 2", got2.NullResultCount)
	}
	if !got2.ExploreFlag {
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
	// Copy-on-write: retrieve updated session from map.
	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("session not found after escalate")
	}
	// Escalate submits reasoning job and sets waiting_reasoning.
	if got.Status != "waiting_reasoning" {
		t.Errorf("Status = %q, want waiting_reasoning", got.Status)
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
	// Copy-on-write: approved-advance stores under newHypID; iterate to find it.
	var got *LoopSession
	o.Sessions.Range(func(_, v any) bool {
		got = v.(*LoopSession)
		return false
	})
	if got == nil {
		t.Fatal("no session found in map after budget gate")
	}
	if got.Round != 5 {
		t.Errorf("Round = %d, want 5", got.Round)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want completed", got.Status)
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
	// Copy-on-write: retrieve updated session from map.
	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("session not found after extract-draft failure")
	}
	// draft extraction failure → null_result count incremented.
	if got.NullResultCount != 1 {
		t.Errorf("NullResultCount = %d, want 1 (extractDraft failure → null_result)", got.NullResultCount)
	}
	// Round should not advance on draft failure.
	if got.Round != 0 {
		t.Errorf("Round = %d, want 0 (no advance on draft failure)", got.Round)
	}
}
