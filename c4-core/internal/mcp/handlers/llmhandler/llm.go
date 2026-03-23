//go:build llm_gateway

package llmhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const costTrackerResourceURI = "ui://cq/cost-tracker"

// RegisterCostTrackerWidget registers the cost-tracker HTML widget in the resource store.
// Call this after RegisterLLMHandlers when the apps ResourceStore is available.
func RegisterCostTrackerWidget(rs *apps.ResourceStore, html string) {
	if rs != nil && html != "" {
		rs.Register(costTrackerResourceURI, html)
	}
}

// RegisterLLMHandlers registers c4_llm_call, c4_llm_providers, c4_llm_costs,
// and c4_llm_usage_stats tools.
// db may be nil (stats tool returns zeros when DB is unavailable).
func RegisterLLMHandlers(reg *mcp.Registry, gateway *llm.Gateway, db *sql.DB) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_llm_call",
		Description: "Send a chat request through the LLM gateway with automatic routing and cost tracking",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"messages": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"role":    map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"role", "content"},
					},
					"description": "Chat messages (role: user/assistant/system)",
				},
				"model":       map[string]any{"type": "string", "description": "Model hint (alias, full ID, or provider/model)"},
				"task_type":   map[string]any{"type": "string", "description": "Task type for routing (e.g. review, implementation)"},
				"max_tokens":  map[string]any{"type": "integer", "description": "Max output tokens"},
				"temperature": map[string]any{"type": "number", "description": "Sampling temperature"},
				"system":               map[string]any{"type": "string", "description": "System prompt"},
				"cache_system_prompt": map[string]any{
					"type":        "boolean",
					"description": "Cache system prompt via Anthropic prompt caching (min 1024 tokens, reduces cost ~90% on cache hits)",
				},
			},
			"required": []string{"messages"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleLLMCall(gateway, raw)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_llm_providers",
		Description: "List registered LLM providers and their available models",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleLLMProviders(gateway)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_llm_costs",
		Description: "Get LLM usage cost report for this session — model breakdown, cache hit rate, and total spend",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget with _meta; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var args struct {
			Format string `json:"format"`
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		data, err := handleLLMCosts(gateway)
		if err != nil {
			return nil, err
		}
		if args.Format == "widget" {
			return map[string]any{
				"data": data,
				"_meta": map[string]any{
					"ui": map[string]any{
						"resourceUri": costTrackerResourceURI,
					},
				},
			}, nil
		}
		return data, nil
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_llm_usage_stats",
		Description: "Get LLM usage cost and cache statistics for the last N hours from persistent storage",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hours": map[string]any{
					"type":        "integer",
					"description": "Number of hours to look back (default: 24)",
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleLLMUsageStats(db, raw)
	})
}

func handleLLMCall(gateway *llm.Gateway, raw json.RawMessage) (any, error) {
	var params struct {
		Messages          []llm.Message `json:"messages"`
		Model             string        `json:"model"`
		TaskType          string        `json:"task_type"`
		MaxTokens         int           `json:"max_tokens"`
		Temperature       float64       `json:"temperature"`
		System            string        `json:"system"`
		CacheSystemPrompt bool          `json:"cache_system_prompt"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if len(params.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}

	req := &llm.ChatRequest{
		Model:             params.Model,
		Messages:          params.Messages,
		MaxTokens:         params.MaxTokens,
		Temperature:       params.Temperature,
		System:            params.System,
		CacheSystemPrompt: params.CacheSystemPrompt || gateway.CacheByDefault(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := gateway.Chat(ctx, params.TaskType, req)
	if err != nil {
		return nil, err
	}

	// Calculate cost for response
	info, _ := llm.LookupModel(resp.Model)
	costUSD := float64(resp.Usage.InputTokens)*info.InputPer1M/1_000_000 +
		float64(resp.Usage.OutputTokens)*info.OutputPer1M/1_000_000

	return map[string]any{
		"content":       resp.Content,
		"model":         resp.Model,
		"finish_reason": resp.FinishReason,
		"usage": map[string]any{
			"input_tokens":        resp.Usage.InputTokens,
			"output_tokens":       resp.Usage.OutputTokens,
			"cache_read_tokens":   resp.Usage.CacheReadTokens,
			"cache_write_tokens":  resp.Usage.CacheWriteTokens,
		},
		"cost_usd": costUSD,
	}, nil
}

func handleLLMProviders(gateway *llm.Gateway) (any, error) {
	providers := gateway.ListProviders()
	result := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		models := make([]map[string]any, 0, len(p.Models))
		for _, m := range p.Models {
			models = append(models, map[string]any{
				"id":             m.ID,
				"name":           m.Name,
				"context_window": m.ContextWindow,
				"input_per_1m":   m.InputPer1M,
				"output_per_1m":  m.OutputPer1M,
			})
		}
		result = append(result, map[string]any{
			"name":      p.Name,
			"available": p.Available,
			"models":    models,
		})
	}
	return map[string]any{"providers": result}, nil
}

func handleLLMCosts(gateway *llm.Gateway) (any, error) {
	report := gateway.CostReport()

	// Aggregate global cache tokens across all providers.
	var globalCacheRead, globalCacheWrite, globalInput int
	byProvider := make(map[string]any)
	for name, pc := range report.ByProvider {
		globalCacheRead += pc.CacheReadTok
		globalCacheWrite += pc.CacheWriteTok
		globalInput += pc.InputTok

		var hitRate, savingsRate float64
		if denom := pc.CacheReadTok + pc.CacheWriteTok; denom > 0 {
			hitRate = float64(pc.CacheReadTok) / float64(denom)
		}
		if denom := pc.InputTok + pc.CacheReadTok + pc.CacheWriteTok; denom > 0 {
			savingsRate = float64(pc.CacheReadTok) / float64(denom)
		}
		byProvider[name] = map[string]any{
			"total_usd":           pc.TotalUSD,
			"requests":            pc.Requests,
			"input_tokens":        pc.InputTok,
			"output_tokens":       pc.OutputTok,
			"cache_read_tokens":   pc.CacheReadTok,
			"cache_write_tokens":  pc.CacheWriteTok,
			"cache_savings_usd":   pc.SavedUSD,
			"cache_hit_rate":      hitRate,
			"cache_savings_rate":  savingsRate,
		}
	}

	var globalHitRate, globalSavingsRate float64
	if denom := globalCacheRead + globalCacheWrite; denom > 0 {
		globalHitRate = float64(globalCacheRead) / float64(denom)
	}
	if denom := globalInput + globalCacheRead + globalCacheWrite; denom > 0 {
		globalSavingsRate = float64(globalCacheRead) / float64(denom)
	}

	return map[string]any{
		"total_usd":                 report.TotalUSD,
		"total_requests":            report.TotalReqs,
		"global_cache_hit_rate":     globalHitRate,
		"global_cache_savings_rate": globalSavingsRate,
		"by_provider":               byProvider,
		"by_model":                  report.ByModel,
	}, nil
}

// handleLLMUsageStats queries the llm_usage SQLite table for the last N hours
// and returns aggregate cost/cache statistics.
func handleLLMUsageStats(db *sql.DB, raw json.RawMessage) (any, error) {
	var params struct {
		Hours int `json:"hours"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Hours <= 0 {
		params.Hours = 24
	}

	// Return zeros when DB is unavailable.
	if db == nil {
		return map[string]any{
			"total_cost_usd":         0.0,
			"cache_utilization_rate": 0.0,
			"total_tokens":           0,
			"top_models":             []map[string]any{},
			"period_hours":           params.Hours,
		}, nil
	}

	since := time.Now().UTC().Add(-time.Duration(params.Hours) * time.Hour).Format(time.RFC3339)

	// Aggregate totals.
	var totalCost float64
	var totalPrompt, totalCompletion, totalCacheRead int64
	err := db.QueryRow(`
		SELECT COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(prompt_tok), 0),
		       COALESCE(SUM(completion_tok), 0),
		       COALESCE(SUM(cache_read_tok), 0)
		FROM llm_usage WHERE ts >= ?`, since,
	).Scan(&totalCost, &totalPrompt, &totalCompletion, &totalCacheRead)
	if err != nil {
		return nil, fmt.Errorf("querying llm_usage: %w", err)
	}

	// cache_utilization_rate = cache_read_tok / (prompt_tok + cache_read_tok)
	// Measures what fraction of total input tokens were served from cache.
	// Distinct from cache_hit_rate in c4_llm_costs (read/(read+write) among cache ops).
	var cacheUtilizationRate float64
	if denom := totalPrompt + totalCacheRead; denom > 0 {
		cacheUtilizationRate = float64(totalCacheRead) / float64(denom)
	}

	totalTokens := totalPrompt + totalCompletion + totalCacheRead

	// Top 5 models by cost desc.
	rows, err := db.Query(`
		SELECT model, COALESCE(SUM(cost_usd), 0) AS total_cost, COUNT(*) AS call_count
		FROM llm_usage WHERE ts >= ?
		GROUP BY model ORDER BY total_cost DESC LIMIT 5`, since)
	if err != nil {
		return nil, fmt.Errorf("querying top models: %w", err)
	}
	defer rows.Close()

	topModels := make([]map[string]any, 0, 5)
	for rows.Next() {
		var model string
		var modelCost float64
		var callCount int
		if err := rows.Scan(&model, &modelCost, &callCount); err != nil {
			return nil, fmt.Errorf("scanning top model row: %w", err)
		}
		topModels = append(topModels, map[string]any{
			"model":          model,
			"total_cost_usd": modelCost,
			"call_count":     callCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating top models: %w", err)
	}

	return map[string]any{
		"total_cost_usd":          totalCost,
		"cache_utilization_rate":  cacheUtilizationRate,
		"total_tokens":            totalTokens,
		"top_models":              topModels,
		"period_hours":            params.Hours,
	}, nil
}
