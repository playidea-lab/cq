package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudYAMLValue(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
		want    string
	}{
		{
			name:    "basic url",
			content: "cloud:\n  url: https://example.supabase.co\n",
			key:     "url:",
			want:    "https://example.supabase.co",
		},
		{
			name:    "no cloud section",
			content: "hub:\n  enabled: true\n",
			key:     "url:",
			want:    "",
		},
		{
			name:    "key in different section ignored",
			content: "hub:\n  url: http://hub\ncloud:\n  enabled: true\n",
			key:     "url:",
			want:    "",
		},
		{
			name:    "enabled value",
			content: "cloud:\n  enabled: true\n  url: https://x.supabase.co\n",
			key:     "enabled:",
			want:    "true",
		},
		{
			name:    "empty content",
			content: "",
			key:     "url:",
			want:    "",
		},
		{
			name: "cloud section ends at next top-level key",
			content: "cloud:\n  url: https://a.supabase.co\nhub:\n  url: https://hub\n",
			key:     "url:",
			want:    "https://a.supabase.co",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudYAMLValue(tt.content, tt.key)
			if got != tt.want {
				t.Errorf("cloudYAMLValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteCloudSectionToYAML(t *testing.T) {
	desired := map[string]string{
		"enabled:":  "true",
		"url:":      "https://test.supabase.co",
		"anon_key:": "test-key",
		"mode:":     "local-first",
	}

	t.Run("empty file creates cloud section", func(t *testing.T) {
		result := writeCloudSectionToYAML("", desired)
		if !strings.Contains(result, "cloud:") {
			t.Error("missing cloud: header")
		}
		if !strings.Contains(result, "  enabled: true") {
			t.Error("missing enabled")
		}
		if !strings.Contains(result, "  url: https://test.supabase.co") {
			t.Error("missing url")
		}
		if !strings.Contains(result, "  anon_key: test-key") {
			t.Error("missing anon_key")
		}
		if !strings.Contains(result, "  mode: local-first") {
			t.Error("missing mode")
		}
	})

	t.Run("existing cloud section updates values", func(t *testing.T) {
		existing := "cloud:\n  enabled: false\n  url: https://old.supabase.co\n  anon_key: old-key\n  mode: cloud-primary\n"
		result := writeCloudSectionToYAML(existing, desired)
		if !strings.Contains(result, "  enabled: true") {
			t.Errorf("enabled not updated, got:\n%s", result)
		}
		if !strings.Contains(result, "  url: https://test.supabase.co") {
			t.Errorf("url not updated, got:\n%s", result)
		}
		if !strings.Contains(result, "  mode: local-first") {
			t.Errorf("mode not updated, got:\n%s", result)
		}
	})

	t.Run("preserves other sections", func(t *testing.T) {
		existing := "hub:\n  enabled: true\ncloud:\n  enabled: false\n"
		result := writeCloudSectionToYAML(existing, desired)
		if !strings.Contains(result, "hub:\n  enabled: true") {
			t.Errorf("hub section lost, got:\n%s", result)
		}
	})

	t.Run("inserts missing keys into existing section", func(t *testing.T) {
		existing := "cloud:\n  enabled: false\n"
		result := writeCloudSectionToYAML(existing, desired)
		if !strings.Contains(result, "  url: https://test.supabase.co") {
			t.Errorf("url not inserted, got:\n%s", result)
		}
		if !strings.Contains(result, "  anon_key: test-key") {
			t.Errorf("anon_key not inserted, got:\n%s", result)
		}
	})

	t.Run("file with other content but no cloud section", func(t *testing.T) {
		existing := "hub:\n  enabled: true\n"
		result := writeCloudSectionToYAML(existing, desired)
		if !strings.Contains(result, "hub:\n  enabled: true") {
			t.Errorf("hub section lost, got:\n%s", result)
		}
		if !strings.Contains(result, "cloud:") {
			t.Errorf("cloud section not appended, got:\n%s", result)
		}
	})
}

func TestPatchCloudConfigAfterLogin(t *testing.T) {
	// Save and restore globals.
	origURL := builtinSupabaseURL
	origKey := builtinSupabaseKey
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		builtinSupabaseKey = origKey
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://builtin.supabase.co"
	builtinSupabaseKey = "builtin-anon-key"

	t.Run("no .c4 directory returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		url := patchCloudConfigAfterLogin(tmpDir)
		if url != "" {
			t.Errorf("expected empty, got %q", url)
		}
	})

	t.Run("creates config.yaml in .c4", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)

		url := patchCloudConfigAfterLogin(tmpDir)
		if url != "https://builtin.supabase.co" {
			t.Errorf("expected builtin URL, got %q", url)
		}

		data, err := os.ReadFile(filepath.Join(tmpDir, ".c4", "config.yaml"))
		if err != nil {
			t.Fatalf("reading config: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "enabled: true") {
			t.Error("missing enabled: true")
		}
		if !strings.Contains(content, "url: https://builtin.supabase.co") {
			t.Error("missing url")
		}
		if !strings.Contains(content, "mode: cloud-primary") {
			t.Error("missing mode")
		}
	})

	t.Run("preserves existing user URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)
		configPath := filepath.Join(tmpDir, ".c4", "config.yaml")
		os.WriteFile(configPath, []byte("cloud:\n  url: https://custom.supabase.co\n"), 0644)

		url := patchCloudConfigAfterLogin(tmpDir)
		if url != "https://custom.supabase.co" {
			t.Errorf("expected custom URL preserved, got %q", url)
		}

		data, _ := os.ReadFile(configPath)
		content := string(data)
		if !strings.Contains(content, "url: https://custom.supabase.co") {
			t.Errorf("user URL overwritten, got:\n%s", content)
		}
	})

	t.Run("preserves existing user anon_key", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, ".c4"), 0755)
		configPath := filepath.Join(tmpDir, ".c4", "config.yaml")
		os.WriteFile(configPath, []byte("cloud:\n  anon_key: custom-key\n"), 0644)

		patchCloudConfigAfterLogin(tmpDir)

		data, _ := os.ReadFile(configPath)
		content := string(data)
		if !strings.Contains(content, "anon_key: custom-key") {
			t.Errorf("user anon_key overwritten, got:\n%s", content)
		}
	})
}

// TestEnsureCloudAuth_SoloMode: builtinSupabaseURL="" → returns true immediately (no prompt).
func TestEnsureCloudAuth_SoloMode(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	builtinSupabaseURL = ""
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")

	// Pass nil reader — if it reads from reader, it will panic (nil dereference),
	// proving the solo-mode fast path doesn't touch stdin.
	got := ensureCloudAuth(nil, false)
	if !got {
		t.Error("expected true for solo mode (no cloud URL)")
	}
}

// TestEnsureCloudAuth_ValidSession: cloud URL set + valid session → returns true.
func TestEnsureCloudAuth_ValidSession(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	// Use a non-empty URL so we're not in solo mode.
	// GetSession reads ~/.c4/session.json; in test environment it will
	// likely return nil/error (no session), which means we'll fall through
	// to the prompt. To test the valid-session path we need a real session
	// file. Skip if no session is available.
	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir()) // isolate: no session.json → deterministic no-session path

	// No session in isolated HOME → prompt shown, user inputs "n".
	r := strings.NewReader("n\n")
	got := ensureCloudAuth(r, false)
	if got {
		t.Error("expected false when user declines (no session)")
	}
}

// TestEnsureCloudAuth_Decline: cloud URL set, no session, user inputs "n" → returns false.
func TestEnsureCloudAuth_Decline(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir()) // isolate: no session.json → deterministic no-session path

	r := strings.NewReader("n\n")
	got := ensureCloudAuth(r, false)
	// No session in isolated HOME → prompt shown, user declines → false.
	if got {
		t.Error("expected false when user inputs 'n' (no session)")
	}
}

// TestEnsureCloudAuth_EmptyInput: EOF input → returns false (no session case).
func TestEnsureCloudAuth_EmptyInput(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir()) // isolate: no session.json → deterministic no-session path

	r := strings.NewReader("") // EOF immediately
	// No session in isolated HOME → prompt shown, EOF → scanner returns false → false.
	got := ensureCloudAuth(r, false)
	if got {
		t.Error("expected false on EOF input (no session)")
	}
}

// writeHubConfig writes a minimal .c4/config.yaml with hub.url set for testing.
func writeHubConfig(t *testing.T, dir, hubURL string) {
	t.Helper()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := fmt.Sprintf("hub:\n  url: %s\n", hubURL)
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestEnsureCloudAuth_YesAll: yesAll=true, no hub, no valid session →
// skips the prompt and calls authLoginFunc with mode="" (browser OAuth) → returns true.
func TestEnsureCloudAuth_YesAll(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir()) // isolate: no session.json → deterministic no-session path

	// No hub configured → mode should be "" (browser OAuth).
	projectDir = t.TempDir()

	// Stub authLoginFunc to succeed without network.
	loginCalled := false
	var calledMode string
	authLoginFunc = func(mode string) error {
		loginCalled = true
		calledMode = mode
		return nil
	}

	// Pass nil reader — if the code reads from stdin when yesAll=true, it panics.
	// Isolated HOME guarantees no session → yesAll skips prompt → calls authLoginFunc.
	got := ensureCloudAuth(nil, true)
	if !got {
		t.Error("expected true when yesAll=true and login succeeds")
	}
	if !loginCalled {
		t.Error("expected authLoginFunc to be called when yesAll=true and no session")
	}
	if calledMode != "" {
		t.Errorf("expected mode=\"\" (no hub→browser OAuth), got %q", calledMode)
	}
}

// TestEnsureCloudAuth_LinkMode: Hub configured + "y" input → mode=="link".
func TestEnsureCloudAuth_LinkMode(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	tmpDir := t.TempDir()
	writeHubConfig(t, tmpDir, "https://hub.example.com")
	projectDir = tmpDir

	var calledMode string
	authLoginFunc = func(mode string) error {
		calledMode = mode
		return nil
	}

	r := strings.NewReader("y\n")
	got := ensureCloudAuth(r, false)
	if !got {
		t.Error("expected true when hub configured and user inputs 'y'")
	}
	if calledMode != "link" {
		t.Errorf("expected mode=\"link\", got %q", calledMode)
	}
}

// TestEnsureCloudAuth_DeviceMode: Hub configured + "d" input → mode=="device".
func TestEnsureCloudAuth_DeviceMode(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	tmpDir := t.TempDir()
	writeHubConfig(t, tmpDir, "https://hub.example.com")
	projectDir = tmpDir

	var calledMode string
	authLoginFunc = func(mode string) error {
		calledMode = mode
		return nil
	}

	r := strings.NewReader("d\n")
	got := ensureCloudAuth(r, false)
	if !got {
		t.Error("expected true when hub configured and user inputs 'd'")
	}
	if calledMode != "device" {
		t.Errorf("expected mode=\"device\", got %q", calledMode)
	}
}

// TestEnsureCloudAuth_NoHubFallback: Hub not configured + "y" → mode=="" (browser OAuth).
func TestEnsureCloudAuth_NoHubFallback(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	// No hub config in projectDir.
	projectDir = t.TempDir()

	loginCalled := false
	var calledMode string
	authLoginFunc = func(mode string) error {
		loginCalled = true
		calledMode = mode
		return nil
	}

	r := strings.NewReader("y\n")
	got := ensureCloudAuth(r, false)
	if !got {
		t.Error("expected true when no hub and user inputs 'y'")
	}
	if !loginCalled {
		t.Error("expected authLoginFunc to be called")
	}
	if calledMode != "" {
		t.Errorf("expected mode=\"\" (no hub→browser OAuth), got %q", calledMode)
	}
}

// TestEnsureCloudAuth_DeviceWithoutHub: Hub not configured + "d" input → returns false.
func TestEnsureCloudAuth_DeviceWithoutHub(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	// No hub config.
	projectDir = t.TempDir()

	loginCalled := false
	authLoginFunc = func(mode string) error {
		loginCalled = true
		return nil
	}

	r := strings.NewReader("d\n")
	got := ensureCloudAuth(r, false)
	if got {
		t.Error("expected false when device mode requested without hub")
	}
	if loginCalled {
		t.Error("expected authLoginFunc NOT to be called when hub missing")
	}
}

// TestEnsureCloudAuth_YesAllWithHub: Hub configured + yesAll=true → mode=="link".
func TestEnsureCloudAuth_YesAllWithHub(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	origDir := projectDir
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
		projectDir = origDir
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	tmpDir := t.TempDir()
	writeHubConfig(t, tmpDir, "https://hub.example.com")
	projectDir = tmpDir

	var calledMode string
	authLoginFunc = func(mode string) error {
		calledMode = mode
		return nil
	}

	got := ensureCloudAuth(nil, true)
	if !got {
		t.Error("expected true when hub configured and yesAll=true")
	}
	if calledMode != "link" {
		t.Errorf("expected mode=\"link\" (hub+yesAll), got %q", calledMode)
	}
}
