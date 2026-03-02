package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	return tmp, func() { projectDir = orig }
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
