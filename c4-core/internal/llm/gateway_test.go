package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResolveAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"opus", "claude-opus-4-6"},
		{"sonnet", "claude-sonnet-4-6"},
		{"haiku", "claude-haiku-4-5-20251001"},
		{"gpt4o", "gpt-4o"},
		{"unknown-model", "unknown-model"},
	}
	for _, tc := range tests {
		if got := ResolveAlias(tc.input); got != tc.want {
			t.Errorf("ResolveAlias(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestLookupModel(t *testing.T) {
	// Direct ID
	info, ok := LookupModel("claude-opus-4-6")
	if !ok {
		t.Fatal("LookupModel(claude-opus-4-6) not found")
	}
	if info.InputPer1M != 5.0 {
		t.Errorf("InputPer1M = %f, want 5.0", info.InputPer1M)
	}

	// Via alias
	info, ok = LookupModel("sonnet")
	if !ok {
		t.Fatal("LookupModel(sonnet) not found")
	}
	if info.ID != "claude-sonnet-4-6" {
		t.Errorf("ID = %q, want %q", info.ID, "claude-sonnet-4-6")
	}

	// Unknown
	_, ok = LookupModel("nonexistent")
	if ok {
		t.Error("LookupModel(nonexistent) should return false")
	}
}

func TestNewGateway(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	if gw.ProviderCount() != 0 {
		t.Errorf("new gateway should have 0 providers, got %d", gw.ProviderCount())
	}
}

func TestGatewayRegister(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	gw.Register(mock)

	if gw.ProviderCount() != 1 {
		t.Errorf("provider count = %d, want 1", gw.ProviderCount())
	}

	providers := gw.ListProviders()
	if len(providers) != 1 {
		t.Fatalf("ListProviders() returned %d, want 1", len(providers))
	}
	if providers[0].Name != "mock" {
		t.Errorf("provider name = %q, want %q", providers[0].Name, "mock")
	}
	if !providers[0].Available {
		t.Error("provider should be available")
	}
}

func TestResolveDirectFormat(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "anthropic"})

	ref := gw.Resolve("", "openai/gpt-4o")
	if ref.Provider != "openai" || ref.Model != "gpt-4o" {
		t.Errorf("Resolve direct = %+v, want openai/gpt-4o", ref)
	}
}

func TestResolveGatewayAlias(t *testing.T) {
	gw := NewGateway(RoutingTable{
		Default: "anthropic",
		Aliases: map[string]string{"fast": "claude-haiku-3-5"},
	})

	ref := gw.Resolve("", "fast")
	if ref.Model != "claude-haiku-3-5" {
		t.Errorf("Resolve alias model = %q, want %q", ref.Model, "claude-haiku-3-5")
	}
	if ref.Provider != "anthropic" {
		t.Errorf("Resolve alias provider = %q, want %q", ref.Provider, "anthropic")
	}
}

func TestResolveTaskTypeRoute(t *testing.T) {
	gw := NewGateway(RoutingTable{
		Default: "anthropic",
		Routes: map[string]ModelRef{
			"review":         {Provider: "anthropic", Model: "claude-opus-4-6"},
			"implementation": {Provider: "anthropic", Model: "claude-sonnet-4-5"},
			"default":        {Provider: "anthropic", Model: "claude-sonnet-4-5"},
		},
	})

	// Task type route
	ref := gw.Resolve("review", "")
	if ref.Model != "claude-opus-4-6" {
		t.Errorf("Resolve review = %q, want claude-opus-4-6", ref.Model)
	}

	// Task type with model override
	ref = gw.Resolve("review", "claude-haiku-3-5")
	if ref.Model != "claude-haiku-3-5" {
		t.Errorf("Resolve review+hint = %q, want claude-haiku-3-5", ref.Model)
	}

	// Unknown task type falls to default route
	ref = gw.Resolve("unknown", "")
	if ref.Model != "claude-sonnet-4-5" {
		t.Errorf("Resolve unknown = %q, want claude-sonnet-4-5", ref.Model)
	}
}

func TestResolveDefaultProvider(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "anthropic"})

	ref := gw.Resolve("", "claude-opus-4-6")
	if ref.Provider != "anthropic" || ref.Model != "claude-opus-4-6" {
		t.Errorf("Resolve default = %+v, want anthropic/claude-opus-4-6", ref)
	}
}

func TestChatSuccess(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Response = &ChatResponse{
		Content:      "hello world",
		Model:        "mock-model",
		FinishReason: "stop",
		Usage:        TokenUsage{InputTokens: 100, OutputTokens: 50},
	}
	gw.Register(mock)

	resp, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "hello world" {
		t.Errorf("Content = %q, want %q", resp.Content, "hello world")
	}
	if mock.CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount)
	}
}

func TestChatProviderNotFound(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "nonexistent"})

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("Chat() should fail with unknown provider")
	}
}

func TestChatProviderUnavailable(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Available = false
	gw.Register(mock)

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("Chat() should fail when provider unavailable")
	}
}

func TestChatProviderError(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Err = errors.New("api error")
	gw.Register(mock)

	_, err := gw.Chat(context.Background(), "", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("Chat() should propagate provider error")
	}
}

func TestChatWithRouting(t *testing.T) {
	gw := NewGateway(RoutingTable{
		Default: "mock-a",
		Routes: map[string]ModelRef{
			"review": {Provider: "mock-b", Model: "review-model"},
		},
	})

	mockA := NewMockProvider("mock-a")
	mockB := NewMockProvider("mock-b")
	mockB.Response = &ChatResponse{
		Content: "review done", Model: "review-model",
		FinishReason: "stop", Usage: TokenUsage{InputTokens: 50, OutputTokens: 25},
	}
	gw.Register(mockA)
	gw.Register(mockB)

	resp, err := gw.Chat(context.Background(), "review", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "review this"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "review done" {
		t.Errorf("Content = %q, want %q", resp.Content, "review done")
	}
	if mockA.CallCount != 0 {
		t.Error("mock-a should not be called for review task type")
	}
	if mockB.CallCount != 1 {
		t.Errorf("mock-b CallCount = %d, want 1", mockB.CallCount)
	}
}

func TestCostTrackerRecord(t *testing.T) {
	ct := NewCostTracker()
	ct.Record("anthropic", "claude-sonnet-4-5", TokenUsage{InputTokens: 1000, OutputTokens: 500}, 100*time.Millisecond)

	if ct.EntryCount() != 1 {
		t.Fatalf("EntryCount() = %d, want 1", ct.EntryCount())
	}

	report := ct.Report()
	if report.TotalReqs != 1 {
		t.Errorf("TotalReqs = %d, want 1", report.TotalReqs)
	}

	// cost = 1000*3.0/1M + 500*15.0/1M = 0.003 + 0.0075 = 0.0105
	expectedCost := 0.0105
	if diff := report.TotalUSD - expectedCost; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("TotalUSD = %f, want ~%f", report.TotalUSD, expectedCost)
	}

	pc, ok := report.ByProvider["anthropic"]
	if !ok {
		t.Fatal("ByProvider[anthropic] missing")
	}
	if pc.Requests != 1 {
		t.Errorf("ByProvider[anthropic].Requests = %d, want 1", pc.Requests)
	}
}

func TestCostTrackerMultipleProviders(t *testing.T) {
	ct := NewCostTracker()
	ct.Record("anthropic", "claude-sonnet-4-5", TokenUsage{InputTokens: 1000, OutputTokens: 500}, 50*time.Millisecond)
	ct.Record("openai", "gpt-4o", TokenUsage{InputTokens: 2000, OutputTokens: 1000}, 100*time.Millisecond)

	report := ct.Report()
	if report.TotalReqs != 2 {
		t.Errorf("TotalReqs = %d, want 2", report.TotalReqs)
	}
	if len(report.ByProvider) != 2 {
		t.Errorf("ByProvider count = %d, want 2", len(report.ByProvider))
	}
	if len(report.ByModel) != 2 {
		t.Errorf("ByModel count = %d, want 2", len(report.ByModel))
	}
}

func TestCostTrackerUnknownModel(t *testing.T) {
	ct := NewCostTracker()
	// Unknown model -> zero cost (not in catalog)
	ct.Record("local", "unknown-model", TokenUsage{InputTokens: 1000, OutputTokens: 500}, 10*time.Millisecond)

	report := ct.Report()
	if report.TotalUSD != 0 {
		t.Errorf("TotalUSD for unknown model = %f, want 0", report.TotalUSD)
	}
}

func TestGatewayCostReport(t *testing.T) {
	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Response = &ChatResponse{
		Content: "ok", Model: "mock-model",
		FinishReason: "stop", Usage: TokenUsage{InputTokens: 100, OutputTokens: 50},
	}
	gw.Register(mock)

	// Make two calls
	for i := 0; i < 2; i++ {
		_, err := gw.Chat(context.Background(), "", &ChatRequest{
			Messages: []Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
	}

	report := gw.CostReport()
	if report.TotalReqs != 2 {
		t.Errorf("TotalReqs = %d, want 2", report.TotalReqs)
	}
}

func TestMockProviderModelOverride(t *testing.T) {
	mock := NewMockProvider("test")
	resp, err := mock.Chat(context.Background(), &ChatRequest{
		Model:    "custom-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Model != "custom-model" {
		t.Errorf("Model = %q, want %q", resp.Model, "custom-model")
	}
}

func TestCatalogCompleteness(t *testing.T) {
	// Verify all aliases point to valid catalog entries
	for alias, fullID := range Aliases {
		if _, ok := Catalog[fullID]; !ok {
			t.Errorf("Alias %q -> %q not found in Catalog", alias, fullID)
		}
	}
}

// mockTraceHook records OnLLMCall invocations for test assertions.
type mockTraceHook struct {
	calls []traceCall
}

type traceCall struct {
	sessionID, taskType, provider, model string
	inputTok, outputTok                  int
	latencyMs                            int64
	errMsg                               string
	success                              bool
}

func (m *mockTraceHook) OnLLMCall(sessionID, taskType, provider, model string, inputTok, outputTok int, latencyMs int64, errMsg string, success bool) {
	m.calls = append(m.calls, traceCall{
		sessionID: sessionID, taskType: taskType,
		provider: provider, model: model,
		inputTok: inputTok, outputTok: outputTok,
		latencyMs: latencyMs, errMsg: errMsg, success: success,
	})
}

// TestTraceHook verifies that SetTraceHook causes OnLLMCall to be invoked on Chat().
func TestTraceHook(t *testing.T) {
	// Reset after test to avoid pollution.
	t.Cleanup(func() { SetTraceHook(nil) })

	hook := &mockTraceHook{}
	SetTraceHook(hook)

	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Response = &ChatResponse{
		Content:      "hello",
		Model:        "mock-model",
		FinishReason: "stop",
		Usage:        TokenUsage{InputTokens: 10, OutputTokens: 5},
	}
	gw.Register(mock)

	_, err := gw.Chat(context.Background(), "test-task", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if len(hook.calls) != 1 {
		t.Fatalf("expected 1 OnLLMCall, got %d", len(hook.calls))
	}
	c := hook.calls[0]
	if c.taskType != "test-task" {
		t.Errorf("taskType = %q, want %q", c.taskType, "test-task")
	}
	if c.provider != "mock" {
		t.Errorf("provider = %q, want %q", c.provider, "mock")
	}
	if c.model != "mock-model" {
		t.Errorf("model = %q, want %q", c.model, "mock-model")
	}
	if c.inputTok != 10 {
		t.Errorf("inputTok = %d, want 10", c.inputTok)
	}
	if c.outputTok != 5 {
		t.Errorf("outputTok = %d, want 5", c.outputTok)
	}
	if !c.success {
		t.Error("success should be true")
	}
	if c.errMsg != "" {
		t.Errorf("errMsg = %q, want empty", c.errMsg)
	}
}

// TestTraceHookOnError verifies that OnLLMCall is called even when Chat() fails.
func TestTraceHookOnError(t *testing.T) {
	t.Cleanup(func() { SetTraceHook(nil) })

	hook := &mockTraceHook{}
	SetTraceHook(hook)

	gw := NewGateway(RoutingTable{Default: "mock"})
	mock := NewMockProvider("mock")
	mock.Err = errors.New("provider failure")
	gw.Register(mock)

	_, err := gw.Chat(context.Background(), "review", &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected Chat() to fail")
	}

	if len(hook.calls) != 1 {
		t.Fatalf("expected 1 OnLLMCall on error, got %d", len(hook.calls))
	}
	c := hook.calls[0]
	if c.success {
		t.Error("success should be false on error")
	}
	if c.errMsg == "" {
		t.Error("errMsg should be non-empty on error")
	}
}
