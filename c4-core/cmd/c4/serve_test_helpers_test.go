package main

import (
	"context"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/llm"
)

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

// stubLLMProvider is a minimal llm.Provider for testing.
type stubLLMProvider struct {
	response string
}

func (s *stubLLMProvider) Name() string                    { return "stub" }
func (s *stubLLMProvider) IsAvailable() bool               { return true }
func (s *stubLLMProvider) Models() []llm.ModelInfo         { return nil }
func (s *stubLLMProvider) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: s.response}, nil
}
