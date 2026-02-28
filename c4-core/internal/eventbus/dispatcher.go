package eventbus

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	store        *Store
	mu           sync.RWMutex
	httpClient   *http.Client
	c1Poster     C1Poster     // optional: for "c1_post" action type
	hubSubmitter JobSubmitter // optional: for "hub_submit" action type
	sem          chan struct{} // bounded concurrency for dispatch goroutines
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher(store *Store) *Dispatcher {
	return &Dispatcher{
		store: store,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		sem: make(chan struct{}, 32), // max 32 concurrent dispatches
	}
}

// Close releases resources held by the dispatcher.
func (d *Dispatcher) Close() error {
	if d.store != nil {
		return d.store.Close()
	}
	return nil
}

// SetC1Poster sets the C1 poster for "c1_post" action type.
func (d *Dispatcher) SetC1Poster(poster C1Poster) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.c1Poster = poster
}

// SetHubSubmitter sets the Hub submitter for "hub_submit" action type.
func (d *Dispatcher) SetHubSubmitter(s JobSubmitter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hubSubmitter = s
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
		d.sem <- struct{}{} // acquire semaphore
		go func(r StoredRule) {
			defer func() { <-d.sem }() // release semaphore
			d.executeRule(eventID, eventType, eventData, r)
		}(rule)
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

// ReplayRule re-executes a specific rule for a specific event.
// Returns nil on success, error on failure. Does NOT insert into DLQ on failure
// (caller is responsible for DLQ management during replay).
func (d *Dispatcher) ReplayRule(eventID, eventType string, eventData json.RawMessage, ruleID string) error {
	rules, err := d.store.MatchRules(eventType)
	if err != nil {
		return fmt.Errorf("match rules: %w", err)
	}

	for _, rule := range rules {
		if rule.ID != ruleID {
			continue
		}

		if rule.FilterJSON != "" && rule.FilterJSON != "{}" {
			if !evaluateFilter(rule.FilterJSON, eventData) {
				return nil // filter excluded — not an error
			}
		}

		start := time.Now()
		var execErr error
		switch rule.ActionType {
		case "log":
			execErr = d.executeLog(eventID, eventType, eventData, rule)
		case "webhook":
			execErr = d.executeWebhook(eventID, eventType, eventData, rule)
		case "c1_post":
			execErr = d.executeC1Post(eventType, eventData, rule)
		case "hub_submit":
			execErr = d.executeHubSubmit(eventType, eventData, rule)
		default:
			execErr = fmt.Errorf("unknown action type: %s", rule.ActionType)
		}

		duration := time.Since(start).Milliseconds()
		logStatus := "ok"
		errMsg := ""
		if execErr != nil {
			logStatus = "replay_error"
			errMsg = execErr.Error()
		}
		d.store.LogDispatch(eventID, rule.ID, logStatus, errMsg, duration)
		return execErr
	}

	return fmt.Errorf("rule %s not found for event type %s", ruleID, eventType)
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
	case "webhook":
		err = d.executeWebhook(eventID, eventType, eventData, rule)
	case "c1_post":
		err = d.executeC1Post(eventType, eventData, rule)
	case "hub_submit":
		err = d.executeHubSubmit(eventType, eventData, rule)
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
		// Extract max_retries from action_config (default: 3)
		maxRetries := 3
		var cfgBase struct {
			MaxRetries int `json:"max_retries"`
		}
		if json.Unmarshal([]byte(rule.ActionConfig), &cfgBase) == nil && cfgBase.MaxRetries > 0 {
			maxRetries = cfgBase.MaxRetries
		}
		d.store.InsertDLQ(eventID, rule.ID, rule.Name, eventType, errMsg, maxRetries)
	}

	d.store.LogDispatch(eventID, rule.ID, logStatus, errMsg, duration)
}

func (d *Dispatcher) executeLog(eventID, eventType string, eventData json.RawMessage, rule StoredRule) error {
	log.Printf("[eventbus] [%s] event=%s id=%s data=%s\n", rule.Name, eventType, eventID, string(eventData))
	return nil
}

func (d *Dispatcher) executeWebhook(eventID, eventType string, eventData json.RawMessage, rule StoredRule) error {
	var cfg struct {
		URL                string            `json:"url"`
		Headers            map[string]string `json:"headers"`
		Secret             string            `json:"secret"` // HMAC secret (optional)
		PayloadTemplate    string            `json:"payload_template"`
		PayloadContentType string            `json:"payload_content_type"`
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

	var body []byte
	var contentType string

	if cfg.PayloadTemplate != "" {
		// Build data map for template resolution
		var data map[string]any
		if err := json.Unmarshal(eventData, &data); err != nil {
			log.Printf("[eventbus] webhook: failed to unmarshal event data for template resolution: %v", err)
			data = make(map[string]any)
		}
		shortType := eventType
		if idx := strings.LastIndex(eventType, "."); idx >= 0 {
			shortType = eventType[idx+1:]
		}
		data["event_type"] = shortType

		// Validate JSON if content type is application/json
		ct := cfg.PayloadContentType
		if ct == "" {
			ct = "application/json"
		}

		var rendered string
		if ct == "application/json" {
			// JSON-escape string values to prevent malformed JSON
			rendered = resolveJSONTemplateString(cfg.PayloadTemplate, data)
		} else {
			rendered = resolveTemplateString(cfg.PayloadTemplate, data)
		}

		if ct == "application/json" && !json.Valid([]byte(rendered)) {
			return fmt.Errorf("payload_template rendered invalid JSON")
		}
		body = []byte(rendered)
		contentType = ct
	} else {
		// CloudEvents-style payload (default)
		payload := map[string]any{
			"id":     eventID,
			"type":   eventType,
			"source": "c4.eventbus",
			"data":   json.RawMessage(eventData),
		}
		body, _ = json.Marshal(payload)
		contentType = "application/cloudevents+json"
		if cfg.PayloadContentType != "" {
			contentType = cfg.PayloadContentType
		}
	}

	req, err := http.NewRequest("POST", cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
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

func (d *Dispatcher) executeHubSubmit(eventType string, eventData json.RawMessage, rule StoredRule) error {
	d.mu.RLock()
	submitter := d.hubSubmitter
	d.mu.RUnlock()

	if submitter == nil {
		return fmt.Errorf("hub submitter not configured")
	}

	var cfg struct {
		Name        string            `json:"name"`
		Workdir     string            `json:"workdir"`
		Command     string            `json:"command"`
		Env         map[string]string `json:"env"`
		Tags        []string          `json:"tags"`
		RequiresGPU bool              `json:"requires_gpu"`
		Priority    int               `json:"priority"`
		ExpID       string            `json:"exp_id"`
		Memo        string            `json:"memo"`
		TimeoutSec  int               `json:"timeout_sec"`
	}
	if err := json.Unmarshal([]byte(rule.ActionConfig), &cfg); err != nil {
		return fmt.Errorf("parse hub_submit action_config: %w", err)
	}
	if cfg.Name == "" || cfg.Workdir == "" || cfg.Command == "" {
		return fmt.Errorf("hub_submit action_config requires name, workdir, and command")
	}

	var data map[string]any
	if err := json.Unmarshal(eventData, &data); err != nil {
		data = make(map[string]any)
	}
	if idx := strings.LastIndex(eventType, "."); idx >= 0 {
		data["event_type"] = eventType[idx+1:]
	} else {
		data["event_type"] = eventType
	}

	// Resolve template placeholders in string fields
	cfg.Name = resolveTemplateString(cfg.Name, data)
	cfg.Workdir = resolveTemplateString(cfg.Workdir, data)
	cfg.Command = resolveTemplateString(cfg.Command, data)
	cfg.ExpID = resolveTemplateString(cfg.ExpID, data)
	cfg.Memo = resolveTemplateString(cfg.Memo, data)

	spec := &JobSubmitSpec{
		Name:        cfg.Name,
		Workdir:     cfg.Workdir,
		Command:     cfg.Command,
		Env:         cfg.Env,
		Tags:        cfg.Tags,
		RequiresGPU: cfg.RequiresGPU,
		Priority:    cfg.Priority,
		ExpID:       cfg.ExpID,
		Memo:        cfg.Memo,
		TimeoutSec:  cfg.TimeoutSec,
	}
	_, err := submitter.Submit(spec)
	return err
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
			if len(pattern) > 256 {
				return false // prevent ReDoS with overly long patterns
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

// resolveTemplateString replaces {{data.key}} or {{nested.path}} with actual values.
// resolveJSONTemplateString resolves template placeholders like resolveTemplateString,
// but JSON-escapes string values so the result is safe to embed inside a JSON document.
// Non-string values (numbers, booleans) are formatted with %v which is already JSON-safe.
func resolveJSONTemplateString(s string, data map[string]any) string {
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
		key = strings.TrimPrefix(key, "data.")

		val := ""
		if v, ok := resolveNestedField(data, key); ok {
			if sv, isStr := v.(string); isStr {
				// JSON-escape string values (strip surrounding quotes produced by Marshal)
				b, _ := json.Marshal(sv)
				val = string(b[1 : len(b)-1])
			} else {
				val = fmt.Sprintf("%v", v)
			}
		}

		result = result[:start] + val + result[end+2:]
	}
	return result
}

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
		if v, ok := resolveNestedField(data, key); ok {
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
