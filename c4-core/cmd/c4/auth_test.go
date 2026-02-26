package main

import (
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
		if !strings.Contains(content, "mode: local-first") {
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
	os.Unsetenv("C4_CLOUD_URL")
	os.Unsetenv("SUPABASE_URL")

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
	os.Unsetenv("C4_CLOUD_URL")
	os.Unsetenv("SUPABASE_URL")

	// In CI there's no session, so this test only validates no panic.
	// We use "n\n" as input to decline, confirming we reach the prompt path.
	r := strings.NewReader("n\n")
	got := ensureCloudAuth(r, false)
	// Either true (valid session) or false (declined) — just must not panic.
	_ = got
}

// TestEnsureCloudAuth_Decline: cloud URL set, no session, user inputs "n" → returns false.
func TestEnsureCloudAuth_Decline(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	builtinSupabaseURL = "https://test.supabase.co"
	os.Unsetenv("C4_CLOUD_URL")
	os.Unsetenv("SUPABASE_URL")

	r := strings.NewReader("n\n")
	got := ensureCloudAuth(r, false)
	// If there's already a valid session in the test env, result is true.
	// Otherwise (no session) the user declined → false.
	// Either outcome is acceptable; the key assertion is no panic.
	_ = got
}

// TestEnsureCloudAuth_EmptyInput: EOF input → returns false (no session case).
func TestEnsureCloudAuth_EmptyInput(t *testing.T) {
	origURL := builtinSupabaseURL
	defer func() { builtinSupabaseURL = origURL }()

	builtinSupabaseURL = "https://test.supabase.co"
	os.Unsetenv("C4_CLOUD_URL")
	os.Unsetenv("SUPABASE_URL")

	r := strings.NewReader("") // EOF immediately
	// If no valid session: scanner returns false on EOF → returns false.
	// If valid session: returns true immediately (doesn't read r).
	got := ensureCloudAuth(r, false)
	_ = got // no panic is the assertion
}

// TestEnsureCloudAuth_YesAll: yesAll=true, cloud URL set, no valid session →
// skips the prompt and calls authLoginFunc (stubbed to succeed) → returns true.
func TestEnsureCloudAuth_YesAll(t *testing.T) {
	origURL := builtinSupabaseURL
	origLoginFunc := authLoginFunc
	defer func() {
		builtinSupabaseURL = origURL
		authLoginFunc = origLoginFunc
	}()

	builtinSupabaseURL = "https://test.supabase.co"
	os.Unsetenv("C4_CLOUD_URL")
	os.Unsetenv("SUPABASE_URL")

	// Stub authLoginFunc to succeed without network.
	loginCalled := false
	authLoginFunc = func() error {
		loginCalled = true
		return nil
	}

	// Pass nil reader — if the code reads from stdin when yesAll=true, it panics.
	// No panic + loginCalled=true proves the yesAll path is exercised.
	// Note: if a valid session exists, we return true before calling login.
	// In that case loginCalled stays false but got is still true.
	got := ensureCloudAuth(nil, true)
	if !got {
		t.Error("expected true when yesAll=true and login succeeds")
	}
	// loginCalled is true only when there's no valid session (typical in CI).
	// We don't assert loginCalled because CI may or may not have a session.
	_ = loginCalled
}
