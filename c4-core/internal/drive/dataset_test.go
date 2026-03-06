package drive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockTokenProvider satisfies the tokenProvider interface for tests.
type mockTokenProvider struct{}

func (m *mockTokenProvider) Token() string { return "test-token" }

// setupTestClient creates a Client pointing at the given test server URL.
func setupTestClient(serverURL, projectID string) *Client {
	return NewClient(serverURL, "anon-key", &mockTokenProvider{}, projectID)
}

// TestDatasetUpload_NoChange verifies that when the manifest hash matches the
// existing latest version, Upload returns Changed=false and makes no storage calls.
func TestDatasetUpload_NoChange(t *testing.T) {
	// Create a temp dir with one file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Compute the expected version_hash without a server round-trip.
	entries, err := WalkDir(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := hashFile(entries[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []ManifestEntry{{Path: "a.txt", Hash: hash, Size: entries[0].Size}}
	mJSON, _ := json.Marshal(manifest)
	vh := manifestVersionHash(mJSON)

	// Mock Supabase server: returns the same version_hash for the GET query.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && contains(r.URL.Path, "c4_datasets") {
			rows := []map[string]any{
				{"version_hash": vh, "manifest": json.RawMessage(mJSON)},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
			return
		}
		// Should not reach storage or insert endpoints.
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-1"))
	result, err := dc.Upload(context.Background(), dir, "my-dataset", "")
	if err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if result.Changed {
		t.Error("want Changed=false, got true")
	}
	if result.FilesSkipped != 1 {
		t.Errorf("want FilesSkipped=1, got %d", result.FilesSkipped)
	}
	if result.VersionHash != vh {
		t.Errorf("want VersionHash=%q, got %q", vh, result.VersionHash)
	}
}

// TestDatasetUpload_NewVersion verifies that when there is no existing version,
// Upload uploads files, inserts a row, and returns Changed=true.
func TestDatasetUpload_NewVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	headCalled := false
	uploadCalled := false
	insertCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// GET c4_datasets — no existing version.
		case r.Method == http.MethodGet && contains(r.URL.Path, "c4_datasets"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))

		// HEAD storage object — not found.
		case r.Method == http.MethodHead && contains(r.URL.Path, "storage"):
			headCalled = true
			http.Error(w, "not found", http.StatusNotFound)

		// POST storage upload.
		case r.Method == http.MethodPost && contains(r.URL.Path, "storage"):
			uploadCalled = true
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)

		// POST c4_datasets insert.
		case r.Method == http.MethodPost && contains(r.URL.Path, "c4_datasets"):
			insertCalled = true
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-2"))
	result, err := dc.Upload(context.Background(), dir, "ds2", "")
	if err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if !result.Changed {
		t.Error("want Changed=true, got false")
	}
	if result.FilesUploaded != 1 {
		t.Errorf("want FilesUploaded=1, got %d", result.FilesUploaded)
	}
	if result.VersionHash == "" {
		t.Error("want non-empty VersionHash")
	}
	if !headCalled {
		t.Error("expected HEAD request to storage")
	}
	if !uploadCalled {
		t.Error("expected POST request to storage")
	}
	if !insertCalled {
		t.Error("expected POST insert to c4_datasets")
	}
}

// TestDatasetPull_Incremental verifies that files whose local hash matches the
// manifest are skipped (FilesSkipped=1) while changed files are downloaded.
func TestDatasetPull_Incremental(t *testing.T) {
	dest := t.TempDir()

	// Pre-populate one file that already matches.
	content := []byte("preexisting content")
	h := sha256file(content)
	localFile := filepath.Join(dest, "existing.txt")
	if err := os.WriteFile(localFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// New file that must be downloaded.
	newHash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	manifest := []ManifestEntry{
		{Path: "existing.txt", Hash: h, Size: int64(len(content))},
		{Path: "new.txt", Hash: newHash, Size: 10},
	}
	mJSON, _ := json.Marshal(manifest)

	downloadCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && contains(r.URL.Path, "c4_datasets"):
			rows := []map[string]any{
				{"version_hash": "vhash001", "manifest": json.RawMessage(mJSON)},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
		case r.Method == http.MethodGet && contains(r.URL.Path, "storage"):
			downloadCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("downloaded content"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-3"))
	result, err := dc.Pull(context.Background(), "ds3", dest, "")
	if err != nil {
		t.Fatalf("Pull error: %v", err)
	}
	if result.FilesSkipped != 1 {
		t.Errorf("want FilesSkipped=1, got %d", result.FilesSkipped)
	}
	if result.FilesDownloaded != 1 {
		t.Errorf("want FilesDownloaded=1, got %d", result.FilesDownloaded)
	}
	if !downloadCalled {
		t.Error("expected download request for new.txt")
	}
}

// TestDatasetList_Empty verifies that List returns an empty slice when there
// are no dataset versions.
func TestDatasetList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && contains(r.URL.Path, "c4_datasets") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-4"))
	versions, err := dc.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("want empty slice, got %d items", len(versions))
	}
}

// --- test helpers ---

// contains checks if s contains substr (avoids import of strings in test logic).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// sha256file returns the hex SHA256 of the given bytes (for test setup).
func sha256file(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// TestDatasetPull_RelativeDest verifies that Pull works when dest is a relative
// path such as "." (the original guard was broken for relative paths).
func TestDatasetPull_RelativeDest(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig) //nolint:errcheck

	content := []byte("hello relative")
	h := sha256file(content)
	manifest := []ManifestEntry{{Path: "out.txt", Hash: h, Size: int64(len(content))}}
	mJSON, _ := json.Marshal(manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "c4_datasets"):
			rows := []map[string]any{{"version_hash": "vhash-rel", "manifest": json.RawMessage(mJSON)}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "storage"):
			w.Write(content)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-rel"))
	result, err := dc.Pull(context.Background(), "ds-rel", ".", "")
	if err != nil {
		t.Fatalf("Pull with relative dest failed: %v", err)
	}
	if result.FilesDownloaded != 1 {
		t.Errorf("want FilesDownloaded=1, got %d", result.FilesDownloaded)
	}
}

// TestDatasetPull_TraversalRejected verifies that a manifest entry containing
// a path traversal sequence is rejected with an error.
func TestDatasetPull_TraversalRejected(t *testing.T) {
	dest := t.TempDir()

	manifest := []ManifestEntry{
		{Path: "../escape.txt", Hash: strings.Repeat("a", 64), Size: 10},
	}
	mJSON, _ := json.Marshal(manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "c4_datasets") {
			rows := []map[string]any{{"version_hash": "vhash-trav", "manifest": json.RawMessage(mJSON)}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
		}
	}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-trav"))
	_, err := dc.Pull(context.Background(), "ds-trav", dest, "")
	if err == nil {
		t.Fatal("expected error for path traversal manifest entry, got nil")
	}
	if !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestValidateName verifies validateName rejects empty, path-separator, and
// dot-dot names while accepting normal names.
func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"", true},
		{"../etc/passwd", true},
		{"a/b", true},
		{`a\b`, true},
		{"valid-name", false},
		{"model_v1.0", false},
	}
	for _, tc := range cases {
		err := validateName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("validateName(%q): want error, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateName(%q): want nil, got %v", tc.name, err)
		}
	}
}

// TestCasStoragePath_ShortHash verifies that casStoragePath returns an error
// for hashes shorter than 64 characters.
func TestCasStoragePath_ShortHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	dc := NewDatasetClient(setupTestClient(srv.URL, "proj-cas"))
	_, err := dc.casStoragePath("tooshort")
	if err == nil {
		t.Fatal("expected error for short hash, got nil")
	}
}
