package eventbus

import "context"

// LLMCaller is a minimal interface for calling an LLM with a system prompt and user message.
// Defined here (not in internal/llm) to prevent cross-import cycles between
// the eventbus package and the llm package.
type LLMCaller interface {
	Call(ctx context.Context, systemPrompt, userMessage, model string) (string, error)
}
