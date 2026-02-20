package handlers

import (
	"encoding/json"
	"strings"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterConfigHandler registers the c4_config_get MCP tool.
func RegisterConfigHandler(reg *mcp.Registry, cfgMgr *config.Manager) {
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
			"enabled":     cfg.PermissionReviewer.Enabled,
			"model":       cfg.PermissionReviewer.Model,
			"api_key_env": cfg.PermissionReviewer.APIKeyEnv,
			"fail_mode":   cfg.PermissionReviewer.FailMode,
			"timeout":     cfg.PermissionReviewer.Timeout,
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
				"enabled": cfg.PermissionReviewer.Enabled,
				"model":   cfg.PermissionReviewer.Model,
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
