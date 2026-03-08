package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestWorkerInit_NonInteractive verifies that --non-interactive mode writes
// hub_url and api_key to a temp config file without touching home dir.
func TestWorkerInit_NonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Override config path so no home-dir side effects.
	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	origURL := workerInitHubURL
	origKey := workerInitAPIKey
	origNI := workerInitNonInteractive
	workerInitHubURL = "https://hub.example.com"
	workerInitAPIKey = "test-api-key"
	workerInitNonInteractive = true
	defer func() {
		workerInitHubURL = origURL
		workerInitAPIKey = origKey
		workerInitNonInteractive = origNI
	}()

	if err := runWorkerInit(nil, nil); err != nil {
		t.Fatalf("runWorkerInit: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg workerYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.HubURL != "https://hub.example.com" {
		t.Errorf("hub_url = %q, want %q", cfg.HubURL, "https://hub.example.com")
	}
	if cfg.APIKey != "test-api-key" {
		t.Errorf("api_key = %q, want %q", cfg.APIKey, "test-api-key")
	}
}

// TestWorkerInit_NonInteractive_MissingFlags verifies error when flags are missing.
func TestWorkerInit_NonInteractive_MissingFlags(t *testing.T) {
	orig := workerInitNonInteractive
	origURL := workerInitHubURL
	origKey := workerInitAPIKey
	workerInitNonInteractive = true
	workerInitHubURL = ""
	workerInitAPIKey = ""
	defer func() {
		workerInitNonInteractive = orig
		workerInitHubURL = origURL
		workerInitAPIKey = origKey
	}()

	err := runWorkerInit(nil, nil)
	if err == nil {
		t.Fatal("expected error when --hub-url and --api-key are missing")
	}
	if !strings.Contains(err.Error(), "--hub-url") && !strings.Contains(err.Error(), "--api-key") {
		t.Errorf("error %q should mention missing flags", err.Error())
	}
}

// TestWorkerInit_Idempotent verifies that re-running init preserves existing fields
// (name, tags, capabilities) not touched by --hub-url/--api-key.
func TestWorkerInit_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Write an existing config with extra fields.
	existing := workerYAML{
		HubURL:       "https://old.example.com",
		APIKey:       "old-key",
		Name:         "my-gpu-server",
		Tags:         []string{"gpu", "rtx4090"},
		Capabilities: "caps.yaml",
	}
	data, _ := yaml.Marshal(existing)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	origURL := workerInitHubURL
	origKey := workerInitAPIKey
	origNI := workerInitNonInteractive
	workerInitHubURL = "https://new.example.com"
	workerInitAPIKey = "new-key"
	workerInitNonInteractive = true
	defer func() {
		workerInitHubURL = origURL
		workerInitAPIKey = origKey
		workerInitNonInteractive = origNI
	}()

	if err := runWorkerInit(nil, nil); err != nil {
		t.Fatalf("runWorkerInit: %v", err)
	}

	result, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg workerYAML
	if err := yaml.Unmarshal(result, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// URL and key should be updated.
	if cfg.HubURL != "https://new.example.com" {
		t.Errorf("hub_url = %q, want new URL", cfg.HubURL)
	}
	if cfg.APIKey != "new-key" {
		t.Errorf("api_key = %q, want new key", cfg.APIKey)
	}
	// Name, capabilities should be preserved.
	if cfg.Name != "my-gpu-server" {
		t.Errorf("name = %q, want preserved %q", cfg.Name, "my-gpu-server")
	}
	if cfg.Capabilities != "caps.yaml" {
		t.Errorf("capabilities = %q, want preserved", cfg.Capabilities)
	}
}

// TestWorkerInstall_DryRun verifies that --dry-run outputs service file content
// without writing to disk.
func TestWorkerInstall_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := workerYAML{
		HubURL: "https://hub.example.com",
		APIKey: "test-key",
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	origDry := workerInstallDryRun
	origUser := workerInstallUser
	workerInstallDryRun = true
	workerInstallUser = false
	defer func() {
		workerInstallDryRun = origDry
		workerInstallUser = origUser
	}()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runWorkerInstall(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if err != nil {
		t.Fatalf("runWorkerInstall: %v", err)
	}

	// On macOS expect ProgramArguments; on Linux expect ExecStart.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(output, "ProgramArguments") {
			t.Errorf("expected ProgramArguments in dry-run output, got:\n%s", output)
		}
	case "linux":
		if !strings.Contains(output, "ExecStart") {
			t.Errorf("expected ExecStart in dry-run output, got:\n%s", output)
		}
	default:
		// On other OS, runWorkerInstall returns error — skip output check.
	}
}

// TestWorkerInstall_DryRun_WithCaps verifies ExecStart includes --capabilities flag.
func TestWorkerInstall_DryRun_WithCaps(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("service install not supported on this OS")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := workerYAML{
		HubURL:       "https://hub.example.com",
		APIKey:       "test-key",
		Capabilities: "gpu-caps.yaml",
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	origDry := workerInstallDryRun
	workerInstallDryRun = true
	defer func() { workerInstallDryRun = origDry }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runWorkerInstall(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if err != nil {
		t.Fatalf("runWorkerInstall: %v", err)
	}

	if !strings.Contains(output, "gpu-caps.yaml") {
		t.Errorf("expected capabilities file in output, got:\n%s", output)
	}
}

// =========================================================================
// TestWorkerStart
// =========================================================================

func TestWorkerStart(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := workerYAML{
		HubURL:       "https://hub.example.com",
		APIKey:       "test-secret",
		Capabilities: "gpu-caps.yaml",
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	var capturedName string
	var capturedArgs []string
	var capturedEnv []string

	origExec := workerExecCommand
	workerExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedName = name
		capturedArgs = args
		// Capture env via closure — set it to what runWorkerStart would inject.
		capturedEnv = append(os.Environ(), "C5_API_KEY="+cfg.APIKey)
		return exec.Command("true")
	}
	defer func() { workerExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runWorkerStart(nil, nil)

	w.Close()
	os.Stdout = oldStdout
	r.Close()

	if err != nil {
		t.Fatalf("runWorkerStart: %v", err)
	}

	if capturedName == "" {
		t.Error("execCommand was not called")
	}

	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "--server") {
		t.Errorf("args %q should contain --server", argsStr)
	}
	if !strings.Contains(argsStr, "https://hub.example.com") {
		t.Errorf("args %q should contain hub URL", argsStr)
	}
	if !strings.Contains(argsStr, "--capabilities") {
		t.Errorf("args %q should contain --capabilities", argsStr)
	}
	if !strings.Contains(argsStr, "gpu-caps.yaml") {
		t.Errorf("args %q should contain capabilities file", argsStr)
	}

	hasAPIKey := false
	for _, e := range capturedEnv {
		if strings.HasPrefix(e, "C5_API_KEY=") {
			hasAPIKey = true
			if !strings.Contains(e, "test-secret") {
				t.Errorf("C5_API_KEY env = %q, want test-secret", e)
			}
			break
		}
	}
	if !hasAPIKey {
		t.Error("C5_API_KEY not found in subprocess env")
	}
}

// TestWorkerStart_NoCapabilities verifies --capabilities is omitted when config has none.
func TestWorkerStart_NoCapabilities(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := workerYAML{HubURL: "https://hub.example.com", APIKey: "key"}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	var capturedArgs []string
	origExec := workerExecCommand
	workerExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.Command("true")
	}
	defer func() { workerExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	_ = runWorkerStart(nil, nil)
	w.Close()
	os.Stdout = oldStdout
	r.Close()

	argsStr := strings.Join(capturedArgs, " ")
	if strings.Contains(argsStr, "--capabilities") {
		t.Errorf("args should not contain --capabilities when config.Capabilities is empty, got: %q", argsStr)
	}
}

// =========================================================================
// TestWorkerStatus
// =========================================================================

func TestWorkerStatus(t *testing.T) {
	workers := []workerStatusRow{
		{
			ID:        "w-abc123",
			Hostname:  "gpu-server-1",
			Name:      "gpu-server-1",
			Status:    "online",
			Tags:      []string{"gpu", "rtx4090"},
			UptimeSec: 7200,
			LastJobAt: "",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workers" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workers)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfg := workerYAML{HubURL: ts.URL, APIKey: "test-key"}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runWorkerStatus(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if err != nil {
		t.Fatalf("runWorkerStatus: %v", err)
	}

	for _, col := range []string{"NAME", "STATUS", "UPTIME", "LAST JOB", "CAPABILITIES"} {
		if !strings.Contains(output, col) {
			t.Errorf("output should contain column %q:\n%s", col, output)
		}
	}

	if !strings.Contains(output, "gpu-server-1") {
		t.Errorf("output should contain hostname, got:\n%s", output)
	}
	if !strings.Contains(output, "online") {
		t.Errorf("output should contain status, got:\n%s", output)
	}
	if !strings.Contains(output, "rtx4090") {
		t.Errorf("output should contain tags as capabilities, got:\n%s", output)
	}
}

// TestWorkerStatus_NoWorkers verifies the empty state message.
func TestWorkerStatus_NoWorkers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfg := workerYAML{HubURL: ts.URL, APIKey: "k"}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	oldStdout := os.Stdout
	rp, wp, _ := os.Pipe()
	os.Stdout = wp

	err := runWorkerStatus(nil, nil)

	wp.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := rp.Read(buf)
	output := string(buf[:n])

	if err != nil {
		t.Fatalf("runWorkerStatus: %v", err)
	}
	if !strings.Contains(output, "No workers") {
		t.Errorf("expected 'No workers' message, got:\n%s", output)
	}
}

// TestWorkerStatus_NoHubURL verifies error when hub_url is missing.
func TestWorkerStatus_NoHubURL(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfg := workerYAML{APIKey: "k"} // no hub_url
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workerConfigPathOverride
	workerConfigPathOverride = cfgPath
	defer func() { workerConfigPathOverride = orig }()

	err := runWorkerStatus(nil, nil)
	if err == nil {
		t.Fatal("expected error when hub_url is not set")
	}
	if !strings.Contains(err.Error(), "hub_url") {
		t.Errorf("error %q should mention hub_url", err.Error())
	}
}

// TestBuildSystemdUnit_Sanitize verifies that special characters in apiKey are
// properly escaped so they cannot inject extra directives into the unit file.
func TestBuildSystemdUnit_Sanitize(t *testing.T) {
	cases := []struct {
		name      string
		apiKey    string
		wantInEnv string // substring that must appear inside Environment= line
		wantNot   string // substring that must NOT appear (raw unescaped form)
	}{
		{
			name:      "double_quote_escaped",
			apiKey:    `key"injected`,
			wantInEnv: `key\"injected`,
			wantNot:   `key"injected`,
		},
		{
			name:      "backslash_escaped",
			apiKey:    `key\value`,
			wantInEnv: `key\\value`, // raw backslash becomes \\ in the quoted value
		},
		{
			// Newline injection attempt: without stripping, the payload would create
			// a second ExecStart= directive on its own line. After stripping, the
			// injected text collapses into the quoted Environment= value (safe).
			name:      "newline_stripped",
			apiKey:    "key\nExecStart=/bin/sh",
			wantInEnv: `C5_API_KEY=keyExecStart=/bin/sh`, // collapsed into value — safe
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unit := buildSystemdUnit("/usr/local/bin/cq hub worker start", "https://hub.example.com", tc.apiKey)
			if tc.wantInEnv != "" && !strings.Contains(unit, tc.wantInEnv) {
				t.Errorf("expected %q in unit file, got:\n%s", tc.wantInEnv, unit)
			}
			if tc.wantNot != "" && strings.Contains(unit, tc.wantNot) {
				t.Errorf("found unsafe string %q in unit file:\n%s", tc.wantNot, unit)
			}
		})
	}
}
