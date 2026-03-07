package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/hub"
)

// TestHubSubmit verifies that runHubSubmit POSTs the correct body to the Hub.
func TestHubSubmit(t *testing.T) {
	// Capture the request body from /v1/jobs/submit.
	var captured hub.JobSubmitRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{
				JobID:         "job-test-001",
				Status:        "QUEUED",
				QueuePosition: 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Point projectDir at a temp dir with a minimal config.yaml that enables the hub.
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + srv.URL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	origRun := hubSubmitRun
	hubSubmitRun = "python3 train.py"
	defer func() { hubSubmitRun = origRun }()

	// Change cwd to tmpDir so os.Getwd returns a deterministic path.
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	// Verify POST body fields.
	if captured.Command != "python3 train.py" {
		t.Errorf("command = %q, want %q", captured.Command, "python3 train.py")
	}
	if !strings.HasSuffix(captured.Workdir, filepath.Base(tmpDir)) &&
		captured.Workdir != tmpDir {
		// Workdir should contain tmpDir (may be resolved via symlink on macOS).
		if !strings.Contains(captured.Workdir, "tmp") && !strings.Contains(captured.Workdir, "Temp") {
			t.Errorf("workdir = %q, expected tmpDir", captured.Workdir)
		}
	}
	// SnapshotVersionHash is empty because Drive is not configured — that is expected.
	if captured.SnapshotVersionHash != "" {
		t.Errorf("expected empty SnapshotVersionHash without drive, got %q", captured.SnapshotVersionHash)
	}
}

// TestHubSubmit_MissingRunFlag verifies that missing --run returns an error.
func TestHubSubmit_MissingRunFlag(t *testing.T) {
	origRun := hubSubmitRun
	hubSubmitRun = ""
	defer func() { hubSubmitRun = origRun }()

	err := runHubSubmit(nil, nil)
	if err == nil {
		t.Fatal("expected error when --run is empty")
	}
	if !strings.Contains(err.Error(), "--run") {
		t.Errorf("error %q should mention --run flag", err.Error())
	}
}
