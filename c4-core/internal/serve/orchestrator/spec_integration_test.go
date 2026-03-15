//go:build research

package orchestrator

import (
	"context"
	"testing"
)

// validSpecJSON is a minimal valid ExperimentSpec JSON for LLM mock responses.
const validSpecJSON = `{"type":"ml_training","metric":"val_loss","budget":{"max_hours":2,"max_cost_usd":5},"success_criteria":"val_loss < 0.05","hypothesis_id":"hyp-001"}`

// TestOnJobDone_SpecPipeline_NullResult verifies that when spec review rejects the spec,
// the session NullResultCount is incremented.
func TestOnJobDone_SpecPipeline_NullResult(t *testing.T) {
	// LLM responses: 3 for debate (Optimizer, Skeptic, Synthesis) + 2 for spec (generate, review).
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
		validSpecJSON,         // generateSpec response
		"rejected: too broad", // reviewSpec response → nullResult=true
	}
	o, kStore := newTestOrchestrator(t, llmResponses)
	// Enable specPipeline using the orchestrator's existing caller and store.
	o.SpecPipeline = &LoopSpecPipeline{Caller: o.Caller, KStore: o.Store}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-spec-null-001",
		Round:         4, // one below limit
		MaxIterations: 5,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-spec-null-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("session not found after spec null_result")
	}
	if got.NullResultCount != 1 {
		t.Errorf("NullResultCount = %d, want 1", got.NullResultCount)
	}
}

// TestOnJobDone_SpecPipeline_NullResult_AtLimit verifies that when spec fails exactly
// at MaxIterations boundary, Status is set to "completed" (budget gate applied on spec path).
func TestOnJobDone_SpecPipeline_NullResult_AtLimit(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
		validSpecJSON,
		"rejected: insufficient budget",
	}
	o, kStore := newTestOrchestrator(t, llmResponses)
	o.SpecPipeline = &LoopSpecPipeline{Caller: o.Caller, KStore: o.Store}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-spec-limit-001",
		Round:         5, // at limit
		MaxIterations: 5,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-spec-limit-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	got := o.GetLoop(hypID)
	if got == nil {
		t.Fatal("session not found after spec null_result at limit")
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want completed (budget gate on spec null_result path)", got.Status)
	}
}

// TestOnJobDone_SpecPipeline_Approved verifies that when spec is approved, ExperimentSpecID
// is set on the submitted Hub job.
func TestOnJobDone_SpecPipeline_Approved(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: collect more samples",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: approved",
		`{"verdict":"approved","next_hypothesis_draft":"collect more samples"}`,
		validSpecJSON, // generateSpec → valid ExperimentSpec
		"approved",    // reviewSpec → approved
	}
	o, kStore := newTestOrchestrator(t, llmResponses)
	o.SpecPipeline = &LoopSpecPipeline{Caller: o.Caller, KStore: o.Store}

	// Capture submitted job to verify ExperimentSpecID.
	var capturedReq LoopHubJobRequest
	o.HubCli = &mockLoopHubClient{
		submitJobFunc: func(_ context.Context, req LoopHubJobRequest) (string, error) {
			capturedReq = req
			return "job-spec-approved-001", nil
		},
	}

	hypID := mustCreateHyp(t, kStore)
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-old-spec-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-old-spec-001", Status: "completed"}

	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}

	// Spec approved → job submitted with non-empty ExperimentSpecID.
	if capturedReq.ExperimentSpecID == "" {
		t.Error("ExperimentSpecID should be set when spec is approved")
	}

	// Session should advance: find under the new hypothesis ID.
	var got *LoopSession
	o.Sessions.Range(func(_, v any) bool {
		got = v.(*LoopSession)
		return false
	})
	if got == nil {
		t.Fatal("no session found after approved spec advance")
	}
	if got.Round != 1 {
		t.Errorf("Round = %d, want 1", got.Round)
	}
	if got.JobID != "job-spec-approved-001" {
		t.Errorf("JobID = %q, want job-spec-approved-001", got.JobID)
	}
}
