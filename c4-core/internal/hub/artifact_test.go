package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// =========================================================================
// GetPresignedURL
// =========================================================================

func TestGetPresignedURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/storage/presigned-url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req PresignedURLRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "outputs/model.pt" {
			t.Errorf("path = %q", req.Path)
		}
		if req.Method != "PUT" {
			t.Errorf("method = %q, want PUT", req.Method)
		}
		jsonResponse(w, PresignedURLResponse{
			URL:       "https://storage.example.com/presigned-put-url",
			ExpiresAt: "2026-01-01T01:00:00Z",
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.GetPresignedURL("outputs/model.pt", "PUT", 3600)
	if err != nil {
		t.Fatalf("GetPresignedURL: %v", err)
	}
	if resp.URL != "https://storage.example.com/presigned-put-url" {
		t.Errorf("URL = %q", resp.URL)
	}
}

// =========================================================================
// ConfirmArtifact
// =========================================================================

func TestConfirmArtifact(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/artifacts/job-1/confirm", func(w http.ResponseWriter, r *http.Request) {
		var req ArtifactConfirmRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "outputs/model.pt" {
			t.Errorf("path = %q", req.Path)
		}
		if req.SizeBytes != 1024 {
			t.Errorf("size = %d", req.SizeBytes)
		}
		jsonResponse(w, ArtifactConfirmResponse{
			ArtifactID: "art-1",
			Confirmed:  true,
		})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.ConfirmArtifact("job-1", "outputs/model.pt", "sha256:abc", 1024)
	if err != nil {
		t.Fatalf("ConfirmArtifact: %v", err)
	}
	if !resp.Confirmed {
		t.Error("expected confirmed=true")
	}
	if resp.ArtifactID != "art-1" {
		t.Errorf("ArtifactID = %q", resp.ArtifactID)
	}
}

// =========================================================================
// GetArtifactURL
// =========================================================================

func TestGetArtifactURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/artifacts/job-2/url/model.pt", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ArtifactURLResponse{URL: "https://cdn.example.com/model.pt"})
	})
	client, _ := newTestServer(t, mux)

	url, err := client.GetArtifactURL("job-2", "model.pt")
	if err != nil {
		t.Fatalf("GetArtifactURL: %v", err)
	}
	if url != "https://cdn.example.com/model.pt" {
		t.Errorf("URL = %q", url)
	}
}

// =========================================================================
// UploadArtifact (full flow: presigned → PUT → confirm)
// =========================================================================

func TestUploadArtifact(t *testing.T) {
	// Create a temp file to upload
	dir := t.TempDir()
	localPath := filepath.Join(dir, "weights.bin")
	content := []byte("fake model weights for testing")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute expected hash
	h := sha256.Sum256(content)
	expectedHash := "sha256:" + hex.EncodeToString(h[:])

	var uploadedData []byte

	// Mock: presigned URL server (simulates S3)
	putServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("PUT server: method = %s", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer putServer.Close()

	// Mock: Hub API
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/storage/presigned-url", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, PresignedURLResponse{
			URL:       putServer.URL + "/upload",
			ExpiresAt: "2026-01-01T01:00:00Z",
		})
	})
	mux.HandleFunc("/v1/artifacts/job-3/confirm", func(w http.ResponseWriter, r *http.Request) {
		var req ArtifactConfirmRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.ContentHash != expectedHash {
			t.Errorf("hash = %q, want %q", req.ContentHash, expectedHash)
		}
		if req.SizeBytes != int64(len(content)) {
			t.Errorf("size = %d, want %d", req.SizeBytes, len(content))
		}

		jsonResponse(w, ArtifactConfirmResponse{ArtifactID: "art-3", Confirmed: true})
	})
	client, _ := newTestServer(t, mux)

	resp, err := client.UploadArtifact("job-3", "outputs/weights.bin", localPath)
	if err != nil {
		t.Fatalf("UploadArtifact: %v", err)
	}
	if !resp.Confirmed {
		t.Error("expected confirmed")
	}
	if string(uploadedData) != string(content) {
		t.Error("uploaded data mismatch")
	}
}

// =========================================================================
// DownloadArtifact
// =========================================================================

func TestDownloadArtifact(t *testing.T) {
	expectedContent := "downloaded model data"

	// Mock: download server
	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expectedContent))
	}))
	defer dlServer.Close()

	// Mock: Hub API
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/artifacts/job-4/url/results.csv", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ArtifactURLResponse{URL: dlServer.URL + "/results.csv"})
	})
	client, _ := newTestServer(t, mux)

	dir := t.TempDir()
	destPath := filepath.Join(dir, "results.csv")

	err := client.DownloadArtifact("job-4", "results.csv", destPath)
	if err != nil {
		t.Fatalf("DownloadArtifact: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expectedContent {
		t.Errorf("content = %q, want %q", string(data), expectedContent)
	}
}

// =========================================================================
// sha256File
// =========================================================================

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	hash, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := sha256.Sum256([]byte("hello world"))
	if hash != hex.EncodeToString(expected[:]) {
		t.Errorf("hash = %s", hash)
	}
}

// =========================================================================
// Error cases
// =========================================================================

func TestUploadArtifact_FileNotFound(t *testing.T) {
	client := &Client{baseURL: "http://localhost", apiKey: "k", httpClient: http.DefaultClient}
	_, err := client.UploadArtifact("job-1", "path", "/nonexistent/file.bin")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGetPresignedURL_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/storage/presigned-url", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.GetPresignedURL("path", "PUT", 3600)
	if err == nil {
		t.Error("expected error on 500")
	}
}
