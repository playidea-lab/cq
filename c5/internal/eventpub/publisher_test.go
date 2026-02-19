package eventpub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublish_Success(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/publish" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New(srv.URL, "test-token")

	if !p.IsEnabled() {
		t.Fatal("expected publisher to be enabled")
	}

	err := p.Publish("hub.job.started", "c5", map[string]any{"job_id": "j1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header 'Bearer test-token', got %q", gotAuth)
	}

	if gotBody["type"] != "hub.job.started" {
		t.Errorf("unexpected event type: %v", gotBody["type"])
	}
}

func TestPublish_Disabled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New("", "")

	if p.IsEnabled() {
		t.Fatal("expected publisher to be disabled when url is empty")
	}

	err := p.Publish("hub.job.started", "c5", map[string]any{"job_id": "j1"})
	if err != nil {
		t.Fatalf("unexpected error for disabled publisher: %v", err)
	}

	if callCount != 0 {
		t.Errorf("expected no HTTP calls for disabled publisher, got %d", callCount)
	}
}

func TestPublish_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := New(srv.URL, "")

	err := p.Publish("hub.job.started", "c5", map[string]any{"job_id": "j2"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestPublish_NoAuthHeader_WhenTokenEmpty(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New(srv.URL, "")

	err := p.Publish("hub.job.completed", "c5", map[string]any{"job_id": "j3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuth != "" {
		t.Errorf("expected no Authorization header when token is empty, got %q", gotAuth)
	}
}
