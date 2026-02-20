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

func TestDoctor_ExtractYAMLValue(t *testing.T) {
	content := `
hub:
  enabled: true
  url: "http://localhost:8585"
  api_key: secret
`
	if v := extractYAMLValue(content, "url:"); v != "http://localhost:8585" {
		t.Errorf("unexpected url: %q", v)
	}
	if v := extractYAMLValue(content, "api_key:"); v != "secret" {
		t.Errorf("unexpected api_key: %q", v)
	}
	if v := extractYAMLValue(content, "missing:"); v != "" {
		t.Errorf("expected empty for missing key, got %q", v)
	}
}
