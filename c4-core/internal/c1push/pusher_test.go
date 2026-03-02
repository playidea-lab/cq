package c1push

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_ReturnsNilOnEmptyConfig(t *testing.T) {
	if New("", "") != nil {
		t.Error("expected nil for empty URL/key")
	}
	if New("https://example.supabase.co", "") != nil {
		t.Error("expected nil for empty key")
	}
	if New("", "key") != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestNew_ReturnsNonNil(t *testing.T) {
	p := New("https://example.supabase.co", "anon-key")
	if p == nil {
		t.Error("expected non-nil Pusher")
	}
}

func TestEnsureChannel_CreatesAndReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/rest/v1/c1_channels" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "channel-uuid-123"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := New(srv.URL, "test-key")
	id, err := p.EnsureChannel(context.Background(), "default", "", "claude_code:session-abc", PlatformClaudeCode)
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if id != "channel-uuid-123" {
		t.Errorf("id = %q, want %q", id, "channel-uuid-123")
	}
}

func TestAppendMessages_SendsBatch(t *testing.T) {
	var gotBody []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/rest/v1/c1_messages" {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				http.Error(w, "bad body", 400)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := New(srv.URL, "test-key")
	msgs := []PushMessage{
		{SenderName: "user", SenderType: "user", Content: "hello"},
		{SenderName: "assistant", SenderType: "assistant", Content: "hi"},
	}
	if err := p.AppendMessages(context.Background(), "channel-uuid-123", msgs); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}
	if len(gotBody) != 2 {
		t.Errorf("expected 2 rows, got %d", len(gotBody))
	}
}

func TestAppendMessages_EmptyNoOp(t *testing.T) {
	p := New("https://example.supabase.co", "key")
	if err := p.AppendMessages(context.Background(), "channel-id", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
