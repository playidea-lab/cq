package eventbus

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// C1Poster posts messages to C1 channels (implemented by ContextKeeper).
type C1Poster interface {
	AutoPost(channelName, content string) error
}

// Dispatcher evaluates rules against events and executes matched actions.
type Dispatcher struct {
	store      *Store
	mu         sync.RWMutex
	rpcAddr    string // JSON-RPC sidecar address (e.g. "127.0.0.1:50051")
	httpClient *http.Client
	c1Poster   C1Poster // optional: for "c1_post" action type
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher(store *Store) *Dispatcher {
	return &Dispatcher{
		store: store,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Close releases resources held by the dispatcher.
func (d *Dispatcher) Close() error {
	if d.store != nil {
		return d.store.Close()
	}
	return nil
}

// SetRPCAddr sets the JSON-RPC sidecar address for "rpc" action type.
func (d *Dispatcher) SetRPCAddr(addr string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rpcAddr = addr
}

// SetC1Poster sets the C1 poster for "c1_post" action type.
func (d *Dispatcher) SetC1Poster(poster C1Poster) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.c1Poster = poster
}

// Dispatch matches rules against an event and executes their actions.
// It runs each action in a goroutine and logs the result.
func (d *Dispatcher) Dispatch(eventID, eventType string, eventData json.RawMessage) {
	rules, err := d.store.MatchRules(eventType)
	if err != nil {
		log.Printf("[eventbus] match rules for %s: %v\n", eventType, err)
		return
	}

	if len(rules) == 0 {
		return
	}

	for _, rule := range rules {
		go d.executeRule(eventID, eventType, eventData, rule)
	}
}

// DispatchSync matches and executes rules synchronously (for testing).
func (d *Dispatcher) DispatchSync(eventID, eventType string, eventData json.RawMessage) {
	rules, err := d.store.MatchRules(eventType)
	if err != nil {
		log.Printf("[eventbus] match rules for %s: %v\n", eventType, err)
		return
	}

	for _, rule := range rules {
		d.executeRule(eventID, eventType, eventData, rule)
	}
}

func (d *Dispatcher) executeRule(eventID, eventType string, eventData json.RawMessage, rule StoredRule) {
	// Check filter
	if rule.FilterJSON != "" && rule.FilterJSON != "{}" {
		if !evaluateFilter(rule.FilterJSON, eventData) {
			return
		}
	}

	start := time.Now()
	var err error

	switch rule.ActionType {
	case "log":
		err = d.executeLog(eventID, eventType, eventData, rule)
	case "rpc":
		err = d.executeRPC(eventData, rule)
	case "webhook":
		err = d.executeWebhook(eventID, eventType, eventData, rule)
	case "c1_post":
		err = d.executeC1Post(eventType, eventData, rule)
	default:
		err = fmt.Errorf("unknown action type: %s", rule.ActionType)
	}

	duration := time.Since(start).Milliseconds()
	logStatus := "ok"
	errMsg := ""
	if err != nil {
		logStatus = "error"
		errMsg = err.Error()
		log.Printf("[eventbus] rule %q dispatch error: %v\n", rule.Name, err)
		// Insert into Dead Letter Queue for retry
		d.store.InsertDLQ(eventID, rule.ID, rule.Name, eventType, errMsg, 3)
	}

	d.store.LogDispatch(eventID, rule.ID, logStatus, errMsg, duration)
}

func (d *Dispatcher) executeLog(eventID, eventType string, eventData json.RawMessage, rule StoredRule) error {
	log.Printf("[eventbus] [%s] event=%s id=%s data=%s\n", rule.Name, eventType, eventID, string(eventData))
	return nil
}

func (d *Dispatcher) executeRPC(eventData json.RawMessage, rule StoredRule) error {
	d.mu.RLock()
	addr := d.rpcAddr
	d.mu.RUnlock()

	if addr == "" {
		return fmt.Errorf("rpc address not configured")
	}

	var cfg struct {
		Method       string         `json:"method"`
		ArgsTemplate map[string]any `json:"args_template"`
	}
	if err := json.Unmarshal([]byte(rule.ActionConfig), &cfg); err != nil {
		return fmt.Errorf("parse action_config: %w", err)
	}
	if cfg.Method == "" {
		return fmt.Errorf("rpc method not specified in action_config")
	}

	// Resolve template variables from event data
	args := resolveTemplate(cfg.ArgsTemplate, eventData)

	// Build JSON-RPC request
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  cfg.Method,
		"params":  args,
	}
	body, _ := json.Marshal(rpcReq)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to sidecar: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}

	// Read response (single line)
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read rpc response: %w", err)
	}

	var rpcResp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf[:n], &rpcResp); err != nil {
		return fmt.Errorf("parse rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error: %s", rpcResp.Error.Message)
	}

	return nil
}

func (d *Dispatcher) executeWebhook(eventID, eventType string, eventData json.RawMessage, rule StoredRule) error {
	var cfg struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Secret  string            `json:"secret"` // HMAC secret (optional)
	}
	if err := json.Unmarshal([]byte(rule.ActionConfig), &cfg); err != nil {
		return fmt.Errorf("parse action_config: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("webhook url not specified in action_config")
	}

	// Validate webhook URL to prevent SSRF
	if err := validateWebhookURL(cfg.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// CloudEvents-style payload
	payload := map[string]any{
		"id":     eventID,
		"type":   eventType,
		"source": "c4.eventbus",
		"data":   json.RawMessage(eventData),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// HMAC-SHA256 signing if secret is configured
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-C4-Signature", "sha256="+sig)
		req.Header.Set("X-C4-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) executeC1Post(eventType string, eventData json.RawMessage, rule StoredRule) error {
	d.mu.RLock()
	poster := d.c1Poster
	d.mu.RUnlock()

	if poster == nil {
		return fmt.Errorf("c1 poster not configured")
	}

	var cfg struct {
		Channel  string `json:"channel"`
		Template string `json:"template"`
	}
	if err := json.Unmarshal([]byte(rule.ActionConfig), &cfg); err != nil {
		return fmt.Errorf("parse action_config: %w", err)
	}
	if cfg.Channel == "" {
		cfg.Channel = "#updates"
	}

	// Build data map including event_type for template resolution
	var data map[string]any
	if err := json.Unmarshal(eventData, &data); err != nil {
		data = make(map[string]any)
	}
	// Extract short event type: "task.completed" → "completed"
	shortType := eventType
	if idx := strings.LastIndex(eventType, "."); idx >= 0 {
		shortType = eventType[idx+1:]
	}
	data["event_type"] = shortType

	msg := cfg.Template
	if msg != "" {
		msg = resolveTemplateString(msg, data)
	} else {
		// Default format
		taskID, _ := data["task_id"].(string)
		title, _ := data["title"].(string)
		msg = fmt.Sprintf("[%s] %s: %s", shortType, taskID, title)
	}

	return poster.AutoPost(cfg.Channel, msg)
}

// evaluateFilter checks if event data matches the filter JSON.
// Supports v2 operators ($eq, $ne, $gt, $lt, $in, $regex, $exists) and dot notation for nested fields.
// Backward compatible: plain {"key": "value"} is treated as $eq.
func evaluateFilter(filterJSON string, eventData json.RawMessage) bool {
	var filter map[string]any
	if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
		return false
	}

	var data map[string]any
	if err := json.Unmarshal(eventData, &data); err != nil {
		return false
	}

	for k, v := range filter {
		dataVal, exists := resolveNestedField(data, k)

		switch cond := v.(type) {
		case map[string]any:
			// Operator mode: {"field": {"$gt": 100}}
			if !evaluateOperators(cond, dataVal, exists) {
				return false
			}
		default:
			// Simple equality (backward compatible)
			if !exists {
				return false
			}
			if fmt.Sprintf("%v", dataVal) != fmt.Sprintf("%v", v) {
				return false
			}
		}
	}
	return true
}

// resolveNestedField resolves dot-notation paths like "data.nested.field".
func resolveNestedField(data map[string]any, dotPath string) (any, bool) {
	parts := strings.Split(dotPath, ".")
	var current any = data

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// evaluateOperators checks a value against operator conditions.
func evaluateOperators(operators map[string]any, value any, exists bool) bool {
	for op, expected := range operators {
		switch op {
		case "$eq":
			if !exists || fmt.Sprintf("%v", value) != fmt.Sprintf("%v", expected) {
				return false
			}
		case "$ne":
			if exists && fmt.Sprintf("%v", value) == fmt.Sprintf("%v", expected) {
				return false
			}
		case "$gt":
			if !exists {
				return false
			}
			if !compareNumeric(value, expected, func(a, b float64) bool { return a > b }) {
				return false
			}
		case "$lt":
			if !exists {
				return false
			}
			if !compareNumeric(value, expected, func(a, b float64) bool { return a < b }) {
				return false
			}
		case "$in":
			if !exists {
				return false
			}
			arr, ok := expected.([]any)
			if !ok {
				return false
			}
			found := false
			valStr := fmt.Sprintf("%v", value)
			for _, item := range arr {
				if fmt.Sprintf("%v", item) == valStr {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case "$regex":
			if !exists {
				return false
			}
			pattern, ok := expected.(string)
			if !ok {
				return false
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false
			}
			if !re.MatchString(fmt.Sprintf("%v", value)) {
				return false
			}
		case "$exists":
			wantExists, ok := expected.(bool)
			if !ok {
				return false
			}
			if wantExists != exists {
				return false
			}
		default:
			// Unknown operator — skip
		}
	}
	return true
}

// compareNumeric converts values to float64 and applies the comparator.
func compareNumeric(a, b any, cmp func(float64, float64) bool) bool {
	af, aOK := toFloat64(a)
	bf, bOK := toFloat64(b)
	if !aOK || !bOK {
		return false
	}
	return cmp(af, bf)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// resolveTemplate replaces {{data.field}} placeholders in template values
// with values from the event data.
func resolveTemplate(template map[string]any, eventData json.RawMessage) map[string]any {
	if template == nil {
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal(eventData, &data); err != nil {
		return template
	}

	result := make(map[string]any, len(template))
	for k, v := range template {
		if s, ok := v.(string); ok && strings.Contains(s, "{{") {
			result[k] = resolveTemplateString(s, data)
		} else {
			result[k] = v
		}
	}
	return result
}

// resolveTemplateString replaces {{data.key}} with actual values.
func resolveTemplateString(s string, data map[string]any) string {
	result := s
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		end += start

		key := strings.TrimSpace(result[start+2 : end])
		// Remove "data." prefix if present
		key = strings.TrimPrefix(key, "data.")

		val := ""
		if v, ok := data[key]; ok {
			val = fmt.Sprintf("%v", v)
		}

		result = result[:start] + val + result[end+2:]
	}
	return result
}

// validateWebhookURL validates a webhook URL to prevent SSRF attacks.
// It checks that:
// - The URL scheme is http or https
// - The resolved IP addresses are not private/internal ranges
func validateWebhookURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	// Check scheme is http or https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %s", parsedURL.Scheme)
	}

	// Extract hostname
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("missing hostname")
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("resolve hostname: %w", err)
	}

	// Check all resolved IPs
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		// Check for private/internal ranges
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL resolves to private IP: %s", ipStr)
		}
	}

	return nil
}

var privateCIDRs []*net.IPNet

func init() {
	// Initialize private CIDR ranges once at package init
	privateRanges := []string{
		"127.0.0.0/8",    // Loopback
		"10.0.0.0/8",     // Private
		"172.16.0.0/12",  // Private
		"192.168.0.0/16", // Private
		"169.254.0.0/16", // Link-local
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil {
			privateCIDRs = append(privateCIDRs, network)
		}
	}
}

// isPrivateIP checks if an IP is in a private/internal range.
func isPrivateIP(ip net.IP) bool {
	// Check for unspecified (0.0.0.0 or ::)
	if ip.IsUnspecified() {
		return true
	}

	// IPv4 private ranges (from init)
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}

	// IPv6 loopback
	if ip.IsLoopback() {
		return true
	}

	// IPv6 link-local (fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// IPv6 unique local addresses (fc00::/7)
	if len(ip) == 16 && (ip[0] == 0xfc || ip[0] == 0xfd) {
		return true
	}

	return false
}
