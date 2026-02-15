package handlers

import (
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
)

func TestRegisterConfigHandlerNilManager(t *testing.T) {
	reg := mcp.NewRegistry()
	RegisterConfigHandler(reg, nil)
	if reg.HasTool("c4_config_get") {
		t.Error("should not register tool with nil manager")
	}
}

func TestConfigGetDefaultSection(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_KEY", "")
	tmpDir := t.TempDir()
	mgr, err := config.New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := handleConfigGet(mgr, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if _, ok := m["project_id"]; !ok {
		t.Error("missing project_id in 'all' section")
	}
	if _, ok := m["economic_mode"]; !ok {
		t.Error("missing economic_mode in 'all' section")
	}
}

func TestConfigGetEconomicSection(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_KEY", "")
	tmpDir := t.TempDir()
	mgr, err := config.New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := handleConfigGet(mgr, json.RawMessage(`{"section":"economic"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if _, ok := m["model_routing"]; !ok {
		t.Error("missing model_routing in economic section")
	}
}

func TestConfigGetCloudSectionMasksKey(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_KEY", "")
	tmpDir := t.TempDir()
	mgr, err := config.New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := handleConfigGet(mgr, json.RawMessage(`{"section":"cloud"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	// anon_key should be empty (not set), not masked
	if m["anon_key"] != "" {
		t.Errorf("anon_key = %v, want empty (not set)", m["anon_key"])
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret(""); got != "" {
		t.Errorf("maskSecret empty = %q, want empty", got)
	}
	if got := maskSecret("my-secret-key"); got != "***masked***" {
		t.Errorf("maskSecret non-empty = %q, want ***masked***", got)
	}
}

func TestMaskIfSecret(t *testing.T) {
	tests := []struct {
		key, val, want string
	}{
		{"url", "https://example.com", "https://example.com"},
		{"api_key", "sk-12345", "***masked***"},
		{"secret_token", "abc", "***masked***"},
		{"name", "", ""},
	}
	for _, tt := range tests {
		got := maskIfSecret(tt.key, tt.val)
		if got != tt.want {
			t.Errorf("maskIfSecret(%q, %q) = %q, want %q", tt.key, tt.val, got, tt.want)
		}
	}
}
