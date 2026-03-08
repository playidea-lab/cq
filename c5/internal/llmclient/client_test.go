package llmclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChat_Disabled(t *testing.T) {
	c := New("http://example.com", "", "gemini-3.0-flash", 0)
	_, err := c.Chat(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
	if !strings.Contains(err.Error(), "apiKey") {
		t.Errorf("error should mention apiKey, got: %v", err)
	}
}

func TestChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Hello, world!"}},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model", 100)
	got, err := c.Chat(context.Background(), "system prompt", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello, world!" {
		t.Errorf("got %q, want %q", got, "Hello, world!")
	}
}

func TestChat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "invalid api key",
				"code":    "invalid_api_key",
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "bad-key", "test-model", 100)
	_, err := c.Chat(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error should contain API message, got: %v", err)
	}
}

func TestAnthropicProvider_Chat_NoAPIKey(t *testing.T) {
	c := NewAnthropic("", "claude-3-haiku-20240307", 100)
	_, err := c.Chat(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
	if !strings.Contains(err.Error(), "apiKey") {
		t.Errorf("error should mention apiKey, got: %v", err)
	}
}

func TestAnthropicProvider_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": "Hello!"},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	// Swap the real Anthropic URL for the test server by building provider directly.
	p := &anthropicProvider{
		apiKey: "test-key",
		model:  "claude-3-haiku-20240307",
		maxTok: 100,
		client: srv.Client(),
	}
	// Override the URL used by the provider via a thin wrapper that redirects.
	// Since anthropicProvider hard-codes the URL we use an httptest transport trick.
	p.client.Transport = rewriteTransport{target: srv.URL, inner: http.DefaultTransport}

	c := &Client{p: p}
	got, err := c.Chat(context.Background(), "system", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello!" {
		t.Errorf("got %q, want %q", got, "Hello!")
	}
}

func TestAnthropicProvider_Chat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "invalid_request_error",
				"message": "bad request",
			},
		})
	}))
	defer srv.Close()

	p := &anthropicProvider{
		apiKey: "test-key",
		model:  "claude-3-haiku-20240307",
		maxTok: 100,
		client: &http.Client{Transport: rewriteTransport{target: srv.URL, inner: http.DefaultTransport}},
	}
	c := &Client{p: p}
	_, err := c.Chat(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("error should contain API message, got: %v", err)
	}
}

func TestAnthropicProvider_SystemPrompt(t *testing.T) {
	var capturedBody []byte
	var capturedAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("x-api-key")
		capturedBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": "ok"},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	p := &anthropicProvider{
		apiKey: "my-key",
		model:  "claude-3-haiku-20240307",
		maxTok: 100,
		client: &http.Client{Transport: rewriteTransport{target: srv.URL, inner: http.DefaultTransport}},
	}
	c := &Client{p: p}
	_, err := c.Chat(context.Background(), "be helpful", "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify x-api-key header.
	if capturedAPIKey != "my-key" {
		t.Errorf("x-api-key header = %q, want %q", capturedAPIKey, "my-key")
	}

	// Verify system is a top-level field, not inside messages.
	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["system"] != "be helpful" {
		t.Errorf("system field = %v, want %q", body["system"], "be helpful")
	}
	msgs, _ := body["messages"].([]any)
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["role"] == "system" {
			t.Errorf("system message found in messages array — should be top-level field only")
		}
	}
}

// rewriteTransport redirects all requests to a fixed target host (test server).
type rewriteTransport struct {
	target string
	inner  http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse target and rewrite host/scheme.
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return rt.inner.RoundTrip(req2)
}
