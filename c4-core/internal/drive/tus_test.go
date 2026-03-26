//go:build c0_drive

package drive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

// noopSetHeaders is a no-op setHeaders callback for tests that don't need auth.
func noopSetHeaders(_ *http.Request) {}

// newTUSTestServer creates a mock TUS server.
// onPatch is called for each PATCH request; it can return a non-zero status to
// simulate failures.
func newTUSTestServer(t *testing.T, onPatch func(offset, length int64) int) (*httptest.Server, *[]byte) {
	t.Helper()

	stored := make([]byte, 0)
	var mu atomic.Int64 // current server-side offset

	mux := http.NewServeMux()

	// POST /storage/v1/upload/resumable — create TUS session
	mux.HandleFunc("POST /storage/v1/upload/resumable", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Tus-Resumable") != "1.0.0" {
			http.Error(w, "missing Tus-Resumable", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Upload-Length") == "" {
			http.Error(w, "missing Upload-Length", http.StatusBadRequest)
			return
		}
		// Reset stored data and offset for each new session.
		stored = stored[:0]
		mu.Store(0)
		w.Header().Set("Location", "http://"+r.Host+"/upload/123")
		w.WriteHeader(http.StatusCreated)
	})

	// HEAD /upload/123 — return current offset
	mux.HandleFunc("HEAD /upload/123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Upload-Offset", strconv.FormatInt(mu.Load(), 10))
		w.WriteHeader(http.StatusOK)
	})

	// PATCH /upload/123 — receive chunk
	mux.HandleFunc("PATCH /upload/123", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Tus-Resumable") != "1.0.0" {
			http.Error(w, "missing Tus-Resumable", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
			http.Error(w, "wrong Content-Type", http.StatusBadRequest)
			return
		}

		offsetStr := r.Header.Get("Upload-Offset")
		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			http.Error(w, "bad Upload-Offset", http.StatusBadRequest)
			return
		}

		// Verify offset matches server state.
		if offset != mu.Load() {
			http.Error(w, fmt.Sprintf("offset mismatch: want %d got %d", mu.Load(), offset), http.StatusConflict)
			return
		}

		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}

		// Delegate to caller — non-zero means simulate failure.
		if onPatch != nil {
			if code := onPatch(offset, int64(len(data))); code != 0 {
				http.Error(w, "simulated failure", code)
				return
			}
		}

		stored = append(stored, data...)
		newOffset := mu.Add(int64(len(data)))

		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &stored
}

// makeTempFile creates a temporary file with the given content.
func makeTempFile(t *testing.T, size int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tus-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data := bytes.Repeat([]byte("A"), size)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// TestTUSUpload_SmallFile verifies a single-chunk upload (file < 6MB).
func TestTUSUpload_SmallFile(t *testing.T) {
	srv, stored := newTUSTestServer(t, nil)

	const fileSize = 1024 // 1KB
	localPath := makeTempFile(t, fileSize)

	err := tusUpload(context.Background(), srv.URL, "c4-drive", "proj/test.bin", localPath, noopSetHeaders)
	if err != nil {
		t.Fatalf("tusUpload failed: %v", err)
	}

	if len(*stored) != fileSize {
		t.Errorf("stored %d bytes, want %d", len(*stored), fileSize)
	}

	// Verify content matches.
	original, _ := os.ReadFile(localPath)
	if !bytes.Equal(*stored, original) {
		t.Error("stored content does not match original file")
	}
}

// TestTUSUpload_MultiChunk verifies a multi-chunk upload (file > 6MB).
func TestTUSUpload_MultiChunk(t *testing.T) {
	var patchCount atomic.Int32
	srv, stored := newTUSTestServer(t, func(offset, length int64) int {
		patchCount.Add(1)
		return 0 // always succeed
	})

	// 13MB — should produce 3 chunks: 6MB + 6MB + 1MB
	const fileSize = 13 << 20
	localPath := makeTempFile(t, fileSize)

	err := tusUpload(context.Background(), srv.URL, "c4-drive", "proj/large.bin", localPath, noopSetHeaders)
	if err != nil {
		t.Fatalf("tusUpload failed: %v", err)
	}

	if len(*stored) != fileSize {
		t.Errorf("stored %d bytes, want %d", len(*stored), fileSize)
	}

	// 13MB / 6MB = 3 chunks (ceil)
	if got := patchCount.Load(); got != 3 {
		t.Errorf("expected 3 PATCH calls, got %d", got)
	}

	original, _ := os.ReadFile(localPath)
	if !bytes.Equal(*stored, original) {
		t.Error("stored content does not match original file")
	}
}

// TestTUSUpload_ResumeAfterFailure simulates a mid-upload failure on the second
// chunk and verifies the client resumes from the server's stored offset.
func TestTUSUpload_ResumeAfterFailure(t *testing.T) {
	// File: 3 chunks of 6MB each = 18MB.
	const fileSize = 18 << 20

	// Fail on the second PATCH call (index 1), succeed on all others.
	var patchAttempt atomic.Int32
	failOnce := true // fail the second call exactly once

	srv, stored := newTUSTestServer(t, func(offset, length int64) int {
		attempt := patchAttempt.Add(1)
		// Second PATCH attempt (first retry) — simulate 500
		if failOnce && attempt == 2 {
			failOnce = false
			return http.StatusInternalServerError
		}
		return 0
	})

	localPath := makeTempFile(t, fileSize)

	err := tusUpload(context.Background(), srv.URL, "c4-drive", "proj/resume.bin", localPath, noopSetHeaders)
	if err != nil {
		t.Fatalf("tusUpload failed: %v", err)
	}

	if len(*stored) != fileSize {
		t.Errorf("stored %d bytes, want %d", len(*stored), fileSize)
	}

	original, _ := os.ReadFile(localPath)
	if !bytes.Equal(*stored, original) {
		t.Error("stored content does not match original file after resume")
	}
}

// TestTUSBuildMetadata verifies the metadata header encoding.
func TestTUSBuildMetadata(t *testing.T) {
	got := tusBuildMetadata("my-bucket", "proj/file.bin")
	want := "bucketName bXktYnVja2V0,objectName cHJvai9maWxlLmJpbg,contentType YXBwbGljYXRpb24vb2N0ZXQtc3RyZWFt"
	if got != want {
		t.Errorf("tusBuildMetadata:\ngot:  %s\nwant: %s", got, want)
	}
}

// TestTUSUpload_ProgressOutput verifies the progress line is printed.
// We redirect stdout capture via a pipe and check for the expected line.
func TestTUSUpload_ProgressOutput(t *testing.T) {
	srv, _ := newTUSTestServer(t, nil)

	const fileSize = 512
	localPath := makeTempFile(t, fileSize)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = tusUpload(context.Background(), srv.URL, "c4-drive", "proj/small.bin", localPath, noopSetHeaders)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	baseName := filepath.Base(localPath)
	expected := fmt.Sprintf("drive: upload %s %d/%d (100%%)\n", baseName, fileSize, fileSize)
	if out != expected {
		t.Errorf("progress output:\ngot:  %q\nwant: %q", out, expected)
	}
}
