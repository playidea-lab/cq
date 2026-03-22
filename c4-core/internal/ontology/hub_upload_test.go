package ontology

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHubUploader_SuccessfulUpload(t *testing.T) {
	var received []collectivePatternRow

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/collective_patterns" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var row collectivePatternRow
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received = append(received, row)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	root := t.TempDir()
	u := NewHubUploader(srv.URL, "test-key", func() string { return "test-token" }, root)

	patterns := []AnonPattern{
		{Domain: "go-backend", Path: "api", Value: "API Gateway", Frequency: 7, Confidence: "high", Tags: []string{"http"}},
		{Domain: "go-backend", Path: "db", Value: "Database", Frequency: 5, Confidence: "high", Tags: nil},
	}

	n, err := u.Upload(patterns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 uploaded, got %d", n)
	}
	if len(received) != 2 {
		t.Errorf("server received %d requests, want 2", len(received))
	}
}

func TestHubUploader_AuthHeaders(t *testing.T) {
	var gotAPIKey, gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("apikey")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	root := t.TempDir()
	u := NewHubUploader(srv.URL, "my-anon-key", func() string { return "my-bearer-token" }, root)

	patterns := []AnonPattern{
		{Domain: "d", Path: "p", Value: "v", Frequency: 5, Confidence: "high"},
	}

	if _, err := u.Upload(patterns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAPIKey != "my-anon-key" {
		t.Errorf("expected apikey=my-anon-key, got %q", gotAPIKey)
	}
	if gotAuth != "Bearer my-bearer-token" {
		t.Errorf("expected Authorization=Bearer my-bearer-token, got %q", gotAuth)
	}
}

func TestHubUploader_FailureQueuesPatterns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	root := t.TempDir()
	u := NewHubUploader(srv.URL, "key", func() string { return "token" }, root)

	patterns := []AnonPattern{
		{Domain: "go-backend", Path: "svc", Value: "Service", Frequency: 6, Confidence: "high"},
	}

	n, err := u.Upload(patterns)
	if err == nil {
		t.Error("expected error when server fails")
	}
	if n != 0 {
		t.Errorf("expected 0 uploaded, got %d", n)
	}

	// Check that the pattern was queued.
	queuePath := filepath.Join(root, pendingUploadFile)
	data, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("pending queue not created: %v", err)
	}
	var queued []AnonPattern
	if err := json.Unmarshal(data, &queued); err != nil {
		t.Fatalf("parse pending queue: %v", err)
	}
	if len(queued) != 1 {
		t.Errorf("expected 1 queued pattern, got %d", len(queued))
	}
	if queued[0].Path != "svc" {
		t.Errorf("queued wrong pattern: %q", queued[0].Path)
	}
}

func TestHubUploader_FailureAppendsToExistingQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	root := t.TempDir()

	// Pre-populate the queue with one pattern.
	existing := []AnonPattern{
		{Domain: "d", Path: "old", Value: "Old", Frequency: 5, Confidence: "high"},
	}
	queuePath := filepath.Join(root, pendingUploadFile)
	if err := os.MkdirAll(filepath.Dir(queuePath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(queuePath, data, 0644); err != nil {
		t.Fatalf("write existing queue: %v", err)
	}

	u := NewHubUploader(srv.URL, "key", func() string { return "token" }, root)

	patterns := []AnonPattern{
		{Domain: "d", Path: "new", Value: "New", Frequency: 7, Confidence: "high"},
	}
	if _, err := u.Upload(patterns); err == nil {
		t.Error("expected error from server failure")
	}

	// Queue should now have 2 entries.
	data, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	var queued []AnonPattern
	if err := json.Unmarshal(data, &queued); err != nil {
		t.Fatalf("parse queue: %v", err)
	}
	if len(queued) != 2 {
		t.Errorf("expected 2 queued patterns, got %d", len(queued))
	}
}

func TestHubUploader_EmptyPatternsIsNoop(t *testing.T) {
	root := t.TempDir()
	u := NewHubUploader("http://unused", "key", func() string { return "token" }, root)

	n, err := u.Upload(nil)
	if err != nil {
		t.Errorf("unexpected error for empty upload: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestHubUploader_NilTagsBecomesEmptyArray(t *testing.T) {
	var gotTags []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var row collectivePatternRow
		if err := json.NewDecoder(r.Body).Decode(&row); err == nil {
			gotTags = row.Tags
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	root := t.TempDir()
	u := NewHubUploader(srv.URL, "key", func() string { return "token" }, root)

	patterns := []AnonPattern{
		{Domain: "d", Path: "p", Value: "v", Frequency: 5, Confidence: "high", Tags: nil},
	}
	if _, err := u.Upload(patterns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTags == nil {
		t.Error("expected non-nil tags array (empty slice), got nil")
	}
}
