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

// DiscoverOpts configures WebMCP tool discovery behavior.
type DiscoverOpts struct {
	// WaitForTools waits for dynamically registered tools (SPA support).
	// Polls every 500ms until tools appear or timeout is reached.
	WaitForTools bool
}

// DiscoverWebMCPTools navigates to a URL and discovers WebMCP tools via navigator.modelContext.
// If navigator.modelContext is not available, returns an empty list (not an error).
func (r *Runner) DiscoverWebMCPTools(ctx context.Context, debugURL, pageURL string, timeoutSecs int) ([]WebMCPTool, error) {
	return r.DiscoverWebMCPToolsWithOpts(ctx, debugURL, pageURL, timeoutSecs, nil)
}

// DiscoverWebMCPToolsWithOpts discovers WebMCP tools with additional options.
func (r *Runner) DiscoverWebMCPToolsWithOpts(ctx context.Context, debugURL, pageURL string, timeoutSecs int, opts *DiscoverOpts) ([]WebMCPTool, error) {
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

	waitForTools := opts != nil && opts.WaitForTools
	pollInterval := 500 // ms

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// Script that discovers tools, with optional polling for SPA dynamic registration.
	script := fmt.Sprintf(`
	(async () => {
		if (typeof navigator === 'undefined' || !navigator.modelContext) {
			return JSON.stringify({available: false, tools: []});
		}
		const waitForTools = %v;
		const pollInterval = %d;
		const deadline = Date.now() + %d;

		async function getToolList() {
			try {
				const tools = await navigator.modelContext.getTools();
				return (tools || []).map(t => ({
					name: t.name || '',
					description: t.description || '',
					inputSchema: t.inputSchema || {},
					origin: window.location.origin
				}));
			} catch(e) {
				return [];
			}
		}

		let mapped = await getToolList();
		if (waitForTools && mapped.length === 0) {
			while (mapped.length === 0 && Date.now() < deadline) {
				await new Promise(r => setTimeout(r, pollInterval));
				mapped = await getToolList();
			}
		}

		return JSON.stringify({
			available: true,
			tools: mapped,
			waited: waitForTools && mapped.length > 0
		});
	})()
	`, waitForTools, pollInterval, timeout.Milliseconds())

	var resultJSON string
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

// WebMCPContextResult holds the result of a context operation.
type WebMCPContextResult struct {
	Context   any    `json:"context,omitempty"`
	Action    string `json:"action"`
	Origin    string `json:"origin"`
	Available bool   `json:"available"`
}

// GetWebMCPContext retrieves the current page context via navigator.modelContext.
func (r *Runner) GetWebMCPContext(ctx context.Context, debugURL, pageURL string, timeoutSecs int) (*WebMCPContextResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if pageURL == "" {
		return nil, fmt.Errorf("cdp: page_url is required")
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
		if (!navigator.modelContext) {
			return JSON.stringify({available: false});
		}
		try {
			const ctx = navigator.modelContext._context || null;
			return JSON.stringify({available: true, context: ctx, origin: window.location.origin});
		} catch(e) {
			return JSON.stringify({available: false, error: e.message});
		}
	})()
	`

	err := chromedp.Run(taskCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(script, &resultJSON, chromedp.EvalAsValue),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp: webmcp context get failed: %w", err)
	}

	var parsed struct {
		Available bool   `json:"available"`
		Context   any    `json:"context"`
		Origin    string `json:"origin"`
		Error     string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return nil, fmt.Errorf("cdp: parse context result: %w", err)
	}

	return &WebMCPContextResult{
		Context:   parsed.Context,
		Action:    "get",
		Origin:    parsed.Origin,
		Available: parsed.Available,
	}, nil
}

// ProvideWebMCPContext sets page context via navigator.modelContext.provideContext.
func (r *Runner) ProvideWebMCPContext(ctx context.Context, debugURL, pageURL string, data map[string]any, timeoutSecs int) (*WebMCPContextResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if pageURL == "" {
		return nil, fmt.Errorf("cdp: page_url is required")
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

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("cdp: marshal context data: %w", err)
	}

	var resultJSON string
	script := fmt.Sprintf(`
	(async () => {
		if (!navigator.modelContext) {
			return JSON.stringify({available: false});
		}
		try {
			if (navigator.modelContext.provideContext) {
				await navigator.modelContext.provideContext(%s);
			} else {
				navigator.modelContext._context = %s;
			}
			return JSON.stringify({available: true, action: "provide", origin: window.location.origin});
		} catch(e) {
			return JSON.stringify({available: false, error: e.message});
		}
	})()
	`, string(dataJSON), string(dataJSON))

	err = chromedp.Run(taskCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(script, &resultJSON, chromedp.EvalAsValue),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp: webmcp context provide failed: %w", err)
	}

	var parsed struct {
		Available bool   `json:"available"`
		Origin    string `json:"origin"`
		Error     string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return nil, fmt.Errorf("cdp: parse context result: %w", err)
	}

	if !parsed.Available && parsed.Error != "" {
		return nil, fmt.Errorf("cdp: provideContext error: %s", parsed.Error)
	}

	return &WebMCPContextResult{
		Action:    "provide",
		Origin:    parsed.Origin,
		Available: parsed.Available,
	}, nil
}

// ClearWebMCPContext clears page context via navigator.modelContext.clearContext.
func (r *Runner) ClearWebMCPContext(ctx context.Context, debugURL, pageURL string, timeoutSecs int) (*WebMCPContextResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if pageURL == "" {
		return nil, fmt.Errorf("cdp: page_url is required")
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
		if (!navigator.modelContext) {
			return JSON.stringify({available: false});
		}
		try {
			if (navigator.modelContext.clearContext) {
				await navigator.modelContext.clearContext();
			} else {
				navigator.modelContext._context = null;
			}
			return JSON.stringify({available: true, action: "clear", origin: window.location.origin});
		} catch(e) {
			return JSON.stringify({available: false, error: e.message});
		}
	})()
	`

	err := chromedp.Run(taskCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(script, &resultJSON, chromedp.EvalAsValue),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp: webmcp context clear failed: %w", err)
	}

	var parsed struct {
		Available bool   `json:"available"`
		Origin    string `json:"origin"`
		Error     string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return nil, fmt.Errorf("cdp: parse context result: %w", err)
	}

	return &WebMCPContextResult{
		Action:    "clear",
		Origin:    parsed.Origin,
		Available: parsed.Available,
	}, nil
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
