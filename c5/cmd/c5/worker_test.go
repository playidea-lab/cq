package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
