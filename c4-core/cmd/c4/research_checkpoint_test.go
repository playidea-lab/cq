//go:build research

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

// fakeCheckpointCaller is a test double implementing checkpointCaller.
// It returns distinct responses for optimizer vs skeptic by call order.
type fakeCheckpointCaller struct {
	responses []string
	callIdx   int
	err       error
}

func (f *fakeCheckpointCaller) call(_ context.Context, _, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.callIdx < len(f.responses) {
		resp := f.responses[f.callIdx]
		f.callIdx++
		return resp, nil
	}
	return "", fmt.Errorf("unexpected call index %d", f.callIdx)
}

func newCheckpointKS(t *testing.T) (*knowledge.Store, string) {
	t.Helper()
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { ks.Close() })
	return ks, dir
}

func seedSpec(t *testing.T, ks *knowledge.Store, body string) string {
	t.Helper()
	id, err := ks.Create(knowledge.TypeExperiment, map[string]any{
		"title":  "Test Spec",
		"domain": "research",
	}, body)
	if err != nil {
		t.Fatalf("Create spec: %v", err)
	}
	return id
}

func TestDebateCheckpoint_Approved(t *testing.T) {
	ks, _ := newCheckpointKS(t)
	specID := seedSpec(t, ks, "success_condition: accuracy > 0.9\nnull_condition: accuracy <= 0.7")

	caller := &fakeCheckpointCaller{
		responses: []string{
			"ASSESSMENT: positive, FEEDBACK: strong DoD",
			"ASSESSMENT: positive, FEEDBACK: no issues found",
		},
	}
	handler := researchCheckpointHandler(ks, caller)

	rawArgs, _ := json.Marshal(map[string]string{"spec_id": specID})
	result, err := handler(context.Background(), rawArgs)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(result.(string)), &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["verdict"] != "approved" {
		t.Errorf("verdict = %q, want %q", m["verdict"], "approved")
	}
	if m["optimizer_feedback"] != "ASSESSMENT: positive, FEEDBACK: strong DoD" {
		t.Errorf("unexpected optimizer_feedback: %v", m["optimizer_feedback"])
	}
}

func TestDebateCheckpoint_RevisionRequested(t *testing.T) {
	ks, _ := newCheckpointKS(t)
	specID := seedSpec(t, ks, "success_condition: model is better")

	caller := &fakeCheckpointCaller{
		responses: []string{
			"ASSESSMENT: positive, FEEDBACK: hypothesis linkage ok",
			"ASSESSMENT: negative, FEEDBACK: too vague\nISSUES: attribution ambiguity\nISSUES: unmeasurable condition",
		},
	}
	handler := researchCheckpointHandler(ks, caller)

	rawArgs, _ := json.Marshal(map[string]string{"spec_id": specID})
	result, err := handler(context.Background(), rawArgs)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(result.(string)), &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["verdict"] != "revision_requested" {
		t.Errorf("verdict = %q, want %q", m["verdict"], "revision_requested")
	}
	suggestions, _ := m["suggestions"].([]any)
	if len(suggestions) == 0 {
		t.Error("expected non-empty suggestions for revision_requested verdict")
	}
}

func TestDebateCheckpoint_UnknownSpec(t *testing.T) {
	ks, _ := newCheckpointKS(t)

	caller := &fakeCheckpointCaller{
		responses: []string{"any", "any"},
	}
	handler := researchCheckpointHandler(ks, caller)

	rawArgs, _ := json.Marshal(map[string]string{"spec_id": "nonexistent-id"})
	_, err := handler(context.Background(), rawArgs)
	if err == nil {
		t.Fatal("expected error for unknown spec_id, got nil")
	}
}
