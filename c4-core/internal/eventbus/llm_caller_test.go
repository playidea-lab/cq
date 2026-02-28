package eventbus

import (
	"context"
	"testing"
)

// mockLLMCaller is a simple LLMCaller mock for tests.
type mockLLMCaller struct {
	response string
	err      error
	called   bool
	lastSys  string
	lastUser string
	lastModel string
}

func (m *mockLLMCaller) Call(_ context.Context, systemPrompt, userMessage, model string) (string, error) {
	m.called = true
	m.lastSys = systemPrompt
	m.lastUser = userMessage
	m.lastModel = model
	return m.response, m.err
}

// TestLLMCallerInterface verifies that mockLLMCaller satisfies the LLMCaller interface.
// This is a compile-time check — if LLMCaller interface changes, this test will fail to compile.
func TestLLMCallerInterface(t *testing.T) {
	var _ LLMCaller = (*mockLLMCaller)(nil)
}
