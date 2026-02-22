package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolsCacheControl_AttachedToLastTool(t *testing.T) {
	var capturedBody anthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []Tool{
			{Name: "tool_one", Description: "first tool"},
			{Name: "tool_two", Description: "second tool"},
		},
		CacheTools: true,
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if len(capturedBody.Tools) != 2 {
		t.Fatalf("expected 2 tools in request, got %d", len(capturedBody.Tools))
	}

	// Only the last tool should have cache_control.
	if capturedBody.Tools[0].CacheControl != nil {
		t.Errorf("tool[0] should not have cache_control, got %+v", capturedBody.Tools[0].CacheControl)
	}
	if capturedBody.Tools[1].CacheControl == nil {
		t.Fatal("tool[1] (last) should have cache_control")
	}
	if capturedBody.Tools[1].CacheControl.Type != "ephemeral" {
		t.Errorf("tool[1].cache_control.type = %q, want %q", capturedBody.Tools[1].CacheControl.Type, "ephemeral")
	}
}

func TestToolsCacheControl_NotAttached_WhenEmpty(t *testing.T) {
	var capturedBody anthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:      "claude-sonnet-4-5",
		Messages:   []Message{{Role: "user", Content: "hi"}},
		Tools:      nil,
		CacheTools: true, // CacheTools=true but no tools — should produce no tools in request
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if len(capturedBody.Tools) != 0 {
		t.Errorf("expected 0 tools in request when Tools=nil, got %d", len(capturedBody.Tools))
	}
}

func TestToolsCacheControl_BetaHeaderSet(t *testing.T) {
	var capturedBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("anthropic-beta")
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:             "claude-sonnet-4-5",
		Messages:          []Message{{Role: "user", Content: "hi"}},
		CacheSystemPrompt: false,
		Tools:             []Tool{{Name: "my_tool", Description: "a tool"}},
		CacheTools:        true,
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if capturedBeta != anthropicBetaCaching {
		t.Errorf("anthropic-beta = %q, want %q", capturedBeta, anthropicBetaCaching)
	}
}
