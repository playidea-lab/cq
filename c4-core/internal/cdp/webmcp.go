package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// WebMCPTool represents a tool exposed by a website via WebMCP (navigator.modelContext).
type WebMCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Origin      string         `json:"origin"`
}

// WebMCPCallResult holds the result of calling a WebMCP tool.
type WebMCPCallResult struct {
	Result    any    `json:"result"`
	ToolName  string `json:"tool_name"`
	Origin    string `json:"origin"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

// DiscoverWebMCPTools navigates to a URL and discovers WebMCP tools via navigator.modelContext.
// If navigator.modelContext is not available, returns an empty list (not an error).
func (r *Runner) DiscoverWebMCPTools(ctx context.Context, debugURL, pageURL string, timeoutSecs int) ([]WebMCPTool, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if pageURL == "" {
		return nil, fmt.Errorf("cdp: page_url is required for WebMCP discovery")
	}

	timeout := DefaultTimeout
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
		if timeout > MaxTimeout {
			timeout = MaxTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var resultJSON string
	script := `
	(async () => {
		if (typeof navigator === 'undefined' || !navigator.modelContext) {
			return JSON.stringify({available: false, tools: []});
		}
		try {
			const tools = await navigator.modelContext.getTools();
			const mapped = (tools || []).map(t => ({
				name: t.name || '',
				description: t.description || '',
				inputSchema: t.inputSchema || {},
				origin: window.location.origin
			}));
			return JSON.stringify({available: true, tools: mapped});
		} catch(e) {
			return JSON.stringify({available: false, error: e.message, tools: []});
		}
	})()
	`

	err := chromedp.Run(taskCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(script, &resultJSON, chromedp.EvalAsValue),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp: webmcp discover failed: %w", err)
	}

	var parsed struct {
		Available bool         `json:"available"`
		Tools     []WebMCPTool `json:"tools"`
		Error     string       `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return nil, fmt.Errorf("cdp: parse webmcp result: %w", err)
	}

	if !parsed.Available {
		if parsed.Error != "" {
			return nil, fmt.Errorf("cdp: WebMCP not available: %s", parsed.Error)
		}
		return []WebMCPTool{}, nil
	}

	return parsed.Tools, nil
}

// CallWebMCPTool calls a specific WebMCP tool on a page via navigator.modelContext.callTool.
func (r *Runner) CallWebMCPTool(ctx context.Context, debugURL, pageURL, toolName string, args map[string]any, timeoutSecs int) (*WebMCPCallResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if pageURL == "" {
		return nil, fmt.Errorf("cdp: page_url is required")
	}
	if toolName == "" {
		return nil, fmt.Errorf("cdp: tool_name is required")
	}

	timeout := DefaultTimeout
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
		if timeout > MaxTimeout {
			timeout = MaxTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("cdp: marshal args: %w", err)
	}

	var resultJSON string
	script := fmt.Sprintf(`
	(async () => {
		if (!navigator.modelContext) {
			return JSON.stringify({error: "WebMCP not available"});
		}
		try {
			const result = await navigator.modelContext.callTool(%q, %s);
			return JSON.stringify({success: true, result: result, origin: window.location.origin});
		} catch(e) {
			return JSON.stringify({error: e.message});
		}
	})()
	`, toolName, string(argsJSON))

	err = chromedp.Run(taskCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(script, &resultJSON, chromedp.EvalAsValue),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp: webmcp call failed: %w", err)
	}

	var parsed struct {
		Success bool   `json:"success"`
		Result  any    `json:"result"`
		Origin  string `json:"origin"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return nil, fmt.Errorf("cdp: parse webmcp call result: %w", err)
	}

	if parsed.Error != "" {
		return nil, fmt.Errorf("cdp: WebMCP tool %q error: %s", toolName, parsed.Error)
	}

	return &WebMCPCallResult{
		Result:    parsed.Result,
		ToolName:  toolName,
		Origin:    parsed.Origin,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}
