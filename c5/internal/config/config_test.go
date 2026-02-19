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
	if cfg.Storage.Path != "~/.local/share/c5" {
		t.Errorf("Storage.Path: got %q, want \"~/.local/share/c5\"", cfg.Storage.Path)
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
