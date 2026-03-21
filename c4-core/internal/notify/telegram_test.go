package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendTelegram_Signature(t *testing.T) {
	// Compile-time check that the exported function has the correct signature.
	var _ func(context.Context, string, string, string) error = SendTelegram
}

func TestSendTelegram_PayloadAndURL(t *testing.T) {
	var received map[string]string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const token = "mytoken"
	const chatID = "123456"
	const message = "hello *world*"

	if err := sendTelegram(context.Background(), srv.URL, token, chatID, message); err != nil {
		t.Fatalf("sendTelegram: %v", err)
	}

	wantPath := "/botmytoken/sendMessage"
	if gotPath != wantPath {
		t.Errorf("URL path: got %q, want %q", gotPath, wantPath)
	}
	if received["chat_id"] != chatID {
		t.Errorf("chat_id: got %q, want %q", received["chat_id"], chatID)
	}
	if received["text"] != message {
		t.Errorf("text: got %q, want %q", received["text"], message)
	}
	if received["parse_mode"] != "Markdown" {
		t.Errorf("parse_mode: got %q, want %q", received["parse_mode"], "Markdown")
	}
}

func TestSendTelegram_HTTP4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := sendTelegram(context.Background(), srv.URL, "badtoken", "chat", "msg")
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

func TestSendTelegram_HTTP5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := sendTelegram(context.Background(), srv.URL, "token", "chat", "msg")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestSendTelegram_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sendTelegram(ctx, srv.URL, "token", "chat", "msg")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
