package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/llm"
)

// hookConfigJSON is the schema written to {projectDir}/.c4/hook-config.json.
// The project hooks (c4-gate.sh, c4-permission-reviewer.sh) read this file via jq
// to determine whether permission review is enabled and how to call the LLM.
type hookConfigJSON struct {
	Enabled              bool     `json:"enabled"`
	BashSecurityEnabled  bool     `json:"bash_security_enabled"`
	Mode                 string   `json:"mode"`          // "hook" | "model"
	AutoApprove          bool     `json:"auto_approve"`
	Model                string   `json:"model"`
	APIKeyEnv            string   `json:"api_key_env"`
	Timeout              int      `json:"timeout"`
	AllowPatterns        []string `json:"allow_patterns"`
	BlockPatterns        []string `json:"block_patterns"`
}

// defaultHookConfig returns the hardcoded defaults used when cfgMgr is nil
// or PermissionReviewer is not configured.
func defaultHookConfig() hookConfigJSON {
	return hookConfigJSON{
		Enabled:             false,
		BashSecurityEnabled: true,
		Mode:                "hook",
		AutoApprove:         true,
		Model:               "claude-haiku-4-5-20251001",
		APIKeyEnv:           "ANTHROPIC_API_KEY",
		Timeout:             10,
		AllowPatterns:       []string{},
		BlockPatterns:       []string{},
	}
}

// hookConfigFromC4Config converts a *config.C4Config into hookConfigJSON.
// Fields not present in C4Config use sensible defaults.
func hookConfigFromC4Config(cfg *config.C4Config) hookConfigJSON {
	pr := cfg.PermissionReviewer

	// Mode: use config value, default to "hook" if unset.
	mode := pr.Mode
	if mode == "" {
		mode = "hook"
	}

	// AutoApprove: prefer explicit config field; fall back to fail_mode=="allow".
	autoApprove := pr.AutoApprove || pr.FailMode == "allow"

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

	// BashSecurity: separate from PermissionReviewer. Default true.
	bashSecEnabled := true
	if cfg.BashSecurity != nil {
		bashSecEnabled = *cfg.BashSecurity
	}

	return hookConfigJSON{
		Enabled:             pr.Enabled,
		BashSecurityEnabled: bashSecEnabled,
		Mode:                mode,
		AutoApprove:         autoApprove,
		Model:               model,
		APIKeyEnv:           apiKeyEnv,
		Timeout:             timeout,
		AllowPatterns:       pr.AllowPatterns,
		BlockPatterns:       pr.BlockPatterns,
	}
}

// mergePatterns returns the union of two string slices, preserving order (base first, then extras).
func mergePatterns(base, extra []string) []string {
	seen := make(map[string]bool, len(base))
	for _, p := range base {
		seen[p] = true
	}
	merged := make([]string, len(base))
	copy(merged, base)
	for _, p := range extra {
		if !seen[p] {
			merged = append(merged, p)
			seen[p] = true
		}
	}
	return merged
}

// resolveHookModel maps short model aliases to full Anthropic model IDs.
func resolveHookModel(model string) string {
	// ResolveAlias does not define "" key — guard required
	if model == "" {
		return "claude-haiku-4-5-20251001"
	}
	return llm.ResolveAlias(model)
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

	// Merge: preserve user-added allow_patterns and block_patterns from existing file.
	if existing, err := os.ReadFile(path); err == nil {
		var prev hookConfigJSON
		if json.Unmarshal(existing, &prev) == nil {
			merged := mergePatterns(hcfg.AllowPatterns, prev.AllowPatterns)
			hcfg.AllowPatterns = merged
			mergedBlock := mergePatterns(hcfg.BlockPatterns, prev.BlockPatterns)
			hcfg.BlockPatterns = mergedBlock
			// Re-marshal with merged patterns
			data, err = json.MarshalIndent(hcfg, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "cq: hook-config.json re-marshal failed: %v\n", err)
				return
			}
		}
	}

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
