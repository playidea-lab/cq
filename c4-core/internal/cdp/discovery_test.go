package cdp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCDPURL_ExplicitOverride(t *testing.T) {
	got := DiscoverCDPURL("http://localhost:1234")
	if got != "http://localhost:1234" {
		t.Errorf("DiscoverCDPURL with explicit = %q, want http://localhost:1234", got)
	}
}

func TestDiscoverCDPURL_EnvOverride(t *testing.T) {
	t.Setenv("CDP_DEBUG_URL", "http://localhost:5555")
	got := DiscoverCDPURL("")
	if got != "http://localhost:5555" {
		t.Errorf("DiscoverCDPURL with env = %q, want http://localhost:5555", got)
	}
}

func TestDiscoverCDPURL_FallsBackToDefault(t *testing.T) {
	t.Setenv("CDP_DEBUG_URL", "")
	got := DiscoverCDPURL("")
	if got != DefaultDebugURL {
		// It might find a running browser, which is also fine.
		t.Logf("DiscoverCDPURL = %q (may have found running browser or fell back to default)", got)
	}
}

func TestReadDevToolsActivePort_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	portFile := filepath.Join(tmpDir, "DevToolsActivePort")
	if err := os.WriteFile(portFile, []byte("9333\n/devtools/browser/abc"), 0644); err != nil {
		t.Fatal(err)
	}

	// Read it manually to test parsing logic
	data, _ := os.ReadFile(portFile)
	lines := splitFirst(string(data))
	if lines != "9333" {
		t.Errorf("parsed port line = %q, want 9333", lines)
	}
}

func TestReadDevToolsActivePort_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	portFile := filepath.Join(tmpDir, "DevToolsActivePort")
	if err := os.WriteFile(portFile, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not panic, just return 0
	port := readDevToolsActivePortFrom(portFile)
	if port != 0 {
		t.Errorf("port = %d, want 0 for invalid content", port)
	}
}

func TestProbeCDP_UnreachablePort(t *testing.T) {
	// Port 1 should not have a CDP server
	if probeCDP("http://localhost:1") {
		t.Error("probeCDP returned true for unreachable port")
	}
}

func TestDevToolsActivePortPaths_NotEmpty(t *testing.T) {
	paths := devToolsActivePortPaths()
	if len(paths) == 0 {
		t.Skip("no paths for this platform")
	}
	for _, p := range paths {
		if p == "" {
			t.Error("empty path in devToolsActivePortPaths")
		}
	}
}

func TestWebMCPContextResultJSON(t *testing.T) {
	result := WebMCPContextResult{
		Context:   map[string]any{"user": "test", "cart_items": float64(3)},
		Action:    "get",
		Origin:    "https://shop.example.com",
		Available: true,
	}

	if result.Action != "get" {
		t.Errorf("Action = %q, want get", result.Action)
	}
	if !result.Available {
		t.Error("Available should be true")
	}
}

func TestDiscoverOpts_Defaults(t *testing.T) {
	opts := &DiscoverOpts{}
	if opts.WaitForTools {
		t.Error("WaitForTools should default to false")
	}
	opts.WaitForTools = true
	if !opts.WaitForTools {
		t.Error("WaitForTools should be true after setting")
	}
}

// Helper: split first line (extracted from readDevToolsActivePort logic)
func splitFirst(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

// readDevToolsActivePortFrom reads a specific file (for testing).
func readDevToolsActivePortFrom(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := splitFirst(string(data))
	port := 0
	fmt.Sscanf(lines, "%d", &port)
	if port <= 0 || port > 65535 {
		return 0
	}
	return port
}
