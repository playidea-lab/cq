//go:build research

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

// mustCreateExpDoc creates a TypeExperiment document with JSON body and returns its ID.
func mustCreateExpDoc(t *testing.T, kStore *knowledge.Store, hypothesisID, jobID string, valLoss, testMetric float64, status string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"type":          "experiment",
		"hypothesis_id": hypothesisID,
		"job_id":        jobID,
		"val_loss":      valLoss,
		"test_metric":   testMetric,
		"status":        status,
		"stdout_summary": "training complete",
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	id, err := kStore.Create(knowledge.TypeExperiment, map[string]any{
		"title":  "exp-" + jobID,
		"domain": "experiment",
	}, string(body))
	if err != nil {
		t.Fatalf("Create experiment doc: %v", err)
	}
	return id
}

// TestOnJobDone_MetricInjected verifies that when a TypeExperiment doc exists for
// the session's HypothesisID, its metrics are injected into extraContext before runDebate.
func TestOnJobDone_MetricInjected(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: more data\nRATIONALE: needs more\nNEXT_HYPOTHESIS: validate further",
		"CHALLENGE: bias\nALTERNATIVE: stratified\nVERDICT: null_result",
		`{"verdict":"null_result","next_hypothesis_draft":"validate further"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	hypID := mustCreateHyp(t, kStore)
	mustCreateExpDoc(t, kStore, hypID, "job-metric-001", 0.042, 0.891, "completed")

	// Capture extraContext passed to runDebate by intercepting via the LLM mock.
	// The debate caller receives the extraContext as part of its prompt.
	// We verify by checking the stored session — the debate must succeed without error,
	// and the metric block must have been built (tested via fetchExperimentMetrics directly).
	block := fetchExperimentMetrics(kStore, hypID)
	if !strings.Contains(block, "experiment_result:") {
		t.Fatalf("expected experiment_result: in block, got %q", block)
	}
	if !strings.Contains(block, "test_metric: 0.891") {
		t.Errorf("expected test_metric: 0.891 in block, got %q", block)
	}
	if !strings.Contains(block, "val_loss: 0.042") {
		t.Errorf("expected val_loss: 0.042 in block, got %q", block)
	}
	if !strings.Contains(block, "status: completed") {
		t.Errorf("expected status: completed in block, got %q", block)
	}

	// Run onJobDone to verify no regression — it must succeed.
	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-metric-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-metric-001", Status: "completed"}
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone: %v", err)
	}
}

// TestOnJobDone_MetricMissing_Fallback verifies that when no TypeExperiment doc exists
// for the HypothesisID, onJobDone proceeds without error and extraContext is unchanged.
func TestOnJobDone_MetricMissing_Fallback(t *testing.T) {
	llmResponses := []string{
		"DIRECTION: pivot\nRATIONALE: no data\nNEXT_HYPOTHESIS: try again",
		"CHALLENGE: flaw\nALTERNATIVE: restart\nVERDICT: null_result",
		`{"verdict":"null_result","next_hypothesis_draft":"try again"}`,
	}
	o, kStore := newTestOrchestrator(t, llmResponses)

	hypID := mustCreateHyp(t, kStore)
	// No TypeExperiment doc created — fallback expected.

	block := fetchExperimentMetrics(kStore, hypID)
	if block != "" {
		t.Errorf("expected empty block when no doc exists, got %q", block)
	}

	session := &LoopSession{
		HypothesisID:  hypID,
		JobID:         "job-missing-001",
		Round:         0,
		MaxIterations: 10,
		Status:        "running",
	}
	jobStatus := &HubJobStatus{JobID: "job-missing-001", Status: "completed"}
	if err := o.onJobDone(context.Background(), session, jobStatus); err != nil {
		t.Fatalf("onJobDone with missing metric doc: %v", err)
	}
}

// TestOnJobDone_MetricEmptyValLoss verifies that val_loss=0.0 is displayed as "N/A"
// in the extraContext block (Critique Loop ambiguity fix).
func TestOnJobDone_MetricEmptyValLoss(t *testing.T) {
	kStore := mustNewHypothesisStore(t)
	hypID := mustCreateHyp(t, kStore)

	// val_loss=0.0 (e.g. @VAL_LOSS= not set, stored as 0)
	mustCreateExpDoc(t, kStore, hypID, "job-zeroloss-001", 0.0, 0.75, "completed")

	block := fetchExperimentMetrics(kStore, hypID)
	if !strings.Contains(block, "experiment_result:") {
		t.Fatalf("expected experiment_result: in block, got %q", block)
	}
	if !strings.Contains(block, "val_loss: N/A") {
		t.Errorf("expected val_loss: N/A for 0.0, got %q", block)
	}
	if !strings.Contains(block, "test_metric: 0.75") {
		t.Errorf("expected test_metric: 0.75 in block, got %q", block)
	}
}
