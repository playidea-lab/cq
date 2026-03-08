package hypothesis_test

import (
	"context"
	"errors"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/knowledge/hypothesis"
	"github.com/changmin/c4-core/internal/llm"
)

func newGatewayWithMock(mock *llm.MockProvider) *llm.Gateway {
	gw := llm.NewGateway(llm.RoutingTable{Default: "mock"})
	gw.Register(mock)
	return gw
}

func TestAnalyze_HappyPath(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	mock.Response = &llm.ChatResponse{
		Content:      "INSIGHT: Experiments show consistent improvement.\nCQ_YAML:\nrun: python train.py\n",
		Model:        "mock-model",
		FinishReason: "stop",
	}
	gw := newGatewayWithMock(mock)

	docs := []knowledge.Document{
		{ID: "exp-001", Title: "Exp 1", Body: "Accuracy improved to 95%."},
	}

	result, err := hypothesis.Analyze(context.Background(), gw, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Insight == "" {
		t.Error("expected non-empty Insight")
	}
	if result.CQYAMLDraft == "" {
		t.Error("expected non-empty CQYAMLDraft")
	}
	if mock.CallCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", mock.CallCount)
	}
}

func TestAnalyze_LLMError(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	mock.Err = errors.New("provider unavailable")
	gw := newGatewayWithMock(mock)

	docs := []knowledge.Document{
		{ID: "exp-001", Title: "Exp 1", Body: "Some result."},
	}

	result, err := hypothesis.Analyze(context.Background(), gw, docs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
	// Must not panic — test passing proves that
}

func TestAnalyze_EmptyDocs(t *testing.T) {
	mock := llm.NewMockProvider("mock")
	gw := newGatewayWithMock(mock)

	result, err := hypothesis.Analyze(context.Background(), gw, []knowledge.Document{})
	if err == nil {
		t.Fatal("expected error for empty docs")
	}
	if err.Error() != "no_experiments" {
		t.Errorf("expected error 'no_experiments', got %q", err.Error())
	}
	if result != nil {
		t.Error("expected nil result for empty docs")
	}
	if mock.CallCount != 0 {
		t.Errorf("expected 0 LLM calls for empty docs, got %d", mock.CallCount)
	}
}
