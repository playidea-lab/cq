package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/llm"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterLLMHandlers registers c4_llm_call, c4_llm_providers, and c4_llm_costs tools.
func RegisterLLMHandlers(reg *mcp.Registry, gateway *llm.Gateway) {
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
		Description: "Get LLM usage cost report for this session",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleLLMCosts(gateway)
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
		CacheSystemPrompt: params.CacheSystemPrompt,
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
	byProvider := make(map[string]any)
	for name, pc := range report.ByProvider {
		byProvider[name] = map[string]any{
			"total_usd":     pc.TotalUSD,
			"requests":      pc.Requests,
			"input_tokens":  pc.InputTok,
			"output_tokens": pc.OutputTok,
		}
	}
	return map[string]any{
		"total_usd":      report.TotalUSD,
		"total_requests": report.TotalReqs,
		"by_provider":    byProvider,
		"by_model":       report.ByModel,
	}, nil
}
