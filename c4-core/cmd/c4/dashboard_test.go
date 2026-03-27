package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDashboard_GlobalConfigReadWrite verifies readGlobalConfig/writeGlobalConfig
// round-trips through a temp home directory.
func TestDashboard_GlobalConfigReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .c4 directory
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write and read back
	if err := writeGlobalConfig("default_tool", "claude"); err != nil {
		t.Fatalf("writeGlobalConfig: %v", err)
	}
	got := readGlobalConfig("default_tool")
	if got != "claude" {
		t.Errorf("readGlobalConfig(default_tool) = %q, want %q", got, "claude")
	}

	// Overwrite should work
	if err := writeGlobalConfig("default_tool", "cursor"); err != nil {
		t.Fatalf("writeGlobalConfig overwrite: %v", err)
	}
	got = readGlobalConfig("default_tool")
	if got != "cursor" {
		t.Errorf("readGlobalConfig after overwrite = %q, want %q", got, "cursor")
	}

	// Nonexistent key returns ""
	got = readGlobalConfig("nonexistent")
	if got != "" {
		t.Errorf("readGlobalConfig(nonexistent) = %q, want empty string", got)
	}
}

// TestDashboard_ToolDetection verifies the LookPath pattern used by probeToolEntry.
func TestDashboard_ToolDetection(t *testing.T) {
	// "ls" or "echo" should always be found on macOS/Linux
	knownTool := "ls"
	path, err := exec.LookPath(knownTool)
	if err != nil || path == "" {
		t.Errorf("LookPath(%q) failed, expected it to be found: %v", knownTool, err)
	}

	// A tool that definitely does not exist
	unknownTool := "nonexistent-tool-xyz-12345"
	_, err = exec.LookPath(unknownTool)
	if err == nil {
		t.Errorf("LookPath(%q) succeeded, expected error", unknownTool)
	}
}

// TestDashboard_NonTTYFallback verifies runCQStartText prints expected content
// and doesn't panic when serve is unavailable.
func TestDashboard_NonTTYFallback(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCQStartText()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if err != nil {
		t.Errorf("runCQStartText() returned error: %v", err)
	}

	// Must contain "CQ" and at least a separator line
	if !strings.Contains(out, "CQ") {
		t.Errorf("output missing 'CQ': %q", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("output missing separator: %q", out)
	}
	// Either "starting..." (serve down) or "running" (serve up)
	if !strings.Contains(out, "starting...") && !strings.Contains(out, "running") {
		t.Errorf("output missing service status line: %q", out)
	}
}

// TestDashboard_WhatsNew verifies the dashboardModel View() shows "New" / "✨"
// only when version differs from lastSeenVersion.
func TestDashboard_WhatsNew(t *testing.T) {
	// New version seen for the first time — should show the badge
	mNew := dashboardModel{
		version:     "1.40.0",
		defaultTool: "claude",
		rows:        []dashboardRow{{label: "Service", value: "starting..."}},
		whatsNew:    fmt.Sprintf("✨ New in %s", "1.40.0"),
	}
	viewNew := mNew.View()
	if !strings.Contains(viewNew, "New") && !strings.Contains(viewNew, "✨") {
		t.Errorf("View() with new version should contain 'New' or '✨', got: %q", viewNew)
	}

	// Same version — whatsNew field is empty, badge must NOT appear
	mSame := dashboardModel{
		version:     "1.40.0",
		defaultTool: "claude",
		rows:        []dashboardRow{{label: "Service", value: "starting..."}},
		whatsNew:    "",
	}
	viewSame := mSame.View()
	if strings.Contains(viewSame, "✨") {
		t.Errorf("View() with same version should NOT contain '✨', got: %q", viewSame)
	}
	// The word "New" might appear in hint text, so only check for ✨ absence.
}
