package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigYaml(t *testing.T) {
	// Create temp directory with .c4/config.yaml
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
project_id: test-project
default_branch: develop
work_branch_prefix: "test/w-"
max_revision: 5
economic_mode:
  enabled: true
  preset: economic
`
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	cfg := mgr.GetConfig()

	if cfg.ProjectID != "test-project" {
		t.Errorf("ProjectID = %q, want %q", cfg.ProjectID, "test-project")
	}
	if cfg.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q, want %q", cfg.DefaultBranch, "develop")
	}
	if cfg.WorkBranchPrefix != "test/w-" {
		t.Errorf("WorkBranchPrefix = %q, want %q", cfg.WorkBranchPrefix, "test/w-")
	}
	if cfg.MaxRevision != 5 {
		t.Errorf("MaxRevision = %d, want %d", cfg.MaxRevision, 5)
	}
}

func TestEconomicPresetResolution(t *testing.T) {
	tests := []struct {
		name           string
		preset         string
		wantImpl       string
		wantReview     string
		wantCheckpoint string
	}{
		{
			name:           "standard preset",
			preset:         "standard",
			wantImpl:       "sonnet",
			wantReview:     "opus",
			wantCheckpoint: "opus",
		},
		{
			name:           "economic preset",
			preset:         "economic",
			wantImpl:       "sonnet",
			wantReview:     "sonnet",
			wantCheckpoint: "sonnet",
		},
		{
			name:           "ultra-economic preset",
			preset:         "ultra-economic",
			wantImpl:       "haiku",
			wantReview:     "sonnet",
			wantCheckpoint: "sonnet",
		},
		{
			name:           "quality preset",
			preset:         "quality",
			wantImpl:       "opus",
			wantReview:     "opus",
			wantCheckpoint: "opus",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			c4Dir := filepath.Join(tmpDir, ".c4")
			if err := os.MkdirAll(c4Dir, 0o755); err != nil {
				t.Fatal(err)
			}

			yaml := "project_id: test\neconomic_mode:\n  enabled: true\n  preset: " + tc.preset + "\n"
			if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
				t.Fatal(err)
			}

			mgr, err := New(tmpDir)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			cfg := mgr.GetConfig()

			if cfg.EconomicMode.ModelRouting.Implementation != tc.wantImpl {
				t.Errorf("Implementation = %q, want %q", cfg.EconomicMode.ModelRouting.Implementation, tc.wantImpl)
			}
			if cfg.EconomicMode.ModelRouting.Review != tc.wantReview {
				t.Errorf("Review = %q, want %q", cfg.EconomicMode.ModelRouting.Review, tc.wantReview)
			}
			if cfg.EconomicMode.ModelRouting.Checkpoint != tc.wantCheckpoint {
				t.Errorf("Checkpoint = %q, want %q", cfg.EconomicMode.ModelRouting.Checkpoint, tc.wantCheckpoint)
			}
		})
	}
}

func TestMissingConfigFileDefaults(t *testing.T) {
	// Isolate from host env (godotenv may have loaded .env with Supabase creds)
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_KEY", "")

	// Use a temp directory with no config file
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir) // isolate from global ~/.c4/config.yaml

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() should not fail with missing config: %v", err)
	}

	cfg := mgr.GetConfig()

	// Verify defaults are applied
	if cfg.ProjectID != "c4" {
		t.Errorf("ProjectID = %q, want %q", cfg.ProjectID, "c4")
	}
	if cfg.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", cfg.DefaultBranch, "main")
	}
	if cfg.MaxRevision != 3 {
		t.Errorf("MaxRevision = %d, want %d", cfg.MaxRevision, 3)
	}
	if !cfg.ReviewAsTask {
		t.Error("ReviewAsTask should be true by default")
	}
	if !cfg.CheckpointAsTask {
		t.Error("CheckpointAsTask should be true by default")
	}
	if cfg.EconomicMode.Enabled {
		t.Error("EconomicMode.Enabled should be false by default")
	}
	if backend := mgr.GetBackend(); backend != "sqlite" {
		t.Errorf("GetBackend() = %q, want %q", backend, "sqlite")
	}
}

func TestOverrideWithEnvVars(t *testing.T) {
	tmpDir := t.TempDir()

	// Set environment variables
	t.Setenv("C4_PROJECT_ID", "env-project")
	t.Setenv("C4_DEFAULT_BRANCH", "env-main")

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Viper's Get should return env var value
	if got := mgr.GetString("project_id"); got != "env-project" {
		t.Errorf("Get(project_id) = %q, want %q", got, "env-project")
	}
	if got := mgr.GetString("default_branch"); got != "env-main" {
		t.Errorf("Get(default_branch) = %q, want %q", got, "env-main")
	}
}

func TestGetModelForTask(t *testing.T) {
	t.Run("economic mode enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := "project_id: test\neconomic_mode:\n  enabled: true\n  preset: standard\n"
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// T- -> implementation (sonnet)
		if got := mgr.GetModelForTask("T-001-0"); got != "sonnet" {
			t.Errorf("T-001-0 -> %q, want %q", got, "sonnet")
		}

		// R- -> review (opus)
		if got := mgr.GetModelForTask("R-001-0"); got != "opus" {
			t.Errorf("R-001-0 -> %q, want %q", got, "opus")
		}

		// CP- -> checkpoint (opus)
		if got := mgr.GetModelForTask("CP-001"); got != "opus" {
			t.Errorf("CP-001 -> %q, want %q", got, "opus")
		}

		// RPR- -> implementation (sonnet)
		if got := mgr.GetModelForTask("RPR-001-1"); got != "sonnet" {
			t.Errorf("RPR-001-1 -> %q, want %q", got, "sonnet")
		}

		// RF- -> review (opus) — refine tasks use review model
		if got := mgr.GetModelForTask("RF-001-0"); got != "opus" {
			t.Errorf("RF-001-0 -> %q, want %q", got, "opus")
		}
	})

	t.Run("economic mode disabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// Should return empty when disabled
		if got := mgr.GetModelForTask("T-001-0"); got != "" {
			t.Errorf("T-001-0 -> %q, want empty", got)
		}
		if got := mgr.GetModelForTask("R-001-0"); got != "" {
			t.Errorf("R-001-0 -> %q, want empty", got)
		}
	})
}


func TestGetBackendDefault(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_KEY", "")

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if got := mgr.GetBackend(); got != "sqlite" {
		t.Errorf("GetBackend() = %q, want %q", got, "sqlite")
	}
}

func TestCloudConfig(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("SUPABASE_URL", "")
		t.Setenv("SUPABASE_KEY", "")

		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if cfg.Cloud.Enabled {
			t.Error("Cloud.Enabled should be false by default")
		}
		if got := mgr.GetBackend(); got != "sqlite" {
			t.Errorf("GetBackend() = %q, want %q", got, "sqlite")
		}
	})

	t.Run("enabled from yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := `project_id: test
cloud:
  enabled: true
  url: "https://abc.supabase.co"
  anon_key: "test-key-123"
  project_id: "cloud-proj"
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		cfg := mgr.GetConfig()
		if !cfg.Cloud.Enabled {
			t.Error("Cloud.Enabled should be true")
		}
		if cfg.Cloud.URL != "https://abc.supabase.co" {
			t.Errorf("Cloud.URL = %q, want %q", cfg.Cloud.URL, "https://abc.supabase.co")
		}
		if cfg.Cloud.AnonKey != "test-key-123" {
			t.Errorf("Cloud.AnonKey = %q, want %q", cfg.Cloud.AnonKey, "test-key-123")
		}
		if cfg.Cloud.ProjectID != "cloud-proj" {
			t.Errorf("Cloud.ProjectID = %q, want %q", cfg.Cloud.ProjectID, "cloud-proj")
		}
		if got := mgr.GetBackend(); got != "hybrid" {
			t.Errorf("GetBackend() = %q, want %q", got, "hybrid")
		}
	})

	t.Run("env var override", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("C4_CLOUD_ENABLED", "true")
		t.Setenv("C4_CLOUD_URL", "https://env.supabase.co")
		t.Setenv("C4_CLOUD_ANON_KEY", "env-key")

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		cfg := mgr.GetConfig()
		if !cfg.Cloud.Enabled {
			t.Error("Cloud.Enabled should be true from env var")
		}
		if cfg.Cloud.URL != "https://env.supabase.co" {
			t.Errorf("Cloud.URL = %q, want %q", cfg.Cloud.URL, "https://env.supabase.co")
		}
		if cfg.Cloud.AnonKey != "env-key" {
			t.Errorf("Cloud.AnonKey = %q, want %q", cfg.Cloud.AnonKey, "env-key")
		}
	})

	t.Run("SUPABASE_URL/KEY fallback", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := "project_id: test\ncloud:\n  enabled: true\n"
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		// Set SUPABASE_* env vars (not C4_CLOUD_*)
		t.Setenv("SUPABASE_URL", "https://fallback.supabase.co")
		t.Setenv("SUPABASE_KEY", "fallback-key")

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		cfg := mgr.GetConfig()
		if cfg.Cloud.URL != "https://fallback.supabase.co" {
			t.Errorf("Cloud.URL = %q, want %q", cfg.Cloud.URL, "https://fallback.supabase.co")
		}
		if cfg.Cloud.AnonKey != "fallback-key" {
			t.Errorf("Cloud.AnonKey = %q, want %q", cfg.Cloud.AnonKey, "fallback-key")
		}
	})
}

func TestLLMGatewayConfig(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if cfg.LLMGateway.Enabled {
			t.Error("LLMGateway.Enabled should be false by default")
		}
	})

	t.Run("enabled from yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := `project_id: test
llm_gateway:
  enabled: true
  default: anthropic
  providers:
    anthropic:
      enabled: true
      api_key_env: ANTHROPIC_API_KEY
    openai:
      enabled: false
      api_key_env: OPENAI_API_KEY
    ollama:
      enabled: true
      base_url: "http://localhost:11434"
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		cfg := mgr.GetConfig()
		if !cfg.LLMGateway.Enabled {
			t.Error("LLMGateway.Enabled should be true")
		}
		if cfg.LLMGateway.Default != "anthropic" {
			t.Errorf("LLMGateway.Default = %q, want %q", cfg.LLMGateway.Default, "anthropic")
		}
		if len(cfg.LLMGateway.Providers) != 3 {
			t.Fatalf("Providers count = %d, want 3", len(cfg.LLMGateway.Providers))
		}

		anthropic := cfg.LLMGateway.Providers["anthropic"]
		if !anthropic.Enabled {
			t.Error("anthropic provider should be enabled")
		}
		// api_key_env is no longer stored in LLMProviderConfig (field removed).
		// The deprecated field is detected via mgr.IsSet() in toLLMGatewayConfig.
		if !mgr.IsSet("llm_gateway.providers.anthropic.api_key_env") {
			t.Error("IsSet(api_key_env) should be true for deprecated field present in config.yaml")
		}

		openai := cfg.LLMGateway.Providers["openai"]
		if openai.Enabled {
			t.Error("openai provider should be disabled")
		}

		ollama := cfg.LLMGateway.Providers["ollama"]
		if !ollama.Enabled {
			t.Error("ollama provider should be enabled")
		}
		if ollama.BaseURL != "http://localhost:11434" {
			t.Errorf("ollama.BaseURL = %q, want %q", ollama.BaseURL, "http://localhost:11434")
		}
	})
}

func TestEventSinkConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if cfg.EventSink.Enabled {
			t.Error("EventSink.Enabled should be false by default")
		}
		if cfg.EventSink.Port != 4141 {
			t.Errorf("EventSink.Port = %d, want 4141", cfg.EventSink.Port)
		}
		if cfg.EventSink.Token != "" {
			t.Errorf("EventSink.Token = %q, want empty string", cfg.EventSink.Token)
		}
	})

	t.Run("yaml parsing", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		yaml := `project_id: test
eventsink:
  enabled: true
  port: 9999
  token: "secret"
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if !cfg.EventSink.Enabled {
			t.Error("EventSink.Enabled should be true")
		}
		if cfg.EventSink.Port != 9999 {
			t.Errorf("EventSink.Port = %d, want 9999", cfg.EventSink.Port)
		}
		if cfg.EventSink.Token != "secret" {
			t.Errorf("EventSink.Token = %q, want %q", cfg.EventSink.Token, "secret")
		}
	})

	t.Run("env override port enables eventsink", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("C4_EVENTSINK_PORT", "5000")
		t.Setenv("C4_EVENTSINK_TOKEN", "tok123")

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if !cfg.EventSink.Enabled {
			t.Error("EventSink.Enabled should be true when C4_EVENTSINK_PORT is set")
		}
		if cfg.EventSink.Port != 5000 {
			t.Errorf("EventSink.Port = %d, want 5000", cfg.EventSink.Port)
		}
		if cfg.EventSink.Token != "tok123" {
			t.Errorf("EventSink.Token = %q, want %q", cfg.EventSink.Token, "tok123")
		}
	})

	t.Run("env override port=0 disables eventsink", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// yaml enables it, but env sets port=0 → disabled
		yaml := `project_id: test
eventsink:
  enabled: true
  port: 4141
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("C4_EVENTSINK_PORT", "0")

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		cfg := mgr.GetConfig()
		if cfg.EventSink.Enabled {
			t.Error("EventSink.Enabled should be false when C4_EVENTSINK_PORT=0")
		}
		if cfg.EventSink.Port != 0 {
			t.Errorf("EventSink.Port = %d, want 0", cfg.EventSink.Port)
		}
	})
}

func TestRiskRouting(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		rr := mgr.GetRiskRouting()
		if rr.Enabled {
			t.Error("RiskRouting.Enabled should be false by default")
		}
		// Default models must be set even when disabled
		if rr.Models.High != "opus" {
			t.Errorf("Models.High = %q, want %q", rr.Models.High, "opus")
		}
		if rr.Models.Low != "sonnet" {
			t.Errorf("Models.Low = %q, want %q", rr.Models.Low, "sonnet")
		}
		if rr.Models.Default != "opus" {
			t.Errorf("Models.Default = %q, want %q", rr.Models.Default, "opus")
		}
	})

	t.Run("enabled from yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		configYAML := `
risk_routing:
  enabled: true
  paths:
    high: ["infra/", "internal/mcp/handlers/"]
    low: ["docs/", "user/", "*.md"]
  models:
    high: "opus"
    low: "sonnet"
    default: "opus"
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		rr := mgr.GetRiskRouting()
		if !rr.Enabled {
			t.Error("RiskRouting.Enabled should be true")
		}
		if len(rr.Paths.High) != 2 {
			t.Errorf("Paths.High len = %d, want 2", len(rr.Paths.High))
		}
		if len(rr.Paths.Low) != 3 {
			t.Errorf("Paths.Low len = %d, want 3", len(rr.Paths.Low))
		}
		if rr.Models.High != "opus" {
			t.Errorf("Models.High = %q, want %q", rr.Models.High, "opus")
		}
		if rr.Models.Low != "sonnet" {
			t.Errorf("Models.Low = %q, want %q", rr.Models.Low, "sonnet")
		}
		if rr.Models.Default != "opus" {
			t.Errorf("Models.Default = %q, want %q", rr.Models.Default, "opus")
		}
	})

	t.Run("independent of economic mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		c4Dir := filepath.Join(tmpDir, ".c4")
		if err := os.MkdirAll(c4Dir, 0o755); err != nil {
			t.Fatal(err)
		}

		// economic_mode disabled, risk_routing enabled
		configYAML := `
economic_mode:
  enabled: false
  preset: economic
risk_routing:
  enabled: true
  models:
    high: "opus"
    low: "haiku"
    default: "sonnet"
`
		if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}

		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// EconomicMode disabled → GetModelForTask returns ""
		if got := mgr.GetModelForTask("R-001-0"); got != "" {
			t.Errorf("GetModelForTask(R-001-0) = %q, want empty (economic mode disabled)", got)
		}
		// RiskRouting enabled independently
		rr := mgr.GetRiskRouting()
		if !rr.Enabled {
			t.Error("RiskRouting.Enabled should be true")
		}
		if rr.Models.Default != "sonnet" {
			t.Errorf("Models.Default = %q, want %q", rr.Models.Default, "sonnet")
		}
	})

	t.Run("GetRiskRouting returns value not pointer", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := New(tmpDir)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		// Verify it returns a value type (no nil pointer panic possible)
		rr := mgr.GetRiskRouting()
		_ = rr.Enabled
		_ = rr.Models.High
		_ = rr.Paths.High
	})
}

func TestStaleCheckerConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	cfg := mgr.GetConfig()
	sc := cfg.Serve.StaleChecker

	if sc.Enabled {
		t.Error("StaleChecker.Enabled should be false by default")
	}
	if sc.ThresholdMinutes != 30 {
		t.Errorf("ThresholdMinutes = %d, want 30", sc.ThresholdMinutes)
	}
	if sc.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds = %d, want 60", sc.IntervalSeconds)
	}
}

// TestNotificationBuildPayloadTemplate_Discord tests discord payload generation.
func TestNotificationBuildPayloadTemplate_Discord(t *testing.T) {
	ch := NotificationChannel{
		Type:            "discord",
		Username:        "CQ Bot",
		MessageTemplate: "**[{{event_type}}]** {{title}} ({{task_id}})",
	}
	payload, ct := BuildPayloadTemplate(ch)
	if ct != "application/json" {
		t.Errorf("contentType = %q, want application/json", ct)
	}
	if !contains(payload, `"content":`) {
		t.Errorf("payload missing content: %s", payload)
	}
	if !contains(payload, `"username":"CQ Bot"`) {
		t.Errorf("payload missing username: %s", payload)
	}
}

// TestNotificationBuildPayloadTemplate_Slack tests slack payload generation.
func TestNotificationBuildPayloadTemplate_Slack(t *testing.T) {
	ch := NotificationChannel{
		Type:            "slack",
		MessageTemplate: "[{{event_type}}] {{title}} - {{task_id}}",
	}
	payload, ct := BuildPayloadTemplate(ch)
	if ct != "application/json" {
		t.Errorf("contentType = %q, want application/json", ct)
	}
	if !contains(payload, `"text":`) {
		t.Errorf("payload missing text: %s", payload)
	}
	if contains(payload, `"username"`) {
		t.Errorf("slack payload should not have username: %s", payload)
	}
}

// TestNotificationBuildPayloadTemplate_Generic tests generic payload passthrough.
func TestNotificationBuildPayloadTemplate_Generic(t *testing.T) {
	rawPayload := `{"custom":"value","msg":"hello"}`
	ch := NotificationChannel{
		Type:            "generic",
		PayloadTemplate: rawPayload,
		ContentType:     "application/json",
	}
	payload, ct := BuildPayloadTemplate(ch)
	if payload != rawPayload {
		t.Errorf("generic payload = %q, want %q", payload, rawPayload)
	}
	if ct != "application/json" {
		t.Errorf("contentType = %q, want application/json", ct)
	}
}

// TestNotificationBuildPayloadTemplate_DefaultMessageTemplate tests that omitting
// message_template falls back to the type-specific default.
func TestNotificationBuildPayloadTemplate_DefaultMessageTemplate(t *testing.T) {
	cases := []struct {
		chType      string
		wantContain string
	}{
		{"discord", "{{title}} ({{task_id}})"},
		{"slack", "[{{event_type}}] {{title}} - {{task_id}}"},
		{"teams", "[{{event_type}}] {{title}}"},
	}
	for _, tc := range cases {
		ch := NotificationChannel{Type: tc.chType}
		payload, _ := BuildPayloadTemplate(ch)
		if !contains(payload, tc.wantContain) {
			t.Errorf("type=%s: payload %q missing %q", tc.chType, payload, tc.wantContain)
		}
	}
}

// TestNotificationBuildPayloadTemplate_JSONEscaping tests that special characters
// in BotName, Username, and MessageTemplate are properly JSON-escaped.
func TestNotificationBuildPayloadTemplate_JSONEscaping(t *testing.T) {
	cases := []struct {
		name        string
		ch          NotificationChannel
		wantContain string
	}{
		{
			name: "discord username with double quote",
			ch: NotificationChannel{
				Type:            "discord",
				Username:        `CQ"Bot`,
				MessageTemplate: "msg",
			},
			wantContain: `"username":"CQ\"Bot"`,
		},
		{
			name: "message template with newline",
			ch: NotificationChannel{
				Type:            "slack",
				MessageTemplate: "line1\nline2",
			},
			wantContain: `"text":"line1\nline2"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := BuildPayloadTemplate(tc.ch)
			if !contains(payload, tc.wantContain) {
				t.Errorf("payload = %q, want substring %q", payload, tc.wantContain)
			}
		})
	}
}

// TestNotificationGetChannel tests GetNotificationChannel helper.
func TestNotificationGetChannel(t *testing.T) {
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := `
notifications:
  channels:
    - name: discord-dev
      type: discord
      url: "https://discord.com/api/webhooks/test"
      username: "CQ Bot"
`
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ch, ok := mgr.GetNotificationChannel("discord-dev")
	if !ok {
		t.Fatal("expected to find discord-dev channel, got not found")
	}
	if ch.Type != "discord" {
		t.Errorf("Type = %q, want discord", ch.Type)
	}
	if ch.Username != "CQ Bot" {
		t.Errorf("Username = %q, want CQ Bot", ch.Username)
	}

	_, missing := mgr.GetNotificationChannel("does-not-exist")
	if missing {
		t.Error("expected not found for missing channel")
	}
}

// TestNotificationBackwardCompat tests that configs without notifications still load fine.
func TestNotificationBackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()
	c4Dir := filepath.Join(tmpDir, ".c4")
	if err := os.MkdirAll(c4Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := `
project_id: legacy-project
`
	if err := os.WriteFile(filepath.Join(c4Dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if len(cfg.Notifications.Channels) != 0 {
		t.Errorf("expected 0 channels in legacy config, got %d", len(cfg.Notifications.Channels))
	}
}

// contains is a helper for substring checks in tests.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

