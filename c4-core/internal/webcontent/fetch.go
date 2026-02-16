package webcontent

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FetchOpts controls content fetch behavior.
type FetchOpts struct {
	PreferMarkdown bool          // Try text/markdown first (default: true)
	MaxBodyBytes   int64         // Max response body size (default: 5MB)
	Timeout        time.Duration // HTTP timeout (default: 15s)
	FollowLLMSTxt  bool          // Also check for llms.txt
	SkipSSRFCheck  bool          // Skip SSRF validation (for testing only)
}

// FetchResult holds the fetched and converted content.
type FetchResult struct {
	Content       string `json:"content"`
	URL           string `json:"url"`
	ContentType   string `json:"content_type"`
	Method        string `json:"method"` // "markdown_native", "html_converted", "plain"
	Title         string `json:"title"`
	TokenEstimate int    `json:"token_estimate"`
}

const (
	defaultMaxBodyBytes = 5 * 1024 * 1024 // 5MB
	defaultTimeout      = 15 * time.Second
	maxRedirects        = 5
	userAgent           = "C4/1.0 (+https://github.com/changmin/c4; agent-fetch)"
)

// Rate limiter: per-origin, max 10 requests per minute.
var (
	rateMu      sync.Mutex
	rateBuckets = map[string][]time.Time{}
	rateLimit   = 10
	rateWindow  = time.Minute
	testMode    bool // disables SSRF checks globally (for handler tests)
)

// SetTestMode disables SSRF checks globally (for testing handlers that call Fetch internally).
func SetTestMode(enabled bool) {
	testMode = enabled
}

// Fetch retrieves content from a URL and converts it to markdown.
//
// Content negotiation strategy:
//  1. Try Accept: text/markdown header -> if response is text/markdown, use directly
//  2. If HTML received, check for <link rel="alternate" type="text/markdown"> -> fetch that URL
//  3. Fallback: convert HTML to markdown using html-to-markdown
func Fetch(rawURL string, opts *FetchOpts) (*FetchResult, error) {
	if opts == nil {
		opts = &FetchOpts{}
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = defaultMaxBodyBytes
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if !opts.SkipSSRFCheck && !testMode {
		if err := validateURL(parsed); err != nil {
			return nil, err
		}
	}

	origin := parsed.Scheme + "://" + parsed.Host
	if err := checkRateLimit(origin); err != nil {
		return nil, err
	}

	skipSSRF := opts.SkipSSRFCheck
	client := &http.Client{
		Timeout: opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			if !skipSSRF && !testMode {
				if err := validateURL(req.URL); err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/markdown, text/html;q=0.9, text/plain;q=0.8, */*;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, opts.MaxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	bodyStr := string(body)

	result := &FetchResult{
		URL:         rawURL,
		ContentType: ct,
	}

	switch {
	case strings.Contains(ct, "text/markdown"):
		result.Content = bodyStr
		result.Method = "markdown_native"
		result.Title = extractMarkdownTitle(bodyStr)

	case strings.Contains(ct, "text/html"):
		// Check for <link rel="alternate" type="text/markdown">
		if altURL := findAlternateMarkdown(bodyStr, parsed); altURL != "" {
			altResult, err := fetchDirect(client, altURL, opts.MaxBodyBytes)
			if err == nil && altResult != "" {
				result.Content = altResult
				result.Method = "markdown_native"
				result.Title = ExtractTitle(bodyStr)
				break
			}
		}

		// Convert HTML to markdown
		md, err := ConvertHTMLToMarkdown(bodyStr)
		if err != nil {
			result.Content = bodyStr
			result.Method = "plain"
		} else {
			result.Content = md
			result.Method = "html_converted"
		}
		result.Title = ExtractTitle(bodyStr)

	default:
		result.Content = bodyStr
		result.Method = "plain"
	}

	result.TokenEstimate = len(result.Content) / 4

	return result, nil
}

// validateURL checks for SSRF and scheme restrictions.
func validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s (only http/https allowed)", u.Scheme)
	}

	host := u.Hostname()

	// Block localhost variants
	if host == "localhost" || host == "0.0.0.0" {
		return fmt.Errorf("SSRF blocked: %s", host)
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("SSRF blocked: private IP %s", ip)
		}
	} else {
		// Resolve hostname and check IPs
		addrs, err := net.LookupHost(host)
		if err == nil {
			for _, addr := range addrs {
				if resolved := net.ParseIP(addr); resolved != nil && isPrivateIP(resolved) {
					return fmt.Errorf("SSRF blocked: %s resolves to private IP %s", host, addr)
				}
			}
		}
	}

	return nil
}

// isPrivateIP checks if an IP is in a private/loopback/link-local range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"127.0.0.0/8"},
		{"169.254.0.0/16"},
		{"::1/128"},
		{"fd00::/8"},
		{"fe80::/10"},
	}

	for _, r := range privateRanges {
		_, cidr, err := net.ParseCIDR(r.network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// checkRateLimit enforces per-origin rate limiting.
func checkRateLimit(origin string) error {
	rateMu.Lock()
	defer rateMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateWindow)

	// Clean old entries
	times := rateBuckets[origin]
	var valid []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rateLimit {
		return fmt.Errorf("rate limit exceeded for %s (max %d/min)", origin, rateLimit)
	}

	rateBuckets[origin] = append(valid, now)
	return nil
}

// findAlternateMarkdown looks for <link rel="alternate" type="text/markdown" href="...">
// and returns the absolute URL if found.
func findAlternateMarkdown(html string, base *url.URL) string {
	re := regexp.MustCompile(`(?i)<link[^>]+rel=["']alternate["'][^>]+type=["']text/markdown["'][^>]+href=["']([^"']+)["']`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		// Try reversed attribute order
		re2 := regexp.MustCompile(`(?i)<link[^>]+type=["']text/markdown["'][^>]+rel=["']alternate["'][^>]+href=["']([^"']+)["']`)
		m = re2.FindStringSubmatch(html)
	}
	if len(m) < 2 {
		return ""
	}

	href := m[1]
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}

	return base.ResolveReference(u).String()
}

// fetchDirect does a simple GET to fetch raw content.
func fetchDirect(client *http.Client, rawURL string, maxBytes int64) (string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// extractMarkdownTitle finds the first # heading in markdown content.
func extractMarkdownTitle(md string) string {
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	return ""
}
