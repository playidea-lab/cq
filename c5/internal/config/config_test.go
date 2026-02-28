package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piqsol/c4/c5/internal/config"
)

func TestLoad_NoFile_ReturnsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "missing.yaml")

	cfg, err := config.Load(nonExistent)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	def := config.Default()
	if cfg.Server.Host != def.Server.Host {
		t.Errorf("Server.Host: got %q, want %q", cfg.Server.Host, def.Server.Host)
	}
	if cfg.Server.Port != def.Server.Port {
		t.Errorf("Server.Port: got %d, want %d", cfg.Server.Port, def.Server.Port)
	}
	if cfg.Storage.Path != def.Storage.Path {
		t.Errorf("Storage.Path: got %q, want %q", cfg.Storage.Path, def.Storage.Path)
	}
}

func TestLoad_EmptyPath_UsesDefault(t *testing.T) {
	// With empty path it falls back to XDG path which likely doesn't exist in CI;
	// if it doesn't exist we should get Default() without error.
	defaultPath := config.DefaultConfigPath()
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		cfg, err := config.Load("")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		def := config.Default()
		if cfg.Server.Port != def.Server.Port {
			t.Errorf("Server.Port: got %d, want %d", cfg.Server.Port, def.Server.Port)
		}
	}
}

func TestLoad_PartialYAML_FillsDefaults(t *testing.T) {
	yaml := `
server:
  port: 9090
`
	f := writeTempConfig(t, yaml)

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Overridden field.
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port: got %d, want 9090", cfg.Server.Port)
	}
	// Missing fields should retain defaults.
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host: got %q, want \"0.0.0.0\"", cfg.Server.Host)
	}
	home, _ := os.UserHomeDir()
	wantPath := filepath.Join(home, ".local", "share", "c5")
	if cfg.Storage.Path != wantPath {
		t.Errorf("Storage.Path: got %q, want %q", cfg.Storage.Path, wantPath)
	}
	if cfg.EventBus.URL != "" {
		t.Errorf("EventBus.URL: got %q, want \"\"", cfg.EventBus.URL)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	f := writeTempConfig(t, "server: [\nbad yaml")

	_, err := config.Load(f)
	if err == nil {
		t.Fatal("expected error for invalid yaml, got nil")
	}
}

func TestIsEventBusEnabled(t *testing.T) {
	cfg := config.Default()

	if cfg.IsEventBusEnabled() {
		t.Error("expected EventBus disabled by default")
	}

	cfg.EventBus.URL = "http://localhost:3000"
	if !cfg.IsEventBusEnabled() {
		t.Error("expected EventBus enabled when URL is set")
	}
}

func TestDefaultConfigPath_ContainsXDG(t *testing.T) {
	p := config.DefaultConfigPath()
	if !strings.Contains(p, ".config") {
		t.Errorf("DefaultConfigPath %q does not contain .config", p)
	}
	if !strings.HasSuffix(p, "c5.yaml") {
		t.Errorf("DefaultConfigPath %q does not end with c5.yaml", p)
	}
}

func TestExampleConfigYAML_NotEmpty(t *testing.T) {
	s := config.ExampleConfigYAML()
	if strings.TrimSpace(s) == "" {
		t.Error("ExampleConfigYAML returned empty string")
	}
}

func TestLoad_FullYAML(t *testing.T) {
	yaml := `
server:
  host: "127.0.0.1"
  port: 7777
eventbus:
  url: "http://eventbus:3000"
  token: "secret"
storage:
  path: "/tmp/c5-data"
`
	f := writeTempConfig(t, yaml)

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host: got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port: got %d", cfg.Server.Port)
	}
	if cfg.EventBus.URL != "http://eventbus:3000" {
		t.Errorf("EventBus.URL: got %q", cfg.EventBus.URL)
	}
	if cfg.EventBus.Token != "secret" {
		t.Errorf("EventBus.Token: got %q", cfg.EventBus.Token)
	}
	if cfg.Storage.Path != "/tmp/c5-data" {
		t.Errorf("Storage.Path: got %q", cfg.Storage.Path)
	}
	if !cfg.IsEventBusEnabled() {
		t.Error("expected EventBus enabled")
	}
}

func TestStorageConfigDefault(t *testing.T) {
	cfg := config.Default()

	home, _ := os.UserHomeDir()
	wantPath := filepath.Join(home, ".local", "share", "c5")
	if cfg.Storage.Path != wantPath {
		t.Errorf("Storage.Path: got %q, want %q", cfg.Storage.Path, wantPath)
	}
	if cfg.Storage.SupabaseURL != "" {
		t.Errorf("Storage.SupabaseURL: got %q, want \"\"", cfg.Storage.SupabaseURL)
	}
	if cfg.Storage.SupabaseKey != "" {
		t.Errorf("Storage.SupabaseKey: got %q, want \"\"", cfg.Storage.SupabaseKey)
	}
}

func TestIsSupabaseEnabled(t *testing.T) {
	cfg := config.Default()

	// Neither set — disabled.
	if cfg.IsSupabaseEnabled() {
		t.Error("expected Supabase disabled by default")
	}

	// Only URL set — still disabled.
	cfg.Storage.SupabaseURL = "https://project.supabase.co"
	if cfg.IsSupabaseEnabled() {
		t.Error("expected Supabase disabled when only URL is set")
	}

	// Only key set — still disabled.
	cfg.Storage.SupabaseURL = ""
	cfg.Storage.SupabaseKey = "service-role-key"
	if cfg.IsSupabaseEnabled() {
		t.Error("expected Supabase disabled when only key is set")
	}

	// Both set — enabled.
	cfg.Storage.SupabaseURL = "https://project.supabase.co"
	if !cfg.IsSupabaseEnabled() {
		t.Error("expected Supabase enabled when both URL and key are set")
	}
}

func TestIsLLMEnabled(t *testing.T) {
	cfg := config.Default()

	// Neither set — disabled.
	if cfg.IsLLMEnabled() {
		t.Error("expected LLM disabled by default")
	}

	// Only BaseURL set — still disabled.
	cfg.LLM.BaseURL = "https://api.example.com/v1"
	if cfg.IsLLMEnabled() {
		t.Error("expected LLM disabled when only BaseURL is set")
	}

	// Both set — enabled.
	cfg.LLM.APIKey = "sk-test"
	if !cfg.IsLLMEnabled() {
		t.Error("expected LLM enabled when both BaseURL and APIKey are set")
	}
}

func TestDoorayChannelParsing(t *testing.T) {
	t.Setenv("C5_DOORAY_CHANNELS", "ch-1=proj-a,ch-2=proj-b")

	cfg := config.Default()
	// Load from path that doesn't exist to force defaults + env overrides.
	loaded, err := config.Load(t.TempDir() + "/missing.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = cfg // unused, just verify loaded
	if len(loaded.Dooray.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(loaded.Dooray.Channels))
	}
	if loaded.Dooray.Channels["ch-1"].ProjectID != "proj-a" {
		t.Errorf("ch-1 projectID: got %q, want proj-a", loaded.Dooray.Channels["ch-1"].ProjectID)
	}
	if loaded.Dooray.Channels["ch-2"].ProjectID != "proj-b" {
		t.Errorf("ch-2 projectID: got %q, want proj-b", loaded.Dooray.Channels["ch-2"].ProjectID)
	}
}

func TestEnvOverrides_LLM(t *testing.T) {
	t.Setenv("C5_LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai")
	t.Setenv("C5_LLM_API_KEY", "gemini-api-key")
	t.Setenv("C5_LLM_MODEL", "gemini-2.0-flash")

	cfg, err := config.Load(t.TempDir() + "/missing.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLM.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Errorf("LLM.BaseURL: got %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "gemini-api-key" {
		t.Errorf("LLM.APIKey: got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "gemini-2.0-flash" {
		t.Errorf("LLM.Model: got %q", cfg.LLM.Model)
	}
	if !cfg.IsLLMEnabled() {
		t.Error("expected LLM enabled")
	}
}

func TestEnvOverrides_Dooray(t *testing.T) {
	t.Setenv("C5_DOORAY_WEBHOOK_URL", "https://dooray.example.com/webhook")
	t.Setenv("C5_DOORAY_CMD_TOKEN", "dooray-secret")

	cfg, err := config.Load(t.TempDir() + "/missing.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Dooray.WebhookURL != "https://dooray.example.com/webhook" {
		t.Errorf("Dooray.WebhookURL: got %q", cfg.Dooray.WebhookURL)
	}
	if cfg.Dooray.CmdToken != "dooray-secret" {
		t.Errorf("Dooray.CmdToken: got %q", cfg.Dooray.CmdToken)
	}
}

func TestLLMDefaults(t *testing.T) {
	cfg := config.Default()
	if cfg.LLM.Model != "gemini-3.0-flash" {
		t.Errorf("LLM.Model default: got %q, want gemini-3.0-flash", cfg.LLM.Model)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("LLM.MaxTokens default: got %d, want 4096", cfg.LLM.MaxTokens)
	}
}

// writeTempConfig writes content to a temp file and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "c5-config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
