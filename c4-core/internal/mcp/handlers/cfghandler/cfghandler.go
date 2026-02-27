package cfghandler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
)

// Register registers the c4_config_get and c4_config_set MCP tools.
func Register(reg *mcp.Registry, mgr *config.Manager, projectRoot string) {
	registerGet(reg, mgr)
	registerSet(reg, mgr, projectRoot)
}

// registerGet registers the c4_config_get MCP tool.
func registerGet(reg *mcp.Registry, cfgMgr *config.Manager) {
	if cfgMgr == nil {
		return
	}
	reg.Register(mcp.ToolSchema{
		Name:        "c4_config_get",
		Description: "Get runtime configuration (sensitive fields masked)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]any{
					"type":        "string",
					"description": "Config section: all, economic, worker, cloud, hub, permission_reviewer",
					"enum":        []string{"all", "economic", "worker", "cloud", "hub", "permission_reviewer"},
					"default":     "all",
				},
			},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleConfigGet(cfgMgr, args)
	})
}

func handleConfigGet(cfgMgr *config.Manager, rawArgs json.RawMessage) (any, error) {
	var params struct {
		Section string `json:"section"`
	}
	json.Unmarshal(rawArgs, &params)
	if params.Section == "" {
		params.Section = "all"
	}

	cfg := cfgMgr.GetConfig()

	switch params.Section {
	case "economic":
		return map[string]any{
			"enabled": cfg.EconomicMode.Enabled,
			"preset":  cfg.EconomicMode.Preset,
			"model_routing": map[string]any{
				"implementation": cfg.EconomicMode.ModelRouting.Implementation,
				"review":         cfg.EconomicMode.ModelRouting.Review,
				"checkpoint":     cfg.EconomicMode.ModelRouting.Checkpoint,
				"scout":          cfg.EconomicMode.ModelRouting.Scout,
				"debug":          cfg.EconomicMode.ModelRouting.Debug,
				"planning":       cfg.EconomicMode.ModelRouting.Planning,
			},
		}, nil
	case "worker":
		return map[string]any{
			"default_branch":     cfg.DefaultBranch,
			"work_branch_prefix": cfg.WorkBranchPrefix,
			"max_revision":       cfg.MaxRevision,
			"review_as_task":     cfg.ReviewAsTask,
			"checkpoint_as_task": cfg.CheckpointAsTask,
			"worktree_enabled":   cfg.Worktree.Enabled,
			"worktree_auto_cleanup": cfg.Worktree.AutoCleanup,
		}, nil
	case "cloud":
		return map[string]any{
			"enabled":    cfg.Cloud.Enabled,
			"url":        maskIfSecret("url", cfg.Cloud.URL),
			"anon_key":   maskSecret(cfg.Cloud.AnonKey),
			"project_id": cfg.Cloud.ProjectID,
		}, nil
	case "hub":
		return map[string]any{
			"enabled":    cfg.Hub.Enabled,
			"url":        cfg.Hub.URL,
			"api_prefix": cfg.Hub.APIPrefix,
			"api_key":    maskSecret(cfg.Hub.APIKey),
			"team_id":    cfg.Hub.TeamID,
		}, nil
	case "permission_reviewer":
		return map[string]any{
			"enabled":        cfg.PermissionReviewer.Enabled,
			"model":          cfg.PermissionReviewer.Model,
			"api_key_env":    cfg.PermissionReviewer.APIKeyEnv,
			"fail_mode":      cfg.PermissionReviewer.FailMode,
			"timeout":        cfg.PermissionReviewer.Timeout,
			"auto_approve":   cfg.PermissionReviewer.AutoApprove,
			"allow_patterns": cfg.PermissionReviewer.AllowPatterns,
			"block_patterns": cfg.PermissionReviewer.BlockPatterns,
		}, nil
	default: // "all"
		return map[string]any{
			"project_id":     cfg.ProjectID,
			"domain":         cfg.Domain,
			"default_branch": cfg.DefaultBranch,
			"economic_mode": map[string]any{
				"enabled": cfg.EconomicMode.Enabled,
				"preset":  cfg.EconomicMode.Preset,
			},
			"cloud": map[string]any{
				"enabled": cfg.Cloud.Enabled,
				"url":     maskIfSecret("url", cfg.Cloud.URL),
			},
			"hub": map[string]any{
				"enabled": cfg.Hub.Enabled,
				"url":     cfg.Hub.URL,
			},
			"eventbus": map[string]any{
				"enabled":    cfg.EventBus.Enabled,
				"auto_start": cfg.EventBus.AutoStart,
				"ws_port":    cfg.EventBus.WSPort,
			},
			"llm_gateway": map[string]any{
				"enabled":          cfg.LLMGateway.Enabled,
				"default":          cfg.LLMGateway.Default,
				"cache_by_default": cfg.LLMGateway.CacheByDefault,
			},
			"validation": map[string]any{
				"lint": cfg.Validation.Lint,
				"unit": cfg.Validation.Unit,
			},
			"permission_reviewer": map[string]any{
				"enabled":        cfg.PermissionReviewer.Enabled,
				"model":          cfg.PermissionReviewer.Model,
				"auto_approve":   cfg.PermissionReviewer.AutoApprove,
				"allow_patterns": cfg.PermissionReviewer.AllowPatterns,
				"block_patterns": cfg.PermissionReviewer.BlockPatterns,
			},
		}, nil
	}
}

// maskSecret replaces non-empty secrets with "***masked***".
func maskSecret(val string) string {
	if val == "" {
		return ""
	}
	return "***masked***"
}

// maskIfSecret masks URL-like values that contain tokens/keys in query params.
func maskIfSecret(key, val string) string {
	lower := strings.ToLower(key)
	if strings.Contains(lower, "key") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
		return maskSecret(val)
	}
	return val
}

// registerSet registers the c4_config_set MCP tool.
func registerSet(reg *mcp.Registry, cfgMgr *config.Manager, projectRoot string) {
	if cfgMgr == nil {
		return
	}
	reg.Register(mcp.ToolSchema{
		Name:        "c4_config_set",
		Description: "Set a configuration value (persists to .c4/config.yaml and applies immediately)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Config key in dot-notation (e.g., permission_reviewer.enabled, economic_mode.preset, hub.url)",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Value to set. Type auto-detected: true/false→bool, integers→int, otherwise→string",
				},
			},
			"required": []string{"key", "value"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleConfigSet(cfgMgr, projectRoot, args)
	})
}

func handleConfigSet(cfgMgr *config.Manager, projectRoot string, rawArgs json.RawMessage) (any, error) {
	var params struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(rawArgs, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Key == "" || params.Value == "" {
		return nil, fmt.Errorf("both key and value are required")
	}

	configPath := filepath.Join(projectRoot, ".c4", "config.yaml")

	// Get old value for reporting
	oldValue := cfgMgr.Get(params.Key)

	// Infer typed value
	typedValue := inferType(params.Value)

	// Update YAML file (preserves comments)
	if err := updateYAMLValue(configPath, params.Key, params.Value); err != nil {
		return nil, fmt.Errorf("failed to update config file: %w", err)
	}

	// Update in-memory config
	cfgMgr.Set(params.Key, typedValue)

	return map[string]any{
		"key":       params.Key,
		"old_value": oldValue,
		"new_value": typedValue,
		"file":      configPath,
	}, nil
}

// inferType converts a string value to its most appropriate Go type.
func inferType(val string) any {
	lower := strings.ToLower(val)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return val
}

// updateYAMLValue modifies a value in a YAML file while preserving comments and formatting.
// Supports dot-notation keys up to 3 levels deep.
func updateYAMLValue(filePath string, dotKey string, rawValue string) error {
	data, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		// Create directory if needed
		dir := filepath.Dir(filePath)
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			return mkErr
		}
		data = []byte{}
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	parts := strings.Split(dotKey, ".")

	newLines, found := yamlSet(lines, parts, rawValue)
	if !found {
		newLines = yamlAppend(newLines, parts, rawValue)
	}

	output := strings.Join(newLines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return os.WriteFile(filePath, []byte(output), 0644)
}

// yamlSet finds and updates a key in YAML lines. Returns (lines, found).
func yamlSet(lines []string, parts []string, value string) ([]string, bool) {
	if len(parts) == 0 {
		return lines, false
	}

	result := make([]string, len(lines))
	copy(result, lines)

	if len(parts) == 1 {
		return setAtDepth(result, parts[0], value, 0)
	}

	// Find section start and range
	sectionStart, sectionEnd := findSection(result, parts[0], 0)
	if sectionStart < 0 {
		return result, false
	}

	if len(parts) == 2 {
		// Update within section
		updated, found := setAtDepth(result[sectionStart+1:sectionEnd], parts[1], value, 1)
		if !found {
			return result, false
		}
		newResult := make([]string, 0, len(result))
		newResult = append(newResult, result[:sectionStart+1]...)
		newResult = append(newResult, updated...)
		newResult = append(newResult, result[sectionEnd:]...)
		return newResult, true
	}

	if len(parts) == 3 {
		// Find sub-section within section
		section := result[sectionStart+1 : sectionEnd]
		subStart, subEnd := findSection(section, parts[1], 1)
		if subStart < 0 {
			return result, false
		}
		subSection := section[subStart+1 : subEnd]
		updated, found := setAtDepth(subSection, parts[2], value, 2)
		if !found {
			return result, false
		}
		// Rebuild
		newResult := make([]string, 0, len(result))
		newResult = append(newResult, result[:sectionStart+1]...)
		newResult = append(newResult, section[:subStart+1]...)
		newResult = append(newResult, updated...)
		newResult = append(newResult, section[subEnd:]...)
		newResult = append(newResult, result[sectionEnd:]...)
		return newResult, true
	}

	return result, false
}

// setAtDepth finds a key at a given indentation depth and updates its value.
func setAtDepth(lines []string, key string, value string, depth int) ([]string, bool) {
	indent := strings.Repeat("  ", depth)
	prefix := indent + key + ":"

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Check indentation matches depth
		lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		if lineIndent != depth*2 {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			// Found — replace value, preserving inline comment
			comment := extractInlineComment(line, prefix)
			if comment != "" {
				lines[i] = prefix + " " + value + comment
			} else {
				lines[i] = prefix + " " + value
			}
			return lines, true
		}
	}
	return lines, false
}

// findSection returns the start and end indices of a YAML section at a given depth.
func findSection(lines []string, sectionName string, depth int) (int, int) {
	indent := strings.Repeat("  ", depth)
	prefix := indent + sectionName + ":"
	start := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))

		if start >= 0 && lineIndent <= depth*2 {
			// Reached next sibling or parent — section ends here
			return start, i
		}

		if lineIndent == depth*2 && strings.HasPrefix(line, prefix) {
			start = i
		}
	}

	if start >= 0 {
		return start, len(lines)
	}
	return -1, -1
}

// extractInlineComment extracts an inline comment from a YAML line.
// Returns the comment including leading spaces and #, or empty string.
func extractInlineComment(line string, prefix string) string {
	rest := line[len(prefix):]
	// Skip the value part, find # that's preceded by whitespace
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
		} else if inQuote && c == quoteChar {
			inQuote = false
		} else if !inQuote && c == '#' && i > 0 && rest[i-1] == ' ' {
			return " " + strings.TrimLeft(rest[i-1:], " ")
		}
	}
	return ""
}

// yamlAppend adds a new key-value to YAML lines when the key doesn't exist.
func yamlAppend(lines []string, parts []string, value string) []string {
	if len(parts) == 1 {
		return appendLine(lines, parts[0]+": "+value, 0)
	}

	if len(parts) == 2 {
		// Find or create section
		sectionStart, sectionEnd := findSection(lines, parts[0], 0)
		if sectionStart >= 0 {
			// Section exists — insert key at end of section
			newLine := "  " + parts[1] + ": " + value
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:sectionEnd]...)
			newLines = append(newLines, newLine)
			newLines = append(newLines, lines[sectionEnd:]...)
			return newLines
		}
		// Section doesn't exist — append both
		lines = appendLine(lines, parts[0]+":", 0)
		lines = append(lines, "  "+parts[1]+": "+value)
		return lines
	}

	if len(parts) == 3 {
		sectionStart, sectionEnd := findSection(lines, parts[0], 0)
		if sectionStart >= 0 {
			section := lines[sectionStart+1 : sectionEnd]
			subStart, subEnd := findSection(section, parts[1], 1)
			if subStart >= 0 {
				// Sub-section exists — insert key
				insertIdx := sectionStart + 1 + subEnd
				newLine := "    " + parts[2] + ": " + value
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:insertIdx]...)
				newLines = append(newLines, newLine)
				newLines = append(newLines, lines[insertIdx:]...)
				return newLines
			}
			// Sub-section doesn't exist — add it
			insertIdx := sectionEnd
			newLines := make([]string, 0, len(lines)+2)
			newLines = append(newLines, lines[:insertIdx]...)
			newLines = append(newLines, "  "+parts[1]+":")
			newLines = append(newLines, "    "+parts[2]+": "+value)
			newLines = append(newLines, lines[insertIdx:]...)
			return newLines
		}
		// Nothing exists — append all three levels
		lines = appendLine(lines, parts[0]+":", 0)
		lines = append(lines, "  "+parts[1]+":")
		lines = append(lines, "    "+parts[2]+": "+value)
		return lines
	}

	return lines
}

// appendLine adds a line at the end, before trailing empty lines.
func appendLine(lines []string, content string, _ int) []string {
	// Find last non-empty line
	insertAt := len(lines)
	for insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	// Add blank line separator if needed
	if insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) != "" {
		newLines := make([]string, 0, len(lines)+2)
		newLines = append(newLines, lines[:insertAt]...)
		newLines = append(newLines, "")
		newLines = append(newLines, content)
		newLines = append(newLines, lines[insertAt:]...)
		return newLines
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, content)
	newLines = append(newLines, lines[insertAt:]...)
	return newLines
}
