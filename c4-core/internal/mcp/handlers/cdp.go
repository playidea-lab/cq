package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/cdp"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterCDPHandlers registers c4_cdp_run and c4_cdp_list tools.
func RegisterCDPHandlers(reg *mcp.Registry, runner *cdp.Runner) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_cdp_run",
		Description: "Execute JavaScript in a Chromium app via CDP. Connect to any app opened with --remote-debugging-port. Runs entire script in one call for token efficiency.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script":          map[string]any{"type": "string", "description": "JavaScript to execute in browser context"},
				"url":             map[string]any{"type": "string", "description": "CDP debug URL (default: http://localhost:9222)"},
				"target_url":      map[string]any{"type": "string", "description": "Navigate to URL before running script"},
				"await_selector":  map[string]any{"type": "string", "description": "Wait for CSS selector before running script"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30, max: 300)"},
			},
			"required": []string{"script"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleCDPRun(runner, raw)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_cdp_list",
		Description: "List open tabs/targets in a Chromium browser connected via CDP",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "CDP debug URL (default: http://localhost:9222)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleCDPList(runner, raw)
	})
}

// defaultCDPURL is the default Chrome DevTools Protocol debug URL.
const defaultCDPURL = "http://localhost:9222"

func handleCDPRun(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		Script         string `json:"script"`
		URL            string `json:"url"`
		TargetURL      string `json:"target_url"`
		AwaitSelector  string `json:"await_selector"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Script == "" {
		return nil, fmt.Errorf("script is required")
	}

	debugURL := params.URL
	if debugURL == "" {
		debugURL = defaultCDPURL
	}

	opts := &cdp.RunOptions{
		TargetURL:      params.TargetURL,
		AwaitSelector:  params.AwaitSelector,
		TimeoutSeconds: params.TimeoutSeconds,
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, debugURL, params.Script, opts)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"result":     result.Result,
		"target_url": result.TargetURL,
		"elapsed_ms": result.ElapsedMs,
	}, nil
}

func handleCDPList(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}

	debugURL := params.URL
	if debugURL == "" {
		debugURL = defaultCDPURL
	}

	ctx := context.Background()
	targets, err := runner.ListTargets(ctx, debugURL)
	if err != nil {
		return nil, err
	}

	// Convert to []map for consistent JSON output
	result := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		result = append(result, map[string]any{
			"id":    t.ID,
			"type":  t.Type,
			"title": t.Title,
			"url":   t.URL,
		})
	}

	return map[string]any{
		"targets": result,
		"count":   len(result),
	}, nil
}
