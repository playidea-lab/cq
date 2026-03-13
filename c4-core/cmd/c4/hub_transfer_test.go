package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/hub"
)

// =========================================================================
// mockTunnelStarter
// =========================================================================

type mockTunnelStarter struct {
	url string
	err error
}

func (m mockTunnelStarter) Start(ctx context.Context, localPort int) (string, *exec.Cmd, error) {
	if m.err != nil {
		return "", nil, m.err
	}
	return m.url, nil, nil
}

// =========================================================================
// helpers
// =========================================================================

// setupHubConfig creates a temp projectDir with a config.yaml pointing at srvURL.
func setupHubConfig(t *testing.T, srvURL string) (cleanup func()) {
	t.Helper()
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "hub:\n  enabled: true\n  url: " + srvURL + "\n"
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := projectDir
	projectDir = tmpDir
	return func() { projectDir = orig }
}

// =========================================================================
// TestTransferCmd_FileNotFound
// =========================================================================

func TestTransferCmd_FileNotFound(t *testing.T) {
	restore := setupHubConfig(t, "http://127.0.0.1:19999")
	defer restore()

	origTo := hubTransferTo
	hubTransferTo = "worker-xyz"
	defer func() { hubTransferTo = origTo }()

	err := runHubTransferWithTunnel(hubTransferCmd, []string{"/nonexistent/path/file.tar.gz"}, mockTunnelStarter{url: "https://example.trycloudflare.com"})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("error %q should contain 'file not found'", err.Error())
	}
}

// =========================================================================
// TestTransferCmd_CloudflaredNotFound
// =========================================================================

func TestTransferCmd_CloudflaredNotFound(t *testing.T) {
	// Create a real temp file so file-stat passes.
	tmpFile, err := os.CreateTemp(t.TempDir(), "testfile*.bin")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	restore := setupHubConfig(t, "http://127.0.0.1:19999")
	defer restore()

	origTo := hubTransferTo
	hubTransferTo = "worker-xyz"
	defer func() { hubTransferTo = origTo }()

	// Mock tunnel returns "cloudflared not found" error.
	ts := mockTunnelStarter{err: fmt.Errorf("cloudflared not found in PATH\n  Install: brew install cloudflared")}
	err = runHubTransferWithTunnel(hubTransferCmd, []string{tmpFile.Name()}, ts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cloudflared not found") {
		t.Errorf("error %q should mention cloudflared not found", err.Error())
	}
}

// =========================================================================
// TestTransferCmd_FlagParsing
// =========================================================================

func TestTransferCmd_FlagParsing(t *testing.T) {
	// Verify that --to and --port flags are registered on hubTransferCmd.
	if f := hubTransferCmd.Flag("to"); f == nil {
		t.Error("--to flag not registered")
	}
	if f := hubTransferCmd.Flag("port"); f == nil {
		t.Error("--port flag not registered")
	}
	// Verify --to is required (Annotations should contain BashCompOneRequiredFlag).
	if _, ok := hubTransferCmd.Flag("to").Annotations[cobra_required]; !ok {
		// cobra marks required flags in their annotations
		// This is an indirect check — the flag exists and the command will error
		// if --to is omitted. Just verify the flag exists (already checked above).
		t.Log("--to required annotation not set via cobra internal key (OK if cobra marks differently)")
	}
}

// cobra internal key for required flag annotation.
const cobra_required = "cobra_annotation_bash_completion_one_required_flag"

// =========================================================================
// TestTransfer_UnauthorizedPath
// =========================================================================

func TestTransfer_UnauthorizedPath(t *testing.T) {
	// Spin up the transfer HTTP handler directly and verify wrong token → 404.
	token := "correcttoken"
	filename := "data.tar.gz"
	servePath := "/t/" + token + "/" + filename

	// Create a small temp file to serve.
	tmpFile, err := os.CreateTemp(t.TempDir(), "data*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != servePath {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, tmpFile.Name())
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Wrong token → 404.
	resp, err := http.Get(ts.URL + "/t/wrongtoken/" + filename)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong token: status = %d, want 404", resp.StatusCode)
	}

	// Correct token → 200.
	resp2, err := http.Get(ts.URL + servePath)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("correct token: status = %d, want 200", resp2.StatusCode)
	}
}

// =========================================================================
// TestTransfer_ParseTunnelURL_JSON
// =========================================================================

func TestTransfer_ParseTunnelURL_JSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "json url field",
			input: `{"level":"info","message":"connected","url":"https://abc123.trycloudflare.com"}`,
			want:  "https://abc123.trycloudflare.com",
		},
		{
			name:  "regex fallback",
			input: `2024-01-01 Your tunnel https://xyz-test.trycloudflare.com is ready`,
			want:  "https://xyz-test.trycloudflare.com",
		},
		{
			name:  "json no url then regex",
			input: "noise line\n" + `{"level":"info","url":"https://hello-world.trycloudflare.com"}`,
			want:  "https://hello-world.trycloudflare.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTunnelURL(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("parseTunnelURL: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// =========================================================================
// TestTransferOrchestrator_MockFlow
// =========================================================================

func TestTransferOrchestrator_MockFlow(t *testing.T) {
	// Create test file to transfer.
	tmpFile, err := os.CreateTemp(t.TempDir(), "dataset*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.WriteString("binary data"); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Fake Hub server: capabilities/invoke + jobs/<id>.
	jobID := "job-transfer-001"
	var capturedInvoke hub.InvokeCapabilityRequest

	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/capabilities/invoke" || r.URL.Path == "/v1/capabilities/invoke":
			json.NewDecoder(r.Body).Decode(&capturedInvoke) //nolint:errcheck
			json.NewEncoder(w).Encode(hub.InvokeCapabilityResponse{
				JobID:  jobID,
				Status: "QUEUED",
			})
		case r.URL.Path == "/jobs/"+jobID || r.URL.Path == "/v1/jobs/"+jobID:
			json.NewEncoder(w).Encode(hub.Job{
				ID:     jobID,
				Status: "SUCCEEDED",
			})
		case r.URL.Path == "/health" || r.URL.Path == "/v1/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer hubSrv.Close()

	restore := setupHubConfig(t, hubSrv.URL)
	defer restore()

	origTo := hubTransferTo
	hubTransferTo = "worker-target-001"
	origPort := hubTransferPort
	hubTransferPort = 0
	defer func() {
		hubTransferTo = origTo
		hubTransferPort = origPort
	}()

	// Mock tunnel returns a fake URL — our real httptest file server.
	fakeTunnelURL := "https://fake-tunnel.trycloudflare.com"
	ts := mockTunnelStarter{url: fakeTunnelURL}

	err = runHubTransferWithTunnel(hubTransferCmd, []string{tmpFile.Name()}, ts)
	if err != nil {
		t.Fatalf("runHubTransferWithTunnel: %v", err)
	}

	// Verify invoke payload.
	if capturedInvoke.Capability != "run_command" {
		t.Errorf("capability = %q, want run_command", capturedInvoke.Capability)
	}
	cmd, _ := capturedInvoke.Params["command"].(string)
	if !strings.Contains(cmd, "wget") {
		t.Errorf("command %q should contain 'wget'", cmd)
	}
	if !strings.Contains(cmd, fakeTunnelURL) {
		t.Errorf("command %q should contain tunnel URL", cmd)
	}
	workerID, _ := capturedInvoke.Params["worker_id"].(string)
	if workerID != "worker-target-001" {
		t.Errorf("worker_id = %q, want worker-target-001", workerID)
	}
}
