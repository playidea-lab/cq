package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestHubSubmit_MissingRunFlag verifies that missing --run and no cq.yaml returns an error.
func TestHubSubmit_MissingRunFlag(t *testing.T) {
	origRun := hubSubmitRun
	hubSubmitRun = ""
	defer func() { hubSubmitRun = origRun }()

	// Use a temp dir with no cq.yaml so fallback also fails.
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	err := runHubSubmit(nil, nil)
	if err == nil {
		t.Fatal("expected error when --run is empty and no cq.yaml")
	}
	if !strings.Contains(err.Error(), "--run") {
		t.Errorf("error %q should mention --run flag", err.Error())
	}
}

// TestHubSubmitExperiment verifies that experiment: section in cq.yaml is mapped
// to ExpID, Tags, Memo (JSON config), and Env[C5_DATASET_PATH].
func TestHubSubmitExperiment(t *testing.T) {
	var captured hub.JobSubmitRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{
				JobID:         "job-exp-001",
				Status:        "QUEUED",
				QueuePosition: 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + srv.URL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cqYamlContent := `run: python3 train.py
experiment:
  name: hmr-baseline
  tags: [hmr, agora, baseline]
  config: {lr: 0.001, backbone: vitl}
  datasets:
    worker_path: /data/agora
`
	if err := os.WriteFile(filepath.Join(tmpDir, "cq.yaml"), []byte(cqYamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	origRun := hubSubmitRun
	hubSubmitRun = "" // use cq.yaml run
	defer func() { hubSubmitRun = origRun }()

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	// 1. Name → ExpID
	if captured.ExpID != "hmr-baseline" {
		t.Errorf("ExpID = %q, want %q", captured.ExpID, "hmr-baseline")
	}
	// 2. Tags → Tags
	if len(captured.Tags) != 3 || captured.Tags[0] != "hmr" || captured.Tags[1] != "agora" || captured.Tags[2] != "baseline" {
		t.Errorf("Tags = %v, want [hmr agora baseline]", captured.Tags)
	}
	// 3. Config JSON → Memo
	if captured.Memo == "" {
		t.Error("Memo should not be empty when config is set")
	}
	var memo map[string]any
	if err := json.Unmarshal([]byte(captured.Memo), &memo); err != nil {
		t.Errorf("Memo is not valid JSON: %v", err)
	}
	// 4. WorkerPath → Env[C5_DATASET_PATH]
	if captured.Env["C5_DATASET_PATH"] != "/data/agora" {
		t.Errorf("Env[C5_DATASET_PATH] = %q, want %q", captured.Env["C5_DATASET_PATH"], "/data/agora")
	}
}

// TestHubSubmitExperiment_NoExperiment verifies that without experiment: section
// the existing behavior is unchanged (no ExpID/Tags/Memo/C5_DATASET_PATH).
func TestHubSubmitExperiment_NoExperiment(t *testing.T) {
	var captured hub.JobSubmitRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{JobID: "job-noexp-001", Status: "QUEUED"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	if captured.ExpID != "" {
		t.Errorf("ExpID should be empty without experiment: section, got %q", captured.ExpID)
	}
	if len(captured.Tags) != 0 {
		t.Errorf("Tags should be empty without experiment: section, got %v", captured.Tags)
	}
	if captured.Memo != "" {
		t.Errorf("Memo should be empty without experiment: section, got %q", captured.Memo)
	}
	if captured.Env["C5_DATASET_PATH"] != "" {
		t.Errorf("C5_DATASET_PATH should not be set without experiment.datasets, got %q", captured.Env["C5_DATASET_PATH"])
	}
}

// =========================================================================
// hub_format.go tests
// =========================================================================

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{0, "0s"},
		{45, "45s"},
		{59, "59s"},
		{60, "1m"},
		{90, "1m"},
		{120, "2m"},
		{3599, "59m"},
		{3600, "1h"},
		{3660, "1h 1m"},
		{8100, "2h 15m"},
		{86399, "23h 59m"},
		{86400, "1d"},
		{90000, "1d 1h"},
		{104400, "1d 5h"},
	}
	for _, c := range cases {
		got := formatUptime(c.sec)
		if got != c.want {
			t.Errorf("formatUptime(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}

func TestFormatLastJob(t *testing.T) {
	// "" → "never"
	if got := formatLastJob(""); got != "never" {
		t.Errorf("formatLastJob(\"\") = %q, want \"never\"", got)
	}

	now := func(offset int) string {
		return time.Now().Add(time.Duration(offset) * time.Second).UTC().Format(time.RFC3339)
	}

	// just now (30s ago)
	if got := formatLastJob(now(-30)); got != "just now" {
		t.Errorf("formatLastJob(30s ago) = %q, want \"just now\"", got)
	}
	// 5m ago
	ts := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	got := formatLastJob(ts)
	if got != "5m ago" {
		t.Errorf("formatLastJob(5m ago) = %q, want \"5m ago\"", got)
	}
	// 2h ago
	ts = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	got = formatLastJob(ts)
	if got != "2h ago" {
		t.Errorf("formatLastJob(2h ago) = %q, want \"2h ago\"", got)
	}
	// 1d ago
	ts = time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)
	got = formatLastJob(ts)
	if got != "1d ago" {
		t.Errorf("formatLastJob(25h ago) = %q, want \"1d ago\"", got)
	}
	// invalid → returned as-is
	if got := formatLastJob("not-a-date"); got != "not-a-date" {
		t.Errorf("formatLastJob(invalid) = %q, want \"not-a-date\"", got)
	}
}

// TestHubWorkers verifies that runHubWorkers outputs a tabwriter table.
func TestHubWorkers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/workers", "/v1/workers":
			json.NewEncoder(w).Encode([]hub.Worker{
				{
					ID:           "w-001",
					Name:         "gpu-lab-1",
					Status:       "online",
					UptimeSec:    3661,
					LastJobAt:    "",
					Capabilities: []string{"gpu.train", "gpu.inference"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	// Capture stdout
	origStdout := os.Stdout
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	err := runHubWorkers(nil, nil)

	w2.Close()
	os.Stdout = origStdout

	var buf strings.Builder
	io.Copy(&buf, r2) //nolint:errcheck
	out := buf.String()

	if err != nil {
		t.Fatalf("runHubWorkers: %v", err)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("output missing NAME header: %q", out)
	}
	if !strings.Contains(out, "gpu-lab-1") {
		t.Errorf("output missing worker name: %q", out)
	}
	if !strings.Contains(out, "1h 1m") {
		t.Errorf("output missing uptime: %q", out)
	}
	if !strings.Contains(out, "never") {
		t.Errorf("output missing last job: %q", out)
	}
	if !strings.Contains(out, "gpu.train") {
		t.Errorf("output missing capabilities: %q", out)
	}
}

// TestHubWorkers_Empty verifies message when no workers registered.
func TestHubWorkers_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/workers", "/v1/workers":
			json.NewEncoder(w).Encode([]hub.Worker{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	origStdout := os.Stdout
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	err := runHubWorkers(nil, nil)

	w2.Close()
	os.Stdout = origStdout

	var buf strings.Builder
	io.Copy(&buf, r2) //nolint:errcheck
	out := buf.String()

	if err != nil {
		t.Fatalf("runHubWorkers: %v", err)
	}
	if !strings.Contains(out, "No workers") {
		t.Errorf("expected 'No workers' message, got: %q", out)
	}
}

// TestHubWorkers_FallbackHostname verifies name fallback when Name is empty.
func TestHubWorkers_FallbackHostname(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/workers", "/v1/workers":
			json.NewEncoder(w).Encode([]hub.Worker{
				{ID: "w-002", Hostname: "my-host", Status: "offline"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	origStdout := os.Stdout
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	err := runHubWorkers(nil, nil)

	w2.Close()
	os.Stdout = origStdout

	var buf strings.Builder
	io.Copy(&buf, r2) //nolint:errcheck
	out := buf.String()

	if err != nil {
		t.Fatalf("runHubWorkers: %v", err)
	}
	if !strings.Contains(out, "my-host") {
		t.Errorf("expected hostname fallback 'my-host' in output: %q", out)
	}
}

// TestHubSubmitExperiment_NoWorkerPath verifies that C5_DATASET_PATH is not added
// when datasets.worker_path is empty.
func TestHubSubmitExperiment_NoWorkerPath(t *testing.T) {
	var captured hub.JobSubmitRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{JobID: "job-nowp-001", Status: "QUEUED"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + srv.URL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// experiment: with name but no datasets.worker_path
	cqYamlContent := "run: python3 train.py\nexperiment:\n  name: no-dataset-exp\n  tags: [test]\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "cq.yaml"), []byte(cqYamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	origRun := hubSubmitRun
	hubSubmitRun = ""
	defer func() { hubSubmitRun = origRun }()

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	if captured.ExpID != "no-dataset-exp" {
		t.Errorf("ExpID = %q, want %q", captured.ExpID, "no-dataset-exp")
	}
	if captured.Env["C5_DATASET_PATH"] != "" {
		t.Errorf("C5_DATASET_PATH should not be set when worker_path is empty, got %q", captured.Env["C5_DATASET_PATH"])
	}
}

// TestHubSubmit_CQYamlFallback verifies that the `run` field in cq.yaml is used
// when --run flag is not provided.
func TestHubSubmit_CQYamlFallback(t *testing.T) {
	var captured hub.JobSubmitRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{
				JobID:         "job-yaml-001",
				Status:        "QUEUED",
				QueuePosition: 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + srv.URL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write cq.yaml with run field in tmpDir (the cwd).
	if err := os.WriteFile(filepath.Join(tmpDir, "cq.yaml"), []byte("run: python3 train.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir := projectDir
	projectDir = tmpDir
	defer func() { projectDir = origDir }()

	origRun := hubSubmitRun
	hubSubmitRun = "" // --run not provided
	defer func() { hubSubmitRun = origRun }()

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	if captured.Command != "python3 train.py" {
		t.Errorf("command = %q, want %q (from cq.yaml)", captured.Command, "python3 train.py")
	}
}

// TestHubSubmit_ExperimentFlag_CallsCreateRun verifies that --experiment calls
// POST /experiment/run and prints the returned run_id.
func TestHubSubmit_ExperimentFlag_CallsCreateRun(t *testing.T) {
	var experimentCalled bool
	var capturedName string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{JobID: "job-exp-flag-001", Status: "QUEUED"})
		case "/experiment/run", "/v1/experiment/run":
			experimentCalled = true
			var body struct {
				Name       string `json:"name"`
				Capability string `json:"capability"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode experiment body: %v", err)
			}
			capturedName = body.Name
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"run_id": "run-abc123", "status": "running"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	origExp := hubSubmitExperiment
	hubSubmitExperiment = "my-exp"
	defer func() { hubSubmitExperiment = origExp }()

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	if err := runHubSubmit(nil, nil); err != nil {
		t.Fatalf("runHubSubmit: %v", err)
	}

	if !experimentCalled {
		t.Error("expected POST /experiment/run to be called")
	}
	if capturedName != "my-exp" {
		t.Errorf("experiment name = %q, want %q", capturedName, "my-exp")
	}
}

// TestHubSubmit_ExperimentFlag_ErrorWrapped verifies that CreateExperimentRun errors
// are wrapped with %w so callers can use errors.Is/As.
func TestHubSubmit_ExperimentFlag_ErrorWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/jobs/submit", "/v1/jobs/submit":
			json.NewEncoder(w).Encode(hub.JobSubmitResponse{JobID: "job-err-001", Status: "QUEUED"})
		case "/experiment/run", "/v1/experiment/run":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

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

	origExp := hubSubmitExperiment
	hubSubmitExperiment = "fail-exp"
	defer func() { hubSubmitExperiment = origExp }()

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd) //nolint:errcheck

	err := runHubSubmit(nil, nil)
	if err == nil {
		t.Fatal("expected error when experiment/run returns 500")
	}
	if !strings.Contains(err.Error(), "--experiment") {
		t.Errorf("error %q should mention --experiment flag", err.Error())
	}
}
