package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/config"
)

// hookConfigJSON is the schema written to {projectDir}/.c4/hook-config.json.
// The bash hook (c4-bash-security-hook.sh) reads this file via jq to determine
// whether permission review is enabled and how to call the LLM.
type hookConfigJSON struct {
	Enabled        bool     `json:"enabled"`
	Mode           string   `json:"mode"`          // "hook" | "model"
	AutoApprove    bool     `json:"auto_approve"`
	Model          string   `json:"model"`
	APIKeyEnv      string   `json:"api_key_env"`
	Timeout        int      `json:"timeout"`
	AllowPatterns  []string `json:"allow_patterns"`
	BlockPatterns  []string `json:"block_patterns"`
}

// defaultHookConfig returns the hardcoded defaults used when cfgMgr is nil
// or PermissionReviewer is not configured.
func defaultHookConfig() hookConfigJSON {
	return hookConfigJSON{
		Enabled:       false,
		Mode:          "hook",
		AutoApprove:   true,
		Model:         "claude-haiku-4-5-20251001",
		APIKeyEnv:     "ANTHROPIC_API_KEY",
		Timeout:       10,
		AllowPatterns: []string{},
		BlockPatterns: []string{},
	}
}

// hookConfigFromC4Config converts a *config.C4Config into hookConfigJSON.
// Fields not present in C4Config use sensible defaults.
func hookConfigFromC4Config(cfg *config.C4Config) hookConfigJSON {
	pr := cfg.PermissionReviewer

	// Derive mode: if model name is set and differs from default, assume model mode.
	// For now default to "hook" unless explicitly overridden.
	mode := "hook"

	// Derive auto_approve from fail_mode: "allow" means approve automatically.
	autoApprove := pr.FailMode == "allow"

	// Resolve full model ID from short alias or keep as-is.
	model := resolveHookModel(pr.Model)

	apiKeyEnv := pr.APIKeyEnv
	if apiKeyEnv == "" {
		apiKeyEnv = "ANTHROPIC_API_KEY"
	}

	timeout := pr.Timeout
	if timeout <= 0 {
		timeout = 10
	}

	return hookConfigJSON{
		Enabled:       pr.Enabled,
		Mode:          mode,
		AutoApprove:   autoApprove,
		Model:         model,
		APIKeyEnv:     apiKeyEnv,
		Timeout:       timeout,
		AllowPatterns: []string{},
		BlockPatterns: []string{},
	}
}

// resolveHookModel maps short model aliases to full Anthropic model IDs.
func resolveHookModel(model string) string {
	switch model {
	case "haiku", "":
		return "claude-haiku-4-5-20251001"
	case "sonnet":
		return "claude-sonnet-4-5-20251001"
	case "opus":
		return "claude-opus-4-5-20241101"
	default:
		return model
	}
}

// writeHookConfigJSON writes {projectDir}/.c4/hook-config.json.
// When cfg is nil, defaults are used (enabled=false, mode=hook, auto_approve=true).
// The file is not written if its content is already identical (bytes.Equal).
func writeHookConfigJSON(projectDir string, cfg *config.C4Config) {
	var hcfg hookConfigJSON
	if cfg == nil {
		hcfg = defaultHookConfig()
	} else {
		hcfg = hookConfigFromC4Config(cfg)
	}

	data, err := json.MarshalIndent(hcfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: hook-config.json marshal failed: %v\n", err)
		return
	}

	dir := filepath.Join(projectDir, ".c4")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cq: hook-config.json mkdir failed: %v\n", err)
		return
	}

	path := filepath.Join(dir, "hook-config.json")

	// Skip write if existing file has identical content.
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cq: hook-config.json write failed: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, "cq: hook-config.json → "+path)
}
