package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEdgeInit_NonInteractive verifies that --non-interactive mode writes
// hub_url and api_key to a temp config file without touching home dir.
func TestEdgeInit_NonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	origURL := edgeInitHubURL
	origKey := edgeInitAPIKey
	origNI := edgeInitNonInteractive
	edgeInitHubURL = "https://hub.example.com"
	edgeInitAPIKey = "edge-api-key"
	edgeInitNonInteractive = true
	defer func() {
		edgeInitHubURL = origURL
		edgeInitAPIKey = origKey
		edgeInitNonInteractive = origNI
	}()

	if err := runEdgeInit(nil, nil); err != nil {
		t.Fatalf("runEdgeInit: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg edgeYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.HubURL != "https://hub.example.com" {
		t.Errorf("hub_url = %q, want %q", cfg.HubURL, "https://hub.example.com")
	}
	if cfg.APIKey != "edge-api-key" {
		t.Errorf("api_key = %q, want %q", cfg.APIKey, "edge-api-key")
	}
}

// TestEdgeInit_NonInteractive_MissingFlags verifies error when flags are missing.
func TestEdgeInit_NonInteractive_MissingFlags(t *testing.T) {
	orig := edgeInitNonInteractive
	origURL := edgeInitHubURL
	origKey := edgeInitAPIKey
	edgeInitNonInteractive = true
	edgeInitHubURL = ""
	edgeInitAPIKey = ""
	defer func() {
		edgeInitNonInteractive = orig
		edgeInitHubURL = origURL
		edgeInitAPIKey = origKey
	}()

	err := runEdgeInit(nil, nil)
	if err == nil {
		t.Fatal("expected error when --hub-url and --api-key are missing")
	}
	if !strings.Contains(err.Error(), "--hub-url") && !strings.Contains(err.Error(), "--api-key") {
		t.Errorf("error %q should mention missing flags", err.Error())
	}
}

// TestEdgeInit_Idempotent verifies that re-running init preserves existing fields
// (edge_name, workdir, metrics_command, etc.) not touched by --hub-url/--api-key.
func TestEdgeInit_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	existing := edgeYAML{
		HubURL:          "https://old.example.com",
		APIKey:          "old-key",
		EdgeName:        "jetson-001",
		Workdir:         "/data/models",
		MetricsCommand:  "nvidia-smi",
		MetricsInterval: 10,
		DriveURL:        "https://drive.example.com",
		DriveAPIKey:     "drive-key",
	}
	data, _ := yaml.Marshal(existing)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	origURL := edgeInitHubURL
	origKey := edgeInitAPIKey
	origNI := edgeInitNonInteractive
	edgeInitHubURL = "https://new.example.com"
	edgeInitAPIKey = "new-key"
	edgeInitNonInteractive = true
	defer func() {
		edgeInitHubURL = origURL
		edgeInitAPIKey = origKey
		edgeInitNonInteractive = origNI
	}()

	if err := runEdgeInit(nil, nil); err != nil {
		t.Fatalf("runEdgeInit: %v", err)
	}

	result, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg edgeYAML
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
	// Other fields should be preserved.
	if cfg.EdgeName != "jetson-001" {
		t.Errorf("edge_name = %q, want preserved %q", cfg.EdgeName, "jetson-001")
	}
	if cfg.Workdir != "/data/models" {
		t.Errorf("workdir = %q, want preserved", cfg.Workdir)
	}
	if cfg.MetricsCommand != "nvidia-smi" {
		t.Errorf("metrics_command = %q, want preserved", cfg.MetricsCommand)
	}
	if cfg.MetricsInterval != 10 {
		t.Errorf("metrics_interval = %d, want 10", cfg.MetricsInterval)
	}
	if cfg.DriveURL != "https://drive.example.com" {
		t.Errorf("drive_url = %q, want preserved", cfg.DriveURL)
	}
}

// TestEdgeInstall_DryRun verifies that --dry-run outputs service file content
// without writing to disk.
func TestEdgeInstall_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	cfg := edgeYAML{
		HubURL: "https://hub.example.com",
		APIKey: "test-key",
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	origDry := edgeInstallDryRun
	origUser := edgeInstallUser
	edgeInstallDryRun = true
	edgeInstallUser = false
	defer func() {
		edgeInstallDryRun = origDry
		edgeInstallUser = origUser
	}()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEdgeInstall(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(r)
	output := string(outBytes)

	if err != nil {
		t.Fatalf("runEdgeInstall: %v", err)
	}

	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(output, "ProgramArguments") {
			t.Errorf("expected ProgramArguments in dry-run output, got:\n%s", output)
		}
		if !strings.Contains(output, "C5_API_KEY") {
			t.Errorf("expected C5_API_KEY in launchd plist dry-run output, got:\n%s", output)
		}
	case "linux":
		if !strings.Contains(output, "ExecStart") {
			t.Errorf("expected ExecStart in dry-run output, got:\n%s", output)
		}
		if !strings.Contains(output, "C5_API_KEY") {
			t.Errorf("expected C5_API_KEY in systemd unit dry-run output, got:\n%s", output)
		}
	default:
		// On other OS, runEdgeInstall returns error — skip output check.
	}
}

// TestEdgeInstall_DryRun_WithMetrics verifies ExecStart includes metrics flags.
func TestEdgeInstall_DryRun_WithMetrics(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("service install not supported on this OS")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	cfg := edgeYAML{
		HubURL:          "https://hub.example.com",
		APIKey:          "test-key",
		EdgeName:        "jetson-001",
		MetricsCommand:  "nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader",
		MetricsInterval: 30,
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	origDry := edgeInstallDryRun
	edgeInstallDryRun = true
	defer func() { edgeInstallDryRun = origDry }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEdgeInstall(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(r)
	output := string(outBytes)

	if err != nil {
		t.Fatalf("runEdgeInstall: %v", err)
	}

	if !strings.Contains(output, "edge-agent") {
		t.Errorf("expected edge-agent in output, got:\n%s", output)
	}
	if !strings.Contains(output, "--edge-name") {
		t.Errorf("expected --edge-name in output, got:\n%s", output)
	}
	if !strings.Contains(output, "jetson-001") {
		t.Errorf("expected edge name value in output, got:\n%s", output)
	}
}

// TestEdgeStart_SpawnArgs verifies that runEdgeStart passes the right flags
// to the c5 edge-agent subprocess.
func TestEdgeStart_SpawnArgs(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	cfg := edgeYAML{
		HubURL:          "https://hub.example.com",
		APIKey:          "test-secret",
		EdgeName:        "jetson-001",
		MetricsCommand:  "nvidia-smi",
		MetricsInterval: 15,
		DriveURL:        "https://drive.example.com",
		DriveAPIKey:     "drive-key",
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	var capturedName string
	var capturedArgs []string
	var capturedCmd *exec.Cmd

	origExec := edgeExecCommand
	edgeExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedName = name
		capturedArgs = args
		capturedCmd = exec.Command("true")
		return capturedCmd
	}
	defer func() { edgeExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEdgeStart(nil, nil)

	w.Close()
	os.Stdout = oldStdout
	r.Close()

	if err != nil {
		t.Fatalf("runEdgeStart: %v", err)
	}

	if capturedName == "" {
		t.Error("execCommand was not called")
	}

	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "edge-agent") {
		t.Errorf("args %q should contain edge-agent", argsStr)
	}
	if !strings.Contains(argsStr, "--hub-url") {
		t.Errorf("args %q should contain --hub-url", argsStr)
	}
	if !strings.Contains(argsStr, "https://hub.example.com") {
		t.Errorf("args %q should contain hub URL", argsStr)
	}
	if !strings.Contains(argsStr, "--edge-name") {
		t.Errorf("args %q should contain --edge-name", argsStr)
	}
	if !strings.Contains(argsStr, "jetson-001") {
		t.Errorf("args %q should contain edge name", argsStr)
	}
	if !strings.Contains(argsStr, "--metrics-command") {
		t.Errorf("args %q should contain --metrics-command", argsStr)
	}
	if !strings.Contains(argsStr, "--metrics-interval") {
		t.Errorf("args %q should contain --metrics-interval", argsStr)
	}
	if !strings.Contains(argsStr, "--drive-url") {
		t.Errorf("args %q should contain --drive-url", argsStr)
	}
	// DriveAPIKey must NOT appear in args (passed via env instead).
	if strings.Contains(argsStr, "--drive-api-key") {
		t.Errorf("args %q must NOT contain --drive-api-key (should be in env)", argsStr)
	}
	// AllowExec=false must NOT produce --allow-exec flag.
	if strings.Contains(argsStr, "--allow-exec") {
		t.Errorf("args %q must NOT contain --allow-exec when AllowExec=false", argsStr)
	}

	// Verify actual Cmd.Env set by runEdgeStart.
	env := capturedCmd.Env
	hasAPIKey := false
	hasDriveKey := false
	for _, e := range env {
		if strings.HasPrefix(e, "C5_API_KEY=") {
			hasAPIKey = true
			if !strings.Contains(e, "test-secret") {
				t.Errorf("C5_API_KEY env = %q, want test-secret", e)
			}
		}
		if strings.HasPrefix(e, "C5_DRIVE_API_KEY=") {
			hasDriveKey = true
			if !strings.Contains(e, "drive-key") {
				t.Errorf("C5_DRIVE_API_KEY env = %q, want drive-key", e)
			}
		}
	}
	if !hasAPIKey {
		t.Error("C5_API_KEY not found in subprocess env")
	}
	if !hasDriveKey {
		t.Error("C5_DRIVE_API_KEY not found in subprocess env")
	}
}

// TestEdgeStart_AllowExecFlags verifies that AllowExec and
// AllowedArtifactURLPrefixes are propagated as CLI flags.
func TestEdgeStart_AllowExecFlags(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	cfg := edgeYAML{
		HubURL:                     "https://hub.example.com",
		APIKey:                     "test-secret",
		AllowExec:                  true,
		AllowedArtifactURLPrefixes: []string{"https://cdn.example.com", "https://storage.example.org"},
	}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	var capturedArgs []string
	origExec := edgeExecCommand
	edgeExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.Command("true")
	}
	defer func() { edgeExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := runEdgeStart(nil, nil)
	w.Close()
	os.Stdout = oldStdout
	r.Close()

	if err != nil {
		t.Fatalf("runEdgeStart: %v", err)
	}

	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "--allow-exec") {
		t.Errorf("args %q should contain --allow-exec", argsStr)
	}
	if !strings.Contains(argsStr, "https://cdn.example.com") {
		t.Errorf("args %q should contain first allowed prefix", argsStr)
	}
	if !strings.Contains(argsStr, "https://storage.example.org") {
		t.Errorf("args %q should contain second allowed prefix", argsStr)
	}
}

// TestEdgeStart_MinimalConfig verifies that optional flags are omitted
// when config has only hub_url and api_key.
func TestEdgeStart_MinimalConfig(t *testing.T) {
	clearBuiltinURLs(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	cfg := edgeYAML{HubURL: "https://hub.example.com", APIKey: "key"}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	var capturedArgs []string
	origExec := edgeExecCommand
	edgeExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.Command("true")
	}
	defer func() { edgeExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	_ = runEdgeStart(nil, nil)
	w.Close()
	os.Stdout = oldStdout
	r.Close()

	argsStr := strings.Join(capturedArgs, " ")
	if strings.Contains(argsStr, "--edge-name") {
		t.Errorf("args %q should NOT contain --edge-name when not configured", argsStr)
	}
	if strings.Contains(argsStr, "--metrics-command") {
		t.Errorf("args %q should NOT contain --metrics-command when not configured", argsStr)
	}
	if strings.Contains(argsStr, "--drive-url") {
		t.Errorf("args %q should NOT contain --drive-url when not configured", argsStr)
	}
}

// TestEdgeStart_JWTFallback verifies that an existing config with no APIKey
// picks up the cloud session JWT automatically (mirrors worker behaviour).
func TestEdgeStart_JWTFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	// Config has hub_url but no api_key.
	cfg := edgeYAML{HubURL: "https://hub.example.com"}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Write a fake session.json so loadCloudSessionJWT returns a token.
	homeDir := t.TempDir()
	sessionDir := filepath.Join(homeDir, ".c4")
	_ = os.MkdirAll(sessionDir, 0o700)
	sessionJSON := `{"access_token":"fake.jwt.token"}`
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), []byte(sessionJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	// Override HOME so loadCloudSessionJWT reads our fake session.
	t.Setenv("HOME", homeDir)

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	var capturedCmd *exec.Cmd
	origExec := edgeExecCommand
	edgeExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedCmd = exec.Command("true")
		return capturedCmd
	}
	defer func() { edgeExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := runEdgeStart(nil, nil)
	w.Close()
	os.Stdout = oldStdout
	r.Close()

	if err != nil {
		t.Fatalf("runEdgeStart with JWT fallback: %v", err)
	}

	// JWT should appear as C5_API_KEY in subprocess env.
	hasJWT := false
	for _, e := range capturedCmd.Env {
		if e == "C5_API_KEY=fake.jwt.token" {
			hasJWT = true
			break
		}
	}
	if !hasJWT {
		t.Error("C5_API_KEY with JWT not found in subprocess env")
	}
}

// TestEdgeStart_EnvAutoInit verifies that missing config is auto-initialized
// from C5_HUB_URL and C5_API_KEY env vars.
func TestEdgeStart_EnvAutoInit(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "edge.yaml")

	orig := edgeConfigPathOverride
	edgeConfigPathOverride = cfgPath
	defer func() { edgeConfigPathOverride = orig }()

	t.Setenv("C5_HUB_URL", "https://env-hub.example.com")
	t.Setenv("C5_API_KEY", "env-api-key")

	var capturedArgs []string
	origExec := edgeExecCommand
	edgeExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.Command("true")
	}
	defer func() { edgeExecCommand = origExec }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := runEdgeStart(nil, nil)
	w.Close()
	os.Stdout = oldStdout
	r.Close()

	if err != nil {
		t.Fatalf("runEdgeStart with env auto-init: %v", err)
	}

	// Config file should now exist.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file should have been created by auto-init")
	}

	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "https://env-hub.example.com") {
		t.Errorf("args %q should contain env hub URL", argsStr)
	}
}
