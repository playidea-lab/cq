//go:build llm_gateway

package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

func setupLLMGateway(t *testing.T) (*mcp.Registry, *llm.Gateway, *llm.MockProvider) {
	t.Helper()
	gw := llm.NewGateway(llm.RoutingTable{
		Default: "mock",
		Routes: map[string]llm.ModelRef{
			"review": {Provider: "mock", Model: "review-model"},
		},
	})
	mock := llm.NewMockProvider("mock")
	mock.Response = &llm.ChatResponse{
		Content:      "test response",
		Model:        "mock-model",
		FinishReason: "stop",
		Usage:        llm.TokenUsage{InputTokens: 100, OutputTokens: 50},
	}
	gw.Register(mock)

	reg := mcp.NewRegistry()
	RegisterLLMHandlers(reg, gw)
	return reg, gw, mock
}

func TestLLMCallBasic(t *testing.T) {
	reg, _, mock := setupLLMGateway(t)

	params := `{"messages": [{"role": "user", "content": "hello"}]}`
	result, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	if m["content"] != "test response" {
		t.Errorf("content = %v, want %q", m["content"], "test response")
	}
	if mock.CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount)
	}
}

func TestLLMCallWithModel(t *testing.T) {
	reg, _, mock := setupLLMGateway(t)

	params := `{"messages": [{"role": "user", "content": "hi"}], "model": "custom-model"}`
	result, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	m := result.(map[string]any)
	if m["model"] != "custom-model" {
		t.Errorf("model = %v, want %q", m["model"], "custom-model")
	}
	if mock.LastRequest.Model != "custom-model" {
		t.Errorf("LastRequest.Model = %q, want %q", mock.LastRequest.Model, "custom-model")
	}
}

func TestLLMCallWithTaskType(t *testing.T) {
	reg, _, mock := setupLLMGateway(t)

	params := `{"messages": [{"role": "user", "content": "review this"}], "task_type": "review"}`
	_, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	if mock.LastRequest.Model != "review-model" {
		t.Errorf("LastRequest.Model = %q, want %q", mock.LastRequest.Model, "review-model")
	}
}

func TestLLMCallEmptyMessages(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	params := `{"messages": []}`
	_, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestLLMCallMissingMessages(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	params := `{}`
	_, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err == nil {
		t.Fatal("expected error for missing messages")
	}
}

func TestLLMProviders(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	result, err := reg.Call("c4_llm_providers", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("c4_llm_providers error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	providers, ok := m["providers"].([]map[string]any)
	if !ok {
		t.Fatalf("providers type = %T, want []map", m["providers"])
	}
	if len(providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(providers))
	}
	if providers[0]["name"] != "mock" {
		t.Errorf("provider name = %v, want %q", providers[0]["name"], "mock")
	}
	if providers[0]["available"] != true {
		t.Error("provider should be available")
	}
}

func TestLLMCosts(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	// Make a call first to generate cost data
	params := `{"messages": [{"role": "user", "content": "hi"}]}`
	_, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	result, err := reg.Call("c4_llm_costs", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("c4_llm_costs error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}

	totalReqs, ok := m["total_requests"].(int)
	if !ok {
		t.Fatalf("total_requests type = %T", m["total_requests"])
	}
	if totalReqs != 1 {
		t.Errorf("total_requests = %d, want 1", totalReqs)
	}
}

func TestLLMCostsEmpty(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	result, err := reg.Call("c4_llm_costs", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("c4_llm_costs error: %v", err)
	}

	m := result.(map[string]any)
	if m["total_requests"] != 0 {
		t.Errorf("total_requests = %v, want 0", m["total_requests"])
	}
}

func TestLLMToolsRegistered(t *testing.T) {
	reg, _, _ := setupLLMGateway(t)

	tools := reg.ListTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"c4_llm_call", "c4_llm_providers", "c4_llm_costs"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestLLMCosts_CacheMetrics(t *testing.T) {
	gw := llm.NewGateway(llm.RoutingTable{Default: "mock"})
	mock := llm.NewMockProvider("mock")
	mock.Response = &llm.ChatResponse{
		Content:      "cached",
		Model:        "mock-model",
		FinishReason: "stop",
		Usage: llm.TokenUsage{
			InputTokens:      1000,
			OutputTokens:     100,
			CacheReadTokens:  500,
			CacheWriteTokens: 500,
		},
	}
	gw.Register(mock)

	reg := mcp.NewRegistry()
	RegisterLLMHandlers(reg, gw)

	// Make one call to populate cost data.
	_, err := reg.Call("c4_llm_call", json.RawMessage(`{"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	result, err := reg.Call("c4_llm_costs", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("c4_llm_costs error: %v", err)
	}

	m := result.(map[string]any)

	globalHitRate, ok := m["global_cache_hit_rate"].(float64)
	if !ok {
		t.Fatalf("global_cache_hit_rate type = %T, want float64", m["global_cache_hit_rate"])
	}
	if globalHitRate != 0.5 {
		t.Errorf("global_cache_hit_rate = %v, want 0.5", globalHitRate)
	}

	globalSavingsRate, ok := m["global_cache_savings_rate"].(float64)
	if !ok {
		t.Fatalf("global_cache_savings_rate type = %T, want float64", m["global_cache_savings_rate"])
	}
	const wantSavings = 0.25
	if diff := globalSavingsRate - wantSavings; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("global_cache_savings_rate = %v, want ~%v", globalSavingsRate, wantSavings)
	}

	byProvider, ok := m["by_provider"].(map[string]any)
	if !ok {
		t.Fatalf("by_provider type = %T, want map", m["by_provider"])
	}
	mockProv, ok := byProvider["mock"].(map[string]any)
	if !ok {
		t.Fatalf("by_provider[mock] type = %T, want map", byProvider["mock"])
	}
	provHitRate, ok := mockProv["cache_hit_rate"].(float64)
	if !ok {
		t.Fatalf("cache_hit_rate type = %T, want float64", mockProv["cache_hit_rate"])
	}
	if provHitRate != 0.5 {
		t.Errorf("by_provider[mock][cache_hit_rate] = %v, want 0.5", provHitRate)
	}
}

func TestLLMCallWithSystemPrompt(t *testing.T) {
	reg, _, mock := setupLLMGateway(t)

	params := `{"messages": [{"role": "user", "content": "hi"}], "system": "You are helpful."}`
	_, err := reg.Call("c4_llm_call", json.RawMessage(params))
	if err != nil {
		t.Fatalf("c4_llm_call error: %v", err)
	}

	if mock.LastRequest.System != "You are helpful." {
		t.Errorf("System = %q, want %q", mock.LastRequest.System, "You are helpful.")
	}
}
