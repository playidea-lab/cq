package llmclient

import (
	"context"
	"encoding/json"
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
