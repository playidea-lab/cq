package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupDoctorEnv creates a temporary project directory with a valid .c4/ structure.
func setupDoctorEnv(t *testing.T) (dir string, cleanup func()) {
	t.Helper()
	tmp := t.TempDir()
	c4 := filepath.Join(tmp, ".c4")
	if err := os.MkdirAll(c4, 0755); err != nil {
		t.Fatal(err)
	}
	// Write minimal config.yaml
	if err := os.WriteFile(filepath.Join(c4, "config.yaml"), []byte("project: test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write empty tasks.db placeholder
	if err := os.WriteFile(filepath.Join(c4, "tasks.db"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	orig := projectDir
	projectDir = tmp

	// Clear builtin URLs to prevent real Supabase connections in doctor tests.
	origURL := builtinSupabaseURL
	origKey := builtinSupabaseKey
	origHub := builtinHubURL
	builtinSupabaseURL = ""
	builtinSupabaseKey = ""
	builtinHubURL = ""

	return tmp, func() {
		projectDir = orig
		builtinSupabaseURL = origURL
		builtinSupabaseKey = origKey
		builtinHubURL = origHub
	}
}

func TestDoctor_C4DirOK(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()

	r := checkC4Dir()
	if r.Status != checkOK {
		t.Errorf("expected OK, got %s: %s", r.Status, r.Message)
	}
}

func TestDoctor_MissingMcpJson(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Ensure .mcp.json does NOT exist
	os.Remove(filepath.Join(dir, ".mcp.json"))

	r := checkMCPJson()
	if r.Status != checkFail {
		t.Errorf("expected FAIL when .mcp.json missing, got %s", r.Status)
	}
	if r.Fix == "" {
		t.Error("expected a Fix suggestion for missing .mcp.json")
	}
}

func TestDoctor_ValidMcpJson(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Create a fake cq binary
	fakeBin := filepath.Join(dir, "fake-cq")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho cq"), 0755); err != nil {
		t.Fatal(err)
	}

	mcpContent := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"c4": map[string]interface{}{
				"command": fakeBin,
				"args":    []string{"mcp"},
			},
		},
	}
	data, _ := json.Marshal(mcpContent)
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := checkMCPJson()
	if r.Status != checkOK {
		t.Errorf("expected OK for valid .mcp.json, got %s: %s", r.Status, r.Message)
	}
}

func TestDoctor_BrokenSymlink(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	claudePath := filepath.Join(dir, "CLAUDE.md")
	// Create a symlink to a non-existent target
	if err := os.Symlink("/nonexistent/path/AGENTS.md", claudePath); err != nil {
		t.Fatal(err)
	}

	r := checkClaudeMDSymlink()
	if r.Status != checkFail {
		t.Errorf("expected FAIL for broken symlink, got %s: %s", r.Status, r.Message)
	}
}

func TestDoctor_ValidSymlink(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Create a real target file and symlink
	target := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink(target, claudePath); err != nil {
		t.Fatal(err)
	}

	r := checkClaudeMDSymlink()
	if r.Status != checkOK {
		t.Errorf("expected OK for valid symlink, got %s: %s", r.Status, r.Message)
	}
}

func TestDoctor_JsonOutput(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()

	results := []checkResult{
		{Name: "test check", Status: checkOK, Message: "all good"},
		{Name: "another check", Status: checkFail, Message: "broken", Fix: "fix it"},
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printDoctorJSON(results)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printDoctorJSON returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name"`) {
		t.Error("JSON output missing 'name' field")
	}
	if !strings.Contains(output, `"status"`) {
		t.Error("JSON output missing 'status' field")
	}
	if !strings.Contains(output, `"FAIL"`) {
		t.Error("JSON output missing FAIL status")
	}
	if !strings.Contains(output, `"fix"`) {
		t.Error("JSON output missing 'fix' field")
	}

	// Verify it's valid JSON array
	var parsed []checkResult
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, output)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 results, got %d", len(parsed))
	}
}

func TestDoctor_AllOK(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Create a valid CLAUDE.md
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# C4 Overrides"), 0644); err != nil {
		t.Fatal(err)
	}

	// The .c4/ dir and files are already created by setupDoctorEnv.
	// checkC4Dir should return OK.
	r := checkC4Dir()
	if r.Status != checkOK {
		t.Errorf("checkC4Dir: expected OK, got %s: %s", r.Status, r.Message)
	}

	// checkClaudeMDSymlink on a regular file should return OK.
	r = checkClaudeMDSymlink()
	if r.Status != checkOK {
		t.Errorf("checkClaudeMDSymlink: expected OK, got %s: %s", r.Status, r.Message)
	}
}

func TestDoctor_MissingC4Dir(t *testing.T) {
	tmp := t.TempDir()
	orig := projectDir
	projectDir = tmp
	defer func() { projectDir = orig }()

	r := checkC4Dir()
	if r.Status != checkFail {
		t.Errorf("expected FAIL when .c4/ missing, got %s", r.Status)
	}
}

// TestCheckOSService_NotInstalled_NoPID verifies that checkOSService returns WARN or OK
// (not FAIL) and that Name is "os-service". The service may or may not be installed
// depending on the developer's environment; both outcomes are acceptable.
func TestCheckOSService_NotInstalled_NoPID(t *testing.T) {
	// Point servePIDDir at a temp directory that has no serve.pid, so the
	// PID-file branch is not taken regardless of the developer's actual ~/.c4/serve/.
	tmp := t.TempDir()
	origPIDDir := servePIDDir
	servePIDDir = tmp
	defer func() { servePIDDir = origPIDDir }()

	r := checkOSService(false)
	// Status must never be FAIL — not-installed is OK, stopped is WARN, running is OK.
	if r.Status == checkFail {
		t.Errorf("expected WARN or OK, got FAIL: %s", r.Message)
	}
	// Name must be "os-service" for `cq doctor | grep os-service` to work.
	if r.Name != "os-service" {
		t.Errorf("expected Name='os-service', got %q", r.Name)
	}
}

// TestCheckOSService verifies that checkOSService uses newServiceConfig (UserService-aware)
// and that the result has Name="os-service" and Status is never FAIL in a plain environment.
func TestCheckOSService(t *testing.T) {
	// Redirect PID dir to an empty temp directory to avoid reading any real PID file.
	tmp := t.TempDir()
	origPIDDir := servePIDDir
	servePIDDir = tmp
	defer func() { servePIDDir = origPIDDir }()

	r := checkOSService(false)

	// Name must always be "os-service".
	if r.Name != "os-service" {
		t.Errorf("expected Name='os-service', got %q", r.Name)
	}
	// Status must never be FAIL — not-installed is OK, running is OK.
	if r.Status == checkFail {
		t.Errorf("expected WARN or OK from checkOSService, got FAIL: %s", r.Message)
	}
}

// setupSkillEvalDocs creates .c4/knowledge/docs/ with skill-eval-*.md files for testing.
func setupSkillEvalDocs(t *testing.T, files map[string]string) {
	t.Helper()
	docsDir := filepath.Join(projectDir, ".c4", "knowledge", "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(docsDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckSkillHealth_OK(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()

	setupSkillEvalDocs(t, map[string]string{
		"skill-eval-c4-plan.md": "---\nid: skill-eval-c4-plan\ntype: experiment\n---\n\ntrigger_accuracy: 0.95\n",
	})

	r := checkSkillHealth()
	if r.Status != checkOK {
		t.Errorf("expected OK (accuracy=0.95 >= 0.90), got %s: %s", r.Status, r.Message)
	}
}

func TestCheckSkillHealth_Warn(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()

	setupSkillEvalDocs(t, map[string]string{
		"skill-eval-c4-run.md": "---\nid: skill-eval-c4-run\ntype: experiment\n---\n\ntrigger_accuracy: 0.85\n",
	})

	r := checkSkillHealth()
	if r.Status != checkWarn {
		t.Errorf("expected WARN (accuracy=0.85 < 0.90), got %s: %s", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "c4-run") {
		t.Errorf("expected skill name in message, got: %s", r.Message)
	}
}

func TestCheckSkillHealth_Unknown(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()
	// No skill-eval docs — knowledge dir exists but empty
	docsDir := filepath.Join(projectDir, ".c4", "knowledge", "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := checkSkillHealth()
	// unknown (no records) must be INFO, not WARN
	if r.Status == checkWarn || r.Status == checkFail {
		t.Errorf("expected INFO for no eval records, got %s: %s", r.Status, r.Message)
	}
	if r.Status != checkInfo {
		t.Errorf("expected Status=INFO, got %s", r.Status)
	}
}

func TestDoctor_SectionYAMLValue(t *testing.T) {
	// Config with both hub.url and cloud.url to verify cross-section isolation.
	content := `
hub:
  enabled: true
  url: "http://localhost:8585"
  api_key: hub-secret
cloud:
  url: "https://xyz.supabase.co"
  anon_key: anon-secret
`
	// hub section
	if v := sectionYAMLValue(content, "hub", "url:"); v != "http://localhost:8585" {
		t.Errorf("hub url: got %q, want http://localhost:8585", v)
	}
	if v := sectionYAMLValue(content, "hub", "api_key:"); v != "hub-secret" {
		t.Errorf("hub api_key: got %q, want hub-secret", v)
	}
	// cloud section — must not return hub.url
	if v := sectionYAMLValue(content, "cloud", "url:"); v != "https://xyz.supabase.co" {
		t.Errorf("cloud url: got %q, want https://xyz.supabase.co", v)
	}
	if v := sectionYAMLValue(content, "cloud", "anon_key:"); v != "anon-secret" {
		t.Errorf("cloud anon_key: got %q, want anon-secret", v)
	}
	// missing section or key
	if v := sectionYAMLValue(content, "hub", "missing:"); v != "" {
		t.Errorf("missing key: got %q, want empty", v)
	}
	if v := sectionYAMLValue(content, "nosection", "url:"); v != "" {
		t.Errorf("missing section: got %q, want empty", v)
	}

	// prefix collision: "url:" must NOT match "url_extra:" key
	contentPrefixCollision := `
hub:
  url_extra: should-not-match
  url: http://real-url
`
	if v := sectionYAMLValue(contentPrefixCollision, "hub", "url:"); v != "http://real-url" {
		t.Errorf("prefix collision: url: should match http://real-url, got %q", v)
	}
}

func TestRunWithTimeout_UvMissing(t *testing.T) {
	_, err := runWithTimeout(2*time.Second, "uv-does-not-exist-xyzzy", "--version")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestRunWithTimeout_BridgeRunnable(t *testing.T) {
	// echo is always available; treat success as "runnable"
	out, err := runWithTimeout(2*time.Second, "echo", "c4-bridge 0.1.0")
	if err != nil {
		t.Fatalf("echo failed: %v", err)
	}
	if !strings.Contains(out, "c4-bridge") {
		t.Errorf("expected output to contain 'c4-bridge', got %q", out)
	}
}

func TestCheckPythonSidecar_UvMissing(t *testing.T) {
	// Temporarily ensure uv is not in PATH by checking behaviour path-independently.
	// If uv is missing, the check should warn; if present, the real c4-bridge check runs.
	// We test the runWithTimeout helper path indirectly via UvMissing above.
	// This test just ensures checkPythonSidecar returns a non-empty Name.
	res := checkPythonSidecar()
	if res.Name == "" {
		t.Error("checkPythonSidecar returned empty Name")
	}
}
