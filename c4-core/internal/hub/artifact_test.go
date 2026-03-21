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
	"strings"
	"testing"
)

// =========================================================================
// GetPresignedURL (PUT) — returns direct storage URL without server call
// =========================================================================

func TestGetPresignedURL_PUT(t *testing.T) {
	// For PUT, GetPresignedURL returns the storage URL directly without a server round-trip.
	client := &Client{
		baseURL:    "http://localhost:54321",
		supabaseURL: "http://localhost:54321",
		apiKey:     "test",
		httpClient: http.DefaultClient,
	}

	resp, err := client.GetPresignedURL("outputs/model.pt", "PUT", 3600)
	if err != nil {
		t.Fatalf("GetPresignedURL(PUT): %v", err)
	}
	if !strings.Contains(resp.URL, "storage/v1/object/c4-drive/outputs/model.pt") {
		t.Errorf("URL = %q, want URL containing storage path", resp.URL)
	}
}

// =========================================================================
// GetPresignedURL (GET) — calls sign endpoint
// =========================================================================

func TestGetPresignedURL_GET(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/storage/v1/object/sign/c4-drive/outputs/model.pt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		jsonResponse(w, map[string]any{
			"signedURL": "/storage/v1/object/sign/c4-drive/outputs/model.pt?token=abc",
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := &Client{
		baseURL:     ts.URL,
		supabaseURL: ts.URL,
		apiKey:      "test",
		httpClient:  http.DefaultClient,
	}

	resp, err := client.GetPresignedURL("outputs/model.pt", "GET", 3600)
	if err != nil {
		t.Fatalf("GetPresignedURL(GET): %v", err)
	}
	if !strings.Contains(resp.URL, "token=abc") {
		t.Errorf("URL = %q, want URL with token", resp.URL)
	}
}

// =========================================================================
// ConfirmArtifact
// =========================================================================

func TestConfirmArtifact(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_artifacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)
		if row["path"] != "outputs/model.pt" {
			t.Errorf("path = %v", row["path"])
		}
		sizeBytes, _ := row["size_bytes"].(float64)
		if int64(sizeBytes) != 1024 {
			t.Errorf("size_bytes = %v", row["size_bytes"])
		}
		jsonResponse(w, []map[string]any{
			{"id": "art-1", "confirmed": true},
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
	mux.HandleFunc("/rest/v1/hub_artifacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jsonResponse(w, []map[string]any{
			{"id": "art-2", "path": "jobs/job-2/model.pt"},
		})
	})
	client, _ := newTestServer(t, mux)

	url, err := client.GetArtifactURL("job-2", "model.pt")
	if err != nil {
		t.Fatalf("GetArtifactURL: %v", err)
	}
	if !strings.Contains(url, "c4-drive") {
		t.Errorf("URL = %q, want URL with bucket path", url)
	}
}

// =========================================================================
// UploadArtifact (full flow: compute hash → get URL → PUT → confirm)
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

	// Single server handles both storage PUT and PostgREST POST.
	mux := http.NewServeMux()

	// Supabase Storage upload endpoint
	mux.HandleFunc("/storage/v1/object/c4-drive/outputs/weights.bin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("storage: method = %s, want PUT", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	})

	// PostgREST confirm
	mux.HandleFunc("/rest/v1/hub_artifacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("artifacts: method = %s, want POST", r.Method)
		}
		var row map[string]any
		json.NewDecoder(r.Body).Decode(&row)

		if row["content_hash"] != expectedHash {
			t.Errorf("hash = %v, want %v", row["content_hash"], expectedHash)
		}
		sizeBytes, _ := row["size_bytes"].(float64)
		if int64(sizeBytes) != int64(len(content)) {
			t.Errorf("size = %v, want %d", row["size_bytes"], len(content))
		}

		jsonResponse(w, []map[string]any{{"id": "art-3", "confirmed": true}})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Client: supabaseURL = ts.URL so both storage and PostgREST go to same server.
	client := &Client{
		baseURL:     ts.URL,
		supabaseURL: ts.URL,
		apiKey:      "test",
		httpClient:  http.DefaultClient,
	}

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

	// Single server handles both PostgREST lookup and Supabase Storage download.
	mux := http.NewServeMux()

	// PostgREST artifact lookup
	mux.HandleFunc("/rest/v1/hub_artifacts", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, []map[string]any{
			{"id": "art-4", "path": "jobs/job-4/results.csv"},
		})
	})

	// Supabase Storage download endpoint
	mux.HandleFunc("/storage/v1/object/c4-drive/jobs/job-4/results.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expectedContent))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Client: supabaseURL = ts.URL so both PostgREST and storage go to same server.
	client := &Client{
		baseURL:     ts.URL,
		supabaseURL: ts.URL,
		apiKey:      "test",
		httpClient:  http.DefaultClient,
	}

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
	client := &Client{baseURL: "http://localhost", supabaseURL: "http://localhost", apiKey: "k", httpClient: http.DefaultClient}
	_, err := client.UploadArtifact("job-1", "path", "/nonexistent/file.bin")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestConfirmArtifact_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1/hub_artifacts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})
	client, _ := newTestServer(t, mux)

	_, err := client.ConfirmArtifact("job-1", "path", "sha256:abc", 100)
	if err == nil {
		t.Error("expected error on 500")
	}
}
