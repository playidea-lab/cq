//go:build cdp


package cdphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	reg.Register(mcp.ToolSchema{
		Name: "c4_cdp_action",
		Description: "Interact with DOM elements via stable ref IDs (resolution-independent). " +
			"Workflow: scan_elements → discover refs → click/type/get_text by ref. " +
			"More reliable than raw JS for SPAs since refs persist across DOM updates.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":          map[string]any{"type": "string", "enum": []string{"scan_elements", "click", "type", "get_text"}, "description": "Action: scan_elements (assign refs), click, type, get_text"},
				"ref":             map[string]any{"type": "string", "description": "Element ref from scan_elements (e.g. c4r-3)"},
				"text":            map[string]any{"type": "string", "description": "Text to type (required for action=type)"},
				"url":             map[string]any{"type": "string", "description": "CDP debug URL (default: http://localhost:9222)"},
				"target_url":      map[string]any{"type": "string", "description": "Navigate to URL before action"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
			},
			"required": []string{"action"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleCDPAction(runner, raw)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_webmcp_discover",
		Description: "Discover WebMCP tools exposed by a web page via navigator.modelContext API (Chrome 146+). CDP auto-detected.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_url":        map[string]any{"type": "string", "description": "URL of the page to discover WebMCP tools from"},
				"cdp_url":         map[string]any{"type": "string", "description": "CDP debug URL (auto-detected if omitted)"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
				"wait_for_tools":  map[string]any{"type": "boolean", "description": "Wait for dynamically registered tools in SPAs (default: false)"},
			},
			"required": []string{"page_url"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWebMCPDiscover(runner, raw)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_webmcp_call",
		Description: "Call a WebMCP tool on a web page via navigator.modelContext API (Chrome 146+). CDP auto-detected.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_url":        map[string]any{"type": "string", "description": "URL of the page with the WebMCP tool"},
				"tool_name":       map[string]any{"type": "string", "description": "Name of the WebMCP tool to call"},
				"arguments":       map[string]any{"type": "object", "description": "Arguments to pass to the tool"},
				"cdp_url":         map[string]any{"type": "string", "description": "CDP debug URL (auto-detected if omitted)"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
			},
			"required": []string{"page_url", "tool_name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWebMCPCall(runner, raw)
	})

	reg.Register(mcp.ToolSchema{
		Name:        "c4_webmcp_context",
		Description: "Get, set, or clear page context via navigator.modelContext (provideContext/clearContext). CDP auto-detected.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_url":        map[string]any{"type": "string", "description": "URL of the page"},
				"action":          map[string]any{"type": "string", "enum": []string{"get", "provide", "clear"}, "description": "Action: get (read context), provide (set context), clear (remove context)"},
				"data":            map[string]any{"type": "object", "description": "Context data to provide (required for action=provide)"},
				"cdp_url":         map[string]any{"type": "string", "description": "CDP debug URL (auto-detected if omitted)"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
			},
			"required": []string{"page_url", "action"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleWebMCPContext(runner, raw)
	})
}

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

	debugURL := cdp.DiscoverCDPURL(params.URL)

	opts := &cdp.RunOptions{
		TargetURL:      params.TargetURL,
		AwaitSelector:  params.AwaitSelector,
		TimeoutSeconds: params.TimeoutSeconds,
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, debugURL, params.Script, opts)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
			return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
		}
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

	debugURL := cdp.DiscoverCDPURL(params.URL)

	ctx := context.Background()
	targets, err := runner.ListTargets(ctx, debugURL)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
			return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
		}
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

func handleCDPAction(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		Action         string `json:"action"`
		Ref            string `json:"ref"`
		Text           string `json:"text"`
		URL            string `json:"url"`
		TargetURL      string `json:"target_url"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Action == "" {
		return nil, fmt.Errorf("action is required (scan_elements, click, type, get_text)")
	}

	debugURL := cdp.DiscoverCDPURL(params.URL)
	ctx := context.Background()

	wrapErr := func(err error) error {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
			return fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
		}
		return err
	}

	switch params.Action {
	case "scan_elements":
		result, err := runner.ScanElements(ctx, debugURL, params.TargetURL, params.TimeoutSeconds)
		if err != nil {
			return nil, wrapErr(err)
		}
		return map[string]any{
			"elements":   result.Elements,
			"count":      result.Count,
			"elapsed_ms": result.ElapsedMs,
		}, nil

	case "click":
		if params.Ref == "" {
			return nil, fmt.Errorf("ref is required for action=click")
		}
		result, err := runner.ClickByRef(ctx, debugURL, params.Ref, params.TargetURL, params.TimeoutSeconds)
		if err != nil {
			return nil, wrapErr(err)
		}
		return map[string]any{
			"action":     result.Action,
			"ref":        result.Ref,
			"elapsed_ms": result.ElapsedMs,
		}, nil

	case "type":
		if params.Ref == "" {
			return nil, fmt.Errorf("ref is required for action=type")
		}
		if params.Text == "" {
			return nil, fmt.Errorf("text is required for action=type")
		}
		result, err := runner.TypeByRef(ctx, debugURL, params.Ref, params.Text, params.TargetURL, params.TimeoutSeconds)
		if err != nil {
			return nil, wrapErr(err)
		}
		return map[string]any{
			"action":     result.Action,
			"ref":        result.Ref,
			"elapsed_ms": result.ElapsedMs,
		}, nil

	case "get_text":
		if params.Ref == "" {
			return nil, fmt.Errorf("ref is required for action=get_text")
		}
		result, err := runner.GetTextByRef(ctx, debugURL, params.Ref, params.TargetURL, params.TimeoutSeconds)
		if err != nil {
			return nil, wrapErr(err)
		}
		return map[string]any{
			"action":     result.Action,
			"ref":        result.Ref,
			"value":      result.Value,
			"elapsed_ms": result.ElapsedMs,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action %q: must be scan_elements, click, type, or get_text", params.Action)
	}
}

func handleWebMCPDiscover(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		PageURL        string `json:"page_url"`
		CDPURL         string `json:"cdp_url"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		WaitForTools   bool   `json:"wait_for_tools"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.PageURL == "" {
		return nil, fmt.Errorf("page_url is required")
	}

	debugURL := cdp.DiscoverCDPURL(params.CDPURL)

	ctx := context.Background()
	opts := &cdp.DiscoverOpts{WaitForTools: params.WaitForTools}
	tools, err := runner.DiscoverWebMCPToolsWithOpts(ctx, debugURL, params.PageURL, params.TimeoutSeconds, opts)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
			return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
		}
		return nil, err
	}

	toolList := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		toolList = append(toolList, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
			"origin":      t.Origin,
		})
	}

	return map[string]any{
		"tools":     toolList,
		"count":     len(toolList),
		"page_url":  params.PageURL,
		"available": len(toolList) > 0,
	}, nil
}

func handleWebMCPCall(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		PageURL        string         `json:"page_url"`
		ToolName       string         `json:"tool_name"`
		Arguments      map[string]any `json:"arguments"`
		CDPURL         string         `json:"cdp_url"`
		TimeoutSeconds int            `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.PageURL == "" {
		return nil, fmt.Errorf("page_url is required")
	}
	if params.ToolName == "" {
		return nil, fmt.Errorf("tool_name is required")
	}

	debugURL := cdp.DiscoverCDPURL(params.CDPURL)

	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	ctx := context.Background()
	result, err := runner.CallWebMCPTool(ctx, debugURL, params.PageURL, params.ToolName, params.Arguments, params.TimeoutSeconds)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
			return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
		}
		return nil, err
	}

	return map[string]any{
		"result":     result.Result,
		"tool_name":  result.ToolName,
		"origin":     result.Origin,
		"elapsed_ms": result.ElapsedMs,
	}, nil
}

func handleWebMCPContext(runner *cdp.Runner, raw json.RawMessage) (any, error) {
	var params struct {
		PageURL        string         `json:"page_url"`
		Action         string         `json:"action"`
		Data           map[string]any `json:"data"`
		CDPURL         string         `json:"cdp_url"`
		TimeoutSeconds int            `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.PageURL == "" {
		return nil, fmt.Errorf("page_url is required")
	}
	if params.Action == "" {
		return nil, fmt.Errorf("action is required (get, provide, clear)")
	}

	debugURL := cdp.DiscoverCDPURL(params.CDPURL)
	ctx := context.Background()

	switch params.Action {
	case "get":
		result, err := runner.GetWebMCPContext(ctx, debugURL, params.PageURL, params.TimeoutSeconds)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
				return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
			}
			return nil, err
		}
		return map[string]any{
			"action":    result.Action,
			"context":   result.Context,
			"origin":    result.Origin,
			"available": result.Available,
		}, nil

	case "provide":
		if params.Data == nil {
			return nil, fmt.Errorf("data is required for action=provide")
		}
		result, err := runner.ProvideWebMCPContext(ctx, debugURL, params.PageURL, params.Data, params.TimeoutSeconds)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
				return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
			}
			return nil, err
		}
		return map[string]any{
			"action":    result.Action,
			"origin":    result.Origin,
			"available": result.Available,
		}, nil

	case "clear":
		result, err := runner.ClearWebMCPContext(ctx, debugURL, params.PageURL, params.TimeoutSeconds)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection") {
				return nil, fmt.Errorf("no Chrome browser found (tried %s). Start Chrome with: --remote-debugging-port=9222, or set CDP_DEBUG_URL env", debugURL)
			}
			return nil, err
		}
		return map[string]any{
			"action":    result.Action,
			"origin":    result.Origin,
			"available": result.Available,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action %q: must be get, provide, or clear", params.Action)
	}
}
