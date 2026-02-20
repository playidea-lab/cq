package storage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSupabaseBackend_EnsureBucket_AlreadyExists verifies that EnsureBucket
// returns nil when the bucket already exists (HTTP 200).
func TestSupabaseBackend_EnsureBucket_AlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/storage/v1/bucket/c5-artifacts" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := NewSupabase(srv.URL, "test-key")
	if err := b.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket() error = %v, want nil", err)
	}
}

// TestSupabaseBackend_EnsureBucket_Creates verifies that EnsureBucket creates
// the bucket when it receives a 404 from the GET check.
func TestSupabaseBackend_EnsureBucket_Creates(t *testing.T) {
	var created bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/storage/v1/bucket/c5-artifacts":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == "POST" && r.URL.Path == "/storage/v1/bucket":
			created = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	b := NewSupabase(srv.URL, "test-key")
	if err := b.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket() error = %v, want nil", err)
	}
	if !created {
		t.Error("EnsureBucket() did not POST to create the bucket")
	}
}

// TestLocalBackend_EnsureBucket verifies that EnsureBucket creates the base
// directory when it does not exist.
func TestLocalBackend_EnsureBucket(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "nested", "artifacts")

	b := &LocalBackend{baseDir: dir, baseURL: "http://localhost:8585"}
	if err := b.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket() error = %v, want nil", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}
