package cdp

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	// DefaultTimeout is the default script execution timeout.
	DefaultTimeout = 30 * time.Second
	// MaxTimeout is the maximum allowed script execution timeout.
	MaxTimeout = 300 * time.Second
	// ConnectTimeout is the timeout for establishing a CDP connection.
	ConnectTimeout = 5 * time.Second
)

// Runner executes JavaScript in Chromium apps via CDP.
type Runner struct{}

// RunOptions configures a CDP script execution.
type RunOptions struct {
	TargetURL      string // Navigate to this URL before running script
	AwaitSelector  string // Wait for CSS selector before running script
	TimeoutSeconds int    // Script timeout (default 30, max 300)
}

// Target represents a browser tab/target.
type Target struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// RunResult holds the result of a script execution.
type RunResult struct {
	Result    any    `json:"result"`
	TargetURL string `json:"target_url,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

// NewRunner creates a new CDP Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// resolveTimeout returns the effective timeout from RunOptions,
// clamped between DefaultTimeout and MaxTimeout.
func resolveTimeout(opts *RunOptions) time.Duration {
	if opts == nil || opts.TimeoutSeconds <= 0 {
		return DefaultTimeout
	}
	d := time.Duration(opts.TimeoutSeconds) * time.Second
	if d > MaxTimeout {
		return MaxTimeout
	}
	return d
}

// validateURL checks that the CDP debug URL is a valid localhost URL.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("cdp: url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("cdp: invalid url: %w", err)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("cdp: only localhost connections are allowed, got host %q", host)
	}
	return nil
}

// Execute connects to a Chromium debug port and runs JavaScript.
// url is the CDP debug URL (e.g., "http://localhost:9222").
// script is the JavaScript to execute.
func (r *Runner) Execute(ctx context.Context, debugURL, script string, opts *RunOptions) (*RunResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if script == "" {
		return nil, fmt.Errorf("cdp: script is required")
	}

	timeout := resolveTimeout(opts)

	// Create a context with the script timeout.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	// Connect to the remote browser via CDP WebSocket endpoint.
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// Build the action chain.
	var actions []chromedp.Action

	if opts != nil && opts.TargetURL != "" {
		actions = append(actions, chromedp.Navigate(opts.TargetURL))
	}
	if opts != nil && opts.AwaitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(opts.AwaitSelector, chromedp.ByQuery))
	}

	var result any
	actions = append(actions, chromedp.Evaluate(script, &result))

	if err := chromedp.Run(taskCtx, actions...); err != nil {
		return nil, fmt.Errorf("cdp: execute failed: %w", err)
	}

	elapsed := time.Since(start)
	res := &RunResult{
		Result:    result,
		ElapsedMs: elapsed.Milliseconds(),
	}
	if opts != nil && opts.TargetURL != "" {
		res.TargetURL = opts.TargetURL
	}
	return res, nil
}

// ListTargets returns open tabs/targets from a Chromium debug port.
func (r *Runner) ListTargets(ctx context.Context, debugURL string) ([]Target, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// We need to initialize the browser connection before listing targets.
	if err := chromedp.Run(taskCtx); err != nil {
		return nil, fmt.Errorf("cdp: connect failed: %w", err)
	}

	infos, err := chromedp.Targets(taskCtx)
	if err != nil {
		return nil, fmt.Errorf("cdp: list targets failed: %w", err)
	}

	targets := make([]Target, 0, len(infos))
	for _, info := range infos {
		targets = append(targets, Target{
			ID:    string(info.TargetID),
			Type:  string(info.Type),
			Title: info.Title,
			URL:   info.URL,
		})
	}
	return targets, nil
}
