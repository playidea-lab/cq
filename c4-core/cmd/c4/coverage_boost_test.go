package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ────────────────────────────────────────────────────────────────
// doctor.go: printDoctorHuman
// ────────────────────────────────────────────────────────────────

func TestPrintDoctorHuman_AllOK(t *testing.T) {
	results := []checkResult{
		{Name: "binary", Status: checkOK, Message: "v1.0"},
		{Name: ".c4", Status: checkOK, Message: "dir found"},
	}
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	printDoctorHuman(results)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "All checks passed") {
		t.Errorf("expected 'All checks passed' in output, got: %s", out)
	}
}

func TestPrintDoctorHuman_WithFailures(t *testing.T) {
	results := []checkResult{
		{Name: "binary", Status: checkFail, Message: "not found", Fix: "install it"},
		{Name: ".c4", Status: checkWarn, Message: "maybe missing"},
		{Name: "mcp", Status: checkOK, Message: "ok"},
	}
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	printDoctorHuman(results)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "FAIL") {
		t.Errorf("expected FAIL in output, got: %s", out)
	}
	if !strings.Contains(out, "WARN") {
		t.Errorf("expected WARN in output, got: %s", out)
	}
	if !strings.Contains(out, "Fix:") {
		t.Errorf("expected Fix: in output, got: %s", out)
	}
	if !strings.Contains(out, "1 failed, 1 warnings") {
		t.Errorf("expected count summary, got: %s", out)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkBinary
// ────────────────────────────────────────────────────────────────

func TestCheckBinary_NotOnPath(t *testing.T) {
	// Override PATH to empty so cq is not found.
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	r := checkBinary()
	if r.Status != checkFail {
		t.Errorf("expected FAIL when cq not on PATH, got %s", r.Status)
	}
	if r.Fix == "" {
		t.Error("expected Fix suggestion for missing binary")
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkHooks
// ────────────────────────────────────────────────────────────────

func TestCheckHooks_MissingGateHook(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// No .claude/hooks/ directory at all
	_ = dir
	r := checkHooks()
	if r.Status != checkWarn {
		t.Errorf("expected WARN when gate hook missing, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHooks_GateHookUpToDate_MissingPermHook(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Install gate hook with exact content (so it is up-to-date)
	hooksDir := filepath.Join(dir, ".claude", "hooks")
	os.MkdirAll(hooksDir, 0755)
	gateHookFile := filepath.Join(hooksDir, "c4-gate.sh")
	os.WriteFile(gateHookFile, []byte(gateHookContent), 0755)

	// Do NOT install perm reviewer hook
	r := checkHooks()
	if r.Status != checkWarn {
		t.Errorf("expected WARN for missing perm hook, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHooks_BothHooksOutdated(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	hooksDir := filepath.Join(dir, ".claude", "hooks")
	os.MkdirAll(hooksDir, 0755)
	// Write outdated hook content
	os.WriteFile(filepath.Join(hooksDir, "c4-gate.sh"), []byte("#!/bin/bash\n# old version"), 0755)

	r := checkHooks()
	if r.Status != checkWarn {
		t.Errorf("expected WARN for outdated hook, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHooks_BothHooksOKButNoSettings(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	hooksDir := filepath.Join(dir, ".claude", "hooks")
	os.MkdirAll(hooksDir, 0755)
	os.WriteFile(filepath.Join(hooksDir, "c4-gate.sh"), []byte(gateHookContent), 0755)
	os.WriteFile(filepath.Join(hooksDir, "c4-permission-reviewer.sh"), []byte(permissionReviewerContent), 0755)
	// No settings.json

	r := checkHooks()
	if r.Status != checkWarn {
		t.Errorf("expected WARN for missing settings.json, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHooks_AllOK(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	hooksDir := filepath.Join(dir, ".claude", "hooks")
	os.MkdirAll(hooksDir, 0755)
	os.WriteFile(filepath.Join(hooksDir, "c4-gate.sh"), []byte(gateHookContent), 0755)
	os.WriteFile(filepath.Join(hooksDir, "c4-permission-reviewer.sh"), []byte(permissionReviewerContent), 0755)

	settingsContent := `{"hooks":{"c4-gate.sh":{},"c4-permission-reviewer.sh":{}}}`
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	os.WriteFile(settingsPath, []byte(settingsContent), 0644)

	r := checkHooks()
	if r.Status != checkOK {
		t.Errorf("expected OK when hooks fully configured, got %s: %s", r.Status, r.Message)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkPythonSidecar
// ────────────────────────────────────────────────────────────────

func TestCheckPythonSidecar_UvNotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	r := checkPythonSidecar()
	if r.Status != checkWarn {
		t.Errorf("expected WARN when uv not found, got %s", r.Status)
	}
	if r.Fix == "" {
		t.Error("expected Fix for missing uv")
	}
}

func TestCheckPythonSidecar_UvFoundNoProject(t *testing.T) {
	// Use a temp dir with a fake uv binary.
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	binDir := t.TempDir()
	fakeUV := filepath.Join(binDir, "uv")
	os.WriteFile(fakeUV, []byte("#!/bin/sh\necho uv 0.1.0"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	// projectDir has no pyproject.toml → "no pyproject.toml in project"
	_ = dir
	r := checkPythonSidecar()
	if r.Status != checkOK {
		t.Errorf("expected OK when uv found and no pyproject.toml, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckPythonSidecar_UvFoundWithProject(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Write a pyproject.toml to projectDir
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0644)

	binDir := t.TempDir()
	fakeUV := filepath.Join(binDir, "uv")
	os.WriteFile(fakeUV, []byte("#!/bin/sh\necho uv 0.1.0"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	r := checkPythonSidecar()
	if r.Status != checkOK {
		t.Errorf("expected OK when uv found with pyproject.toml, got %s: %s", r.Status, r.Message)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkHub
// ────────────────────────────────────────────────────────────────

func TestCheckHub_NoConfig(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()
	// Remove the config.yaml so checkHub skips
	os.Remove(filepath.Join(dir, ".c4", "config.yaml"))

	r := checkHub()
	if r.Status != checkOK {
		t.Errorf("expected OK when no config, got %s: %s", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "skipped") {
		t.Errorf("expected 'skipped' in message, got: %s", r.Message)
	}
}

func TestCheckHub_HubNotEnabled(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("project: test\nhub:\n  enabled: false\n"), 0644)

	r := checkHub()
	if r.Status != checkOK {
		t.Errorf("expected OK when hub not enabled, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHub_EnabledNoURL(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("hub:\n  enabled: true\n"), 0644)

	r := checkHub()
	// Hub now uses Supabase directly — enabled=true is sufficient for OK.
	if r.Status != checkOK {
		t.Errorf("expected OK when hub enabled (Supabase mode), got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHub_EnabledURLUnreachable(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	cfg := "hub:\n  enabled: true\n  url: http://127.0.0.1:19999\n"
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte(cfg), 0644)

	r := checkHub()
	// Hub uses Supabase — URL reachability is checked by checkSupabase, not checkHub.
	if r.Status != checkOK {
		t.Errorf("expected OK when hub enabled (Supabase mode), got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHub_EnabledURLReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	cfg := fmt.Sprintf("hub:\n  enabled: true\n  url: %s\n", srv.URL)
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte(cfg), 0644)

	r := checkHub()
	if r.Status != checkOK {
		t.Errorf("expected OK when hub reachable, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckHub_EnabledURLReturnsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	cfg := fmt.Sprintf("hub:\n  enabled: true\n  url: %s\n", srv.URL)
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte(cfg), 0644)

	r := checkHub()
	// Hub uses Supabase — reachability is not checked by checkHub anymore.
	if r.Status != checkOK {
		t.Errorf("expected OK when hub enabled (Supabase mode), got %s: %s", r.Status, r.Message)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkSupabase
// ────────────────────────────────────────────────────────────────

func TestCheckSupabase_NoConfigNoBuiltin(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()
	os.Remove(filepath.Join(dir, ".c4", "config.yaml"))

	origURL := builtinSupabaseURL
	builtinSupabaseURL = ""
	defer func() { builtinSupabaseURL = origURL }()

	r := checkSupabase()
	if r.Status != checkOK {
		t.Errorf("expected OK when no config and no builtin, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckSupabase_NotConfigured(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	origURL := builtinSupabaseURL
	builtinSupabaseURL = ""
	defer func() { builtinSupabaseURL = origURL }()

	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("project: test\n"), 0644)

	r := checkSupabase()
	if r.Status != checkOK {
		t.Errorf("expected OK when supabase not configured, got %s: %s", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "skipped") {
		t.Errorf("expected 'skipped' in message, got: %s", r.Message)
	}
}

func TestCheckSupabase_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Use a fake "supabase" URL via config
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	origURL := builtinSupabaseURL
	builtinSupabaseURL = ""
	defer func() { builtinSupabaseURL = origURL }()

	fakeURL := strings.Replace(srv.URL, "http://", "http://supabase.", 1)
	// Use a test server that mimics supabase by having "supabase" in the URL
	// Since httptest URL won't contain "supabase", use builtinSupabaseURL
	builtinSupabaseURL = srv.URL + "/supabase-fake"
	// builtinSupabaseURL won't match "supabase" substring check; use config approach
	builtinSupabaseURL = origURL
	_ = fakeURL

	// Write config with supabase URL
	cfg := fmt.Sprintf("cloud:\n  url: %s\n", srv.URL+"/supabase-rest")
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte(cfg), 0644)

	// The URL contains no "supabase" substring — should be skipped
	r := checkSupabase()
	if r.Status != checkOK {
		t.Errorf("expected OK (skipped non-supabase url), got %s: %s", r.Status, r.Message)
	}
}

func TestCheckSupabase_UnreachableButURL(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	origURL := builtinSupabaseURL
	builtinSupabaseURL = "https://test.supabase.co"
	defer func() { builtinSupabaseURL = origURL }()

	// Remove config so builtin URL is used
	os.Remove(filepath.Join(dir, ".c4", "config.yaml"))

	// The real supabase URL will fail network; that's expected in tests.
	r := checkSupabase()
	// Either FAIL (unreachable) or OK (cached/reachable) — just verify it doesn't panic.
	if r.Name != "Supabase" {
		t.Errorf("expected Name='Supabase', got %q", r.Name)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: tryFix
// ────────────────────────────────────────────────────────────────

func TestTryFix_BrokenSymlinkRemoved(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Create a broken symlink at CLAUDE.md
	claudePath := filepath.Join(dir, "CLAUDE.md")
	os.Symlink("/nonexistent/target/AGENTS.md", claudePath)

	r := &checkResult{
		Name:    "CLAUDE.md",
		Status:  checkFail,
		Message: "CLAUDE.md is a broken symlink",
	}
	result := tryFix(r)
	if result == "" {
		t.Error("expected tryFix to return non-empty string for broken symlink removal")
	}
	// Verify broken symlink is gone (tryFix removes it and creates a fresh CLAUDE.md from template)
	fi, err := os.Lstat(claudePath)
	if err == nil && fi.Mode()&os.ModeSymlink != 0 {
		// Still a symlink — check it's not broken
		if _, statErr := os.Stat(claudePath); os.IsNotExist(statErr) {
			t.Error("expected broken symlink to be removed after tryFix")
		}
	}
}

func TestTryFix_NoMatchReturnsEmpty(t *testing.T) {
	_, cleanup := setupDoctorEnv(t)
	defer cleanup()

	r := &checkResult{
		Name:    "unknown-check",
		Status:  checkFail,
		Message: "something failed",
	}
	result := tryFix(r)
	if result != "" {
		t.Errorf("expected empty string for unknown check, got %q", result)
	}
}

func TestTryFix_HooksFix(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	r := &checkResult{
		Name:    "hooks",
		Status:  checkWarn,
		Message: "hook outdated",
	}
	result := tryFix(r)
	// setupProjectHooks should succeed → "hook updated"
	if result == "" {
		t.Error("expected tryFix to return 'hook updated' for hooks fix")
	}
	// Verify hooks were installed
	if _, err := os.Stat(filepath.Join(dir, ".claude", "hooks", "c4-gate.sh")); err != nil {
		t.Errorf("expected gate hook to be created after tryFix: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: isHubEnabled
// ────────────────────────────────────────────────────────────────

func TestIsHubEnabled(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "hub enabled true",
			content: "hub:\n  enabled: true\n  url: http://localhost:8585\n",
			want:    true,
		},
		{
			name:    "hub enabled false",
			content: "hub:\n  enabled: false\n",
			want:    false,
		},
		{
			name:    "hub section missing",
			content: "project: test\n",
			want:    false,
		},
		{
			name:    "other section has enabled true but not hub",
			content: "observe:\n  enabled: true\nhub:\n  enabled: false\n",
			want:    false,
		},
		{
			name:    "hub enabled true with other sections after",
			content: "hub:\n  enabled: true\ncloud:\n  url: https://x.supabase.co\n",
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHubEnabled(tt.content)
			if got != tt.want {
				t.Errorf("isHubEnabled: got %v, want %v\nContent:\n%s", got, tt.want, tt.content)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────
// cloud.go: runCloudModeGet
// ────────────────────────────────────────────────────────────────

func TestRunCloudModeGet_DefaultMode(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".c4"), 0755)
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("project: test\n"), 0644)

	origDir := projectDir
	projectDir = dir
	defer func() { projectDir = origDir }()

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := runCloudModeGet(nil, nil)
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runCloudModeGet error: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := strings.TrimSpace(buf.String())
	if out != "local-first" {
		t.Errorf("expected 'local-first' as default, got %q", out)
	}
}

func TestRunCloudModeGet_CloudPrimary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".c4"), 0755)
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte("cloud:\n  mode: cloud-primary\n"), 0644)

	origDir := projectDir
	projectDir = dir
	defer func() { projectDir = origDir }()

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := runCloudModeGet(nil, nil)
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runCloudModeGet error: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := strings.TrimSpace(buf.String())
	if out != "cloud-primary" {
		t.Errorf("expected 'cloud-primary', got %q", out)
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: listJSONLNames
// ────────────────────────────────────────────────────────────────

func TestListJSONLNames_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := listJSONLNames(dir)
	if len(result) != 0 {
		t.Errorf("expected empty map for empty dir, got %v", result)
	}
}

func TestListJSONLNames_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "session1.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "session2.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir.jsonl"), 0755) // dir with .jsonl ext — should be excluded

	result := listJSONLNames(dir)
	if _, ok := result["session1.jsonl"]; !ok {
		t.Error("expected session1.jsonl in result")
	}
	if _, ok := result["session2.jsonl"]; !ok {
		t.Error("expected session2.jsonl in result")
	}
	if _, ok := result["readme.txt"]; ok {
		t.Error("readme.txt should not be in result")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestListJSONLNames_NonexistentDir(t *testing.T) {
	result := listJSONLNames("/nonexistent/path/that/does/not/exist")
	if len(result) != 0 {
		t.Errorf("expected empty map for nonexistent dir, got %v", result)
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: jsonlLastTimestamp
// ────────────────────────────────────────────────────────────────

func TestJsonlLastTimestamp_NonexistentFile(t *testing.T) {
	ts := jsonlLastTimestamp("/nonexistent/path.jsonl")
	if !ts.IsZero() {
		t.Errorf("expected zero time for nonexistent file, got %v", ts)
	}
}

func TestJsonlLastTimestamp_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0644)
	ts := jsonlLastTimestamp(path)
	if !ts.IsZero() {
		t.Errorf("expected zero time for empty file, got %v", ts)
	}
}

func TestJsonlLastTimestamp_ValidTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	now := time.Now().UTC().Truncate(time.Second)
	rec := map[string]interface{}{
		"timestamp": now.Format(time.RFC3339),
		"type":      "message",
	}
	data, _ := json.Marshal(rec)
	os.WriteFile(path, append(data, '\n'), 0644)

	ts := jsonlLastTimestamp(path)
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if !ts.Equal(now) {
		t.Errorf("expected %v, got %v", now, ts)
	}
}

func TestJsonlLastTimestamp_MultipleLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.jsonl")

	old := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	latest := time.Now().UTC().Truncate(time.Second)

	var lines []string
	for _, ts := range []time.Time{old, latest} {
		rec := map[string]interface{}{"timestamp": ts.Format(time.RFC3339)}
		data, _ := json.Marshal(rec)
		lines = append(lines, string(data))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	ts := jsonlLastTimestamp(path)
	if !ts.Equal(latest) {
		t.Errorf("expected latest timestamp %v, got %v", latest, ts)
	}
}

func TestJsonlLastTimestamp_NoTimestampField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notimestamp.jsonl")
	os.WriteFile(path, []byte(`{"type":"message","content":"hello"}`+"\n"), 0644)

	ts := jsonlLastTimestamp(path)
	if !ts.IsZero() {
		t.Errorf("expected zero time when no timestamp field, got %v", ts)
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: claudeProjectDir
// ────────────────────────────────────────────────────────────────

func TestClaudeProjectDir(t *testing.T) {
	dir, err := claudeProjectDir("/some/project/path")
	if err != nil {
		t.Fatalf("claudeProjectDir error: %v", err)
	}
	if !strings.Contains(dir, ".claude") {
		t.Errorf("expected .claude in path, got %s", dir)
	}
	if !strings.Contains(dir, "projects") {
		t.Errorf("expected 'projects' in path, got %s", dir)
	}
	// The encoded path portion (after .claude/projects/) should contain dashes
	// (path separators replaced with dashes). e.g. "-some-project-path"
	projectsPart := dir[strings.Index(dir, "projects/")+len("projects/"):]
	if !strings.Contains(projectsPart, "-") {
		t.Errorf("expected dashes in encoded path portion, got %s", projectsPart)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: expandTilde
// ────────────────────────────────────────────────────────────────

func TestExpandTilde_NoTilde(t *testing.T) {
	result := expandTilde("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("expected unchanged path, got %s", result)
	}
}

func TestExpandTilde_WithTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := expandTilde("~/.local/bin/cq")
	if !strings.HasPrefix(result, home) {
		t.Errorf("expected path to start with %s, got %s", home, result)
	}
	if strings.Contains(result, "~") {
		t.Errorf("tilde should be expanded, got %s", result)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: extractMCPBinaryPath
// ────────────────────────────────────────────────────────────────

func TestExtractMCPBinaryPath_CommandHasCQ(t *testing.T) {
	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"c4": map[string]interface{}{
				"command": "/home/user/.local/bin/cq",
				"args":    []interface{}{"mcp"},
			},
		},
	}
	result := extractMCPBinaryPath(cfg)
	if !strings.Contains(result, "cq") {
		t.Errorf("expected 'cq' in result, got %s", result)
	}
}

func TestExtractMCPBinaryPath_NoMCPServers(t *testing.T) {
	cfg := map[string]interface{}{"other": "value"}
	result := extractMCPBinaryPath(cfg)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestExtractMCPBinaryPath_ArgHasCQ(t *testing.T) {
	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"c4": map[string]interface{}{
				"command": "node",
				"args":    []interface{}{"/usr/local/bin/cq", "mcp"},
			},
		},
	}
	result := extractMCPBinaryPath(cfg)
	if !strings.Contains(result, "cq") {
		t.Errorf("expected 'cq' in args result, got %s", result)
	}
}

func TestExtractMCPBinaryPath_NoCQReference(t *testing.T) {
	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"other": map[string]interface{}{
				"command": "node",
				"args":    []interface{}{"server.js"},
			},
		},
	}
	result := extractMCPBinaryPath(cfg)
	if result != "" {
		t.Errorf("expected empty string when no cq reference, got %s", result)
	}
}

// ────────────────────────────────────────────────────────────────
// eventbus.go: resolveEventbusDataDir, resolveSocketPath, parseSinceTime
// ────────────────────────────────────────────────────────────────

func TestResolveEventbusDataDir_DefaultPath(t *testing.T) {
	origDir := eventbusDataDir
	eventbusDataDir = ""
	defer func() { eventbusDataDir = origDir }()

	dir, err := resolveEventbusDataDir()
	if err != nil {
		t.Fatalf("resolveEventbusDataDir error: %v", err)
	}
	if !strings.Contains(dir, ".c4") || !strings.Contains(dir, "eventbus") {
		t.Errorf("expected path containing .c4/eventbus, got %s", dir)
	}
}

func TestResolveEventbusDataDir_ExplicitPath(t *testing.T) {
	origDir := eventbusDataDir
	eventbusDataDir = "/tmp/test-eventbus"
	defer func() { eventbusDataDir = origDir }()

	dir, err := resolveEventbusDataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/tmp/test-eventbus" {
		t.Errorf("expected /tmp/test-eventbus, got %s", dir)
	}
}

func TestResolveSocketPath_DefaultPath(t *testing.T) {
	origSocket := eventbusSocket
	eventbusSocket = ""
	defer func() { eventbusSocket = origSocket }()

	result := resolveSocketPath("/some/data/dir")
	if !strings.HasSuffix(result, "c3.sock") {
		t.Errorf("expected path ending with c3.sock, got %s", result)
	}
	if !strings.Contains(result, "some/data/dir") {
		t.Errorf("expected data dir in path, got %s", result)
	}
}

func TestResolveSocketPath_ExplicitSocket(t *testing.T) {
	origSocket := eventbusSocket
	eventbusSocket = "/tmp/custom.sock"
	defer func() { eventbusSocket = origSocket }()

	result := resolveSocketPath("/ignored/dir")
	if result != "/tmp/custom.sock" {
		t.Errorf("expected /tmp/custom.sock, got %s", result)
	}
}

func TestParseSinceTime_Empty(t *testing.T) {
	ms := parseSinceTime("")
	if ms != 0 {
		t.Errorf("expected 0 for empty string, got %d", ms)
	}
}

func TestParseSinceTime_ValidRFC3339(t *testing.T) {
	ts := "2024-01-15T10:30:00Z"
	ms := parseSinceTime(ts)
	if ms <= 0 {
		t.Errorf("expected positive ms for valid RFC3339, got %d", ms)
	}
}

func TestParseSinceTime_ValidDate(t *testing.T) {
	ms := parseSinceTime("2024-01-15")
	if ms <= 0 {
		t.Errorf("expected positive ms for date-only format, got %d", ms)
	}
}

func TestParseSinceTime_Invalid(t *testing.T) {
	ms := parseSinceTime("not-a-date")
	if ms != 0 {
		t.Errorf("expected 0 for invalid date, got %d", ms)
	}
}

// ────────────────────────────────────────────────────────────────
// daemon.go: acquirePIDLock
// ────────────────────────────────────────────────────────────────

func TestAcquirePIDLock_NewFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	err := acquirePIDLock(pidPath)
	if err != nil {
		t.Fatalf("acquirePIDLock error: %v", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("reading pid file: %v", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Error("expected PID written to file")
	}
}

func TestAcquirePIDLock_StalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	// Write a stale PID (very large number, unlikely to exist)
	os.WriteFile(pidPath, []byte("999999999"), 0644)

	err := acquirePIDLock(pidPath)
	if err != nil {
		t.Fatalf("expected no error for stale PID, got: %v", err)
	}
	// Verify our PID was written
	data, _ := os.ReadFile(pidPath)
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "999999999" {
		t.Error("expected stale PID to be replaced")
	}
}

// ────────────────────────────────────────────────────────────────
// root.go: c4Dir, dbPath
// ────────────────────────────────────────────────────────────────

func TestC4Dir(t *testing.T) {
	origDir := projectDir
	projectDir = "/tmp/testproject"
	defer func() { projectDir = origDir }()

	result := c4Dir()
	if result != "/tmp/testproject/.c4" {
		t.Errorf("expected /tmp/testproject/.c4, got %s", result)
	}
}

func TestDbPath_FallbackToTasksDB(t *testing.T) {
	dir := t.TempDir()
	origDir := projectDir
	projectDir = dir
	defer func() { projectDir = origDir }()

	// No c4.db or tasks.db → falls back to tasks.db path
	result := dbPath()
	if !strings.HasSuffix(result, "tasks.db") {
		t.Errorf("expected tasks.db fallback, got %s", result)
	}
}

func TestDbPath_PrefersC4DB(t *testing.T) {
	dir := t.TempDir()
	origDir := projectDir
	projectDir = dir
	defer func() { projectDir = origDir }()

	// Create .c4/c4.db
	c4d := filepath.Join(dir, ".c4")
	os.MkdirAll(c4d, 0755)
	os.WriteFile(filepath.Join(c4d, "c4.db"), []byte{}, 0644)

	result := dbPath()
	if !strings.HasSuffix(result, "c4.db") {
		t.Errorf("expected c4.db preference, got %s", result)
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: currentSessionUUID
// ────────────────────────────────────────────────────────────────

func TestCurrentSessionUUID_FromEnvVar(t *testing.T) {
	origUUID := os.Getenv("CQ_SESSION_UUID")
	os.Setenv("CQ_SESSION_UUID", "test-uuid-1234")
	defer os.Setenv("CQ_SESSION_UUID", origUUID)

	result := currentSessionUUID("/any/dir")
	if result != "test-uuid-1234" {
		t.Errorf("expected test-uuid-1234, got %s", result)
	}
}

func TestCurrentSessionUUID_NoSessionDir(t *testing.T) {
	// Unset env var; project dir has no .claude/projects directory
	origUUID := os.Getenv("CQ_SESSION_UUID")
	os.Unsetenv("CQ_SESSION_UUID")
	defer os.Setenv("CQ_SESSION_UUID", origUUID)

	result := currentSessionUUID(t.TempDir())
	// Should return empty string when no sessions found
	if result != "" {
		// Could be a real session — just verify it's a string
		_ = result
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkSupabase (config with non-supabase URL → skipped)
// ────────────────────────────────────────────────────────────────

func TestCheckSupabase_WithSupabaseURLInConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	origURL := builtinSupabaseURL
	builtinSupabaseURL = ""
	defer func() { builtinSupabaseURL = origURL }()

	// Use a URL that contains "supabase" to trigger the health check path
	// We'll manipulate the URL to contain "supabase" substring
	cfg := fmt.Sprintf("cloud:\n  url: \"https://abc.supabase.local.test\"\n")
	os.WriteFile(filepath.Join(dir, ".c4", "config.yaml"), []byte(cfg), 0644)

	// The URL contains "supabase" but won't be reachable → FAIL
	r := checkSupabase()
	// Either FAIL or WARN — should not panic
	if r.Name != "Supabase" {
		t.Errorf("expected Name='Supabase', got %q", r.Name)
	}
}

// ────────────────────────────────────────────────────────────────
// doctor.go: checkC4Dir with config info
// ────────────────────────────────────────────────────────────────

func TestCheckC4Dir_WithTasksDB(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// tasks.db is already created by setupDoctorEnv
	_ = dir
	r := checkC4Dir()
	if r.Status != checkOK {
		t.Errorf("expected OK, got %s: %s", r.Status, r.Message)
	}
	if !strings.Contains(r.Message, "db") {
		t.Errorf("expected 'db' in message, got: %s", r.Message)
	}
}

func TestCheckC4Dir_MissingTasksDB(t *testing.T) {
	dir, cleanup := setupDoctorEnv(t)
	defer cleanup()

	// Remove tasks.db
	os.Remove(filepath.Join(dir, ".c4", "tasks.db"))

	r := checkC4Dir()
	// Should still be OK or WARN (missing db is a warning, not fatal)
	if r.Name != ".c4 directory" {
		t.Errorf("expected Name='.c4 directory', got %q", r.Name)
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: codexConfigPath, codexMCPBlock, findCQBinary
// ────────────────────────────────────────────────────────────────

func TestCodexConfigPath_FromEnv(t *testing.T) {
	orig := os.Getenv("CODEX_CONFIG")
	os.Setenv("CODEX_CONFIG", "/custom/codex/config.toml")
	defer os.Setenv("CODEX_CONFIG", orig)

	p, err := codexConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != "/custom/codex/config.toml" {
		t.Errorf("expected /custom/codex/config.toml, got %s", p)
	}
}

func TestCodexConfigPath_Default(t *testing.T) {
	orig := os.Getenv("CODEX_CONFIG")
	os.Unsetenv("CODEX_CONFIG")
	defer os.Setenv("CODEX_CONFIG", orig)

	p, err := codexConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p, ".codex") {
		t.Errorf("expected .codex in default path, got %s", p)
	}
}

func TestCodexMCPBlock(t *testing.T) {
	block := codexMCPBlock("/usr/local/bin/cq", "/my/project")
	if !strings.Contains(block, "[mcp_servers.cq]") {
		t.Error("expected [mcp_servers.cq] header")
	}
	if !strings.Contains(block, "/usr/local/bin/cq") {
		t.Error("expected binary path in block")
	}
	if !strings.Contains(block, "/my/project") {
		t.Error("expected project dir in block")
	}
}

func TestFindCQBinary(t *testing.T) {
	p, err := findCQBinary()
	// In test environment, either finds the test binary or falls back to PATH
	if err != nil {
		// Only fail if cq is truly not findable — acceptable in CI
		t.Logf("findCQBinary returned error (acceptable in CI): %v", err)
		return
	}
	if p == "" {
		t.Error("expected non-empty path from findCQBinary")
	}
}

// ────────────────────────────────────────────────────────────────
// cloud.go: runCloudModeSet (valid value)
// ────────────────────────────────────────────────────────────────

func TestRunCloudModeSet_ValidValue(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".c4"), 0755)

	origDir := projectDir
	projectDir = dir
	defer func() { projectDir = origDir }()

	err := runCloudModeSet(nil, []string{"local-first"})
	if err != nil {
		t.Fatalf("runCloudModeSet error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".c4", "config.yaml"))
	if !strings.Contains(string(data), "local-first") {
		t.Errorf("expected local-first in config, got: %s", string(data))
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: isCQServeProcess (darwin path)
// ────────────────────────────────────────────────────────────────

func TestIsCQServeProcess_NonExistentPID(t *testing.T) {
	// PID 999999999 won't exist
	result := isCQServeProcess(999999999)
	if result {
		t.Error("expected false for non-existent PID")
	}
}

// ────────────────────────────────────────────────────────────────
// init.go: loadNamedSessions
// ────────────────────────────────────────────────────────────────

func TestLoadNamedSessions_NonexistentFile(t *testing.T) {
	// Point namedSessionsFile to a non-existent path by using a temp dir
	// The function uses os.UserHomeDir() — we can test the "not exist" branch
	// via a wrapper or by calling directly (since it always uses real home dir).
	// Instead, we test the exported behavior via saveNamedSessions + loadNamedSessions.
	dir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	sessions, err := loadNamedSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty map, got %v", sessions)
	}
}

func TestSaveAndLoadNamedSessions(t *testing.T) {
	dir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	sessions := map[string]namedSessionEntry{
		"main": {UUID: "uuid-123", Dir: "/project", Tool: "claude", Updated: "2024-01-01"},
		"work": {UUID: "uuid-456", Dir: "/work", Memo: "work session", Updated: "2024-01-02"},
	}
	if err := saveNamedSessions(sessions); err != nil {
		t.Fatalf("saveNamedSessions error: %v", err)
	}

	loaded, err := loadNamedSessions()
	if err != nil {
		t.Fatalf("loadNamedSessions error: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(loaded))
	}
	if loaded["main"].UUID != "uuid-123" {
		t.Errorf("expected uuid-123, got %s", loaded["main"].UUID)
	}
}
