package config

import (
	"testing"
)

func TestApplyEnvOverrides_URL(t *testing.T) {
	cfg := Default()
	t.Setenv("C5_SUPABASE_URL", "https://example.supabase.co")
	applyEnvOverrides(&cfg)
	if cfg.Storage.SupabaseURL != "https://example.supabase.co" {
		t.Errorf("expected SupabaseURL %q, got %q", "https://example.supabase.co", cfg.Storage.SupabaseURL)
	}
}

func TestApplyEnvOverrides_Key(t *testing.T) {
	cfg := Default()
	t.Setenv("C5_SUPABASE_KEY", "anon-key-abc123")
	applyEnvOverrides(&cfg)
	if cfg.Storage.SupabaseKey != "anon-key-abc123" {
		t.Errorf("expected SupabaseKey %q, got %q", "anon-key-abc123", cfg.Storage.SupabaseKey)
	}
}

func TestApplyEnvOverrides_Port(t *testing.T) {
	cfg := Default()
	t.Setenv("C5_PORT", "9090")
	applyEnvOverrides(&cfg)
	if cfg.Server.Port != 9090 {
		t.Errorf("expected Port 9090, got %d", cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_InvalidPort(t *testing.T) {
	cfg := Default()
	original := cfg.Server.Port
	t.Setenv("C5_PORT", "abc")
	applyEnvOverrides(&cfg)
	if cfg.Server.Port != original {
		t.Errorf("expected Port %d (unchanged), got %d", original, cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_Unset(t *testing.T) {
	cfg := Default()
	before := cfg
	applyEnvOverrides(&cfg)
	if cfg.Storage.SupabaseURL != before.Storage.SupabaseURL {
		t.Errorf("SupabaseURL should not change when env var unset")
	}
	if cfg.Storage.SupabaseKey != before.Storage.SupabaseKey {
		t.Errorf("SupabaseKey should not change when env var unset")
	}
	if cfg.Server.Port != before.Server.Port {
		t.Errorf("Port should not change when env var unset")
	}
}
