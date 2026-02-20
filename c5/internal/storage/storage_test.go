package storage

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestSupabaseBackend_PresignedURL_PUT_SignedUpload verifies that PUT presigned
// URLs call the Supabase createSignedUploadUrl API and return a full absolute URL.
func TestSupabaseBackend_PresignedURL_PUT_SignedUpload(t *testing.T) {
	const token = "test-upload-token-abc123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the correct endpoint is called.
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		wantPath := "/storage/v1/object/upload/sign/c5-artifacts/jobs/j1/artifact.tar.gz"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify auth headers are present.
		if r.Header.Get("apikey") != "test-key" {
			t.Errorf("missing or wrong apikey header: %q", r.Header.Get("apikey"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header: %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		// Supabase returns a relative URL in the "url" field.
		fmt.Fprintf(w, `{"url":"/object/upload/sign/c5-artifacts/jobs/j1/artifact.tar.gz?token=%s"}`, token)
	}))
	defer srv.Close()

	b := NewSupabase(srv.URL, "test-key")
	url, expiresAt, err := b.PresignedURL("jobs/j1/artifact.tar.gz", "PUT", 600)
	if err != nil {
		t.Fatalf("PresignedURL(PUT) error = %v", err)
	}

	// URL should be absolute, starting with the server URL.
	if !strings.HasPrefix(url, srv.URL+"/storage/v1/object/upload/sign/") {
		t.Errorf("url = %q, want prefix %q", url, srv.URL+"/storage/v1/object/upload/sign/")
	}
	// URL should contain the token.
	if !strings.Contains(url, "token="+token) {
		t.Errorf("url = %q, want to contain token=%s", url, token)
	}
	if expiresAt.IsZero() {
		t.Error("expiresAt should not be zero")
	}
}

// TestSupabaseBackend_PresignedURL_PUT_Error verifies that a non-200 response
// from the signed upload URL API returns an error.
func TestSupabaseBackend_PresignedURL_PUT_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"access denied"}`)
	}))
	defer srv.Close()

	b := NewSupabase(srv.URL, "bad-key")
	_, _, err := b.PresignedURL("jobs/j1/artifact.tar.gz", "PUT", 600)
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
	if !strings.Contains(err.Error(), "upload sign failed") {
		t.Errorf("error = %q, want to contain 'upload sign failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want to contain '403'", err.Error())
	}
}

// TestSupabaseBackend_PresignedURL_PUT_PathTraversal verifies that path
// traversal attacks are rejected for PUT signed upload URLs.
func TestSupabaseBackend_PresignedURL_PUT_PathTraversal(t *testing.T) {
	b := NewSupabase("https://example.supabase.co", "test-key")
	_, _, err := b.PresignedURL("../../etc/passwd", "PUT", 600)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "invalid storage path") {
		t.Errorf("error = %q, want 'invalid storage path'", err.Error())
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
