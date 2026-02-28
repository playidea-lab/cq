package drive

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// staticTP is a test-only tokenProvider that returns a fixed token.
type staticTP struct{ token string }

func (s *staticTP) Token() string              { return s.token }
func (s *staticTP) Refresh() (string, error)   { return s.token, nil }

// newTestServer creates an httptest server that simulates Supabase Storage + PostgREST.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// In-memory file store
	storedFiles := make(map[string][]byte)
	metadataRows := make([]map[string]any, 0)

	mux := http.NewServeMux()

	// Supabase Storage: upload
	mux.HandleFunc("POST /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		body, _ := io.ReadAll(r.Body)
		storedFiles[storagePath] = body
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"Key": storagePath})
	})

	// Supabase Storage: download
	mux.HandleFunc("GET /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		data, ok := storedFiles[storagePath]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// Supabase Storage: delete
	mux.HandleFunc("DELETE /storage/v1/object/c4-drive/", func(w http.ResponseWriter, r *http.Request) {
		storagePath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/c4-drive/")
		delete(storedFiles, storagePath)
		w.WriteHeader(http.StatusOK)
	})

	// PostgREST: insert/upsert metadata
	mux.HandleFunc("POST /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		var row map[string]any
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		row["id"] = "test-uuid-" + row["path"].(string)
		row["created_at"] = "2026-02-14T00:00:00Z"
		row["updated_at"] = "2026-02-14T00:00:00Z"

		// Upsert: replace if path matches
		found := false
		for i, existing := range metadataRows {
			if existing["path"] == row["path"] && existing["project_id"] == row["project_id"] {
				metadataRows[i] = row
				found = true
				break
			}
		}
		if !found {
			metadataRows = append(metadataRows, row)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode([]map[string]any{row})
	})

	// PostgREST: query metadata
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		var result []map[string]any

		for _, row := range metadataRows {
			match := true
			if strings.Contains(query, "path=eq.") {
				pathVal := extractFilter(query, "path=eq.")
				if row["path"] != pathVal {
					match = false
				}
			}
			if strings.Contains(query, "project_id=eq.") {
				projVal := extractFilter(query, "project_id=eq.")
				if row["project_id"] != projVal {
					match = false
				}
			}
			if strings.Contains(query, "path=like.") {
				prefix := extractFilter(query, "path=like.")
				prefix = strings.TrimSuffix(prefix, "*")
				p, _ := row["path"].(string)
				if !strings.HasPrefix(p, prefix) {
					match = false
				}
			}
			// Handle path=not.like.X (root depth filter)
			if strings.Contains(query, "path=not.like.") {
				pattern := extractFilter(query, "path=not.like.")
				p, _ := row["path"].(string)
				if matchLikePattern(p, pattern) {
					match = false
				}
			}
			// Handle and=(path.like.X,path.not.like.Y) composite filter
			if andIdx := strings.Index(query, "and="); andIdx >= 0 {
				rest := query[andIdx+len("and="):]
				if closeIdx := strings.Index(rest, ")"); closeIdx >= 0 {
					inner := rest[1:closeIdx] // skip '('
					if unescaped, err := url.QueryUnescape(inner); err == nil {
						inner = unescaped
					}
					p, _ := row["path"].(string)
					if likeIdx := strings.Index(inner, "path.like."); likeIdx >= 0 {
						val := inner[likeIdx+len("path.like."):]
						if ci := strings.Index(val, ","); ci >= 0 {
							val = val[:ci]
						}
						if !matchLikePattern(p, val) {
							match = false
						}
					}
					if notLikeIdx := strings.Index(inner, "path.not.like."); notLikeIdx >= 0 {
						val := inner[notLikeIdx+len("path.not.like."):]
						if ci := strings.Index(val, ","); ci >= 0 {
							val = val[:ci]
						}
						if matchLikePattern(p, val) {
							match = false
						}
					}
				}
			}
			if match {
				result = append(result, row)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// PostgREST: delete metadata
	mux.HandleFunc("DELETE /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		newRows := make([]map[string]any, 0)
		for _, row := range metadataRows {
			pathVal := extractFilter(query, "path=eq.")
			if row["path"] != pathVal {
				newRows = append(newRows, row)
			}
		}
		metadataRows = newRows
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

// extractFilter extracts a filter value from a PostgREST query string (URL-decodes the value).
func extractFilter(query, prefix string) string {
	idx := strings.Index(query, prefix)
	if idx < 0 {
		return ""
	}
	val := query[idx+len(prefix):]
	if ampIdx := strings.Index(val, "&"); ampIdx >= 0 {
		val = val[:ampIdx]
	}
	if unescaped, err := url.QueryUnescape(val); err == nil {
		return unescaped
	}
	return val
}

// matchLikePattern matches a string against a PostgREST LIKE pattern where * = any chars.
func matchLikePattern(s, pattern string) bool {
	si, pi := 0, 0
	starIdx, match := -1, 0
	for si < len(s) {
		if pi < len(pattern) && (pattern[pi] == s[si] || pattern[pi] == '*') {
			if pattern[pi] == '*' {
				starIdx = pi
				match = si
				pi++
				continue
			}
			si++
			pi++
		} else if starIdx >= 0 {
			pi = starIdx + 1
			match++
			si = match
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func TestUploadAndDownload(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	// Create a temp file to upload
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(srcPath, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Upload
	info, err := client.Upload(srcPath, "/docs/hello.txt", nil)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if info.Name != "hello.txt" {
		t.Errorf("Name = %q, want %q", info.Name, "hello.txt")
	}
	if info.Path != "/docs/hello.txt" {
		t.Errorf("Path = %q, want %q", info.Path, "/docs/hello.txt")
	}
	if info.SizeBytes != 11 {
		t.Errorf("SizeBytes = %d, want %d", info.SizeBytes, 11)
	}
	if !strings.HasPrefix(info.ContentHash, "sha256:") {
		t.Errorf("ContentHash = %q, want sha256: prefix", info.ContentHash)
	}

	// Download
	destPath := filepath.Join(tmpDir, "downloaded.txt")
	if err := client.Download("/docs/hello.txt", destPath); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("downloaded content = %q, want %q", string(data), "hello world")
	}
}

func TestUploadWithMetadata(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "meta.txt")
	os.WriteFile(srcPath, []byte("with metadata"), 0o644)

	meta := json.RawMessage(`{"tags":["important"],"version":2}`)
	info, err := client.Upload(srcPath, "/docs/meta.txt", meta)
	if err != nil {
		t.Fatalf("Upload with metadata failed: %v", err)
	}
	if info.Name != "meta.txt" {
		t.Errorf("Name = %q, want %q", info.Name, "meta.txt")
	}
}

func TestList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	// Create folder + files
	if _, err := client.Mkdir("/docs", nil); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "a.txt")
	os.WriteFile(f1, []byte("aaa"), 0o644)
	f2 := filepath.Join(tmpDir, "b.txt")
	os.WriteFile(f2, []byte("bbb"), 0o644)

	if _, err := client.Upload(f1, "/docs/a.txt", nil); err != nil {
		t.Fatalf("Upload a.txt failed: %v", err)
	}
	if _, err := client.Upload(f2, "/docs/b.txt", nil); err != nil {
		t.Fatalf("Upload b.txt failed: %v", err)
	}

	files, err := client.List("/docs")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("List returned %d files, want 2", len(files))
	}
}

func TestMkdir(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	info, err := client.Mkdir("/reports", nil)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	if info.Name != "reports" {
		t.Errorf("Name = %q, want %q", info.Name, "reports")
	}
	if info.Path != "/reports" {
		t.Errorf("Path = %q, want %q", info.Path, "/reports")
	}
	if !info.IsFolder {
		t.Error("IsFolder should be true")
	}
}

func TestDelete(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "delete-me.txt")
	os.WriteFile(srcPath, []byte("delete me"), 0o644)

	if _, err := client.Upload(srcPath, "/temp/delete-me.txt", nil); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify exists
	if _, err := client.Info("/temp/delete-me.txt"); err != nil {
		t.Fatalf("Info before delete failed: %v", err)
	}

	// Delete
	if err := client.Delete("/temp/delete-me.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	if _, err := client.Info("/temp/delete-me.txt"); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestInfo(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "info-test.txt")
	os.WriteFile(srcPath, []byte("info test content"), 0o644)

	if _, err := client.Upload(srcPath, "/data/info-test.txt", nil); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	info, err := client.Info("/data/info-test.txt")
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if info.Name != "info-test.txt" {
		t.Errorf("Name = %q, want %q", info.Name, "info-test.txt")
	}
	if info.Path != "/data/info-test.txt" {
		t.Errorf("Path = %q, want %q", info.Path, "/data/info-test.txt")
	}
	if info.SizeBytes != 17 {
		t.Errorf("SizeBytes = %d, want %d", info.SizeBytes, 17)
	}
}

func TestInfoNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	_, err := client.Info("/nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' substring", err.Error())
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/", "/"},
		{"docs", "/docs"},
		{"/docs/", "/docs"},
		{"//docs//file.txt//", "/docs/file.txt"},
		{"/a/b/../c", "/a/c"},
	}

	for _, tc := range tests {
		got := normalizePath(tc.input)
		if got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestRetry5xxThenSuccess verifies doWithRetry retries on 5xx and succeeds on a later attempt.
// Server returns 503 on the first request, then 200 on the second.
// This exercises the "resp.StatusCode >= 500 → continue" branch and the eventual success path.
func TestRetry5xxThenSuccess(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	// List endpoint: fail with 503 once, then succeed.
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: return 503 to trigger retry.
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`service unavailable`))
			return
		}
		// Second call: return empty list.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	files, err := client.List("/")
	if err != nil {
		t.Fatalf("List failed after retry: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("List returned %d files, want 0", len(files))
	}
	if got := callCount.Load(); got < 2 {
		t.Errorf("expected at least 2 server calls (retry), got %d", got)
	}
}

// TestRetryExhausted5xx verifies doWithRetry returns an error after all retries are exhausted on 5xx.
func TestRetryExhausted5xx(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	// Always return 503.
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`always unavailable`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	_, err := client.List("/")
	if err == nil {
		t.Fatal("expected error when all retries exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %q, want to contain '503'", err.Error())
	}
	// All driveMaxRetries (3) attempts should have been made.
	if got := callCount.Load(); got != int32(driveMaxRetries) {
		t.Errorf("expected %d server calls, got %d", driveMaxRetries, got)
	}
}

// TestRetryConnectionFailure verifies doWithRetry returns an error when the server is unreachable.
// Uses a port that is not listening to trigger a net.Error (connection refused).
func TestRetryConnectionFailure(t *testing.T) {
	// Use a server that we immediately close so the port is not listening.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // Close before making requests — port no longer accepts connections.

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	_, err := client.List("/")
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

// TestRetry4xxNoRetry verifies doWithRetry does NOT retry on 4xx responses.
// A 4xx response means the request is returned immediately (non-retryable).
func TestRetry4xxNoRetry(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	// Return 401 Unauthorized.
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`unauthorized`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	// List calls doWithRetry; 401 should be returned without retry.
	// The 401 response propagates as an HTTP error from the List() caller.
	_, err := client.List("/")
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	// Only 1 server call should have been made (no retry on 4xx).
	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 server call (no retry on 4xx), got %d", got)
	}
}

// TestNewClientCustomBucket verifies that a non-empty bucketName variadic arg is used.
func TestNewClientCustomBucket(t *testing.T) {
	client := NewClient("https://example.supabase.co", "key", &staticTP{"tok"}, "proj", "custom-bucket")
	if client.bucketName != "custom-bucket" {
		t.Errorf("bucketName = %q, want %q", client.bucketName, "custom-bucket")
	}
}

// TestNewClientDefaultBucket verifies that an empty bucketName variadic arg uses the default.
func TestNewClientDefaultBucket(t *testing.T) {
	client := NewClient("https://example.supabase.co", "key", &staticTP{"tok"}, "proj", "")
	if client.bucketName != DefaultBucketName {
		t.Errorf("bucketName = %q, want %q", client.bucketName, DefaultBucketName)
	}
}

// TestNormalizePathRoot verifies normalizePath returns "/" for empty string.
func TestNormalizePathRoot(t *testing.T) {
	if got := normalizePath(""); got != "/" {
		t.Errorf("normalizePath(%q) = %q, want %q", "", got, "/")
	}
}

// TestDownloadIsFolder verifies Download returns an error when the path is a folder.
func TestDownloadIsFolder(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	// Create a folder entry.
	if _, err := client.Mkdir("/downloads", nil); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	tmpDir := t.TempDir()
	err := client.Download("/downloads", filepath.Join(tmpDir, "out"))
	if err == nil {
		t.Fatal("expected error downloading a folder, got nil")
	}
	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("error = %q, want 'folder' substring", err.Error())
	}
}

// TestListError4xx verifies List returns an error on a 4xx response.
func TestListError4xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`forbidden`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")
	_, err := client.List("/")
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want '403' substring", err.Error())
	}
}

// TestMkdirError4xx verifies Mkdir returns an error on a 4xx response.
func TestMkdirError4xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`forbidden`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")
	_, err := client.Mkdir("/nope", nil)
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want '403' substring", err.Error())
	}
}

// TestInfoError4xx verifies Info returns an error on a 4xx response.
func TestInfoError4xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`forbidden`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")
	_, err := client.Info("/some/path")
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want '403' substring", err.Error())
	}
}

// TestMkdirWithMetadata verifies Mkdir passes metadata correctly and succeeds.
func TestMkdirWithMetadata(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")
	meta := json.RawMessage(`{"color":"blue"}`)
	info, err := client.Mkdir("/colored-folder", meta)
	if err != nil {
		t.Fatalf("Mkdir with metadata failed: %v", err)
	}
	if !info.IsFolder {
		t.Error("IsFolder should be true")
	}
}

// TestRetryGetBodyRefresh verifies that doWithRetry re-reads the body via GetBody on retry.
// Uses Mkdir (POST with body) and a server that fails the first POST with 503.
func TestRetryGetBodyRefresh(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/v1/c4_drive_files", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second call succeeds: decode and echo back.
		var row map[string]any
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		row["id"] = "test-uuid"
		row["created_at"] = "2026-01-01T00:00:00Z"
		row["updated_at"] = "2026-01-01T00:00:00Z"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode([]map[string]any{row})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, "test-key", &staticTP{"test-token"}, "test-project")

	info, err := client.Mkdir("/test-retry-body", nil)
	if err != nil {
		t.Fatalf("Mkdir failed after retry: %v", err)
	}
	if info == nil {
		t.Fatal("Mkdir returned nil FileInfo")
	}
	if got := callCount.Load(); got < 2 {
		t.Errorf("expected at least 2 server calls (retry), got %d", got)
	}
}
