package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
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

// --- Element-ref API (resolution-independent DOM interaction) ---
// Based on browser-use-demo pattern: scan DOM → assign stable refs → interact by ref.

// ElementRef describes a DOM element with its assigned data-cdp-ref attribute.
type ElementRef struct {
	Ref         string `json:"ref"`
	Tag         string `json:"tag"`
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Href        string `json:"href,omitempty"`
	Visible     bool   `json:"visible"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
}

// ScanResult holds element refs returned by ScanElements.
type ScanResult struct {
	Elements  []ElementRef `json:"elements"`
	Count     int          `json:"count"`
	ElapsedMs int64        `json:"elapsed_ms"`
}

// ActionResult holds the result of a ref-based DOM action.
type ActionResult struct {
	Action    string `json:"action"`
	Ref       string `json:"ref,omitempty"`
	Value     string `json:"value,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

// scanElementsJS assigns data-cdp-ref to interactive DOM elements and returns metadata.
const scanElementsJS = `(function() {
  var attr = 'data-cdp-ref';
  var sel = 'a,button,input,select,textarea,[role="button"],[role="link"],[role="textbox"],[onclick],[tabindex]:not([tabindex="-1"])';
  var els = Array.from(document.querySelectorAll(sel));
  var out = [];
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    var ref = 'c4r-' + i;
    el.setAttribute(attr, ref);
    var r = el.getBoundingClientRect();
    out.push({
      ref: ref,
      tag: el.tagName.toLowerCase(),
      type: el.getAttribute('type') || '',
      text: (el.innerText || el.value || el.getAttribute('aria-label') || '').slice(0, 120),
      placeholder: el.getAttribute('placeholder') || '',
      href: el.getAttribute('href') || '',
      visible: r.width > 0 && r.height > 0,
      x: Math.round(r.x), y: Math.round(r.y),
      w: Math.round(r.width), h: Math.round(r.height)
    });
  }
  return out;
})()`

// validRef accepts only refs generated by scanElementsJS (c4r-<digits>).
var validRef = regexp.MustCompile(`^c4r-\d+$`)

func validateRef(ref string) error {
	if !validRef.MatchString(ref) {
		return fmt.Errorf("cdp: invalid ref %q (expected format: c4r-N)", ref)
	}
	return nil
}

// ScanElements assigns data-cdp-ref attributes to interactive DOM elements and returns their metadata.
// Call this first to discover element refs, then use ClickByRef/TypeByRef/GetTextByRef.
func (r *Runner) ScanElements(ctx context.Context, debugURL, targetURL string, timeoutSeconds int) (*ScanResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	res, err := r.Execute(ctx, debugURL, scanElementsJS, &RunOptions{
		TargetURL:      targetURL,
		TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("cdp: scan elements: %w", err)
	}
	// chromedp returns JS arrays as []interface{}; re-marshal for typed parsing.
	raw, err := json.Marshal(res.Result)
	if err != nil {
		return nil, fmt.Errorf("cdp: scan elements marshal: %w", err)
	}
	var elements []ElementRef
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("cdp: scan elements parse: %w", err)
	}
	return &ScanResult{
		Elements:  elements,
		Count:     len(elements),
		ElapsedMs: res.ElapsedMs,
	}, nil
}

// ClickByRef clicks the element identified by a ref from ScanElements.
func (r *Runner) ClickByRef(ctx context.Context, debugURL, ref, targetURL string, timeoutSeconds int) (*ActionResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if err := validateRef(ref); err != nil {
		return nil, err
	}

	timeout := resolveTimeout(&RunOptions{TimeoutSeconds: timeoutSeconds})
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	selector := fmt.Sprintf(`[data-cdp-ref="%s"]`, ref)
	var actions []chromedp.Action
	if targetURL != "" {
		actions = append(actions, chromedp.Navigate(targetURL))
	}
	actions = append(actions, chromedp.Click(selector, chromedp.ByQuery))

	if err := chromedp.Run(taskCtx, actions...); err != nil {
		return nil, fmt.Errorf("cdp: click %s: %w", ref, err)
	}
	return &ActionResult{Action: "click", Ref: ref, ElapsedMs: time.Since(start).Milliseconds()}, nil
}

// TypeByRef clears and types text into the element identified by a ref from ScanElements.
func (r *Runner) TypeByRef(ctx context.Context, debugURL, ref, text, targetURL string, timeoutSeconds int) (*ActionResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if err := validateRef(ref); err != nil {
		return nil, err
	}

	timeout := resolveTimeout(&RunOptions{TimeoutSeconds: timeoutSeconds})
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	selector := fmt.Sprintf(`[data-cdp-ref="%s"]`, ref)
	var actions []chromedp.Action
	if targetURL != "" {
		actions = append(actions, chromedp.Navigate(targetURL))
	}
	actions = append(actions, chromedp.Clear(selector, chromedp.ByQuery))
	actions = append(actions, chromedp.SendKeys(selector, text, chromedp.ByQuery))

	if err := chromedp.Run(taskCtx, actions...); err != nil {
		return nil, fmt.Errorf("cdp: type into %s: %w", ref, err)
	}
	return &ActionResult{Action: "type", Ref: ref, Value: text, ElapsedMs: time.Since(start).Milliseconds()}, nil
}

// GetTextByRef returns the text content of the element identified by a ref from ScanElements.
func (r *Runner) GetTextByRef(ctx context.Context, debugURL, ref, targetURL string, timeoutSeconds int) (*ActionResult, error) {
	if err := validateURL(debugURL); err != nil {
		return nil, err
	}
	if err := validateRef(ref); err != nil {
		return nil, err
	}
	script := fmt.Sprintf(`(function() {
  var el = document.querySelector('[data-cdp-ref="%s"]');
  if (!el) return null;
  return el.innerText || el.value || el.textContent || '';
})()`, ref)
	res, err := r.Execute(ctx, debugURL, script, &RunOptions{
		TargetURL:      targetURL,
		TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("cdp: get text %s: %w", ref, err)
	}
	text := ""
	if s, ok := res.Result.(string); ok {
		text = s
	}
	return &ActionResult{Action: "get_text", Ref: ref, Value: text, ElapsedMs: res.ElapsedMs}, nil
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
