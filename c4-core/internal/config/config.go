// Package config handles C4 project configuration loading and validation.
//
// Configuration is loaded from .c4/config.yaml and includes:
//   - Project metadata (name, domain, description)
//   - Validation rules (lint, unit test, integration test commands)
//   - Worker settings (max workers, heartbeat interval, TTL)
//   - Economic mode presets (standard, economic, quality, etc.)
//
// Environment variable overrides use the C4_ prefix (e.g., C4_PROJECT_ID).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ModelRouting maps task type prefixes to Claude model names.
type ModelRouting struct {
	Implementation string `mapstructure:"implementation" yaml:"implementation"`
	Review         string `mapstructure:"review"         yaml:"review"`
	Checkpoint     string `mapstructure:"checkpoint"     yaml:"checkpoint"`
	Scout          string `mapstructure:"scout"          yaml:"scout"`    // Used by c4-plan scout phase (no task prefix yet)
	Debug          string `mapstructure:"debug"          yaml:"debug"`    // Used by c4-refine debug rounds (no task prefix yet)
	Planning       string `mapstructure:"planning"       yaml:"planning"` // Used by c4-plan planning phase (no task prefix yet)
}

// EconomicMode holds economic mode configuration.
type EconomicMode struct {
	Enabled      bool         `mapstructure:"enabled"       yaml:"enabled"`
	Preset       string       `mapstructure:"preset"        yaml:"preset"`
	ModelRouting ModelRouting `mapstructure:"model_routing" yaml:"model_routing"`
}

// CloudConfig holds cloud (Supabase) connection settings.
type CloudConfig struct {
	Enabled      bool   `mapstructure:"enabled"       yaml:"enabled"`
	URL          string `mapstructure:"url"            yaml:"url"`            // Supabase project URL
	AnonKey      string `mapstructure:"anon_key"       yaml:"anon_key"`       // from env C4_CLOUD_ANON_KEY
	ProjectID    string `mapstructure:"project_id"     yaml:"project_id"`     // cloud project identifier
	BucketName   string `mapstructure:"bucket_name"    yaml:"bucket_name"`    // default "c4-drive"
	Mode         string `mapstructure:"mode"           yaml:"mode"`           // "local-first" (default) or "cloud-primary"
}

// LLMProviderConfig holds per-provider settings.
// API keys are stored in the secret store (~/.c4/secrets.db) via "cq secret set <provider>.api_key".
type LLMProviderConfig struct {
	Enabled      bool   `mapstructure:"enabled"       yaml:"enabled"`
	BaseURL      string `mapstructure:"base_url"      yaml:"base_url"`
	DefaultModel string `mapstructure:"default_model" yaml:"default_model"`
}

// LLMGatewayConfig holds LLM gateway settings.
type LLMGatewayConfig struct {
	Enabled        bool                         `mapstructure:"enabled"          yaml:"enabled"`
	Default        string                       `mapstructure:"default"          yaml:"default"`
	CacheByDefault bool                         `mapstructure:"cache_by_default" yaml:"cache_by_default"`
	Providers      map[string]LLMProviderConfig `mapstructure:"providers"        yaml:"providers"`
}

// WorktreeConfig holds worktree settings.
type WorktreeConfig struct {
	Enabled     bool `mapstructure:"enabled"      yaml:"enabled"`
	AutoCleanup bool `mapstructure:"auto_cleanup" yaml:"auto_cleanup"`
}

// CritiqueLoopConfig controls the Plan Critique Loop (Phase 4.5) in c4-plan.
type CritiqueLoopConfig struct {
	Enabled   bool   `mapstructure:"enabled"    yaml:"enabled"`
	MaxRounds int    `mapstructure:"max_rounds" yaml:"max_rounds"`
	// Mode: "auto" (run silently), "interactive" (pause per round), "skip" (disable loop)
	Mode      string `mapstructure:"mode"       yaml:"mode"`
}

// ResearchLoopConfig controls the autonomous research loop (LoopOrchestrator).
type ResearchLoopConfig struct {
	// GateDuration is the default wait duration between debate rounds.
	// Accepts Go duration strings (e.g. "24h", "30m"). Default: "24h".
	GateDuration string `mapstructure:"gate_duration" yaml:"gate_duration"`
	// Patience is the default max rounds without improvement before convergence.
	// 0 means no convergence check. Default: 0.
	Patience int `mapstructure:"patience" yaml:"patience"`
	// MetricLowerIsBetter sets the direction for metric comparison.
	// true = lower metric is better (e.g. loss). nil = default (true).
	MetricLowerIsBetter *bool `mapstructure:"metric_lower_is_better" yaml:"metric_lower_is_better"`
	// ConvergenceThreshold is the minimum improvement to reset patience count.
	// Default: 0.5.
	ConvergenceThreshold float64 `mapstructure:"convergence_threshold" yaml:"convergence_threshold"`
}

// PlanningConfig controls c4-plan skill behavior.
type PlanningConfig struct {
	CritiqueLoop CritiqueLoopConfig `mapstructure:"critique_loop" yaml:"critique_loop"`
}

// ValidationConfig holds validation command settings.
type ValidationConfig struct {
	Lint string `mapstructure:"lint" yaml:"lint"`
	Unit string `mapstructure:"unit" yaml:"unit"`
}

// EventBusConfig holds C3 EventBus settings.
type EventBusConfig struct {
	Enabled       bool   `mapstructure:"enabled"        yaml:"enabled"`
	AutoStart     bool   `mapstructure:"auto_start"     yaml:"auto_start"`
	SocketPath    string `mapstructure:"socket_path"    yaml:"socket_path"`
	DataDir       string `mapstructure:"data_dir"       yaml:"data_dir"`
	RetentionDays int    `mapstructure:"retention_days" yaml:"retention_days"` // 0 = no auto-purge
	MaxEvents     int    `mapstructure:"max_events"     yaml:"max_events"`     // 0 = unlimited
	WSPort        int    `mapstructure:"ws_port"        yaml:"ws_port"`        // 0 = WebSocket bridge disabled
	WSHost        string `mapstructure:"ws_host"        yaml:"ws_host"`        // default "127.0.0.1"
}

// HubConfig holds PiQ Hub connection settings.
type HubConfig struct {
	Enabled   bool   `mapstructure:"enabled"     yaml:"enabled"`
	URL       string `mapstructure:"url"         yaml:"url"`
	APIPrefix string `mapstructure:"api_prefix"  yaml:"api_prefix"` // "/v1" for Hub server, "" for local daemon
	APIKey    string `mapstructure:"api_key"     yaml:"api_key"`
	APIKeyEnv string `mapstructure:"api_key_env" yaml:"api_key_env"`
	TeamID    string `mapstructure:"team_id"     yaml:"team_id"`
}

// EventSinkConfig holds EventSink HTTP server settings.
// EventSink receives events from C5 Hub and publishes them to the local EventBus.
type EventSinkConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"` // default false
	Port    int    `mapstructure:"port"    yaml:"port"`    // default 4141
	Token   string `mapstructure:"token"   yaml:"token"`   // default "", no auth
}

// GateSlackConnectorConfig holds Slack connector settings.
type GateSlackConnectorConfig struct {
	Enabled    bool   `mapstructure:"enabled"     yaml:"enabled"`
	WebhookURL string `mapstructure:"webhook_url" yaml:"webhook_url"`
}

// GateGitHubConnectorConfig holds GitHub connector settings.
type GateGitHubConnectorConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	PAT     string `mapstructure:"pat"     yaml:"pat"`
}

// GateConnectorsConfig groups all connector configs.
type GateConnectorsConfig struct {
	Slack  GateSlackConnectorConfig  `mapstructure:"slack"  yaml:"slack"`
	GitHub GateGitHubConnectorConfig `mapstructure:"github" yaml:"github"`
}

// GateConfig holds C8 Gate settings (webhooks, scheduler, connectors).
type GateConfig struct {
	Enabled    bool                 `mapstructure:"enabled"    yaml:"enabled"`
	Connectors GateConnectorsConfig `mapstructure:"connectors" yaml:"connectors"`
}

// ObserveConfig holds C7 observability settings (logging, metrics, health).
type ObserveConfig struct {
	Enabled   bool   `mapstructure:"enabled"    yaml:"enabled"`
	LogLevel  string `mapstructure:"log_level"  yaml:"log_level"`  // debug, info, warn, error
	LogFormat string `mapstructure:"log_format" yaml:"log_format"` // json, text
}

// GuardPolicyRule mirrors guard.PolicyRule for YAML-based configuration.
type GuardPolicyRule struct {
	Tool     string `mapstructure:"tool"     yaml:"tool"`
	Action   string `mapstructure:"action"   yaml:"action"`   // allow | deny | audit_only
	Reason   string `mapstructure:"reason"   yaml:"reason"`
	Priority int    `mapstructure:"priority" yaml:"priority"`
}

// GuardConfig holds C6 guard engine settings.
type GuardConfig struct {
	Enabled        bool              `mapstructure:"enabled"          yaml:"enabled"`
	DefaultPolicy  string            `mapstructure:"default_policy"   yaml:"default_policy"`  // allow | deny | audit_only
	AuditRetention string            `mapstructure:"audit_retention"  yaml:"audit_retention"` // e.g. "30d"
	Policies       []GuardPolicyRule `mapstructure:"policies"         yaml:"policies"`
}

// NotificationChannel holds a single notification destination configuration.
type NotificationChannel struct {
	Name            string            `mapstructure:"name"             yaml:"name"`
	Type            string            `mapstructure:"type"             yaml:"type"` // dooray|discord|slack|teams|generic
	URL             string            `mapstructure:"url"              yaml:"url"`
	MessageTemplate string            `mapstructure:"message_template" yaml:"message_template"`
	BotName         string            `mapstructure:"bot_name"         yaml:"bot_name"`
	Username        string            `mapstructure:"username"         yaml:"username"`
	Headers         map[string]string `mapstructure:"headers"          yaml:"headers"`
	PayloadTemplate string            `mapstructure:"payload_template" yaml:"payload_template"` // generic only
	ContentType     string            `mapstructure:"content_type"     yaml:"content_type"`     // generic optional
}

// defaultMessageTemplate returns the type-specific default message template.
func defaultMessageTemplate(channelType string) string {
	switch channelType {
	case "dooray":
		return "[{{event_type}}] {{title}}"
	case "discord":
		return "**[{{event_type}}]** {{title}} ({{task_id}})"
	case "slack":
		return "[{{event_type}}] {{title}} - {{task_id}}"
	case "teams":
		return "[{{event_type}}] {{title}}"
	default:
		return "[{{event_type}}] {{title}}"
	}
}

// jsonStr returns the JSON-encoded string content of s (without surrounding quotes).
// Uses encoding/json.Marshal to properly escape \, ", \n, \r, \t, and control chars.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1]) // strip surrounding quotes
}

// BuildPayloadTemplate returns (payload, contentType) for the given channel.
// For typed channels it auto-generates the payload from message_template.
// For generic channels it returns PayloadTemplate as-is.
func BuildPayloadTemplate(ch NotificationChannel) (string, string) {
	msg := ch.MessageTemplate
	if msg == "" {
		msg = defaultMessageTemplate(ch.Type)
	}

	switch ch.Type {
	case "dooray":
		payload := `{"botName":"` + jsonStr(ch.BotName) + `","text":"` + jsonStr(msg) + `"}`
		return payload, "application/json"
	case "discord":
		payload := `{"content":"` + jsonStr(msg) + `","username":"` + jsonStr(ch.Username) + `"}`
		return payload, "application/json"
	case "slack":
		payload := `{"text":"` + jsonStr(msg) + `"}`
		return payload, "application/json"
	case "teams":
		payload := `{"@type":"MessageCard","text":"` + jsonStr(msg) + `"}`
		return payload, "application/json"
	default: // generic
		ct := ch.ContentType
		if ct == "" {
			ct = "application/json"
		}
		return ch.PayloadTemplate, ct
	}
}

// NotificationsConfig holds all notification channel configurations.
type NotificationsConfig struct {
	Channels []NotificationChannel `mapstructure:"channels" yaml:"channels"`
}

// RiskPathsConfig holds scope path lists for risk classification.
type RiskPathsConfig struct {
	High []string `mapstructure:"high" yaml:"high"`
	Low  []string `mapstructure:"low"  yaml:"low"`
}

// RiskModelsConfig holds model aliases for each risk tier.
type RiskModelsConfig struct {
	High    string `mapstructure:"high"    yaml:"high"`
	Low     string `mapstructure:"low"     yaml:"low"`
	Default string `mapstructure:"default" yaml:"default"`
}

// RiskRoutingConfig controls scope-based reviewer model selection for R- tasks.
// Independent of EconomicMode.ModelRouting — scope-based override only.
type RiskRoutingConfig struct {
	Enabled bool             `mapstructure:"enabled" yaml:"enabled"`
	Paths   RiskPathsConfig  `mapstructure:"paths"   yaml:"paths"`
	Models  RiskModelsConfig `mapstructure:"models"  yaml:"models"`
}

// SessionsConfig holds session limit warning settings.
type SessionsConfig struct {
	Limit   int  `mapstructure:"limit"   yaml:"limit"`   // warn when active session count exceeds this (default 4)
	Enabled bool `mapstructure:"enabled" yaml:"enabled"` // false → skip the check entirely
}

// ServeComponentToggle holds a single enabled flag for a serve sub-component.
type ServeComponentToggle struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

// StaleCheckerConfig holds configuration for the stale task checker component.
type StaleCheckerConfig struct {
	Enabled          bool `mapstructure:"enabled"           yaml:"enabled"`
	ThresholdMinutes int  `mapstructure:"threshold_minutes" yaml:"threshold_minutes"` // default 30
	IntervalSeconds  int  `mapstructure:"interval_seconds"  yaml:"interval_seconds"`  // default 60
}

// ServeHubConfig holds configuration for the C5 Hub subprocess component.
type ServeHubConfig struct {
	Enabled bool     `mapstructure:"enabled" yaml:"enabled"`
	Binary  string   `mapstructure:"binary"  yaml:"binary"`  // default: "c5"
	Port    int      `mapstructure:"port"    yaml:"port"`    // default: 8585
	Args    []string `mapstructure:"args"    yaml:"args"`    // extra CLI args
}

// ServeMCPHTTPConfig holds configuration for the Streamable HTTP MCP endpoint.
// This exposes the same MCP tools as the stdio transport over HTTP,
// enabling remote Claude Code instances to connect as MCP clients.
type ServeMCPHTTPConfig struct {
	Enabled bool   `mapstructure:"enabled"  yaml:"enabled"`
	Port    int    `mapstructure:"port"     yaml:"port"`    // default: 4142 (4141 is used by EventSink)
	Bind    string `mapstructure:"bind"     yaml:"bind"`    // default: "127.0.0.1"
	APIKey  string `mapstructure:"api_key"  yaml:"api_key"` // dev/test only; prefer secrets.db or CQ_MCP_API_KEY env
}

// ServeHypothesisSuggesterConfig holds settings for the hypothesis suggester component.
type ServeHypothesisSuggesterConfig struct {
	Enabled   bool          `mapstructure:"enabled"    yaml:"enabled"`
	Threshold int           `mapstructure:"threshold"  yaml:"threshold"` // number of new experiments to trigger
	Interval  string        `mapstructure:"interval"   yaml:"interval"`  // poll interval, e.g. "30s"
	TTL       time.Duration `mapstructure:"ttl"        yaml:"ttl"`       // hypothesis expiry duration
}

// ServeSecretsConfig holds settings for the secrets-sync serve component.
// EnvInject lists secret keys to inject as env vars into C5 subprocess on startup.
// Example: ["anthropic.api_key", "openai.api_key"]
type ServeSecretsConfig struct {
	EnvInject []string `mapstructure:"env_inject" yaml:"env_inject"`
}

// ServeConfig holds settings for the cq serve command.
type ServeConfig struct {
	HealthPort          int                             `mapstructure:"health_port"           yaml:"health_port"`
	Agent               ServeComponentToggle            `mapstructure:"agent"                 yaml:"agent"`
	EventBus            ServeComponentToggle            `mapstructure:"eventbus"              yaml:"eventbus"`
	EventSink           ServeComponentToggle            `mapstructure:"eventsink"             yaml:"eventsink"`
	HubPoller           ServeComponentToggle            `mapstructure:"hubpoller"             yaml:"hubpoller"`
	GPU                 ServeComponentToggle            `mapstructure:"gpu"                   yaml:"gpu"`
	SSESubscriber       ServeComponentToggle            `mapstructure:"ssesubscriber"         yaml:"ssesubscriber"`
	StaleChecker        StaleCheckerConfig              `mapstructure:"stale_checker"         yaml:"stale_checker"`
	Hub                 ServeHubConfig                  `mapstructure:"hub"                   yaml:"hub"`
	MCPHTTP             ServeMCPHTTPConfig              `mapstructure:"mcp_http"              yaml:"mcp_http"`
	HypothesisSuggester ServeHypothesisSuggesterConfig  `mapstructure:"hypothesis_suggester"  yaml:"hypothesis_suggester"`
	Secrets             ServeSecretsConfig              `mapstructure:"secrets"               yaml:"secrets"`
	ResearchLoop        ResearchLoopConfig              `mapstructure:"research_loop"         yaml:"research_loop"`
}

// PermissionReviewerConfig holds settings for the permission auto-reviewer hook.
// SSOT: .c4/config.yaml → permission_reviewer section.
// Runtime: .c4/hook-config.json (generated by MCP server on startup).
type PermissionReviewerConfig struct {
	Enabled       bool     `mapstructure:"enabled"        yaml:"enabled"`
	Mode          string   `mapstructure:"mode"           yaml:"mode"`           // "hook" (regex only) or "model" (LLM API)
	Model         string   `mapstructure:"model"          yaml:"model"`          // claude model: haiku, sonnet, opus (or full model ID)
	APIKeyEnv     string   `mapstructure:"api_key_env"    yaml:"api_key_env"`    // env var name for Anthropic API key
	FailMode      string   `mapstructure:"fail_mode"      yaml:"fail_mode"`      // "ask" (prompt user) or "allow" (auto-approve on failure)
	Timeout       int      `mapstructure:"timeout"        yaml:"timeout"`        // API call timeout in seconds
	AutoApprove   bool     `mapstructure:"auto_approve"   yaml:"auto_approve"`   // auto-approve safe commands without user prompt
	AllowPatterns []string `mapstructure:"allow_patterns" yaml:"allow_patterns"` // regex patterns always allowed (checked first)
	BlockPatterns []string `mapstructure:"block_patterns" yaml:"block_patterns"` // regex patterns always blocked
}

// C4Config is the top-level configuration schema.
// It mirrors the Python C4Config model for YAML format compatibility.
type C4Config struct {
	ProjectID        string           `mapstructure:"project_id"          yaml:"project_id"`
	DefaultBranch    string           `mapstructure:"default_branch"      yaml:"default_branch"`
	WorkBranchPrefix string           `mapstructure:"work_branch_prefix"  yaml:"work_branch_prefix"`
	Domain           string           `mapstructure:"domain"              yaml:"domain"`
	MaxRevision      int              `mapstructure:"max_revision"        yaml:"max_revision"`
	Validation       ValidationConfig `mapstructure:"validation"          yaml:"validation"`
	Worktree         WorktreeConfig   `mapstructure:"worktree"            yaml:"worktree"`
	EconomicMode     EconomicMode     `mapstructure:"economic_mode"       yaml:"economic_mode"`
	Cloud            CloudConfig      `mapstructure:"cloud"               yaml:"cloud"`
	LLMGateway       LLMGatewayConfig `mapstructure:"llm_gateway"         yaml:"llm_gateway"`
	EventBus         EventBusConfig   `mapstructure:"eventbus"            yaml:"eventbus"`
	Hub              HubConfig                `mapstructure:"hub"                  yaml:"hub"`
	EventSink        EventSinkConfig          `mapstructure:"eventsink"            yaml:"eventsink"`
	PermissionReviewer PermissionReviewerConfig `mapstructure:"permission_reviewer"  yaml:"permission_reviewer"`
	ReviewAsTask     bool                       `mapstructure:"review_as_task"       yaml:"review_as_task"`
	CheckpointAsTask bool                       `mapstructure:"checkpoint_as_task"  yaml:"checkpoint_as_task"`
	Planning         PlanningConfig             `mapstructure:"planning"             yaml:"planning"`
	Gate             GateConfig                 `mapstructure:"gate"                 yaml:"gate"`
	Observe          ObserveConfig              `mapstructure:"observe"              yaml:"observe"`
	Guard            GuardConfig                `mapstructure:"guard"                yaml:"guard"`
	Serve            ServeConfig                `mapstructure:"serve"                yaml:"serve"`
	Sessions         SessionsConfig             `mapstructure:"sessions"             yaml:"sessions"`
	RiskRouting      RiskRoutingConfig          `mapstructure:"risk_routing"         yaml:"risk_routing"`
	Notifications    NotificationsConfig        `mapstructure:"notifications"        yaml:"notifications"`
}

// presetConfigs defines the economic mode presets.
// These match the Python PRESET_CONFIGS dictionary exactly.
var presetConfigs = map[string]ModelRouting{
	"standard": {
		Implementation: "sonnet",
		Review:         "opus",
		Checkpoint:     "opus",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "sonnet",
	},
	"economic": {
		Implementation: "sonnet",
		Review:         "sonnet",
		Checkpoint:     "sonnet",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "sonnet",
	},
	"ultra-economic": {
		Implementation: "haiku",
		Review:         "sonnet",
		Checkpoint:     "sonnet",
		Scout:          "haiku",
		Debug:          "haiku",
		Planning:       "haiku",
	},
	"quality": {
		Implementation: "opus",
		Review:         "opus",
		Checkpoint:     "opus",
		Scout:          "sonnet",
		Debug:          "sonnet",
		Planning:       "opus",
	},
}

// defaultConfig returns a C4Config with sane defaults.
func defaultConfig() C4Config {
	return C4Config{
		ProjectID:        "c4",
		DefaultBranch:    "main",
		WorkBranchPrefix: "c4/w-",
		MaxRevision:      3,
		ReviewAsTask:     true,
		CheckpointAsTask: true,
		Worktree: WorktreeConfig{
			Enabled:     true,
			AutoCleanup: true,
		},
		EconomicMode: EconomicMode{
			Enabled:      false,
			Preset:       "economic",
			ModelRouting: presetConfigs["standard"],
		},
		EventSink: EventSinkConfig{
			Enabled: false,
			Port:    4141,
		},
		Planning: PlanningConfig{
			CritiqueLoop: CritiqueLoopConfig{
				Enabled:   true,
				MaxRounds: 3,
				Mode:      "auto",
			},
		},
		Observe: ObserveConfig{
			Enabled:   true,
			LogLevel:  "info",
			LogFormat: "json",
		},
		Serve: ServeConfig{
			HealthPort: 4140,
		},
		Sessions: SessionsConfig{
			Limit:   4,
			Enabled: true,
		},
	}
}

// CloudDefaults holds built-in Supabase credentials injected at build time.
// These are PUBLIC values (anon key + RLS = safe to embed in binary).
type CloudDefaults struct {
	URL     string
	AnonKey string
}

// Manager provides access to C4 configuration.
type Manager struct {
	v      *viper.Viper
	config C4Config
}

// New creates a new config Manager and loads configuration from the
// given project root directory. It looks for .c4/config.yaml.
//
// If the config file does not exist, defaults are used.
// Environment variables with the C4_ prefix override config values.
func New(projectRoot string, cloudDefaults ...CloudDefaults) (*Manager, error) {
	v := viper.New()

	// Set defaults
	defaults := defaultConfig()
	v.SetDefault("project_id", defaults.ProjectID)
	v.SetDefault("default_branch", defaults.DefaultBranch)
	v.SetDefault("work_branch_prefix", defaults.WorkBranchPrefix)
	v.SetDefault("max_revision", defaults.MaxRevision)
	v.SetDefault("review_as_task", defaults.ReviewAsTask)
	v.SetDefault("checkpoint_as_task", defaults.CheckpointAsTask)
	v.SetDefault("worktree.enabled", defaults.Worktree.Enabled)
	v.SetDefault("worktree.auto_cleanup", defaults.Worktree.AutoCleanup)
	v.SetDefault("economic_mode.enabled", defaults.EconomicMode.Enabled)
	v.SetDefault("economic_mode.preset", defaults.EconomicMode.Preset)
	v.SetDefault("cloud.enabled", false)
	v.SetDefault("cloud.url", "")
	v.SetDefault("cloud.anon_key", "")
	v.SetDefault("cloud.project_id", "")
	v.SetDefault("cloud.mode", "local-first")
	v.SetDefault("llm_gateway.enabled", false)
	v.SetDefault("llm_gateway.default", "anthropic")
	v.SetDefault("llm_gateway.cache_by_default", true)
	v.SetDefault("eventbus.enabled", false)
	v.SetDefault("eventbus.auto_start", false)
	v.SetDefault("eventbus.socket_path", "")
	v.SetDefault("eventbus.data_dir", "")
	v.SetDefault("eventbus.retention_days", 30)
	v.SetDefault("eventbus.max_events", 10000)
	v.SetDefault("eventbus.ws_port", 0)
	v.SetDefault("hub.enabled", false)
	v.SetDefault("hub.url", "")
	v.SetDefault("hub.api_key_env", "C5_API_KEY")
	v.SetDefault("eventsink.enabled", false)
	v.SetDefault("eventsink.port", 4141)
	v.SetDefault("eventsink.token", "")
	v.SetDefault("serve.health_port", 4140)
	v.SetDefault("serve.agent.enabled", false)
	v.SetDefault("serve.eventbus.enabled", false)
	v.SetDefault("serve.eventsink.enabled", false)
	v.SetDefault("serve.hubpoller.enabled", false)
	v.SetDefault("serve.gpu.enabled", false)
	v.SetDefault("serve.ssesubscriber.enabled", false)
	v.SetDefault("serve.stale_checker.enabled", false)
	v.SetDefault("serve.stale_checker.threshold_minutes", 30)
	v.SetDefault("serve.stale_checker.interval_seconds", 60)
	v.SetDefault("serve.hub.enabled", false)
	v.SetDefault("serve.hub.binary", "c5")
	v.SetDefault("serve.hub.port", 8585)
	v.SetDefault("serve.hub.args", []string{})
	v.SetDefault("serve.mcp_http.enabled", false)
	v.SetDefault("serve.mcp_http.port", 4142)
	v.SetDefault("serve.mcp_http.bind", "127.0.0.1")
	v.SetDefault("serve.mcp_http.api_key", "")
	v.SetDefault("permission_reviewer.enabled", false)
	v.SetDefault("planning.critique_loop.enabled", true)
	v.SetDefault("planning.critique_loop.max_rounds", 3)
	v.SetDefault("planning.critique_loop.mode", "auto")
	v.SetDefault("permission_reviewer.model", "haiku")
	v.SetDefault("permission_reviewer.api_key_env", "ANTHROPIC_API_KEY")
	v.SetDefault("permission_reviewer.fail_mode", "ask")
	v.SetDefault("permission_reviewer.timeout", 10)
	v.SetDefault("permission_reviewer.auto_approve", true)
	v.SetDefault("permission_reviewer.allow_patterns", []string{})
	v.SetDefault("permission_reviewer.block_patterns", []string{})
	v.SetDefault("sessions.limit", defaults.Sessions.Limit)
	v.SetDefault("sessions.enabled", defaults.Sessions.Enabled)
	v.SetDefault("risk_routing.enabled", false)
	v.SetDefault("risk_routing.paths.high", []string{"infra/", "internal/mcp/handlers/"})
	v.SetDefault("risk_routing.paths.low", []string{"docs/", "user/", "*.md"})
	v.SetDefault("risk_routing.models.high", "opus")
	v.SetDefault("risk_routing.models.low", "sonnet")
	v.SetDefault("risk_routing.models.default", "opus")

	// Config file location — 2-tier: global (~/.c4/) then project (.c4/)
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// 1. Read global config as base layer (if exists)
	if home, err := os.UserHomeDir(); err == nil {
		globalConfig := filepath.Join(home, ".c4", "config.yaml")
		if _, err := os.Stat(globalConfig); err == nil {
			v.SetConfigFile(globalConfig)
			_ = v.ReadInConfig() // ignore errors — global is optional
		}
	}

	// 2. Merge project config on top (overrides global)
	configDir := filepath.Join(projectRoot, ".c4")
	projectConfig := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(projectConfig); err == nil {
		v.SetConfigFile(projectConfig)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("reading project config: %w", err)
		}
	}

	// Environment variable overrides (C4_PROJECT_ID, C4_DEFAULT_BRANCH, etc.)
	v.SetEnvPrefix("C4")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Unmarshal into struct
	var cfg C4Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Cloud credential resolution priority:
	//  1. Config file (cloud.url / cloud.anon_key)
	//  2. Environment: C4_CLOUD_URL / C4_CLOUD_ANON_KEY (via viper)
	//  3. Environment: SUPABASE_URL / SUPABASE_KEY (legacy)
	//  4. Built-in defaults (set via ldflags at build time)
	if cfg.Cloud.URL == "" {
		if u := os.Getenv("SUPABASE_URL"); u != "" {
			cfg.Cloud.URL = u
		}
	}
	if cfg.Cloud.AnonKey == "" {
		if k := os.Getenv("SUPABASE_KEY"); k != "" {
			cfg.Cloud.AnonKey = k
		}
	}
	// Fallback to built-in defaults (baked into binary via ldflags)
	if len(cloudDefaults) > 0 && cloudDefaults[0].URL != "" {
		if cfg.Cloud.URL == "" {
			cfg.Cloud.URL = cloudDefaults[0].URL
		}
		if cfg.Cloud.AnonKey == "" {
			cfg.Cloud.AnonKey = cloudDefaults[0].AnonKey
		}
	}
	// Auto-enable cloud if credentials are available
	if !cfg.Cloud.Enabled && cfg.Cloud.URL != "" && cfg.Cloud.AnonKey != "" {
		cfg.Cloud.Enabled = true
	}

	// EventSink environment variable overrides (priority: env > config.yaml > defaults)
	// C4_EVENTSINK_PORT: if set, overrides port; if "0", disables eventsink
	// C4_EVENTSINK_TOKEN: if set, overrides token
	if v := os.Getenv("C4_EVENTSINK_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.EventSink.Port = p
			cfg.EventSink.Enabled = p != 0
		}
	}
	if v := os.Getenv("C4_EVENTSINK_TOKEN"); v != "" {
		cfg.EventSink.Token = v
	}

	// Resolve preset if economic mode is enabled
	if cfg.EconomicMode.Enabled {
		cfg.resolvePreset()
	} else {
		// When disabled, still set default routing
		cfg.EconomicMode.ModelRouting = presetConfigs["standard"]
	}

	return &Manager{v: v, config: cfg}, nil
}

// resolvePreset applies the preset model routing if no custom routing is set.
func (c *C4Config) resolvePreset() {
	preset, ok := presetConfigs[c.EconomicMode.Preset]
	if !ok {
		// Unknown preset, default to standard
		preset = presetConfigs["standard"]
	}

	// Only apply preset values where config doesn't have explicit overrides
	routing := &c.EconomicMode.ModelRouting
	if routing.Implementation == "" {
		routing.Implementation = preset.Implementation
	}
	if routing.Review == "" {
		routing.Review = preset.Review
	}
	if routing.Checkpoint == "" {
		routing.Checkpoint = preset.Checkpoint
	}
	if routing.Scout == "" {
		routing.Scout = preset.Scout
	}
	if routing.Debug == "" {
		routing.Debug = preset.Debug
	}
	if routing.Planning == "" {
		routing.Planning = preset.Planning
	}
}

// Get returns a configuration value by key using dot-notation.
// For example: Get("economic_mode.preset") or Get("project_id").
func (m *Manager) Get(key string) any {
	return m.v.Get(key)
}

// IsSet reports whether a configuration key was explicitly set in the config file or environment.
// Unlike Get, this returns false for keys that only have defaults.
func (m *Manager) IsSet(key string) bool {
	return m.v.IsSet(key)
}

// GetString returns a configuration value as a string.
func (m *Manager) GetString(key string) string {
	return m.v.GetString(key)
}

// GetConfig returns the parsed C4Config struct.
func (m *Manager) GetConfig() C4Config {
	return m.config
}

// GetBackend returns the store backend type.
// Returns "hybrid" if cloud is enabled, "sqlite" otherwise.
func (m *Manager) GetBackend() string {
	if m.config.Cloud.Enabled {
		return "hybrid"
	}
	backend := m.v.GetString("store.backend")
	if backend == "" {
		return "sqlite"
	}
	return backend
}

// GetModelForTask returns the recommended model for a task type.
//
// Task type is determined from the task ID prefix:
//   - T-XXX -> implementation
//   - R-XXX -> review
//   - CP-XXX -> checkpoint
//   - RF-XXX -> review (refine uses review model)
//   - RPR-XXX -> implementation (repair)
//
// If economic mode is disabled, returns empty string (use default).
func (m *Manager) GetModelForTask(taskID string) string {
	if !m.config.EconomicMode.Enabled {
		return ""
	}

	routing := m.config.EconomicMode.ModelRouting

	switch {
	case strings.HasPrefix(taskID, "RF-"):
		return routing.Review
	case strings.HasPrefix(taskID, "R-"):
		return routing.Review
	case strings.HasPrefix(taskID, "CP-"):
		return routing.Checkpoint
	default:
		// T- prefix, RPR- prefix, or any other defaults to implementation
		return routing.Implementation
	}
}

// GetRiskRouting returns the risk routing configuration.
// Independent of EconomicMode.GetModelForTask — scope-based override only.
func (m *Manager) GetRiskRouting() RiskRoutingConfig {
	return m.config.RiskRouting
}

// GetNotificationChannel returns a copy of the named channel and true, or zero value and false if not found.
// Returns a value copy to avoid aliasing the live config slice.
func (m *Manager) GetNotificationChannel(name string) (NotificationChannel, bool) {
	for _, ch := range m.config.Notifications.Channels {
		if ch.Name == name {
			return ch, true
		}
	}
	return NotificationChannel{}, false
}

// Set updates a configuration value in memory (does not persist to file).
// Use dot-notation keys (e.g., "permission_reviewer.enabled").
func (m *Manager) Set(key string, value any) {
	m.v.Set(key, value)
	_ = m.v.Unmarshal(&m.config)
}

