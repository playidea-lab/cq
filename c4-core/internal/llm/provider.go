// Package llm provides an LLM Gateway framework for multi-provider orchestration.
//
// It defines the Provider interface that all LLM backends must implement,
// a routing table for task-type-based model selection, and cost tracking.
// Actual provider implementations (Anthropic, OpenAI, etc.) are separate.
package llm

import "context"

// Provider is the interface that all LLM backends must implement.
type Provider interface {
	Name() string
	Models() []ModelInfo
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	IsAvailable() bool
}

// ChatRequest holds parameters for an LLM chat call.
type ChatRequest struct {
	Model             string         `json:"model,omitempty"`
	Messages          []Message      `json:"messages"`
	MaxTokens         int            `json:"max_tokens,omitempty"`
	Temperature       float64        `json:"temperature,omitempty"`
	System            string         `json:"system,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CacheSystemPrompt bool           `json:"cache_system_prompt,omitempty"`
}

// ChatResponse holds the result of an LLM chat call.
type ChatResponse struct {
	Content      string         `json:"content"`
	Model        string         `json:"model"`
	FinishReason string         `json:"finish_reason"`
	Usage        TokenUsage     `json:"usage"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TokenUsage tracks token consumption for a single request.
type TokenUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// ModelInfo describes a model's capabilities and pricing.
type ModelInfo struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	InputPer1M     float64 `json:"input_per_1m"`
	OutputPer1M    float64 `json:"output_per_1m"`
	SupportsTools  bool    `json:"supports_tools"`
	SupportsVision bool    `json:"supports_vision"`
}

// ProviderStatus summarizes a provider's availability and models.
type ProviderStatus struct {
	Name      string      `json:"name"`
	Available bool        `json:"available"`
	Models    []ModelInfo `json:"models"`
}
