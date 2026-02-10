package llm

import "context"

// MockProvider is a test provider that returns fixed responses without API calls.
type MockProvider struct {
	ProviderName string
	ProviderModels []ModelInfo
	Available    bool
	Response     *ChatResponse
	Err          error
	CallCount    int
	LastRequest  *ChatRequest
}

// NewMockProvider creates a mock provider with a default echo response.
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		ProviderName: name,
		Available:    true,
		ProviderModels: []ModelInfo{
			{ID: "mock-model", Name: "Mock Model", ContextWindow: 4096, MaxOutput: 1024},
		},
		Response: &ChatResponse{
			Content:      "mock response",
			Model:        "mock-model",
			FinishReason: "stop",
			Usage:        TokenUsage{InputTokens: 10, OutputTokens: 20},
		},
	}
}

func (m *MockProvider) Name() string        { return m.ProviderName }
func (m *MockProvider) Models() []ModelInfo  { return m.ProviderModels }
func (m *MockProvider) IsAvailable() bool    { return m.Available }

func (m *MockProvider) Chat(_ context.Context, req *ChatRequest) (*ChatResponse, error) {
	m.CallCount++
	m.LastRequest = req
	if m.Err != nil {
		return nil, m.Err
	}
	resp := *m.Response
	if req.Model != "" {
		resp.Model = req.Model
	}
	return &resp, nil
}
