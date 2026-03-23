package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCQProxyProvider_Name(t *testing.T) {
	p := NewCQProxyProvider("http://localhost", func() string { return "tok" })
	if got := p.Name(); got != "cq-proxy" {
		t.Errorf("Name() = %q, want %q", got, "cq-proxy")
	}
}

func TestCQProxyProvider_IsAvailable(t *testing.T) {
	if NewCQProxyProvider("http://localhost", nil).IsAvailable() {
		t.Error("IsAvailable() should be false when tokenFunc is nil")
	}
	if !NewCQProxyProvider("http://localhost", func() string { return "tok" }).IsAvailable() {
		t.Error("IsAvailable() should be true when tokenFunc is non-nil")
	}
}

func TestCQProxyProvider_Models_HaikuOnly(t *testing.T) {
	p := NewCQProxyProvider("http://localhost", func() string { return "tok" })
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("Models() returned empty slice")
	}
	for _, m := range models {
		if !strings.Contains(m.ID, "haiku") {
			t.Errorf("Models() returned non-haiku model: %s", m.ID)
		}
	}
}

func TestCQProxyProvider_Chat(t *testing.T) {
	wantContent := "Hello from proxy"
	wantModel := "claude-haiku-4-5-20251001"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Return a valid anthropicResponse
		resp := anthropicResponse{
			Model:      wantModel,
			StopReason: "end_turn",
		}
		resp.Content = []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: wantContent}}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewCQProxyProvider(srv.URL, func() string { return "test-token" })

	chatResp, err := p.Chat(context.Background(), &ChatRequest{
		Model:     wantModel,
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if chatResp.Content != wantContent {
		t.Errorf("Content = %q, want %q", chatResp.Content, wantContent)
	}
	if chatResp.Model != wantModel {
		t.Errorf("Model = %q, want %q", chatResp.Model, wantModel)
	}
	if chatResp.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", chatResp.Usage.InputTokens)
	}
}

func TestCQProxyProvider_Chat_ErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		errResp := anthropicErrorResponse{}
		errResp.Error.Type = "rate_limit_error"
		errResp.Error.Message = "rate limit exceeded"
		_ = json.NewEncoder(w).Encode(errResp)
	}))
	defer srv.Close()

	p := NewCQProxyProvider(srv.URL, func() string { return "tok" })
	_, err := p.Chat(context.Background(), &ChatRequest{
		Model:    "claude-haiku-4-5-20251001",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("Chat() expected error on 429, got nil")
	}
}
