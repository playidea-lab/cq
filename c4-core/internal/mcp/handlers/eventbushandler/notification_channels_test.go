//go:build c3_eventbus

package eventbushandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/config"
)

// newTestCfgMgr creates a config.Manager from a YAML string for testing.
func newTestCfgMgr(t *testing.T, yaml string) *config.Manager {
	t.Helper()
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	mgr, err := config.New(tmpDir)
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	return mgr
}

// TestNotification_ResolveChannelDooray verifies Dooray channel injection.
func TestNotification_ResolveChannelDooray(t *testing.T) {
	cfgYAML := `
notifications:
  channels:
    - name: dooray-team
      type: dooray
      url: "https://hook.dooray.com/services/123/456"
      bot_name: C4Bot
`
	mgr := newTestCfgMgr(t, cfgYAML)

	result, err := resolveChannelConfig(`{"channel":"dooray-team"}`, mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(result), &obj); err != nil {
		t.Fatalf("result is not valid JSON: %s — %v", result, err)
	}
	if _, ok := obj["channel"]; ok {
		t.Error("channel key should have been removed from result")
	}
	if obj["url"] != "https://hook.dooray.com/services/123/456" {
		t.Errorf("expected dooray url, got %v", obj["url"])
	}
	template, _ := obj["payload_template"].(string)
	if template == "" {
		t.Error("expected payload_template to be set")
	}
	if !strings.Contains(template, `"botName"`) {
		t.Errorf("dooray template should contain botName, got: %s", template)
	}
	if obj["payload_content_type"] != "application/json" {
		t.Errorf("expected application/json content type, got %v", obj["payload_content_type"])
	}
}

// TestNotification_ResolveChannelDiscord verifies Discord channel injection.
func TestNotification_ResolveChannelDiscord(t *testing.T) {
	cfgYAML := `
notifications:
  channels:
    - name: discord-dev
      type: discord
      url: "https://discord.com/api/webhooks/999/abc"
      username: C4Robot
`
	mgr := newTestCfgMgr(t, cfgYAML)

	result, err := resolveChannelConfig(`{"channel":"discord-dev"}`, mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(result), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := obj["channel"]; ok {
		t.Error("channel key should have been removed")
	}
	if obj["url"] != "https://discord.com/api/webhooks/999/abc" {
		t.Errorf("expected discord url, got %v", obj["url"])
	}
	template, _ := obj["payload_template"].(string)
	if !strings.Contains(template, `"content"`) {
		t.Errorf("discord template should contain 'content', got: %s", template)
	}
}

// TestNotification_ResolveChannelNotFound verifies error for unknown channel.
func TestNotification_ResolveChannelNotFound(t *testing.T) {
	cfgYAML := `
notifications:
  channels: []
`
	mgr := newTestCfgMgr(t, cfgYAML)

	_, err := resolveChannelConfig(`{"channel":"nonexistent"}`, mgr)
	if err == nil {
		t.Fatal("expected error for nonexistent channel")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention channel name, got: %v", err)
	}
}

// TestNotification_ResolveChannelNoKey verifies backward compat when no channel key.
func TestNotification_ResolveChannelNoKey(t *testing.T) {
	mgr := newTestCfgMgr(t, "notifications:\n  channels: []\n")

	original := `{"url":"https://example.com/hook","headers":{"X-Token":"abc"}}`
	result, err := resolveChannelConfig(original, mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != original {
		t.Errorf("expected unchanged result, got: %s", result)
	}
}

// TestNotification_MaskURL verifies URL path masking.
func TestNotification_MaskURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://hook.dooray.com/services/123/456",
			expected: "https://hook.dooray.com/****",
		},
		{
			input:    "https://discord.com/api/webhooks/999/abc",
			expected: "https://discord.com/****",
		},
		{
			input:    "https://hooks.slack.com/services/T00/B00/xxx",
			expected: "https://hooks.slack.com/****",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tc := range tests {
		got := maskURL(tc.input)
		if got != tc.expected {
			t.Errorf("maskURL(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
