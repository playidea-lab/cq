package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piqsol/c4/c5/internal/model"
)

// withTempCwd changes to a temp directory for the duration of the test,
// returning the temp dir path. downloadInputArtifacts uses relative paths,
// so tests must chdir into a temp dir.
func withTempCwd(t *testing.T) string {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	return tmp
}

// TestDownloadInputArtifacts_RequiredSuccess verifies that required artifacts
// are downloaded and the file exists at local_path.
func TestDownloadInputArtifacts_RequiredSuccess(t *testing.T) {
	const payload = "artifact-content-here"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	tmp := withTempCwd(t)

	arts := []model.InputPresignedArtifact{
		{Path: "jobs/j1/data.bin", LocalPath: "data.bin", URL: srv.URL + "/data.bin", Required: true},
	}

	client := &http.Client{}
	if err := downloadInputArtifacts(client, arts); err != nil {
		t.Fatalf("downloadInputArtifacts() error = %v", err)
	}

	// Verify file exists and has correct content.
	got, err := os.ReadFile(filepath.Join(tmp, "data.bin"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(got) != payload {
		t.Errorf("content = %q, want %q", string(got), payload)
	}
}

// TestDownloadInputArtifacts_RequiredFailure verifies that a required artifact
// download failure returns an error.
func TestDownloadInputArtifacts_RequiredFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	withTempCwd(t)

	arts := []model.InputPresignedArtifact{
		{Path: "jobs/j1/missing.bin", LocalPath: "missing.bin", URL: srv.URL + "/missing", Required: true},
	}

	client := &http.Client{}
	err := downloadInputArtifacts(client, arts)
	if err == nil {
		t.Fatal("expected error for required artifact download failure, got nil")
	}
}

// TestDownloadInputArtifacts_OptionalFailure verifies that an optional artifact
// (required=false) download failure is logged as a warning and does not fail.
func TestDownloadInputArtifacts_OptionalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmp := withTempCwd(t)

	arts := []model.InputPresignedArtifact{
		{Path: "jobs/j1/optional.bin", LocalPath: "optional.bin", URL: srv.URL + "/opt", Required: false},
	}

	client := &http.Client{}
	if err := downloadInputArtifacts(client, arts); err != nil {
		t.Fatalf("expected no error for optional artifact failure, got: %v", err)
	}

	// File should NOT exist.
	if _, err := os.Stat(filepath.Join(tmp, "optional.bin")); err == nil {
		t.Error("optional artifact file should not exist after download failure")
	}
}

// TestDownloadInputArtifacts_MixedRequiredOptional verifies that when a mix of
// required and optional artifacts is provided, optional failures are skipped
// while required artifacts succeed.
func TestDownloadInputArtifacts_MixedRequiredOptional(t *testing.T) {
	const payload = "good-data"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, payload)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tmp := withTempCwd(t)

	arts := []model.InputPresignedArtifact{
		{Path: "jobs/j1/required.bin", LocalPath: "required.bin", URL: srv.URL + "/good", Required: true},
		{Path: "jobs/j1/optional.bin", LocalPath: "optional.bin", URL: srv.URL + "/bad", Required: false},
	}

	client := &http.Client{}
	if err := downloadInputArtifacts(client, arts); err != nil {
		t.Fatalf("downloadInputArtifacts() error = %v", err)
	}

	// Required file should exist.
	got, err := os.ReadFile(filepath.Join(tmp, "required.bin"))
	if err != nil {
		t.Fatalf("required artifact not found: %v", err)
	}
	if string(got) != payload {
		t.Errorf("required content = %q, want %q", string(got), payload)
	}

	// Optional file should NOT exist.
	if _, err := os.Stat(filepath.Join(tmp, "optional.bin")); err == nil {
		t.Error("optional artifact file should not exist after download failure")
	}
}

// TestDownloadInputArtifacts_DefaultLocalPath verifies that when LocalPath is
// empty, the base name of Path is used.
func TestDownloadInputArtifacts_DefaultLocalPath(t *testing.T) {
	const payload = "default-path-data"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	tmp := withTempCwd(t)

	arts := []model.InputPresignedArtifact{
		{Path: "jobs/j1/model.onnx", URL: srv.URL + "/model", Required: true},
	}

	client := &http.Client{}
	if err := downloadInputArtifacts(client, arts); err != nil {
		t.Fatalf("downloadInputArtifacts() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "model.onnx"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(got) != payload {
		t.Errorf("content = %q, want %q", string(got), payload)
	}
}

// TestDownloadInputArtifacts_PathTraversal verifies path traversal is rejected
// for required artifacts and skipped for optional ones.
func TestDownloadInputArtifacts_PathTraversal(t *testing.T) {
	client := &http.Client{}

	t.Run("required_path_traversal", func(t *testing.T) {
		arts := []model.InputPresignedArtifact{
			{Path: "../../etc/passwd", LocalPath: "../../etc/passwd", URL: "http://example.com/x", Required: true},
		}
		err := downloadInputArtifacts(client, arts)
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("optional_path_traversal", func(t *testing.T) {
		arts := []model.InputPresignedArtifact{
			{Path: "../../etc/shadow", LocalPath: "../../etc/shadow", URL: "http://example.com/x", Required: false},
		}
		// Should skip without error.
		if err := downloadInputArtifacts(client, arts); err != nil {
			t.Fatalf("expected no error for optional path traversal skip, got: %v", err)
		}
	})
}

// =========================================================================
// Upload output artifact tests
// =========================================================================

// newUploadTestServer creates a mock C5 server that accepts presigned URL
// requests and artifact confirm calls. uploadedData captures what was uploaded.
func newUploadTestServer(t *testing.T, uploadedData map[string][]byte, confirmedArts map[string]bool) (*httptest.Server, *httptest.Server) {
	t.Helper()

	// Storage server (simulates Supabase presigned upload endpoint)
	storageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		data, _ := io.ReadAll(r.Body)
		uploadedData[r.URL.Path] = data
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storageSrv.Close)

	// C5 API server (presigned URL + confirm)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/storage/presigned-url" && r.Method == "POST":
			var req model.PresignedURLRequest
			json.NewDecoder(r.Body).Decode(&req)
			resp := model.PresignedURLResponse{
				URL:       storageSrv.URL + "/" + req.Path,
				ExpiresAt: "2099-01-01T00:00:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/confirm") && r.Method == "POST":
			var req model.ArtifactConfirmRequest
			json.NewDecoder(r.Body).Decode(&req)
			confirmedArts[req.Path] = true
			resp := model.ArtifactConfirmResponse{
				ArtifactID: "art-001",
				Confirmed:  true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiSrv.Close)

	return apiSrv, storageSrv
}

// TestUploadOutputArtifacts_Success verifies that an existing output file is
// uploaded via presigned URL and confirmed with the C5 server.
func TestUploadOutputArtifacts_Success(t *testing.T) {
	tmp := withTempCwd(t)

	// Create output file
	content := []byte("model-output-data")
	if err := os.WriteFile(filepath.Join(tmp, "result.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/result.bin", LocalPath: "result.bin", Required: true},
	}

	if err := uploadOutputArtifacts(client, "job-1", arts); err != nil {
		t.Fatalf("uploadOutputArtifacts() error = %v", err)
	}

	// Verify file was uploaded
	if got, ok := uploadedData["/outputs/result.bin"]; !ok {
		t.Error("expected upload to /outputs/result.bin, not found")
	} else if string(got) != string(content) {
		t.Errorf("uploaded content = %q, want %q", string(got), string(content))
	}

	// Verify artifact was confirmed
	if !confirmedArts["outputs/result.bin"] {
		t.Error("expected artifact outputs/result.bin to be confirmed")
	}
}

// TestUploadOutputArtifacts_RequiredMissing verifies that a missing required
// output artifact returns an error.
func TestUploadOutputArtifacts_RequiredMissing(t *testing.T) {
	withTempCwd(t)

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/missing.bin", LocalPath: "missing.bin", Required: true},
	}

	err := uploadOutputArtifacts(client, "job-1", arts)
	if err == nil {
		t.Fatal("expected error for missing required output artifact, got nil")
	}

	// Should not have uploaded or confirmed anything
	if len(uploadedData) != 0 {
		t.Error("expected no uploads for missing artifact")
	}
	if len(confirmedArts) != 0 {
		t.Error("expected no confirms for missing artifact")
	}
}

// TestUploadOutputArtifacts_OptionalMissing verifies that a missing optional
// output artifact is skipped with a warning (no error).
func TestUploadOutputArtifacts_OptionalMissing(t *testing.T) {
	withTempCwd(t)

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/optional.bin", LocalPath: "optional.bin", Required: false},
	}

	if err := uploadOutputArtifacts(client, "job-1", arts); err != nil {
		t.Fatalf("expected no error for optional missing artifact, got: %v", err)
	}

	// Should not have uploaded or confirmed anything
	if len(uploadedData) != 0 {
		t.Error("expected no uploads for missing optional artifact")
	}
	if len(confirmedArts) != 0 {
		t.Error("expected no confirms for missing optional artifact")
	}
}

// TestUploadOutputArtifacts_MixedRequiredOptional verifies correct behavior
// when both required (exists) and optional (missing) artifacts are provided.
func TestUploadOutputArtifacts_MixedRequiredOptional(t *testing.T) {
	tmp := withTempCwd(t)

	// Create only the required file
	content := []byte("required-data")
	if err := os.WriteFile(filepath.Join(tmp, "required.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/required.bin", LocalPath: "required.bin", Required: true},
		{Path: "outputs/optional.bin", LocalPath: "optional.bin", Required: false},
	}

	if err := uploadOutputArtifacts(client, "job-1", arts); err != nil {
		t.Fatalf("uploadOutputArtifacts() error = %v", err)
	}

	// Required should be uploaded and confirmed
	if _, ok := uploadedData["/outputs/required.bin"]; !ok {
		t.Error("expected required artifact to be uploaded")
	}
	if !confirmedArts["outputs/required.bin"] {
		t.Error("expected required artifact to be confirmed")
	}

	// Optional should NOT be uploaded or confirmed
	if _, ok := uploadedData["/outputs/optional.bin"]; ok {
		t.Error("expected optional artifact to NOT be uploaded")
	}
	if confirmedArts["outputs/optional.bin"] {
		t.Error("expected optional artifact to NOT be confirmed")
	}
}

// TestUploadOutputArtifacts_DefaultLocalPath verifies that when LocalPath is
// empty, filepath.Base(Path) is used as the local path.
func TestUploadOutputArtifacts_DefaultLocalPath(t *testing.T) {
	tmp := withTempCwd(t)

	content := []byte("default-path-output")
	if err := os.WriteFile(filepath.Join(tmp, "model.onnx"), content, 0644); err != nil {
		t.Fatal(err)
	}

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/model.onnx", Required: true}, // LocalPath empty
	}

	if err := uploadOutputArtifacts(client, "job-1", arts); err != nil {
		t.Fatalf("uploadOutputArtifacts() error = %v", err)
	}

	if _, ok := uploadedData["/outputs/model.onnx"]; !ok {
		t.Error("expected upload with default local path")
	}
	if !confirmedArts["outputs/model.onnx"] {
		t.Error("expected artifact confirmed with default local path")
	}
}

// TestUploadOutputArtifacts_PathTraversal verifies path traversal is rejected.
func TestUploadOutputArtifacts_PathTraversal(t *testing.T) {
	withTempCwd(t)

	uploadedData := make(map[string][]byte)
	confirmedArts := make(map[string]bool)
	apiSrv, _ := newUploadTestServer(t, uploadedData, confirmedArts)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "../../etc/passwd", LocalPath: "../../etc/passwd", Required: true},
	}

	err := uploadOutputArtifacts(client, "job-1", arts)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

// TestUploadOutputArtifacts_ContentHash verifies that the confirm request
// includes a valid SHA-256 content hash.
func TestUploadOutputArtifacts_ContentHash(t *testing.T) {
	tmp := withTempCwd(t)

	content := []byte("hash-test-data")
	if err := os.WriteFile(filepath.Join(tmp, "hashfile.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}

	var capturedHash string
	var capturedSize int64

	// Custom API server to capture confirm details
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/storage/presigned-url" && r.Method == "POST":
			// Return a URL that just accepts the upload
			storageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(storageSrv.Close)
			resp := model.PresignedURLResponse{URL: storageSrv.URL + "/upload", ExpiresAt: "2099-01-01T00:00:00Z"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/confirm") && r.Method == "POST":
			var req model.ArtifactConfirmRequest
			json.NewDecoder(r.Body).Decode(&req)
			capturedHash = req.ContentHash
			capturedSize = req.SizeBytes
			resp := model.ArtifactConfirmResponse{ArtifactID: "art-002", Confirmed: true}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiSrv.Close)

	client := &workerClient{
		baseURL:      apiSrv.URL,
		http:         &http.Client{},
		artifactHTTP: &http.Client{},
	}

	arts := []model.ArtifactRef{
		{Path: "outputs/hashfile.bin", LocalPath: "hashfile.bin", Required: true},
	}

	if err := uploadOutputArtifacts(client, "job-1", arts); err != nil {
		t.Fatalf("uploadOutputArtifacts() error = %v", err)
	}

	// Verify hash is a valid SHA-256 hex string (64 chars)
	if len(capturedHash) != 64 {
		t.Errorf("expected SHA-256 hash (64 hex chars), got %q (len=%d)", capturedHash, len(capturedHash))
	}

	// Verify size matches
	if capturedSize != int64(len(content)) {
		t.Errorf("confirmed size = %d, want %d", capturedSize, len(content))
	}
}

// TestExecuteJob_OutputDirEnv verifies that C5_OUTPUT_DIR env is set.
func TestExecuteJob_OutputDirEnv(t *testing.T) {
	tmp := withTempCwd(t)
	outFile := filepath.Join(tmp, "env_check.txt")

	// Use a simple job that writes C5_OUTPUT_DIR to a file
	job := &model.Job{
		ID:      "env-test-1",
		Name:    "env-check",
		Command: fmt.Sprintf("echo $C5_OUTPUT_DIR > %s && echo $C5_INPUT_DIR >> %s", outFile, outFile),
		Workdir: tmp,
	}

	client := &workerClient{
		baseURL: "http://localhost:0", // not used for this test
		http:    &http.Client{},
	}

	exitCode, _ := executeJob(client, job, "lease-1", "worker-1", 0)
	if exitCode != 0 {
		t.Fatalf("executeJob() exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "." {
		t.Errorf("C5_OUTPUT_DIR = %q, want %q", lines[0], ".")
	}
	if lines[1] != "." {
		t.Errorf("C5_INPUT_DIR = %q, want %q", lines[1], ".")
	}
}

// TestWorkerRegister_VersionSet verifies CQ_VERSION is used in getWorkerVersion.
func TestWorkerRegister_VersionSet(t *testing.T) {
	t.Setenv("CQ_VERSION", "v0.62.0")
	if got := getWorkerVersion(); got != "v0.62.0" {
		t.Errorf("getWorkerVersion() = %q, want %q", got, "v0.62.0")
	}
}

// TestWorkerRegister_VersionUnset verifies "unknown" fallback when CQ_VERSION is not set.
func TestWorkerRegister_VersionUnset(t *testing.T) {
	t.Setenv("CQ_VERSION", "")
	if got := getWorkerVersion(); got != "unknown" {
		t.Errorf("getWorkerVersion() = %q, want %q", got, "unknown")
	}
}
