package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

)

// --- Anthropic Provider Tests ---

func TestAnthropicChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key test-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Errorf("expected anthropic-version %s", anthropicAPIVersion)
		}

		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if string(reqBody.System) != `"You are helpful"` {
			t.Errorf("system = %s, want %q", reqBody.System, "You are helpful")
		}

		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "Hello!"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "claude-sonnet-4-5",
		System:   "You are helpful",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello!")
	}
	if resp.Model != "claude-sonnet-4-5" {
		t.Errorf("Model = %q, want %q", resp.Model, "claude-sonnet-4-5")
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v, want {10, 5}", resp.Usage)
	}
}

func TestAnthropicChatWithCaching(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// beta header must be present
		if r.Header.Get("anthropic-beta") != anthropicBetaCaching {
			t.Errorf("anthropic-beta = %q, want %q", r.Header.Get("anthropic-beta"), anthropicBetaCaching)
		}

		// system must be a content block array with cache_control
		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		var blocks []anthropicSystemBlock
		if err := json.Unmarshal(reqBody.System, &blocks); err != nil {
			t.Fatalf("system should be content block array: %v", err)
		}
		if len(blocks) != 1 {
			t.Fatalf("expected 1 system block, got %d", len(blocks))
		}
		if blocks[0].Text != "You are helpful" {
			t.Errorf("system text = %q, want %q", blocks[0].Text, "You are helpful")
		}
		if blocks[0].CacheControl == nil || blocks[0].CacheControl.Type != "ephemeral" {
			t.Errorf("expected cache_control ephemeral, got %+v", blocks[0].CacheControl)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "Cached!"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage": map[string]int{
				"input_tokens":               5,
				"output_tokens":              3,
				"cache_creation_input_tokens": 100,
				"cache_read_input_tokens":    0,
			},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Model:             "claude-sonnet-4-5",
		System:            "You are helpful",
		Messages:          []Message{{Role: "user", Content: "hi"}},
		CacheSystemPrompt: true,
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Cached!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Cached!")
	}
	if resp.Usage.CacheWriteTokens != 100 {
		t.Errorf("CacheWriteTokens = %d, want 100", resp.Usage.CacheWriteTokens)
	}
	if resp.Usage.CacheReadTokens != 0 {
		t.Errorf("CacheReadTokens = %d, want 0", resp.Usage.CacheReadTokens)
	}
}

func TestAnthropicChatNoCachingByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// beta header must NOT be present when caching disabled
		if r.Header.Get("anthropic-beta") == anthropicBetaCaching {
			t.Errorf("anthropic-beta should not be set without CacheSystemPrompt")
		}
		// system must be plain JSON string, not array
		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if string(reqBody.System)[0] == '[' {
			t.Errorf("system should be plain string, not array")
		}

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
		System:   "You are helpful",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
}

func TestAnthropicChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"type": "invalid_request_error", "message": "bad request"},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("error = %q, should contain 'bad request'", err.Error())
	}
}

func TestAnthropicIsAvailable(t *testing.T) {
	p1 := NewAnthropicProvider("key", "")
	if !p1.IsAvailable() {
		t.Error("should be available with key")
	}

	p2 := NewAnthropicProvider("", "")
	if p2.IsAvailable() {
		t.Error("should not be available without key")
	}
}

func TestAnthropicModels(t *testing.T) {
	p := NewAnthropicProvider("key", "")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if !strings.HasPrefix(m.ID, "claude-") {
			t.Errorf("unexpected model %q", m.ID)
		}
	}
}

func TestAnthropicName(t *testing.T) {
	p := NewAnthropicProvider("key", "")
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q, want %q", p.Name(), "anthropic")
	}
}

// --- OpenAI Provider Tests ---

func TestOpenAIChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("expected Bearer token in Authorization header")
		}

		var reqBody openaiRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		// System prompt should be prepended as first message
		if len(reqBody.Messages) < 2 {
			t.Fatalf("expected at least 2 messages (system + user), got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" || reqBody.Messages[0].Content != "Be concise" {
			t.Errorf("first message = %+v, want system/Be concise", reqBody.Messages[0])
		}

		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Hi there"}, "finish_reason": "stop"},
			},
			"model": "gpt-4o",
			"usage": map[string]int{"prompt_tokens": 15, "completion_tokens": 8},
		})
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL)
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		System:   "Be concise",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Hi there" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hi there")
	}
	if resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 8 {
		t.Errorf("Usage = %+v, want {15, 8}", resp.Usage)
	}
}

func TestOpenAIChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limit exceeded", "type": "rate_limit_error"},
		})
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, should contain 'rate limit'", err.Error())
	}
}

func TestOpenAIIsAvailable(t *testing.T) {
	p1 := NewOpenAIProvider("key", "")
	if !p1.IsAvailable() {
		t.Error("should be available with key")
	}
	p2 := NewOpenAIProvider("", "")
	if p2.IsAvailable() {
		t.Error("should not be available without key")
	}
}

func TestOpenAIModels(t *testing.T) {
	p := NewOpenAIProvider("key", "")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if !strings.HasPrefix(m.ID, "gpt-") {
			t.Errorf("unexpected model %q", m.ID)
		}
	}
}

func TestOpenAIName(t *testing.T) {
	p := NewOpenAIProvider("key", "")
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}

// --- Gemini Provider Tests ---

func TestGeminiChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "generateContent") {
			t.Errorf("expected generateContent in path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected key=test-key in query, got %s", r.URL.Query().Get("key"))
		}

		var reqBody geminiRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.SystemInstruction == nil {
			t.Error("expected systemInstruction to be set")
		}
		// Check role mapping: assistant -> model
		for _, c := range reqBody.Contents {
			if c.Role == "assistant" {
				t.Error("assistant role should be mapped to model")
			}
		}

		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"content":      map[string]any{"parts": []map[string]string{{"text": "Gemini says hi"}}},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]int{"promptTokenCount": 20, "candidatesTokenCount": 10},
		})
	}))
	defer server.Close()

	p := NewGeminiProvider("test-key", server.URL)
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "gemini-2.0-flash",
		System:   "You are helpful",
		Messages: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hey"}, {Role: "user", Content: "how are you"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Gemini says hi" {
		t.Errorf("Content = %q, want %q", resp.Content, "Gemini says hi")
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 10 {
		t.Errorf("Usage = %+v, want {20, 10}", resp.Usage)
	}
}

func TestGeminiChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": 403, "message": "forbidden", "status": "PERMISSION_DENIED"},
		})
	}))
	defer server.Close()

	p := NewGeminiProvider("test-key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error = %q, should contain 'forbidden'", err.Error())
	}
}

func TestGeminiIsAvailable(t *testing.T) {
	p1 := NewGeminiProvider("key", "")
	if !p1.IsAvailable() {
		t.Error("should be available with key")
	}
	p2 := NewGeminiProvider("", "")
	if p2.IsAvailable() {
		t.Error("should not be available without key")
	}
}

func TestGeminiModels(t *testing.T) {
	p := NewGeminiProvider("key", "")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if !strings.HasPrefix(m.ID, "gemini-") {
			t.Errorf("unexpected model %q", m.ID)
		}
	}
}

func TestGeminiName(t *testing.T) {
	p := NewGeminiProvider("key", "")
	if p.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gemini")
	}
}

// --- Ollama Provider Tests ---

func TestOllamaChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}

		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Stream {
			t.Error("expected stream=false")
		}
		// System prompt as first message
		if len(reqBody.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("first message role = %q, want system", reqBody.Messages[0].Role)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.1:70b",
			"message": map[string]string{"content": "Local response"},
			"done":    true,
			"eval_count":        30,
			"prompt_eval_count": 20,
		})
	}))
	defer server.Close()

	p := NewOllamaProvider(server.URL)
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "llama3.1:70b",
		System:   "You are helpful",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Local response" {
		t.Errorf("Content = %q, want %q", resp.Content, "Local response")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 30 {
		t.Errorf("Usage = %+v, want {20, 30}", resp.Usage)
	}
}

func TestOllamaChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "model not found"})
	}))
	defer server.Close()

	p := NewOllamaProvider(server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "nonexistent",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error = %q, should contain 'model not found'", err.Error())
	}
}

func TestOllamaIsAvailable(t *testing.T) {
	p1 := NewOllamaProvider("http://localhost:11434")
	if !p1.IsAvailable() {
		t.Error("should be available with baseURL")
	}
	p2 := NewOllamaProvider("")
	// Default baseURL is set, so still available
	if !p2.IsAvailable() {
		t.Error("should be available with default baseURL")
	}
}

func TestOllamaModels(t *testing.T) {
	p := NewOllamaProvider("")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if !strings.Contains(m.ID, ":") {
			t.Errorf("unexpected model %q (expected colon for local models)", m.ID)
		}
	}
}

func TestOllamaName(t *testing.T) {
	p := NewOllamaProvider("")
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

// --- Factory Tests ---

func TestNewGatewayFromConfig(t *testing.T) {
	// APIKey is pre-resolved by the caller (mcp_init.go); factory just consumes it.
	cfg := GatewayConfig{
		Default: "anthropic",
		Providers: map[string]GatewayProviderConfig{
			"anthropic": {Enabled: true, APIKey: "ak-test"},
			"openai":    {Enabled: true, APIKey: "sk-test"},
			"gemini":    {Enabled: false, APIKey: ""},
			"ollama":    {Enabled: true, BaseURL: "http://localhost:11434"},
		},
	}

	gw := NewGatewayFromConfig(cfg)

	// 3 providers registered (gemini disabled)
	if gw.ProviderCount() != 3 {
		t.Errorf("ProviderCount() = %d, want 3", gw.ProviderCount())
	}

	providers := gw.ListProviders()
	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name] = true
	}
	if !names["anthropic"] {
		t.Error("anthropic provider missing")
	}
	if !names["openai"] {
		t.Error("openai provider missing")
	}
	if !names["ollama"] {
		t.Error("ollama provider missing")
	}
	if names["gemini"] {
		t.Error("gemini should be disabled")
	}
}

func TestNewGatewayFromConfigEmpty(t *testing.T) {
	cfg := GatewayConfig{
		Default: "anthropic",
	}

	gw := NewGatewayFromConfig(cfg)
	if gw.ProviderCount() != 0 {
		t.Errorf("ProviderCount() = %d, want 0 (no providers configured)", gw.ProviderCount())
	}
}

func TestNewGatewayFromConfigAvailability(t *testing.T) {
	// APIKey is pre-resolved by the caller; empty = no key = provider registered but unavailable.
	cfg := GatewayConfig{
		Default: "anthropic",
		Providers: map[string]GatewayProviderConfig{
			"anthropic": {Enabled: true, APIKey: ""},
			"openai":    {Enabled: true, APIKey: "sk-present"},
		},
	}

	gw := NewGatewayFromConfig(cfg)

	// Both registered but anthropic unavailable
	if gw.ProviderCount() != 2 {
		t.Errorf("ProviderCount() = %d, want 2", gw.ProviderCount())
	}

	providers := gw.ListProviders()
	for _, p := range providers {
		switch p.Name {
		case "anthropic":
			if p.Available {
				t.Error("anthropic should not be available (no key)")
			}
		case "openai":
			if !p.Available {
				t.Error("openai should be available")
			}
		}
	}
}

func TestNewGatewayFromConfigDefaultModel(t *testing.T) {
	cfg := GatewayConfig{
		Default: "openai",
		Providers: map[string]GatewayProviderConfig{
			"openai": {Enabled: true, APIKey: "sk-test", DefaultModel: "gpt-4o-mini"},
		},
	}

	gw := NewGatewayFromConfig(cfg)

	// Resolve("scout", "") should fall through to Routes["default"] and pick gpt-4o-mini
	ref := gw.Resolve("scout", "")
	if ref.Provider != "openai" {
		t.Errorf("Resolve provider = %q, want %q", ref.Provider, "openai")
	}
	if ref.Model != "gpt-4o-mini" {
		t.Errorf("Resolve model = %q, want %q", ref.Model, "gpt-4o-mini")
	}
}

// --- Integration: Provider + Gateway ---

func TestGatewayWithRealProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]string{{"type": "text", "text": "integrated!"}},
			"model":       "claude-sonnet-4-5",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 3},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL)
	gw := NewGateway(RoutingTable{Default: "anthropic", Aliases: Aliases})
	gw.Register(p)

	resp, err := gw.Chat(context.Background(), "", &ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "integrated!" {
		t.Errorf("Content = %q, want %q", resp.Content, "integrated!")
	}

	// Cost should be recorded
	report := gw.CostReport()
	if report.TotalReqs != 1 {
		t.Errorf("TotalReqs = %d, want 1", report.TotalReqs)
	}
}

func TestOpenAIChatNoSystem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openaiRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		// No system message should be prepended
		if len(reqBody.Messages) != 1 {
			t.Errorf("expected 1 message (no system), got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "user" {
			t.Errorf("first message role = %q, want user", reqBody.Messages[0].Role)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}, "finish_reason": "stop"},
			},
			"model": "gpt-4o",
			"usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2},
		})
	}))
	defer server.Close()

	p := NewOpenAIProvider("key", server.URL)
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
}
