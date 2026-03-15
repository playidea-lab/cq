//go:build research

package orchestrator

import (
	"context"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

// specMockCaller is a test double for DebateCaller.
type specMockCaller struct {
	response string
	err      error
}

func (m *specMockCaller) Call(_ context.Context, _, _ string) (string, error) {
	return m.response, m.err
}

// specMockStore is a test double for DebateStore.
type specMockStore struct {
	created []knowledge.DocumentType
}

func (m *specMockStore) Get(_ string) (*knowledge.Document, error) { return nil, nil }
func (m *specMockStore) Create(dt knowledge.DocumentType, _ map[string]any, _ string) (string, error) {
	m.created = append(m.created, dt)
	return "esp-test", nil
}

func TestGenerateSpec_ValidResponse(t *testing.T) {
	raw := `{"type":"ml_training","metric":"val_loss","budget":{"max_hours":2,"max_cost_usd":5},"success_criteria":"val_loss < 0.05","hypothesis_id":"hyp-001"}`
	caller := &specMockCaller{response: raw}

	spec, err := GenerateSpec(context.Background(), caller, "test hypothesis", 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if spec.Type != "ml_training" {
		t.Errorf("expected type ml_training, got %s", spec.Type)
	}
	if spec.Metric != "val_loss" {
		t.Errorf("expected metric val_loss, got %s", spec.Metric)
	}
	if spec.SuccessCriteria != "val_loss < 0.05" {
		t.Errorf("unexpected success_criteria: %s", spec.SuccessCriteria)
	}
	if spec.HypothesisID != "hyp-001" {
		t.Errorf("unexpected hypothesis_id: %s", spec.HypothesisID)
	}
}

func TestGenerateSpec_ParseError(t *testing.T) {
	caller := &specMockCaller{response: "not valid json"}

	_, err := GenerateSpec(context.Background(), caller, "test hypothesis", 1)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestReviewSpec_Approved(t *testing.T) {
	caller := &specMockCaller{response: "approved"}
	spec := ExperimentSpec{Type: "ml_training", Metric: "val_loss"}

	approved, reason := ReviewSpec(context.Background(), caller, spec)
	if !approved {
		t.Errorf("expected approved=true, got false (reason: %s)", reason)
	}
}

func TestReviewSpec_Rejected(t *testing.T) {
	caller := &specMockCaller{response: "rejected: budget too high"}
	spec := ExperimentSpec{Type: "ml_training", Metric: "val_loss"}

	approved, reason := ReviewSpec(context.Background(), caller, spec)
	if approved {
		t.Error("expected approved=false")
	}
	if reason != "budget too high" {
		t.Errorf("expected reason 'budget too high', got %q", reason)
	}
}
