//go:build c0_drive

package drive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newResumeTestServer returns a mock HTTP server that supports Range requests.
// behavior controls how the server responds:
//   - "range"   — responds with 206 Partial Content when Range header is present
//   - "norange" — always responds with 200 OK (ignores Range header)
func newResumeTestServer(t *testing.T, content []byte, behavior string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rangeHeader := r.Header.Get("Range")
		if behavior == "range" && rangeHeader != "" {
			// Parse "bytes=N-"
			var offset int64
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &offset); err != nil || offset < 0 || offset >= int64(len(content)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(content)-1, len(content)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", int64(len(content))-offset))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(content[offset:])
			return
		}
		// Full response (200 OK)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestResumeDownload_Fresh verifies a clean download (no .part file) completes
// and produces the correct output file.
func TestResumeDownload_Fresh(t *testing.T) {
	content := []byte("hello, fresh download!")
	srv := newResumeTestServer(t, content, "range")

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.bin")

	err := resumeDownload(context.Background(), srv.URL+"/file", destPath, func(req *http.Request) {
		req.Header.Set("X-Test-Auth", "token123")
	})
	if err != nil {
		t.Fatalf("resumeDownload returned error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// .part file must be cleaned up after success
	if _, err := os.Stat(destPath + ".part"); !os.IsNotExist(err) {
		t.Error(".part file should not exist after successful download")
	}
}

// TestResumeDownload_Resume verifies that a pre-existing .part file causes the
// client to send a Range header and append only the missing bytes.
func TestResumeDownload_Resume(t *testing.T) {
	content := []byte("abcdefghijklmnopqrstuvwxyz")
	partialContent := content[:10] // first 10 bytes already downloaded

	// Track whether the server received a Range header
	rangeHeaderReceived := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeaderReceived = r.Header.Get("Range")
		var offset int64
		if rangeHeaderReceived != "" {
			fmt.Sscanf(rangeHeaderReceived, "bytes=%d-", &offset)
		}
		if offset > 0 && int(offset) < len(content) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(content)-1, len(content)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", int64(len(content))-offset))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(content[offset:])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	t.Cleanup(srv.Close)

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.bin")
	partPath := destPath + ".part"

	// Pre-seed .part file
	if err := os.WriteFile(partPath, partialContent, 0o644); err != nil {
		t.Fatalf("write .part file: %v", err)
	}

	err := resumeDownload(context.Background(), srv.URL+"/file", destPath, func(req *http.Request) {})
	if err != nil {
		t.Fatalf("resumeDownload returned error: %v", err)
	}

	// Verify Range header was sent with the correct offset
	if rangeHeaderReceived == "" {
		t.Error("expected Range header to be sent, but none was received")
	}
	expectedRange := "bytes=10-"
	if rangeHeaderReceived != expectedRange {
		t.Errorf("Range header: got %q, want %q", rangeHeaderReceived, expectedRange)
	}

	// Verify final file content
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// .part file must be cleaned up
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Error(".part file should not exist after successful download")
	}
}

// TestResumeDownload_ServerNoRange verifies that when the server returns 200
// instead of 206 in response to a Range request, the client discards the .part
// file and performs a full re-download.
func TestResumeDownload_ServerNoRange(t *testing.T) {
	content := []byte("full content after server ignores range")

	// Server always returns 200, ignoring any Range header
	srv := newResumeTestServer(t, content, "norange")

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.bin")
	partPath := destPath + ".part"

	// Pre-seed .part file with stale data
	staleData := []byte("stale stale stale")
	if err := os.WriteFile(partPath, staleData, 0o644); err != nil {
		t.Fatalf("write .part file: %v", err)
	}

	err := resumeDownload(context.Background(), srv.URL+"/file", destPath, func(req *http.Request) {})
	if err != nil {
		t.Fatalf("resumeDownload returned error: %v", err)
	}

	// Final file should contain full content, not stale + full
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Content must not contain stale data prefix
	if strings.HasPrefix(string(got), string(staleData)) && len(got) > len(staleData) {
		t.Error("output file appears to contain stale data from old .part file")
	}

	// .part file must be cleaned up
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Error(".part file should not exist after successful download")
	}
}
